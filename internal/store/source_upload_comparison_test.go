package store

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

// Mirror the crawler's two immutable representations: extracted source text
// and normalized documentation. Their IDs and hashes intentionally differ.
func seedUploadComparisonImport(t *testing.T, backend Store, source model.Source, index int, bodies map[string]string) (model.CrawlJob, []model.DocumentationDocument, map[string]model.CrawlReviewDocument) {
	t.Helper()
	job, documents := seedDocumentationLibraryJob(t, backend, source, index, "running", bodies)
	extracted := map[string]model.CrawlReviewDocument{}
	for _, document := range documents {
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(document.NormalizedMarkdown)))
		value := model.CrawlReviewDocument{ID: storeTestUUID(t), CrawlJobID: job.ID, SnapshotID: storeTestUUID(t), Title: document.Title, CanonicalURL: document.SourcePath, State: "validated", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + hash, Changed: true}
		extracted[document.SourcePath] = value
		switch b := backend.(type) {
		case *Memory:
			b.crawlReviewDocuments[job.ID] = append(b.crawlReviewDocuments[job.ID], value)
			b.knowledge[source.ProductID] = append(b.knowledge[source.ProductID], model.KnowledgeRecord{ID: value.ID, ProductID: source.ProductID, SourceID: source.ID, Title: value.Title, Text: document.NormalizedMarkdown, URL: value.CanonicalURL, Visibility: source.Visibility})
		case *Postgres:
			for _, statement := range []struct {
				sql  string
				args []any
			}{
				{`INSERT INTO source_snapshots(id,organisation_id,product_id,source_id,crawl_job_id,canonical_url,object_key,content_sha256,content_type,response_status,trust_indicators) VALUES($1,$2,$3,$4,$5,$6,$7,decode($8,'hex'),'text/markdown',200,'{}')`, []any{value.SnapshotID, source.OrganisationID, source.ProductID, source.ID, job.ID, value.CanonicalURL, "comparison-" + value.SnapshotID, hash}},
				{`INSERT INTO knowledge_documents(id,organisation_id,product_id,source_id,snapshot_id,title,canonical_url,body,visibility,state,trust_level,injection_indicators) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'validated',70,'[]')`, []any{value.ID, source.OrganisationID, source.ProductID, source.ID, value.SnapshotID, value.Title, value.CanonicalURL, document.NormalizedMarkdown, source.Visibility}},
				{`INSERT INTO crawl_job_documents(crawl_job_id,knowledge_document_id,changed,assessment_state,assessment_trust_level,assessment_injection_indicators) VALUES($1,$2,true,'validated',70,'[]')`, []any{job.ID, value.ID}},
			} {
				if _, err := b.pool.Exec(t.Context(), statement.sql, statement.args...); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	job.State, job.FinishedAt = "review", &job.QueuedAt
	switch b := backend.(type) {
	case *Memory:
		for index := range b.crawls[source.ID] {
			if b.crawls[source.ID][index].ID == job.ID {
				b.crawls[source.ID][index] = job
			}
		}
	case *Postgres:
		if _, err := b.pool.Exec(t.Context(), `UPDATE crawl_jobs SET state='review',finished_at=$2 WHERE id=$1`, job.ID, job.QueuedAt); err != nil {
			t.Fatal(err)
		}
	}
	return job, documents, extracted
}

func seedUploadComparisonPublication(t *testing.T, backend Store, source model.Source, job model.CrawlJob, revision int64, documents []model.DocumentationDocument, extracted map[string]model.CrawlReviewDocument, excluded string) model.SourcePublication {
	t.Helper()
	publication := seedDocumentationLibraryReview(t, backend, source, job, revision, documents, excluded)
	for path, document := range extracted {
		if path == excluded {
			continue
		}
		switch b := backend.(type) {
		case *Memory:
			if b.publicationDocuments[publication.ID] == nil {
				b.publicationDocuments[publication.ID] = map[string]bool{}
			}
			b.publicationDocuments[publication.ID][document.ID] = true
		case *Postgres:
			if _, err := b.pool.Exec(t.Context(), `INSERT INTO source_publication_documents(source_publication_id,knowledge_document_id) VALUES($1,$2)`, publication.ID, document.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	return publication
}

func testUploadReviewComparisons(t *testing.T, backend Store) {
	t.Helper()
	ctx := t.Context()
	deployment, err := backend.Deployment(ctx)
	if errors.Is(err, ErrNotFound) {
		id := storeTestUUID(t)
		org, createErr := backend.CreateOrganisation(ctx, model.Organisation{ID: id, Name: "Upload comparisons", Slug: "comparisons-" + id})
		if createErr != nil {
			t.Fatal(createErr)
		}
		deployment, err = backend.CreateDeployment(ctx, model.Deployment{ID: storeTestUUID(t), OrganisationID: org.ID, Name: "Upload comparisons", Slug: "upload-comparisons"})
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name, kind                 string
		beforePaths, afterPaths    []string
		excluded, expectedPrevious string
	}{
		{"replaced file", "upload", []string{"old.md"}, []string{"new.md"}, "", "old.md"},
		{"same path", "upload", []string{"old.md"}, []string{"old.md"}, "", "old.md"},
		{"website rename", "website", []string{"old.md"}, []string{"new.md"}, "", ""},
		{"ambiguous previous import even with one included file", "upload", []string{"old.md", "extra.md"}, []string{"new.md"}, "extra.md", ""},
		{"ambiguous current import", "upload", []string{"old.md"}, []string{"new.md", "extra.md"}, "", ""},
		{"exact path survives ambiguous imports", "upload", []string{"old.md", "extra.md"}, []string{"old.md", "more.md"}, "extra.md", "old.md"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			createSource := func() model.Source {
				value, err := backend.CreateSource(ctx, model.Source{ID: storeTestUUID(t), ProductID: deployment.ID, OrganisationID: deployment.OrganisationID, Name: scenario.name, Kind: scenario.kind, Location: "original-storage.md", Visibility: model.VisibilityPrivate})
				if err != nil {
					t.Fatal(err)
				}
				return value
			}
			source := createSource()
			path := func(name string) string {
				if scenario.kind == "website" {
					return "https://docs.example.test/" + name
				}
				return "upload://storage/" + name
			}
			bodies := func(paths []string, body string) map[string]string {
				values := map[string]string{}
				for _, name := range paths {
					values[path(name)] = body + " " + name
				}
				return values
			}
			first, firstDocs, firstText := seedUploadComparisonImport(t, backend, source, 0, bodies(scenario.beforePaths, "Approved original"))
			excluded := ""
			if scenario.excluded != "" {
				excluded = path(scenario.excluded)
			}
			firstPublication := seedUploadComparisonPublication(t, backend, source, first, 1, firstDocs, firstText, excluded)
			currentPath := path(scenario.afterPaths[0])
			seedUploadComparisonImport(t, backend, source, 1, map[string]string{currentPath: "Unapproved intermediate"})
			omitted, omittedDocs, omittedText := seedUploadComparisonImport(t, backend, source, 2, map[string]string{currentPath: "Excluded intermediate", path("unrelated.md"): "Approved unrelated"})
			seedUploadComparisonPublication(t, backend, source, omitted, 2, omittedDocs, omittedText, currentPath)
			foreign := createSource()
			foreignJob, foreignDocs, foreignText := seedUploadComparisonImport(t, backend, foreign, 3, map[string]string{currentPath: "Foreign source private text"})
			seedUploadComparisonPublication(t, backend, foreign, foreignJob, 99, foreignDocs, foreignText, "")
			current, currentDocs, currentText := seedUploadComparisonImport(t, backend, source, 4, bodies(scenario.afterPaths, "Replacement text"))
			content, err := backend.SourceReviewContent(ctx, deployment.ID, source.ID, current.ID, currentText[currentPath].ID)
			if err != nil || content.Body != "Replacement text "+scenario.afterPaths[0] || content.Document.CanonicalURL != currentPath {
				t.Fatalf("current extracted content: %#v %v", content, err)
			}
			if scenario.expectedPrevious == "" {
				if content.Previous != nil {
					t.Fatalf("ambiguous comparison returned: %#v", content.Previous)
				}
			} else {
				original := firstText[path(scenario.expectedPrevious)]
				if content.Previous == nil || content.Previous.PublicationID != firstPublication.ID || content.Previous.DocumentID != original.ID || content.Previous.ContentHash != original.ContentHash || content.Previous.Body != "Approved original "+scenario.expectedPrevious {
					t.Fatalf("approved extracted comparison: %#v", content.Previous)
				}
			}
			seedUploadComparisonPublication(t, backend, source, current, 3, currentDocs, currentText, "")
			page, err := backend.DocumentationLibrary(ctx, DocumentationLibraryQuery{DeploymentID: deployment.ID, SourceID: source.ID, Reviewed: true, Limit: 50})
			if err != nil || len(page.Items) != len(currentDocs) {
				t.Fatalf("reviewed library: %#v %v", page, err)
			}
			expectedID := ""
			for _, original := range firstDocs {
				if scenario.expectedPrevious != "" && original.SourcePath == path(scenario.expectedPrevious) {
					expectedID = original.ID
				}
			}
			found := false
			for _, item := range page.Items {
				if item.SourcePath == currentPath {
					found = true
					if item.PreviousDocumentID != expectedID {
						t.Fatalf("normalized comparison=%s, want %s: %#v", item.PreviousDocumentID, expectedID, item)
					}
				}
			}
			if !found {
				t.Fatal("replacement missing from reviewed library")
			}
			for _, original := range firstDocs {
				stored, err := backend.DocumentationCandidateDocument(ctx, deployment.ID, original.ID)
				if err != nil || stored.Document.ContentHash != original.ContentHash || stored.Document.SourcePath != original.SourcePath || stored.Document.NormalizedMarkdown != original.NormalizedMarkdown {
					t.Fatalf("historical evidence changed: %#v %v", stored, err)
				}
			}
			old, err := backend.SourceReviewContent(ctx, deployment.ID, source.ID, first.ID, firstText[path(scenario.beforePaths[0])].ID)
			if err != nil || old.Previous != nil || old.Body != "Approved original "+scenario.beforePaths[0] {
				t.Fatalf("future replacement became historical comparison: %#v %v", old, err)
			}
			if value, err := backend.SourceReviewContent(ctx, deployment.ID, foreign.ID, current.ID, currentText[currentPath].ID); !errors.Is(err, ErrNotFound) || value.Body != "" || value.Previous != nil {
				t.Fatalf("foreign membership read: %#v %v", value, err)
			}
		})
	}
}

func TestMemoryUploadReviewComparisons(t *testing.T) { testUploadReviewComparisons(t, NewMemory()) }
func TestPostgresUploadReviewComparisons(t *testing.T) {
	_, backend := migratedPostgresForStoreTest(t)
	testUploadReviewComparisons(t, backend)
}
