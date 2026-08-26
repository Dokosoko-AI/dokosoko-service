package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

var (
	_ DeveloperAssetStore = (*Memory)(nil)
	_ DeveloperAssetStore = (*Postgres)(nil)
)

func developerAssetTestHash(digit string) string {
	return "sha256:" + strings.Repeat(digit, 64)
}

func completeMemoryDeveloperAssetIndex(t *testing.T, memory *Memory, generation model.SearchIndexGeneration, units []model.KnowledgeUnit, scopes []model.KnowledgeUnitAPIScope) {
	t.Helper()
	ctx := context.Background()
	created, err := memory.CreateSearchIndexGeneration(ctx, generation)
	if err != nil {
		t.Fatalf("create search index generation %s: %v", generation.ID, err)
	}
	readyAt := time.Now().UTC()
	created.State = "ready"
	created.ReadyAt = &readyAt
	created.UnitCount = len(units)
	created.ContentHash = developerAssetTestHash("a")
	if _, err := memory.CompleteSearchIndexGeneration(ctx, SearchIndexGenerationRecord{
		Generation: created,
		Units:      units,
		APIScopes:  scopes,
	}, "queued"); err != nil {
		t.Fatalf("complete search index generation %s: %v", generation.ID, err)
	}
}

func TestMemoryDeveloperAssetRetrievalUsesExactPublishedScopeAndHybridRanking(t *testing.T) {
	memory := NewMemory()
	const (
		deploymentID = "prod_acme"
		globalPubID  = "global-publication"
		apiPubAID    = "api-publication-a"
		apiPubBID    = "api-publication-b"
		apiAID       = "api-a"
		apiBID       = "api-b"
	)
	dimensions := 2
	baseGeneration := func(id, publicationKind, publicationID, assetKind string) model.SearchIndexGeneration {
		return model.SearchIndexGeneration{
			ID: id, DeploymentID: deploymentID, PublicationKind: publicationKind, PublicationID: publicationID,
			AssetKind: assetKind, BuilderVersion: "builder-v1", RetrievalProfileVersion: "retrieval-v1",
			EmbeddingModel: "test-embedding", EmbeddingDimensions: &dimensions, State: "queued",
			Diagnostics: json.RawMessage(`{}`),
		}
	}
	unit := func(id, generationID, kind, content, assetKind string, embedding []float32, ordinal int) model.KnowledgeUnit {
		return model.KnowledgeUnit{
			ID: id, SearchIndexGenerationID: generationID, DeploymentID: deploymentID, Kind: kind,
			SourcePublicationKind: "api", SourcePublicationID: apiPubAID, SourceEntityID: id + "-source",
			Title: id, Content: content, Embedding: embedding, Visibility: model.VisibilityPrivate,
			Identifiers: []string{}, Breadcrumb: []string{}, Citation: json.RawMessage(`{}`),
			Metadata:    json.RawMessage(`{"asset_kind":"` + assetKind + `","sdk_release_id":"release-a","exact_version":"1.2.3"}`),
			ContentHash: developerAssetTestHash("b"), Ordinal: ordinal,
		}
	}

	globalUnit := unit("global-guide", "generation-global", "section", "shared getting started guide", "documentation", []float32{0, 1}, 0)
	globalUnit.SourcePublicationKind = "global_documentation"
	globalUnit.SourcePublicationID = globalPubID
	completeMemoryDeveloperAssetIndex(t, memory,
		baseGeneration("generation-global", "global_documentation", globalPubID, "documentation"),
		[]model.KnowledgeUnit{globalUnit}, nil)

	apiAUnits := []model.KnowledgeUnit{
		unit("api-a-sdk", "generation-api-a", "sdk_section", "shared client authentication setup", "sdk", []float32{1, 0}, 0),
		unit("api-a-contract", "generation-api-a", "contract_operation", "payments operation", "contract", []float32{0, 1}, 1),
		unit("wrong-api-scope", "generation-api-a", "sdk_section", "shared private client internals", "sdk", []float32{1, 0}, 2),
	}
	completeMemoryDeveloperAssetIndex(t, memory,
		baseGeneration("generation-api-a", "api", apiPubAID, "mixed"), apiAUnits,
		[]model.KnowledgeUnitAPIScope{
			{KnowledgeUnitID: "api-a-sdk", DeploymentID: deploymentID, APIID: apiAID, ScopeKind: "selected"},
			{KnowledgeUnitID: "api-a-contract", DeploymentID: deploymentID, APIID: apiAID, ScopeKind: "selected"},
			{KnowledgeUnitID: "wrong-api-scope", DeploymentID: deploymentID, APIID: apiBID, ScopeKind: "selected"},
		})

	staleGeneration := baseGeneration("generation-api-a-stale", "api", apiPubAID, "mixed")
	staleGeneration.BuilderVersion = "builder-v0"
	staleGeneration.RetrievalProfileVersion = "retrieval-v0"
	staleUnit := unit("forbidden-stale-generation-marker", staleGeneration.ID, "sdk_section", "FORBIDDEN_STALE_GENERATION_MARKER shared client authentication setup", "sdk", []float32{1, 0}, 0)
	completeMemoryDeveloperAssetIndex(t, memory, staleGeneration, []model.KnowledgeUnit{staleUnit},
		[]model.KnowledgeUnitAPIScope{{KnowledgeUnitID: staleUnit.ID, DeploymentID: deploymentID, APIID: apiAID, ScopeKind: "selected"}})

	apiBUnit := unit("api-b-secret", "generation-api-b", "sdk_section", "shared secret SDK", "sdk", []float32{1, 0}, 0)
	apiBUnit.SourcePublicationID = apiPubBID
	completeMemoryDeveloperAssetIndex(t, memory,
		baseGeneration("generation-api-b", "api", apiPubBID, "mixed"), []model.KnowledgeUnit{apiBUnit},
		[]model.KnowledgeUnitAPIScope{{KnowledgeUnitID: apiBUnit.ID, DeploymentID: deploymentID, APIID: apiBID, ScopeKind: "selected"}})

	ctx := context.Background()
	semantic, err := memory.RetrieveDeveloperAssetKnowledge(ctx, DeveloperAssetKnowledgeQuery{
		DeploymentID: deploymentID, DeploymentDocumentationPublicationID: globalPubID,
		APIDeveloperAssetPublicationID: apiPubAID, APIID: apiAID, AssetKinds: []string{"sdk"},
		BuilderVersion: "builder-v1", RetrievalProfileVersion: "retrieval-v1",
		QueryText: "zephyr quokka", QueryEmbedding: []float32{1, 0}, Limit: 10,
	})
	if err != nil {
		t.Fatalf("hybrid retrieve: %v", err)
	}
	if len(semantic) != 1 || semantic[0].Unit.ID != "api-a-sdk" {
		t.Fatalf("hybrid result = %#v, want only exact API A SDK unit", semantic)
	}
	if semantic[0].LexicalScore != 0 || semantic[0].SemanticScore < 0.99 || semantic[0].FusedScore <= 0 {
		t.Fatalf("unexpected score breakdown: %#v", semantic[0])
	}
	if len(semantic[0].Unit.Embedding) != dimensions {
		t.Fatalf("retrieval dropped internal embedding: %#v", semantic[0].Unit.Embedding)
	}

	lexical, err := memory.RetrieveDeveloperAssetKnowledge(ctx, DeveloperAssetKnowledgeQuery{
		DeploymentID: deploymentID, DeploymentDocumentationPublicationID: globalPubID,
		APIDeveloperAssetPublicationID: apiPubAID, APIID: apiAID, QueryText: "shared", Limit: 10,
		BuilderVersion: "builder-v1", RetrievalProfileVersion: "retrieval-v1",
	})
	if err != nil {
		t.Fatalf("lexical retrieve: %v", err)
	}
	gotIDs := make(map[string]bool, len(lexical))
	for _, result := range lexical {
		gotIDs[result.Unit.ID] = true
	}
	if !gotIDs["global-guide"] || !gotIDs["api-a-sdk"] || gotIDs["wrong-api-scope"] || gotIDs["api-b-secret"] || gotIDs[staleUnit.ID] {
		t.Fatalf("published-scope union leaked or omitted units: %#v", gotIDs)
	}

	stored, err := memory.SearchIndexGeneration(ctx, deploymentID, "generation-api-a")
	if err != nil {
		t.Fatalf("read search index generation: %v", err)
	}
	if len(stored.Units) != len(apiAUnits) || len(stored.Units[0].Embedding) != dimensions {
		t.Fatalf("stored generation lost embeddings: %#v", stored.Units)
	}
	if _, err := memory.RetrieveDeveloperAssetKnowledge(ctx, DeveloperAssetKnowledgeQuery{
		DeploymentID: deploymentID, APIDeveloperAssetPublicationID: apiPubAID, APIID: apiAID,
		BuilderVersion: "builder-v1", RetrievalProfileVersion: "retrieval-v1",
		QueryEmbedding: []float32{1, 0, 0}, Limit: 10,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("dimension mismatch error = %v, want ErrConflict", err)
	}
	if _, err := memory.RetrieveDeveloperAssetKnowledge(ctx, DeveloperAssetKnowledgeQuery{DeploymentID: deploymentID, Limit: 10}); !errors.Is(err, ErrConflict) {
		t.Fatalf("unpublished scope error = %v, want ErrConflict", err)
	}
	if _, err := memory.RetrieveDeveloperAssetKnowledge(ctx, DeveloperAssetKnowledgeQuery{
		DeploymentID: deploymentID, APIDeveloperAssetPublicationID: apiPubAID, APIID: apiAID, Limit: 10,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("missing processor identity error = %v, want ErrConflict", err)
	}
}

func TestMemoryDocumentationBindingOwnsCanonicalSelectorHash(t *testing.T) {
	memory := NewMemory()
	ctx := context.Background()
	const (
		deploymentID = "prod_acme"
		apiID        = "selector-api"
		collectionID = "selector-collection"
		revisionID   = "selector-collection-revision"
		bindingID    = "selector-binding"
	)
	if _, err := memory.CreateIntegration(ctx, model.Integration{
		ID: apiID, DeploymentID: deploymentID, OrganisationID: "org_acme", FamilyKey: "selector-api",
		VersionKey: "v1", DisplayName: "Selector API", Visibility: model.VisibilityPrivate,
	}); err != nil {
		t.Fatalf("create integration: %v", err)
	}
	collectionRevision := model.DocumentationCollectionRevision{
		ID: revisionID, DeploymentID: deploymentID, DocumentationCollectionID: collectionID, Revision: 1,
		Visibility: model.VisibilityPrivate, ContentHash: developerAssetTestHash("c"),
		SelectionManifest: json.RawMessage(`[]`), ReviewedBy: "reviewer", ReviewedAt: time.Now().UTC(),
	}
	if _, err := memory.CreateDocumentationCollection(ctx, model.DocumentationCollection{
		ID: collectionID, DeploymentID: deploymentID, OrganisationID: "org_acme", Name: "Selector docs",
		Slug: "selector-docs", Visibility: model.VisibilityPrivate,
	}, DocumentationCollectionRevisionRecord{Revision: collectionRevision}); err != nil {
		t.Fatalf("create documentation collection: %v", err)
	}
	storedCollectionRevision, err := memory.DocumentationCollectionRevision(ctx, deploymentID, revisionID)
	if err != nil {
		t.Fatal(err)
	}
	if storedCollectionRevision.Revision.DocumentationCollectionName != "Selector docs" ||
		storedCollectionRevision.Revision.DocumentationCollectionSlug != "selector-docs" {
		t.Fatalf("documentation revision did not snapshot root identity: %#v", storedCollectionRevision.Revision)
	}

	selector := json.RawMessage(`{"long":true,"a":1}`)
	binding, err := memory.SaveAPIDocumentationBinding(ctx, model.APIDocumentationBinding{
		ID: bindingID, DeploymentID: deploymentID, APIID: apiID, DocumentationCollectionID: collectionID,
		FollowLatest: true, Selector: selector, Visibility: model.VisibilityPrivate,
	}, 0)
	if err != nil {
		t.Fatalf("save documentation binding: %v", err)
	}
	canonical, err := documentationSelectorCanonicalJSON(selector)
	if err != nil {
		t.Fatalf("canonicalize selector: %v", err)
	}
	if canonical != `{"a": 1, "long": true}` {
		t.Fatalf("canonical selector = %q", canonical)
	}
	wantHash, err := documentationSelectorHash(selector)
	if err != nil {
		t.Fatal(err)
	}
	if binding.SelectorHash != wantHash {
		t.Fatalf("binding selector hash = %q, want %q", binding.SelectorHash, wantHash)
	}

	publishedAt := time.Now().UTC()
	apiRevision, err := memory.CreateIntegrationRevision(ctx, model.IntegrationRevision{
		ID: "selector-api-revision", IntegrationID: apiID, Revision: 1, State: "published",
		Snapshot: json.RawMessage(`{}`), ManifestHash: developerAssetTestHash("d"), PublishedBy: "reviewer", PublishedAt: &publishedAt,
	})
	if err != nil {
		t.Fatalf("create API revision: %v", err)
	}
	publication := model.APIDeveloperAssetPublication{
		ID: "selector-api-publication", DeploymentID: deploymentID, APIID: apiID, APIRevisionID: apiRevision.ID,
		SnapshotSchemaVersion: "v1", SnapshotHash: developerAssetTestHash("e"), PublishedBy: "reviewer",
		Documentation: []model.APIPublicationDocumentationAsset{{
			BindingID: bindingID, DocumentationCollectionRevisionID: revisionID,
			Selector: json.RawMessage(`{"a":1,"long":true}`), SelectorHash: developerAssetTestHash("0"),
			ContentHash: collectionRevision.ContentHash, Visibility: model.VisibilityPrivate,
		}},
	}
	if _, err := memory.CreateAPIDeveloperAssetPublication(ctx, publication); !errors.Is(err, ErrConflict) {
		t.Fatalf("publication with client-derived selector hash error = %v, want ErrConflict", err)
	}
	publication.Documentation[0].SelectorHash = binding.SelectorHash
	created, err := memory.CreateAPIDeveloperAssetPublication(ctx, publication)
	if err != nil {
		t.Fatalf("publish exact binding selector: %v", err)
	}
	if created.Documentation[0].SelectorHash != binding.SelectorHash {
		t.Fatalf("publication selector hash = %q", created.Documentation[0].SelectorHash)
	}
	if created.Documentation[0].DocumentationCollectionID != collectionID ||
		created.Documentation[0].DocumentationCollectionName != "Selector docs" ||
		created.Documentation[0].DocumentationCollectionSlug != "selector-docs" {
		t.Fatalf("publication did not snapshot documentation root identity: %#v", created.Documentation[0])
	}
}

func TestDocumentationBindingSelectorHashMigrationIsAppendOnly(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0058_documentation_binding_selector_hash.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, fragment := range []string{
		"ALTER TABLE api_documentation_bindings",
		"ADD COLUMN selector_hash text",
		"CREATE TRIGGER api_documentation_bindings_selector_hash_trigger",
		"NEW.selector_hash := 'sha256:'",
		"NEW.selector_hash IS DISTINCT FROM binding_selector_hash",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("0058 is missing %q", fragment)
		}
	}
}

func TestMemorySDKCandidatePublicationIsAtomicAndKeepsLegacyProjection(t *testing.T) {
	memory := NewMemory()
	ctx := context.Background()
	const (
		deploymentID  = "prod_acme"
		packageID     = "sdk-package"
		releaseID     = "sdk-release"
		runID         = "sdk-ingestion"
		candidateID   = "sdk-candidate"
		publicationID = "sdk-publication"
		apiID         = "sdk-api"
	)
	packageValue := model.SDKPackage{
		ID: packageID, DeploymentID: deploymentID, OrganisationID: "org_acme", Ecosystem: "npm",
		CanonicalCoordinate: "@acme/client", DisplayCoordinate: "@acme/client", Name: "Acme Client",
		Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}
	if _, err := memory.SaveSDKPackage(ctx, packageValue, 0); err != nil {
		t.Fatalf("create SDK package: %v", err)
	}
	release := model.SDKRelease{
		ID: releaseID, DeploymentID: deploymentID, SDKPackageID: packageID, ExactVersion: "1.2.3",
		InstallCommand: "npm install @acme/client@1.2.3", UpstreamDigest: developerAssetTestHash("1"),
		IdentityAssurance: "verified_digest", Visibility: model.VisibilityPrivate, Lifecycle: "active",
		ReleaseHash: developerAssetTestHash("2"),
	}
	if _, err := memory.CreateSDKRelease(ctx, release); err != nil {
		t.Fatalf("create SDK release: %v", err)
	}
	startedAt := time.Now().UTC()
	run, err := memory.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: runID, DeploymentID: deploymentID, OrganisationID: "org_acme", AssetKind: model.DeveloperAssetSDK,
		TargetID: releaseID, TargetKey: "sdk-release:" + releaseID, State: model.DeveloperAssetIngestionReviewReady,
		Attempt: 1, Versions: model.ProcessorVersions{Pipeline: "pipeline-v1", Parser: "parser-v1", Normalizer: "normalizer-v1", Mapper: "mapper-v1"},
		RawManifest: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`{}`), FailedCount: 1, StartedAt: &startedAt,
	})
	if err != nil {
		t.Fatalf("create SDK ingestion run: %v", err)
	}
	fileHash, sampleHash, uncheckedSampleHash, mapHash := developerAssetTestHash("3"), developerAssetTestHash("4"), developerAssetTestHash("unchecked"), developerAssetTestHash("5")
	candidateHash := developerAssetTestHash("6")
	candidate := SDKContentCandidateRecord{
		Candidate: model.SDKContentCandidate{
			ID: candidateID, DeploymentID: deploymentID, SDKReleaseID: releaseID, IngestionRunID: runID,
			Versions: run.Versions, MapVersion: "sdk-map-v1", SourceManifest: json.RawMessage(`[]`),
			ContentHash: candidateHash, Visibility: model.VisibilityPrivate, Diagnostics: json.RawMessage(`{}`),
		},
		Files: []model.SDKPublicationFile{{
			ID: "sdk-file", SDKContentCandidateID: candidateID, SourcePath: "README.md", Role: "guide",
			MediaType: "text/markdown", SuggestedDisposition: "included", NormalizedContent: "Use the client.",
			ContentHash: fileHash, Metadata: json.RawMessage(`{}`),
		}},
		Samples: []model.SDKCodeSample{
			{
				ID: "sdk-sample", DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
				Language: "typescript", Title: "Create a client", Intent: "Initialize the SDK", Code: "new Client()",
				Imports: []string{"@acme/client"}, Prerequisites: []string{}, Origin: model.SDKSampleCurated,
				ValidationStatus: model.SDKSampleSyntaxChecked, ValidationEvidence: json.RawMessage(`{"validated":true,"validator":"test/parser"}`),
				Visibility: model.VisibilityPrivate, ContentHash: sampleHash,
			},
			{
				ID: "sdk-sample-not-checked", DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
				Language: "rust", Title: "Create a Rust client", Intent: "Initialize the SDK", Code: "Client::new()",
				Origin: model.SDKSampleCurated, ValidationStatus: model.SDKSampleNotChecked,
				ValidationEvidence: json.RawMessage(`{"validated":false,"result":"not_checked"}`),
				Visibility:         model.VisibilityPrivate, ContentHash: uncheckedSampleHash,
			},
		},
		Map: &model.SDKMap{
			ID: "sdk-candidate-map", DeploymentID: deploymentID, SDKContentCandidateID: candidateID, MapVersion: "sdk-candidate-map-v1",
			Map: model.SDKMapBody{Overview: "SDK contents"}, AgentMarkdown: "# SDK contents", ContentHash: mapHash,
		},
	}
	if _, err := memory.CreateSDKContentCandidate(ctx, candidate); err != nil {
		t.Fatalf("create SDK candidate: %v", err)
	}
	ordinal, reviewedOrdinal := 0, 1
	reviewedAt := time.Now().UTC()
	publication := SDKContentPublicationRecord{
		Publication: model.SDKContentPublication{
			ID: publicationID, DeploymentID: deploymentID, SDKReleaseID: releaseID, SDKContentCandidateID: candidateID,
			ContentHash: candidateHash, Visibility: model.VisibilityPrivate, ReviewedBy: "reviewer", ReviewedAt: reviewedAt,
		},
		FileSelections: []model.SDKContentPublicationFileSelection{{
			SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
			SDKPublicationFileID: "sdk-file", Decision: "included", Ordinal: &ordinal, ContentHash: fileHash,
		}},
		SampleSelections: []model.SDKContentPublicationSampleSelection{
			{
				SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
				SDKCodeSampleID: "sdk-sample", Decision: "approved", Ordinal: &ordinal, ReviewedBy: "reviewer",
				ReviewedAt: reviewedAt, ContentHash: sampleHash,
			},
			{
				SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
				SDKCodeSampleID: "sdk-sample-not-checked", Decision: "approved", Ordinal: &reviewedOrdinal, ReviewedBy: "reviewer",
				ReviewedAt: reviewedAt, ReviewEvidence: json.RawMessage(`{"summary":"Reviewer used a pinned Rust grammar parser."}`), ContentHash: uncheckedSampleHash,
			},
		},
	}
	publishedMap, err := BuildReviewedSDKPublicationMap(packageValue, release, candidate, publication)
	if err != nil {
		t.Fatalf("build reviewed SDK map: %v", err)
	}
	publication.PublishedMap = publishedMap
	publication.Map = &model.SDKContentPublicationMap{
		SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
		SDKMapID: publishedMap.ID, ContentHash: publishedMap.ContentHash,
	}
	if _, err := memory.PublishSDKContentCandidate(ctx, publication); !errors.Is(err, ErrConflict) {
		t.Fatalf("publish with failed ingestion output error = %v, want ErrConflict", err)
	}
	storedRun, err := memory.DeveloperAssetIngestionRun(ctx, deploymentID, runID)
	if err != nil {
		t.Fatal(err)
	}
	if storedRun.State != model.DeveloperAssetIngestionReviewReady {
		t.Fatalf("failed publication changed run state to %s", storedRun.State)
	}
	storedRun.FailedCount = 0
	if _, err := memory.TransitionDeveloperAssetIngestionRun(ctx, storedRun, model.DeveloperAssetIngestionReviewReady); err != nil {
		t.Fatalf("clear resolved review counter: %v", err)
	}
	withoutReviewEvidence := publication
	withoutReviewEvidence.SampleSelections = append([]model.SDKContentPublicationSampleSelection(nil), publication.SampleSelections...)
	withoutReviewEvidence.SampleSelections[1].ReviewEvidence = nil
	if _, err := memory.PublishSDKContentCandidate(ctx, withoutReviewEvidence); !errors.Is(err, ErrConflict) {
		t.Fatalf("approve not-checked sample without review evidence error = %v, want ErrConflict", err)
	}
	incomplete := publication
	incomplete.SampleSelections = nil
	if _, err := memory.PublishSDKContentCandidate(ctx, incomplete); !errors.Is(err, ErrConflict) {
		t.Fatalf("publish with incomplete sample decisions error = %v, want ErrConflict", err)
	}
	tamperedMarkdown := publication
	tamperedMarkdown.PublishedMap = memoryClone(publication.PublishedMap)
	tamperedMarkdown.PublishedMap.AgentMarkdown += "\nFORBIDDEN-CALLER-SUPPLIED-MARKER"
	if _, err := memory.PublishSDKContentCandidate(ctx, tamperedMarkdown); !errors.Is(err, ErrConflict) {
		t.Fatalf("publish with caller-supplied map markdown error = %v, want ErrConflict", err)
	}
	tamperedBody := publication
	tamperedBody.PublishedMap = memoryClone(publication.PublishedMap)
	tamperedBody.PublishedMap.Map.Overview = "FORBIDDEN-CALLER-SUPPLIED-MARKER"
	if _, err := memory.PublishSDKContentCandidate(ctx, tamperedBody); !errors.Is(err, ErrConflict) {
		t.Fatalf("publish with caller-supplied map body error = %v, want ErrConflict", err)
	}
	createdPublication, err := memory.PublishSDKContentCandidate(ctx, publication)
	if err != nil {
		t.Fatalf("publish complete SDK candidate: %v", err)
	}
	if createdPublication.Revision != 1 {
		t.Fatalf("SDK publication revision = %d", createdPublication.Revision)
	}
	storedPublication, err := memory.SDKContentPublication(ctx, deploymentID, createdPublication.ID)
	if err != nil || len(storedPublication.SampleSelections) != 2 ||
		len(storedPublication.SampleSelections[0].ReviewEvidence) != 0 ||
		!model.ValidSDKSampleReviewEvidence(storedPublication.SampleSelections[1].ReviewEvidence) {
		t.Fatalf("memory publication review evidence = %#v, err=%v", storedPublication.SampleSelections, err)
	}
	storedRun, err = memory.DeveloperAssetIngestionRun(ctx, deploymentID, runID)
	if err != nil || storedRun.State != model.DeveloperAssetIngestionPublished {
		t.Fatalf("published run = %#v, err=%v", storedRun, err)
	}

	if _, err := memory.CreateIntegration(ctx, model.Integration{
		ID: apiID, DeploymentID: deploymentID, OrganisationID: "org_acme", FamilyKey: "sdk-api", VersionKey: "v1",
		DisplayName: "SDK API", Visibility: model.VisibilityPrivate,
	}); err != nil {
		t.Fatalf("create SDK API: %v", err)
	}
	binding, err := memory.SaveAPISDKBinding(ctx, model.APISDKBinding{
		ID: "sdk-binding", DeploymentID: deploymentID, APIID: apiID, SDKPackageID: packageID, SDKReleaseID: releaseID,
		SDKContentPublicationID: publicationID, State: "ready", Coverage: model.SDKCoverageFull,
		Assurance: model.SDKAssuranceDocumented, Selector: json.RawMessage(`{}`), SelectorHash: developerAssetTestHash("7"),
		Visibility: model.VisibilityPrivate,
	}, 0)
	if err != nil {
		t.Fatalf("save API SDK binding: %v", err)
	}
	legacy, err := memory.SDKReferences(ctx, apiID)
	if err != nil || len(legacy) != 1 || legacy[0].ID != binding.ID || legacy[0].ExactVersion != "1.2.3" {
		t.Fatalf("legacy SDK projection = %#v, err=%v", legacy, err)
	}
	detached, err := memory.DetachAPISDKBinding(ctx, deploymentID, apiID, binding.ID, binding.Revision)
	if err != nil || detached.State != "detached" {
		t.Fatalf("detach SDK binding = %#v, err=%v", detached, err)
	}
	legacy, err = memory.SDKReferences(ctx, apiID)
	if err != nil || len(legacy) != 0 {
		t.Fatalf("detached legacy SDK projection = %#v, err=%v", legacy, err)
	}
	if _, err := memory.DetachAPISDKBinding(ctx, deploymentID, apiID, binding.ID, binding.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale detach error = %v, want ErrConflict", err)
	}
}

func TestMemoryDocumentationExplorerAndCompleteSourceReview(t *testing.T) {
	memory := NewMemory()
	ctx := context.Background()
	const (
		deploymentID  = "prod_acme"
		runID         = "documentation-ingestion"
		publicationID = "pub_docs_seed"
	)
	startedAt := time.Now().UTC()
	if _, err := memory.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: runID, DeploymentID: deploymentID, OrganisationID: "org_acme", AssetKind: model.DeveloperAssetDocumentation,
		TargetID: "src_docs", TargetKey: "source:src_docs", SourceID: "src_docs", State: model.DeveloperAssetIngestionReviewReady,
		Attempt: 1, Versions: model.ProcessorVersions{Pipeline: "pipeline-v1", Parser: "parser-v1", Normalizer: "normalizer-v1", Mapper: "mapper-v1"},
		RawManifest: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`{"quality":"ready"}`), StartedAt: &startedAt,
	}); err != nil {
		t.Fatalf("create documentation run: %v", err)
	}
	documentAHash, documentBHash := developerAssetTestHash("8"), developerAssetTestHash("9")
	mapHash := developerAssetTestHash("a")
	output := DocumentationIngestionOutput{
		Documents: []model.DocumentationDocument{
			{ID: "document-a", DeploymentID: deploymentID, IngestionRunID: runID, SourcePath: "guide.md", Title: "Guide", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "# Guide\nAuthentication", ContentHash: documentAHash, Visibility: model.VisibilityPrivate, Metadata: json.RawMessage(`{}`)},
			{ID: "document-b", DeploymentID: deploymentID, IngestionRunID: runID, SourcePath: "legacy.md", Title: "Legacy", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "# Legacy", ContentHash: documentBHash, Visibility: model.VisibilityPrivate, Ordinal: 1, Metadata: json.RawMessage(`{}`)},
		},
		Sections: []model.DocumentationSection{{
			ID: "section-auth", DeploymentID: deploymentID, DocumentationDocumentID: "document-a", HeadingLevel: 2,
			Heading: "Authentication", Breadcrumb: []string{"Guide", "Authentication"}, ContentKind: "prose",
			NormalizedText: "Use an API key.", TokenEstimate: 5, ContentHash: developerAssetTestHash("b"), Metadata: json.RawMessage(`{}`),
		}},
		Map: &model.DocumentationMap{
			ID: "documentation-map", DeploymentID: deploymentID, IngestionRunID: runID, MapVersion: "map-v1",
			Map: model.DocumentationMapBody{Overview: "Documentation overview"}, AgentMarkdown: "# Documentation overview",
			ContentHash: mapHash, Visibility: model.VisibilityPrivate,
		},
	}
	if err := memory.SaveDocumentationIngestionOutput(ctx, deploymentID, output); err != nil {
		t.Fatalf("save documentation output: %v", err)
	}
	records, err := memory.DocumentationCandidateDocuments(ctx, DocumentationCandidateQuery{
		DeploymentID: deploymentID, IngestionRunID: runID, QueryText: "api key", Limit: 20,
	})
	if err != nil || len(records.Items) != 1 || records.Total != 1 || records.HasMore || records.Items[0].Document.ID != "document-a" || records.Items[0].Run.Diagnostics == nil || records.Items[0].DocumentationMap == nil || records.Items[0].DocumentationMap.ID != "documentation-map" {
		t.Fatalf("documentation explorer records = %#v, err=%v", records, err)
	}
	firstPage, err := memory.DocumentationCandidateDocuments(ctx, DocumentationCandidateQuery{DeploymentID: deploymentID, IngestionRunID: runID, Limit: 1})
	if err != nil || len(firstPage.Items) != 1 || firstPage.Total != 2 || !firstPage.HasMore || firstPage.Items[0].Document.ID != "document-a" {
		t.Fatalf("first documentation page = %#v, err=%v", firstPage, err)
	}
	secondPage, err := memory.DocumentationCandidateDocuments(ctx, DocumentationCandidateQuery{DeploymentID: deploymentID, IngestionRunID: runID, Limit: 1, Offset: 1})
	if err != nil || len(secondPage.Items) != 1 || secondPage.Total != 2 || secondPage.HasMore || secondPage.Items[0].Document.ID != "document-b" || secondPage.Items[0].DocumentationMap == nil || secondPage.Items[0].DocumentationMap.Map.Overview != "Documentation overview" {
		t.Fatalf("second documentation page = %#v, err=%v", secondPage, err)
	}
	section, parent, err := memory.DocumentationCandidateSection(ctx, deploymentID, "section-auth")
	if err != nil || section.Heading != "Authentication" || parent.Document.ID != "document-a" {
		t.Fatalf("exact section lookup = %#v parent=%#v err=%v", section, parent, err)
	}

	ordinal := 0
	reviewedAt := time.Now().UTC()
	review := SourcePublicationDocumentationReview{
		Selections: []model.SourcePublicationDocumentSelection{
			{SourcePublicationID: publicationID, DeploymentID: deploymentID, DocumentationDocumentID: "document-a", Decision: "included", Ordinal: &ordinal, ContentHash: documentAHash, ReviewedBy: "reviewer", ReviewedAt: reviewedAt},
			{SourcePublicationID: publicationID, DeploymentID: deploymentID, DocumentationDocumentID: "document-b", Decision: "excluded", Reason: "obsolete", ContentHash: documentBHash, ReviewedBy: "reviewer", ReviewedAt: reviewedAt},
		},
		MapLink: &model.SourcePublicationDocumentationMap{
			SourcePublicationID: publicationID, DeploymentID: deploymentID, DocumentationMapID: "documentation-map", ContentHash: mapHash,
		},
	}
	incomplete := review
	incomplete.Selections = incomplete.Selections[:1]
	if err := memory.SaveSourcePublicationDocumentationReview(ctx, deploymentID, incomplete); !errors.Is(err, ErrConflict) {
		t.Fatalf("incomplete documentation review error = %v, want ErrConflict", err)
	}
	if err := memory.SaveSourcePublicationDocumentationReview(ctx, deploymentID, review); err != nil {
		t.Fatalf("save complete documentation review: %v", err)
	}
	storedReview, err := memory.SourcePublicationDocumentationReview(ctx, deploymentID, publicationID)
	if err != nil || len(storedReview.Selections) != 2 || storedReview.MapLink == nil {
		t.Fatalf("stored documentation review = %#v, err=%v", storedReview, err)
	}
	run, err := memory.DeveloperAssetIngestionRun(ctx, deploymentID, runID)
	if err != nil || run.State != model.DeveloperAssetIngestionPublished {
		t.Fatalf("documentation run = %#v, err=%v", run, err)
	}
	publicationRecords, err := memory.DocumentationCandidateDocuments(ctx, DocumentationCandidateQuery{
		DeploymentID: deploymentID, SourcePublicationID: publicationID, Limit: 20,
	})
	if err != nil || len(publicationRecords.Items) != 2 || publicationRecords.Total != 2 || publicationRecords.HasMore {
		t.Fatalf("publication document explorer = %#v, err=%v", publicationRecords, err)
	}
}

func TestMemoryContractSourceReassignmentAndCandidatePublication(t *testing.T) {
	memory := NewMemory()
	ctx := context.Background()
	const (
		deploymentID = "prod_acme"
		sourceID     = "src_api"
		contractAID  = "contract-a"
		contractBID  = "contract-b"
		runID        = "contract-ingestion"
		candidateID  = "contract-candidate"
		revisionID   = "contract-revision"
	)
	createContract := func(id, slug string) model.APIContract {
		t.Helper()
		created, err := memory.SaveAPIContract(ctx, model.APIContract{
			ID: id, DeploymentID: deploymentID, OrganisationID: "org_acme", Name: id, Slug: slug,
			Kind: "openapi", Visibility: model.VisibilityPrivate, Lifecycle: "active",
		}, 0)
		if err != nil {
			t.Fatalf("create contract %s: %v", id, err)
		}
		return created
	}
	contractA := createContract(contractAID, "contract-a")
	contractB := createContract(contractBID, "contract-b")
	attachedA, err := memory.SaveAPIContractSource(ctx, model.APIContractSource{
		ID: "contract-source-a", DeploymentID: deploymentID, APIContractID: contractA.ID, SourceID: sourceID,
		SourceRole: "primary", Lifecycle: "attached", CreatedBy: "reviewer",
	}, 0)
	if err != nil {
		t.Fatalf("attach first contract source: %v", err)
	}
	if _, err := memory.SaveAPIContractSource(ctx, model.APIContractSource{
		ID: "contract-source-conflict", DeploymentID: deploymentID, APIContractID: contractB.ID, SourceID: sourceID,
		SourceRole: "primary", Lifecycle: "attached",
	}, 0); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate active source target error = %v, want ErrConflict", err)
	}
	if _, err := memory.DetachAPIContractSource(ctx, deploymentID, attachedA.ID, attachedA.Revision); err != nil {
		t.Fatalf("detach first contract source: %v", err)
	}
	attachedB, err := memory.SaveAPIContractSource(ctx, model.APIContractSource{
		ID: "contract-source-b", DeploymentID: deploymentID, APIContractID: contractB.ID, SourceID: sourceID,
		SourceRole: "primary", Lifecycle: "attached", CreatedBy: "reviewer",
	}, 0)
	if err != nil {
		t.Fatalf("reassign contract source: %v", err)
	}
	active, err := memory.ActiveAPIContractSourceBySource(ctx, deploymentID, sourceID)
	if err != nil || active.ID != attachedB.ID || active.APIContractID != contractB.ID {
		t.Fatalf("active contract source = %#v, err=%v", active, err)
	}

	startedAt := time.Now().UTC()
	if _, err := memory.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: runID, DeploymentID: deploymentID, OrganisationID: "org_acme", AssetKind: model.DeveloperAssetContract,
		TargetID: contractB.ID, TargetKey: "contract:" + contractB.ID, SourceID: sourceID,
		State: model.DeveloperAssetIngestionReviewReady, Attempt: 1,
		Versions:    model.ProcessorVersions{Pipeline: "pipeline-v1", Parser: "openapi-v1", Normalizer: "normalizer-v1", Mapper: "mapper-v1"},
		RawManifest: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`{}`), StartedAt: &startedAt,
	}); err != nil {
		t.Fatalf("create contract ingestion run: %v", err)
	}
	candidateHash := developerAssetTestHash("c")
	if _, err := memory.CreateAPIContractCandidate(ctx, APIContractCandidateRecord{
		Candidate: model.APIContractCandidate{
			ID: candidateID, DeploymentID: deploymentID, APIContractID: contractB.ID, IngestionRunID: runID,
			OpenAPIVersion: "3.1.0", SourceFormat: "json", NormalizedContract: json.RawMessage(`{"openapi":"3.1.0"}`),
			SourceHash: developerAssetTestHash("d"), ContentHash: candidateHash, ValidationResult: json.RawMessage(`{}`),
			ParserVersion: "openapi-v1", Visibility: model.VisibilityPrivate, Diagnostics: json.RawMessage(`{}`),
		},
		Operations: []model.APIContractOperation{{
			ID: "contract-operation", APIContractCandidateID: candidateID, OperationKey: "GET /customers", Method: "GET",
			PathTemplate: "/customers", Tags: []string{}, Security: json.RawMessage(`{}`), RequestSchemaRefs: []string{},
			ResponseSchemaRefs: []string{}, ContentHash: developerAssetTestHash("e"),
		}},
	}); err != nil {
		t.Fatalf("create contract candidate: %v", err)
	}
	reviewedAt := time.Now().UTC()
	revision := model.APIContractRevision{
		ID: revisionID, DeploymentID: deploymentID, APIContractID: contractB.ID, APIContractCandidateID: candidateID,
		ContentHash: candidateHash, Visibility: model.VisibilityPrivate, ReviewedBy: "reviewer", ReviewedAt: reviewedAt,
	}
	if _, _, err := memory.PublishAPIContractCandidate(ctx, contractB, contractB.Revision, revision, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("contract publication without source evidence error = %v, want ErrConflict", err)
	}
	evidence := &model.APIContractRevisionSourcePublication{
		APIContractRevisionID: revisionID, DeploymentID: deploymentID, APIContractCandidateID: candidateID,
		SourcePublicationID: "pub_api_seed", ContentHash: developerAssetTestHash("2"),
	}
	updatedContract, publishedRevision, err := memory.PublishAPIContractCandidate(ctx, contractB, contractB.Revision, revision, evidence)
	if err != nil {
		t.Fatalf("publish contract candidate: %v", err)
	}
	if updatedContract.Revision != contractB.Revision+1 || publishedRevision.Revision != 1 {
		t.Fatalf("published contract=%#v revision=%#v", updatedContract, publishedRevision)
	}
	if publishedRevision.APIContractName != contractB.Name || publishedRevision.APIContractSlug != contractB.Slug ||
		publishedRevision.APIContractDescription != contractB.Description || publishedRevision.APIContractKind != contractB.Kind {
		t.Fatalf("contract revision did not snapshot root identity: %#v", publishedRevision)
	}
	run, err := memory.DeveloperAssetIngestionRun(ctx, deploymentID, runID)
	if err != nil || run.State != model.DeveloperAssetIngestionPublished {
		t.Fatalf("contract run = %#v, err=%v", run, err)
	}
}
