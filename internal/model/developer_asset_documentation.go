package model

import (
	"encoding/json"
	"strings"
	"time"
)

type KnowledgeMapEntry struct {
	ID       string              `json:"id"`
	Kind     string              `json:"kind"`
	Title    string              `json:"title"`
	Summary  string              `json:"summary"`
	Aliases  []string            `json:"aliases,omitempty"`
	Children []KnowledgeMapEntry `json:"children,omitempty"`
}

type KnowledgeMapGap struct {
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

type DocumentationMapBody struct {
	Overview          string              `json:"overview"`
	Documents         []KnowledgeMapEntry `json:"documents"`
	Topics            []KnowledgeMapEntry `json:"topics"`
	Workflows         []KnowledgeMapEntry `json:"workflows"`
	Authentication    []KnowledgeMapEntry `json:"authentication,omitempty"`
	Errors            []KnowledgeMapEntry `json:"errors,omitempty"`
	Examples          []KnowledgeMapEntry `json:"examples,omitempty"`
	Versions          []string            `json:"versions,omitempty"`
	Languages         []string            `json:"languages,omitempty"`
	Gaps              []KnowledgeMapGap   `json:"gaps,omitempty"`
	QualityWarnings   []string            `json:"quality_warnings,omitempty"`
	ExcludedSourceIDs []string            `json:"excluded_source_ids,omitempty"`
}

type DocumentationDocument struct {
	ID                        string          `json:"id"`
	DeploymentID              string          `json:"deployment_id"`
	IngestionRunID            string          `json:"ingestion_run_id"`
	LegacyKnowledgeDocumentID string          `json:"legacy_knowledge_document_id,omitempty"`
	SourcePath                string          `json:"source_path"`
	CanonicalURL              string          `json:"canonical_url,omitempty"`
	Title                     string          `json:"title"`
	Kind                      string          `json:"document_kind"`
	Language                  string          `json:"language,omitempty"`
	MediaType                 string          `json:"media_type"`
	NormalizedMarkdown        string          `json:"normalized_markdown"`
	ContentHash               string          `json:"content_hash"`
	Visibility                Visibility      `json:"visibility"`
	Ordinal                   int             `json:"ordinal"`
	Metadata                  json.RawMessage `json:"metadata"`
	CreatedAt                 time.Time       `json:"created_at"`
}

type DocumentationSection struct {
	ID                      string          `json:"id"`
	DeploymentID            string          `json:"deployment_id"`
	DocumentationDocumentID string          `json:"documentation_document_id"`
	ParentSectionID         string          `json:"parent_section_id,omitempty"`
	Ordinal                 int             `json:"ordinal"`
	HeadingLevel            int             `json:"heading_level"`
	Heading                 string          `json:"heading,omitempty"`
	Anchor                  string          `json:"anchor,omitempty"`
	Breadcrumb              []string        `json:"breadcrumb"`
	ContentKind             string          `json:"content_kind"`
	NormalizedText          string          `json:"normalized_text"`
	CodeLanguage            string          `json:"code_language,omitempty"`
	TokenEstimate           int             `json:"token_estimate"`
	SourceStart             *int            `json:"source_start,omitempty"`
	SourceEnd               *int            `json:"source_end,omitempty"`
	ContentHash             string          `json:"content_hash"`
	Metadata                json.RawMessage `json:"metadata"`
	CreatedAt               time.Time       `json:"created_at"`
}

type DocumentationCollection struct {
	ID             string     `json:"id"`
	DeploymentID   string     `json:"deployment_id"`
	OrganisationID string     `json:"organisation_id"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Description    string     `json:"description"`
	Visibility     Visibility `json:"visibility"`
	Lifecycle      string     `json:"lifecycle"`
	Revision       int64      `json:"revision"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type DocumentationCollectionRevision struct {
	ID                                 string          `json:"id"`
	DeploymentID                       string          `json:"deployment_id"`
	DocumentationCollectionID          string          `json:"documentation_collection_id"`
	DocumentationCollectionName        string          `json:"documentation_collection_name"`
	DocumentationCollectionSlug        string          `json:"documentation_collection_slug"`
	DocumentationCollectionDescription string          `json:"documentation_collection_description"`
	Revision                           int64           `json:"revision"`
	Visibility                         Visibility      `json:"visibility"`
	ContentHash                        string          `json:"content_hash"`
	SelectionManifest                  json.RawMessage `json:"selection_manifest"`
	ReviewedBy                         string          `json:"reviewed_by"`
	ReviewedAt                         time.Time       `json:"reviewed_at"`
	PublishedAt                        time.Time       `json:"published_at"`
	CreatedAt                          time.Time       `json:"created_at"`
}

func (revision DocumentationCollectionRevision) HasHistoricalIdentity() bool {
	return strings.TrimSpace(revision.DocumentationCollectionID) != "" &&
		strings.TrimSpace(revision.DocumentationCollectionName) != "" &&
		strings.TrimSpace(revision.DocumentationCollectionSlug) != ""
}

type DocumentationCollectionMember struct {
	ID                                string          `json:"id"`
	DocumentationCollectionRevisionID string          `json:"documentation_collection_revision_id"`
	Kind                              string          `json:"member_kind"`
	SourcePublicationID               string          `json:"source_publication_id,omitempty"`
	DocumentationDocumentID           string          `json:"documentation_document_id,omitempty"`
	DocumentationSectionID            string          `json:"documentation_section_id,omitempty"`
	Ordinal                           int             `json:"ordinal"`
	IncludeDescendants                bool            `json:"include_descendants"`
	Selector                          json.RawMessage `json:"selector"`
}

type DocumentationMap struct {
	ID                                string               `json:"id"`
	DeploymentID                      string               `json:"deployment_id"`
	IngestionRunID                    string               `json:"ingestion_run_id,omitempty"`
	DocumentationCollectionRevisionID string               `json:"documentation_collection_revision_id,omitempty"`
	MapVersion                        string               `json:"map_version"`
	Map                               DocumentationMapBody `json:"map"`
	AgentMarkdown                     string               `json:"agent_markdown"`
	ContentHash                       string               `json:"content_hash"`
	Visibility                        Visibility           `json:"visibility"`
	CreatedAt                         time.Time            `json:"created_at"`
}

type SourcePublicationDocumentSelection struct {
	SourcePublicationID     string    `json:"source_publication_id"`
	DeploymentID            string    `json:"deployment_id"`
	DocumentationDocumentID string    `json:"documentation_document_id"`
	Decision                string    `json:"decision"`
	Reason                  string    `json:"reason,omitempty"`
	Ordinal                 *int      `json:"ordinal,omitempty"`
	ContentHash             string    `json:"content_hash"`
	ReviewedBy              string    `json:"reviewed_by"`
	ReviewedAt              time.Time `json:"reviewed_at"`
	CreatedAt               time.Time `json:"created_at"`
}

type SourcePublicationDocumentationMap struct {
	SourcePublicationID string    `json:"source_publication_id"`
	DeploymentID        string    `json:"deployment_id"`
	DocumentationMapID  string    `json:"documentation_map_id"`
	ContentHash         string    `json:"content_hash"`
	CreatedAt           time.Time `json:"created_at"`
}

type DeploymentDocumentationPublication struct {
	ID                    string                                     `json:"id"`
	DeploymentID          string                                     `json:"deployment_id"`
	Revision              int64                                      `json:"revision"`
	Visibility            Visibility                                 `json:"visibility"`
	SnapshotSchemaVersion string                                     `json:"snapshot_schema_version"`
	SnapshotHash          string                                     `json:"snapshot_hash"`
	Members               []DeploymentDocumentationPublicationMember `json:"members"`
	PublishedBy           string                                     `json:"published_by"`
	PublishedAt           time.Time                                  `json:"published_at"`
	CreatedAt             time.Time                                  `json:"created_at"`
}

type DeploymentDocumentationPublicationMember struct {
	DocumentationCollectionRevisionID string     `json:"documentation_collection_revision_id"`
	Ordinal                           int        `json:"ordinal"`
	ContentHash                       string     `json:"content_hash"`
	Visibility                        Visibility `json:"visibility"`
}
