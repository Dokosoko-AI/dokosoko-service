package platform_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/tools"
)

type postgresToolTestDoer struct{ calls int }

func (d *postgresToolTestDoer) Do(*http.Request) (*http.Response, error) {
	d.calls++
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
}

func TestPostgresToolTestConfirmationAndSanitizedEvidence(t *testing.T) {
	pool, postgres := migratedPostgresForTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	organisationID, productID := testUUID(t), testUUID(t)
	if _, err := postgres.CreateOrganisation(ctx, model.Organisation{ID: organisationID, Name: "Live test", Slug: "live-test-" + organisationID[:8]}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM organisations WHERE id=$1`, organisationID)
	})
	if _, err := postgres.CreateProduct(ctx, model.Product{ID: productID, OrganisationID: organisationID, Name: "Live test", Slug: "live-test"}); err != nil {
		t.Fatal(err)
	}
	storage := noAuditStore{Store: postgres}
	service := platform.New(storage)
	tool, err := service.CreateTool(ctx, platform.ToolInput{ProductID: productID, Namespace: "live", Name: "postgres", Description: "Test PostgreSQL evidence.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"label":{"type":"string"}},"required":["label"]}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`), Endpoint: "https://api.vendor.example/items", HTTPMethod: http.MethodPost, UpstreamAuth: json.RawMessage(`{"type":"none"}`), AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":true,"risk":"high","idempotency_required":true}`), TimeoutMS: 1000}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	arguments := map[string]any{"label": "private-request-value"}
	actor := platform.Actor{ID: "root", RequestID: "postgres-live-test"}
	confirmation, err := service.CreateToolTestConfirmation(ctx, productID, tool.ID, platform.ToolTestConfirmationInput{Revision: tool.Revision, Arguments: arguments, TypedToolName: "live.postgres", AcknowledgeSideEffects: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	doer := &postgresToolTestDoer{}
	runtime := tools.NewRuntime(storage, postgresMCPResolver{}, doer)
	run, err := service.RunToolTest(ctx, runtime, productID, tool.ID, platform.ToolTestRunInput{Revision: tool.Revision, Arguments: arguments, ConfirmationNonce: confirmation.ConfirmationNonce, IdempotencyKey: "postgres-live-test-01"}, actor)
	if err != nil || doer.calls != 1 || run.Outcome != "success" {
		t.Fatalf("run=%#v calls=%d err=%v", run, doer.calls, err)
	}
	if _, err := service.RunToolTest(ctx, runtime, productID, tool.ID, platform.ToolTestRunInput{Revision: tool.Revision, Arguments: arguments, ConfirmationNonce: confirmation.ConfirmationNonce, IdempotencyKey: "postgres-live-test-01"}, actor); !errors.Is(err, platform.ErrToolTestConfirmationReplayed) {
		t.Fatalf("replay error=%v", err)
	}
	digest := sha256.Sum256([]byte(confirmation.ConfirmationNonce))
	var storedDigest []byte
	var consumptions int
	if err := pool.QueryRow(ctx, `SELECT nonce_digest FROM tool_test_confirmations WHERE tool_id=$1`, tool.ID).Scan(&storedDigest); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tool_test_confirmation_consumptions c JOIN tool_test_confirmations t ON t.id=c.confirmation_id WHERE t.tool_id=$1`, tool.ID).Scan(&consumptions); err != nil {
		t.Fatal(err)
	}
	if string(storedDigest) != string(digest[:]) || consumptions != 1 {
		t.Fatalf("digest or consumption mismatch: digest=%x consumptions=%d", storedDigest, consumptions)
	}
	stored, err := postgres.ToolTestRun(ctx, productID, tool.ID, run.ID, time.Now().UTC())
	if err != nil || stored.RequestShape.Properties["label"].Type != "string" || stored.ResponseShape == nil || stored.ResponseShape.Properties["ok"].Type != "boolean" {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tool_test_confirmations SET created_at=now()-interval '2 hours',expires_at=now()-interval '1 hour' WHERE tool_id=$1`, tool.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tool_test_runs SET created_at=now()-interval '2 hours',expires_at=now()-interval '1 hour' WHERE tool_id=$1`, tool.ID); err != nil {
		t.Fatal(err)
	}
	deleted, err := postgres.DeleteExpiredToolTestData(ctx, time.Now().UTC(), 100)
	if err != nil || deleted != 2 {
		t.Fatalf("expired cleanup deleted=%d err=%v", deleted, err)
	}
	var confirmationsRemaining, runsRemaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tool_test_confirmations WHERE tool_id=$1`, tool.ID).Scan(&confirmationsRemaining); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tool_test_runs WHERE tool_id=$1`, tool.ID).Scan(&runsRemaining); err != nil {
		t.Fatal(err)
	}
	if confirmationsRemaining != 0 || runsRemaining != 0 {
		t.Fatalf("expired rows remain: confirmations=%d runs=%d", confirmationsRemaining, runsRemaining)
	}
}
