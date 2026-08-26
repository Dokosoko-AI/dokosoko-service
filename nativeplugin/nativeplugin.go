// Package nativeplugin defines the stable public contract for trusted Go
// source packages compiled into DokoSoko. Native plugins are application code,
// not a sandbox or a dynamic binary extension mechanism.
package nativeplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const SDKVersion = 1

type Effect string

const (
	EffectRead        Effect = "read"
	EffectWrite       Effect = "write"
	EffectDestructive Effect = "destructive"
)

type IdentityRequirement string

const (
	IdentityNone                     IdentityRequirement = "none"
	IdentityOptional                 IdentityRequirement = "optional"
	IdentityActorRequired            IdentityRequirement = "actor_required"
	IdentityCustomerRequired         IdentityRequirement = "customer_required"
	IdentityActorAndCustomerRequired IdentityRequirement = "actor_and_customer_required"
	IdentityInstallationRequired     IdentityRequirement = "installation_required"
)

type StateScope string

const (
	StateNone         StateScope = "none"
	StatePlugin       StateScope = "plugin"
	StateActor        StateScope = "actor"
	StateCustomer     StateScope = "customer"
	StateInstallation StateScope = "installation"
)

type Idempotency string

const (
	IdempotencyNone      Idempotency = "none"
	IdempotencySupported Idempotency = "supported"
	IdempotencyRequired  Idempotency = "required"
)

type ConfigType string

const (
	ConfigString   ConfigType = "string"
	ConfigSecret   ConfigType = "secret"
	ConfigBoolean  ConfigType = "boolean"
	ConfigInteger  ConfigType = "integer"
	ConfigDuration ConfigType = "duration"
	ConfigURL      ConfigType = "url"
)

type Capability string

const (
	CapabilityNetwork Capability = "network"
)

type ConfigSpec struct {
	Key         string     `json:"key"`
	Type        ConfigType `json:"type"`
	Required    bool       `json:"required"`
	Description string     `json:"description"`
}

type NetworkClaim struct {
	Host      string `json:"host,omitempty"`
	ConfigKey string `json:"config_key,omitempty"`
}

type ToolSpec struct {
	ID                   string              `json:"id"`
	Namespace            string              `json:"namespace"`
	Name                 string              `json:"name"`
	Description          string              `json:"description"`
	InputSchema          json.RawMessage     `json:"input_schema"`
	OutputSchema         json.RawMessage     `json:"output_schema"`
	Effect               Effect              `json:"effect"`
	Identity             IdentityRequirement `json:"identity"`
	StateScope           StateScope          `json:"state_scope"`
	RequiredGrants       []string            `json:"required_grants,omitempty"`
	ConfirmationRequired bool                `json:"confirmation_required"`
	Idempotency          Idempotency         `json:"idempotency"`
	Timeout              time.Duration       `json:"timeout"`
	MaxConcurrency       int                 `json:"max_concurrency"`
	MaxResultBytes       int64               `json:"max_result_bytes"`
}

type Manifest struct {
	ID           string         `json:"id"`
	Version      string         `json:"version"`
	SDKVersion   int            `json:"sdk_version"`
	Description  string         `json:"description"`
	Config       []ConfigSpec   `json:"config"`
	StateVersion uint32         `json:"state_version"`
	Network      []NetworkClaim `json:"network"`
	Capabilities []Capability   `json:"capabilities"`
	Tools        []ToolSpec     `json:"tools"`
}

type Plugin interface {
	// Describe must be deterministic and side-effect free.
	Describe() Manifest
	Open(context.Context, Host) (Instance, error)
}

type Instance interface {
	Invoke(context.Context, Invocation) (Result, error)
	Close(context.Context) error
}

// StateUpgrader is optional. DokoSoko invokes it transactionally before Open.
type StateUpgrader interface {
	UpgradeState(context.Context, uint32, uint32, UpgradeStore) error
}

type Host interface {
	Config() Config
	Logger() Logger
	Clock() Clock
	HTTP() HTTPClient
}

type HTTPClient interface {
	Do(context.Context, HTTPRequest) (HTTPResponse, error)
}

type HTTPRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

type HTTPResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
}

type Clock interface {
	Now() time.Time
}

type Logger interface {
	Debug(string, ...Field)
	Info(string, ...Field)
	Warn(string, ...Field)
	Error(string, ...Field)
}

type Field struct {
	Key   string
	Value any
}

func F(key string, value any) Field { return Field{Key: key, Value: value} }

type IdentityRef struct {
	ID string `json:"id"`
}

type IdentityView struct {
	Actor        *IdentityRef `json:"actor,omitempty"`
	Customer     *IdentityRef `json:"customer,omitempty"`
	Installation *IdentityRef `json:"installation,omitempty"`
}

type Invocation struct {
	ToolID         string
	Arguments      map[string]any
	Identity       IdentityView
	State          State
	RequestID      string
	IdempotencyKey string
}

type Result struct {
	Structured map[string]any
}

type ErrorCode string

const (
	ErrorInvalidArgument ErrorCode = "invalid_argument"
	ErrorNotFound        ErrorCode = "not_found"
	ErrorConflict        ErrorCode = "conflict"
	ErrorUnauthorized    ErrorCode = "unauthorized"
	ErrorRateLimited     ErrorCode = "rate_limited"
	ErrorTemporary       ErrorCode = "temporary"
	ErrorInternal        ErrorCode = "internal"
)

// CallError separates an MCP-safe message from an internal diagnostic cause.
type CallError struct {
	Code        ErrorCode
	SafeMessage string
	Cause       error
}

func (e *CallError) Error() string {
	if e == nil {
		return ""
	}
	if e.SafeMessage != "" {
		return e.SafeMessage
	}
	return string(e.Code)
}

func (e *CallError) Unwrap() error { return e.Cause }

func Fail(code ErrorCode, safeMessage string, cause error) error {
	return &CallError{Code: code, SafeMessage: safeMessage, Cause: cause}
}

func (m Manifest) Tool(id string) (ToolSpec, bool) {
	for _, tool := range m.Tools {
		if tool.ID == id {
			return tool, true
		}
	}
	return ToolSpec{}, false
}

func (t ToolSpec) FullName() string { return t.Namespace + "." + t.Name }

func (m Manifest) String() string { return fmt.Sprintf("%s@%s", m.ID, m.Version) }
