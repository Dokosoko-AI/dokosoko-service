-- Keep the product model intentionally small: one strong build-time workload
-- and one fast runtime workload. Historical usage is preserved under the new
-- names instead of exposing the implementation's former four-way split.

ALTER TABLE ai_workload_profiles DROP CONSTRAINT IF EXISTS ai_workload_profiles_workload_check;
ALTER TABLE ai_budget_days DROP CONSTRAINT IF EXISTS ai_budget_days_workload_check;
ALTER TABLE ai_budget_reservations DROP CONSTRAINT IF EXISTS ai_budget_reservations_workload_check;
ALTER TABLE ai_usage_events DROP CONSTRAINT IF EXISTS ai_usage_events_workload_check;

CREATE TEMP TABLE migrated_ai_workload_profiles ON COMMIT DROP AS
SELECT DISTINCT ON (product_id, mapped_workload)
    id,
    organisation_id,
    product_id,
    mapped_workload AS workload,
    provider_connection_id,
    model,
    max_input_tokens,
    max_output_tokens,
    daily_token_budget,
    hardening,
    enabled,
    revision,
    created_at,
    updated_at
FROM (
    SELECT
        profile.*,
        CASE WHEN workload = 'support' THEN 'assistant' ELSE 'analysis' END AS mapped_workload,
        CASE workload
            WHEN 'review' THEN 1
            WHEN 'authoring' THEN 2
            WHEN 'extraction' THEN 3
            WHEN 'support' THEN 1
            ELSE 4
        END AS migration_priority
    FROM ai_workload_profiles profile
) ranked
ORDER BY product_id, mapped_workload, migration_priority, updated_at DESC;

DELETE FROM ai_workload_profiles;
INSERT INTO ai_workload_profiles(
    id, organisation_id, product_id, workload, provider_connection_id, model,
    max_input_tokens, max_output_tokens, daily_token_budget, hardening,
    enabled, revision, created_at, updated_at
)
SELECT
    id, organisation_id, product_id, workload, provider_connection_id, model,
    max_input_tokens, max_output_tokens, daily_token_budget, hardening,
    enabled, revision, created_at, updated_at
FROM migrated_ai_workload_profiles;

CREATE TEMP TABLE migrated_ai_budget_days ON COMMIT DROP AS
SELECT
    product_id,
    CASE WHEN workload = 'support' THEN 'assistant' ELSE 'analysis' END AS workload,
    day,
    sum(used_tokens) AS used_tokens,
    max(updated_at) AS updated_at
FROM ai_budget_days
GROUP BY product_id, CASE WHEN workload = 'support' THEN 'assistant' ELSE 'analysis' END, day;

DELETE FROM ai_budget_days;
INSERT INTO ai_budget_days(product_id, workload, day, used_tokens, updated_at)
SELECT product_id, workload, day, used_tokens, updated_at
FROM migrated_ai_budget_days;

UPDATE ai_budget_reservations
SET workload = CASE WHEN workload = 'support' THEN 'assistant' ELSE 'analysis' END;

UPDATE ai_usage_events
SET workload = CASE WHEN workload = 'support' THEN 'assistant' ELSE 'analysis' END;

ALTER TABLE ai_workload_profiles
    ADD CONSTRAINT ai_workload_profiles_workload_check
    CHECK (workload IN ('analysis','assistant'));
ALTER TABLE ai_budget_days
    ADD CONSTRAINT ai_budget_days_workload_check
    CHECK (workload IN ('analysis','assistant'));
ALTER TABLE ai_budget_reservations
    ADD CONSTRAINT ai_budget_reservations_workload_check
    CHECK (workload IN ('analysis','assistant'));
ALTER TABLE ai_usage_events
    ADD CONSTRAINT ai_usage_events_workload_check
    CHECK (workload IN ('analysis','assistant'));

-- A deployment can nominate one provider as its transparent transient-error
-- fallback. Models are provider-specific, so both workload model IDs live on
-- the backup connection instead of being guessed at runtime.
ALTER TABLE ai_provider_connections
    ADD COLUMN is_backup boolean NOT NULL DEFAULT false,
    ADD COLUMN backup_models jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT ai_provider_connection_backup_models CHECK (
        jsonb_typeof(backup_models) = 'object'
        AND (NOT is_backup OR (
            nullif(trim(backup_models->>'analysis'), '') IS NOT NULL
            AND nullif(trim(backup_models->>'assistant'), '') IS NOT NULL
        ))
    );

CREATE UNIQUE INDEX ai_provider_connections_one_backup_idx
    ON ai_provider_connections(deployment_id)
    WHERE is_backup;

ALTER TABLE ai_usage_events
    ADD COLUMN provider_role text NOT NULL DEFAULT 'primary'
        CHECK (provider_role IN ('primary','backup')),
    ADD COLUMN fallback_reason text NOT NULL DEFAULT '';
