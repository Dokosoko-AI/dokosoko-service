package platform

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"
)

func toolBuilderShellWords(raw string) ([]string, error) {
	if !utf8.ValidString(raw) {
		return nil, fmt.Errorf("%w: cURL text is not valid UTF-8", ErrToolBuilderInvalidInput)
	}
	words := make([]string, 0)
	var current strings.Builder
	quote := rune(0)
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	for _, char := range raw {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if char == ' ' || char == '\t' || char == '\r' || char == '\n' {
			flush()
			continue
		}
		current.WriteRune(char)
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("%w: cURL quoting is incomplete", ErrToolBuilderInvalidInput)
	}
	flush()
	return words, nil
}

type toolBuilderCurl struct {
	Method             string
	URL                string
	Headers            []string
	Body               string
	Basic              string
	OAuthBearerPresent bool
	CredentialDetected bool
}

func parseToolBuilderCurlCommand(raw string) (toolBuilderCurl, error) {
	words, err := toolBuilderShellWords(raw)
	if err != nil {
		return toolBuilderCurl{}, err
	}
	if len(words) == 0 || (words[0] != "curl" && !strings.HasSuffix(words[0], "/curl")) {
		return toolBuilderCurl{}, fmt.Errorf("%w: source must be a cURL command", ErrToolBuilderInvalidInput)
	}
	parsed := toolBuilderCurl{Method: "GET"}
	take := func(index *int, inlineValue string, hasInlineValue bool) (string, error) {
		if hasInlineValue {
			return inlineValue, nil
		}
		*index++
		if *index >= len(words) {
			return "", fmt.Errorf("%w: cURL option is missing its value", ErrToolBuilderInvalidInput)
		}
		value := words[*index]
		// A flag-looking token is ambiguous here: treating it as a value could
		// hide an unsupported authentication or request option. Callers can use
		// the unambiguous --option=-value form for a literal leading hyphen.
		if strings.HasPrefix(value, "-") {
			return "", fmt.Errorf("%w: cURL option is missing an unambiguous value", ErrToolBuilderInvalidInput)
		}
		return value, nil
	}
	bodySeen := false
	for index := 1; index < len(words); index++ {
		word, inlineValue, hasInlineValue := words[index], "", false
		if strings.HasPrefix(word, "--") {
			word, inlineValue, hasInlineValue = strings.Cut(word, "=")
		}
		if unsupportedToolBuilderCurlAuthOption(word) {
			// Do not include the original token in this error. An attached value
			// can itself be a credential (for example --proxy-user=user:secret).
			return toolBuilderCurl{}, fmt.Errorf("%w: cURL authentication or signing option is not supported by the safe importer", ErrToolBuilderInvalidInput)
		}
		if unsupportedToolBuilderCurlRequestOption(word) {
			return toolBuilderCurl{}, fmt.Errorf("%w: cURL request option is not supported by the safe importer", ErrToolBuilderInvalidInput)
		}
		switch word {
		case "-X", "--request":
			value, valueErr := take(&index, inlineValue, hasInlineValue)
			if valueErr != nil {
				return toolBuilderCurl{}, valueErr
			}
			if strings.TrimSpace(value) == "" {
				return toolBuilderCurl{}, fmt.Errorf("%w: cURL request method is empty", ErrToolBuilderInvalidInput)
			}
			parsed.Method = strings.ToUpper(value)
		case "-H", "--header":
			value, valueErr := take(&index, inlineValue, hasInlineValue)
			if valueErr != nil {
				return toolBuilderCurl{}, valueErr
			}
			headerParts := strings.SplitN(value, ":", 2)
			if len(headerParts) != 2 || !validHTTPHeaderName(strings.TrimSpace(headerParts[0])) || strings.ContainsAny(headerParts[1], "\r\n") || strings.HasPrefix(value, "@") {
				return toolBuilderCurl{}, fmt.Errorf("%w: cURL header must be an inline name and value", ErrToolBuilderInvalidInput)
			}
			parsed.Headers = append(parsed.Headers, value)
		case "-d", "--data", "--data-ascii", "--data-raw", "--data-binary", "--data-urlencode", "--json":
			value, valueErr := take(&index, inlineValue, hasInlineValue)
			if valueErr != nil {
				return toolBuilderCurl{}, valueErr
			}
			fileBacked := word != "--data-raw" && strings.HasPrefix(value, "@")
			if word == "--data-urlencode" && !strings.Contains(value, "=") && strings.Contains(value, "@") {
				fileBacked = true
			}
			if fileBacked {
				return toolBuilderCurl{}, fmt.Errorf("%w: file-backed cURL bodies are not supported", ErrToolBuilderInvalidInput)
			}
			if bodySeen {
				return toolBuilderCurl{}, fmt.Errorf("%w: multiple cURL body options cannot be represented safely", ErrToolBuilderInvalidInput)
			}
			bodySeen = true
			parsed.Body = value
			if parsed.Method == "GET" {
				parsed.Method = "POST"
			}
		case "--url":
			value, valueErr := take(&index, inlineValue, hasInlineValue)
			if valueErr != nil {
				return toolBuilderCurl{}, valueErr
			}
			if strings.TrimSpace(value) == "" || parsed.URL != "" {
				return toolBuilderCurl{}, fmt.Errorf("%w: cURL command must contain exactly one URL", ErrToolBuilderInvalidInput)
			}
			parsed.URL = value
		case "-u", "--user":
			value, valueErr := take(&index, inlineValue, hasInlineValue)
			if valueErr != nil {
				return toolBuilderCurl{}, valueErr
			}
			if value == "" {
				return toolBuilderCurl{}, fmt.Errorf("%w: cURL user credential is empty", ErrToolBuilderInvalidInput)
			}
			parsed.Basic, parsed.CredentialDetected = value, true
		case "--oauth2-bearer":
			value, valueErr := take(&index, inlineValue, hasInlineValue)
			if valueErr != nil {
				return toolBuilderCurl{}, valueErr
			}
			if value == "" {
				return toolBuilderCurl{}, fmt.Errorf("%w: cURL OAuth bearer credential is empty", ErrToolBuilderInvalidInput)
			}
			parsed.OAuthBearerPresent, parsed.CredentialDetected = true, true
		case "-s", "--silent", "-S", "--show-error", "-f", "--fail", "--fail-with-body", "-i", "--include", "-v", "--verbose", "-g", "--globoff", "--compressed", "--no-progress-meter", "--basic":
			// These flags only change cURL output presentation, compression, or
			// explicitly select the already-supported Basic authentication mode.
			if hasInlineValue {
				return toolBuilderCurl{}, fmt.Errorf("%w: valueless cURL option was given a value", ErrToolBuilderInvalidInput)
			}
		default:
			if strings.HasPrefix(word, "-") {
				return toolBuilderCurl{}, fmt.Errorf("%w: cURL option is not supported by the safe importer", ErrToolBuilderInvalidInput)
			}
			if hasInlineValue || parsed.URL != "" {
				return toolBuilderCurl{}, fmt.Errorf("%w: cURL command must contain exactly one URL", ErrToolBuilderInvalidInput)
			}
			parsed.URL = word
		}
	}
	if parsed.URL == "" {
		return toolBuilderCurl{}, fmt.Errorf("%w: cURL command has no URL", ErrToolBuilderInvalidInput)
	}
	return parsed, nil
}

func unsupportedToolBuilderCurlAuthOption(option string) bool {
	switch option {
	case "--anyauth", "--aws-sigv4", "--cert", "--delegation", "--digest", "--key", "--login-options", "--negotiate", "--netrc", "--netrc-file", "--netrc-optional", "--ntlm", "--ntlm-wb", "--pass", "--proxy-anyauth", "--proxy-basic", "--proxy-cert", "--proxy-digest", "--proxy-key", "--proxy-negotiate", "--proxy-ntlm", "--proxy-pass", "--proxy-user", "-U", "--service-name", "--socks5-gssapi-service", "--tlsauthtype", "--tlspassword", "--tlsuser":
		return true
	default:
		// Future proxy authentication variants must not silently become a
		// direct, unauthenticated tool contract.
		return strings.HasPrefix(option, "--proxy-")
	}
}

func unsupportedToolBuilderCurlRequestOption(option string) bool {
	switch option {
	case "--config", "-K", "--proxy", "-x", "--resolve", "--connect-to", "--form", "-F", "--get", "-G", "--head", "-I", "--location", "-L", "--request-target", "--upload-file", "-T", "--url-query":
		return true
	default:
		return false
	}
}

func toolBuilderSchemaObject() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}
}

func mergeToolBuilderProperties(target map[string]any, source map[string]any) {
	properties, _ := target["properties"].(map[string]any)
	children, _ := source["properties"].(map[string]any)
	for name, schema := range children {
		if _, exists := properties[name]; !exists {
			properties[name] = schema
		}
	}
	requiredSet := map[string]bool{}
	for _, raw := range target["required"].([]any) {
		if value, ok := raw.(string); ok {
			requiredSet[value] = true
		}
	}
	if sourceRequired, ok := source["required"].([]any); ok {
		for _, raw := range sourceRequired {
			if value, ok := raw.(string); ok {
				requiredSet[value] = true
			}
		}
	}
	if sourceRequired, ok := source["required"].([]string); ok {
		for _, value := range sourceRequired {
			requiredSet[value] = true
		}
	}
	required := make([]string, 0, len(requiredSet))
	for value := range requiredSet {
		required = append(required, value)
	}
	sort.Strings(required)
	if len(required) > 0 {
		target["required"] = required
	}
}

func buildToolDraftFromCurl(base ToolDraft, raw string) (ToolDraft, bool, error) {
	command, err := parseToolBuilderCurlCommand(raw)
	if err != nil {
		return ToolDraft{}, false, err
	}
	draft := cloneToolBuilderDraft(base)
	if draft.Namespace == "" {
		draft.Namespace = "api"
	}
	draft.HTTPMethod = command.Method
	parsed, err := url.Parse(command.URL)
	if err != nil || parsed.Host == "" {
		return ToolDraft{}, false, fmt.Errorf("%w: cURL URL must be absolute", ErrToolBuilderInvalidInput)
	}
	input := toolBuilderSchemaObject()
	input["required"] = []any{}
	properties := input["properties"].(map[string]any)
	mapping := map[string]string{}
	required := map[string]bool{}
	if parsed.User != nil {
		command.CredentialDetected = true
	}
	for name := range parsed.Query() {
		// The mapping contract uses the input-property name as the literal
		// upstream query name. Normalizing `user-id` to `user_id` here would
		// produce a valid-looking tool that calls a different API contract.
		if !toolQueryNamePattern.MatchString(name) {
			return ToolDraft{}, false, fmt.Errorf("%w: query parameter %q cannot be represented safely", ErrToolBuilderInvalidInput, name)
		}
		key := name
		lower := strings.ToLower(name)
		if strings.Contains(lower, "key") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") {
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "api_key_query", QueryName: name}
			command.CredentialDetected = true
			continue
		}
		properties[key] = map[string]any{"type": "string"}
		mapping[key] = "query"
		required[key] = true
	}
	draft.Endpoint, _ = sanitizeToolBuilderEndpoint(command.URL)
	for _, match := range toolBuilderPlaceholderPattern.FindAllStringSubmatch(parsed.Path, -1) {
		properties[match[1]] = map[string]any{"type": "string"}
		mapping[match[1]] = "path"
		required[match[1]] = true
	}
	for _, header := range command.Headers {
		parts := strings.SplitN(header, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		lower := strings.ToLower(name)
		if lower == "authorization" {
			command.CredentialDetected = command.CredentialDetected || value != ""
			parts := strings.Fields(value)
			if strings.EqualFold(firstToolBuilderValue(parts), "basic") {
				draft.UpstreamAuth.Type = "basic"
			} else if strings.EqualFold(firstToolBuilderValue(parts), "bearer") || len(parts) == 0 {
				draft.UpstreamAuth.Type = "bearer"
			} else if validAuthorizationScheme(parts[0]) {
				draft.UpstreamAuth = ToolUpstreamAuth{Type: "authorization_scheme", Scheme: parts[0]}
			} else {
				draft.UpstreamAuth.Type = "bearer"
			}
			continue
		}
		if strings.Contains(lower, "api-key") || strings.Contains(lower, "apikey") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") {
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "api_key_header", HeaderName: name}
			command.CredentialDetected = command.CredentialDetected || value != ""
			continue
		}
		if lower == "content-type" || lower == "accept" || lower == "user-agent" {
			continue
		}
		key := toolBuilderParameterName(name)
		properties[key] = map[string]any{"type": "string"}
		mapping[key] = "header"
	}
	if command.Basic != "" {
		username, _, _ := strings.Cut(command.Basic, ":")
		if containsToolBuilderSecretText(username) {
			username = ""
		}
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "basic", Username: username}
	}
	if command.OAuthBearerPresent {
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "bearer"}
	}
	if command.Body != "" {
		decoder := json.NewDecoder(strings.NewReader(command.Body))
		decoder.UseNumber()
		var body any
		if decoder.Decode(&body) == nil {
			if object, ok := body.(map[string]any); ok {
				bodySchema := inferredToolBuilderSchema(object, 0)
				mergeToolBuilderProperties(input, bodySchema)
				for name := range object {
					mapping[name] = "body"
					required[name] = true
				}
			}
		}
	}
	requiredNames := make([]string, 0, len(required))
	for name := range required {
		requiredNames = append(requiredNames, name)
	}
	sort.Strings(requiredNames)
	if len(requiredNames) > 0 {
		input["required"] = requiredNames
	} else {
		delete(input, "required")
	}
	draft.InputSchema = encodeToolBuilderSchema(input)
	draft.OutputSchema = append(json.RawMessage(nil), emptyToolBuilderSchema...)
	draft.RequestMapping = ToolRequestMapping{ParameterLocations: mapping}
	draft.ResponseMapping = ToolResponseMapping{}
	draft.RequestExample, draft.ResponseExample = nil, nil
	draft.CredentialPresent = false
	if draft.Name == "" {
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		candidate := "request"
		if len(segments) > 0 && segments[len(segments)-1] != "" {
			candidate = segments[len(segments)-1]
		}
		draft.Name = toolBuilderIdentifier(strings.ToLower(command.Method)+"_"+candidate, "request")
	}
	if draft.Description == "" {
		draft.Description = "Imported HTTP operation."
	}
	return draft, command.CredentialDetected, nil
}

func firstToolBuilderValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
