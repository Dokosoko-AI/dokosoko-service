package store

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"sort"
	"time"
)

func (m *Memory) Tools(_ context.Context, productID string, publishedOnly bool) ([]model.Tool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.tools[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.Tool, 0, len(values))
	for _, value := range values {
		if !publishedOnly || value.State == "published" {
			result = append(result, m.enrichToolRuntimeTargetsLocked(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Namespace+result[i].Name < result[j].Namespace+result[j].Name })
	return result, nil
}

func (m *Memory) Tool(_ context.Context, productID, id string) (model.Tool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.tools[productID][id]
	if !ok {
		return model.Tool{}, ErrNotFound
	}
	return m.enrichToolRuntimeTargetsLocked(value), nil
}

func (m *Memory) CreateTool(_ context.Context, value model.Tool) (model.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.Tool{}, ErrNotFound
	}
	var err error
	value.Scope, value.OwnerIntegrationID, err = normalizeToolOwnership(value.Scope, value.OwnerIntegrationID)
	if err != nil {
		return model.Tool{}, err
	}
	if value.Scope == model.ToolScopeAPI {
		owner, ok := m.integrations[value.OwnerIntegrationID]
		if !ok || owner.DeploymentID != value.ProductID || owner.OrganisationID != value.OrganisationID {
			return model.Tool{}, ErrConflict
		}
	}
	if value.RuntimeServiceConnectionID != "" {
		connection, ok := m.runtimeConnections[value.RuntimeServiceConnectionID]
		if !ok || value.Scope != model.ToolScopeAPI || value.BackendKind != "http" || value.APIConnectionID != "" || connection.IntegrationID != value.OwnerIntegrationID || connection.DeploymentID != value.ProductID || connection.State != "active" || value.HTTPPath == "" {
			return model.Tool{}, ErrConflict
		}
		current := false
		for _, revision := range m.runtimeConnectionHistory[connection.ID] {
			current = current || revision.Current
		}
		if !current {
			return model.Tool{}, ErrConflict
		}
	}
	for _, current := range m.tools[value.ProductID] {
		if current.Namespace == value.Namespace && current.Name == value.Name {
			return model.Tool{}, ErrConflict
		}
	}
	value.State = "draft"
	if (value.BackendKind == "mcp" || value.BackendKind == "native") && len(value.UpstreamAuth) == 0 {
		value.UpstreamAuth = json.RawMessage(`{"type":"none"}`)
	}
	applyToolExecutionDefaults(&value)
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.tools[value.ProductID][value.ID] = cloneTool(value)
	return m.enrichToolRuntimeTargetsLocked(value), nil
}

func (m *Memory) UpdateImportedTool(_ context.Context, value model.Tool, expected int64) (model.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.tools[value.ProductID][value.ID]
	if !ok {
		return model.Tool{}, ErrNotFound
	}
	if current.Revision != expected || (current.BackendKind != "mcp" && current.BackendKind != "native") || current.State == "published" {
		return model.Tool{}, ErrConflict
	}
	value.Scope, value.OwnerIntegrationID = current.Scope, current.OwnerIntegrationID
	value.State = current.State
	value.Revision = current.Revision + 1
	value.CreatedAt = current.CreatedAt
	value.UpdatedAt = time.Now().UTC()
	m.tools[value.ProductID][value.ID] = cloneTool(value)
	return cloneTool(value), nil
}

func (m *Memory) MarkImportedToolDrift(_ context.Context, productID, id string, drifted bool) (model.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.tools[productID][id]
	if !ok || (value.BackendKind != "mcp" && value.BackendKind != "native") {
		return model.Tool{}, ErrNotFound
	}
	value.UpstreamDrifted = drifted
	value.UpdatedAt = time.Now().UTC()
	m.tools[productID][id] = cloneTool(value)
	return cloneTool(value), nil
}

func (m *Memory) StageNativeTool(_ context.Context, value model.Tool, expected int64) (model.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.tools[value.ProductID][value.ID]
	if !ok {
		return model.Tool{}, ErrNotFound
	}
	if current.Revision != expected || current.BackendKind != "native" || current.State == "retired" {
		return model.Tool{}, ErrConflict
	}
	value.Scope, value.OwnerIntegrationID = current.Scope, current.OwnerIntegrationID
	value.State = "draft"
	value.Revision = current.Revision + 1
	value.CreatedAt = current.CreatedAt
	value.UpdatedAt = time.Now().UTC()
	value.UpstreamDrifted = false
	m.tools[value.ProductID][value.ID] = cloneTool(value)
	return cloneTool(value), nil
}

func applyToolExecutionDefaults(value *model.Tool) {
	if value.Effect == "" {
		switch {
		case value.BackendKind == "http" && value.HTTPMethod == "GET":
			value.Effect = "read"
		case value.BackendKind == "http" && value.HTTPMethod == "DELETE":
			value.Effect = "destructive"
		default:
			value.Effect = "write"
		}
	}
	if value.IdempotencyMode == "" {
		if value.BackendKind == "http" && value.HTTPMethod == "GET" {
			value.IdempotencyMode = "supported"
		} else if value.BackendKind == "http" {
			value.IdempotencyMode = "required"
		} else {
			value.IdempotencyMode = "none"
		}
	}
	if value.IdentityRequirement == "" {
		value.IdentityRequirement = "none"
	}
	if value.StateScope == "" {
		value.StateScope = "none"
	}
	if value.MaxResultBytes == 0 {
		value.MaxResultBytes = 1 << 20
	}
}

func (m *Memory) PublishTool(_ context.Context, productID, id string, expected int64, _ string) (model.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.tools[productID][id]
	if !ok {
		return model.Tool{}, ErrNotFound
	}
	if value.Revision != expected || value.State != "draft" {
		return model.Tool{}, ErrConflict
	}
	if value.RuntimeServiceConnectionID != "" {
		connection, ok := m.runtimeConnections[value.RuntimeServiceConnectionID]
		if !ok || connection.State != "active" || connection.IntegrationID != value.OwnerIntegrationID {
			return model.Tool{}, ErrConflict
		}
		targets := m.currentToolRuntimeTargetsLocked(connection.ID)
		if len(targets) == 0 {
			return model.Tool{}, ErrConflict
		}
		if m.toolRuntimeTargets[value.ID] == nil {
			m.toolRuntimeTargets[value.ID] = make(map[int64][]model.ToolRuntimeTarget)
		}
		m.toolRuntimeTargets[value.ID][expected+1] = cloneToolRuntimeTargets(targets)
	}
	value.State = "published"
	value.Revision++
	value.UpdatedAt = time.Now().UTC()
	m.tools[productID][id] = cloneTool(value)
	return m.enrichToolRuntimeTargetsLocked(value), nil
}

func (m *Memory) CreateToolTestConfirmation(_ context.Context, value model.ToolTestConfirmation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tool, ok := m.tools[value.ProductID][value.ToolID]
	if !ok || tool.Revision != value.ToolRevision || value.ID == "" || len(value.NonceDigest) != 32 || len(value.ArgumentHash) != 32 {
		return ErrConflict
	}
	key := hex.EncodeToString(value.NonceDigest)
	if _, exists := m.toolTestConfirmations[key]; exists {
		return ErrConflict
	}
	value.NonceDigest = append([]byte(nil), value.NonceDigest...)
	value.ArgumentHash = append([]byte(nil), value.ArgumentHash...)
	m.toolTestConfirmations[key] = value
	return nil
}

func (m *Memory) ConsumeToolTestConfirmation(_ context.Context, digest []byte, productID, toolID string, revision int64, argumentHash []byte, actorID, _ string, now time.Time) (model.ToolTestConfirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.toolTestConfirmations[hex.EncodeToString(digest)]
	if !ok || value.ProductID != productID || value.ToolID != toolID || value.ToolRevision != revision || value.ActorID != actorID || !bytes.Equal(value.ArgumentHash, argumentHash) || !now.Before(value.ExpiresAt) {
		return model.ToolTestConfirmation{}, ErrNotFound
	}
	tool, ok := m.tools[productID][toolID]
	if !ok || tool.Revision != revision {
		return model.ToolTestConfirmation{}, ErrConflict
	}
	if _, used := m.toolTestConfirmationUses[value.ID]; used {
		return model.ToolTestConfirmation{}, ErrConflict
	}
	m.toolTestConfirmationUses[value.ID] = now
	value.NonceDigest = append([]byte(nil), value.NonceDigest...)
	value.ArgumentHash = append([]byte(nil), value.ArgumentHash...)
	return value, nil
}

func (m *Memory) CreateManagedOperationConfirmation(_ context.Context, value model.ManagedOperationConfirmation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok || value.ID == "" || value.OperationKey == "" || len(value.NonceDigest) != 32 || len(value.ArgumentHash) != 32 {
		return ErrConflict
	}
	key := hex.EncodeToString(value.NonceDigest)
	if _, exists := m.managedOperationConfirmations[key]; exists {
		return ErrConflict
	}
	value.NonceDigest = append([]byte(nil), value.NonceDigest...)
	value.ArgumentHash = append([]byte(nil), value.ArgumentHash...)
	m.managedOperationConfirmations[key] = value
	return nil
}

func (m *Memory) ConsumeManagedOperationConfirmation(_ context.Context, digest []byte, productID, operationKey string, argumentHash []byte, actorID, _ string, now time.Time) (model.ManagedOperationConfirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.managedOperationConfirmations[hex.EncodeToString(digest)]
	if !ok || value.ProductID != productID || value.OperationKey != operationKey || value.ActorID != actorID || !bytes.Equal(value.ArgumentHash, argumentHash) || !now.Before(value.ExpiresAt) {
		return model.ManagedOperationConfirmation{}, ErrNotFound
	}
	if _, used := m.managedOperationConfirmationUses[value.ID]; used {
		return model.ManagedOperationConfirmation{}, ErrConflict
	}
	m.managedOperationConfirmationUses[value.ID] = now
	value.NonceDigest = append([]byte(nil), value.NonceDigest...)
	value.ArgumentHash = append([]byte(nil), value.ArgumentHash...)
	return value, nil
}

func (m *Memory) DeleteExpiredToolTestData(_ context.Context, now time.Time, limit int) (int64, error) {
	limit = boundedToolTestCleanupLimit(limit)
	if limit == 0 {
		return 0, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var deleted int64
	confirmationDeletes := 0
	for digest, value := range m.toolTestConfirmations {
		if confirmationDeletes >= limit {
			break
		}
		if now.Before(value.ExpiresAt) {
			continue
		}
		delete(m.toolTestConfirmations, digest)
		delete(m.toolTestConfirmationUses, value.ID)
		confirmationDeletes++
		deleted++
	}
	managedDeletes := 0
	for digest, value := range m.managedOperationConfirmations {
		if managedDeletes >= limit {
			break
		}
		if now.Before(value.ExpiresAt) {
			continue
		}
		delete(m.managedOperationConfirmations, digest)
		delete(m.managedOperationConfirmationUses, value.ID)
		managedDeletes++
		deleted++
	}
	runDeletes := 0
	kept := m.toolTestRuns[:0]
	for _, value := range m.toolTestRuns {
		if runDeletes < limit && !now.Before(value.ExpiresAt) {
			runDeletes++
			deleted++
			continue
		}
		kept = append(kept, value)
	}
	for index := len(kept); index < len(m.toolTestRuns); index++ {
		m.toolTestRuns[index] = model.ToolTestRun{}
	}
	m.toolTestRuns = kept
	return deleted, nil
}

func cloneJSONShape(value model.JSONShape) model.JSONShape {
	result := value
	if value.Properties != nil {
		result.Properties = make(map[string]model.JSONShape, len(value.Properties))
		for key, child := range value.Properties {
			result.Properties[key] = cloneJSONShape(child)
		}
	}
	if value.Items != nil {
		result.Items = make([]model.JSONShape, len(value.Items))
		for index, child := range value.Items {
			result.Items[index] = cloneJSONShape(child)
		}
	}
	return result
}

func cloneToolTestRun(value model.ToolTestRun) model.ToolTestRun {
	result := value
	result.ArgumentHash = append([]byte(nil), value.ArgumentHash...)
	result.RequestShape = cloneJSONShape(value.RequestShape)
	if value.ResponseShape != nil {
		shape := cloneJSONShape(*value.ResponseShape)
		result.ResponseShape = &shape
	}
	result.Findings = append([]model.ToolTestFinding(nil), value.Findings...)
	return result
}

func (m *Memory) AppendToolTestRun(_ context.Context, value model.ToolTestRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.tools[value.ProductID][value.ToolID]
	if !ok || value.ID == "" || len(value.ArgumentHash) != 32 {
		return ErrConflict
	}
	for _, current := range m.toolTestRuns {
		if current.ID == value.ID {
			return ErrConflict
		}
	}
	m.toolTestRuns = append(m.toolTestRuns, cloneToolTestRun(value))
	return nil
}

func (m *Memory) ToolTestRuns(_ context.Context, productID, toolID string, now time.Time) ([]model.ToolTestRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.tools[productID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.ToolTestRun, 0)
	for _, value := range m.toolTestRuns {
		if value.ProductID == productID && (toolID == "" || value.ToolID == toolID) && now.Before(value.ExpiresAt) {
			result = append(result, cloneToolTestRun(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > 100 {
		result = result[:100]
	}
	return result, nil
}

func (m *Memory) ToolTestRun(_ context.Context, productID, toolID, runID string, now time.Time) (model.ToolTestRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, value := range m.toolTestRuns {
		if value.ID == runID && value.ProductID == productID && value.ToolID == toolID && now.Before(value.ExpiresAt) {
			return cloneToolTestRun(value), nil
		}
	}
	return model.ToolTestRun{}, ErrNotFound
}
