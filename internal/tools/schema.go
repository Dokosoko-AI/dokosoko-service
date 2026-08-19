package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func ValidateSchema(raw json.RawMessage) error {
	if len(raw) == 0 || len(raw) > 64<<10 {
		return errors.New("schema must be present and no larger than 64 KiB")
	}
	var schema map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&schema); err != nil {
		return fmt.Errorf("invalid JSON schema: %w", err)
	}
	if schema["type"] != "object" {
		return errors.New("tool schema root type must be object")
	}
	if additional, ok := schema["additionalProperties"]; !ok || additional != false {
		return errors.New("tool schema must set additionalProperties to false")
	}
	return validateNode(schema, 0)
}

func validateNode(node map[string]any, depth int) error {
	if depth > 10 {
		return errors.New("schema nesting exceeds 10 levels")
	}
	if reference, ok := node["$ref"].(string); ok && (strings.Contains(reference, "://") || !strings.HasPrefix(reference, "#/$defs/")) {
		return errors.New("remote or unsupported schema references are forbidden")
	}
	typeName, _ := node["type"].(string)
	allowed := map[string]bool{"object": true, "array": true, "string": true, "number": true, "integer": true, "boolean": true, "null": true}
	if typeName != "" && !allowed[typeName] {
		return fmt.Errorf("unsupported schema type %q", typeName)
	}
	if properties, ok := node["properties"].(map[string]any); ok {
		if len(properties) > 64 {
			return errors.New("schema has more than 64 properties")
		}
		for name, raw := range properties {
			property, ok := raw.(map[string]any)
			if !ok || name == "" || len(name) > 100 {
				return errors.New("schema property is invalid")
			}
			if err := validateNode(property, depth+1); err != nil {
				return fmt.Errorf("property %s: %w", name, err)
			}
		}
	}
	if items, ok := node["items"].(map[string]any); ok {
		if err := validateNode(items, depth+1); err != nil {
			return fmt.Errorf("array items: %w", err)
		}
	}
	return nil
}

func ValidateArguments(schemaRaw json.RawMessage, arguments map[string]any) error {
	var schema map[string]any
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		return err
	}
	return validateValue(schema, arguments, "arguments", 0)
}

func validateValue(schema map[string]any, value any, path string, depth int) error {
	if depth > 10 {
		return errors.New("input nesting exceeds schema limit")
	}
	typeName, _ := schema["type"].(string)
	switch typeName {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		properties, _ := schema["properties"].(map[string]any)
		for key := range object {
			if _, ok := properties[key]; !ok {
				return fmt.Errorf("%s.%s is not allowed", path, key)
			}
		}
		if required, ok := schema["required"].([]any); ok {
			for _, raw := range required {
				name, _ := raw.(string)
				if _, ok := object[name]; !ok {
					return fmt.Errorf("%s.%s is required", path, name)
				}
			}
		}
		for key, childValue := range object {
			child, _ := properties[key].(map[string]any)
			if err := validateValue(child, childValue, path+"."+key, depth+1); err != nil {
				return err
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		if maximum, ok := schema["maxLength"].(float64); ok && len(text) > int(maximum) {
			return fmt.Errorf("%s exceeds maxLength", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	case "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("%s must be a number", path)
		}
	case "integer":
		number, ok := value.(float64)
		if !ok || number != float64(int64(number)) {
			return fmt.Errorf("%s must be an integer", path)
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		items, _ := schema["items"].(map[string]any)
		for index, item := range array {
			if err := validateValue(items, item, fmt.Sprintf("%s[%d]", path, index), depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
