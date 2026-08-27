// Package runtimeauth owns the encrypted plaintext format used by reusable
// runtime Authorizations. The format is deliberately private to DokoSoko: API
// responses expose header names, but values exist only inside the encrypted
// credential version.
package runtimeauth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	MaxHeaders         = 16
	MaxCredentialBytes = 16 << 10
)

var envelopePrefix = []byte("\x00dokosoko-runtime-authorization:v1\n")

type Header struct {
	Name  string `json:"name"`
	Value []byte `json:"value"`
}

type envelope struct {
	Credential []byte   `json:"credential"`
	Headers    []Header `json:"headers,omitempty"`
}

func ValidHeaderName(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(char)) {
			continue
		}
		return false
	}
	return true
}

// SafeHeaderName rejects headers that could override routing, framing,
// delegation, or the built-in Authorization modes.
func SafeHeaderName(value string) bool {
	if !ValidHeaderName(value) {
		return false
	}
	switch strings.ToLower(value) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "host", "content-length", "transfer-encoding", "connection", "upgrade", "te", "trailer", "forwarded", "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto", "x-forwarded-uri", "x-http-method", "x-http-method-override", "x-method-override", "x-original-url", "x-original-uri", "x-rewrite-url", "x-envoy-original-path":
		return false
	default:
		return true
	}
}

func validate(primary []byte, headers []Header) error {
	if len(primary) == 0 {
		return errors.New("primary credential is empty")
	}
	if len(headers) > MaxHeaders {
		return fmt.Errorf("at most %d additional authorization headers are allowed", MaxHeaders)
	}
	seen := make(map[string]bool, len(headers))
	for _, header := range headers {
		name := strings.TrimSpace(header.Name)
		if !SafeHeaderName(name) {
			return fmt.Errorf("authorization header %q is not safe", name)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("authorization header %q is duplicated", name)
		}
		seen[key] = true
		if len(header.Value) == 0 || bytes.ContainsAny(header.Value, "\r\n\x00") {
			return fmt.Errorf("authorization header %q has an invalid value", name)
		}
	}
	return nil
}

func Encode(primary []byte, headers []Header) ([]byte, error) {
	if err := validate(primary, headers); err != nil {
		return nil, err
	}
	if len(headers) == 0 {
		return bytes.Clone(primary), nil
	}
	canonical := make([]Header, len(headers))
	for index, header := range headers {
		canonical[index] = Header{Name: strings.TrimSpace(header.Name), Value: bytes.Clone(header.Value)}
	}
	payload, err := json.Marshal(envelope{Credential: bytes.Clone(primary), Headers: canonical})
	if err != nil {
		return nil, err
	}
	result := make([]byte, 0, len(envelopePrefix)+len(payload))
	result = append(result, envelopePrefix...)
	result = append(result, payload...)
	if len(result) > MaxCredentialBytes {
		return nil, errors.New("runtime Authorization credential bundle must not exceed 16 KB")
	}
	return result, nil
}

// Decode accepts legacy scalar credentials as well as the versioned bundle.
// The NUL-prefixed marker cannot collide with credentials accepted by the
// legacy API, which rejects NUL bytes before encryption.
func Decode(raw []byte) (primary []byte, headers []Header, bundled bool, err error) {
	if !bytes.HasPrefix(raw, envelopePrefix) {
		if len(raw) == 0 {
			return nil, nil, false, errors.New("runtime Authorization credential is empty")
		}
		return bytes.Clone(raw), nil, false, nil
	}
	var value envelope
	decoder := json.NewDecoder(bytes.NewReader(raw[len(envelopePrefix):]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return nil, nil, true, errors.New("runtime Authorization credential bundle is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, nil, true, errors.New("runtime Authorization credential bundle is invalid")
	}
	if err := validate(value.Credential, value.Headers); err != nil {
		return nil, nil, true, err
	}
	return bytes.Clone(value.Credential), cloneHeaders(value.Headers), true, nil
}

func cloneHeaders(values []Header) []Header {
	result := make([]Header, len(values))
	for index, value := range values {
		result[index] = Header{Name: value.Name, Value: bytes.Clone(value.Value)}
	}
	return result
}
