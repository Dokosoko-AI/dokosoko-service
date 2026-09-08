package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

type compactCatalogRPC struct {
	Result struct {
		CatalogVersion int                    `json:"catalogVersion"`
		Catalog        map[string]any         `json:"catalog"`
		Deployment     *model.ProductManifest `json:"deployment"`
		Product        *model.ProductManifest `json:"product"`
		Next           string                 `json:"nextCursor"`
		Tools          []struct {
			Name string `json:"name"`
		} `json:"tools"`
		Structured struct {
			APIs        []model.IntegrationManifest `json:"apis"`
			API         model.IntegrationManifest   `json:"api"`
			Next        string                      `json:"next_cursor"`
			Publication struct {
				ID            string `json:"id"`
				APIRevisionID string `json:"api_revision_id"`
				Hash          string `json:"snapshot_hash"`
				URI           string `json:"uri"`
			} `json:"publication"`
		} `json:"structuredContent"`
	} `json:"result"`
	Error *struct {
		Code int `json:"code"`
	} `json:"error"`
}

func compactCatalogRequest(t *testing.T, handler http.Handler, public bool, method string, params map[string]any) compactCatalogRPC {
	t.Helper()
	path, token := "/mcp", "doko_private_demo"
	if public {
		path, token = "/mcp/public", ""
	}
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	response := request(t, handler, http.MethodPost, path, token, string(body))
	var result compactCatalogRPC
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
		t.Fatalf("invalid catalog response: %d %.1024s", response.Code, response.Body.String())
	}
	return result
}

func TestMCPCompactCatalogVersionsAndExactAPISelection(t *testing.T) {
	ctx, memory := context.Background(), store.NewMemory()
	service := platform.New(memory)
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{ID: "catalog-reviewer"}
	for index := 0; index < 36; index++ {
		visibility, name := model.VisibilityPublic, fmt.Sprintf("Orders API %02d", index)
		if index == 35 {
			visibility, name = model.VisibilityPrivate, "PRIVATE-CATALOG-TITLE"
		}
		api, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: fmt.Sprintf("orders-%02d", index), VersionKey: "v1", DisplayName: name, Description: fmt.Sprintf("Catalog example %02d", index), Visibility: visibility, AcknowledgePublic: visibility == model.VisibilityPublic, Lifecycle: "active"}, actor)
		if err != nil {
			t.Fatal(err)
		}
		attachMCPTestContract(t, service, api.ID)
		if _, err = service.PublishIntegration(ctx, api.ID, actor); err != nil {
			t.Fatal(err)
		}
	}
	enablePublicMCPForDeveloperAssetTest(t, ctx, memory, deployment.ID)
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})
	for _, public := range []bool{false, true} {
		want := 36
		if public {
			want = 35
		}
		for _, method := range []string{"server/discover", "tools/list"} {
			compact := compactCatalogRequest(t, handler, public, method, map[string]any{})
			if compact.Error != nil || compact.Result.CatalogVersion != 2 || compact.Result.Deployment != nil || compact.Result.Product != nil || compact.Result.Catalog["api_count"] != float64(want) {
				t.Fatalf("invalid compact projection: %#v", compact.Result.Catalog)
			}
			legacy := compactCatalogRequest(t, handler, public, method, map[string]any{"_meta": map[string]any{"com.dokosoko/catalogVersion": 1}})
			if legacy.Error != nil || legacy.Result.CatalogVersion != 1 || legacy.Result.Deployment == nil || legacy.Result.Product == nil || len(legacy.Result.Deployment.Integrations) != want || len(legacy.Result.Product.Integrations) != want {
				t.Fatal("legacy manifest aliases were not preserved")
			}
		}
	}
	for _, invalid := range []any{nil, 0, 3, "2", true, 1.5} {
		result := compactCatalogRequest(t, handler, true, "server/discover", map[string]any{"_meta": map[string]any{"com.dokosoko/catalogVersion": invalid}})
		if result.Error == nil || result.Error.Code != -32602 {
			t.Fatalf("invalid catalog version accepted: %#v", invalid)
		}
	}
	call := func(public bool, name string, arguments map[string]any) compactCatalogRPC {
		return compactCatalogRequest(t, handler, public, "tools/call", map[string]any{"name": name, "arguments": arguments})
	}
	first := call(true, "deployment.apis.list", map[string]any{})
	second := call(true, "deployment.apis.list", map[string]any{"cursor": first.Result.Structured.Next})
	if first.Error != nil || second.Error != nil || len(first.Result.Structured.APIs) != 32 || len(second.Result.Structured.APIs) != 3 || first.Result.Structured.Next == "" || second.Result.Structured.Next != "" {
		t.Fatal("API lookup did not return two complete pages")
	}
	seen := map[string]bool{}
	for _, api := range append(first.Result.Structured.APIs, second.Result.Structured.APIs...) {
		if seen[api.ID] || api.DisplayName == "PRIVATE-CATALOG-TITLE" || api.ManifestHash == "" || api.Revision < 1 || api.VersionKey != "v1" || api.Resources != nil {
			t.Fatalf("invalid compact API: %#v", api)
		}
		seen[api.ID] = true
	}
	selected := second.Result.Structured.APIs[0]
	exact := call(true, "deployment.apis.get", map[string]any{"api_id": selected.ID, "manifest_hash": selected.ManifestHash})
	if exact.Error != nil || exact.Result.Structured.API.ID != selected.ID || exact.Result.Structured.API.ManifestHash != selected.ManifestHash {
		t.Fatal("targeted manifest did not preserve the selected hash")
	}
	publication, err := service.ReadyAPIDeveloperAssetPublication(ctx, selected.ID)
	if err != nil {
		t.Fatal(err)
	}
	route := exact.Result.Structured.Publication
	if route.ID != publication.ID || route.APIRevisionID != publication.APIRevisionID || route.Hash != publication.SnapshotHash || route.URI != "dokosoko://developer-assets/apis/"+selected.ID+"/publications/"+publication.ID+"/map-v2" {
		t.Fatal("API lookup omitted or changed the exact developer-asset route")
	}
	read := request(t, handler, http.MethodPost, "/mcp/public", "", fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":%q}}`, route.URI))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"api_developer_asset_publication_id":"`+route.ID+`"`) || !strings.Contains(read.Body.String(), route.Hash) {
		t.Fatal("selected publication map was not readable with its exact pins")
	}
	wrong := call(true, "deployment.apis.get", map[string]any{"api_id": selected.ID, "manifest_hash": "changed"})
	if wrong.Error == nil || wrong.Error.Code != -32009 {
		t.Fatal("changed API publication was silently substituted")
	}
	private := call(false, "deployment.apis.list", map[string]any{"query": "PRIVATE-CATALOG-TITLE"})
	if private.Error != nil || len(private.Result.Structured.APIs) != 1 {
		t.Fatal("authorized private API lookup failed")
	}
	privateAPI := private.Result.Structured.APIs[0]
	for _, result := range []compactCatalogRPC{
		call(true, "deployment.apis.get", map[string]any{"api_id": privateAPI.ID, "manifest_hash": privateAPI.ManifestHash}),
		call(true, "deployment.apis.get", map[string]any{"api_id": "foreign-deployment-api", "manifest_hash": selected.ManifestHash}),
	} {
		if result.Error == nil || result.Error.Code != -32004 {
			t.Fatal("private or foreign API was disclosed")
		}
	}
	publicPrivateQuery := call(true, "deployment.apis.list", map[string]any{"query": "PRIVATE-CATALOG-TITLE"})
	if publicPrivateQuery.Error != nil || publicPrivateQuery.Result.Structured.APIs == nil || len(publicPrivateQuery.Result.Structured.APIs) != 0 {
		t.Fatal("private query did not return an empty public catalog")
	}
	for _, result := range []compactCatalogRPC{
		call(false, "deployment.apis.list", map[string]any{"cursor": first.Result.Structured.Next}),
		call(true, "deployment.apis.list", map[string]any{"query": "Orders", "cursor": first.Result.Structured.Next}),
	} {
		if result.Error == nil || result.Error.Code != -32602 {
			t.Fatal("foreign query/audience cursor was accepted")
		}
	}
	query := call(true, "deployment.apis.list", map[string]any{"query": "  ORDERS    34 V1 "})
	if query.Error != nil || len(query.Result.Structured.APIs) != 1 || query.Result.Structured.APIs[0].FamilyKey != "orders-34" {
		t.Fatal("API name/version query did not select the expected API")
	}
	for _, arguments := range []map[string]any{{"query": nil}, {"query": 12}, {"cursor": nil}, {"cursor": ""}, {"unknown": true}, {"query": strings.Repeat("a", 501)}} {
		result := call(true, "deployment.apis.list", arguments)
		if result.Error == nil || result.Error.Code != -32602 {
			t.Fatalf("invalid API list arguments accepted: %#v", arguments)
		}
	}
	for _, arguments := range []map[string]any{{}, {"api_id": selected.ID}, {"api_id": selected.ID, "manifest_hash": nil}, {"api_id": selected.ID, "manifest_hash": selected.ManifestHash, "extra": true}} {
		result := call(true, "deployment.apis.get", arguments)
		if result.Error == nil || result.Error.Code != -32602 {
			t.Fatal("invalid exact API arguments accepted")
		}
	}
	if _, err = service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "changed", VersionKey: "v1", DisplayName: "Changed catalog", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor); err != nil {
		t.Fatal(err)
	}
	stale := call(true, "deployment.apis.list", map[string]any{"cursor": first.Result.Structured.Next})
	if stale.Error == nil || stale.Error.Code != -32602 {
		t.Fatal("changed catalog retained an old continuation")
	}
	root, err := memory.Integration(ctx, deployment.ID, selected.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.UpdateIntegration(ctx, root.ID, platform.IntegrationInput{FamilyKey: root.FamilyKey, VersionKey: root.VersionKey, DisplayName: root.DisplayName, Description: "Revised published guidance", Visibility: root.Visibility, AcknowledgePublic: true, Lifecycle: root.Lifecycle, Revision: root.Revision}, actor); err != nil {
		t.Fatal(err)
	}
	// A draft edit must not replace the published selection.
	draft := call(true, "deployment.apis.get", map[string]any{"api_id": selected.ID, "manifest_hash": selected.ManifestHash})
	if draft.Error != nil || draft.Result.Structured.Publication.ID != route.ID {
		t.Fatal("draft metadata replaced a publication")
	}
	if _, err = service.PublishIntegration(ctx, root.ID, actor); err != nil {
		t.Fatal(err)
	}
	changed := call(true, "deployment.apis.get", map[string]any{"api_id": selected.ID, "manifest_hash": selected.ManifestHash})
	if changed.Error == nil || changed.Error.Code != -32009 {
		t.Fatal("a newly published API silently replaced an exact selection")
	}
}

func TestMCPToolPagesRecheckAuthorizationAndRetainLegacyCatalog(t *testing.T) {
	ctx, memory := context.Background(), store.NewMemory()
	service := platform.New(memory)
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 35; index++ {
		tool, err := memory.CreateTool(ctx, model.Tool{ID: fmt.Sprintf("paged-tool-%02d", index), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, Namespace: "catalog", Name: fmt.Sprintf("read_%02d", index), Description: "PRIVATE-TOOL-CATALOG", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), BaseURL: "https://api.example.test/read", HTTPMethod: "GET", BackendKind: "http", AuthorizationPolicy: json.RawMessage(`{"required_grants":["catalog.read"],"confirmation_required":false}`), TimeoutMS: 5000})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = memory.PublishTool(ctx, deployment.ID, tool.ID, tool.Revision, "fixture-reviewer"); err != nil {
			t.Fatal(err)
		}
	}
	enablePublicMCPForDeveloperAssetTest(t, ctx, memory, deployment.ID)
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true, ToolRuntime: toolruntime.NewRuntime(memory, authorizationResolver{}, &authorizationDoer{})})
	path := "/api/v1/products/" + deployment.ID + "/mcp-preview?audience=private&method=tools/list"
	preview := func(query string) compactCatalogRPC {
		t.Helper()
		response := request(t, handler, http.MethodGet, path+query, "doko_admin_demo", "")
		var body struct {
			Response compactCatalogRPC `json:"response"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
			t.Fatalf("preview failed: %.1024s", response.Body.String())
		}
		return body.Response
	}
	first := preview("&grant=catalog.read")
	second := preview("&grant=catalog.read&cursor=" + url.QueryEscape(first.Result.Next))
	if first.Error != nil || second.Error != nil || len(first.Result.Tools) != 32 || len(second.Result.Tools) != 10 || second.Result.Next != "" {
		t.Fatalf("tool pages: %d, %d, %#v, %#v", len(first.Result.Tools), len(second.Result.Tools), first.Error, second.Error)
	}
	if first.Result.Tools[0].Name != "integration.plan" || first.Result.Tools[1].Name != "deployment.apis.list" || first.Result.Tools[2].Name != "deployment.apis.get" {
		t.Fatal("task/API routing tools were displaced from the first page")
	}
	seen, previous := map[string]bool{}, ""
	for _, tool := range append(first.Result.Tools, second.Result.Tools...) {
		if seen[tool.Name] || (strings.HasPrefix(tool.Name, "common.") && tool.Name <= previous) {
			t.Fatal("tool order or uniqueness changed across pages")
		}
		seen[tool.Name] = true
		if strings.HasPrefix(tool.Name, "common.") {
			previous = tool.Name
		}
	}
	denied := preview("&cursor=" + url.QueryEscape(first.Result.Next))
	if denied.Error == nil || denied.Error.Code != -32602 {
		t.Fatal("tool cursor survived grant loss")
	}
	for _, result := range []compactCatalogRPC{preview(""), compactCatalogRequest(t, handler, true, "tools/list", map[string]any{})} {
		if result.Error != nil || len(result.Result.Tools) != 7 {
			t.Fatal("restricted catalog was not limited to built-ins")
		}
		for _, tool := range result.Result.Tools {
			if strings.HasPrefix(tool.Name, "common.read_") {
				t.Fatal("restricted catalog disclosed a private tool")
			}
		}
	}
	legacy := compactCatalogRequest(t, handler, false, "tools/list", map[string]any{"_meta": map[string]any{"com.dokosoko/catalogVersion": 1}})
	if legacy.Error != nil || legacy.Result.CatalogVersion != 1 || legacy.Result.Deployment == nil || legacy.Result.Next != "" {
		t.Fatal("legacy tool catalog changed shape")
	}
	legacyGranted := preview("&grant=catalog.read&catalog_version=1")
	if legacyGranted.Error != nil || legacyGranted.Result.CatalogVersion != 1 || len(legacyGranted.Result.Tools) != 41 || legacyGranted.Result.Next != "" {
		t.Fatal("legacy authorized tool catalog was paginated or lost definitions")
	}
	for _, query := range []string{"&catalog_version=", "&catalog_version=0", "&catalog_version=2&catalog_version=1"} {
		response := request(t, handler, http.MethodGet, path+query, "doko_admin_demo", "")
		if response.Code != http.StatusBadRequest {
			t.Fatal("invalid preview catalog version was accepted")
		}
	}
	bad := compactCatalogRequest(t, handler, false, "tools/list", map[string]any{"cursor": first.Result.Next, "_meta": map[string]any{"com.dokosoko/catalogVersion": 1}})
	if bad.Error == nil || bad.Error.Code != -32602 {
		t.Fatal("version 2 cursor was accepted by legacy tool discovery")
	}
}
