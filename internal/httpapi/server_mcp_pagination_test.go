package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
)

func paginationResources(count int) []map[string]any {
	values := make([]map[string]any, count)
	for index := range values {
		values[index] = map[string]any{"uri": fmt.Sprintf("dokosoko://fixture/%03d", count-index), "title": "Reviewed publication"}
	}
	return values
}

func cursorParams(cursor string) json.RawMessage {
	data, _ := json.Marshal(map[string]any{"cursor": cursor})
	return data
}

func TestMCPResourcePaginationCompletesInStableOrder(t *testing.T) {
	resources := paginationResources(75)
	params, seen, count := json.RawMessage(`{}`), map[string]bool{}, 0
	previous := ""
	for page := 0; page < 3; page++ {
		items, next, err := mcpResourcePage(params, resources, "scope")
		if err != nil || len(items) == 0 || len(items) > 32 {
			t.Fatalf("page %d: %d items, %v", page, len(items), err)
		}
		for _, item := range items {
			uri := item["uri"].(string)
			if seen[uri] || uri <= previous {
				t.Fatalf("duplicate or unstable order: %s", uri)
			}
			seen[uri], previous = true, uri
			count++
		}
		if (page == 2) != (next == "") {
			t.Fatalf("page %d continuation: %q", page, next)
		}
		params = cursorParams(next)
	}
	if count != 75 {
		t.Fatalf("lost resources: %d", count)
	}
	empty, next, err := mcpResourcePage(json.RawMessage(`{}`), []map[string]any{}, "scope")
	if err != nil || next != "" || empty == nil || len(empty) != 0 {
		t.Fatal("empty catalog did not return an empty array")
	}
}

func TestMCPResourcePaginationHonorsEncodedByteBudget(t *testing.T) {
	resources := paginationResources(3)
	for _, value := range resources {
		value["description"] = strings.Repeat("<", 5000)
	} // JSON escaping costs six bytes per rune.
	items, next, err := mcpResourcePage(json.RawMessage(`{}`), resources, "scope")
	encoded, _ := json.Marshal(items)
	if err != nil || len(items) != 2 || next == "" || len(encoded) > mcpResourcePageBytes {
		t.Fatalf("byte budget: %d, %d, %v", len(items), len(encoded), err)
	}
	resources[0]["description"] = strings.Repeat("<", mcpResourcePageBytes)
	if _, _, err = mcpResourcePage(json.RawMessage(`{}`), resources, "scope"); !errors.Is(err, errMCPResourceSize) {
		t.Fatalf("oversize descriptor: %v", err)
	}
}

func TestMCPToolPageKeepsRoutingAndLargeSchemasWithinBudget(t *testing.T) {
	schema := map[string]any{"type": "object", "description": strings.Repeat("<", 64000)}
	tools := append(compactAPIToolDefinitions(), map[string]any{"name": "common.first", "inputSchema": schema, "outputSchema": schema}, map[string]any{"name": "common.second", "inputSchema": schema, "outputSchema": schema})
	page, next, err := mcpToolCatalogPage(json.RawMessage(`{}`), tools, "scope")
	encoded, _ := json.Marshal(page)
	if err != nil || len(page) != 3 || next == "" || len(encoded) > mcpToolPageBytes || page[0]["name"] != "deployment.apis.list" {
		t.Fatalf("bounded schemas/routing: %d entries, %d bytes, %v", len(page), len(encoded), err)
	}
	last, done, err := mcpToolCatalogPage(cursorParams(next), tools, "scope")
	if err != nil || done != "" || len(last) != 1 || last[0]["name"] != "common.second" {
		t.Fatal("later large tool schema was lost")
	}
	tools[2]["description"] = strings.Repeat("x", mcpToolPageBytes)
	if _, _, err = mcpToolCatalogPage(json.RawMessage(`{}`), tools, "scope"); !errors.Is(err, errMCPResourceSize) {
		t.Fatal("oversized tool descriptor was not rejected")
	}
}

func TestMCPResourcePaginationRejectsChangedScopesAndMalformedCursors(t *testing.T) {
	resources := paginationResources(40)
	principal := identity.Principal{Issuer: "issuer", Subject: "one", ClientID: "client", Grants: map[string]bool{"read": true}, Scopes: []string{"mcp:private"}}
	scope := func(p identity.Principal, deployment string, public bool, revision int64) any {
		r := httptest.NewRequest("POST", "/mcp", nil).WithContext(context.WithValue(context.Background(), principalKey, p))
		return mcpListScope(r, deployment, public, revision)
	}
	original := scope(principal, "deployment", false, 2)
	_, cursor, err := mcpResourcePage(json.RawMessage(`{}`), resources, original)
	if err != nil || cursor == "" {
		t.Fatal(err)
	}
	rotated := principal
	rotated.AccessEvaluationID, rotated.AccessEvaluatedAt, rotated.UpstreamAccessToken = "new-evaluation", time.Now(), "must-never-be-in-a-cursor"
	if _, _, err = mcpResourcePage(cursorParams(cursor), resources, scope(rotated, "deployment", false, 2)); err != nil {
		t.Fatalf("fresh evaluation invalidated stable scope: %v", err)
	}
	for _, mutate := range []func(*identity.Principal){
		func(p *identity.Principal) { p.Subject = "two" }, func(p *identity.Principal) { p.ClientID = "other" },
		func(p *identity.Principal) { p.Grants = map[string]bool{} }, func(p *identity.Principal) { p.PolicyVersion = "new" },
	} {
		changed := principal
		mutate(&changed)
		if _, _, err = mcpResourcePage(cursorParams(cursor), resources, scope(changed, "deployment", false, 2)); !errors.Is(err, errMCPCursor) {
			t.Fatal("foreign principal accepted")
		}
	}
	for _, changed := range []any{scope(principal, "other", false, 2), scope(principal, "deployment", true, 2), scope(principal, "deployment", false, 3)} {
		if _, _, err = mcpResourcePage(cursorParams(cursor), resources, changed); !errors.Is(err, errMCPCursor) {
			t.Fatal("foreign catalog accepted")
		}
	}
	data, _ := base64.RawURLEncoding.DecodeString(cursor)
	var parsed mcpListCursor
	_ = json.Unmarshal(data, &parsed)
	badOffsets := []int{-1, 0, len(resources), len(resources) + 1}
	invalid := []json.RawMessage{json.RawMessage(`{"cursor":null}`), json.RawMessage(`{"cursor":12}`), cursorParams(""), cursorParams("not-a-cursor"), cursorParams(strings.Repeat("a", 513)), cursorParams(base64.RawURLEncoding.EncodeToString(append(data, []byte(`{}`)...)))}
	for _, offset := range badOffsets {
		parsed.Offset = offset
		raw, _ := json.Marshal(parsed)
		invalid = append(invalid, cursorParams(base64.RawURLEncoding.EncodeToString(raw)))
	}
	for _, params := range invalid {
		if _, _, err = mcpResourcePage(params, resources, original); !errors.Is(err, errMCPCursor) {
			t.Fatalf("invalid cursor accepted: %s", params)
		}
	}
	resources[0]["title"] = "Changed publication"
	if _, _, err = mcpResourcePage(cursorParams(cursor), resources, original); !errors.Is(err, errMCPCursor) {
		t.Fatal("changed descriptors accepted")
	}
}
