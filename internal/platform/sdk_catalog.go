package platform

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

var (
	sdkChecksumPattern        = regexp.MustCompile(`^(sha256|sha384|sha512):[a-f0-9]+$`)
	sdkNPMCoordinatePattern   = regexp.MustCompile(`^(?:@[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?/)?[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?$`)
	sdkPyPICoordinatePattern  = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$`)
	sdkGoCoordinatePattern    = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?(?:/[A-Za-z0-9](?:[A-Za-z0-9._~-]*[A-Za-z0-9])?)+$`)
	sdkCargoCoordinatePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9_-]*[A-Za-z0-9])?$`)
	sdkSemverPattern          = regexp.MustCompile(`^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
	sdkCargoVersionPattern    = regexp.MustCompile(`^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
	sdkGoVersionPattern       = regexp.MustCompile(`^v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+incompatible)?$`)
	sdkPyPIVersionPattern     = regexp.MustCompile(`(?i)^[0-9]+(?:\.[0-9]+)*(?:(?:a|b|rc)[0-9]+)?(?:\.post[0-9]+)?(?:\.dev[0-9]+)?(?:\+[a-z0-9]+(?:[._-][a-z0-9]+)*)?$`)
)

type SDKReferenceInput struct {
	Ecosystem        string
	Coordinate       string
	ExactVersion     string
	InstallCommand   string
	DocumentationURL string
	SourceURL        string
	Checksum         string
	Visibility       model.Visibility
	Revision         int64
}

func validSDKURL(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

func sdkCommandHasUnsafeWhitespace(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsControl(r) || (unicode.IsSpace(r) && r != ' ')
	}) >= 0
}

func canonicalSDKInstallCommand(ecosystem, coordinate, exactVersion string) (string, error) {
	switch ecosystem {
	case "npm":
		if !sdkNPMCoordinatePattern.MatchString(coordinate) {
			return "", errors.New("npm coordinate must be one registry package name, optionally with a scope")
		}
		if !sdkSemverPattern.MatchString(exactVersion) {
			return "", errors.New("npm exact_version must be one complete semantic version")
		}
		return fmt.Sprintf("npm install %s@%s", coordinate, exactVersion), nil
	case "pypi":
		if !sdkPyPICoordinatePattern.MatchString(coordinate) {
			return "", errors.New("pypi coordinate must be one registry project name without extras or a URL")
		}
		if !sdkPyPIVersionPattern.MatchString(exactVersion) {
			return "", errors.New("pypi exact_version must be one fixed PEP 440 version without shell-significant epoch syntax")
		}
		return fmt.Sprintf("python -m pip install %s==%s", coordinate, exactVersion), nil
	case "go":
		firstElement, _, _ := strings.Cut(coordinate, "/")
		if !strings.Contains(firstElement, ".") || !sdkGoCoordinatePattern.MatchString(coordinate) {
			return "", errors.New("go coordinate must be one module path, not a URL or package pattern")
		}
		if !sdkGoVersionPattern.MatchString(exactVersion) {
			return "", errors.New("go exact_version must be one canonical v-prefixed semantic or pseudo-version")
		}
		return fmt.Sprintf("go get %s@%s", coordinate, exactVersion), nil
	case "cargo":
		if !sdkCargoCoordinatePattern.MatchString(coordinate) {
			return "", errors.New("cargo coordinate must be one registry crate name")
		}
		if !sdkCargoVersionPattern.MatchString(exactVersion) {
			return "", errors.New("cargo exact_version must be one complete semantic version without ignored build metadata")
		}
		return fmt.Sprintf("cargo add %s@=%s", coordinate, exactVersion), nil
	default:
		return "", errors.New("unsupported SDK ecosystem; supported ecosystems are npm, pypi, go, and cargo")
	}
}

func normalizeSDKReference(input SDKReferenceInput) (SDKReferenceInput, error) {
	rawInstallCommand := input.InstallCommand
	input.Ecosystem = strings.ToLower(strings.TrimSpace(input.Ecosystem))
	input.Coordinate = strings.TrimSpace(input.Coordinate)
	input.ExactVersion = strings.TrimSpace(input.ExactVersion)
	input.InstallCommand = strings.TrimSpace(input.InstallCommand)
	input.DocumentationURL = strings.TrimSpace(input.DocumentationURL)
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	input.Checksum = strings.ToLower(strings.TrimSpace(input.Checksum))
	if input.Visibility == "" {
		input.Visibility = model.VisibilityPrivate
	}
	if input.Ecosystem == "" || len(input.Ecosystem) > 40 || input.Coordinate == "" || len(input.Coordinate) > 240 || input.ExactVersion == "" || len(input.ExactVersion) > 120 || input.InstallCommand == "" || len(input.InstallCommand) > 500 {
		return SDKReferenceInput{}, errors.New("ecosystem, coordinate, exact_version, and install_command are required and must fit their limits")
	}
	if strings.EqualFold(input.ExactVersion, "latest") || strings.ContainsAny(input.ExactVersion, "*<>=~^") {
		return SDKReferenceInput{}, errors.New("exact_version must name one immutable version, not a range or latest")
	}
	if sdkCommandHasUnsafeWhitespace(rawInstallCommand) {
		return SDKReferenceInput{}, errors.New("install_command must be one canonical command on one line")
	}
	canonicalInstallCommand, err := canonicalSDKInstallCommand(input.Ecosystem, input.Coordinate, input.ExactVersion)
	if err != nil {
		return SDKReferenceInput{}, err
	}
	if input.InstallCommand != canonicalInstallCommand {
		return SDKReferenceInput{}, errors.New("install_command must exactly match the canonical command for ecosystem, coordinate, and exact_version")
	}
	input.InstallCommand = canonicalInstallCommand
	if !validSDKURL(input.DocumentationURL) || !validSDKURL(input.SourceURL) {
		return SDKReferenceInput{}, errors.New("SDK documentation and source URLs must be fixed public HTTPS URLs")
	}
	if input.Checksum != "" && !sdkChecksumPattern.MatchString(input.Checksum) {
		return SDKReferenceInput{}, errors.New("checksum must be sha256, sha384, or sha512 followed by a lowercase hexadecimal digest")
	}
	if input.Visibility != model.VisibilityPrivate && input.Visibility != model.VisibilityPublic {
		return SDKReferenceInput{}, errors.New("visibility must be private or public")
	}
	return input, nil
}

func (s *Service) SaveSDKReference(ctx context.Context, integrationID, referenceID string, input SDKReferenceInput, actor Actor) (model.SDKReference, error) {
	input, err := normalizeSDKReference(input)
	if err != nil {
		return model.SDKReference{}, err
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.SDKReference{}, err
	}
	integration, err := s.store.Integration(ctx, deployment.ID, strings.TrimSpace(integrationID))
	if err != nil {
		return model.SDKReference{}, err
	}
	if integration.Visibility == model.VisibilityPublic && input.Visibility != model.VisibilityPublic {
		return model.SDKReference{}, errors.New("a public API can only publish public SDK references")
	}
	if strings.TrimSpace(referenceID) == "" {
		referenceID, err = randomUUID()
		if err != nil {
			return model.SDKReference{}, err
		}
	}
	value, err := s.store.SaveSDKReference(ctx, model.SDKReference{ID: referenceID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, IntegrationID: integration.ID, Ecosystem: input.Ecosystem, Coordinate: input.Coordinate, ExactVersion: input.ExactVersion, InstallCommand: input.InstallCommand, DocumentationURL: input.DocumentationURL, SourceURL: input.SourceURL, Checksum: input.Checksum, Visibility: input.Visibility}, input.Revision)
	if err != nil {
		return model.SDKReference{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "sdk_reference.saved", TargetType: "sdk_reference", TargetID: value.ID, Current: map[string]any{"integration_id": integration.ID, "ecosystem": value.Ecosystem, "coordinate": value.Coordinate, "exact_version": value.ExactVersion, "visibility": value.Visibility, "revision": value.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func (s *Service) DeleteSDKReference(ctx context.Context, integrationID, referenceID string, actor Actor) error {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return err
	}
	integration, err := s.store.Integration(ctx, deployment.ID, strings.TrimSpace(integrationID))
	if err != nil {
		return err
	}
	if err := s.store.DeleteSDKReference(ctx, integration.ID, strings.TrimSpace(referenceID)); err != nil {
		return err
	}
	return s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "sdk_reference.deleted", TargetType: "sdk_reference", TargetID: referenceID, Current: map[string]any{"integration_id": integration.ID}, RequestID: actor.RequestID, CreatedAt: s.now()})
}
