package httpapi

import (
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/platform"
)

type grantDefinitionRequest struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Risk        string `json:"risk"`
	State       string `json:"state"`
	Revision    int64  `json:"revision"`
}

type grantDefinitionReplacementRequest struct {
	Key         *string `json:"key"`
	DisplayName *string `json:"display_name"`
	Description *string `json:"description"`
	Risk        *string `json:"risk"`
	State       *string `json:"state"`
	Revision    *int64  `json:"revision"`
}

func (s *Server) grantDefinitions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.GrantDefinitions(r.Context())
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input grantDefinitionRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveGrantDefinition(r.Context(), "", platform.GrantDefinitionInput{Key: input.Key, DisplayName: input.DisplayName, Description: input.Description, Risk: input.Risk, State: input.State}, actor(r))
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

func (s *Server) grantDefinition(w http.ResponseWriter, r *http.Request, grantID string) {
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", "PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	var input grantDefinitionReplacementRequest
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if input.Key == nil || input.DisplayName == nil || input.Description == nil || input.Risk == nil || input.State == nil || input.Revision == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "key, display_name, description, risk, state, and revision are required.", nil)
		return
	}
	value, err := s.service.SaveGrantDefinition(r.Context(), grantID, platform.GrantDefinitionInput{Key: *input.Key, DisplayName: *input.DisplayName, Description: *input.Description, Risk: *input.Risk, State: *input.State, Revision: *input.Revision}, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

type authorizationPointRequest struct {
	Key                  string   `json:"key"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	ActionType           string   `json:"action_type"`
	RequiredGrants       []string `json:"required_grants"`
	ConfirmationRequired bool     `json:"confirmation_required"`
	DecisionTTLSeconds   int      `json:"decision_ttl_seconds"`
	State                string   `json:"state"`
	Revision             int64    `json:"revision"`
}

type authorizationPointReplacementRequest struct {
	Key                  *string   `json:"key"`
	Name                 *string   `json:"name"`
	Description          *string   `json:"description"`
	ActionType           *string   `json:"action_type"`
	RequiredGrants       *[]string `json:"required_grants"`
	ConfirmationRequired *bool     `json:"confirmation_required"`
	DecisionTTLSeconds   *int      `json:"decision_ttl_seconds"`
	State                *string   `json:"state"`
	Revision             *int64    `json:"revision"`
}

func (s *Server) authorizationPoints(w http.ResponseWriter, r *http.Request, integrationID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.AuthorizationPoints(r.Context(), integrationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input authorizationPointRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveAuthorizationPoint(r.Context(), integrationID, "", platform.AuthorizationPointInput{Key: input.Key, Name: input.Name, Description: input.Description, ActionType: input.ActionType, RequiredGrants: input.RequiredGrants, ConfirmationRequired: input.ConfirmationRequired, DecisionTTLSeconds: input.DecisionTTLSeconds, State: input.State}, actor(r))
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

func (s *Server) authorizationPoint(w http.ResponseWriter, r *http.Request, integrationID, pointID string) {
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", "PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	var input authorizationPointReplacementRequest
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if input.Key == nil || input.Name == nil || input.Description == nil || input.ActionType == nil || input.RequiredGrants == nil || input.ConfirmationRequired == nil || input.DecisionTTLSeconds == nil || input.State == nil || input.Revision == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "key, name, description, action_type, required_grants, confirmation_required, decision_ttl_seconds, state, and revision are required.", nil)
		return
	}
	value, err := s.service.SaveAuthorizationPoint(r.Context(), integrationID, pointID, platform.AuthorizationPointInput{Key: *input.Key, Name: *input.Name, Description: *input.Description, ActionType: *input.ActionType, RequiredGrants: *input.RequiredGrants, ConfirmationRequired: *input.ConfirmationRequired, DecisionTTLSeconds: *input.DecisionTTLSeconds, State: *input.State, Revision: *input.Revision}, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) simulateAuthorizationPoint(w http.ResponseWriter, r *http.Request, integrationID, pointID string) {
	var input struct {
		GrantedGrants []string `json:"granted_grants"`
		Confirmed     bool     `json:"confirmed"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.SimulateAuthorizationPoint(r.Context(), integrationID, pointID, input.GrantedGrants, input.Confirmed)
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) integrationTools(w http.ResponseWriter, r *http.Request, integrationID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.IntegrationToolBindings(r.Context(), integrationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPut:
		var input struct {
			Tools *[]platform.ToolRevisionSelection `json:"tools"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Tools == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "tools is required; use an explicit empty array to clear all bindings.", nil)
			return
		}
		values, err := s.service.SetIntegrationToolBindings(r.Context(), integrationID, *input.Tools, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}
