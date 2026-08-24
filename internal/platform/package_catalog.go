package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

var (
	packageEcosystemPattern     = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)
	packagePURLTypePattern      = regexp.MustCompile(`^[a-z][a-z0-9.+-]{0,63}$`)
	packageDigestPattern        = regexp.MustCompile(`^(sha256|sha384|sha512):([0-9a-f]+)$`)
	packageInstallSecretPattern = regexp.MustCompile(`(?i)(?:^|[\s;&|])(?:[A-Z0-9_]*(?:TOKEN|PASSWORD|PASSWD|SECRET|API_KEY|AUTH)[A-Z0-9_]*)\s*=|--(?:token|password|passwd|api[-_]?key|client[-_]?secret)(?:=|\s)|(?:_authToken|authToken|access[_-]?token|api[_-]?key|client[_-]?secret|password|passwd)\s*(?:=|:|\s)|https?://[^\s/@:]+:[^\s/@]+@`)
)

type PackageArtifactInput struct {
	Name              string
	Description       string
	Ecosystem         string
	Coordinate        string
	PURL              string
	RegistryURL       string
	SourceURL         string
	Language          string
	Platform          string
	Visibility        model.Visibility
	AcknowledgePublic bool
	Revision          int64
}

func validPackageText(value string, maximum int) bool {
	if value == "" || len(value) > maximum {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

type parsedPackagePURL struct {
	Type    string
	Base    string
	Version string
}

func parsePackagePURL(value string) (parsedPackagePURL, error) {
	if len(value) < len("pkg:a/x") || len(value) > 1000 || !strings.HasPrefix(value, "pkg:") || strings.TrimSpace(value) != value || strings.ContainsAny(value, "?#\t\r\n ") {
		return parsedPackagePURL{}, errors.New("package URL must be a query-free and fragment-free canonical PURL")
	}
	remainder := strings.TrimPrefix(value, "pkg:")
	separator := strings.IndexByte(remainder, '/')
	if separator <= 0 || separator == len(remainder)-1 {
		return parsedPackagePURL{}, errors.New("package URL must include a type and package path")
	}
	purlType := remainder[:separator]
	if !packagePURLTypePattern.MatchString(purlType) {
		return parsedPackagePURL{}, errors.New("package URL type is invalid")
	}
	pathAndVersion := remainder[separator+1:]
	if strings.Contains(pathAndVersion, "//") || strings.Contains(pathAndVersion, "://") {
		return parsedPackagePURL{}, errors.New("package URL path is invalid")
	}
	versionSeparator := strings.LastIndexByte(pathAndVersion, '@')
	lastSlash := strings.LastIndexByte(pathAndVersion, '/')
	base, version := value, ""
	packagePath := pathAndVersion
	if versionSeparator > lastSlash {
		if versionSeparator == len(pathAndVersion)-1 {
			return parsedPackagePURL{}, errors.New("package URL version is empty")
		}
		packagePath = pathAndVersion[:versionSeparator]
		base = value[:len("pkg:")+separator+1+versionSeparator]
		decodedVersion, err := url.PathUnescape(pathAndVersion[versionSeparator+1:])
		if err != nil || decodedVersion == "" || strings.ContainsAny(decodedVersion, "?#\r\n\t") {
			return parsedPackagePURL{}, errors.New("package URL version is invalid")
		}
		version = decodedVersion
	}
	decodedPath, err := url.PathUnescape(packagePath)
	if err != nil || decodedPath == "" || strings.ContainsAny(decodedPath, "?#\r\n\t") {
		return parsedPackagePURL{}, errors.New("package URL path is invalid")
	}
	return parsedPackagePURL{Type: purlType, Base: base, Version: version}, nil
}

func normalizedPackagePURLType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "go":
		return "golang"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func packagePURLTypeCompatible(ecosystem, purlType string) bool {
	return normalizedPackagePURLType(ecosystem) == normalizedPackagePURLType(purlType)
}

func validPackageMetadataURI(value string) bool {
	if !validHTTPSURI(value) {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func normalizePackageArtifactInput(input PackageArtifactInput) (PackageArtifactInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Ecosystem = strings.ToLower(strings.TrimSpace(input.Ecosystem))
	input.Coordinate = strings.TrimSpace(input.Coordinate)
	input.PURL = strings.TrimSpace(input.PURL)
	input.RegistryURL = strings.TrimSpace(input.RegistryURL)
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	input.Language = strings.TrimSpace(input.Language)
	input.Platform = strings.TrimSpace(input.Platform)
	parsedPURL, purlErr := parsePackagePURL(input.PURL)
	if !validPackageText(input.Name, 120) || len(input.Description) > 2000 || !packageEcosystemPattern.MatchString(input.Ecosystem) || !validPackageText(input.Coordinate, 500) || purlErr != nil || parsedPURL.Version != "" || !packagePURLTypeCompatible(input.Ecosystem, parsedPURL.Type) {
		return input, errors.New("package name, ecosystem, coordinate, or PURL is invalid")
	}
	if !validPackageMetadataURI(input.RegistryURL) || (input.SourceURL != "" && !validPackageMetadataURI(input.SourceURL)) {
		return input, errors.New("package registry and source must be credential-free, query-free, fragment-free HTTPS URLs (loopback HTTP is allowed for local development)")
	}
	if len(input.Language) > 120 || len(input.Platform) > 200 {
		return input, errors.New("package language or platform is too long")
	}
	if input.Visibility == "" {
		input.Visibility = model.VisibilityPrivate
	}
	if !input.Visibility.Valid() {
		return input, ErrInvalidVisibility
	}
	return input, nil
}

func (s *Service) CreatePackageArtifact(ctx context.Context, input PackageArtifactInput, actor Actor) (model.PackageArtifact, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	input, err = normalizePackageArtifactInput(input)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	if input.Visibility == model.VisibilityPublic && !input.AcknowledgePublic {
		return model.PackageArtifact{}, ErrConfirmationRequired
	}
	id, err := randomUUID()
	if err != nil {
		return model.PackageArtifact{}, err
	}
	value, err := s.store.CreatePackageArtifact(ctx, model.PackageArtifact{ID: id, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, Name: input.Name, Description: input.Description, Ecosystem: input.Ecosystem, Coordinate: input.Coordinate, PURL: input.PURL, RegistryURL: input.RegistryURL, SourceURL: input.SourceURL, Language: input.Language, Platform: input.Platform, Visibility: input.Visibility, Lifecycle: "draft"})
	if err != nil {
		return model.PackageArtifact{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "package_artifact.created", TargetType: "package_artifact", TargetID: value.ID, Current: map[string]any{"ecosystem": value.Ecosystem, "coordinate": value.Coordinate, "visibility": value.Visibility, "delivery": "external_registry"}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, nil
}

func (s *Service) UpdatePackageArtifact(ctx context.Context, artifactID string, input PackageArtifactInput, actor Actor) (model.PackageArtifact, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	current, err := s.store.PackageArtifact(ctx, deployment.ID, artifactID)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	input, err = normalizePackageArtifactInput(input)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	if current.Lifecycle != "draft" {
		return model.PackageArtifact{}, errors.New("published package identity metadata is immutable; create a replacement artifact instead")
	}
	if current.Visibility != model.VisibilityPublic && input.Visibility == model.VisibilityPublic && !input.AcknowledgePublic {
		return model.PackageArtifact{}, ErrConfirmationRequired
	}
	current.Name, current.Description = input.Name, input.Description
	current.Ecosystem, current.Coordinate, current.PURL = input.Ecosystem, input.Coordinate, input.PURL
	current.RegistryURL, current.SourceURL = input.RegistryURL, input.SourceURL
	current.Language, current.Platform, current.Visibility = input.Language, input.Platform, input.Visibility
	updated, err := s.store.UpdatePackageArtifact(ctx, current, input.Revision)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "package_artifact.updated", TargetType: "package_artifact", TargetID: updated.ID, Current: map[string]any{"ecosystem": updated.Ecosystem, "coordinate": updated.Coordinate, "visibility": updated.Visibility, "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

type PackageReleaseInput struct {
	Version           string
	PURL              string
	InstallCommand    string
	Digest            string
	ProvenanceURL     string
	SBOMURL           string
	ArtifactRevision  int64
	AcknowledgePublic bool
}

func validPackageDigest(value string) bool {
	matches := packageDigestPattern.FindStringSubmatch(value)
	if len(matches) != 3 {
		return false
	}
	want := map[string]int{"sha256": 64, "sha384": 96, "sha512": 128}[matches[1]]
	return len(matches[2]) == want
}

func normalizePackageReleaseInput(input PackageReleaseInput, artifact model.PackageArtifact) (PackageReleaseInput, error) {
	input.Version = strings.TrimSpace(input.Version)
	input.PURL = strings.TrimSpace(input.PURL)
	input.InstallCommand = strings.TrimSpace(input.InstallCommand)
	input.Digest = strings.ToLower(strings.TrimSpace(input.Digest))
	input.ProvenanceURL = strings.TrimSpace(input.ProvenanceURL)
	input.SBOMURL = strings.TrimSpace(input.SBOMURL)
	parsedPURL, purlErr := parsePackagePURL(input.PURL)
	if !validPackageText(input.Version, 200) || purlErr != nil || parsedPURL.Version != input.Version || parsedPURL.Base != artifact.PURL || !packagePURLTypeCompatible(artifact.Ecosystem, parsedPURL.Type) || !validPackageText(input.InstallCommand, 2000) || packageInstallSecretPattern.MatchString(input.InstallCommand) || !validPackageDigest(input.Digest) {
		return input, errors.New("exact package version, release PURL, install command, or digest is invalid")
	}
	if (input.ProvenanceURL != "" && !validPackageMetadataURI(input.ProvenanceURL)) || (input.SBOMURL != "" && !validPackageMetadataURI(input.SBOMURL)) {
		return input, errors.New("provenance and SBOM locations must be credential-free, query-free, fragment-free HTTPS URLs (loopback HTTP is allowed for local development)")
	}
	return input, nil
}

func packageArtifactUnavailableMessage(artifact model.PackageArtifact, now time.Time) string {
	unavailable := artifact.Lifecycle != "active"
	if artifact.SunsetAt != nil && !artifact.SunsetAt.After(now) {
		unavailable = true
	}
	if !unavailable {
		return ""
	}
	parts := []string{fmt.Sprintf("package %s is %s", artifact.Name, artifact.Lifecycle)}
	if artifact.ReplacementPackageArtifactID != "" {
		parts = append(parts, "replacement="+artifact.ReplacementPackageArtifactID)
	}
	if artifact.DeprecationMessage != "" {
		parts = append(parts, "message="+artifact.DeprecationMessage)
	}
	if artifact.SunsetAt != nil {
		parts = append(parts, "sunset_at="+artifact.SunsetAt.UTC().Format(time.RFC3339))
	}
	return strings.Join(parts, "; ")
}

func packageArtifactCanPublish(artifact model.PackageArtifact, now time.Time) bool {
	if artifact.Lifecycle != "draft" && artifact.Lifecycle != "active" {
		return false
	}
	return artifact.SunsetAt == nil || artifact.SunsetAt.After(now)
}

func packageReleaseHash(value model.PackageRelease) (string, error) {
	canonical, err := json.Marshal(struct {
		ArtifactName   string           `json:"artifact_name"`
		Ecosystem      string           `json:"ecosystem"`
		Coordinate     string           `json:"coordinate"`
		Version        string           `json:"version"`
		PURL           string           `json:"purl"`
		RegistryURL    string           `json:"registry_url"`
		SourceURL      string           `json:"source_url,omitempty"`
		Language       string           `json:"language,omitempty"`
		Platform       string           `json:"platform,omitempty"`
		InstallCommand string           `json:"install_command"`
		Digest         string           `json:"digest"`
		ProvenanceURL  string           `json:"provenance_url,omitempty"`
		SBOMURL        string           `json:"sbom_url,omitempty"`
		Visibility     model.Visibility `json:"visibility"`
	}{value.ArtifactName, value.Ecosystem, value.Coordinate, value.Version, value.PURL, value.RegistryURL, value.SourceURL, value.Language, value.Platform, value.InstallCommand, value.Digest, value.ProvenanceURL, value.SBOMURL, value.Visibility})
	if err != nil {
		return "", err
	}
	return contentHash(canonical), nil
}

func (s *Service) PublishPackageArtifact(ctx context.Context, artifactID string, input PackageReleaseInput, actor Actor) (model.PackageArtifact, model.PackageRelease, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.PackageArtifact{}, model.PackageRelease{}, err
	}
	artifact, err := s.store.PackageArtifact(ctx, deployment.ID, artifactID)
	if err != nil {
		return model.PackageArtifact{}, model.PackageRelease{}, err
	}
	now := s.now()
	input, err = normalizePackageReleaseInput(input, artifact)
	if err != nil {
		return model.PackageArtifact{}, model.PackageRelease{}, err
	}
	if !packageArtifactCanPublish(artifact, now) {
		return model.PackageArtifact{}, model.PackageRelease{}, errors.New("deprecated, retired, or sunset package artifacts cannot publish releases")
	}
	if artifact.Visibility == model.VisibilityPublic && !input.AcknowledgePublic {
		return model.PackageArtifact{}, model.PackageRelease{}, ErrConfirmationRequired
	}
	id, err := randomUUID()
	if err != nil {
		return model.PackageArtifact{}, model.PackageRelease{}, err
	}
	release := model.PackageRelease{ID: id, PackageArtifactID: artifact.ID, ArtifactName: artifact.Name, Ecosystem: artifact.Ecosystem, Coordinate: artifact.Coordinate, Version: input.Version, PURL: input.PURL, RegistryURL: artifact.RegistryURL, SourceURL: artifact.SourceURL, Language: artifact.Language, Platform: artifact.Platform, InstallCommand: input.InstallCommand, Digest: input.Digest, ProvenanceURL: input.ProvenanceURL, SBOMURL: input.SBOMURL, Visibility: artifact.Visibility, PublishedBy: actor.ID, PublishedAt: now}
	release.ContentHash, err = packageReleaseHash(release)
	if err != nil {
		return model.PackageArtifact{}, model.PackageRelease{}, err
	}
	updatedArtifact, createdRelease, err := s.store.CreatePackageRelease(ctx, deployment.ID, release, input.ArtifactRevision)
	if err != nil {
		return model.PackageArtifact{}, model.PackageRelease{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "package_release.published", TargetType: "package_release", TargetID: createdRelease.ID, Current: map[string]any{"package_artifact_id": artifact.ID, "version": createdRelease.Version, "digest": createdRelease.Digest, "content_hash": createdRelease.ContentHash, "delivery": "external_registry"}, RequestID: actor.RequestID, CreatedAt: now})
	return updatedArtifact, createdRelease, nil
}

type PackageDeprecationInput struct {
	ReplacementPackageArtifactID string
	Message                      string
	SunsetAt                     *time.Time
	Revision                     int64
}

type PackageRetirementInput struct {
	ReplacementPackageArtifactID string
	Message                      string
	Revision                     int64
}

func validatePackageLifecycleMessage(message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" || len(message) > 1000 {
		return message, errors.New("a lifecycle message of no more than 1000 characters is required")
	}
	return message, nil
}

func (s *Service) validatePackageReplacement(ctx context.Context, deploymentID, artifactID, replacementID string) error {
	if replacementID == "" {
		return nil
	}
	if replacementID == artifactID {
		return errors.New("a package artifact cannot replace itself")
	}
	replacement, err := s.store.PackageArtifact(ctx, deploymentID, replacementID)
	if err != nil || replacement.Lifecycle != "active" || replacement.LatestRelease == nil || packageArtifactUnavailableMessage(replacement, s.now()) != "" {
		return errors.New("replacement package artifact must be active and have a published release")
	}
	return nil
}

func (s *Service) DeprecatePackageArtifact(ctx context.Context, artifactID string, input PackageDeprecationInput, actor Actor) (model.PackageArtifact, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	current, err := s.store.PackageArtifact(ctx, deployment.ID, artifactID)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	input.ReplacementPackageArtifactID = strings.TrimSpace(input.ReplacementPackageArtifactID)
	input.Message, err = validatePackageLifecycleMessage(input.Message)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	if input.SunsetAt != nil && !input.SunsetAt.After(s.now()) {
		return model.PackageArtifact{}, errors.New("package sunset must be in the future")
	}
	if current.Lifecycle == "retired" {
		return model.PackageArtifact{}, errors.New("retired package artifacts cannot be deprecated")
	}
	if err := s.validatePackageReplacement(ctx, deployment.ID, artifactID, input.ReplacementPackageArtifactID); err != nil {
		return model.PackageArtifact{}, err
	}
	current.Lifecycle = "deprecated"
	current.ReplacementPackageArtifactID = input.ReplacementPackageArtifactID
	current.DeprecationMessage, current.SunsetAt = input.Message, input.SunsetAt
	updated, err := s.store.UpdatePackageArtifact(ctx, current, input.Revision)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "package_artifact.deprecated", TargetType: "package_artifact", TargetID: updated.ID, Current: map[string]any{"replacement_package_artifact_id": updated.ReplacementPackageArtifactID, "sunset_at": updated.SunsetAt, "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func (s *Service) RetirePackageArtifact(ctx context.Context, artifactID string, input PackageRetirementInput, actor Actor) (model.PackageArtifact, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	current, err := s.store.PackageArtifact(ctx, deployment.ID, artifactID)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	input.ReplacementPackageArtifactID = strings.TrimSpace(input.ReplacementPackageArtifactID)
	input.Message, err = validatePackageLifecycleMessage(input.Message)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	if current.Lifecycle == "retired" {
		return model.PackageArtifact{}, errors.New("package artifact is already retired")
	}
	if err := s.validatePackageReplacement(ctx, deployment.ID, artifactID, input.ReplacementPackageArtifactID); err != nil {
		return model.PackageArtifact{}, err
	}
	current.Lifecycle = "retired"
	current.ReplacementPackageArtifactID = input.ReplacementPackageArtifactID
	current.DeprecationMessage = input.Message
	updated, err := s.store.UpdatePackageArtifact(ctx, current, input.Revision)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "package_artifact.retired", TargetType: "package_artifact", TargetID: updated.ID, Current: map[string]any{"replacement_package_artifact_id": updated.ReplacementPackageArtifactID, "sunset_at": updated.SunsetAt, "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func (s *Service) BindPackageRelease(ctx context.Context, integrationID, releaseID string, actor Actor) (model.IntegrationPackageBinding, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	integration, err := s.store.Integration(ctx, deployment.ID, integrationID)
	if err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	release, err := s.store.PackageRelease(ctx, deployment.ID, strings.TrimSpace(releaseID))
	if err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	artifact, err := s.store.PackageArtifact(ctx, deployment.ID, release.PackageArtifactID)
	if err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	if unavailable := packageArtifactUnavailableMessage(artifact, s.now()); unavailable != "" {
		return model.IntegrationPackageBinding{}, fmt.Errorf("unavailable package artifacts cannot be bound: %s", unavailable)
	}
	if integration.Visibility == model.VisibilityPublic && release.Visibility != model.VisibilityPublic {
		return model.IntegrationPackageBinding{}, errors.New("a public integration cannot expose a private package release")
	}
	id, err := randomUUID()
	if err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	value, err := s.store.SaveIntegrationPackageBinding(ctx, model.IntegrationPackageBinding{ID: id, IntegrationID: integration.ID, PackageArtifactID: artifact.ID, PackageReleaseID: release.ID, CreatedBy: actor.ID})
	if err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.package_release.bound", TargetType: "integration", TargetID: integration.ID, Current: map[string]any{"package_artifact_id": artifact.ID, "package_release_id": release.ID, "version": release.Version}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, nil
}

func (s *Service) UnbindPackageArtifact(ctx context.Context, integrationID, artifactID string, actor Actor) error {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return err
	}
	if _, err := s.store.Integration(ctx, deployment.ID, integrationID); err != nil {
		return err
	}
	if err := s.store.DeleteIntegrationPackageBinding(ctx, integrationID, artifactID); err != nil {
		return err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.package_release.unbound", TargetType: "integration", TargetID: integrationID, Current: map[string]any{"package_artifact_id": artifactID}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return nil
}
