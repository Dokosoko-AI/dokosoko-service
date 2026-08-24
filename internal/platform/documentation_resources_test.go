package platform_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestDocumentationResourceSetRequiresExactReviewedPublication(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "reviewer", RequestID: "request-doc-resource"}

	if _, err := service.CreateResourceSet(ctx, platform.ResourceSetInput{Kind: "documentation", Name: "Unsafe docs", Manifest: json.RawMessage(`[{"url":"https://docs.example.test"}]`)}, actor); err == nil {
		t.Fatal("arbitrary documentation manifest was accepted")
	}
	publications, err := memory.SourcePublications(ctx, "prod_acme", "src_docs")
	if err != nil || len(publications) == 0 {
		t.Fatalf("seed publication = %#v, err = %v", publications, err)
	}
	publication := publications[0]
	manifest, err := json.Marshal([]map[string]any{{"source_publication_id": publication.ID, "source_id": publication.SourceID, "revision": publication.Revision, "content_hash": publication.ContentHash, "name": "Reviewed docs"}})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := service.CreateResourceSet(ctx, platform.ResourceSetInput{Kind: "documentation", Name: "Reviewed docs", Manifest: manifest}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if resource.Latest == nil || len(resource.Latest.Manifest) == 0 {
		t.Fatalf("resource = %#v", resource)
	}
	var entries []map[string]any
	if err := json.Unmarshal(resource.Latest.Manifest, &entries); err != nil || len(entries) != 1 || entries[0]["source_publication_id"] != publication.ID {
		t.Fatalf("canonical manifest = %s, err = %v", resource.Latest.Manifest, err)
	}
	if _, err := service.UpdateResourceSet(ctx, resource.ID, platform.ResourceSetInput{Kind: "documentation", Name: resource.Name, State: resource.State, Revision: resource.Revision, Manifest: json.RawMessage(`[{"source_publication_id":"pub_docs_seed","source_id":"src_docs","revision":1,"content_hash":"sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","name":"Tampered"}]`)}, actor); err == nil {
		t.Fatal("tampered publication hash was accepted")
	}
	publicIntegration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "public-docs", VersionKey: "v1", DisplayName: "Public docs", Visibility: "public", AcknowledgePublic: true, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AttachResourceSet(ctx, publicIntegration.ID, resource.ID, resource.Latest.ID, actor); err != nil {
		t.Fatal(err)
	}
	status, err := service.IntegrationPublishStatus(ctx, publicIntegration.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundPrivatePublication := false
	for _, validation := range status.Validations {
		foundPrivatePublication = foundPrivatePublication || validation.Code == "documentation_not_public" && validation.Level == "error"
	}
	if status.Ready || !foundPrivatePublication {
		t.Fatalf("public Integration accepted a private documentation publication: %#v", status.Validations)
	}
}
