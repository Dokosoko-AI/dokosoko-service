package tools_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/tools"
)

func TestSchemaAndArgumentsRejectUnexpectedAuthority(t *testing.T) {
	t.Parallel()
	schema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"project":{"type":"string","maxLength":40}},"required":["project"]}`)
	if err := tools.ValidateSchema(schema); err != nil {
		t.Fatal(err)
	}
	if err := tools.ValidateArguments(schema, map[string]any{"project": "demo"}); err != nil {
		t.Fatal(err)
	}
	if err := tools.ValidateArguments(schema, map[string]any{"project": "demo", "url": "http://metadata/"}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("unexpected argument error = %v", err)
	}
	if err := tools.ValidateSchema(json.RawMessage(`{"type":"object","additionalProperties":false,"$ref":"https://attacker/schema"}`)); err == nil {
		t.Fatal("remote schema reference was accepted")
	}
}

func TestSensitiveToolFieldDetectionSeparatesCredentialsFromOrdinaryData(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"authorization", "Authorization", "api_key", "apiKey", "X-API-Key", "X-Vendor-Token",
		"access_token", "refreshToken", "client-secret", "password", "nestedCredential", "session_token",
	} {
		if !tools.SensitiveToolFieldName(name) {
			t.Errorf("SensitiveToolFieldName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{
		"monkey", "secretary", "token_count", "page_token", "continuationToken", "cursor_token", "api_version", "cookie_count",
	} {
		if tools.SensitiveToolFieldName(name) {
			t.Errorf("SensitiveToolFieldName(%q) = true, want false", name)
		}
	}

	sensitiveSchema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"request":{"type":"object","additionalProperties":false,"properties":{"X-Vendor-Token":{"type":"string"}}}}}`)
	if !tools.SchemaContainsSensitiveFields(sensitiveSchema) {
		t.Fatal("nested credential-shaped schema field was not detected")
	}
	ordinarySchema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"monkey":{"type":"string"},"page_token":{"type":"string"}}}`)
	if tools.SchemaContainsSensitiveFields(ordinarySchema) {
		t.Fatal("ordinary schema fields were classified as credentials")
	}
	if !tools.ValueContainsSensitiveFields(map[string]any{"nested": map[string]any{"apiKey": "value"}}) {
		t.Fatal("credential-shaped runtime argument was not detected")
	}
	if tools.ValueContainsSensitiveFields(map[string]any{"monkey": "value", "page_token": "next"}) {
		t.Fatal("ordinary runtime arguments were classified as credentials")
	}
}

func TestValidateArgumentsSupportsOnlyHistoricalEmptyObjectSchemas(t *testing.T) {
	t.Parallel()
	compatible := []struct {
		name   string
		schema string
	}{
		{name: "type only", schema: `{"type":"object"}`},
		{name: "empty properties", schema: `{"type":"object","properties":{}}`},
		{name: "empty required", schema: `{"type":"object","required":[]}`},
		{name: "empty properties and required", schema: `{ "required": [], "properties": {}, "type": "object" }`},
	}
	for _, test := range compatible {
		t.Run(test.name, func(t *testing.T) {
			schema := json.RawMessage(test.schema)
			if err := tools.ValidateSchema(schema); err == nil || !strings.Contains(err.Error(), "additionalProperties") {
				t.Fatalf("ValidateSchema() error = %v, want strict closed-object rejection", err)
			}
			if err := tools.ValidateArguments(schema, map[string]any{}); err != nil {
				t.Fatalf("ValidateArguments() rejected historical empty schema: %v", err)
			}
			if err := tools.ValidateArguments(schema, map[string]any{"unexpected": true}); err == nil || !strings.Contains(err.Error(), "not allowed") {
				t.Fatalf("ValidateArguments() extra argument error = %v, want not allowed", err)
			}
		})
	}
}

func TestValidateArgumentsKeepsEmptyObjectVariantsStrict(t *testing.T) {
	t.Parallel()
	strictlyInvalid := []struct {
		name   string
		schema string
	}{
		{name: "description annotation", schema: `{"type":"object","description":"legacy"}`},
		{name: "title annotation", schema: `{"type":"object","title":"Legacy"}`},
		{name: "unknown annotation", schema: `{"type":"object","x-note":"legacy"}`},
		{name: "nonempty properties", schema: `{"type":"object","properties":{"value":{"type":"string"}}}`},
		{name: "nonempty required", schema: `{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`},
		{name: "properties wrong type", schema: `{"type":"object","properties":[]}`},
		{name: "required wrong type", schema: `{"type":"object","required":{}}`},
		{name: "additional properties true", schema: `{"type":"object","additionalProperties":true}`},
		{name: "additional properties null", schema: `{"type":"object","additionalProperties":null}`},
	}
	for _, test := range strictlyInvalid {
		t.Run(test.name, func(t *testing.T) {
			if err := tools.ValidateArguments(json.RawMessage(test.schema), map[string]any{}); err == nil {
				t.Fatal("ValidateArguments() accepted a non-historical schema variant")
			}
		})
	}

	closedAnnotated := json.RawMessage(`{"type":"object","description":"strict","properties":{},"required":[],"additionalProperties":false}`)
	if err := tools.ValidateArguments(closedAnnotated, map[string]any{}); err != nil {
		t.Fatalf("ValidateArguments() rejected an explicitly closed annotated schema: %v", err)
	}
	if err := tools.ValidateArguments(closedAnnotated, map[string]any{"unexpected": true}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("ValidateArguments() closed schema extra argument error = %v, want not allowed", err)
	}
}

func TestValidateSchemaRejectsAmbiguousOrUnenforcedContracts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		schema string
		want   string
	}{
		{name: "trailing JSON value", schema: `{"type":"object","additionalProperties":false}{"type":"object"}`, want: "trailing JSON value"},
		{name: "trailing non-JSON content", schema: `{"type":"object","additionalProperties":false} trailing`, want: "trailing content"},
		{name: "duplicate root key", schema: `{"type":"object","additionalProperties":false,"type":"object"}`, want: "duplicate object property"},
		{name: "duplicate nested key", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","type":"number"}}}`, want: "duplicate object property"},
		{name: "local reference", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"$ref":"#/$defs/value"}}}`, want: "references are unsupported"},
		{name: "remote reference", schema: `{"type":"object","additionalProperties":false,"$ref":"https://attacker.example/schema"}`, want: "references are unsupported"},
		{name: "definitions", schema: `{"type":"object","additionalProperties":false,"$defs":{"value":{"type":"string"}}}`, want: `unsupported schema keyword "$defs"`},
		{name: "missing property type", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{}}}`, want: "must declare one supported type"},
		{name: "properties is not object", schema: `{"type":"object","additionalProperties":false,"properties":[]}`, want: "properties must be an object"},
		{name: "required is not array", schema: `{"type":"object","additionalProperties":false,"required":"value"}`, want: "required must be an array"},
		{name: "required is not string", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string"}},"required":[7]}`, want: "non-empty property names"},
		{name: "required is undefined", schema: `{"type":"object","additionalProperties":false,"properties":{},"required":["value"]}`, want: "is not defined"},
		{name: "required is duplicated", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string"}},"required":["value","value"]}`, want: "is duplicated"},
		{name: "nested open object", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"object","properties":{}}}}`, want: "every object schema"},
		{name: "open object array item", schema: `{"type":"object","additionalProperties":false,"properties":{"values":{"type":"array","items":{"type":"object","properties":{}}}}}`, want: "every object schema"},
		{name: "array without items", schema: `{"type":"object","additionalProperties":false,"properties":{"values":{"type":"array"}}}`, want: "must define"},
		{name: "ignored pattern", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","pattern":".*"}}}`, want: `unsupported schema keyword "pattern"`},
		{name: "ignored format", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","format":"uri"}}}`, want: `unsupported schema keyword "format"`},
		{name: "ignored composition", schema: `{"type":"object","additionalProperties":false,"oneOf":[]}`, want: `unsupported schema keyword "oneOf"`},
		{name: "ignored default", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","default":"secret"}}}`, want: `unsupported schema keyword "default"`},
		{name: "wrong annotation type", schema: `{"type":"object","additionalProperties":false,"description":7}`, want: "description must be a string"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := tools.ValidateSchema(json.RawMessage(test.schema))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateSchema() error = %v, want containing %q", err, test.want)
			}
		})
	}

	validWithWhitespace := json.RawMessage("  {\"type\":\"object\",\"additionalProperties\":false}\n\t")
	if err := tools.ValidateSchema(validWithWhitespace); err != nil {
		t.Fatalf("trailing whitespace was rejected: %v", err)
	}
	if err := tools.ValidateArguments(json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"value":{"$ref":"#/$defs/value"}}}`), map[string]any{"value": map[string]any{"unchecked": true}}); err == nil {
		t.Fatal("argument validation accepted a schema with an unresolved local reference")
	}
}

func TestValidateSchemaAndArgumentsEnforceSupportedConstraints(t *testing.T) {
	t.Parallel()
	schema := json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"properties":{
			"mode":{"type":"string","title":"Mode","description":"Execution mode.","minLength":4,"maxLength":4,"enum":["fast","safe"]},
			"count":{"type":"integer","minimum":1,"maximum":3},
			"ratio":{"type":"number","minimum":0.5,"maximum":1.5},
			"flags":{"type":"array","minItems":1,"maxItems":2,"uniqueItems":true,"items":{"type":"boolean"}},
			"metadata":{"type":"object","additionalProperties":false,"properties":{"enabled":{"type":"boolean"}},"required":["enabled"]},
			"empty":{"type":"null"}
		},
		"required":["mode","count","ratio","flags","metadata","empty"]
	}`)
	if err := tools.ValidateSchema(schema); err != nil {
		t.Fatal(err)
	}
	valid := map[string]any{
		"mode": "fast", "count": float64(2), "ratio": float64(1.25),
		"flags": []any{true, false}, "metadata": map[string]any{"enabled": true}, "empty": nil,
	}
	if err := tools.ValidateArguments(schema, valid); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		field string
		value any
		want  string
	}{
		{name: "short string", field: "mode", value: "go", want: "minLength"},
		{name: "long string", field: "mode", value: "faster", want: "maxLength"},
		{name: "enum", field: "mode", value: "slow", want: "enum"},
		{name: "integer minimum", field: "count", value: float64(0), want: "minimum"},
		{name: "integer maximum", field: "count", value: float64(4), want: "maximum"},
		{name: "integer type", field: "count", value: float64(1.5), want: "integer"},
		{name: "number minimum", field: "ratio", value: float64(0.25), want: "minimum"},
		{name: "number maximum", field: "ratio", value: float64(2), want: "maximum"},
		{name: "array minimum", field: "flags", value: []any{}, want: "minItems"},
		{name: "array maximum", field: "flags", value: []any{true, false, true}, want: "maxItems"},
		{name: "array uniqueness", field: "flags", value: []any{true, true}, want: "duplicates"},
		{name: "array item", field: "flags", value: []any{"true"}, want: "boolean"},
		{name: "nested additional property", field: "metadata", value: map[string]any{"enabled": true, "authority": "attacker"}, want: "not allowed"},
		{name: "null type", field: "empty", value: "", want: "null"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments := make(map[string]any, len(valid))
			for key, value := range valid {
				arguments[key] = value
			}
			arguments[test.field] = test.value
			err := tools.ValidateArguments(schema, arguments)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateArguments() error = %v, want containing %q", err, test.want)
			}
		})
	}

	unicodeSchema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","maxLength":1}},"required":["value"]}`)
	if err := tools.ValidateArguments(unicodeSchema, map[string]any{"value": "é"}); err != nil {
		t.Fatalf("maxLength counted UTF-8 bytes instead of characters: %v", err)
	}
}

func TestValidateSchemaRejectsInvalidConstraintDefinitionsAndBounds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		schema string
		want   string
	}{
		{name: "negative min length", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","minLength":-1}}}`, want: "non-negative integer"},
		{name: "fractional max length", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","maxLength":1.5}}}`, want: "non-negative integer"},
		{name: "overflowing max length", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","maxLength":9223372036854775808}}}`, want: "no larger than"},
		{name: "string inverted bounds", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","minLength":2,"maxLength":1}}}`, want: "must not exceed"},
		{name: "negative min items", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"array","minItems":-1,"items":{"type":"string"}}}}`, want: "non-negative integer"},
		{name: "array inverted bounds", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"array","minItems":2,"maxItems":1,"items":{"type":"string"}}}}`, want: "must not exceed"},
		{name: "wrong unique items", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"array","uniqueItems":"yes","items":{"type":"string"}}}}`, want: "must be a boolean"},
		{name: "number inverted bounds", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"number","minimum":2,"maximum":1}}}`, want: "must not exceed"},
		{name: "empty enum", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","enum":[]}}}`, want: "between 1 and 128"},
		{name: "enum type mismatch", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","enum":[7]}}}`, want: "must be a string"},
		{name: "enum violates bound", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","maxLength":2,"enum":["long"]}}}`, want: "maxLength"},
		{name: "structured enum", schema: `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"array","items":{"type":"string"},"enum":[[]]}}}`, want: "only for scalar"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := tools.ValidateSchema(json.RawMessage(test.schema))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateSchema() error = %v, want containing %q", err, test.want)
			}
		})
	}

	tooManyEnums := make([]string, 129)
	for index := range tooManyEnums {
		tooManyEnums[index] = fmt.Sprintf("value-%d", index)
	}
	encodedEnums, err := json.Marshal(tooManyEnums)
	if err != nil {
		t.Fatal(err)
	}
	enumSchema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","enum":` + string(encodedEnums) + `}}}`)
	if err := tools.ValidateSchema(enumSchema); err == nil || !strings.Contains(err.Error(), "between 1 and 128") {
		t.Fatalf("oversized enum error = %v", err)
	}

	properties := make(map[string]any, 65)
	for index := 0; index < 64; index++ {
		properties[fmt.Sprintf("field_%d", index)] = map[string]any{"type": "string"}
	}
	maximumProperties, err := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "properties": properties})
	if err != nil {
		t.Fatal(err)
	}
	if err := tools.ValidateSchema(maximumProperties); err != nil {
		t.Fatalf("64-property schema was rejected: %v", err)
	}
	properties["field_64"] = map[string]any{"type": "string"}
	tooManyProperties, err := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "properties": properties})
	if err != nil {
		t.Fatal(err)
	}
	if err := tools.ValidateSchema(tooManyProperties); err == nil || !strings.Contains(err.Error(), "more than 64 properties") {
		t.Fatalf("property bound error = %v", err)
	}

	var nested any = map[string]any{"type": "string"}
	for range 11 {
		nested = map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"child": nested}}
	}
	tooDeep, err := json.Marshal(nested)
	if err != nil {
		t.Fatal(err)
	}
	if err := tools.ValidateSchema(tooDeep); err == nil || !strings.Contains(err.Error(), "nesting exceeds") {
		t.Fatalf("schema depth bound error = %v", err)
	}

	if err := tools.ValidateSchema(json.RawMessage(strings.Repeat(" ", (64<<10)+1))); err == nil || !strings.Contains(err.Error(), "64 KiB") {
		t.Fatalf("schema byte bound error = %v", err)
	}
}
