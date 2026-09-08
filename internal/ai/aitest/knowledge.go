// Package aitest provides explicit, network-free AI responses for tests.
package aitest

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

type Knowledge struct {
	Calls     atomic.Int32
	Before    func(*http.Request)
	Transform func(int, json.RawMessage) (json.RawMessage, error)
}

func (d *Knowledge) Do(request *http.Request) (*http.Response, error) {
	call := int(d.Calls.Add(1))
	if d.Before != nil {
		d.Before(request)
	}
	var payload struct {
		Messages []struct{ Role, Content string }
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		return nil, err
	}
	var input struct {
		Evidence []struct{ ID, Content string }
	}
	for _, message := range payload.Messages {
		if message.Role == "user" {
			if err := json.Unmarshal([]byte(message.Content), &input); err != nil {
				return nil, err
			}
		}
	}
	items := []any{}
	for _, item := range input.Evidence {
		runes := []rune(strings.TrimSpace(item.Content))
		items = append(items, map[string]any{"id": item.ID, "summary": "Review the cited integration guidance.", "evidence_quote": string(runes[:min(64, len(runes))]), "findings": []string{}})
	}
	content, err := json.Marshal(map[string]any{"items": items})
	if err == nil && d.Transform != nil {
		content, err = d.Transform(call, content)
	}
	if err != nil {
		return nil, err
	}
	payloadJSON, err := json.Marshal(map[string]any{"model": "knowledge-fixture", "choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": string(content)}}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10}})
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(payloadJSON))), Request: request}, nil
}
