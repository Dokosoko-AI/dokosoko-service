package platform

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type toolBuilderPostmanCollection struct {
	Info struct {
		Schema string `json:"schema"`
	} `json:"info"`
	Auth *toolBuilderPostmanAuth  `json:"auth"`
	Item []toolBuilderPostmanItem `json:"item"`
}

type toolBuilderPostmanItem struct {
	Name    string                   `json:"name"`
	Auth    *toolBuilderPostmanAuth  `json:"auth"`
	Request json.RawMessage          `json:"request"`
	Item    []toolBuilderPostmanItem `json:"item"`
}

type toolBuilderPostmanAuth struct {
	Type   string                        `json:"type"`
	APIKey []toolBuilderPostmanAuthValue `json:"apikey"`
	OAuth2 []toolBuilderPostmanAuthValue `json:"oauth2"`
	Basic  []toolBuilderPostmanAuthValue `json:"basic"`
}

type toolBuilderPostmanRequest struct {
	Method string                        `json:"method"`
	Header []struct{ Key, Value string } `json:"header"`
	URL    any                           `json:"url"`
	Body   struct {
		Mode string `json:"mode"`
		Raw  string `json:"raw"`
	} `json:"body"`
	Auth *toolBuilderPostmanAuth `json:"auth"`
}

type toolBuilderPostmanAuthValue struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

func postmanAuthString(values []toolBuilderPostmanAuthValue, names ...string) string {
	for _, value := range values {
		for _, name := range names {
			if strings.EqualFold(strings.TrimSpace(value.Key), name) {
				if text, ok := value.Value.(string); ok {
					return strings.TrimSpace(text)
				}
			}
		}
	}
	return ""
}

func postmanURL(value any) string {
	switch current := value.(type) {
	case string:
		return current
	case map[string]any:
		if raw, ok := current["raw"].(string); ok {
			return raw
		}
	}
	return ""
}

type toolBuilderPostmanOperation struct {
	Item          toolBuilderPostmanItem
	InheritedAuth *toolBuilderPostmanAuth
}

func inheritedPostmanAuth(parent, candidate *toolBuilderPostmanAuth) *toolBuilderPostmanAuth {
	if candidate == nil || strings.EqualFold(strings.TrimSpace(candidate.Type), "inherit") {
		return parent
	}
	return candidate
}

func collectPostmanItems(items []toolBuilderPostmanItem, target *[]toolBuilderPostmanOperation, inheritedAuth *toolBuilderPostmanAuth, depth int) error {
	if depth > 32 {
		return fmt.Errorf("%w: Postman folder nesting exceeds 32 levels", ErrToolBuilderInvalidInput)
	}
	for _, item := range items {
		effectiveAuth := inheritedPostmanAuth(inheritedAuth, item.Auth)
		if len(item.Request) > 0 {
			*target = append(*target, toolBuilderPostmanOperation{Item: item, InheritedAuth: effectiveAuth})
			if len(*target) > maxToolBuilderCandidates {
				return fmt.Errorf("%w: Postman collection contains more than %d requests", ErrToolBuilderInvalidInput, maxToolBuilderCandidates)
			}
		}
		if err := collectPostmanItems(item.Item, target, effectiveAuth, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func applyPostmanAuth(draft *ToolDraft, auth *toolBuilderPostmanAuth, curlDetected bool) bool {
	if auth == nil {
		if !curlDetected {
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "none"}
		}
		return false
	}
	switch strings.ToLower(strings.TrimSpace(auth.Type)) {
	case "bearer":
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "bearer"}
		return true
	case "oauth2":
		grantType := strings.ToLower(postmanAuthString(auth.OAuth2, "grant_type", "grantType"))
		tokenURL := postmanAuthString(auth.OAuth2, "accessTokenUrl", "access_token_url", "tokenUrl", "token_url")
		if (grantType == "client_credentials" || grantType == "clientcredentials") && tokenURL != "" {
			draft.UpstreamAuth = ToolUpstreamAuth{
				Type:     "oauth_client_credentials",
				ClientID: postmanAuthString(auth.OAuth2, "clientId", "client_id"),
				TokenURL: tokenURL,
				Scopes:   strings.Fields(postmanAuthString(auth.OAuth2, "scope")),
				Audience: postmanAuthString(auth.OAuth2, "audience"),
				Resource: postmanAuthString(auth.OAuth2, "resource"),
			}
		} else {
			// Non-client-credentials Postman OAuth normally represents one
			// workspace access token. Model it as an independently supplied
			// bearer credential without importing that token.
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "bearer"}
		}
		return true
	case "basic":
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "basic", Username: postmanAuthString(auth.Basic, "username")}
		return true
	case "apikey":
		name := postmanAuthString(auth.APIKey, "key")
		switch strings.ToLower(postmanAuthString(auth.APIKey, "in")) {
		case "query":
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "api_key_query", QueryName: name}
		case "header":
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "api_key_header", HeaderName: name}
		default:
			draft.UpstreamAuth = ToolUpstreamAuth{Type: "unsupported_postman_auth"}
		}
		return true
	case "noauth":
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "none"}
		return false
	default:
		// Digest, Hawk, AWS v4, NTLM, and any future unknown mode must be
		// reviewed explicitly instead of inheriting a weaker/default mode.
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "unsupported_postman_auth"}
		return true
	}
}

func buildToolDraftsFromPostman(base ToolDraft, raw string) ([]ToolDraft, bool, error) {
	var collection toolBuilderPostmanCollection
	if err := json.Unmarshal([]byte(raw), &collection); err != nil || !strings.Contains(collection.Info.Schema, "schema.getpostman.com") {
		return nil, false, fmt.Errorf("%w: source is not a Postman v2.1 collection", ErrToolBuilderInvalidInput)
	}
	items := make([]toolBuilderPostmanOperation, 0)
	if err := collectPostmanItems(collection.Item, &items, inheritedPostmanAuth(nil, collection.Auth), 0); err != nil {
		return nil, false, err
	}
	if len(items) == 0 || len(items) > maxToolBuilderCandidates {
		return nil, false, fmt.Errorf("%w: Postman collection must contain between 1 and %d requests", ErrToolBuilderInvalidInput, maxToolBuilderCandidates)
	}
	result := make([]ToolDraft, 0, len(items))
	credentialDetected := false
	for _, operation := range items {
		item := operation.Item
		var request toolBuilderPostmanRequest
		if err := json.Unmarshal(item.Request, &request); err != nil {
			continue
		}
		requestURL := postmanURL(request.URL)
		if requestURL == "" || strings.Contains(requestURL, "{{") {
			continue
		}
		var command strings.Builder
		command.WriteString("curl -X ")
		command.WriteString(strconv.Quote(strings.ToUpper(request.Method)))
		command.WriteByte(' ')
		command.WriteString(strconv.Quote(requestURL))
		for _, header := range request.Header {
			command.WriteString(" -H ")
			command.WriteString(strconv.Quote(header.Key + ": " + header.Value))
		}
		if request.Body.Mode == "raw" && request.Body.Raw != "" {
			command.WriteString(" --data-raw ")
			command.WriteString(strconv.Quote(request.Body.Raw))
		}
		draft, detected, err := buildToolDraftFromCurl(base, command.String())
		if err != nil {
			continue
		}
		effectiveAuth := inheritedPostmanAuth(operation.InheritedAuth, request.Auth)
		detected = applyPostmanAuth(&draft, effectiveAuth, detected) || detected
		draft.Name = toolBuilderIdentifier(item.Name, draft.Name)
		draft.Description = "Imported Postman operation."
		credentialDetected = credentialDetected || detected
		result = append(result, draft)
	}
	if len(result) == 0 {
		return nil, credentialDetected, fmt.Errorf("%w: Postman collection has no fixed absolute request URLs", ErrToolBuilderInvalidInput)
	}
	return result, credentialDetected, nil
}
