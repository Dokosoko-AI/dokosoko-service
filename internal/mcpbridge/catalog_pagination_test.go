package mcpbridge_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/mcpbridge"
)

func upstreamCatalogTool(name string) map[string]any {
	return map[string]any{"name": name, "description": "Read status", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}, "outputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}}
}

func TestUpstreamInspectionAndImportFollowEveryPage(t *testing.T) {
	ctx := context.Background()
	paged, calls := true, 0
	manager, memory, _ := managerForTest(t, recordingDoer(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer upstream-token" || request.URL.String() != "https://mcp.vendor.example/v2" || request.Header.Get("Mcp-Method") != "tools/list" {
			t.Fatal("continuation changed destination, credential or method")
		}
		deadline, ok := request.Context().Deadline()
		if !ok || time.Until(deadline) > 20*time.Second {
			t.Fatal("inspection has no total deadline")
		}
		body := decodeRequest(t, request)
		params := body["params"].(map[string]any)
		cursor, hasCursor := params["cursor"]
		calls++
		result := map[string]any{"resultType": "complete", "catalogRevision": 7, "ttlMs": 30000}
		if !paged {
			result["tools"] = []any{upstreamCatalogTool("z.last"), upstreamCatalogTool("a.first")}
		} else if !hasCursor {
			result["tools"], result["nextCursor"] = []any{upstreamCatalogTool("a.first")}, ""
		} else {
			if cursor != "" {
				t.Fatalf("opaque empty cursor was changed: %#v", cursor)
			}
			result["tools"], result["ttlMs"], result["nextCursor"] = []any{upstreamCatalogTool("z.last")}, 10000, nil
		}
		raw, _ := json.Marshal(result)
		return response("application/json", rpcResult(t, body, string(raw))), nil
	}))
	connection, err := manager.CreateConnection(ctx, mcpbridge.ConnectionInput{OrganisationID: "org_acme", ProductID: "prod_acme", Name: "Paged upstream", Namespace: "paged", Endpoint: "https://mcp.vendor.example/v2", AccessToken: "upstream-token"}, mcpbridge.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := manager.Inspect(ctx, "prod_acme", connection.ID)
	if err != nil || calls != 2 || len(catalog.Tools) != 2 || catalog.Tools[1].Name != "z.last" || catalog.TTLMS != 10000 {
		t.Fatalf("incomplete catalog: %v, %d calls, %#v", err, calls, catalog)
	}
	paged = false
	unpaged, err := manager.Inspect(ctx, "prod_acme", connection.ID)
	if err != nil || unpaged.CatalogHash != catalog.CatalogHash {
		t.Fatal("pagination changed the catalog fingerprint")
	}
	paged = true
	imported, err := manager.Import(ctx, "prod_acme", connection.ID, mcpbridge.ImportInput{ToolNames: []string{"z.last"}}, mcpbridge.Actor{ID: "root"})
	if err != nil || len(imported.Created) != 1 || imported.Created[0].UpstreamToolName != "z.last" || imported.Created[0].State != "draft" {
		t.Fatalf("later-page import failed: %v, %#v", err, imported)
	}
	stored, err := memory.Tools(ctx, "prod_acme", false)
	if err != nil || len(stored) != 1 || stored[0].UpstreamSchemaHash != catalog.Tools[1].SchemaHash {
		t.Fatal("later-page import lost its exact upstream schema")
	}
}

func TestIncompleteUpstreamCatalogNeverImportsPartialTools(t *testing.T) {
	for _, failure := range []string{"duplicate", "cursor", "cursor-type", "revision", "missing-array", "page-budget", "byte-budget", "tool-budget", "cancelled", "connection-changed"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			var changeConnection func()
			manager, memory, _ := managerForTest(t, recordingDoer(func(request *http.Request) (*http.Response, error) {
				body := decodeRequest(t, request)
				calls++
				result := map[string]any{"resultType": "complete", "tools": []any{upstreamCatalogTool("target")}, "catalogRevision": 7, "nextCursor": "next"}
				if calls > 1 {
					result["tools"] = []any{upstreamCatalogTool(fmt.Sprintf("other_%d", calls))}
					delete(result, "nextCursor")
					switch failure {
					case "duplicate":
						result["tools"] = []any{upstreamCatalogTool("target")}
					case "cursor":
						result["nextCursor"] = "next"
					case "cursor-type":
						result["nextCursor"] = 12
					case "revision":
						result["catalogRevision"] = 8
					case "missing-array":
						delete(result, "tools")
					case "connection-changed":
						changeConnection()
					}
				}
				if failure == "cancelled" {
					cancel()
				}
				if failure == "page-budget" || failure == "byte-budget" {
					result["tools"], result["nextCursor"] = []any{upstreamCatalogTool(fmt.Sprint(calls))}, fmt.Sprint(calls)
				}
				if failure == "byte-budget" {
					result["padding"] = strings.Repeat("a", 800<<10)
				}
				if failure == "tool-budget" {
					values := make([]any, 4097)
					for i := range values {
						values[i] = upstreamCatalogTool(fmt.Sprint(i))
					}
					result["tools"] = values
				}
				raw, _ := json.Marshal(result)
				return response("application/json", rpcResult(t, body, string(raw))), nil
			}))
			connection, err := manager.CreateConnection(ctx, mcpbridge.ConnectionInput{OrganisationID: "org_acme", ProductID: "prod_acme", Name: "Incomplete upstream", Namespace: "incomplete", Endpoint: "https://mcp.vendor.example/v2", AccessToken: "upstream-token"}, mcpbridge.Actor{ID: "root"})
			if err != nil {
				t.Fatal(err)
			}
			changeConnection = func() {
				if _, err := memory.UpdateMCPConnectionSync(ctx, "prod_acme", connection.ID, "concurrent-sync", time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			result, err := manager.Import(ctx, "prod_acme", connection.ID, mcpbridge.ImportInput{ToolNames: []string{"target"}}, mcpbridge.Actor{ID: "root"})
			if err == nil || len(result.Created) != 0 {
				t.Fatal("partial upstream catalog was imported")
			}
			if failure == "cancelled" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation: %v", err)
				}
			} else if failure == "connection-changed" {
				if !errors.Is(err, mcpbridge.ErrInvalidConnection) {
					t.Fatalf("connection change: %v", err)
				}
			} else if !errors.Is(err, mcpbridge.ErrUpstreamProtocol) {
				t.Fatalf("catalog failure: %v", err)
			}
			stored, lookupErr := memory.Tools(context.Background(), "prod_acme", false)
			if lookupErr != nil || len(stored) != 0 {
				t.Fatal("failed inspection modified local tools")
			}
			want := 2
			switch failure {
			case "cancelled", "tool-budget":
				want = 1
			case "page-budget":
				want = 64
			case "byte-budget":
				want = 6
			}
			if calls != want {
				t.Fatalf("calls=%d, want %d", calls, want)
			}
		})
	}
}
