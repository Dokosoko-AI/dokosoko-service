package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
)

type toolEditorRequest struct {
	Description                *string          `json:"description"`
	InputSchema                *json.RawMessage `json:"input_schema"`
	OutputSchema               *json.RawMessage `json:"output_schema"`
	Endpoint                   *string          `json:"endpoint"`
	RuntimeServiceConnectionID *string          `json:"runtime_service_connection_id"`
	HTTPPath                   *string          `json:"http_path"`
	HTTPMethod                 *string          `json:"http_method"`
	UpstreamAuth               *json.RawMessage `json:"upstream_auth"`
	Credential                 string           `json:"credential"`
	RequestMapping             *json.RawMessage `json:"request_mapping"`
	ResponseMapping            *json.RawMessage `json:"response_mapping"`
	RequestExample             json.RawMessage  `json:"request_example"`
	ResponseExample            json.RawMessage  `json:"response_example"`
	AuthorizationPolicy        *json.RawMessage `json:"authorization_policy"`
	TimeoutMS                  *int             `json:"timeout_ms"`
	Revision                   *int64           `json:"revision"`
}

func adminTool(value model.Tool) map[string]any {
	encoded, _ := json.Marshal(value)
	result := make(map[string]any)
	_ = json.Unmarshal(encoded, &result)
	if value.BackendKind == "http" && value.RuntimeServiceConnectionID != "" {
		result["endpoint"] = ""
		result["endpoint_managed_by_runtime_service"] = true
	} else if value.BackendKind == "http" {
		parsed, err := url.Parse(value.BaseURL)
		if err != nil || parsed.Host == "" {
			result["endpoint"] = ""
			result["endpoint_requires_review"] = true
		} else {
			redacted := parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != ""
			parsed.User, parsed.RawQuery, parsed.Fragment, parsed.RawFragment = nil, "", "", ""
			parsed.ForceQuery = false
			result["endpoint"] = parsed.String()
			if redacted {
				result["endpoint_requires_review"] = true
			}
		}
	}
	return result
}

func (s *Server) toolEditorResource(w http.ResponseWriter, r *http.Request, productID, toolID string) {
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().Tool(r.Context(), productID, toolID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, adminTool(value))
	case http.MethodPut:
		var input toolEditorRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Description == nil || input.InputSchema == nil || input.OutputSchema == nil || input.AuthorizationPolicy == nil || input.TimeoutMS == nil || input.Revision == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "description, input_schema, output_schema, authorization_policy, timeout_ms, and revision are required.", nil)
			return
		}
		if *input.TimeoutMS == 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "timeout_ms must be between 100 and 60000 milliseconds.", nil)
			return
		}
		current, err := s.service.Store().Tool(r.Context(), productID, toolID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		endpoint, method := "", ""
		upstreamAuth, requestMapping, responseMapping := current.UpstreamAuth, current.RequestMapping, current.ResponseMapping
		requestExample, responseExample := current.RequestExample, current.ResponseExample
		if current.BackendKind == "http" {
			if input.HTTPMethod == nil || current.RuntimeServiceConnectionID == "" && input.Endpoint == nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "endpoint and http_method are required for HTTP tools.", nil)
				return
			}
			method = *input.HTTPMethod
			if input.Endpoint != nil {
				endpoint = *input.Endpoint
			}
			if input.UpstreamAuth != nil {
				upstreamAuth = *input.UpstreamAuth
			}
			if input.RequestMapping != nil {
				requestMapping = *input.RequestMapping
			}
			if input.ResponseMapping != nil {
				responseMapping = *input.ResponseMapping
			}
		}
		if len(input.RequestExample) > 0 {
			requestExample = input.RequestExample
		}
		if len(input.ResponseExample) > 0 {
			responseExample = input.ResponseExample
		}
		runtimeConnectionID, httpPath := current.RuntimeServiceConnectionID, current.HTTPPath
		if input.RuntimeServiceConnectionID != nil {
			runtimeConnectionID = *input.RuntimeServiceConnectionID
		}
		if input.HTTPPath != nil {
			httpPath = *input.HTTPPath
		}
		value, err := s.service.UpdateTool(r.Context(), productID, toolID, platform.ToolInput{OrganisationID: current.OrganisationID, ProductID: productID, Scope: current.Scope, OwnerIntegrationID: current.OwnerIntegrationID, RuntimeServiceConnectionID: runtimeConnectionID, HTTPPath: httpPath, Namespace: current.Namespace, Name: current.Name, Description: *input.Description, InputSchema: *input.InputSchema, OutputSchema: *input.OutputSchema, Endpoint: endpoint, HTTPMethod: method, UpstreamAuth: upstreamAuth, Credential: input.Credential, RequestMapping: requestMapping, ResponseMapping: responseMapping, RequestExample: requestExample, ResponseExample: responseExample, AuthorizationPolicy: *input.AuthorizationPolicy, TimeoutMS: *input.TimeoutMS}, *input.Revision, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, adminTool(value))
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) cloneTool(w http.ResponseWriter, r *http.Request, productID, toolID string) {
	var input struct {
		Namespace  string `json:"namespace"`
		Name       string `json:"name"`
		Credential string `json:"credential"`
		Revision   *int64 `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if input.Revision == nil || *input.Revision < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required and must be a positive integer.", nil)
		return
	}
	value, err := s.service.CloneTool(r.Context(), productID, toolID, platform.ToolCloneInput{Namespace: input.Namespace, Name: input.Name, Credential: input.Credential, Revision: *input.Revision}, actor(r))
	if err != nil {
		if errors.Is(err, platform.ErrToolCloneRevisionStale) {
			writeError(w, http.StatusConflict, "revision_conflict", "The source tool changed. Refresh and try again.", nil)
			return
		}
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, adminTool(value))
}

func (s *Server) dryRunTool(w http.ResponseWriter, r *http.Request, productID, toolID string) {
	var input struct {
		Arguments map[string]any `json:"arguments"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if input.Arguments == nil {
		input.Arguments = map[string]any{}
	}
	value, err := s.service.DryRunTool(r.Context(), productID, toolID, input.Arguments)
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) retireTool(w http.ResponseWriter, r *http.Request, productID, toolID string) {
	var input struct {
		Revision int64 `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.RetireTool(r.Context(), productID, toolID, input.Revision, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminTool(value))
}
