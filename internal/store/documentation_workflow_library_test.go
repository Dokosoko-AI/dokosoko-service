package store

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

// Seed immutable crawler output directly: the crawler, not the Go service,
// writes terminal jobs. Exercise the same list/read contracts in both stores.
func seedDocumentationLibraryJob(t *testing.T, backend Store, source model.Source, index int, state string, bodies map[string]string) (model.CrawlJob, []model.DocumentationDocument) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond).Add(time.Duration(index-24) * time.Hour)
	job := model.CrawlJob{ID: storeTestUUID(t), ProductID: source.ProductID, OrganisationID: source.OrganisationID, SourceID: source.ID, State: state, QueuedAt: now, FetchedCount: len(bodies), ChangedCount: len(bodies)}
	switch b := backend.(type) {
	case *Memory:
		b.crawls[source.ID] = append(b.crawls[source.ID], job)
	case *Postgres:
		_, err := b.pool.Exec(t.Context(), `INSERT INTO crawl_jobs(id,product_id,organisation_id,source_id,state,queued_at,fetched_count,changed_count) VALUES($1,$2,$3,$4,$5,$6,$7,$7)`, job.ID, job.ProductID, job.OrganisationID, job.SourceID, job.State, job.QueuedAt, job.FetchedCount)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(bodies) == 0 {
		return job, nil
	}
	_, err := backend.CreateDeveloperAssetIngestionRun(t.Context(), model.DeveloperAssetIngestionRun{ID: job.ID, DeploymentID: source.ProductID, OrganisationID: source.OrganisationID, AssetKind: model.DeveloperAssetDocumentation, TargetID: source.ID, TargetKey: "source:" + source.ID, SourceID: source.ID, State: model.DeveloperAssetIngestionReviewReady, Attempt: 1, Versions: model.ProcessorVersions{Pipeline: "v1", Parser: "v1", Normalizer: "v1", Mapper: "v1"}, RawManifest: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`{}`), QueuedAt: now, StartedAt: &now})
	if err != nil {
		t.Fatal(err)
	}
	documents := []model.DocumentationDocument{}
	for path, body := range bodies {
		documents = append(documents, model.DocumentationDocument{ID: storeTestUUID(t), DeploymentID: source.ProductID, IngestionRunID: job.ID, SourcePath: path, Title: path, Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: body, ContentHash: developerAssetTestHash(string(rune('a' + index))), Visibility: source.Visibility, Ordinal: len(documents), Metadata: json.RawMessage(`{}`)})
	}
	if err = backend.SaveDocumentationIngestionOutput(t.Context(), source.ProductID, DocumentationIngestionOutput{Documents: documents}); err != nil {
		t.Fatal(err)
	}
	return job, documents
}

func seedDocumentationLibraryReview(t *testing.T, backend Store, source model.Source, job model.CrawlJob, revision int64, documents []model.DocumentationDocument, excluded string) model.SourcePublication {
	t.Helper()
	publication := model.SourcePublication{ID: storeTestUUID(t), ProductID: source.ProductID, OrganisationID: source.OrganisationID, SourceID: source.ID, CrawlJobID: job.ID, Revision: revision, Visibility: source.Visibility, ContentHash: developerAssetTestHash("f"), DocumentCount: len(documents), ReviewedBy: "reviewer", ReviewedAt: job.QueuedAt, PublishedAt: job.QueuedAt}
	if excluded != "" {
		publication.DocumentCount--
	}
	switch b := backend.(type) {
	case *Memory:
		b.sourcePublications[source.ProductID][publication.ID] = publication
	case *Postgres:
		_, err := b.pool.Exec(t.Context(), `INSERT INTO source_publications(id,product_id,organisation_id,source_id,crawl_job_id,revision,visibility,content_hash,document_count,reviewed_by,reviewed_at,published_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)`, publication.ID, publication.ProductID, publication.OrganisationID, publication.SourceID, publication.CrawlJobID, publication.Revision, publication.Visibility, publication.ContentHash, publication.DocumentCount, publication.ReviewedBy, publication.ReviewedAt)
		if err != nil {
			t.Fatal(err)
		}
	}
	review := SourcePublicationDocumentationReview{}
	for _, document := range documents {
		ordinal := len(review.Selections)
		selection := model.SourcePublicationDocumentSelection{SourcePublicationID: publication.ID, DeploymentID: source.ProductID, DocumentationDocumentID: document.ID, Decision: "included", Ordinal: &ordinal, ContentHash: document.ContentHash, ReviewedBy: publication.ReviewedBy, ReviewedAt: publication.ReviewedAt}
		if document.SourcePath == excluded {
			selection.Decision, selection.Ordinal, selection.Reason = "excluded", nil, "obsolete"
		}
		review.Selections = append(review.Selections, selection)
	}
	if err := backend.SaveSourcePublicationDocumentationReview(t.Context(), source.ProductID, review); err != nil {
		t.Fatal(err)
	}
	return publication
}

func testDocumentationWorkflowLibrary(t *testing.T, backend Store) {
	t.Helper()
	ctx := t.Context()
	deployment, err := backend.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	source, err := backend.CreateSource(ctx, model.Source{ID: storeTestUUID(t), ProductID: deployment.ID, OrganisationID: deployment.OrganisationID, Name: "Pending upload", Kind: "upload", Location: "private-storage-sentinel.md", Visibility: model.VisibilityPrivate})
	if err != nil {
		t.Fatal(err)
	}
	assertAttention := func(status, jobID string) {
		t.Helper()
		page, err := backend.DocumentationAttention(ctx, deployment.ID, source.ID, 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		if status == "" {
			if page.Total != 0 || len(page.Items) != 0 {
				t.Fatalf("finished source remains pending: %#v", page)
			}
			return
		}
		if page.Total != 1 || len(page.Items) != 1 || page.HasMore || page.Items[0].Status != status || page.Items[0].CrawlJobID != jobID {
			t.Fatalf("want %s/%s, got %#v", status, jobID, page)
		}
		wire, _ := json.Marshal(page)
		for _, secret := range []string{"private-storage-sentinel", "normalized_markdown", "diagnostics", "error_message"} {
			if strings.Contains(string(wire), secret) {
				t.Fatalf("summary leaked %s", wire)
			}
		}
		next, err := backend.DocumentationAttention(ctx, deployment.ID, source.ID, 1, 1)
		if err != nil || next.Total != 1 || len(next.Items) != 0 || next.HasMore {
			t.Fatalf("offset: %#v %v", next, err)
		}
	}
	assertAttention("not_imported", "")
	queued, _ := seedDocumentationLibraryJob(t, backend, source, 0, "queued", nil)
	assertAttention("importing", queued.ID)
	failed, _ := seedDocumentationLibraryJob(t, backend, source, 1, "failed", nil)
	assertAttention("import_failed", failed.ID)
	first, firstDocs := seedDocumentationLibraryJob(t, backend, source, 2, "review", map[string]string{"guide.md": "approved first", "removed.md": "obsoletephrase"})
	assertAttention("review", first.ID)
	seedDocumentationLibraryReview(t, backend, source, first, 1, firstDocs, "")
	assertAttention("save_version", first.ID)
	query := DocumentationLibraryQuery{DeploymentID: deployment.ID, SourceID: source.ID, Reviewed: true, Limit: 50}
	page, err := backend.DocumentationLibrary(ctx, query)
	if err != nil || page.Total != 2 {
		t.Fatalf("first review: %#v %v", page, err)
	}
	var original string
	for _, item := range page.Items {
		if item.SourcePath == "guide.md" {
			original = item.ID
		}
	}
	middle, _ := seedDocumentationLibraryJob(t, backend, source, 3, "review", map[string]string{"guide.md": "unapprovedphrase"})
	assertAttention("review", middle.ID)
	query.Query = "unapprovedphrase"
	page, err = backend.DocumentationLibrary(ctx, query)
	if err != nil || page.Total != 0 {
		t.Fatalf("unreviewed replaced approved: %#v %v", page, err)
	}
	query.Query = "obsoletephrase"
	page, err = backend.DocumentationLibrary(ctx, query)
	if err != nil || page.Total != 1 {
		t.Fatalf("pending removal removed approved: %#v %v", page, err)
	}
	final, finalDocs := seedDocumentationLibraryJob(t, backend, source, 4, "review", map[string]string{"guide.md": "approved final", "excluded.md": "excludedphrase"})
	publication := seedDocumentationLibraryReview(t, backend, source, final, 2, finalDocs, "excluded.md")
	query.Query = ""
	page, err = backend.DocumentationLibrary(ctx, query)
	if err != nil || page.Total != 1 || page.Items[0].SourcePath != "guide.md" || page.Items[0].Decision != "included" || page.Items[0].PreviousDocumentID != original || !page.Items[0].QueuedAt.Equal(final.QueuedAt) {
		t.Fatalf("latest reviewed selection/comparison: %#v %v", page, err)
	}
	for _, needle := range []string{"obsoletephrase", "excludedphrase", "unapprovedphrase"} {
		query.Query = needle
		page, err = backend.DocumentationLibrary(ctx, query)
		if err != nil || page.Total != 0 {
			t.Fatalf("resurrected %s: %#v %v", needle, page, err)
		}
	}
	query.Query = "approved final"
	page, err = backend.DocumentationLibrary(ctx, query)
	if err != nil || page.Total != 1 {
		t.Fatalf("body search: %#v %v", page, err)
	}
	wire, _ := json.Marshal(page)
	if strings.Contains(string(wire), "approved final") || strings.Contains(string(wire), "normalized_markdown") {
		t.Fatalf("list hydrated body: %s", wire)
	}
	old, err := backend.DocumentationCandidateDocument(ctx, deployment.ID, original)
	if err != nil || old.Document.NormalizedMarkdown != "approved first" {
		t.Fatalf("historical content changed: %#v %v", old, err)
	}
	assertAttention("save_version", final.ID)
	// A partial or differently addressed collection does not finish the source setup.
	for _, selector := range []string{`{"paths":["guide.md"]}`, `{}`} {
		rootID, revisionID := storeTestUUID(t), storeTestUUID(t)
		_, err = backend.CreateDocumentationCollection(ctx, model.DocumentationCollection{ID: rootID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, Name: "Reviewed docs", Slug: "reviewed-" + rootID, Visibility: source.Visibility, Lifecycle: "active"}, DocumentationCollectionRevisionRecord{Revision: model.DocumentationCollectionRevision{ID: revisionID, DeploymentID: deployment.ID, DocumentationCollectionID: rootID, Revision: 1, Visibility: source.Visibility, ContentHash: developerAssetTestHash("e"), SelectionManifest: json.RawMessage(`[]`), ReviewedBy: "reviewer", ReviewedAt: final.QueuedAt, PublishedAt: final.QueuedAt}, Members: []model.DocumentationCollectionMember{{ID: storeTestUUID(t), DocumentationCollectionRevisionID: revisionID, Kind: "source_publication", SourcePublicationID: publication.ID, IncludeDescendants: true, Selector: json.RawMessage(selector)}}})
		if err != nil {
			t.Fatal(err)
		}
		if selector == `{}` {
			assertAttention("", "")
		} else {
			assertAttention("save_version", final.ID)
		}
	}
	// A later failed import remains visible, without changing reviewed files.
	latest, _ := seedDocumentationLibraryJob(t, backend, source, 5, "failed", nil)
	assertAttention("import_failed", latest.ID)
	page, err = backend.DocumentationLibrary(ctx, query)
	if err != nil || page.Total != 1 {
		t.Fatalf("failed import replaced reviewed content: %#v %v", page, err)
	}

	// Coverage failures and quarantine remain visible, even with older reviewed versions.
	blocked, _ := seedDocumentationLibraryJob(t, backend, source, 6, "review", nil)
	for _, issue := range []string{"failed", "skipped", "quarantined"} {
		switch b := backend.(type) {
		case *Memory:
			for i := range b.crawls[source.ID] {
				if b.crawls[source.ID][i].ID == blocked.ID {
					b.crawls[source.ID][i].FailedCount = 0
					b.crawls[source.ID][i].SkippedCount = 0
					if issue == "failed" {
						b.crawls[source.ID][i].FailedCount = 1
					}
					if issue == "skipped" {
						b.crawls[source.ID][i].SkippedCount = 1
					}
				}
			}
			if issue == "quarantined" {
				value := b.sources[deployment.ID][source.ID]
				value.Quarantined = true
				b.sources[deployment.ID][source.ID] = value
			}
		case *Postgres:
			failed, skipped := 0, 0
			if issue == "failed" {
				failed = 1
			}
			if issue == "skipped" {
				skipped = 1
			}
			if _, err := b.pool.Exec(ctx, `UPDATE crawl_jobs SET failed_count=$2,skipped_count=$3 WHERE id=$1`, blocked.ID, failed, skipped); err != nil {
				t.Fatal(err)
			}
			if issue == "quarantined" {
				if _, err := b.pool.Exec(ctx, `UPDATE sources SET state='quarantined' WHERE id=$1`, source.ID); err != nil {
					t.Fatal(err)
				}
			}
		}
		assertAttention("blocked", blocked.ID)
	}
	// Contract inputs belong to their contract workflow, and return after detachment.
	contractID := storeTestUUID(t)
	_, err = backend.SaveAPIContract(ctx, model.APIContract{ID: contractID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, Name: "Contract input", Slug: "contract-" + contractID, Kind: "openapi", Visibility: source.Visibility, Lifecycle: "active"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := backend.SaveAPIContractSource(ctx, model.APIContractSource{ID: storeTestUUID(t), DeploymentID: deployment.ID, APIContractID: contractID, SourceID: source.ID, SourceRole: "primary", Lifecycle: "attached", CreatedBy: "reviewer"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertAttention("", "")
	if _, err = backend.DetachAPIContractSource(ctx, deployment.ID, binding.ID, binding.Revision); err != nil {
		t.Fatal(err)
	}
	assertAttention("blocked", blocked.ID)
	query.DeploymentID = storeTestUUID(t)
	page, err = backend.DocumentationLibrary(ctx, query)
	if err != nil || page.Total != 0 {
		t.Fatalf("foreign reviewed content: %#v %v", page, err)
	}
	attention, err := backend.DocumentationAttention(ctx, query.DeploymentID, source.ID, 50, 0)
	if err != nil || attention.Total != 0 {
		t.Fatalf("foreign source metadata: %#v %v", attention, err)
	}
}

func TestMemoryDocumentationWorkflowLibrary(t *testing.T) {
	testDocumentationWorkflowLibrary(t, NewMemory())
}
func TestPostgresDocumentationWorkflowLibrary(t *testing.T) {
	_, backend := migratedPostgresForStoreTest(t)
	if _, err := backend.Deployment(t.Context()); err == ErrNotFound {
		id := storeTestUUID(t)
		org, err := backend.CreateOrganisation(t.Context(), model.Organisation{ID: id, Name: "Workflow library", Slug: "workflow-" + id})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = backend.CreateDeployment(t.Context(), model.Deployment{ID: storeTestUUID(t), OrganisationID: org.ID, Name: "Workflow library", Slug: "workflow-library"}); err != nil {
			t.Fatal(err)
		}
	}
	testDocumentationWorkflowLibrary(t, backend)
}
