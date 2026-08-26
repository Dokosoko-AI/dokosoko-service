package store

import (
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/docreview"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

const (
	sourcePublicationDocumentExcludedReason    = "not_selected_in_source_review"
	sourcePublicationDocumentQuarantinedReason = "failed_deterministic_safety_assessment"
)

// buildSourcePublicationDocumentationReview derives the typed publication
// review from one exact crawl generation. The legacy-to-typed mapping must be
// a complete bijection so a publication can never silently omit typed output.
func buildSourcePublicationDocumentationReview(
	deploymentID string,
	run model.DeveloperAssetIngestionRun,
	publication model.SourcePublication,
	typedDocuments []model.DocumentationDocument,
	documentationMap *model.DocumentationMap,
	legacyDocuments map[string]model.CrawlReviewDocument,
	selectedLegacyDocumentIDs []string,
) (SourcePublicationDocumentationReview, error) {
	if deploymentID == "" || run.ID == "" || publication.ID == "" ||
		run.DeploymentID != deploymentID || run.ID != publication.CrawlJobID ||
		run.AssetKind != model.DeveloperAssetDocumentation ||
		run.SourceID == "" || run.SourceID != publication.SourceID || run.TargetID != publication.SourceID ||
		run.State != model.DeveloperAssetIngestionReviewReady ||
		run.AcquiredCount <= 0 || run.FailedCount != 0 || run.SkippedCount != 0 ||
		strings.TrimSpace(publication.ReviewedBy) == "" || publication.ReviewedAt.IsZero() {
		return SourcePublicationDocumentationReview{}, ErrConflict
	}
	if len(typedDocuments) != run.AcquiredCount || len(legacyDocuments) != run.AcquiredCount ||
		documentationMap == nil || documentationMap.ID == "" || strings.TrimSpace(documentationMap.MapVersion) == "" ||
		strings.TrimSpace(documentationMap.AgentMarkdown) == "" || documentationMap.ContentHash == "" ||
		documentationMap.DeploymentID != deploymentID || documentationMap.IngestionRunID != run.ID ||
		documentationMap.DocumentationCollectionRevisionID != "" ||
		(publication.Visibility == model.VisibilityPublic && documentationMap.Visibility != model.VisibilityPublic) {
		return SourcePublicationDocumentationReview{}, ErrConflict
	}

	selected := make(map[string]bool, len(selectedLegacyDocumentIDs))
	for _, documentID := range selectedLegacyDocumentIDs {
		documentID = strings.TrimSpace(documentID)
		if documentID == "" || selected[documentID] {
			return SourcePublicationDocumentationReview{}, ErrConflict
		}
		selected[documentID] = true
	}
	if len(selected) == 0 {
		return SourcePublicationDocumentationReview{}, ErrConflict
	}

	ordered := append([]model.DocumentationDocument(nil), typedDocuments...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Ordinal == ordered[j].Ordinal {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Ordinal < ordered[j].Ordinal
	})

	typedIDs := make(map[string]bool, len(ordered))
	mappedLegacyIDs := make(map[string]bool, len(ordered))
	review := SourcePublicationDocumentationReview{
		Selections: make([]model.SourcePublicationDocumentSelection, 0, len(ordered)),
		MapLink: &model.SourcePublicationDocumentationMap{
			SourcePublicationID: publication.ID,
			DeploymentID:        deploymentID,
			DocumentationMapID:  documentationMap.ID,
			ContentHash:         documentationMap.ContentHash,
		},
	}
	includedOrdinal := 0
	for _, document := range ordered {
		legacyID := strings.TrimSpace(document.LegacyKnowledgeDocumentID)
		legacy, mapped := legacyDocuments[legacyID]
		if document.ID == "" || document.ContentHash == "" ||
			document.DeploymentID != deploymentID || document.IngestionRunID != run.ID ||
			legacyID == "" || !mapped || legacy.ID != legacyID || legacy.CrawlJobID != run.ID ||
			typedIDs[document.ID] || mappedLegacyIDs[legacyID] ||
			(publication.Visibility == model.VisibilityPublic && document.Visibility != model.VisibilityPublic) {
			return SourcePublicationDocumentationReview{}, ErrConflict
		}
		typedIDs[document.ID] = true
		mappedLegacyIDs[legacyID] = true

		selection := model.SourcePublicationDocumentSelection{
			SourcePublicationID:     publication.ID,
			DeploymentID:            deploymentID,
			DocumentationDocumentID: document.ID,
			ContentHash:             document.ContentHash,
			ReviewedBy:              publication.ReviewedBy,
			ReviewedAt:              publication.ReviewedAt,
		}
		if selected[legacyID] {
			if !docreview.SafeAssessment(legacy.State, legacy.InjectionIndicators) {
				return SourcePublicationDocumentationReview{}, ErrConflict
			}
			ordinal := includedOrdinal
			includedOrdinal++
			selection.Decision = "included"
			selection.Ordinal = &ordinal
			delete(selected, legacyID)
		} else if docreview.SafeAssessment(legacy.State, legacy.InjectionIndicators) {
			selection.Decision = "excluded"
			selection.Reason = sourcePublicationDocumentExcludedReason
		} else {
			selection.Decision = "quarantined"
			selection.Reason = sourcePublicationDocumentQuarantinedReason
		}
		review.Selections = append(review.Selections, selection)
	}
	if len(selected) != 0 || len(mappedLegacyIDs) != len(legacyDocuments) || includedOrdinal == 0 {
		return SourcePublicationDocumentationReview{}, ErrConflict
	}
	return review, nil
}
