package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func newReportingService() (*Service, *store.Memory) {
	memory := store.NewMemory()
	return New(memory), memory
}

func submitContext() SubmitContext {
	return SubmitContext{
		Principal: identity.Principal{
			Issuer: "https://identity.vendor.example", Subject: "user-123",
			Email: "developer@example.com", DisplayName: "Developer",
			ExternalCustomerID: "customer-1", InstallationID: "install-1",
		},
		ActorPseudonym: "actor-pseudonym",
		Product:        ProductContext{ProductID: "prod_acme", ProductName: "Acme Platform", CatalogRevision: 4},
		RequestID:      "req-test",
	}
}

func TestSubmissionIsPlaintextIdempotentAndConsentBound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService()
	input := BugInput{Summary: "Connector returns the wrong status", Description: "The response status is inconsistent with the documented connector behavior.", RelatedTool: "projects.create", IdempotencyKey: "bug-report-idempotency-0001"}
	first, err := service.SubmitBug(ctx, input, submitContext())
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.SubmitBug(ctx, input, submitContext())
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID || first.State != "queued" {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	stored, err := memory.ReportSubmission(ctx, "prod_acme", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stored.Payload, []byte(input.Summary)) || len(stored.IdempotencyDigest) != 32 {
		t.Fatalf("unexpected outbox record: %#v", stored)
	}
	var envelope Envelope
	if err := json.Unmarshal(stored.Payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != "1" || envelope.Product.ProductID != "prod_acme" || envelope.Channel != "private_mcp" || envelope.Reporter.Email != "" || envelope.Reporter.DisplayName != "" || envelope.Reporter.ExternalCustomerID != "customer-1" || envelope.Bug == nil || envelope.Bug.IdempotencyKey != "" {
		t.Fatalf("envelope=%#v", envelope)
	}
	detail, err := service.Submission(ctx, "prod_acme", first.ID)
	if err != nil || detail.Content["description"] != input.Description {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
}

func TestContactDetailsRequireConsentAndSecretsAreRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, memory := newReportingService()
	view, err := service.SubmitFeedback(ctx, FeedbackInput{Message: "The docs are useful.", AllowContact: true, IdempotencyKey: "feedback-idempotency-0001"}, submitContext())
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := memory.ReportSubmission(ctx, "prod_acme", view.ID)
	var envelope Envelope
	_ = json.Unmarshal(stored.Payload, &envelope)
	if envelope.Reporter.Email != "developer@example.com" || envelope.Reporter.DisplayName != "Developer" {
		t.Fatalf("consented identity missing: %#v", envelope.Reporter)
	}
	_, err = service.SubmitBug(ctx, BugInput{Summary: "Secret", Description: "authorization: Bearer abcdefghijklmnop", IdempotencyKey: "bug-report-idempotency-0002"}, submitContext())
	if !errors.Is(err, ErrSensitiveContent) {
		t.Fatalf("secret error=%v", err)
	}
}

func TestCapabilitiesAndPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, _ := newReportingService()
	capabilities, err := service.Capabilities(ctx, "prod_acme")
	if err != nil || len(capabilities) != 1 || !capabilities[0].BugReportsEnabled || !capabilities[0].FeedbackEnabled {
		t.Fatalf("capabilities=%#v err=%v", capabilities, err)
	}
	for _, message := range []string{"First item", "Second item"} {
		if _, err := service.SubmitFeedback(ctx, FeedbackInput{Message: message, IdempotencyKey: "feedback-idempotency-" + message}, submitContext()); err != nil {
			t.Fatal(err)
		}
	}
	page, hasMore, err := service.Submissions(ctx, "prod_acme", "", 1)
	if err != nil || len(page) != 1 || !hasMore || page[0].Content != nil {
		t.Fatalf("page=%#v has_more=%v err=%v", page, hasMore, err)
	}
	next, hasMore, err := service.Submissions(ctx, "prod_acme", page[0].ID, 1)
	if err != nil || len(next) != 1 || hasMore {
		t.Fatalf("next=%#v has_more=%v err=%v", next, hasMore, err)
	}
	if _, _, err := service.Submissions(ctx, "prod_acme", "missing", 1); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cursor error=%v", err)
	}
}
