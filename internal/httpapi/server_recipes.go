package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Server) publishedRecipes(ctx context.Context, productID string, public bool) ([]model.Recipe, error) {
	values, err := s.service.ReconcileRecipeDrift(ctx, productID)
	if err != nil {
		return nil, err
	}
	result := make([]model.Recipe, 0, len(values))
	for _, recipe := range values {
		if recipe.State != "published" || recipe.NeedsAttention || (public && recipe.Visibility != model.VisibilityPublic) {
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
		var input struct {
			Answers map[string]string `json:"answers"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.AnswerIntegrationUnknowns(r.Context(), productID, analysisID, input.Answers, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
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
		s.creationError(w, err)
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
			Prompt        string `json:"prompt"`
			IntegrationID string `json:"integration_id"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateRecipeFromPromptFor(r.Context(), productID, input.IntegrationID, input.Prompt, actor(r))
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

func (s *Server) recipe(w http.ResponseWriter, r *http.Request, productID, recipeID string) {
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().Recipe(r.Context(), productID, recipeID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		revisions, _ := s.service.Store().RecipeRevisions(r.Context(), value.ID)
		writeJSON(w, http.StatusOK, map[string]any{"recipe": value, "revisions": revisions})
	case http.MethodPatch:
		var input struct {
			Markdown   string                  `json:"markdown"`
			References []model.RecipeReference `json:"references"`
			Visibility model.Visibility        `json:"visibility"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.UpdateRecipeMarkdown(r.Context(), productID, recipeID, input.Markdown, input.References, input.Visibility, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) reworkRecipe(w http.ResponseWriter, r *http.Request, productID, recipeID string) {
	var input struct {
		Instruction string `json:"instruction"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.ReworkRecipe(r.Context(), productID, recipeID, input.Instruction, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) approveRecipe(w http.ResponseWriter, r *http.Request, productID, recipeID string) {
	value, err := s.service.ApproveRecipe(r.Context(), productID, recipeID, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) publishRecipe(w http.ResponseWriter, r *http.Request, productID, recipeID string) {
	value, err := s.service.PublishRecipe(r.Context(), productID, recipeID, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) attention(w http.ResponseWriter, r *http.Request, productID string) {
	recipes, err := s.service.ReconcileRecipeDrift(r.Context(), productID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	analyses, err := s.service.Store().IntegrationAnalyses(r.Context(), productID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	attentionRecipes := make([]model.Recipe, 0)
	for _, recipe := range recipes {
		if recipe.NeedsAttention {
			attentionRecipes = append(attentionRecipes, recipe)
		}
	}
	questions := make([]map[string]any, 0)
	for _, analysis := range analyses {
		for _, unknown := range analysis.Unknowns {
			if strings.TrimSpace(unknown.Answer) == "" {
				questions = append(questions, map[string]any{"analysis_id": analysis.ID, "question": unknown})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"recipes": attentionRecipes, "questions": questions, "count": len(attentionRecipes) + len(questions)})
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
