package platform

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

func cloneToolBuilderDraft(value ToolDraft) ToolDraft {
	encoded, _ := json.Marshal(value)
	var cloned ToolDraft
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}

func toolBuilderIdentifier(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastUnderscore := false
	for _, char := range value {
		if builder.Len() >= 64 {
			break
		}
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' {
			builder.WriteRune(char)
			lastUnderscore = char == '_'
		} else if builder.Len() > 0 && !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	value = strings.Trim(builder.String(), "_")
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		value = fallback
	}
	if len(value) > 64 {
		value = strings.TrimRight(value[:64], "_")
	}
	return value
}

func toolBuilderParameterName(value string) string {
	return toolBuilderIdentifier(strings.ReplaceAll(value, "-", "_"), "parameter")
}

func inferredToolBuilderSchema(value any, depth int) map[string]any {
	if depth > 8 {
		return map[string]any{"type": "string"}
	}
	switch current := value.(type) {
	case map[string]any:
		properties := make(map[string]any, len(current))
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		required := make([]string, 0, len(keys))
		for _, key := range keys {
			if key == "" || len(key) > 100 {
				continue
			}
			properties[key] = inferredToolBuilderSchema(current[key], depth+1)
			required = append(required, key)
		}
		schema := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	case []any:
		item := any("")
		if len(current) > 0 {
			item = current[0]
		}
		return map[string]any{"type": "array", "items": inferredToolBuilderSchema(item, depth+1)}
	case bool:
		return map[string]any{"type": "boolean"}
	case float64, float32:
		return map[string]any{"type": "number"}
	case json.Number:
		if _, err := strconv.ParseInt(string(current), 10, 64); err == nil {
			return map[string]any{"type": "integer"}
		}
		return map[string]any{"type": "number"}
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return map[string]any{"type": "integer"}
	case nil:
		return map[string]any{"type": "null"}
	default:
		return map[string]any{"type": "string"}
	}
}

func encodeToolBuilderSchema(schema map[string]any) json.RawMessage {
	encoded, err := json.Marshal(schema)
	if err != nil || len(encoded) > 64<<10 {
		return append(json.RawMessage(nil), emptyToolBuilderSchema...)
	}
	return encoded
}

func detectToolBuilderImportKind(kind, raw string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "curl", "postman", "postman_collection", "openapi", "openapi_json", "openapi_yaml", "openapi_document", "openapi_url":
		return kind
	}
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "curl ") || strings.Contains(trimmed, "/curl ") {
		return "curl"
	}
	if strings.Contains(trimmed, "schema.getpostman.com") {
		return "postman"
	}
	return "openapi_document"
}
