package model

import (
	"encoding/json"
	"strings"
	"time"
)

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
