package mcpbridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

func normalizeInputSchema(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > 64<<10 {
		return nil, errors.New("input schema is missing or too large")
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, errors.New("input schema is invalid JSON")
	}
	if schema["type"] != "object" {
		return nil, errors.New("input schema root must be an object")
	}
	if _, ok := schema["properties"]; !ok {
		schema["properties"] = map[string]any{}
	}
	if _, ok := schema["additionalProperties"]; !ok {
		schema["additionalProperties"] = false
	}
	encoded, _ := json.Marshal(schema)
	if err := toolruntime.ValidateSchema(encoded); err != nil {
		return nil, err
	}
	return encoded, nil
}

func normalizeOutputSchema(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if len(raw) > 64<<10 {
		return nil, errors.New("output schema is too large")
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, errors.New("output schema is invalid JSON")
	}
	if schema["type"] != "object" {
		return nil, errors.New("output schema root must be an object")
	}
	if _, ok := schema["properties"]; !ok {
		schema["properties"] = map[string]any{}
	}
	if _, ok := schema["additionalProperties"]; !ok {
		schema["additionalProperties"] = false
	}
	encoded, _ := json.Marshal(schema)
	if err := toolruntime.ValidateSchema(encoded); err != nil {
		return nil, err
	}
	return encoded, nil
}

func catalogToolHash(value CatalogTool) string {
	encoded, _ := json.Marshal(struct {
		Name   string
		Input  json.RawMessage
		Output json.RawMessage
	}{value.Name, value.InputSchema, value.OutputSchema})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func (m *Manager) Inspect(ctx context.Context, productID, connectionID string) (Catalog, error) {
	connection, err := m.store.MCPConnection(ctx, productID, connectionID)
	if err != nil {
		return Catalog{}, err
	}
	if connection.ProtocolVersion != model.StatelessMCPv2Protocol || connection.State != "active" {
		return Catalog{}, ErrInvalidConnection
	}
	bearer, err := m.connectionBearer(ctx, connection)
	if err != nil {
		return Catalog{}, err
	}
	raw, err := m.invoke(ctx, connection, "tools/list", "", nil, bearer, 20*time.Second)
	if err != nil {
		return Catalog{}, err
	}
	var result struct {
		ResultType string `json:"resultType"`
		Tools      []struct {
			Name         string          `json:"name"`
			Title        string          `json:"title"`
			Description  string          `json:"description"`
			InputSchema  json.RawMessage `json:"inputSchema"`
			OutputSchema json.RawMessage `json:"outputSchema"`
			Annotations  json.RawMessage `json:"annotations"`
		} `json:"tools"`
		TTLMS int64 `json:"ttlMs"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || result.ResultType != "complete" {
		return Catalog{}, ErrUpstreamProtocol
	}
	catalog := Catalog{Connection: connection, Tools: make([]CatalogTool, 0, len(result.Tools)), TTLMS: result.TTLMS}
	for _, upstream := range result.Tools {
		tool := CatalogTool{Name: upstream.Name, Title: upstream.Title, Description: upstream.Description, InputSchema: upstream.InputSchema, OutputSchema: upstream.OutputSchema, Annotations: upstream.Annotations}
		tool.SchemaHash = catalogToolHash(tool)
		catalog.Tools = append(catalog.Tools, tool)
	}
	sort.Slice(catalog.Tools, func(i, j int) bool { return catalog.Tools[i].Name < catalog.Tools[j].Name })
	encoded, _ := json.Marshal(catalog.Tools)
	digest := sha256.Sum256(encoded)
	catalog.CatalogHash = hex.EncodeToString(digest[:])
	return catalog, nil
}

func (m *Manager) Import(ctx context.Context, productID, connectionID string, input ImportInput, actor Actor) (ImportResult, error) {
	if len(input.ToolNames) == 0 {
		return ImportResult{}, errors.New("select at least one upstream tool")
	}
	if input.TimeoutMS == 0 {
		input.TimeoutMS = 15000
	}
	if input.TimeoutMS < 100 || input.TimeoutMS > 60000 {
		return ImportResult{}, errors.New("tool timeout must be between 100 and 60000 milliseconds")
	}
	catalog, err := m.Inspect(ctx, productID, connectionID)
	if err != nil {
		return ImportResult{}, err
	}
	selected := make(map[string]bool, len(input.ToolNames))
	for _, name := range input.ToolNames {
		selected[name] = true
	}
	existingValues, err := m.store.Tools(ctx, productID, false)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return ImportResult{}, err
	}
	existing := make(map[string][]model.Tool)
	for _, value := range existingValues {
		if value.BackendKind == "mcp" && value.MCPConnectionID == connectionID {
			existing[value.UpstreamToolName] = append(existing[value.UpstreamToolName], value)
		}
	}
	result := ImportResult{Connection: catalog.Connection, Rejected: make(map[string]string)}
	policy, _ := json.Marshal(map[string]any{"required_grants": normalizeScopes(input.RequiredGrants), "confirmation_required": input.ConfirmationRequired, "risk": "medium", "idempotency_required": false})
	for _, upstream := range catalog.Tools {
		if !selected[upstream.Name] {
			continue
		}
		matching := existing[upstream.Name]
		if len(matching) > 1 {
			for _, duplicate := range matching {
				if duplicate.State == "published" && !duplicate.UpstreamDrifted {
					drifted, markErr := m.store.MarkImportedToolDrift(ctx, productID, duplicate.ID, true)
					if markErr != nil {
						return ImportResult{}, markErr
					}
					result.Drifted = append(result.Drifted, drifted)
				}
			}
			result.Rejected[upstream.Name] = "multiple local tools reference this upstream identity; retire duplicates before importing"
			continue
		}
		if !upstreamToolPattern.MatchString(upstream.Name) || len(catalog.Connection.Namespace)+1+len(upstream.Name) > 128 {
			result.Rejected[upstream.Name] = "tool name cannot be safely namespaced"
			continue
		}
		inputSchema, err := normalizeInputSchema(upstream.InputSchema)
		if err != nil {
			result.Rejected[upstream.Name] = err.Error()
			continue
		}
		outputSchema, err := normalizeOutputSchema(upstream.OutputSchema)
		if err != nil {
			result.Rejected[upstream.Name] = err.Error()
			continue
		}
		annotations := upstream.Annotations
		if len(annotations) == 0 {
			annotations = json.RawMessage(`{}`)
		}
		candidate := model.Tool{OrganisationID: catalog.Connection.OrganisationID, ProductID: productID, Scope: model.ToolScopeCommon, Namespace: catalog.Connection.Namespace, Name: upstream.Name, Description: strings.TrimSpace(upstream.Description), InputSchema: inputSchema, OutputSchema: outputSchema, HTTPMethod: "MCP", AuthorizationPolicy: policy, TimeoutMS: input.TimeoutMS, BackendKind: "mcp", MCPConnectionID: connectionID, UpstreamToolName: upstream.Name, UpstreamSchemaHash: upstream.SchemaHash, UpstreamAnnotations: annotations}
		if candidate.Description == "" {
			candidate.Description = upstream.Title
		}
		if candidate.Description == "" {
			candidate.Description = "Imported Stateless MCPv2 tool " + upstream.Name
		}
		current, ok := model.Tool{}, len(matching) == 1
		if ok {
			current = matching[0]
		}
		if !ok {
			candidate.ID, err = randomUUID()
			if err != nil {
				return ImportResult{}, err
			}
			created, err := m.store.CreateTool(ctx, candidate)
			if err != nil {
				result.Rejected[upstream.Name] = err.Error()
				continue
			}
			result.Created = append(result.Created, created)
			continue
		}
		if current.UpstreamSchemaHash == upstream.SchemaHash {
			if current.UpstreamDrifted {
				current, err = m.store.MarkImportedToolDrift(ctx, productID, current.ID, false)
				if err != nil {
					return ImportResult{}, err
				}
			}
			result.Unchanged = append(result.Unchanged, current)
			continue
		}
		if current.State == "published" {
			drifted, markErr := m.store.MarkImportedToolDrift(ctx, productID, current.ID, true)
			if markErr != nil {
				return ImportResult{}, markErr
			}
			result.Drifted = append(result.Drifted, drifted)
			continue
		}
		candidate.ID = current.ID
		updated, err := m.store.UpdateImportedTool(ctx, candidate, current.Revision)
		if err != nil {
			return ImportResult{}, err
		}
		result.Updated = append(result.Updated, updated)
	}
	syncedAt := m.now()
	result.Connection, err = m.store.UpdateMCPConnectionSync(ctx, productID, connectionID, catalog.CatalogHash, syncedAt)
	if err != nil {
		return ImportResult{}, err
	}
	if err := m.store.AppendAudit(ctx, model.AuditEvent{ID: auditID(), OrganisationID: result.Connection.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "mcp.connection.imported", TargetType: "mcp_connection", TargetID: connectionID, Current: map[string]any{"protocol_version": model.StatelessMCPv2Protocol, "created": len(result.Created), "updated": len(result.Updated), "drifted": len(result.Drifted), "rejected": len(result.Rejected), "catalog_hash": catalog.CatalogHash}, RequestID: actor.RequestID, CreatedAt: syncedAt}); err != nil {
		return ImportResult{}, err
	}
	return result, nil
}
