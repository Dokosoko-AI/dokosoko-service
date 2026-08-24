package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type firstIntegrationRevisionsErrorStore struct {
	store.Store
	calls int
}

func (s *firstIntegrationRevisionsErrorStore) IntegrationRevisions(ctx context.Context, integrationID string) ([]model.IntegrationRevision, error) {
	s.calls++
	if s.calls == 1 {
		return nil, errors.New("integration revision read failed")
	}
	return s.Store.IntegrationRevisions(ctx, integrationID)
}

func TestIntegrationGETHandlesRevisionListError(t *testing.T) {
	t.Parallel()
	failingStore := &firstIntegrationRevisionsErrorStore{Store: store.NewMemory()}
	handler := httpapi.New(platform.New(failingStore), "https://dokosoko.example")
	created := request(t, handler, http.MethodPost, "/api/v1/integrations", "doko_admin_demo", `{"family_key":"failing-revisions","version_key":"v1","display_name":"Failing revisions","description":"Exercise the Integration GET error boundary.","visibility":"private","lifecycle":"active"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create integration = %d: %s", created.Code, created.Body.String())
	}
	var integration model.Integration
	if err := json.Unmarshal(created.Body.Bytes(), &integration); err != nil {
		t.Fatal(err)
	}

	response := request(t, handler, http.MethodGet, "/api/v1/integrations/"+integration.ID, "doko_admin_demo", "")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("integration revision error = %d: %s", response.Code, response.Body.String())
	}
	if failingStore.calls != 1 {
		t.Fatalf("IntegrationRevisions calls = %d, want 1", failingStore.calls)
	}
}
