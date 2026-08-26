package model

import (
	"encoding/json"
	"strings"
	"time"
)

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
