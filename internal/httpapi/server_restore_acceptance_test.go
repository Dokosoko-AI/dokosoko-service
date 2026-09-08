package httpapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/ai/aitest"
	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This opt-in drill creates and drops only its own random databases. It uses
// actual PostgreSQL archives and HTTP root authentication, with fixture AI and
// an in-process handler; it is not a deployed-image or production OAuth drill.
func TestPostgresBackupRestoreAcceptance(t *testing.T) {
	databaseURL := os.Getenv("DOKOSOKO_RESTORE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set DOKOSOKO_RESTORE_TEST_DATABASE_URL to a disposable loopback PostgreSQL 17 cluster with CREATEDB permission")
	}
	started := time.Now()
	ctx := t.Context()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("invalid restore test database configuration")
	}
	if config.ConnConfig.Host != "127.0.0.1" && config.ConnConfig.Host != "::1" {
		t.Fatal("restore fixture requires a literal loopback database host")
	}
	for _, fallback := range config.ConnConfig.Fallbacks {
		if fallback.Host != config.ConnConfig.Host || fallback.Port != config.ConnConfig.Port {
			t.Fatal("restore fixture cannot use another fallback database destination")
		}
	}
	admin, err := pgxpool.NewWithConfig(ctx, config.Copy())
	if err != nil {
		t.Fatal("open restore fixture cluster")
	}
	t.Cleanup(admin.Close)
	var serverVersion, vectorVersion string
	if err := admin.QueryRow(ctx, `SHOW server_version_num`).Scan(&serverVersion); err != nil || !strings.HasPrefix(serverVersion, "17") {
		t.Fatal("restore fixture requires a reachable PostgreSQL 17 server")
	}
	dumpBinary, err := exec.LookPath("pg_dump")
	if err != nil {
		t.Fatal("configured restore drill requires pg_dump on PATH")
	}
	restoreBinary, err := exec.LookPath("pg_restore")
	if err != nil {
		t.Fatal("configured restore drill requires pg_restore on PATH")
	}
	command := func(binary, database string, args ...string) ([]byte, error) {
		commandCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		cmd := exec.CommandContext(commandCtx, binary, args...)
		// Never inherit a service file, password file, search path or database
		// destination from the operator's normal PostgreSQL environment.
		for _, value := range os.Environ() {
			if !strings.HasPrefix(strings.ToUpper(value), "PG") {
				cmd.Env = append(cmd.Env, value)
			}
		}
		cmd.Env = append(cmd.Env, "PGHOST="+config.ConnConfig.Host, "PGPORT="+strconv.Itoa(int(config.ConnConfig.Port)), "PGUSER="+config.ConnConfig.User, "PGPASSWORD="+config.ConnConfig.Password, "PGDATABASE="+database, "PGSSLMODE=disable", "PGCONNECT_TIMEOUT=5", "PGPASSFILE="+os.DevNull)
		return cmd.CombinedOutput()
	}
	dumpVersion, err := command(dumpBinary, config.ConnConfig.Database, "--version")
	if err != nil {
		t.Fatal("read pg_dump version")
	}
	restoreVersion, err := command(restoreBinary, config.ConnConfig.Database, "--version")
	if err != nil {
		t.Fatal("read pg_restore version")
	}
	createDatabase := func(label string) (string, *pgxpool.Pool) {
		name := "dokosoko_restore_" + strings.ReplaceAll(restoreFixtureID(), "-", "") + "_" + label
		if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE template0"); err != nil {
			t.Fatalf("create isolated %s database: %v", label, err)
		}
		var pool *pgxpool.Pool
		t.Cleanup(func() {
			if pool != nil {
				pool.Close()
			}
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
				t.Errorf("cleanup owned restore database %s: %v", name, err)
			}
		})
		copy := config.Copy()
		copy.ConnConfig.Database = name
		copy.ConnConfig.RuntimeParams = map[string]string{}
		pool, err = pgxpool.NewWithConfig(ctx, copy)
		if err != nil {
			t.Fatal("open isolated restore database")
		}
		return name, pool
	}
	sourceName, sourcePool := createDatabase("source")
	targetName, targetPool := createDatabase("target")
	migrations := filepath.Join("..", "..", "migrations")
	if err := store.Migrate(ctx, sourcePool, migrations); err != nil {
		t.Fatal(err)
	}
	if err := sourcePool.QueryRow(ctx, `SELECT extversion FROM pg_extension WHERE extname='vector'`).Scan(&vectorVersion); err != nil {
		t.Fatal("pgvector was not installed")
	}
	backend := store.NewPostgres(sourcePool, "https://dokosoko.example")
	orgID, deploymentID := restoreFixtureID(), restoreFixtureID()
	if _, err := backend.CreateOrganisation(ctx, model.Organisation{ID: orgID, Name: "Restore fixture", Slug: "restore"}); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.CreateDeployment(ctx, model.Deployment{ID: deploymentID, OrganisationID: orgID, Name: "Restore fixture", Slug: "restore"}); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	vault, err := secrets.New(key)
	if err != nil {
		t.Fatal(err)
	}
	provider := &aitest.Knowledge{}
	service := platform.NewWithVaultAndProductBuilderDoer(backend, vault, provider)
	if err := service.ConfigureEnvironmentAI(ctx, platform.AIEnvironmentConfig{Provider: "openai-compatible", APIKey: "fixture-only", Endpoint: "https://llm.example.test", Models: map[ai.Workload]string{ai.WorkloadAnalysis: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	manager, err := auth.New(backend, auth.Config{SetupToken: "restore-fixture-setup-token", MasterKey: key, PublicURL: "https://dokosoko.example", SessionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	const email, password = "restore@example.test", "Correct-Horse-47!Battery"
	enrollment, err := manager.BeginSetup(ctx, "restore-fixture-setup-token", auth.SetupInput{Email: email, DisplayName: "Restore operator", Password: password})
	if err != nil {
		t.Fatal("begin root setup")
	}
	totpSecret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal("decode fixture TOTP secret")
	}
	setup, err := manager.CompleteSetup(ctx, enrollment.ID, auth.TOTP(totpSecret, time.Now().UTC()))
	if err != nil {
		t.Fatal("complete root MFA setup")
	}
	actor := platform.Actor{ID: setup.User.ID}
	integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "restore-orders", VersionKey: "v1", DisplayName: "Restore Orders", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	prepareTaskConnectionSDK(t, service, integration.ID, model.VisibilityPrivate, actor)
	if provider.Calls.Load() == 0 {
		t.Fatal("required AI processing did not run")
	}
	if _, err := service.PublishIntegration(ctx, integration.ID, actor); err != nil {
		t.Fatal(err)
	}
	publication, err := service.ReadyAPIDeveloperAssetPublication(ctx, integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	query := platform.DeveloperAssetQueryLabInput{Scope: "api", APIID: integration.ID, APIDeveloperAssetPublicationID: publication.ID, Query: "Orders SDK read order status", ExactVersions: []string{"1.2.3"}, Limit: 5, ContextTokenLimit: 2000}
	beforeQuery, err := service.RunDeveloperAssetQueryLab(ctx, query)
	if err != nil || len(beforeQuery.Results) == 0 {
		t.Fatalf("query prepared guidance: %v", err)
	}
	uploadDir, backupDir, restoredDir := t.TempDir(), t.TempDir(), t.TempDir()
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", Auth: manager, UploadDirectory: uploadDir, UploadMaxBytes: 4096})
	var uploadBody bytes.Buffer
	writer := multipart.NewWriter(&uploadBody)
	if err := writer.WriteField("organisation_id", orgID); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("file", "recovery-guide.md")
	if err != nil {
		t.Fatal(err)
	}
	uploadContent := []byte("# Recovery guide\n\nKeep the exact package version with the reviewed guidance.\n")
	if _, err := file.Write(uploadContent); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	uploadRequest := httptest.NewRequest(http.MethodPost, "/api/v1/products/"+deploymentID+"/sources/upload", &uploadBody)
	uploadRequest.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRequest.Header.Set("Origin", "https://dokosoko.example")
	uploadRequest.Header.Set("X-CSRF-Token", setup.CSRFToken)
	uploadRequest.AddCookie(&http.Cookie{Name: "dokosoko_session", Value: setup.Session.Token})
	uploadResponse := httptest.NewRecorder()
	handler.ServeHTTP(uploadResponse, uploadRequest)
	if uploadResponse.Code != http.StatusCreated {
		t.Fatalf("authenticated fixture upload status: %d", uploadResponse.Code)
	}
	var source model.Source
	if err := json.Unmarshal(uploadResponse.Body.Bytes(), &source); err != nil || !filepath.IsLocal(source.Location) {
		t.Fatal("upload did not retain a relative storage location")
	}
	secretID := restoreFixtureID()
	const plaintext = "local-restore-fixture-credential"
	aad := orgID + ":restore:" + secretID
	encrypted, err := vault.Encrypt([]byte(plaintext), aad)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: orgID, Name: "Recovery fixture", Purpose: "restore", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, Fingerprint: encrypted.Fingerprint, KeyVersion: encrypted.KeyVersion}); err != nil {
		t.Fatal(err)
	}
	before := restoreDatabaseSnapshot(t, sourcePool)
	// All fixture writers have finished. No service or crawler background worker
	// is started. Close the source application pool before copying both stores.
	sourcePool.Close()
	archive := filepath.Join(backupDir, "database.dump")
	if _, err := command(dumpBinary, sourceName, "--format=custom", "--no-owner", "--no-acl", "--file="+archive); err != nil {
		t.Fatalf("pg_dump failed: %v", err)
	}
	uploadBytes, err := os.ReadFile(filepath.Join(uploadDir, source.Location))
	if err != nil || !bytes.Equal(uploadBytes, uploadContent) {
		t.Fatal("source upload bytes differ from submitted content")
	}
	if err := os.WriteFile(filepath.Join(backupDir, source.Location), uploadBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := command(restoreBinary, targetName, "--exit-on-error", "--single-transaction", "--no-owner", "--no-acl", "--dbname="+targetName, archive); err != nil {
		t.Fatalf("pg_restore failed: %v", err)
	}
	backupUpload, err := os.ReadFile(filepath.Join(backupDir, source.Location))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(restoredDir, source.Location), backupUpload, 0600); err != nil {
		t.Fatal(err)
	}
	if after := restoreDatabaseSnapshot(t, targetPool); !reflect.DeepEqual(before, after) {
		t.Fatal("restored table row counts/content hashes differ from the quiesced source")
	}
	if err := store.Migrate(ctx, targetPool, migrations); err != nil {
		t.Fatalf("restored migration ledger cannot replay: %v", err)
	}
	if after := restoreDatabaseSnapshot(t, targetPool); !reflect.DeepEqual(before, after) {
		t.Fatal("migration replay changed restored rows")
	}
	restored := store.NewPostgres(targetPool, "https://dokosoko.example")
	restoredVault, _ := secrets.New(key)
	restoredService := platform.NewWithVault(restored, restoredVault)
	restoredManager, err := auth.New(restored, auth.Config{MasterKey: key, PublicURL: "https://dokosoko.example", SessionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	restoredHandler := httpapi.NewWithOptions(restoredService, httpapi.Options{BaseURL: "https://dokosoko.example", Auth: restoredManager, UploadDirectory: restoredDir, UploadMaxBytes: 4096})
	for _, path := range []string{"/healthz", "/readyz"} {
		if response := requestWithCookies(t, restoredHandler, http.MethodGet, path, "", nil, ""); response.Code != http.StatusOK {
			t.Fatalf("restored %s status: %d", path, response.Code)
		}
	}
	login := func(handler http.Handler, code string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"email": email, "password": password, "code": code})
		return requestWithCookies(t, handler, http.MethodPost, "/api/v1/auth/login", string(body), nil, "")
	}
	if response := login(restoredHandler, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("root login without MFA status: %d", response.Code)
	}
	loggedIn := login(restoredHandler, auth.TOTP(totpSecret, time.Now().UTC()))
	if loggedIn.Code != http.StatusOK || len(loggedIn.Result().Cookies()) != 2 {
		t.Fatal("restored root login with MFA failed")
	}
	if response := requestWithCookies(t, restoredHandler, http.MethodGet, "/api/v1/root/users", "", loggedIn.Result().Cookies(), ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), email) {
		t.Fatal("new restored root session could not access root administration")
	}
	if response := requestWithCookies(t, restoredHandler, http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous private MCP status after restore: %d", response.Code)
	}
	storedSecret, err := restored.Secret(ctx, orgID, secretID)
	if err != nil {
		t.Fatal(err)
	}
	storedEncrypted := secrets.Encrypted{Ciphertext: storedSecret.Ciphertext, Nonce: storedSecret.Nonce, Fingerprint: storedSecret.Fingerprint, KeyVersion: storedSecret.KeyVersion}
	decrypted, err := restoredVault.Decrypt(storedEncrypted, aad)
	if err != nil || string(decrypted) != plaintext {
		t.Fatal("restored credential failed authenticated decryption")
	}
	wrongKey := bytes.Repeat([]byte{0xa5}, 32)
	wrongVault, _ := secrets.New(wrongKey)
	if _, err := wrongVault.Decrypt(storedEncrypted, aad); err == nil {
		t.Fatal("wrong escrow key decrypted a credential")
	}
	wrongManager, err := auth.New(restored, auth.Config{MasterKey: wrongKey, PublicURL: "https://dokosoko.example", SessionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	wrongHandler := httpapi.NewWithOptions(restoredService, httpapi.Options{BaseURL: "https://dokosoko.example", Auth: wrongManager})
	if response := login(wrongHandler, auth.TOTP(totpSecret, time.Now().UTC())); response.Code != http.StatusUnauthorized {
		t.Fatal("wrong escrow key permitted root MFA login")
	}
	afterPublication, err := restoredService.ReadyAPIDeveloperAssetPublication(ctx, integration.ID)
	if err != nil || afterPublication.ID != publication.ID || afterPublication.SnapshotHash != publication.SnapshotHash {
		t.Fatal("restored API no longer serves the same publication")
	}
	afterQuery, err := restoredService.RunDeveloperAssetQueryLab(ctx, query)
	if err != nil || !reflect.DeepEqual(beforeQuery.Results, afterQuery.Results) {
		t.Fatal("restored exact guidance query differs from the original evidence")
	}
	storedSource, err := restored.Source(ctx, deploymentID, source.ID)
	if err != nil || storedSource.Location != source.Location {
		t.Fatal("restored upload source lost its relative location")
	}
	wantUploadHash := restoreBytesHash(uploadContent)
	checkUpload := func() bool {
		path := filepath.Join(restoredDir, storedSource.Location)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return false
		}
		body, err := os.ReadFile(path)
		return err == nil && restoreBytesHash(body) == wantUploadHash
	}
	if !checkUpload() {
		t.Fatal("restored upload bytes or permissions differ")
	}
	if err := os.Remove(filepath.Join(restoredDir, source.Location)); err != nil {
		t.Fatal(err)
	}
	if checkUpload() {
		t.Fatal("missing upload snapshot was accepted")
	}
	if err := os.WriteFile(filepath.Join(restoredDir, source.Location), []byte("corrupt snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	if checkUpload() {
		t.Fatal("corrupt upload snapshot was accepted")
	}
	if err := os.WriteFile(filepath.Join(restoredDir, source.Location), backupUpload, 0600); err != nil || !checkUpload() {
		t.Fatal("upload snapshot could not be recovered after negative checks")
	}
	archiveBytes, err := os.ReadFile(archive)
	if err != nil || len(archiveBytes) < 100 {
		t.Fatal("database archive is missing")
	}
	badArchive := filepath.Join(backupDir, "truncated.dump")
	if err := os.WriteFile(badArchive, archiveBytes[:20], 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := command(restoreBinary, targetName, "--list", badArchive); err == nil {
		t.Fatal("truncated archive was accepted as readable")
	}
	report := map[string]any{"format": "local-postgres-restore-v1", "status": "pass", "postgres_version_num": serverVersion, "pgvector_version": vectorVersion, "pg_dump": strings.TrimSpace(string(dumpVersion)), "pg_restore": strings.TrimSpace(string(restoreVersion)), "duration_ms": time.Since(started).Milliseconds(), "tables": before, "database_archive_sha256": restoreBytesHash(archiveBytes), "database_archive_bytes": len(archiveBytes), "upload_sha256": wantUploadHash, "api_publication_id": publication.ID, "api_publication_hash": publication.SnapshotHash, "guidance_results": len(afterQuery.Results), "root_mfa": "pass", "credential_decryption": "pass", "migration_replay": "pass", "anonymous_private_denial": "pass", "negative_checks": []string{"missing MFA", "wrong master key for root login", "wrong master key for credential", "missing upload", "corrupt upload", "truncated archive"}, "limits": []string{"fixture AI", "in-process HTTP handlers", "no deployed service/crawler image restart", "no production OAuth or private runtime tool call", "no production-size recovery-time claim", "no escrow service integration"}}
	if directory := os.Getenv("DOKOSOKO_ACCEPTANCE_EVIDENCE_DIR"); directory != "" {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "postgres-restore.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("restore passed: %d tables, %d guidance results; PostgreSQL %s, %s, %s", len(before), len(afterQuery.Results), serverVersion, strings.TrimSpace(string(dumpVersion)), strings.TrimSpace(string(restoreVersion)))
}

func restoreFixtureID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}

func restoreBytesHash(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

type restoreTableSnapshot struct {
	Rows   int    `json:"rows"`
	SHA256 string `json:"sha256"`
}

func restoreDatabaseSnapshot(t *testing.T, pool *pgxpool.Pool) map[string]restoreTableSnapshot {
	t.Helper()
	tables, err := pool.Query(t.Context(), `SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename`)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	tables.Close()
	if err := tables.Err(); err != nil {
		t.Fatal(err)
	}
	result := map[string]restoreTableSnapshot{}
	for _, name := range names {
		rows, err := pool.Query(t.Context(), `SELECT to_jsonb(t)::text FROM `+pgx.Identifier{"public", name}.Sanitize()+` t ORDER BY to_jsonb(t)::text`)
		if err != nil {
			t.Fatal(err)
		}
		digest, count := sha256.New(), 0
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				t.Fatal(err)
			}
			_, _ = digest.Write([]byte(value + "\n"))
			count++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		result[name] = restoreTableSnapshot{Rows: count, SHA256: "sha256:" + hex.EncodeToString(digest.Sum(nil))}
	}
	return result
}
