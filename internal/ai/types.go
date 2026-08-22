package ai

import (
	"context"
	"encoding/json"
	"time"
)

type Workload string

const (
	WorkloadExtraction Workload = "extraction"
	WorkloadAuthoring  Workload = "authoring"
	WorkloadReview     Workload = "review"
	WorkloadSupport    Workload = "support"
)

func ValidWorkload(value string) bool {
	switch Workload(value) {
	case WorkloadExtraction, WorkloadAuthoring, WorkloadReview, WorkloadSupport:
		return true
	default:
		return false
	}
}

type ProviderConfig struct {
	Provider   string
	Endpoint   string
	Credential string
}

type TextRequest struct {
	Provider        ProviderConfig
	Model           string
	System          string
	User            string
	MaxOutputTokens int
	Temperature     float64
}

type StructuredRequest struct {
	Provider        ProviderConfig
	Model           string
	System          string
	User            string
	SchemaName      string
	Schema          json.RawMessage
	MaxOutputTokens int
	Temperature     float64
}

type Result struct {
	Text           string
	JSON           json.RawMessage
	Provider       string
	RequestedModel string
	ResolvedModel  string
	RequestID      string
	FinishReason   string
	InputTokens    int64
	OutputTokens   int64
	Duration       time.Duration
}

type TextStream interface {
	Next() bool
	Result() Result
	Err() error
	Close() error
}

type Runtime interface {
	GenerateStructured(context.Context, StructuredRequest) (Result, error)
	GenerateText(context.Context, TextRequest) (Result, error)
	StreamText(context.Context, TextRequest) (TextStream, error)
}

type Adapter interface {
	GenerateStructured(context.Context, StructuredRequest) (Result, error)
	GenerateText(context.Context, TextRequest) (Result, error)
}
