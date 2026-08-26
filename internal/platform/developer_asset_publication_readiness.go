package platform

import (
	"context"
	"errors"
	"fmt"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const developerAssetActivationAuditAction = "developer_asset.publication_activated"

func developerAssetActivationAuditID(kind, publicationID string) string {
	return fmt.Sprintf("developer-asset-activation:%s:%s", kind, publicationID)
}

func (s *Service) recordDeveloperAssetPublicationActivation(
	ctx context.Context,
	deployment model.Deployment,
	actor Actor,
	kind, publicationID string,
	current map[string]any,
) error {
	return s.store.AppendAudit(ctx, model.AuditEvent{
		ID: developerAssetActivationAuditID(kind, publicationID), OrganisationID: deployment.OrganisationID,
		ProductID: deployment.ID, ActorID: actor.ID, Action: developerAssetActivationAuditAction,
		TargetType: kind, TargetID: publicationID, Current: current, RequestID: actor.RequestID,
		Outcome: "success", CreatedAt: s.now(),
	})
}

func (s *Service) activateDeploymentDocumentationPublication(
	ctx context.Context,
	deployment model.Deployment,
	publication model.DeploymentDocumentationPublication,
	actor Actor,
) error {
	current := map[string]any{
		"revision": publication.Revision, "snapshot_hash": publication.SnapshotHash,
		"collection_revision_count": len(publication.Members), "visibility": publication.Visibility,
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{
		ID:             "deployment-documentation-publication:" + publication.ID,
		OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID,
		Action: "deployment_documentation.published", TargetType: "deployment_documentation_publication",
		TargetID: publication.ID, Current: current, RequestID: actor.RequestID,
		Outcome: "success", CreatedAt: publication.PublishedAt,
	}); err != nil {
		return err
	}
	if err := s.recordDeveloperAssetPublicationActivation(ctx, deployment, actor, "global_documentation", publication.ID, current); err != nil {
		return err
	}
	_, err := s.BuildDeveloperAssetSearchIndex(ctx, "global_documentation", publication.ID)
	return err
}

// ActivateDeveloperAssetPublication durably records the domain publication
// audit and exact activation marker before building the ready search
// projection. It is idempotent and is the only supported activation path for
// an already-created immutable developer-asset publication.
func (s *Service) ActivateDeveloperAssetPublication(ctx context.Context, kind, publicationID string, actor Actor) error {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return err
	}
	switch kind {
	case "global_documentation":
		publication, lookupErr := s.store.DeploymentDocumentationPublication(ctx, deployment.ID, publicationID)
		if lookupErr != nil {
			return lookupErr
		}
		return s.activateDeploymentDocumentationPublication(ctx, deployment, publication, actor)
	case "api":
		publication, lookupErr := s.store.APIDeveloperAssetPublication(ctx, deployment.ID, publicationID)
		if lookupErr != nil {
			return lookupErr
		}
		integration, lookupErr := s.store.Integration(ctx, deployment.ID, publication.APIID)
		if lookupErr != nil {
			return lookupErr
		}
		revisions, lookupErr := s.store.IntegrationRevisions(ctx, publication.APIID)
		if lookupErr != nil {
			return lookupErr
		}
		for _, revision := range revisions {
			if revision.ID == publication.APIRevisionID {
				return s.activateIntegrationPublication(ctx, deployment, integration, revision, publication, actor)
			}
		}
		return store.ErrNotFound
	default:
		return errors.New("developer-asset activation requires global_documentation or api publication kind")
	}
}

func (s *Service) developerAssetPublicationActivationRecorded(ctx context.Context, deploymentID, kind, publicationID string) (bool, error) {
	event, err := s.store.AuditEvent(ctx, developerAssetActivationAuditID(kind, publicationID))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return event.ProductID == deploymentID && event.Action == developerAssetActivationAuditAction &&
		event.TargetType == kind && event.TargetID == publicationID && event.Outcome == "success", nil
}

func (s *Service) developerAssetPublicationReady(ctx context.Context, deploymentID, kind, publicationID string) (bool, error) {
	activated, err := s.developerAssetPublicationActivationRecorded(ctx, deploymentID, kind, publicationID)
	if err != nil || !activated {
		return false, err
	}
	generations, err := s.store.SearchIndexGenerations(ctx, deploymentID, kind, publicationID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	for _, generation := range generations {
		if generation.State == "ready" &&
			generation.BuilderVersion == DeveloperAssetIndexBuilderVersion &&
			generation.RetrievalProfileVersion == DeveloperAssetRetrievalProfileVersion {
			return true, nil
		}
	}
	return false, nil
}

// ReadyDeveloperAssetSearchIndex returns the exact current-version immutable
// index generation for an activated publication. Callers that expose retained
// historical publication identifiers must use this method rather than reading
// source revisions directly: activation and a ready projection are both part
// of the publication boundary.
func (s *Service) ReadyDeveloperAssetSearchIndex(ctx context.Context, kind, publicationID string) (store.SearchIndexGenerationRecord, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return store.SearchIndexGenerationRecord{}, err
	}
	ready, err := s.developerAssetPublicationReady(ctx, deployment.ID, kind, publicationID)
	if err != nil {
		return store.SearchIndexGenerationRecord{}, err
	}
	if !ready {
		return store.SearchIndexGenerationRecord{}, store.ErrNotFound
	}
	generations, err := s.store.SearchIndexGenerations(ctx, deployment.ID, kind, publicationID)
	if err != nil {
		return store.SearchIndexGenerationRecord{}, err
	}
	for _, generation := range generations {
		if generation.State != "ready" ||
			generation.BuilderVersion != DeveloperAssetIndexBuilderVersion ||
			generation.RetrievalProfileVersion != DeveloperAssetRetrievalProfileVersion {
			continue
		}
		record, lookupErr := s.store.SearchIndexGeneration(ctx, deployment.ID, generation.ID)
		if lookupErr != nil {
			return store.SearchIndexGenerationRecord{}, lookupErr
		}
		if !generation.Valid() || !record.Generation.Valid() ||
			record.Generation.ID != generation.ID || record.Generation.DeploymentID != deployment.ID ||
			record.Generation.PublicationKind != kind || record.Generation.PublicationID != publicationID ||
			record.Generation.State != "ready" ||
			record.Generation.BuilderVersion != DeveloperAssetIndexBuilderVersion ||
			record.Generation.RetrievalProfileVersion != DeveloperAssetRetrievalProfileVersion ||
			record.Generation.AssetKind != generation.AssetKind ||
			record.Generation.UnitCount != generation.UnitCount ||
			record.Generation.ContentHash != generation.ContentHash ||
			record.Generation.UnitCount != len(record.Units) {
			return store.SearchIndexGenerationRecord{}, errors.New("ready developer-asset index record is inconsistent")
		}
		return record, nil
	}
	return store.SearchIndexGenerationRecord{}, store.ErrNotFound
}

func (s *Service) readyDeploymentDocumentationPublication(
	ctx context.Context,
	deploymentID string,
) (model.DeploymentDocumentationPublication, error) {
	publications, err := s.store.DeploymentDocumentationPublications(ctx, deploymentID)
	if err != nil {
		return model.DeploymentDocumentationPublication{}, err
	}
	for _, publication := range publications {
		ready, readyErr := s.developerAssetPublicationReady(ctx, deploymentID, "global_documentation", publication.ID)
		if readyErr != nil {
			return model.DeploymentDocumentationPublication{}, readyErr
		}
		if ready {
			return publication, nil
		}
	}
	return model.DeploymentDocumentationPublication{}, store.ErrNotFound
}

// ReadyDeploymentDocumentationPublication returns the newest immutable global
// documentation publication whose exact search projection is ready. A newer
// failed/preparing row does not move discovery.
func (s *Service) ReadyDeploymentDocumentationPublication(ctx context.Context) (model.DeploymentDocumentationPublication, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.DeploymentDocumentationPublication{}, err
	}
	return s.readyDeploymentDocumentationPublication(ctx, deployment.ID)
}

func (s *Service) readyAPIDeveloperAssetPublication(
	ctx context.Context,
	deploymentID, apiID string,
) (model.APIDeveloperAssetPublication, error) {
	publications, err := s.store.APIDeveloperAssetPublications(ctx, deploymentID, apiID)
	if err != nil {
		return model.APIDeveloperAssetPublication{}, err
	}
	for _, publication := range publications {
		ready, readyErr := s.developerAssetPublicationReady(ctx, deploymentID, "api", publication.ID)
		if readyErr != nil {
			return model.APIDeveloperAssetPublication{}, readyErr
		}
		if ready {
			return publication, nil
		}
	}
	return model.APIDeveloperAssetPublication{}, store.ErrNotFound
}

// ReadyAPIDeveloperAssetPublication returns the latest exact API asset
// publication with a ready immutable index generation.
func (s *Service) ReadyAPIDeveloperAssetPublication(ctx context.Context, apiID string) (model.APIDeveloperAssetPublication, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.APIDeveloperAssetPublication{}, err
	}
	if _, err := s.store.Integration(ctx, deployment.ID, apiID); err != nil {
		return model.APIDeveloperAssetPublication{}, err
	}
	return s.readyAPIDeveloperAssetPublication(ctx, deployment.ID, apiID)
}
