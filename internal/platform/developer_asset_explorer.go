package platform

import (
	"context"
	"errors"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type DocumentationExplorerInput struct {
	IngestionRunID      string
	SourceID            string
	SourcePublicationID string
	Query               string
	Limit               int
	Offset              int
}

func (s *Service) DocumentationCandidates(ctx context.Context, input DocumentationExplorerInput) (store.DocumentationCandidatePage, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return store.DocumentationCandidatePage{}, err
	}
	input.IngestionRunID = strings.TrimSpace(input.IngestionRunID)
	input.SourceID = strings.TrimSpace(input.SourceID)
	input.SourcePublicationID = strings.TrimSpace(input.SourcePublicationID)
	input.Query = strings.TrimSpace(input.Query)
	if len(input.Query) > 500 {
		return store.DocumentationCandidatePage{}, errors.New("documentation search query must be no more than 500 characters")
	}
	if input.Limit == 0 {
		input.Limit = 100
	}
	if input.Limit < 1 || input.Limit > 500 {
		return store.DocumentationCandidatePage{}, errors.New("documentation explorer limit must be between 1 and 500")
	}
	if input.Offset < 0 {
		return store.DocumentationCandidatePage{}, errors.New("documentation explorer offset must be zero or greater")
	}
	if input.SourceID != "" {
		if _, err := s.store.Source(ctx, deployment.ID, input.SourceID); err != nil {
			return store.DocumentationCandidatePage{}, err
		}
	}
	if input.SourcePublicationID != "" {
		publication, err := s.store.SourcePublication(ctx, deployment.ID, input.SourcePublicationID)
		if err != nil {
			return store.DocumentationCandidatePage{}, err
		}
		if input.SourceID != "" && publication.SourceID != input.SourceID {
			return store.DocumentationCandidatePage{}, errors.New("source publication does not belong to the selected source")
		}
		input.SourceID = publication.SourceID
	}
	if input.IngestionRunID != "" {
		run, err := s.store.DeveloperAssetIngestionRun(ctx, deployment.ID, input.IngestionRunID)
		if err != nil {
			return store.DocumentationCandidatePage{}, err
		}
		if run.AssetKind != model.DeveloperAssetDocumentation || (input.SourceID != "" && run.SourceID != input.SourceID) {
			return store.DocumentationCandidatePage{}, errors.New("ingestion run is not documentation for the selected source")
		}
	}
	return s.store.DocumentationCandidateDocuments(ctx, store.DocumentationCandidateQuery{
		DeploymentID: deployment.ID, IngestionRunID: input.IngestionRunID, SourceID: input.SourceID,
		SourcePublicationID: input.SourcePublicationID, QueryText: input.Query, Limit: input.Limit, Offset: input.Offset,
	})
}

func (s *Service) DocumentationCandidate(ctx context.Context, documentID string) (store.DocumentationCandidateRecord, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return store.DocumentationCandidateRecord{}, err
	}
	return s.store.DocumentationCandidateDocument(ctx, deployment.ID, strings.TrimSpace(documentID))
}

type DeveloperAssetIngestionSummary struct {
	Run    model.DeveloperAssetIngestionRun     `json:"run"`
	Stages []model.DeveloperAssetIngestionStage `json:"stages"`
}

func (s *Service) DeveloperAssetIngestionRuns(ctx context.Context, kind model.DeveloperAssetKind, targetKey string) ([]model.DeveloperAssetIngestionRun, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return nil, err
	}
	if kind != "" && !kind.Valid() {
		return nil, errors.New("developer asset kind is invalid")
	}
	return s.store.DeveloperAssetIngestionRuns(ctx, deployment.ID, kind, strings.TrimSpace(targetKey))
}

func (s *Service) DeveloperAssetIngestion(ctx context.Context, runID string) (DeveloperAssetIngestionSummary, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return DeveloperAssetIngestionSummary{}, err
	}
	run, err := s.store.DeveloperAssetIngestionRun(ctx, deployment.ID, strings.TrimSpace(runID))
	if err != nil {
		return DeveloperAssetIngestionSummary{}, err
	}
	stages, err := s.store.DeveloperAssetIngestionStages(ctx, run.ID)
	if err != nil {
		return DeveloperAssetIngestionSummary{}, err
	}
	return DeveloperAssetIngestionSummary{Run: run, Stages: stages}, nil
}
