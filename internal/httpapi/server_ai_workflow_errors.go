package httpapi

import (
	"errors"
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/platform"
)

// Keep provider response bodies, endpoints, and credential causes out of the
// operator response. Each error identifies the action needed to resume work.
func writeAIWorkflowError(w http.ResponseWriter, err error) bool {
	var setup *platform.AISetupError
	if errors.As(err, &setup) {
		messages := map[string]string{
			"model_missing":          "Choose an Analysis model in AI settings before processing.",
			"workload_disabled":      "Enable the Analysis workload in AI settings before processing.",
			"provider_missing":       "Select an available AI provider for the Analysis workload.",
			"provider_disabled":      "Enable the selected AI provider before processing.",
			"credential_unavailable": "The AI provider credential is unavailable. Restore the deployment credential or update the connection in AI settings.",
			"invalid_configuration":  "Correct the Analysis model, provider, and token limits in AI settings.",
		}
		writeError(w, http.StatusServiceUnavailable, "ai_"+setup.Code, messages[setup.Code], nil)
		return true
	}
	if errors.Is(err, platform.ErrAIUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "ai_unavailable", "AI processing is required. Enable an analysis model and a working provider in AI settings, then retry.", nil)
		return true
	}
	var failure *ai.Error
	if !errors.As(err, &failure) {
		return false
	}
	status, message := http.StatusBadGateway, "AI processing failed. Check the provider connection and retry."
	switch failure.Code {
	case ai.ErrorInvalidCredential:
		status, message = http.StatusUnprocessableEntity, "The AI provider rejected its credential. Update and test the provider connection, then retry."
	case ai.ErrorInvalidConfiguration, ai.ErrorUnsupportedModel:
		status, message = http.StatusUnprocessableEntity, "The configured AI model cannot process this request. Check the analysis workload and test its provider connection."
	case ai.ErrorBudgetExhausted:
		status, message = http.StatusTooManyRequests, "The configured AI processing budget is exhausted. Review the workload budget before retrying."
	case ai.ErrorQuotaExhausted:
		status, message = http.StatusTooManyRequests, "The AI provider quota is exhausted. Restore provider capacity, then retry."
	case ai.ErrorRateLimited:
		status, message = http.StatusTooManyRequests, "The AI provider is rate limiting requests. Wait briefly, then retry."
	case ai.ErrorTimeout:
		status, message = http.StatusGatewayTimeout, "The AI provider timed out. Retry this processing stage."
	case ai.ErrorContextTooLarge:
		status, message = http.StatusUnprocessableEntity, "The selected evidence exceeds the model context limit. Reduce the selection or choose a model with more capacity."
	case ai.ErrorUnsafeInput:
		status, message = http.StatusUnprocessableEntity, "The selected evidence failed the AI input safety checks. Review the flagged material before retrying."
	case ai.ErrorInvalidStructuredOutput:
		message = "The AI result did not match the required format or evidence. Processing stopped; retry or select another analysis model."
	case ai.ErrorRefusedOutput:
		status, message = http.StatusUnprocessableEntity, "The AI provider declined this request. Review the task and selected evidence before retrying."
	}
	writeError(w, status, "ai_"+string(failure.Code), message, nil)
	return true
}
