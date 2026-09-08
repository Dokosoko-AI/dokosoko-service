package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/dokosoko/dokosoko-service/internal/store"
)

const mcpPublicationMapSuffix = "/map-v2"
const mcpPublicationMapBytes = 128 << 10

var errMCPMapQuery = errors.New("Invalid publication index query; use query, exact source filters and the returned cursor")

// Versioned URIs leave existing publication and evidence reads intact. These
// response bounds do not bound ready-generation validation's backend work.
func compactDeveloperAssetMap(root mcpDeveloperAssetResource) mcpDeveloperAssetResource {
	metadata := make(map[string]any, len(root.Meta)+4)
	for key, value := range root.Meta {
		metadata[key] = value
	}
	entries, _ := root.ReadMeta["evidence_resources"].([]map[string]any)
	uri := root.URI + mcpPublicationMapSuffix
	metadata["map_version"], metadata["publication_uri"] = 2, root.URI
	metadata["evidence_count"], metadata["index_uri"] = len(entries), uri+"/index"
	text := fmt.Sprintf("# %s\n\n- Exact publication: `%s`\n- Exact index generation: `%s`\n- Selected evidence units: %d\n\nRead [Publication evidence](%s/index) to browse one evidence page. Filter that index by query words or exact source_publication_kind, source_publication_id, source_entity_id and content_hash. Follow next_uri for more matches. Read only the selected evidence URIs; every link stays inside this exact publication.\n", catalogExcerpt(root.Title, 200), root.URI, metadata["search_index_generation_id"], len(entries), uri)
	return mcpDeveloperAssetResource{URI: uri, Name: root.Name, Title: catalogExcerpt(root.Title, 200), Description: "Exact publication summary with paged, targeted evidence lookup.", MIMEType: "text/markdown", Text: text, Meta: metadata}
}

func boundedMCPPublicationMap(resource mcpDeveloperAssetResource) (mcpDeveloperAssetResource, error) {
	encoded, err := json.Marshal(map[string]any{"uri": resource.URI, "mimeType": resource.MIMEType, "text": resource.Text, "_meta": resource.Meta})
	if err != nil || len(encoded) > mcpPublicationMapBytes {
		return mcpDeveloperAssetResource{}, errMCPResourceSize
	}
	return resource, nil
}

func (s *Server) exactCompactDeveloperAssetMap(ctx context.Context, deploymentID, uri string, public bool) (mcpDeveloperAssetResource, error) {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "dokosoko" || parsed.Host != "developer-assets" || parsed.User != nil || parsed.Fragment != "" || parsed.RawPath != "" || len(parsed.RawQuery) > 4096 {
		return mcpDeveloperAssetResource{}, errMCPMapQuery
	}
	path := "dokosoko://developer-assets" + parsed.Path
	index := strings.HasSuffix(path, mcpPublicationMapSuffix+"/index")
	rootURI := strings.TrimSuffix(path, mcpPublicationMapSuffix)
	if index {
		rootURI = strings.TrimSuffix(path, mcpPublicationMapSuffix+"/index")
	}
	parts, ok := parseDeveloperAssetResourceURI(rootURI)
	if !ok || !((len(parts) == 2 && parts[0] == "global-documentation") || (len(parts) == 4 && parts[0] == "apis" && parts[2] == "publications")) {
		return mcpDeveloperAssetResource{}, store.ErrNotFound
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || (!index && (len(query) != 0 || parsed.ForceQuery)) {
		return mcpDeveloperAssetResource{}, errMCPMapQuery
	}
	for key, values := range query {
		limit := 500
		switch key {
		case "cursor":
			limit = mcpCursorMaxBytes
		case "query", "source_publication_kind", "source_publication_id", "source_entity_id", "content_hash":
		default:
			return mcpDeveloperAssetResource{}, errMCPMapQuery
		}
		if len(values) != 1 || values[0] == "" || !utf8.ValidString(values[0]) || utf8.RuneCountInString(values[0]) > limit {
			return mcpDeveloperAssetResource{}, errMCPMapQuery
		}
	}
	// Resolve historical reads through the original deployment, audience,
	// selector and ready-generation checks before projecting any metadata.
	root, err := s.exactPublishedDeveloperAssetResource(ctx, deploymentID, rootURI, public)
	if err != nil {
		return mcpDeveloperAssetResource{}, err
	}
	compact := compactDeveloperAssetMap(root)
	compact.URI = uri // Preserve the literal request, including query order.
	if !index {
		return boundedMCPPublicationMap(compact)
	}
	words := strings.Fields(strings.ToLower(query.Get("query")))
	query.Del("query")
	if len(words) != 0 {
		query.Set("query", strings.Join(words, " "))
	}
	entries := make([]map[string]any, 0)
	all, _ := root.ReadMeta["evidence_resources"].([]map[string]any)
	for _, entry := range all {
		matches := true
		for _, key := range []string{"source_publication_kind", "source_publication_id", "source_entity_id", "content_hash"} {
			if value := query.Get(key); value != "" && entry[key] != value {
				matches = false
			}
		}
		searchable := strings.ToLower(fmt.Sprint(entry["title"], " ", entry["kind"], " ", entry["source_entity_id"]))
		for _, word := range words {
			matches = matches && strings.Contains(searchable, word)
		}
		if matches {
			copy := make(map[string]any, len(entry))
			for key, value := range entry {
				copy[key] = value
			}
			copy["title"] = catalogExcerpt(fmt.Sprint(entry["title"]), 200)
			entries = append(entries, copy)
		}
	}
	params := map[string]any{}
	if query.Has("cursor") {
		params["cursor"] = query.Get("cursor")
		query.Del("cursor")
	}
	raw, _ := json.Marshal(params)
	scope := map[string]any{"request": mcpCatalogScope(ctx, deploymentID, public, 0, "resources/read"), "publication": root.Meta, "uri": rootURI, "query": query.Encode()}
	page, next, err := mcpResourcePage(raw, entries, scope)
	if err != nil {
		return mcpDeveloperAssetResource{}, err
	}
	compact.Meta["match_count"], compact.Meta["evidence_resources"] = len(entries), page
	var text strings.Builder
	fmt.Fprintf(&text, "# Publication evidence\n\n- Exact publication: `%s`\n- Exact index generation: `%s`\n- Matching evidence units: %d\n- Entries in this page: %d\n\n", rootURI, compact.Meta["search_index_generation_id"], len(entries), len(page))
	for _, entry := range page {
		fmt.Fprintf(&text, "- %s — `%s` — `%s`\n", entry["title"], entry["kind"], entry["uri"])
	}
	if next != "" {
		query.Set("cursor", next)
		nextURI := rootURI + mcpPublicationMapSuffix + "/index?" + query.Encode()
		compact.Meta["next_uri"] = nextURI
		fmt.Fprintf(&text, "\n[Next evidence page](%s)\n", nextURI)
	}
	compact.Text = text.String()
	return boundedMCPPublicationMap(compact)
}
