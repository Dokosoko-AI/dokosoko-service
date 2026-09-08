package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestSourceReviewContentHTTPRequiresExactScopeAndAuthentication(t *testing.T) {
	_, service, handler := newDeveloperAssetServer()
	jobs, err := service.Store().CrawlJobs(t.Context(), "prod_acme", "src_docs")
	if err != nil || len(jobs) == 0 {
		t.Fatalf("fixture: %#v %v", jobs, err)
	}
	path := "/api/v1/products/prod_acme/sources/src_docs/review/documents/doc_api_keys"
	res := request(t, handler, http.MethodGet, path+"?crawl_job_id="+jobs[0].ID, "doko_admin_demo", "")
	var content model.SourceReviewContent
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &content) != nil || content.Document.ID != "doc_api_keys" || !strings.Contains(content.Body, "Acme dashboard") {
		t.Fatalf("content=%d %s", res.Code, res.Body.String())
	}
	for _, token := range []string{"", "doko_customer_demo"} {
		res = request(t, handler, http.MethodGet, path+"?crawl_job_id="+jobs[0].ID, token, "")
		if res.Code != http.StatusUnauthorized && res.Code != http.StatusForbidden {
			t.Fatalf("non-admin read=%d %s", res.Code, res.Body.String())
		}
		if strings.Contains(res.Body.String(), "Acme dashboard") {
			t.Fatal("private text leaked")
		}
	}
	missingGeneration := request(t, handler, http.MethodGet, path, "doko_admin_demo", "")
	if missingGeneration.Code != http.StatusBadRequest {
		t.Fatalf("missing generation=%d", missingGeneration.Code)
	}
	for _, invalidPath := range []string{path + "?crawl_job_id=other", strings.Replace(path, "src_docs", "src_api", 1) + "?crawl_job_id=" + jobs[0].ID, strings.Replace(path, "prod_acme", "other", 1) + "?crawl_job_id=" + jobs[0].ID} {
		res = request(t, handler, http.MethodGet, invalidPath, "doko_admin_demo", "")
		if res.Code != http.StatusNotFound {
			t.Fatalf("wrong membership=%d %s", res.Code, res.Body.String())
		}
	}
	reviewResponse := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/sources/src_docs/review?crawl_job_id="+jobs[0].ID, "doko_admin_demo", "")
	var review model.SourceReview
	if reviewResponse.Code != http.StatusOK || json.Unmarshal(reviewResponse.Body.Bytes(), &review) != nil || len(review.PublishedDocumentIDs) != 1 || review.PublishedDocumentIDs[0] != "doc_api_keys" {
		t.Fatalf("included selections=%d %s", reviewResponse.Code, reviewResponse.Body.String())
	}
}
