package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// writeCanonicalPostgresJSONBText reproduces PostgreSQL's jsonb text form. The
// database uses that representation when computing documentation selector
// hashes, so the in-memory store must use the same ordering and spacing.
func writeCanonicalPostgresJSONBText(builder *strings.Builder, value any) error {
	switch typed := value.(type) {
	case nil:
		builder.WriteString("null")
	case bool:
		if typed {
			builder.WriteString("true")
		} else {
			builder.WriteString("false")
		}
	case json.Number:
		builder.WriteString(typed.String())
	case string:
		encoded, _ := json.Marshal(typed)
		builder.Write(encoded)
	case []any:
		builder.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				builder.WriteString(", ")
			}
			if err := writeCanonicalPostgresJSONBText(builder, item); err != nil {
				return err
			}
		}
		builder.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		// PostgreSQL jsonb stores object keys by length and then byte value.
		sort.Slice(keys, func(i, j int) bool {
			if len(keys[i]) == len(keys[j]) {
				return keys[i] < keys[j]
			}
			return len(keys[i]) < len(keys[j])
		})
		builder.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				builder.WriteString(", ")
			}
			encoded, _ := json.Marshal(key)
			builder.Write(encoded)
			builder.WriteString(": ")
			if err := writeCanonicalPostgresJSONBText(builder, typed[key]); err != nil {
				return err
			}
		}
		builder.WriteByte('}')
	default:
		return ErrConflict
	}
	return nil
}

func documentationSelectorCanonicalJSON(selector json.RawMessage) (string, error) {
	if len(selector) == 0 {
		selector = json.RawMessage(`{}`)
	}
	if !json.Valid(selector) {
		return "", ErrConflict
	}
	decoder := json.NewDecoder(strings.NewReader(string(selector)))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return "", ErrConflict
	}
	if _, ok := decoded.(map[string]any); !ok {
		return "", ErrConflict
	}
	var builder strings.Builder
	if err := writeCanonicalPostgresJSONBText(&builder, decoded); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func documentationSelectorHash(selector json.RawMessage) (string, error) {
	canonical, err := documentationSelectorCanonicalJSON(selector)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func documentationSelectorsEqual(left, right json.RawMessage) bool {
	leftCanonical, leftErr := documentationSelectorCanonicalJSON(left)
	rightCanonical, rightErr := documentationSelectorCanonicalJSON(right)
	return leftErr == nil && rightErr == nil && leftCanonical == rightCanonical
}
