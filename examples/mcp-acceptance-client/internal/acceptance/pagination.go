package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
)

const maxDiscoveryPages = 64
const maxDiscoveryBytes = 4 << 20

type listCatalog struct {
	Items    []json.RawMessage
	Requests map[string]callOutcome
	Check    Check
}

func (client mcpClient) listAll(ctx context.Context, method, field, key string) listCatalog {
	result := listCatalog{Requests: map[string]callOutcome{}}
	params, cursors := map[string]any{}, map[string]bool{}
	pages := []DiscoveryPage{}
	totalBytes := 0
	var revision json.RawMessage
	for page := 0; page < maxDiscoveryPages; page++ {
		outcome := client.callWithParams(ctx, method, params)
		check := outcomeCheck(method, outcome, nil)
		pages = append(pages, DiscoveryPage{RequestID: outcome.RequestID, ResponseRequestID: outcome.ResponseRequestID, HTTPStatus: outcome.HTTPStatus, ResultBytes: len(outcome.Response.Result)})
		check.Pages = pages
		result.Check = check
		if check.Status != Pass {
			return result
		}
		totalBytes += len(outcome.Response.Result)
		fail := func(detail string) listCatalog {
			result.Check.Status, result.Check.Detail = Fail, detail
			return result
		}
		if totalBytes > maxDiscoveryBytes {
			return fail("discovery exceeded the 4 MiB total result budget")
		}
		var body map[string]json.RawMessage
		var items []json.RawMessage
		if json.Unmarshal(outcome.Response.Result, &body) != nil || json.Unmarshal(body[field], &items) != nil || items == nil {
			return fail("result." + field + " was not an array")
		}
		if page == 0 {
			revision = body["catalogRevision"]
		} else if string(revision) != string(body["catalogRevision"]) {
			return fail("catalog revision changed between discovery pages; restart the check")
		}
		for _, item := range items {
			var entry map[string]json.RawMessage
			var identifier string
			if json.Unmarshal(item, &entry) != nil || json.Unmarshal(entry[key], &identifier) != nil || identifier == "" {
				return fail("discovery returned an invalid " + key)
			}
			if _, exists := result.Requests[identifier]; exists {
				return fail("discovery repeated a " + key + " across its catalog")
			}
			// Retain correlation evidence, not copies of response bodies.
			result.Requests[identifier] = callOutcome{RequestID: outcome.RequestID, ResponseRequestID: outcome.ResponseRequestID, HTTPStatus: outcome.HTTPStatus}
			result.Items = append(result.Items, item)
		}
		next, exists := body["nextCursor"]
		if !exists || string(next) == "null" {
			result.Check.Detail = fmt.Sprintf("read all %d discovery pages (%d entries, %d result bytes)", page+1, len(result.Items), totalBytes)
			return result
		}
		var cursor string
		if json.Unmarshal(next, &cursor) != nil || len(cursor) > 4096 || cursors[cursor] {
			return fail("discovery returned an invalid or repeated cursor")
		}
		// An empty string is an opaque cursor too; only absence/null ends a list.
		cursors[cursor], params["cursor"] = true, cursor
	}
	result.Check.Status, result.Check.Detail = Fail, "discovery exceeded the 64 page budget"
	return result
}
