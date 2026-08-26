package platform

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type documentationCollectionMapStore struct {
	store.Store
	publication model.SourcePublication
	review      store.SourcePublicationDocumentationReview
}

func (s *documentationCollectionMapStore) SourcePublication(_ context.Context, deploymentID, publicationID string) (model.SourcePublication, error) {
	if deploymentID != s.publication.ProductID || publicationID != s.publication.ID {
		return model.SourcePublication{}, store.ErrNotFound
	}
	return s.publication, nil
}

func (s *documentationCollectionMapStore) SourcePublications(_ context.Context, deploymentID, sourceID string) ([]model.SourcePublication, error) {
	if deploymentID != s.publication.ProductID || sourceID != s.publication.SourceID {
		return nil, store.ErrNotFound
	}
	return []model.SourcePublication{s.publication}, nil
}

func (s *documentationCollectionMapStore) SourcePublicationDocumentationReview(_ context.Context, deploymentID, publicationID string) (store.SourcePublicationDocumentationReview, error) {
	if deploymentID != s.publication.ProductID || publicationID != s.publication.ID {
		return store.SourcePublicationDocumentationReview{}, store.ErrNotFound
	}
	return s.review, nil
}

func documentationCollectionMapHash(digit string) string {
	return "sha256:" + strings.Repeat(digit, 64)
}

func TestSaveDocumentationCollectionMergesOnlyExactReviewedMapEvidenceDeterministically(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	now := time.Now().UTC()
	run, err := memory.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: "10000000-0000-4000-8000-000000000001", DeploymentID: "prod_acme", OrganisationID: "org_acme",
		AssetKind: model.DeveloperAssetDocumentation, TargetID: "src_docs", TargetKey: "source:src_docs", SourceID: "src_docs",
		State: model.DeveloperAssetIngestionReviewReady, Attempt: 1, QueuedAt: now,
		Versions:    model.ProcessorVersions{Pipeline: "pipeline-v1", Parser: "parser-v1", Normalizer: "normalizer-v1", Mapper: "mapper-v1"},
		RawManifest: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	documentA := model.DocumentationDocument{
		ID: "10000000-0000-4000-8000-000000000002", DeploymentID: "prod_acme", IngestionRunID: run.ID,
		SourcePath: "a.md", Title: "Reviewed A", Kind: "guide", Language: "en", MediaType: "text/markdown",
		NormalizedMarkdown: "# Reviewed A", ContentHash: documentationCollectionMapHash("1"), Visibility: model.VisibilityPrivate, Metadata: json.RawMessage(`{}`),
	}
	documentB := model.DocumentationDocument{
		ID: "10000000-0000-4000-8000-000000000003", DeploymentID: "prod_acme", IngestionRunID: run.ID,
		SourcePath: "b.md", Title: "Excluded B", Kind: "guide", Language: "en", MediaType: "text/markdown",
		NormalizedMarkdown: "# Excluded B", ContentHash: documentationCollectionMapHash("2"), Visibility: model.VisibilityPrivate, Ordinal: 1, Metadata: json.RawMessage(`{}`),
	}
	sectionAuth := model.DocumentationSection{
		ID: "10000000-0000-4000-8000-000000000004", DeploymentID: "prod_acme", DocumentationDocumentID: documentA.ID,
		Ordinal: 0, HeadingLevel: 2, Heading: "Authentication", ContentKind: "prose", NormalizedText: "Authenticate.", TokenEstimate: 2,
		ContentHash: documentationCollectionMapHash("3"), Breadcrumb: []string{"Reviewed A", "Authentication"}, Metadata: json.RawMessage(`{}`),
	}
	sectionErrors := model.DocumentationSection{
		ID: "10000000-0000-4000-8000-000000000005", DeploymentID: "prod_acme", DocumentationDocumentID: documentA.ID,
		ParentSectionID: sectionAuth.ID, Ordinal: 1, HeadingLevel: 3, Heading: "Errors", ContentKind: "prose", NormalizedText: "Handle errors.", TokenEstimate: 2,
		ContentHash: documentationCollectionMapHash("4"), Breadcrumb: []string{"Reviewed A", "Authentication", "Errors"}, Metadata: json.RawMessage(`{}`),
	}
	sectionExcluded := model.DocumentationSection{
		ID: "10000000-0000-4000-8000-000000000006", DeploymentID: "prod_acme", DocumentationDocumentID: documentB.ID,
		Ordinal: 0, HeadingLevel: 2, Heading: "Excluded secret", ContentKind: "prose", NormalizedText: "Excluded.", TokenEstimate: 1,
		ContentHash: documentationCollectionMapHash("5"), Breadcrumb: []string{"Excluded B"}, Metadata: json.RawMessage(`{}`),
	}
	entry := func(id, kind, title string, aliases ...string) model.KnowledgeMapEntry {
		return model.KnowledgeMapEntry{ID: id, Kind: kind, Title: title, Summary: title + " summary", Aliases: aliases}
	}
	mapID, mapHash := "10000000-0000-4000-8000-000000000007", documentationCollectionMapHash("6")
	persistedMap := &model.DocumentationMap{
		ID: mapID, DeploymentID: "prod_acme", IngestionRunID: run.ID, MapVersion: "documentation-map-v1",
		Map: model.DocumentationMapBody{
			Overview: "Two normalized documents.",
			Documents: []model.KnowledgeMapEntry{
				{ID: documentA.ID, Kind: "document", Title: documentA.Title, Summary: "Reviewed document.", Children: []model.KnowledgeMapEntry{{ID: sectionAuth.ID, Kind: "section", Title: sectionAuth.Heading, Summary: "Auth.", Children: []model.KnowledgeMapEntry{entry(sectionErrors.ID, "section", sectionErrors.Heading)}}}},
				{ID: documentB.ID, Kind: "document", Title: documentB.Title, Summary: "Excluded document.", Children: []model.KnowledgeMapEntry{entry(sectionExcluded.ID, "section", sectionExcluded.Heading)}},
			},
			Topics:         []model.KnowledgeMapEntry{entry("topic-shared", "topic", "Shared", documentB.ID, documentA.ID), entry("topic-excluded", "topic", "Excluded topic", documentB.ID)},
			Workflows:      []model.KnowledgeMapEntry{entry(sectionAuth.ID, "setup", "Setup"), entry(sectionExcluded.ID, "setup", "Excluded setup")},
			Authentication: []model.KnowledgeMapEntry{entry(sectionAuth.ID, "authentication", "Authentication")},
			Errors:         []model.KnowledgeMapEntry{entry(sectionErrors.ID, "errors", "Errors"), entry(sectionExcluded.ID, "errors", "Excluded errors")},
			Examples:       []model.KnowledgeMapEntry{entry(sectionErrors.ID, "examples", "Example")},
			Versions:       []string{"v2", "v1"}, Languages: []string{"typescript", "go", "go"},
			Gaps: []model.KnowledgeMapGap{
				{Kind: "reviewed-gap", Description: "Reviewed gap", EvidenceIDs: []string{sectionAuth.ID}},
				{Kind: "excluded-gap", Description: "Excluded gap", EvidenceIDs: []string{sectionExcluded.ID}},
			},
			QualityWarnings: []string{"z warning", "a warning", "a warning"}, ExcludedSourceIDs: []string{"crawler-duplicate"},
		},
		AgentMarkdown: "# Source map\n", ContentHash: mapHash, Visibility: model.VisibilityPrivate, CreatedAt: now,
	}
	if err := memory.SaveDocumentationIngestionOutput(ctx, "prod_acme", store.DocumentationIngestionOutput{
		Documents: []model.DocumentationDocument{documentA, documentB},
		Sections:  []model.DocumentationSection{sectionAuth, sectionErrors, sectionExcluded}, Map: persistedMap,
	}); err != nil {
		t.Fatal(err)
	}
	ordinal := 0
	publication := model.SourcePublication{
		ID: "10000000-0000-4000-8000-000000000008", OrganisationID: "org_acme", ProductID: "prod_acme", SourceID: "src_docs",
		Visibility: model.VisibilityPrivate, ContentHash: documentationCollectionMapHash("7"), DocumentCount: 1, ReviewedBy: "reviewer", ReviewedAt: now, PublishedAt: now,
	}
	fixture := &documentationCollectionMapStore{Store: memory, publication: publication, review: store.SourcePublicationDocumentationReview{
		Selections: []model.SourcePublicationDocumentSelection{
			{SourcePublicationID: publication.ID, DeploymentID: "prod_acme", DocumentationDocumentID: documentA.ID, Decision: "included", Ordinal: &ordinal, ContentHash: documentA.ContentHash, ReviewedBy: "reviewer", ReviewedAt: now},
			{SourcePublicationID: publication.ID, DeploymentID: "prod_acme", DocumentationDocumentID: documentB.ID, Decision: "excluded", Reason: "outside review boundary", ContentHash: documentB.ContentHash, ReviewedBy: "reviewer", ReviewedAt: now},
		},
		MapLink: &model.SourcePublicationDocumentationMap{SourcePublicationID: publication.ID, DeploymentID: "prod_acme", DocumentationMapID: mapID, ContentHash: mapHash},
	}}
	service := New(fixture)
	members := []DocumentationCollectionMemberInput{
		{Kind: "source_publication", ID: publication.ID, IncludeDescendants: true},
		{Kind: "document", ID: documentA.ID},
		{Kind: "section", ID: sectionAuth.ID, IncludeDescendants: true},
	}
	create := func(slug string) store.DocumentationCollectionRevisionRecord {
		t.Helper()
		collection, err := service.SaveDocumentationCollection(ctx, "", DocumentationCollectionInput{
			Name: "Reviewed guides", Slug: slug, Description: "Exact reviewed documentation.", Visibility: model.VisibilityPrivate,
			Members: members, AcknowledgeReviewed: true,
		}, Actor{ID: "reviewer"})
		if err != nil {
			t.Fatal(err)
		}
		revisions, err := memory.DocumentationCollectionRevisions(ctx, "prod_acme", collection.ID)
		if err != nil || len(revisions) != 1 {
			t.Fatalf("collection revisions = %#v, err=%v", revisions, err)
		}
		record, err := memory.DocumentationCollectionRevision(ctx, "prod_acme", revisions[0].ID)
		if err != nil || record.Map == nil {
			t.Fatalf("collection revision record = %#v, err=%v", record, err)
		}
		return record
	}
	first, second := create("reviewed-guides-a"), create("reviewed-guides-b")
	if !reflect.DeepEqual(first.Map.Map, second.Map.Map) || first.Map.ContentHash != second.Map.ContentHash || first.Map.AgentMarkdown != second.Map.AgentMarkdown {
		t.Fatalf("collection map merge was not deterministic:\nfirst=%#v\nsecond=%#v", first.Map, second.Map)
	}
	body := first.Map.Map
	if len(body.Documents) != 3 || body.Documents[0].ID != "source-publication:"+publication.ID || body.Documents[1].ID != "document:"+documentA.ID || body.Documents[2].ID != "section:"+sectionAuth.ID {
		t.Fatalf("exact member evidence was not preserved: %#v", body.Documents)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	mapJSON := string(encoded)
	for _, required := range []string{"topic-shared", sectionAuth.ID, sectionErrors.ID, "typescript", "reviewed-gap", "a warning", "crawler-duplicate", documentB.ID} {
		if !strings.Contains(mapJSON, required) {
			t.Errorf("merged map is missing %q: %s", required, mapJSON)
		}
	}
	for _, excluded := range []string{"topic-excluded", "Excluded setup", "Excluded errors", "excluded-gap", sectionExcluded.ID} {
		if strings.Contains(mapJSON, excluded) {
			t.Errorf("merged map crossed the reviewed boundary with %q: %s", excluded, mapJSON)
		}
	}
	if len(body.Topics) != 1 || !reflect.DeepEqual(body.Topics[0].Aliases, []string{documentA.ID}) || len(body.Authentication) != 1 || len(body.Errors) != 1 || len(body.Examples) != 1 || !reflect.DeepEqual(body.Languages, []string{"go", "typescript"}) || !reflect.DeepEqual(body.QualityWarnings, []string{"a warning", "z warning"}) {
		t.Fatalf("merged documentation map fields = %#v", body)
	}

	selector, err := json.Marshal(map[string]any{
		"include_map": true,
		"section_ids": []string{sectionAuth.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	selectedCollection, err := service.SaveDocumentationCollection(ctx, "", DocumentationCollectionInput{
		Name: "Selected authentication", Slug: "selected-authentication", Description: "Only selected authentication evidence.", Visibility: model.VisibilityPrivate,
		Members: []DocumentationCollectionMemberInput{{
			Kind: "source_publication", ID: publication.ID, IncludeDescendants: true, Selector: selector,
		}},
		AcknowledgeReviewed: true,
	}, Actor{ID: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	selectedRevisions, err := memory.DocumentationCollectionRevisions(ctx, "prod_acme", selectedCollection.ID)
	if err != nil || len(selectedRevisions) != 1 {
		t.Fatalf("selected collection revisions = %#v, err=%v", selectedRevisions, err)
	}
	selectedRecord, err := memory.DocumentationCollectionRevision(ctx, "prod_acme", selectedRevisions[0].ID)
	if err != nil || selectedRecord.Map == nil {
		t.Fatalf("selected collection record = %#v, err=%v", selectedRecord, err)
	}
	selectedMapJSON, err := json.Marshal(selectedRecord.Map.Map)
	if err != nil {
		t.Fatal(err)
	}
	selectedMapText := string(selectedMapJSON)
	if !strings.Contains(selectedMapText, sectionAuth.ID) || !strings.Contains(selectedMapText, documentA.ID) {
		t.Fatalf("selected map lost the chosen section or its routing parent: %s", selectedMapText)
	}
	for _, forbidden := range []string{sectionErrors.ID, sectionExcluded.ID, documentB.ID, "Excluded setup", "typescript", "v2", "z warning", "crawler-duplicate"} {
		if strings.Contains(selectedMapText, forbidden) {
			t.Errorf("member selector leaked %q into the persisted collection map: %s", forbidden, selectedMapText)
		}
	}
	if len(selectedRecord.Map.Map.Versions) != 0 || len(selectedRecord.Map.Map.Languages) != 0 || len(selectedRecord.Map.Map.QualityWarnings) != 0 || len(selectedRecord.Map.Map.ExcludedSourceIDs) != 0 {
		t.Fatalf("selected map retained unattributed map-level fields: %#v", selectedRecord.Map.Map)
	}

	outerSelector, _, err := parseDeveloperAssetSelector(selector, developerAssetDocumentationSelector)
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := service.buildDocumentationRevisionIndex(ctx, "prod_acme", first, developerAssetDocumentationBuildOptions{
		visibility:             model.VisibilityPrivate,
		outerSelector:          outerSelector,
		outerSelectorHash:      documentationCollectionMapHash("8"),
		wrapperPublicationKind: "api",
		wrapperPublicationID:   "10000000-0000-4000-8000-000000000009",
		apiID:                  "10000000-0000-4000-8000-000000000010",
		bindingID:              "10000000-0000-4000-8000-000000000011",
	})
	if err != nil {
		t.Fatal(err)
	}
	var projectedMap *model.KnowledgeUnit
	for index := range drafts {
		if drafts[index].unit.Kind == "map" {
			value := drafts[index].unit
			projectedMap = &value
		}
	}
	if projectedMap == nil {
		t.Fatal("restrictive API selector with include_map=true did not create a map projection")
	}
	if projectedMap.SourceEntityID == first.Map.ID || projectedMap.ContentHash == first.Map.ContentHash {
		t.Fatalf("selected map reused the unscoped entity identity/hash: %#v", projectedMap)
	}
	if !strings.Contains(projectedMap.Content, sectionAuth.ID) {
		t.Fatalf("selected index map lost its chosen section: %s", projectedMap.Content)
	}
	for _, forbidden := range []string{sectionErrors.ID, sectionExcluded.ID, documentB.ID, "Excluded setup", "typescript", "v2", "z warning", "crawler-duplicate"} {
		if strings.Contains(projectedMap.Content, forbidden) {
			t.Errorf("API selector leaked %q into the indexed map projection: %s", forbidden, projectedMap.Content)
		}
	}
	var citation map[string]any
	if err := json.Unmarshal(projectedMap.Citation, &citation); err != nil {
		t.Fatal(err)
	}
	if citation["documentation_map_id"] != first.Map.ID || citation["documentation_map_content_hash"] != first.Map.ContentHash || citation["content_hash"] != projectedMap.ContentHash {
		t.Fatalf("selected map citation does not retain source and projection hashes: %#v", citation)
	}
}
