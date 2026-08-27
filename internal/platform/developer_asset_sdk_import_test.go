package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type sdkImportDoerFunc func(*http.Request) (*http.Response, error)

func (function sdkImportDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func sdkImportJSONResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func npmSDKImportMetadata(integrity string) string {
	return `{
		"name":"@acme/payments","description":"Typed payments SDK.","homepage":"https://docs.acme.example/sdk",
		"repository":{"type":"git","url":"git+https://github.com/acme/payments-sdk.git"},
		"versions":{"1.2.3":{"name":"@acme/payments","version":"1.2.3","types":"dist/index.d.ts","dist":{"integrity":"` + integrity + `"}}}
	}`
}

func TestImportSDKPackagePinsNPMReleaseAndIsIdempotent(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	service := New(memory)
	integrity := "sha512-" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("d", 64)))
	calls := 0
	service.SetSDKImportDoerForTesting(sdkImportDoerFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Host != "registry.npmjs.org" || request.URL.EscapedPath() != "/@acme%2Fpayments" {
			t.Fatalf("unexpected npm metadata URL: %s", request.URL)
		}
		if request.Header.Get("Authorization") != "" {
			t.Fatal("public import sent authorization")
		}
		return sdkImportJSONResponse(http.StatusOK, npmSDKImportMetadata(integrity)), nil
	}))

	input := SDKPackageImportInput{
		Ecosystem: "npm", SourceKind: "registry", SourceURL: "https://www.npmjs.com/package/@acme/payments",
		Coordinate: "@acme/payments", ExactVersion: "1.2.3", Visibility: model.VisibilityPrivate,
		Authentication: SDKImportAuthenticationInput{Type: "none"},
	}
	result, err := service.ImportSDKPackage(context.Background(), input, Actor{ID: "sdk-admin", RequestID: "import-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyImported || result.Package.DisplayCoordinate != "@acme/payments" || result.Package.Language != "TypeScript" {
		t.Fatalf("unexpected package import: %#v", result)
	}
	if result.Release.ExactVersion != "1.2.3" || result.Release.IdentityAssurance != "verified_digest" || !strings.HasPrefix(result.Release.UpstreamDigest, "sha512:") {
		t.Fatalf("release is not pinned to verified metadata: %#v", result.Release)
	}
	if result.Release.SourceURL != "https://github.com/acme/payments-sdk.git" {
		t.Fatalf("source URL was not normalized: %q", result.Release.SourceURL)
	}

	retried, err := service.ImportSDKPackage(context.Background(), input, Actor{ID: "sdk-admin", RequestID: "import-2"})
	if err != nil {
		t.Fatal(err)
	}
	if !retried.AlreadyImported || retried.Package.ID != result.Package.ID || retried.Release.ID != result.Release.ID || calls != 2 {
		t.Fatalf("retry was not idempotent: %#v calls=%d", retried, calls)
	}
	packages, err := memory.SDKPackages(context.Background(), result.Package.DeploymentID)
	if err != nil || len(packages) != 1 {
		t.Fatalf("package roots after retry = %#v, %v", packages, err)
	}
}

func TestImportSDKPackageDoesNotClaimAnUnverifiedExistingReleaseMatches(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	service := New(memory)
	actor := Actor{ID: "sdk-admin"}
	packageValue, err := service.SaveSDKPackage(context.Background(), "", SDKPackageInput{
		Ecosystem: "npm", Coordinate: "@acme/payments", Name: "Payments SDK",
		Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.CreateSDKRelease(context.Background(), packageValue.ID, SDKReleaseInput{
		ExactVersion: "1.2.3", IdentityAssurance: "metadata_only",
		Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	integrity := "sha512-" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("v", 64)))
	service.SetSDKImportDoerForTesting(sdkImportDoerFunc(func(*http.Request) (*http.Response, error) {
		return sdkImportJSONResponse(http.StatusOK, npmSDKImportMetadata(integrity)), nil
	}))
	_, err = service.ImportSDKPackage(context.Background(), SDKPackageImportInput{
		Ecosystem: "npm", SourceKind: "registry", SourceURL: "https://registry.npmjs.org/@acme/payments",
		Coordinate: "@acme/payments", ExactVersion: "1.2.3", Visibility: model.VisibilityPrivate,
		Authentication: SDKImportAuthenticationInput{Type: "none"},
	}, actor)
	if !errors.Is(err, ErrSDKImportConflict) {
		t.Fatalf("verified metadata silently matched an unverified immutable release: %v", err)
	}
	releases, listErr := memory.SDKReleases(context.Background(), packageValue.DeploymentID, packageValue.ID)
	if listErr != nil || len(releases) != 1 || releases[0].UpstreamDigest != "" {
		t.Fatalf("conflicting immutable release was mutated: %#v, %v", releases, listErr)
	}
}

func TestImportSDKPackageResolvesEveryRegistryEcosystem(t *testing.T) {
	t.Parallel()
	sha256 := strings.Repeat("a", 64)
	tests := []struct {
		name             string
		input            SDKPackageImportInput
		expectedPath     string
		response         string
		expectedLanguage string
		expectedPlatform string
		expectedDigest   string
		expectedRevision string
	}{
		{
			name: "PyPI",
			input: SDKPackageImportInput{
				Ecosystem: "pypi", SourceKind: "registry", SourceURL: "https://pypi.org/project/acme-sdk",
				Coordinate: "acme-sdk", ExactVersion: "2.3.4", Visibility: model.VisibilityPrivate,
				Authentication: SDKImportAuthenticationInput{Type: "none"},
			},
			expectedPath:     "/pypi/acme-sdk/2.3.4/json",
			response:         `{"info":{"name":"acme-sdk","version":"2.3.4","summary":"Python SDK.","home_page":"https://acme.example/sdk","project_urls":{"Source":"https://github.com/acme/python-sdk","Documentation":"https://docs.acme.example/python"}},"urls":[{"url":"https://files.pythonhosted.org/acme-sdk.whl","digests":{"sha256":"` + sha256 + `"}}]}`,
			expectedLanguage: "Python", expectedPlatform: "Python", expectedDigest: "sha256:" + sha256,
		},
		{
			name: "Cargo",
			input: SDKPackageImportInput{
				Ecosystem: "cargo", SourceKind: "registry", SourceURL: "https://crates.io/crates/acme-sdk",
				Coordinate: "acme-sdk", ExactVersion: "3.4.5", Visibility: model.VisibilityPrivate,
				Authentication: SDKImportAuthenticationInput{Type: "none"},
			},
			expectedPath:     "/api/v1/crates/acme-sdk/3.4.5",
			response:         `{"crate":{"name":"acme-sdk","description":"Rust SDK.","repository":"https://github.com/acme/rust-sdk","documentation":"https://docs.rs/acme-sdk"},"version":{"num":"3.4.5","checksum":"` + sha256 + `"}}`,
			expectedLanguage: "Rust", expectedPlatform: "Rust", expectedDigest: "sha256:" + sha256,
		},
		{
			name: "Go module",
			input: SDKPackageImportInput{
				Ecosystem: "go", SourceKind: "registry", SourceURL: "https://proxy.golang.org",
				Coordinate: "github.com/acme/sdk", ExactVersion: "v1.2.3", Visibility: model.VisibilityPrivate,
				Authentication: SDKImportAuthenticationInput{Type: "none"},
			},
			expectedPath:     "/github.com/acme/sdk/@v/v1.2.3.info",
			response:         `{"Version":"v1.2.3","Origin":{"URL":"https://github.com/acme/sdk","Hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}`,
			expectedLanguage: "Go", expectedPlatform: "Go", expectedRevision: strings.Repeat("b", 40),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service := New(store.NewMemory())
			service.SetSDKImportDoerForTesting(sdkImportDoerFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.EscapedPath() != test.expectedPath {
					t.Fatalf("metadata path = %q, want %q", request.URL.EscapedPath(), test.expectedPath)
				}
				return sdkImportJSONResponse(http.StatusOK, test.response), nil
			}))
			result, err := service.ImportSDKPackage(context.Background(), test.input, Actor{ID: "sdk-admin"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Package.Language != test.expectedLanguage || result.Package.Platform != test.expectedPlatform {
				t.Fatalf("package runtime metadata = %q/%q", result.Package.Language, result.Package.Platform)
			}
			if result.Release.UpstreamDigest != test.expectedDigest || result.Release.ResolvedSourceRevision != test.expectedRevision {
				t.Fatalf("release identity metadata = %#v", result.Release)
			}
		})
	}
}

func TestImportSDKPackagePinsPrivateGitLabReleaseWithBasicAuthentication(t *testing.T) {
	t.Parallel()
	service := New(store.NewMemory())
	service.SetSDKImportDoerForTesting(sdkImportDoerFunc(func(request *http.Request) (*http.Response, error) {
		username, password, ok := request.BasicAuth()
		if !ok || username != "oauth2" || password != "private-token" {
			t.Fatalf("basic authorization was not applied")
		}
		switch request.URL.EscapedPath() {
		case "/api/v4/projects/acme%2Fsdk":
			return sdkImportJSONResponse(http.StatusOK, `{"name":"sdk","description":"Private SDK.","web_url":"https://gitlab.com/acme/sdk"}`), nil
		case "/api/v4/projects/acme%2Fsdk/repository/commits/v1.0.0":
			return sdkImportJSONResponse(http.StatusOK, `{"id":"cccccccccccccccccccccccccccccccccccccccc"}`), nil
		default:
			t.Fatalf("unexpected GitLab API path: %s", request.URL.EscapedPath())
			return nil, nil
		}
	}))
	result, err := service.ImportSDKPackage(context.Background(), SDKPackageImportInput{
		Ecosystem: "pypi", SourceKind: "git", SourceURL: "https://gitlab.com/acme/sdk",
		Coordinate: "acme-sdk", ExactVersion: "1.0.0", SourceRef: "v1.0.0",
		Visibility:     model.VisibilityPrivate,
		Authentication: SDKImportAuthenticationInput{Type: "basic", Username: "oauth2", Credential: "private-token"},
	}, Actor{ID: "sdk-admin"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Release.ResolvedSourceRevision != strings.Repeat("c", 40) || result.Package.Platform != "GitLab" {
		t.Fatalf("GitLab release was not pinned: %#v", result)
	}
}

func TestImportSDKPackageUsesWriteOnlyAuthorizationWithoutEchoingIt(t *testing.T) {
	t.Parallel()
	const token = "github_pat_TEST_DO_NOT_ECHO_1234567890"
	service := New(store.NewMemory())
	service.SetSDKImportDoerForTesting(sdkImportDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("authorization header = %q", request.Header.Get("Authorization"))
		}
		if strings.HasSuffix(request.URL.Path, "/commits/main") {
			return sdkImportJSONResponse(http.StatusOK, `{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`), nil
		}
		return sdkImportJSONResponse(http.StatusOK, `{"name":"private-sdk","description":"Private SDK.","html_url":"https://github.com/acme/private-sdk","language":"Go"}`), nil
	}))
	result, err := service.ImportSDKPackage(context.Background(), SDKPackageImportInput{
		Ecosystem: "go", SourceKind: "git", SourceURL: "https://github.com/acme/private-sdk",
		Coordinate: "github.com/acme/private-sdk", ExactVersion: "v1.2.3", SourceRef: "main",
		Visibility: model.VisibilityPrivate, Authentication: SDKImportAuthenticationInput{Type: "bearer", Credential: token},
	}, Actor{ID: "sdk-admin"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), token) || result.Release.ResolvedSourceRevision != strings.Repeat("a", 40) || result.Release.IdentityAssurance != "resolved_source" {
		t.Fatalf("private Git import leaked authorization or lost its pinned commit: %s", encoded)
	}
}

func TestImportSDKPackageFailsClosedWithoutLeakingCredential(t *testing.T) {
	t.Parallel()
	const token = "npm_PRIVATE_TOKEN_DO_NOT_ECHO_123456789"
	service := New(store.NewMemory())
	service.SetSDKImportDoerForTesting(sdkImportDoerFunc(func(*http.Request) (*http.Response, error) {
		return sdkImportJSONResponse(http.StatusUnauthorized, token), nil
	}))
	_, err := service.ImportSDKPackage(context.Background(), SDKPackageImportInput{
		Ecosystem: "npm", SourceKind: "registry", SourceURL: "https://registry.example.com/@acme/private",
		Coordinate: "@acme/private", ExactVersion: "1.0.0", Visibility: model.VisibilityPrivate,
		Authentication: SDKImportAuthenticationInput{Type: "bearer", Credential: token},
	}, Actor{ID: "sdk-admin"})
	if err == nil || strings.Contains(err.Error(), token) || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("unsafe authentication error: %v", err)
	}

	for _, sourceURL := range []string{
		"https://token@example.com/package",
		"https://example.com/package?token=secret",
		"https://10.0.0.1/package",
		"http://example.com/package",
		"https://example.com:8443/package",
	} {
		_, _, validationErr := normalizeSDKPackageImportInput(SDKPackageImportInput{
			Ecosystem: "npm", SourceKind: "registry", SourceURL: sourceURL,
			Coordinate: "@acme/private", ExactVersion: "1.0.0", Visibility: model.VisibilityPrivate,
			Authentication: SDKImportAuthenticationInput{Type: "none"},
		})
		if validationErr == nil {
			t.Errorf("unsafe source URL was accepted: %s", sourceURL)
		}
	}
	_, _, validationErr := normalizeSDKPackageImportInput(SDKPackageImportInput{
		Ecosystem: "npm", SourceKind: "registry", SourceURL: "https://registry.example.com/package",
		Coordinate: "@acme/private", ExactVersion: "latest", Visibility: model.VisibilityPrivate,
		Authentication: SDKImportAuthenticationInput{Type: "none"},
	})
	if validationErr == nil {
		t.Fatal("floating npm latest tag was accepted")
	}
}

func TestImportSDKPackageHidesTransportFailures(t *testing.T) {
	t.Parallel()
	service := New(store.NewMemory())
	service.SetSDKImportDoerForTesting(sdkImportDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial https://user:secret@internal.invalid")
	}))
	_, err := service.ImportSDKPackage(context.Background(), SDKPackageImportInput{
		Ecosystem: "cargo", SourceKind: "registry", SourceURL: "https://crates.example.com",
		Coordinate: "acme-sdk", ExactVersion: "1.0.0", Visibility: model.VisibilityPrivate,
		Authentication: SDKImportAuthenticationInput{Type: "none"},
	}, Actor{ID: "sdk-admin"})
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "internal.invalid") {
		t.Fatalf("transport error was not sanitized: %v", err)
	}
}

func TestImportSDKPackageRejectsMutableGoOriginMetadata(t *testing.T) {
	t.Parallel()
	service := New(store.NewMemory())
	service.SetSDKImportDoerForTesting(sdkImportDoerFunc(func(*http.Request) (*http.Response, error) {
		return sdkImportJSONResponse(http.StatusOK, `{"Version":"v1.2.3","Origin":{"URL":"https://github.com/acme/sdk","Hash":"main"}}`), nil
	}))
	_, err := service.ImportSDKPackage(context.Background(), SDKPackageImportInput{
		Ecosystem: "go", SourceKind: "registry", SourceURL: "https://proxy.golang.org",
		Coordinate: "github.com/acme/sdk", ExactVersion: "v1.2.3", Visibility: model.VisibilityPrivate,
		Authentication: SDKImportAuthenticationInput{Type: "none"},
	}, Actor{ID: "sdk-admin"})
	if err == nil || !strings.Contains(err.Error(), "immutable source revision") {
		t.Fatalf("mutable Go origin was accepted: %v", err)
	}
}
