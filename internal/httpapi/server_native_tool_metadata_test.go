package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestNativeToolMetadataUsesEffectAndIdempotencyContract(t *testing.T) {
	tool := model.Tool{
		Namespace: "native", Name: "write", Description: "Write through trusted source.",
		HTTPMethod: "NATIVE", BackendKind: "native", Effect: "write", IdempotencyMode: "required",
		InputSchema:         json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"medium","idempotency_required":false}`),
	}
	definition := customToolDefinition(model.ProductManifest{}, tool)
	annotations := definition["annotations"].(map[string]any)
	if annotations["readOnlyHint"] != false || annotations["destructiveHint"] != false || annotations["idempotentHint"] != true {
		t.Fatalf("annotations = %#v", annotations)
	}
	metadata := definition["_meta"].(map[string]any)
	if metadata["com.dokosoko/idempotencyKeyRequired"] != true {
		t.Fatalf("metadata = %#v", metadata)
	}
}
