package platform_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type toolTestAnalysisDoer struct {
	calls   int
	request []byte
	result  string
	status  int
	cancel  context.CancelFunc
}

func (d *toolTestAnalysisDoer) Do(request *http.Request) (*http.Response, error) {
	d.calls++
	d.request, _ = io.ReadAll(request.Body)
	if d.cancel != nil {
		d.cancel()
	}
	status := d.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(d.result)), Request: request}, nil
}

type toolTestAnalysisAuditStore struct {
	store.Store
	analysisContextErrors []error
	analysisDeadlines     []time.Time
	failIntent            bool
	afterIntent           func(context.Context) error
}

func (s *toolTestAnalysisAuditStore) AppendAudit(ctx context.Context, event model.AuditEvent) error {
	if event.Action == "tool.test.analysis.intent" && s.failIntent {
		return errors.New("intent audit unavailable")
	}
	if event.Action == "tool.test.analysis" {
		s.analysisContextErrors = append(s.analysisContextErrors, ctx.Err())
		deadline, _ := ctx.Deadline()
		s.analysisDeadlines = append(s.analysisDeadlines, deadline)
	}
	if err := s.Store.AppendAudit(ctx, event); err != nil {
		return err
	}
	if event.Action == "tool.test.analysis.intent" && s.afterIntent != nil {
		callback := s.afterIntent
		s.afterIntent = nil
		return callback(ctx)
	}
	return nil
}

type toolTestAnalysisFixture struct {
	service *platform.Service
	memory  *store.Memory
	doer    *toolTestAnalysisDoer
	audits  *toolTestAnalysisAuditStore
	tool    model.Tool
	run     model.ToolTestRun
}

func newToolTestAnalysisFixture(t *testing.T, published bool) toolTestAnalysisFixture {
	t.Helper()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x72}, 32))
	if err != nil {
		t.Fatal(err)
	}
	doer := &toolTestAnalysisDoer{}
	audits := &toolTestAnalysisAuditStore{Store: memory}
	service := platform.NewWithVaultAndProductBuilderDoer(audits, vault, doer)
	actor := platform.Actor{ID: "fixture-admin", RequestID: "fixture-request"}
	connection, err := service.SaveAIProviderConnection(ctx, platform.AIProviderConnectionInput{
		OrganisationID: "org_acme", DeploymentID: "prod_acme", Provider: "openai-compatible", Endpoint: "https://analysis.example.test", Credential: "provider-secret-not-for-prompt", Enabled: true,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveAIWorkloadProfile(ctx, platform.AIWorkloadProfileInput{
		OrganisationID: "org_acme", ProductID: "prod_acme", Workload: "analysis", ProviderConnectionID: connection.ID,
		Model: "analysis-model", MaxInputTokens: 8192, MaxOutputTokens: 4096, DailyTokenBudget: 20000, Enabled: true,
	}, actor); err != nil {
		t.Fatal(err)
	}
	requestExample := json.RawMessage(`{"item_id":"private-request-example"}`)
	responseExample := json.RawMessage(`{"found":true,"label":"private-response-example"}`)
	tool, err := service.CreateTool(ctx, platform.ToolInput{
		ProductID: "prod_acme", Namespace: "catalog", Name: "get_item", Description: "Get one catalog item.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"item_id":{"type":"string"},"note":{"type":"string"}},"required":["item_id"]}`),
		OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"found":{"type":"boolean"},"label":{"type":"string"}},"required":["found"]}`),
		Endpoint:     "https://api.vendor.example/v1/items/{item_id}", HTTPMethod: http.MethodGet,
		UpstreamAuth: json.RawMessage(`{"type":"none"}`), RequestMapping: json.RawMessage(`{"parameter_locations":{"item_id":"path"}}`), ResponseMapping: json.RawMessage(`{}`),
		RequestExample: requestExample, ResponseExample: responseExample,
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low","idempotency_required":false}`), TimeoutMS: 1500,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if published {
		tool, err = service.PublishTool(ctx, tool.ProductID, tool.ID, tool.Revision, actor)
		if err != nil {
			t.Fatal(err)
		}
	}
	responseShape := model.JSONShape{Type: "object", Properties: map[string]model.JSONShape{"found": {Type: "boolean"}, "label": {Type: "string"}}}
	argumentHash := sha256.Sum256([]byte("discarded-private-arguments"))
	run := model.ToolTestRun{
		ID: "run-private-internal-id", OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, ToolID: tool.ID, ToolRevision: tool.Revision,
		ToolName: tool.Namespace + "." + tool.Name, ActorID: "private-actor-id", RequestID: "private-request-id", ArgumentHash: argumentHash[:],
		Method: tool.HTTPMethod, AuthenticationType: "none", Outcome: "success", Phase: "complete", NetworkCallPerformed: true,
		UpstreamStatusCode: http.StatusOK, ResponseBytes: 58, DurationMS: 24,
		RequestShape:  model.JSONShape{Type: "object", Properties: map[string]model.JSONShape{"item_id": {Type: "string"}, "password": {Type: "string"}}},
		ResponseShape: &responseShape, Findings: []model.ToolTestFinding{{Phase: "response", Code: "response_shape_observed", Message: "The upstream response matched the declared object boundary."}},
		CreatedAt: time.Now().UTC().Add(-time.Minute), ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := memory.AppendToolTestRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	return toolTestAnalysisFixture{service: service, memory: memory, doer: doer, audits: audits, tool: tool, run: run}
}

func toolTestAnalysisProviderResult(t *testing.T, tool model.Tool) string {
	t.Helper()
	proposal, err := json.Marshal(map[string]any{
		"description": "Get one catalog item and report its response shape.", "http_method": "GET", "timeout_ms": 1500,
		"input_schema": json.RawMessage(tool.InputSchema), "output_schema": json.RawMessage(tool.OutputSchema),
		"request_mapping":      map[string]any{"parameter_locations": map[string]string{"item_id": "path"}},
		"response_mapping":     map[string]any{},
		"authorization_policy": map[string]any{"required_grants": []string{}, "confirmation_required": false, "risk": "low", "idempotency_required": false},
	})
	if err != nil {
		t.Fatal(err)
	}
	structured, err := json.Marshal(map[string]any{
		"reply":         "The sanitized evidence supports a clearer description; review the candidate before editing.",
		"findings":      []map[string]any{{"level": "info", "code": "response_shape_confirmed", "field": "output_schema", "message": "The retained response shape is consistent with the declared object contract.", "suggestion": "Keep the declared response properties explicit."}},
		"proposal_json": string(proposal),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(structured)}}}, "usage": map[string]any{"total_tokens": 40}})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func toolTestAnalysisAdvisoryProviderResult(t *testing.T) string {
	t.Helper()
	structured, err := json.Marshal(map[string]any{
		"reply":         "Review the sanitized structural evidence.",
		"findings":      []any{},
		"proposal_json": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(structured)}}}, "usage": map[string]any{"total_tokens": 20}})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func toolTestAnalysisProviderUserPayload(t *testing.T, request []byte) map[string]any {
	t.Helper()
	var envelope struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(request, &envelope); err != nil {
		t.Fatalf("decode provider request: %v\n%s", err, request)
	}
	for index := len(envelope.Messages) - 1; index >= 0; index-- {
		if envelope.Messages[index].Role != "user" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(envelope.Messages[index].Content), &payload); err != nil {
			t.Fatalf("decode provider user payload: %v\n%s", err, envelope.Messages[index].Content)
		}
		return payload
	}
	t.Fatal("provider request omitted a user message")
	return nil
}

func updateToolTestAnalysisFixture(t *testing.T, fixture *toolTestAnalysisFixture, mutate func(*model.Tool)) {
	t.Helper()
	updated := fixture.tool
	mutate(&updated)
	var err error
	updated, err = fixture.memory.UpdateTool(context.Background(), updated, fixture.tool.Revision)
	if err != nil {
		t.Fatal(err)
	}
	run := fixture.run
	run.ID += "-updated"
	run.ToolRevision = updated.Revision
	run.CreatedAt = time.Now().UTC().Add(-time.Minute)
	run.ExpiresAt = time.Now().UTC().Add(time.Hour)
	if err := fixture.memory.AppendToolTestRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	fixture.tool, fixture.run = updated, run
}

func TestToolTestAnalysisEvidenceHashUsesDurableTimestampPrecision(t *testing.T) {
	t.Parallel()
	run := model.ToolTestRun{
		ToolRevision: 1,
		Method:       "GET", AuthenticationType: "none", Outcome: "failure", Phase: "output_schema",
		NetworkCallPerformed: true, UpstreamStatusCode: http.StatusOK, ResponseBytes: 16, DurationMS: 5,
		RequestShape: model.JSONShape{Type: "object"}, Findings: []model.ToolTestFinding{},
		CreatedAt: time.Date(2026, 8, 24, 10, 47, 49, 123456789, time.UTC),
		ExpiresAt: time.Date(2026, 8, 25, 10, 47, 49, 987654321, time.UTC),
	}
	durable := run
	durable.CreatedAt = durable.CreatedAt.Truncate(time.Microsecond)
	durable.ExpiresAt = durable.ExpiresAt.Truncate(time.Microsecond)
	if got, want := platform.ToolTestAnalysisEvidenceHash(run), platform.ToolTestAnalysisEvidenceHash(durable); got != want {
		t.Fatalf("fresh and durable evidence bindings differ: fresh=%s durable=%s", got, want)
	}
	durable.CreatedAt = durable.CreatedAt.Add(time.Microsecond)
	if platform.ToolTestAnalysisEvidenceHash(run) == platform.ToolTestAnalysisEvidenceHash(durable) {
		t.Fatal("evidence binding ignored a durable timestamp change")
	}
}

func TestAnalyseToolTestRunSendsOnlyConsentedSanitizedEvidenceAndPreservesExamples(t *testing.T) {
	t.Parallel()
	fixture := newToolTestAnalysisFixture(t, true)
	fixture.doer.result = toolTestAnalysisProviderResult(t, fixture.tool)
	hash := platform.ToolTestAnalysisEvidenceHash(fixture.run)
	history := []platform.ToolTestAnalysisMessage{{Role: "user", Content: "Does the structural response match?"}, {Role: "assistant", Content: "Only the sanitized shape can answer that."}}
	result, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, platform.ToolTestAnalysisInput{
		Revision: fixture.tool.Revision, EvidenceHash: hash, ConsentToSend: true, Question: "Should the non-secret contract change?", History: history,
	}, platform.Actor{ID: "analysis-admin-private-id", RequestID: "analysis-request-private-id"})
	if err != nil {
		t.Fatal(err)
	}
	if fixture.doer.calls != 1 || result.EvidenceHash != hash || result.ToolRevision != fixture.tool.Revision || !result.Advisory || result.ProviderOutcome != "succeeded" {
		t.Fatalf("analysis result = %#v calls=%d", result, fixture.doer.calls)
	}
	if result.Proposal == nil || result.Proposal.BaseToolID != fixture.tool.ID || result.Proposal.BaseRevision != fixture.tool.Revision || !result.Proposal.RequiresClone || result.Proposal.BaseFingerprint == "" || len(result.Proposal.Changes) != 1 || result.Proposal.Changes[0].Field != "description" {
		t.Fatalf("proposal was not an exact-revision reviewable diff: %#v", result.Proposal)
	}
	if !reflect.DeepEqual(result.Proposal.Draft.RequestExample, map[string]any{"item_id": "private-request-example"}) || !reflect.DeepEqual(result.Proposal.Draft.ResponseExample, map[string]any{"found": true, "label": "private-response-example"}) {
		t.Fatalf("omitted examples were not preserved: request=%#v response=%#v", result.Proposal.Draft.RequestExample, result.Proposal.Draft.ResponseExample)
	}
	request := string(fixture.doer.request)
	for _, expected := range []string{"Should the non-secret contract change?", "Does the structural response match?", "request_shape", "response_shape", "path_parameter_names"} {
		if !strings.Contains(request, expected) {
			t.Fatalf("provider request omitted %q: %s", expected, request)
		}
	}
	for _, forbidden := range []string{
		"private-request-example", "private-response-example", "provider-secret-not-for-prompt", "api.vendor.example", fixture.tool.ID,
		fixture.run.ID, "private-actor-id", "private-request-id", "analysis-admin-private-id", "analysis-request-private-id",
		"credential_present", "request_example", "response_example", "header_name", "token_url", "evidence_hash", hash,
	} {
		if strings.Contains(request, forbidden) {
			t.Fatalf("provider request leaked %q: %s", forbidden, request)
		}
	}
	audits, err := fixture.memory.AuditEvents(context.Background(), fixture.tool.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	var analysisAudit, intentAudit *model.AuditEvent
	for index := range audits {
		switch audits[index].Action {
		case "tool.test.analysis":
			analysisAudit = &audits[index]
		case "tool.test.analysis.intent":
			intentAudit = &audits[index]
		}
	}
	if intentAudit == nil || intentAudit.Current["provider_connection_id"] == "" || intentAudit.Current["provider_connection_revision"] == nil || intentAudit.Current["workload_profile_id"] == "" || intentAudit.Current["workload_profile_revision"] == nil {
		t.Fatalf("consent intent did not pin exact workload and provider revisions: %#v", intentAudit)
	}
	if analysisAudit == nil || len(analysisAudit.Current) != 4 || analysisAudit.Current["consent"] != true || analysisAudit.Current["provider_outcome"] != "succeeded" || analysisAudit.Current["finding_count"] != 1 || analysisAudit.Current["change_count"] != 1 {
		t.Fatalf("analysis audit must contain counts and outcome only: %#v", analysisAudit)
	}
	encodedAudit, _ := json.Marshal(analysisAudit)
	if bytes.Contains(encodedAudit, []byte("Should the non-secret contract change?")) || bytes.Contains(encodedAudit, []byte(hash)) || bytes.Contains(encodedAudit, []byte("private-request-example")) {
		t.Fatalf("analysis audit retained evidence or conversation: %s", encodedAudit)
	}
}

func TestAnalyseToolTestRunRejectsMissingConsentMismatchedEvidenceUnsafeChatAndScope(t *testing.T) {
	t.Parallel()
	fixture := newToolTestAnalysisFixture(t, false)
	fixture.doer.result = toolTestAnalysisProviderResult(t, fixture.tool)
	hash := platform.ToolTestAnalysisEvidenceHash(fixture.run)
	base := platform.ToolTestAnalysisInput{Revision: fixture.tool.Revision, EvidenceHash: hash, Question: "What does the sanitized evidence show?"}
	if _, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, base, platform.Actor{ID: "admin"}); !errors.Is(err, platform.ErrToolTestAnalysisConsentRequired) {
		t.Fatalf("missing consent error = %v", err)
	}
	base.ConsentToSend = true
	base.EvidenceHash = "sha256:" + strings.Repeat("0", 64)
	if _, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, base, platform.Actor{ID: "admin"}); !errors.Is(err, platform.ErrToolTestAnalysisEvidenceMismatch) {
		t.Fatalf("mismatched evidence error = %v", err)
	}
	base.EvidenceHash = hash
	base.Question = "Inspect https://api.example.test/items?access_token=raw-query-value"
	if _, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, base, platform.Actor{ID: "admin"}); !errors.Is(err, platform.ErrToolTestAnalysisInvalidInput) {
		t.Fatalf("raw query chat error = %v", err)
	}
	base.Question = "Inspect https://api.example.test/private/path"
	if _, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, base, platform.Actor{ID: "admin"}); !errors.Is(err, platform.ErrToolTestAnalysisInvalidInput) {
		t.Fatalf("raw destination chat error = %v", err)
	}
	base.Question = "Inspect internal run 123e4567-e89b-42d3-a456-426614174000"
	if _, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, base, platform.Actor{ID: "admin"}); !errors.Is(err, platform.ErrToolTestAnalysisInvalidInput) {
		t.Fatalf("internal ID chat error = %v", err)
	}
	base.Question = "Reuse ttc_abcdefghijklmnopqrstuvwxyz0123456789"
	if _, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, base, platform.Actor{ID: "admin"}); !errors.Is(err, platform.ErrToolTestAnalysisInvalidInput) {
		t.Fatalf("nonce chat error = %v", err)
	}
	base.Question = "Use Authorization: Bearer super-secret-token-value"
	if _, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, base, platform.Actor{ID: "admin"}); !errors.Is(err, platform.ErrToolTestAnalysisInvalidInput) {
		t.Fatalf("credential chat error = %v", err)
	}
	base.Question = "What does the sanitized evidence show?"
	base.Revision++
	if _, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, base, platform.Actor{ID: "admin"}); !errors.Is(err, platform.ErrToolTestRevisionStale) {
		t.Fatalf("stale revision error = %v", err)
	}
	base.Revision = fixture.tool.Revision
	other, err := fixture.service.CreateTool(context.Background(), platform.ToolInput{
		ProductID: fixture.tool.ProductID, Namespace: "catalog", Name: "other_item", Description: "Get another item.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		Endpoint: "https://other.vendor.example/items", HTTPMethod: http.MethodGet, UpstreamAuth: json.RawMessage(`{"type":"none"}`), AuthorizationPolicy: json.RawMessage(`{"required_grants":[]}`), TimeoutMS: 1000,
	}, platform.Actor{ID: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, other.ID, fixture.run.ID, base, platform.Actor{ID: "admin"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-tool run error = %v", err)
	}
	expired := fixture.run
	expired.ID = "expired-run-private-id"
	expired.CreatedAt = time.Now().UTC().Add(-2 * time.Hour)
	expired.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	if err := fixture.memory.AppendToolTestRun(context.Background(), expired); err != nil {
		t.Fatal(err)
	}
	base.EvidenceHash = platform.ToolTestAnalysisEvidenceHash(expired)
	if _, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, expired.ID, base, platform.Actor{ID: "admin"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expired run error = %v", err)
	}
	if fixture.doer.calls != 0 {
		t.Fatalf("provider was called for a rejected analysis: %d", fixture.doer.calls)
	}
}

func TestAnalyseToolTestRunDoesNotSendConsentedEvidenceToBackupProvider(t *testing.T) {
	t.Parallel()
	fixture := newToolTestAnalysisFixture(t, false)
	if _, err := fixture.service.SaveAIProviderConnection(context.Background(), platform.AIProviderConnectionInput{
		OrganisationID: fixture.tool.OrganisationID, DeploymentID: fixture.tool.ProductID, Provider: "deepseek",
		Credential: "backup-provider-secret", Enabled: true, IsBackup: true,
		BackupModels: map[string]string{"analysis": "backup-analysis-model"},
	}, platform.Actor{ID: "admin"}); err != nil {
		t.Fatal(err)
	}
	fixture.doer.status = http.StatusServiceUnavailable
	fixture.doer.result = `{"error":{"message":"primary unavailable"}}`
	_, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, platform.ToolTestAnalysisInput{
		Revision: fixture.tool.Revision, EvidenceHash: platform.ToolTestAnalysisEvidenceHash(fixture.run), ConsentToSend: true,
		Question: "What does the sanitized evidence show?",
	}, platform.Actor{ID: "admin"})
	if err == nil {
		t.Fatal("retryable primary failure unexpectedly succeeded")
	}
	if fixture.doer.calls != 1 {
		t.Fatalf("consented evidence reached a fallback provider: calls=%d", fixture.doer.calls)
	}
}

func TestAnalyseToolTestRunFailsClosedWhenConsentIntentCannotBePersisted(t *testing.T) {
	t.Parallel()
	fixture := newToolTestAnalysisFixture(t, false)
	fixture.doer.result = toolTestAnalysisAdvisoryProviderResult(t)
	fixture.audits.failIntent = true
	_, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, platform.ToolTestAnalysisInput{
		Revision: fixture.tool.Revision, EvidenceHash: platform.ToolTestAnalysisEvidenceHash(fixture.run), ConsentToSend: true,
		Question: "What does the sanitized evidence show?",
	}, platform.Actor{ID: "admin"})
	if err == nil || !strings.Contains(err.Error(), "intent audit unavailable") {
		t.Fatalf("intent persistence error = %v", err)
	}
	if fixture.doer.calls != 0 {
		t.Fatalf("provider was called without a durable consent intent: %d", fixture.doer.calls)
	}
}

func TestAnalyseToolTestRunFailsClosedWhenConsentedAITargetChanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *toolTestAnalysisFixture) error
	}{
		{
			name: "provider connection revision",
			mutate: func(ctx context.Context, fixture *toolTestAnalysisFixture) error {
				connections, err := fixture.memory.AIProviderConnections(ctx, fixture.tool.ProductID)
				if err != nil {
					return err
				}
				connection := connections[0]
				_, err = fixture.service.SaveAIProviderConnection(ctx, platform.AIProviderConnectionInput{
					OrganisationID: fixture.tool.OrganisationID, DeploymentID: fixture.tool.ProductID,
					Provider: connection.Provider, Endpoint: "https://changed-analysis.example.test", Enabled: true,
					Revision: connection.Revision,
				}, platform.Actor{ID: "concurrent-admin"})
				return err
			},
		},
		{
			name: "workload profile revision",
			mutate: func(ctx context.Context, fixture *toolTestAnalysisFixture) error {
				profile, err := fixture.memory.AIWorkloadProfile(ctx, fixture.tool.ProductID, "analysis")
				if err != nil {
					return err
				}
				_, err = fixture.service.SaveAIWorkloadProfile(ctx, platform.AIWorkloadProfileInput{
					OrganisationID: fixture.tool.OrganisationID, ProductID: fixture.tool.ProductID, Workload: profile.Workload,
					ProviderConnectionID: profile.ProviderConnectionID, Model: "changed-analysis-model",
					MaxInputTokens: profile.MaxInputTokens, MaxOutputTokens: profile.MaxOutputTokens,
					DailyTokenBudget: profile.DailyTokenBudget, Enabled: true, Revision: profile.Revision,
				}, platform.Actor{ID: "concurrent-admin"})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newToolTestAnalysisFixture(t, false)
			fixture.doer.result = toolTestAnalysisAdvisoryProviderResult(t)
			fixture.audits.afterIntent = func(ctx context.Context) error {
				return test.mutate(ctx, &fixture)
			}
			_, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, platform.ToolTestAnalysisInput{
				Revision: fixture.tool.Revision, EvidenceHash: platform.ToolTestAnalysisEvidenceHash(fixture.run), ConsentToSend: true,
				Question: "What does the sanitized evidence show?",
			}, platform.Actor{ID: "admin"})
			if !errors.Is(err, platform.ErrAIUnavailable) {
				t.Fatalf("changed consented target error = %v", err)
			}
			if fixture.doer.calls != 0 {
				t.Fatalf("provider was called after the consented target changed: %d", fixture.doer.calls)
			}
		})
	}
}

func TestAnalyseToolTestRunRejectsUnsafeProviderProposal(t *testing.T) {
	t.Parallel()
	fixture := newToolTestAnalysisFixture(t, false)
	proposal, err := json.Marshal(map[string]any{
		"description": "Use Authorization: Bearer provider-invented-secret-value", "http_method": "GET", "timeout_ms": 1500,
		"input_schema": json.RawMessage(fixture.tool.InputSchema), "output_schema": json.RawMessage(fixture.tool.OutputSchema),
		"request_mapping":      map[string]any{"parameter_locations": map[string]string{"item_id": "path"}},
		"response_mapping":     map[string]any{},
		"authorization_policy": map[string]any{"required_grants": []string{}, "confirmation_required": false, "risk": "low", "idempotency_required": false},
	})
	if err != nil {
		t.Fatal(err)
	}
	structured, _ := json.Marshal(map[string]any{"reply": "Review the proposed description.", "findings": []any{}, "proposal_json": string(proposal)})
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(structured)}}}, "usage": map[string]any{"total_tokens": 20}})
	fixture.doer.result = string(body)
	result, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, platform.ToolTestAnalysisInput{
		Revision: fixture.tool.Revision, EvidenceHash: platform.ToolTestAnalysisEvidenceHash(fixture.run), ConsentToSend: true, Question: "Should the non-secret contract change?",
	}, platform.Actor{ID: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderOutcome != "unusable" || result.Proposal != nil || len(result.Findings) != 1 || result.Findings[0].Code != "ai_proposal_rejected" {
		t.Fatalf("unsafe provider proposal was not discarded: %#v", result)
	}
}

func TestAnalyseToolTestRunProjectsStoredContractToStructuralSchema(t *testing.T) {
	t.Parallel()
	fixture := newToolTestAnalysisFixture(t, false)
	updateToolTestAnalysisFixture(t, &fixture, func(tool *model.Tool) {
		tool.Description = "Authorization: Bearer stored-description-secret-value"
		tool.InputSchema = json.RawMessage(`{
			"type":"object",
			"title":"INPUT_TITLE_PRIVATE_SENTINEL",
			"description":"Authorization: Bearer input-schema-description-secret",
			"additionalProperties":false,
			"properties":{
				"item_id":{"type":"string","title":"ITEM_TITLE_PRIVATE_SENTINEL","description":"sk_live_property_description_secret","enum":["sk_live_enum_secret_value","safe"]},
				"password":{"type":"string"},
				"AuthorizationBearerLiveSecret":{"type":"string"}
			},
			"required":["item_id"],
			"examples":[{"item_id":"INPUT_EXAMPLE_PRIVATE_SENTINEL"}],
			"default":{"item_id":"INPUT_DEFAULT_PRIVATE_SENTINEL"},
			"$comment":"INPUT_COMMENT_PRIVATE_SENTINEL"
		}`)
		tool.OutputSchema = json.RawMessage(`{
			"type":"object",
			"title":"OUTPUT_TITLE_PRIVATE_SENTINEL",
			"description":"OUTPUT_DESCRIPTION_PRIVATE_SENTINEL",
			"additionalProperties":false,
			"properties":{"found":{"type":"boolean","const":true},"label":{"type":"string","pattern":"OUTPUT_PATTERN_PRIVATE_SENTINEL"}},
			"required":["found"]
		}`)
	})
	fixture.doer.result = toolTestAnalysisAdvisoryProviderResult(t)
	if _, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, platform.ToolTestAnalysisInput{
		Revision: fixture.tool.Revision, EvidenceHash: platform.ToolTestAnalysisEvidenceHash(fixture.run), ConsentToSend: true, Question: "What does the structural contract show?",
	}, platform.Actor{ID: "admin"}); err != nil {
		t.Fatal(err)
	}
	payload := toolTestAnalysisProviderUserPayload(t, fixture.doer.request)
	contract, ok := payload["non_secret_contract"].(map[string]any)
	if !ok {
		t.Fatalf("provider contract = %#v", payload["non_secret_contract"])
	}
	encodedContract, _ := json.Marshal(contract)
	for _, forbidden := range []string{
		"stored-description-secret-value", "INPUT_TITLE_PRIVATE_SENTINEL", "input-schema-description-secret",
		"ITEM_TITLE_PRIVATE_SENTINEL", "sk_live_property_description_secret", "sk_live_enum_secret_value",
		"AuthorizationBearerLiveSecret",
		"INPUT_EXAMPLE_PRIVATE_SENTINEL", "INPUT_DEFAULT_PRIVATE_SENTINEL", "INPUT_COMMENT_PRIVATE_SENTINEL",
		"OUTPUT_TITLE_PRIVATE_SENTINEL", "OUTPUT_DESCRIPTION_PRIVATE_SENTINEL", "OUTPUT_PATTERN_PRIVATE_SENTINEL",
	} {
		if bytes.Contains(encodedContract, []byte(forbidden)) || bytes.Contains(fixture.doer.request, []byte(forbidden)) {
			t.Fatalf("provider request leaked stored schema/description literal %q: %s", forbidden, fixture.doer.request)
		}
	}
	if _, exists := contract["description"]; exists {
		t.Fatalf("stored tool description crossed provider boundary: %s", encodedContract)
	}
	for _, forbiddenKeyword := range []string{`"title"`, `"description"`, `"enum"`, `"const"`, `"pattern"`, `"examples"`, `"default"`, `"$comment"`} {
		if bytes.Contains(encodedContract, []byte(forbiddenKeyword)) {
			t.Fatalf("provider contract retained literal/annotation keyword %s: %s", forbiddenKeyword, encodedContract)
		}
	}
	for _, expectedMarker := range []string{`"x-dokosoko-enum-value-count":2`, `"x-dokosoko-const-present":true`} {
		if !bytes.Contains(encodedContract, []byte(expectedMarker)) {
			t.Fatalf("provider contract omitted value-free literal-constraint marker %s: %s", expectedMarker, encodedContract)
		}
	}
	for _, expected := range []string{`"properties"`, `"item_id"`, `"found"`, `"required"`, `"additionalProperties"`} {
		if !bytes.Contains(encodedContract, []byte(expected)) {
			t.Fatalf("provider contract omitted safe structural keyword/name %s: %s", expected, encodedContract)
		}
	}
}

func TestAnalyseToolTestRunProjectsEvidenceKeysAndDropsDiagnosticPaths(t *testing.T) {
	t.Parallel()
	fixture := newToolTestAnalysisFixture(t, false)
	run := fixture.run
	run.ID = "run-malicious-upstream-key-private-id"
	run.ResponseShape = &model.JSONShape{Type: "object", Properties: map[string]model.JSONShape{
		"found": {Type: "boolean"},
		"Authorization: Bearer live-response-secret": {Type: "boolean"},
	}}
	run.Findings = []model.ToolTestFinding{{
		Phase: "output_schema", Code: "output_schema_mismatch", Message: "Authorization: Bearer live-response-secret",
		InstancePath: "/Authorization: Bearer live-response-secret", SchemaPath: "/properties/Authorization: Bearer live-response-secret/type",
	}}
	if err := fixture.memory.AppendToolTestRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	fixture.run = run
	fixture.doer.result = toolTestAnalysisAdvisoryProviderResult(t)
	if _, err := fixture.service.AnalyseToolTestRun(context.Background(), fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, platform.ToolTestAnalysisInput{
		Revision: fixture.tool.Revision, EvidenceHash: platform.ToolTestAnalysisEvidenceHash(fixture.run), ConsentToSend: true, Question: "What does the structural evidence show?",
	}, platform.Actor{ID: "admin"}); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(fixture.doer.request, []byte("live-response-secret")) || bytes.Contains(fixture.doer.request, []byte("instance_path")) || bytes.Contains(fixture.doer.request, []byte("schema_path")) {
		t.Fatalf("provider request leaked an upstream property name or diagnostic path: %s", fixture.doer.request)
	}
	payload := toolTestAnalysisProviderUserPayload(t, fixture.doer.request)
	evidence, ok := payload["sanitized_evidence"].(map[string]any)
	if !ok {
		t.Fatalf("provider evidence = %#v", payload["sanitized_evidence"])
	}
	encodedEvidence, _ := json.Marshal(evidence)
	if !bytes.Contains(encodedEvidence, []byte(`"found"`)) || !bytes.Contains(encodedEvidence, []byte(`"output_schema_mismatch"`)) || !bytes.Contains(encodedEvidence, []byte(`"truncated":true`)) {
		t.Fatalf("provider evidence lost its safe declared shape/finding boundary: %s", encodedEvidence)
	}
}

func TestAnalyseToolTestRunAuditSurvivesRequestCancellation(t *testing.T) {
	t.Parallel()
	fixture := newToolTestAnalysisFixture(t, false)
	fixture.doer.result = toolTestAnalysisProviderResult(t, fixture.tool)
	ctx, cancel := context.WithCancel(context.Background())
	fixture.doer.cancel = cancel
	_, _ = fixture.service.AnalyseToolTestRun(ctx, fixture.tool.ProductID, fixture.tool.ID, fixture.run.ID, platform.ToolTestAnalysisInput{
		Revision: fixture.tool.Revision, EvidenceHash: platform.ToolTestAnalysisEvidenceHash(fixture.run), ConsentToSend: true, Question: "What does the sanitized evidence show?",
	}, platform.Actor{ID: "admin"})
	if len(fixture.audits.analysisContextErrors) != 1 || fixture.audits.analysisContextErrors[0] != nil {
		t.Fatalf("analysis audit inherited request cancellation: %#v", fixture.audits.analysisContextErrors)
	}
	deadline := fixture.audits.analysisDeadlines[0]
	if deadline.IsZero() || time.Until(deadline) <= 0 || time.Until(deadline) > 2*time.Second {
		t.Fatalf("analysis audit deadline = %v", deadline)
	}
}
