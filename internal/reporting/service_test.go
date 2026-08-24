package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
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
	service := New(memory, vault)
	service.Resolver = resolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	return service, memory
}

func configureBackend(t *testing.T, service *Service, name, credential string) model.BackendConnection {
	t.Helper()
	connection, err := platform.NewWithVault(service.store, service.vault).CreateBackendConnection(context.Background(), platform.BackendConnectionInput{Name: name, BaseURL: "https://api.vendor.example", AuthenticationType: "bearer", Credential: credential, State: "active"}, platform.Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	return connection
}

func submitContext() SubmitContext {
	return SubmitContext{
		Principal: identity.Principal{
			Issuer: "https://identity.vendor.example", Subject: "user-123",
			Email: "developer@example.com", DisplayName: "Developer",
			CustomerAccountID: "account_internal_1", ExternalCustomerID: "customer-1", InstallationID: "install-1",
		},
		ActorPseudonym: "actor-pseudonym",
		Product:        ProductContext{ProductID: "prod_acme", ProductName: "Acme Platform", ProductVersionID: "version-1", ProductVersion: "2026.8", ManifestHash: "sha256:test", CatalogRevision: 4, EnvironmentID: "env_prod", InstallationID: "install-1"},
		RequestID:      "req-test",
	}
}

func configureReporting(t *testing.T, service *Service) {
	t.Helper()
	connection := configureBackend(t, service, "Default support backend", "delivery-secret")
	route, err := service.SaveRoute(context.Background(), "prod_acme", "", RouteInput{Name: "Default support", IsDefault: true, BugReportsEnabled: true, FeedbackEnabled: true, BackendConnectionID: connection.ID, RetentionDays: 30, State: "active"}, "root-test", "req-config")
	if err != nil {
		t.Fatal(err)
	}
	if route.BackendConnectionID != connection.ID {
		t.Fatal("backend connection was not attached")
	}
}

func TestEnabledReportingRequiresActiveBackendConnection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, _ := secrets.New(bytes.Repeat([]byte{0x42}, 32))
	service := New(memory, vault)
	if _, err := service.SaveRoute(ctx, "prod_acme", "", RouteInput{Name: "Default support", IsDefault: true, BugReportsEnabled: true, RetentionDays: 30}, "root", "req"); !errors.Is(err, ErrInvalidReport) {
		t.Fatalf("reporting enabled without backend connection: %v", err)
	}
	connection, err := platform.NewWithVault(memory, vault).CreateBackendConnection(ctx, platform.BackendConnectionInput{Name: "Disabled backend", BaseURL: "https://api.vendor.example", AuthenticationType: "bearer", Credential: "secret", State: "disabled"}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveRoute(ctx, "prod_acme", "", RouteInput{Name: "Default support", IsDefault: true, BugReportsEnabled: true, BackendConnectionID: connection.ID, RetentionDays: 30}, "root", "req"); !errors.Is(err, ErrInvalidReport) {
		t.Fatalf("reporting enabled with disabled backend connection: %v", err)
	}
}

func TestSubmissionIsEncryptedIdempotentAndConsentBound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService(t)
	configureReporting(t, service)
	input := BugInput{Summary: "Connector returns the wrong status", Description: "The response status is inconsistent with the documented connector behavior.", ReproductionSteps: []string{"Call support.example with a valid request"}, RelatedTool: "projects.create", IdempotencyKey: "bug-report-idempotency-0001"}
	first, err := service.SubmitBug(ctx, input, submitContext())
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.SubmitBug(ctx, input, submitContext())
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID || first.State != "pending" {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	stored, err := memory.ReportSubmission(ctx, "prod_acme", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored.PayloadCiphertext, []byte(input.Summary)) || len(stored.PayloadCiphertext) == 0 || len(stored.IdempotencyDigest) != 32 {
		t.Fatalf("unsafe outbox record: %#v", stored)
	}
	envelope, err := service.decrypt(stored)
	if err != nil || envelope.Reporter.Email != "" || envelope.Reporter.DisplayName != "" || envelope.Reporter.ExternalCustomerID != "customer-1" || envelope.Reporter.Principal.Subject != "user-123" || envelope.Bug == nil || envelope.Bug.IdempotencyKey != "" {
		t.Fatalf("envelope=%#v err=%v", envelope, err)
	}
	detail, err := service.Submission(ctx, "prod_acme", first.ID)
	if err != nil || detail.Content["description"] != input.Description {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
	if _, err := service.SubmitFeedback(ctx, FeedbackInput{Message: "Pagination companion", IdempotencyKey: "feedback-pagination-0001"}, submitContext()); err != nil {
		t.Fatal(err)
	}
	page, hasMore, err := service.Submissions(ctx, "prod_acme", "", 1)
	if err != nil || len(page) != 1 || !hasMore {
		t.Fatalf("first page=%#v has_more=%v err=%v", page, hasMore, err)
	}
	next, hasMore, err := service.Submissions(ctx, "prod_acme", page[0].ID, 1)
	if err != nil || len(next) != 1 || hasMore || next[0].ID == page[0].ID {
		t.Fatalf("second page=%#v has_more=%v err=%v", next, hasMore, err)
	}
	if _, _, err := service.Submissions(ctx, "prod_acme", "missing-submission", 1); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("invalid cursor error = %v", err)
	}
}

func TestDeliveryUsesNormativeEndpointAndRetrySafeContract(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService(t)
	configureReporting(t, service)
	var delivered map[string]any
	var idempotencyKey, requestID string
	service.Client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "https://api.vendor.example/v1/support-submissions" || request.Header.Get("Authorization") != "Bearer delivery-secret" {
			t.Fatalf("request=%s %s headers=%v", request.Method, request.URL, request.Header)
		}
		idempotencyKey, requestID = request.Header.Get("Idempotency-Key"), request.Header.Get("X-DokoSoko-Request-ID")
		if err := json.NewDecoder(request.Body).Decode(&delivered); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusAccepted, Status: "202 Accepted", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"receipt_42","status":"accepted","external_id":"BUG-42","external_url":"https://support.vendor.example/tickets/BUG-42","future_field":true}`))}, nil
	})}
	input := BugInput{Summary: "Delivery test", Description: "A sanitized connector defect.", AllowContact: true, IdempotencyKey: "bug-report-idempotency-0003"}
	view, err := service.SubmitBug(ctx, input, submitContext())
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessPending(ctx, 10)
	if err != nil || processed != 1 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	stored, err := memory.ReportSubmission(ctx, "prod_acme", view.ID)
	if err != nil || stored.State != "delivered" || stored.ExternalID != "BUG-42" || stored.Attempts != 1 {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	if idempotencyKey != view.ID || !strings.HasPrefix(requestID, "req_") || delivered["submission_id"] != view.ID || delivered["submission"] == nil {
		t.Fatalf("idempotency=%q request_id=%q body=%#v", idempotencyKey, requestID, delivered)
	}
	encoded, _ := json.Marshal(delivered)
	if !bytes.Contains(encoded, []byte(`"principal":{"issuer":"https://identity.vendor.example","subject":"user-123"}`)) || !bytes.Contains(encoded, []byte(`"external_customer_id":"customer-1"`)) || !bytes.Contains(encoded, []byte(`"email":"developer@example.com"`)) {
		t.Fatalf("trusted delivery context is incomplete: %s", encoded)
	}
}

func TestNonRetryableVendorResponseFailsPermanently(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService(t)
	configureReporting(t, service)
	service.Client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid","message":"invalid submission"}}`))}, nil
	})}
	view, err := service.SubmitFeedback(ctx, FeedbackInput{Message: "Useful workflow", IdempotencyKey: "feedback-idempotency-0001"}, submitContext())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ProcessPending(ctx, 1); err != nil {
		t.Fatal(err)
	}
	stored, err := memory.ReportSubmission(ctx, "prod_acme", view.ID)
	if err != nil || stored.State != "failed" || stored.NextAttemptAt != nil {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
}

func TestSensitiveContentIsRejectedBeforeStorage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService(t)
	configureReporting(t, service)
	_, err := service.SubmitBug(ctx, BugInput{Summary: "Authentication failure", Description: "authorization: Bearer abcdefghijklmnopqrstuvwxyz012345", IdempotencyKey: "bug-report-idempotency-0002"}, submitContext())
	if !errors.Is(err, ErrSensitiveContent) {
		t.Fatalf("err=%v", err)
	}
	values, _, listErr := memory.ReportSubmissions(ctx, "prod_acme", "", 10)
	if listErr != nil || len(values) != 0 {
		t.Fatalf("sensitive report was stored: values=%#v err=%v", values, listErr)
	}
}

func TestIntegrationRunReferenceMustBelongToReporter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService(t)
	configureReporting(t, service)
	if _, err := memory.CreateIntegrationRun(ctx, model.IntegrationRun{ID: "run_other", OrganisationID: "org_acme", ProductID: "prod_acme", EnvironmentID: "env_prod", ActorPseudonym: "another-reporter", RequestedOutcome: "Test another reporter's integration"}); err != nil {
		t.Fatal(err)
	}
	_, err := service.SubmitBug(ctx, BugInput{Summary: "Cross-account run", Description: "This submission must not reference another authenticated reporter's run.", IntegrationRunID: "run_other", IdempotencyKey: "cross-account-run-0001"}, submitContext())
	if !errors.Is(err, ErrInvalidReport) {
		t.Fatalf("cross-account run reference was accepted: %v", err)
	}
	if _, err := memory.CreateIntegrationRun(ctx, model.IntegrationRun{ID: "run_own", OrganisationID: "org_acme", ProductID: "prod_acme", EnvironmentID: "env_prod", ActorPseudonym: submitContext().ActorPseudonym, RequestedOutcome: "Test this reporter's integration"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SubmitBug(ctx, BugInput{Summary: "Owned run", Description: "This submission references the authenticated reporter's own run.", IntegrationRunID: "run_own", IdempotencyKey: "owned-integration-run-0001"}, submitContext()); err != nil {
		t.Fatalf("owned run reference was rejected: %v", err)
	}
}

func TestIntegrationRouteAndRevisionArePinnedAtSubmission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService(t)
	catalog := platform.New(memory)
	integration, err := catalog.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "voice", VersionKey: "v2", DisplayName: "Voice API", Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: "active"}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := catalog.PublishIntegration(ctx, integration.ID, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	connection := configureBackend(t, service, "Voice API backend", "route-secret")
	route, err := service.SaveRoute(ctx, "prod_acme", "", RouteInput{Name: "Voice API support", BugReportsEnabled: true, BackendConnectionID: connection.ID, RetentionDays: 45, IntegrationIDs: []string{integration.ID}}, "root", "req-route")
	if err != nil {
		t.Fatal(err)
	}
	submit := submitContext()
	submit.Integration = &IntegrationContext{IntegrationID: integration.ID, FamilyKey: integration.FamilyKey, VersionKey: integration.VersionKey, DisplayName: integration.DisplayName, Lifecycle: integration.Lifecycle, Revision: revision.Revision, ManifestHash: revision.ManifestHash, Snapshot: revision.Snapshot}
	view, err := service.SubmitBug(ctx, BugInput{IntegrationID: integration.ID, Summary: "Voice API regression", Description: "The versioned Voice API connector returned an unexpected response.", IdempotencyKey: "integration-route-report-0001"}, submit)
	if err != nil || view.State != "pending" || view.TrustedIntegration == nil || view.TrustedIntegration.ManifestHash != revision.ManifestHash {
		t.Fatalf("submission=%#v err=%v", view, err)
	}
	stored, err := memory.ReportSubmission(ctx, "prod_acme", view.ID)
	if err != nil || stored.IntegrationID != integration.ID || stored.SupportRouteID != route.ID || len(stored.IntegrationSnapshot) == 0 {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
}

func TestArchivedExplicitRouteDoesNotFallBackToDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService(t)
	catalog := platform.New(memory)
	integration, err := catalog.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "voice", VersionKey: "v2", DisplayName: "Voice API", Lifecycle: "active"}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	defaultConnection := configureBackend(t, service, "Default backend", "default-secret")
	if _, err := service.SaveRoute(ctx, "prod_acme", "", RouteInput{Name: "Default support", IsDefault: true, BugReportsEnabled: true, BackendConnectionID: defaultConnection.ID, RetentionDays: 30, State: "active"}, "root", "req-default"); err != nil {
		t.Fatal(err)
	}
	voiceConnection := configureBackend(t, service, "Voice backend", "voice-secret")
	specific, err := service.SaveRoute(ctx, "prod_acme", "", RouteInput{Name: "Voice support", BugReportsEnabled: true, BackendConnectionID: voiceConnection.ID, RetentionDays: 30, State: "active", IntegrationIDs: []string{integration.ID}}, "root", "req-specific")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveRoute(ctx, "prod_acme", specific.ID, RouteInput{Name: specific.Name, BugReportsEnabled: true, BackendConnectionID: voiceConnection.ID, RetentionDays: 30, State: "archived", IntegrationIDs: []string{integration.ID}, Revision: specific.Revision}, "root", "req-archive"); err != nil {
		t.Fatal(err)
	}
	submit := submitContext()
	submit.Integration = &IntegrationContext{IntegrationID: integration.ID, FamilyKey: integration.FamilyKey, VersionKey: integration.VersionKey, DisplayName: integration.DisplayName, Lifecycle: integration.Lifecycle, Revision: integration.Revision}
	_, err = service.SubmitBug(ctx, BugInput{IntegrationID: integration.ID, Summary: "Archived route", Description: "This report must not fall through to the deployment default.", IdempotencyKey: "archived-route-report-0001"}, submit)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("explicit archived route fell back to the default: %v", err)
	}
}

func TestRetryAfterParsesSecondsAndHTTPDate(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	if got := retryAfter("120", now); !got.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("seconds Retry-After = %s", got)
	}
	want := now.Add(5 * time.Minute)
	if got := retryAfter(want.Format(http.TimeFormat), now); !got.Equal(want) {
		t.Fatalf("date Retry-After = %s", got)
	}
}
