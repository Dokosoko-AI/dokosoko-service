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

type aiFailoverDoer struct {
	primaryStatus int
	requests      []string
}

func (d *aiFailoverDoer) Do(request *http.Request) (*http.Response, error) {
	d.requests = append(d.requests, request.URL.String())
	status := http.StatusOK
	body := `{"id":"resp_backup","model":"gpt-5.6-luna","status":"completed","output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"{\"description\":\"A grounded answer from the backup provider.\"}","annotations":[]}]}],"usage":{"input_tokens":12,"output_tokens":5,"total_tokens":17}}`
	if request.URL.Host == "primary.example.com" {
		status = d.primaryStatus
		body = `{"error":{"type":"provider_error"}}`
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}, Request: request}, nil
}

func configureAIPrimaryAndBackup(t *testing.T, primaryStatus int) (*store.Memory, *platform.Service, model.Product, platform.Actor, *aiFailoverDoer) {
	t.Helper()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x52}, 32))
	if err != nil {
		t.Fatal(err)
	}
	doer := &aiFailoverDoer{primaryStatus: primaryStatus}
	service := platform.NewWithVaultAndProductBuilderDoer(memory, vault, doer)
	product, err := memory.Product(ctx, "prod_acme")
	if err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{ID: "root_ai", RequestID: "req_ai_failover"}
	primary, err := service.SaveAIProviderConnection(ctx, platform.AIProviderConnectionInput{OrganisationID: product.OrganisationID, DeploymentID: product.ID, Provider: "openai-compatible", Endpoint: "https://primary.example.com", Credential: "primary-secret", Enabled: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveAIProviderConnection(ctx, platform.AIProviderConnectionInput{OrganisationID: product.OrganisationID, DeploymentID: product.ID, Provider: "openai", Credential: "backup-secret", Enabled: true, IsBackup: true, BackupModels: map[string]string{"analysis": "gpt-5.6-terra", "assistant": "gpt-5.6-luna"}}, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveAIWorkloadProfile(ctx, platform.AIWorkloadProfileInput{OrganisationID: product.OrganisationID, ProductID: product.ID, Workload: "assistant", ProviderConnectionID: primary.ID, Model: "primary-assistant", MaxInputTokens: 4096, MaxOutputTokens: 1024, DailyTokenBudget: 10000, Enabled: true}, actor); err != nil {
		t.Fatal(err)
	}
	return memory, service, product, actor, doer
}

func TestAIBackupRetriesOneTransientFailureAndAuditsBothAttempts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory, service, product, actor, doer := configureAIPrimaryAndBackup(t, http.StatusServiceUnavailable)

	draft, err := service.RewriteProductDescription(ctx, product.ID, "Explain the API without inventing capabilities.", actor)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(draft, "backup provider") || len(doer.requests) != 2 || !strings.Contains(doer.requests[0], "primary.example.com") || !strings.Contains(doer.requests[1], "api.openai.com/v1/responses") {
		t.Fatalf("fallback result=%q requests=%#v", draft, doer.requests)
	}
	events, err := memory.AIUsageEvents(ctx, product.ID, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var primary, backup *model.AIUsageEvent
	for index := range events {
		if events[index].ProviderRole == "backup" {
			backup = &events[index]
		} else {
			primary = &events[index]
		}
	}
	if len(events) != 2 || primary == nil || primary.Outcome != "failed" || primary.ErrorCode != "provider_unavailable" || backup == nil || backup.FallbackReason != "provider_unavailable" || backup.Outcome != "succeeded" {
		t.Fatalf("fallback usage events = %#v", events)
	}
}

func TestAIBackupNeverHidesInvalidCredentials(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory, service, product, actor, doer := configureAIPrimaryAndBackup(t, http.StatusUnauthorized)

	if _, err := service.RewriteProductDescription(ctx, product.ID, "Explain the API.", actor); err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("invalid credential error = %v", err)
	}
	if len(doer.requests) != 1 || !strings.Contains(doer.requests[0], "primary.example.com") {
		t.Fatalf("invalid credentials unexpectedly used backup: %#v", doer.requests)
	}
	events, err := memory.AIUsageEvents(ctx, product.ID, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ProviderRole != "primary" || events[0].ErrorCode != "invalid_credential" {
		t.Fatalf("invalid credential usage events = %#v", events)
	}
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

	shared, err := service.CreateResourceSet(ctx, platform.ResourceSetInput{Kind: "api", Name: "Voice API", Manifest: json.RawMessage(`[{"name":"calls.create","path":"/v1/calls"}]`)}, actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, integrationID := range []string{voiceV2.ID, voiceV1.ID} {
		if _, err := service.AttachResourceSet(ctx, integrationID, shared.ID, "", actor); err != nil {
			t.Fatal(err)
		}
	}

	shared, err = service.UpdateResourceSet(ctx, shared.ID, platform.ResourceSetInput{Kind: "api", Name: shared.Name, State: "active", Manifest: json.RawMessage(`[{"name":"calls.create","path":"/v2/calls"}]`), Revision: shared.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, integrationID := range []string{voiceV2.ID, voiceV1.ID} {
		integration, err := memory.Integration(ctx, "prod_acme", integrationID)
		if err != nil || len(integration.Resources) != 1 || integration.Resources[0].ResolvedRevision == nil || integration.Resources[0].ResolvedRevision.ID != shared.Latest.ID {
			t.Fatalf("shared resource did not advance for %s: integration=%#v err=%v", integrationID, integration, err)
		}
	}

	isolated, err := service.DuplicateResourceSet(ctx, shared.ID, "Voice v1 frozen API", actor)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DetachResourceSet(ctx, voiceV1.ID, shared.ID, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AttachResourceSet(ctx, voiceV1.ID, isolated.ID, isolated.Latest.ID, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateResourceSet(ctx, shared.ID, platform.ResourceSetInput{Kind: "api", Name: shared.Name, State: "active", Manifest: json.RawMessage(`[{"name":"calls.create","path":"/v3/calls"}]`), Revision: shared.Revision}, actor); err != nil {
		t.Fatal(err)
	}
	deprecated, err := memory.Integration(ctx, "prod_acme", voiceV1.ID)
	if err != nil || len(deprecated.Resources) != 1 || deprecated.Resources[0].ResourceSetID != isolated.ID || deprecated.Resources[0].ResolvedRevision.ID != isolated.Latest.ID {
		t.Fatalf("duplicated resource set did not isolate v1: integration=%#v err=%v", deprecated, err)
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
	supportProfile, err := memory.AIWorkloadProfile(ctx, product.ID, "assistant")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveAIWorkloadProfile(ctx, platform.AIWorkloadProfileInput{OrganisationID: product.OrganisationID, ProductID: product.ID, Workload: "assistant", ProviderConnectionID: supportProfile.ProviderConnectionID, Model: supportProfile.Model, MaxInputTokens: supportProfile.MaxInputTokens, MaxOutputTokens: supportProfile.MaxOutputTokens, DailyTokenBudget: 1, Enabled: true, Revision: supportProfile.Revision}, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RewriteProductDescription(ctx, product.ID, "A second bounded draft.", actor); err == nil || !strings.Contains(err.Error(), "daily token budget") {
		t.Fatalf("daily rewrite budget error = %v", err)
	}
}

func TestPublicManifestContainsOnlyAcknowledgedPublicIntegrations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root", RequestID: "visibility-test"}

	privateIntegration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "private-api", VersionKey: "v1", DisplayName: "Private API", Description: "Private customer API.", Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	configurePrivateIntegrationFoundations(t, service, memory, privateIntegration, actor)
	configurePrivateIntegrationPolicyTool(t, service, memory, privateIntegration, actor)
	if _, err := service.PublishIntegration(ctx, privateIntegration.ID, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "public-api", VersionKey: "v1", DisplayName: "Public API", Description: "Public API.", Visibility: model.VisibilityPublic, Lifecycle: "active"}, actor); !errors.Is(err, platform.ErrConfirmationRequired) {
		t.Fatalf("public integration created without confirmation: %v", err)
	}
	publicIntegration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "public-api", VersionKey: "v1", DisplayName: "Public API", Description: "Public API.", Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishIntegration(ctx, publicIntegration.ID, actor); err != nil {
		t.Fatal(err)
	}
	privateIntegration, err = service.UpdateIntegration(ctx, privateIntegration.ID, platform.IntegrationInput{FamilyKey: privateIntegration.FamilyKey, VersionKey: privateIntegration.VersionKey, DisplayName: privateIntegration.DisplayName, Description: privateIntegration.Description, Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: privateIntegration.Lifecycle, Revision: privateIntegration.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	publicIntegration, err = service.UpdateIntegration(ctx, publicIntegration.ID, platform.IntegrationInput{FamilyKey: publicIntegration.FamilyKey, VersionKey: publicIntegration.VersionKey, DisplayName: "Unpublished public name", Description: publicIntegration.Description, Visibility: publicIntegration.Visibility, Lifecycle: publicIntegration.Lifecycle, Revision: publicIntegration.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}

	publicManifest, err := service.ProductManifestFor(ctx, "prod_acme", model.CatalogScope{Public: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(publicManifest.Integrations) != 1 || publicManifest.Integrations[0].ID != publicIntegration.ID || publicManifest.Integrations[0].DisplayName != "Public API" {
		t.Fatalf("public manifest leaked private or versioned state: %#v", publicManifest)
	}
	privateManifest, err := service.ProductManifestFor(ctx, "prod_acme", model.CatalogScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(privateManifest.Integrations) != 2 {
		t.Fatalf("private manifest omitted integrations: %#v", privateManifest.Integrations)
	}
}
