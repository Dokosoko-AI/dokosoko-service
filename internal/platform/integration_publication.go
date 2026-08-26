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

type integrationPackageSnapshot struct {
	PackageArtifactID            string           `json:"package_artifact_id"`
	PackageReleaseID             string           `json:"package_release_id"`
	Name                         string           `json:"name"`
	Ecosystem                    string           `json:"ecosystem"`
	Coordinate                   string           `json:"coordinate"`
	Version                      string           `json:"version"`
	PURL                         string           `json:"purl"`
	RegistryURL                  string           `json:"registry_url"`
	SourceURL                    string           `json:"source_url,omitempty"`
	Language                     string           `json:"language,omitempty"`
	Platform                     string           `json:"platform,omitempty"`
	InstallCommand               string           `json:"install_command"`
	Digest                       string           `json:"digest"`
	ProvenanceURL                string           `json:"provenance_url,omitempty"`
	SBOMURL                      string           `json:"sbom_url,omitempty"`
	Visibility                   model.Visibility `json:"visibility"`
	Lifecycle                    string           `json:"lifecycle"`
	ReplacementPackageArtifactID string           `json:"replacement_package_artifact_id,omitempty"`
	DeprecationMessage           string           `json:"deprecation_message,omitempty"`
	SunsetAt                     *time.Time       `json:"sunset_at,omitempty"`
	ContentHash                  string           `json:"content_hash"`
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

type integrationAccessSnapshot struct {
	ConnectionID             string `json:"connection_id"`
	ConnectionRevision       int64  `json:"connection_revision"`
	AccessDefinitionID       string `json:"access_definition_id"`
	AccessDefinitionRevision int64  `json:"access_definition_revision"`
	EnvironmentID            string `json:"environment_id,omitempty"`
	State                    string `json:"state"`
	ContentHash              string `json:"content_hash"`
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
	Packages                 []integrationPackageSnapshot           `json:"packages"`
	AuthorizationPoints      []integrationAuthorizationSnapshot     `json:"authorization_points"`
	Tools                    []integrationToolSnapshot              `json:"tools"`
	ServiceConnections       []integrationServiceConnectionSnapshot `json:"service_connections"`
	AccessConnections        []integrationAccessSnapshot            `json:"access_connections"`
	AccessConnectionIDs      []string                               `json:"access_connection_ids,omitempty"`
	SupportRouteID           string                                 `json:"support_route_id,omitempty"`
}

type integrationPublicationInputSet struct {
	GrantDefinitions              []model.GrantDefinition
	AuthorizationPoints           []model.AuthorizationPoint
	ToolBindings                  []model.IntegrationToolBinding
	ProviderManagementConnections []model.AccessConnection
	RuntimeServiceConnections     []model.RuntimeServiceConnection
	RuntimeCredentialSets         []model.RuntimeCredentialSet
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
	connections := make([]model.AccessConnection, 0, len(integration.AccessConnections))
	for _, connectionID := range integration.AccessConnections {
		connection, connectionErr := s.store.AccessConnection(ctx, integration.DeploymentID, connectionID)
		if connectionErr != nil {
			return result, connectionErr
		}
		connections = append(connections, connection)
	}
	result.ProviderManagementConnections = connections
	result.RuntimeServiceConnections, err = s.store.RuntimeServiceConnections(ctx, integration.DeploymentID, integration.ID)
	if err != nil {
		return result, err
	}
	result.RuntimeCredentialSets, err = s.store.RuntimeCredentialSets(ctx, integration.DeploymentID, "")
	if err != nil {
		return result, err
	}
	return result, nil
}

func buildIntegrationSnapshot(integration model.Integration, inputs integrationPublicationInputSet, now time.Time) (json.RawMessage, []IntegrationPublishValidation, error) {
	grantDefinitions, authorizationPoints, toolBindings := inputs.GrantDefinitions, inputs.AuthorizationPoints, inputs.ToolBindings
	validations := make([]IntegrationPublishValidation, 0)
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
	packages := make([]integrationPackageSnapshot, 0, len(integration.Packages))
	for _, binding := range integration.Packages {
		if binding.Artifact == nil || binding.Artifact.ID != binding.PackageArtifactID || binding.Release == nil || binding.Release.PackageArtifactID != binding.PackageArtifactID || binding.Release.ID != binding.PackageReleaseID {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "package_release_unresolved", Message: "A package binding has no resolvable exact release.", Tab: "resources"})
			continue
		}
		if integration.Visibility == model.VisibilityPublic && binding.Release.Visibility != model.VisibilityPublic {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "private_package_on_public_integration", Message: fmt.Sprintf("%s is private and cannot be published on a public integration", binding.Release.ArtifactName), Tab: "resources"})
			continue
		}
		if unavailable := packageArtifactUnavailableMessage(*binding.Artifact, now); unavailable != "" {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "package_artifact_unavailable", Message: unavailable, Tab: "resources"})
		}
		release := binding.Release
		artifact := binding.Artifact
		packages = append(packages, integrationPackageSnapshot{PackageArtifactID: binding.PackageArtifactID, PackageReleaseID: binding.PackageReleaseID, Name: release.ArtifactName, Ecosystem: release.Ecosystem, Coordinate: release.Coordinate, Version: release.Version, PURL: release.PURL, RegistryURL: release.RegistryURL, SourceURL: release.SourceURL, Language: release.Language, Platform: release.Platform, InstallCommand: release.InstallCommand, Digest: release.Digest, ProvenanceURL: release.ProvenanceURL, SBOMURL: release.SBOMURL, Visibility: release.Visibility, Lifecycle: artifact.Lifecycle, ReplacementPackageArtifactID: artifact.ReplacementPackageArtifactID, DeprecationMessage: artifact.DeprecationMessage, SunsetAt: artifact.SunsetAt, ContentHash: release.ContentHash})
	}
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Ecosystem == packages[j].Ecosystem {
			if packages[i].Coordinate == packages[j].Coordinate {
				return packages[i].PackageReleaseID < packages[j].PackageReleaseID
			}
			return packages[i].Coordinate < packages[j].Coordinate
		}
		return packages[i].Ecosystem < packages[j].Ecosystem
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

	accessConnections := make([]integrationAccessSnapshot, 0, len(inputs.ProviderManagementConnections))
	seenAccessConnections := make(map[string]bool, len(inputs.ProviderManagementConnections))
	for _, connection := range inputs.ProviderManagementConnections {
		definition := connection.Definition
		if connection.ID == "" || seenAccessConnections[connection.ID] || connection.Revision < 1 || connection.State != "active" || definition == nil || definition.ID != connection.AccessDefinitionID || definition.Revision < 1 || definition.State != "active" {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "provider_management_connection_unresolved", Message: "An optional provider-management connection does not resolve to one exact active connection and service-definition revision.", Tab: "access"})
			continue
		}
		seenAccessConnections[connection.ID] = true
		// ManagementSecretID is intentionally excluded. Config is the bounded,
		// non-secret JSON configuration accepted separately from management
		// credentials; only its canonical digest enters the public manifest.
		canonical, err := json.Marshal(struct {
			ConnectionID             string          `json:"connection_id"`
			ConnectionRevision       int64           `json:"connection_revision"`
			AccessDefinitionID       string          `json:"access_definition_id"`
			AccessDefinitionRevision int64           `json:"access_definition_revision"`
			EnvironmentID            string          `json:"environment_id,omitempty"`
			Name                     string          `json:"name"`
			Region                   string          `json:"region,omitempty"`
			BaseURL                  string          `json:"base_url"`
			Config                   json.RawMessage `json:"config"`
			State                    string          `json:"state"`
			ServiceKey               string          `json:"service_key"`
			InstanceCardinality      string          `json:"instance_cardinality"`
			CredentialScope          string          `json:"credential_scope"`
			ManagementAuthType       string          `json:"management_auth_type"`
			APIResourceSetID         string          `json:"api_resource_set_id,omitempty"`
			Operations               json.RawMessage `json:"operations"`
			DefinitionState          string          `json:"definition_state"`
		}{ConnectionID: connection.ID, ConnectionRevision: connection.Revision, AccessDefinitionID: definition.ID, AccessDefinitionRevision: definition.Revision, EnvironmentID: connection.EnvironmentID, Name: connection.Name, Region: connection.Region, BaseURL: connection.BaseURL, Config: connection.Config, State: connection.State, ServiceKey: definition.ServiceKey, InstanceCardinality: definition.InstanceCardinality, CredentialScope: definition.CredentialScope, ManagementAuthType: definition.ManagementAuthType, APIResourceSetID: definition.APIResourceSetID, Operations: definition.Operations, DefinitionState: definition.State})
		if err != nil {
			return nil, validations, err
		}
		accessConnections = append(accessConnections, integrationAccessSnapshot{ConnectionID: connection.ID, ConnectionRevision: connection.Revision, AccessDefinitionID: definition.ID, AccessDefinitionRevision: definition.Revision, EnvironmentID: connection.EnvironmentID, State: connection.State, ContentHash: contentHash(canonical)})
	}
	sort.Slice(accessConnections, func(i, j int) bool { return accessConnections[i].ConnectionID < accessConnections[j].ConnectionID })
	if len(accessConnections) != len(integration.AccessConnections) {
		validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "provider_management_connection_selection_incomplete", Message: "Every selected optional provider-management connection must resolve to an exact immutable manifest entry.", Tab: "access"})
	}
	if integration.SupportRouteID == "" {
		validations = append(validations, IntegrationPublishValidation{Level: "warning", Code: "support_inherited", Message: "Bug reports and feedback use the deployment default, if one is configured.", Tab: "overview"})
	}
	snapshot, err := json.Marshal(integrationSnapshot{FamilyKey: integration.FamilyKey, VersionKey: integration.VersionKey, DisplayName: integration.DisplayName, Description: integration.Description, Visibility: integration.Visibility, Lifecycle: integration.Lifecycle, ReplacementIntegrationID: integration.ReplacementIntegrationID, SunsetAt: integration.SunsetAt, Resources: resources, Packages: packages, AuthorizationPoints: authorization, Tools: boundTools, ServiceConnections: serviceConnections, AccessConnections: accessConnections, AccessConnectionIDs: integration.AccessConnections, SupportRouteID: integration.SupportRouteID})
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
	fields := []string{"family_key", "version_key", "display_name", "description", "visibility", "lifecycle", "replacement_integration_id", "sunset_at", "resource_sets", "packages", "authorization_points", "tools", "service_connections", "access_connections", "access_connection_ids", "support_route_id"}
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
	snapshot, validations, err := buildIntegrationSnapshot(integration, inputs, s.now())
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
	snapshot, validations, err := buildIntegrationSnapshot(integration, inputs, s.now())
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
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.published", TargetType: "integration_revision", TargetID: revision.ID, Current: map[string]any{"integration_id": integration.ID, "revision": revision.Revision, "manifest_hash": revision.ManifestHash}, RequestID: actor.RequestID, CreatedAt: now}); err != nil {
		return model.IntegrationRevision{}, err
	}
	return revision, nil
}
