package backend

import (
	"context"
	"encoding/json"
	"errors"
)

var (
	ErrIdempotencyConflict = errors.New("idempotency key was already used with different content")
	ErrSubmissionConflict  = errors.New("submission ID was already accepted under another idempotency key")
)

type Acceptor interface {
	Accept(context.Context, AcceptInput) (AcceptResult, error)
	Ready(context.Context) error
}

type AcceptInput struct {
	IdempotencyKey string
	RequestID      string
	RequestSHA256  []byte
	CanonicalBody  []byte
	ReceiptBody    []byte
	ReceiptID      string
	SubmissionID   string
	Kind           string
}

type AcceptResult struct {
	StatusCode int
	Body       []byte
	Replayed   bool
}

type SupportSubmissionRequest struct {
	SubmissionID string            `json:"submission_id"`
	CreatedAt    string            `json:"created_at"`
	Submission   SupportSubmission `json:"submission"`
}

type SupportSubmission struct {
	SchemaVersion string              `json:"schema_version"`
	Kind          string              `json:"kind"`
	Bug           *BugReport          `json:"bug,omitempty"`
	Feedback      *FeedbackReport     `json:"feedback,omitempty"`
	Reporter      ReporterContext     `json:"reporter"`
	Product       ProductContext      `json:"product"`
	Integration   *IntegrationContext `json:"integration,omitempty"`
	Source        string              `json:"source"`
	ConfirmedAt   string              `json:"confirmed_at"`
	RequestID     string              `json:"request_id"`
}

type ReporterContext struct {
	Principal          ReporterPrincipal `json:"principal"`
	DisplayName        string            `json:"display_name,omitempty"`
	Email              string            `json:"email,omitempty"`
	ExternalCustomerID string            `json:"external_customer_id,omitempty"`
	InstallationID     string            `json:"installation_id,omitempty"`
	AllowContact       *bool             `json:"allow_contact"`
}

type ReporterPrincipal struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
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

type IntegrationContext struct {
	IntegrationID string                     `json:"integration_id"`
	FamilyKey     string                     `json:"family_key"`
	VersionKey    string                     `json:"version_key"`
	DisplayName   string                     `json:"display_name"`
	Lifecycle     string                     `json:"lifecycle"`
	Revision      int64                      `json:"revision"`
	ManifestHash  string                     `json:"manifest_hash,omitempty"`
	Snapshot      map[string]json.RawMessage `json:"snapshot,omitempty"`
}

type BugReport struct {
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
	Severity          *string  `json:"severity,omitempty"`
	AllowContact      *bool    `json:"allow_contact,omitempty"`
}

type FeedbackReport struct {
	Message          string  `json:"message"`
	Category         *string `json:"category,omitempty"`
	Rating           *int    `json:"rating,omitempty"`
	RelatedTool      string  `json:"related_tool,omitempty"`
	IntegrationRunID string  `json:"integration_run_id,omitempty"`
	AllowContact     *bool   `json:"allow_contact,omitempty"`
}

type SupportSubmissionReceipt struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	ExternalID  string `json:"external_id,omitempty"`
	ExternalURL string `json:"external_url,omitempty"`
}

type ErrorEnvelope struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}
