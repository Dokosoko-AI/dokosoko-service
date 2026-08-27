package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestDeploymentSubmissionURLsRoundTripThroughSettingsAPI(t *testing.T) {
	t.Parallel()
	handler := newServer()
	read := request(t, handler, http.MethodGet, "/api/v1/deployment", "doko_admin_demo", "")
	if read.Code != http.StatusOK {
		t.Fatalf("read deployment status=%d body=%s", read.Code, read.Body.String())
	}
	var deployment model.Deployment
	if err := json.Unmarshal(read.Body.Bytes(), &deployment); err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"name":%q,"slug":%q,"description":%q,"feedback_submission_url":"https://support.example.test/feedback","error_submission_url":"https://support.example.test/errors","public_mcp_enabled":%t,"revision":%d}`, deployment.Name, deployment.Slug, deployment.Description, deployment.PublicMCPEnabled, deployment.Revision)
	updated := request(t, handler, http.MethodPatch, "/api/v1/deployment", "doko_admin_demo", body)
	if updated.Code != http.StatusOK {
		t.Fatalf("update deployment status=%d body=%s", updated.Code, updated.Body.String())
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &deployment); err != nil {
		t.Fatal(err)
	}
	if deployment.FeedbackSubmissionURL != "https://support.example.test/feedback" || deployment.ErrorSubmissionURL != "https://support.example.test/errors" {
		t.Fatalf("deployment submission URLs = %#v", deployment)
	}

	body = fmt.Sprintf(`{"name":%q,"slug":%q,"description":"updated by an older client","public_mcp_enabled":%t,"revision":%d}`, deployment.Name, deployment.Slug, deployment.PublicMCPEnabled, deployment.Revision)
	updated = request(t, handler, http.MethodPatch, "/api/v1/deployment", "doko_admin_demo", body)
	if updated.Code != http.StatusOK {
		t.Fatalf("update without submission URLs status=%d body=%s", updated.Code, updated.Body.String())
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &deployment); err != nil {
		t.Fatal(err)
	}
	if deployment.FeedbackSubmissionURL != "https://support.example.test/feedback" || deployment.ErrorSubmissionURL != "https://support.example.test/errors" {
		t.Fatalf("omitted deployment submission URLs were not preserved: %#v", deployment)
	}
}
