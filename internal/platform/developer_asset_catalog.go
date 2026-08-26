package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

// DeveloperAssetCatalog is the root, deployment-owned catalogue. API
// workspaces consume these assets through typed bindings; they do not own
// mutable copies of them.
type DeveloperAssetCatalog struct {
	Documentation []model.DocumentationCollection `json:"documentation"`
	Contracts     []model.APIContract             `json:"contracts"`
	SDKPackages   []model.SDKPackage              `json:"sdk_packages"`
}

func (s *Service) DeveloperAssetCatalog(ctx context.Context) (DeveloperAssetCatalog, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return DeveloperAssetCatalog{}, err
	}
	documentation, err := s.store.DocumentationCollections(ctx, deployment.ID)
	if err != nil {
		return DeveloperAssetCatalog{}, err
	}
	contracts, err := s.store.APIContracts(ctx, deployment.ID)
	if err != nil {
		return DeveloperAssetCatalog{}, err
	}
	sdks, err := s.store.SDKPackages(ctx, deployment.ID)
	if err != nil {
		return DeveloperAssetCatalog{}, err
	}
	return DeveloperAssetCatalog{Documentation: documentation, Contracts: contracts, SDKPackages: sdks}, nil
}

type SDKPackageInput struct {
	Ecosystem               string
	Coordinate              string
	Name                    string
	Description             string
	RegistryURL             string
	SourceURL               string
	Language                string
	Platform                string
	Visibility              model.Visibility
	Lifecycle               string
	ReplacementSDKPackageID string
	DeprecationMessage      string
	Revision                int64
}

func normalizeSDKPackageInput(input SDKPackageInput) (SDKPackageInput, error) {
	input.Ecosystem = strings.ToLower(strings.TrimSpace(input.Ecosystem))
	input.Coordinate = strings.TrimSpace(input.Coordinate)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.RegistryURL = strings.TrimSpace(input.RegistryURL)
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	input.Language = strings.TrimSpace(input.Language)
	input.Platform = strings.TrimSpace(input.Platform)
	input.ReplacementSDKPackageID = strings.TrimSpace(input.ReplacementSDKPackageID)
	input.DeprecationMessage = strings.TrimSpace(input.DeprecationMessage)
	if input.Visibility == "" {
		input.Visibility = model.VisibilityPrivate
	}
	if input.Lifecycle == "" {
		input.Lifecycle = "draft"
	}
	if input.Ecosystem == "" || input.Coordinate == "" || input.Name == "" || len(input.Name) > 120 || len(input.Description) > 4000 {
		return SDKPackageInput{}, errors.New("ecosystem, coordinate, and name are required and must fit their limits")
	}
	if !input.Visibility.Valid() {
		return SDKPackageInput{}, ErrInvalidVisibility
	}
	switch input.Lifecycle {
	case "draft", "active", "deprecated", "archived":
	default:
		return SDKPackageInput{}, errors.New("SDK package lifecycle must be draft, active, deprecated, or archived")
	}
	if input.Lifecycle != "deprecated" && (input.ReplacementSDKPackageID != "" || input.DeprecationMessage != "") {
		return SDKPackageInput{}, errors.New("replacement and deprecation message are only valid for deprecated SDK packages")
	}
	if !validSDKURL(input.RegistryURL) || !validSDKURL(input.SourceURL) {
		return SDKPackageInput{}, errors.New("SDK registry and source URLs must be fixed public HTTPS URLs")
	}
	if _, err := canonicalSDKInstallCommand(input.Ecosystem, input.Coordinate, sdkValidationPlaceholderVersion(input.Ecosystem)); err != nil {
		return SDKPackageInput{}, fmt.Errorf("invalid SDK package coordinate: %w", err)
	}
	return input, nil
}

func sdkValidationPlaceholderVersion(ecosystem string) string {
	if ecosystem == "go" {
		return "v1.0.0"
	}
	return "1.0.0"
}

func (s *Service) SaveSDKPackage(ctx context.Context, packageID string, input SDKPackageInput, actor Actor) (model.SDKPackage, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.SDKPackage{}, err
	}
	input, err = normalizeSDKPackageInput(input)
	if err != nil {
		return model.SDKPackage{}, err
	}
	packageID = strings.TrimSpace(packageID)
	if packageID == "" {
		packageID, err = randomUUID()
		if err != nil {
			return model.SDKPackage{}, err
		}
	} else if input.Revision > 0 {
		current, lookupErr := s.store.SDKPackage(ctx, deployment.ID, packageID)
		if lookupErr != nil {
			return model.SDKPackage{}, lookupErr
		}
		canonicalCoordinate := model.CanonicalSDKCoordinate(input.Ecosystem, input.Coordinate)
		if current.Ecosystem != input.Ecosystem ||
			current.CanonicalCoordinate != canonicalCoordinate ||
			current.DisplayCoordinate != input.Coordinate {
			return model.SDKPackage{}, errors.New("SDK package ecosystem and coordinate are immutable; create a new package identity")
		}
	}
	if input.ReplacementSDKPackageID == packageID {
		return model.SDKPackage{}, errors.New("an SDK package cannot replace itself")
	}
	value := model.SDKPackage{
		ID: packageID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID,
		Ecosystem: input.Ecosystem, CanonicalCoordinate: model.CanonicalSDKCoordinate(input.Ecosystem, input.Coordinate),
		DisplayCoordinate: input.Coordinate, Name: input.Name, Description: input.Description,
		RegistryURL: input.RegistryURL, SourceURL: input.SourceURL, Language: input.Language, Platform: input.Platform,
		Visibility: input.Visibility, Lifecycle: input.Lifecycle, ReplacementSDKPackageID: input.ReplacementSDKPackageID,
		DeprecationMessage: input.DeprecationMessage,
	}
	value, err = s.store.SaveSDKPackage(ctx, value, input.Revision)
	if err != nil {
		return model.SDKPackage{}, err
	}
	if err := s.appendDeveloperAssetAudit(ctx, deployment, actor, "sdk_package.saved", "sdk_package", value.ID, map[string]any{
		"ecosystem": value.Ecosystem, "coordinate": value.CanonicalCoordinate, "visibility": value.Visibility,
		"lifecycle": value.Lifecycle, "revision": value.Revision,
	}); err != nil {
		return model.SDKPackage{}, err
	}
	return value, nil
}

type SDKReleaseInput struct {
	ExactVersion           string
	InstallCommand         string
	DocumentationURL       string
	SourceURL              string
	ResolvedSourceRevision string
	UpstreamDigest         string
	IdentityAssurance      string
	Visibility             model.Visibility
	Lifecycle              string
}

func (s *Service) CreateSDKRelease(ctx context.Context, packageID string, input SDKReleaseInput, actor Actor) (model.SDKRelease, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.SDKRelease{}, err
	}
	parent, err := s.store.SDKPackage(ctx, deployment.ID, strings.TrimSpace(packageID))
	if err != nil {
		return model.SDKRelease{}, err
	}
	input.ExactVersion = strings.TrimSpace(input.ExactVersion)
	input.InstallCommand = strings.TrimSpace(input.InstallCommand)
	input.DocumentationURL = strings.TrimSpace(input.DocumentationURL)
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	input.ResolvedSourceRevision = strings.TrimSpace(input.ResolvedSourceRevision)
	input.UpstreamDigest = strings.ToLower(strings.TrimSpace(input.UpstreamDigest))
	input.IdentityAssurance = strings.TrimSpace(input.IdentityAssurance)
	input.Lifecycle = strings.TrimSpace(input.Lifecycle)
	if input.Visibility == "" {
		input.Visibility = parent.Visibility
	}
	if input.IdentityAssurance == "" {
		input.IdentityAssurance = "metadata_only"
	}
	if input.Lifecycle == "" {
		input.Lifecycle = "active"
	}
	wantCommand, err := canonicalSDKInstallCommand(parent.Ecosystem, parent.DisplayCoordinate, input.ExactVersion)
	if err != nil {
		return model.SDKRelease{}, err
	}
	if input.InstallCommand == "" {
		input.InstallCommand = wantCommand
	}
	if input.InstallCommand != wantCommand {
		return model.SDKRelease{}, errors.New("install_command must exactly match the package coordinate and exact version")
	}
	if !validSDKURL(input.DocumentationURL) || !validSDKURL(input.SourceURL) {
		return model.SDKRelease{}, errors.New("SDK documentation and source URLs must be fixed public HTTPS URLs")
	}
	if input.UpstreamDigest != "" && !sdkChecksumPattern.MatchString(input.UpstreamDigest) {
		return model.SDKRelease{}, errors.New("upstream_digest must be a lowercase sha256, sha384, or sha512 digest")
	}
	if input.IdentityAssurance == "verified_digest" && input.UpstreamDigest == "" {
		return model.SDKRelease{}, errors.New("verified_digest assurance requires an upstream digest")
	}
	switch input.IdentityAssurance {
	case "metadata_only", "resolved_source", "verified_digest":
	default:
		return model.SDKRelease{}, errors.New("identity_assurance must be metadata_only, resolved_source, or verified_digest")
	}
	switch input.Lifecycle {
	case "active", "deprecated", "yanked", "archived":
	default:
		return model.SDKRelease{}, errors.New("SDK release lifecycle must be active, deprecated, yanked, or archived")
	}
	if !input.Visibility.Valid() || (input.Visibility == model.VisibilityPublic && parent.Visibility != model.VisibilityPublic) {
		return model.SDKRelease{}, errors.New("an SDK release cannot widen package visibility")
	}
	canonical, err := json.Marshal(map[string]any{
		"sdk_package_id": parent.ID, "ecosystem": parent.Ecosystem, "coordinate": parent.CanonicalCoordinate,
		"exact_version": input.ExactVersion, "install_command": input.InstallCommand,
		"documentation_url": input.DocumentationURL, "source_url": input.SourceURL,
		"resolved_source_revision": input.ResolvedSourceRevision, "upstream_digest": input.UpstreamDigest,
		"identity_assurance": input.IdentityAssurance, "visibility": input.Visibility,
	})
	if err != nil {
		return model.SDKRelease{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.SDKRelease{}, err
	}
	value := model.SDKRelease{
		ID: id, DeploymentID: deployment.ID, SDKPackageID: parent.ID, ExactVersion: input.ExactVersion,
		InstallCommand: input.InstallCommand, DocumentationURL: input.DocumentationURL, SourceURL: input.SourceURL,
		ResolvedSourceRevision: input.ResolvedSourceRevision, UpstreamDigest: input.UpstreamDigest,
		IdentityAssurance: input.IdentityAssurance, Visibility: input.Visibility, Lifecycle: input.Lifecycle,
		ReleaseHash: contentHash(canonical),
	}
	if input.Lifecycle == "active" {
		now := s.now()
		value.PublishedAt = &now
	}
	value, err = s.store.CreateSDKRelease(ctx, value)
	if err != nil {
		return model.SDKRelease{}, err
	}
	if err := s.appendDeveloperAssetAudit(ctx, deployment, actor, "sdk_release.created", "sdk_release", value.ID, map[string]any{
		"sdk_package_id": parent.ID, "exact_version": value.ExactVersion, "release_hash": value.ReleaseHash,
		"visibility": value.Visibility, "identity_assurance": value.IdentityAssurance,
	}); err != nil {
		return model.SDKRelease{}, err
	}
	return value, nil
}

type APISDKBindingInput struct {
	SDKPackageID             string
	SDKReleaseID             string
	SDKContentPublicationID  string
	APIContractRevisionID    string
	CompatibilityAssertionID string
	State                    string
	Coverage                 model.SDKCompatibilityCoverage
	Assurance                model.SDKCompatibilityAssurance
	ApplicableModules        []string
	ApplicableCapabilities   []string
	ApplicableOperationKeys  []string
	Selector                 json.RawMessage
	Visibility               model.Visibility
	Revision                 int64
}

func normalizeJSONObject(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if len(raw) > 1<<20 {
		return nil, errors.New("JSON object exceeds 1 MiB")
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, errors.New("value must be a JSON object")
	}
	return json.Marshal(value)
}

func normalizeStringSet(values []string, maximum int) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 240 || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
		if len(result) > maximum {
			return nil, errors.New("too many selector values")
		}
	}
	return result, nil
}

func (s *Service) SaveAPISDKBinding(ctx context.Context, apiID, bindingID string, input APISDKBindingInput, actor Actor) (model.APISDKBinding, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.APISDKBinding{}, err
	}
	api, err := s.store.Integration(ctx, deployment.ID, strings.TrimSpace(apiID))
	if err != nil {
		return model.APISDKBinding{}, err
	}
	packageValue, err := s.store.SDKPackage(ctx, deployment.ID, strings.TrimSpace(input.SDKPackageID))
	if err != nil {
		return model.APISDKBinding{}, err
	}
	release, err := s.store.SDKRelease(ctx, deployment.ID, strings.TrimSpace(input.SDKReleaseID))
	if err != nil || release.SDKPackageID != packageValue.ID {
		return model.APISDKBinding{}, errors.New("SDK release does not belong to the selected package")
	}
	if err := s.ensureSDKReleaseSelectable(ctx, deployment.ID, release); err != nil {
		return model.APISDKBinding{}, err
	}
	var contentPublication *model.SDKContentPublication
	if input.SDKContentPublicationID != "" {
		publication, publicationErr := s.store.SDKContentPublication(ctx, deployment.ID, strings.TrimSpace(input.SDKContentPublicationID))
		if publicationErr != nil || publication.Publication.SDKReleaseID != release.ID {
			return model.APISDKBinding{}, errors.New("SDK content publication does not belong to the selected exact release")
		}
		contentPublication = &publication.Publication
	}
	if input.Visibility == "" {
		input.Visibility = release.Visibility
	}
	if !input.Visibility.Valid() ||
		(input.Visibility == model.VisibilityPublic &&
			(packageValue.Visibility != model.VisibilityPublic || release.Visibility != model.VisibilityPublic ||
				(contentPublication != nil && contentPublication.Visibility != model.VisibilityPublic))) {
		return model.APISDKBinding{}, errors.New("SDK binding visibility cannot widen package, release, or content-publication visibility")
	}
	if api.Visibility == model.VisibilityPublic && input.Visibility != model.VisibilityPublic {
		return model.APISDKBinding{}, errors.New("a public API can only bind public SDK releases")
	}
	if input.State == "" {
		input.State = "draft"
	}
	switch input.State {
	case "draft", "ready":
	default:
		return model.APISDKBinding{}, errors.New("SDK binding state must be draft or ready")
	}
	if input.Coverage == "" {
		input.Coverage = model.SDKCoverageUnknown
	}
	if input.Assurance == "" {
		input.Assurance = model.SDKAssuranceRelated
	}
	if !input.Coverage.Valid() || !input.Assurance.Valid() {
		return model.APISDKBinding{}, errors.New("SDK compatibility coverage or assurance is invalid")
	}
	contractRevisionID := strings.TrimSpace(input.APIContractRevisionID)
	if contractRevisionID != "" {
		contractRevision, lookupErr := s.store.APIContractRevision(ctx, deployment.ID, contractRevisionID)
		if lookupErr != nil {
			return model.APISDKBinding{}, errors.New("SDK binding contract revision does not exist")
		}
		bindings, lookupErr := s.store.APIContractBindings(ctx, deployment.ID, api.ID)
		if lookupErr != nil {
			return model.APISDKBinding{}, lookupErr
		}
		attached := false
		for _, binding := range bindings {
			if binding.Lifecycle == "attached" && binding.APIContractID == contractRevision.APIContractID {
				attached = true
				break
			}
		}
		if !attached {
			return model.APISDKBinding{}, errors.New("SDK binding contract revision must belong to a contract attached to the API")
		}
	}
	assertionID := strings.TrimSpace(input.CompatibilityAssertionID)
	if assertionID != "" {
		assertion, lookupErr := s.store.SDKCompatibilityAssertion(ctx, deployment.ID, assertionID)
		if lookupErr != nil || assertion.APIID != api.ID || assertion.SDKReleaseID != release.ID {
			return model.APISDKBinding{}, errors.New("SDK compatibility assertion does not belong to the selected API and exact release")
		}
		if assertion.State != "active" || assertion.APIContractRevisionID != contractRevisionID ||
			assertion.Coverage != input.Coverage || assertion.Assurance != input.Assurance {
			return model.APISDKBinding{}, errors.New("SDK binding must copy an active compatibility assertion's exact contract, coverage, and assurance")
		}
	}
	input.Selector, err = normalizeJSONObject(input.Selector)
	if err != nil {
		return model.APISDKBinding{}, err
	}
	modules, err := normalizeStringSet(input.ApplicableModules, 200)
	if err != nil {
		return model.APISDKBinding{}, err
	}
	capabilities, err := normalizeStringSet(input.ApplicableCapabilities, 200)
	if err != nil {
		return model.APISDKBinding{}, err
	}
	operations, err := normalizeStringSet(input.ApplicableOperationKeys, 1000)
	if err != nil {
		return model.APISDKBinding{}, err
	}
	bindingID = strings.TrimSpace(bindingID)
	if bindingID == "" {
		bindingID, err = randomUUID()
		if err != nil {
			return model.APISDKBinding{}, err
		}
	}
	value := model.APISDKBinding{
		ID: bindingID, DeploymentID: deployment.ID, APIID: api.ID, SDKPackageID: packageValue.ID, SDKReleaseID: release.ID,
		SDKContentPublicationID: strings.TrimSpace(input.SDKContentPublicationID), APIContractRevisionID: contractRevisionID,
		CompatibilityAssertionID: assertionID, State: input.State, Coverage: input.Coverage,
		Assurance: input.Assurance, ApplicableModules: modules, ApplicableCapabilities: capabilities, ApplicableOperationKeys: operations,
		Selector: input.Selector, SelectorHash: contentHash(input.Selector), Visibility: input.Visibility,
	}
	if !value.Valid() {
		return model.APISDKBinding{}, errors.New("SDK binding is incomplete for its selected state and assurance")
	}
	value, err = s.store.SaveAPISDKBinding(ctx, value, input.Revision)
	if err != nil {
		return model.APISDKBinding{}, err
	}
	if err := s.appendDeveloperAssetAudit(ctx, deployment, actor, "api.sdk_binding.saved", "api_sdk_binding", value.ID, map[string]any{
		"api_id": api.ID, "sdk_package_id": packageValue.ID, "sdk_release_id": release.ID,
		"exact_version": release.ExactVersion, "visibility": value.Visibility, "revision": value.Revision,
	}); err != nil {
		return model.APISDKBinding{}, err
	}
	return value, nil
}

func (s *Service) DetachAPISDKBinding(ctx context.Context, apiID, bindingID string, revision int64, actor Actor) (model.APISDKBinding, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.APISDKBinding{}, err
	}
	value, err := s.store.DetachAPISDKBinding(ctx, deployment.ID, strings.TrimSpace(apiID), strings.TrimSpace(bindingID), revision)
	if err != nil {
		return model.APISDKBinding{}, err
	}
	if err := s.appendDeveloperAssetAudit(ctx, deployment, actor, "api.sdk_binding.detached", "api_sdk_binding", value.ID, map[string]any{
		"api_id": value.APIID, "sdk_release_id": value.SDKReleaseID, "revision": value.Revision,
	}); err != nil {
		return model.APISDKBinding{}, err
	}
	return value, nil
}

func (s *Service) appendDeveloperAssetAudit(ctx context.Context, deployment model.Deployment, actor Actor, action, targetType, targetID string, current map[string]any) error {
	return s.store.AppendAudit(ctx, model.AuditEvent{
		ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID,
		ActorID: actor.ID, Action: action, TargetType: targetType, TargetID: targetID,
		Current: current, RequestID: actor.RequestID, CreatedAt: s.now(),
	})
}
