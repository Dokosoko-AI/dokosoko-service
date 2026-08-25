package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestAdminToolRedactsLegacyEndpointURLMetadata(t *testing.T) {
	public := adminTool(model.Tool{BackendKind: "http", BaseURL: "https://user:password@api.example.test/items?api_key=legacy-secret#token"})
	encoded, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"user", "password", "api_key", "legacy-secret", "token"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("admin tool leaked %q: %s", forbidden, encoded)
		}
	}
	if public["endpoint"] != "https://api.example.test/items" || public["endpoint_requires_review"] != true {
		t.Fatalf("redacted endpoint = %#v", public)
	}
}

func TestToolCallParamsPreserveLargeJSONIntegers(t *testing.T) {
	params, err := decodeToolCallParams(json.RawMessage(`{"name":"records.read","arguments":{"record_id":9007199254740993},"_meta":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	identifier, ok := params.Arguments["record_id"].(json.Number)
	if !ok || identifier.String() != "9007199254740993" {
		t.Fatalf("large integer = %#v", params.Arguments["record_id"])
	}
}
