package mcpbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

func validateStructuredOutput(schemaRaw json.RawMessage, result map[string]any) error {
	if len(schemaRaw) == 0 || string(schemaRaw) == "{}" {
		return nil
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		return errors.New("structuredContent is required by the imported output schema")
	}
	var schema map[string]any
	if json.Unmarshal(schemaRaw, &schema) != nil || schema["type"] != "object" || schema["additionalProperties"] != false {
		return errors.New("imported output schema is invalid")
	}
	return toolruntime.ValidateArguments(schemaRaw, structured)
}

func (m *Manager) ExecuteMCP(ctx context.Context, tool model.Tool, arguments map[string]any, principal toolruntime.Principal) (toolruntime.MCPCallResult, error) {
	connection, err := m.store.MCPConnection(ctx, tool.ProductID, tool.MCPConnectionID)
	if err != nil || connection.State != "active" || connection.ProtocolVersion != model.StatelessMCPv2Protocol || tool.UpstreamDrifted {
		return toolruntime.MCPCallResult{}, ErrInvalidConnection
	}
	bearer, err := m.connectionBearer(ctx, connection)
	if err != nil {
		return toolruntime.MCPCallResult{}, err
	}
	timeout := time.Duration(tool.TimeoutMS) * time.Millisecond
	raw, err := m.invoke(ctx, connection, "tools/call", tool.UpstreamToolName, map[string]any{"name": tool.UpstreamToolName, "arguments": arguments}, bearer, &principal, timeout)
	if err != nil {
		return toolruntime.MCPCallResult{}, err
	}
	if len(raw) > maxMCPBody {
		return toolruntime.MCPCallResult{}, ErrUpstreamProtocol
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil || result["resultType"] != "complete" {
		return toolruntime.MCPCallResult{}, ErrUpstreamProtocol
	}
	if err := validateStructuredOutput(tool.OutputSchema, result); err != nil {
		return toolruntime.MCPCallResult{}, fmt.Errorf("upstream tool output schema mismatch: %w", err)
	}
	if err := m.store.AppendAudit(ctx, model.AuditEvent{ID: auditID(), OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, ActorID: principal.Subject, Action: "mcp.tool.executed", TargetType: "tool", TargetID: tool.ID, Current: map[string]any{"connection_id": connection.ID, "upstream_tool": tool.UpstreamToolName, "protocol_version": model.StatelessMCPv2Protocol, "auth_mode": connection.AuthMode, "is_error": result["isError"] == true}, RequestID: principal.RequestID, CreatedAt: m.now()}); err != nil {
		return toolruntime.MCPCallResult{}, err
	}
	return toolruntime.MCPCallResult{Result: result}, nil
}

func ParseExpiresIn(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(typed, 10, 64)
		return parsed
	default:
		return 0
	}
}
