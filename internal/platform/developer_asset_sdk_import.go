package platform

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/netpolicy"
)

const (
	maxSDKImportCredentialBytes = 16 << 10
	maxSDKImportResponseBytes   = 2 << 20
)

var (
	ErrSDKImportConflict     = errors.New("SDK package import conflicts with an existing immutable release")
	sdkImportRefPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/@+!-]{0,199}$`)
	sdkImportRevisionPattern = regexp.MustCompile(`^[a-f0-9]{40,64}$`)
)

// SDKImportDoer exists so resolver behavior can be tested without network
// access. Production requests use the DNS-pinned outbound boundary when no
// explicit doer is installed.
type SDKImportDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type SDKImportAuthenticationInput struct {
	Type       string `json:"type"`
	Username   string `json:"username,omitempty"`
	Credential string `json:"credential,omitempty"`
}

type SDKPackageImportInput struct {
	Ecosystem      string                       `json:"ecosystem"`
	SourceKind     string                       `json:"source_kind"`
	SourceURL      string                       `json:"source_url"`
	Coordinate     string                       `json:"coordinate"`
	ExactVersion   string                       `json:"exact_version"`
	SourceRef      string                       `json:"source_ref,omitempty"`
	Visibility     model.Visibility             `json:"visibility"`
	Authentication SDKImportAuthenticationInput `json:"authentication"`
}

type SDKPackageImportResult struct {
	Package         model.SDKPackage `json:"package"`
	Release         model.SDKRelease `json:"release"`
	SourceKind      string           `json:"source_kind"`
	AlreadyImported bool             `json:"already_imported"`
}

type sdkImportMetadata struct {
	Name                   string
	Description            string
	RegistryURL            string
	SourceURL              string
	DocumentationURL       string
	Language               string
	Platform               string
	ResolvedSourceRevision string
	UpstreamDigest         string
}

// SetSDKImportDoerForTesting installs a deterministic transport. DokoSoko still
// applies its own response limit and never follows redirects.
func (s *Service) SetSDKImportDoerForTesting(doer SDKImportDoer) { s.sdkImportDoer = doer }

func normalizeSDKPackageImportInput(input SDKPackageImportInput) (SDKPackageImportInput, *url.URL, error) {
	input.Ecosystem = strings.ToLower(strings.TrimSpace(input.Ecosystem))
	input.SourceKind = strings.ToLower(strings.TrimSpace(input.SourceKind))
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	input.Coordinate = strings.TrimSpace(input.Coordinate)
	input.ExactVersion = strings.TrimSpace(input.ExactVersion)
	input.SourceRef = strings.TrimSpace(input.SourceRef)
	input.Authentication.Type = strings.ToLower(strings.TrimSpace(input.Authentication.Type))
	input.Authentication.Username = strings.TrimSpace(input.Authentication.Username)
	if input.Visibility == "" {
		input.Visibility = model.VisibilityPrivate
	}
	if input.SourceKind != "registry" && input.SourceKind != "git" {
		return SDKPackageImportInput{}, nil, errors.New("source_kind must be registry or git")
	}
	if !input.Visibility.Valid() {
		return SDKPackageImportInput{}, nil, ErrInvalidVisibility
	}
	if _, err := canonicalSDKInstallCommand(input.Ecosystem, input.Coordinate, input.ExactVersion); err != nil {
		return SDKPackageImportInput{}, nil, fmt.Errorf("invalid exact package identity: %w", err)
	}
	if len(input.SourceURL) > 2048 {
		return SDKPackageImportInput{}, nil, errors.New("source_url must be no more than 2048 characters")
	}
	parsed, err := url.Parse(input.SourceURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return SDKPackageImportInput{}, nil, errors.New("source_url must be one credential-free HTTPS URL without a query or fragment")
	}
	localDevelopment := identity.IsLocalDevelopmentHostname(parsed.Hostname())
	if localDevelopment && parsed.Scheme != "http" || !localDevelopment && parsed.Scheme != "https" {
		return SDKPackageImportInput{}, nil, errors.New("source_url must use HTTPS, or HTTP only for local development")
	}
	if !localDevelopment && parsed.Port() != "" && parsed.Port() != "443" {
		return SDKPackageImportInput{}, nil, errors.New("public package sources must use HTTPS on port 443")
	}
	if address := net.ParseIP(parsed.Hostname()); address != nil && !localDevelopment && netpolicy.UnsafeIP(address) {
		return SDKPackageImportInput{}, nil, errors.New("source_url must not select a private or special-use network address")
	}
	if input.SourceKind == "git" {
		if input.SourceRef == "" {
			input.SourceRef = input.ExactVersion
		}
		if !sdkImportRefPattern.MatchString(input.SourceRef) {
			return SDKPackageImportInput{}, nil, errors.New("source_ref must be one bounded Git ref without whitespace or URL control characters")
		}
	} else if input.SourceRef != "" {
		return SDKPackageImportInput{}, nil, errors.New("source_ref is only valid for Git imports")
	}

	auth := &input.Authentication
	if auth.Type == "" {
		auth.Type = "none"
	}
	if len(auth.Credential) > maxSDKImportCredentialBytes || strings.ContainsAny(auth.Credential, "\r\n\x00") || strings.ContainsAny(auth.Username, "\r\n:\x00") {
		return SDKPackageImportInput{}, nil, errors.New("package credential is invalid or exceeds 16 KiB")
	}
	switch auth.Type {
	case "none":
		if auth.Username != "" || auth.Credential != "" {
			return SDKPackageImportInput{}, nil, errors.New("public package imports must not include credentials")
		}
	case "bearer":
		if auth.Username != "" || strings.TrimSpace(auth.Credential) == "" {
			return SDKPackageImportInput{}, nil, errors.New("bearer authentication requires one write-only token and no username")
		}
	case "basic":
		if auth.Username == "" || auth.Credential == "" {
			return SDKPackageImportInput{}, nil, errors.New("basic authentication requires a username and write-only password or token")
		}
	default:
		return SDKPackageImportInput{}, nil, errors.New("authentication type must be none, bearer, or basic")
	}
	return input, parsed, nil
}

func (s *Service) ImportSDKPackage(ctx context.Context, input SDKPackageImportInput, actor Actor) (SDKPackageImportResult, error) {
	input, source, err := normalizeSDKPackageImportInput(input)
	if err != nil {
		return SDKPackageImportResult{}, err
	}
	metadata, err := s.resolveSDKImport(ctx, input, source)
	if err != nil {
		return SDKPackageImportResult{}, err
	}
	metadata.Name = boundedSDKImportText(metadata.Name, 120)
	if metadata.Name == "" {
		metadata.Name = boundedSDKImportText(sdkImportFallbackName(input.Coordinate), 120)
	}
	metadata.Description = boundedSDKImportText(metadata.Description, 4000)
	metadata.RegistryURL = storedSDKImportURL(metadata.RegistryURL)
	metadata.SourceURL = storedSDKImportURL(metadata.SourceURL)
	metadata.DocumentationURL = storedSDKImportURL(metadata.DocumentationURL)

	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return SDKPackageImportResult{}, err
	}
	packages, err := s.store.SDKPackages(ctx, deployment.ID)
	if err != nil {
		return SDKPackageImportResult{}, err
	}
	canonicalCoordinate := model.CanonicalSDKCoordinate(input.Ecosystem, input.Coordinate)
	var packageValue model.SDKPackage
	for _, candidate := range packages {
		if candidate.Ecosystem == input.Ecosystem && candidate.CanonicalCoordinate == canonicalCoordinate {
			packageValue = candidate
			break
		}
	}
	if packageValue.ID != "" {
		if packageValue.Lifecycle == "archived" {
			return SDKPackageImportResult{}, fmt.Errorf("%w: the existing package root is archived", ErrSDKImportConflict)
		}
		if packageValue.Visibility != input.Visibility {
			return SDKPackageImportResult{}, fmt.Errorf("%w: the existing package visibility is %s", ErrSDKImportConflict, packageValue.Visibility)
		}
	} else {
		packageValue, err = s.SaveSDKPackage(ctx, "", SDKPackageInput{
			Ecosystem: input.Ecosystem, Coordinate: input.Coordinate, Name: metadata.Name,
			Description: metadata.Description, RegistryURL: metadata.RegistryURL, SourceURL: metadata.SourceURL,
			Language: metadata.Language, Platform: metadata.Platform, Visibility: input.Visibility, Lifecycle: "active",
		}, actor)
		if err != nil {
			return SDKPackageImportResult{}, err
		}
	}

	releases, err := s.store.SDKReleases(ctx, deployment.ID, packageValue.ID)
	if err != nil {
		return SDKPackageImportResult{}, err
	}
	for _, release := range releases {
		if release.ExactVersion != input.ExactVersion {
			continue
		}
		if metadata.ResolvedSourceRevision != "" && metadata.ResolvedSourceRevision != release.ResolvedSourceRevision {
			return SDKPackageImportResult{}, fmt.Errorf("%w: the existing release resolves to a different source revision", ErrSDKImportConflict)
		}
		if metadata.UpstreamDigest != "" && metadata.UpstreamDigest != release.UpstreamDigest {
			return SDKPackageImportResult{}, fmt.Errorf("%w: the existing release has a different upstream digest", ErrSDKImportConflict)
		}
		return SDKPackageImportResult{Package: packageValue, Release: release, SourceKind: input.SourceKind, AlreadyImported: true}, nil
	}

	identityAssurance := "metadata_only"
	if metadata.UpstreamDigest != "" {
		identityAssurance = "verified_digest"
	} else if metadata.ResolvedSourceRevision != "" {
		identityAssurance = "resolved_source"
	}
	release, err := s.CreateSDKRelease(ctx, packageValue.ID, SDKReleaseInput{
		ExactVersion: input.ExactVersion, DocumentationURL: metadata.DocumentationURL,
		SourceURL: metadata.SourceURL, ResolvedSourceRevision: metadata.ResolvedSourceRevision,
		UpstreamDigest: metadata.UpstreamDigest, IdentityAssurance: identityAssurance,
		Visibility: input.Visibility, Lifecycle: "active",
	}, actor)
	if err != nil {
		return SDKPackageImportResult{}, err
	}
	return SDKPackageImportResult{Package: packageValue, Release: release, SourceKind: input.SourceKind}, nil
}

func (s *Service) resolveSDKImport(ctx context.Context, input SDKPackageImportInput, source *url.URL) (sdkImportMetadata, error) {
	if input.SourceKind == "git" {
		return s.resolveSDKGitImport(ctx, input, source)
	}
	target, err := sdkRegistryMetadataURL(input, source)
	if err != nil {
		return sdkImportMetadata{}, err
	}
	switch input.Ecosystem {
	case "npm":
		return s.resolveNPMImport(ctx, input, target)
	case "pypi":
		return s.resolvePyPIImport(ctx, input, target)
	case "cargo":
		return s.resolveCargoImport(ctx, input, target)
	case "go":
		return s.resolveGoImport(ctx, input, target)
	default:
		return sdkImportMetadata{}, errors.New("unsupported SDK ecosystem")
	}
}

func sdkRegistryMetadataURL(input SDKPackageImportInput, source *url.URL) (*url.URL, error) {
	target := *source
	host := strings.ToLower(source.Hostname())
	switch input.Ecosystem {
	case "npm":
		if host == "npmjs.com" || host == "www.npmjs.com" {
			target = url.URL{Scheme: "https", Host: "registry.npmjs.org", Path: "/" + input.Coordinate, RawPath: "/" + url.PathEscape(input.Coordinate)}
		} else if source.EscapedPath() == "" || source.EscapedPath() == "/" {
			target.Path = "/" + input.Coordinate
			target.RawPath = "/" + url.PathEscape(input.Coordinate)
		}
	case "pypi":
		if host == "pypi.org" || source.EscapedPath() == "" || source.EscapedPath() == "/" {
			target.Path = path.Join("/pypi", input.Coordinate, input.ExactVersion, "json")
		}
	case "cargo":
		if host == "crates.io" || source.EscapedPath() == "" || source.EscapedPath() == "/" {
			target.Path = path.Join("/api/v1/crates", input.Coordinate, input.ExactVersion)
		}
	case "go":
		if host == "pkg.go.dev" {
			target = url.URL{Scheme: "https", Host: "proxy.golang.org"}
		}
		if !strings.Contains(target.EscapedPath(), "/@v/") {
			target.Path = path.Join(target.Path, goModuleProxyEscape(input.Coordinate), "@v", input.ExactVersion+".info")
		}
	default:
		return nil, errors.New("unsupported SDK ecosystem")
	}
	return &target, nil
}

func (s *Service) sdkImportJSON(ctx context.Context, input SDKPackageImportInput, target *url.URL, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return errors.New("package metadata request is invalid")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "DokoSokoPackageImporter/1.0")
	switch input.Authentication.Type {
	case "bearer":
		request.Header.Set("Authorization", "Bearer "+input.Authentication.Credential)
	case "basic":
		request.SetBasicAuth(input.Authentication.Username, input.Authentication.Credential)
	}
	var response *http.Response
	if s.sdkImportDoer != nil {
		response, err = s.sdkImportDoer.Do(request)
	} else {
		client, clientErr := identity.SafeOutboundClient(ctx, target, nil, nil)
		if clientErr != nil {
			return errors.New("package source did not resolve inside the permitted outbound network boundary")
		}
		response, err = client.Do(request)
	}
	if err != nil {
		return errors.New("package source could not be reached")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return errors.New("package source rejected the supplied authorization")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("package source returned HTTP %d", response.StatusCode)
	}
	limited := io.LimitReader(response.Body, maxSDKImportResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return errors.New("package metadata response could not be read")
	}
	if len(body) > maxSDKImportResponseBytes {
		return errors.New("package metadata response exceeds the 2 MiB limit")
	}
	if err := json.Unmarshal(body, output); err != nil {
		return errors.New("package source did not return supported JSON metadata")
	}
	return nil
}

func (s *Service) resolveNPMImport(ctx context.Context, input SDKPackageImportInput, target *url.URL) (sdkImportMetadata, error) {
	var value struct {
		Name        string                     `json:"name"`
		Description string                     `json:"description"`
		Homepage    string                     `json:"homepage"`
		Repository  json.RawMessage            `json:"repository"`
		Versions    map[string]json.RawMessage `json:"versions"`
	}
	if err := s.sdkImportJSON(ctx, input, target, &value); err != nil {
		return sdkImportMetadata{}, err
	}
	raw, ok := value.Versions[input.ExactVersion]
	if !ok {
		return sdkImportMetadata{}, errors.New("the npm source does not contain the requested exact version")
	}
	var release struct {
		Name        string          `json:"name"`
		Version     string          `json:"version"`
		Description string          `json:"description"`
		Homepage    string          `json:"homepage"`
		Repository  json.RawMessage `json:"repository"`
		Types       string          `json:"types"`
		Typings     string          `json:"typings"`
		Dist        struct {
			Tarball   string `json:"tarball"`
			Integrity string `json:"integrity"`
		} `json:"dist"`
	}
	if err := json.Unmarshal(raw, &release); err != nil || release.Version != input.ExactVersion {
		return sdkImportMetadata{}, errors.New("the npm source returned inconsistent exact-version metadata")
	}
	name := firstSDKImportValue(release.Name, value.Name)
	if model.CanonicalSDKCoordinate("npm", name) != model.CanonicalSDKCoordinate("npm", input.Coordinate) {
		return sdkImportMetadata{}, errors.New("the npm metadata name does not match the requested coordinate")
	}
	language := "JavaScript"
	if release.Types != "" || release.Typings != "" {
		language = "TypeScript"
	}
	return sdkImportMetadata{
		Name: name, Description: firstSDKImportValue(release.Description, value.Description),
		RegistryURL: input.SourceURL, SourceURL: firstSDKImportValue(repositoryURL(release.Repository), repositoryURL(value.Repository)),
		DocumentationURL: firstSDKImportValue(release.Homepage, value.Homepage), Language: language, Platform: "Node.js",
		UpstreamDigest: npmIntegrityDigest(release.Dist.Integrity),
	}, nil
}

func (s *Service) resolvePyPIImport(ctx context.Context, input SDKPackageImportInput, target *url.URL) (sdkImportMetadata, error) {
	var value struct {
		Info struct {
			Name        string            `json:"name"`
			Version     string            `json:"version"`
			Summary     string            `json:"summary"`
			Description string            `json:"description"`
			HomePage    string            `json:"home_page"`
			ProjectURLs map[string]string `json:"project_urls"`
		} `json:"info"`
		URLs []struct {
			URL     string `json:"url"`
			Digests struct {
				SHA256 string `json:"sha256"`
			} `json:"digests"`
		} `json:"urls"`
	}
	if err := s.sdkImportJSON(ctx, input, target, &value); err != nil {
		return sdkImportMetadata{}, err
	}
	if value.Info.Version != input.ExactVersion || model.CanonicalSDKCoordinate("pypi", value.Info.Name) != model.CanonicalSDKCoordinate("pypi", input.Coordinate) {
		return sdkImportMetadata{}, errors.New("the PyPI metadata does not match the requested coordinate and exact version")
	}
	projectURL := func(keys ...string) string {
		for _, key := range keys {
			for label, candidate := range value.Info.ProjectURLs {
				if strings.EqualFold(label, key) && candidate != "" {
					return candidate
				}
			}
		}
		return ""
	}
	digest := ""
	// A PyPI release may contain platform-specific files with different hashes.
	// Persist a release-level digest only when metadata identifies one artifact.
	if len(value.URLs) == 1 && regexp.MustCompile(`^[a-fA-F0-9]{64}$`).MatchString(value.URLs[0].Digests.SHA256) {
		digest = "sha256:" + strings.ToLower(value.URLs[0].Digests.SHA256)
	}
	return sdkImportMetadata{
		Name: value.Info.Name, Description: firstSDKImportValue(value.Info.Summary, value.Info.Description),
		RegistryURL: input.SourceURL, SourceURL: projectURL("Source", "Source Code", "Repository"),
		DocumentationURL: firstSDKImportValue(projectURL("Documentation", "Docs", "Homepage"), value.Info.HomePage),
		Language:         "Python", Platform: "Python", UpstreamDigest: digest,
	}, nil
}

func (s *Service) resolveCargoImport(ctx context.Context, input SDKPackageImportInput, target *url.URL) (sdkImportMetadata, error) {
	var value struct {
		Crate struct {
			Name          string `json:"name"`
			Description   string `json:"description"`
			Repository    string `json:"repository"`
			Documentation string `json:"documentation"`
			Homepage      string `json:"homepage"`
		} `json:"crate"`
		Version struct {
			Num      string `json:"num"`
			Checksum string `json:"checksum"`
		} `json:"version"`
	}
	if err := s.sdkImportJSON(ctx, input, target, &value); err != nil {
		return sdkImportMetadata{}, err
	}
	if value.Version.Num != input.ExactVersion || model.CanonicalSDKCoordinate("cargo", value.Crate.Name) != model.CanonicalSDKCoordinate("cargo", input.Coordinate) {
		return sdkImportMetadata{}, errors.New("the Cargo metadata does not match the requested coordinate and exact version")
	}
	digest := ""
	if regexp.MustCompile(`^[a-fA-F0-9]{64}$`).MatchString(value.Version.Checksum) {
		digest = "sha256:" + strings.ToLower(value.Version.Checksum)
	}
	return sdkImportMetadata{
		Name: value.Crate.Name, Description: value.Crate.Description, RegistryURL: input.SourceURL,
		SourceURL: value.Crate.Repository, DocumentationURL: firstSDKImportValue(value.Crate.Documentation, value.Crate.Homepage),
		Language: "Rust", Platform: "Rust", UpstreamDigest: digest,
	}, nil
}

func (s *Service) resolveGoImport(ctx context.Context, input SDKPackageImportInput, target *url.URL) (sdkImportMetadata, error) {
	var value struct {
		Version string `json:"Version"`
		Origin  struct {
			URL  string `json:"URL"`
			Hash string `json:"Hash"`
		} `json:"Origin"`
	}
	if err := s.sdkImportJSON(ctx, input, target, &value); err != nil {
		return sdkImportMetadata{}, err
	}
	if value.Version != input.ExactVersion {
		return sdkImportMetadata{}, errors.New("the Go module proxy did not resolve the requested exact version")
	}
	revision := strings.ToLower(strings.TrimSpace(value.Origin.Hash))
	if revision != "" && !sdkImportRevisionPattern.MatchString(revision) {
		return sdkImportMetadata{}, errors.New("the Go module proxy did not return an immutable source revision")
	}
	return sdkImportMetadata{
		Name: sdkImportFallbackName(input.Coordinate), Description: "Imported Go module " + input.Coordinate + ".",
		RegistryURL: input.SourceURL, SourceURL: value.Origin.URL, Language: "Go", Platform: "Go",
		ResolvedSourceRevision: revision,
	}, nil
}

func (s *Service) resolveSDKGitImport(ctx context.Context, input SDKPackageImportInput, source *url.URL) (sdkImportMetadata, error) {
	host := strings.ToLower(source.Hostname())
	parts := strings.Split(strings.Trim(strings.TrimSuffix(source.Path, ".git"), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return sdkImportMetadata{}, errors.New("Git imports require one repository URL in owner/repository form")
	}
	if host == "github.com" {
		base := &url.URL{Scheme: "https", Host: "api.github.com", Path: path.Join("/repos", parts[0], parts[1])}
		var repository struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			HTMLURL     string `json:"html_url"`
			Homepage    string `json:"homepage"`
			Language    string `json:"language"`
		}
		if err := s.sdkImportJSON(ctx, input, base, &repository); err != nil {
			return sdkImportMetadata{}, err
		}
		commitURL := *base
		commitURL.Path = base.Path + "/commits/" + input.SourceRef
		commitURL.RawPath = base.EscapedPath() + "/commits/" + url.PathEscape(input.SourceRef)
		var commit struct {
			SHA string `json:"sha"`
		}
		if err := s.sdkImportJSON(ctx, input, &commitURL, &commit); err != nil {
			return sdkImportMetadata{}, err
		}
		if !sdkImportRevisionPattern.MatchString(strings.ToLower(commit.SHA)) {
			return sdkImportMetadata{}, errors.New("the GitHub repository did not resolve the requested ref to an immutable commit")
		}
		return sdkImportMetadata{
			Name: repository.Name, Description: repository.Description, RegistryURL: input.SourceURL,
			SourceURL: firstSDKImportValue(repository.HTMLURL, input.SourceURL), DocumentationURL: repository.Homepage,
			Language: repository.Language, Platform: "GitHub", ResolvedSourceRevision: strings.ToLower(commit.SHA),
		}, nil
	}
	if host == "gitlab.com" {
		project := strings.Join(parts, "/")
		base := &url.URL{Scheme: "https", Host: "gitlab.com", Path: "/api/v4/projects/" + project, RawPath: "/api/v4/projects/" + url.PathEscape(project)}
		var repository struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			WebURL      string `json:"web_url"`
		}
		if err := s.sdkImportJSON(ctx, input, base, &repository); err != nil {
			return sdkImportMetadata{}, err
		}
		commitURL := *base
		commitURL.Path = base.Path + "/repository/commits/" + input.SourceRef
		commitURL.RawPath = base.EscapedPath() + "/repository/commits/" + url.PathEscape(input.SourceRef)
		var commit struct {
			ID string `json:"id"`
		}
		if err := s.sdkImportJSON(ctx, input, &commitURL, &commit); err != nil {
			return sdkImportMetadata{}, err
		}
		if !sdkImportRevisionPattern.MatchString(strings.ToLower(commit.ID)) {
			return sdkImportMetadata{}, errors.New("the GitLab repository did not resolve the requested ref to an immutable commit")
		}
		return sdkImportMetadata{
			Name: repository.Name, Description: repository.Description, RegistryURL: input.SourceURL,
			SourceURL: firstSDKImportValue(repository.WebURL, input.SourceURL), Language: sdkImportLanguage(input.Ecosystem),
			Platform: "GitLab", ResolvedSourceRevision: strings.ToLower(commit.ID),
		}, nil
	}
	return sdkImportMetadata{}, errors.New("Git imports currently support GitHub or GitLab HTTPS repository URLs")
}

func storedSDKImportURL(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "git+")
	if strings.HasPrefix(value, "git://github.com/") {
		value = "https://github.com/" + strings.TrimPrefix(value, "git://github.com/")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	return parsed.String()
}

func repositoryURL(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var direct string
	if json.Unmarshal(raw, &direct) == nil {
		return direct
	}
	var object struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(raw, &object)
	return object.URL
}

func npmIntegrityDigest(value string) string {
	algorithm, encoded, ok := strings.Cut(strings.TrimSpace(value), "-")
	if !ok || (algorithm != "sha256" && algorithm != "sha384" && algorithm != "sha512") {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) == 0 {
		return ""
	}
	return algorithm + ":" + hex.EncodeToString(decoded)
}

func boundedSDKImportText(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= maximum {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maximum]))
}

func firstSDKImportValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sdkImportFallbackName(coordinate string) string {
	value := strings.Trim(strings.TrimSpace(coordinate), "/")
	if index := strings.LastIndex(value, "/"); index >= 0 && index+1 < len(value) {
		value = value[index+1:]
	}
	return value
}

func sdkImportLanguage(ecosystem string) string {
	switch ecosystem {
	case "npm":
		return "JavaScript"
	case "pypi":
		return "Python"
	case "go":
		return "Go"
	case "cargo":
		return "Rust"
	default:
		return ""
	}
}

func goModuleProxyEscape(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if character >= 'A' && character <= 'Z' {
			builder.WriteByte('!')
			builder.WriteRune(character + ('a' - 'A'))
		} else {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}
