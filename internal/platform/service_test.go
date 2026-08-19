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

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type productBuilderDoer struct {
	authorization string
	requestBody   []byte
	requestURL    string
}

func (d *productBuilderDoer) Do(request *http.Request) (*http.Response, error) {
	d.authorization = request.Header.Get("Authorization")
	d.requestURL = request.URL.String()
	d.requestBody, _ = io.ReadAll(request.Body)
	response := `{"choices":[{"message":{"content":"{\"assignments\":[{\"input_index\":0,\"capability_slug\":\"voice\",\"capability_name\":\"Voice API\",\"api_version\":\"v3\",\"confidence\":0.94,\"evidence\":\"The artifact describes voice calling.\"}]}"}}]}`
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
