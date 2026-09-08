package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

var mcpPreviewGrantPattern = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{0,127}$`)

var (
	errInvalidMCPPreviewGrant  = errors.New("grants must use lower-case MCP grant identifiers")
	errTooManyMCPPreviewGrants = errors.New("at most 64 grants may be previewed")
)

var mcpPreviewMethods = map[string]bool{
	"server/discover":          true,
	"tools/list":               true,
	"resources/list":           true,
	"resources/read":           true,
	"resources/templates/list": true,
}

type mcpPreviewResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newMCPPreviewResponseWriter() *mcpPreviewResponseWriter {
	return &mcpPreviewResponseWriter{header: make(http.Header)}
}

func (w *mcpPreviewResponseWriter) Header() http.Header { return w.header }

func (w *mcpPreviewResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *mcpPreviewResponseWriter) Write(value []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(value)
}

func mcpPreviewGrants(values []string) ([]string, error) {
	seen := make(map[string]bool, len(values))
	grants := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		if !mcpPreviewGrantPattern.MatchString(value) {
			return nil, errInvalidMCPPreviewGrant
		}
		seen[value] = true
		grants = append(grants, value)
	}
	if len(grants) > 64 {
		return nil, errTooManyMCPPreviewGrants
	}
	sort.Strings(grants)
	return grants, nil
}

// mcpPreview renders the exact JSON-RPC response produced by handleMCP for a
// read-only discovery or resource-read method. Private previews use an explicit simulated grant
// context; they never mint a customer token or execute a tool.
func (s *Server) mcpPreview(w http.ResponseWriter, r *http.Request, productID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	if _, err := s.service.Store().Product(r.Context(), productID); err != nil {
		s.storeError(w, err)
		return
	}

	audience := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("audience")))
	if audience == "" {
		audience = "private"
	}
	if audience != "private" && audience != "public" {
		writeError(w, http.StatusBadRequest, "invalid_request", "audience must be private or public.", nil)
		return
	}
	method := strings.TrimSpace(r.URL.Query().Get("method"))
	if method == "" {
		method = "tools/list"
	}
	if !mcpPreviewMethods[method] {
		writeError(w, http.StatusBadRequest, "invalid_request", "method must be server/discover, tools/list, resources/list, resources/read, or resources/templates/list.", nil)
		return
	}
	resourceURIs, hasURI := r.URL.Query()["uri"]
	if method == "resources/read" {
		if len(resourceURIs) != 1 || resourceURIs[0] == "" || len(resourceURIs[0]) > 2048 || strings.TrimSpace(resourceURIs[0]) != resourceURIs[0] {
			writeError(w, http.StatusBadRequest, "invalid_request", "resources/read requires one exact resource URI, at most 2048 bytes.", nil)
			return
		}
	} else if hasURI {
		writeError(w, http.StatusBadRequest, "invalid_request", "uri is only accepted for resources/read.", nil)
		return
	}
	cursors, hasCursor := r.URL.Query()["cursor"]
	if hasCursor && ((method != "resources/list" && method != "tools/list") || len(cursors) != 1 || len(cursors[0]) == 0 || len(cursors[0]) > mcpCursorMaxBytes) {
		writeError(w, http.StatusBadRequest, "invalid_request", "cursor requires resources/list or tools/list and one opaque value of at most 512 bytes.", nil)
		return
	}
	versions, hasVersion := r.URL.Query()["catalog_version"]
	if hasVersion && (len(versions) != 1 || (versions[0] != "1" && versions[0] != "2")) {
		writeError(w, http.StatusBadRequest, "invalid_request", "catalog_version must be 1 (legacy full catalog) or 2 (compact catalog).", nil)
		return
	}
	grants, err := mcpPreviewGrants(r.URL.Query()["grant"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if audience == "public" && len(grants) > 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "Public MCP previews cannot include grants.", nil)
		return
	}

	params := map[string]any{"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": model.StatelessMCPv2Protocol}}
	if hasVersion {
		version := 2
		if versions[0] == "1" {
			version = 1
		}
		params["_meta"].(map[string]any)[mcpCatalogVersionKey] = version
	}
	if method == "resources/read" {
		params["uri"] = resourceURIs[0]
	}
	if hasCursor {
		params["cursor"] = cursors[0]
	}
	rpc := map[string]any{"jsonrpc": "2.0", "id": "preview", "method": method, "params": params}
	body, err := json.Marshal(rpc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "mcp_preview_failed", "The MCP preview request could not be created.", nil)
		return
	}
	previewRequest, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.baseURL+"/mcp", bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "mcp_preview_failed", "The MCP preview request could not be created.", nil)
		return
	}
	previewRequest.Header.Set("Content-Type", "application/json")
	previewRequest.Header.Set("MCP-Protocol-Version", model.StatelessMCPv2Protocol)
	previewRequest.Header.Set("Mcp-Method", method)

	public := audience == "public"
	if !public {
		grantSet := make(map[string]bool, len(grants))
		for _, grant := range grants {
			grantSet[grant] = true
		}
		principal := identity.Principal{
			ProductID:          productID,
			ClientID:           "root-mcp-preview",
			Issuer:             "urn:dokosoko:mcp-preview",
			Subject:            "root-preview",
			Grants:             grantSet,
			AccessEvaluationID: "mcp-preview",
			AccessEvaluatedAt:  time.Now().UTC(),
			PolicyVersion:      "mcp-preview",
			Scopes:             []string{"mcp:private"},
		}
		previewRequest = previewRequest.WithContext(context.WithValue(previewRequest.Context(), principalKey, principal))
	}

	recorder := newMCPPreviewResponseWriter()
	s.handleMCP(recorder, previewRequest, productID, public)
	if recorder.status != http.StatusOK {
		writeError(w, http.StatusBadGateway, "mcp_preview_failed", "The MCP runtime could not render this preview.", map[string]any{"status": recorder.status})
		return
	}
	response := json.RawMessage(bytes.TrimSpace(recorder.body.Bytes()))
	if !json.Valid(response) {
		writeError(w, http.StatusInternalServerError, "mcp_preview_failed", "The MCP runtime returned an invalid preview response.", nil)
		return
	}

	endpoint := "/mcp"
	authorization := map[string]any{"mode": "simulated", "grants": grants}
	if public {
		endpoint = "/mcp/public"
		authorization = map[string]any{"mode": "anonymous", "grants": []string{}}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"audience":         audience,
		"method":           method,
		"protocol_version": model.StatelessMCPv2Protocol,
		"endpoint":         endpoint,
		"generated_at":     time.Now().UTC(),
		"authorization":    authorization,
		"request":          json.RawMessage(body),
		"response":         response,
	})
}
