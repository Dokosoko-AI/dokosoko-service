package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) supportSubmissions(w http.ResponseWriter, r *http.Request) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Reporting is unavailable.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
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
	values, hasMore, err := s.reporting.Submissions(r.Context(), deployment.ID, startingAfter, limit)
	if startingAfter != "" && errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "invalid_request", "starting_after does not identify a support submission in this deployment.", nil)
		return
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values, "has_more": hasMore})
}

func (s *Server) supportSubmission(w http.ResponseWriter, r *http.Request, submissionID string) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Reporting is unavailable.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.reporting.Submission(r.Context(), deployment.ID, submissionID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) createSupportDeliveryAttempt(w http.ResponseWriter, r *http.Request, submissionID string) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Reporting is unavailable.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.reporting.Retry(r.Context(), deployment.ID, submissionID)
	if errors.Is(err, reporting.ErrDeliveryUnavailable) {
		writeError(w, http.StatusConflict, "reporting_delivery_unavailable", "Configure support delivery before retrying.", nil)
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "submission_not_retryable", "Only unexpired held or failed submissions can be retried.", nil)
		return
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	currentActor := actor(r)
	requestID, _ := r.Context().Value(requestIDKey).(string)
	if err := s.service.Store().AppendAudit(r.Context(), model.AuditEvent{ID: "audit_" + strconv.FormatInt(time.Now().UnixNano(), 10), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: currentActor.ID, Action: "support_submission.delivery_attempt_created", TargetType: "support_submission", TargetID: submissionID, Current: map[string]any{"kind": value.Kind, "state": value.State}, RequestID: requestID, CreatedAt: time.Now().UTC()}); err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, value)
}

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

func (s *Server) auditEvents(w http.ResponseWriter, r *http.Request, organisationID string) {
	values, err := s.service.Store().AuditEvents(r.Context(), organisationID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if len(values) > 500 {
		values = values[:500]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) integrationRuns(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().IntegrationRuns(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			EnvironmentID    string `json:"environment_id"`
			RequestedOutcome string `json:"requested_outcome"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.StartIntegrationRun(r.Context(), productID, input.EnvironmentID, input.RequestedOutcome, actor(r))
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

func (s *Server) completeIntegrationRun(w http.ResponseWriter, r *http.Request, productID, runID string) {
	var input struct {
		ReportedSuccess  *bool  `json:"reported_success"`
		ValidatedSuccess *bool  `json:"validated_success"`
		FailureCode      string `json:"failure_code"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.CompleteIntegrationRun(r.Context(), productID, runID, input.ReportedSuccess, input.ValidatedSuccess, input.FailureCode, actor(r))
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "integration_run_complete", "The integration run was already completed.", nil)
			return
		}
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

type namedResourceInput struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (s *Server) organisations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Organisations(r.Context())
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input namedResourceInput
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateOrganisation(r.Context(), input.Name, input.Slug, actor(r))
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

func (s *Server) products(w http.ResponseWriter, r *http.Request, organisationID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Products(r.Context(), organisationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input namedResourceInput
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateProduct(r.Context(), organisationID, input.Name, input.Slug, actor(r))
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

func (s *Server) environments(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Environments(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			namedResourceInput
			OrganisationID string `json:"organisation_id"`
			IsProduction   bool   `json:"is_production"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateEnvironment(r.Context(), input.OrganisationID, productID, input.Name, input.Slug, input.IsProduction, actor(r))
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

func (s *Server) creationError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "resource_conflict", "A resource with this slug or production role already exists.", nil)
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		s.storeError(w, err)
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_resource", err.Error(), nil)
}

func (s *Server) productCatalogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict):
		s.storeError(w, err)
	case errors.Is(err, platform.ErrProductDescriptionRequired):
		writeError(w, http.StatusUnprocessableEntity, "product_description_required", err.Error(), nil)
	case errors.Is(err, platform.ErrProductVersionDeprecated):
		writeError(w, http.StatusConflict, "product_version_deprecated", err.Error(), nil)
	case errors.Is(err, platform.ErrProductVersionLifecycle):
		writeError(w, http.StatusUnprocessableEntity, "invalid_product_version_lifecycle", err.Error(), nil)
	case errors.Is(err, platform.ErrProductVersionDrifted):
		writeError(w, http.StatusConflict, "product_version_drifted", err.Error(), nil)
	case errors.Is(err, platform.ErrPromotionApprovalRequired), errors.Is(err, platform.ErrPromotionSeparationOfDuties):
		writeError(w, http.StatusConflict, "promotion_approval_required", err.Error(), nil)
	case errors.Is(err, platform.ErrProductVersionImpact):
		writeError(w, http.StatusConflict, "product_version_impact_acknowledgement_required", err.Error(), nil)
	default:
		writeError(w, http.StatusBadRequest, "invalid_product_catalog", err.Error(), nil)
	}
}

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
