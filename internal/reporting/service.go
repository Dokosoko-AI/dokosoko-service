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
	"strconv"
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
	maxDeliveryResponse = 1 << 20
)

var (
	ErrNotConfigured       = errors.New("reporting is not configured")
	ErrDisabled            = errors.New("this report type is disabled")
	ErrInvalidReport       = errors.New("report is invalid")
	ErrSensitiveContent    = errors.New("report may contain a credential or secret")
	ErrDeliveryUnavailable = errors.New("support delivery is unavailable")

	toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,160}$`)
	secretPatterns  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password)\s*[:=]\s*(bearer\s+)?[A-Za-z0-9_./+\-=]{8,}`),
		regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9_./+\-=]{12,}`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----`),
		regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
	}
)

type RouteInput struct {
	Name                string
	IsDefault           bool
	BugReportsEnabled   bool
	FeedbackEnabled     bool
	BackendConnectionID string
	RetentionDays       int
	State               string
	IntegrationIDs      []string
	Revision            int64
}

type BugInput struct {
	IntegrationID     string   `json:"integration_id,omitempty"`
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
	IntegrationID    string `json:"integration_id,omitempty"`
	Message          string `json:"message"`
	Category         string `json:"category,omitempty"`
	Rating           *int   `json:"rating,omitempty"`
	RelatedTool      string `json:"related_tool,omitempty"`
	IntegrationRunID string `json:"integration_run_id,omitempty"`
	AllowContact     bool   `json:"allow_contact,omitempty"`
	IdempotencyKey   string `json:"idempotency_key,omitempty"`
}

type ReporterPrincipal struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}

type ReporterContext struct {
	Principal          ReporterPrincipal `json:"principal"`
	DisplayName        string            `json:"display_name,omitempty"`
	Email              string            `json:"email,omitempty"`
	ExternalCustomerID string            `json:"external_customer_id,omitempty"`
	InstallationID     string            `json:"installation_id,omitempty"`
	AllowContact       bool              `json:"allow_contact"`
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

// ProviderContext identifies DokoSoko at the vendor-neutral support-delivery
// boundary without requiring the receiver to understand DokoSoko's catalog.
type ProviderContext struct {
	Key     string `json:"key"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

// ResourceContext is the provider-neutral identity of a deployment or related
// API that was active when the customer confirmed the report.
type ResourceContext struct {
	Type           string `json:"type"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	VersionID      string `json:"version_id,omitempty"`
	Version        string `json:"version,omitempty"`
	EnvironmentID  string `json:"environment_id,omitempty"`
	InstallationID string `json:"installation_id,omitempty"`
	State          string `json:"state,omitempty"`
	Revision       int64  `json:"revision,omitempty"`
}

// IntegrationContext is resolved by DokoSoko, never trusted from agent-authored
// diagnostic text. It pins support submissions to the API family/version and
// immutable Integration revision that were active at submission time.
type IntegrationContext struct {
	IntegrationID string          `json:"integration_id"`
	FamilyKey     string          `json:"family_key"`
	VersionKey    string          `json:"version_key"`
	DisplayName   string          `json:"display_name"`
	Lifecycle     string          `json:"lifecycle"`
	Revision      int64           `json:"revision"`
	ManifestHash  string          `json:"manifest_hash,omitempty"`
	Snapshot      json.RawMessage `json:"snapshot,omitempty"`
}

// DokoSokoExtension contains catalog details that only the DokoSoko adapter
// profile understands. Receivers may preserve it as opaque JSON.
type DokoSokoExtension struct {
	ManifestHash    string              `json:"manifest_hash,omitempty"`
	CatalogRevision int64               `json:"catalog_revision,omitempty"`
	SelectionSource string              `json:"selection_source,omitempty"`
	Integration     *IntegrationContext `json:"integration,omitempty"`
}

type EnvelopeExtensions struct {
	DokoSoko DokoSokoExtension `json:"dokosoko"`
}

type Envelope struct {
	SchemaVersion    string              `json:"schema_version"`
	Kind             string              `json:"kind"`
	Bug              *BugInput           `json:"bug,omitempty"`
	Feedback         *FeedbackInput      `json:"feedback,omitempty"`
	Reporter         ReporterContext     `json:"reporter"`
	Provider         ProviderContext     `json:"provider"`
	Resource         ResourceContext     `json:"resource"`
	RelatedResources []ResourceContext   `json:"related_resources,omitempty"`
	Channel          string              `json:"channel"`
	Extensions       *EnvelopeExtensions `json:"extensions,omitempty"`
	ConfirmedAt      time.Time           `json:"confirmed_at"`
	RequestID        string              `json:"request_id"`

	// Legacy fields are decoded only so already-encrypted development records
	// remain readable. New envelopes never populate or deliver them.
	LegacyProduct     *ProductContext     `json:"product,omitempty"`
	LegacyIntegration *IntegrationContext `json:"integration,omitempty"`
	LegacySource      string              `json:"source,omitempty"`
}

type SubmissionView struct {
	ID                 string              `json:"id"`
	SupportRouteID     string              `json:"support_route_id"`
	Kind               string              `json:"kind"`
	State              string              `json:"state"`
	Summary            string              `json:"summary"`
	Category           string              `json:"category,omitempty"`
	Rating             *int                `json:"rating,omitempty"`
	RelatedTool        string              `json:"related_tool,omitempty"`
	Attempts           int                 `json:"attempts"`
	LastError          string              `json:"last_error,omitempty"`
	ExternalID         string              `json:"external_id,omitempty"`
	ExternalURL        string              `json:"external_url,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	DeliveredAt        *time.Time          `json:"delivered_at,omitempty"`
	ExpiresAt          time.Time           `json:"expires_at"`
	Content            map[string]any      `json:"content,omitempty"`
	TrustedContext     ProductContext      `json:"trusted_context"`
	TrustedIntegration *IntegrationContext `json:"trusted_integration,omitempty"`
}

type SubmitContext struct {
	Principal      identity.Principal
	ActorPseudonym string
	Product        ProductContext
	Integration    *IntegrationContext
	RequestID      string
}

type SupportCapability struct {
	Scope             string `json:"scope"`
	SupportRouteID    string `json:"support_route_id"`
	IntegrationID     string `json:"integration_id,omitempty"`
	FamilyKey         string `json:"family_key,omitempty"`
	VersionKey        string `json:"version_key,omitempty"`
	BugReportsEnabled bool   `json:"bug_reports_enabled"`
	FeedbackEnabled   bool   `json:"feedback_enabled"`
}

func (s *Service) Capabilities(ctx context.Context, deploymentID string) ([]SupportCapability, error) {
	result := make([]SupportCapability, 0)
	defaultRoute, defaultErr := s.store.SupportRouteForIntegration(ctx, deploymentID, "")
	if defaultErr == nil && (defaultRoute.BugReportsEnabled || defaultRoute.FeedbackEnabled) {
		result = append(result, SupportCapability{Scope: "default", SupportRouteID: defaultRoute.ID, BugReportsEnabled: defaultRoute.BugReportsEnabled, FeedbackEnabled: defaultRoute.FeedbackEnabled})
	} else if defaultErr != nil && !errors.Is(defaultErr, store.ErrNotFound) {
		return nil, defaultErr
	}
	integrations, err := s.store.Integrations(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	for _, integration := range integrations {
		if integration.Lifecycle == "retired" {
			continue
		}
		route, routeErr := s.store.SupportRouteForIntegration(ctx, deploymentID, integration.ID)
		if routeErr != nil {
			if errors.Is(routeErr, store.ErrNotFound) {
				continue
			}
			return nil, routeErr
		}
		if route.BugReportsEnabled || route.FeedbackEnabled {
			result = append(result, SupportCapability{Scope: "integration", SupportRouteID: route.ID, IntegrationID: integration.ID, FamilyKey: integration.FamilyKey, VersionKey: integration.VersionKey, BugReportsEnabled: route.BugReportsEnabled, FeedbackEnabled: route.FeedbackEnabled})
		}
	}
	return result, nil
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

func requestID() string {
	buffer := make([]byte, 16)
	_, _ = rand.Read(buffer)
	return "req_" + hex.EncodeToString(buffer)
}

func validDeliveryURL(raw string) bool {
	if raw == "" {
		return true
	}
	parsed, err := url.Parse(raw)
	local := err == nil && identity.IsLocalDevelopmentHostname(parsed.Hostname())
	return err == nil && ((parsed.Scheme == "https" && (parsed.Port() == "" || parsed.Port() == "443")) || (parsed.Scheme == "http" && local)) && parsed.Hostname() != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func (s *Service) SaveRoute(ctx context.Context, deploymentID, routeID string, input RouteInput, actorID, requestID string) (model.SupportRoute, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil || deployment.ID != deploymentID {
		return model.SupportRoute{}, store.ErrNotFound
	}
	input.Name, input.BackendConnectionID = strings.TrimSpace(input.Name), strings.TrimSpace(input.BackendConnectionID)
	if input.Name == "" || len(input.Name) > 120 {
		return model.SupportRoute{}, fmt.Errorf("%w: route name is invalid", ErrInvalidReport)
	}
	if input.RetentionDays == 0 {
		input.RetentionDays = 30
	}
	if input.RetentionDays < 1 || input.RetentionDays > 365 {
		return model.SupportRoute{}, fmt.Errorf("%w: retention_days must be between 1 and 365", ErrInvalidReport)
	}
	if input.State == "" {
		input.State = "active"
	}
	if input.State != "active" && input.State != "archived" {
		return model.SupportRoute{}, fmt.Errorf("%w: support route state is invalid", ErrInvalidReport)
	}
	if input.BugReportsEnabled || input.FeedbackEnabled {
		connection, connectionErr := s.store.BackendConnection(ctx, deploymentID, input.BackendConnectionID)
		if connectionErr != nil || connection.State != "active" || connection.AuthenticationType != "bearer" || connection.CredentialSecretID == "" {
			return model.SupportRoute{}, fmt.Errorf("%w: an active authenticated backend connection is required when support submission is enabled", ErrInvalidReport)
		}
	}
	if !input.IsDefault && len(input.IntegrationIDs) == 0 {
		return model.SupportRoute{}, fmt.Errorf("%w: a non-default route must be attached to at least one Integration", ErrInvalidReport)
	}
	seen := make(map[string]bool, len(input.IntegrationIDs))
	integrationIDs := make([]string, 0, len(input.IntegrationIDs))
	for _, integrationID := range input.IntegrationIDs {
		integrationID = strings.TrimSpace(integrationID)
		if integrationID == "" || seen[integrationID] {
			continue
		}
		if _, err := s.store.Integration(ctx, deploymentID, integrationID); err != nil {
			return model.SupportRoute{}, fmt.Errorf("%w: support route Integration does not exist", ErrInvalidReport)
		}
		seen[integrationID] = true
		integrationIDs = append(integrationIDs, integrationID)
	}
	value := model.SupportRoute{ID: routeID, DeploymentID: deploymentID, OrganisationID: deployment.OrganisationID, Name: input.Name, IsDefault: input.IsDefault, BugReportsEnabled: input.BugReportsEnabled, FeedbackEnabled: input.FeedbackEnabled, BackendConnectionID: input.BackendConnectionID, RetentionDays: input.RetentionDays, State: input.State, IntegrationIDs: integrationIDs}
	if routeID == "" {
		if input.Revision != 0 {
			return model.SupportRoute{}, store.ErrConflict
		}
		value.ID, err = randomUUID()
		if err != nil {
			return model.SupportRoute{}, err
		}
	} else {
		current, currentErr := s.store.SupportRoute(ctx, deploymentID, routeID)
		if currentErr != nil {
			return model.SupportRoute{}, currentErr
		}
		if current.Revision != input.Revision {
			return model.SupportRoute{}, store.ErrConflict
		}
	}
	saved, err := s.store.SaveSupportRoute(ctx, value, input.Revision)
	if err != nil {
		return model.SupportRoute{}, err
	}
	if saved.State == "active" && saved.BackendConnectionID != "" {
		if saved.BugReportsEnabled {
			_ = s.store.ActivateHeldReportSubmissions(ctx, deploymentID, saved.ID, KindBug, s.now())
		}
		if saved.FeedbackEnabled {
			_ = s.store.ActivateHeldReportSubmissions(ctx, deploymentID, saved.ID, KindFeedback, s.now())
		}
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: auditID(), OrganisationID: deployment.OrganisationID, ProductID: deploymentID, ActorID: actorID, Action: "support_route.saved", TargetType: "support_route", TargetID: saved.ID, Current: map[string]any{"name": saved.Name, "is_default": saved.IsDefault, "integration_ids": saved.IntegrationIDs, "backend_connection_id": saved.BackendConnectionID, "bug_reports_enabled": saved.BugReportsEnabled, "feedback_enabled": saved.FeedbackEnabled}, RequestID: requestID, CreatedAt: s.now()}); err != nil {
		return model.SupportRoute{}, err
	}
	return saved, nil
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

func (s *Service) ProcessPending(ctx context.Context, limit int) (int, error) {
	values, err := s.store.ClaimReportSubmissions(ctx, s.now(), limit)
	if err != nil {
		return 0, err
	}
	for index, value := range values {
		if err := s.deliver(ctx, value); err != nil {
			return index, fmt.Errorf("persist report delivery state for %s: %w", value.ID, err)
		}
	}
	return len(values), nil
}

func (s *Service) deliver(ctx context.Context, value model.ReportSubmission) error {
	endpoint, credentialID, err := s.deliveryRoute(ctx, value)
	if err != nil {
		if errors.Is(err, ErrDeliveryUnavailable) || errors.Is(err, ErrNotConfigured) || errors.Is(err, store.ErrNotFound) {
			value.State, value.NextAttemptAt, value.DeliveryStartedAt, value.LastError = "held", nil, nil, ""
			_, updateErr := s.store.UpdateReportSubmissionDelivery(ctx, value)
			return updateErr
		}
		return s.deliveryFailed(ctx, value, err)
	}
	if endpoint == "" || credentialID == "" {
		value.State, value.NextAttemptAt, value.DeliveryStartedAt, value.LastError = "held", nil, nil, ""
		_, updateErr := s.store.UpdateReportSubmissionDelivery(ctx, value)
		return updateErr
	}
	envelope, err := s.decrypt(value)
	if err != nil {
		return s.deliveryFailed(ctx, value, errors.New("encrypted report payload could not be opened"))
	}
	credential, err := s.deliveryCredential(ctx, value.OrganisationID, credentialID)
	if err != nil {
		return s.deliveryFailed(ctx, value, errors.New("support delivery credential is unavailable"))
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || !validDeliveryURL(endpoint) {
		return s.deliveryFailed(ctx, value, errors.New("support API destination is unsafe"))
	}
	body, _ := json.Marshal(map[string]any{"submission_id": value.ID, "created_at": value.CreatedAt, "submission": envelope})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return s.deliveryFailed(ctx, value, err)
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Idempotency-Key", value.ID)
	request.Header.Set("X-External-Request-ID", requestID())
	client, err := identity.SafeOutboundClient(ctx, parsed, s.Client, s.Resolver)
	if err != nil {
		return s.deliveryFailed(ctx, value, errors.New("support API destination is unsafe"))
	}
	response, err := client.Do(request)
	if err != nil {
		return s.deliveryFailed(ctx, value, errors.New("support API request failed"))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		failure := fmt.Errorf("support API returned status %d", response.StatusCode)
		if response.StatusCode != http.StatusRequestTimeout && response.StatusCode != http.StatusTooManyRequests && response.StatusCode < 500 {
			return s.deliveryFailedPermanently(ctx, value, failure)
		}
		return s.deliveryFailedAfter(ctx, value, failure, retryAfter(response.Header.Get("Retry-After"), s.now()))
	}
	result := struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		ExternalID  string `json:"external_id"`
		ExternalURL string `json:"external_url"`
	}{}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxDeliveryResponse+1))
	if readErr != nil || len(raw) > maxDeliveryResponse {
		return s.deliveryFailed(ctx, value, errors.New("support API response is too large"))
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		if err := decoder.Decode(&result); err != nil {
			return s.deliveryFailed(ctx, value, errors.New("support API returned invalid JSON"))
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			return s.deliveryFailed(ctx, value, errors.New("support API returned multiple JSON values"))
		}
	}
	if result.ID == "" || len(result.ID) > 200 || result.Status != "accepted" || len(result.ExternalID) > 200 || len(result.ExternalURL) > 2000 || (result.ExternalURL != "" && !validExternalURL(result.ExternalURL)) {
		return s.deliveryFailed(ctx, value, errors.New("support API returned invalid receipt"))
	}
	now := s.now()
	value.State, value.NextAttemptAt, value.DeliveryStartedAt, value.LastError = "delivered", nil, nil, ""
	value.ExternalID, value.ExternalURL, value.DeliveredAt = result.ExternalID, result.ExternalURL, &now
	_, err = s.store.UpdateReportSubmissionDelivery(ctx, value)
	return err
}

func (s *Service) deliveryRoute(ctx context.Context, value model.ReportSubmission) (endpoint, credentialID string, err error) {
	if value.SupportRouteID == "" {
		return "", "", ErrNotConfigured
	}
	route, routeErr := s.store.SupportRoute(ctx, value.ProductID, value.SupportRouteID)
	if routeErr != nil {
		return "", "", routeErr
	}
	enabled := route.BugReportsEnabled
	if value.Kind == KindFeedback {
		enabled = route.FeedbackEnabled
	}
	if route.State != "active" || !enabled || route.BackendConnectionID == "" {
		return "", "", ErrDeliveryUnavailable
	}
	connection, connectionErr := s.store.BackendConnection(ctx, value.ProductID, route.BackendConnectionID)
	if connectionErr != nil || connection.State != "active" || connection.AuthenticationType != "bearer" || connection.CredentialSecretID == "" {
		return "", "", ErrDeliveryUnavailable
	}
	return connection.BaseURL + "/v1/support-submissions", connection.CredentialSecretID, nil
}

func validExternalURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil
}

func (s *Service) deliveryCredential(ctx context.Context, organisationID, id string) (string, error) {
	if s.vault == nil {
		return "", errors.New("credential vault is unavailable")
	}
	stored, err := s.store.Secret(ctx, organisationID, id)
	if err != nil {
		return "", err
	}
	plain, err := s.vault.Decrypt(secrets.Encrypted{Ciphertext: stored.Ciphertext, Nonce: stored.Nonce, KeyVersion: stored.KeyVersion, Fingerprint: stored.Fingerprint}, organisationID+":backend_connection:"+id)
	return string(plain), err
}

func (s *Service) deliveryFailed(ctx context.Context, value model.ReportSubmission, failure error) error {
	return s.deliveryFailedAfter(ctx, value, failure, time.Time{})
}

func (s *Service) deliveryFailedPermanently(ctx context.Context, value model.ReportSubmission, failure error) error {
	value.Attempts = max(value.Attempts, maxDeliveryAttempts)
	return s.deliveryFailedAfter(ctx, value, failure, time.Time{})
}

func retryAfter(raw string, now time.Time) time.Time {
	if seconds, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && seconds >= 0 {
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if parsed, err := http.ParseTime(raw); err == nil && parsed.After(now) {
		return parsed
	}
	return time.Time{}
}

func (s *Service) deliveryFailedAfter(ctx context.Context, value model.ReportSubmission, failure error, retryAt time.Time) error {
	value.DeliveryStartedAt = nil
	message := failure.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	value.LastError = message
	if value.Attempts >= maxDeliveryAttempts {
		value.State, value.NextAttemptAt = "failed", nil
	} else {
		next := retryAt
		if next.IsZero() {
			delay := time.Minute * time.Duration(1<<min(value.Attempts-1, 8))
			if delay > 6*time.Hour {
				delay = 6 * time.Hour
			}
			next = s.now().Add(delay)
		}
		value.State, value.NextAttemptAt = "pending", &next
	}
	_, err := s.store.UpdateReportSubmissionDelivery(ctx, value)
	return err
}

func (s *Service) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := s.ProcessPending(ctx, 50); err != nil {
				return fmt.Errorf("process pending reports: %w", err)
			}
			if _, err := s.store.DeleteExpiredReportSubmissions(ctx, s.now()); err != nil {
				return fmt.Errorf("delete expired reports: %w", err)
			}
		}
	}
}
