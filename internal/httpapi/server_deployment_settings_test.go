package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
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

func TestDeploymentSettingsExposeAndEnforceCentralConfigurationOwnership(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	service := platform.New(memory)
	description := "Configured description"
	if err := service.ConfigureControlPlane(context.Background(), platform.ControlPlaneConfiguration{
		Deployment: platform.ControlPlaneDeploymentConfiguration{Description: &description},
	}); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})

	read := request(t, handler, http.MethodGet, "/api/v1/deployment", "doko_admin_demo", "")
	if read.Code != http.StatusOK || !containsJSONField(read.Body.Bytes(), "managed_fields", "description") {
		t.Fatalf("managed deployment status=%d body=%s", read.Code, read.Body.String())
	}
	var deployment model.Deployment
	if err := json.Unmarshal(read.Body.Bytes(), &deployment); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"name":%q,"slug":%q,"description":"UI override","public_mcp_enabled":%t,"revision":%d}`, deployment.Name, deployment.Slug, deployment.PublicMCPEnabled, deployment.Revision)
	updated := request(t, handler, http.MethodPatch, "/api/v1/deployment", "doko_admin_demo", body)
	if updated.Code != http.StatusConflict || !strings.Contains(updated.Body.String(), `"code":"configuration_managed"`) {
		t.Fatalf("managed update status=%d body=%s", updated.Code, updated.Body.String())
	}
}

func containsJSONField(contents []byte, key, expected string) bool {
	var value map[string]any
	if json.Unmarshal(contents, &value) != nil {
		return false
	}
	if actual, ok := value[key].(string); ok {
		return actual == expected
	}
	items, ok := value[key].([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}
