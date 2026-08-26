package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Server) sources(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Sources(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			OrganisationID string `json:"organisation_id"`
			Name           string `json:"name"`
			Kind           string `json:"kind"`
			Location       string `json:"location"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if strings.EqualFold(strings.TrimSpace(input.Kind), "upload") {
			writeError(w, http.StatusBadRequest, "source_upload_requires_multipart", "Create uploaded sources with the reviewed multipart upload endpoint.", nil)
			return
		}
		value, err := s.service.CreateSource(r.Context(), input.OrganisationID, productID, input.Name, input.Kind, input.Location, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) queueCrawl(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	value, err := s.service.QueueCrawl(r.Context(), productID, sourceID, actor(r))
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "crawl_already_active", "This source already has a queued or running crawl.", nil)
			return
		}
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, value)
}

func (s *Server) crawlJobs(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	values, err := s.service.Store().CrawlJobs(r.Context(), productID, sourceID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) sourceReview(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	value, err := s.service.SourceReview(r.Context(), productID, sourceID, strings.TrimSpace(r.URL.Query().Get("crawl_job_id")))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) sourcePublications(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	values, err := s.service.Store().SourcePublications(r.Context(), productID, sourceID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func revisionInput(r *http.Request) (int64, error) {
	var input struct {
		Revision int64 `json:"revision"`
	}
	err := decodeJSON(r.Body, &input)
	if err == nil && input.Revision < 1 {
		err = errors.New("revision must be positive")
	}
	return input.Revision, err
}

func (s *Server) publishSource(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	var input struct {
		Revision            int64    `json:"revision"`
		CrawlJobID          string   `json:"crawl_job_id"`
		DocumentIDs         []string `json:"document_ids"`
		AcknowledgeReviewed bool     `json:"acknowledge_reviewed"`
	}
	if err := decodeJSON(r.Body, &input); err != nil || input.Revision < 1 {
		if err == nil {
			err = errors.New("revision must be positive")
		}
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, publication, err := s.service.PublishSource(r.Context(), productID, sourceID, platform.SourcePublicationInput{Revision: input.Revision, CrawlJobID: input.CrawlJobID, DocumentIDs: input.DocumentIDs, AcknowledgeReviewed: input.AcknowledgeReviewed}, actor(r))
	if err != nil {
		s.platformError(w, err, "Quarantined source content cannot be published.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": value, "publication": publication})
}

func (s *Server) tools(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		if err := s.syncNativePlugins(r.Context(), productID); err != nil {
			writeError(w, http.StatusServiceUnavailable, "native_plugin_catalog_unavailable", "Native tool catalog synchronization failed.", nil)
			return
		}
		values, err := s.service.Store().Tools(r.Context(), productID, false)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			OrganisationID             string          `json:"organisation_id,omitempty"`
			Scope                      string          `json:"scope"`
			OwnerIntegrationID         string          `json:"owner_integration_id"`
			RuntimeServiceConnectionID string          `json:"runtime_service_connection_id"`
			HTTPPath                   string          `json:"http_path"`
			Namespace                  string          `json:"namespace"`
			Name                       string          `json:"name"`
			Description                string          `json:"description"`
			InputSchema                json.RawMessage `json:"input_schema"`
			OutputSchema               json.RawMessage `json:"output_schema"`
			Endpoint                   string          `json:"endpoint"`
			HTTPMethod                 string          `json:"http_method"`
			UpstreamAuth               json.RawMessage `json:"upstream_auth"`
			Credential                 string          `json:"credential"`
			RequestMapping             json.RawMessage `json:"request_mapping"`
			ResponseMapping            json.RawMessage `json:"response_mapping"`
			RequestExample             json.RawMessage `json:"request_example"`
			ResponseExample            json.RawMessage `json:"response_example"`
			AuthorizationPolicy        json.RawMessage `json:"authorization_policy"`
			TimeoutMS                  int             `json:"timeout_ms"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateTool(r.Context(), platform.ToolInput{ProductID: productID, Scope: input.Scope, OwnerIntegrationID: input.OwnerIntegrationID, RuntimeServiceConnectionID: input.RuntimeServiceConnectionID, HTTPPath: input.HTTPPath, Namespace: input.Namespace, Name: input.Name, Description: input.Description, InputSchema: input.InputSchema, OutputSchema: input.OutputSchema, Endpoint: input.Endpoint, HTTPMethod: input.HTTPMethod, UpstreamAuth: input.UpstreamAuth, Credential: input.Credential, RequestMapping: input.RequestMapping, ResponseMapping: input.ResponseMapping, RequestExample: input.RequestExample, ResponseExample: input.ResponseExample, AuthorizationPolicy: input.AuthorizationPolicy, TimeoutMS: input.TimeoutMS}, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) publishTool(w http.ResponseWriter, r *http.Request, productID, toolID string) {
	revision, err := revisionInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.PublishTool(r.Context(), productID, toolID, revision, actor(r))
	if err != nil {
		if errors.Is(err, platform.ErrToolDrifted) {
			writeError(w, http.StatusConflict, "upstream_schema_drift", "Re-inspect and review the upstream schema before publishing this tool.", nil)
			return
		}
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

type distributionPatch struct {
	PublicMCPEnabled  bool  `json:"public_mcp_enabled"`
	AcknowledgePublic bool  `json:"acknowledge_public"`
	Revision          int64 `json:"revision"`
}

func (s *Server) distribution(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		product, err := s.service.Store().Product(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		sources, _ := s.service.Store().Sources(r.Context(), productID)
		publicSources := 0
		for _, item := range sources {
			if item.Visibility == model.VisibilityPublic && item.Published {
				publicSources++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"product":              product,
			"public_mcp_endpoint":  s.baseURL + "/mcp/public",
			"private_mcp_endpoint": s.baseURL + "/mcp",
			"public_sources":       publicSources,
			"agent_setup":          s.agentSetupLinks(r.Context(), product),
		})
	case http.MethodPatch:
		var input distributionPatch
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SetPublicMCP(r.Context(), productID, input.PublicMCPEnabled, input.AcknowledgePublic, input.Revision, actor(r))
		if err != nil {
			s.platformError(w, err, "Enabling Public MCP makes explicitly public, published resources available without authentication.")
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

type visibilityPatch struct {
	Visibility        model.Visibility `json:"visibility"`
	AcknowledgePublic bool             `json:"acknowledge_public"`
	Revision          int64            `json:"revision"`
}

func (s *Server) sourceVisibility(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	var input visibilityPatch
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.SetSourceVisibility(r.Context(), productID, sourceID, input.Visibility, input.AcknowledgePublic, input.Revision, actor(r))
	if err != nil {
		s.platformError(w, err, "This source's published content will be accessible without authentication through Public MCP.")
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func actor(r *http.Request) platform.Actor {
	requestID, _ := r.Context().Value(requestIDKey).(string)
	actorID, _ := r.Context().Value(actorIDKey).(string)
	if actorID == "" {
		actorID = "anonymous"
	}
	return platform.Actor{ID: actorID, RequestID: requestID}
}
