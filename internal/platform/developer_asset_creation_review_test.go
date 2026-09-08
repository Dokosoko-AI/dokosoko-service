package platform_test

import (
	"encoding/json"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"testing"
)

func TestCollectionCreationRequestKeepsExactRevisionAndRequiresReview(t *testing.T) {
	backend := store.NewMemory()
	seedReviewedAIDocumentation(t, backend)
	service := platform.New(backend)
	actor := platform.Actor{ID: "creator"}
	input := platform.DocumentationCollectionInput{Name: "Exact guides", Slug: "exact-guides", Members: []platform.DocumentationCollectionMemberInput{{Kind: "source_publication", ID: "pub_docs_seed", IncludeDescendants: true, Selector: json.RawMessage(`{}`)}}, AcknowledgeReviewed: true, RequestKey: "collection-creation-request-0001"}
	first, err := service.SaveDocumentationCollection(t.Context(), "", input, actor)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := service.SaveDocumentationCollection(t.Context(), "", input, actor)
	if err != nil || first.ID != retry.ID {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	revisions, err := backend.DocumentationCollectionRevisions(t.Context(), "prod_acme", first.ID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("revisions: %+v %v", revisions, err)
	}
	input.AcknowledgeReviewed = false
	if _, err := service.SaveDocumentationCollection(t.Context(), "", input, actor); !errors.Is(err, platform.ErrSourceReviewRequired) {
		t.Fatalf("replay skipped human review: %v", err)
	}
	input.AcknowledgeReviewed = true
	input.Members[0].IncludeDescendants = false
	if _, err := service.SaveDocumentationCollection(t.Context(), "", input, actor); !errors.Is(err, store.ErrDeveloperAssetCreationConflict) {
		t.Fatalf("changed member input: %v", err)
	}
}
