package reporting

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type resolverFunc func(context.Context, string, string) ([]net.IP, error)

func (f resolverFunc) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	return f(ctx, network, host)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func newReportingService(t *testing.T) (*Service, *store.Memory) {
	t.Helper()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x52}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return New(memory, vault), memory
}

func submitContext() SubmitContext {
	return SubmitContext{
		Principal:      identity.Principal{Issuer: "https://identity.vendor.example", Subject: "user-123", Email: "developer@example.com", DisplayName: "Developer", VendorOrganisation: "customer-1", InstallationID: "install-1"},
		ActorPseudonym: "actor-pseudonym",
		Product:        ProductContext{ProductID: "prod_acme", ProductName: "Acme Platform", ProductVersionID: "version-1", ProductVersion: "2026.8", ManifestHash: "sha256:test", CatalogRevision: 4, EnvironmentID: "env_prod", InstallationID: "install-1"},
		RequestID:      "req-test",
	}
}

func TestSubmissionIsEncryptedHeldAndIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService(t)
	config, err := service.Configure(ctx, "prod_acme", ConfigInput{BugReportsEnabled: true, FeedbackEnabled: true, RetentionDays: 30}, "root-test", "req-config")
	if err != nil {
		t.Fatal(err)
	}
	if config.BugHookCredentialID != "" || config.FeedbackHookCredentialID != "" {
		t.Fatal("holding-only configuration unexpectedly created a hook credential")
	}
	input := BugInput{Summary: "Connector returns the wrong status", Description: "The response status is inconsistent with the documented connector behavior.", ReproductionSteps: []string{"Call support.example with a valid request"}, ExpectedBehavior: "Return accepted", ActualBehavior: "Return rejected", RelatedTool: "projects.create", IdempotencyKey: "bug-report-idempotency-0001"}
	first, err := service.SubmitBug(ctx, input, submitContext())
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.SubmitBug(ctx, input, submitContext())
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID || first.State != "held" {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	stored, err := memory.ReportSubmission(ctx, "prod_acme", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored.PayloadCiphertext, []byte(input.Summary)) || len(stored.PayloadCiphertext) == 0 || len(stored.PayloadNonce) == 0 {
		t.Fatal("report payload was not encrypted at rest")
	}
	if stored.ActorPseudonym != "actor-pseudonym" || len(stored.IdempotencyDigest) != 32 {
		t.Fatalf("unsafe or incomplete outbox metadata: %#v", stored)
	}
	envelope, err := service.decrypt(stored)
	if err != nil || envelope.Reporter.Email != "" || envelope.Reporter.DisplayName != "" || envelope.Bug == nil || envelope.Bug.IdempotencyKey != "" {
		t.Fatalf("non-consented contact or idempotency data leaked into the encrypted envelope: envelope=%#v err=%v", envelope, err)
	}
	views, err := service.Submissions(ctx, "prod_acme", 10)
	if err != nil || len(views) != 1 || views[0].Summary != input.Summary || views[0].TrustedContext.ManifestHash != "sha256:test" || views[0].Content != nil {
		t.Fatalf("views=%#v err=%v", views, err)
	}
	detail, err := service.Submission(ctx, "prod_acme", first.ID)
	if err != nil || detail.Content["description"] != input.Description {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
}

func TestSensitiveContentIsRejectedBeforeStorage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService(t)
	if _, err := service.Configure(ctx, "prod_acme", ConfigInput{BugReportsEnabled: true, RetentionDays: 30}, "root-test", "req-config"); err != nil {
		t.Fatal(err)
	}
	_, err := service.SubmitBug(ctx, BugInput{Summary: "Authentication failure", Description: "authorization: Bearer abcdefghijklmnopqrstuvwxyz012345", IdempotencyKey: "bug-report-idempotency-0002"}, submitContext())
	if !errorsIs(err, ErrSensitiveContent) {
		t.Fatalf("err=%v", err)
	}
	values, listErr := memory.ReportSubmissions(ctx, "prod_acme", 10)
	if listErr != nil || len(values) != 0 {
		t.Fatalf("sensitive report was stored: values=%#v err=%v", values, listErr)
	}
}

func errorsIs(err, target error) bool {
	return err != nil && (err == target || strings.Contains(err.Error(), target.Error()))
}

func TestHookDeliveryUsesEncryptedCredentialAndTrustedEnvelope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService(t)
	service.Resolver = resolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	var deliveredBody string
	service.Client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		deliveredBody = string(body)
		if request.URL.String() != "https://hooks.vendor.example/bugs" || request.Header.Get("Authorization") != "Bearer delivery-secret" || request.Header.Get("Idempotency-Key") == "" {
			t.Fatalf("unsafe delivery request: url=%s headers=%v", request.URL, request.Header)
		}
		return &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"external_id":"BUG-42","external_url":"https://support.vendor.example/tickets/BUG-42"}`))}, nil
	})}
	config, err := service.Configure(ctx, "prod_acme", ConfigInput{BugReportsEnabled: true, BugHookURL: "https://hooks.vendor.example/bugs", BugHookCredential: "delivery-secret", RetentionDays: 30}, "root-test", "req-config")
	if err != nil || config.BugHookCredentialID == "" {
		t.Fatalf("config=%#v err=%v", config, err)
	}
	if _, err := service.Configure(ctx, "prod_acme", ConfigInput{BugReportsEnabled: true, BugHookURL: "https://different.vendor.example/bugs", RetentionDays: 30, Revision: config.Revision}, "root-test", "req-config-rotate"); !errorsIs(err, ErrInvalidReport) {
		t.Fatalf("changed hook destination reused an old credential: %v", err)
	}
	view, err := service.SubmitBug(ctx, BugInput{Summary: "Delivery test", Description: "A sanitized connector defect.", AllowContact: true, IdempotencyKey: "bug-report-idempotency-0003"}, submitContext())
	if err != nil || view.State != "pending" {
		t.Fatalf("view=%#v err=%v", view, err)
	}
	if _, err := service.Retry(ctx, "prod_acme", view.ID); !errorsIs(err, store.ErrConflict) {
		t.Fatalf("an in-flight submission was incorrectly retryable: %v", err)
	}
	processed, err := service.ProcessPending(ctx, 10)
	if err != nil || processed != 1 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	stored, err := memory.ReportSubmission(ctx, "prod_acme", view.ID)
	if err != nil || stored.State != "delivered" || stored.ExternalID != "BUG-42" || stored.Attempts != 1 {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	if !strings.Contains(deliveredBody, `"subject":"https://identity.vendor.example|user-123"`) || !strings.Contains(deliveredBody, `"manifest_hash":"sha256:test"`) || !strings.Contains(deliveredBody, `"email":"developer@example.com"`) {
		t.Fatalf("trusted hook context is incomplete: %s", deliveredBody)
	}
}

func TestStaleReportingRevisionFailsBeforeCredentialStorage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, _ := newReportingService(t)
	config, err := service.Configure(ctx, "prod_acme", ConfigInput{BugReportsEnabled: true, RetentionDays: 30}, "root-test", "req-config")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Configure(ctx, "prod_acme", ConfigInput{BugReportsEnabled: true, BugHookURL: "https://hooks.vendor.example/bugs", BugHookCredential: "must-not-be-stored", RetentionDays: 30, Revision: config.Revision - 1}, "root-test", "req-stale")
	if !errorsIs(err, store.ErrConflict) {
		t.Fatalf("stale config write did not fail with conflict: %v", err)
	}
}

func TestAddingHookActivatesHeldSubmissions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService(t)
	config, err := service.Configure(ctx, "prod_acme", ConfigInput{FeedbackEnabled: true, RetentionDays: 30}, "root-test", "req-config")
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.SubmitFeedback(ctx, FeedbackInput{Message: "The connector workflow was clear and useful.", Category: "usability", IdempotencyKey: "feedback-idempotency-0001"}, submitContext())
	if err != nil || view.State != "held" {
		t.Fatalf("view=%#v err=%v", view, err)
	}
	_, err = service.Configure(ctx, "prod_acme", ConfigInput{FeedbackEnabled: true, FeedbackHookURL: "https://hooks.vendor.example/feedback", FeedbackHookCredential: "feedback-secret", RetentionDays: 30, Revision: config.Revision}, "root-test", "req-config-2")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := memory.ReportSubmission(ctx, "prod_acme", view.ID)
	if err != nil || stored.State != "pending" || stored.NextAttemptAt == nil || stored.NextAttemptAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("held submission was not activated: %#v err=%v", stored, err)
	}
}
