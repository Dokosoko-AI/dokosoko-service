package reporting

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const (
	KindBug             = "bug"
	KindFeedback        = "feedback"
	maxDeliveryAttempts = 8
	maxHookResponse     = 1 << 20
)

var (
	ErrNotConfigured    = errors.New("reporting is not configured")
	ErrDisabled         = errors.New("this report type is disabled")
	ErrInvalidReport    = errors.New("report is invalid")
	ErrSensitiveContent = errors.New("report may contain a credential or secret")
	ErrHookUnavailable  = errors.New("report delivery hook is unavailable")

	toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,160}$`)
	secretPatterns  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password)\s*[:=]\s*(bearer\s+)?[A-Za-z0-9_./+\-=]{8,}`),
		regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9_./+\-=]{12,}`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----`),
		regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
	}
)

type ConfigInput struct {
	BugReportsEnabled      bool
	FeedbackEnabled        bool
	BugHookURL             string
	BugHookCredential      string
	FeedbackHookURL        string
	FeedbackHookCredential string
	RetentionDays          int
	Revision               int64
}

type BugInput struct {
	Summary           string   `json:"summary"`
	Description       string   `json:"description"`
	ReproductionSteps []string `json:"reproduction_steps,omitempty"`
	ExpectedBehavior  string   `json:"expected_behavior,omitempty"`
	ActualBehavior    string   `json:"actual_behavior,omitempty"`
	ErrorCode         string   `json:"error_code,omitempty"`
	ErrorMessage      string   `json:"error_message,omitempty"`
	StackTrace        string   `json:"stack_trace,omitempty"`
	DiagnosticContext string   `json:"diagnostic_context,omitempty"`
	RelatedTool       string   `json:"related_tool,omitempty"`
	IntegrationRunID  string   `json:"integration_run_id,omitempty"`
	Severity          string   `json:"severity,omitempty"`
	AllowContact      bool     `json:"allow_contact,omitempty"`
	IdempotencyKey    string   `json:"idempotency_key,omitempty"`
}

type FeedbackInput struct {
	Message          string `json:"message"`
	Category         string `json:"category,omitempty"`
	Rating           *int   `json:"rating,omitempty"`
	RelatedTool      string `json:"related_tool,omitempty"`
	IntegrationRunID string `json:"integration_run_id,omitempty"`
	AllowContact     bool   `json:"allow_contact,omitempty"`
	IdempotencyKey   string `json:"idempotency_key,omitempty"`
}

type ReporterContext struct {
	Subject              string `json:"subject"`
	DisplayName          string `json:"display_name,omitempty"`
	Email                string `json:"email,omitempty"`
	VendorOrganisationID string `json:"vendor_organisation_id,omitempty"`
	InstallationID       string `json:"installation_id,omitempty"`
	AllowContact         bool   `json:"allow_contact"`
}

type ProductContext struct {
	ProductID        string `json:"product_id"`
	ProductName      string `json:"product_name"`
	ProductVersionID string `json:"product_version_id,omitempty"`
	ProductVersion   string `json:"product_version,omitempty"`
	ManifestHash     string `json:"manifest_hash,omitempty"`
	CatalogRevision  int64  `json:"catalog_revision,omitempty"`
	SelectionSource  string `json:"selection_source,omitempty"`
	EnvironmentID    string `json:"environment_id,omitempty"`
	InstallationID   string `json:"installation_id,omitempty"`
}

type Envelope struct {
	SchemaVersion string          `json:"schema_version"`
	Kind          string          `json:"kind"`
	Bug           *BugInput       `json:"bug,omitempty"`
	Feedback      *FeedbackInput  `json:"feedback,omitempty"`
	Reporter      ReporterContext `json:"reporter"`
	Product       ProductContext  `json:"product"`
	Source        string          `json:"source"`
	ConfirmedAt   time.Time       `json:"confirmed_at"`
	RequestID     string          `json:"request_id"`
}

type SubmissionView struct {
	ID             string         `json:"id"`
	Kind           string         `json:"kind"`
	State          string         `json:"state"`
	Summary        string         `json:"summary"`
	Category       string         `json:"category,omitempty"`
	Rating         *int           `json:"rating,omitempty"`
	RelatedTool    string         `json:"related_tool,omitempty"`
	Attempts       int            `json:"attempts"`
	LastError      string         `json:"last_error,omitempty"`
	ExternalID     string         `json:"external_id,omitempty"`
	ExternalURL    string         `json:"external_url,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	DeliveredAt    *time.Time     `json:"delivered_at,omitempty"`
	ExpiresAt      time.Time      `json:"expires_at"`
	Content        map[string]any `json:"content,omitempty"`
	TrustedContext ProductContext `json:"trusted_context"`
}

type SubmitContext struct {
	Principal      identity.Principal
	ActorPseudonym string
	Product        ProductContext
	RequestID      string
}

type Service struct {
	store    store.Store
	vault    *secrets.Vault
	Client   *http.Client
	Resolver identity.IPResolver
	now      func() time.Time
}

func New(repository store.Store, vault *secrets.Vault) *Service {
	return &Service{store: repository, vault: vault, now: func() time.Time { return time.Now().UTC() }}
}

func randomUUID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	buffer[6] = buffer[6]&0x0f | 0x40
	buffer[8] = buffer[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buffer[:4], buffer[4:6], buffer[6:8], buffer[8:10], buffer[10:]), nil
}

func auditID() string {
	buffer := make([]byte, 16)
	_, _ = rand.Read(buffer)
	return "audit_" + hex.EncodeToString(buffer)
}

func validHookURL(raw string) bool {
	if raw == "" {
		return true
	}
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && (parsed.Port() == "" || parsed.Port() == "443")
}

func (s *Service) Config(ctx context.Context, productID string) (model.ReportingConfig, error) {
	value, err := s.store.ReportingConfig(ctx, productID)
	if errors.Is(err, store.ErrNotFound) {
		return model.ReportingConfig{}, ErrNotConfigured
	}
	return value, err
}

func (s *Service) Configure(ctx context.Context, productID string, input ConfigInput, actorID, requestID string) (model.ReportingConfig, error) {
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.ReportingConfig{}, err
	}
	input.BugHookURL = strings.TrimSpace(input.BugHookURL)
	input.FeedbackHookURL = strings.TrimSpace(input.FeedbackHookURL)
	input.BugHookCredential = strings.TrimSpace(input.BugHookCredential)
	input.FeedbackHookCredential = strings.TrimSpace(input.FeedbackHookCredential)
	if !validHookURL(input.BugHookURL) || !validHookURL(input.FeedbackHookURL) {
		return model.ReportingConfig{}, fmt.Errorf("%w: hook URLs must be fixed HTTPS URLs without credentials, query strings, fragments, or non-standard ports", ErrInvalidReport)
	}
	if input.RetentionDays == 0 {
		input.RetentionDays = 30
	}
	if input.RetentionDays < 1 || input.RetentionDays > 365 {
		return model.ReportingConfig{}, fmt.Errorf("%w: retention_days must be between 1 and 365", ErrInvalidReport)
	}
	current, currentErr := s.store.ReportingConfig(ctx, productID)
	if currentErr != nil && !errors.Is(currentErr, store.ErrNotFound) {
		return model.ReportingConfig{}, currentErr
	}
	if errors.Is(currentErr, store.ErrNotFound) && input.Revision != 0 {
		return model.ReportingConfig{}, store.ErrConflict
	}
	if currentErr == nil && current.Revision != input.Revision {
		return model.ReportingConfig{}, store.ErrConflict
	}
	config := model.ReportingConfig{OrganisationID: product.OrganisationID, ProductID: productID, BugReportsEnabled: input.BugReportsEnabled, FeedbackEnabled: input.FeedbackEnabled, BugHookURL: input.BugHookURL, FeedbackHookURL: input.FeedbackHookURL, RetentionDays: input.RetentionDays}
	if currentErr == nil {
		config.ID = current.ID
		config.BugHookCredentialID = current.BugHookCredentialID
		config.FeedbackHookCredentialID = current.FeedbackHookCredentialID
	} else {
		config.ID, err = randomUUID()
		if err != nil {
			return model.ReportingConfig{}, err
		}
	}
	if input.BugHookURL != "" && input.BugHookCredential == "" && (currentErr != nil || current.BugHookURL != input.BugHookURL || current.BugHookCredentialID == "") {
		return model.ReportingConfig{}, fmt.Errorf("%w: a new bug hook destination requires a new credential", ErrInvalidReport)
	}
	if input.FeedbackHookURL != "" && input.FeedbackHookCredential == "" && (currentErr != nil || current.FeedbackHookURL != input.FeedbackHookURL || current.FeedbackHookCredentialID == "") {
		return model.ReportingConfig{}, fmt.Errorf("%w: a new feedback hook destination requires a new credential", ErrInvalidReport)
	}
	if input.BugHookURL == "" {
		config.BugHookCredentialID = ""
	} else if input.BugHookCredential != "" {
		config.BugHookCredentialID, err = s.storeHookCredential(ctx, product.OrganisationID, KindBug, input.BugHookCredential)
		if err != nil {
			return model.ReportingConfig{}, err
		}
	}
	if input.FeedbackHookURL == "" {
		config.FeedbackHookCredentialID = ""
	} else if input.FeedbackHookCredential != "" {
		config.FeedbackHookCredentialID, err = s.storeHookCredential(ctx, product.OrganisationID, KindFeedback, input.FeedbackHookCredential)
		if err != nil {
			return model.ReportingConfig{}, err
		}
	}
	if input.BugHookURL != "" && config.BugHookCredentialID == "" {
		return model.ReportingConfig{}, fmt.Errorf("%w: bug hook credential is required", ErrInvalidReport)
	}
	if input.FeedbackHookURL != "" && config.FeedbackHookCredentialID == "" {
		return model.ReportingConfig{}, fmt.Errorf("%w: feedback hook credential is required", ErrInvalidReport)
	}
	updated, err := s.store.SaveReportingConfig(ctx, config, input.Revision)
	if err != nil {
		return model.ReportingConfig{}, err
	}
	if updated.BugReportsEnabled && updated.BugHookURL != "" {
		_ = s.store.ActivateHeldReportSubmissions(ctx, productID, KindBug, s.now())
	}
	if updated.FeedbackEnabled && updated.FeedbackHookURL != "" {
		_ = s.store.ActivateHeldReportSubmissions(ctx, productID, KindFeedback, s.now())
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: auditID(), OrganisationID: product.OrganisationID, ProductID: productID, ActorID: actorID, Action: "reporting.configured", TargetType: "reporting_config", TargetID: updated.ID, Current: map[string]any{"bug_reports_enabled": updated.BugReportsEnabled, "feedback_enabled": updated.FeedbackEnabled, "bug_hook": updated.BugHookURL != "", "feedback_hook": updated.FeedbackHookURL != "", "bug_credential_rotated": input.BugHookCredential != "", "feedback_credential_rotated": input.FeedbackHookCredential != "", "retention_days": updated.RetentionDays}, RequestID: requestID, CreatedAt: s.now()})
	return updated, nil
}

func (s *Service) storeHookCredential(ctx context.Context, organisationID, kind, plaintext string) (string, error) {
	if s.vault == nil {
		return "", errors.New("reporting credential vault is unavailable")
	}
	id, err := randomUUID()
	if err != nil {
		return "", err
	}
	encrypted, err := s.vault.Encrypt([]byte(plaintext), organisationID+":reporting:"+kind+":"+id)
	if err != nil {
		return "", err
	}
	_, err = s.store.CreateSecret(ctx, model.Secret{ID: id, OrganisationID: organisationID, Name: "reporting-" + kind + "-" + id, Purpose: "reporting_" + kind, Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint})
	return id, err
}

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
	if input.IntegrationRunID != "" {
		if _, err := s.store.IntegrationRun(ctx, submit.Product.ProductID, input.IntegrationRunID); err != nil {
			return SubmissionView{}, fmt.Errorf("%w: integration_run_id does not belong to this product", ErrInvalidReport)
		}
	}
	idempotencyKey := input.IdempotencyKey
	input.IdempotencyKey = ""
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
	if input.IntegrationRunID != "" {
		if _, err := s.store.IntegrationRun(ctx, submit.Product.ProductID, input.IntegrationRunID); err != nil {
			return SubmissionView{}, fmt.Errorf("%w: integration_run_id does not belong to this product", ErrInvalidReport)
		}
	}
	idempotencyKey := input.IdempotencyKey
	input.IdempotencyKey = ""
	envelope := s.envelope(KindFeedback, submit, input.AllowContact)
	envelope.Feedback = &input
	return s.submit(ctx, idempotencyKey, envelope, submit.ActorPseudonym)
}

func (s *Service) envelope(kind string, submit SubmitContext, allowContact bool) Envelope {
	subject := submit.Principal.Subject
	if submit.Principal.Issuer != "" {
		subject = submit.Principal.Issuer + "|" + subject
	}
	reporter := ReporterContext{Subject: subject, VendorOrganisationID: submit.Principal.VendorOrganisation, InstallationID: submit.Principal.InstallationID, AllowContact: allowContact}
	if allowContact {
		reporter.DisplayName, reporter.Email = submit.Principal.DisplayName, submit.Principal.Email
	}
	return Envelope{SchemaVersion: "2026-08-19", Kind: kind, Reporter: reporter, Product: submit.Product, Source: "private_mcp", ConfirmedAt: s.now(), RequestID: submit.RequestID}
}

func (s *Service) submit(ctx context.Context, idempotencyKey string, envelope Envelope, actorPseudonym string) (SubmissionView, error) {
	config, err := s.Config(ctx, envelope.Product.ProductID)
	if err != nil {
		return SubmissionView{}, err
	}
	enabled := config.BugReportsEnabled
	hookURL := config.BugHookURL
	if envelope.Kind == KindFeedback {
		enabled, hookURL = config.FeedbackEnabled, config.FeedbackHookURL
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
	encrypted, err := s.vault.Encrypt(encoded, config.OrganisationID+":report:"+id)
	if err != nil {
		return SubmissionView{}, err
	}
	digest := sha256.Sum256([]byte(envelope.Product.ProductID + "\x00" + actorPseudonym + "\x00" + envelope.Kind + "\x00" + idempotencyKey))
	now := s.now()
	state := "held"
	var next *time.Time
	if hookURL != "" {
		state, next = "pending", &now
	}
	value, err := s.store.CreateReportSubmission(ctx, model.ReportSubmission{ID: id, OrganisationID: config.OrganisationID, ProductID: envelope.Product.ProductID, Kind: envelope.Kind, State: state, ActorPseudonym: actorPseudonym, IdempotencyDigest: digest[:], PayloadCiphertext: encrypted.Ciphertext, PayloadNonce: encrypted.Nonce, PayloadKeyVersion: encrypted.KeyVersion, PayloadFingerprint: encrypted.Fingerprint, NextAttemptAt: next, ExpiresAt: now.AddDate(0, 0, config.RetentionDays)})
	if err != nil {
		return SubmissionView{}, err
	}
	return s.view(value)
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
	return envelope, nil
}

func (s *Service) view(value model.ReportSubmission) (SubmissionView, error) {
	envelope, err := s.decrypt(value)
	if err != nil {
		return SubmissionView{}, err
	}
	view := SubmissionView{ID: value.ID, Kind: value.Kind, State: value.State, Attempts: value.Attempts, LastError: value.LastError, ExternalID: value.ExternalID, ExternalURL: value.ExternalURL, CreatedAt: value.CreatedAt, DeliveredAt: value.DeliveredAt, ExpiresAt: value.ExpiresAt, TrustedContext: envelope.Product}
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

func (s *Service) Submissions(ctx context.Context, productID string, limit int) ([]SubmissionView, error) {
	values, err := s.store.ReportSubmissions(ctx, productID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]SubmissionView, 0, len(values))
	for _, value := range values {
		view, viewErr := s.view(value)
		if viewErr != nil {
			return nil, viewErr
		}
		view.Content = nil
		result = append(result, view)
	}
	return result, nil
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
	config, err := s.Config(ctx, productID)
	if err != nil {
		return SubmissionView{}, err
	}
	hookURL := config.BugHookURL
	if value.Kind == KindFeedback {
		hookURL = config.FeedbackHookURL
	}
	if hookURL == "" {
		return SubmissionView{}, ErrHookUnavailable
	}
	value, err = s.store.RetryReportSubmission(ctx, productID, id, s.now())
	if err != nil {
		return SubmissionView{}, err
	}
	return s.view(value)
}

func (s *Service) ProcessPending(ctx context.Context, limit int) (int, error) {
	values, err := s.store.ClaimReportSubmissions(ctx, s.now(), limit)
	if err != nil {
		return 0, err
	}
	for _, value := range values {
		s.deliver(ctx, value)
	}
	return len(values), nil
}

func (s *Service) deliver(ctx context.Context, value model.ReportSubmission) {
	config, err := s.Config(ctx, value.ProductID)
	if err != nil {
		s.deliveryFailed(ctx, value, err)
		return
	}
	hookURL, credentialID := config.BugHookURL, config.BugHookCredentialID
	if value.Kind == KindFeedback {
		hookURL, credentialID = config.FeedbackHookURL, config.FeedbackHookCredentialID
	}
	if hookURL == "" || credentialID == "" {
		value.State, value.NextAttemptAt, value.DeliveryStartedAt, value.LastError = "held", nil, nil, ""
		_, _ = s.store.UpdateReportSubmissionDelivery(ctx, value)
		return
	}
	envelope, err := s.decrypt(value)
	if err != nil {
		s.deliveryFailed(ctx, value, errors.New("encrypted report payload could not be opened"))
		return
	}
	credential, err := s.hookCredential(ctx, value.OrganisationID, value.Kind, credentialID)
	if err != nil {
		s.deliveryFailed(ctx, value, errors.New("reporting hook credential is unavailable"))
		return
	}
	parsed, err := url.Parse(hookURL)
	if err != nil || !validHookURL(hookURL) {
		s.deliveryFailed(ctx, value, errors.New("reporting hook destination is unsafe"))
		return
	}
	body, _ := json.Marshal(map[string]any{"submission_id": value.ID, "created_at": value.CreatedAt, "report": envelope})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		s.deliveryFailed(ctx, value, err)
		return
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Idempotency-Key", value.ID)
	request.Header.Set("X-DokoSoko-Submission-ID", value.ID)
	client, err := identity.SafeHookClient(ctx, parsed, s.Client, s.Resolver)
	if err != nil {
		s.deliveryFailed(ctx, value, errors.New("reporting hook destination is unsafe"))
		return
	}
	response, err := client.Do(request)
	if err != nil {
		s.deliveryFailed(ctx, value, errors.New("reporting hook request failed"))
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		s.deliveryFailed(ctx, value, fmt.Errorf("reporting hook returned status %d", response.StatusCode))
		return
	}
	result := struct {
		ExternalID  string `json:"external_id"`
		ExternalURL string `json:"external_url"`
	}{}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxHookResponse+1))
	if readErr != nil || len(raw) > maxHookResponse {
		s.deliveryFailed(ctx, value, errors.New("reporting hook response is too large"))
		return
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&result); err != nil {
			s.deliveryFailed(ctx, value, errors.New("reporting hook returned invalid JSON"))
			return
		}
	}
	if len(result.ExternalID) > 200 || len(result.ExternalURL) > 2000 || (result.ExternalURL != "" && !validExternalURL(result.ExternalURL)) {
		s.deliveryFailed(ctx, value, errors.New("reporting hook returned invalid ticket metadata"))
		return
	}
	now := s.now()
	value.State, value.NextAttemptAt, value.DeliveryStartedAt, value.LastError = "delivered", nil, nil, ""
	value.ExternalID, value.ExternalURL, value.DeliveredAt = result.ExternalID, result.ExternalURL, &now
	_, _ = s.store.UpdateReportSubmissionDelivery(ctx, value)
}

func validExternalURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil
}

func (s *Service) hookCredential(ctx context.Context, organisationID, kind, id string) (string, error) {
	if s.vault == nil {
		return "", errors.New("credential vault is unavailable")
	}
	stored, err := s.store.Secret(ctx, organisationID, id)
	if err != nil {
		return "", err
	}
	plain, err := s.vault.Decrypt(secrets.Encrypted{Ciphertext: stored.Ciphertext, Nonce: stored.Nonce, KeyVersion: stored.KeyVersion, Fingerprint: stored.Fingerprint}, organisationID+":reporting:"+kind+":"+id)
	return string(plain), err
}

func (s *Service) deliveryFailed(ctx context.Context, value model.ReportSubmission, failure error) {
	value.DeliveryStartedAt = nil
	message := failure.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	value.LastError = message
	if value.Attempts >= maxDeliveryAttempts {
		value.State, value.NextAttemptAt = "failed", nil
	} else {
		delay := time.Minute * time.Duration(1<<min(value.Attempts-1, 8))
		if delay > 6*time.Hour {
			delay = 6 * time.Hour
		}
		next := s.now().Add(delay)
		value.State, value.NextAttemptAt = "pending", &next
	}
	_, _ = s.store.UpdateReportSubmissionDelivery(ctx, value)
}

func (s *Service) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.ProcessPending(ctx, 50)
			_, _ = s.store.DeleteExpiredReportSubmissions(ctx, s.now())
		}
	}
}
