package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

var (
	ErrToolTestConsentInvalid       = errors.New("tool test consent is invalid")
	ErrToolTestConfirmationInvalid  = errors.New("tool test confirmation is invalid or expired")
	ErrToolTestConfirmationReplayed = errors.New("tool test confirmation was already consumed")
	ErrToolTestRevisionStale        = errors.New("tool test revision is stale")
	ErrToolTestUnavailable          = errors.New("tool test runtime is unavailable")
	ErrToolTestNotEligible          = errors.New("tool is not eligible for a live HTTP test")
	ErrToolTestRequiresReview       = errors.New("tool requires deterministic review before live testing")
	ErrToolTestOutcomeIndeterminate = errors.New("the upstream test outcome could not be durably recorded; do not retry")
)

const (
	toolTestConfirmationTTL = 5 * time.Minute
	toolTestEvidenceTTL     = 24 * time.Hour
	toolTestCleanupTimeout  = 250 * time.Millisecond
	toolTestCleanupBatch    = 100
)

type ToolTestConfirmationInput struct {
	Revision               int64
	Arguments              map[string]any
	TypedToolName          string
	AcknowledgeSideEffects bool
}

type ToolTestConfirmationResult struct {
	ConfirmationNonce string    `json:"confirmation_nonce"`
	ExpiresAt         time.Time `json:"expires_at"`
	ToolID            string    `json:"tool_id"`
	ToolRevision      int64     `json:"tool_revision"`
}

type ToolTestRunInput struct {
	Revision          int64
	Arguments         map[string]any
	ConfirmationNonce string
	IdempotencyKey    string
}

func canonicalToolTestArgumentHash(arguments map[string]any) ([]byte, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, errors.New("tool test arguments are not valid JSON")
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte("dokosoko-tool-test-arguments-v1\x00"))
	_, _ = digest.Write(encoded)
	return digest.Sum(nil), nil
}

func randomToolTestNonce() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	nonce := "ttc_" + base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(nonce))
	return nonce, digest[:], nil
}

func (s *Service) toolForExactTestRevision(ctx context.Context, productID, toolID string, revision int64) (model.Tool, error) {
	tool, err := s.store.Tool(ctx, productID, toolID)
	if err != nil {
		return model.Tool{}, err
	}
	if revision < 1 || tool.Revision != revision {
		return model.Tool{}, ErrToolTestRevisionStale
	}
	if (tool.State != "draft" && tool.State != "published") || tool.BackendKind != "http" || tool.UpstreamDrifted {
		return model.Tool{}, ErrToolTestNotEligible
	}
	return tool, nil
}

func (s *Service) cleanupExpiredToolTestData(ctx context.Context) {
	cleanupCtx, cancel := context.WithTimeout(ctx, toolTestCleanupTimeout)
	defer cancel()
	_, _ = s.store.DeleteExpiredToolTestData(cleanupCtx, s.now(), toolTestCleanupBatch)
}

func toolTestConfirmationRequired(method string, policy ToolPolicy) bool {
	return strings.ToUpper(method) != http.MethodGet || policy.ConfirmationRequired
}

func (s *Service) CreateToolTestConfirmation(ctx context.Context, productID, toolID string, input ToolTestConfirmationInput, actor Actor) (ToolTestConfirmationResult, error) {
	s.cleanupExpiredToolTestData(ctx)
	tool, err := s.toolForExactTestRevision(ctx, productID, toolID, input.Revision)
	if err != nil {
		return ToolTestConfirmationResult{}, err
	}
	if err := s.validateStoredHTTPTool(ctx, tool); err != nil {
		return ToolTestConfirmationResult{}, ErrToolTestRequiresReview
	}
	_, policy, err := normalizeToolPolicy(tool.AuthorizationPolicy, strings.ToUpper(tool.HTTPMethod))
	if err != nil {
		return ToolTestConfirmationResult{}, ErrToolTestRequiresReview
	}
	if strings.ToUpper(tool.HTTPMethod) != http.MethodGet && !policy.IdempotencyRequired {
		return ToolTestConfirmationResult{}, ErrToolTestRequiresReview
	}
	if !toolTestConfirmationRequired(tool.HTTPMethod, policy) {
		return ToolTestConfirmationResult{}, errors.New("this tool test does not require confirmation")
	}
	if !input.AcknowledgeSideEffects || strings.TrimSpace(input.TypedToolName) != tool.Namespace+"."+tool.Name {
		return ToolTestConfirmationResult{}, ErrToolTestConsentInvalid
	}
	if input.Arguments == nil {
		input.Arguments = map[string]any{}
	}
	if err := toolruntime.ValidateArguments(tool.InputSchema, input.Arguments); err != nil {
		return ToolTestConfirmationResult{}, errors.New("tool test arguments do not match the declared input schema")
	}
	argumentHash, err := canonicalToolTestArgumentHash(input.Arguments)
	if err != nil {
		return ToolTestConfirmationResult{}, err
	}
	nonce, nonceDigest, err := randomToolTestNonce()
	if err != nil {
		return ToolTestConfirmationResult{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return ToolTestConfirmationResult{}, err
	}
	now := s.now()
	confirmation := model.ToolTestConfirmation{ID: id, OrganisationID: tool.OrganisationID, ProductID: productID, ToolID: tool.ID, ToolRevision: tool.Revision, ArgumentHash: argumentHash, NonceDigest: nonceDigest, ActorID: actor.ID, ExpiresAt: now.Add(toolTestConfirmationTTL), CreatedAt: now}
	if err := s.store.CreateToolTestConfirmation(ctx, confirmation); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return ToolTestConfirmationResult{}, ErrToolTestRevisionStale
		}
		return ToolTestConfirmationResult{}, err
	}
	return ToolTestConfirmationResult{ConfirmationNonce: nonce, ExpiresAt: confirmation.ExpiresAt, ToolID: tool.ID, ToolRevision: tool.Revision}, nil
}

func (s *Service) appendToolTestRun(ctx context.Context, runID string, tool model.Tool, argumentHash []byte, report toolruntime.DraftTestReport, actor Actor) (model.ToolTestRun, error) {
	if runID == "" {
		var err error
		runID, err = randomUUID()
		if err != nil {
			return model.ToolTestRun{}, err
		}
	}
	now := s.now()
	run := model.ToolTestRun{
		ID: runID, OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, ToolID: tool.ID, ToolRevision: tool.Revision,
		ToolName: tool.Namespace + "." + tool.Name, ActorID: actor.ID, RequestID: actor.RequestID, ArgumentHash: append([]byte(nil), argumentHash...),
		Method: strings.ToUpper(tool.HTTPMethod), AuthenticationType: report.AuthenticationType, Outcome: report.Outcome, Phase: report.Phase,
		NetworkCallPerformed: report.NetworkCallPerformed, UpstreamStatusCode: report.UpstreamStatusCode, ResponseBytes: report.ResponseBytes,
		RequestShape: report.RequestShape, ResponseShape: report.ResponseShape, Findings: report.Findings, DurationMS: report.DurationMS,
		ExpiresAt: now.Add(toolTestEvidenceTTL), CreatedAt: now,
	}
	if run.Findings == nil {
		run.Findings = []model.ToolTestFinding{}
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := s.store.AppendToolTestRun(persistCtx, run); err != nil {
		return model.ToolTestRun{}, err
	}
	// The short-lived run is the authoritative durable outcome. Do not turn a
	// successfully persisted result into a retryable API error solely because
	// the secondary activity entry could not be appended.
	if err := s.store.AppendAudit(persistCtx, model.AuditEvent{ID: randomID("audit"), OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, ActorID: actor.ID, Action: "tool.test.executed", TargetType: "tool_test_run", TargetID: run.ID, Current: map[string]any{"tool_id": tool.ID, "tool_revision": tool.Revision, "outcome": run.Outcome, "phase": run.Phase, "network_call_performed": run.NetworkCallPerformed, "status_code": run.UpstreamStatusCode}, RequestID: actor.RequestID, Outcome: run.Outcome, CreatedAt: now}); err != nil {
		return model.ToolTestRun{}, err
	}
	return run, nil
}

func (s *Service) appendToolTestExecutionIntent(ctx context.Context, runID string, tool model.Tool, actor Actor) error {
	persistCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.store.AppendAudit(persistCtx, model.AuditEvent{
		ID: randomID("audit"), OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, ActorID: actor.ID,
		Action: "tool.test.execution.intent", TargetType: "tool_test_run", TargetID: runID,
		Current: map[string]any{
			"tool_id": tool.ID, "tool_revision": tool.Revision, "method": strings.ToUpper(tool.HTTPMethod),
			"authentication_type": storedToolAuthType(tool), "possible_side_effects": strings.ToUpper(tool.HTTPMethod) != http.MethodGet,
		},
		RequestID: actor.RequestID, Outcome: "accepted", CreatedAt: s.now(),
	})
}

func (s *Service) appendToolTestIndeterminateAudit(ctx context.Context, runID string, tool model.Tool, report toolruntime.DraftTestReport, actor Actor) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	return s.store.AppendAudit(persistCtx, model.AuditEvent{
		ID: randomID("audit"), OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, ActorID: actor.ID,
		Action: "tool.test.execution.indeterminate", TargetType: "tool_test_run", TargetID: runID,
		Current: map[string]any{
			"tool_id": tool.ID, "tool_revision": tool.Revision, "phase": report.Phase,
			"network_call_performed": report.NetworkCallPerformed, "status_code": report.UpstreamStatusCode,
		},
		RequestID: actor.RequestID, Outcome: "indeterminate", CreatedAt: s.now(),
	})
}

func (s *Service) RunToolTest(ctx context.Context, runtime *toolruntime.Runtime, productID, toolID string, input ToolTestRunInput, actor Actor) (model.ToolTestRun, error) {
	s.cleanupExpiredToolTestData(ctx)
	if runtime == nil {
		return model.ToolTestRun{}, ErrToolTestUnavailable
	}
	tool, err := s.toolForExactTestRevision(ctx, productID, toolID, input.Revision)
	if err != nil {
		return model.ToolTestRun{}, err
	}
	if input.Arguments == nil {
		input.Arguments = map[string]any{}
	}
	argumentHash, err := canonicalToolTestArgumentHash(input.Arguments)
	if err != nil {
		return model.ToolTestRun{}, err
	}
	method := strings.ToUpper(tool.HTTPMethod)
	_, policy, err := normalizeToolPolicy(tool.AuthorizationPolicy, method)
	if err != nil {
		return model.ToolTestRun{}, errors.New("stored tool authorization policy is invalid")
	}
	if method != http.MethodGet && !policy.IdempotencyRequired {
		return model.ToolTestRun{}, ErrToolTestRequiresReview
	}
	if method != http.MethodGet && !toolruntime.ValidIdempotencyKey(input.IdempotencyKey) {
		return model.ToolTestRun{}, toolruntime.ErrInvalidIdempotencyKey
	}
	if toolTestConfirmationRequired(method, policy) {
		if strings.TrimSpace(input.ConfirmationNonce) == "" {
			return model.ToolTestRun{}, ErrToolTestConfirmationInvalid
		}
		digest := sha256.Sum256([]byte(input.ConfirmationNonce))
		consumptionID, randomErr := randomUUID()
		if randomErr != nil {
			return model.ToolTestRun{}, randomErr
		}
		_, err = s.store.ConsumeToolTestConfirmation(ctx, digest[:], productID, toolID, tool.Revision, argumentHash, actor.ID, consumptionID, s.now())
		if err != nil {
			if errors.Is(err, store.ErrConflict) {
				return model.ToolTestRun{}, ErrToolTestConfirmationReplayed
			}
			if errors.Is(err, store.ErrNotFound) {
				return model.ToolTestRun{}, ErrToolTestConfirmationInvalid
			}
			return model.ToolTestRun{}, err
		}
	}
	if err := s.validateStoredHTTPTool(ctx, tool); err != nil {
		if method != http.MethodGet || policy.ConfirmationRequired {
			return model.ToolTestRun{}, ErrToolTestRequiresReview
		}
		authenticationType := storedToolAuthType(tool)
		if !map[string]bool{"delegated_oauth": true, "none": true, "bearer": true, "authorization_scheme": true, "api_key_header": true, "api_key_query": true, "basic": true, "oauth_client_credentials": true, "custom_header": true}[authenticationType] {
			return model.ToolTestRun{}, ErrToolTestRequiresReview
		}
		report := toolruntime.DraftTestReport{AuthenticationType: authenticationType, Outcome: "failure", Phase: "preflight", RequestShape: toolruntime.SanitizedJSONShape(input.Arguments), Findings: []model.ToolTestFinding{{Phase: "preflight", Code: "stored_tool_requires_review", Message: "The stored HTTP tool failed deterministic review before a network call."}}}
		return s.appendToolTestRun(ctx, "", tool, argumentHash, report, actor)
	}
	// Re-read immediately before the one-shot call. A changed draft invalidates
	// the exact-revision consent and must perform no network request.
	latest, err := s.toolForExactTestRevision(ctx, productID, toolID, input.Revision)
	if err != nil {
		return model.ToolTestRun{}, err
	}
	runID, err := randomUUID()
	if err != nil {
		return model.ToolTestRun{}, err
	}
	if err := s.appendToolTestExecutionIntent(ctx, runID, latest, actor); err != nil {
		return model.ToolTestRun{}, err
	}
	principal := toolruntime.Principal{Subject: actor.ID, RequestID: actor.RequestID, IdempotencyKey: input.IdempotencyKey}
	report := runtime.ExecuteHTTPDraftTest(ctx, productID, latest, input.Arguments, principal)
	run, persistErr := s.appendToolTestRun(ctx, runID, latest, argumentHash, report, actor)
	if persistErr != nil {
		return model.ToolTestRun{}, errors.Join(ErrToolTestOutcomeIndeterminate, persistErr, s.appendToolTestIndeterminateAudit(ctx, runID, latest, report, actor))
	}
	return run, nil
}

func (s *Service) ToolTestRuns(ctx context.Context, productID, toolID string) ([]model.ToolTestRun, error) {
	s.cleanupExpiredToolTestData(ctx)
	return s.store.ToolTestRuns(ctx, productID, toolID, s.now())
}

func (s *Service) ToolTestRun(ctx context.Context, productID, toolID, runID string) (model.ToolTestRun, error) {
	s.cleanupExpiredToolTestData(ctx)
	return s.store.ToolTestRun(ctx, productID, toolID, runID, s.now())
}
