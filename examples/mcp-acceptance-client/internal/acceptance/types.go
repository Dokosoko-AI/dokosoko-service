package acceptance

import (
	"encoding/json"
	"net/http"
	"time"
)

const ProtocolVersion = "2026-07-28"

type Config struct {
	Endpoint                   string
	AllowedLoopbackHTTP        []string
	Origin                     string
	Token                      string
	RestrictedToken            string
	ExpectedTools              []string
	ExpectedResources          []string
	CallTool                   string
	CallArguments              map[string]any
	CallConfirmed              bool
	GrantTool                  string
	GrantArguments             map[string]any
	VerifyRestrictedCallDenied bool
	ConfirmationTool           string
	ConfirmationArguments      map[string]any
	VerifyConfirmedCall        bool
	CheckUnauthenticated       bool
	Timeout                    time.Duration
	HTTPClient                 *http.Client
}

type Status string

const (
	Pass Status = "pass"
	Fail Status = "fail"
	Skip Status = "skip"
)

type Check struct {
	Name              string `json:"name"`
	Status            Status `json:"status"`
	Required          bool   `json:"required"`
	Detail            string `json:"detail,omitempty"`
	RequestID         string `json:"request_id,omitempty"`
	ResponseRequestID string `json:"response_request_id,omitempty"`
	HTTPStatus        int    `json:"http_status,omitempty"`
	RPCErrorCode      *int   `json:"rpc_error_code,omitempty"`
	DurationMS        int64  `json:"duration_ms"`
}

type Summary struct {
	Passed          int `json:"passed"`
	Failed          int `json:"failed"`
	Skipped         int `json:"skipped"`
	RequiredSkipped int `json:"required_skipped"`
}

type Report struct {
	Endpoint        string    `json:"endpoint"`
	ProtocolVersion string    `json:"protocol_version"`
	StartedAt       time.Time `json:"started_at"`
	DurationMS      int64     `json:"duration_ms"`
	Checks          []Check   `json:"checks"`
	Summary         Summary   `json:"summary"`
}

func (r *Report) Add(check Check) {
	r.Checks = append(r.Checks, check)
	switch check.Status {
	case Pass:
		r.Summary.Passed++
	case Fail:
		r.Summary.Failed++
	case Skip:
		r.Summary.Skipped++
		if check.Required {
			r.Summary.RequiredSkipped++
		}
	}
}

// Accepted reports whether every required check ran and every executed check
// passed. Optional skipped checks are retained as evidence but do not make an
// otherwise valid acceptance run fail.
func (r Report) Accepted() bool {
	return r.Summary.Failed == 0 && r.Summary.RequiredSkipped == 0
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type callOutcome struct {
	RequestID         string
	ResponseRequestID string
	HTTPStatus        int
	Duration          time.Duration
	Response          rpcResponse
	TransportError    error
}
