package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (s *Server) validateStatelessMCPv2(r *http.Request, request rpcRequest) error {
	if request.JSONRPC != "2.0" || request.Method == "" {
		return errors.New("a JSON-RPC 2.0 method is required")
	}
	if r.Header.Get("MCP-Protocol-Version") != model.StatelessMCPv2Protocol {
		return errors.New("this endpoint is Stateless MCPv2 Only and requires MCP-Protocol-Version 2026-07-28")
	}
	if r.Header.Get("Mcp-Method") != request.Method {
		return errors.New("Mcp-Method must exactly match the JSON-RPC method")
	}
	var params map[string]json.RawMessage
	if len(request.Params) == 0 || json.Unmarshal(request.Params, &params) != nil {
		return errors.New("request params must contain Stateless MCPv2 metadata")
	}
	var meta map[string]json.RawMessage
	if json.Unmarshal(params["_meta"], &meta) != nil {
		return errors.New("request params._meta is required")
	}
	var protocolVersion string
	if json.Unmarshal(meta["io.modelcontextprotocol/protocolVersion"], &protocolVersion) != nil || protocolVersion != model.StatelessMCPv2Protocol {
		return errors.New("params._meta must declare protocol version 2026-07-28")
	}
	if request.Method == "tools/call" {
		var name string
		if json.Unmarshal(params["name"], &name) != nil || name == "" || r.Header.Get("Mcp-Name") != name {
			return errors.New("Mcp-Name must exactly match the requested tool name")
		}
	}
	if origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/"); origin != "" && origin != s.baseURL {
		return errors.New("the request Origin is not allowed")
	}
	return nil
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Meta      struct {
		Confirmed             bool   `json:"confirmed"`
		ConfirmationChallenge string `json:"confirmation_challenge"`
		IdempotencyKey        string `json:"idempotency_key"`
	} `json:"_meta"`
}

func decodeToolCallParams(raw json.RawMessage) (toolCallParams, error) {
	var params toolCallParams
	paramsDecoder := json.NewDecoder(bytes.NewReader(raw))
	paramsDecoder.UseNumber()
	err := paramsDecoder.Decode(&params)
	return params, err
}
