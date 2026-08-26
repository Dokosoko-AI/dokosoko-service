package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type IntegrationPublishChange struct {
	Field  string `json:"field"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

type IntegrationPublishValidation struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Tab     string `json:"tab"`
}

type IntegrationPublishStatus struct {
	Ready               bool                           `json:"ready"`
	HasChanges          bool                           `json:"has_changes"`
	CurrentManifestHash string                         `json:"current_manifest_hash"`
	CurrentSnapshot     json.RawMessage                `json:"current_snapshot"`
	LatestRevision      *model.IntegrationRevision     `json:"latest_revision,omitempty"`
	Changes             []IntegrationPublishChange     `json:"changes"`
	Validations         []IntegrationPublishValidation `json:"validations"`
}

type integrationResourceSnapshot struct {
	SetID              string                       `json:"set_id"`
	Kind               string                       `json:"kind"`
	Name               string                       `json:"name"`
	RevisionID         string                       `json:"revision_id"`
	Revision           int64                        `json:"revision"`
	ContentHash        string                       `json:"content_hash"`
	SourcePublications []documentationManifestEntry `json:"source_publications,omitempty"`
}

type integrationSDKSnapshot struct {
	ID               string           `json:"id"`
	Ecosystem        string           `json:"ecosystem"`
	Coordinate       string           `json:"coordinate"`
	ExactVersion     string           `json:"exact_version"`
	InstallCommand   string           `json:"install_command"`
	DocumentationURL string           `json:"documentation_url,omitempty"`
	SourceURL        string           `json:"source_url,omitempty"`
	Checksum         string           `json:"checksum,omitempty"`
	Visibility       model.Visibility `json:"visibility"`
	Revision         int64            `json:"revision"`
	ContentHash      string           `json:"content_hash"`
}

type integrationAuthorizationSnapshot struct {
	ID                   string   `json:"id"`
	Key                  string   `json:"key"`
	Name                 string   `json:"name"`
	ActionType           string   `json:"action_type"`
	RequiredGrants       []string `json:"required_grants"`
	ConfirmationRequired bool     `json:"confirmation_required"`
	DecisionTTLSeconds   int      `json:"decision_ttl_seconds"`
	Revision             int64    `json:"revision"`
}

type integrationToolSnapshot struct {
	ToolID                     string `json:"tool_id"`
	ToolRevision               int64  `json:"tool_revision"`
	AuthorizationPointID       string `json:"authorization_point_id"`
	AuthorizationPointRevision int64  `json:"authorization_point_revision"`
	Scope                      string `json:"scope"`
	OwnerIntegrationID         string `json:"owner_integration_id,omitempty"`
	RuntimeServiceConnectionID string `json:"runtime_service_connection_id,omitempty"`
	Namespace                  string `json:"namespace"`
	Name                       string `json:"name"`
	BackendKind                string `json:"backend_kind"`
	Effect                     string `json:"effect"`
	IdempotencyMode            string `json:"idempotency_mode"`
	IdentityRequirement        string `json:"identity_requirement"`
	StateScope                 string `json:"state_scope"`
	MaxConcurrency             int    `json:"max_concurrency,omitempty"`
	MaxResultBytes             int64  `json:"max_result_bytes,omitempty"`
	ContentHash                string `json:"content_hash"`
	UpstreamSchemaHash         string `json:"upstream_schema_hash,omitempty"`
	NativePluginID             string `json:"native_plugin_id,omitempty"`
	NativeToolID               string `json:"native_tool_id,omitempty"`
	NativePluginVersion        string `json:"native_plugin_version,omitempty"`
	NativeSDKVersion           int    `json:"native_sdk_version,omitempty"`
	NativeManifestHash         string `json:"native_manifest_hash,omitempty"`
	NativeContractHash         string `json:"native_contract_hash,omitempty"`
}

type integrationServiceConnectionRevisionSnapshot struct {
	RevisionID         string          `json:"revision_id"`
	Revision           int64           `json:"revision"`
	EnvironmentID      string          `json:"environment_id"`
	BaseURL            string          `json:"base_url"`
	AuthenticationType string          `json:"authentication_type"`
	CredentialSetID    string          `json:"credential_set_id,omitempty"`
	AuthConfig         json.RawMessage `json:"auth_config"`
	ContentHash        string          `json:"content_hash"`
	Current            bool            `json:"current"`
	CredentialReady    bool            `json:"credential_ready"`
}

type integrationServiceConnectionSnapshot struct {
	ConnectionID       string                                         `json:"connection_id"`
	ConnectionRevision int64                                          `json:"connection_revision"`
	Name               string                                         `json:"name"`
	Description        string                                         `json:"description,omitempty"`
	State              string                                         `json:"state"`
	CurrentRevisions   []integrationServiceConnectionRevisionSnapshot `json:"current_revisions"`
}

type integrationSnapshot struct {
	FamilyKey                string                                 `json:"family_key"`
	VersionKey               string                                 `json:"version_key"`
	DisplayName              string                                 `json:"display_name"`
	Description              string                                 `json:"description"`
	Visibility               model.Visibility                       `json:"visibility"`
	Lifecycle                string                                 `json:"lifecycle"`
	ReplacementIntegrationID string                                 `json:"replacement_integration_id,omitempty"`
	SunsetAt                 *time.Time                             `json:"sunset_at,omitempty"`
	Resources                []integrationResourceSnapshot          `json:"resource_sets"`
	SDKs                     []integrationSDKSnapshot               `json:"sdks"`
	AuthorizationPoints      []integrationAuthorizationSnapshot     `json:"authorization_points"`
	Tools                    []integrationToolSnapshot              `json:"tools"`
	ServiceConnections       []integrationServiceConnectionSnapshot `json:"service_connections"`
	DeveloperAssets          integrationDeveloperAssetSnapshot      `json:"developer_assets"`
}

type integrationPublicationInputSet struct {
	GrantDefinitions          []model.GrantDefinition
	AuthorizationPoints       []model.AuthorizationPoint
	ToolBindings              []model.IntegrationToolBinding
	RuntimeServiceConnections []model.RuntimeServiceConnection
	RuntimeCredentialSets     []model.RuntimeCredentialSet
	DeveloperAssets           integrationDeveloperAssetSnapshot
	DeveloperAssetValidations []IntegrationPublishValidation
}

func (s *Service) integrationPublicationInputs(ctx context.Context, integration model.Integration) (integrationPublicationInputSet, error) {
	var result integrationPublicationInputSet
	grants, err := s.store.GrantDefinitions(ctx, integration.DeploymentID)
	if err != nil {
		return result, err
	}
	result.GrantDefinitions = grants
	points, err := s.store.AuthorizationPoints(ctx, integration.ID)
	if err != nil {
		return result, err
	}
	result.AuthorizationPoints = points
	bindings, err := s.store.IntegrationToolBindings(ctx, integration.ID)
	if err != nil {
		return result, err
	}
	result.ToolBindings = bindings
	result.RuntimeServiceConnections, err = s.store.RuntimeServiceConnections(ctx, integration.DeploymentID, integration.ID)
	if err != nil {
		return result, err
	}
	result.RuntimeCredentialSets, err = s.store.RuntimeCredentialSets(ctx, integration.DeploymentID, "")
	if err != nil {
		return result, err
	}
	result.DeveloperAssets, result.DeveloperAssetValidations, err = s.resolveIntegrationDeveloperAssets(ctx, integration)
	if err != nil {
		return result, err
	}
	return result, nil
}

func buildIntegrationSnapshot(integration model.Integration, inputs integrationPublicationInputSet) (json.RawMessage, []IntegrationPublishValidation, error) {
	grantDefinitions, authorizationPoints, toolBindings := inputs.GrantDefinitions, inputs.AuthorizationPoints, inputs.ToolBindings
	validations := make([]IntegrationPublishValidation, 0)
	validations = append(validations, inputs.DeveloperAssetValidations...)
	resources := make([]integrationResourceSnapshot, 0, len(integration.Resources))
	for _, link := range integration.Resources {
		if link.ResolvedRevision == nil {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "resource_revision_unresolved", Message: fmt.Sprintf("%s has no resolvable revision", link.Name), Tab: "resources"})
			continue
		}
		resource := integrationResourceSnapshot{SetID: link.ResourceSetID, Kind: link.Kind, Name: link.Name, RevisionID: link.ResolvedRevision.ID, Revision: link.ResolvedRevision.Revision, ContentHash: link.ResolvedRevision.ContentHash}
		if link.Kind == "documentation" {
			publications, parseErr := parseDocumentationManifest(link.ResolvedRevision.Manifest)
			if parseErr != nil || len(publications) == 0 {
				validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "documentation_publication_unresolved", Message: fmt.Sprintf("%s must pin at least one exact reviewed source publication", link.Name), Tab: "resources"})
				continue
			}
			resource.SourcePublications = publications
		}
		resources = append(resources, resource)
	}
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Kind == resources[j].Kind {
			return resources[i].SetID < resources[j].SetID
		}
		return resources[i].Kind < resources[j].Kind
	})
	if len(integration.Resources) == 0 {
		validations = append(validations, IntegrationPublishValidation{Level: "warning", Code: "resources_missing", Message: "No documentation or API set is attached.", Tab: "resources"})
	}
	// Typed SDK assets are the canonical publication evidence. The deprecated
	// SDK-reference list is only a compatibility projection of those bindings,
	// so omit matching IDs instead of publishing the same release twice.
	typedSDKBindingIDs := make(map[string]bool, len(inputs.DeveloperAssets.SDKs))
	for _, asset := range inputs.DeveloperAssets.SDKs {
		typedSDKBindingIDs[asset.BindingID] = true
	}
	sdks := make([]integrationSDKSnapshot, 0, len(integration.SDKs))
	for _, reference := range integration.SDKs {
		if typedSDKBindingIDs[reference.ID] {
			continue
		}
		if reference.ID == "" || reference.Revision < 1 || reference.IntegrationID != integration.ID || reference.DeploymentID != integration.DeploymentID {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "sdk_reference_unresolved", Message: "An SDK reference does not resolve to this API.", Tab: "resources"})
			continue
		}
		if integration.Visibility == model.VisibilityPublic && reference.Visibility != model.VisibilityPublic {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "private_sdk_on_public_integration", Message: fmt.Sprintf("%s is private and cannot be published on a public API", reference.Coordinate), Tab: "resources"})
			continue
		}
		canonical, err := json.Marshal(map[string]any{"id": reference.ID, "ecosystem": reference.Ecosystem, "coordinate": reference.Coordinate, "exact_version": reference.ExactVersion, "install_command": reference.InstallCommand, "documentation_url": reference.DocumentationURL, "source_url": reference.SourceURL, "checksum": reference.Checksum, "visibility": reference.Visibility, "revision": reference.Revision})
		if err != nil {
			return nil, validations, err
		}
		sdks = append(sdks, integrationSDKSnapshot{ID: reference.ID, Ecosystem: reference.Ecosystem, Coordinate: reference.Coordinate, ExactVersion: reference.ExactVersion, InstallCommand: reference.InstallCommand, DocumentationURL: reference.DocumentationURL, SourceURL: reference.SourceURL, Checksum: reference.Checksum, Visibility: reference.Visibility, Revision: reference.Revision, ContentHash: contentHash(canonical)})
	}
	sort.Slice(sdks, func(i, j int) bool {
		if sdks[i].Ecosystem == sdks[j].Ecosystem {
			return sdks[i].Coordinate < sdks[j].Coordinate
		}
		return sdks[i].Ecosystem < sdks[j].Ecosystem
	})
	authorization := make([]integrationAuthorizationSnapshot, 0, len(authorizationPoints))
	for _, point := range authorizationPoints {
		if point.State == "draft" {
			validations = append(validations, IntegrationPublishValidation{Level: "warning", Code: "authorization_point_draft", Message: fmt.Sprintf("%s is still a draft and will not be published", point.Name), Tab: "authorization"})
			continue
		}
		if point.State != "active" {
			continue
		}
		if missing := missingRegisteredGrants(grantDefinitions, point.RequiredGrants); len(missing) > 0 {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "authorization_grant_unregistered", Message: fmt.Sprintf("%s references unregistered grants: %s", point.Name, strings.Join(missing, ", ")), Tab: "authorization"})
			continue
		}
		authorization = append(authorization, integrationAuthorizationSnapshot{ID: point.ID, Key: point.Key, Name: point.Name, ActionType: point.ActionType, RequiredGrants: append([]string(nil), point.RequiredGrants...), ConfirmationRequired: point.ConfirmationRequired, DecisionTTLSeconds: point.DecisionTTLSeconds, Revision: point.Revision})
	}
	sort.Slice(authorization, func(i, j int) bool { return authorization[i].Key < authorization[j].Key })
	if integration.Visibility == model.VisibilityPrivate && len(authorization) == 0 {
		validations = append(validations, IntegrationPublishValidation{Level: "warning", Code: "authorization_missing", Message: "No active authorization point is configured for this private API.", Tab: "authorization"})
	}
	boundTools := make([]integrationToolSnapshot, 0, len(toolBindings))
	for _, binding := range toolBindings {
		if binding.Tool == nil || binding.Tool.ID != binding.ToolID || binding.Tool.State != "published" || binding.Tool.Revision != binding.ToolRevision {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "tool_revision_unresolved", Message: "A tool binding does not resolve to its exact published revision.", Tab: "tools"})
			continue
		}
		if binding.AuthorizationPoint == nil || binding.AuthorizationPoint.ID != binding.AuthorizationPointID || binding.AuthorizationPoint.IntegrationID != integration.ID || binding.AuthorizationPoint.State != "active" || binding.AuthorizationPoint.Revision != binding.AuthorizationPointRevision {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "tool_authorization_unresolved", Message: "A tool binding does not resolve to its exact active authorization-point revision.", Tab: "tools"})
			continue
		}
		tool := binding.Tool
		if err := validateToolBindingOwnership(*tool, integration); err != nil {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "tool_ownership_invalid", Message: err.Error(), Tab: "tools"})
			continue
		}
		definition, err := json.Marshal(map[string]any{"id": tool.ID, "revision": tool.Revision, "scope": tool.Scope, "owner_integration_id": tool.OwnerIntegrationID, "runtime_service_connection_id": tool.RuntimeServiceConnectionID, "http_path": tool.HTTPPath, "namespace": tool.Namespace, "name": tool.Name, "description": tool.Description, "input_schema": tool.InputSchema, "output_schema": tool.OutputSchema, "http_method": tool.HTTPMethod, "authorization_policy": tool.AuthorizationPolicy, "timeout_ms": tool.TimeoutMS, "backend_kind": tool.BackendKind, "effect": tool.Effect, "idempotency_mode": tool.IdempotencyMode, "identity_requirement": tool.IdentityRequirement, "state_scope": tool.StateScope, "max_concurrency": tool.MaxConcurrency, "max_result_bytes": tool.MaxResultBytes, "mcp_connection_id": tool.MCPConnectionID, "upstream_tool_name": tool.UpstreamToolName, "upstream_schema_hash": tool.UpstreamSchemaHash, "native_plugin_id": tool.NativePluginID, "native_tool_id": tool.NativeToolID, "native_plugin_version": tool.NativePluginVersion, "native_sdk_version": tool.NativeSDKVersion, "native_manifest_hash": tool.NativeManifestHash, "native_contract_hash": tool.NativeContractHash})
		if err != nil {
			return nil, validations, err
		}
		boundTools = append(boundTools, integrationToolSnapshot{ToolID: tool.ID, ToolRevision: tool.Revision, AuthorizationPointID: binding.AuthorizationPointID, AuthorizationPointRevision: binding.AuthorizationPointRevision, Scope: tool.Scope, OwnerIntegrationID: tool.OwnerIntegrationID, RuntimeServiceConnectionID: tool.RuntimeServiceConnectionID, Namespace: tool.Namespace, Name: tool.Name, BackendKind: tool.BackendKind, Effect: tool.Effect, IdempotencyMode: tool.IdempotencyMode, IdentityRequirement: tool.IdentityRequirement, StateScope: tool.StateScope, MaxConcurrency: tool.MaxConcurrency, MaxResultBytes: tool.MaxResultBytes, ContentHash: contentHash(definition), UpstreamSchemaHash: tool.UpstreamSchemaHash, NativePluginID: tool.NativePluginID, NativeToolID: tool.NativeToolID, NativePluginVersion: tool.NativePluginVersion, NativeSDKVersion: tool.NativeSDKVersion, NativeManifestHash: tool.NativeManifestHash, NativeContractHash: tool.NativeContractHash})
	}
	sort.Slice(boundTools, func(i, j int) bool {
		if boundTools[i].Namespace == boundTools[j].Namespace {
			return boundTools[i].Name < boundTools[j].Name
		}
		return boundTools[i].Namespace < boundTools[j].Namespace
	})
	if len(boundTools) == 0 {
		validations = append(validations, IntegrationPublishValidation{Level: "warning", Code: "tools_missing", Message: "No reviewed tool revision is bound to this API.", Tab: "tools"})
	}
	credentialSets := runtimeCredentialSetIndex(inputs.RuntimeCredentialSets)
	serviceConnections := make([]integrationServiceConnectionSnapshot, 0, len(inputs.RuntimeServiceConnections))
	seenServiceConnections := make(map[string]bool, len(inputs.RuntimeServiceConnections))
	readyServiceConnections := 0
	for _, connection := range inputs.RuntimeServiceConnections {
		readiness := runtimeServiceConnectionReadinessFrom(connection, credentialSets)
		configurationReady, credentialsReady := true, true
		for _, check := range readiness.Checks {
			if check.Ready {
				continue
			}
			if check.Key == "credential" {
				credentialsReady = false
			} else {
				configurationReady = false
			}
		}
		if connection.ID == "" || seenServiceConnections[connection.ID] || connection.DeploymentID != integration.DeploymentID || connection.OrganisationID != integration.OrganisationID || connection.IntegrationID != integration.ID {
			configurationReady = false
		}
		seenServiceConnections[connection.ID] = true

		revisions := append([]model.RuntimeServiceConnectionRevision(nil), connection.CurrentRevisions...)
		sort.Slice(revisions, func(i, j int) bool {
			if revisions[i].EnvironmentID == revisions[j].EnvironmentID {
				if revisions[i].Revision == revisions[j].Revision {
					return revisions[i].ID < revisions[j].ID
				}
				return revisions[i].Revision < revisions[j].Revision
			}
			return revisions[i].EnvironmentID < revisions[j].EnvironmentID
		})
		currentRevisions := make([]integrationServiceConnectionRevisionSnapshot, 0, len(revisions))
		for _, revision := range revisions {
			normalized, exact := normalizedRuntimeServiceConnectionRevision(connection, revision)
			if !exact {
				configurationReady = false
				continue
			}
			credentialReady := runtimeServiceCredentialReady(connection, revision, credentialSets)
			currentRevisions = append(currentRevisions, integrationServiceConnectionRevisionSnapshot{
				RevisionID:         revision.ID,
				Revision:           revision.Revision,
				EnvironmentID:      revision.EnvironmentID,
				BaseURL:            normalized.BaseURL,
				AuthenticationType: normalized.AuthenticationType,
				CredentialSetID:    normalized.CredentialSetID,
				AuthConfig:         normalized.AuthConfig,
				ContentHash:        revision.ContentHash,
				Current:            revision.Current,
				CredentialReady:    credentialReady,
			})
		}
		serviceConnections = append(serviceConnections, integrationServiceConnectionSnapshot{
			ConnectionID:       connection.ID,
			ConnectionRevision: connection.Revision,
			Name:               connection.Name,
			Description:        connection.Description,
			State:              connection.State,
			CurrentRevisions:   currentRevisions,
		})
		if configurationReady && credentialsReady && readiness.Ready {
			readyServiceConnections++
		}
		if !configurationReady {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "runtime_service_connection_unresolved", Message: fmt.Sprintf("%s does not resolve to exact active API-owned runtime connection revisions.", connection.Name), Tab: "access"})
		}
		if !credentialsReady {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "runtime_service_credential_unavailable", Message: fmt.Sprintf("%s requires an active compatible runtime credential for every configured environment.", connection.Name), Tab: "access"})
		}
	}
	sort.Slice(serviceConnections, func(i, j int) bool { return serviceConnections[i].ConnectionID < serviceConnections[j].ConnectionID })
	if readyServiceConnections == 0 {
		validations = append(validations, IntegrationPublishValidation{Level: "warning", Code: "access_missing", Message: "No publish-ready API-owned runtime service connection is configured.", Tab: "access"})
	}

	snapshot, err := json.Marshal(integrationSnapshot{FamilyKey: integration.FamilyKey, VersionKey: integration.VersionKey, DisplayName: integration.DisplayName, Description: integration.Description, Visibility: integration.Visibility, Lifecycle: integration.Lifecycle, ReplacementIntegrationID: integration.ReplacementIntegrationID, SunsetAt: integration.SunsetAt, Resources: resources, SDKs: sdks, AuthorizationPoints: authorization, Tools: boundTools, ServiceConnections: serviceConnections, DeveloperAssets: inputs.DeveloperAssets})
	return snapshot, validations, err
}

func (s *Service) validateIntegrationDocumentationPublications(ctx context.Context, integration model.Integration) []IntegrationPublishValidation {
	validations := make([]IntegrationPublishValidation, 0)
	seen := make(map[string]bool)
	for _, link := range integration.Resources {
		if link.Kind != "documentation" || link.ResolvedRevision == nil {
			continue
		}
		entries, err := parseDocumentationManifest(link.ResolvedRevision.Manifest)
		if err != nil {
			continue // buildIntegrationSnapshot reports the malformed manifest.
		}
		for _, entry := range entries {
			if seen[entry.SourcePublicationID] {
				continue
			}
			seen[entry.SourcePublicationID] = true
			publication, publicationErr := s.store.SourcePublication(ctx, integration.DeploymentID, entry.SourcePublicationID)
			if publicationErr != nil || publication.SourceID != entry.SourceID || publication.Revision != entry.Revision || publication.ContentHash != entry.ContentHash {
				validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "documentation_publication_stale", Message: fmt.Sprintf("%s references a documentation publication that no longer resolves exactly", link.Name), Tab: "resources"})
				continue
			}
			source, sourceErr := s.store.Source(ctx, integration.DeploymentID, publication.SourceID)
			if sourceErr != nil || !source.Published || source.Quarantined {
				validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "documentation_source_unavailable", Message: fmt.Sprintf("%s references documentation that is no longer in a published, non-quarantined state", link.Name), Tab: "resources"})
				continue
			}
			if integration.Visibility == model.VisibilityPublic && (publication.Visibility != model.VisibilityPublic || source.Visibility != model.VisibilityPublic) {
				validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "documentation_not_public", Message: fmt.Sprintf("%s references documentation that was not explicitly reviewed and published for public discovery", link.Name), Tab: "resources"})
			}
		}
	}
	return validations
}

func publishChanges(previous, current json.RawMessage) []IntegrationPublishChange {
	var before, after map[string]any
	_ = json.Unmarshal(previous, &before)
	_ = json.Unmarshal(current, &after)
	fields := []string{"family_key", "version_key", "display_name", "description", "visibility", "lifecycle", "replacement_integration_id", "sunset_at", "resource_sets", "sdks", "authorization_points", "tools", "service_connections", "developer_assets", "support_route_id"}
	changes := make([]IntegrationPublishChange, 0)
	for _, field := range fields {
		beforeJSON, _ := json.Marshal(before[field])
		afterJSON, _ := json.Marshal(after[field])
		if string(beforeJSON) != string(afterJSON) {
			changes = append(changes, IntegrationPublishChange{Field: field, Before: before[field], After: after[field]})
		}
	}
	return changes
}

func (s *Service) IntegrationPublishStatus(ctx context.Context, integrationID string) (IntegrationPublishStatus, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return IntegrationPublishStatus{}, err
	}
	integration, err := s.store.Integration(ctx, deployment.ID, integrationID)
	if err != nil {
		return IntegrationPublishStatus{}, err
	}
	if integration.Lifecycle == "draft" {
		integration.Lifecycle = "active"
	}
	inputs, err := s.integrationPublicationInputs(ctx, integration)
	if err != nil {
		return IntegrationPublishStatus{}, err
	}
	snapshot, validations, err := buildIntegrationSnapshot(integration, inputs)
	if err != nil {
		return IntegrationPublishStatus{}, err
	}
	validations = append(validations, s.validateIntegrationDocumentationPublications(ctx, integration)...)
	revisions, err := s.store.IntegrationRevisions(ctx, integration.ID)
	if err != nil {
		return IntegrationPublishStatus{}, err
	}
	var latest *model.IntegrationRevision
	for index := range revisions {
		if revisions[index].State == "published" && (latest == nil || revisions[index].Revision > latest.Revision) {
			copy := revisions[index]
			latest = &copy
		}
	}
	hash := contentHash(snapshot)
	changes := publishChanges(nil, snapshot)
	if latest != nil {
		changes = publishChanges(latest.Snapshot, snapshot)
	}
	ready := true
	for _, validation := range validations {
		if validation.Level == "error" {
			ready = false
			break
		}
	}
	return IntegrationPublishStatus{Ready: ready, HasChanges: latest == nil || latest.ManifestHash != hash, CurrentManifestHash: hash, CurrentSnapshot: snapshot, LatestRevision: latest, Changes: changes, Validations: validations}, nil
}

func (s *Service) activateIntegrationPublication(
	ctx context.Context,
	deployment model.Deployment,
	integration model.Integration,
	revision model.IntegrationRevision,
	assetPublication model.APIDeveloperAssetPublication,
	actor Actor,
) error {
	createdAt := s.now()
	if revision.PublishedAt != nil {
		createdAt = *revision.PublishedAt
	}
	current := map[string]any{
		"integration_id": integration.ID, "revision": revision.Revision, "manifest_hash": revision.ManifestHash,
		"developer_asset_publication_id": assetPublication.ID, "developer_asset_snapshot_hash": assetPublication.SnapshotHash,
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{
		ID: "integration-publication:" + revision.ID, OrganisationID: deployment.OrganisationID,
		ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.published",
		TargetType: "integration_revision", TargetID: revision.ID, Current: current,
		RequestID: actor.RequestID, Outcome: "success", CreatedAt: createdAt,
	}); err != nil {
		return err
	}
	if err := s.recordDeveloperAssetPublicationActivation(ctx, deployment, actor, "api", assetPublication.ID, current); err != nil {
		return err
	}
	_, err := s.BuildDeveloperAssetSearchIndex(ctx, "api", assetPublication.ID)
	return err
}

func (s *Service) PublishIntegration(ctx context.Context, integrationID string, actor Actor) (model.IntegrationRevision, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.IntegrationRevision{}, err
	}
	integration, err := s.store.Integration(ctx, deployment.ID, integrationID)
	if err != nil {
		return model.IntegrationRevision{}, err
	}
	preflight, err := s.IntegrationPreflight(ctx, integration.ID)
	if err != nil {
		return model.IntegrationRevision{}, err
	}
	if !preflight.Ready {
		return model.IntegrationRevision{}, integrationPreflightError(preflight)
	}
	if integration.Lifecycle == "draft" {
		integration.Lifecycle = "active"
		integration, err = s.store.UpdateIntegration(ctx, integration, integration.Revision)
		if err != nil {
			return model.IntegrationRevision{}, err
		}
	}
	inputs, err := s.integrationPublicationInputs(ctx, integration)
	if err != nil {
		return model.IntegrationRevision{}, err
	}
	snapshot, validations, err := buildIntegrationSnapshot(integration, inputs)
	if err != nil {
		return model.IntegrationRevision{}, err
	}
	validations = append(validations, s.validateIntegrationDocumentationPublications(ctx, integration)...)
	for _, validation := range validations {
		if validation.Level == "error" {
			return model.IntegrationRevision{}, errors.New(validation.Message)
		}
	}
	revisions, err := s.store.IntegrationRevisions(ctx, integration.ID)
	if err != nil {
		return model.IntegrationRevision{}, err
	}
	manifestHash := contentHash(snapshot)
	if expectation, ok := integrationPublishExpectationFromContext(ctx); ok && (integration.Revision != expectation.CandidateRevision || manifestHash != expectation.ManifestHash) {
		return model.IntegrationRevision{}, errors.New("the Integration candidate changed after preflight; run preflight again")
	}
	nextRevision := int64(1)
	for _, existing := range revisions {
		if existing.Revision >= nextRevision {
			nextRevision = existing.Revision + 1
		}
		if existing.State == "published" && existing.ManifestHash == manifestHash {
			assetPublication, ensureErr := s.ensureAPIDeveloperAssetPublication(ctx, integration, existing, inputs.DeveloperAssets, actor)
			if ensureErr != nil {
				return model.IntegrationRevision{}, ensureErr
			}
			if err := s.activateIntegrationPublication(ctx, deployment, integration, existing, assetPublication, actor); err != nil {
				return model.IntegrationRevision{}, err
			}
			return existing, nil
		}
	}
	id, err := randomUUID()
	if err != nil {
		return model.IntegrationRevision{}, err
	}
	now := s.now()
	revision, err := s.store.CreateIntegrationRevision(ctx, model.IntegrationRevision{ID: id, IntegrationID: integration.ID, Revision: nextRevision, State: "published", Snapshot: snapshot, ManifestHash: manifestHash, PublishedBy: actor.ID, PublishedAt: &now})
	if err != nil {
		if err == store.ErrConflict {
			return model.IntegrationRevision{}, errors.New("this integration revision is already published")
		}
		return model.IntegrationRevision{}, err
	}
	assetPublication, err := s.ensureAPIDeveloperAssetPublication(ctx, integration, revision, inputs.DeveloperAssets, actor)
	if err != nil {
		return model.IntegrationRevision{}, err
	}
	if err := s.activateIntegrationPublication(ctx, deployment, integration, revision, assetPublication, actor); err != nil {
		return model.IntegrationRevision{}, err
	}
	return revision, nil
}
