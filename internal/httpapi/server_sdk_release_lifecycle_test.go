package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestSDKReleaseLifecycleControlPlaneBlocksNewBinding(t *testing.T) {
	t.Parallel()
	_, service, handler := newDeveloperAssetServer()
	api := createDeveloperAssetAPI(t, service, "lifecycle-http", "v1")
	createdPackage := request(t, handler, http.MethodPost, "/api/v1/developer-assets/sdk-packages", "doko_admin_demo", `{
		"ecosystem":"npm","coordinate":"@acme/lifecycle-http","name":"Lifecycle HTTP SDK",
		"visibility":"private","lifecycle":"active"
	}`)
	if createdPackage.Code != http.StatusCreated {
		t.Fatalf("create SDK package = %d: %s", createdPackage.Code, createdPackage.Body.String())
	}
	var sdkPackage model.SDKPackage
	if err := json.Unmarshal(createdPackage.Body.Bytes(), &sdkPackage); err != nil {
		t.Fatal(err)
	}
	createdRelease := request(t, handler, http.MethodPost, "/api/v1/developer-assets/sdk-packages/"+sdkPackage.ID+"/releases", "doko_admin_demo", `{
		"exact_version":"3.1.4","visibility":"private","lifecycle":"active"
	}`)
	if createdRelease.Code != http.StatusCreated {
		t.Fatalf("create SDK release = %d: %s", createdRelease.Code, createdRelease.Body.String())
	}
	var release model.SDKRelease
	if err := json.Unmarshal(createdRelease.Body.Bytes(), &release); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/developer-assets/sdk-packages/" + sdkPackage.ID + "/releases/" + release.ID + "/lifecycle-events"
	initial := request(t, handler, http.MethodGet, path, "doko_admin_demo", "")
	if initial.Code != http.StatusOK || !strings.Contains(initial.Body.String(), `"initial_lifecycle":"active"`) || !strings.Contains(initial.Body.String(), `"effective_lifecycle":"active"`) || !strings.Contains(initial.Body.String(), `"selectable":true`) {
		t.Fatalf("initial SDK lifecycle = %d: %s", initial.Code, initial.Body.String())
	}
	missingReason := request(t, handler, http.MethodPost, path, "doko_admin_demo", `{"lifecycle":"yanked"}`)
	if missingReason.Code != http.StatusBadRequest || !strings.Contains(missingReason.Body.String(), "reason is required") {
		t.Fatalf("missing lifecycle reason = %d: %s", missingReason.Code, missingReason.Body.String())
	}
	yanked := request(t, handler, http.MethodPost, path, "doko_admin_demo", `{
		"lifecycle":"yanked","reason":"Withdrawn by the upstream registry.",
		"observed_source_uri":"https://registry.example/releases/3.1.4"
	}`)
	if yanked.Code != http.StatusCreated || !strings.Contains(yanked.Body.String(), `"effective_lifecycle":"yanked"`) || !strings.Contains(yanked.Body.String(), `"selectable":false`) || !strings.Contains(yanked.Body.String(), `"recorded_by":"root_demo"`) {
		t.Fatalf("yanked SDK lifecycle = %d: %s", yanked.Code, yanked.Body.String())
	}
	resolved := request(t, handler, http.MethodGet, path, "doko_admin_demo", "")
	if resolved.Code != http.StatusOK || !strings.Contains(resolved.Body.String(), "Withdrawn by the upstream registry.") {
		t.Fatalf("resolved SDK lifecycle = %d: %s", resolved.Code, resolved.Body.String())
	}
	wrongPackage := request(t, handler, http.MethodGet, "/api/v1/developer-assets/sdk-packages/not-this-package/releases/"+release.ID+"/lifecycle-events", "doko_admin_demo", "")
	if wrongPackage.Code != http.StatusNotFound {
		t.Fatalf("cross-package lifecycle = %d: %s", wrongPackage.Code, wrongPackage.Body.String())
	}
	bindingBody, err := json.Marshal(map[string]any{
		"sdk_package_id": sdkPackage.ID, "sdk_release_id": release.ID,
		"state": "draft", "selector": map[string]any{}, "visibility": "private",
	})
	if err != nil {
		t.Fatal(err)
	}
	attached := request(t, handler, http.MethodPost, "/api/v1/integrations/"+api.ID+"/resources/sdks", "doko_admin_demo", string(bindingBody))
	if attached.Code != http.StatusConflict || !strings.Contains(attached.Body.String(), `"code":"sdk_release_unavailable"`) || !strings.Contains(attached.Body.String(), "is yanked") {
		t.Fatalf("binding yanked SDK release = %d: %s", attached.Code, attached.Body.String())
	}
}
