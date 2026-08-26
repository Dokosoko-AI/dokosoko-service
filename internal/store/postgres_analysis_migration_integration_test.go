package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	secretvault "github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/jackc/pgx/v5/pgxpool"
)

func copyMigrationsThrough(t *testing.T, maximum int) string {
	t.Helper()
	destination := t.TempDir()
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") || len(name) < 4 {
			continue
		}
		sequence, err := strconv.Atoi(name[:4])
		if err != nil || sequence > maximum {
			continue
		}
		content, err := os.ReadFile(filepath.Join("../../migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destination, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return destination
}

func TestPostgresAnalysisOnlyMigrationPreservesAssistantOnlyConfiguration(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("DOKOSOKO_TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("DOKOSOKO_TEST_DATABASE_URL or TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Skipf("PostgreSQL is unavailable: %v", err)
	}
	t.Cleanup(admin.Close)

	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("analysis_migration_%x", random)
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Skipf("cannot create isolated PostgreSQL schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = admin.Exec(cleanupCtx, `DROP SCHEMA `+schema+` CASCADE`)
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

	if err := Migrate(ctx, pool, copyMigrationsThrough(t, 16)); err != nil {
		t.Fatalf("migrate through 0016: %v", err)
	}
	organisationID := storeTestUUID(t)
	productID := storeTestUUID(t)
	secretID := storeTestUUID(t)
	profileID := storeTestUUID(t)
	vault, err := secretvault.New(bytes.Repeat([]byte{0x51}, 32))
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := vault.Encrypt([]byte("assistant-credential"), organisationID+":llm:"+secretID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organisations(id,name,slug) VALUES ($1,'Assistant upgrade','assistant-upgrade')`, organisationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO products(id,organisation_id,name,slug,description) VALUES ($1,$2,'Assistant upgrade','assistant-upgrade','Upgrade fixture')`, productID, organisationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO secrets(id,organisation_id,name,purpose,ciphertext,nonce,key_version,fingerprint) VALUES ($1,$2,'assistant-key','llm_provider',$3,$4,$5,$6)`, secretID, organisationID, encrypted.Ciphertext, encrypted.Nonce, encrypted.KeyVersion, encrypted.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO llm_profiles(id,organisation_id,product_id,role,provider,endpoint,model,credential_secret_id,max_input_tokens,max_output_tokens,daily_token_budget,hardening,enabled) VALUES ($1,$2,$3,'assistant','openai','https://api.openai.com','assistant-only-model',$4,8192,1024,10000,'{"context_is_untrusted":true,"migration_fixture":"keep"}'::jsonb,true)`, profileID, organisationID, productID, secretID); err != nil {
		t.Fatal(err)
	}

	if err := Migrate(ctx, pool, copyMigrationsThrough(t, 50)); err != nil {
		t.Fatalf("migrate through 0050: %v", err)
	}
	var (
		profileBeforeID, connectionBeforeID, assistantModel, hardeningBefore string
		maxInputBefore, maxOutputBefore                                      int
		dailyBudgetBefore, profileRevisionBefore                             int64
		profileEnabledBefore                                                 bool
		profileCreatedBefore, profileUpdatedBefore                           time.Time
	)
	if err := pool.QueryRow(ctx, `SELECT id::text,provider_connection_id::text,model,max_input_tokens,max_output_tokens,daily_token_budget,hardening::text,enabled,revision,created_at,updated_at FROM ai_workload_profiles WHERE product_id=$1 AND workload='assistant'`, productID).Scan(&profileBeforeID, &connectionBeforeID, &assistantModel, &maxInputBefore, &maxOutputBefore, &dailyBudgetBefore, &hardeningBefore, &profileEnabledBefore, &profileRevisionBefore, &profileCreatedBefore, &profileUpdatedBefore); err != nil {
		t.Fatalf("assistant-only profile was not present before 0051: %v", err)
	}
	if assistantModel != "assistant-only-model" {
		t.Fatalf("assistant model = %q", assistantModel)
	}
	var (
		credentialBeforeID, providerBefore, endpointBefore, managedByBefore string
		connectionEnabledBefore                                             bool
		connectionRevisionBefore                                            int64
		connectionCreatedBefore, connectionUpdatedBefore                    time.Time
	)
	if err := pool.QueryRow(ctx, `SELECT credential_secret_id::text,provider,endpoint,managed_by,enabled,revision,created_at,updated_at FROM ai_provider_connections WHERE id=$1`, connectionBeforeID).Scan(&credentialBeforeID, &providerBefore, &endpointBefore, &managedByBefore, &connectionEnabledBefore, &connectionRevisionBefore, &connectionCreatedBefore, &connectionUpdatedBefore); err != nil {
		t.Fatal(err)
	}
	if credentialBeforeID != secretID {
		t.Fatalf("migrated provider credential = %q, want %q", credentialBeforeID, secretID)
	}

	// The former backup constraint permitted extra keys. Keep the configured
	// Analysis model and credential while proving 0051 canonicalises that shape.
	backupSecretID, backupConnectionID := storeTestUUID(t), storeTestUUID(t)
	backupEncrypted, err := vault.Encrypt([]byte("backup-credential"), organisationID+":ai:"+backupSecretID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO secrets(id,organisation_id,name,purpose,ciphertext,nonce,key_version,fingerprint) VALUES ($1,$2,'backup-key','ai_provider',$3,$4,$5,$6)`, backupSecretID, organisationID, backupEncrypted.Ciphertext, backupEncrypted.Nonce, backupEncrypted.KeyVersion, backupEncrypted.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_provider_connections(id,organisation_id,deployment_id,provider,endpoint,credential_secret_id,managed_by,enabled,is_backup,backup_models) VALUES ($1,$2,$3,'anthropic','https://api.anthropic.com',$4,'console',true,true,'{"analysis":"backup-analysis-model","assistant":"backup-assistant-model","obsolete":"remove-me"}'::jsonb)`, backupConnectionID, organisationID, productID, backupSecretID); err != nil {
		t.Fatal(err)
	}

	day := time.Date(2026, time.August, 25, 0, 0, 0, 0, time.UTC)
	analysisBudgetUpdated := time.Date(2026, time.August, 25, 8, 0, 0, 0, time.UTC)
	assistantBudgetUpdated := analysisBudgetUpdated.Add(time.Hour)
	if _, err := pool.Exec(ctx, `INSERT INTO ai_budget_days(product_id,workload,day,used_tokens,updated_at) VALUES ($1,'analysis',$2,7,$3),($1,'assistant',$2,11,$4)`, productID, day, analysisBudgetUpdated, assistantBudgetUpdated); err != nil {
		t.Fatal(err)
	}
	reservationID := storeTestUUID(t)
	reservationCreated := time.Date(2026, time.August, 25, 9, 15, 0, 0, time.UTC)
	reservationExpires := time.Date(2099, time.August, 25, 9, 17, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO ai_budget_reservations(id,product_id,workload,day,reserved_tokens,expires_at,created_at) VALUES ($1,$2,'assistant',$3,13,$4,$5)`, reservationID, productID, day, reservationExpires, reservationCreated); err != nil {
		t.Fatal(err)
	}
	usageEventID := storeTestUUID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO ai_usage_events(id,organisation_id,product_id,workload,action,provider,requested_model,input_tokens,output_tokens,outcome,prompt_version,created_at) VALUES ($1,$2,$3,'assistant','legacy_assistant','openai','assistant-only-model',3,5,'succeeded','assistant-v1',$4)`, usageEventID, organisationID, productID, assistantBudgetUpdated); err != nil {
		t.Fatal(err)
	}

	if err := Migrate(ctx, pool, copyMigrationsThrough(t, 54)); err != nil {
		t.Fatalf("migrate through 0054: %v", err)
	}
	var (
		profileAfterID, connectionAfterID, workload, model, hardeningAfter string
		maxInputAfter, maxOutputAfter                                      int
		dailyBudgetAfter, profileRevisionAfter                             int64
		profileEnabledAfter                                                bool
		profileCreatedAfter, profileUpdatedAfter                           time.Time
	)
	if err := pool.QueryRow(ctx, `SELECT id::text,provider_connection_id::text,workload,model,max_input_tokens,max_output_tokens,daily_token_budget,hardening::text,enabled,revision,created_at,updated_at FROM ai_workload_profiles WHERE product_id=$1`, productID).Scan(&profileAfterID, &connectionAfterID, &workload, &model, &maxInputAfter, &maxOutputAfter, &dailyBudgetAfter, &hardeningAfter, &profileEnabledAfter, &profileRevisionAfter, &profileCreatedAfter, &profileUpdatedAfter); err != nil {
		t.Fatal(err)
	}
	if workload != "analysis" || model != "assistant-only-model" {
		t.Fatalf("promoted profile = workload %q model %q", workload, model)
	}
	if profileAfterID != profileBeforeID || connectionAfterID != connectionBeforeID || maxInputAfter != maxInputBefore || maxOutputAfter != maxOutputBefore || dailyBudgetAfter != dailyBudgetBefore || hardeningAfter != hardeningBefore || profileEnabledAfter != profileEnabledBefore || profileRevisionAfter != profileRevisionBefore || !profileCreatedAfter.Equal(profileCreatedBefore) || !profileUpdatedAfter.Equal(profileUpdatedBefore) {
		t.Fatalf("promoted profile did not retain its identity and configuration: before id=%q connection=%q input=%d output=%d budget=%d hardening=%s enabled=%t revision=%d created=%s updated=%s; after id=%q connection=%q input=%d output=%d budget=%d hardening=%s enabled=%t revision=%d created=%s updated=%s", profileBeforeID, connectionBeforeID, maxInputBefore, maxOutputBefore, dailyBudgetBefore, hardeningBefore, profileEnabledBefore, profileRevisionBefore, profileCreatedBefore, profileUpdatedBefore, profileAfterID, connectionAfterID, maxInputAfter, maxOutputAfter, dailyBudgetAfter, hardeningAfter, profileEnabledAfter, profileRevisionAfter, profileCreatedAfter, profileUpdatedAfter)
	}

	var (
		credentialAfterID, providerAfter, endpointAfter, managedByAfter string
		connectionEnabledAfter                                          bool
		connectionRevisionAfter                                         int64
		connectionCreatedAfter, connectionUpdatedAfter                  time.Time
	)
	if err := pool.QueryRow(ctx, `SELECT credential_secret_id::text,provider,endpoint,managed_by,enabled,revision,created_at,updated_at FROM ai_provider_connections WHERE id=$1`, connectionAfterID).Scan(&credentialAfterID, &providerAfter, &endpointAfter, &managedByAfter, &connectionEnabledAfter, &connectionRevisionAfter, &connectionCreatedAfter, &connectionUpdatedAfter); err != nil {
		t.Fatal(err)
	}
	if credentialAfterID != credentialBeforeID || providerAfter != providerBefore || endpointAfter != endpointBefore || managedByAfter != managedByBefore || connectionEnabledAfter != connectionEnabledBefore || connectionRevisionAfter != connectionRevisionBefore || !connectionCreatedAfter.Equal(connectionCreatedBefore) || !connectionUpdatedAfter.Equal(connectionUpdatedBefore) {
		t.Fatalf("provider connection changed while promoting its profile")
	}
	var storedCredential struct {
		ciphertext  []byte
		nonce       []byte
		keyVersion  int
		fingerprint string
	}
	if err := pool.QueryRow(ctx, `SELECT ciphertext,nonce,key_version,fingerprint FROM secrets WHERE id=$1`, secretID).Scan(&storedCredential.ciphertext, &storedCredential.nonce, &storedCredential.keyVersion, &storedCredential.fingerprint); err != nil {
		t.Fatalf("promoted credential was not retained: %v", err)
	}
	plaintext, err := vault.Decrypt(secretvault.Encrypted{Ciphertext: storedCredential.ciphertext, Nonce: storedCredential.nonce, KeyVersion: storedCredential.keyVersion, Fingerprint: storedCredential.fingerprint}, organisationID+":llm:"+secretID)
	if err != nil || !bytes.Equal(plaintext, []byte("assistant-credential")) {
		t.Fatalf("promoted credential no longer decrypts with its original context: plaintext=%q err=%v", plaintext, err)
	}

	var backupModelsJSON []byte
	var backupCredentialAfter string
	if err := pool.QueryRow(ctx, `SELECT backup_models,credential_secret_id::text FROM ai_provider_connections WHERE id=$1`, backupConnectionID).Scan(&backupModelsJSON, &backupCredentialAfter); err != nil {
		t.Fatal(err)
	}
	var backupModels map[string]string
	if err := json.Unmarshal(backupModelsJSON, &backupModels); err != nil {
		t.Fatal(err)
	}
	if backupCredentialAfter != backupSecretID || len(backupModels) != 1 || backupModels["analysis"] != "backup-analysis-model" {
		t.Fatalf("canonical backup = credential %q models %#v", backupCredentialAfter, backupModels)
	}

	var usedTokens int64
	var budgetUpdated time.Time
	if err := pool.QueryRow(ctx, `SELECT used_tokens,updated_at FROM ai_budget_days WHERE product_id=$1 AND workload='analysis' AND day=$2`, productID, day).Scan(&usedTokens, &budgetUpdated); err != nil {
		t.Fatal(err)
	}
	if usedTokens != 18 || !budgetUpdated.Equal(assistantBudgetUpdated) {
		t.Fatalf("merged Analysis budget = tokens %d updated %s, want 18 and %s", usedTokens, budgetUpdated, assistantBudgetUpdated)
	}
	var reservedTokens int64
	var reservationDay, reservationExpiresAfter, reservationCreatedAfter time.Time
	if err := pool.QueryRow(ctx, `SELECT workload,day,reserved_tokens,expires_at,created_at FROM ai_budget_reservations WHERE id=$1`, reservationID).Scan(&workload, &reservationDay, &reservedTokens, &reservationExpiresAfter, &reservationCreatedAfter); err != nil {
		t.Fatal(err)
	}
	if workload != "analysis" || !reservationDay.Equal(day) || reservedTokens != 13 || !reservationExpiresAfter.Equal(reservationExpires) || !reservationCreatedAfter.Equal(reservationCreated) {
		t.Fatalf("promoted reservation = workload %q day %s tokens %d expires %s created %s", workload, reservationDay, reservedTokens, reservationExpiresAfter, reservationCreatedAfter)
	}
	if err := pool.QueryRow(ctx, `SELECT workload FROM ai_usage_events WHERE id=$1`, usageEventID).Scan(&workload); err != nil {
		t.Fatal(err)
	}
	if workload != "assistant" {
		t.Fatalf("historical Assistant usage workload = %q", workload)
	}

	var legacyTablesDropped, redundantTablesDropped, analyticsFunctionDropped bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.llm_profiles') IS NULL`).Scan(&legacyTablesDropped); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.ai_jobs') IS NULL AND to_regclass(current_schema() || '.analytics_events') IS NULL`).Scan(&redundantTablesDropped); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regprocedure(current_schema() || '.prevent_analytics_mutation()') IS NULL`).Scan(&analyticsFunctionDropped); err != nil {
		t.Fatal(err)
	}
	if !legacyTablesDropped || !redundantTablesDropped || !analyticsFunctionDropped {
		t.Fatalf("removed schema remains: llm_profiles=%t redundant_tables=%t analytics_function=%t", !legacyTablesDropped, !redundantTablesDropped, !analyticsFunctionDropped)
	}
	var promptRevision int64
	if err := pool.QueryRow(ctx, `INSERT INTO ai_prompt_settings(product_id,prompt_key,instructions) VALUES ($1,'integration.analysis','Ground every claim in reviewed evidence.') RETURNING revision`, productID).Scan(&promptRevision); err != nil {
		t.Fatalf("0054 prompt settings contract is unusable: %v", err)
	}
	if promptRevision != 2 {
		t.Fatalf("initial prompt revision = %d, want 2", promptRevision)
	}

	if err := Migrate(ctx, pool, copyMigrationsThrough(t, 55)); err != nil {
		t.Fatalf("migrate through 0055: %v", err)
	}
	var hardeningColumnDropped bool
	if err := pool.QueryRow(ctx, `SELECT NOT EXISTS (
		SELECT 1
		FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND table_name='ai_workload_profiles'
		  AND column_name='hardening'
	)`).Scan(&hardeningColumnDropped); err != nil {
		t.Fatal(err)
	}
	if !hardeningColumnDropped {
		t.Fatal("0055 retained the inert ai_workload_profiles.hardening column")
	}
}
