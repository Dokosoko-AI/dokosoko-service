package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestMemoryDocumentationCandidateReturnsScopedNewestFirstSelectionHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := NewMemory()
	base := time.Date(2026, time.August, 26, 9, 0, 0, 0, time.UTC)
	run, err := memory.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: "run_selection_history", DeploymentID: "prod_acme", OrganisationID: "org_acme",
		AssetKind: model.DeveloperAssetDocumentation, TargetID: "src_docs", TargetKey: "source:src_docs", SourceID: "src_docs",
		State: model.DeveloperAssetIngestionReviewReady, Attempt: 1,
		Versions:    model.ProcessorVersions{Pipeline: "pipeline-v1", Parser: "parser-v1", Normalizer: "normalizer-v1", Mapper: "mapper-v1"},
		RawManifest: json.RawMessage(`{}`), Diagnostics: json.RawMessage(`{}`), QueuedAt: base,
	})
	if err != nil {
		t.Fatal(err)
	}
	documentHash := "sha256:" + strings.Repeat("a", 64)
	if err := memory.SaveDocumentationIngestionOutput(ctx, "prod_acme", DocumentationIngestionOutput{
		Documents: []model.DocumentationDocument{
			{ID: "doc_selection_history", DeploymentID: "prod_acme", IngestionRunID: run.ID, SourcePath: "history.md", Title: "History", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "History", ContentHash: documentHash, Visibility: model.VisibilityPrivate, Metadata: json.RawMessage(`{}`)},
			{ID: "doc_without_history", DeploymentID: "prod_acme", IngestionRunID: run.ID, SourcePath: "unreviewed.md", Title: "Unreviewed", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "Unreviewed", ContentHash: "sha256:" + strings.Repeat("b", 64), Visibility: model.VisibilityPrivate, Ordinal: 1, Metadata: json.RawMessage(`{}`)},
		},
	}); err != nil {
		t.Fatal(err)
	}

	selections := []model.SourcePublicationDocumentSelection{
		{SourcePublicationID: "pub_history_old", DeploymentID: "prod_acme", DocumentationDocumentID: "doc_selection_history", Decision: "excluded", Reason: "Superseded setup guidance.", ContentHash: documentHash, ReviewedBy: "old-reviewer", ReviewedAt: base.Add(time.Hour), CreatedAt: base.Add(2 * time.Hour)},
		{SourcePublicationID: "pub_history_quarantine", DeploymentID: "prod_acme", DocumentationDocumentID: "doc_selection_history", Decision: "quarantined", Reason: "Prompt-injection language requires investigation.", ContentHash: documentHash, ReviewedBy: "security-reviewer", ReviewedAt: base.Add(3 * time.Hour), CreatedAt: base.Add(4 * time.Hour)},
		{SourcePublicationID: "pub_history_new", DeploymentID: "prod_acme", DocumentationDocumentID: "doc_selection_history", Decision: "included", Ordinal: intPointer(2), ContentHash: documentHash, ReviewedBy: "new-reviewer", ReviewedAt: base.Add(5 * time.Hour), CreatedAt: base.Add(6 * time.Hour)},
	}
	memory.mu.Lock()
	for _, selection := range selections {
		memory.sourcePublications["prod_acme"][selection.SourcePublicationID] = model.SourcePublication{
			ID: selection.SourcePublicationID, ProductID: "prod_acme", SourceID: "src_docs",
		}
		memory.developerAssets.sourcePublicationReviews[selection.SourcePublicationID] = SourcePublicationDocumentationReview{
			Selections: []model.SourcePublicationDocumentSelection{selection},
		}
	}
	memory.developerAssets.sourcePublicationReviews["pub_missing_scope"] = SourcePublicationDocumentationReview{
		Selections: []model.SourcePublicationDocumentSelection{{
			SourcePublicationID: "pub_missing_scope", DeploymentID: "prod_acme", DocumentationDocumentID: "doc_selection_history",
			Decision: "excluded", Reason: "This review has no publication in the document deployment.", ContentHash: documentHash,
			ReviewedBy: "invalid-reviewer", ReviewedAt: base.Add(7 * time.Hour), CreatedAt: base.Add(8 * time.Hour),
		}},
	}
	memory.mu.Unlock()

	record, err := memory.DocumentationCandidateDocument(ctx, "prod_acme", "doc_selection_history")
	if err != nil {
		t.Fatal(err)
	}
	if len(record.SourcePublicationSelections) != 3 {
		t.Fatalf("selection history = %#v", record.SourcePublicationSelections)
	}
	wantDecisions := []string{"included", "quarantined", "excluded"}
	for index, want := range wantDecisions {
		if record.SourcePublicationSelections[index].Decision != want {
			t.Fatalf("selection %d = %#v, want decision %q", index, record.SourcePublicationSelections[index], want)
		}
	}
	if record.SourcePublicationSelections[0].SourcePublicationID != "pub_history_new" || record.SourcePublicationSelections[1].Reason != "Prompt-injection language requires investigation." || record.SourcePublicationSelections[2].ReviewedBy != "old-reviewer" {
		t.Fatalf("selection evidence was not preserved: %#v", record.SourcePublicationSelections)
	}

	page, err := memory.DocumentationCandidateDocuments(ctx, DocumentationCandidateQuery{DeploymentID: "prod_acme", IngestionRunID: run.ID, Limit: 10})
	if err != nil || len(page.Items) != 2 || len(page.Items[0].SourcePublicationSelections) != 3 {
		t.Fatalf("candidate page = %#v, err=%v", page, err)
	}
	unreviewed, err := memory.DocumentationCandidateDocument(ctx, "prod_acme", "doc_without_history")
	if err != nil || unreviewed.SourcePublicationSelections == nil || len(unreviewed.SourcePublicationSelections) != 0 {
		t.Fatalf("unreviewed candidate history = %#v, err=%v", unreviewed.SourcePublicationSelections, err)
	}
}

func intPointer(value int) *int {
	return &value
}
