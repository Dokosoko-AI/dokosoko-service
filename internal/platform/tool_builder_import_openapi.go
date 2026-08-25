package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v4"
)

func toolBuilderMap(value any) map[string]any {
	if result, ok := value.(map[string]any); ok {
		return result
	}
	return nil
}

func toolBuilderString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func decodeToolBuilderDocument(raw string) (map[string]any, error) {
	var value any
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("%w: source is not valid JSON", ErrToolBuilderInvalidInput)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: source contains trailing JSON", ErrToolBuilderInvalidInput)
		}
	} else {
		if err := yaml.Unmarshal([]byte(raw), &value); err != nil {
			return nil, fmt.Errorf("%w: source is neither valid JSON nor YAML", ErrToolBuilderInvalidInput)
		}
	}
	if expanded, err := json.Marshal(value); err != nil || len(expanded) > 2<<20 {
		return nil, fmt.Errorf("%w: expanded document is too large", ErrToolBuilderInvalidInput)
	}
	root := toolBuilderMap(value)
	if root == nil {
		return nil, fmt.Errorf("%w: imported document must be an object", ErrToolBuilderInvalidInput)
	}
	return root, nil
}

func toolBuilderJSONPointer(root map[string]any, reference string) (any, error) {
	if !strings.HasPrefix(reference, "#/") {
		return nil, fmt.Errorf("external OpenAPI references are not supported")
	}
	var current any = root
	for _, raw := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
		part := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		object := toolBuilderMap(current)
		if object == nil {
			return nil, fmt.Errorf("OpenAPI reference is invalid")
		}
		var ok bool
		current, ok = object[part]
		if !ok {
			return nil, fmt.Errorf("OpenAPI reference is unresolved")
		}
	}
	return current, nil
}

func convertOpenAPISchema(root map[string]any, value any, seen map[string]bool, depth int) (map[string]any, error) {
	if depth > 10 {
		return nil, errors.New("OpenAPI schema nesting exceeds the supported limit")
	}
	node := toolBuilderMap(value)
	if node == nil {
		return nil, errors.New("OpenAPI schema must be an object")
	}
	if reference := toolBuilderString(node["$ref"]); reference != "" {
		if seen[reference] {
			return nil, errors.New("cyclic OpenAPI schema references are not supported")
		}
		resolved, err := toolBuilderJSONPointer(root, reference)
		if err != nil {
			return nil, err
		}
		next := make(map[string]bool, len(seen)+1)
		for key, present := range seen {
			next[key] = present
		}
		next[reference] = true
		return convertOpenAPISchema(root, resolved, next, depth+1)
	}
	typeName := toolBuilderString(node["type"])
	if typeName == "" {
		if node["properties"] != nil {
			typeName = "object"
		} else {
			typeName = "string"
		}
	}
	allowed := map[string]bool{"object": true, "array": true, "string": true, "number": true, "integer": true, "boolean": true, "null": true}
	if !allowed[typeName] {
		return nil, fmt.Errorf("OpenAPI schema type %q is not supported", typeName)
	}
	result := map[string]any{"type": typeName}
	if description := toolBuilderString(node["description"]); description != "" && len(description) <= 1000 && !containsToolBuilderSecretText(description) {
		result["description"] = description
	}
	if title := toolBuilderString(node["title"]); title != "" && len(title) <= 300 && !containsToolBuilderSecretText(title) {
		result["title"] = title
	}
	if enum, ok := node["enum"].([]any); ok && len(enum) > 0 && len(enum) <= 128 && !containsToolBuilderSecretValue(enum) {
		result["enum"] = enum
	}
	switch typeName {
	case "object":
		result["additionalProperties"] = false
		properties := map[string]any{}
		for name, child := range toolBuilderMap(node["properties"]) {
			if name == "" || len(name) > 100 || len(properties) >= 64 {
				continue
			}
			converted, err := convertOpenAPISchema(root, child, seen, depth+1)
			if err != nil {
				return nil, fmt.Errorf("property %s: %w", name, err)
			}
			properties[name] = converted
		}
		result["properties"] = properties
		if required, ok := node["required"].([]any); ok {
			values := make([]string, 0, len(required))
			known := map[string]bool{}
			for _, raw := range required {
				if name, ok := raw.(string); ok {
					if _, exists := properties[name]; exists && !known[name] {
						values = append(values, name)
						known[name] = true
					}
				}
			}
			if len(values) > 0 {
				result["required"] = values
			}
		}
	case "array":
		items, err := convertOpenAPISchema(root, node["items"], seen, depth+1)
		if err != nil {
			return nil, err
		}
		result["items"] = items
		for _, keyword := range []string{"minItems", "maxItems", "uniqueItems"} {
			if value, ok := node[keyword]; ok {
				result[keyword] = value
			}
		}
	case "string":
		for _, keyword := range []string{"minLength", "maxLength"} {
			if value, ok := node[keyword]; ok {
				result[keyword] = value
			}
		}
	case "number", "integer":
		for _, keyword := range []string{"minimum", "maximum"} {
			if value, ok := node[keyword]; ok {
				result[keyword] = value
			}
		}
	}
	return result, nil
}

func toolBuilderOpenAPIServer(root, pathItem, operation map[string]any, base ToolDraft) string {
	collections := []any{operation["servers"], pathItem["servers"], root["servers"]}
	for _, collection := range collections {
		servers, ok := collection.([]any)
		if !ok || len(servers) == 0 {
			continue
		}
		server := toolBuilderMap(servers[0])
		value := toolBuilderString(server["url"])
		if value == "" || strings.Contains(value, "{") {
			continue
		}
		if parsed, err := url.Parse(value); err == nil && parsed.IsAbs() {
			clean, _ := sanitizeToolBuilderEndpoint(value)
			return strings.TrimRight(clean, "/")
		}
		if baseURL, err := url.Parse(base.Endpoint); err == nil && baseURL.Host != "" {
			origin := baseURL.Scheme + "://" + baseURL.Host
			if resolved, err := url.Parse(origin + "/"); err == nil {
				if relative, err := url.Parse(value); err == nil {
					return strings.TrimRight(resolved.ResolveReference(relative).String(), "/")
				}
			}
		}
	}
	if parsed, err := url.Parse(base.Endpoint); err == nil && parsed.Host != "" {
		return parsed.Scheme + "://" + parsed.Host
	}
	return ""
}

func mergeOpenAPIParameters(root map[string]any, raw any, properties map[string]any, required map[string]bool, locations map[string]string) error {
	parameters, _ := raw.([]any)
	for _, candidate := range parameters {
		parameter := toolBuilderMap(candidate)
		if reference := toolBuilderString(parameter["$ref"]); reference != "" {
			resolved, err := toolBuilderJSONPointer(root, reference)
			if err != nil {
				return err
			}
			parameter = toolBuilderMap(resolved)
		}
		name, location := toolBuilderString(parameter["name"]), strings.ToLower(toolBuilderString(parameter["in"]))
		if name == "" || !map[string]bool{"path": true, "query": true, "header": true}[location] {
			continue
		}
		key := name
		if location == "header" {
			key = toolBuilderParameterName(name)
		}
		schema, err := convertOpenAPISchema(root, parameter["schema"], map[string]bool{}, 0)
		if err != nil {
			return err
		}
		properties[key], locations[key] = schema, location
		if requiredValue, _ := parameter["required"].(bool); requiredValue || location == "path" {
			required[key] = true
		}
	}
	return nil
}

func openAPIResponseSchema(root, operation map[string]any) map[string]any {
	responses := toolBuilderMap(operation["responses"])
	keys := make([]string, 0, len(responses))
	for key := range responses {
		if strings.HasPrefix(key, "2") || key == "default" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		response := toolBuilderMap(responses[key])
		if reference := toolBuilderString(response["$ref"]); reference != "" {
			if resolved, err := toolBuilderJSONPointer(root, reference); err == nil {
				response = toolBuilderMap(resolved)
			}
		}
		content := toolBuilderMap(response["content"])
		for _, mediaType := range []string{"application/json", "application/problem+json"} {
			media := toolBuilderMap(content[mediaType])
			if converted, err := convertOpenAPISchema(root, media["schema"], map[string]bool{}, 0); err == nil {
				return converted
			}
		}
	}
	return toolBuilderSchemaObject()
}

func openAPIRequirementScopes(requirement map[string]any, schemeName string) ([]string, bool) {
	raw, ok := requirement[schemeName]
	if !ok {
		return nil, false
	}
	values := make([]any, 0)
	switch current := raw.(type) {
	case []any:
		values = current
	case []string:
		for _, value := range current {
			values = append(values, value)
		}
	default:
		return nil, false
	}
	seen := make(map[string]bool, len(values))
	scopes := make([]string, 0, len(values))
	for _, value := range values {
		scope, ok := value.(string)
		scope = strings.TrimSpace(scope)
		if !ok || scope == "" {
			return nil, false
		}
		if !seen[scope] {
			seen[scope] = true
			scopes = append(scopes, scope)
		}
	}
	sort.Strings(scopes)
	return scopes, true
}

func applyOpenAPIAuth(root, operation map[string]any, draft *ToolDraft) {
	security, operationDefinesSecurity := operation["security"]
	if !operationDefinesSecurity {
		var rootDefinesSecurity bool
		security, rootDefinesSecurity = root["security"]
		if !rootDefinesSecurity {
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "none"}
			return
		}
	}
	items, validSecurity := security.([]any)
	if !validSecurity {
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "unsupported_openapi_security"}
		return
	}
	if len(items) == 0 {
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "none"}
		return
	}
	// This tool contract supports exactly one upstream authentication mode.
	// OpenAPI security arrays are alternatives (OR), while multiple names in
	// one requirement are cumulative (AND); silently choosing either would
	// generate a tool with weaker or simply wrong authentication semantics.
	if len(items) != 1 {
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "unsupported_openapi_security"}
		return
	}
	requirement := toolBuilderMap(items[0])
	names := make([]string, 0, len(requirement))
	for name := range requirement {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "none"}
		return
	}
	if len(names) != 1 {
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "unsupported_openapi_security"}
		return
	}
	schemes := toolBuilderMap(toolBuilderMap(root["components"])["securitySchemes"])
	scheme := toolBuilderMap(schemes[names[0]])
	if reference := toolBuilderString(scheme["$ref"]); reference != "" {
		if resolved, err := toolBuilderJSONPointer(root, reference); err == nil {
			scheme = toolBuilderMap(resolved)
		}
	}
	switch strings.ToLower(toolBuilderString(scheme["type"])) {
	case "apikey":
		name, location := toolBuilderString(scheme["name"]), strings.ToLower(toolBuilderString(scheme["in"]))
		if name == "" {
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "unsupported_openapi_security"}
		} else if location == "query" {
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "api_key_query", QueryName: name}
		} else if location == "header" {
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "api_key_header", HeaderName: name}
		} else {
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "unsupported_openapi_security"}
		}
	case "http":
		httpScheme := toolBuilderString(scheme["scheme"])
		if strings.EqualFold(httpScheme, "basic") {
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "basic"}
		} else if strings.EqualFold(httpScheme, "bearer") {
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "bearer"}
		} else {
			// Challenge-response schemes such as Digest cannot be represented
			// by a static credential prefix, and unknown schemes are ambiguous.
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "unsupported_openapi_security"}
		}
	case "oauth2":
		flows := toolBuilderMap(scheme["flows"])
		flowNames := make([]string, 0, 4)
		for _, name := range []string{"authorizationCode", "clientCredentials", "implicit", "password"} {
			if _, ok := flows[name]; ok {
				flowNames = append(flowNames, name)
			}
		}
		selectedScopes, scopesValid := openAPIRequirementScopes(requirement, names[0])
		if len(flowNames) != 1 || !scopesValid {
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "unsupported_openapi_security"}
			return
		}
		flow := toolBuilderMap(flows[flowNames[0]])
		catalog := toolBuilderMap(flow["scopes"])
		for _, scope := range selectedScopes {
			if _, ok := catalog[scope]; !ok {
				draft.UpstreamAuth = ToolUpstreamAuth{Type: "unsupported_openapi_security"}
				return
			}
		}
		switch flowNames[0] {
		case "clientCredentials":
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "oauth_client_credentials", TokenURL: toolBuilderString(flow["tokenUrl"]), Scopes: selectedScopes}
		case "authorizationCode", "implicit":
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "delegated_oauth"}
		default:
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "unsupported_openapi_security"}
		}
	case "openidconnect":
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "delegated_oauth"}
	default:
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "unsupported_openapi_security"}
	}
}

func buildToolDraftsFromOpenAPI(base ToolDraft, raw string) ([]ToolDraft, error) {
	root, err := decodeToolBuilderDocument(raw)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(toolBuilderString(root["openapi"]), "3.") {
		return nil, fmt.Errorf("%w: document must use OpenAPI 3.x", ErrToolBuilderInvalidInput)
	}
	paths := toolBuilderMap(root["paths"])
	if len(paths) == 0 {
		return nil, fmt.Errorf("%w: OpenAPI document has no paths", ErrToolBuilderInvalidInput)
	}
	pathNames := make([]string, 0, len(paths))
	for name := range paths {
		pathNames = append(pathNames, name)
	}
	sort.Strings(pathNames)
	methods := []string{"get", "post", "put", "patch", "delete"}
	result := make([]ToolDraft, 0)
	for _, pathName := range pathNames {
		pathItem := toolBuilderMap(paths[pathName])
		for _, method := range methods {
			operation := toolBuilderMap(pathItem[method])
			if operation == nil {
				continue
			}
			if len(result) >= maxToolBuilderCandidates {
				return nil, fmt.Errorf("%w: OpenAPI document contains more than %d supported operations", ErrToolBuilderInvalidInput, maxToolBuilderCandidates)
			}
			draft := cloneToolBuilderDraft(base)
			if draft.Namespace == "" {
				draft.Namespace = "api"
			}
			draft.HTTPMethod = strings.ToUpper(method)
			server := toolBuilderOpenAPIServer(root, pathItem, operation, base)
			draft.Endpoint = strings.TrimRight(server, "/") + "/" + strings.TrimLeft(pathName, "/")
			operationID := toolBuilderString(operation["operationId"])
			if operationID == "" {
				operationID = method + "_" + strings.Trim(pathName, "/")
			}
			draft.Name = toolBuilderIdentifier(operationID, "operation")
			description := toolBuilderString(operation["description"])
			if description == "" {
				description = toolBuilderString(operation["summary"])
			}
			if description == "" {
				description = "Imported OpenAPI operation."
			}
			draft.Description = description
			properties, required, locations := map[string]any{}, map[string]bool{}, map[string]string{}
			if err := mergeOpenAPIParameters(root, pathItem["parameters"], properties, required, locations); err != nil {
				return nil, err
			}
			if err := mergeOpenAPIParameters(root, operation["parameters"], properties, required, locations); err != nil {
				return nil, err
			}
			if requestBody := toolBuilderMap(operation["requestBody"]); requestBody != nil {
				if reference := toolBuilderString(requestBody["$ref"]); reference != "" {
					if resolved, resolveErr := toolBuilderJSONPointer(root, reference); resolveErr == nil {
						requestBody = toolBuilderMap(resolved)
					}
				}
				media := toolBuilderMap(toolBuilderMap(requestBody["content"])["application/json"])
				if bodySchema, schemaErr := convertOpenAPISchema(root, media["schema"], map[string]bool{}, 0); schemaErr == nil && bodySchema["type"] == "object" {
					for name, child := range toolBuilderMap(bodySchema["properties"]) {
						properties[name], locations[name] = child, "body"
					}
					if bodyRequired, ok := bodySchema["required"].([]string); ok {
						for _, name := range bodyRequired {
							required[name] = true
						}
					}
					if bodyRequired, ok := bodySchema["required"].([]any); ok {
						for _, raw := range bodyRequired {
							if name, ok := raw.(string); ok {
								required[name] = true
							}
						}
					}
				}
			}
			inputSchema := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
			requiredNames := make([]string, 0, len(required))
			for name := range required {
				requiredNames = append(requiredNames, name)
			}
			sort.Strings(requiredNames)
			if len(requiredNames) > 0 {
				inputSchema["required"] = requiredNames
			}
			draft.InputSchema, draft.OutputSchema = encodeToolBuilderSchema(inputSchema), encodeToolBuilderSchema(openAPIResponseSchema(root, operation))
			draft.RequestMapping, draft.ResponseMapping = ToolRequestMapping{ParameterLocations: locations}, ToolResponseMapping{}
			draft.RequestExample, draft.ResponseExample, draft.CredentialPresent = nil, nil, false
			applyOpenAPIAuth(root, operation, &draft)
			result = append(result, draft)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%w: OpenAPI document has no supported HTTP operations", ErrToolBuilderInvalidInput)
	}
	return result, nil
}
