package httpapi_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestDocumentationWorkflowListsRequireAdminAndRemainReadOnly(t *testing.T) {
	_, service, handler := newDeveloperAssetServer()
	ctx := t.Context()
	source, err := service.Store().CreateSource(ctx, model.Source{ID: "pending-content", OrganisationID: "org_acme", ProductID: "prod_acme", Name: "Private pending content", Kind: "upload", Location: "hidden-upload-location.md", Visibility: model.VisibilityPrivate})
	if err != nil {
		t.Fatal(err)
	}
	before, err := service.Store().Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/developer-assets/documentation/attention?source_id=" + source.ID, "/api/v1/developer-assets/documentation/library?view=reviewed&source_id=" + source.ID} {
		for _, token := range []string{"", "doko_customer_demo"} {
			response := request(t, handler, http.MethodGet, path, token, "")
			if response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden {
				t.Fatalf("non-admin: %d %s", response.Code, response.Body)
			}
			if strings.Contains(response.Body.String(), source.Name) {
				t.Fatal("private name leaked")
			}
		}
		response := request(t, handler, http.MethodGet, path, "doko_admin_demo", "")
		if response.Code != http.StatusOK {
			t.Fatalf("admin list: %d %s", response.Code, response.Body)
		}

		for _, suffix := range []string{"&limit=0", "&limit=101", "&offset=-1", "&limit=oops"} {
			response := request(t, handler, http.MethodGet, path+suffix, "doko_admin_demo", "")
			if response.Code != http.StatusBadRequest {
				t.Fatalf("bad query: %d %s", response.Code, response.Body)
			}
		}
		response = request(t, handler, http.MethodGet, strings.Replace(path, source.ID, "foreign-source", 1), "doko_admin_demo", "")
		if response.Code != http.StatusNotFound {
			t.Fatalf("foreign source: %d %s", response.Code, response.Body)
		}
		response = request(t, handler, http.MethodPost, path, "doko_admin_demo", `{}`)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("write to list: %d %s", response.Code, response.Body)
		}
	}
	response := request(t, handler, http.MethodGet, "/api/v1/developer-assets/documentation/attention?source_id="+source.ID, "doko_admin_demo", "")
	var page store.DocumentationAttentionPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Status != "not_imported" || page.Items[0].CrawlJobID != "" {
		t.Fatalf("wire: %s %v", response.Body, err)
	}
	if strings.Contains(response.Body.String(), source.Location) {
		t.Fatal("list exposed input location")
	}
	current, err := service.Store().Source(ctx, "prod_acme", source.ID)
	if err != nil || !reflect.DeepEqual(source, current) {
		t.Fatalf("read changed source: %#v %v", current, err)
	}
	jobs, err := service.Store().CrawlJobs(ctx, "prod_acme", source.ID)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("read queued import: %#v %v", jobs, err)
	}
	after, err := service.Store().Deployment(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("read changed deployment: %#v %v", after, err)
	}
}
