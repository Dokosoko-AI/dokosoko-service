package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestAdminWidgetPreviewUsesOriginBoundRuntimeSession(t *testing.T) {
	t.Parallel()
	handler := newWidgetServer(t)

	createdIntegration := request(t, handler, http.MethodPost, "/api/v1/integrations", "doko_admin_demo", `{"family_key":"preview-api","version_key":"v1","display_name":"Preview API","description":"Admin widget preview test API","visibility":"public","acknowledge_public":true,"lifecycle":"active"}`)
	if createdIntegration.Code != http.StatusCreated {
		t.Fatalf("create integration = %d: %s", createdIntegration.Code, createdIntegration.Body.String())
	}
	var integration model.Integration
	if err := json.Unmarshal(createdIntegration.Body.Bytes(), &integration); err != nil {
		t.Fatal(err)
	}
	if published := preflightAndPublishIntegration(t, handler, integration.ID); published.Code != http.StatusCreated {
		t.Fatalf("publish integration = %d: %s", published.Code, published.Body.String())
	}

	createdWidget := request(t, handler, http.MethodPost, "/api/v1/widgets", "doko_admin_demo", `{"name":"Preview assistant","allowed_origins":["https://app.customer.example"],"integration_ids":["`+integration.ID+`"],"appearance":{"theme":"auto","launcher_position":"right","greeting":"How can I help?"}}`)
	if createdWidget.Code != http.StatusCreated {
		t.Fatalf("create widget = %d: %s", createdWidget.Code, createdWidget.Body.String())
	}
	var provisioned struct {
		Widget model.Widget `json:"widget"`
		Secret string       `json:"secret"`
	}
	if err := json.Unmarshal(createdWidget.Body.Bytes(), &provisioned); err != nil {
		t.Fatal(err)
	}
	previewPath := "/api/v1/widgets/" + provisioned.Widget.ID + "/preview-session"

	if unauthenticated := request(t, handler, http.MethodPost, previewPath, "", ""); unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated preview = %d: %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	if draft := request(t, handler, http.MethodPost, previewPath, "doko_admin_demo", ""); draft.Code != http.StatusConflict {
		t.Fatalf("draft preview = %d: %s", draft.Code, draft.Body.String())
	}
	activated := request(t, handler, http.MethodPost, "/api/v1/widgets/"+provisioned.Widget.ID+"/activate", "doko_admin_demo", `{"revision":1}`)
	if activated.Code != http.StatusOK {
		t.Fatalf("activate widget = %d: %s", activated.Code, activated.Body.String())
	}

	preview := request(t, handler, http.MethodPost, previewPath, "doko_admin_demo", "")
	if preview.Code != http.StatusCreated || preview.Header().Get("Cache-Control") != "no-store" || preview.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("preview bootstrap = %d headers=%v: %s", preview.Code, preview.Header(), preview.Body.String())
	}
	var bootstrap struct {
		BootstrapToken string `json:"bootstrapToken"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &bootstrap); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(bootstrap.BootstrapToken, "doko_wbt_") || strings.Contains(preview.Body.String(), provisioned.Secret) || strings.Contains(preview.Body.String(), "sessionToken") {
		t.Fatalf("preview bootstrap leaked or omitted bearer material: %s", preview.Body.String())
	}

	wrongOrigin := request(t, handler, http.MethodPost, "/v1/widget-sessions/exchange", "", `{"bootstrapToken":"`+bootstrap.BootstrapToken+`","origin":"https://evil.example"}`)
	if wrongOrigin.Code != http.StatusForbidden {
		t.Fatalf("wrong-origin preview exchange = %d: %s", wrongOrigin.Code, wrongOrigin.Body.String())
	}
	if consumed := request(t, handler, http.MethodPost, "/v1/widget-sessions/exchange", "", `{"bootstrapToken":"`+bootstrap.BootstrapToken+`","origin":"https://dokosoko.example"}`); consumed.Code != http.StatusUnauthorized {
		t.Fatalf("preview bootstrap was reusable after denial = %d: %s", consumed.Code, consumed.Body.String())
	}

	preview = request(t, handler, http.MethodPost, previewPath, "doko_admin_demo", "")
	if err := json.Unmarshal(preview.Body.Bytes(), &bootstrap); err != nil {
		t.Fatal(err)
	}
	exchanged := request(t, handler, http.MethodPost, "/v1/widget-sessions/exchange", "", `{"bootstrapToken":"`+bootstrap.BootstrapToken+`","origin":"https://dokosoko.example"}`)
	if exchanged.Code != http.StatusCreated || exchanged.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("preview exchange = %d headers=%v: %s", exchanged.Code, exchanged.Header(), exchanged.Body.String())
	}
	var session struct {
		SessionToken string `json:"sessionToken"`
		SessionID    string `json:"sessionId"`
	}
	if err := json.Unmarshal(exchanged.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	current := request(t, handler, http.MethodGet, "/v1/widget-session", session.SessionToken, "")
	if current.Code != http.StatusOK || !strings.Contains(current.Body.String(), `"kind":"admin_preview"`) || !strings.Contains(current.Body.String(), `"userId":"admin-preview:root_demo"`) || !strings.Contains(current.Body.String(), `"origin":"https://dokosoko.example"`) {
		t.Fatalf("current preview session = %d: %s", current.Code, current.Body.String())
	}
	chat := request(t, handler, http.MethodPost, "/v1/widget-chat", session.SessionToken, `{"message":"What can I access?"}`)
	if chat.Code != http.StatusOK || !strings.Contains(chat.Body.String(), `"type":"trace"`) || !strings.Contains(chat.Body.String(), `"text":"Widget API."`) || !strings.Contains(chat.Body.String(), "[DONE]") {
		t.Fatalf("preview chat did not use widget runtime = %d: %s", chat.Code, chat.Body.String())
	}
	sessions := request(t, handler, http.MethodGet, "/api/v1/widgets/"+provisioned.Widget.ID+"/sessions", "doko_admin_demo", "")
	if sessions.Code != http.StatusOK || !strings.Contains(sessions.Body.String(), session.SessionID) || !strings.Contains(sessions.Body.String(), `"kind":"admin_preview"`) || strings.Contains(sessions.Body.String(), session.SessionToken) || strings.Contains(sessions.Body.String(), "digest") {
		t.Fatalf("preview session metadata = %d: %s", sessions.Code, sessions.Body.String())
	}
}
