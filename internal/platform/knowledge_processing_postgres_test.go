package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/ai/aitest"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func knowledgePostgresFixture(t *testing.T) *store.Postgres {
	t.Helper()
	_, backend := knowledgePostgresFixtureWithPool(t)
	return backend
}

func knowledgePostgresFixtureWithPool(t *testing.T) (*pgxpool.Pool, *store.Postgres) {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DOKOSOKO_TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("disposable PostgreSQL test URL is not configured")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	// Coordinate extension setup with the repository migration lock. Test data
	// lives in an isolated schema and is removed after closing its pool.
	tx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(2811042026)`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public; CREATE EXTENSION IF NOT EXISTS citext WITH SCHEMA public; CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	id, err := randomUUID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "knowledge_processing_" + strings.ReplaceAll(id, "-", "")
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("cleanup schema: %v", err)
		}
	})
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = store.Migrate(ctx, pool, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	backend := store.NewPostgres(pool, "https://dokosoko.example.test")
	org, err := backend.CreateOrganisation(ctx, model.Organisation{ID: id, Name: "Knowledge", Slug: "knowledge"})
	if err != nil {
		t.Fatal(err)
	}
	deploymentID, err := randomUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = backend.CreateDeployment(ctx, model.Deployment{ID: deploymentID, OrganisationID: org.ID, Name: "Knowledge", Slug: "knowledge"}); err != nil {
		t.Fatal(err)
	}
	return pool, backend
}

func TestPostgresKnowledgeProcessingRetainsExactResultsAndClaims(t *testing.T) {
	backend := knowledgePostgresFixture(t)
	runID, _ := randomUUID()
	seedKnowledgeDocuments(t, backend, runID, []string{"Install version 1.2.3. Unicode 東京; HTML <tag> stays text."})
	entered, release := make(chan struct{}), make(chan struct{})
	provider := &aitest.Knowledge{Before: func(r *http.Request) {
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}}
	s := configuredKnowledgeService(t, backend, provider)
	done := make(chan error, 1)
	go func() { _, err := s.ProcessKnowledgeBatch(t.Context(), runID, Actor{ID: "reviewer"}); done <- err }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("provider not entered")
	}
	_, err := s.ProcessKnowledgeBatch(t.Context(), runID, Actor{})
	close(release)
	if !errors.Is(err, ErrKnowledgeProcessingBusy) {
		t.Fatalf("concurrent=%v", err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	// jsonb canonicalization must not invalidate the result's content hash.
	status, err := s.KnowledgeProcessingStatus(t.Context(), runID)
	if err != nil || status.State != "ready" || len(status.Assessments) != 1 {
		t.Fatalf("stored result=%#v %v", status, err)
	}
	if _, err = s.ProcessKnowledgeBatch(t.Context(), runID, Actor{}); err != nil || provider.Calls.Load() != 1 {
		t.Fatalf("repeat=%v calls=%d", err, provider.Calls.Load())
	}

	deployment, err := backend.Deployment(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := json.RawMessage(`{"workflow_version":"claim-test"}`)
	old := time.Now().UTC().Add(-4 * time.Minute)
	stageID, _ := randomUUID()
	first, err := backend.ClaimKnowledgeProcessingStage(t.Context(), deployment.ID, model.DeveloperAssetIngestionStage{ID: stageID, IngestionRunID: runID, Name: model.IngestionStageAIEnrich, State: "running", StartedAt: &old, Checkpoint: checkpoint, Diagnostics: json.RawMessage(`{}`)}, old.Add(-3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	nextID, _ := randomUUID()
	next := model.DeveloperAssetIngestionStage{ID: nextID, IngestionRunID: runID, Name: model.IngestionStageAIEnrich, State: "running", StartedAt: &now, Checkpoint: checkpoint, Diagnostics: json.RawMessage(`{}`)}
	if _, err = backend.ClaimKnowledgeProcessingStage(t.Context(), nextID, next, now.Add(-3*time.Minute)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross deployment claim=%v", err)
	}
	resumed, err := backend.ClaimKnowledgeProcessingStage(t.Context(), deployment.ID, next, now.Add(-3*time.Minute))
	if err != nil || resumed.Attempt != first.Attempt+1 {
		t.Fatalf("claim resume=%#v %v", resumed, err)
	}
	first.State = "succeeded"
	first.FinishedAt = &now
	if _, err = backend.SaveDeveloperAssetIngestionStage(t.Context(), first, "running"); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("expired owner committed: %v", err)
	}
}
