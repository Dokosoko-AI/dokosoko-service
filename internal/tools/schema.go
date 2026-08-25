package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxSchemaBytes      = 64 << 10
	maxSchemaDepth      = 10
	maxSchemaProperties = 64
	maxSchemaEnums      = 128
	maxConstraintBound  = 1 << 30
)

var annotationKeywords = map[string]bool{
	"description": true,
	"title":       true,
}

func decodeSchema(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || len(raw) > maxSchemaBytes {
		return nil, errors.New("schema must be present and no larger than 64 KiB")
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return nil, err
	}
	var schema map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&schema); err != nil {
		return nil, fmt.Errorf("invalid JSON schema: %w", err)
	}
	if schema == nil {
		return nil, errors.New("tool schema must be a JSON object")
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	return schema, nil
}

func rejectDuplicateJSONKeys(raw json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return fmt.Errorf("invalid JSON schema: %w", err)
	}
	return requireJSONEOF(decoder)
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			rawKey, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := rawKey.(string)
			if !ok {
				return errors.New("object property name must be a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate object property %q is forbidden", key)
			}
			seen[key] = true
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return errors.New("object is not terminated")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return errors.New("array is not terminated")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("invalid JSON schema: trailing JSON value is forbidden")
		}
		return fmt.Errorf("invalid JSON schema: trailing content is forbidden: %w", err)
	}
	return nil
}

func ValidateSchema(raw json.RawMessage) error {
	schema, err := decodeSchema(raw)
	if err != nil {
		return err
	}
	if schema["type"] != "object" {
		return errors.New("tool schema root type must be object")
	}
	return validateNode(schema, 0)
}

// SensitiveToolFieldName identifies credential-shaped contract fields. API
// authentication belongs in the tool's write-only connection configuration,
// never in agent-supplied arguments or agent-visible results.
func SensitiveToolFieldName(value string) bool {
	words := sensitiveFieldWords(value)
	if len(words) == 0 {
		return false
	}
	joined := strings.Join(words, "")
	for _, exact := range []string{
		"authorization", "proxyauthorization", "bearer", "bearertoken", "credential", "credentials",
		"password", "passwd", "secret", "apikey", "xapikey", "accesstoken", "refreshtoken", "clientsecret",
		"privatekey", "signingkey", "sessionkey", "sessiontoken", "cookie", "setcookie",
	} {
		if joined == exact {
			return true
		}
	}
	for _, word := range words {
		switch word {
		case "authorization", "bearer", "credential", "credentials", "password", "passwd", "secret":
			return true
		}
	}
	last := words[len(words)-1]
	if last == "key" {
		for _, word := range words[:len(words)-1] {
			switch word {
			case "api", "client", "private", "signing", "session", "auth", "authentication", "vendor":
				return true
			}
		}
	}
	if last == "token" {
		// Cursor-like tokens are ordinary API data. Every other named token is
		// credential-shaped and belongs in the write-only connection boundary.
		for _, word := range words[:len(words)-1] {
			switch word {
			case "page", "pagination", "continuation", "cursor", "next":
				return false
			}
		}
		return len(words) > 1
	}
	return false
}

func sensitiveFieldWords(value string) []string {
	var words []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		words = append(words, strings.ToLower(string(current)))
		current = current[:0]
	}
	for _, character := range []rune(value) {
		if character >= 'A' && character <= 'Z' && len(current) > 0 {
			flush()
		}
		if character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			current = append(current, character)
			continue
		}
		flush()
	}
	flush()
	return words
}

func schemaNodeContainsSensitiveField(node map[string]any) bool {
	if properties, ok := node["properties"].(map[string]any); ok {
		for name, raw := range properties {
			if SensitiveToolFieldName(name) {
				return true
			}
			if child, ok := raw.(map[string]any); ok && schemaNodeContainsSensitiveField(child) {
				return true
			}
		}
	}
	if items, ok := node["items"].(map[string]any); ok {
		return schemaNodeContainsSensitiveField(items)
	}
	return false
}

func SchemaContainsSensitiveFields(raw json.RawMessage) bool {
	schema, err := decodeSchema(raw)
	return err == nil && schemaNodeContainsSensitiveField(schema)
}

func ValueContainsSensitiveFields(value any) bool {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if SensitiveToolFieldName(key) || ValueContainsSensitiveFields(child) {
				return true
			}
		}
	case []any:
		for _, child := range current {
			if ValueContainsSensitiveFields(child) {
				return true
			}
		}
	}
	return false
}

func validateNode(node map[string]any, depth int) error {
	if depth > maxSchemaDepth {
		return errors.New("schema nesting exceeds 10 levels")
	}
	if _, ok := node["$ref"]; ok {
		return errors.New("schema references are unsupported until local reference resolution is implemented")
	}
	typeName, ok := node["type"].(string)
	if !ok || typeName == "" {
		return errors.New("schema node must declare one supported type")
	}
	allowed := map[string]bool{"object": true, "array": true, "string": true, "number": true, "integer": true, "boolean": true, "null": true}
	if !allowed[typeName] {
		return fmt.Errorf("unsupported schema type %q", typeName)
	}
	if err := validateKeywords(node, typeName); err != nil {
		return err
	}
	for keyword := range annotationKeywords {
		if value, present := node[keyword]; present {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("schema %s must be a string", keyword)
			}
		}
	}

	switch typeName {
	case "object":
		if additional, ok := node["additionalProperties"]; !ok || additional != false {
			return errors.New("every object schema must set additionalProperties to false")
		}
		properties := map[string]any{}
		if rawProperties, present := node["properties"]; present {
			var ok bool
			properties, ok = rawProperties.(map[string]any)
			if !ok {
				return errors.New("schema properties must be an object")
			}
		}
		if len(properties) > maxSchemaProperties {
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
		if rawRequired, present := node["required"]; present {
			required, ok := rawRequired.([]any)
			if !ok {
				return errors.New("schema required must be an array of property names")
			}
			seen := make(map[string]bool, len(required))
			for _, raw := range required {
				name, ok := raw.(string)
				if !ok || name == "" {
					return errors.New("schema required must contain only non-empty property names")
				}
				if seen[name] {
					return fmt.Errorf("schema required property %q is duplicated", name)
				}
				if _, ok := properties[name]; !ok {
					return fmt.Errorf("schema required property %q is not defined", name)
				}
				seen[name] = true
			}
		}
	case "array":
		items, ok := node["items"].(map[string]any)
		if !ok {
			return errors.New("array schema must define one object-valued items schema")
		}
		if err := validateNode(items, depth+1); err != nil {
			return fmt.Errorf("array items: %w", err)
		}
		minimum, hasMinimum, err := nonNegativeIntegerKeyword(node, "minItems")
		if err != nil {
			return err
		}
		maximum, hasMaximum, err := nonNegativeIntegerKeyword(node, "maxItems")
		if err != nil {
			return err
		}
		if hasMinimum && hasMaximum && minimum > maximum {
			return errors.New("array minItems must not exceed maxItems")
		}
		if unique, present := node["uniqueItems"]; present {
			if _, ok := unique.(bool); !ok {
				return errors.New("array uniqueItems must be a boolean")
			}
		}
	case "string":
		minimum, hasMinimum, err := nonNegativeIntegerKeyword(node, "minLength")
		if err != nil {
			return err
		}
		maximum, hasMaximum, err := nonNegativeIntegerKeyword(node, "maxLength")
		if err != nil {
			return err
		}
		if hasMinimum && hasMaximum && minimum > maximum {
			return errors.New("string minLength must not exceed maxLength")
		}
	case "number", "integer":
		minimum, hasMinimum, err := numberKeyword(node, "minimum")
		if err != nil {
			return err
		}
		maximum, hasMaximum, err := numberKeyword(node, "maximum")
		if err != nil {
			return err
		}
		if hasMinimum && hasMaximum && minimum > maximum {
			return errors.New("schema minimum must not exceed maximum")
		}
	}

	if rawEnum, present := node["enum"]; present {
		if typeName == "object" || typeName == "array" {
			return errors.New("enum is supported only for scalar schema types")
		}
		values, ok := rawEnum.([]any)
		if !ok || len(values) == 0 || len(values) > maxSchemaEnums {
			return errors.New("schema enum must contain between 1 and 128 scalar values")
		}
		for index, value := range values {
			if err := validateValue(node, value, fmt.Sprintf("schema enum[%d]", index), depth); err != nil {
				return fmt.Errorf("invalid enum value: %w", err)
			}
		}
	}
	return nil
}

func validateKeywords(node map[string]any, typeName string) error {
	allowed := map[string]bool{"type": true, "title": true, "description": true, "enum": true}
	switch typeName {
	case "object":
		allowed["properties"], allowed["required"], allowed["additionalProperties"] = true, true, true
	case "array":
		allowed["items"], allowed["minItems"], allowed["maxItems"], allowed["uniqueItems"] = true, true, true, true
	case "string":
		allowed["minLength"], allowed["maxLength"] = true, true
	case "number", "integer":
		allowed["minimum"], allowed["maximum"] = true, true
	}
	for keyword := range node {
		if !allowed[keyword] {
			return fmt.Errorf("unsupported schema keyword %q", keyword)
		}
	}
	return nil
}

func nonNegativeIntegerKeyword(node map[string]any, keyword string) (int, bool, error) {
	raw, present := node[keyword]
	if !present {
		return 0, false, nil
	}
	number, ok := numberValue(raw)
	if !ok || number < 0 || math.Trunc(number) != number || number > maxConstraintBound {
		return 0, false, fmt.Errorf("schema %s must be a non-negative integer no larger than %d", keyword, maxConstraintBound)
	}
	return int(number), true, nil
}

func numberKeyword(node map[string]any, keyword string) (float64, bool, error) {
	raw, present := node[keyword]
	if !present {
		return 0, false, nil
	}
	number, ok := numberValue(raw)
	if !ok || math.IsInf(number, 0) || math.IsNaN(number) {
		return 0, false, fmt.Errorf("schema %s must be a finite number", keyword)
	}
	return number, true, nil
}

func numberValue(value any) (float64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseFloat(string(number), 64)
		return parsed, err == nil
	case float64:
		return number, !math.IsInf(number, 0) && !math.IsNaN(number)
	case float32:
		parsed := float64(number)
		return parsed, !math.IsInf(parsed, 0) && !math.IsNaN(parsed)
	case int:
		return float64(number), true
	case int8:
		return float64(number), true
	case int16:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint8:
		return float64(number), true
	case uint16:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		return float64(number), true
	default:
		return 0, false
	}
}

func ValidateArguments(schemaRaw json.RawMessage, arguments map[string]any) error {
	schema, err := decodeSchema(schemaRaw)
	if err != nil {
		return err
	}
	if schema["type"] != "object" {
		return errors.New("tool schema root type must be object")
	}
	schema = normalizeHistoricalEmptyObjectSchemaForArguments(schema)
	if err := validateNode(schema, 0); err != nil {
		return err
	}
	return validateValue(schema, arguments, "arguments", 0)
}

// normalizeHistoricalEmptyObjectSchemaForArguments preserves runtime support
// for schemas written before object schemas were required to be explicitly
// closed. The exception is deliberately limited to an otherwise unannotated
// empty root object; ValidateSchema remains strict for every stored schema.
func normalizeHistoricalEmptyObjectSchemaForArguments(schema map[string]any) map[string]any {
	for keyword, value := range schema {
		switch keyword {
		case "type":
			if value != "object" {
				return schema
			}
		case "properties":
			properties, ok := value.(map[string]any)
			if !ok || len(properties) != 0 {
				return schema
			}
		case "required":
			required, ok := value.([]any)
			if !ok || len(required) != 0 {
				return schema
			}
		default:
			return schema
		}
	}

	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

func validateValue(schema map[string]any, value any, path string, depth int) error {
	if depth > maxSchemaDepth {
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
		length := utf8.RuneCountInString(text)
		if minimum, present, _ := nonNegativeIntegerKeyword(schema, "minLength"); present && length < minimum {
			return fmt.Errorf("%s is shorter than minLength", path)
		}
		if maximum, present, _ := nonNegativeIntegerKeyword(schema, "maxLength"); present && length > maximum {
			return fmt.Errorf("%s exceeds maxLength", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	case "null":
		if value != nil {
			return fmt.Errorf("%s must be null", path)
		}
	case "number", "integer":
		number, ok := numberValue(value)
		if !ok {
			return fmt.Errorf("%s must be a number", path)
		}
		if typeName == "integer" && math.Trunc(number) != number {
			return fmt.Errorf("%s must be an integer", path)
		}
		if minimum, present, _ := numberKeyword(schema, "minimum"); present && number < minimum {
			return fmt.Errorf("%s is less than minimum", path)
		}
		if maximum, present, _ := numberKeyword(schema, "maximum"); present && number > maximum {
			return fmt.Errorf("%s exceeds maximum", path)
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if minimum, present, _ := nonNegativeIntegerKeyword(schema, "minItems"); present && len(array) < minimum {
			return fmt.Errorf("%s has fewer than minItems", path)
		}
		if maximum, present, _ := nonNegativeIntegerKeyword(schema, "maxItems"); present && len(array) > maximum {
			return fmt.Errorf("%s exceeds maxItems", path)
		}
		items, _ := schema["items"].(map[string]any)
		unique, _ := schema["uniqueItems"].(bool)
		seen := make(map[string]bool, len(array))
		for index, item := range array {
			if err := validateValue(items, item, fmt.Sprintf("%s[%d]", path, index), depth+1); err != nil {
				return err
			}
			if unique {
				key, err := jsonValueKey(item)
				if err != nil {
					return fmt.Errorf("%s[%d] is not a JSON value: %w", path, index, err)
				}
				if seen[key] {
					return fmt.Errorf("%s[%d] duplicates an earlier item", path, index)
				}
				seen[key] = true
			}
		}
	default:
		return fmt.Errorf("%s uses an unsupported schema type", path)
	}
	if values, ok := schema["enum"].([]any); ok {
		matched := false
		for _, candidate := range values {
			matched = matched || jsonValueEqual(candidate, value)
		}
		if !matched {
			return fmt.Errorf("%s is not one of the allowed enum values", path)
		}
	}
	return nil
}

func jsonValueEqual(left, right any) bool {
	leftNumber, leftIsNumber := numberValue(left)
	rightNumber, rightIsNumber := numberValue(right)
	if leftIsNumber || rightIsNumber {
		return leftIsNumber && rightIsNumber && leftNumber == rightNumber
	}
	return reflect.DeepEqual(left, right)
}

func jsonValueKey(value any) (string, error) {
	if number, ok := numberValue(value); ok {
		return "number:" + strconv.FormatFloat(number, 'g', -1, 64), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%T:%s", value, encoded), nil
}
