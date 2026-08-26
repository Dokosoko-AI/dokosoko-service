package model

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DeveloperAssetAIAdvisoryRun is an immutable, append-only record of one
// strictly validated AI suggestion. It deliberately stores neither the system
// prompt nor source payloads: exact evidence IDs and hashes make the result
// inspectable without duplicating untrusted documentation or code.
type DeveloperAssetAIAdvisoryRun struct {
	ID                             string          `json:"id"`
	DeploymentID                   string          `json:"deployment_id"`
	PromptKey                      string          `json:"prompt_key"`
	PromptVersion                  string          `json:"prompt_version"`
	ScopeKind                      string          `json:"scope_kind"`
	ScopeID                        string          `json:"scope_id"`
	ScopeVisibility                Visibility      `json:"scope_visibility"`
	IngestionRunID                 string          `json:"ingestion_run_id,omitempty"`
	SourcePublicationID            string          `json:"source_publication_id,omitempty"`
	SDKPackageID                   string          `json:"sdk_package_id,omitempty"`
	SDKReleaseID                   string          `json:"sdk_release_id,omitempty"`
	SDKContentCandidateID          string          `json:"sdk_content_candidate_id,omitempty"`
	SDKContentPublicationID        string          `json:"sdk_content_publication_id,omitempty"`
	APIID                          string          `json:"api_id,omitempty"`
	APIDeveloperAssetPublicationID string          `json:"api_developer_asset_publication_id,omitempty"`
	APISDKBindingID                string          `json:"api_sdk_binding_id,omitempty"`
	SDKCodeSampleID                string          `json:"sdk_code_sample_id,omitempty"`
	AllowedEvidenceIDs             []string        `json:"allowed_evidence_ids"`
	EvidenceHash                   string          `json:"evidence_hash"`
	InputHash                      string          `json:"input_hash"`
	Result                         json.RawMessage `json:"result"`
	ResultHash                     string          `json:"result_hash"`
	CreatedBy                      string          `json:"created_by"`
	CreatedAt                      time.Time       `json:"created_at"`
}

func (run DeveloperAssetAIAdvisoryRun) Valid() bool {
	if strings.TrimSpace(run.ID) == "" || strings.TrimSpace(run.DeploymentID) == "" ||
		strings.TrimSpace(run.PromptVersion) == "" || strings.TrimSpace(run.ScopeID) == "" ||
		!run.ScopeVisibility.Valid() || strings.TrimSpace(run.CreatedBy) == "" ||
		!developerAssetHashPattern.MatchString(run.EvidenceHash) ||
		!developerAssetHashPattern.MatchString(run.InputHash) ||
		!developerAssetHashPattern.MatchString(run.ResultHash) || len(run.Result) == 0 {
		return false
	}
	resultDigest := sha256.Sum256(run.Result)
	if run.ResultHash != "sha256:"+fmt.Sprintf("%x", resultDigest) {
		return false
	}
	var result map[string]any
	if json.Unmarshal(run.Result, &result) != nil || result == nil || len(run.AllowedEvidenceIDs) == 0 {
		return false
	}
	for index, id := range run.AllowedEvidenceIDs {
		if strings.TrimSpace(id) != id || id == "" || len(id) > 200 || index > 0 && run.AllowedEvidenceIDs[index-1] >= id {
			return false
		}
	}
	switch run.PromptKey {
	case "documentation.map_enrichment":
		return run.ScopeKind == "documentation_publication" && run.ScopeID == run.SourcePublicationID &&
			run.IngestionRunID != "" && run.SourcePublicationID != "" &&
			run.SDKPackageID == "" && run.SDKReleaseID == "" && run.SDKContentCandidateID == "" &&
			run.SDKContentPublicationID == "" && run.APIID == "" &&
			run.APIDeveloperAssetPublicationID == "" && run.APISDKBindingID == "" && run.SDKCodeSampleID == ""
	case "sdk.map_enrichment":
		return run.ScopeKind == "sdk_content_publication" && run.ScopeID == run.SDKContentPublicationID &&
			run.IngestionRunID != "" && run.SDKPackageID != "" && run.SDKReleaseID != "" &&
			run.SDKContentCandidateID != "" && run.SDKContentPublicationID != "" &&
			run.SourcePublicationID == "" && run.APIID == "" &&
			run.APIDeveloperAssetPublicationID == "" && run.APISDKBindingID == "" && run.SDKCodeSampleID == ""
	case "sdk.applicability_suggestion":
		return run.ScopeKind == "sdk_api_binding" && run.ScopeID == run.APISDKBindingID &&
			run.IngestionRunID != "" && run.SDKPackageID != "" && run.SDKReleaseID != "" &&
			run.SDKContentCandidateID != "" && run.SDKContentPublicationID != "" && run.APIID != "" &&
			run.APIDeveloperAssetPublicationID != "" && run.APISDKBindingID != "" &&
			run.SourcePublicationID == "" && run.SDKCodeSampleID == ""
	case "sdk.sample_review":
		return run.ScopeKind == "sdk_sample" && run.ScopeID == run.SDKCodeSampleID &&
			run.IngestionRunID != "" && run.SDKPackageID != "" && run.SDKReleaseID != "" &&
			run.SDKContentCandidateID != "" && run.SDKContentPublicationID != "" && run.APIID != "" &&
			run.APIDeveloperAssetPublicationID != "" && run.APISDKBindingID != "" && run.SDKCodeSampleID != "" &&
			run.SourcePublicationID == ""
	default:
		return false
	}
}
