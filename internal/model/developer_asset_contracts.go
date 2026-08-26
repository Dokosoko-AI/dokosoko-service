package model

import (
	"encoding/json"
	"strings"
	"time"
)

type APIContract struct {
	ID             string     `json:"id"`
	DeploymentID   string     `json:"deployment_id"`
	OrganisationID string     `json:"organisation_id"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Description    string     `json:"description"`
	Kind           string     `json:"contract_kind"`
	Visibility     Visibility `json:"visibility"`
	Lifecycle      string     `json:"lifecycle"`
	Revision       int64      `json:"revision"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type APIContractSource struct {
	ID            string    `json:"id"`
	DeploymentID  string    `json:"deployment_id"`
	APIContractID string    `json:"api_contract_id"`
	SourceID      string    `json:"source_id"`
	SourceRole    string    `json:"source_role"`
	Lifecycle     string    `json:"lifecycle"`
	Revision      int64     `json:"revision"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (binding APIContractSource) Valid() bool {
	return strings.TrimSpace(binding.DeploymentID) != "" &&
		strings.TrimSpace(binding.APIContractID) != "" &&
		strings.TrimSpace(binding.SourceID) != "" &&
		(binding.SourceRole == "primary" || binding.SourceRole == "supplemental") &&
		(binding.Lifecycle == "attached" || binding.Lifecycle == "detached") &&
		binding.Revision > 0
}

type APIContractCandidate struct {
	ID                 string          `json:"id"`
	DeploymentID       string          `json:"deployment_id"`
	APIContractID      string          `json:"api_contract_id"`
	IngestionRunID     string          `json:"ingestion_run_id"`
	OpenAPIVersion     string          `json:"openapi_version"`
	SourceFormat       string          `json:"source_format"`
	NormalizedContract json.RawMessage `json:"normalized_contract"`
	SourceHash         string          `json:"source_hash"`
	ContentHash        string          `json:"content_hash"`
	ValidationResult   json.RawMessage `json:"validation_result"`
	ParserVersion      string          `json:"parser_version"`
	Visibility         Visibility      `json:"visibility"`
	Diagnostics        json.RawMessage `json:"diagnostics"`
	CreatedAt          time.Time       `json:"created_at"`
}

type APIContractRevision struct {
	ID                     string     `json:"id"`
	DeploymentID           string     `json:"deployment_id"`
	APIContractID          string     `json:"api_contract_id"`
	APIContractName        string     `json:"api_contract_name"`
	APIContractSlug        string     `json:"api_contract_slug"`
	APIContractDescription string     `json:"api_contract_description"`
	APIContractKind        string     `json:"api_contract_kind"`
	APIContractCandidateID string     `json:"api_contract_candidate_id"`
	Revision               int64      `json:"revision"`
	ContentHash            string     `json:"content_hash"`
	Visibility             Visibility `json:"visibility"`
	ReviewedBy             string     `json:"reviewed_by"`
	ReviewedAt             time.Time  `json:"reviewed_at"`
	PublishedAt            time.Time  `json:"published_at"`
	CreatedAt              time.Time  `json:"created_at"`
}

func (revision APIContractRevision) HasHistoricalIdentity() bool {
	return strings.TrimSpace(revision.APIContractID) != "" && strings.TrimSpace(revision.APIContractName) != "" &&
		strings.TrimSpace(revision.APIContractSlug) != "" && strings.TrimSpace(revision.APIContractKind) != ""
}

type APIContractRevisionSourcePublication struct {
	APIContractRevisionID  string    `json:"api_contract_revision_id"`
	DeploymentID           string    `json:"deployment_id"`
	APIContractCandidateID string    `json:"api_contract_candidate_id"`
	SourcePublicationID    string    `json:"source_publication_id"`
	ContentHash            string    `json:"content_hash"`
	CreatedAt              time.Time `json:"created_at"`
}

type APIContractOperation struct {
	ID                     string          `json:"id"`
	APIContractCandidateID string          `json:"api_contract_candidate_id"`
	OperationKey           string          `json:"operation_key"`
	OperationID            string          `json:"operation_id,omitempty"`
	Method                 string          `json:"method"`
	PathTemplate           string          `json:"path_template"`
	Tags                   []string        `json:"tags"`
	Summary                string          `json:"summary"`
	Description            string          `json:"description"`
	Security               json.RawMessage `json:"security"`
	RequestSchemaRefs      []string        `json:"request_schema_refs"`
	ResponseSchemaRefs     []string        `json:"response_schema_refs"`
	ContentHash            string          `json:"content_hash"`
	Ordinal                int             `json:"ordinal"`
}

type APIContractSchema struct {
	ID                     string          `json:"id"`
	APIContractCandidateID string          `json:"api_contract_candidate_id"`
	SchemaKey              string          `json:"schema_key"`
	Schema                 json.RawMessage `json:"schema"`
	ContentHash            string          `json:"content_hash"`
}

type APIContractExample struct {
	ID                     string          `json:"id"`
	APIContractCandidateID string          `json:"api_contract_candidate_id"`
	APIContractOperationID string          `json:"api_contract_operation_id,omitempty"`
	Name                   string          `json:"name"`
	Kind                   string          `json:"example_kind"`
	MediaType              string          `json:"media_type"`
	StatusCode             string          `json:"status_code,omitempty"`
	Value                  json.RawMessage `json:"value"`
	ContentHash            string          `json:"content_hash"`
}

type ContractMapBody struct {
	Overview        string              `json:"overview"`
	Servers         []string            `json:"servers,omitempty"`
	Authentication  []KnowledgeMapEntry `json:"authentication,omitempty"`
	Capabilities    []KnowledgeMapEntry `json:"capabilities"`
	Operations      []KnowledgeMapEntry `json:"operations"`
	Schemas         []KnowledgeMapEntry `json:"schemas,omitempty"`
	Errors          []KnowledgeMapEntry `json:"errors,omitempty"`
	Pagination      []KnowledgeMapEntry `json:"pagination,omitempty"`
	Webhooks        []KnowledgeMapEntry `json:"webhooks,omitempty"`
	Gaps            []KnowledgeMapGap   `json:"gaps,omitempty"`
	QualityWarnings []string            `json:"quality_warnings,omitempty"`
}

type APIContractMap struct {
	ID                     string          `json:"id"`
	DeploymentID           string          `json:"deployment_id"`
	APIContractCandidateID string          `json:"api_contract_candidate_id"`
	MapVersion             string          `json:"map_version"`
	Map                    ContractMapBody `json:"map"`
	AgentMarkdown          string          `json:"agent_markdown"`
	ContentHash            string          `json:"content_hash"`
	CreatedAt              time.Time       `json:"created_at"`
}
