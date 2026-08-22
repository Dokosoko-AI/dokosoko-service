package httpapi

import (
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/platform"
)

func (s *Server) aiProviderConnections(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().AIProviderConnections(r.Context(), deployment.ID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			OrganisationID string `json:"organisation_id"`
			Provider       string `json:"provider"`
			Endpoint       string `json:"endpoint"`
			Credential     string `json:"credential"`
			Enabled        bool   `json:"enabled"`
			Revision       int64  `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveAIProviderConnection(r.Context(), platform.AIProviderConnectionInput{OrganisationID: input.OrganisationID, DeploymentID: deployment.ID, Provider: input.Provider, Endpoint: input.Endpoint, Credential: input.Credential, Enabled: input.Enabled, Revision: input.Revision}, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		status := http.StatusOK
		if value.Revision == 1 {
			status = http.StatusCreated
		}
		writeJSON(w, status, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) testAIProviderConnection(w http.ResponseWriter, r *http.Request, connectionID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.TestAIProviderConnection(r.Context(), deployment.ID, connectionID, actor(r))
	if err != nil {
		writeError(w, http.StatusBadGateway, "ai_provider_test_failed", "The provider did not accept the connection test. Check the credential, model access, and provider status.", map[string]any{"connection": value, "error_code": value.LastErrorCode})
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) aiWorkloadProfiles(w http.ResponseWriter, r *http.Request, productID string) {
	values, err := s.service.Store().AIWorkloadProfiles(r.Context(), productID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) aiWorkloadProfile(w http.ResponseWriter, r *http.Request, productID, workload string) {
	var input struct {
		OrganisationID       string `json:"organisation_id"`
		ProviderConnectionID string `json:"provider_connection_id"`
		Model                string `json:"model"`
		MaxInputTokens       int    `json:"max_input_tokens"`
		MaxOutputTokens      int    `json:"max_output_tokens"`
		DailyTokenBudget     int64  `json:"daily_token_budget"`
		Enabled              bool   `json:"enabled"`
		Revision             int64  `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.SaveAIWorkloadProfile(r.Context(), platform.AIWorkloadProfileInput{OrganisationID: input.OrganisationID, ProductID: productID, Workload: workload, ProviderConnectionID: input.ProviderConnectionID, Model: input.Model, MaxInputTokens: input.MaxInputTokens, MaxOutputTokens: input.MaxOutputTokens, DailyTokenBudget: input.DailyTokenBudget, Enabled: input.Enabled, Revision: input.Revision}, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
