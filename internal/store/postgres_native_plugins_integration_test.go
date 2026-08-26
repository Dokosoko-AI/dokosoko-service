package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/nativepluginstate"
	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

func TestPostgresNativePluginStateAndToolContract(t *testing.T) {
	pool, postgres := migratedPostgresForStoreTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	organisationID, productID := storeTestUUID(t), storeTestUUID(t)
	if _, err := postgres.CreateOrganisation(ctx, model.Organisation{ID: organisationID, Name: "Native plugin store", Slug: "native-plugin-" + organisationID[:8]}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM native_plugin_state WHERE plugin_id=$1`, "postgres_native_test"); err != nil {
			t.Errorf("remove native state fixture: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM organisations WHERE id=$1`, organisationID); err != nil {
			t.Errorf("remove native plugin store fixture: %v", err)
		}
	})
	if _, err := postgres.CreateProduct(ctx, model.Product{ID: productID, OrganisationID: organisationID, Name: "Native plugin store", Slug: "native-plugin-store"}); err != nil {
		t.Fatal(err)
	}

	state := nativepluginstate.Bind(postgres, "postgres_native_test", string(nativeplugin.StateCustomer), "opaque_customer")
	createdState, err := state.Put(ctx, "counter", json.RawMessage(`1`), nativeplugin.PutOptions{ExpectedRevision: 0})
	if err != nil || createdState.Revision != 1 {
		t.Fatalf("create state=%#v err=%v", createdState, err)
	}
	if _, err := state.Put(ctx, "counter", json.RawMessage(`2`), nativeplugin.PutOptions{ExpectedRevision: 0}); !errors.Is(err, nativeplugin.ErrStateConflict) {
		t.Fatalf("duplicate create error = %v", err)
	}
	rollback := errors.New("rollback")
	err = state.Transaction(ctx, func(tx nativeplugin.StateTransaction) error {
		if _, err := tx.Put("counter", json.RawMessage(`2`), nativeplugin.PutOptions{ExpectedRevision: 1}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback error = %v", err)
	}
	currentState, err := state.Get(ctx, "counter")
	if err != nil || string(currentState.Value) != "1" || currentState.Revision != 1 {
		t.Fatalf("rolled back state=%#v err=%v", currentState, err)
	}

	tool, err := postgres.CreateTool(ctx, model.Tool{
		ID: storeTestUUID(t), OrganisationID: organisationID, ProductID: productID, Scope: model.ToolScopeCommon,
		Namespace: "postgres_native", Name: "status", Description: "Exercise native tool persistence.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
		HTTPMethod: "NATIVE", AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low","idempotency_required":false}`), TimeoutMS: 1000,
		BackendKind: "native", Effect: "read", IdempotencyMode: "supported", IdentityRequirement: "optional", StateScope: "none", MaxConcurrency: 2, MaxResultBytes: 4096,
		UpstreamAnnotations: json.RawMessage(`{}`), NativePluginID: "postgres_native_test", NativeToolID: "status", NativePluginVersion: "1.0.0", NativeSDKVersion: 1, NativeManifestHash: "sha256:manifest", NativeContractHash: "sha256:contract",
	})
	if err != nil {
		t.Fatal(err)
	}
	published, err := postgres.PublishTool(ctx, productID, tool.ID, tool.Revision, "")
	if err != nil {
		t.Fatal(err)
	}
	if published.State != "published" || published.BackendKind != "native" || published.NativePluginID != "postgres_native_test" || published.IdentityRequirement != "optional" || published.MaxConcurrency != 2 {
		t.Fatalf("published native tool = %#v", published)
	}
}
