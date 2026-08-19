package tools_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko/v2/internal/tools"
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
