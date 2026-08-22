package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var integrationVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type DeploymentInput struct {
	OrganisationID           string
	Name                     string
	Slug                     string
	Description              string
	DefaultReleasePolicy     string
	RequirePromotionApproval bool
	PublicMCPEnabled         bool
	Revision                 int64
}

func (s *Service) CreateDeployment(ctx context.Context, input DeploymentInput, actor Actor) (model.Deployment, error) {
	input.OrganisationID, input.Name, input.Slug = strings.TrimSpace(input.OrganisationID), strings.TrimSpace(input.Name), strings.TrimSpace(input.Slug)
	input.Description = strings.TrimSpace(input.Description)
	if input.OrganisationID == "" || validateNameSlug(input.Name, input.Slug) != nil {
		return model.Deployment{}, errors.New("organisation, deployment name, and a valid slug are required")
	}
	if len(input.Description) > 2000 {
		return model.Deployment{}, errors.New("deployment description must be no more than 2000 characters")
	}
	if input.DefaultReleasePolicy == "" {
		input.DefaultReleasePolicy = "latest"
	}
	if input.DefaultReleasePolicy != "latest" && input.DefaultReleasePolicy != "lts" {
		return model.Deployment{}, errors.New("default release policy must be latest or lts")
	}
	id, err := randomUUID()
	if err != nil {
		return model.Deployment{}, err
	}
	value, err := s.store.CreateDeployment(ctx, model.Deployment{ID: id, OrganisationID: input.OrganisationID, Name: input.Name, Slug: input.Slug, Description: input.Description, DefaultReleasePolicy: input.DefaultReleasePolicy, RequirePromotionApproval: input.RequirePromotionApproval, PublicMCPEnabled: input.PublicMCPEnabled})
	if err != nil {
		return model.Deployment{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: value.OrganisationID, ProductID: value.ID, ActorID: actor.ID, Action: "deployment.created", TargetType: "deployment", TargetID: value.ID, Current: map[string]any{"name": value.Name, "slug": value.Slug}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, nil
}

func (s *Service) UpdateDeployment(ctx context.Context, input DeploymentInput, actor Actor) (model.Deployment, error) {
	current, err := s.store.Deployment(ctx)
	if err != nil {
		return model.Deployment{}, err
	}
	input.Name, input.Slug, input.Description = strings.TrimSpace(input.Name), strings.TrimSpace(input.Slug), strings.TrimSpace(input.Description)
	if validateNameSlug(input.Name, input.Slug) != nil || len(input.Description) > 2000 {
		return model.Deployment{}, errors.New("deployment name, slug, or description is invalid")
	}
	if input.DefaultReleasePolicy != "latest" && input.DefaultReleasePolicy != "lts" {
		return model.Deployment{}, errors.New("default release policy must be latest or lts")
	}
	current.Name, current.Slug, current.Description = input.Name, input.Slug, input.Description
	current.DefaultReleasePolicy, current.RequirePromotionApproval, current.PublicMCPEnabled = input.DefaultReleasePolicy, input.RequirePromotionApproval, input.PublicMCPEnabled
	updated, err := s.store.UpdateDeployment(ctx, current, input.Revision)
	if err != nil {
		return model.Deployment{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.ID, ActorID: actor.ID, Action: "deployment.updated", TargetType: "deployment", TargetID: updated.ID, Current: map[string]any{"name": updated.Name, "slug": updated.Slug, "default_release_policy": updated.DefaultReleasePolicy}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

type IntegrationInput struct {
	FamilyKey                string
	VersionKey               string
	DisplayName              string
	Description              string
	Visibility               model.Visibility
	AcknowledgePublic        bool
	Lifecycle                string
	ReplacementIntegrationID string
	SunsetAt                 *time.Time
	Revision                 int64
}

func normalizeIntegrationInput(input IntegrationInput) (IntegrationInput, error) {
	input.FamilyKey, input.VersionKey = strings.ToLower(strings.TrimSpace(input.FamilyKey)), strings.TrimSpace(input.VersionKey)
	input.DisplayName, input.Description = strings.TrimSpace(input.DisplayName), strings.TrimSpace(input.Description)
	input.ReplacementIntegrationID = strings.TrimSpace(input.ReplacementIntegrationID)
	if !slugPattern.MatchString(input.FamilyKey) || len(input.FamilyKey) > 63 {
		return input, errors.New("integration family key must use lower-case letters, numbers, and single hyphens")
	}
	if !integrationVersionPattern.MatchString(input.VersionKey) {
		return input, errors.New("integration version key is invalid")
	}
	if input.DisplayName == "" || len(input.DisplayName) > 120 || len(input.Description) > 2000 {
		return input, errors.New("integration display name or description is invalid")
	}
	if input.Visibility == "" {
		input.Visibility = model.VisibilityPrivate
	}
	if !input.Visibility.Valid() {
		return input, ErrInvalidVisibility
	}
	if input.Lifecycle == "" {
		input.Lifecycle = "draft"
	}
	switch input.Lifecycle {
	case "draft", "active", "deprecated", "retired":
	default:
		return input, errors.New("integration lifecycle is invalid")
	}
	if (input.ReplacementIntegrationID != "" || input.SunsetAt != nil) && input.Lifecycle != "deprecated" && input.Lifecycle != "retired" {
		return input, errors.New("replacement and sunset are only valid for deprecated or retired integrations")
	}
	return input, nil
}

func (s *Service) CreateIntegration(ctx context.Context, input IntegrationInput, actor Actor) (model.Integration, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.Integration{}, err
	}
	input, err = normalizeIntegrationInput(input)
	if err != nil {
		return model.Integration{}, err
	}
	if input.ReplacementIntegrationID != "" {
		if _, err := s.store.Integration(ctx, deployment.ID, input.ReplacementIntegrationID); err != nil {
			return model.Integration{}, errors.New("replacement integration does not exist in this deployment")
		}
	}
	if input.Visibility == model.VisibilityPublic && !input.AcknowledgePublic {
		return model.Integration{}, ErrConfirmationRequired
	}
	id, err := randomUUID()
	if err != nil {
		return model.Integration{}, err
	}
	value, err := s.store.CreateIntegration(ctx, model.Integration{ID: id, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, FamilyKey: input.FamilyKey, VersionKey: input.VersionKey, DisplayName: input.DisplayName, Description: input.Description, Visibility: input.Visibility, Lifecycle: input.Lifecycle, ReplacementIntegrationID: input.ReplacementIntegrationID, SunsetAt: input.SunsetAt})
	if err != nil {
		return model.Integration{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.created", TargetType: "integration", TargetID: value.ID, Current: map[string]any{"family_key": value.FamilyKey, "version_key": value.VersionKey, "visibility": value.Visibility, "lifecycle": value.Lifecycle}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, nil
}

func (s *Service) UpdateIntegration(ctx context.Context, integrationID string, input IntegrationInput, actor Actor) (model.Integration, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.Integration{}, err
	}
	current, err := s.store.Integration(ctx, deployment.ID, integrationID)
	if err != nil {
		return model.Integration{}, err
	}
	if input.Visibility == "" {
		input.Visibility = current.Visibility
	}
	input, err = normalizeIntegrationInput(input)
	if err != nil {
		return model.Integration{}, err
	}
	if input.ReplacementIntegrationID == integrationID {
		return model.Integration{}, errors.New("an integration cannot replace itself")
	}
	if current.Visibility != model.VisibilityPublic && input.Visibility == model.VisibilityPublic && !input.AcknowledgePublic {
		return model.Integration{}, ErrConfirmationRequired
	}
	if input.ReplacementIntegrationID != "" {
		if _, err := s.store.Integration(ctx, deployment.ID, input.ReplacementIntegrationID); err != nil {
			return model.Integration{}, errors.New("replacement integration does not exist in this deployment")
		}
	}
	current.FamilyKey, current.VersionKey, current.DisplayName, current.Description = input.FamilyKey, input.VersionKey, input.DisplayName, input.Description
	current.Visibility, current.Lifecycle, current.ReplacementIntegrationID, current.SunsetAt = input.Visibility, input.Lifecycle, input.ReplacementIntegrationID, input.SunsetAt
	updated, err := s.store.UpdateIntegration(ctx, current, input.Revision)
	if err != nil {
		return model.Integration{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.updated", TargetType: "integration", TargetID: updated.ID, Current: map[string]any{"family_key": updated.FamilyKey, "version_key": updated.VersionKey, "visibility": updated.Visibility, "lifecycle": updated.Lifecycle, "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func (s *Service) SetIntegrationAccessConnections(ctx context.Context, integrationID string, connectionIDs []string, actor Actor) (model.Integration, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.Integration{}, err
	}
	if _, err := s.store.Integration(ctx, deployment.ID, integrationID); err != nil {
		return model.Integration{}, err
	}
	selected := make([]string, 0, len(connectionIDs))
	seen := make(map[string]bool, len(connectionIDs))
	for _, connectionID := range connectionIDs {
		connectionID = strings.TrimSpace(connectionID)
		if connectionID == "" || seen[connectionID] {
			continue
		}
		if _, err := s.store.AccessConnection(ctx, deployment.ID, connectionID); err != nil {
			return model.Integration{}, errors.New("every access binding must reference a connection in this deployment")
		}
		seen[connectionID] = true
		selected = append(selected, connectionID)
	}
	if err := s.store.SetIntegrationAccessConnections(ctx, deployment.ID, integrationID, selected, actor.ID); err != nil {
		return model.Integration{}, err
	}
	updated, err := s.store.Integration(ctx, deployment.ID, integrationID)
	if err != nil {
		return model.Integration{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.access_connections.updated", TargetType: "integration", TargetID: integrationID, Current: map[string]any{"access_connection_ids": updated.AccessConnections}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func (s *Service) SetIntegrationSupportRoute(ctx context.Context, integrationID, routeID string, actor Actor) (model.Integration, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.Integration{}, err
	}
	if _, err := s.store.Integration(ctx, deployment.ID, integrationID); err != nil {
		return model.Integration{}, err
	}
	routeID = strings.TrimSpace(routeID)
	if routeID != "" {
		route, err := s.store.SupportRoute(ctx, deployment.ID, routeID)
		if err != nil || route.State != "active" {
			return model.Integration{}, errors.New("support binding must reference an active route in this deployment")
		}
		if route.IsDefault {
			routeID = ""
		}
	}
	if err := s.store.SetIntegrationSupportRoute(ctx, deployment.ID, integrationID, routeID, actor.ID); err != nil {
		return model.Integration{}, err
	}
	updated, err := s.store.Integration(ctx, deployment.ID, integrationID)
	if err != nil {
		return model.Integration{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.support_route.updated", TargetType: "integration", TargetID: integrationID, Current: map[string]any{"support_route_id": updated.SupportRouteID}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func normalizeManifest(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`[]`)
	}
	if len(raw) > 1<<20 {
		return nil, errors.New("resource set manifest exceeds 1 MiB")
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, errors.New("resource set manifest must be valid JSON")
	}
	if _, ok := value.([]any); !ok {
		return nil, errors.New("resource set manifest must be a JSON array")
	}
	return json.Marshal(value)
}

func contentHash(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

type ResourceSetInput struct {
	Kind        string
	Name        string
	Description string
	State       string
	Manifest    json.RawMessage
	Revision    int64
}

func normalizeResourceSetInput(input ResourceSetInput) (ResourceSetInput, error) {
	input.Kind, input.Name, input.Description, input.State = strings.TrimSpace(input.Kind), strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), strings.TrimSpace(input.State)
	if input.Name == "" || len(input.Name) > 120 || len(input.Description) > 2000 {
		return input, errors.New("resource set name or description is invalid")
	}
	if input.Kind != "documentation" && input.Kind != "api" {
		return input, errors.New("resource set kind must be documentation or api")
	}
	if input.State == "" {
		input.State = "active"
	}
	if input.State != "active" && input.State != "archived" {
		return input, errors.New("resource set state is invalid")
	}
	manifest, err := normalizeManifest(input.Manifest)
	if err != nil {
		return input, err
	}
	input.Manifest = manifest
	return input, nil
}

func (s *Service) CreateResourceSet(ctx context.Context, input ResourceSetInput, actor Actor) (model.ResourceSet, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.ResourceSet{}, err
	}
	input, err = normalizeResourceSetInput(input)
	if err != nil {
		return model.ResourceSet{}, err
	}
	setID, err := randomUUID()
	if err != nil {
		return model.ResourceSet{}, err
	}
	revisionID, err := randomUUID()
	if err != nil {
		return model.ResourceSet{}, err
	}
	value, err := s.store.CreateResourceSet(ctx, model.ResourceSet{ID: setID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, Kind: input.Kind, Name: input.Name, Description: input.Description, State: input.State}, model.ResourceSetRevision{ID: revisionID, ResourceSetID: setID, Manifest: input.Manifest, ContentHash: contentHash(input.Manifest), CreatedBy: actor.ID})
	if err != nil {
		return model.ResourceSet{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "resource_set.created", TargetType: input.Kind + "_set", TargetID: value.ID, Current: map[string]any{"name": value.Name, "revision": value.Revision, "content_hash": value.Latest.ContentHash}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, nil
}

func (s *Service) UpdateResourceSet(ctx context.Context, setID string, input ResourceSetInput, actor Actor) (model.ResourceSet, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.ResourceSet{}, err
	}
	current, err := s.store.ResourceSet(ctx, deployment.ID, setID)
	if err != nil {
		return model.ResourceSet{}, err
	}
	if input.Kind == "" {
		input.Kind = current.Kind
	}
	input, err = normalizeResourceSetInput(input)
	if err != nil {
		return model.ResourceSet{}, err
	}
	if input.Kind != current.Kind {
		return model.ResourceSet{}, errors.New("resource set kind cannot be changed")
	}
	revisionID, err := randomUUID()
	if err != nil {
		return model.ResourceSet{}, err
	}
	current.Name, current.Description, current.State = input.Name, input.Description, input.State
	updated, err := s.store.UpdateResourceSet(ctx, current, model.ResourceSetRevision{ID: revisionID, ResourceSetID: current.ID, Manifest: input.Manifest, ContentHash: contentHash(input.Manifest), CreatedBy: actor.ID}, input.Revision)
	if err != nil {
		return model.ResourceSet{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "resource_set.updated", TargetType: input.Kind + "_set", TargetID: updated.ID, Current: map[string]any{"name": updated.Name, "revision": updated.Revision, "content_hash": updated.Latest.ContentHash, "affected_integrations": updated.UsedBy}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func (s *Service) DuplicateResourceSet(ctx context.Context, setID, name string, actor Actor) (model.ResourceSet, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.ResourceSet{}, err
	}
	current, err := s.store.ResourceSet(ctx, deployment.ID, setID)
	if err != nil {
		return model.ResourceSet{}, err
	}
	if current.Latest == nil {
		return model.ResourceSet{}, errors.New("resource set has no revision to duplicate")
	}
	return s.CreateResourceSet(ctx, ResourceSetInput{Kind: current.Kind, Name: name, Description: "Duplicated from " + current.Name, State: "active", Manifest: current.Latest.Manifest}, actor)
}

func (s *Service) AttachResourceSet(ctx context.Context, integrationID, setID, pinnedRevisionID string, actor Actor) (model.IntegrationResourceLink, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.IntegrationResourceLink{}, err
	}
	if _, err := s.store.Integration(ctx, deployment.ID, integrationID); err != nil {
		return model.IntegrationResourceLink{}, err
	}
	if _, err := s.store.ResourceSet(ctx, deployment.ID, setID); err != nil {
		return model.IntegrationResourceLink{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.IntegrationResourceLink{}, err
	}
	link, err := s.store.SaveIntegrationResourceLink(ctx, model.IntegrationResourceLink{ID: id, IntegrationID: integrationID, ResourceSetID: setID, FollowLatest: pinnedRevisionID == "", PinnedRevisionID: pinnedRevisionID})
	if err != nil {
		return model.IntegrationResourceLink{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.resource_set.attached", TargetType: "integration", TargetID: integrationID, Current: map[string]any{"resource_set_id": setID, "follow_latest": link.FollowLatest, "pinned_revision_id": link.PinnedRevisionID}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return link, nil
}

func (s *Service) DetachResourceSet(ctx context.Context, integrationID, setID string, actor Actor) error {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return err
	}
	if err := s.store.DeleteIntegrationResourceLink(ctx, integrationID, setID); err != nil {
		return err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.resource_set.detached", TargetType: "integration", TargetID: integrationID, Current: map[string]any{"resource_set_id": setID}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return nil
}

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
	SetID       string `json:"set_id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	RevisionID  string `json:"revision_id"`
	Revision    int64  `json:"revision"`
	ContentHash string `json:"content_hash"`
}

type integrationSnapshot struct {
	FamilyKey                string                        `json:"family_key"`
	VersionKey               string                        `json:"version_key"`
	DisplayName              string                        `json:"display_name"`
	Description              string                        `json:"description"`
	Visibility               model.Visibility              `json:"visibility"`
	Lifecycle                string                        `json:"lifecycle"`
	ReplacementIntegrationID string                        `json:"replacement_integration_id,omitempty"`
	SunsetAt                 *time.Time                    `json:"sunset_at,omitempty"`
	Resources                []integrationResourceSnapshot `json:"resource_sets"`
	AccessConnectionIDs      []string                      `json:"access_connection_ids"`
	SupportRouteID           string                        `json:"support_route_id,omitempty"`
}

func buildIntegrationSnapshot(integration model.Integration) (json.RawMessage, []IntegrationPublishValidation, error) {
	validations := make([]IntegrationPublishValidation, 0)
	resources := make([]integrationResourceSnapshot, 0, len(integration.Resources))
	for _, link := range integration.Resources {
		if link.ResolvedRevision == nil {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "resource_revision_unresolved", Message: fmt.Sprintf("%s has no resolvable revision", link.Name), Tab: "resources"})
			continue
		}
		resources = append(resources, integrationResourceSnapshot{SetID: link.ResourceSetID, Kind: link.Kind, Name: link.Name, RevisionID: link.ResolvedRevision.ID, Revision: link.ResolvedRevision.Revision, ContentHash: link.ResolvedRevision.ContentHash})
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
	if len(integration.AccessConnections) == 0 {
		validations = append(validations, IntegrationPublishValidation{Level: "warning", Code: "access_missing", Message: "No service connection is attached.", Tab: "access"})
	}
	if integration.SupportRouteID == "" {
		validations = append(validations, IntegrationPublishValidation{Level: "warning", Code: "support_inherited", Message: "Bug reports and feedback use the deployment default, if one is configured.", Tab: "overview"})
	}
	snapshot, err := json.Marshal(integrationSnapshot{FamilyKey: integration.FamilyKey, VersionKey: integration.VersionKey, DisplayName: integration.DisplayName, Description: integration.Description, Visibility: integration.Visibility, Lifecycle: integration.Lifecycle, ReplacementIntegrationID: integration.ReplacementIntegrationID, SunsetAt: integration.SunsetAt, Resources: resources, AccessConnectionIDs: integration.AccessConnections, SupportRouteID: integration.SupportRouteID})
	return snapshot, validations, err
}

func publishChanges(previous, current json.RawMessage) []IntegrationPublishChange {
	var before, after map[string]any
	_ = json.Unmarshal(previous, &before)
	_ = json.Unmarshal(current, &after)
	fields := []string{"family_key", "version_key", "display_name", "description", "visibility", "lifecycle", "replacement_integration_id", "sunset_at", "resource_sets", "access_connection_ids", "support_route_id"}
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
	snapshot, validations, err := buildIntegrationSnapshot(integration)
	if err != nil {
		return IntegrationPublishStatus{}, err
	}
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
	if integration.Lifecycle == "draft" {
		integration.Lifecycle = "active"
		integration, err = s.store.UpdateIntegration(ctx, integration, integration.Revision)
		if err != nil {
			return model.IntegrationRevision{}, err
		}
	}
	snapshot, validations, err := buildIntegrationSnapshot(integration)
	if err != nil {
		return model.IntegrationRevision{}, err
	}
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
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.published", TargetType: "integration_revision", TargetID: revision.ID, Current: map[string]any{"integration_id": integration.ID, "revision": revision.Revision, "manifest_hash": revision.ManifestHash}, RequestID: actor.RequestID, CreatedAt: now})
	return revision, nil
}
