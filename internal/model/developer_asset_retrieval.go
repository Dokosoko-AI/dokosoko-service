package model

import (
	"encoding/json"
	"strings"
	"time"
)

type SearchIndexGeneration struct {
	ID                      string          `json:"id"`
	DeploymentID            string          `json:"deployment_id"`
	PublicationKind         string          `json:"publication_kind"`
	PublicationID           string          `json:"publication_id"`
	AssetKind               string          `json:"asset_kind"`
	BuilderVersion          string          `json:"builder_version"`
	RetrievalProfileVersion string          `json:"retrieval_profile_version"`
	EmbeddingModel          string          `json:"embedding_model,omitempty"`
	EmbeddingDimensions     *int            `json:"embedding_dimensions,omitempty"`
	State                   string          `json:"state"`
	UnitCount               int             `json:"unit_count"`
	ContentHash             string          `json:"content_hash,omitempty"`
	Diagnostics             json.RawMessage `json:"diagnostics"`
	StartedAt               *time.Time      `json:"started_at,omitempty"`
	ReadyAt                 *time.Time      `json:"ready_at,omitempty"`
	CreatedAt               time.Time       `json:"created_at"`
}

func (generation SearchIndexGeneration) Valid() bool {
	if strings.TrimSpace(generation.PublicationKind) == "" || strings.TrimSpace(generation.PublicationID) == "" ||
		strings.TrimSpace(generation.BuilderVersion) == "" || strings.TrimSpace(generation.RetrievalProfileVersion) == "" {
		return false
	}
	if (generation.EmbeddingModel == "") != (generation.EmbeddingDimensions == nil) {
		return false
	}
	expectedAssetKind := map[string]string{
		"source":                   "documentation",
		"documentation_collection": "documentation",
		"global_documentation":     "documentation",
		"contract":                 "contract",
		"sdk":                      "sdk",
		"api":                      "mixed",
	}[generation.PublicationKind]
	if expectedAssetKind == "" || generation.AssetKind != expectedAssetKind {
		return false
	}
	return generation.State != "ready" || (generation.ReadyAt != nil && developerAssetHashPattern.MatchString(generation.ContentHash))
}

type KnowledgeUnit struct {
	ID                      string          `json:"id"`
	SearchIndexGenerationID string          `json:"search_index_generation_id"`
	DeploymentID            string          `json:"deployment_id"`
	Kind                    string          `json:"unit_kind"`
	SourcePublicationKind   string          `json:"source_publication_kind"`
	SourcePublicationID     string          `json:"source_publication_id"`
	SourceEntityID          string          `json:"source_entity_id"`
	ParentSourceEntityID    string          `json:"parent_source_entity_id,omitempty"`
	Title                   string          `json:"title,omitempty"`
	Breadcrumb              []string        `json:"breadcrumb"`
	Content                 string          `json:"content"`
	Embedding               []float32       `json:"-"`
	Language                string          `json:"language,omitempty"`
	Ecosystem               string          `json:"ecosystem,omitempty"`
	Identifiers             []string        `json:"identifiers"`
	Visibility              Visibility      `json:"visibility"`
	Citation                json.RawMessage `json:"citation"`
	Metadata                json.RawMessage `json:"metadata"`
	ContentHash             string          `json:"content_hash"`
	Ordinal                 int             `json:"ordinal"`
}

type KnowledgeUnitAPIScope struct {
	KnowledgeUnitID string    `json:"knowledge_unit_id"`
	DeploymentID    string    `json:"deployment_id"`
	APIID           string    `json:"api_id"`
	APISDKBindingID string    `json:"api_sdk_binding_id,omitempty"`
	ScopeKind       string    `json:"scope_kind"`
	SelectorHash    string    `json:"selector_hash,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type RetrievalQueryTrace struct {
	ID                                   string          `json:"id"`
	DeploymentID                         string          `json:"deployment_id"`
	DeploymentDocumentationPublicationID string          `json:"deployment_documentation_publication_id,omitempty"`
	APIDeveloperAssetPublicationID       string          `json:"api_developer_asset_publication_id,omitempty"`
	RetrievalProfileVersion              string          `json:"retrieval_profile_version"`
	QueryText                            string          `json:"query_text"`
	QueryHash                            string          `json:"query_hash"`
	RequestedFilters                     json.RawMessage `json:"requested_filters"`
	ResolvedScope                        json.RawMessage `json:"resolved_scope"`
	RoutingDecision                      json.RawMessage `json:"routing_decision"`
	State                                string          `json:"state"`
	CandidateCount                       int             `json:"candidate_count"`
	ResultCount                          int             `json:"result_count"`
	ContextTokens                        int             `json:"context_tokens"`
	LatencyMS                            int             `json:"latency_ms"`
	Diagnostics                          json.RawMessage `json:"diagnostics"`
	ExpiresAt                            *time.Time      `json:"expires_at,omitempty"`
	CreatedAt                            time.Time       `json:"created_at"`
}

type RetrievalQueryTraceResult struct {
	RetrievalQueryTraceID string          `json:"retrieval_query_trace_id"`
	Rank                  int             `json:"rank"`
	KnowledgeUnitID       string          `json:"knowledge_unit_id,omitempty"`
	SourcePublicationKind string          `json:"source_publication_kind"`
	SourcePublicationID   string          `json:"source_publication_id"`
	SourceEntityID        string          `json:"source_entity_id"`
	LexicalScore          *float64        `json:"lexical_score,omitempty"`
	SemanticScore         *float64        `json:"semantic_score,omitempty"`
	RerankScore           *float64        `json:"rerank_score,omitempty"`
	Selected              bool            `json:"selected"`
	ExclusionReason       string          `json:"exclusion_reason,omitempty"`
	Citation              json.RawMessage `json:"citation"`
	Excerpt               string          `json:"excerpt"`
	ContentHash           string          `json:"content_hash"`
}
