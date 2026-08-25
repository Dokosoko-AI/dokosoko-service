package platform

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func decodeToolTestStoredDraft(tool model.Tool) (ToolDraft, error) {
	draft := ToolDraft{
		Namespace: tool.Namespace, Name: tool.Name, Description: tool.Description, HTTPMethod: tool.HTTPMethod,
		Endpoint: tool.BaseURL, TimeoutMS: tool.TimeoutMS, InputSchema: append(json.RawMessage(nil), tool.InputSchema...),
		OutputSchema: append(json.RawMessage(nil), tool.OutputSchema...), CredentialPresent: tool.CredentialID != "",
	}
	if len(tool.UpstreamAuth) == 0 {
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "delegated_oauth"}
	} else if err := strictJSON(tool.UpstreamAuth, &draft.UpstreamAuth); err != nil {
		return ToolDraft{}, errors.New("stored upstream authentication is invalid")
	}
	if len(tool.RequestMapping) > 0 {
		if err := strictJSON(tool.RequestMapping, &draft.RequestMapping); err != nil {
			return ToolDraft{}, errors.New("stored request mapping is invalid")
		}
	}
	if draft.RequestMapping.ParameterLocations == nil {
		draft.RequestMapping.ParameterLocations = map[string]string{}
	}
	if len(tool.ResponseMapping) > 0 {
		if err := strictJSON(tool.ResponseMapping, &draft.ResponseMapping); err != nil {
			return ToolDraft{}, errors.New("stored response mapping is invalid")
		}
	}
	if err := strictJSON(tool.AuthorizationPolicy, &draft.AuthorizationPolicy); err != nil {
		return ToolDraft{}, errors.New("stored authorization policy is invalid")
	}
	if draft.AuthorizationPolicy.RequiredGrants == nil {
		draft.AuthorizationPolicy.RequiredGrants = []string{}
	}
	if len(tool.RequestExample) > 0 && string(tool.RequestExample) != "null" {
		if err := strictJSON(tool.RequestExample, &draft.RequestExample); err != nil {
			return ToolDraft{}, errors.New("stored request example is invalid")
		}
	}
	if len(tool.ResponseExample) > 0 && string(tool.ResponseExample) != "null" {
		if err := strictJSON(tool.ResponseExample, &draft.ResponseExample); err != nil {
			return ToolDraft{}, errors.New("stored response example is invalid")
		}
	}
	return draft, nil
}

func safeToolTestAnalysisLabel(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && toolTestAnalysisSafeName.MatchString(value) && !unsafeToolTestAnalysisText(value)
}

func safeToolTestAnalysisName(value string) bool {
	if !safeToolTestAnalysisLabel(value) {
		return false
	}
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(value))
	for _, credentialPattern := range []string{
		"authorization", "authentication", "bearer", "credential", "password", "passwd",
		"secret", "token", "apikey", "accesskey", "privatekey", "signingkey", "sessionkey", "cookie",
	} {
		if strings.Contains(normalized, credentialPattern) {
			return false
		}
	}
	return true
}

func safeToolTestSchemaType(value string) bool {
	switch value {
	case "null", "boolean", "object", "array", "number", "integer", "string":
		return true
	default:
		return false
	}
}

func toolTestMethodForAI(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}[value] {
		return value
	}
	return "UNKNOWN"
}

func toolTestAuthenticationTypeForAI(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if map[string]bool{
		"delegated_oauth": true, "none": true, "bearer": true, "authorization_scheme": true,
		"api_key_header": true, "api_key_query": true, "basic": true, "oauth_client_credentials": true, "custom_header": true,
	}[value] {
		return value
	}
	return "unknown"
}

func toolTestOutcomeForAI(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "success" || value == "failure" {
		return value
	}
	return "unknown"
}

func toolTestPhaseForAI(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if map[string]bool{
		"preflight": true, "auth": true, "token_exchange": true, "transport": true, "upstream_status": true,
		"json": true, "response_mapping": true, "output_schema": true, "request": true, "response": true, "success": true, "complete": true,
	}[value] {
		return value
	}
	return "unknown"
}

func toolTestFindingCodeForAI(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if map[string]bool{
		"rate_limited": true, "unsafe_destination": true, "configuration_invalid": true, "request_mapping_failed": true,
		"token_exchange_failed": true, "upstream_authentication_failed": true, "transport_failed": true, "upstream_status_rejected": true,
		"response_size_or_read_failed": true, "invalid_json_response": true, "response_mapping_failed": true, "output_schema_mismatch": true,
		"preflight_failed": true, "http_tool_required": true, "authentication_configuration_invalid": true, "input_schema_mismatch": true,
		"test_authorization_unavailable": true, "stored_tool_requires_review": true, "response_shape_observed": true,
	}[value] {
		return value
	}
	return ""
}

// toolTestSchemaForAI is deliberately a structural projection rather than a
// generic keyword blacklist. Stored/legacy schemas can contain arbitrary
// annotations and literal enums, so only type relationships, safe property
// names, and value-free literal-constraint markers cross the provider boundary.
// Titles, descriptions, examples, defaults, comments, const/enum literals,
// patterns, formats and references are omitted.
func toolTestSchemaForAI(raw json.RawMessage) json.RawMessage {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return append(json.RawMessage(nil), emptyToolBuilderSchema...)
	}

	var project func(any, int) (any, bool)
	project = func(current any, depth int) (any, bool) {
		if depth > 16 {
			return map[string]any{}, true
		}
		if boolean, ok := current.(bool); ok {
			return boolean, true
		}
		node, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		result := make(map[string]any)
		switch typed := node["type"].(type) {
		case string:
			if safeToolTestSchemaType(typed) {
				result["type"] = typed
			}
		case []any:
			types := make([]string, 0, len(typed))
			seen := map[string]bool{}
			for _, candidate := range typed {
				name, stringValue := candidate.(string)
				if stringValue && safeToolTestSchemaType(name) && !seen[name] {
					seen[name] = true
					types = append(types, name)
				}
			}
			if len(types) > 0 {
				result["type"] = types
			}
		}
		if enumValues, exists := node["enum"].([]any); exists && len(enumValues) > 0 {
			result["x-dokosoko-enum-value-count"] = len(enumValues)
		}
		if _, exists := node["const"]; exists {
			result["x-dokosoko-const-present"] = true
		}

		retainedProperties := map[string]bool{}
		if properties, ok := node["properties"].(map[string]any); ok {
			projected := make(map[string]any)
			for name, child := range properties {
				if !safeToolTestAnalysisName(name) {
					continue
				}
				if childProjection, valid := project(child, depth+1); valid {
					projected[name] = childProjection
					retainedProperties[name] = true
				}
			}
			result["properties"] = projected
		}
		if required, ok := node["required"].([]any); ok {
			projected := make([]string, 0, len(required))
			for _, child := range required {
				name, stringValue := child.(string)
				if stringValue && retainedProperties[name] {
					projected = append(projected, name)
				}
			}
			if len(projected) > 0 {
				result["required"] = projected
			}
		}
		if child, exists := node["items"]; exists {
			if childProjection, valid := project(child, depth+1); valid {
				result["items"] = childProjection
			}
		}
		if child, exists := node["additionalProperties"]; exists {
			switch typed := child.(type) {
			case bool:
				result["additionalProperties"] = typed
			case map[string]any:
				if childProjection, valid := project(typed, depth+1); valid {
					result["additionalProperties"] = childProjection
				}
			}
		}
		for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
			children, ok := node[keyword].([]any)
			if !ok {
				continue
			}
			projected := make([]any, 0, len(children))
			for _, child := range children {
				if childProjection, valid := project(child, depth+1); valid {
					projected = append(projected, childProjection)
				}
			}
			if len(projected) > 0 {
				result[keyword] = projected
			}
		}
		if child, exists := node["not"]; exists {
			if childProjection, valid := project(child, depth+1); valid {
				result["not"] = childProjection
			}
		}
		return result, true
	}

	projected, ok := project(value, 0)
	if !ok {
		return append(json.RawMessage(nil), emptyToolBuilderSchema...)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		return append(json.RawMessage(nil), emptyToolBuilderSchema...)
	}
	return encoded
}
