package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Server) productSettings(w http.ResponseWriter, r *http.Request, productID string) {
	var input struct {
		Description              string `json:"description"`
		DefaultVersionPolicy     string `json:"default_version_policy"`
		RequirePromotionApproval bool   `json:"require_promotion_approval"`
		Revision                 int64  `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.UpdateProductSettingsWithApproval(r.Context(), productID, input.Description, input.DefaultVersionPolicy, input.RequirePromotionApproval, input.Revision, actor(r))
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) rewriteProductDescription(w http.ResponseWriter, r *http.Request, productID string) {
	var input struct {
		Draft string `json:"draft"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.RewriteProductDescription(r.Context(), productID, input.Draft, actor(r))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "description_rewrite_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"description": value})
}

func (s *Server) productVersions(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().ProductVersions(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			Version           string `json:"version"`
			ProfileID         string `json:"profile_id"`
			IsLatest          bool   `json:"is_latest"`
			IsLTS             bool   `json:"is_lts"`
			ReleaseStage      string `json:"release_stage"`
			RolloutPercentage int    `json:"rollout_percentage"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateProductVersion(r.Context(), productID, platform.ProductVersionInput{Version: input.Version, ProfileID: input.ProfileID, IsLatest: input.IsLatest, IsLTS: input.IsLTS, ReleaseStage: input.ReleaseStage, RolloutPercentage: input.RolloutPercentage}, actor(r))
		if err != nil {
			s.productCatalogError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) productVersionLifecycle(w http.ResponseWriter, r *http.Request, productID, versionID string) {
	var input struct {
		IsLatest           bool       `json:"is_latest"`
		IsLTS              bool       `json:"is_lts"`
		Deprecated         bool       `json:"deprecated"`
		DeprecationMessage string     `json:"deprecation_message"`
		ReplacementVersion string     `json:"replacement_version"`
		SunsetAt           *time.Time `json:"sunset_at"`
		RolloutPercentage  int        `json:"rollout_percentage"`
		AcknowledgeImpact  bool       `json:"acknowledge_impact"`
		Revision           int64      `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.UpdateProductVersionLifecycle(r.Context(), productID, versionID, platform.ProductVersionLifecycleInput{IsLatest: input.IsLatest, IsLTS: input.IsLTS, Deprecated: input.Deprecated, DeprecationMessage: input.DeprecationMessage, ReplacementVersion: input.ReplacementVersion, SunsetAt: input.SunsetAt, RolloutPercentage: input.RolloutPercentage, AcknowledgeImpact: input.AcknowledgeImpact, Revision: input.Revision}, actor(r))
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) productVersionPins(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().ProductVersionPins(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			Scope             string `json:"scope"`
			ScopeID           string `json:"scope_id"`
			CustomerAccountID string `json:"customer_account_id"`
			EnvironmentID     string `json:"environment_id"`
			InstallationID    string `json:"installation_id"`
			ProductVersionID  string `json:"product_version_id"`
			Reason            string `json:"reason"`
			Revision          int64  `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Scope == "" {
			input.Scope, input.ScopeID = "customer", input.CustomerAccountID
		}
		value, err := s.service.SaveScopedProductVersionPin(r.Context(), productID, platform.ProductVersionPinInput{Scope: input.Scope, ScopeID: input.ScopeID, CustomerAccountID: input.CustomerAccountID, EnvironmentID: input.EnvironmentID, InstallationID: input.InstallationID, ProductVersionID: input.ProductVersionID, Reason: input.Reason, Revision: input.Revision}, actor(r))
		if err != nil {
			s.productCatalogError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) productVersionPinHistory(w http.ResponseWriter, r *http.Request, productID string) {
	values, err := s.service.Store().ProductVersionPinHistory(r.Context(), productID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) productInstallations(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().ProductInstallations(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			ID                string `json:"id"`
			CustomerAccountID string `json:"customer_account_id"`
			EnvironmentID     string `json:"environment_id"`
			ExternalID        string `json:"external_id"`
			Name              string `json:"name"`
			State             string `json:"state"`
			Revision          int64  `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveProductInstallation(r.Context(), productID, platform.ProductInstallationInput{ID: input.ID, CustomerAccountID: input.CustomerAccountID, EnvironmentID: input.EnvironmentID, ExternalID: input.ExternalID, Name: input.Name, State: input.State, Revision: input.Revision}, actor(r))
		if err != nil {
			s.productCatalogError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) customerAccounts(w http.ResponseWriter, r *http.Request, productID string) {
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsed < 1 || parsed > 200 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be an integer between 1 and 200.", nil)
			return
		}
		limit = parsed
	}
	startingAfter := r.URL.Query().Get("starting_after")
	if len(startingAfter) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_request", "starting_after is invalid.", nil)
		return
	}
	values, hasMore, err := s.service.Store().CustomerAccounts(r.Context(), productID, startingAfter, limit)
	if startingAfter != "" && errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "invalid_request", "starting_after does not identify a customer account in this product.", nil)
		return
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values, "has_more": hasMore})
}

func (s *Server) customerAccount(w http.ResponseWriter, r *http.Request, productID, accountID string) {
	var input struct {
		State    string `json:"state"`
		Revision int64  `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.UpdateCustomerAccountState(r.Context(), productID, accountID, input.State, input.Revision, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) productVersionImpact(w http.ResponseWriter, r *http.Request, productID, versionID string) {
	value, err := s.service.ProductVersionImpact(r.Context(), productID, versionID)
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) productVersionDiff(w http.ResponseWriter, r *http.Request, productID, versionID string) {
	value, err := s.service.Store().ProductVersion(r.Context(), productID, versionID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value.Diff)
}

func (s *Server) reconcileProductVersion(w http.ResponseWriter, r *http.Request, productID, versionID string) {
	var input struct {
		Revision int64 `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.ReconcileProductVersion(r.Context(), productID, versionID, input.Revision, actor(r))
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) promoteProductVersion(w http.ResponseWriter, r *http.Request, productID, versionID string) {
	var input struct {
		Action   string `json:"action"`
		Note     string `json:"note"`
		Revision int64  `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.PromoteProductVersion(r.Context(), productID, versionID, platform.ProductVersionPromotionInput{Action: input.Action, Note: input.Note, Revision: input.Revision}, actor(r))
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) deleteProductVersionPin(w http.ResponseWriter, r *http.Request, productID, pinID string) {
	if err := s.service.DeleteProductVersionPin(r.Context(), productID, pinID, actor(r)); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) productDefinition(w http.ResponseWriter, r *http.Request, productID string) {
	value, err := s.service.Store().ProductDefinition(r.Context(), productID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) productBuilds(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().ProductBuilds(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			Inputs []model.ProductBuildInput `json:"inputs"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.BuildProductDefinition(r.Context(), productID, input.Inputs, actor(r))
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

func (s *Server) publishProductBuild(w http.ResponseWriter, r *http.Request, productID, buildID string) {
	value, err := s.service.PublishProductDefinition(r.Context(), productID, buildID, actor(r))
	if errors.Is(err, platform.ErrProductDefinitionInvalid) {
		writeError(w, http.StatusUnprocessableEntity, "product_definition_invalid", "Resolve blocking product definition findings before publication.", nil)
		return
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
