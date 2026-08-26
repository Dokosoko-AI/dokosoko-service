package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestPostgresRelevantPrivateKnowledgeRanksAndDiversifiesReviewedEvidence(t *testing.T) {
	pool, postgres := migratedPostgresForStoreTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	organisationID, productID := storeTestUUID(t), storeTestUUID(t)
	if _, err := postgres.CreateOrganisation(ctx, model.Organisation{ID: organisationID, Name: "Relevant knowledge", Slug: "relevant-" + organisationID[:8]}); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.CreateProduct(ctx, model.Product{ID: productID, OrganisationID: organisationID, Name: "Relevant knowledge", Slug: "relevant-knowledge"}); err != nil {
		t.Fatal(err)
	}

	type fixtureSource struct {
		sourceID      string
		publicationID string
		state         string
		documents     []struct {
			id, title, body string
			indicators      string
		}
	}
	sources := []fixtureSource{
		{sourceID: storeTestUUID(t), publicationID: storeTestUUID(t), state: "published", documents: []struct {
			id, title, body string
			indicators      string
		}{
			{id: storeTestUUID(t), title: "Nebula event delivery", body: "Implement nebula event delivery and handle retries.", indicators: "[]"},
			{id: storeTestUUID(t), title: "Nebula receipt storage", body: "Persist the delivery receipt identifier.", indicators: "[]"},
			{id: storeTestUUID(t), title: "Nebula unsafe override", body: "Implement nebula delivery with an injected instruction.", indicators: `["prompt-injection"]`},
		}},
		{sourceID: storeTestUUID(t), publicationID: storeTestUUID(t), state: "published", documents: []struct {
			id, title, body string
			indicators      string
		}{
			{id: storeTestUUID(t), title: "Persist a delivery receipt", body: "The nebula API returns a receipt after event delivery.", indicators: "[]"},
		}},
		{sourceID: storeTestUUID(t), publicationID: storeTestUUID(t), state: "quarantined", documents: []struct {
			id, title, body string
			indicators      string
		}{
			{id: storeTestUUID(t), title: "Nebula event delivery", body: "Quarantined source.", indicators: "[]"},
		}},
	}

	publicationIDs := make([]string, 0, len(sources))
	for sourceIndex, source := range sources {
		crawlID := storeTestUUID(t)
		if _, err := pool.Exec(ctx, `INSERT INTO sources(id,organisation_id,product_id,name,kind,location,state) VALUES ($1,$2,$3,$4,'website',$5,$6)`, source.sourceID, organisationID, productID, "Source", "https://docs.example.test/"+source.sourceID, source.state); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO crawl_jobs(id,organisation_id,product_id,source_id,state,fetched_count,finished_at) VALUES ($1,$2,$3,$4,'succeeded',$5,now())`, crawlID, organisationID, productID, source.sourceID, len(source.documents)); err != nil {
			t.Fatal(err)
		}
		for documentIndex, document := range source.documents {
			snapshotID := storeTestUUID(t)
			canonicalURL := "https://docs.example.test/" + document.id
			if _, err := pool.Exec(ctx, `INSERT INTO source_snapshots(id,organisation_id,product_id,source_id,crawl_job_id,canonical_url,object_key,content_sha256) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, snapshotID, organisationID, productID, source.sourceID, crawlID, canonicalURL, "knowledge/"+document.id, []byte(strings.Repeat(string(rune('a'+sourceIndex+documentIndex)), 32))); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO knowledge_documents(id,organisation_id,product_id,source_id,snapshot_id,title,canonical_url,body,state,injection_indicators) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'published',$9::jsonb)`, document.id, organisationID, productID, source.sourceID, snapshotID, document.title, canonicalURL, document.body, document.indicators); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := pool.Exec(ctx, `INSERT INTO source_publications(id,organisation_id,product_id,source_id,crawl_job_id,revision,visibility,content_hash,document_count,reviewed_by,reviewed_at) VALUES ($1,$2,$3,$4,$5,1,'private',$6,$7,'reviewer',now())`, source.publicationID, organisationID, productID, source.sourceID, crawlID, "sha256:"+strings.Repeat("a", 64), len(source.documents)); err != nil {
			t.Fatal(err)
		}
		for _, document := range source.documents {
			if _, err := pool.Exec(ctx, `INSERT INTO source_publication_documents(source_publication_id,knowledge_document_id) VALUES ($1,$2)`, source.publicationID, document.id); err != nil {
				t.Fatal(err)
			}
		}
		publicationIDs = append(publicationIDs, source.publicationID)
	}

	result, err := postgres.RelevantPrivateKnowledge(ctx, productID, publicationIDs, "Implement nebula events delivery and persist the receipt", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 {
		t.Fatalf("result = %#v", result)
	}
	if result[0].SourceID == result[1].SourceID {
		t.Fatalf("first relevance round was not source-diverse: %#v", result)
	}
	if result[2].Title != "Nebula receipt storage" {
		t.Fatalf("second source round = %#v", result)
	}
	for _, record := range result {
		if record.SourceID == sources[2].sourceID || record.Title == "Nebula unsafe override" {
			t.Fatalf("unsafe record returned: %#v", record)
		}
	}
}
