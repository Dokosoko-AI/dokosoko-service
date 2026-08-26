package store

import (
	"context"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestMemoryRelevantPrivateKnowledgeRanksAndDiversifiesReviewedEvidence(t *testing.T) {
	t.Parallel()
	memory := NewMemory()
	productID := "prod_acme"
	for _, source := range []model.Source{
		{ID: "source_quasar_guide", ProductID: productID, Published: true},
		{ID: "source_quasar_api", ProductID: productID, Published: true},
		{ID: "source_quasar_draft", ProductID: productID},
		{ID: "source_quasar_quarantined", ProductID: productID, Published: true, Quarantined: true},
	} {
		memory.sources[productID][source.ID] = source
	}
	publications := []model.SourcePublication{
		{ID: "publication_quasar_guide", ProductID: productID, SourceID: "source_quasar_guide"},
		{ID: "publication_quasar_api", ProductID: productID, SourceID: "source_quasar_api"},
		{ID: "publication_quasar_draft", ProductID: productID, SourceID: "source_quasar_draft"},
		{ID: "publication_quasar_quarantined", ProductID: productID, SourceID: "source_quasar_quarantined"},
	}
	for _, publication := range publications {
		memory.sourcePublications[productID][publication.ID] = publication
		memory.publicationDocuments[publication.ID] = make(map[string]bool)
	}
	records := []model.KnowledgeRecord{
		{ID: "guide_best", ProductID: productID, SourceID: "source_quasar_guide", Title: "Quasar telemetry ingestion", Text: "Implement quasar telemetry ingestion in the client and handle retries.", Published: true},
		{ID: "guide_second", ProductID: productID, SourceID: "source_quasar_guide", Title: "Quasar receipt storage", Text: "Persist the telemetry receipt after ingestion.", Published: true},
		{ID: "api_best", ProductID: productID, SourceID: "source_quasar_api", Title: "Persist an ingestion receipt", Text: "The quasar operation returns the receipt identifier.", Published: true},
		{ID: "draft_match", ProductID: productID, SourceID: "source_quasar_draft", Title: "Quasar telemetry ingestion", Text: "Unreviewed draft instructions.", Published: true},
		{ID: "quarantined_match", ProductID: productID, SourceID: "source_quasar_quarantined", Title: "Quasar telemetry ingestion", Text: "Quarantined instructions.", Published: true},
		{ID: "unpublished_match", ProductID: productID, SourceID: "source_quasar_api", Title: "Quasar telemetry ingestion", Text: "Not published.", Published: false},
		{ID: "unpinned_match", ProductID: productID, SourceID: "source_quasar_api", Title: "Quasar telemetry ingestion", Text: "Not selected by the reviewed publication.", Published: true},
	}
	memory.knowledge[productID] = append(memory.knowledge[productID], records...)
	memory.publicationDocuments["publication_quasar_guide"]["guide_best"] = true
	memory.publicationDocuments["publication_quasar_guide"]["guide_second"] = true
	memory.publicationDocuments["publication_quasar_api"]["api_best"] = true
	memory.publicationDocuments["publication_quasar_api"]["unpublished_match"] = true
	memory.publicationDocuments["publication_quasar_draft"]["draft_match"] = true
	memory.publicationDocuments["publication_quasar_quarantined"]["quarantined_match"] = true

	publicationIDs := []string{
		"publication_quasar_guide",
		"publication_quasar_api",
		"publication_quasar_draft",
		"publication_quasar_quarantined",
	}
	result, err := memory.RelevantPrivateKnowledge(context.Background(), productID, publicationIDs, "Implement quasar telemetry ingestion and persist its receipt", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 {
		t.Fatalf("result = %#v", result)
	}
	if result[0].SourceID == result[1].SourceID {
		t.Fatalf("first relevance round was not source-diverse: %#v", result)
	}
	if result[2].ID != "guide_second" {
		t.Fatalf("second source round = %#v", result)
	}
	for _, record := range result {
		switch record.ID {
		case "draft_match", "quarantined_match", "unpublished_match", "unpinned_match":
			t.Fatalf("unsafe or unreviewed record returned: %#v", record)
		}
	}
}

func TestRelevantPrivateKnowledgeBoundsQueryAndResultSet(t *testing.T) {
	t.Parallel()
	memory := NewMemory()

	if result, err := memory.RelevantPrivateKnowledge(context.Background(), "prod_acme", []string{"pub_docs_seed"}, "the and how to", 10); err != nil || len(result) != 0 {
		t.Fatalf("stop-word query = %#v, %v", result, err)
	}
	if result, err := memory.RelevantPrivateKnowledge(context.Background(), "prod_acme", []string{"pub_docs_seed"}, "api key", 0); err != nil || len(result) != 0 {
		t.Fatalf("zero-limit query = %#v, %v", result, err)
	}

	query := "api key " + strings.Repeat("x", maxRelevantKnowledgeQueryRunes+100)
	if got := boundedRelevantKnowledgeQuery(query); len([]rune(got)) > maxRelevantKnowledgeQueryRunes {
		t.Fatalf("bounded query has %d runes", len([]rune(got)))
	}
	if got := boundedRelevantKnowledgeLimit(maxRelevantKnowledgeResults + 100); got != maxRelevantKnowledgeResults {
		t.Fatalf("bounded limit = %d", got)
	}
}
