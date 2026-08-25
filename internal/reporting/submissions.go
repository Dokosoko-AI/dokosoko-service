package reporting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func validateCommon(idempotencyKey, relatedTool, integrationRunID string) error {
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		return errors.New("idempotency_key must be between 16 and 200 characters")
	}
	if relatedTool != "" && !toolNamePattern.MatchString(relatedTool) {
		return errors.New("related_tool is invalid")
	}
	if len(integrationRunID) > 160 {
		return errors.New("integration_run_id is too long")
	}
	return nil
}

func trimExact(value string) string { return strings.TrimSpace(value) }

func validateBug(input *BugInput) error {
	input.IntegrationID = trimExact(input.IntegrationID)
	input.Summary, input.Description = trimExact(input.Summary), trimExact(input.Description)
	input.RelatedTool, input.IntegrationRunID = trimExact(input.RelatedTool), trimExact(input.IntegrationRunID)
	input.Severity = strings.ToLower(trimExact(input.Severity))
	if input.Summary == "" || len(input.Summary) > 160 || input.Description == "" || len(input.Description) > 10000 {
		return errors.New("summary and description are required and must fit their limits")
	}
	if len(input.ReproductionSteps) > 20 {
		return errors.New("reproduction_steps must contain no more than 20 items")
	}
	for index, step := range input.ReproductionSteps {
		input.ReproductionSteps[index] = trimExact(step)
		if input.ReproductionSteps[index] == "" || len(input.ReproductionSteps[index]) > 1000 {
			return errors.New("each reproduction step must be between 1 and 1000 characters")
		}
	}
	for label, value := range map[string]string{"expected_behavior": input.ExpectedBehavior, "actual_behavior": input.ActualBehavior} {
		if len(value) > 4000 {
			return fmt.Errorf("%s is too long", label)
		}
	}
	if len(input.ErrorCode) > 120 || len(input.ErrorMessage) > 8000 || len(input.StackTrace) > 16000 || len(input.DiagnosticContext) > 20000 {
		return errors.New("error or diagnostic context is too long")
	}
	if input.Severity == "" {
		input.Severity = "unknown"
	}
	if input.Severity != "unknown" && input.Severity != "low" && input.Severity != "medium" && input.Severity != "high" && input.Severity != "critical" {
		return errors.New("severity is invalid")
	}
	return validateCommon(input.IdempotencyKey, input.RelatedTool, input.IntegrationRunID)
}

func validateFeedback(input *FeedbackInput) error {
	input.IntegrationID = trimExact(input.IntegrationID)
	input.Message, input.Category = trimExact(input.Message), strings.ToLower(trimExact(input.Category))
	input.RelatedTool, input.IntegrationRunID = trimExact(input.RelatedTool), trimExact(input.IntegrationRunID)
	if input.Message == "" || len(input.Message) > 10000 {
		return errors.New("message is required and must contain no more than 10000 characters")
	}
	if input.Category == "" {
		input.Category = "general"
	}
	if input.Category != "general" && input.Category != "usability" && input.Category != "documentation" && input.Category != "performance" && input.Category != "feature_request" && input.Category != "other" {
		return errors.New("category is invalid")
	}
	if input.Rating != nil && (*input.Rating < 1 || *input.Rating > 5) {
		return errors.New("rating must be between 1 and 5")
	}
	return validateCommon(input.IdempotencyKey, input.RelatedTool, input.IntegrationRunID)
}

func containsSensitiveContent(value any) bool {
	encoded, _ := json.Marshal(value)
	text := string(encoded)
	for _, pattern := range secretPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func (s *Service) SubmitBug(ctx context.Context, input BugInput, submit SubmitContext) (SubmissionView, error) {
	if err := validateBug(&input); err != nil {
		return SubmissionView{}, fmt.Errorf("%w: %v", ErrInvalidReport, err)
	}
	if containsSensitiveContent(input) {
		return SubmissionView{}, ErrSensitiveContent
	}
	if input.IntegrationID != "" && (submit.Integration == nil || submit.Integration.IntegrationID != input.IntegrationID) {
		return SubmissionView{}, fmt.Errorf("%w: integration_id does not belong to the trusted connector context", ErrInvalidReport)
	}
	if input.IntegrationRunID != "" {
		run, err := s.store.IntegrationRun(ctx, submit.Product.ProductID, input.IntegrationRunID)
		if err != nil || run.ActorPseudonym != submit.ActorPseudonym {
			return SubmissionView{}, fmt.Errorf("%w: integration_run_id does not belong to the authenticated reporter", ErrInvalidReport)
		}
	}
	idempotencyKey := input.IdempotencyKey
	input.IdempotencyKey, input.IntegrationID = "", ""
	envelope := s.envelope(KindBug, submit, input.AllowContact)
	envelope.Bug = &input
	return s.submit(ctx, idempotencyKey, envelope, submit.ActorPseudonym)
}

func (s *Service) SubmitFeedback(ctx context.Context, input FeedbackInput, submit SubmitContext) (SubmissionView, error) {
	if err := validateFeedback(&input); err != nil {
		return SubmissionView{}, fmt.Errorf("%w: %v", ErrInvalidReport, err)
	}
	if containsSensitiveContent(input) {
		return SubmissionView{}, ErrSensitiveContent
	}
	if input.IntegrationID != "" && (submit.Integration == nil || submit.Integration.IntegrationID != input.IntegrationID) {
		return SubmissionView{}, fmt.Errorf("%w: integration_id does not belong to the trusted connector context", ErrInvalidReport)
	}
	if input.IntegrationRunID != "" {
		run, err := s.store.IntegrationRun(ctx, submit.Product.ProductID, input.IntegrationRunID)
		if err != nil || run.ActorPseudonym != submit.ActorPseudonym {
			return SubmissionView{}, fmt.Errorf("%w: integration_run_id does not belong to the authenticated reporter", ErrInvalidReport)
		}
	}
	idempotencyKey := input.IdempotencyKey
	input.IdempotencyKey, input.IntegrationID = "", ""
	envelope := s.envelope(KindFeedback, submit, input.AllowContact)
	envelope.Feedback = &input
	return s.submit(ctx, idempotencyKey, envelope, submit.ActorPseudonym)
}

func (s *Service) envelope(kind string, submit SubmitContext, allowContact bool) Envelope {
	reporter := ReporterContext{Principal: ReporterPrincipal{Issuer: submit.Principal.Issuer, Subject: submit.Principal.Subject}, ExternalCustomerID: submit.Principal.ExternalCustomerID, InstallationID: submit.Principal.InstallationID, AllowContact: allowContact}
	if allowContact {
		reporter.DisplayName, reporter.Email = submit.Principal.DisplayName, submit.Principal.Email
	}
	resource := ResourceContext{Type: "deployment", ID: submit.Product.ProductID, Name: submit.Product.ProductName, VersionID: submit.Product.ProductVersionID, Version: submit.Product.ProductVersion, EnvironmentID: submit.Product.EnvironmentID, InstallationID: submit.Product.InstallationID}
	related := make([]ResourceContext, 0, 1)
	if submit.Integration != nil {
		related = append(related, ResourceContext{Type: "api", ID: submit.Integration.IntegrationID, Name: submit.Integration.DisplayName, Version: submit.Integration.VersionKey, State: submit.Integration.Lifecycle, Revision: submit.Integration.Revision})
	}
	extensions := &EnvelopeExtensions{DokoSoko: DokoSokoExtension{ManifestHash: submit.Product.ManifestHash, CatalogRevision: submit.Product.CatalogRevision, SelectionSource: submit.Product.SelectionSource, Integration: submit.Integration}}
	return Envelope{SchemaVersion: "2026-08-25", Kind: kind, Reporter: reporter, Provider: ProviderContext{Key: "dokosoko", Name: "DokoSoko"}, Resource: resource, RelatedResources: related, Channel: "private_mcp", Extensions: extensions, ConfirmedAt: s.now(), RequestID: submit.RequestID}
}

func envelopeProduct(envelope Envelope) ProductContext {
	if envelope.Resource.ID != "" {
		product := ProductContext{ProductID: envelope.Resource.ID, ProductName: envelope.Resource.Name, ProductVersionID: envelope.Resource.VersionID, ProductVersion: envelope.Resource.Version, EnvironmentID: envelope.Resource.EnvironmentID, InstallationID: envelope.Resource.InstallationID}
		if envelope.Extensions != nil {
			product.ManifestHash = envelope.Extensions.DokoSoko.ManifestHash
			product.CatalogRevision = envelope.Extensions.DokoSoko.CatalogRevision
			product.SelectionSource = envelope.Extensions.DokoSoko.SelectionSource
		}
		return product
	}
	if envelope.LegacyProduct != nil {
		return *envelope.LegacyProduct
	}
	return ProductContext{}
}

func envelopeIntegration(envelope Envelope) *IntegrationContext {
	if envelope.Extensions != nil && envelope.Extensions.DokoSoko.Integration != nil {
		return envelope.Extensions.DokoSoko.Integration
	}
	return envelope.LegacyIntegration
}

func (s *Service) submit(ctx context.Context, idempotencyKey string, envelope Envelope, actorPseudonym string) (SubmissionView, error) {
	organisationID, supportRouteID, retentionDays, enabled, deliveryCredentialID, err := s.submissionRoute(ctx, envelope)
	if err != nil {
		return SubmissionView{}, err
	}
	if !enabled {
		return SubmissionView{}, ErrDisabled
	}
	if s.vault == nil {
		return SubmissionView{}, errors.New("report payload vault is unavailable")
	}
	id, err := randomUUID()
	if err != nil {
		return SubmissionView{}, err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return SubmissionView{}, err
	}
	encrypted, err := s.vault.Encrypt(encoded, organisationID+":report:"+id)
	if err != nil {
		return SubmissionView{}, err
	}
	product := envelopeProduct(envelope)
	integration := envelopeIntegration(envelope)
	integrationID := ""
	integrationSnapshot := json.RawMessage(`{}`)
	if integration != nil {
		integrationID = integration.IntegrationID
		integrationSnapshot, err = json.Marshal(integration)
		if err != nil {
			return SubmissionView{}, err
		}
	}
	digest := sha256.Sum256([]byte(product.ProductID + "\x00" + integrationID + "\x00" + actorPseudonym + "\x00" + envelope.Kind + "\x00" + idempotencyKey))
	now := s.now()
	state := "held"
	var next *time.Time
	if deliveryCredentialID != "" {
		state, next = "pending", &now
	}
	value, err := s.store.CreateReportSubmission(ctx, model.ReportSubmission{ID: id, OrganisationID: organisationID, ProductID: product.ProductID, IntegrationID: integrationID, IntegrationSnapshot: integrationSnapshot, SupportRouteID: supportRouteID, Kind: envelope.Kind, State: state, ActorPseudonym: actorPseudonym, IdempotencyDigest: digest[:], PayloadCiphertext: encrypted.Ciphertext, PayloadNonce: encrypted.Nonce, PayloadKeyVersion: encrypted.KeyVersion, PayloadFingerprint: encrypted.Fingerprint, NextAttemptAt: next, ExpiresAt: now.AddDate(0, 0, retentionDays)})
	if err != nil {
		return SubmissionView{}, err
	}
	return s.view(value)
}

func (s *Service) submissionRoute(ctx context.Context, envelope Envelope) (organisationID, routeID string, retentionDays int, enabled bool, deliveryCredentialID string, err error) {
	product := envelopeProduct(envelope)
	integration := envelopeIntegration(envelope)
	integrationID := ""
	if integration != nil {
		integrationID = integration.IntegrationID
	}
	route, routeErr := s.store.SupportRouteForIntegration(ctx, product.ProductID, integrationID)
	if routeErr != nil {
		if errors.Is(routeErr, store.ErrNotFound) {
			err = ErrNotConfigured
		} else {
			err = routeErr
		}
		return
	}
	organisationID, routeID, retentionDays = route.OrganisationID, route.ID, route.RetentionDays
	enabled = route.BugReportsEnabled
	if envelope.Kind == KindFeedback {
		enabled = route.FeedbackEnabled
	}
	if enabled && route.State == "active" && route.BackendConnectionID != "" {
		connection, connectionErr := s.store.BackendConnection(ctx, product.ProductID, route.BackendConnectionID)
		if connectionErr == nil && connection.State == "active" {
			deliveryCredentialID = connection.CredentialSecretID
		}
	}
	return
}

func (s *Service) decrypt(value model.ReportSubmission) (Envelope, error) {
	if s.vault == nil {
		return Envelope{}, errors.New("report payload vault is unavailable")
	}
	plain, err := s.vault.Decrypt(secrets.Encrypted{Ciphertext: value.PayloadCiphertext, Nonce: value.PayloadNonce, KeyVersion: value.PayloadKeyVersion, Fingerprint: value.PayloadFingerprint}, value.OrganisationID+":report:"+value.ID)
	if err != nil {
		return Envelope{}, err
	}
	var envelope Envelope
	decoder := json.NewDecoder(bytes.NewReader(plain))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return Envelope{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Envelope{}, errors.New("report payload contains multiple JSON values")
	}
	return envelope, nil
}

func (s *Service) view(value model.ReportSubmission) (SubmissionView, error) {
	envelope, err := s.decrypt(value)
	if err != nil {
		return SubmissionView{}, err
	}
	view := SubmissionView{ID: value.ID, SupportRouteID: value.SupportRouteID, Kind: value.Kind, State: value.State, Attempts: value.Attempts, LastError: value.LastError, ExternalID: value.ExternalID, ExternalURL: value.ExternalURL, CreatedAt: value.CreatedAt, DeliveredAt: value.DeliveredAt, ExpiresAt: value.ExpiresAt, TrustedContext: envelopeProduct(envelope), TrustedIntegration: envelopeIntegration(envelope)}
	if envelope.Bug != nil {
		view.Summary, view.RelatedTool = envelope.Bug.Summary, envelope.Bug.RelatedTool
		encoded, _ := json.Marshal(envelope.Bug)
		_ = json.Unmarshal(encoded, &view.Content)
	}
	if envelope.Feedback != nil {
		view.Summary, view.Category, view.Rating, view.RelatedTool = envelope.Feedback.Message, envelope.Feedback.Category, envelope.Feedback.Rating, envelope.Feedback.RelatedTool
		encoded, _ := json.Marshal(envelope.Feedback)
		_ = json.Unmarshal(encoded, &view.Content)
	}
	return view, nil
}

func (s *Service) Submissions(ctx context.Context, productID, startingAfter string, limit int) ([]SubmissionView, bool, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	values, hasMore, err := s.store.ReportSubmissions(ctx, productID, startingAfter, limit)
	if err != nil {
		return nil, false, err
	}
	result := make([]SubmissionView, 0, len(values))
	for _, value := range values {
		view, viewErr := s.view(value)
		if viewErr != nil {
			return nil, false, viewErr
		}
		view.Content = nil
		result = append(result, view)
	}
	return result, hasMore, nil
}

func (s *Service) Submission(ctx context.Context, productID, id string) (SubmissionView, error) {
	value, err := s.store.ReportSubmission(ctx, productID, id)
	if err != nil {
		return SubmissionView{}, err
	}
	return s.view(value)
}

func (s *Service) Retry(ctx context.Context, productID, id string) (SubmissionView, error) {
	value, err := s.store.ReportSubmission(ctx, productID, id)
	if err != nil {
		return SubmissionView{}, err
	}
	if value.State == "pending" || value.State == "delivering" {
		return s.view(value)
	}
	endpoint, credentialID, err := s.deliveryRoute(ctx, value)
	if err != nil {
		if errors.Is(err, ErrNotConfigured) || errors.Is(err, store.ErrNotFound) {
			return SubmissionView{}, ErrDeliveryUnavailable
		}
		return SubmissionView{}, err
	}
	if endpoint == "" || credentialID == "" {
		return SubmissionView{}, ErrDeliveryUnavailable
	}
	value, err = s.store.RetryReportSubmission(ctx, productID, id, s.now())
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			current, lookupErr := s.store.ReportSubmission(ctx, productID, id)
			if lookupErr == nil && (current.State == "pending" || current.State == "delivering") {
				return s.view(current)
			}
		}
		return SubmissionView{}, err
	}
	return s.view(value)
}
