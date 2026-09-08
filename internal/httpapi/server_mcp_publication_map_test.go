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

type compactMapRead struct {
	Result struct {
		Contents []struct {
			URI  string `json:"uri"`
			Text string `json:"text"`
			Meta struct {
				Version    int              `json:"map_version"`
				Root       string           `json:"publication_uri"`
				Index      string           `json:"index_uri"`
				Count      int              `json:"evidence_count"`
				Matches    int              `json:"match_count"`
				Next       string           `json:"next_uri"`
				Generation string           `json:"search_index_generation_id"`
				Entries    []map[string]any `json:"evidence_resources"`
			} `json:"_meta"`
		} `json:"contents"`
	} `json:"result"`
	Error *struct {
		Code int `json:"code"`
	} `json:"error"`
}

func TestMCPCompactPublicationMapGrowthAndExactLookup(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{ID: "map-reviewer"}
	api, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "map", VersionKey: "v1", DisplayName: "Payments API", Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: "active"}, actor)
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
	root := "dokosoko://developer-assets/apis/" + api.ID + "/publications/" + publication.ID
	for _, count := range []int{1, 100, 1000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			units := make([]scopedMCPUnit, count)
			for i := range units {
				units[i] = scopedMCPUnit{id: fmt.Sprintf("map-unit-%04d", i), title: fmt.Sprintf("Payment webhook %04d", i), content: fmt.Sprintf("Verify the exact webhook signature %04d.", i), selectorHash: mcpAssetHash("selected")}
			}
			overlay := &scopedMCPDeveloperAssetStore{Memory: memory, apiIndexes: map[string]store.SearchIndexGenerationRecord{publication.ID: scopedMCPAPIIndex(deployment.ID, api.ID, publication.ID, "map-generation", units...)}}
			handler := httpapi.NewWithOptions(platform.New(overlay), httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})
			read := func(uri string, public bool) (compactMapRead, int) {
				t.Helper()
				path, token := "/mcp/public", ""
				if !public {
					path, token = "/mcp", "doko_private_demo"
				}
				raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": map[string]any{"uri": uri}})
				response := request(t, handler, http.MethodPost, path, token, string(raw))
				var value compactMapRead
				if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &value) != nil {
					t.Fatalf("read failed: %.200s", response.Body.String())
				}
				return value, response.Body.Len()
			}
			legacyList := request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":2,"method":"resources/list","params":{"_meta":{"com.dokosoko/catalogVersion":1}}}`)
			if legacyList.Code != http.StatusOK || !strings.Contains(legacyList.Body.String(), root+`"`) || strings.Contains(legacyList.Body.String(), "/map-v2") {
				t.Fatal("legacy resource discovery changed its full-map URI")
			}
			legacy, oldBytes := read(root, true)
			summary, newBytes := read(root+"/map-v2", true)
			if legacy.Error != nil || len(legacy.Result.Contents) != 1 || len(legacy.Result.Contents[0].Meta.Entries) != count {
				t.Fatal("legacy map changed")
			}
			if summary.Error != nil || len(summary.Result.Contents) != 1 {
				t.Fatal("compact map missing")
			}
			meta := summary.Result.Contents[0].Meta
			if meta.Version != 2 || meta.Count != count || meta.Root != root || meta.Index != root+"/map-v2/index" || meta.Entries != nil || newBytes > 2400 {
				t.Fatalf("unbounded compact map: %+v", meta)
			}
			if strings.Contains(summary.Result.Contents[0].Text, units[count-1].content) {
				t.Fatal("summary included evidence bodies")
			}
			t.Logf("units=%d legacy_map_bytes=%d compact_map_bytes=%d", count, oldBytes, newBytes)
			seen := map[string]bool{}
			next := meta.Index
			pages := 0
			firstNext := ""
			for next != "" {
				page, bytes := read(next, true)
				if page.Error != nil || len(page.Result.Contents) != 1 || bytes > 128<<10 {
					t.Fatal("evidence page unavailable or unbounded")
				}
				value := page.Result.Contents[0]
				if value.URI != next || value.Meta.Matches != count || value.Meta.Generation != meta.Generation || len(value.Meta.Entries) > 32 {
					t.Fatal("page scope changed")
				}
				for _, entry := range value.Meta.Entries {
					uri := entry["uri"].(string)
					if seen[uri] || !strings.HasPrefix(uri, root+"/evidence/") {
						t.Fatal("duplicate or foreign evidence")
					}
					seen[uri] = true
				}
				pages++
				if pages == 1 {
					firstNext = value.Meta.Next
				}
				next = value.Meta.Next
				if pages > 32 {
					t.Fatal("pagination did not finish")
				}
			}
			if len(seen) != count {
				t.Fatal("pagination omitted evidence")
			}
			selected := units[count-1]
			query := url.Values{"source_publication_kind": {"documentation_collection"}, "source_publication_id": {"shared-documentation-revision"}, "source_entity_id": {selected.id + "-source"}, "content_hash": {mcpAssetHash(selected.content)}}
			exact, bytes := read(meta.Index+"?"+query.Encode(), true)
			if exact.Error != nil || len(exact.Result.Contents) != 1 || exact.Result.Contents[0].Meta.Matches != 1 || len(exact.Result.Contents[0].Meta.Entries) != 1 || exact.Result.Contents[0].Meta.Next != "" || bytes > 3200 {
				t.Fatal("exact lookup was ambiguous, incomplete or oversized")
			}
			selectedURI := exact.Result.Contents[0].Meta.Entries[0]["uri"].(string)
			evidence, _ := read(selectedURI, true)
			if evidence.Error != nil || !strings.Contains(evidence.Result.Contents[0].Text, selected.content) {
				t.Fatal("last exact evidence lost")
			}
			query.Set("content_hash", mcpAssetHash("wrong version"))
			wrong, _ := read(meta.Index+"?"+query.Encode(), true)
			if wrong.Error != nil || wrong.Result.Contents[0].Meta.Matches != 0 {
				t.Fatal("wrong content hash widened selection")
			}
			words, _ := read(meta.Index+"?query="+url.QueryEscape(fmt.Sprintf("  WEBHOOK   %04d ", count-1)), true)
			if words.Error != nil || words.Result.Contents[0].Meta.Matches != 1 {
				t.Fatal("all-word query failed")
			}
			for _, suffix := range []string{"?unknown=x", "?query=one&query=two", "?query=", "?cursor=", "?cursor=bad", "?query=%FF", "?query=" + strings.Repeat("a", 501), "?source_entity_id="} {
				invalid, _ := read(meta.Index+suffix, true)
				if invalid.Error == nil || invalid.Error.Code != -32602 {
					t.Fatalf("invalid query accepted: %.50s", suffix)
				}
			}
			for _, suffix := range []string{"?query=x", "?"} {
				invalid, _ := read(root+"/map-v2"+suffix, true)
				if invalid.Error == nil || invalid.Error.Code != -32602 {
					t.Fatal("summary accepted index parameters")
				}
			}
			foreign, _ := read(strings.Replace(root, api.ID, "foreign-api", 1)+"/map-v2/index", true)
			if foreign.Error == nil || foreign.Error.Code != -32004 {
				t.Fatal("cross-API index allowed")
			}
			if firstNext != "" {
				denied, _ := read(firstNext, false)
				if denied.Error == nil || denied.Error.Code != -32602 {
					t.Fatal("cross-audience cursor allowed")
				}
				changed, _ := read(firstNext+"&query=webhook", true)
				if changed.Error == nil || changed.Error.Code != -32602 {
					t.Fatal("changed query cursor allowed")
				}
				overlay.apiIndexes[publication.ID] = scopedMCPAPIIndex(deployment.ID, api.ID, publication.ID, "changed-generation", units...)
				stale, _ := read(firstNext, true)
				if stale.Error == nil || stale.Error.Code != -32602 {
					t.Fatal("changed-generation cursor allowed")
				}
			}
			if count > 32 {
				preview := func(uri, grant string) compactMapRead {
					t.Helper()
					path := "/api/v1/products/" + deployment.ID + "/mcp-preview?audience=private&method=resources/read&uri=" + url.QueryEscape(uri)
					if grant != "" {
						path += "&grant=" + url.QueryEscape(grant)
					}
					response := request(t, handler, http.MethodGet, path, "doko_admin_demo", "")
					var result struct {
						Response compactMapRead `json:"response"`
					}
					if json.Unmarshal(response.Body.Bytes(), &result) != nil || response.Code != http.StatusOK {
						t.Fatal("preview failed")
					}
					return result.Response
				}
				first := preview(meta.Index, "catalog.read")
				if first.Error != nil || len(first.Result.Contents) != 1 || first.Result.Contents[0].Meta.Next == "" {
					t.Fatal("preview index continuation missing")
				}
				denied := preview(first.Result.Contents[0].Meta.Next, "")
				if denied.Error == nil || denied.Error.Code != -32602 {
					t.Fatal("changed-grant index continuation allowed")
				}
			}
			record := scopedMCPAPIIndex(deployment.ID, api.ID, publication.ID, "private-generation", units...)
			record.Units[count-1].Visibility = model.VisibilityPrivate
			record.Units[count-1].Title = "PRIVATE-TITLE-DO-NOT-LEAK"
			overlay.apiIndexes[publication.ID] = record
			for _, uri := range []string{root + "/map-v2", meta.Index + "?query=no-match"} {
				denied, _ := read(uri, true)
				if denied.Error == nil || len(denied.Result.Contents) != 0 {
					t.Fatal("private unit leaked through compact projection")
				}
			}
			record = scopedMCPAPIIndex(deployment.ID, api.ID, publication.ID, "oversized-generation", units...)
			record.Units[0].SourceEntityID = strings.Repeat("x", 70<<10)
			overlay.apiIndexes[publication.ID] = record
			oversized, _ := read(meta.Index, true)
			if oversized.Error == nil || oversized.Error.Code != -32010 {
				t.Fatal("oversized evidence descriptor was silently omitted")
			}

		})
	}
}
