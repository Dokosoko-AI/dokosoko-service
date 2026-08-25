package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

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

type documentationManifestEntry struct {
	SourcePublicationID string `json:"source_publication_id"`
	SourceID            string `json:"source_id"`
	Revision            int64  `json:"revision"`
	ContentHash         string `json:"content_hash"`
	Name                string `json:"name"`
}

func parseDocumentationManifest(raw json.RawMessage) ([]documentationManifestEntry, error) {
	var objects []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &objects); err != nil {
		return nil, errors.New("documentation manifest must be a JSON array of exact source publications")
	}
	entries := make([]documentationManifestEntry, 0, len(objects))
	allowed := map[string]bool{"source_publication_id": true, "source_id": true, "revision": true, "content_hash": true, "name": true}
	seen := make(map[string]bool, len(objects))
	for _, object := range objects {
		for key := range object {
			if !allowed[key] {
				return nil, fmt.Errorf("documentation manifest contains unsupported field %q", key)
			}
		}
		encoded, _ := json.Marshal(object)
		var entry documentationManifestEntry
		if err := json.Unmarshal(encoded, &entry); err != nil {
			return nil, errors.New("documentation manifest entry is invalid")
		}
		entry.SourcePublicationID, entry.SourceID, entry.ContentHash, entry.Name = strings.TrimSpace(entry.SourcePublicationID), strings.TrimSpace(entry.SourceID), strings.TrimSpace(entry.ContentHash), strings.TrimSpace(entry.Name)
		if entry.SourcePublicationID == "" || entry.SourceID == "" || entry.Revision < 1 || !strings.HasPrefix(entry.ContentHash, "sha256:") || len(entry.ContentHash) != 71 || seen[entry.SourcePublicationID] {
			return nil, errors.New("each documentation entry must reference one unique exact source publication revision and hash")
		}
		seen[entry.SourcePublicationID] = true
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].SourcePublicationID < entries[j].SourcePublicationID })
	return entries, nil
}

func (s *Service) validateResourceManifest(ctx context.Context, productID, kind string, raw json.RawMessage) (json.RawMessage, error) {
	if kind != "documentation" {
		return raw, nil
	}
	entries, err := parseDocumentationManifest(raw)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("documentation resource sets require at least one reviewed source publication")
	}
	for index := range entries {
		publication, lookupErr := s.store.SourcePublication(ctx, productID, entries[index].SourcePublicationID)
		if lookupErr != nil || publication.SourceID != entries[index].SourceID || publication.Revision != entries[index].Revision || publication.ContentHash != entries[index].ContentHash {
			return nil, errors.New("documentation resource sets may reference only exact reviewed publications in this deployment")
		}
		source, sourceErr := s.store.Source(ctx, productID, publication.SourceID)
		if sourceErr != nil || !source.Published || source.Quarantined {
			return nil, errors.New("documentation resource sets may reference only currently published, non-quarantined sources")
		}
		if entries[index].Name == "" {
			entries[index].Name = source.Name
		}
	}
	return json.Marshal(entries)
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
	input.Manifest, err = s.validateResourceManifest(ctx, deployment.ID, input.Kind, input.Manifest)
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
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "resource_set.created", TargetType: input.Kind + "_set", TargetID: value.ID, Current: map[string]any{"name": value.Name, "revision": value.Revision, "content_hash": value.Latest.ContentHash}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.ResourceSet{}, err
	}
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
	input.Manifest, err = s.validateResourceManifest(ctx, deployment.ID, input.Kind, input.Manifest)
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
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "resource_set.updated", TargetType: input.Kind + "_set", TargetID: updated.ID, Current: map[string]any{"name": updated.Name, "revision": updated.Revision, "content_hash": updated.Latest.ContentHash, "affected_integrations": updated.UsedBy}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.ResourceSet{}, err
	}
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
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.resource_set.attached", TargetType: "integration", TargetID: integrationID, Current: map[string]any{"resource_set_id": setID, "follow_latest": link.FollowLatest, "pinned_revision_id": link.PinnedRevisionID}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.IntegrationResourceLink{}, err
	}
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
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.resource_set.detached", TargetType: "integration", TargetID: integrationID, Current: map[string]any{"resource_set_id": setID}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return err
	}
	return nil
}
