package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func developerAssetAIResultHash(value json.RawMessage) string {
	digest := sha256.Sum256(value)
	return "sha256:" + fmt.Sprintf("%x", digest)
}

func validMemoryDeveloperAssetAIAdvisory() model.DeveloperAssetAIAdvisoryRun {
	result := json.RawMessage(`{"status":"uncertain","entries":[],"gaps":[{"code":"missing_evidence","evidence_ids":[]}]}`)
	return model.DeveloperAssetAIAdvisoryRun{
		ID: "ai-advisory-1", DeploymentID: "prod_acme", PromptKey: "documentation.map_enrichment",
		PromptVersion: "documentation-map-enrichment-v1", ScopeKind: "documentation_publication",
		ScopeID: "publication-1", ScopeVisibility: model.VisibilityPrivate,
		IngestionRunID: "ingestion-1", SourcePublicationID: "publication-1",
		AllowedEvidenceIDs: []string{"document-1", "map-1"}, EvidenceHash: "sha256:" + strings.Repeat("1", 64),
		InputHash: "sha256:" + strings.Repeat("2", 64), Result: result, ResultHash: developerAssetAIResultHash(result),
		CreatedBy: "reviewer", CreatedAt: time.Now().UTC(),
	}
}

func TestMemoryDeveloperAssetAIAdvisoriesAreImmutableAndIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := NewMemory()
	input := validMemoryDeveloperAssetAIAdvisory()

	created, err := memory.CreateDeveloperAssetAIAdvisoryRun(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	input.AllowedEvidenceIDs[0] = "mutated-by-caller"
	input.Result[0] = '['
	stored, err := memory.DeveloperAssetAIAdvisoryRun(ctx, "prod_acme", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AllowedEvidenceIDs[0] != "document-1" || string(stored.Result) != `{"status":"uncertain","entries":[],"gaps":[{"code":"missing_evidence","evidence_ids":[]}]}` {
		t.Fatalf("caller mutation changed immutable advisory: %#v", stored)
	}

	retry := validMemoryDeveloperAssetAIAdvisory()
	retry.ID = "ai-advisory-retry"
	retried, err := memory.CreateDeveloperAssetAIAdvisoryRun(ctx, retry)
	if err != nil || retried.ID != created.ID {
		t.Fatalf("idempotent create = %#v, err=%v", retried, err)
	}
	byHash, err := memory.DeveloperAssetAIAdvisoryRunByInputHash(ctx, "prod_acme", retry.PromptKey, retry.InputHash)
	if err != nil || byHash.ID != created.ID {
		t.Fatalf("lookup by deterministic input = %#v, err=%v", byHash, err)
	}
	items, err := memory.DeveloperAssetAIAdvisoryRuns(ctx, DeveloperAssetAIAdvisoryQuery{
		DeploymentID: "prod_acme", PromptKey: retry.PromptKey, ScopeID: retry.ScopeID, Limit: 10,
	})
	if err != nil || len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("advisory list = %#v, err=%v", items, err)
	}

	conflict := validMemoryDeveloperAssetAIAdvisory()
	conflict.InputHash = "sha256:" + strings.Repeat("3", 64)
	if _, err := memory.CreateDeveloperAssetAIAdvisoryRun(ctx, conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("same artifact ID mutation error = %v, want ErrConflict", err)
	}
}

func TestDeveloperAssetAIAdvisoryStoresRejectInvalidModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := NewMemory()
	for name, mutate := range map[string]func(*model.DeveloperAssetAIAdvisoryRun){
		"result hash mismatch": func(value *model.DeveloperAssetAIAdvisoryRun) { value.ResultHash = "sha256:" + strings.Repeat("f", 64) },
		"duplicate evidence":   func(value *model.DeveloperAssetAIAdvisoryRun) { value.AllowedEvidenceIDs = []string{"map-1", "map-1"} },
		"unsorted evidence": func(value *model.DeveloperAssetAIAdvisoryRun) {
			value.AllowedEvidenceIDs = []string{"map-1", "document-1"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := validMemoryDeveloperAssetAIAdvisory()
			mutate(&value)
			if value.Valid() {
				t.Fatalf("invalid advisory passed model validation: %#v", value)
			}
			if _, err := memory.CreateDeveloperAssetAIAdvisoryRun(ctx, value); !errors.Is(err, ErrConflict) {
				t.Fatalf("invalid advisory create error = %v, want ErrConflict", err)
			}
		})
	}
}

func TestDeveloperAssetAIAdvisoryMigrationIsAppendOnlyAndLineageGuarded(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0064_developer_asset_ai_advisories.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, required := range []string{
		"CREATE TABLE developer_asset_ai_advisory_runs",
		"CREATE FUNCTION guard_developer_asset_ai_advisory_lineage()",
		"CREATE TRIGGER developer_asset_ai_advisory_lineage_guard",
		"BEFORE INSERT ON developer_asset_ai_advisory_runs",
		"CREATE TRIGGER developer_asset_immutable_guard",
		"BEFORE UPDATE OR DELETE ON developer_asset_ai_advisory_runs",
		"EXECUTE FUNCTION reject_developer_asset_immutable_mutation()",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("0064 migration is missing %q", required)
		}
	}
}
