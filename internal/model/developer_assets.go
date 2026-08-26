package model

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

var (
	developerAssetHashPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	developerAssetDigestPattern = regexp.MustCompile(`^(sha256|sha384|sha512):[0-9a-f]+$`)
	forbiddenVersionRange       = regexp.MustCompile(`[*<>=~^]`)
	pypiCoordinateSeparator     = regexp.MustCompile(`[-_.]+`)
)

type DeveloperAssetKind string

const (
	DeveloperAssetDocumentation DeveloperAssetKind = "documentation"
	DeveloperAssetContract      DeveloperAssetKind = "contract"
	DeveloperAssetSDK           DeveloperAssetKind = "sdk"
)

func (kind DeveloperAssetKind) Valid() bool {
	return kind == DeveloperAssetDocumentation || kind == DeveloperAssetContract || kind == DeveloperAssetSDK
}

type DeveloperAssetIngestionState string

const (
	DeveloperAssetIngestionQueued      DeveloperAssetIngestionState = "queued"
	DeveloperAssetIngestionRunning     DeveloperAssetIngestionState = "running"
	DeveloperAssetIngestionReviewReady DeveloperAssetIngestionState = "review_ready"
	DeveloperAssetIngestionFailed      DeveloperAssetIngestionState = "failed"
	DeveloperAssetIngestionCancelled   DeveloperAssetIngestionState = "cancelled"
	DeveloperAssetIngestionPublished   DeveloperAssetIngestionState = "published"
)

func (state DeveloperAssetIngestionState) Valid() bool {
	switch state {
	case DeveloperAssetIngestionQueued, DeveloperAssetIngestionRunning,
		DeveloperAssetIngestionReviewReady, DeveloperAssetIngestionFailed,
		DeveloperAssetIngestionCancelled, DeveloperAssetIngestionPublished:
		return true
	default:
		return false
	}
}

type IngestionStageName string

const (
	IngestionStageAcquire      IngestionStageName = "acquire"
	IngestionStageValidate     IngestionStageName = "validate"
	IngestionStageParse        IngestionStageName = "parse"
	IngestionStageNormalize    IngestionStageName = "normalize"
	IngestionStageSegment      IngestionStageName = "segment"
	IngestionStageExtract      IngestionStageName = "extract"
	IngestionStageMap          IngestionStageName = "map"
	IngestionStageAIEnrich     IngestionStageName = "ai_enrich"
	IngestionStageQualityCheck IngestionStageName = "quality_check"
	IngestionStageBuildIndex   IngestionStageName = "build_index"
	IngestionStageReview       IngestionStageName = "review"
	IngestionStagePublish      IngestionStageName = "publish"
)

func (stage IngestionStageName) Valid() bool {
	switch stage {
	case IngestionStageAcquire, IngestionStageValidate, IngestionStageParse,
		IngestionStageNormalize, IngestionStageSegment, IngestionStageExtract,
		IngestionStageMap, IngestionStageAIEnrich, IngestionStageQualityCheck,
		IngestionStageBuildIndex, IngestionStageReview, IngestionStagePublish:
		return true
	default:
		return false
	}
}

type ProcessorVersions struct {
	Pipeline   string `json:"pipeline"`
	Parser     string `json:"parser"`
	Normalizer string `json:"normalizer"`
	Mapper     string `json:"mapper"`
}

func (versions ProcessorVersions) Valid() bool {
	return strings.TrimSpace(versions.Pipeline) != "" &&
		strings.TrimSpace(versions.Parser) != "" &&
		strings.TrimSpace(versions.Normalizer) != "" &&
		strings.TrimSpace(versions.Mapper) != ""
}

type DeveloperAssetRawBlob struct {
	ID           string          `json:"id"`
	DeploymentID string          `json:"deployment_id"`
	SHA256       string          `json:"sha256"`
	ObjectKey    string          `json:"object_key"`
	MediaType    string          `json:"media_type"`
	ByteSize     int64           `json:"byte_size"`
	SourceURI    string          `json:"source_uri,omitempty"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    time.Time       `json:"created_at"`
}

type DeveloperAssetIngestionRun struct {
	ID                     string                       `json:"id"`
	DeploymentID           string                       `json:"deployment_id"`
	OrganisationID         string                       `json:"organisation_id"`
	AssetKind              DeveloperAssetKind           `json:"asset_kind"`
	TargetID               string                       `json:"target_id,omitempty"`
	TargetKey              string                       `json:"target_key"`
	SourceID               string                       `json:"source_id,omitempty"`
	ResolvedSourceURI      string                       `json:"resolved_source_uri,omitempty"`
	ResolvedSourceRevision string                       `json:"resolved_source_revision,omitempty"`
	ResolvedSourceHash     string                       `json:"resolved_source_hash,omitempty"`
	State                  DeveloperAssetIngestionState `json:"state"`
	Attempt                int                          `json:"attempt"`
	Versions               ProcessorVersions            `json:"versions"`
	RawManifest            json.RawMessage              `json:"raw_manifest"`
	RawManifestHash        string                       `json:"raw_manifest_hash,omitempty"`
	Diagnostics            json.RawMessage              `json:"diagnostics"`
	DiscoveredCount        int                          `json:"discovered_count"`
	AcquiredCount          int                          `json:"acquired_count"`
	FailedCount            int                          `json:"failed_count"`
	SkippedCount           int                          `json:"skipped_count"`
	QuarantinedCount       int                          `json:"quarantined_count"`
	LeaseOwner             string                       `json:"lease_owner,omitempty"`
	LeaseExpiresAt         *time.Time                   `json:"lease_expires_at,omitempty"`
	HeartbeatAt            *time.Time                   `json:"heartbeat_at,omitempty"`
	ErrorCode              string                       `json:"error_code,omitempty"`
	ErrorMessage           string                       `json:"error_message,omitempty"`
	QueuedAt               time.Time                    `json:"queued_at"`
	StartedAt              *time.Time                   `json:"started_at,omitempty"`
	FinishedAt             *time.Time                   `json:"finished_at,omitempty"`
}

func (run DeveloperAssetIngestionRun) Valid() bool {
	if !run.AssetKind.Valid() || !run.State.Valid() || !run.Versions.Valid() ||
		strings.TrimSpace(run.TargetID) == "" || strings.TrimSpace(run.TargetKey) == "" || run.Attempt < 1 {
		return false
	}
	if run.AssetKind != DeveloperAssetSDK && strings.TrimSpace(run.SourceID) == "" {
		return false
	}
	if run.AssetKind == DeveloperAssetDocumentation && run.TargetID != run.SourceID {
		return false
	}
	if (strings.TrimSpace(run.LeaseOwner) == "") != (run.LeaseExpiresAt == nil) {
		return false
	}
	if run.ResolvedSourceHash != "" && !developerAssetHashPattern.MatchString(run.ResolvedSourceHash) {
		return false
	}
	return run.RawManifestHash == "" || developerAssetHashPattern.MatchString(run.RawManifestHash)
}

func (state DeveloperAssetIngestionState) CanTransitionTo(next DeveloperAssetIngestionState) bool {
	switch state {
	case DeveloperAssetIngestionQueued:
		return next == DeveloperAssetIngestionRunning || next == DeveloperAssetIngestionFailed || next == DeveloperAssetIngestionCancelled
	case DeveloperAssetIngestionRunning:
		return next == DeveloperAssetIngestionReviewReady || next == DeveloperAssetIngestionFailed || next == DeveloperAssetIngestionCancelled
	case DeveloperAssetIngestionReviewReady:
		return next == DeveloperAssetIngestionPublished || next == DeveloperAssetIngestionCancelled
	default:
		return false
	}
}

type DeveloperAssetIngestionStage struct {
	ID             string             `json:"id"`
	IngestionRunID string             `json:"ingestion_run_id"`
	Name           IngestionStageName `json:"stage_name"`
	Attempt        int                `json:"attempt"`
	State          string             `json:"state"`
	InputHash      string             `json:"input_hash,omitempty"`
	OutputHash     string             `json:"output_hash,omitempty"`
	Checkpoint     json.RawMessage    `json:"checkpoint"`
	Diagnostics    json.RawMessage    `json:"diagnostics"`
	ErrorCode      string             `json:"error_code,omitempty"`
	ErrorMessage   string             `json:"error_message,omitempty"`
	StartedAt      *time.Time         `json:"started_at,omitempty"`
	FinishedAt     *time.Time         `json:"finished_at,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

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

type SDKPackage struct {
	ID                      string     `json:"id"`
	DeploymentID            string     `json:"deployment_id"`
	OrganisationID          string     `json:"organisation_id"`
	Ecosystem               string     `json:"ecosystem"`
	CanonicalCoordinate     string     `json:"canonical_coordinate"`
	DisplayCoordinate       string     `json:"display_coordinate"`
	Name                    string     `json:"name"`
	Description             string     `json:"description"`
	RegistryURL             string     `json:"registry_url,omitempty"`
	SourceURL               string     `json:"source_url,omitempty"`
	Language                string     `json:"language,omitempty"`
	Platform                string     `json:"platform,omitempty"`
	Visibility              Visibility `json:"visibility"`
	Lifecycle               string     `json:"lifecycle"`
	ReplacementSDKPackageID string     `json:"replacement_sdk_package_id,omitempty"`
	DeprecationMessage      string     `json:"deprecation_message,omitempty"`
	Revision                int64      `json:"revision"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

func CanonicalSDKCoordinate(ecosystem, coordinate string) string {
	value := strings.TrimSpace(coordinate)
	switch strings.ToLower(strings.TrimSpace(ecosystem)) {
	case "pypi":
		return pypiCoordinateSeparator.ReplaceAllString(strings.ToLower(value), "-")
	case "npm", "cargo", "nuget":
		return strings.ToLower(value)
	default:
		return value
	}
}

func (sdk SDKPackage) Valid() bool {
	return sdk.Visibility.Valid() && strings.TrimSpace(sdk.Ecosystem) != "" &&
		strings.TrimSpace(sdk.CanonicalCoordinate) != "" &&
		sdk.CanonicalCoordinate == CanonicalSDKCoordinate(sdk.Ecosystem, sdk.CanonicalCoordinate)
}

type SDKRelease struct {
	ID                     string     `json:"id"`
	DeploymentID           string     `json:"deployment_id"`
	SDKPackageID           string     `json:"sdk_package_id"`
	ExactVersion           string     `json:"exact_version"`
	InstallCommand         string     `json:"install_command"`
	DocumentationURL       string     `json:"documentation_url,omitempty"`
	SourceURL              string     `json:"source_url,omitempty"`
	ResolvedSourceRevision string     `json:"resolved_source_revision,omitempty"`
	UpstreamDigest         string     `json:"upstream_digest,omitempty"`
	IdentityAssurance      string     `json:"identity_assurance"`
	Visibility             Visibility `json:"visibility"`
	Lifecycle              string     `json:"lifecycle"`
	ReleaseHash            string     `json:"release_hash"`
	PublishedAt            *time.Time `json:"published_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
}

func (release SDKRelease) Valid() bool {
	version := strings.TrimSpace(release.ExactVersion)
	if version == "" || strings.EqualFold(version, "latest") || forbiddenVersionRange.MatchString(version) {
		return false
	}
	if strings.TrimSpace(release.InstallCommand) == "" || !release.Visibility.Valid() || !developerAssetHashPattern.MatchString(release.ReleaseHash) {
		return false
	}
	if release.UpstreamDigest != "" && !developerAssetDigestPattern.MatchString(release.UpstreamDigest) {
		return false
	}
	return release.IdentityAssurance != "verified_digest" || release.UpstreamDigest != ""
}

type SDKReleaseLifecycleEvent struct {
	ID                string    `json:"id"`
	SDKReleaseID      string    `json:"sdk_release_id"`
	Lifecycle         string    `json:"lifecycle"`
	Reason            string    `json:"reason,omitempty"`
	ObservedSourceURI string    `json:"observed_source_uri,omitempty"`
	ObservedAt        time.Time `json:"observed_at"`
	RecordedBy        string    `json:"recorded_by"`
	CreatedAt         time.Time `json:"created_at"`
}

type SDKContentCandidate struct {
	ID             string            `json:"id"`
	DeploymentID   string            `json:"deployment_id"`
	SDKReleaseID   string            `json:"sdk_release_id"`
	IngestionRunID string            `json:"ingestion_run_id"`
	Versions       ProcessorVersions `json:"versions"`
	MapVersion     string            `json:"map_version"`
	SourceManifest json.RawMessage   `json:"source_manifest"`
	ContentHash    string            `json:"content_hash"`
	Visibility     Visibility        `json:"visibility"`
	Diagnostics    json.RawMessage   `json:"diagnostics"`
	CreatedAt      time.Time         `json:"created_at"`
}

type SDKContentPublication struct {
	ID                    string     `json:"id"`
	DeploymentID          string     `json:"deployment_id"`
	SDKReleaseID          string     `json:"sdk_release_id"`
	SDKContentCandidateID string     `json:"sdk_content_candidate_id"`
	Revision              int64      `json:"revision"`
	ContentHash           string     `json:"content_hash"`
	Visibility            Visibility `json:"visibility"`
	ReviewedBy            string     `json:"reviewed_by"`
	ReviewedAt            time.Time  `json:"reviewed_at"`
	PublishedAt           time.Time  `json:"published_at"`
	CreatedAt             time.Time  `json:"created_at"`
}

type SDKPublicationFile struct {
	ID                    string          `json:"id"`
	SDKContentCandidateID string          `json:"sdk_content_candidate_id"`
	RawBlobID             string          `json:"raw_blob_id,omitempty"`
	SourcePath            string          `json:"source_path"`
	Role                  string          `json:"file_role"`
	MediaType             string          `json:"media_type"`
	Language              string          `json:"language,omitempty"`
	SuggestedDisposition  string          `json:"suggested_disposition"`
	ExclusionReason       string          `json:"exclusion_reason,omitempty"`
	NormalizedContent     string          `json:"normalized_content,omitempty"`
	ContentHash           string          `json:"content_hash"`
	ByteSize              int64           `json:"byte_size"`
	Metadata              json.RawMessage `json:"metadata"`
	Ordinal               int             `json:"ordinal"`
}

type SDKSection struct {
	ID                    string          `json:"id"`
	SDKContentCandidateID string          `json:"sdk_content_candidate_id"`
	SDKPublicationFileID  string          `json:"sdk_publication_file_id"`
	ParentSectionID       string          `json:"parent_section_id,omitempty"`
	Ordinal               int             `json:"ordinal"`
	Heading               string          `json:"heading,omitempty"`
	Anchor                string          `json:"anchor,omitempty"`
	Breadcrumb            []string        `json:"breadcrumb"`
	ContentKind           string          `json:"content_kind"`
	NormalizedText        string          `json:"normalized_text"`
	CodeLanguage          string          `json:"code_language,omitempty"`
	TokenEstimate         int             `json:"token_estimate"`
	SourceStart           *int            `json:"source_start,omitempty"`
	SourceEnd             *int            `json:"source_end,omitempty"`
	ContentHash           string          `json:"content_hash"`
	Metadata              json.RawMessage `json:"metadata"`
}

type SDKSymbol struct {
	ID                    string          `json:"id"`
	SDKContentCandidateID string          `json:"sdk_content_candidate_id"`
	SDKPublicationFileID  string          `json:"sdk_publication_file_id,omitempty"`
	SDKSectionID          string          `json:"sdk_section_id,omitempty"`
	Language              string          `json:"language"`
	Kind                  string          `json:"symbol_kind"`
	QualifiedName         string          `json:"qualified_name"`
	DisplayName           string          `json:"display_name"`
	Signature             string          `json:"signature,omitempty"`
	Documentation         string          `json:"documentation,omitempty"`
	Identifiers           []string        `json:"identifiers"`
	SourceStart           *int            `json:"source_start,omitempty"`
	SourceEnd             *int            `json:"source_end,omitempty"`
	ContentHash           string          `json:"content_hash"`
	Metadata              json.RawMessage `json:"metadata"`
}

type SDKSampleOrigin string

const (
	SDKSampleExtracted SDKSampleOrigin = "extracted"
	SDKSampleCurated   SDKSampleOrigin = "curated"
	SDKSampleGenerated SDKSampleOrigin = "generated"
)

func (origin SDKSampleOrigin) Valid() bool {
	return origin == SDKSampleExtracted || origin == SDKSampleCurated || origin == SDKSampleGenerated
}

type SDKSampleValidation string

const (
	SDKSampleNotChecked     SDKSampleValidation = "not_checked"
	SDKSampleUnvalidated    SDKSampleValidation = "unvalidated"
	SDKSampleSyntaxChecked  SDKSampleValidation = "syntax_checked"
	SDKSampleCompiled       SDKSampleValidation = "compiled"
	SDKSampleContractTested SDKSampleValidation = "contract_tested"
	SDKSampleExecuted       SDKSampleValidation = "executed"
)

func (validation SDKSampleValidation) Valid() bool {
	switch validation {
	case SDKSampleNotChecked, SDKSampleUnvalidated, SDKSampleSyntaxChecked, SDKSampleCompiled,
		SDKSampleContractTested, SDKSampleExecuted:
		return true
	default:
		return false
	}
}

// HasPositiveMachineValidationEvidence is deliberately stricter than trusting
// a status label. Immutable publication approval requires structured evidence
// that a named validator positively passed the exact sample.
func (sample SDKCodeSample) HasPositiveMachineValidationEvidence() bool {
	if sample.ValidationStatus == SDKSampleNotChecked || sample.ValidationStatus == SDKSampleUnvalidated ||
		!sample.ValidationStatus.Valid() {
		return false
	}
	var evidence struct {
		Validated  bool   `json:"validated"`
		Passed     bool   `json:"passed"`
		Validator  string `json:"validator"`
		EvidenceID string `json:"evidence_id"`
	}
	if json.Unmarshal(sample.ValidationEvidence, &evidence) != nil || (!evidence.Validated && !evidence.Passed) {
		return false
	}
	return strings.TrimSpace(evidence.Validator) != "" || strings.TrimSpace(evidence.EvidenceID) != ""
}

// ValidSDKSampleReviewEvidence requires a bounded structured record rather
// than treating the approval click itself as evidence.
func ValidSDKSampleReviewEvidence(value json.RawMessage) bool {
	if len(value) == 0 || len(value) > 8000 {
		return false
	}
	var evidence struct {
		Summary string `json:"summary"`
	}
	return json.Unmarshal(value, &evidence) == nil && strings.TrimSpace(evidence.Summary) != ""
}

type SDKCodeSample struct {
	ID                    string              `json:"id"`
	DeploymentID          string              `json:"deployment_id"`
	SDKContentCandidateID string              `json:"sdk_content_candidate_id"`
	SDKPublicationFileID  string              `json:"sdk_publication_file_id,omitempty"`
	SDKSectionID          string              `json:"sdk_section_id,omitempty"`
	Language              string              `json:"language"`
	Title                 string              `json:"title"`
	Intent                string              `json:"intent"`
	Code                  string              `json:"code"`
	Imports               []string            `json:"imports"`
	Prerequisites         []string            `json:"prerequisites"`
	Origin                SDKSampleOrigin     `json:"origin"`
	SourceURI             string              `json:"source_uri,omitempty"`
	SourceRevision        string              `json:"source_revision,omitempty"`
	SourcePath            string              `json:"source_path,omitempty"`
	SourceStart           *int                `json:"source_start,omitempty"`
	SourceEnd             *int                `json:"source_end,omitempty"`
	Attribution           string              `json:"attribution,omitempty"`
	LicenseExpression     string              `json:"license_expression,omitempty"`
	ValidationStatus      SDKSampleValidation `json:"validation_status"`
	ValidationEvidence    json.RawMessage     `json:"validation_evidence"`
	Visibility            Visibility          `json:"visibility"`
	ContentHash           string              `json:"content_hash"`
	CreatedAt             time.Time           `json:"created_at"`
}

func (sample SDKCodeSample) Valid() bool {
	if !sample.Origin.Valid() || !sample.ValidationStatus.Valid() || !sample.Visibility.Valid() ||
		strings.TrimSpace(sample.Language) == "" || strings.TrimSpace(sample.Title) == "" ||
		strings.TrimSpace(sample.Intent) == "" || strings.TrimSpace(sample.Code) == "" ||
		!developerAssetHashPattern.MatchString(sample.ContentHash) {
		return false
	}
	if sample.Origin == SDKSampleExtracted && (strings.TrimSpace(sample.SourcePath) == "" || strings.TrimSpace(sample.SourceRevision) == "") {
		return false
	}
	return true
}

type SDKMapBody struct {
	Overview        string              `json:"overview"`
	Installation    []KnowledgeMapEntry `json:"installation"`
	Initialization  []KnowledgeMapEntry `json:"initialization,omitempty"`
	Authentication  []KnowledgeMapEntry `json:"authentication,omitempty"`
	SupportedAPIs   []KnowledgeMapEntry `json:"supported_apis"`
	Modules         []KnowledgeMapEntry `json:"modules,omitempty"`
	Symbols         []KnowledgeMapEntry `json:"symbols,omitempty"`
	Workflows       []KnowledgeMapEntry `json:"workflows,omitempty"`
	Samples         []KnowledgeMapEntry `json:"samples,omitempty"`
	Errors          []KnowledgeMapEntry `json:"errors,omitempty"`
	Pagination      []KnowledgeMapEntry `json:"pagination,omitempty"`
	Retries         []KnowledgeMapEntry `json:"retries,omitempty"`
	Webhooks        []KnowledgeMapEntry `json:"webhooks,omitempty"`
	Deprecations    []KnowledgeMapEntry `json:"deprecations,omitempty"`
	Migrations      []KnowledgeMapEntry `json:"migrations,omitempty"`
	Gaps            []KnowledgeMapGap   `json:"gaps,omitempty"`
	QualityWarnings []string            `json:"quality_warnings,omitempty"`
}

type SDKMap struct {
	ID                    string     `json:"id"`
	DeploymentID          string     `json:"deployment_id"`
	SDKContentCandidateID string     `json:"sdk_content_candidate_id"`
	MapVersion            string     `json:"map_version"`
	Map                   SDKMapBody `json:"map"`
	AgentMarkdown         string     `json:"agent_markdown"`
	ContentHash           string     `json:"content_hash"`
	CreatedAt             time.Time  `json:"created_at"`
}

type SDKContentPublicationFileSelection struct {
	SDKContentPublicationID string    `json:"sdk_content_publication_id"`
	DeploymentID            string    `json:"deployment_id"`
	SDKContentCandidateID   string    `json:"sdk_content_candidate_id"`
	SDKPublicationFileID    string    `json:"sdk_publication_file_id"`
	Decision                string    `json:"decision"`
	Reason                  string    `json:"reason,omitempty"`
	Ordinal                 *int      `json:"ordinal,omitempty"`
	ContentHash             string    `json:"content_hash"`
	CreatedAt               time.Time `json:"created_at"`
}

type SDKContentPublicationSampleSelection struct {
	SDKContentPublicationID string          `json:"sdk_content_publication_id"`
	DeploymentID            string          `json:"deployment_id"`
	SDKContentCandidateID   string          `json:"sdk_content_candidate_id"`
	SDKCodeSampleID         string          `json:"sdk_code_sample_id"`
	Decision                string          `json:"decision"`
	Reason                  string          `json:"reason,omitempty"`
	ReviewEvidence          json.RawMessage `json:"review_evidence,omitempty"`
	Ordinal                 *int            `json:"ordinal,omitempty"`
	ReviewedBy              string          `json:"reviewed_by"`
	ReviewedAt              time.Time       `json:"reviewed_at"`
	ContentHash             string          `json:"content_hash"`
	CreatedAt               time.Time       `json:"created_at"`
}

func (selection SDKContentPublicationSampleSelection) ValidFor(sample SDKCodeSample) bool {
	if selection.SDKContentCandidateID != sample.SDKContentCandidateID ||
		selection.SDKCodeSampleID != sample.ID || selection.ContentHash != sample.ContentHash {
		return false
	}
	switch selection.Decision {
	case "approved":
		return selection.Ordinal != nil && selection.Reason == "" &&
			(sample.HasPositiveMachineValidationEvidence() || ValidSDKSampleReviewEvidence(selection.ReviewEvidence))
	case "excluded", "quarantined":
		return selection.Ordinal == nil && strings.TrimSpace(selection.Reason) != ""
	default:
		return false
	}
}

type SDKContentPublicationMap struct {
	SDKContentPublicationID string    `json:"sdk_content_publication_id"`
	DeploymentID            string    `json:"deployment_id"`
	SDKContentCandidateID   string    `json:"sdk_content_candidate_id"`
	SDKMapID                string    `json:"sdk_map_id"`
	ContentHash             string    `json:"content_hash"`
	CreatedAt               time.Time `json:"created_at"`
}

type SDKSampleAPIReference struct {
	ID                     string    `json:"id"`
	SDKCodeSampleID        string    `json:"sdk_code_sample_id"`
	SDKContentCandidateID  string    `json:"sdk_content_candidate_id"`
	DeploymentID           string    `json:"deployment_id"`
	APIID                  string    `json:"api_id"`
	APIContractRevisionID  string    `json:"api_contract_revision_id,omitempty"`
	APIContractCandidateID string    `json:"api_contract_candidate_id,omitempty"`
	APIContractOperationID string    `json:"api_contract_operation_id,omitempty"`
	APISDKBindingID        string    `json:"api_sdk_binding_id,omitempty"`
	ReferenceKind          string    `json:"reference_kind"`
	CreatedAt              time.Time `json:"created_at"`
}

type SDKCompatibilityCoverage string

const (
	SDKCoverageFull    SDKCompatibilityCoverage = "full"
	SDKCoveragePartial SDKCompatibilityCoverage = "partial"
	SDKCoverageUnknown SDKCompatibilityCoverage = "unknown"
)

func (coverage SDKCompatibilityCoverage) Valid() bool {
	return coverage == SDKCoverageFull || coverage == SDKCoveragePartial || coverage == SDKCoverageUnknown
}

type SDKCompatibilityAssurance string

const (
	SDKAssuranceRelated    SDKCompatibilityAssurance = "related"
	SDKAssuranceDocumented SDKCompatibilityAssurance = "documented"
	SDKAssuranceReviewed   SDKCompatibilityAssurance = "reviewed"
	SDKAssuranceTested     SDKCompatibilityAssurance = "tested"
	SDKAssuranceVerified   SDKCompatibilityAssurance = "verified"
)

func (assurance SDKCompatibilityAssurance) Valid() bool {
	switch assurance {
	case SDKAssuranceRelated, SDKAssuranceDocumented, SDKAssuranceReviewed,
		SDKAssuranceTested, SDKAssuranceVerified:
		return true
	default:
		return false
	}
}

type SDKCompatibilityAssertion struct {
	ID                      string                    `json:"id"`
	DeploymentID            string                    `json:"deployment_id"`
	APIID                   string                    `json:"api_id"`
	SDKReleaseID            string                    `json:"sdk_release_id"`
	APIContractRevisionID   string                    `json:"api_contract_revision_id,omitempty"`
	SupersedesAssertionID   string                    `json:"supersedes_assertion_id,omitempty"`
	Coverage                SDKCompatibilityCoverage  `json:"coverage"`
	Assurance               SDKCompatibilityAssurance `json:"assurance"`
	State                   string                    `json:"assertion_state"`
	ApplicableModules       []string                  `json:"applicable_modules"`
	ApplicableCapabilities  []string                  `json:"applicable_capabilities"`
	ApplicableOperationKeys []string                  `json:"applicable_operation_keys"`
	KnownGaps               []string                  `json:"known_gaps"`
	Evidence                json.RawMessage           `json:"evidence"`
	ContentHash             string                    `json:"content_hash"`
	ReviewedBy              string                    `json:"reviewed_by"`
	ReviewedAt              time.Time                 `json:"reviewed_at"`
	CreatedAt               time.Time                 `json:"created_at"`
}

type APIDocumentationBinding struct {
	ID                        string          `json:"id"`
	DeploymentID              string          `json:"deployment_id"`
	APIID                     string          `json:"api_id"`
	DocumentationCollectionID string          `json:"documentation_collection_id"`
	FollowLatest              bool            `json:"follow_latest"`
	PinnedRevisionID          string          `json:"pinned_revision_id,omitempty"`
	Selector                  json.RawMessage `json:"selector"`
	SelectorHash              string          `json:"selector_hash"`
	Visibility                Visibility      `json:"visibility"`
	Lifecycle                 string          `json:"lifecycle"`
	Revision                  int64           `json:"revision"`
}

func (binding APIDocumentationBinding) Valid() bool {
	return binding.Visibility.Valid() && (binding.FollowLatest == (binding.PinnedRevisionID == ""))
}

type APIContractBinding struct {
	ID               string     `json:"id"`
	DeploymentID     string     `json:"deployment_id"`
	APIID            string     `json:"api_id"`
	APIContractID    string     `json:"api_contract_id"`
	FollowLatest     bool       `json:"follow_latest"`
	PinnedRevisionID string     `json:"pinned_revision_id,omitempty"`
	Primary          bool       `json:"primary"`
	Visibility       Visibility `json:"visibility"`
	Lifecycle        string     `json:"lifecycle"`
	Revision         int64      `json:"revision"`
}

func (binding APIContractBinding) Valid() bool {
	return binding.Visibility.Valid() && (binding.FollowLatest == (binding.PinnedRevisionID == ""))
}

type APISDKBinding struct {
	ID                       string                    `json:"id"`
	DeploymentID             string                    `json:"deployment_id"`
	APIID                    string                    `json:"api_id"`
	SDKPackageID             string                    `json:"sdk_package_id"`
	SDKReleaseID             string                    `json:"sdk_release_id"`
	SDKContentPublicationID  string                    `json:"sdk_content_publication_id,omitempty"`
	APIContractRevisionID    string                    `json:"api_contract_revision_id,omitempty"`
	CompatibilityAssertionID string                    `json:"compatibility_assertion_id,omitempty"`
	State                    string                    `json:"state"`
	Coverage                 SDKCompatibilityCoverage  `json:"coverage"`
	Assurance                SDKCompatibilityAssurance `json:"assurance"`
	ApplicableModules        []string                  `json:"applicable_modules"`
	ApplicableCapabilities   []string                  `json:"applicable_capabilities"`
	ApplicableOperationKeys  []string                  `json:"applicable_operation_keys"`
	Selector                 json.RawMessage           `json:"selector"`
	SelectorHash             string                    `json:"selector_hash"`
	Visibility               Visibility                `json:"visibility"`
	Revision                 int64                     `json:"revision"`
	CreatedAt                time.Time                 `json:"created_at"`
	UpdatedAt                time.Time                 `json:"updated_at"`
}

func (binding APISDKBinding) Valid() bool {
	if strings.TrimSpace(binding.APIID) == "" || strings.TrimSpace(binding.SDKPackageID) == "" ||
		strings.TrimSpace(binding.SDKReleaseID) == "" || !binding.Coverage.Valid() ||
		!binding.Assurance.Valid() || !binding.Visibility.Valid() ||
		!developerAssetHashPattern.MatchString(binding.SelectorHash) {
		return false
	}
	if binding.State == "ready" && binding.SDKContentPublicationID == "" {
		return false
	}
	return (binding.Assurance != SDKAssuranceTested && binding.Assurance != SDKAssuranceVerified) || binding.CompatibilityAssertionID != ""
}

type APIDeveloperAssetPublication struct {
	ID                                   string                             `json:"id"`
	DeploymentID                         string                             `json:"deployment_id"`
	APIID                                string                             `json:"api_id"`
	APIRevisionID                        string                             `json:"api_revision_id"`
	DeploymentDocumentationPublicationID string                             `json:"deployment_documentation_publication_id,omitempty"`
	SnapshotSchemaVersion                string                             `json:"snapshot_schema_version"`
	SnapshotHash                         string                             `json:"snapshot_hash"`
	Documentation                        []APIPublicationDocumentationAsset `json:"documentation"`
	Contracts                            []APIPublicationContractAsset      `json:"contracts"`
	SDKs                                 []APIPublicationSDKAsset           `json:"sdks"`
	PublishedBy                          string                             `json:"published_by"`
	PublishedAt                          time.Time                          `json:"published_at"`
	CreatedAt                            time.Time                          `json:"created_at"`
}

type APIPublicationDocumentationAsset struct {
	BindingID                          string          `json:"binding_id"`
	DocumentationCollectionID          string          `json:"documentation_collection_id"`
	DocumentationCollectionName        string          `json:"documentation_collection_name"`
	DocumentationCollectionSlug        string          `json:"documentation_collection_slug"`
	DocumentationCollectionDescription string          `json:"documentation_collection_description"`
	DocumentationCollectionRevisionID  string          `json:"documentation_collection_revision_id"`
	Selector                           json.RawMessage `json:"selector"`
	SelectorHash                       string          `json:"selector_hash"`
	ContentHash                        string          `json:"content_hash"`
	Visibility                         Visibility      `json:"visibility"`
	Ordinal                            int             `json:"ordinal"`
}

func (asset APIPublicationDocumentationAsset) MatchesRevisionIdentity(revision DocumentationCollectionRevision) bool {
	return revision.HasHistoricalIdentity() && asset.DocumentationCollectionID == revision.DocumentationCollectionID &&
		asset.DocumentationCollectionName == revision.DocumentationCollectionName &&
		asset.DocumentationCollectionSlug == revision.DocumentationCollectionSlug &&
		asset.DocumentationCollectionDescription == revision.DocumentationCollectionDescription
}

type APIPublicationContractAsset struct {
	BindingID              string     `json:"binding_id"`
	APIContractID          string     `json:"api_contract_id"`
	APIContractName        string     `json:"api_contract_name"`
	APIContractSlug        string     `json:"api_contract_slug"`
	APIContractDescription string     `json:"api_contract_description"`
	APIContractKind        string     `json:"api_contract_kind"`
	APIContractRevisionID  string     `json:"api_contract_revision_id"`
	Primary                bool       `json:"primary"`
	ContentHash            string     `json:"content_hash"`
	Visibility             Visibility `json:"visibility"`
	Ordinal                int        `json:"ordinal"`
}

func (asset APIPublicationContractAsset) MatchesRevisionIdentity(revision APIContractRevision) bool {
	return revision.HasHistoricalIdentity() && asset.APIContractID == revision.APIContractID &&
		asset.APIContractName == revision.APIContractName && asset.APIContractSlug == revision.APIContractSlug &&
		asset.APIContractDescription == revision.APIContractDescription && asset.APIContractKind == revision.APIContractKind
}

type APIPublicationSDKAsset struct {
	BindingID                   string          `json:"binding_id"`
	SDKPackageID                string          `json:"sdk_package_id"`
	SDKPackageEcosystem         string          `json:"sdk_package_ecosystem"`
	SDKPackageCoordinate        string          `json:"sdk_package_coordinate"`
	SDKPackageDisplayCoordinate string          `json:"sdk_package_display_coordinate"`
	SDKPackageDisplayName       string          `json:"sdk_package_display_name"`
	SDKPackageLanguage          string          `json:"sdk_package_language,omitempty"`
	SDKPackagePlatform          string          `json:"sdk_package_platform,omitempty"`
	SDKReleaseID                string          `json:"sdk_release_id"`
	SDKContentPublicationID     string          `json:"sdk_content_publication_id,omitempty"`
	CompatibilityAssertionID    string          `json:"compatibility_assertion_id,omitempty"`
	Selector                    json.RawMessage `json:"selector"`
	SelectorHash                string          `json:"selector_hash"`
	ContentHash                 string          `json:"content_hash"`
	Visibility                  Visibility      `json:"visibility"`
	Ordinal                     int             `json:"ordinal"`
}

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

type RetrievalEvaluationSet struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deployment_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Lifecycle    string    `json:"lifecycle"`
	Revision     int64     `json:"revision"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type RetrievalEvaluationSetRevision struct {
	ID                       string    `json:"id"`
	RetrievalEvaluationSetID string    `json:"retrieval_evaluation_set_id"`
	Revision                 int64     `json:"revision"`
	ContentHash              string    `json:"content_hash"`
	CreatedBy                string    `json:"created_by"`
	CreatedAt                time.Time `json:"created_at"`
}

type RetrievalEvaluationCase struct {
	ID                               string          `json:"id"`
	RetrievalEvaluationSetRevisionID string          `json:"retrieval_evaluation_set_revision_id"`
	CaseKey                          string          `json:"case_key"`
	Query                            string          `json:"query"`
	RequestedFilters                 json.RawMessage `json:"requested_filters"`
	ExpectedEvidence                 json.RawMessage `json:"expected_evidence"`
	ForbiddenEvidence                json.RawMessage `json:"forbidden_evidence"`
	ExpectNoResults                  bool            `json:"expect_no_results"`
}

type RetrievalEvaluationRun struct {
	ID                                   string          `json:"id"`
	DeploymentID                         string          `json:"deployment_id"`
	RetrievalEvaluationSetRevisionID     string          `json:"retrieval_evaluation_set_revision_id"`
	DeploymentDocumentationPublicationID string          `json:"deployment_documentation_publication_id,omitempty"`
	APIDeveloperAssetPublicationID       string          `json:"api_developer_asset_publication_id,omitempty"`
	RetrievalProfileVersion              string          `json:"retrieval_profile_version"`
	State                                string          `json:"state"`
	Metrics                              json.RawMessage `json:"metrics"`
	StartedAt                            *time.Time      `json:"started_at,omitempty"`
	FinishedAt                           *time.Time      `json:"finished_at,omitempty"`
	CreatedBy                            string          `json:"created_by"`
	CreatedAt                            time.Time       `json:"created_at"`
}

type RetrievalEvaluationCaseResult struct {
	RetrievalEvaluationRunID         string          `json:"retrieval_evaluation_run_id"`
	DeploymentID                     string          `json:"deployment_id"`
	RetrievalEvaluationSetRevisionID string          `json:"retrieval_evaluation_set_revision_id"`
	RetrievalEvaluationCaseID        string          `json:"retrieval_evaluation_case_id"`
	RetrievalQueryTraceID            string          `json:"retrieval_query_trace_id,omitempty"`
	Passed                           bool            `json:"passed"`
	Metrics                          json.RawMessage `json:"metrics"`
	FailureReason                    string          `json:"failure_reason,omitempty"`
	CreatedAt                        time.Time       `json:"created_at"`
}

type LegacySDKReferenceMigration struct {
	LegacySDKReferenceID string          `json:"legacy_sdk_reference_id"`
	DeploymentID         string          `json:"deployment_id"`
	APIID                string          `json:"api_id"`
	SDKPackageID         string          `json:"sdk_package_id,omitempty"`
	SDKReleaseID         string          `json:"sdk_release_id,omitempty"`
	APISDKBindingID      string          `json:"api_sdk_binding_id,omitempty"`
	Status               string          `json:"status"`
	ConflictCode         string          `json:"conflict_code,omitempty"`
	LegacySnapshot       json.RawMessage `json:"legacy_snapshot"`
	MigratedAt           time.Time       `json:"migrated_at"`
}
