package store

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func testSourceReplacementQueueAndAudit(t *testing.T, backend Store) {
	t.Helper()
	ctx := t.Context()
	deployment, err := backend.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	makeInput := func() (model.Source, SourceInputReplacement) {
		t.Helper()
		source, err := backend.CreateSource(ctx, model.Source{ID: storeTestUUID(t), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, Kind: "upload", Location: "original.md", Name: "Replacement concurrency"})
		if err != nil {
			t.Fatal(err)
		}
		return source, SourceInputReplacement{ProductID: source.ProductID, SourceID: source.ID, ExpectedRevision: source.Revision, Location: "replacement.md", RequestDigest: strings.Repeat("a", 64), InputDigest: strings.Repeat("b", 64), CrawlJobID: storeTestUUID(t), Audit: model.AuditEvent{ID: storeTestUUID(t), OrganisationID: source.OrganisationID, ProductID: source.ProductID, ActorID: "reviewer", Action: "source.input.replaced", TargetType: "source", TargetID: source.ID, CreatedAt: time.Now().UTC()}}
	}

	// Quarantine is worker-owned state. Replacing bytes cannot clear it.
	quarantined, replacement := makeInput()
	switch b := backend.(type) {
	case *Memory:
		value := b.sources[deployment.ID][quarantined.ID]
		value.Quarantined = true
		b.sources[deployment.ID][quarantined.ID] = value
	case *Postgres:
		if _, err := b.pool.Exec(ctx, `UPDATE sources SET state='quarantined' WHERE id=$1`, quarantined.ID); err != nil {
			t.Fatal(err)
		}
	}
	replaced, err := backend.ReplaceSourceInput(ctx, replacement)
	if err != nil || !replaced.Source.Quarantined {
		t.Fatalf("replacement cleared quarantine: %#v %v", replaced, err)
	}
	// Even a worker whose lease expired still owns an active import's input.
	switch b := backend.(type) {
	case *Memory:
		b.crawls[quarantined.ID][0].State = "running"
	case *Postgres:
		if _, err := b.pool.Exec(ctx, `UPDATE crawl_jobs SET state='running',lease_owner='expired-fixture',lease_expires_at=now()-interval '1 minute' WHERE id=$1`, replaced.CrawlJob.ID); err != nil {
			t.Fatal(err)
		}
	}
	replacement.RequestDigest = strings.Repeat("c", 64)
	replacement.ExpectedRevision = replaced.Source.Revision
	replacement.CrawlJobID = storeTestUUID(t)
	if _, err := backend.ReplaceSourceInput(ctx, replacement); !errors.Is(err, ErrSourceImportActive) {
		t.Fatalf("replacement changed running input: %v", err)
	}
	source, input := makeInput()
	event := input.Audit
	event.Outcome = "success"
	if err := backend.AppendAudit(ctx, event); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.ReplaceSourceInput(ctx, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate audit must roll back: %v", err)
	}
	current, err := backend.Source(ctx, source.ProductID, source.ID)
	if err != nil || current.Location != source.Location || current.Revision != source.Revision {
		t.Fatalf("audit failure changed source: %#v %v", current, err)
	}
	jobs, err := backend.CrawlJobs(ctx, source.ProductID, source.ID)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("audit failure queued import: %#v %v", jobs, err)
	}
	if _, err := backend.SourceInputReplacement(ctx, source.ProductID, source.ID, input.RequestDigest); !errors.Is(err, ErrNotFound) {
		t.Fatalf("audit failure retained request: %v", err)
	}
	// A queue racing replacement must consume either the original or replacement
	// file; a replacement can never change the input of the winning queued job.
	for range 8 {
		source, input := makeInput()
		start := make(chan struct{})
		var group sync.WaitGroup
		group.Add(2)
		var replaceErr, queueErr error
		go func() { defer group.Done(); <-start; _, replaceErr = backend.ReplaceSourceInput(ctx, input) }()
		go func() {
			defer group.Done()
			<-start
			_, queueErr = backend.CreateCrawlJob(ctx, model.CrawlJob{ID: storeTestUUID(t), OrganisationID: source.OrganisationID, ProductID: source.ProductID, SourceID: source.ID})
		}()
		close(start)
		group.Wait()
		current, err := backend.Source(ctx, source.ProductID, source.ID)
		if err != nil {
			t.Fatal(err)
		}
		jobs, err := backend.CrawlJobs(ctx, source.ProductID, source.ID)
		if err != nil || len(jobs) != 1 {
			t.Fatalf("race imports: %#v %v", jobs, err)
		}
		if replaceErr == nil {
			if !errors.Is(queueErr, ErrConflict) || jobs[0].ID != input.CrawlJobID || current.Location != input.Location {
				t.Fatalf("replacement winner: %#v %v", current, queueErr)
			}
		} else if !errors.Is(replaceErr, ErrSourceImportActive) || queueErr != nil || current.Location != source.Location || current.Revision != source.Revision {
			t.Fatalf("queue winner: %#v %v %v", current, replaceErr, queueErr)
		}
	}
}
func TestMemorySourceReplacementQueueAndAudit(t *testing.T) {
	testSourceReplacementQueueAndAudit(t, NewMemory())
}
func TestPostgresSourceReplacementQueueAndAudit(t *testing.T) {
	_, backend := migratedPostgresForStoreTest(t)
	if _, err := backend.Deployment(t.Context()); err == ErrNotFound {
		id := storeTestUUID(t)
		org, err := backend.CreateOrganisation(t.Context(), model.Organisation{ID: id, Name: "Replacements", Slug: "replace-" + id})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = backend.CreateDeployment(t.Context(), model.Deployment{ID: storeTestUUID(t), OrganisationID: org.ID, Name: "Replacements", Slug: "replacements"}); err != nil {
			t.Fatal(err)
		}
	}
	testSourceReplacementQueueAndAudit(t, backend)
}
