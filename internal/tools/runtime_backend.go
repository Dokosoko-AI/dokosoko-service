package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

func (r *Runtime) executeRegisteredBackend(ctx context.Context, productID, fullName string, tool model.Tool, arguments map[string]any, principal Principal, executor BackendExecutor, trace *executionTrace, recordAudit bool) (returnValue any, returnErr error) {
	if SchemaContainsSensitiveFields(tool.InputSchema) || SchemaContainsSensitiveFields(tool.OutputSchema) || ValueContainsSensitiveFields(arguments) {
		return nil, ErrDenied
	}
	if principal.IdempotencyKey != "" && !validIdempotencyKey(principal.IdempotencyKey) || tool.IdempotencyMode == "required" && !validIdempotencyKey(principal.IdempotencyKey) {
		return nil, ErrInvalidIdempotencyKey
	}
	auditCategory, auditOutcome := "preflight_failed", "failure"
	defer func() {
		if trace != nil {
			trace.Category, trace.Phase = auditCategory, tracePhase(auditCategory, trace.Phase)
		}
		if !recordAudit || tool.BackendKind == "mcp" {
			return
		}
		current := map[string]any{"tool": fullName, "backend_kind": tool.BackendKind, "category": auditCategory}
		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if err := r.store.AppendAudit(auditCtx, model.AuditEvent{ID: auditID(), OrganisationID: tool.OrganisationID, ProductID: productID, ActorID: principal.Subject, Action: "tool.executed", TargetType: "tool", TargetID: tool.ID, Current: current, RequestID: principal.RequestID, Outcome: auditOutcome, CreatedAt: r.now()}); err != nil {
			returnValue, returnErr = nil, errors.Join(returnErr, fmt.Errorf("append tool execution audit: %w", err))
		}
	}()
	timeout := time.Duration(tool.TimeoutMS) * time.Millisecond
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	auditCategory = "backend_failed"
	value, err := executor.Execute(callCtx, tool, arguments, principal)
	if err != nil {
		var callErr *nativeplugin.CallError
		if errors.As(err, &callErr) {
			auditCategory = "native_" + string(callErr.Code)
		}
		return nil, err
	}
	if tool.BackendKind == "mcp" {
		return value, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("native tool output schema mismatch: result must be an object")
	}
	auditCategory = "response_schema_mismatch"
	if err := ValidateArguments(tool.OutputSchema, object); err != nil {
		return nil, fmt.Errorf("native tool output schema mismatch: %w", err)
	}
	encoded, err := json.Marshal(object)
	limit := tool.MaxResultBytes
	if limit <= 0 || limit > 1<<20 {
		limit = 1 << 20
	}
	if err != nil || int64(len(encoded)) > limit {
		return nil, errors.New("native tool result exceeds its declared size limit")
	}
	auditCategory, auditOutcome = "success", "success"
	return object, nil
}
