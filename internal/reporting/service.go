package reporting

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const (
	KindBug      = "bug"
	KindFeedback = "feedback"
)

var (
	ErrInvalidReport    = errors.New("report is invalid")
	ErrSensitiveContent = errors.New("report may contain a credential or secret")
	ErrDeliveryDisabled = errors.New("support submission delivery is not configured")

	toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,160}$`)
	secretPatterns  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password)\s*[:=]\s*(bearer\s+)?[A-Za-z0-9_./+\-=]{8,}`),
		regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9_./+\-=]{12,}`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----`),
		regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
	}
)

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
	Severity          string   `json:"severity,omitempty"`
	AllowContact      bool     `json:"allow_contact,omitempty"`
	IdempotencyKey    string   `json:"idempotency_key,omitempty"`
}

type FeedbackInput struct {
	IntegrationID  string `json:"integration_id,omitempty"`
	Message        string `json:"message"`
	Category       string `json:"category,omitempty"`
	Rating         *int   `json:"rating,omitempty"`
	RelatedTool    string `json:"related_tool,omitempty"`
	AllowContact   bool   `json:"allow_contact,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
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
	ProductID       string `json:"product_id"`
	ProductName     string `json:"product_name"`
	CatalogRevision int64  `json:"catalog_revision,omitempty"`
}

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

type Envelope struct {
	SchemaVersion string              `json:"schema_version"`
	Kind          string              `json:"kind"`
	Bug           *BugInput           `json:"bug,omitempty"`
	Feedback      *FeedbackInput      `json:"feedback,omitempty"`
	Reporter      ReporterContext     `json:"reporter"`
	Product       ProductContext      `json:"product"`
	Integration   *IntegrationContext `json:"integration,omitempty"`
	Channel       string              `json:"channel"`
	ConfirmedAt   time.Time           `json:"confirmed_at"`
	RequestID     string              `json:"request_id"`
}

type SubmissionView struct {
	ID                 string              `json:"id"`
	Kind               string              `json:"kind"`
	State              string              `json:"state"`
	Attempts           int                 `json:"attempts"`
	LastError          string              `json:"last_error,omitempty"`
	DeliveredAt        *time.Time          `json:"delivered_at,omitempty"`
	Summary            string              `json:"summary"`
	Category           string              `json:"category,omitempty"`
	Rating             *int                `json:"rating,omitempty"`
	RelatedTool        string              `json:"related_tool,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
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
	BugReportsEnabled bool   `json:"bug_reports_enabled"`
	FeedbackEnabled   bool   `json:"feedback_enabled"`
}

type Service struct {
	store    store.Store
	now      func() time.Time
	resolver interface {
		LookupIP(context.Context, string, string) ([]net.IP, error)
	}
	doer interface {
		Do(*http.Request) (*http.Response, error)
	}
}

func New(repository store.Store) *Service {
	return &Service{store: repository, now: func() time.Time { return time.Now().UTC() }, resolver: net.DefaultResolver}
}

func (s *Service) Capabilities(ctx context.Context, deploymentID string) ([]SupportCapability, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil || deployment.ID != deploymentID {
		return nil, store.ErrNotFound
	}
	return []SupportCapability{{Scope: "deployment", BugReportsEnabled: deployment.ErrorSubmissionURL != "", FeedbackEnabled: deployment.FeedbackSubmissionURL != ""}}, nil
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
