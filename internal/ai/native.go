package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func nativeHTTPError(provider string, status int, codeValue, typeValue string, cause error) error {
	encoded, _ := json.Marshal(map[string]any{"error": map[string]string{"code": codeValue, "type": typeValue}})
	value := providerHTTPError(provider, status, encoded)
	if normalized, ok := value.(*Error); ok {
		normalized.Cause = cause
		return normalized
	}
	return &Error{Code: ErrorProviderUnavailable, Provider: provider, Cause: cause}
}

func nativeTransportError(provider string, err error) error {
	code := ErrorProviderUnavailable
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		code = ErrorTimeout
	}
	return &Error{Code: code, Provider: provider, Retryable: true, Cause: err}
}

func permissiveObjectSchema(raw json.RawMessage) (map[string]any, error) {
	schema := map[string]any{"type": "object", "additionalProperties": true}
	if len(raw) == 0 {
		return schema, nil
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, &Error{Code: ErrorInvalidConfiguration, Cause: err}
	}
	return schema, nil
}

// validateStructuredFinishReason accepts only an explicit, provider-specific
// successful terminal reason. Structured JSON that happens to parse is still
// incomplete when the provider reports truncation, and an unknown future reason
// must fail closed until its semantics are reviewed.
func validateStructuredFinishReason(provider, value string, successful ...string) error {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range successful {
		if value == strings.ToLower(strings.TrimSpace(candidate)) {
			return nil
		}
	}

	code := ErrorInvalidStructuredOutput
	switch {
	case value == "refusal",
		strings.Contains(value, "safety"),
		strings.Contains(value, "prohibited"),
		strings.Contains(value, "blocklist"),
		strings.Contains(value, "content_filter"),
		value == "recitation",
		value == "spii":
		code = ErrorRefusedOutput
	case value == "model_context_window_exceeded", value == "context_length_exceeded":
		code = ErrorContextTooLarge
	}
	return &Error{Code: code, Provider: provider}
}

func statusFromResponse(response *http.Response) int {
	if response == nil {
		return 0
	}
	return response.StatusCode
}
