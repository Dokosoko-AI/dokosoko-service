package platform

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func testSourceInputReplacement(t *testing.T, backend store.Store) {
	t.Helper()
	ctx := t.Context()
	deployment, err := backend.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	service := New(backend)
	actor := Actor{ID: "replacement-reviewer"}
	source, err := service.CreateSource(ctx, deployment.OrganisationID, deployment.ID, "Recovery source", "upload", "old-file.md", actor)
	if err != nil {
		t.Fatal(err)
	}
	source.Visibility = model.VisibilityPublic
	source, err = backend.UpdateSource(ctx, source, source.Revision)
	if err != nil {
		t.Fatal(err)
	}
	input := SourceInputReplacementInput{ProductID: source.ProductID, SourceID: source.ID, Revision: source.Revision, Location: "clean-file.md", Filename: "clean.md", ContentDigest: strings.Repeat("a", 64), RequestKey: "replacement-request-0001"}
	const count = 12
	results := make(chan store.SourceInputReplacementResult, count)
	failures := make(chan error, count)
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := service.ReplaceSourceInput(ctx, input, actor)
			results <- value
			failures <- err
		}()
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	first := <-results
	for result := range results {
		if result.Source.ID != source.ID || result.CrawlJob.ID != first.CrawlJob.ID {
			t.Fatalf("duplicate replacement: %#v", result)
		}
	}
	if first.Source.Revision != source.Revision+1 || first.Source.Location != input.Location || first.Source.Name != source.Name || first.Source.Quarantined != source.Quarantined || first.Source.Visibility != source.Visibility || first.CrawlJob.State != "queued" {
		t.Fatalf("replacement changed protected state: %#v", first)
	}
	recovered, err := service.SourceInputReplacement(ctx, source.ProductID, source.ID, input.RequestKey, actor)
	if err != nil || recovered.CrawlJob.ID != first.CrawlJob.ID {
		t.Fatalf("recover: %#v %v", recovered, err)
	}
	jobs, err := backend.CrawlJobs(ctx, source.ProductID, source.ID)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("imports: %#v %v", jobs, err)
	}
	events, err := backend.AuditEvents(ctx, source.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	audits := 0
	for _, event := range events {
		if event.Action == "source.input.replaced" && event.TargetID == source.ID {
			audits++
		}
	}
	if audits != 1 {
		t.Fatalf("replacement audits=%d", audits)
	}
	for _, field := range []string{"bytes", "filename", "revision"} {
		changed := input
		switch field {
		case "bytes":
			changed.ContentDigest = strings.Repeat("b", 64)
		case "filename":
			changed.Filename = "other.md"
		case "revision":
			changed.Revision++
		}
		if _, err := service.ReplaceSourceInput(ctx, changed, actor); !errors.Is(err, store.ErrSourceInputChanged) {
			t.Fatalf("changed %s reused key: %v", field, err)
		}
	}
	changed := input
	changed.RequestKey = "replacement-request-0002"
	changed.Revision = first.Source.Revision
	if _, err := service.ReplaceSourceInput(ctx, changed, actor); !errors.Is(err, store.ErrSourceImportActive) {
		t.Fatalf("changed active input: %v", err)
	}
	for _, actorID := range []string{"other-reviewer", ""} {
		if _, err := service.SourceInputReplacement(ctx, source.ProductID, source.ID, input.RequestKey, Actor{ID: actorID}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign reviewer recovery: %v", err)
		}
	}
	if _, err := service.SourceInputReplacement(ctx, source.ProductID, "foreign", input.RequestKey, actor); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign source recovery: %v", err)
	}
	for _, key := range []string{"", "short", "invalid key with spaces"} {
		changed := input
		changed.RequestKey = key
		if _, err := service.ReplaceSourceInput(ctx, changed, actor); err == nil {
			t.Fatal("invalid request key accepted")
		}
	}
	current, err := backend.Source(ctx, source.ProductID, source.ID)
	if err != nil || current.Revision != first.Source.Revision || current.Location != first.Source.Location || current.Quarantined != source.Quarantined {
		t.Fatalf("failed replacement changed input: %#v %v", current, err)
	}
}
func TestSourceInputReplacement(t *testing.T) { testSourceInputReplacement(t, store.NewMemory()) }
func TestSourceInputReplacementPostgres(t *testing.T) {
	testSourceInputReplacement(t, knowledgePostgresFixture(t))
}
