package platform_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/mcpbridge"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// noAuditStore keeps this integration fixture removable. Production audit
// events are deliberately append-only and hold restrictive organisation and
// product foreign keys, while this test only concerns migrated tool storage.
type noAuditStore struct{ store.Store }

func (noAuditStore) AppendAudit(context.Context, model.AuditEvent) error { return nil }

type postgresMCPResolver struct{}

func (postgresMCPResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("8.8.8.8")}, nil
}

type postgresMCPDoer struct{ t *testing.T }

func (d postgresMCPDoer) Do(request *http.Request) (*http.Response, error) {
	d.t.Helper()
	var rpc struct {
		JSONRPC string         `json:"jsonrpc"`
		ID      string         `json:"id"`
		Method  string         `json:"method"`
		Params  map[string]any `json:"params"`
	}
	if err := json.NewDecoder(request.Body).Decode(&rpc); err != nil {
		return nil, err
	}
	if rpc.JSONRPC != "2.0" || rpc.ID == "" || rpc.Method != "tools/list" {
		return nil, fmt.Errorf("unexpected MCP request: %#v", rpc)
	}
	if request.Header.Get("MCP-Protocol-Version") != model.StatelessMCPv2Protocol {
		return nil, errors.New("missing Stateless MCPv2 protocol header")
	}
	result := map[string]any{
		"_meta": map[string]any{
			"io.modelcontextprotocol/serverInfo": map[string]any{"name": "PostgreSQL regression upstream", "version": "2.0.0"},
		},
		"resultType": "complete",
		"tools": []map[string]any{{
			"name":        "health.check",
			"description": "Check upstream health",
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           map[string]any{},
			},
			"outputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"ok": map[string]any{"type": "boolean"},
				},
				"required": []string{"ok"},
			},
		}},
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": result})
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

func TestPostgresMigratedToolLifecycleAndMCPImport(t *testing.T) {
	pool, postgres := migratedPostgresForTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	organisationID := testUUID(t)
	productID := testUUID(t)
	suffix := strings.ReplaceAll(productID[:13], "-", "")
	if _, err := postgres.CreateOrganisation(ctx, model.Organisation{
		ID: organisationID, Name: "Tool lifecycle " + suffix, Slug: "tool-lifecycle-" + suffix,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM organisations WHERE id=$1`, organisationID); err != nil {
			t.Errorf("remove PostgreSQL tool fixture: %v", err)
		}
	})
	if _, err := postgres.CreateProduct(ctx, model.Product{
		ID: productID, OrganisationID: organisationID, Name: "Tool lifecycle", Slug: "tool-lifecycle",
	}); err != nil {
		t.Fatal(err)
	}

	vault, err := secrets.New(bytes.Repeat([]byte{0x5c}, 32))
	if err != nil {
		t.Fatal(err)
	}
	storage := noAuditStore{Store: postgres}
	service := platform.NewWithVault(storage, vault)

	inputSchema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`)
	outputSchema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`)
	policy := json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low","idempotency_required":false}`)
	created, err := service.CreateTool(ctx, platform.ToolInput{
		ProductID:           productID,
		Namespace:           "status",
		Name:                "check",
		Description:         "Check the vendor API status.",
		InputSchema:         inputSchema,
		OutputSchema:        outputSchema,
		Endpoint:            "https://api.vendor.example/v1/status",
		HTTPMethod:          http.MethodGet,
		UpstreamAuth:        json.RawMessage(`{"type":"bearer"}`),
		Credential:          "credential-v1",
		RequestMapping:      json.RawMessage(`{}`),
		ResponseMapping:     json.RawMessage(`{}`),
		AuthorizationPolicy: policy,
		TimeoutMS:           5_000,
	}, platform.Actor{RequestID: "postgres-create"})
	if err != nil {
		t.Fatal(err)
	}
	if created.State != "draft" || created.CredentialID == "" || created.APIConnectionID == "" {
		t.Fatalf("created HTTP tool = %#v", created)
	}
	oldCredentialID := created.CredentialID

	rotated, err := service.UpdateTool(ctx, productID, created.ID, platform.ToolInput{
		Namespace:           created.Namespace,
		Name:                created.Name,
		Description:         created.Description,
		InputSchema:         created.InputSchema,
		OutputSchema:        created.OutputSchema,
		Endpoint:            created.BaseURL,
		HTTPMethod:          created.HTTPMethod,
		UpstreamAuth:        created.UpstreamAuth,
		Credential:          "credential-v2",
		RequestMapping:      created.RequestMapping,
		ResponseMapping:     created.ResponseMapping,
		AuthorizationPolicy: created.AuthorizationPolicy,
		TimeoutMS:           created.TimeoutMS,
	}, created.Revision, platform.Actor{RequestID: "postgres-rotate"})
	if err != nil {
		t.Fatal(err)
	}
	if rotated.CredentialID == "" || rotated.CredentialID == oldCredentialID {
		t.Fatalf("rotated credential id = %q, old = %q", rotated.CredentialID, oldCredentialID)
	}
	if _, err := postgres.Secret(ctx, organisationID, oldCredentialID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("old credential still exists after rotation: %v", err)
	}

	published, err := service.PublishTool(ctx, productID, rotated.ID, rotated.Revision, platform.Actor{RequestID: "postgres-publish"})
	if err != nil {
		t.Fatal(err)
	}
	if published.State != "published" {
		t.Fatalf("published HTTP tool state = %q", published.State)
	}
	retired, err := service.RetireTool(ctx, productID, published.ID, published.Revision, platform.Actor{RequestID: "postgres-retire"})
	if err != nil {
		t.Fatal(err)
	}
	if retired.State != "retired" || retired.CredentialID != "" {
		t.Fatalf("retired HTTP tool = %#v", retired)
	}
	if _, err := postgres.Secret(ctx, organisationID, rotated.CredentialID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("active credential still exists after retirement: %v", err)
	}
	var authenticationType string
	var authConfig []byte
	var credentialSecretID *string
	if err := pool.QueryRow(ctx, `SELECT authentication_type,auth_config,credential_secret_id::text FROM api_connections WHERE id=$1`, created.APIConnectionID).Scan(&authenticationType, &authConfig, &credentialSecretID); err != nil {
		t.Fatal(err)
	}
	if authenticationType != "none" || credentialSecretID != nil || !jsonObjectHasString(authConfig, "type", "none") {
		t.Fatalf("retired connection auth type=%q config=%s credential=%v", authenticationType, authConfig, credentialSecretID)
	}

	manager := mcpbridge.New(storage, vault, postgresMCPResolver{}, postgresMCPDoer{t: t})
	connection, err := manager.CreateConnection(ctx, mcpbridge.ConnectionInput{
		OrganisationID: organisationID,
		ProductID:      productID,
		Name:           "Regression MCP",
		Namespace:      "upstream",
		Endpoint:       "https://mcp.vendor.example/v2",
		AccessToken:    "regression-access-token",
	}, mcpbridge.Actor{RequestID: "postgres-mcp-connection"})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := manager.Import(ctx, productID, connection.ID, mcpbridge.ImportInput{
		ToolNames: []string{"health.check"}, TimeoutMS: 5_000,
	}, mcpbridge.Actor{RequestID: "postgres-mcp-import"})
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Created) != 1 || len(imported.Rejected) != 0 {
		t.Fatalf("MCP import = %#v", imported)
	}
	mcpDraft := imported.Created[0]
	if mcpDraft.HTTPMethod != "MCP" || mcpDraft.BackendKind != "mcp" || !jsonObjectIsEmpty(mcpDraft.RequestMapping) || !jsonObjectIsEmpty(mcpDraft.ResponseMapping) {
		t.Fatalf("MCP draft method=%q backend=%q request_mapping=%s response_mapping=%s", mcpDraft.HTTPMethod, mcpDraft.BackendKind, mcpDraft.RequestMapping, mcpDraft.ResponseMapping)
	}
	mcpPublished, err := service.PublishTool(ctx, productID, mcpDraft.ID, mcpDraft.Revision, platform.Actor{RequestID: "postgres-mcp-publish"})
	if err != nil {
		t.Fatal(err)
	}
	if mcpPublished.State != "published" || mcpPublished.HTTPMethod != "MCP" {
		t.Fatalf("published MCP tool = %#v", mcpPublished)
	}
	var releaseBackend, releaseConnectionID string
	var requestMapping, responseMapping []byte
	if err := pool.QueryRow(ctx, `SELECT backend_kind,mcp_connection_id::text,request_mapping,response_mapping FROM tool_releases WHERE tool_definition_id=$1`, mcpPublished.ID).Scan(&releaseBackend, &releaseConnectionID, &requestMapping, &responseMapping); err != nil {
		t.Fatal(err)
	}
	if releaseBackend != "mcp" || releaseConnectionID != connection.ID || !jsonObjectIsEmpty(requestMapping) || !jsonObjectIsEmpty(responseMapping) {
		t.Fatalf("MCP release backend=%q connection=%q request_mapping=%s response_mapping=%s", releaseBackend, releaseConnectionID, requestMapping, responseMapping)
	}
}

func migratedPostgresForTest(t *testing.T) (*pgxpool.Pool, *store.Postgres) {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DOKOSOKO_TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("DOKOSOKO_TEST_DATABASE_URL or TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Skipf("PostgreSQL is unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("PostgreSQL is unavailable: %v", err)
	}
	if err := store.Migrate(ctx, pool, filepath.Join("..", "..", "migrations")); err != nil {
		pool.Close()
		t.Fatalf("migrate PostgreSQL fixture: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, store.NewPostgres(pool, "https://dokosoko.example")
}

func testUUID(t *testing.T) string {
	t.Helper()
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		t.Fatal(err)
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}

func jsonObjectIsEmpty(raw []byte) bool {
	var value map[string]any
	return json.Unmarshal(raw, &value) == nil && len(value) == 0
}

func jsonObjectHasString(raw []byte, key, want string) bool {
	var value map[string]any
	return json.Unmarshal(raw, &value) == nil && value[key] == want
}
