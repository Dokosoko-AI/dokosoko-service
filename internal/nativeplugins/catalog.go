package nativeplugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

type CatalogStore interface {
	Tools(context.Context, string, bool) ([]model.Tool, error)
	Tool(context.Context, string, string) (model.Tool, error)
	CreateTool(context.Context, model.Tool) (model.Tool, error)
	StageNativeTool(context.Context, model.Tool, int64) (model.Tool, error)
	MarkImportedToolDrift(context.Context, string, string, bool) (model.Tool, error)
	AppendAudit(context.Context, model.AuditEvent) error
}

func (m *Manager) SyncCatalog(ctx context.Context, store CatalogStore, deployment model.Deployment) error {
	m.syncMu.Lock()
	defer m.syncMu.Unlock()
	if store == nil || deployment.ID == "" {
		return errors.New("native plugin catalog sync requires a deployment store")
	}
	existing, err := store.Tools(ctx, deployment.ID, false)
	if err != nil {
		return err
	}
	byNative := make(map[string]model.Tool)
	for _, tool := range existing {
		if tool.BackendKind == "native" {
			byNative[tool.NativePluginID+"\x00"+tool.NativeToolID] = tool
		}
	}
	m.mu.RLock()
	entries := make([]*entry, 0, len(m.entries))
	for _, id := range sortedEntryIDs(m.entries) {
		entries = append(entries, m.entries[id])
	}
	m.mu.RUnlock()
	seen := make(map[string]bool)
	for _, entry := range entries {
		for _, spec := range entry.manifest.Tools {
			key := entry.manifest.ID + "\x00" + spec.ID
			seen[key] = true
			desired, err := catalogTool(deployment, entry.manifest, entry.manifestHash, spec)
			if err != nil {
				return err
			}
			current, exists := byNative[key]
			if !exists {
				if _, err := store.CreateTool(ctx, desired); err != nil {
					persisted, lookupErr := store.Tool(ctx, deployment.ID, desired.ID)
					if lookupErr != nil || !sameNativeSource(persisted, desired) {
						return fmt.Errorf("create native tool %s: %w", spec.FullName(), err)
					}
					continue
				}
				_ = appendSyncAudit(ctx, store, deployment, desired, "native_tool.discovered", map[string]any{"plugin_id": entry.manifest.ID, "tool_id": spec.ID, "contract_hash": desired.NativeContractHash})
				continue
			}
			if current.State == "retired" {
				continue
			}
			if sameNativeSource(current, desired) {
				if current.UpstreamDrifted {
					_, err = store.MarkImportedToolDrift(ctx, deployment.ID, current.ID, false)
				}
				if err != nil {
					return err
				}
				continue
			}
			desired.ID, desired.Revision, desired.CreatedAt = current.ID, current.Revision, current.CreatedAt
			if _, err := store.StageNativeTool(ctx, desired, current.Revision); err != nil {
				persisted, lookupErr := store.Tool(ctx, deployment.ID, desired.ID)
				if lookupErr != nil || !sameNativeSource(persisted, desired) || persisted.State != "draft" {
					return fmt.Errorf("stage native tool %s source update: %w", spec.FullName(), err)
				}
				continue
			}
			_ = appendSyncAudit(ctx, store, deployment, desired, "native_tool.source_changed", map[string]any{"plugin_id": entry.manifest.ID, "tool_id": spec.ID, "prior_contract_hash": current.NativeContractHash, "contract_hash": desired.NativeContractHash, "state": "draft"})
		}
	}
	for key, tool := range byNative {
		if seen[key] || tool.State == "retired" || tool.UpstreamDrifted {
			continue
		}
		if _, err := store.MarkImportedToolDrift(ctx, deployment.ID, tool.ID, true); err != nil {
			return err
		}
		_ = appendSyncAudit(ctx, store, deployment, tool, "native_tool.source_missing", map[string]any{"plugin_id": tool.NativePluginID, "tool_id": tool.NativeToolID})
	}
	return nil
}

func sameNativeSource(current, desired model.Tool) bool {
	return current.BackendKind == "native" && current.NativePluginID == desired.NativePluginID && current.NativeToolID == desired.NativeToolID && current.NativePluginVersion == desired.NativePluginVersion && current.NativeSDKVersion == desired.NativeSDKVersion && current.NativeManifestHash == desired.NativeManifestHash && current.NativeContractHash == desired.NativeContractHash
}

func catalogTool(deployment model.Deployment, manifest nativeplugin.Manifest, manifestHash string, spec nativeplugin.ToolSpec) (model.Tool, error) {
	contractHash, err := nativeplugin.ToolContractHash(spec)
	if err != nil {
		return model.Tool{}, err
	}
	grants := append([]string(nil), spec.RequiredGrants...)
	sort.Strings(grants)
	risk := "low"
	if spec.Effect == nativeplugin.EffectWrite {
		risk = "medium"
	}
	if spec.Effect == nativeplugin.EffectDestructive {
		risk = "critical"
	}
	policy, _ := json.Marshal(map[string]any{"required_grants": grants, "confirmation_required": spec.ConfirmationRequired, "risk": risk, "idempotency_required": spec.Idempotency == nativeplugin.IdempotencyRequired})
	return model.Tool{ID: nativeToolID(manifest.ID, spec.ID), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, Scope: model.ToolScopeCommon, Namespace: spec.Namespace, Name: spec.Name, Description: spec.Description, InputSchema: append(json.RawMessage(nil), spec.InputSchema...), OutputSchema: append(json.RawMessage(nil), spec.OutputSchema...), HTTPMethod: "NATIVE", UpstreamAuth: json.RawMessage(`{"type":"none"}`), RequestMapping: json.RawMessage(`{}`), ResponseMapping: json.RawMessage(`{}`), AuthorizationPolicy: policy, TimeoutMS: int(spec.Timeout / time.Millisecond), BackendKind: "native", Effect: string(spec.Effect), IdempotencyMode: string(spec.Idempotency), IdentityRequirement: string(spec.Identity), StateScope: string(spec.StateScope), MaxConcurrency: spec.MaxConcurrency, MaxResultBytes: spec.MaxResultBytes, UpstreamAnnotations: json.RawMessage(`{}`), NativePluginID: manifest.ID, NativeToolID: spec.ID, NativePluginVersion: manifest.Version, NativeSDKVersion: manifest.SDKVersion, NativeManifestHash: manifestHash, NativeContractHash: contractHash}, nil
}

func appendSyncAudit(ctx context.Context, store CatalogStore, deployment model.Deployment, tool model.Tool, action string, current map[string]any) error {
	now := time.Now().UTC()
	return store.AppendAudit(ctx, model.AuditEvent{ID: "audit_" + nativeToolID(action, tool.ID+strconv.FormatInt(now.UnixNano(), 10)), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: "native_plugin_manager", Action: action, TargetType: "tool", TargetID: tool.ID, Current: current, Outcome: "success", CreatedAt: now})
}
