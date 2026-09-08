package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestContractCreationHTTPRecoveryAndAuthorization(t *testing.T) {
	_, _, handler := newDeveloperAssetServer()
	const path = "/api/v1/developer-assets/api-contracts"
	body := `{"name":"Orders","slug":"orders","visibility":"private","lifecycle":"active"}`
	send := func(token, key, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	key := "contract-creation-http-0001"
	first := send("doko_admin_demo", key, body)
	if first.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", first.Code, first.Body.String())
	}
	var original model.APIContract
	if err := json.Unmarshal(first.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	retry := send("doko_admin_demo", key, body)
	var recovered model.APIContract
	if retry.Code != http.StatusCreated || json.Unmarshal(retry.Body.Bytes(), &recovered) != nil || recovered.ID != original.ID || recovered.Revision != 1 {
		t.Fatalf("retry=%d %s", retry.Code, retry.Body.String())
	}
	for _, item := range []struct {
		token, key, body string
		status           int
	}{
		{"", key, body, 401},
		{"doko_admin_demo", "short", body, 400},
		{"doko_admin_demo", key, strings.Replace(body, "Orders", "Changed", 1), 409},
		{"doko_admin_demo", "contract-distinct-request-0002", body, 409},
	} {
		result := send(item.token, item.key, item.body)
		if result.Code != item.status {
			t.Fatalf("status=%d want=%d %s", result.Code, item.status, result.Body.String())
		}
	}
}
