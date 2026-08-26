package httpapi_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func mcpAssetHash(value string) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(value)))
}

type scopedMCPDeveloperAssetStore struct {
	*store.Memory
	apiIndexes map[string]store.SearchIndexGenerationRecord
}

func (s *scopedMCPDeveloperAssetStore) SearchIndexGenerations(ctx context.Context, deploymentID, publicationKind, publicationID string) ([]model.SearchIndexGeneration, error) {
	if publicationKind == "api" {
		if record, ok := s.apiIndexes[publicationID]; ok && record.Generation.DeploymentID == deploymentID {
			return []model.SearchIndexGeneration{record.Generation}, nil
		}
	}
	return s.Memory.SearchIndexGenerations(ctx, deploymentID, publicationKind, publicationID)
}

func (s *scopedMCPDeveloperAssetStore) SearchIndexGeneration(ctx context.Context, deploymentID, id string) (store.SearchIndexGenerationRecord, error) {
	for _, record := range s.apiIndexes {
		if record.Generation.ID == id {
			if record.Generation.DeploymentID != deploymentID {
				return store.SearchIndexGenerationRecord{}, store.ErrNotFound
			}
			return record, nil
		}
	}
	return s.Memory.SearchIndexGeneration(ctx, deploymentID, id)
}

type scopedMCPUnit struct {
	id           string
	title        string
	content      string
	selectorHash string
}

func scopedMCPAPIIndex(deploymentID, apiID, publicationID, generationID string, values ...scopedMCPUnit) store.SearchIndexGenerationRecord {
	now := time.Now().UTC()
	record := store.SearchIndexGenerationRecord{
		Generation: model.SearchIndexGeneration{
			ID: generationID, DeploymentID: deploymentID, PublicationKind: "api", PublicationID: publicationID,
			AssetKind: "mixed", BuilderVersion: platform.DeveloperAssetIndexBuilderVersion,
			RetrievalProfileVersion: platform.DeveloperAssetRetrievalProfileVersion, State: "ready",
			UnitCount: len(values), ContentHash: mcpAssetHash("generation-" + generationID), ReadyAt: &now, CreatedAt: now,
		},
	}
	for ordinal, value := range values {
		record.Units = append(record.Units, model.KnowledgeUnit{
			ID: value.id, SearchIndexGenerationID: generationID, DeploymentID: deploymentID,
			Kind: "documentation_section", SourcePublicationKind: "documentation_collection",
			SourcePublicationID: "shared-documentation-revision", SourceEntityID: value.id + "-source",
			Title: value.title, Breadcrumb: []string{"Shared guide", value.title}, Content: value.content,
			Identifiers: []string{value.id, value.title}, Visibility: model.VisibilityPublic,
			Citation: json.RawMessage(fmt.Sprintf(`{"index_publication_kind":"api","index_publication_id":%q,"documentation_section_id":%q}`, publicationID, value.id)),
			Metadata: json.RawMessage(`{"asset_kind":"documentation"}`), ContentHash: mcpAssetHash(value.content), Ordinal: ordinal,
		})
		record.APIScopes = append(record.APIScopes, model.KnowledgeUnitAPIScope{
			KnowledgeUnitID: value.id, DeploymentID: deploymentID, APIID: apiID,
			ScopeKind: "selected", SelectorHash: value.selectorHash, CreatedAt: now,
		})
	}
	return record
}

func TestMCPPublishesExactDocumentationMapsAndScopedSearch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	collectionID := "10000000-0000-4000-8000-000000000001"
	revisionID := "10000000-0000-4000-8000-000000000002"
	mapID := "10000000-0000-4000-8000-000000000003"
	revisionHash, mapHash := mcpAssetHash("developer-guide-v1"), mcpAssetHash("developer-guide-map-v1")
	_, err = memory.CreateDocumentationCollection(ctx, model.DocumentationCollection{
		ID: collectionID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID,
		Name: "Developer Guide", Slug: "developer-guide", Description: "Reviewed public guidance.",
		Visibility: model.VisibilityPublic, Lifecycle: "active",
	}, store.DocumentationCollectionRevisionRecord{
		Revision: model.DocumentationCollectionRevision{
			ID: revisionID, DeploymentID: deployment.ID, DocumentationCollectionID: collectionID,
			Revision: 1, Visibility: model.VisibilityPublic, ContentHash: revisionHash,
			SelectionManifest: []byte(`[]`), ReviewedBy: "reviewer", ReviewedAt: time.Now().UTC(), PublishedAt: time.Now().UTC(),
		},
		Map: &model.DocumentationMap{
			ID: mapID, DeploymentID: deployment.ID, DocumentationCollectionRevisionID: revisionID,
			MapVersion: "documentation-map-v1", Map: model.DocumentationMapBody{
				Overview: "Authentication and account setup for developers.",
				Topics:   []model.KnowledgeMapEntry{{ID: "authentication", Kind: "topic", Title: "Authentication", Summary: "Configure OAuth before API calls."}},
			},
			AgentMarkdown: "# Documentation Map\n\n## Table of contents\n\n- Authentication — configure OAuth before API calls.\n",
			ContentHash:   mapHash, Visibility: model.VisibilityPublic,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	publicationID := "10000000-0000-4000-8000-000000000004"
	publication, err := memory.PublishDeploymentDocumentation(ctx, model.DeploymentDocumentationPublication{
		ID: publicationID, DeploymentID: deployment.ID, Revision: 1, Visibility: model.VisibilityPublic,
		SnapshotSchemaVersion: "developer-assets-v1", SnapshotHash: mcpAssetHash("global-docs-v1"),
		Members: []model.DeploymentDocumentationPublicationMember{{
			DocumentationCollectionRevisionID: revisionID, Ordinal: 0, ContentHash: revisionHash, Visibility: model.VisibilityPublic,
		}},
		PublishedBy: "reviewer", PublishedAt: time.Now().UTC(),
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.ActivateDeveloperAssetPublication(ctx, "global_documentation", publication.ID, platform.Actor{ID: "mcp-test"}); err != nil {
		t.Fatal(err)
	}
	generations, err := memory.SearchIndexGenerations(ctx, deployment.ID, "global_documentation", publication.ID)
	if err != nil || len(generations) != 1 {
		t.Fatalf("global index generations = %#v, %v", generations, err)
	}
	index, err := memory.SearchIndexGeneration(ctx, deployment.ID, generations[0].ID)
	if err != nil || len(index.Units) != 1 {
		t.Fatalf("global index = %#v, %v", index, err)
	}
	product, err := memory.Product(ctx, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	product.PublicMCPEnabled = true
	if _, err = memory.UpdateProduct(ctx, product, product.Revision); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})

	listed := request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`)
	globalURI := "dokosoko://developer-assets/global-documentation/" + publication.ID
	mapURI := globalURI + "/evidence/" + index.Units[0].ID
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), globalURI) || !strings.Contains(listed.Body.String(), mapURI) || !strings.Contains(listed.Body.String(), mapHash) {
		t.Fatalf("resources/list = %d: %s", listed.Code, listed.Body.String())
	}
	read := request(t, handler, http.MethodPost, "/mcp/public", "", fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":%q}}`, mapURI))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), "Table of contents") || !strings.Contains(read.Body.String(), mapHash) {
		t.Fatalf("resources/read = %d: %s", read.Code, read.Body.String())
	}
	tools := request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`)
	if tools.Code != http.StatusOK || !strings.Contains(tools.Body.String(), `"name":"developer_assets.search"`) || !strings.Contains(tools.Body.String(), "never cross into another API") {
		t.Fatalf("tools/list = %d: %s", tools.Code, tools.Body.String())
	}
	searched := request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"developer_assets.search","arguments":{"query":"How do I configure authentication?","scope":"global","limit":5}}}`)
	if searched.Code != http.StatusOK || !strings.Contains(searched.Body.String(), `"trace_id"`) || !strings.Contains(searched.Body.String(), `"source_publication_kind":"documentation_collection"`) || !strings.Contains(searched.Body.String(), publication.ID) {
		t.Fatalf("developer_assets.search = %d: %s", searched.Code, searched.Body.String())
	}

	api, err := service.CreateIntegration(ctx, platform.IntegrationInput{
		FamilyKey: "payments", VersionKey: "v1", DisplayName: "Payments API",
		Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: "active",
	}, platform.Actor{ID: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.PublishIntegration(ctx, api.ID, platform.Actor{ID: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	privateGlobal, err := memory.PublishDeploymentDocumentation(ctx, model.DeploymentDocumentationPublication{
		ID: "10000000-0000-4000-8000-000000000007", DeploymentID: deployment.ID, Revision: 2,
		Visibility: model.VisibilityPrivate, SnapshotSchemaVersion: "developer-assets-v1", SnapshotHash: mcpAssetHash("private-global-docs-v2"),
		Members: []model.DeploymentDocumentationPublicationMember{{
			DocumentationCollectionRevisionID: revisionID, Ordinal: 0, ContentHash: revisionHash, Visibility: model.VisibilityPublic,
		}}, PublishedBy: "reviewer", PublishedAt: time.Now().UTC(),
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ActivateDeveloperAssetPublication(ctx, "global_documentation", privateGlobal.ID, platform.Actor{ID: "mcp-test"}); err != nil {
		t.Fatal(err)
	}
	// Use a fresh server instance so this assertion exercises the newly bumped
	// catalog revision rather than the earlier discovery cache entry.
	handler = httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})
	combined := request(t, handler, http.MethodPost, "/mcp/public", "", fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"developer_assets.search","arguments":{"query":"authentication","scope":"combined","api_id":%q}}}`, api.ID))
	if combined.Code != http.StatusOK || !strings.Contains(combined.Body.String(), "public global documentation is unavailable") || strings.Contains(combined.Body.String(), `"trace_id"`) {
		t.Fatalf("public combined private-global scope = %d: %s", combined.Code, combined.Body.String())
	}
}

func enablePublicMCPForDeveloperAssetTest(t *testing.T, ctx context.Context, memory *store.Memory, deploymentID string) {
	t.Helper()
	product, err := memory.Product(ctx, deploymentID)
	if err != nil {
		t.Fatal(err)
	}
	product.PublicMCPEnabled = true
	if _, err = memory.UpdateProduct(ctx, product, product.Revision); err != nil {
		t.Fatal(err)
	}
}

func TestMCPAPIEvidenceHonorsDifferentSelectorsForSharedDocumentation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{ID: "selector-reviewer"}
	apiA, err := service.CreateIntegration(ctx, platform.IntegrationInput{
		FamilyKey: "selector-a", VersionKey: "v1", DisplayName: "Selector A API",
		Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.PublishIntegration(ctx, apiA.ID, actor); err != nil {
		t.Fatal(err)
	}
	publicationA, err := service.ReadyAPIDeveloperAssetPublication(ctx, apiA.ID)
	if err != nil {
		t.Fatal(err)
	}
	apiB, err := service.CreateIntegration(ctx, platform.IntegrationInput{
		FamilyKey: "selector-b", VersionKey: "v1", DisplayName: "Selector B API",
		Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.PublishIntegration(ctx, apiB.ID, actor); err != nil {
		t.Fatal(err)
	}
	publicationB, err := service.ReadyAPIDeveloperAssetPublication(ctx, apiB.ID)
	if err != nil {
		t.Fatal(err)
	}

	unitA := scopedMCPUnit{id: "selector-a-unit", title: "Only API A", content: "EVIDENCE-ONLY-FOR-API-A", selectorHash: mcpAssetHash("selector-a")}
	unitB := scopedMCPUnit{id: "selector-b-unit", title: "Only API B", content: "EVIDENCE-ONLY-FOR-API-B", selectorHash: mcpAssetHash("selector-b")}
	overlay := &scopedMCPDeveloperAssetStore{Memory: memory, apiIndexes: map[string]store.SearchIndexGenerationRecord{
		publicationA.ID: scopedMCPAPIIndex(deployment.ID, apiA.ID, publicationA.ID, "selector-a-generation", unitA),
		publicationB.ID: scopedMCPAPIIndex(deployment.ID, apiB.ID, publicationB.ID, "selector-b-generation", unitB),
	}}
	enablePublicMCPForDeveloperAssetTest(t, ctx, memory, deployment.ID)
	handler := httpapi.NewWithOptions(platform.New(overlay), httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})

	prefixA := "dokosoko://developer-assets/apis/" + apiA.ID + "/publications/" + publicationA.ID
	prefixB := "dokosoko://developer-assets/apis/" + apiB.ID + "/publications/" + publicationB.ID
	uriA := prefixA + "/evidence/" + unitA.id
	uriB := prefixB + "/evidence/" + unitB.id
	listed := request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), uriA) || !strings.Contains(listed.Body.String(), uriB) ||
		!strings.Contains(listed.Body.String(), unitA.selectorHash) || !strings.Contains(listed.Body.String(), unitB.selectorHash) {
		t.Fatalf("selector-scoped resources/list = %d: %s", listed.Code, listed.Body.String())
	}
	if strings.Contains(listed.Body.String(), "dokosoko://developer-assets/documentation/shared-documentation-revision/map") {
		t.Fatalf("resources/list exposed an unscoped source revision: %s", listed.Body.String())
	}
	readA := request(t, handler, http.MethodPost, "/mcp/public", "", fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":%q}}`, uriA))
	if readA.Code != http.StatusOK || !strings.Contains(readA.Body.String(), unitA.content) || strings.Contains(readA.Body.String(), unitB.content) {
		t.Fatalf("API A evidence read = %d: %s", readA.Code, readA.Body.String())
	}
	for _, deniedURI := range []string{
		prefixA + "/evidence/" + unitB.id,
		"dokosoko://developer-assets/documentation/shared-documentation-revision/map",
	} {
		response := request(t, handler, http.MethodPost, "/mcp/public", "", fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":%q}}`, deniedURI))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":-32004`) || strings.Contains(response.Body.String(), unitB.content) {
			t.Fatalf("selector-forbidden resources/read %q = %d: %s", deniedURI, response.Code, response.Body.String())
		}
	}
}

func TestMCPHistoricalPublicationReadsOnlyItsExactReadyEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{ID: "historical-selector-reviewer"}
	api, err := service.CreateIntegration(ctx, platform.IntegrationInput{
		FamilyKey: "historical-selector", VersionKey: "v1", DisplayName: "Historical Selector API",
		Description: "Old snapshot", Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.PublishIntegration(ctx, api.ID, actor); err != nil {
		t.Fatal(err)
	}
	oldPublication, err := service.ReadyAPIDeveloperAssetPublication(ctx, api.ID)
	if err != nil {
		t.Fatal(err)
	}
	api, err = service.UpdateIntegration(ctx, api.ID, platform.IntegrationInput{
		FamilyKey: api.FamilyKey, VersionKey: api.VersionKey, DisplayName: api.DisplayName,
		Description: "New snapshot", Visibility: model.VisibilityPublic, Lifecycle: "active", Revision: api.Revision,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.PublishIntegration(ctx, api.ID, actor); err != nil {
		t.Fatal(err)
	}
	newPublication, err := service.ReadyAPIDeveloperAssetPublication(ctx, api.ID)
	if err != nil {
		t.Fatal(err)
	}
	if newPublication.ID == oldPublication.ID {
		t.Fatalf("publishing a new API revision did not create a new developer-asset publication")
	}

	oldUnit := scopedMCPUnit{id: "historical-old-unit", title: "Old selected evidence", content: "HISTORICAL-ALLOWED-EVIDENCE", selectorHash: mcpAssetHash("old-selector")}
	newUnit := scopedMCPUnit{id: "historical-new-unit", title: "New selected evidence", content: "CURRENT-ONLY-EVIDENCE", selectorHash: mcpAssetHash("new-selector")}
	overlay := &scopedMCPDeveloperAssetStore{Memory: memory, apiIndexes: map[string]store.SearchIndexGenerationRecord{
		oldPublication.ID: scopedMCPAPIIndex(deployment.ID, api.ID, oldPublication.ID, "historical-old-generation", oldUnit),
		newPublication.ID: scopedMCPAPIIndex(deployment.ID, api.ID, newPublication.ID, "historical-new-generation", newUnit),
	}}
	enablePublicMCPForDeveloperAssetTest(t, ctx, memory, deployment.ID)
	handler := httpapi.NewWithOptions(platform.New(overlay), httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})

	oldPrefix := "dokosoko://developer-assets/apis/" + api.ID + "/publications/" + oldPublication.ID
	newPrefix := "dokosoko://developer-assets/apis/" + api.ID + "/publications/" + newPublication.ID
	oldURI := oldPrefix + "/evidence/" + oldUnit.id
	newURI := newPrefix + "/evidence/" + newUnit.id
	listed := request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), newURI) || strings.Contains(listed.Body.String(), oldURI) || strings.Contains(listed.Body.String(), oldPrefix) {
		t.Fatalf("current-only resources/list = %d: %s", listed.Code, listed.Body.String())
	}
	for index, readCase := range []struct {
		uri    string
		marker string
	}{{oldPrefix, oldPublication.SnapshotHash}, {oldURI, oldUnit.content}} {
		response := request(t, handler, http.MethodPost, "/mcp/public", "", fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"resources/read","params":{"uri":%q}}`, index+2, readCase.uri))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), readCase.marker) || strings.Contains(response.Body.String(), `"code":-32004`) {
			t.Fatalf("historical resources/read %q = %d: %s", readCase.uri, response.Code, response.Body.String())
		}
	}
	for _, deniedURI := range []string{
		oldPrefix + "/evidence/" + newUnit.id,
		oldPrefix + "/evidence/never-selected-evidence",
		"dokosoko://developer-assets/sdks/retained-source-publication/samples/forbidden-sample",
	} {
		response := request(t, handler, http.MethodPost, "/mcp/public", "", fmt.Sprintf(`{"jsonrpc":"2.0","id":10,"method":"resources/read","params":{"uri":%q}}`, deniedURI))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":-32004`) ||
			strings.Contains(response.Body.String(), newUnit.content) {
			t.Fatalf("historical forbidden resources/read %q = %d: %s", deniedURI, response.Code, response.Body.String())
		}
	}
}
