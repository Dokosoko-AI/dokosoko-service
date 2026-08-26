package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func newDeveloperAssetServer() (*store.Memory, *platform.Service, http.Handler) {
	memory := store.NewMemory()
	service := platform.New(memory)
	return memory, service, httpapi.New(service, "https://dokosoko.example")
}

func createDeveloperAssetAPI(t *testing.T, service *platform.Service, family, version string) model.Integration {
	t.Helper()
	value, err := service.CreateIntegration(context.Background(), platform.IntegrationInput{
		FamilyKey: family, VersionKey: version, DisplayName: family + " " + version,
		Description: "API used to verify deployment-owned developer assets.",
	}, platform.Actor{ID: "asset-admin", RequestID: "request-" + family + "-" + version})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestDeveloperAssetCatalogSharesOneExactSDKReleaseAcrossAPIs(t *testing.T) {
	t.Parallel()
	_, service, handler := newDeveloperAssetServer()
	apiV1 := createDeveloperAssetAPI(t, service, "payments", "v1")
	apiV2 := createDeveloperAssetAPI(t, service, "payments", "v2")

	createdPackage := request(t, handler, http.MethodPost, "/api/v1/developer-assets/sdk-packages", "doko_admin_demo", `{
		"ecosystem":"npm","coordinate":"@acme/payments","name":"Acme Payments SDK",
		"description":"One deployment-owned package reused by versioned APIs.","visibility":"private","lifecycle":"active"
	}`)
	if createdPackage.Code != http.StatusCreated {
		t.Fatalf("create package = %d: %s", createdPackage.Code, createdPackage.Body.String())
	}
	var sdkPackage model.SDKPackage
	if err := json.Unmarshal(createdPackage.Body.Bytes(), &sdkPackage); err != nil {
		t.Fatal(err)
	}

	createdRelease := request(t, handler, http.MethodPost, "/api/v1/developer-assets/sdk-packages/"+sdkPackage.ID+"/releases", "doko_admin_demo", `{
		"exact_version":"1.4.0","source_url":"https://github.com/acme/payments-sdk","visibility":"private","lifecycle":"active"
	}`)
	if createdRelease.Code != http.StatusCreated {
		t.Fatalf("create exact release = %d: %s", createdRelease.Code, createdRelease.Body.String())
	}
	var release model.SDKRelease
	if err := json.Unmarshal(createdRelease.Body.Bytes(), &release); err != nil {
		t.Fatal(err)
	}
	if release.ExactVersion != "1.4.0" || !strings.Contains(release.InstallCommand, "@1.4.0") {
		t.Fatalf("release is not exact: %#v", release)
	}
	ingestionBody := `{"files":[{"source_path":"README.md","content":"# Payments SDK\n\nUse the exact 1.4.0 release.\n","role":"readme"}]}`
	ingested := request(t, handler, http.MethodPost, "/api/v1/developer-assets/sdk-releases/"+release.ID+"/ingestions", "doko_admin_demo", ingestionBody)
	if ingested.Code != http.StatusCreated || !strings.Contains(ingested.Body.String(), `"state":"review_ready"`) {
		t.Fatalf("ingest exact release content = %d: %s", ingested.Code, ingested.Body.String())
	}
	retried := request(t, handler, http.MethodPost, "/api/v1/developer-assets/sdk-releases/"+release.ID+"/ingestions", "doko_admin_demo", ingestionBody)
	if retried.Code != http.StatusOK || !strings.Contains(retried.Body.String(), `"already_ingested":true`) {
		t.Fatalf("idempotent exact release ingestion = %d: %s", retried.Code, retried.Body.String())
	}

	for _, api := range []model.Integration{apiV1, apiV2} {
		body, err := json.Marshal(map[string]any{
			"sdk_package_id": sdkPackage.ID, "sdk_release_id": release.ID,
			"state": "draft", "selector": map[string]any{}, "visibility": "private",
		})
		if err != nil {
			t.Fatal(err)
		}
		attached := request(t, handler, http.MethodPost, "/api/v1/integrations/"+api.ID+"/resources/sdks", "doko_admin_demo", string(body))
		if attached.Code != http.StatusCreated {
			t.Fatalf("attach release to %s = %d: %s", api.ID, attached.Code, attached.Body.String())
		}
		resources := request(t, handler, http.MethodGet, "/api/v1/integrations/"+api.ID+"/resources", "doko_admin_demo", "")
		if resources.Code != http.StatusOK || !strings.Contains(resources.Body.String(), release.ID) {
			t.Fatalf("resources for %s = %d: %s", api.ID, resources.Code, resources.Body.String())
		}
	}

	catalog := request(t, handler, http.MethodGet, "/api/v1/developer-assets", "doko_admin_demo", "")
	if catalog.Code != http.StatusOK || !strings.Contains(catalog.Body.String(), sdkPackage.ID) {
		t.Fatalf("catalog = %d: %s", catalog.Code, catalog.Body.String())
	}
	legacy := request(t, handler, http.MethodGet, "/api/v1/integrations/"+apiV1.ID+"/sdks", "doko_admin_demo", "")
	if legacy.Code != http.StatusOK || !strings.Contains(legacy.Body.String(), `"exact_version":"1.4.0"`) {
		t.Fatalf("legacy SDK projection = %d: %s", legacy.Code, legacy.Body.String())
	}
}

func TestDeveloperAssetDocumentationExplorerSearchesNormalizedContent(t *testing.T) {
	t.Parallel()
	memory, _, handler := newDeveloperAssetServer()
	ctx := context.Background()
	now := time.Now().UTC()
	run, err := memory.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: "run_docs_http", DeploymentID: "prod_acme", OrganisationID: "org_acme",
		AssetKind: model.DeveloperAssetDocumentation, TargetID: "src_docs", TargetKey: "source:src_docs",
		SourceID: "src_docs", State: model.DeveloperAssetIngestionReviewReady, Attempt: 1,
		Versions:    model.ProcessorVersions{Pipeline: "pipeline-v1", Parser: "parser-v1", Normalizer: "normalizer-v1", Mapper: "mapper-v1"},
		RawManifest: json.RawMessage(`{}`), Diagnostics: json.RawMessage(`{}`),
		DiscoveredCount: 2, AcquiredCount: 2, StartedAt: &now, FinishedAt: &now, QueuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	documents := []model.DocumentationDocument{
		{ID: "doc_auth_http", DeploymentID: "prod_acme", IngestionRunID: run.ID, SourcePath: "guides/auth.md", Title: "Authentication", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "Use scoped OAuth tokens for authentication.", ContentHash: "sha256:" + strings.Repeat("1", 64), Visibility: model.VisibilityPrivate, Metadata: json.RawMessage(`{}`)},
		{ID: "doc_webhooks_http", DeploymentID: "prod_acme", IngestionRunID: run.ID, SourcePath: "guides/webhooks.md", Title: "Webhooks", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "Verify webhook signatures.", ContentHash: "sha256:" + strings.Repeat("2", 64), Visibility: model.VisibilityPrivate, Ordinal: 1, Metadata: json.RawMessage(`{}`)},
	}
	documentationMap := &model.DocumentationMap{
		ID: "map_docs_http", DeploymentID: "prod_acme", IngestionRunID: run.ID, MapVersion: "documentation-map-v1",
		Map: model.DocumentationMapBody{
			Overview: "Two reviewed guides.", Documents: []model.KnowledgeMapEntry{}, Topics: []model.KnowledgeMapEntry{}, Workflows: []model.KnowledgeMapEntry{},
			Authentication: []model.KnowledgeMapEntry{{ID: "doc_auth_http", Kind: "authentication", Title: "Authentication", Summary: "Scoped OAuth."}},
		},
		AgentMarkdown: "# Documentation\n", ContentHash: "sha256:" + strings.Repeat("3", 64), Visibility: model.VisibilityPrivate, CreatedAt: now,
	}
	if err := memory.SaveDocumentationIngestionOutput(ctx, "prod_acme", store.DocumentationIngestionOutput{Documents: documents, Map: documentationMap}); err != nil {
		t.Fatal(err)
	}
	includedOrdinal := 0
	reviewedAt := now.Add(time.Minute)
	if err := memory.SaveSourcePublicationDocumentationReview(ctx, "prod_acme", store.SourcePublicationDocumentationReview{
		Selections: []model.SourcePublicationDocumentSelection{
			{SourcePublicationID: "pub_docs_seed", DeploymentID: "prod_acme", DocumentationDocumentID: "doc_auth_http", Decision: "included", Ordinal: &includedOrdinal, ContentHash: documents[0].ContentHash, ReviewedBy: "documentation-admin", ReviewedAt: reviewedAt, CreatedAt: reviewedAt},
			{SourcePublicationID: "pub_docs_seed", DeploymentID: "prod_acme", DocumentationDocumentID: "doc_webhooks_http", Decision: "excluded", Reason: "Webhook guidance is awaiting a security review.", ContentHash: documents[1].ContentHash, ReviewedBy: "documentation-admin", ReviewedAt: reviewedAt, CreatedAt: reviewedAt},
		},
		MapLink: &model.SourcePublicationDocumentationMap{SourcePublicationID: "pub_docs_seed", DeploymentID: "prod_acme", DocumentationMapID: documentationMap.ID, ContentHash: documentationMap.ContentHash, CreatedAt: reviewedAt},
	}); err != nil {
		t.Fatal(err)
	}

	search := request(t, handler, http.MethodGet, "/api/v1/developer-assets/documentation/documents?query="+url.QueryEscape("scoped OAuth")+"&limit=10", "doko_admin_demo", "")
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), "doc_auth_http") || strings.Contains(search.Body.String(), "doc_webhooks_http") || !strings.Contains(search.Body.String(), `"total":1`) || !strings.Contains(search.Body.String(), `"has_more":false`) || !strings.Contains(search.Body.String(), `"documentation_map":{"id":"map_docs_http"`) || !strings.Contains(search.Body.String(), `"source_publication_selections":[{"source_publication_id":"pub_docs_seed"`) || !strings.Contains(search.Body.String(), `"decision":"included"`) {
		t.Fatalf("documentation search = %d: %s", search.Code, search.Body.String())
	}
	page := request(t, handler, http.MethodGet, "/api/v1/developer-assets/documentation/documents?limit=1&offset=1", "doko_admin_demo", "")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `"total":2`) || !strings.Contains(page.Body.String(), `"has_more":false`) || !strings.Contains(page.Body.String(), `"document":{"id":"doc_webhooks_http"`) || strings.Count(page.Body.String(), `"document":{"id":`) != 1 || !strings.Contains(page.Body.String(), `"decision":"excluded"`) || !strings.Contains(page.Body.String(), `"reason":"Webhook guidance is awaiting a security review."`) || strings.Contains(page.Body.String(), `"Document"`) || strings.Contains(page.Body.String(), `"DocumentationMap"`) {
		t.Fatalf("second documentation page = %d: %s", page.Code, page.Body.String())
	}
	invalidOffset := request(t, handler, http.MethodGet, "/api/v1/developer-assets/documentation/documents?offset=-1", "doko_admin_demo", "")
	if invalidOffset.Code != http.StatusBadRequest || !strings.Contains(invalidOffset.Body.String(), "offset must be zero or greater") {
		t.Fatalf("invalid documentation offset = %d: %s", invalidOffset.Code, invalidOffset.Body.String())
	}
	detail := request(t, handler, http.MethodGet, "/api/v1/developer-assets/documentation/documents/doc_auth_http", "doko_admin_demo", "")
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"normalized_markdown"`) || !strings.Contains(detail.Body.String(), `"documentation_map":{"id":"map_docs_http"`) || !strings.Contains(detail.Body.String(), `"source_publication_selections"`) || !strings.Contains(detail.Body.String(), `"reviewed_by":"documentation-admin"`) || strings.Contains(detail.Body.String(), `"Document"`) || strings.Contains(detail.Body.String(), `"Run"`) {
		t.Fatalf("documentation detail = %d: %s", detail.Code, detail.Body.String())
	}
	ingestion := request(t, handler, http.MethodGet, "/api/v1/developer-assets/ingestion-runs/"+run.ID, "doko_admin_demo", "")
	if ingestion.Code != http.StatusOK || !strings.Contains(ingestion.Body.String(), `"state":"published"`) {
		t.Fatalf("ingestion detail = %d: %s", ingestion.Code, ingestion.Body.String())
	}
}

func TestDeveloperAssetQueryLabRejectsCrossAPIPublicationAndMalformedReview(t *testing.T) {
	t.Parallel()
	memory, service, handler := newDeveloperAssetServer()
	ctx := context.Background()
	apiA := createDeveloperAssetAPI(t, service, "accounts", "v1")
	apiB := createDeveloperAssetAPI(t, service, "billing", "v1")
	now := time.Now().UTC()
	revision, err := memory.CreateIntegrationRevision(ctx, model.IntegrationRevision{
		ID: "revision_b_http", IntegrationID: apiB.ID, Revision: 1, State: "published",
		Snapshot: json.RawMessage(`{}`), ManifestHash: "sha256:" + strings.Repeat("3", 64),
		PublishedBy: "asset-admin", PublishedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	publication, err := memory.CreateAPIDeveloperAssetPublication(ctx, model.APIDeveloperAssetPublication{
		ID: "publication_b_http", DeploymentID: "prod_acme", APIID: apiB.ID, APIRevisionID: revision.ID,
		SnapshotSchemaVersion: "developer-assets-v1", SnapshotHash: "sha256:" + strings.Repeat("4", 64),
		Documentation: []model.APIPublicationDocumentationAsset{}, Contracts: []model.APIPublicationContractAsset{},
		SDKs: []model.APIPublicationSDKAsset{}, PublishedBy: "asset-admin", PublishedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	listed := request(t, handler, http.MethodGet, "/api/v1/integrations/"+apiB.ID+"/resources/publications", "doko_admin_demo", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), publication.ID) || !strings.Contains(listed.Body.String(), publication.SnapshotHash) {
		t.Fatalf("API developer-asset publication history = %d: %s", listed.Code, listed.Body.String())
	}
	detail := request(t, handler, http.MethodGet, "/api/v1/integrations/"+apiB.ID+"/resources/publications/"+publication.ID, "doko_admin_demo", "")
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"api_revision_id":"`+revision.ID+`"`) {
		t.Fatalf("API developer-asset publication detail = %d: %s", detail.Code, detail.Body.String())
	}
	wrongAPI := request(t, handler, http.MethodGet, "/api/v1/integrations/"+apiA.ID+"/resources/publications/"+publication.ID, "doko_admin_demo", "")
	if wrongAPI.Code != http.StatusNotFound {
		t.Fatalf("cross-API publication detail = %d: %s", wrongAPI.Code, wrongAPI.Body.String())
	}
	body, err := json.Marshal(map[string]any{
		"scope": "api", "api_id": apiA.ID, "api_developer_asset_publication_id": publication.ID,
		"query": "authentication", "limit": 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	leak := request(t, handler, http.MethodPost, "/api/v1/developer-assets/query-lab", "doko_admin_demo", string(body))
	if leak.Code != http.StatusBadRequest || !strings.Contains(leak.Body.String(), "does not belong") {
		t.Fatalf("cross-API query scope = %d: %s", leak.Code, leak.Body.String())
	}
	secretQuery := request(t, handler, http.MethodPost, "/api/v1/developer-assets/query-lab", "doko_admin_demo", `{
		"scope":"api","api_id":"`+apiB.ID+`","query":"Authorization: Bearer ghp_abcdefghijklmnopqrstuvwxyz1234567890"
	}`)
	if secretQuery.Code != http.StatusBadRequest || !strings.Contains(secretQuery.Body.String(), "secret-like") {
		t.Fatalf("secret-like Query Lab input = %d: %s", secretQuery.Code, secretQuery.Body.String())
	}
	invalidKind := request(t, handler, http.MethodPost, "/api/v1/developer-assets/query-lab", "doko_admin_demo", `{
		"scope":"api","api_id":"`+apiB.ID+`","query":"authentication","asset_kinds":["mixed"]
	}`)
	if invalidKind.Code != http.StatusBadRequest || !strings.Contains(invalidKind.Body.String(), "asset_kinds") {
		t.Fatalf("invalid Query Lab asset kind = %d: %s", invalidKind.Code, invalidKind.Body.String())
	}

	malformed := request(t, handler, http.MethodPost, "/api/v1/developer-assets/api-contracts/contract-a/candidates/candidate-a/publish", "doko_admin_demo", `{
		"contract_revision":1,"acknowledge_reviewed":true,"auto_publish_without_review":true
	}`)
	if malformed.Code != http.StatusBadRequest || !strings.Contains(malformed.Body.String(), "unknown field") {
		t.Fatalf("malformed review = %d: %s", malformed.Code, malformed.Body.String())
	}
}
