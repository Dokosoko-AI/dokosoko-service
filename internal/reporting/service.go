package reporting

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
