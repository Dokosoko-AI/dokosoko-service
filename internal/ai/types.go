package ai

import (
	"context"
	"encoding/json"
	"time"
)

type Workload string

const (
	WorkloadAnalysis Workload = "analysis"
)

func ValidWorkload(value string) bool {
	switch Workload(value) {
	case WorkloadAnalysis:
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

type Runtime interface {
	GenerateStructured(context.Context, StructuredRequest) (Result, error)
}

type Adapter interface {
	GenerateStructured(context.Context, StructuredRequest) (Result, error)
}
