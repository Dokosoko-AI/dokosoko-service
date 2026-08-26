package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

var errProviderResponseTooLarge = errors.New("AI provider response exceeds the size limit")

type boundedProviderResponseBody struct {
	body      io.ReadCloser
	remaining int64
	closed    bool
}

func (body *boundedProviderResponseBody) Read(buffer []byte) (int, error) {
	if body.remaining < 0 {
		return 0, errProviderResponseTooLarge
	}
	if int64(len(buffer)) > body.remaining+1 {
		buffer = buffer[:body.remaining+1]
	}
	read, err := body.body.Read(buffer)
	if int64(read) > body.remaining {
		allowed := int(body.remaining)
		body.remaining = -1
		_ = body.close()
		return allowed, errProviderResponseTooLarge
	}
	body.remaining -= int64(read)
	return read, err
}

func (body *boundedProviderResponseBody) close() error {
	if body.closed {
		return nil
	}
	body.closed = true
	return body.body.Close()
}

func (body *boundedProviderResponseBody) Close() error { return body.close() }

type boundedProviderTransport struct {
	next http.RoundTripper
}

func (transport boundedProviderTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.next.RoundTrip(request)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}
	if response.ContentLength > maxProviderResponse {
		_ = response.Body.Close()
		return nil, errProviderResponseTooLarge
	}
	response.Body = &boundedProviderResponseBody{body: response.Body, remaining: maxProviderResponse}
	return response, nil
}

// boundedNativeHTTPClient applies the same response limit to SDK-backed
// providers that the compatible adapter applies before decoding. The wrapper
// is installed around custom clients too, so tests, injected transports, and
// production transports have one memory boundary.
func boundedNativeHTTPClient(client *http.Client) (*http.Client, error) {
	if client == nil {
		return nil, errors.New("provider HTTP client is nil")
	}
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &http.Client{
		Transport:     boundedProviderTransport{next: transport},
		CheckRedirect: client.CheckRedirect,
		Jar:           client.Jar,
		Timeout:       client.Timeout,
	}, nil
}

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
		strings.Contains(value, "recitation"),
		value == "spii":
		code = ErrorRefusedOutput
	case value == "model_context_window_exceeded", value == "context_length_exceeded":
		code = ErrorContextTooLarge
	}
	return &Error{Code: code, Provider: provider, Retryable: code == ErrorInvalidStructuredOutput}
}

func statusFromResponse(response *http.Response) int {
	if response == nil {
		return 0
	}
	return response.StatusCode
}
