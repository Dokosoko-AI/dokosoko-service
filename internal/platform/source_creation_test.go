package platform

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func testSourceCreationRetries(t *testing.T, backend store.Store) {
	t.Helper()
	product, err := backend.Deployment(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	service := New(backend)
	actor := Actor{ID: "source-creation-reviewer"}
	input := SourceCreationInput{OrganisationID: product.OrganisationID, ProductID: product.ID, Name: "Reviewed source", Kind: "website", Location: "https://docs.example.test/guides", RequestKey: "source-creation-retry-0001"}
	const count = 12
	values := make(chan model.Source, count)
	failures := make(chan error, count)
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := service.CreateSourceWithRequest(t.Context(), input, actor)
			values <- value
			failures <- err
		}()
	}
	group.Wait()
	close(values)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first model.Source
	for value := range values {
		if first.ID == "" {
			first = value
		}
		if value.ID != first.ID || value.Visibility != model.VisibilityPrivate || value.Published || value.Revision != 1 {
			t.Fatalf("duplicate or incorrect source: %#v", value)
		}
	}
	events, err := backend.AuditEvents(t.Context(), product.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	audits := 0
	for _, event := range events {
		if event.Action == "source.created" && event.TargetID == first.ID {
			audits++
		}
	}
	if audits != 1 {
		t.Fatalf("creation audits=%d", audits)
	}
	// Recovery returns the current source without resetting its audience/state.
	first.Visibility = model.VisibilityPublic
	updated, err := backend.UpdateSource(t.Context(), first, first.Revision)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := service.CreateSourceWithRequest(t.Context(), input, actor)
	if err != nil || retried.ID != first.ID || retried.Revision != updated.Revision || retried.Visibility != model.VisibilityPublic {
		t.Fatalf("retry reset source: %#v err=%v", retried, err)
	}
	for _, changed := range []SourceCreationInput{
		{OrganisationID: input.OrganisationID, ProductID: input.ProductID, Name: "Changed name", Kind: input.Kind, Location: input.Location, RequestKey: input.RequestKey},
		{OrganisationID: input.OrganisationID, ProductID: input.ProductID, Name: input.Name, Kind: input.Kind, Location: "https://docs.example.test/other", RequestKey: input.RequestKey},
	} {
		_, err := service.CreateSourceWithRequest(t.Context(), changed, actor)
		if !errors.Is(err, store.ErrSourceCreationConflict) {
			t.Fatalf("different input reused key: %v", err)
		}
	}
	input.RequestKey = "source-creation-new-0002"
	separate, err := service.CreateSourceWithRequest(t.Context(), input, actor)
	if err != nil || separate.ID == first.ID {
		t.Fatalf("explicit new request reused old source: %#v err=%v", separate, err)
	}
	input.OrganisationID = "foreign-organisation"
	_, err = service.CreateSourceWithRequest(t.Context(), input, actor)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign organisation accepted: %v", err)
	}
}

func TestSourceCreationRetries(t *testing.T) { testSourceCreationRetries(t, store.NewMemory()) }
func TestSourceCreationRetriesPostgres(t *testing.T) {
	testSourceCreationRetries(t, knowledgePostgresFixture(t))
}

func testSourceCreationAuditAtomic(t *testing.T, backend store.Store) {
	t.Helper()
	product, err := backend.Deployment(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	service := New(backend)
	first, err := service.CreateSourceWithRequest(t.Context(), SourceCreationInput{OrganisationID: product.OrganisationID, ProductID: product.ID, Kind: "website", Location: "https://docs.example.test", RequestKey: "atomic-source-first-request"}, Actor{ID: "atomic-reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	events, err := backend.AuditEvents(t.Context(), product.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	var audit model.AuditEvent
	for _, event := range events {
		if event.TargetID == first.ID && event.Action == "source.created" {
			audit = event
		}
	}
	if audit.ID == "" {
		t.Fatal("missing first creation audit")
	}
	id, err := randomUUID()
	if err != nil {
		t.Fatal(err)
	}
	value := model.Source{ID: id, OrganisationID: product.OrganisationID, ProductID: product.ID, Name: "Atomic source", Kind: "website", Location: "https://docs.example.test/atomic"}
	audit.TargetID = id
	input := store.SourceCreation{Source: value, Audit: audit, RequestDigest: strings.Repeat("f", 64), InputDigest: strings.Repeat("e", 64)}
	if _, err := backend.CreateSourceOnce(t.Context(), input); err == nil {
		t.Fatal("creation succeeded when its audit key conflicted")
	}
	if _, err := backend.Source(t.Context(), product.ID, id); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("source survived failed audit: %v", err)
	}
	input.Audit.ID = randomID("audit")
	saved, err := backend.CreateSourceOnce(t.Context(), input)
	if err != nil || saved.ID != id {
		t.Fatalf("failed transaction retained its request key: %#v err=%v", saved, err)
	}
}

func TestSourceCreationAuditAtomic(t *testing.T) { testSourceCreationAuditAtomic(t, store.NewMemory()) }
func TestSourceCreationAuditAtomicPostgres(t *testing.T) {
	testSourceCreationAuditAtomic(t, knowledgePostgresFixture(t))
}
