package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/platform"
)

func TestAIWorkflowErrorsAreActionableAndDoNotExposeProviderCauses(t *testing.T) {
	for _, value := range []struct {
		err    error
		status int
		code   string
	}{
		{platform.ErrAIUnavailable, 503, "ai_unavailable"},
		{&platform.AISetupError{Code: "model_missing"}, 503, "ai_model_missing"},
		{&platform.AISetupError{Code: "workload_disabled"}, 503, "ai_workload_disabled"},
		{&platform.AISetupError{Code: "provider_disabled"}, 503, "ai_provider_disabled"},
		{&platform.AISetupError{Code: "credential_unavailable"}, 503, "ai_credential_unavailable"},
		{&ai.Error{Code: ai.ErrorInvalidCredential, Cause: errors.New("secret-provider-payload")}, 422, "ai_invalid_credential"},
		{&ai.Error{Code: ai.ErrorBudgetExhausted}, 429, "ai_budget_exhausted"},
		{&ai.Error{Code: ai.ErrorTimeout}, 504, "ai_timeout"},
		{&ai.Error{Code: ai.ErrorInvalidStructuredOutput}, 502, "ai_invalid_structured_output"},
	} {
		t.Run(value.code, func(t *testing.T) {
			response := httptest.NewRecorder()
			server := &Server{}
			server.recipeCreationError(response, value.err)
			if response.Code != value.status || !strings.Contains(response.Body.String(), value.code) || strings.Contains(response.Body.String(), "secret-provider-payload") || strings.Contains(response.Body.String(), "recipe_evidence_gap") {
				t.Fatalf("response=%d %s", response.Code, response.Body.String())
			}
		})
	}
	if writeAIWorkflowError(httptest.NewRecorder(), http.ErrNotSupported) {
		t.Fatal("non-AI error was misclassified")
	}
}
