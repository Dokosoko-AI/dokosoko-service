package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type recipeDeveloperAssetEvidenceStore struct {
	store.Store
	deployment   model.Deployment
	integration  model.Integration
	global       model.DeploymentDocumentationPublication
	api          model.APIDeveloperAssetPublication
	otherAPI     model.APIDeveloperAssetPublication
	apiRevision  model.IntegrationRevision
	globalUnits  []store.DeveloperAssetKnowledgeResult
	apiUnits     []store.DeveloperAssetKnowledgeResult
	otherAPIUnit store.DeveloperAssetKnowledgeResult
	queries      []store.DeveloperAssetKnowledgeQuery
}

func (s *recipeDeveloperAssetEvidenceStore) Deployment(ctx context.Context) (model.Deployment, error) {
	if s.deployment.ID != "" {
		return s.deployment, nil
	}
	return s.Store.Deployment(ctx)
}

func (s *recipeDeveloperAssetEvidenceStore) Integration(ctx context.Context, deploymentID, id string) (model.Integration, error) {
	if s.integration.DeploymentID == deploymentID && s.integration.ID == id {
		return s.integration, nil
	}
	return s.Store.Integration(ctx, deploymentID, id)
}

func (s *recipeDeveloperAssetEvidenceStore) APIDeveloperAssetPublications(_ context.Context, deploymentID, apiID string) ([]model.APIDeveloperAssetPublication, error) {
	if deploymentID != s.api.DeploymentID {
		return nil, store.ErrNotFound
	}
	switch apiID {
	case s.api.APIID:
		return []model.APIDeveloperAssetPublication{s.api}, nil
	case s.otherAPI.APIID:
		return []model.APIDeveloperAssetPublication{s.otherAPI}, nil
	default:
		return nil, store.ErrNotFound
	}
}

func (s *recipeDeveloperAssetEvidenceStore) APIDeveloperAssetPublication(_ context.Context, deploymentID, id string) (model.APIDeveloperAssetPublication, error) {
	for _, publication := range []model.APIDeveloperAssetPublication{s.api, s.otherAPI} {
		if publication.DeploymentID == deploymentID && publication.ID == id {
			return publication, nil
		}
	}
	return model.APIDeveloperAssetPublication{}, store.ErrNotFound
}

func (s *recipeDeveloperAssetEvidenceStore) DeploymentDocumentationPublication(_ context.Context, deploymentID, id string) (model.DeploymentDocumentationPublication, error) {
	if s.global.DeploymentID != deploymentID || s.global.ID != id {
		return model.DeploymentDocumentationPublication{}, store.ErrNotFound
	}
	return s.global, nil
}

func (s *recipeDeveloperAssetEvidenceStore) IntegrationRevisions(_ context.Context, integrationID string) ([]model.IntegrationRevision, error) {
	if s.apiRevision.IntegrationID != integrationID {
		return nil, store.ErrNotFound
	}
	return []model.IntegrationRevision{s.apiRevision}, nil
}

func (s *recipeDeveloperAssetEvidenceStore) SearchIndexGenerations(_ context.Context, deploymentID, publicationKind, publicationID string) ([]model.SearchIndexGeneration, error) {
	dimensions := developerAssetEmbeddingDimensions
	assetKind := "mixed"
	if publicationKind == "global_documentation" {
		assetKind = "documentation"
	}
	return []model.SearchIndexGeneration{{
		ID: "generation-" + publicationID, DeploymentID: deploymentID,
		PublicationKind: publicationKind, PublicationID: publicationID, AssetKind: assetKind,
		BuilderVersion: DeveloperAssetIndexBuilderVersion, RetrievalProfileVersion: DeveloperAssetRetrievalProfileVersion,
		EmbeddingModel: developerAssetEmbeddingModel, EmbeddingDimensions: &dimensions, State: "ready",
	}}, nil
}

func (s *recipeDeveloperAssetEvidenceStore) RetrieveDeveloperAssetKnowledge(_ context.Context, query store.DeveloperAssetKnowledgeQuery) ([]store.DeveloperAssetKnowledgeResult, error) {
	s.queries = append(s.queries, query)
	if query.DeploymentDocumentationPublicationID == s.global.ID && query.APIDeveloperAssetPublicationID == "" {
		return append([]store.DeveloperAssetKnowledgeResult(nil), s.globalUnits...), nil
	}
	if query.APIDeveloperAssetPublicationID == s.api.ID && query.APIID == s.api.APIID && query.DeploymentDocumentationPublicationID == "" {
		return append([]store.DeveloperAssetKnowledgeResult(nil), s.apiUnits...), nil
	}
	// A broad or incorrectly API-scoped request would expose the sentinel from
	// the other API and make the test fail visibly.
	return []store.DeveloperAssetKnowledgeResult{s.otherAPIUnit}, nil
}

func recipeDeveloperAssetTestDigest(label string) string {
	return contentHash([]byte(label))
}

func recipeDeveloperAssetTestUnit(scopeKind, indexID, assetKind, sourceKind, sourceID, entityID, title, body string) model.KnowledgeUnit {
	unitHash := recipeDeveloperAssetTestDigest(entityID + "-content")
	metadata := map[string]any{"asset_kind": assetKind}
	switch assetKind {
	case "documentation":
		metadata["collection_revision"] = 3
	case "contract":
		metadata["contract_revision"] = 7
	case "sdk":
		metadata["sdk_content_publication_revision"] = 5
		metadata["exact_version"] = "4.2.1"
		metadata["sdk_content_hash"] = recipeDeveloperAssetTestDigest(sourceID + "-publication")
	}
	metadataJSON, _ := json.Marshal(metadata)
	citationJSON, _ := json.Marshal(map[string]any{
		"publication_kind": sourceKind, "publication_id": sourceID,
		"index_publication_kind": scopeKind, "index_publication_id": indexID,
		"content_hash": unitHash, "canonical_url": "https://docs.example.test/" + entityID,
	})
	return model.KnowledgeUnit{
		ID: "unit-" + entityID, Kind: "section", SourcePublicationKind: sourceKind,
		SourcePublicationID: sourceID, SourceEntityID: entityID, Title: title, Content: body,
		Identifiers: []string{entityID, title}, Visibility: model.VisibilityPrivate,
		Citation: citationJSON, Metadata: metadataJSON, ContentHash: unitHash,
	}
}

func TestRecipeDeveloperAssetEvidenceUsesExactGlobalAndSelectedAPIPublications(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	globalRevisionID := "global-doc-revision"
	apiDocRevisionID := "selected-doc-revision"
	contractRevisionID := "selected-contract-revision"
	sdkPublicationID := "selected-sdk-publication"
	global := model.DeploymentDocumentationPublication{
		ID: "global-publication-v3", DeploymentID: "prod_acme", Revision: 3,
		Visibility: model.VisibilityPrivate, SnapshotHash: recipeDeveloperAssetTestDigest("global-snapshot"),
		Members: []model.DeploymentDocumentationPublicationMember{{
			DocumentationCollectionRevisionID: globalRevisionID,
			ContentHash:                       recipeDeveloperAssetTestDigest("global-revision"), Visibility: model.VisibilityPrivate,
		}},
	}
	api := model.APIDeveloperAssetPublication{
		ID: "selected-api-publication", DeploymentID: "prod_acme", APIID: "api-selected", APIRevisionID: "api-revision-selected",
		DeploymentDocumentationPublicationID: global.ID, SnapshotHash: recipeDeveloperAssetTestDigest("selected-api-snapshot"),
		Documentation: []model.APIPublicationDocumentationAsset{{DocumentationCollectionRevisionID: apiDocRevisionID, ContentHash: recipeDeveloperAssetTestDigest("selected-doc-revision"), Visibility: model.VisibilityPrivate}},
		Contracts:     []model.APIPublicationContractAsset{{APIContractRevisionID: contractRevisionID, ContentHash: recipeDeveloperAssetTestDigest("selected-contract-revision"), Visibility: model.VisibilityPrivate}},
		SDKs: []model.APIPublicationSDKAsset{{
			SDKContentPublicationID: sdkPublicationID, ContentHash: recipeDeveloperAssetTestDigest("selected-sdk-asset"), Visibility: model.VisibilityPrivate,
		}},
	}
	otherAPI := model.APIDeveloperAssetPublication{
		ID: "other-api-publication", DeploymentID: "prod_acme", APIID: "api-other", APIRevisionID: "api-revision-other",
		SnapshotHash: recipeDeveloperAssetTestDigest("other-api-snapshot"),
	}
	globalUnit := recipeDeveloperAssetTestUnit("global_documentation", global.ID, "documentation", "documentation_collection", globalRevisionID, "global-auth", "Global authentication guidance.", "Authenticate requests globally.")
	apiDocUnit := recipeDeveloperAssetTestUnit("api", api.ID, "documentation", "documentation_collection", apiDocRevisionID, "selected-doc", "Selected API guide", "Create payments with the selected API.")
	contractUnit := recipeDeveloperAssetTestUnit("api", api.ID, "contract", "contract", contractRevisionID, "selected-operation", "Create payment operation", "POST /payments")
	sdkUnit := recipeDeveloperAssetTestUnit("api", api.ID, "sdk", "sdk", sdkPublicationID, "selected-sdk-sample", "Create payment sample", "client.payments.create(input)")
	otherSDKUnit := recipeDeveloperAssetTestUnit("api", otherAPI.ID, "sdk", "sdk", "other-sdk-publication", "other-secret-sdk", "Other API SDK", "otherApi.secretOperation()")
	backend := &recipeDeveloperAssetEvidenceStore{
		Store: memory,
		deployment: model.Deployment{
			ID: api.DeploymentID, OrganisationID: "org_acme",
		},
		integration: model.Integration{
			ID: api.APIID, DeploymentID: api.DeploymentID, DisplayName: "Selected API", FamilyKey: "payments", VersionKey: "v1",
		},
		global: global, api: api, otherAPI: otherAPI,
		apiRevision: model.IntegrationRevision{ID: api.APIRevisionID, IntegrationID: api.APIID, Revision: 4, State: "published"},
		globalUnits: []store.DeveloperAssetKnowledgeResult{{Unit: globalUnit, FusedScore: 1}},
		apiUnits: []store.DeveloperAssetKnowledgeResult{
			{Unit: apiDocUnit, FusedScore: .9}, {Unit: contractUnit, FusedScore: .8}, {Unit: sdkUnit, FusedScore: .7},
		},
		otherAPIUnit: store.DeveloperAssetKnowledgeResult{Unit: otherSDKUnit, FusedScore: 10},
	}
	service := New(backend)
	for _, target := range []struct{ kind, id string }{
		{kind: "global_documentation", id: global.ID},
		{kind: "api", id: api.ID},
	} {
		if err := service.ActivateDeveloperAssetPublication(ctx, target.kind, target.id, Actor{ID: "recipe-evidence-test"}); err != nil {
			t.Fatalf("activate %s publication %s: %v", target.kind, target.id, err)
		}
	}
	integration := backend.integration
	evidence, err := service.scopedRecipeDeveloperAssetEvidence(ctx, integration, "create a payment")
	if err != nil {
		t.Fatal(err)
	}

	wantKinds := map[string]bool{
		recipeDeveloperAssetGlobalPublicationKind: false,
		recipeDeveloperAssetAPIPublicationKind:    false,
		recipeDeveloperAssetDocumentationKind:     false,
		recipeDeveloperAssetContractKind:          false,
		recipeDeveloperAssetSDKKind:               false,
	}
	selectedUnits := 0
	for _, item := range evidence {
		if _, tracked := wantKinds[item.Kind]; tracked {
			wantKinds[item.Kind] = true
		}
		switch item.Kind {
		case recipeDeveloperAssetGlobalPublicationKind:
			if item.ResourceID != global.ID || item.Version != "3" || recipeEvidenceField(item.Excerpt, "Publication ID") != global.ID || recipeEvidenceField(item.Excerpt, "Snapshot hash") != global.SnapshotHash {
				t.Fatalf("global publication evidence is not exact: %#v", item)
			}
		case recipeDeveloperAssetAPIPublicationKind:
			if item.ResourceID != api.ID || item.Version != api.APIRevisionID || recipeEvidenceField(item.Excerpt, "Publication ID") != api.ID || recipeEvidenceField(item.Excerpt, "API revision ID") != api.APIRevisionID || recipeEvidenceField(item.Excerpt, "Snapshot hash") != api.SnapshotHash || recipeEvidenceField(item.Excerpt, "Global documentation publication ID") != global.ID {
				t.Fatalf("API publication evidence is not exact: %#v", item)
			}
		}
		if strings.Contains(item.Excerpt, otherAPI.ID) || strings.Contains(item.Excerpt, "other-secret-sdk") || strings.Contains(item.Label, "Other API") {
			t.Fatalf("another API's developer asset leaked into selected evidence: %#v", item)
		}
		if recipeDeveloperAssetSupportingKind(item.Kind) {
			selectedUnits++
			if item.Version == "" || item.Fingerprint == "" || !strings.Contains(item.Excerpt, "Index publication snapshot hash:") || !strings.Contains(item.Excerpt, "Source publication content hash:") || !strings.Contains(item.Excerpt, "Source entity content hash:") {
				t.Fatalf("developer-asset evidence lost exact fingerprints: %#v", item)
			}
			wantIndexID, wantIndexHash := api.ID, api.SnapshotHash
			if recipeEvidenceField(item.Excerpt, "Developer asset scope") == "global_documentation" {
				wantIndexID, wantIndexHash = global.ID, global.SnapshotHash
			}
			if recipeEvidenceField(item.Excerpt, "Index publication ID") != wantIndexID || recipeEvidenceField(item.Excerpt, "Index publication snapshot hash") != wantIndexHash {
				t.Fatalf("selected evidence lost its exact aggregate publication: %#v", item)
			}
		}
	}
	for kind, found := range wantKinds {
		if !found {
			t.Fatalf("missing %s from scoped evidence: %#v", kind, evidence)
		}
	}
	if selectedUnits != 4 {
		t.Fatalf("selected developer-asset units = %d, want 4", selectedUnits)
	}
	if len(backend.queries) != 2 || backend.queries[0].DeploymentDocumentationPublicationID != global.ID || backend.queries[0].APIDeveloperAssetPublicationID != "" || backend.queries[1].APIDeveloperAssetPublicationID != api.ID || backend.queries[1].APIID != api.APIID || backend.queries[1].DeploymentDocumentationPublicationID != "" {
		t.Fatalf("retrieval was not split into exact global and selected-API scopes: %#v", backend.queries)
	}

	dependencies := recipeDependencies(recipeProductEvidence(evidence))
	versions := make(map[string]string, len(dependencies))
	for _, dependency := range dependencies {
		versions[dependency.ResourceID] = dependency.Version
		if recipeDeveloperAssetSupportingKind(dependency.Kind) && (!strings.Contains(dependency.Version, "sha256:") || !strings.Contains(dependency.Version, "@")) {
			t.Fatalf("recipe dependency lost exact publication/revision/content hashes: %#v", dependency)
		}
	}
	for _, item := range recipeProductEvidence(evidence) {
		if recipeDeveloperAssetSupportingKind(item.Kind) && versions[item.ResourceID] != item.Version {
			t.Fatalf("dependency did not preserve the explicit exact asset version: evidence=%#v dependencies=%#v", item, dependencies)
		}
	}
}

func TestRecipeDeveloperAssetDependenciesIgnoreUnrelatedPublicationChanges(t *testing.T) {
	unit := recipeDeveloperAssetTestUnit("api", "api-publication-v1", "contract", "contract", "contract-revision-7", "operation-create", "Create payment", "POST /payments")
	baseContext := recipeDeveloperAssetUnitContext{
		scopeKind: "api", apiID: "api-selected", indexPublicationID: "api-publication-v1",
		indexPublicationHash:  recipeDeveloperAssetTestDigest("api-snapshot-v1"),
		sourcePublicationHash: recipeDeveloperAssetTestDigest("contract-revision"),
		assetContentHash:      recipeDeveloperAssetTestDigest("contract-revision"),
	}
	selected, err := recipeDeveloperAssetEvidenceFromUnit(unit, baseContext)
	if err != nil {
		t.Fatal(err)
	}
	newWrapper := baseContext
	newWrapper.indexPublicationID = "api-publication-v2"
	newWrapper.indexPublicationHash = recipeDeveloperAssetTestDigest("api-snapshot-v2-with-unrelated-sdk")
	unit.Citation, _ = json.Marshal(map[string]any{
		"publication_kind": "contract", "publication_id": unit.SourcePublicationID,
		"index_publication_kind": "api", "index_publication_id": newWrapper.indexPublicationID,
		"content_hash": unit.ContentHash,
	})
	unchanged, err := recipeDeveloperAssetEvidenceFromUnit(unit, newWrapper)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ResourceID != unchanged.ResourceID || selected.Version != unchanged.Version || selected.Fingerprint != unchanged.Fingerprint {
		t.Fatalf("unrelated aggregate publication change altered selected fact:\nold=%#v\nnew=%#v", selected, unchanged)
	}
	if !strings.Contains(selected.Excerpt, baseContext.indexPublicationID) || !strings.Contains(selected.Excerpt, baseContext.indexPublicationHash) {
		t.Fatalf("historical evidence lost its original exact publication: %#v", selected)
	}
	if !strings.Contains(unchanged.Excerpt, newWrapper.indexPublicationID) || !strings.Contains(unchanged.Excerpt, newWrapper.indexPublicationHash) {
		t.Fatalf("current evidence lost its exact new publication: %#v", unchanged)
	}

	changedUnit := unit
	changedUnit.ContentHash = recipeDeveloperAssetTestDigest("operation-create-v2")
	changedUnit.Citation, _ = json.Marshal(map[string]any{
		"publication_kind": "contract", "publication_id": changedUnit.SourcePublicationID,
		"index_publication_kind": "api", "index_publication_id": newWrapper.indexPublicationID,
		"content_hash": changedUnit.ContentHash,
	})
	changed, err := recipeDeveloperAssetEvidenceFromUnit(changedUnit, newWrapper)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Version == selected.Version || changed.Fingerprint == selected.Fingerprint {
		t.Fatalf("selected unit content change retained its old fingerprint: old=%#v new=%#v", selected, changed)
	}

	tool := model.IntegrationEvidence{Kind: "tool", ResourceID: "tool-create", Label: "payments.create", Fingerprint: "tool-v1"}
	integration := model.IntegrationEvidence{Kind: "integration", ResourceID: "api-selected", Label: "Selected API", Fingerprint: "api-v1"}
	scope := model.IntegrationEvidence{Kind: integrationScopeEvidenceKind, ResourceID: "api-selected", Fingerprint: "scope-v1"}
	analysis := model.IntegrationAnalysis{ID: "analysis-v1", Evidence: []model.IntegrationEvidence{scope, integration, tool, selected}}
	seed := model.RecipeSeed{Slug: "create-payment", Title: "Create a payment", Outcome: "Create one payment.", Audience: "coding_agent", CapabilityIDs: []string{tool.ResourceID}, EvidenceIDs: []string{integration.ResourceID, tool.ResourceID, selected.ResourceID}}
	recipe := model.Recipe{
		IntegrationID: "api-selected", AnalysisID: analysis.ID, ContractVersion: model.RecipeContractProductIntegrationV2,
		Title: seed.Title, Outcome: seed.Outcome, Audience: seed.Audience,
		CurrentRevisionID: "recipe-revision", CurrentRevision: &model.RecipeRevision{SpecVersion: model.RecipeSpecVersion2},
		Dependencies: recipeGroundingDependencies(analysis, seed),
	}
	current := analysis
	current.Evidence = []model.IntegrationEvidence{scope, integration, tool, unchanged, {
		Kind: recipeDeveloperAssetSDKKind, ResourceID: "developer_asset:api:api-selected:sdk:unrelated:sdk-sample",
		Version: "unrelated", Fingerprint: "unrelated", Visibility: model.VisibilityPrivate,
	}}
	if !recipeGroundingMatches(recipe, current, seed) {
		t.Fatal("unrelated developer-asset publication change made a dependent recipe stale")
	}
	current.Evidence[3] = changed
	if recipeGroundingMatches(recipe, current, seed) {
		t.Fatal("selected developer-asset content change did not make its dependent recipe stale")
	}

	otherSeed := seed
	otherSeed.Slug, otherSeed.EvidenceIDs = "create-payment-without-contract", []string{integration.ResourceID, tool.ResourceID}
	otherRecipe := recipe
	otherRecipe.Dependencies = recipeGroundingDependencies(analysis, otherSeed)
	if !recipeGroundingMatches(otherRecipe, current, otherSeed) {
		t.Fatal("a recipe which did not select the changed developer asset became stale")
	}
}

func TestRecipeDeveloperAssetEvidenceRemainsSupportingOnly(t *testing.T) {
	values := []model.IntegrationEvidence{
		{Kind: recipeDeveloperAssetDocumentationKind, ResourceID: "docs"},
		{Kind: recipeDeveloperAssetContractKind, ResourceID: "contract"},
		{Kind: recipeDeveloperAssetSDKKind, ResourceID: "sdk"},
		{Kind: "tool", ResourceID: "tool"},
	}
	product := recipeProductEvidence(values)
	if len(product) != len(values) {
		t.Fatalf("developer assets were not admitted as supporting evidence: %#v", product)
	}
	capabilities := recipeProductCapabilityIDs(values)
	if len(capabilities) != 1 || capabilities[0] != "tool" {
		t.Fatalf("developer assets weakened tool capability grounding: %#v", capabilities)
	}
}

type recipeDeveloperAssetHistoricalEvidenceStore struct {
	*recipeDeveloperAssetEvidenceStore
	historicalGlobal model.DeploymentDocumentationPublication
	historicalAPI    model.APIDeveloperAssetPublication
	historicalUnits  []store.DeveloperAssetKnowledgeResult
}

func (s *recipeDeveloperAssetHistoricalEvidenceStore) APIDeveloperAssetPublication(ctx context.Context, deploymentID, id string) (model.APIDeveloperAssetPublication, error) {
	if s.historicalAPI.DeploymentID == deploymentID && s.historicalAPI.ID == id {
		return s.historicalAPI, nil
	}
	return s.recipeDeveloperAssetEvidenceStore.APIDeveloperAssetPublication(ctx, deploymentID, id)
}

func (s *recipeDeveloperAssetHistoricalEvidenceStore) DeploymentDocumentationPublication(ctx context.Context, deploymentID, id string) (model.DeploymentDocumentationPublication, error) {
	if s.historicalGlobal.DeploymentID == deploymentID && s.historicalGlobal.ID == id {
		return s.historicalGlobal, nil
	}
	return s.recipeDeveloperAssetEvidenceStore.DeploymentDocumentationPublication(ctx, deploymentID, id)
}

func (s *recipeDeveloperAssetHistoricalEvidenceStore) RetrieveDeveloperAssetKnowledge(ctx context.Context, query store.DeveloperAssetKnowledgeQuery) ([]store.DeveloperAssetKnowledgeResult, error) {
	if query.DeploymentDocumentationPublicationID == s.historicalGlobal.ID && query.APIDeveloperAssetPublicationID == "" {
		s.queries = append(s.queries, query)
		return nil, nil
	}
	if query.APIDeveloperAssetPublicationID == s.historicalAPI.ID && query.APIID == s.historicalAPI.APIID && query.DeploymentDocumentationPublicationID == "" {
		s.queries = append(s.queries, query)
		return append([]store.DeveloperAssetKnowledgeResult(nil), s.historicalUnits...), nil
	}
	return s.recipeDeveloperAssetEvidenceStore.RetrieveDeveloperAssetKnowledge(ctx, query)
}

func TestRecipeDeveloperAssetHistoricalPublicationEvidenceRemainsReadable(t *testing.T) {
	ctx := context.Background()
	integration := model.Integration{ID: "api-selected", DeploymentID: "prod_acme", DisplayName: "Selected API", FamilyKey: "payments", VersionKey: "v1"}
	historicalGlobal := model.DeploymentDocumentationPublication{
		ID: "global-publication-v1", DeploymentID: integration.DeploymentID, Revision: 1,
		Visibility: model.VisibilityPrivate, SnapshotHash: recipeDeveloperAssetTestDigest("global-snapshot-v1"),
	}
	historicalAPI := model.APIDeveloperAssetPublication{
		ID: "api-publication-v1", DeploymentID: integration.DeploymentID, APIID: integration.ID, APIRevisionID: "api-revision-v1",
		DeploymentDocumentationPublicationID: historicalGlobal.ID, SnapshotHash: recipeDeveloperAssetTestDigest("api-snapshot-v1"),
		Contracts: []model.APIPublicationContractAsset{{APIContractRevisionID: "contract-revision-v1", ContentHash: recipeDeveloperAssetTestDigest("contract-revision-v1"), Visibility: model.VisibilityPrivate}},
	}
	currentGlobal := historicalGlobal
	currentGlobal.ID, currentGlobal.Revision, currentGlobal.SnapshotHash = "global-publication-v2", 2, recipeDeveloperAssetTestDigest("global-snapshot-v2")
	currentAPI := historicalAPI
	currentAPI.ID, currentAPI.APIRevisionID, currentAPI.DeploymentDocumentationPublicationID, currentAPI.SnapshotHash = "api-publication-v2", "api-revision-v2", currentGlobal.ID, recipeDeveloperAssetTestDigest("api-snapshot-v2")

	selectedUnit := recipeDeveloperAssetTestUnit("api", historicalAPI.ID, "contract", "contract", "contract-revision-v1", "operation-selected", "Selected historical operation", "POST /payments")
	selected, err := recipeDeveloperAssetEvidenceFromUnit(selectedUnit, recipeDeveloperAssetUnitContext{
		scopeKind: "api", apiID: integration.ID, indexPublicationID: historicalAPI.ID,
		indexPublicationHash: historicalAPI.SnapshotHash, sourcePublicationHash: historicalAPI.Contracts[0].ContentHash,
		assetContentHash: historicalAPI.Contracts[0].ContentHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	freshUnit := recipeDeveloperAssetTestUnit("api", historicalAPI.ID, "contract", "contract", "contract-revision-v1", "operation-fresh", "Freshly ranked historical operation", "GET /payments/{id}")
	base := &recipeDeveloperAssetEvidenceStore{
		Store: store.NewMemory(),
		deployment: model.Deployment{
			ID: integration.DeploymentID, OrganisationID: "org_acme",
		},
		integration: integration, global: currentGlobal, api: currentAPI,
		apiRevision: model.IntegrationRevision{ID: currentAPI.APIRevisionID, IntegrationID: integration.ID, State: "published"},
	}
	backend := &recipeDeveloperAssetHistoricalEvidenceStore{
		recipeDeveloperAssetEvidenceStore: base, historicalGlobal: historicalGlobal, historicalAPI: historicalAPI,
		historicalUnits: []store.DeveloperAssetKnowledgeResult{{Unit: freshUnit, FusedScore: 1}},
	}
	service := New(backend)
	for _, target := range []struct{ kind, id string }{
		{kind: "global_documentation", id: currentGlobal.ID},
		{kind: "api", id: currentAPI.ID},
	} {
		if err := service.ActivateDeveloperAssetPublication(ctx, target.kind, target.id, Actor{ID: "recipe-history-test"}); err != nil {
			t.Fatalf("activate current %s publication %s: %v", target.kind, target.id, err)
		}
	}
	markers := recipeDeveloperAssetPublicationEvidence(recipeDeveloperAssetScope{global: &historicalGlobal, api: &historicalAPI})
	analysis := model.IntegrationAnalysis{
		Evidence: append([]model.IntegrationEvidence{{Kind: integrationScopeEvidenceKind, ResourceID: integration.ID}}, append(markers, selected)...),
		Plan:     model.IntegrationPlan{Recipes: []model.RecipeSeed{{EvidenceIDs: []string{selected.ResourceID}}}},
	}

	latest, err := service.latestRecipeDeveloperAssetScope(ctx, integration)
	if err != nil {
		t.Fatal(err)
	}
	if latest.api == nil || latest.api.ID != currentAPI.ID {
		t.Fatalf("latest developer-asset scope = %#v, want %s", latest.api, currentAPI.ID)
	}
	historicalRetrieved, err := service.retrieveRecipeDeveloperAssetEvidence(ctx, integration, recipeDeveloperAssetScope{global: &historicalGlobal, api: &historicalAPI}, "create a payment")
	if err != nil || len(historicalRetrieved) != 1 || !strings.Contains(historicalRetrieved[0].ResourceID, "operation-fresh") {
		t.Fatalf("exact historical retrieval = %#v, %v", historicalRetrieved, err)
	}
	grounded, err := service.relevantRecipeDeveloperAssetAnalysis(ctx, model.Product{ID: integration.DeploymentID}, analysis, "create a payment")
	if err != nil {
		t.Fatal(err)
	}
	foundSelected, foundFresh := false, false
	for _, item := range grounded.Evidence {
		foundSelected = foundSelected || item.ResourceID == selected.ResourceID
		foundFresh = foundFresh || strings.Contains(item.ResourceID, "operation-fresh")
		if recipeDeveloperAssetSupportingKind(item.Kind) && strings.Contains(item.Excerpt, currentAPI.ID) {
			t.Fatalf("historical retrieval was silently rebound to the current publication: %#v", item)
		}
	}
	if !foundSelected || !foundFresh {
		t.Fatalf("historical or analysis-selected evidence was not readable after reranking: %#v", grounded.Evidence)
	}
}

type recipeDeveloperAssetPublicEvidenceStore struct {
	store.Store
	integration model.Integration
	publication model.APIDeveloperAssetPublication
	revision    store.DocumentationCollectionRevisionRecord
}

func (s *recipeDeveloperAssetPublicEvidenceStore) Integration(_ context.Context, deploymentID, id string) (model.Integration, error) {
	if s.integration.DeploymentID == deploymentID && s.integration.ID == id {
		return s.integration, nil
	}
	return model.Integration{}, store.ErrNotFound
}

func (s *recipeDeveloperAssetPublicEvidenceStore) APIDeveloperAssetPublication(_ context.Context, deploymentID, id string) (model.APIDeveloperAssetPublication, error) {
	if s.publication.DeploymentID == deploymentID && s.publication.ID == id {
		return s.publication, nil
	}
	return model.APIDeveloperAssetPublication{}, store.ErrNotFound
}

func (s *recipeDeveloperAssetPublicEvidenceStore) DocumentationCollectionRevision(_ context.Context, deploymentID, id string) (store.DocumentationCollectionRevisionRecord, error) {
	if s.revision.Revision.DeploymentID == deploymentID && s.revision.Revision.ID == id {
		return s.revision, nil
	}
	return store.DocumentationCollectionRevisionRecord{}, store.ErrNotFound
}

func TestRecipeDeveloperAssetPublicEvidenceChecksUnderlyingExactRevision(t *testing.T) {
	contentHash := recipeDeveloperAssetTestDigest("public-documentation-revision")
	publication := model.APIDeveloperAssetPublication{
		ID: "api-publication-public", DeploymentID: "prod_acme", APIID: "api-selected", APIRevisionID: "api-revision-public",
		SnapshotHash: recipeDeveloperAssetTestDigest("public-api-snapshot"),
		Documentation: []model.APIPublicationDocumentationAsset{{
			DocumentationCollectionRevisionID: "documentation-revision-public", ContentHash: contentHash, Visibility: model.VisibilityPublic,
		}},
	}
	unit := recipeDeveloperAssetTestUnit("api", publication.ID, "documentation", "documentation_collection", publication.Documentation[0].DocumentationCollectionRevisionID, "public-section", "Public section", "Use the public operation.")
	unit.Visibility = model.VisibilityPublic
	evidence, err := recipeDeveloperAssetEvidenceFromUnit(unit, recipeDeveloperAssetUnitContext{
		scopeKind: "api", apiID: publication.APIID, indexPublicationID: publication.ID,
		indexPublicationHash: publication.SnapshotHash, sourcePublicationHash: contentHash, assetContentHash: contentHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	backend := &recipeDeveloperAssetPublicEvidenceStore{
		Store:       store.NewMemory(),
		integration: model.Integration{ID: publication.APIID, DeploymentID: publication.DeploymentID, Visibility: model.VisibilityPublic},
		publication: publication,
		revision: store.DocumentationCollectionRevisionRecord{Revision: model.DocumentationCollectionRevision{
			ID: publication.Documentation[0].DocumentationCollectionRevisionID, DeploymentID: publication.DeploymentID,
			Visibility: model.VisibilityPublic, ContentHash: contentHash,
		}},
	}
	service := New(backend)
	recipe := model.Recipe{IntegrationID: publication.APIID}
	if err := service.validatePublicRecipeDeveloperAssetEvidence(context.Background(), publication.DeploymentID, recipe, evidence); err != nil {
		t.Fatalf("exact public developer evidence was rejected: %v", err)
	}
	backend.revision.Revision.Visibility = model.VisibilityPrivate
	if err := service.validatePublicRecipeDeveloperAssetEvidence(context.Background(), publication.DeploymentID, recipe, evidence); !errors.Is(err, errPublicRecipeEvidence) {
		t.Fatalf("private underlying revision passed public evidence checks: %v", err)
	}
}
