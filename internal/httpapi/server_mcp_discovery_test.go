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
)

// The synthetic index isolates catalog growth from acquisition and AI quality.
// Publication creation and the MCP handler use their normal service paths.
func TestMCPDiscoveryPayloadGrowth(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{ID: "discovery-reviewer"}
	api, err := service.CreateIntegration(ctx, platform.IntegrationInput{
		FamilyKey: "discovery", VersionKey: "v1", DisplayName: "Discovery API",
		Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	attachMCPTestContract(t, service, api.ID)
	if _, err = service.PublishIntegration(ctx, api.ID, actor); err != nil {
		t.Fatal(err)
	}
	publication, err := service.ReadyAPIDeveloperAssetPublication(ctx, api.ID)
	if err != nil {
		t.Fatal(err)
	}
	enablePublicMCPForDeveloperAssetTest(t, ctx, memory, deployment.ID)
	for _, count := range []int{1, 100, 1000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			units := make([]scopedMCPUnit, count)
			for index := range units {
				units[index] = scopedMCPUnit{id: fmt.Sprintf("evidence-%04d", index), title: fmt.Sprintf("Integration guidance %04d", index), content: "Create an authenticated API client and verify its response before proceeding.", selectorHash: mcpAssetHash("selected")}
			}
			overlay := &scopedMCPDeveloperAssetStore{Memory: memory, apiIndexes: map[string]store.SearchIndexGenerationRecord{
				publication.ID: scopedMCPAPIIndex(deployment.ID, api.ID, publication.ID, "discovery-generation", units...),
			}}
			handler := httpapi.NewWithOptions(platform.New(overlay), httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})
			for _, method := range []string{"server/discover", "resources/list", "tools/list"} {
				response := request(t, handler, http.MethodPost, "/mcp/public", "", fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":{}}`, method))
				if response.Code != http.StatusOK {
					t.Fatalf("%s: %d", method, response.Code)
				}
				t.Logf("units=%d method=%s response_bytes=%d", count, method, response.Body.Len())
				if method == "resources/list" {
					var body struct {
						Result struct {
							Resources []struct {
								URI string `json:"uri"`
							} `json:"resources"`
						} `json:"result"`
					}
					if json.Unmarshal(response.Body.Bytes(), &body) != nil || len(body.Result.Resources) != 1 || strings.Contains(response.Body.String(), "/evidence/") || response.Body.Len() > 1200 {
						t.Fatalf("evidence growth expanded initial discovery: %s", response.Body.String())
					}
					mapURI := strings.TrimSuffix(body.Result.Resources[0].URI, "/map-v2")
					unit := units[len(units)-1]
					unitURI := mapURI + "/evidence/" + unit.id
					mapRead := request(t, handler, http.MethodPost, "/mcp/public", "", fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":%q}}`, mapURI))
					var mapBody struct {
						Result struct {
							Contents []struct {
								Metadata struct {
									Evidence []struct {
										URI  string `json:"uri"`
										Hash string `json:"content_hash"`
									} `json:"evidence_resources"`
								} `json:"_meta"`
							} `json:"contents"`
						} `json:"result"`
					}
					if json.Unmarshal(mapRead.Body.Bytes(), &mapBody) != nil || len(mapBody.Result.Contents) != 1 || len(mapBody.Result.Contents[0].Metadata.Evidence) != count {
						t.Fatal("publication map lost its complete evidence index")
					}
					last := mapBody.Result.Contents[0].Metadata.Evidence[count-1]
					if last.URI != unitURI || last.Hash != mcpAssetHash(unit.content) {
						t.Fatal("last evidence index entry changed")
					}
					read := request(t, handler, http.MethodPost, "/mcp/public", "", fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":%q}}`, unitURI))
					if !strings.Contains(read.Body.String(), unit.content) || !strings.Contains(read.Body.String(), mcpAssetHash(unit.content)) {
						t.Fatal("targeted evidence was lost")
					}
				}
			}
		})
	}
}

func TestMCPResourcePagesAndPreviewUseCurrentPublicationScope(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{ID: "pagination-reviewer"}
	for index := 0; index < 35; index++ {
		api, err := service.CreateIntegration(ctx, platform.IntegrationInput{
			FamilyKey: fmt.Sprintf("paged-%02d", index), VersionKey: "v1", DisplayName: fmt.Sprintf("Paged API %02d", index),
			Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: "active",
		}, actor)
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
	for _, method := range []string{"server/discover", "tools/list"} {
		response := request(t, handler, http.MethodPost, "/mcp/public", "", fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":{}}`, method))
		t.Logf("apis=35 method=%s response_bytes=%d", method, response.Body.Len())
	}
	type pageBody struct {
		Result struct {
			Resources []struct {
				URI string `json:"uri"`
			} `json:"resources"`
			Next string `json:"nextCursor"`
		} `json:"result"`
	}
	list := func(path, token, cursor string) pageBody {
		t.Helper()
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "resources/list", "params": params})
		response := request(t, handler, http.MethodPost, path, token, string(body))
		var page pageBody
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.Result.Resources == nil {
			t.Fatalf("page failed: %.1024s", response.Body.String())
		}
		return page
	}
	first := list("/mcp/public", "", "")
	second := list("/mcp/public", "", first.Result.Next)
	if len(first.Result.Resources) != 32 || len(second.Result.Resources) != 3 || first.Result.Next == "" || second.Result.Next != "" {
		t.Fatalf("pages: %d, %d", len(first.Result.Resources), len(second.Result.Resources))
	}
	seen := map[string]bool{}
	for _, value := range append(first.Result.Resources, second.Result.Resources...) {
		if seen[value.URI] || strings.Contains(value.URI, "/evidence/") {
			t.Fatalf("invalid map discovery: %s", value.URI)
		}
		seen[value.URI] = true
	}
	private := list("/mcp", "doko_private_demo", "")
	if first.Result.Next == private.Result.Next {
		t.Fatal("public and private cursors share a scope")
	}
	previewPath := "/api/v1/products/" + deployment.ID + "/mcp-preview?audience=private&method=resources/list"
	previewFirst := request(t, handler, http.MethodGet, previewPath, "doko_admin_demo", "")
	var preview struct {
		Response pageBody `json:"response"`
	}
	if json.Unmarshal(previewFirst.Body.Bytes(), &preview) != nil || preview.Response.Result.Next == "" {
		t.Fatal("preview did not expose continuation")
	}
	previewSecond := request(t, handler, http.MethodGet, previewPath+"&cursor="+url.QueryEscape(preview.Response.Result.Next), "doko_admin_demo", "")
	var complete struct {
		Response pageBody `json:"response"`
		Request  struct {
			Params struct {
				Cursor string `json:"cursor"`
			} `json:"params"`
		} `json:"request"`
	}
	if json.Unmarshal(previewSecond.Body.Bytes(), &complete) != nil || len(complete.Response.Result.Resources) != 3 || complete.Request.Params.Cursor != preview.Response.Result.Next {
		t.Fatal("preview did not continue its own request scope")
	}
	for _, suffix := range []string{"&grant=read", "&audience=public"} {
		// Use one explicit audience when testing a cross-audience continuation.
		path := previewPath + "&cursor=" + url.QueryEscape(preview.Response.Result.Next) + suffix
		if suffix == "&audience=public" {
			path = strings.ReplaceAll(previewPath, "audience=private", "audience=public") + "&cursor=" + url.QueryEscape(preview.Response.Result.Next)
		}
		denied := request(t, handler, http.MethodGet, path, "doko_admin_demo", "")
		if !strings.Contains(denied.Body.String(), `"code":-32602`) {
			t.Fatal("preview accepted a changed audience/grant scope")
		}
	}
	for _, query := range []string{"method=resources/read&cursor=bad", "method=server/discover&cursor=bad", "method=resources/list&cursor=", "method=resources/list&cursor=one&cursor=two"} {
		response := request(t, handler, http.MethodGet, "/api/v1/products/"+deployment.ID+"/mcp-preview?"+query, "doko_admin_demo", "")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid preview cursor accepted: %s", query)
		}
	}
	if _, err = service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "catalog-changed", VersionKey: "v1", DisplayName: "Changed catalog", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor); err != nil {
		t.Fatal(err)
	}
	stale := request(t, handler, http.MethodPost, "/mcp/public", "", fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"resources/list","params":{"cursor":%q}}`, first.Result.Next))
	if !strings.Contains(stale.Body.String(), `"code":-32602`) {
		t.Fatal("stale catalog cursor was accepted")
	}
}
