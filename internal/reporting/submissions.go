package reporting

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func validateCommon(idempotencyKey, relatedTool string) error {
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		return errors.New("idempotency_key must be between 16 and 200 characters")
	}
	if relatedTool != "" && !toolNamePattern.MatchString(relatedTool) {
		return errors.New("related_tool is invalid")
	}
	return nil
}

func trimExact(value string) string { return strings.TrimSpace(value) }

func validateBug(input *BugInput) error {
	input.IntegrationID = trimExact(input.IntegrationID)
	input.Summary, input.Description = trimExact(input.Summary), trimExact(input.Description)
	input.RelatedTool, input.Severity = trimExact(input.RelatedTool), strings.ToLower(trimExact(input.Severity))
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
	if len(input.ExpectedBehavior) > 4000 || len(input.ActualBehavior) > 4000 || len(input.ErrorCode) > 120 || len(input.ErrorMessage) > 8000 || len(input.StackTrace) > 16000 || len(input.DiagnosticContext) > 20000 {
		return errors.New("report details exceed their limits")
	}
	if input.Severity == "" {
		input.Severity = "unknown"
	}
	if input.Severity != "unknown" && input.Severity != "low" && input.Severity != "medium" && input.Severity != "high" && input.Severity != "critical" {
		return errors.New("severity is invalid")
	}
	return validateCommon(input.IdempotencyKey, input.RelatedTool)
}

func validateFeedback(input *FeedbackInput) error {
	input.IntegrationID = trimExact(input.IntegrationID)
	input.Message, input.Category = trimExact(input.Message), strings.ToLower(trimExact(input.Category))
	input.RelatedTool = trimExact(input.RelatedTool)
	if input.Message == "" || len(input.Message) > 10000 {
		return errors.New("message is required and must contain no more than 10000 characters")
	}
	if input.Category == "" {
		input.Category = "general"
	}
	validCategory := input.Category == "general" || input.Category == "usability" || input.Category == "documentation" || input.Category == "performance" || input.Category == "feature_request" || input.Category == "other"
	if !validCategory {
		return errors.New("category is invalid")
	}
	if input.Rating != nil && (*input.Rating < 1 || *input.Rating > 5) {
		return errors.New("rating must be between 1 and 5")
	}
	return validateCommon(input.IdempotencyKey, input.RelatedTool)
}

func containsSensitiveContent(value any) bool {
	encoded, _ := json.Marshal(value)
	for _, pattern := range secretPatterns {
		if pattern.Match(encoded) {
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
	return Envelope{SchemaVersion: "1", Kind: kind, Reporter: reporter, Product: submit.Product, Integration: submit.Integration, Channel: "private_mcp", ConfirmedAt: s.now(), RequestID: submit.RequestID}
}

func (s *Service) submit(ctx context.Context, idempotencyKey string, envelope Envelope, actorPseudonym string) (SubmissionView, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return SubmissionView{}, err
	}
	if deployment.ID != envelope.Product.ProductID {
		return SubmissionView{}, fmt.Errorf("%w: product context is invalid", ErrInvalidReport)
	}
	deliveryURL := deployment.FeedbackSubmissionURL
	if envelope.Kind == KindBug {
		deliveryURL = deployment.ErrorSubmissionURL
	}
	if deliveryURL == "" {
		return SubmissionView{}, ErrDeliveryDisabled
	}
	id, err := randomUUID()
	if err != nil {
		return SubmissionView{}, err
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return SubmissionView{}, err
	}
	integrationID := ""
	if envelope.Integration != nil {
		integrationID = envelope.Integration.IntegrationID
	}
	digest := sha256.Sum256([]byte(envelope.Product.ProductID + "\x00" + integrationID + "\x00" + actorPseudonym + "\x00" + envelope.Kind + "\x00" + idempotencyKey))
	now := s.now()
	value, err := s.store.CreateReportSubmission(ctx, model.ReportSubmission{ID: id, OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, IntegrationID: integrationID, Kind: envelope.Kind, State: "queued", DeliveryURL: deliveryURL, AvailableAt: now, ActorPseudonym: actorPseudonym, IdempotencyDigest: digest[:], Payload: payload, ExpiresAt: now.AddDate(0, 0, 90)})
	if err != nil {
		return SubmissionView{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: auditID(), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actorPseudonym, Action: "support_submission.queued", TargetType: "support_submission", TargetID: value.ID, Current: map[string]any{"kind": value.Kind, "state": value.State}, RequestID: envelope.RequestID, CreatedAt: now}); err != nil {
		return SubmissionView{}, err
	}
	return s.view(value)
}

func (s *Service) view(value model.ReportSubmission) (SubmissionView, error) {
	var envelope Envelope
	if err := json.Unmarshal(value.Payload, &envelope); err != nil {
		return SubmissionView{}, err
	}
	view := SubmissionView{ID: value.ID, Kind: value.Kind, State: value.State, Attempts: value.Attempts, LastError: value.LastError, DeliveredAt: value.DeliveredAt, CreatedAt: value.CreatedAt, ExpiresAt: value.ExpiresAt, TrustedContext: envelope.Product, TrustedIntegration: envelope.Integration}
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
