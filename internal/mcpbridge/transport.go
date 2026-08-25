package mcpbridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func decodeRPCJSON(reader io.Reader) (rpcResponse, error) {
	var response rpcResponse
	encoded, err := io.ReadAll(io.LimitReader(reader, maxMCPBody+1))
	if err != nil || len(encoded) > maxMCPBody {
		return rpcResponse{}, ErrUpstreamProtocol
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		return rpcResponse{}, err
	}
	if response.JSONRPC != "2.0" || (len(response.Result) == 0 && response.Error == nil) {
		return rpcResponse{}, ErrUpstreamProtocol
	}
	return response, nil
}

func decodeRPCSSE(reader io.Reader) (rpcResponse, error) {
	limited := &io.LimitedReader{R: reader, N: maxMCPBody + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 64<<10), maxMCPBody)
	data := make([]string, 0)
	var final rpcResponse
	found := false
	consume := func() error {
		if len(data) == 0 {
			return nil
		}
		encoded := strings.Join(data, "\n")
		data = data[:0]
		var candidate rpcResponse
		if err := json.Unmarshal([]byte(encoded), &candidate); err != nil {
			return err
		}
		if candidate.JSONRPC == "2.0" && (len(candidate.Result) > 0 || candidate.Error != nil) {
			final, found = candidate, true
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := consume(); err != nil {
				return rpcResponse{}, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return rpcResponse{}, err
	}
	if limited.N == 0 {
		return rpcResponse{}, ErrUpstreamProtocol
	}
	if err := consume(); err != nil {
		return rpcResponse{}, err
	}
	if !found {
		return rpcResponse{}, ErrUpstreamProtocol
	}
	return final, nil
}

func (m *Manager) invoke(ctx context.Context, connection model.MCPConnection, method, name string, params map[string]any, bearer string, timeout time.Duration) (json.RawMessage, error) {
	parsed, address, err := m.safeDestination(ctx, connection.Endpoint)
	if err != nil {
		return nil, err
	}
	meta := map[string]any{
		"io.modelcontextprotocol/protocolVersion":    model.StatelessMCPv2Protocol,
		"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "DokoSoko", "version": "2.0.0"},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	if params == nil {
		params = make(map[string]any)
	}
	params["_meta"] = meta
	id, err := randomToken(12)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", model.StatelessMCPv2Protocol)
	request.Header.Set("Mcp-Method", method)
	if name != "" {
		request.Header.Set("Mcp-Name", name)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := m.client(parsed, address, timeout).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream MCP returned %s", response.Status)
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	var rpc rpcResponse
	if strings.Contains(contentType, "text/event-stream") {
		rpc, err = decodeRPCSSE(response.Body)
	} else if strings.Contains(contentType, "application/json") {
		rpc, err = decodeRPCJSON(response.Body)
	} else {
		return nil, ErrUpstreamProtocol
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstreamProtocol, err)
	}
	if rpc.Error != nil {
		return nil, fmt.Errorf("upstream MCP error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}
	if responseID, ok := rpc.ID.(string); !ok || responseID != id {
		return nil, ErrUpstreamProtocol
	}
	var resultEnvelope struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if json.Unmarshal(rpc.Result, &resultEnvelope) != nil {
		return nil, ErrUpstreamProtocol
	}
	var serverInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if json.Unmarshal(resultEnvelope.Meta["io.modelcontextprotocol/serverInfo"], &serverInfo) != nil || serverInfo.Name == "" || serverInfo.Version == "" {
		return nil, ErrUpstreamProtocol
	}
	return rpc.Result, nil
}

func (m *Manager) connectionBearer(ctx context.Context, connection model.MCPConnection) (string, error) {
	return m.decryptSecret(ctx, connection.OrganisationID, connection.CredentialID, connection.OrganisationID+":mcp-connection:"+connection.ID+":service:")
}
