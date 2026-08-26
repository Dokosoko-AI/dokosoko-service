package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func historicalIdentityMCPHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func TestMCPHistoricalResourcesUseImmutableIndexUnitSnapshotsAfterRename(t *testing.T) {
	const (
		deploymentID  = "prod_acme"
		apiID         = "payments-api"
		publicationID = "historical-api-publication"
		generationID  = "historical-index-generation"
	)
	now := time.Now().UTC()
	units := []model.KnowledgeUnit{
		{
			ID: "historical-documentation-unit", SearchIndexGenerationID: generationID, DeploymentID: deploymentID,
			Kind: "map", SourcePublicationKind: "documentation_collection", SourcePublicationID: "historical-documentation-revision",
			SourceEntityID: "historical-documentation-map", Title: "Original Guide documentation map",
			Breadcrumb: []string{"Original Guide"}, Content: "# Original Guide map", Visibility: model.VisibilityPrivate,
			Citation:    json.RawMessage(`{"documentation_collection_revision_id":"historical-documentation-revision"}`),
			Metadata:    json.RawMessage(`{"asset_kind":"documentation","documentation_collection_name":"Original Guide","documentation_collection_description":"Original guide description."}`),
			ContentHash: historicalIdentityMCPHash("documentation-map"), Ordinal: 0,
		},
		{
			ID: "historical-contract-unit", SearchIndexGenerationID: generationID, DeploymentID: deploymentID,
			Kind: "contract_operation", SourcePublicationKind: "contract", SourcePublicationID: "historical-contract-revision",
			SourceEntityID: "historical-contract-operation", Title: "Original Contract operation",
			Breadcrumb: []string{"Original Contract", "GET /payments"}, Content: "GET /payments", Visibility: model.VisibilityPrivate,
			Citation:    json.RawMessage(`{"api_contract_revision_id":"historical-contract-revision"}`),
			Metadata:    json.RawMessage(`{"asset_kind":"contract","api_contract_name":"Original Contract","api_contract_description":"Original contract description."}`),
			ContentHash: historicalIdentityMCPHash("contract-operation"), Ordinal: 1,
		},
	}
	record := store.SearchIndexGenerationRecord{
		Generation: model.SearchIndexGeneration{
			ID: generationID, DeploymentID: deploymentID, PublicationKind: "api", PublicationID: publicationID,
			AssetKind: "mixed", BuilderVersion: "builder", RetrievalProfileVersion: "profile", State: "ready",
			UnitCount: len(units), ContentHash: historicalIdentityMCPHash("generation"), ReadyAt: &now,
		},
		Units: units,
		APIScopes: []model.KnowledgeUnitAPIScope{
			{KnowledgeUnitID: units[0].ID, DeploymentID: deploymentID, APIID: apiID, ScopeKind: "attached"},
			{KnowledgeUnitID: units[1].ID, DeploymentID: deploymentID, APIID: apiID, ScopeKind: "attached"},
		},
	}
	resources, toc, err := publicationScopedDeveloperAssetEvidence(record, []string{"apis", apiID, "publications", publicationID}, apiID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != len(units) {
		t.Fatalf("historical MCP resource count = %d, want %d", len(resources), len(units))
	}
	if !strings.Contains(toc, "Original Guide") || !strings.Contains(toc, "Original Contract") {
		t.Fatalf("historical MCP table of contents lost snapshot identity: %s", toc)
	}
	for _, resource := range resources {
		if strings.Contains(resource.Title, "Renamed") || strings.Contains(resource.Text, "Renamed") ||
			!strings.HasPrefix(resource.URI, "dokosoko://developer-assets/apis/"+apiID+"/publications/"+publicationID+"/evidence/") {
			t.Fatalf("historical MCP evidence escaped its immutable publication snapshot: %#v", resource)
		}
	}
}
