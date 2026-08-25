package platform_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/internal/tools"
)

type liveTestResolver struct{ address net.IP }

func (r liveTestResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{r.address}, nil
}

type liveTestDoer func(*http.Request) (*http.Response, error)

func (f liveTestDoer) Do(request *http.Request) (*http.Response, error) { return f(request) }

type liveTestCleanupTrackingStore struct {
	store.Store
	calls int
	limit int
}

type liveTestPersistenceFailureStore struct {
	store.Store
	failIntent bool
	failRun    bool
}

func (s *liveTestPersistenceFailureStore) AppendAudit(ctx context.Context, event model.AuditEvent) error {
	if s.failIntent && event.Action == "tool.test.execution.intent" {
		return errors.New("execution intent unavailable")
	}
	return s.Store.AppendAudit(ctx, event)
}

func (s *liveTestPersistenceFailureStore) AppendToolTestRun(ctx context.Context, run model.ToolTestRun) error {
	if s.failRun {
		return errors.New("test outcome unavailable")
	}
	return s.Store.AppendToolTestRun(ctx, run)
}

func (s *liveTestCleanupTrackingStore) DeleteExpiredToolTestData(ctx context.Context, now time.Time, limit int) (int64, error) {
	s.calls++
	s.limit = limit
	return s.Store.DeleteExpiredToolTestData(ctx, now, limit)
}

func createMutationTool(t *testing.T, service *platform.Service, name string) (string, int64) {
	t.Helper()
	value, err := service.CreateTool(context.Background(), platform.ToolInput{
		ProductID: "prod_acme", Namespace: "live", Name: name, Description: "Test one existing API operation.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"label":{"type":"string"}},"required":["label"]}`),
		OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"},"detail":{"type":"string"}},"required":["ok","detail"]}`),
		Endpoint:     "https://api.example.test/items", HTTPMethod: http.MethodPost, UpstreamAuth: json.RawMessage(`{"type":"none"}`),
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":true,"risk":"high","idempotency_required":true}`), TimeoutMS: 1000,
	}, platform.Actor{ID: "root", RequestID: "create"})
	if err != nil {
		t.Fatal(err)
	}
	return value.ID, value.Revision
}

func TestMutationToolTestRequiresExactSingleUseConsentAndStoresSanitizedEvidence(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	toolID, revision := createMutationTool(t, service, "mutate")
	arguments := map[string]any{"label": "private-request-value"}
	actor := platform.Actor{ID: "root", RequestID: "live-request"}

	if _, err := service.CreateToolTestConfirmation(context.Background(), "prod_acme", toolID, platform.ToolTestConfirmationInput{Revision: revision, Arguments: arguments, TypedToolName: "wrong.name", AcknowledgeSideEffects: true}, actor); !errors.Is(err, platform.ErrToolTestConsentInvalid) {
		t.Fatalf("wrong typed name error = %v", err)
	}
	confirmation, err := service.CreateToolTestConfirmation(context.Background(), "prod_acme", toolID, platform.ToolTestConfirmationInput{Revision: revision, Arguments: arguments, TypedToolName: "live.mutate", AcknowledgeSideEffects: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := tools.NewRuntime(memory, liveTestResolver{address: net.ParseIP("8.8.8.8")}, liveTestDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"detail":"private-response-value"}`))}, nil
	}))
	run, err := service.RunToolTest(context.Background(), runtime, "prod_acme", toolID, platform.ToolTestRunInput{Revision: revision, Arguments: arguments, ConfirmationNonce: confirmation.ConfirmationNonce, IdempotencyKey: "live-test-request-0001"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || run.Outcome != "success" || !run.NetworkCallPerformed || run.RequestShape.Properties["label"].Type != "string" || run.ResponseShape == nil || run.ResponseShape.Properties["detail"].Type != "string" {
		t.Fatalf("calls=%d run=%#v", calls, run)
	}
	encoded, _ := json.Marshal(run)
	for _, forbidden := range []string{"private-request-value", "private-response-value", confirmation.ConfirmationNonce, "Idempotency-Key", "api.example.test"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("evidence leaked %q: %s", forbidden, encoded)
		}
	}
	if _, err := service.RunToolTest(context.Background(), runtime, "prod_acme", toolID, platform.ToolTestRunInput{Revision: revision, Arguments: arguments, ConfirmationNonce: confirmation.ConfirmationNonce, IdempotencyKey: "live-test-request-0001"}, actor); !errors.Is(err, platform.ErrToolTestConfirmationReplayed) {
		t.Fatalf("replay error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("replay performed %d total calls", calls)
	}
	stored, err := service.ToolTestRun(context.Background(), "prod_acme", toolID, run.ID)
	if err != nil || stored.ID != run.ID {
		t.Fatalf("stored run=%#v err=%v", stored, err)
	}
	audits, err := memory.AuditEvents(context.Background(), "org_acme")
	if err != nil {
		t.Fatal(err)
	}
	foundAudit := false
	for _, event := range audits {
		foundAudit = foundAudit || event.Action == "tool.test.executed" && event.TargetID == run.ID
	}
	if !foundAudit {
		t.Fatal("tool.test.executed audit was not recorded")
	}
}

func TestToolTestFailsClosedBeforeNetworkWhenExecutionIntentCannotBePersisted(t *testing.T) {
	memory := store.NewMemory()
	failing := &liveTestPersistenceFailureStore{Store: memory, failIntent: true}
	service := platform.New(failing)
	toolID, revision := createMutationTool(t, service, "intent_failure")
	arguments := map[string]any{"label": "x"}
	actor := platform.Actor{ID: "root", RequestID: "intent-failure-test"}
	confirmation, err := service.CreateToolTestConfirmation(context.Background(), "prod_acme", toolID, platform.ToolTestConfirmationInput{Revision: revision, Arguments: arguments, TypedToolName: "live.intent_failure", AcknowledgeSideEffects: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := tools.NewRuntime(failing, liveTestResolver{address: net.ParseIP("8.8.8.8")}, liveTestDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"detail":"x"}`))}, nil
	}))
	_, err = service.RunToolTest(context.Background(), runtime, "prod_acme", toolID, platform.ToolTestRunInput{Revision: revision, Arguments: arguments, ConfirmationNonce: confirmation.ConfirmationNonce, IdempotencyKey: "intent-failure-0001"}, actor)
	if err == nil || !strings.Contains(err.Error(), "execution intent unavailable") || calls != 0 {
		t.Fatalf("run error=%v calls=%d", err, calls)
	}
}

func TestToolTestReturnsIndeterminateAndKeepsDurableIntentWhenOutcomeWriteFails(t *testing.T) {
	memory := store.NewMemory()
	failing := &liveTestPersistenceFailureStore{Store: memory, failRun: true}
	service := platform.New(failing)
	toolID, revision := createMutationTool(t, service, "outcome_failure")
	arguments := map[string]any{"label": "x"}
	actor := platform.Actor{ID: "root", RequestID: "outcome-failure-test"}
	confirmation, err := service.CreateToolTestConfirmation(context.Background(), "prod_acme", toolID, platform.ToolTestConfirmationInput{Revision: revision, Arguments: arguments, TypedToolName: "live.outcome_failure", AcknowledgeSideEffects: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := tools.NewRuntime(failing, liveTestResolver{address: net.ParseIP("8.8.8.8")}, liveTestDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"detail":"x"}`))}, nil
	}))
	_, err = service.RunToolTest(context.Background(), runtime, "prod_acme", toolID, platform.ToolTestRunInput{Revision: revision, Arguments: arguments, ConfirmationNonce: confirmation.ConfirmationNonce, IdempotencyKey: "outcome-failure-0001"}, actor)
	if !errors.Is(err, platform.ErrToolTestOutcomeIndeterminate) || calls != 1 {
		t.Fatalf("run error=%v calls=%d", err, calls)
	}
	audits, lookupErr := memory.AuditEvents(context.Background(), "org_acme")
	if lookupErr != nil {
		t.Fatal(lookupErr)
	}
	intent, indeterminate := false, false
	for _, event := range audits {
		intent = intent || event.Action == "tool.test.execution.intent" && event.TargetID != ""
		indeterminate = indeterminate || event.Action == "tool.test.execution.indeterminate" && event.TargetID != "" && event.Current["network_call_performed"] == true
	}
	if !intent || !indeterminate {
		t.Fatalf("durability audits missing: %#v", audits)
	}
}

func TestToolTestRejectsStaleRevisionBeforeNetworkCall(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	toolID, revision := createMutationTool(t, service, "stale")
	actor := platform.Actor{ID: "root", RequestID: "stale-request"}
	arguments := map[string]any{"label": "x"}
	confirmation, err := service.CreateToolTestConfirmation(context.Background(), "prod_acme", toolID, platform.ToolTestConfirmationInput{Revision: revision, Arguments: arguments, TypedToolName: "live.stale", AcknowledgeSideEffects: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := memory.Tool(context.Background(), "prod_acme", toolID)
	if err != nil {
		t.Fatal(err)
	}
	tool.Description = "Changed after confirmation."
	if _, err := memory.UpdateTool(context.Background(), tool, revision); err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := tools.NewRuntime(memory, liveTestResolver{address: net.ParseIP("8.8.8.8")}, liveTestDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("must not call")
	}))
	_, err = service.RunToolTest(context.Background(), runtime, "prod_acme", toolID, platform.ToolTestRunInput{Revision: revision, Arguments: arguments, ConfirmationNonce: confirmation.ConfirmationNonce, IdempotencyKey: "live-test-request-0002"}, actor)
	if !errors.Is(err, platform.ErrToolTestRevisionStale) || calls != 0 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
}

func TestGETToolTestHonorsPolicyRequiredConfirmationBeforeNetworkCall(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	tool, err := service.CreateTool(context.Background(), platform.ToolInput{
		ProductID: "prod_acme", Namespace: "live", Name: "critical_read", Description: "Read one critical upstream record.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"label":{"type":"string"}},"required":["label"]}`),
		OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"},"detail":{"type":"string"}},"required":["ok","detail"]}`),
		Endpoint:     "https://api.example.test/items", HTTPMethod: http.MethodGet, UpstreamAuth: json.RawMessage(`{"type":"none"}`),
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":true,"risk":"critical","idempotency_required":false}`), TimeoutMS: 1000,
	}, platform.Actor{ID: "root", RequestID: "create"})
	if err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{ID: "root", RequestID: "critical-read-test"}
	arguments := map[string]any{"label": "private-value"}
	calls := 0
	runtime := tools.NewRuntime(memory, liveTestResolver{address: net.ParseIP("8.8.8.8")}, liveTestDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"detail":"x"}`))}, nil
	}))
	_, err = service.RunToolTest(context.Background(), runtime, "prod_acme", tool.ID, platform.ToolTestRunInput{Revision: tool.Revision, Arguments: arguments}, actor)
	if !errors.Is(err, platform.ErrToolTestConfirmationInvalid) || calls != 0 {
		t.Fatalf("missing confirmation error=%v calls=%d", err, calls)
	}
	if _, err := service.CreateToolTestConfirmation(context.Background(), "prod_acme", tool.ID, platform.ToolTestConfirmationInput{Revision: tool.Revision, Arguments: arguments, TypedToolName: "live.critical_read", AcknowledgeSideEffects: false}, actor); !errors.Is(err, platform.ErrToolTestConsentInvalid) {
		t.Fatalf("missing acknowledgement error=%v", err)
	}
	confirmation, err := service.CreateToolTestConfirmation(context.Background(), "prod_acme", tool.ID, platform.ToolTestConfirmationInput{Revision: tool.Revision, Arguments: arguments, TypedToolName: "live.critical_read", AcknowledgeSideEffects: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.RunToolTest(context.Background(), runtime, "prod_acme", tool.ID, platform.ToolTestRunInput{Revision: tool.Revision, Arguments: arguments, ConfirmationNonce: confirmation.ConfirmationNonce}, actor)
	if err != nil || calls != 1 || run.Outcome != "success" {
		t.Fatalf("run=%#v error=%v calls=%d", run, err, calls)
	}
	_, err = service.RunToolTest(context.Background(), runtime, "prod_acme", tool.ID, platform.ToolTestRunInput{Revision: tool.Revision, Arguments: arguments, ConfirmationNonce: confirmation.ConfirmationNonce}, actor)
	if !errors.Is(err, platform.ErrToolTestConfirmationReplayed) || calls != 1 {
		t.Fatalf("replay error=%v calls=%d", err, calls)
	}
}

func TestGETToolTestPersistsAuthenticationTypeAndSchemaProjectedEvidence(t *testing.T) {
	const maliciousKey = "Authorization: Bearer live-secret"
	memory := store.NewMemory()
	service := platform.New(memory)
	tool, err := service.CreateTool(context.Background(), platform.ToolInput{
		ProductID: "prod_acme", Namespace: "live", Name: "projected_read", Description: "Read one upstream record.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"label":{"type":"string"}},"required":["label"]}`),
		OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
		Endpoint:     "https://api.example.test/items", HTTPMethod: http.MethodGet, UpstreamAuth: json.RawMessage(`{"type":"none"}`),
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low","idempotency_required":false}`), TimeoutMS: 1000,
	}, platform.Actor{ID: "root", RequestID: "create"})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := tools.NewRuntime(memory, liveTestResolver{address: net.ParseIP("8.8.8.8")}, liveTestDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"Authorization: Bearer live-secret":true}`))}, nil
	}))
	actor := platform.Actor{ID: "root", RequestID: "projected-read-test"}
	preflight, err := service.RunToolTest(context.Background(), runtime, "prod_acme", tool.ID, platform.ToolTestRunInput{Revision: tool.Revision, Arguments: map[string]any{"label": 7}}, actor)
	if err != nil || calls != 0 || preflight.AuthenticationType != "none" || preflight.Phase != "preflight" {
		t.Fatalf("preflight=%#v err=%v calls=%d", preflight, err, calls)
	}
	run, err := service.RunToolTest(context.Background(), runtime, "prod_acme", tool.ID, platform.ToolTestRunInput{Revision: tool.Revision, Arguments: map[string]any{"label": "x"}}, actor)
	if err != nil || calls != 1 || run.Outcome != "failure" || run.Phase != "output_schema" || run.AuthenticationType != "none" || run.ResponseShape == nil {
		t.Fatalf("run=%#v err=%v calls=%d", run, err, calls)
	}
	if _, ok := run.ResponseShape.Properties["[unexpected-property-1]"]; !ok || run.Findings[0].InstancePath != "/[unexpected-property]" || run.Findings[0].SchemaPath != "/additionalProperties" {
		t.Fatalf("projected evidence=%#v findings=%#v", run.ResponseShape, run.Findings)
	}
	stored, err := service.ToolTestRun(context.Background(), "prod_acme", tool.ID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(stored)
	if strings.Contains(string(encoded), maliciousKey) || strings.Contains(string(encoded), "live-secret") || strings.Contains(string(encoded), "Authorization") {
		t.Fatalf("stored evidence leaked an unexpected response key: %s", encoded)
	}
}

func TestToolCreationRejectsDeclaredCredentialResultField(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	_, err := service.CreateTool(context.Background(), platform.ToolInput{
		ProductID: "prod_acme", Namespace: "live", Name: "declared_projected_read", Description: "Read one upstream record.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"label":{"type":"string"}},"required":["label"]}`),
		OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"AuthorizationBearerLiveSecret":{"type":"string"}},"required":["AuthorizationBearerLiveSecret"]}`),
		Endpoint:     "https://api.example.test/items", HTTPMethod: http.MethodGet, UpstreamAuth: json.RawMessage(`{"type":"none"}`),
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low","idempotency_required":false}`), TimeoutMS: 1000,
	}, platform.Actor{ID: "root", RequestID: "create-declared"})
	if err == nil || !strings.Contains(err.Error(), "sensitive_output_field") {
		t.Fatalf("declared credential-shaped result field error = %v", err)
	}
}

func TestGETToolTestControlledStoredReviewFailureClassifiesAuthentication(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	tool, err := service.CreateTool(context.Background(), platform.ToolInput{
		ProductID: "prod_acme", Namespace: "live", Name: "legacy_read", Description: "Read one legacy upstream record.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
		Endpoint:     "https://api.example.test/items", HTTPMethod: http.MethodGet, UpstreamAuth: json.RawMessage(`{"type":"none"}`),
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low","idempotency_required":false}`), TimeoutMS: 1000,
	}, platform.Actor{ID: "root", RequestID: "create"})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := memory.Tool(context.Background(), "prod_acme", tool.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacy.OutputSchema = json.RawMessage(`{"type":"string"}`)
	legacy, err = memory.UpdateTool(context.Background(), legacy, legacy.Revision)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := tools.NewRuntime(memory, liveTestResolver{address: net.ParseIP("8.8.8.8")}, liveTestDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("must not call")
	}))
	run, err := service.RunToolTest(context.Background(), runtime, "prod_acme", legacy.ID, platform.ToolTestRunInput{Revision: legacy.Revision, Arguments: map[string]any{}}, platform.Actor{ID: "root", RequestID: "legacy-read-test"})
	if err != nil || calls != 0 || run.Outcome != "failure" || run.AuthenticationType != "none" || len(run.Findings) != 1 || run.Findings[0].Code != "stored_tool_requires_review" {
		t.Fatalf("run=%#v err=%v calls=%d", run, err, calls)
	}
}

func TestPublishedExactRevisionCanBeLiveTested(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	toolID, revision := createMutationTool(t, service, "published")
	published, err := service.PublishTool(context.Background(), "prod_acme", toolID, revision, platform.Actor{ID: "root", RequestID: "publish"})
	if err != nil {
		t.Fatal(err)
	}
	arguments := map[string]any{"label": "x"}
	actor := platform.Actor{ID: "root", RequestID: "published-test"}
	confirmation, err := service.CreateToolTestConfirmation(context.Background(), "prod_acme", toolID, platform.ToolTestConfirmationInput{Revision: published.Revision, Arguments: arguments, TypedToolName: "live.published", AcknowledgeSideEffects: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	runtime := tools.NewRuntime(memory, liveTestResolver{address: net.ParseIP("8.8.8.8")}, liveTestDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"detail":"x"}`))}, nil
	}))
	run, err := service.RunToolTest(context.Background(), runtime, "prod_acme", toolID, platform.ToolTestRunInput{Revision: published.Revision, Arguments: arguments, ConfirmationNonce: confirmation.ConfirmationNonce, IdempotencyKey: "published-test-0001"}, actor)
	if err != nil || run.Outcome != "success" || run.ToolRevision != published.Revision {
		t.Fatalf("run=%#v err=%v", run, err)
	}
}

func TestPostCallRevisionChangeDoesNotLoseHistoricalEvidence(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	toolID, revision := createMutationTool(t, service, "during_call")
	arguments := map[string]any{"label": "x"}
	actor := platform.Actor{ID: "root", RequestID: "during-call-test"}
	confirmation, err := service.CreateToolTestConfirmation(context.Background(), "prod_acme", toolID, platform.ToolTestConfirmationInput{Revision: revision, Arguments: arguments, TypedToolName: "live.during_call", AcknowledgeSideEffects: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	runtime := tools.NewRuntime(memory, liveTestResolver{address: net.ParseIP("8.8.8.8")}, liveTestDoer(func(*http.Request) (*http.Response, error) {
		current, lookupErr := memory.Tool(context.Background(), "prod_acme", toolID)
		if lookupErr != nil {
			t.Fatal(lookupErr)
		}
		current.Description = "Changed while the exact prior revision was executing."
		if _, updateErr := memory.UpdateTool(context.Background(), current, revision); updateErr != nil {
			t.Fatal(updateErr)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"detail":"x"}`))}, nil
	}))
	run, err := service.RunToolTest(context.Background(), runtime, "prod_acme", toolID, platform.ToolTestRunInput{Revision: revision, Arguments: arguments, ConfirmationNonce: confirmation.ConfirmationNonce, IdempotencyKey: "during-call-test-01"}, actor)
	if err != nil || run.ToolRevision != revision {
		t.Fatalf("run=%#v err=%v", run, err)
	}
	stored, err := service.ToolTestRun(context.Background(), "prod_acme", toolID, run.ID)
	if err != nil || stored.ToolRevision != revision {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
}

func TestToolTestRunListingIsBoundedToNewestHundred(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	toolID, revision := createMutationTool(t, service, "bounded")
	now := time.Now().UTC()
	for index := 0; index < 101; index++ {
		createdAt := now.Add(time.Duration(index) * time.Second)
		if err := memory.AppendToolTestRun(context.Background(), model.ToolTestRun{ID: fmt.Sprintf("run-%03d", index), OrganisationID: "org_acme", ProductID: "prod_acme", ToolID: toolID, ToolRevision: revision, ToolName: "live.bounded", ActorID: "root", ArgumentHash: make([]byte, 32), Method: http.MethodPost, AuthenticationType: "none", Outcome: "success", Phase: "success", RequestShape: model.JSONShape{Type: "object"}, Findings: []model.ToolTestFinding{}, ExpiresAt: now.Add(48 * time.Hour), CreatedAt: createdAt}); err != nil {
			t.Fatal(err)
		}
	}
	values, err := service.ToolTestRuns(context.Background(), "prod_acme", toolID)
	if err != nil || len(values) != 100 || values[0].ID != "run-100" || values[99].ID != "run-001" {
		t.Fatalf("runs=%d first=%q last=%q err=%v", len(values), values[0].ID, values[len(values)-1].ID, err)
	}
}

func TestToolTestOperationsLazilyRunBoundedRetentionCleanup(t *testing.T) {
	memory := store.NewMemory()
	tracking := &liveTestCleanupTrackingStore{Store: memory}
	service := platform.New(tracking)
	toolID, _ := createMutationTool(t, service, "retention_cleanup")
	if _, err := service.ToolTestRuns(context.Background(), "prod_acme", toolID); err != nil {
		t.Fatal(err)
	}
	if tracking.calls != 1 || tracking.limit != 100 {
		t.Fatalf("cleanup calls=%d limit=%d", tracking.calls, tracking.limit)
	}
}
