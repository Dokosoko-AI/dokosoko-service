package platform_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func developerAssetTestHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func developerAssetTestInt(value int) *int { return &value }

func TestBuildDeveloperAssetSearchIndexGlobalDocumentationIsDeterministic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	const deploymentID = "prod_acme"

	run, err := memory.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: "00000000-0000-4000-8000-000000000101", DeploymentID: deploymentID, OrganisationID: "org_acme",
		AssetKind: model.DeveloperAssetDocumentation, TargetID: "src_docs", TargetKey: "src_docs", SourceID: "src_docs",
		State:    model.DeveloperAssetIngestionPublished,
		Versions: model.ProcessorVersions{Pipeline: "test-v1", Parser: "test-v1", Normalizer: "test-v1", Mapper: "test-v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	documentHash := developerAssetTestHash("global-document")
	sectionHash := developerAssetTestHash("global-section")
	document := model.DocumentationDocument{
		ID: "00000000-0000-4000-8000-000000000102", DeploymentID: deploymentID, IngestionRunID: run.ID,
		SourcePath: "guides/authentication.md", CanonicalURL: "https://docs.example.test/authentication",
		Title: "Authentication", Kind: "guide", Language: "en", MediaType: "text/markdown",
		NormalizedMarkdown: "# Authentication\n\nCreate an exact API token.", ContentHash: documentHash,
		Visibility: model.VisibilityPrivate, Metadata: json.RawMessage(`{"audience":"developers"}`),
	}
	section := model.DocumentationSection{
		ID: "00000000-0000-4000-8000-000000000103", DeploymentID: deploymentID,
		DocumentationDocumentID: document.ID, Ordinal: 0, HeadingLevel: 2, Heading: "Create a token", Anchor: "create-token",
		Breadcrumb: []string{"Authentication", "Create a token"}, ContentKind: "prose",
		NormalizedText: "Create the token in developer settings and store it server-side.", TokenEstimate: 14,
		ContentHash: sectionHash, Metadata: json.RawMessage(`{"topic":"auth"}`),
	}
	if err := memory.SaveDocumentationIngestionOutput(ctx, deploymentID, store.DocumentationIngestionOutput{
		Documents: []model.DocumentationDocument{document}, Sections: []model.DocumentationSection{section},
	}); err != nil {
		t.Fatal(err)
	}

	revisionID := "00000000-0000-4000-8000-000000000104"
	collectionID := "00000000-0000-4000-8000-000000000105"
	mapHash := developerAssetTestHash("global-map")
	revisionHash := developerAssetTestHash("global-revision")
	_, err = memory.CreateDocumentationCollection(ctx, model.DocumentationCollection{
		ID: collectionID, DeploymentID: deploymentID, OrganisationID: "org_acme", Name: "Platform guides", Slug: "platform-guides",
		Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, store.DocumentationCollectionRevisionRecord{
		Revision: model.DocumentationCollectionRevision{
			ID: revisionID, DeploymentID: deploymentID, DocumentationCollectionID: collectionID, Revision: 1,
			Visibility: model.VisibilityPrivate, ContentHash: revisionHash, SelectionManifest: json.RawMessage(`[]`),
		},
		Members: []model.DocumentationCollectionMember{{
			ID: "00000000-0000-4000-8000-000000000106", DocumentationCollectionRevisionID: revisionID,
			Kind: "document", DocumentationDocumentID: document.ID, Ordinal: 0, IncludeDescendants: true, Selector: json.RawMessage(`{}`),
		}},
		Map: &model.DocumentationMap{
			ID: "00000000-0000-4000-8000-000000000107", DeploymentID: deploymentID,
			DocumentationCollectionRevisionID: revisionID, MapVersion: "documentation-map-v1",
			Map: model.DocumentationMapBody{Overview: "Reviewed platform guides."}, AgentMarkdown: "# Platform guides\n\nReviewed platform guides.",
			ContentHash: mapHash, Visibility: model.VisibilityPrivate,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	publicationID := "00000000-0000-4000-8000-000000000108"
	publication, err := memory.PublishDeploymentDocumentation(ctx, model.DeploymentDocumentationPublication{
		ID: publicationID, DeploymentID: deploymentID, Revision: 1, Visibility: model.VisibilityPrivate,
		SnapshotSchemaVersion: "developer-assets-v1", SnapshotHash: developerAssetTestHash("global-publication"),
		Members: []model.DeploymentDocumentationPublicationMember{{
			DocumentationCollectionRevisionID: revisionID, Ordinal: 0, ContentHash: revisionHash, Visibility: model.VisibilityPrivate,
		}}, PublishedBy: "test",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.BuildDeveloperAssetSearchIndex(ctx, "global_documentation", publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.BuildDeveloperAssetSearchIndex(ctx, "global_documentation", publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.ContentHash != second.ContentHash || first.State != "ready" {
		t.Fatalf("generations are not deterministic/idempotent: first=%#v second=%#v", first, second)
	}
	if first.BuilderVersion != platform.DeveloperAssetIndexBuilderVersion || first.RetrievalProfileVersion != platform.DeveloperAssetRetrievalProfileVersion {
		t.Fatalf("generation versions = %q / %q", first.BuilderVersion, first.RetrievalProfileVersion)
	}
	if _, readyErr := service.ReadyDeploymentDocumentationPublication(ctx); !errors.Is(readyErr, store.ErrNotFound) {
		t.Fatalf("index without a durable activation audit became discoverable: %v", readyErr)
	}
	if err := service.ActivateDeveloperAssetPublication(ctx, "global_documentation", publication.ID, platform.Actor{ID: "activation-test"}); err != nil {
		t.Fatal(err)
	}
	readyPublication, err := service.ReadyDeploymentDocumentationPublication(ctx)
	if err != nil || readyPublication.ID != publication.ID {
		t.Fatalf("audited publication did not become ready: %#v err=%v", readyPublication, err)
	}
	record, err := memory.SearchIndexGeneration(ctx, deploymentID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Units) != 3 || len(record.APIScopes) != 0 {
		t.Fatalf("global record has %d units and %d API scopes", len(record.Units), len(record.APIScopes))
	}
	wantHashes := map[string]bool{mapHash: true, documentHash: true, sectionHash: true}
	for ordinal, unit := range record.Units {
		if unit.Ordinal != ordinal || !wantHashes[unit.ContentHash] || len(unit.Embedding) != 384 {
			t.Fatalf("global unit %d = %#v (embedding dimensions %d)", ordinal, unit, len(unit.Embedding))
		}
		delete(wantHashes, unit.ContentHash)
	}
	if len(wantHashes) != 0 {
		t.Fatalf("missing exact source hashes: %#v", wantHashes)
	}
}

func TestBuildDeveloperAssetSearchIndexPersistsFailedGeneration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	const deploymentID = "prod_acme"
	revisionID := "00000000-0000-4000-8000-000000000121"
	revisionHash := developerAssetTestHash("missing-map-revision")
	_, err := memory.CreateDocumentationCollection(ctx, model.DocumentationCollection{
		ID: "00000000-0000-4000-8000-000000000122", DeploymentID: deploymentID, OrganisationID: "org_acme",
		Name: "Missing map", Slug: "missing-map", Visibility: model.VisibilityPrivate,
	}, store.DocumentationCollectionRevisionRecord{Revision: model.DocumentationCollectionRevision{
		ID: revisionID, DeploymentID: deploymentID, DocumentationCollectionID: "00000000-0000-4000-8000-000000000122",
		Revision: 1, Visibility: model.VisibilityPrivate, ContentHash: revisionHash, SelectionManifest: json.RawMessage(`[]`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	publication, err := memory.PublishDeploymentDocumentation(ctx, model.DeploymentDocumentationPublication{
		ID: "00000000-0000-4000-8000-000000000123", DeploymentID: deploymentID, Revision: 1,
		Visibility: model.VisibilityPrivate, SnapshotSchemaVersion: "developer-assets-v1",
		SnapshotHash: developerAssetTestHash("missing-map-publication"),
		Members: []model.DeploymentDocumentationPublicationMember{{
			DocumentationCollectionRevisionID: revisionID, ContentHash: revisionHash, Visibility: model.VisibilityPrivate,
		}},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.BuildDeveloperAssetSearchIndex(ctx, "global_documentation", publication.ID); err == nil {
		t.Fatal("index build succeeded without the publication's exact Documentation Map")
	}
	generations, err := memory.SearchIndexGenerations(ctx, deploymentID, "global_documentation", publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 1 || generations[0].State != "failed" || len(generations[0].Diagnostics) == 0 {
		t.Fatalf("failed generation was not persisted cleanly: %#v", generations)
	}
}

func TestBuildDeveloperAssetSearchIndexAPISDKUsesExactReleaseAndApprovedSamples(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "developer-asset-test"}
	const deploymentID = "prod_acme"

	api, err := service.CreateIntegration(ctx, platform.IntegrationInput{
		FamilyKey: "indexed-api", VersionKey: "v1", DisplayName: "Indexed API", Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	revision, err := memory.CreateIntegrationRevision(ctx, model.IntegrationRevision{
		ID: "00000000-0000-4000-8000-000000000201", IntegrationID: api.ID, Revision: 1, State: "published",
		Snapshot: json.RawMessage(`{}`), ManifestHash: developerAssetTestHash("api-revision"), PublishedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	packageValue, err := service.SaveSDKPackage(ctx, "", platform.SDKPackageInput{
		Ecosystem: "npm", Coordinate: "@example/indexed-sdk", Name: "Indexed JavaScript SDK",
		Language: "javascript", Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	release, err := service.CreateSDKRelease(ctx, packageValue.ID, platform.SDKReleaseInput{
		ExactVersion: "3.7.11", Visibility: model.VisibilityPrivate,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	run, err := memory.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: "00000000-0000-4000-8000-000000000202", DeploymentID: deploymentID, OrganisationID: "org_acme",
		AssetKind: model.DeveloperAssetSDK, TargetID: release.ID, TargetKey: release.ID,
		State:    model.DeveloperAssetIngestionReviewReady,
		Versions: model.ProcessorVersions{Pipeline: "test-v1", Parser: "test-v1", Normalizer: "test-v1", Mapper: "test-v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	candidateID := "00000000-0000-4000-8000-000000000203"
	fileID := "00000000-0000-4000-8000-000000000204"
	sectionID := "00000000-0000-4000-8000-000000000205"
	symbolID := "00000000-0000-4000-8000-000000000206"
	approvedSampleID := "00000000-0000-4000-8000-000000000207"
	excludedSampleID := "00000000-0000-4000-8000-000000000208"
	candidateMapID := "00000000-0000-4000-8000-000000000219"
	fileHash, sectionHash := developerAssetTestHash("sdk-file"), developerAssetTestHash("sdk-section")
	symbolHash, approvedHash := developerAssetTestHash("sdk-symbol"), developerAssetTestHash("sdk-approved-sample")
	excludedHash, mapHash := developerAssetTestHash("sdk-excluded-sample"), developerAssetTestHash("sdk-map")
	candidateHash := developerAssetTestHash("sdk-candidate")
	candidate := store.SDKContentCandidateRecord{
		Candidate: model.SDKContentCandidate{
			ID: candidateID, DeploymentID: deploymentID, SDKReleaseID: release.ID, IngestionRunID: run.ID,
			ContentHash: candidateHash, Visibility: model.VisibilityPrivate,
		},
		Files: []model.SDKPublicationFile{{
			ID: fileID, SDKContentCandidateID: candidateID, SourcePath: "src/client.ts", Role: "source",
			MediaType: "text/typescript", Language: "typescript", ContentHash: fileHash, Metadata: json.RawMessage(`{"module":"client"}`),
		}},
		Sections: []model.SDKSection{{
			ID: sectionID, SDKContentCandidateID: candidateID, SDKPublicationFileID: fileID,
			Ordinal: 0, Heading: "Create a client", Anchor: "create-client", Breadcrumb: []string{"Client", "Create a client"},
			ContentKind: "prose", NormalizedText: "Create the version 3.7.11 client with an API token.",
			ContentHash: sectionHash, Metadata: json.RawMessage(`{"topic":"initialization"}`),
		}},
		Symbols: []model.SDKSymbol{{
			ID: symbolID, SDKContentCandidateID: candidateID, SDKPublicationFileID: fileID, SDKSectionID: sectionID,
			Language: "typescript", Kind: "class", QualifiedName: "sdk.Client", DisplayName: "Client",
			Signature: "new Client(token: string)", Documentation: "Creates an exact-version client.",
			Identifiers: []string{"Client"}, ContentHash: symbolHash, Metadata: json.RawMessage(`{}`),
		}},
		Samples: []model.SDKCodeSample{
			{
				ID: approvedSampleID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
				SDKPublicationFileID: fileID, SDKSectionID: sectionID, Language: "typescript", Title: "Initialize the client",
				Intent: "Create a client", Code: "const client = new Client(process.env.API_TOKEN!);", Imports: []string{"Client"},
				Origin: model.SDKSampleCurated, ValidationStatus: model.SDKSampleSyntaxChecked, ValidationEvidence: json.RawMessage(`{"passed":true,"validator":"test/parser"}`),
				Visibility: model.VisibilityPrivate, ContentHash: approvedHash,
			},
			{
				ID: excludedSampleID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
				SDKPublicationFileID: fileID, SDKSectionID: sectionID, Language: "typescript", Title: "Unsafe placeholder",
				Intent: "Demonstrate an excluded sample", Code: "const token = 'secret';", Origin: model.SDKSampleCurated,
				ValidationStatus: model.SDKSampleSyntaxChecked, ValidationEvidence: json.RawMessage(`{"passed":true,"validator":"test/parser"}`),
				Visibility: model.VisibilityPrivate, ContentHash: excludedHash,
			},
		},
		Map: &model.SDKMap{
			ID: candidateMapID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID, MapVersion: "sdk-candidate-map-v1",
			Map:           model.SDKMapBody{Overview: "Exact 3.7.11 SDK map.", Samples: []model.KnowledgeMapEntry{{ID: excludedSampleID, Kind: "code_sample", Title: "Unsafe placeholder"}}},
			AgentMarkdown: "# Candidate SDK 3.7.11\n\n- Unsafe placeholder", ContentHash: mapHash,
		},
	}
	if _, err := memory.CreateSDKContentCandidate(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	publicationID := "00000000-0000-4000-8000-000000000210"
	sdkPublicationRecord := store.SDKContentPublicationRecord{
		Publication: model.SDKContentPublication{
			ID: publicationID, DeploymentID: deploymentID, SDKReleaseID: release.ID,
			SDKContentCandidateID: candidateID, ContentHash: candidateHash, Visibility: model.VisibilityPrivate,
		},
		FileSelections: []model.SDKContentPublicationFileSelection{{
			SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
			SDKPublicationFileID: fileID, Decision: "included", Ordinal: developerAssetTestInt(0), ContentHash: fileHash,
		}},
		SampleSelections: []model.SDKContentPublicationSampleSelection{
			{
				SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
				SDKCodeSampleID: approvedSampleID, Decision: "approved", Ordinal: developerAssetTestInt(0), ContentHash: approvedHash,
			},
			{
				SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
				SDKCodeSampleID: excludedSampleID, Decision: "excluded", Reason: "unsafe literal", ContentHash: excludedHash,
			},
		},
	}
	publishedMap, err := store.BuildReviewedSDKPublicationMap(packageValue, release, candidate, sdkPublicationRecord)
	if err != nil {
		t.Fatal(err)
	}
	sdkPublicationRecord.PublishedMap = publishedMap
	sdkPublicationRecord.Map = &model.SDKContentPublicationMap{
		SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
		SDKMapID: publishedMap.ID, ContentHash: publishedMap.ContentHash,
	}
	sdkPublication, err := memory.PublishSDKContentCandidate(ctx, sdkPublicationRecord)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := service.SaveAPISDKBinding(ctx, api.ID, "", platform.APISDKBindingInput{
		SDKPackageID: packageValue.ID, SDKReleaseID: release.ID, SDKContentPublicationID: sdkPublication.ID,
		State: "ready", Selector: json.RawMessage(`{}`), Visibility: model.VisibilityPrivate,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	apiPublication, err := memory.CreateAPIDeveloperAssetPublication(ctx, model.APIDeveloperAssetPublication{
		ID: "00000000-0000-4000-8000-000000000211", DeploymentID: deploymentID, APIID: api.ID, APIRevisionID: revision.ID,
		SnapshotSchemaVersion: "developer-assets-v1", SnapshotHash: developerAssetTestHash("api-publication"),
		SDKs: []model.APIPublicationSDKAsset{{
			BindingID: binding.ID, SDKPackageID: packageValue.ID, SDKReleaseID: release.ID,
			SDKPackageEcosystem: packageValue.Ecosystem, SDKPackageCoordinate: packageValue.CanonicalCoordinate,
			SDKPackageDisplayCoordinate: packageValue.DisplayCoordinate, SDKPackageDisplayName: packageValue.Name,
			SDKPackageLanguage: packageValue.Language, SDKPackagePlatform: packageValue.Platform,
			SDKContentPublicationID: sdkPublication.ID, Selector: binding.Selector, SelectorHash: binding.SelectorHash,
			ContentHash: sdkPublication.ContentHash, Visibility: model.VisibilityPrivate, Ordinal: 0,
		}}, PublishedBy: actor.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	mutatedPackage := packageValue
	mutatedPackage.Name = "Renamed TypeScript SDK"
	mutatedPackage.Language = "typescript"
	mutatedPackage.Platform = "browser"
	if _, err := memory.SaveSDKPackage(ctx, mutatedPackage, packageValue.Revision); err != nil {
		t.Fatal(err)
	}

	generation, err := service.BuildDeveloperAssetSearchIndex(ctx, "api", apiPublication.ID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := memory.SearchIndexGeneration(ctx, deploymentID, generation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Units) != 4 || len(record.APIScopes) != len(record.Units) {
		t.Fatalf("API generation has %d units and %d scopes", len(record.Units), len(record.APIScopes))
	}
	seenKinds := make(map[string]bool)
	sawSnapshotTitle := false
	for _, unit := range record.Units {
		if unit.SourceEntityID == excludedSampleID || unit.ContentHash == excludedHash || strings.Contains(unit.Content, "Unsafe placeholder") {
			t.Fatalf("excluded sample was indexed: %#v", unit)
		}
		seenKinds[unit.Kind] = true
		var metadata map[string]any
		if err := json.Unmarshal(unit.Metadata, &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata["exact_version"] != release.ExactVersion || metadata["sdk_release_id"] != release.ID {
			t.Fatalf("unit does not carry exact SDK release metadata: %#v", metadata)
		}
		if strings.Contains(unit.Title, mutatedPackage.Name) || strings.Contains(strings.Join(unit.Breadcrumb, " / "), mutatedPackage.Name) {
			t.Fatalf("unit was rendered from mutable SDK package metadata: %#v", unit)
		}
		if unit.Kind == "map" {
			if unit.Language != packageValue.Language {
				t.Fatalf("SDK map used mutable package language: %#v", unit)
			}
			sawSnapshotTitle = strings.Contains(unit.Title, packageValue.Name)
		}
	}
	if !sawSnapshotTitle {
		t.Fatal("SDK index did not preserve the package display-name snapshot")
	}
	for _, kind := range []string{"map", "sdk_section", "sdk_symbol", "sdk_sample"} {
		if !seenKinds[kind] {
			t.Fatalf("missing %s unit: %#v", kind, seenKinds)
		}
	}
	for _, scope := range record.APIScopes {
		if scope.APIID != api.ID || scope.APISDKBindingID != binding.ID || scope.ScopeKind != "attached" || scope.SelectorHash != binding.SelectorHash {
			t.Fatalf("API scope = %#v", scope)
		}
	}
	second, err := service.BuildDeveloperAssetSearchIndex(ctx, "api", apiPublication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != generation.ID || second.ContentHash != generation.ContentHash {
		t.Fatalf("ready API generation was not reused: %#v / %#v", generation, second)
	}
}
