package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListAllFollowsOpaqueCursorsAndRetainsPageCorrelation(t *testing.T) {
	for _, kind := range []struct{ method, field, key string }{{"resources/list", "resources", "uri"}, {"tools/list", "tools", "name"}} {
		t.Run(kind.method, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     string         `json:"id"`
					Params map[string]any `json:"params"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Error("invalid request")
					return
				}
				cursor, exists := request.Params["cursor"]
				if (calls == 0 && exists) || (calls == 1 && (!exists || cursor != "")) || (calls == 2 && cursor != "last") {
					t.Errorf("page %d cursor %#v", calls, request.Params)
				}
				calls++
				result := map[string]any{kind.field: []map[string]any{{kind.key: fmt.Sprint(calls)}}, "catalogRevision": 8}
				if calls == 1 {
					result["nextCursor"] = ""
				} else if calls == 2 {
					result["nextCursor"] = "last"
				} else {
					result["nextCursor"] = nil
				}
				w.Header().Set("X-Request-ID", request.ID)
				writeTestRPC(w, request.ID, result, nil)
			}))
			defer server.Close()
			client := mcpClient{endpoint: server.URL, httpClient: server.Client()}
			result := client.listAll(context.Background(), kind.method, kind.field, kind.key)
			if result.Check.Status != Pass || len(result.Items) != 3 || calls != 3 || len(result.Check.Pages) != 3 {
				t.Fatalf("incomplete discovery: %#v", result.Check)
			}
			for index, page := range result.Check.Pages {
				request := result.Requests[fmt.Sprint(index+1)]
				if request.RequestID != page.RequestID || request.ResponseRequestID != page.RequestID || page.ResultBytes == 0 {
					t.Fatal("entry correlation does not point to its actual page")
				}
			}
		})
	}
}

func TestListAllRejectsIncompleteOrInconsistentCatalogs(t *testing.T) {
	for _, failure := range []string{"duplicate", "cursor", "cursor-type", "cursor-size", "revision", "missing-array", "invalid-entry", "stale", "request-id", "pages", "bytes"} {
		t.Run(failure, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID string `json:"id"`
				}
				_ = json.NewDecoder(r.Body).Decode(&request)
				calls++
				uri := fmt.Sprint(calls)
				if failure == "duplicate" {
					uri = "duplicate"
				}
				result := map[string]any{"resources": []map[string]any{{"uri": uri}}, "catalogRevision": 8}
				if calls == 1 {
					result["nextCursor"] = "next"
				}
				if calls > 1 {
					switch failure {
					case "cursor":
						result["nextCursor"] = "next"
					case "cursor-type":
						result["nextCursor"] = 1
					case "cursor-size":
						result["nextCursor"] = strings.Repeat("a", 4097)
					case "revision":
						result["catalogRevision"] = 9
					case "missing-array":
						result["resources"] = nil
					case "invalid-entry":
						result["resources"] = []any{map[string]any{"uri": 2}}
					case "stale":
						writeTestRPC(w, request.ID, nil, &rpcError{Code: -32602, Message: "restart"})
						return
					case "request-id":
						request.ID = "wrong"
					}
				}
				if failure == "pages" {
					result["nextCursor"] = fmt.Sprint(calls)
				}
				if failure == "bytes" {
					result["padding"] = strings.Repeat("a", 2<<20)
				}
				writeTestRPC(w, request.ID, result, nil)
			}))
			defer server.Close()
			client := mcpClient{endpoint: server.URL, httpClient: server.Client()}
			result := client.listAll(context.Background(), "resources/list", "resources", "uri")
			if result.Check.Status != Fail {
				t.Fatalf("incomplete catalog accepted: %#v", result.Check)
			}
			wanted := 2
			if failure == "pages" {
				wanted = 64
			}
			if calls != wanted {
				t.Fatalf("requests %d, want %d", calls, wanted)
			}
		})
	}
}

func TestRestrictedDiscoveryChecksEveryPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		switch request.Method {
		case "server/discover":
			writeTestRPC(w, request.ID, map[string]any{"supportedVersions": []string{ProtocolVersion}}, nil)
		case "resources/list":
			writeTestRPC(w, request.ID, map[string]any{"resources": []any{}}, nil)
		case "tools/list":
			if _, exists := request.Params["cursor"]; !exists {
				writeTestRPC(w, request.ID, map[string]any{"tools": []any{}, "nextCursor": "next"}, nil)
			} else {
				writeTestRPC(w, request.ID, map[string]any{"tools": []map[string]any{{"name": "granted.write"}}}, nil)
			}
		}
	}))
	defer server.Close()
	report, err := Run(context.Background(), Config{Endpoint: server.URL, AllowedLoopbackHTTP: []string{strings.TrimPrefix(server.URL, "http://")}, Token: "primary", RestrictedToken: "restricted", GrantTool: "granted.write"})
	if err != nil || report.Accepted() || !hasCheck(report, "authorization.grant.positive", Pass) || !hasCheck(report, "authorization.grant.negative", Fail) {
		t.Fatalf("restricted later-page disclosure was missed: %v, %#v", err, report.Summary)
	}
}
