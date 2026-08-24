package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
)

type toolEditorRequest struct {
	Description         *string          `json:"description"`
	InputSchema         *json.RawMessage `json:"input_schema"`
	OutputSchema        *json.RawMessage `json:"output_schema"`
	Endpoint            *string          `json:"endpoint"`
	HTTPMethod          *string          `json:"http_method"`
	AuthorizationPolicy *json.RawMessage `json:"authorization_policy"`
	TimeoutMS           *int             `json:"timeout_ms"`
	Revision            *int64           `json:"revision"`
}

func adminTool(value model.Tool) map[string]any {
	encoded, _ := json.Marshal(value)
	result := make(map[string]any)
	_ = json.Unmarshal(encoded, &result)
	if value.BackendKind == "http" {
		result["endpoint"] = value.BaseURL
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
		if current.BackendKind == "http" {
			if input.Endpoint == nil || input.HTTPMethod == nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "endpoint and http_method are required for HTTP tools.", nil)
				return
			}
			endpoint, method = *input.Endpoint, *input.HTTPMethod
		}
		value, err := s.service.UpdateTool(r.Context(), productID, toolID, platform.ToolInput{OrganisationID: current.OrganisationID, ProductID: productID, Namespace: current.Namespace, Name: current.Name, Description: *input.Description, InputSchema: *input.InputSchema, OutputSchema: *input.OutputSchema, Endpoint: endpoint, HTTPMethod: method, AuthorizationPolicy: *input.AuthorizationPolicy, TimeoutMS: *input.TimeoutMS}, *input.Revision, actor(r))
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
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.CloneTool(r.Context(), productID, toolID, platform.ToolCloneInput{Namespace: input.Namespace, Name: input.Name}, actor(r))
	if err != nil {
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
