package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

type sensitiveFieldResolver struct{ calls int }

func (r *sensitiveFieldResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	r.calls++
	return []net.IP{net.ParseIP("203.0.113.8")}, nil
}

func TestExecutionRejectsCredentialShapedContractsBeforeNetwork(t *testing.T) {
	for _, test := range []struct {
		name      string
		input     json.RawMessage
		output    json.RawMessage
		arguments map[string]any
	}{
		{
			name:      "input schema",
			input:     json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"api_key":{"type":"string"}}}`),
			output:    json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
			arguments: map[string]any{},
		},
		{
			name:      "output schema",
			input:     json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
			output:    json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"accessToken":{"type":"string"}}}`),
			arguments: map[string]any{},
		},
		{
			name:      "argument defense in depth",
			input:     json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
			output:    json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
			arguments: map[string]any{"nested": map[string]any{"X-Vendor-Token": "value"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver := &sensitiveFieldResolver{}
			runtime := NewRuntime(nil, resolver, nil)
			tool := model.Tool{
				BackendKind: "http", HTTPMethod: "GET", BaseURL: "https://api.vendor.example/action",
				InputSchema: test.input, OutputSchema: test.output, AuthorizationPolicy: json.RawMessage(`{}`),
			}
			_, err := runtime.executeAuthorizedTraced(context.Background(), "product", "vendor.action", tool, test.arguments, Principal{}, nil, false)
			if !errors.Is(err, ErrDenied) {
				t.Fatalf("execution returned %v, want ErrDenied", err)
			}
			if resolver.calls != 0 {
				t.Fatalf("resolver calls = %d, want zero", resolver.calls)
			}
		})
	}
}
