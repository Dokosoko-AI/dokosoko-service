package tools

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func requestScalarText(value any) (string, error) {
	switch current := value.(type) {
	case string:
		return current, nil
	case bool:
		return strconv.FormatBool(current), nil
	case json.Number:
		return current.String(), nil
	case float64:
		return strconv.FormatFloat(current, 'g', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(current), 'g', -1, 32), nil
	case int:
		return strconv.Itoa(current), nil
	case int8:
		return strconv.FormatInt(int64(current), 10), nil
	case int16:
		return strconv.FormatInt(int64(current), 10), nil
	case int32:
		return strconv.FormatInt(int64(current), 10), nil
	case int64:
		return strconv.FormatInt(current, 10), nil
	case uint:
		return strconv.FormatUint(uint64(current), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(current), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(current), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(current), 10), nil
	case uint64:
		return strconv.FormatUint(current, 10), nil
	default:
		return "", errors.New("request mapping requires a scalar value")
	}
}

func applyPathArgument(path, name string, value any) (string, error) {
	text, err := requestScalarText(value)
	if err != nil {
		return "", err
	}
	if text == "" || text == "." || text == ".." || strings.ContainsAny(text, "/\\?#\r\n\x00") {
		return "", fmt.Errorf("path argument %s is unsafe", name)
	}
	placeholder := "{" + name + "}"
	if !strings.Contains(path, placeholder) {
		return "", fmt.Errorf("path argument %s has no endpoint placeholder", name)
	}
	return strings.ReplaceAll(path, placeholder, text), nil
}

func extractMappedResult(value any, raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return value, nil
	}
	var segments []string
	if strings.HasPrefix(raw, "/") {
		for _, segment := range strings.Split(strings.TrimPrefix(raw, "/"), "/") {
			segments = append(segments, strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~"))
		}
	} else {
		segments = strings.Split(raw, ".")
	}
	current := value
	for _, segment := range segments {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, errors.New("response result path does not resolve to an object field")
		}
		current, ok = object[segment]
		if !ok {
			return nil, errors.New("response result path was not found")
		}
	}
	return current, nil
}

func validIdempotencyKey(value string) bool {
	if len(value) < minIdempotencyKeyLength || len(value) > maxIdempotencyKeyLength {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func ValidIdempotencyKey(value string) bool { return validIdempotencyKey(value) }

func upstreamIdempotencyKey(productID string, tool model.Tool, principal Principal) string {
	digest := sha256.New()
	for _, field := range []string{
		"dokosoko-http-tool-idempotency-v1",
		productID,
		tool.ID,
		strconv.FormatInt(tool.Revision, 10),
		principal.Issuer,
		principal.Subject,
		principal.CustomerAccountID,
		principal.InstallationID,
		principal.IdempotencyKey,
	} {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write([]byte(field))
	}
	return "doko_" + hex.EncodeToString(digest.Sum(nil))
}

// allowUpstreamConnection is intentionally scoped to one Runtime process.
// Deployments with multiple replicas multiply the aggregate allowance. The
// fixed map cap prevents unbounded memory growth; a new connection fails
// closed when all slots are occupied by active windows.
func (r *Runtime) allowUpstreamConnection(productID string, tool model.Tool) bool {
	key := upstreamConnectionKey(productID, tool)
	return r.rateLimiter.Allow(key, upstreamConnectionLimit, r.now())
}

func upstreamConnectionKey(productID string, tool model.Tool) string {
	if tool.RuntimeServiceConnectionID != "" {
		return productID + "\x00runtime:" + tool.RuntimeServiceConnectionID + "\x00" + tool.RuntimeConnectionRevisionID
	}
	key := productID + "\x00" + tool.APIConnectionID
	if tool.APIConnectionID == "" {
		key = productID + "\x00tool:" + tool.ID
	}
	return key
}

func prepareRuntimeTool(tool model.Tool, environmentID string) (model.Tool, error) {
	if tool.RuntimeServiceConnectionID == "" {
		return tool, nil
	}
	var selected *model.ToolRuntimeTarget
	for index := range tool.RuntimeTargets {
		candidate := &tool.RuntimeTargets[index]
		if candidate.RuntimeServiceConnectionID != tool.RuntimeServiceConnectionID {
			return model.Tool{}, ErrDenied
		}
		if environmentID != "" && candidate.EnvironmentID == environmentID {
			selected = candidate
			break
		}
	}
	if environmentID == "" && len(tool.RuntimeTargets) == 1 {
		selected = &tool.RuntimeTargets[0]
	}
	if selected == nil || selected.BaseURL == "" || tool.HTTPPath == "" || !strings.HasPrefix(tool.HTTPPath, "/") || strings.HasPrefix(tool.HTTPPath, "//") || strings.ContainsAny(tool.HTTPPath, "?#\\\r\n\x00") {
		return model.Tool{}, ErrDenied
	}
	authConfig := map[string]any{}
	if len(selected.AuthConfig) > 0 && json.Unmarshal(selected.AuthConfig, &authConfig) != nil {
		return model.Tool{}, ErrDenied
	}
	authConfig["type"] = selected.AuthenticationType
	if selected.AuthenticationType == "api_key_header" || selected.AuthenticationType == "custom_header" {
		authConfig["header_name"] = selected.HeaderName
	}
	authRaw, err := json.Marshal(authConfig)
	if err != nil {
		return model.Tool{}, ErrDenied
	}
	tool.BaseURL = strings.TrimRight(selected.BaseURL, "/") + tool.HTTPPath
	tool.UpstreamAuth = authRaw
	tool.CredentialID = selected.CredentialSecretID
	tool.CredentialFingerprint = selected.CredentialFingerprint
	tool.CredentialPresent = selected.CredentialSetID == "" || selected.CredentialSecretID != ""
	tool.RuntimeConnectionRevisionID = selected.ConnectionRevisionID
	tool.RuntimeCredentialSetID = selected.CredentialSetID
	tool.RuntimeCredentialVersionID = selected.CredentialVersionID
	return tool, nil
}

// acquireUpstreamSlot places an independent hard ceiling on concurrent
// outbound work. The fixed-window limiter remains a per-process best-effort
// request bound; this cap prevents a burst (or many connection windows) from
// exhausting sockets and goroutines at once.
func (r *Runtime) acquireUpstreamSlot(productID string, tool model.Tool) bool {
	key := upstreamConnectionKey(productID, tool)
	r.concurrencyMu.Lock()
	defer r.concurrencyMu.Unlock()
	if r.globalInFlight >= maxUpstreamConcurrency || r.connectionInFlight[key] >= maxConnectionConcurrency {
		return false
	}
	r.globalInFlight++
	r.connectionInFlight[key]++
	return true
}

func (r *Runtime) releaseUpstreamSlot(productID string, tool model.Tool) {
	key := upstreamConnectionKey(productID, tool)
	r.concurrencyMu.Lock()
	defer r.concurrencyMu.Unlock()
	if count := r.connectionInFlight[key]; count <= 1 {
		delete(r.connectionInFlight, key)
	} else {
		r.connectionInFlight[key] = count - 1
	}
	if r.globalInFlight > 0 {
		r.globalInFlight--
	}
}
