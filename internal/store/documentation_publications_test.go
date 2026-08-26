package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/docreview"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestMemorySourcePublicationPinsExactCrawlDocuments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := NewMemory()
	source, err := memory.CreateSource(ctx, model.Source{ID: "source_review_test", OrganisationID: "org_acme", ProductID: "prod_acme", Name: "Reviewed docs", Kind: "upload", Location: "review.md"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := model.CrawlJob{ID: "crawl_review_test", OrganisationID: source.OrganisationID, ProductID: source.ProductID, SourceID: source.ID, State: "review", DiscoveredCount: 2, FetchedCount: 2, ChangedCount: 2, QueuedAt: now, FinishedAt: &now}
	runningJob := model.CrawlJob{ID: "crawl_running_test", OrganisationID: source.OrganisationID, ProductID: source.ProductID, SourceID: source.ID, State: "running", QueuedAt: now.Add(time.Second)}
	memory.mu.Lock()
	memory.crawls[source.ID] = []model.CrawlJob{job, runningJob}
	memory.crawlReviewDocuments[job.ID] = []model.CrawlReviewDocument{
		{ID: "document_selected", CrawlJobID: job.ID, SnapshotID: "snapshot_selected", Title: "Selected", CanonicalURL: "https://docs.example.test/selected", State: "validated", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + strings.Repeat("a", 64), Changed: true},
		{ID: "document_excluded", CrawlJobID: job.ID, SnapshotID: "snapshot_excluded", Title: "Excluded", CanonicalURL: "https://docs.example.test/excluded", State: "validated", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + strings.Repeat("b", 64), Changed: true},
	}
	memory.knowledge[source.ProductID] = append(memory.knowledge[source.ProductID],
		model.KnowledgeRecord{ID: "document_selected", ProductID: source.ProductID, SourceID: source.ID, Title: "Selected", Text: "selected text", Published: false, Visibility: model.VisibilityPrivate},
		model.KnowledgeRecord{ID: "document_excluded", ProductID: source.ProductID, SourceID: source.ID, Title: "Excluded", Text: "excluded text", Published: false, Visibility: model.VisibilityPrivate},
	)
	memory.mu.Unlock()
	if _, err := memory.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: job.ID, DeploymentID: source.ProductID, OrganisationID: source.OrganisationID,
		AssetKind: model.DeveloperAssetDocumentation, TargetID: source.ID, TargetKey: "source:" + source.ID, SourceID: source.ID,
		State: model.DeveloperAssetIngestionReviewReady, Attempt: 1,
		Versions:    model.ProcessorVersions{Pipeline: "pipeline-v1", Parser: "parser-v1", Normalizer: "normalizer-v1", Mapper: "mapper-v1"},
		RawManifest: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`{}`), DiscoveredCount: 2, AcquiredCount: 2,
		QueuedAt: now, StartedAt: &now, FinishedAt: &now,
	}); err != nil {
		t.Fatalf("create typed documentation run: %v", err)
	}
	typedSelectedHash := "sha256:" + strings.Repeat("c", 64)
	typedExcludedHash := "sha256:" + strings.Repeat("d", 64)
	mapHash := "sha256:" + strings.Repeat("e", 64)
	if err := memory.SaveDocumentationIngestionOutput(ctx, source.ProductID, DocumentationIngestionOutput{
		Documents: []model.DocumentationDocument{
			{ID: "typed_excluded", DeploymentID: source.ProductID, IngestionRunID: job.ID, LegacyKnowledgeDocumentID: "document_excluded", SourcePath: "excluded.md", Title: "Excluded", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "Excluded", ContentHash: typedExcludedHash, Visibility: model.VisibilityPrivate, Ordinal: 0, Metadata: json.RawMessage(`{}`)},
			{ID: "typed_selected", DeploymentID: source.ProductID, IngestionRunID: job.ID, LegacyKnowledgeDocumentID: "document_selected", SourcePath: "selected.md", Title: "Selected", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "Selected", ContentHash: typedSelectedHash, Visibility: model.VisibilityPrivate, Ordinal: 1, Metadata: json.RawMessage(`{}`)},
		},
		Map: &model.DocumentationMap{ID: "typed_documentation_map", DeploymentID: source.ProductID, IngestionRunID: job.ID, MapVersion: "map-v1", Map: model.DocumentationMapBody{Overview: "Exact typed documentation map."}, AgentMarkdown: "# Exact typed documentation map\n", ContentHash: mapHash, Visibility: model.VisibilityPrivate},
	}); err != nil {
		t.Fatalf("save typed documentation output: %v", err)
	}

	review, err := memory.SourceReview(ctx, source.ProductID, source.ID, job.ID)
	if err != nil || len(review.Documents) != 2 || review.Publication != nil {
		t.Fatalf("review = %#v, err = %v", review, err)
	}
	publicationHash, err := docreview.PublicationContentHash([]model.CrawlReviewDocument{review.Documents[0]})
	if err != nil {
		t.Fatal(err)
	}
	publication := model.SourcePublication{ID: "publication_review_test", CrawlJobID: job.ID, ContentHash: publicationHash, ReviewedBy: "reviewer", ReviewedAt: now, PublishedAt: now}
	if _, _, err := memory.PublishSource(ctx, source.ProductID, source.ID, source.Revision, publication, []string{"document_selected"}); err != ErrConflict {
		t.Fatalf("older finished crawl published while a newer crawl was active with err %v", err)
	}
	memory.mu.Lock()
	memory.crawls[source.ID] = []model.CrawlJob{job}
	memory.mu.Unlock()
	partialJob := job
	partialJob.FailedCount = 1
	memory.mu.Lock()
	memory.crawls[source.ID] = []model.CrawlJob{partialJob}
	memory.mu.Unlock()
	if _, _, err := memory.PublishSource(ctx, source.ProductID, source.ID, source.Revision, publication, []string{"document_selected"}); err != ErrConflict {
		t.Fatalf("crawl with failed documents published with err %v", err)
	}
	partialJob.FailedCount = 0
	partialJob.SkippedCount = 1
	memory.mu.Lock()
	memory.crawls[source.ID] = []model.CrawlJob{partialJob}
	memory.mu.Unlock()
	if _, _, err := memory.PublishSource(ctx, source.ProductID, source.ID, source.Revision, publication, []string{"document_selected"}); err != ErrConflict {
		t.Fatalf("crawl with skipped documents published with err %v", err)
	}
	memory.mu.Lock()
	memory.crawls[source.ID] = []model.CrawlJob{job}
	memory.mu.Unlock()
	staleHash := publication
	staleHash.ContentHash = "sha256:" + strings.Repeat("f", 64)
	if _, _, err := memory.PublishSource(ctx, source.ProductID, source.ID, source.Revision, staleHash, []string{"document_selected"}); err != ErrConflict {
		t.Fatalf("stale service-computed publication hash was accepted with err %v", err)
	}
	updated, published, err := memory.PublishSource(ctx, source.ProductID, source.ID, source.Revision, publication, []string{"document_selected"})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Published || published.DocumentCount != 1 || published.CrawlJobID != job.ID {
		t.Fatalf("source = %#v, publication = %#v", updated, published)
	}
	typedReview, err := memory.SourcePublicationDocumentationReview(ctx, source.ProductID, published.ID)
	if err != nil || len(typedReview.Selections) != 2 || typedReview.MapLink == nil || typedReview.MapLink.DocumentationMapID != "typed_documentation_map" || typedReview.MapLink.ContentHash != mapHash {
		t.Fatalf("typed publication review = %#v, err=%v", typedReview, err)
	}
	decisions := make(map[string]model.SourcePublicationDocumentSelection, len(typedReview.Selections))
	for _, decision := range typedReview.Selections {
		decisions[decision.DocumentationDocumentID] = decision
	}
	included := decisions["typed_selected"]
	if included.Decision != "included" || included.Ordinal == nil || *included.Ordinal != 0 || included.Reason != "" || included.ContentHash != typedSelectedHash || included.ReviewedBy != publication.ReviewedBy || !included.ReviewedAt.Equal(publication.ReviewedAt) {
		t.Fatalf("included typed decision = %#v", included)
	}
	excluded := decisions["typed_excluded"]
	if excluded.Decision != "excluded" || excluded.Ordinal != nil || excluded.Reason != sourcePublicationDocumentExcludedReason || excluded.ContentHash != typedExcludedHash {
		t.Fatalf("excluded typed decision = %#v", excluded)
	}
	typedRun, err := memory.DeveloperAssetIngestionRun(ctx, source.ProductID, job.ID)
	if err != nil || typedRun.State != model.DeveloperAssetIngestionPublished {
		t.Fatalf("typed documentation run = %#v, err=%v", typedRun, err)
	}
	typedRecords, err := memory.DocumentationCandidateDocuments(ctx, DocumentationCandidateQuery{DeploymentID: source.ProductID, SourcePublicationID: published.ID, Limit: 20})
	if err != nil || len(typedRecords.Items) != 2 || typedRecords.Total != 2 {
		t.Fatalf("typed publication retrieval = %#v, err=%v", typedRecords, err)
	}
	for _, record := range typedRecords.Items {
		if record.DocumentationMap == nil || record.DocumentationMap.ID != typedReview.MapLink.DocumentationMapID || record.DocumentationMap.ContentHash != typedReview.MapLink.ContentHash {
			t.Fatalf("typed publication record did not resolve its exact map: %#v", record)
		}
	}
	items, err := memory.PrivateKnowledge(ctx, source.ProductID, []string{published.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "document_selected" {
		t.Fatalf("exact publication returned %#v", items)
	}
	updated.Visibility = model.VisibilityPublic
	updated, err = memory.UpdateSource(ctx, updated, updated.Revision)
	if err != nil {
		t.Fatal(err)
	}
	publicItems, err := memory.PublicKnowledge(ctx, source.ProductID, []string{published.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(publicItems) != 0 {
		t.Fatalf("a private publication became public after only changing current source visibility: %#v", publicItems)
	}
	if _, _, err := memory.PublishSource(ctx, source.ProductID, source.ID, updated.Revision, publication, []string{"document_excluded"}); err != ErrConflict {
		t.Fatalf("same crawl generation republished with err %v", err)
	}
}

type memoryDocumentationBridgeFixture struct {
	memory      *Memory
	source      model.Source
	job         model.CrawlJob
	publication model.SourcePublication
}

func newMemoryDocumentationBridgeFixture(t *testing.T, sourceKind, typedMode string) memoryDocumentationBridgeFixture {
	t.Helper()
	ctx := context.Background()
	memory := NewMemory()
	source, err := memory.CreateSource(ctx, model.Source{
		ID: "source_atomic_" + typedMode, OrganisationID: "org_acme", ProductID: "prod_acme",
		Name: "Atomic documentation bridge", Kind: sourceKind, Location: "atomic.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := model.CrawlJob{
		ID: "crawl_atomic_" + typedMode, OrganisationID: source.OrganisationID, ProductID: source.ProductID, SourceID: source.ID,
		State: "review", DiscoveredCount: 2, FetchedCount: 2, ChangedCount: 2, QueuedAt: now, FinishedAt: &now,
	}
	legacyDocuments := []model.CrawlReviewDocument{
		{ID: "legacy_atomic_selected_" + typedMode, CrawlJobID: job.ID, SnapshotID: "snapshot-selected", Title: "Selected", CanonicalURL: "https://docs.example.test/selected", State: "validated", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + strings.Repeat("1", 64), Changed: true},
		{ID: "legacy_atomic_excluded_" + typedMode, CrawlJobID: job.ID, SnapshotID: "snapshot-excluded", Title: "Excluded", CanonicalURL: "https://docs.example.test/excluded", State: "validated", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + strings.Repeat("2", 64), Changed: true},
	}
	memory.mu.Lock()
	memory.crawls[source.ID] = []model.CrawlJob{job}
	memory.crawlReviewDocuments[job.ID] = legacyDocuments
	memory.knowledge[source.ProductID] = append(memory.knowledge[source.ProductID],
		model.KnowledgeRecord{ID: legacyDocuments[0].ID, ProductID: source.ProductID, SourceID: source.ID, Published: false, Visibility: model.VisibilityPrivate},
		model.KnowledgeRecord{ID: legacyDocuments[1].ID, ProductID: source.ProductID, SourceID: source.ID, Published: false, Visibility: model.VisibilityPrivate},
	)
	memory.mu.Unlock()
	publicationHash, err := docreview.PublicationContentHash(legacyDocuments[:1])
	if err != nil {
		t.Fatal(err)
	}
	publication := model.SourcePublication{
		ID: "publication_atomic_" + typedMode, CrawlJobID: job.ID, ContentHash: publicationHash,
		ReviewedBy: "reviewer", ReviewedAt: now, PublishedAt: now,
	}
	if typedMode == "missing_run" {
		return memoryDocumentationBridgeFixture{memory: memory, source: source, job: job, publication: publication}
	}
	if _, err := memory.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: job.ID, DeploymentID: source.ProductID, OrganisationID: source.OrganisationID,
		AssetKind: model.DeveloperAssetDocumentation, TargetID: source.ID, TargetKey: "source:" + source.ID, SourceID: source.ID,
		State: model.DeveloperAssetIngestionReviewReady, Attempt: 1,
		Versions:    model.ProcessorVersions{Pipeline: "pipeline-v1", Parser: "parser-v1", Normalizer: "normalizer-v1", Mapper: "mapper-v1"},
		RawManifest: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`{}`), DiscoveredCount: 2, AcquiredCount: 2,
		QueuedAt: now, StartedAt: &now, FinishedAt: &now,
	}); err != nil {
		t.Fatal(err)
	}
	secondMapping := legacyDocuments[1].ID
	if typedMode == "mapping_mismatch" {
		secondMapping = "legacy_not_in_exact_crawl"
	}
	output := DocumentationIngestionOutput{Documents: []model.DocumentationDocument{
		{ID: "typed_atomic_selected_" + typedMode, DeploymentID: source.ProductID, IngestionRunID: job.ID, LegacyKnowledgeDocumentID: legacyDocuments[0].ID, SourcePath: "selected.md", Title: "Selected", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "Selected", ContentHash: "sha256:" + strings.Repeat("3", 64), Visibility: model.VisibilityPrivate, Ordinal: 0, Metadata: json.RawMessage(`{}`)},
		{ID: "typed_atomic_excluded_" + typedMode, DeploymentID: source.ProductID, IngestionRunID: job.ID, LegacyKnowledgeDocumentID: secondMapping, SourcePath: "excluded.md", Title: "Excluded", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "Excluded", ContentHash: "sha256:" + strings.Repeat("4", 64), Visibility: model.VisibilityPrivate, Ordinal: 1, Metadata: json.RawMessage(`{}`)},
	}}
	if typedMode != "missing_map" {
		output.Map = &model.DocumentationMap{
			ID: "map_atomic_" + typedMode, DeploymentID: source.ProductID, IngestionRunID: job.ID, MapVersion: "map-v1",
			Map: model.DocumentationMapBody{Overview: "Atomic bridge map."}, AgentMarkdown: "# Atomic bridge map\n",
			ContentHash: "sha256:" + strings.Repeat("5", 64), Visibility: model.VisibilityPrivate,
		}
	}
	if err := memory.SaveDocumentationIngestionOutput(ctx, source.ProductID, output); err != nil {
		t.Fatal(err)
	}
	return memoryDocumentationBridgeFixture{memory: memory, source: source, job: job, publication: publication}
}

func TestMemorySourcePublicationTypedBridgeFailsAtomically(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"missing_run", "missing_map", "mapping_mismatch"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			fixture := newMemoryDocumentationBridgeFixture(t, "upload", mode)
			_, _, err := fixture.memory.PublishSource(context.Background(), fixture.source.ProductID, fixture.source.ID, fixture.source.Revision, fixture.publication, []string{"legacy_atomic_selected_" + mode})
			if err != ErrConflict {
				t.Fatalf("PublishSource error = %v, want ErrConflict", err)
			}
			storedSource, lookupErr := fixture.memory.Source(context.Background(), fixture.source.ProductID, fixture.source.ID)
			if lookupErr != nil || storedSource.Published || storedSource.Revision != fixture.source.Revision {
				t.Fatalf("source mutated after failed bridge: %#v, err=%v", storedSource, lookupErr)
			}
			if _, lookupErr = fixture.memory.SourcePublication(context.Background(), fixture.source.ProductID, fixture.publication.ID); lookupErr != ErrNotFound {
				t.Fatalf("legacy publication survived failed bridge: %v", lookupErr)
			}
			if _, lookupErr = fixture.memory.SourcePublicationDocumentationReview(context.Background(), fixture.source.ProductID, fixture.publication.ID); lookupErr != ErrNotFound {
				t.Fatalf("typed review survived failed bridge: %v", lookupErr)
			}
			fixture.memory.mu.RLock()
			for _, record := range fixture.memory.knowledge[fixture.source.ProductID] {
				if record.SourceID == fixture.source.ID && record.Published {
					fixture.memory.mu.RUnlock()
					t.Fatalf("legacy document %s was published after failed bridge", record.ID)
				}
			}
			fixture.memory.mu.RUnlock()
			if mode != "missing_run" {
				run, runErr := fixture.memory.DeveloperAssetIngestionRun(context.Background(), fixture.source.ProductID, fixture.job.ID)
				if runErr != nil || run.State != model.DeveloperAssetIngestionReviewReady {
					t.Fatalf("typed run mutated after failed bridge: %#v, err=%v", run, runErr)
				}
			}
		})
	}
}

func TestMemoryUnboundOpenAPISourcePublicationPreservesLegacyCompatibility(t *testing.T) {
	t.Parallel()
	fixture := newMemoryDocumentationBridgeFixture(t, "openapi", "missing_run")
	updated, publication, err := fixture.memory.PublishSource(context.Background(), fixture.source.ProductID, fixture.source.ID, fixture.source.Revision, fixture.publication, []string{"legacy_atomic_selected_missing_run"})
	if err != nil || !updated.Published || publication.ID != fixture.publication.ID {
		t.Fatalf("unbound OpenAPI legacy publication = %#v source=%#v err=%v", publication, updated, err)
	}
	if _, err := fixture.memory.SourcePublicationDocumentationReview(context.Background(), fixture.source.ProductID, publication.ID); err != ErrNotFound {
		t.Fatalf("unbound OpenAPI unexpectedly acquired a documentation review: %v", err)
	}
}

func TestMemoryUnchangedUploadedContractSourcePreservesLegacyCompatibility(t *testing.T) {
	t.Parallel()
	fixture := newMemoryDocumentationBridgeFixture(t, "upload", "missing_run")
	contract, err := fixture.memory.SaveAPIContract(context.Background(), model.APIContract{
		ID: "contract-unchanged-upload", DeploymentID: fixture.source.ProductID, OrganisationID: fixture.source.OrganisationID,
		Name: "Unchanged upload", Slug: "unchanged-upload", Kind: "openapi", Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.memory.SaveAPIContractSource(context.Background(), model.APIContractSource{
		ID: "contract-source-unchanged-upload", DeploymentID: fixture.source.ProductID, APIContractID: contract.ID,
		SourceID: fixture.source.ID, SourceRole: "primary", Lifecycle: "attached", CreatedBy: "reviewer",
	}, 0); err != nil {
		t.Fatal(err)
	}
	updated, publication, err := fixture.memory.PublishSource(context.Background(), fixture.source.ProductID, fixture.source.ID, fixture.source.Revision, fixture.publication, []string{"legacy_atomic_selected_missing_run"})
	if err != nil || !updated.Published || publication.ID != fixture.publication.ID {
		t.Fatalf("unchanged uploaded contract legacy publication = %#v source=%#v err=%v", publication, updated, err)
	}
}

func TestTypedSourcePublicationBridgeUsesStableQuarantineDecision(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	run := model.DeveloperAssetIngestionRun{
		ID: "crawl-quarantine", DeploymentID: "prod_acme", OrganisationID: "org_acme",
		AssetKind: model.DeveloperAssetDocumentation, TargetID: "source-quarantine", TargetKey: "source:source-quarantine", SourceID: "source-quarantine",
		State: model.DeveloperAssetIngestionReviewReady, Attempt: 1, AcquiredCount: 2, QuarantinedCount: 1,
		Versions: model.ProcessorVersions{Pipeline: "pipeline-v1", Parser: "parser-v1", Normalizer: "normalizer-v1", Mapper: "mapper-v1"},
	}
	publication := model.SourcePublication{
		ID: "publication-quarantine", ProductID: run.DeploymentID, SourceID: run.SourceID, CrawlJobID: run.ID,
		Visibility: model.VisibilityPrivate, ReviewedBy: "reviewer", ReviewedAt: now,
	}
	documents := []model.DocumentationDocument{
		{ID: "typed-safe", DeploymentID: run.DeploymentID, IngestionRunID: run.ID, LegacyKnowledgeDocumentID: "legacy-safe", ContentHash: "sha256:" + strings.Repeat("1", 64), Visibility: model.VisibilityPrivate, Ordinal: 0},
		{ID: "typed-unsafe", DeploymentID: run.DeploymentID, IngestionRunID: run.ID, LegacyKnowledgeDocumentID: "legacy-unsafe", ContentHash: "sha256:" + strings.Repeat("2", 64), Visibility: model.VisibilityPrivate, Ordinal: 1},
	}
	documentationMap := &model.DocumentationMap{
		ID: "map-quarantine", DeploymentID: run.DeploymentID, IngestionRunID: run.ID, MapVersion: "map-v1", AgentMarkdown: "# Quarantine map\n",
		ContentHash: "sha256:" + strings.Repeat("3", 64), Visibility: model.VisibilityPrivate,
	}
	legacyDocuments := map[string]model.CrawlReviewDocument{
		"legacy-safe":   {ID: "legacy-safe", CrawlJobID: run.ID, State: "validated", InjectionIndicators: json.RawMessage(`[]`)},
		"legacy-unsafe": {ID: "legacy-unsafe", CrawlJobID: run.ID, State: "quarantined", InjectionIndicators: json.RawMessage(`["instruction_override"]`)},
	}
	review, err := buildSourcePublicationDocumentationReview(run.DeploymentID, run, publication, documents, documentationMap, legacyDocuments, []string{"legacy-safe"})
	if err != nil || len(review.Selections) != 2 {
		t.Fatalf("review=%#v err=%v", review, err)
	}
	if review.Selections[1].Decision != "quarantined" || review.Selections[1].Reason != sourcePublicationDocumentQuarantinedReason || review.Selections[1].Ordinal != nil {
		t.Fatalf("quarantine decision=%#v", review.Selections[1])
	}
}
