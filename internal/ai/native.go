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

func textFinishReason(value string) error {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "refusal" || strings.Contains(value, "safety") || strings.Contains(value, "prohibited") || strings.Contains(value, "blocklist") {
		return &Error{Code: ErrorRefusedOutput}
	}
	if value == "model_context_window_exceeded" {
		return &Error{Code: ErrorContextTooLarge}
	}
	return nil
}

func statusFromResponse(response *http.Response) int {
	if response == nil {
		return 0
	}
	return response.StatusCode
}
