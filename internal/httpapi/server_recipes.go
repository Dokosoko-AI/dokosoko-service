package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Server) recipeCreationError(w http.ResponseWriter, err error) {
	if errors.Is(err, platform.ErrRecipeGroundingChanged) {
		writeError(w, http.StatusConflict, "recipe_grounding_changed", "The reviewed product evidence changed. Reload, analyse, and regenerate the recipe before retrying.", nil)
		return
	}
	if errors.Is(err, platform.ErrPublicMCPRecipe) {
		writeError(w, http.StatusUnprocessableEntity, "recipe_public_visibility_unsupported", "MCP-backed product recipes are private-only until public custom-tool exposure is supported.", nil)
		return
	}
	if errors.Is(err, platform.ErrRecipeAnalysisScope) {
		writeError(w, http.StatusBadRequest, "recipe_scope_mismatch", err.Error(), nil)
		return
	}
	if errors.Is(err, platform.ErrRecipeNeedsInput) {
		writeError(w, http.StatusUnprocessableEntity, "recipe_evidence_gap", err.Error(), nil)
		return
	}
	s.creationError(w, err)
}

func (s *Server) recipeUpdateError(w http.ResponseWriter, err error) {
	if errors.Is(err, platform.ErrRecipeDeletionNotAllowed) {
		writeError(w, http.StatusUnprocessableEntity, "recipe_delete_not_allowed", err.Error(), nil)
		return
	}
	if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrCatalogConflict) {
		writeError(w, http.StatusConflict, "recipe_revision_conflict", "The recipe changed. Reload it and review the latest revision before saving again.", nil)
		return
	}
	s.recipeCreationError(w, err)
}

type recipeRevisionExpectation struct {
	Revision          int64  `json:"revision"`
	CurrentRevisionID string `json:"current_revision_id"`
}

func (value recipeRevisionExpectation) valid() bool {
	return value.Revision >= 1 && strings.TrimSpace(value.CurrentRevisionID) != ""
}

func recipeAvailableForMCP(recipe model.Recipe, public bool) bool {
	if recipe.State != "published" || recipe.NeedsAttention || (recipe.ContractVersion != model.RecipeContractProductIntegrationV2 && recipe.ContractVersion != model.RecipeContractDeploymentV3) {
		return false
	}
	apiIDs := recipeAPIIDs(recipe)
	if recipe.PublishedAt == nil || len(apiIDs) == 0 || strings.TrimSpace(recipe.StableURI) == "" || strings.TrimSpace(recipe.CurrentRevisionID) == "" {
		return false
	}
	if recipe.ContractVersion == model.RecipeContractDeploymentV3 {
		bindingIDs, valid := recipeRevisionBindingAPIIDs(recipe.CurrentRevision)
		if recipe.CurrentRevision == nil || recipe.CurrentRevision.SpecVersion != model.RecipeSpecVersion3 || !valid || !slices.Equal(apiIDs, bindingIDs) {
			return false
		}
	}
	return !public || recipe.Visibility == model.VisibilityPublic
}

func recipeRevisionBindingAPIIDs(revision *model.RecipeRevision) ([]string, bool) {
	if revision == nil || len(revision.APIBindings) == 0 {
		return nil, false
	}
	values := make([]string, 0, len(revision.APIBindings))
	seen := make(map[string]bool, len(revision.APIBindings))
	for _, binding := range revision.APIBindings {
		id := strings.TrimSpace(binding.IntegrationID)
		if id == "" || strings.TrimSpace(binding.IntegrationRevisionID) == "" || strings.TrimSpace(binding.IntegrationManifestHash) == "" || seen[id] {
			return nil, false
		}
		seen[id] = true
		values = append(values, id)
	}
	sort.Strings(values)
	return values, true
}

func (s *Server) publishedRecipes(ctx context.Context, productID string, public bool) ([]model.Recipe, error) {
	values, err := s.service.ReconcileRecipeDrift(ctx, productID)
	if err != nil {
		return nil, err
	}
	result := make([]model.Recipe, 0, len(values))
	for _, recipe := range values {
		if !recipeAvailableForMCP(recipe, public) {
			continue
		}
		result = append(result, recipe)
	}
	return result, nil
}

func (s *Server) publishedRecipeByURI(ctx context.Context, productID, uri string, public bool) (model.Recipe, error) {
	values, err := s.publishedRecipes(ctx, productID, public)
	if err != nil {
		return model.Recipe{}, err
	}
	for _, recipe := range values {
		if recipe.StableURI == uri {
			return recipe, nil
		}
	}
	return model.Recipe{}, store.ErrNotFound
}

func (s *Server) integrationAnalyses(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().IntegrationAnalyses(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			IntegrationID string `json:"integration_id"`
		}
		if r.ContentLength != 0 {
			if err := decodeJSON(r.Body, &input); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
				return
			}
		}
		var value model.IntegrationAnalysis
		var err error
		if strings.TrimSpace(input.IntegrationID) == "" {
			value, err = s.service.AnalyseIntegration(r.Context(), productID, actor(r))
		} else {
			value, err = s.service.AnalyseIntegrationFor(r.Context(), productID, input.IntegrationID, actor(r))
		}
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

func (s *Server) integrationAnalysis(w http.ResponseWriter, r *http.Request, productID, analysisID string) {
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().IntegrationAnalysis(r.Context(), productID, analysisID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		writeError(w, http.StatusGone, "analysis_answers_removed", "Analysis unknowns are read-only evidence gaps. Change the evidence or configuration and run a new analysis.", nil)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) generateRecipes(w http.ResponseWriter, r *http.Request, productID, analysisID string) {
	var input struct {
		IntegrationID string `json:"integration_id"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
	}
	var values []model.Recipe
	var err error
	if strings.TrimSpace(input.IntegrationID) == "" {
		values, err = s.service.GenerateRecipes(r.Context(), productID, analysisID, actor(r))
	} else {
		values, err = s.service.GenerateRecipesForIntegration(r.Context(), productID, analysisID, input.IntegrationID, actor(r))
	}
	if err != nil {
		s.recipeCreationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"items": values})
}

func (s *Server) recipes(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.ReconcileRecipeDrift(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			Prompt         string   `json:"prompt"`
			IntegrationID  string   `json:"integration_id"`
			IntegrationIDs []string `json:"integration_ids"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if strings.TrimSpace(input.IntegrationID) != "" {
			if len(input.IntegrationIDs) != 0 {
				writeError(w, http.StatusBadRequest, "invalid_request", "use integration_ids instead of combining it with integration_id", nil)
				return
			}
			input.IntegrationIDs = []string{input.IntegrationID}
		}
		value, err := s.service.CreateRecipeFromPromptWithAPIs(r.Context(), productID, input.IntegrationIDs, input.Prompt, actor(r))
		if err != nil {
			s.recipeCreationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) recipe(w http.ResponseWriter, r *http.Request, productID, recipeID string) {
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().Recipe(r.Context(), productID, recipeID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		revisions, err := s.service.Store().RecipeRevisions(r.Context(), value.ID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"recipe": value, "revisions": revisions})
	case http.MethodPatch:
		var input struct {
			ReferenceIDs      *[]string        `json:"reference_ids"`
			Visibility        model.Visibility `json:"visibility"`
			Revision          int64            `json:"revision"`
			CurrentRevisionID string           `json:"current_revision_id"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.ReferenceIDs == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "reference_ids is required", nil)
			return
		}
		if input.Revision < 1 || strings.TrimSpace(input.CurrentRevisionID) == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision and current_revision_id are required", nil)
			return
		}
		value, err := s.service.UpdateRecipeReferences(r.Context(), productID, recipeID, input.Revision, input.CurrentRevisionID, *input.ReferenceIDs, input.Visibility, actor(r))
		if err != nil {
			s.recipeUpdateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodDelete:
		var input recipeRevisionExpectation
		if err := decodeJSON(r.Body, &input); err != nil || !input.valid() {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision and current_revision_id are required", nil)
			return
		}
		if err := s.service.DeleteRecipe(r.Context(), productID, recipeID, input.Revision, input.CurrentRevisionID, actor(r)); err != nil {
			s.recipeUpdateError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PATCH, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) reworkRecipe(w http.ResponseWriter, r *http.Request, productID, recipeID string) {
	var input struct {
		recipeRevisionExpectation
		Instruction string `json:"instruction"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if !input.recipeRevisionExpectation.valid() {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision and current_revision_id are required", nil)
		return
	}
	value, err := s.service.ReworkRecipe(r.Context(), productID, recipeID, input.Revision, input.CurrentRevisionID, input.Instruction, actor(r))
	if err != nil {
		s.recipeUpdateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) approveRecipe(w http.ResponseWriter, r *http.Request, productID, recipeID string) {
	var input recipeRevisionExpectation
	if err := decodeJSON(r.Body, &input); err != nil || !input.valid() {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision and current_revision_id are required", nil)
		return
	}
	value, err := s.service.ApproveRecipe(r.Context(), productID, recipeID, input.Revision, input.CurrentRevisionID, actor(r))
	if err != nil {
		s.recipeUpdateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) publishRecipe(w http.ResponseWriter, r *http.Request, productID, recipeID string) {
	var input recipeRevisionExpectation
	if err := decodeJSON(r.Body, &input); err != nil || !input.valid() {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision and current_revision_id are required", nil)
		return
	}
	value, err := s.service.PublishRecipe(r.Context(), productID, recipeID, input.Revision, input.CurrentRevisionID, actor(r))
	if err != nil {
		s.recipeUpdateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) attention(w http.ResponseWriter, r *http.Request, productID string) {
	writeError(w, http.StatusGone, "attention_endpoint_removed", "Read recipe needs_attention state directly. Resolve analysis unknowns by changing evidence or configuration and running a new analysis.", nil)
}

func operationalSince(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 365 {
			writeError(w, http.StatusBadRequest, "invalid_period", "days must be between 1 and 365.", nil)
			return time.Time{}, false
		}
		days = value
	}
	return time.Now().UTC().AddDate(0, 0, -days), true
}

func (s *Server) aiUsage(w http.ResponseWriter, r *http.Request, productID string) {
	since, ok := operationalSince(w, r)
	if !ok {
		return
	}
	events, err := s.service.Store().AIUsageEvents(r.Context(), productID, since)
	if err != nil {
		s.storeError(w, err)
		return
	}
	type workloadUsage struct {
		Workload     string `json:"workload"`
		Calls        int64  `json:"calls"`
		Errors       int64  `json:"errors"`
		InputTokens  int64  `json:"input_tokens"`
		OutputTokens int64  `json:"output_tokens"`
		DurationMS   int64  `json:"duration_ms"`
	}
	type providerUsage struct {
		Provider     string    `json:"provider"`
		Calls        int64     `json:"calls"`
		Errors       int64     `json:"errors"`
		InputTokens  int64     `json:"input_tokens"`
		OutputTokens int64     `json:"output_tokens"`
		DurationMS   int64     `json:"duration_ms"`
		BackupCalls  int64     `json:"backup_calls"`
		LastUsedAt   time.Time `json:"last_used_at"`
	}
	byWorkload := map[string]workloadUsage{}
	byProvider := map[string]providerUsage{}
	for _, event := range events {
		value := byWorkload[event.Workload]
		value.Workload = event.Workload
		value.Calls++
		if event.Outcome != "succeeded" {
			value.Errors++
		}
		value.InputTokens += event.InputTokens
		value.OutputTokens += event.OutputTokens
		value.DurationMS += event.DurationMS
		byWorkload[event.Workload] = value

		providerValue := byProvider[event.Provider]
		providerValue.Provider = event.Provider
		providerValue.Calls++
		if event.Outcome != "succeeded" {
			providerValue.Errors++
		}
		if event.ProviderRole == "backup" {
			providerValue.BackupCalls++
		}
		providerValue.InputTokens += event.InputTokens
		providerValue.OutputTokens += event.OutputTokens
		providerValue.DurationMS += event.DurationMS
		if event.CreatedAt.After(providerValue.LastUsedAt) {
			providerValue.LastUsedAt = event.CreatedAt
		}
		byProvider[event.Provider] = providerValue
	}
	workloads := make([]workloadUsage, 0, len(byWorkload))
	for _, name := range []string{"analysis", "assistant"} {
		if value, exists := byWorkload[name]; exists {
			workloads = append(workloads, value)
		}
	}
	providers := make([]providerUsage, 0, len(byProvider))
	for _, name := range []string{"openai", "google", "anthropic", "digitalocean", "xai", "deepseek", "openai-compatible"} {
		if value, exists := byProvider[name]; exists {
			providers = append(providers, value)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": events, "workloads": workloads, "providers": providers, "since": since, "generated_at": time.Now().UTC()})
}
