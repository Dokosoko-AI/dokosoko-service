package ai

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	ErrorInvalidCredential       ErrorCode = "invalid_credential"
	ErrorUnsupportedModel        ErrorCode = "unsupported_model"
	ErrorRateLimited             ErrorCode = "rate_limited"
	ErrorQuotaExhausted          ErrorCode = "quota_exhausted"
	ErrorProviderUnavailable     ErrorCode = "provider_unavailable"
	ErrorTimeout                 ErrorCode = "timeout"
	ErrorContextTooLarge         ErrorCode = "context_too_large"
	ErrorInvalidStructuredOutput ErrorCode = "invalid_structured_output"
	ErrorRefusedOutput           ErrorCode = "refused_output"
	ErrorBudgetExhausted         ErrorCode = "budget_exhausted"
	ErrorInvalidConfiguration    ErrorCode = "invalid_configuration"
	ErrorUnsafeInput             ErrorCode = "unsafe_input"
)

type Error struct {
	Code      ErrorCode
	Provider  string
	Retryable bool
	Cause     error
}

func (e *Error) Error() string {
	if e.Provider == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Provider, e.Code)
}

func (e *Error) Unwrap() error { return e.Cause }

func Code(err error) ErrorCode {
	var value *Error
	if errors.As(err, &value) {
		return value.Code
	}
	return ErrorProviderUnavailable
}

func Retryable(err error) bool {
	var value *Error
	return errors.As(err, &value) && value.Retryable
}
