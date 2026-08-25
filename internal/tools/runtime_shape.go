package tools

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

const (
	maxTestShapeDepth      = 5
	maxTestShapeKeys       = 64
	maxTestShapeArrayItems = 8
	maxTestShapeKeyBytes   = 128
)

func jsonShape(value any) model.JSONShape {
	remaining := maxTestShapeKeys
	return jsonShapeBounded(value, nil, 0, &remaining)
}

func SanitizedJSONShape(value any) model.JSONShape { return jsonShape(value) }

func jsonShapeForSchema(value any, rawSchema json.RawMessage) model.JSONShape {
	if ValidateSchema(rawSchema) != nil {
		return jsonShape(value)
	}
	var schema map[string]any
	if json.Unmarshal(rawSchema, &schema) != nil {
		return jsonShape(value)
	}
	remaining := maxTestShapeKeys
	return jsonShapeBounded(value, schema, 0, &remaining)
}

func safeUnexpectedShapeKey(properties map[string]any, existing map[string]model.JSONShape, index int) string {
	for {
		candidate := fmt.Sprintf("[unexpected-property-%d]", index)
		_, declared := properties[candidate]
		_, used := existing[candidate]
		if !declared && !used {
			return candidate
		}
		index++
	}
}

func safeDeclaredShapeKey(value string) bool {
	if len(value) == 0 || len(value) > maxTestShapeKeyBytes {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		if index == 0 {
			if !letter {
				return false
			}
			continue
		}
		if !letter && !(character >= '0' && character <= '9') && character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	normalized := strings.ToLower(value)
	normalized = strings.NewReplacer("_", "", "-", "", ".", "").Replace(normalized)
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

func safeSchemaShapeKey(existing map[string]model.JSONShape, index int) string {
	for {
		candidate := fmt.Sprintf("[schema-property-%d]", index)
		if _, used := existing[candidate]; !used {
			return candidate
		}
		index++
	}
}

func jsonShapeBounded(value any, schema map[string]any, depth int, remaining *int) model.JSONShape {
	if depth >= maxTestShapeDepth || *remaining <= 0 {
		return model.JSONShape{Type: jsonType(value), Truncated: true}
	}
	*remaining--
	switch current := value.(type) {
	case map[string]any:
		result := model.JSONShape{Type: "object", Properties: make(map[string]model.JSONShape)}
		properties, _ := schema["properties"].(map[string]any)
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		unexpectedIndex := 1
		schemaIndex := 1
		for _, key := range keys {
			if *remaining <= 0 {
				result.Truncated = true
				break
			}
			childSchema, declared := properties[key].(map[string]any)
			evidenceKey := key
			if !declared {
				evidenceKey = safeUnexpectedShapeKey(properties, result.Properties, unexpectedIndex)
				unexpectedIndex++
			} else if !safeDeclaredShapeKey(evidenceKey) {
				evidenceKey = safeSchemaShapeKey(result.Properties, schemaIndex)
				schemaIndex++
				result.Truncated = true
			}
			result.Properties[evidenceKey] = jsonShapeBounded(current[key], childSchema, depth+1, remaining)
		}
		if len(result.Properties) == 0 {
			result.Properties = nil
		}
		return result
	case []any:
		result := model.JSONShape{Type: "array", Length: len(current)}
		itemSchema, _ := schema["items"].(map[string]any)
		limit := len(current)
		if limit > maxTestShapeArrayItems {
			limit = maxTestShapeArrayItems
			result.Truncated = true
		}
		for index := 0; index < limit; index++ {
			result.Items = append(result.Items, jsonShapeBounded(current[index], itemSchema, depth+1, remaining))
		}
		return result
	default:
		return model.JSONShape{Type: jsonType(value)}
	}
}

func jsonType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "number"
	default:
		return "unknown"
	}
}
