package platform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type productBuilderDoer struct {
	authorization string
	requestBody   []byte
	requestURL    string
	response      string
}

func (d *productBuilderDoer) Do(request *http.Request) (*http.Response, error) {
	d.authorization = request.Header.Get("Authorization")
	d.requestURL = request.URL.String()
	d.requestBody, _ = io.ReadAll(request.Body)
	response := d.response
	if response == "" {
		response = `{"choices":[{"message":{"content":"{\"assignments\":[{\"input_index\":0,\"capability_slug\":\"voice\",\"capability_name\":\"Voice API\",\"api_version\":\"v3\",\"confidence\":0.94,\"evidence\":\"The artifact describes voice calling.\"}]}"}}]}`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(response)), Header: make(http.Header)}, nil
}

func TestPrivateDefaultsAndGuardedPublication(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)

	product, err := memory.Product(ctx, "prod_acme")
	if err != nil {
		t.Fatal(err)
	}
	if product.PublicMCPEnabled {
		t.Fatal("Public MCP must default to off")
	}

	source, err := memory.Source(ctx, product.ID, "src_docs")
	if err != nil {
		t.Fatal(err)
	}
	if source.Visibility != model.VisibilityPrivate {
		t.Fatalf("new source visibility = %q, want private", source.Visibility)
	}

	_, err = service.SetSourceVisibility(ctx, product.ID, source.ID, model.VisibilityPublic, false, source.Revision, platform.Actor{ID: "root"})
	if !errors.Is(err, platform.ErrConfirmationRequired) {
		t.Fatalf("unconfirmed publication error = %v", err)
	}

	source, err = service.SetSourceVisibility(ctx, product.ID, source.ID, model.VisibilityPublic, true, source.Revision, platform.Actor{ID: "root", RequestID: "req_1"})
	if err != nil {
		t.Fatal(err)
	}
	if source.Visibility != model.VisibilityPublic {
		t.Fatalf("visibility = %q", source.Visibility)
	}

	events, err := memory.AuditEvents(ctx, product.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != "source.visibility.changed" {
		t.Fatalf("audit events = %#v", events)
	}
}

func TestPackageCredentialIsEncryptedAndExcludedFromAPIRepresentation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x73}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	pkg, err := service.CreatePackage(ctx, platform.PackageInput{OrganisationID: "org_acme", ProductID: "prod_acme", Name: "@acme/private", Ecosystem: "npm", Version: "1.0.0", Mode: "proxy", UpstreamURL: "https://registry.example.com/acme.tgz", Credential: "upstream-secret-token"}, platform.Actor{ID: "root", RequestID: "req_package"})
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Visibility != model.VisibilityPrivate || pkg.Published {
		t.Fatalf("unsafe defaults: %#v", pkg)
	}
	encoded, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("upstream-secret-token")) || bytes.Contains(encoded, []byte("registry.example.com")) || bytes.Contains(encoded, []byte(pkg.CredentialID)) {
		t.Fatalf("internal delivery data leaked in JSON: %s", encoded)
	}
	stored, err := memory.Secret(ctx, "org_acme", pkg.CredentialID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored.Ciphertext, []byte("upstream-secret-token")) {
		t.Fatal("credential was stored in plaintext")
	}
}

func TestPackageDownloadRequiresTheVersionedContractEndpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x74}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	input := platform.PackageInput{
		OrganisationID:    "org_acme",
		ProductID:         "prod_acme",
		Name:              "@acme/private",
		Ecosystem:         "npm",
		Version:           "1.0.0",
		ExternalPackageID: "vendor-sdk-node-1.0.0",
		Mode:              "download",
		DownloadURL:       "https://packages.example.com/v1/package/download",
		Credential:        "download-service-token",
	}
	pkg, err := service.CreatePackage(ctx, input, platform.Actor{ID: "root", RequestID: "req_package_download"})
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Mode != "download" || pkg.DownloadURL != input.DownloadURL || pkg.ExternalPackageID != input.ExternalPackageID {
		t.Fatalf("package download configuration = %#v", pkg)
	}
	encoded, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("download-service-token")) || bytes.Contains(encoded, []byte(input.DownloadURL)) {
		t.Fatalf("download configuration leaked in JSON: %s", encoded)
	}

	invalid := []struct {
		name       string
		mode       string
		url        string
		externalID string
	}{
		{name: "legacy mode", mode: "fetch", url: input.DownloadURL, externalID: input.ExternalPackageID},
		{name: "legacy path", mode: "download", url: "https://packages.example.com/package-fetch", externalID: input.ExternalPackageID},
		{name: "encoded path", mode: "download", url: "https://packages.example.com/v1/package/%64ownload", externalID: input.ExternalPackageID},
		{name: "query string", mode: "download", url: input.DownloadURL + "?tenant=acme", externalID: input.ExternalPackageID},
		{name: "nonstandard port", mode: "download", url: "https://packages.example.com:8443/v1/package/download", externalID: input.ExternalPackageID},
		{name: "missing external package id", mode: "download", url: input.DownloadURL},
		{name: "external package id on proxy", mode: "proxy", url: "https://packages.example.com/artifact", externalID: input.ExternalPackageID},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			candidate := input
			candidate.Mode = test.mode
			candidate.DownloadURL = test.url
			candidate.ExternalPackageID = test.externalID
			if test.mode == "proxy" {
				candidate.UpstreamURL = test.url
			}
			if _, err := service.CreatePackage(ctx, candidate, platform.Actor{ID: "root"}); err == nil {
				t.Fatalf("CreatePackage accepted mode %q and URL %q", test.mode, test.url)
			}
		})
	}
}

func TestUsageHookCredentialIsEncryptedAndExcludedFromIdentityAPI(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x55}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	config, err := service.ConfigureIdentity(ctx, platform.IdentityInput{
		OrganisationID:      "org_acme",
		ProductID:           "prod_acme",
		Issuer:              "https://identity.vendor.example",
		ClientID:            "vendor-client",
		ClientSecret:        "oidc-client-secret",
		AllowedRedirectURIs: []string{"https://client.example/callback"},
		UsageHookURL:        "https://hooks.vendor.example/usage",
		UsageCredential:     "usage-service-secret",
	}, platform.Actor{ID: "root", RequestID: "req_usage_config"})
	if err != nil {
		t.Fatal(err)
	}
	if config.UsageHookURL == "" || config.UsageCredentialID == "" {
		t.Fatalf("usage configuration = %#v", config)
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("usage-service-secret")) || bytes.Contains(encoded, []byte(config.UsageCredentialID)) {
		t.Fatalf("usage credential leaked in identity JSON: %s", encoded)
	}
	stored, err := memory.Secret(ctx, "org_acme", config.UsageCredentialID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Purpose != "vendor_usage" || bytes.Contains(stored.Ciphertext, []byte("usage-service-secret")) {
		t.Fatalf("usage credential was not safely stored: %#v", stored)
	}
}

func TestPublicMCPRequiresConfirmationAndPrivateTransitionDoesNot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	product, _ := memory.Product(ctx, "prod_acme")

	_, err := service.SetPublicMCP(ctx, product.ID, true, false, product.Revision, platform.Actor{ID: "root"})
	if !errors.Is(err, platform.ErrConfirmationRequired) {
		t.Fatalf("unconfirmed enable error = %v", err)
	}

	product, err = service.SetPublicMCP(ctx, product.ID, true, true, product.Revision, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if !product.PublicMCPEnabled {
		t.Fatal("Public MCP was not enabled")
	}

	product, err = service.SetPublicMCP(ctx, product.ID, false, false, product.Revision, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if product.PublicMCPEnabled {
		t.Fatal("Public MCP was not disabled")
	}
}

func TestIntegrationsCanShareAndThenIsolateResourceSets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root", RequestID: "req-integration-sharing"}

	voiceV2, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "voice", VersionKey: "v2", DisplayName: "Voice API", Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	voiceV1, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "voice", VersionKey: "v1", DisplayName: "Voice API (deprecated)", Lifecycle: "deprecated", ReplacementIntegrationID: voiceV2.ID}, actor)
	if err != nil {
		t.Fatal(err)
	}

	shared, err := service.CreateResourceSet(ctx, platform.ResourceSetInput{Kind: "hook", Name: "Voice hooks", Manifest: json.RawMessage(`[{"name":"calls.create","path":"/v1/calls"}]`)}, actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, integrationID := range []string{voiceV2.ID, voiceV1.ID} {
		if _, err := service.AttachResourceSet(ctx, integrationID, shared.ID, "", actor); err != nil {
			t.Fatal(err)
		}
	}

	shared, err = service.UpdateResourceSet(ctx, shared.ID, platform.ResourceSetInput{Kind: "hook", Name: shared.Name, State: "active", Manifest: json.RawMessage(`[{"name":"calls.create","path":"/v2/calls"}]`), Revision: shared.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, integrationID := range []string{voiceV2.ID, voiceV1.ID} {
		integration, err := memory.Integration(ctx, "prod_acme", integrationID)
		if err != nil || len(integration.Resources) != 1 || integration.Resources[0].ResolvedRevision == nil || integration.Resources[0].ResolvedRevision.ID != shared.Latest.ID {
			t.Fatalf("shared resource did not advance for %s: integration=%#v err=%v", integrationID, integration, err)
		}
	}

	isolated, err := service.DuplicateResourceSet(ctx, shared.ID, "Voice v1 frozen hooks", actor)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DetachResourceSet(ctx, voiceV1.ID, shared.ID, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AttachResourceSet(ctx, voiceV1.ID, isolated.ID, isolated.Latest.ID, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateResourceSet(ctx, shared.ID, platform.ResourceSetInput{Kind: "hook", Name: shared.Name, State: "active", Manifest: json.RawMessage(`[{"name":"calls.create","path":"/v3/calls"}]`), Revision: shared.Revision}, actor); err != nil {
		t.Fatal(err)
	}
	deprecated, err := memory.Integration(ctx, "prod_acme", voiceV1.ID)
	if err != nil || len(deprecated.Resources) != 1 || deprecated.Resources[0].ResourceSetID != isolated.ID || deprecated.Resources[0].ResolvedRevision.ID != isolated.Latest.ID {
		t.Fatalf("duplicated resource set did not isolate v1: integration=%#v err=%v", deprecated, err)
	}
}

func TestCredentialBackedPackagePublicationIsStillGuarded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	pkg, _ := memory.Package(ctx, "prod_acme", "pkg_node")

	_, err := service.SetPackageVisibility(ctx, pkg.ProductID, pkg.ID, model.VisibilityPublic, false, pkg.Revision, platform.Actor{ID: "root"})
	if !errors.Is(err, platform.ErrConfirmationRequired) {
		t.Fatalf("unconfirmed proxy package publication error = %v", err)
	}
	updated, err := service.SetPackageVisibility(ctx, pkg.ProductID, pkg.ID, model.VisibilityPublic, true, pkg.Revision, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Visibility != model.VisibilityPublic {
		t.Fatalf("visibility = %q", updated.Visibility)
	}
}

func TestAIProductBuilderJoinsIndependentAPIVersionsAndPublishes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root_builder", RequestID: "req_builder"}
	product, err := service.CreateProduct(ctx, "org_acme", "Communications Platform", "communications", actor)
	if err != nil {
		t.Fatal(err)
	}

	build, err := service.BuildProductDefinition(ctx, product.ID, []model.ProductBuildInput{
		{Kind: "openapi", Name: "Voice API", Location: "https://api.example.com/voice/v3/openapi.yaml", Version: "v3"},
		{Kind: "docs", Name: "Voice documentation", Location: "https://docs.example.com/voice/v3", Version: "v3"},
		{Kind: "package", Name: "@acme/voice-node", Location: "npm:@acme/voice-node@7.2.1", Version: "7.2.1", Metadata: map[string]string{"api_version": "v3"}},
		{Kind: "mcp", Name: "Voice tools", Location: "https://mcp.example.com/v2", Version: "2026-07-28"},
		{Kind: "openapi", Name: "Messages API", Location: "https://api.example.com/messages/v2/openapi.yaml", Version: "v2"},
		{Kind: "package", Name: "@acme/messages", Location: "npm:@acme/messages@5.1.3", Version: "5.1.3", Metadata: map[string]string{"api_version": "v2"}},
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if build.State != "review" || build.AnalysisMode != "automatic" {
		t.Fatalf("unexpected build state: %#v", build)
	}
	if len(build.Proposal.Components) != 2 {
		t.Fatalf("components = %#v", build.Proposal.Components)
	}
	if len(build.Proposal.Profiles) != 1 || len(build.Proposal.Profiles[0].Selections) != 2 {
		t.Fatalf("profile = %#v", build.Proposal.Profiles)
	}
	for _, component := range build.Proposal.Components {
		if len(component.Releases) != 1 || component.Releases[0].Version == "unversioned" {
			t.Fatalf("component release was not resolved: %#v", component)
		}
		if len(component.Releases[0].Bindings) < 2 {
			t.Fatalf("release artifacts were not joined: %#v", component.Releases[0])
		}
		if component.Slug == "voice" && len(component.Releases[0].Bindings) != 4 {
			t.Fatalf("MCP protocol /v2 was not joined to Voice API v3: %#v", component.Releases[0])
		}
	}

	definition, err := service.PublishProductDefinition(ctx, product.ID, build.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if definition.State != "published" || definition.Revision != 1 || definition.PublishedAt == nil {
		t.Fatalf("definition = %#v", definition)
	}
	stored, err := memory.ProductDefinition(ctx, product.ID)
	if err != nil || stored.SourceBuildID != build.ID {
		t.Fatalf("stored definition = %#v, err = %v", stored, err)
	}
	publishedBuild, err := memory.ProductBuild(ctx, product.ID, build.ID)
	if err != nil || publishedBuild.State != "published" {
		t.Fatalf("published build = %#v, err = %v", publishedBuild, err)
	}
}

func TestAIProductBuilderFailsClosedWhenNoCapabilityCanBeFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root_builder"}
	product, err := service.CreateProduct(ctx, "org_acme", "Empty Product", "empty-product", actor)
	if err != nil {
		t.Fatal(err)
	}
	build, err := service.BuildProductDefinition(ctx, product.ID, []model.ProductBuildInput{{Kind: "docs", Name: "General guide", Location: "https://docs.example.com/start"}}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(build.Unresolved) != 2 || build.Unresolved[1].Code != "no_api_capabilities" {
		t.Fatalf("unresolved findings = %#v", build.Unresolved)
	}
	if _, err := service.PublishProductDefinition(ctx, product.ID, build.ID, actor); !errors.Is(err, platform.ErrProductDefinitionInvalid) {
		t.Fatalf("publish error = %v", err)
	}
}

func TestAIProductBuilderUsesConfiguredExtractionModelWithoutLeakingCredential(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x41}, 32))
	if err != nil {
		t.Fatal(err)
	}
	doer := &productBuilderDoer{}
	service := platform.NewWithVaultAndProductBuilderDoer(memory, vault, doer)
	actor := platform.Actor{ID: "root_builder", RequestID: "req_ai_builder"}
	product, err := service.CreateProduct(ctx, "org_acme", "Communications Platform", "communications-ai", actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveLLMProfile(ctx, platform.LLMProfileInput{
		OrganisationID:   product.OrganisationID,
		ProductID:        product.ID,
		Role:             "extraction",
		Provider:         "openai-compatible",
		Endpoint:         "https://llm.example.com",
		Model:            "extractor-1",
		Credential:       "provider-secret",
		MaxInputTokens:   4096,
		MaxOutputTokens:  1024,
		DailyTokenBudget: 10000,
		Enabled:          true,
	}, actor); err != nil {
		t.Fatal(err)
	}
	build, err := service.BuildProductDefinition(ctx, product.ID, []model.ProductBuildInput{{Kind: "openapi", Name: "Public schema", Location: "https://api.example.com/spec.yaml?token=prompt-secret#section"}}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if build.AnalysisMode != "ai_assisted" || len(build.Proposal.Components) != 1 || build.Proposal.Components[0].Slug != "voice" || build.Proposal.Components[0].Releases[0].Version != "v3" {
		t.Fatalf("AI-assisted product definition = %#v", build)
	}
	if doer.authorization != "Bearer provider-secret" || doer.requestURL != "https://llm.example.com/v1/chat/completions" {
		t.Fatalf("extraction request headers or destination were wrong: auth=%q url=%q", doer.authorization, doer.requestURL)
	}
	if bytes.Contains(doer.requestBody, []byte("provider-secret")) || bytes.Contains(doer.requestBody, []byte("prompt-secret")) {
		t.Fatal("a credential leaked into the extraction prompt")
	}
}

func TestAIProductBuilderRejectsCredentialsInInputURLs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	product, err := service.CreateProduct(ctx, "org_acme", "Unsafe Input Product", "unsafe-input-product", platform.Actor{ID: "root_builder"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.BuildProductDefinition(ctx, product.ID, []model.ProductBuildInput{{Kind: "docs", Name: "Private docs", Location: "https://token@example.com/docs"}}, platform.Actor{ID: "root_builder"})
	if err == nil || !strings.Contains(err.Error(), "cannot include credentials") {
		t.Fatalf("credential-bearing URL error = %v", err)
	}
}

func TestProductVersionDiscoveryPinsAndLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root_catalog", RequestID: "req_catalog"}
	product, err := service.CreateProduct(ctx, "org_acme", "Communications Platform", "communications-catalog", actor)
	if err != nil {
		t.Fatal(err)
	}
	build, err := service.BuildProductDefinition(ctx, product.ID, []model.ProductBuildInput{
		{Kind: "openapi", Name: "Voice API", Location: "https://api.example.com/voice/v3/openapi.yaml", Version: "v3"},
		{Kind: "package", Name: "@acme/voice", Location: "npm:@acme/voice@7.2.1", Version: "7.2.1", Metadata: map[string]string{"api_version": "v3"}},
		{Kind: "tool", Name: "voice.calls_create", Location: "tool:voice.calls_create", Version: "v3", Metadata: map[string]string{"api_version": "v3", "capability_slug": "voice", "capability_name": "Voice API"}},
		{Kind: "openapi", Name: "Messages API", Location: "https://api.example.com/messages/v2/openapi.yaml", Version: "v2"},
		{Kind: "package", Name: "@acme/messages", Location: "npm:@acme/messages@5.1.3", Version: "5.1.3", Metadata: map[string]string{"api_version": "v2"}},
		{Kind: "docs", Name: "Platform changelog", Location: "https://docs.example.com/changelog/2026.8", Version: "2026.8"},
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := service.PublishProductDefinition(ctx, product.ID, build.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateProductVersion(ctx, product.ID, platform.ProductVersionInput{Version: "2026.5", ProfileID: definition.Profiles[0].ID}, actor); !errors.Is(err, platform.ErrProductDescriptionRequired) {
		t.Fatalf("missing description error = %v", err)
	}
	product, err = service.UpdateProductSettings(ctx, product.ID, "Build voice and messaging integrations using independently versioned APIs and compatible SDKs.", "latest", product.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	ltsVersion, err := service.CreateProductVersion(ctx, product.ID, platform.ProductVersionInput{Version: "2026.5", ProfileID: definition.Profiles[0].ID, IsLTS: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	latestVersion, err := service.CreateProductVersion(ctx, product.ID, platform.ProductVersionInput{Version: "2026.8", ProfileID: definition.Profiles[0].ID, IsLatest: true, IsLTS: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if latestVersion.ManifestHash == "" || latestVersion.Diff.FromVersionID != ltsVersion.ID || latestVersion.Diff.Summary != "0 added, 0 removed, 0 changed" {
		t.Fatalf("generated release integrity or diff = %#v", latestVersion)
	}

	manifest, err := service.ProductManifest(ctx, product.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.EffectiveVersion == nil || manifest.EffectiveVersion.ID != latestVersion.ID || manifest.SelectionSource != "default_latest" || len(manifest.Capabilities) != 2 || len(manifest.Artifacts) != 1 || manifest.Artifacts[0].Name != "Platform changelog" {
		t.Fatalf("default manifest = %#v", manifest)
	}
	managed, allowed, err := service.ProductVersionAllowsTool(ctx, product.ID, "", model.Tool{ID: "tool_voice", Namespace: "voice", Name: "calls_create"})
	if err != nil || !managed || !allowed {
		t.Fatalf("version-matched tool managed=%v allowed=%v err=%v", managed, allowed, err)
	}
	managed, allowed, err = service.ProductVersionAllowsTool(ctx, product.ID, "", model.Tool{ID: "tool_future", Namespace: "voice", Name: "calls_delete"})
	if err != nil || !managed || allowed {
		t.Fatalf("tool outside snapshot managed=%v allowed=%v err=%v", managed, allowed, err)
	}
	if _, err := service.SaveProductVersionPin(ctx, product.ID, "contoso", ltsVersion.ID, "Production stability window", actor); err != nil {
		t.Fatal(err)
	}
	pinned, err := service.ProductManifest(ctx, product.ID, "contoso")
	if err != nil {
		t.Fatal(err)
	}
	if pinned.EffectiveVersion == nil || pinned.EffectiveVersion.ID != ltsVersion.ID || pinned.SelectionSource != "customer_pin" {
		t.Fatalf("pinned manifest = %#v", pinned)
	}
	environment, err := service.CreateEnvironment(ctx, product.OrganisationID, product.ID, "Customer production", "customer-production", true, actor)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := service.SaveProductInstallation(ctx, product.ID, platform.ProductInstallationInput{CustomerID: "contoso", EnvironmentID: environment.ID, ExternalID: "contoso-voice-prod", Name: "Contoso voice production", State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveScopedProductVersionPin(ctx, product.ID, platform.ProductVersionPinInput{Scope: "environment", ScopeID: environment.ID, ProductVersionID: latestVersion.ID, Reason: "Production rollout"}, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveScopedProductVersionPin(ctx, product.ID, platform.ProductVersionPinInput{Scope: "installation", ScopeID: installation.ID, ProductVersionID: ltsVersion.ID, Reason: "Voice certification"}, actor); err != nil {
		t.Fatal(err)
	}
	installationManifest, err := service.ProductManifestFor(ctx, product.ID, model.ProductSelectionContext{CustomerID: "contoso", InstallationID: "contoso-voice-prod"})
	if err != nil || installationManifest.EffectiveVersion == nil || installationManifest.EffectiveVersion.ID != ltsVersion.ID || installationManifest.SelectionSource != "installation_pin" || installationManifest.EnvironmentID != environment.ID {
		t.Fatalf("installation-scoped manifest = %#v err=%v", installationManifest, err)
	}
	secondInstallation, err := service.SaveProductInstallation(ctx, product.ID, platform.ProductInstallationInput{CustomerID: "contoso", EnvironmentID: environment.ID, ExternalID: "contoso-messages-prod", Name: "Contoso messages production", State: "active"}, actor)
	if err != nil || secondInstallation.ID == "" {
		t.Fatal(err)
	}
	environmentManifest, err := service.ProductManifestFor(ctx, product.ID, model.ProductSelectionContext{CustomerID: "contoso", InstallationID: "contoso-messages-prod"})
	if err != nil || environmentManifest.EffectiveVersion == nil || environmentManifest.EffectiveVersion.ID != latestVersion.ID || environmentManifest.SelectionSource != "environment_pin" {
		t.Fatalf("environment-scoped manifest = %#v err=%v", environmentManifest, err)
	}
	history, err := memory.ProductVersionPinHistory(ctx, product.ID)
	if err != nil || len(history) != 3 {
		t.Fatalf("pin history = %#v err=%v", history, err)
	}

	ltsVersion, err = memory.ProductVersion(ctx, product.ID, ltsVersion.ID)
	if err != nil {
		t.Fatal(err)
	}
	impact, err := service.ProductVersionImpact(ctx, product.ID, ltsVersion.ID)
	if err != nil || impact.CustomerPins != 1 || impact.InstallationPins != 1 {
		t.Fatalf("deprecation impact = %#v err=%v", impact, err)
	}
	ltsVersion, err = service.UpdateProductVersionLifecycle(ctx, product.ID, ltsVersion.ID, platform.ProductVersionLifecycleInput{Deprecated: true, DeprecationMessage: "Move to 2026.8 for continued support.", ReplacementVersion: "2026.8", AcknowledgeImpact: true, Revision: ltsVersion.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err = service.ProductManifest(ctx, product.ID, "contoso")
	if err != nil {
		t.Fatal(err)
	}
	if pinned.EffectiveVersion == nil || !pinned.EffectiveVersion.Deprecated || pinned.EffectiveVersion.ReplacementVersion != "2026.8" {
		t.Fatalf("deprecated pin moved or lost guidance: %#v", pinned)
	}
	if _, err := service.SaveProductVersionPin(ctx, product.ID, "fabrikam", ltsVersion.ID, "", actor); !errors.Is(err, platform.ErrProductVersionDeprecated) {
		t.Fatalf("new deprecated pin error = %v", err)
	}

	product, err = service.UpdateProductSettings(ctx, product.ID, product.Description, "lts", product.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err = service.ProductManifest(ctx, product.ID, "fabrikam")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.EffectiveVersion == nil || manifest.EffectiveVersion.ID != latestVersion.ID || manifest.SelectionSource != "default_lts" {
		t.Fatalf("LTS default manifest = %#v", manifest)
	}

	product, err = service.UpdateProductSettingsWithApproval(ctx, product.ID, product.Description, "latest", true, product.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	publisher := platform.Actor{ID: "root_publisher", RequestID: "req_publish_preview"}
	preview, err := service.CreateProductVersion(ctx, product.ID, platform.ProductVersionInput{Version: "2026.9", ProfileID: definition.Profiles[0].ID, IsLatest: true, ReleaseStage: "active", RolloutPercentage: 25}, publisher)
	if err != nil {
		t.Fatal(err)
	}
	if preview.PromotionState != "pending" || preview.ReleaseStage != "preview" || preview.IsLatest {
		t.Fatalf("approval-gated preview = %#v", preview)
	}
	unpinnedPreviewManifest, err := service.ProductManifest(ctx, product.ID, "preview-unpinned")
	if err != nil || unpinnedPreviewManifest.EffectiveVersion == nil || unpinnedPreviewManifest.EffectiveVersion.ID == preview.ID {
		t.Fatalf("pending preview was selected without an exact pin: %#v err=%v", unpinnedPreviewManifest, err)
	}
	for _, available := range unpinnedPreviewManifest.AvailableVersions {
		if available.ID == preview.ID {
			t.Fatalf("pending preview leaked into ordinary discovery: %#v", unpinnedPreviewManifest.AvailableVersions)
		}
	}
	if _, err := service.SaveScopedProductVersionPin(ctx, product.ID, platform.ProductVersionPinInput{Scope: "customer", ScopeID: "preview-customer", ProductVersionID: preview.ID, Reason: "Pre-production acceptance", Revision: 0}, actor); err != nil {
		t.Fatal(err)
	}
	pinnedPreviewManifest, err := service.ProductManifest(ctx, product.ID, "preview-customer")
	if err != nil || pinnedPreviewManifest.EffectiveVersion == nil || pinnedPreviewManifest.EffectiveVersion.ID != preview.ID || len(pinnedPreviewManifest.OperationalWarnings) == 0 {
		t.Fatalf("explicit preview pin = %#v err=%v", pinnedPreviewManifest, err)
	}
	if _, err := service.PromoteProductVersion(ctx, product.ID, preview.ID, platform.ProductVersionPromotionInput{Action: "approve", Revision: preview.Revision}, publisher); !errors.Is(err, platform.ErrPromotionSeparationOfDuties) {
		t.Fatalf("publisher approval error = %v", err)
	}
	preview, err = service.PromoteProductVersion(ctx, product.ID, preview.ID, platform.ProductVersionPromotionInput{Action: "approve", Note: "Reviewed generated diff and artifact health.", Revision: preview.Revision}, platform.Actor{ID: "root_approver", RequestID: "req_approve_preview"})
	if err != nil || preview.PromotionState != "approved" || preview.ReleaseStage != "active" || !preview.IsLatest {
		t.Fatalf("approved release = %#v err=%v", preview, err)
	}
	preview, err = service.UpdateProductVersionLifecycle(ctx, product.ID, preview.ID, platform.ProductVersionLifecycleInput{IsLatest: true, IsLTS: true, RolloutPercentage: preview.RolloutPercentage, Revision: preview.Revision}, actor)
	if err != nil || preview.PromotionState != "pending" || preview.IsLTS || !preview.RequestedLTS || preview.PromotionRequestedBy != actor.ID {
		t.Fatalf("approval-gated channel promotion = %#v err=%v", preview, err)
	}
	if _, err := service.PromoteProductVersion(ctx, product.ID, preview.ID, platform.ProductVersionPromotionInput{Action: "approve", Revision: preview.Revision}, actor); !errors.Is(err, platform.ErrPromotionSeparationOfDuties) {
		t.Fatalf("channel requester approval error = %v", err)
	}
	preview, err = service.PromoteProductVersion(ctx, product.ID, preview.ID, platform.ProductVersionPromotionInput{Action: "approve", Note: "LTS channel reviewed.", Revision: preview.Revision}, platform.Actor{ID: "root_second_approver", RequestID: "req_approve_lts"})
	if err != nil || !preview.IsLatest || !preview.IsLTS || preview.PromotionState != "approved" {
		t.Fatalf("approved channel promotion = %#v err=%v", preview, err)
	}
}

func TestProductDescriptionAIRewriteReturnsAnUnsavedDraft(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x31}, 32))
	if err != nil {
		t.Fatal(err)
	}
	doer := &productBuilderDoer{response: `{"choices":[{"message":{"content":"{\"description\":\"Build voice and messaging integrations with version-matched APIs, SDKs, documentation, and authorized tools.\"}"}}]}`}
	service := platform.NewWithVaultAndProductBuilderDoer(memory, vault, doer)
	actor := platform.Actor{ID: "root_description", RequestID: "req_description"}
	product, err := service.CreateProduct(ctx, "org_acme", "Communications Platform", "communications-description", actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveLLMProfile(ctx, platform.LLMProfileInput{OrganisationID: product.OrganisationID, ProductID: product.ID, Role: "assistant", Provider: "openai-compatible", Endpoint: "https://llm.example.com", Model: "description-1", Credential: "provider-secret", MaxInputTokens: 4096, MaxOutputTokens: 1024, DailyTokenBudget: 10000, Enabled: true}, actor); err != nil {
		t.Fatal(err)
	}
	rewritten, err := service.RewriteProductDescription(ctx, product.ID, "Voice API v3 and Messages API v2 for developers.", actor)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rewritten, "version-matched APIs") {
		t.Fatalf("rewrite = %q", rewritten)
	}
	stored, err := memory.Product(ctx, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Description != "" {
		t.Fatalf("AI rewrite silently saved product description: %q", stored.Description)
	}
	if doer.authorization != "Bearer provider-secret" || bytes.Contains(doer.requestBody, []byte("provider-secret")) || !bytes.Contains(doer.requestBody, []byte("never invent capabilities")) {
		t.Fatalf("rewrite request was not hardened: auth=%q body=%s", doer.authorization, doer.requestBody)
	}
	used, err := memory.LLMTokensUsed(ctx, product.ID, "assistant", time.Now().UTC().Add(-24*time.Hour))
	if err != nil || used <= 0 {
		t.Fatalf("rewrite token accounting = %d err=%v", used, err)
	}
	audits, err := memory.AuditEvents(ctx, product.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	encodedAudits, _ := json.Marshal(audits)
	if !bytes.Contains(encodedAudits, []byte("mcp-product-description-v1")) || bytes.Contains(encodedAudits, []byte("Voice API v3 and Messages API v2 for developers.")) {
		t.Fatalf("rewrite audit omitted prompt version or retained raw draft: %s", encodedAudits)
	}
	if err := memory.AppendAnalytics(ctx, model.AnalyticsEvent{OrganisationID: product.OrganisationID, ProductID: product.ID, EventName: "llm.tokens", Dimensions: map[string]any{"role": "assistant"}, Value: 10000, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RewriteProductDescription(ctx, product.ID, "A second bounded draft.", actor); err == nil || !strings.Contains(err.Error(), "daily token budget") {
		t.Fatalf("daily rewrite budget error = %v", err)
	}
}
