-- Analysis is the only active AI workload. Historical Assistant usage events
-- remain available for operational history, but no active profile, budget, or
-- backup model can target that retired workload.

-- Preserve a deployment that configured only the former Assistant workload.
-- Where Analysis already exists it remains authoritative; otherwise the
-- Assistant profile becomes the Analysis profile without moving credentials.
UPDATE ai_workload_profiles AS assistant
SET workload = 'analysis'
WHERE assistant.workload = 'assistant'
  AND NOT EXISTS (
      SELECT 1
      FROM ai_workload_profiles AS analysis
      WHERE analysis.product_id = assistant.product_id
        AND analysis.workload = 'analysis'
  );
DELETE FROM ai_workload_profiles WHERE workload = 'assistant';

-- Count tokens already spent by either former workload against today's single
-- Analysis budget. In-flight reservations retain their IDs and expiry.
INSERT INTO ai_budget_days(product_id, workload, day, used_tokens, updated_at)
SELECT product_id, 'analysis', day, used_tokens, updated_at
FROM ai_budget_days
WHERE workload = 'assistant'
ON CONFLICT (product_id, workload, day) DO UPDATE
SET used_tokens = ai_budget_days.used_tokens + EXCLUDED.used_tokens,
    updated_at = GREATEST(ai_budget_days.updated_at, EXCLUDED.updated_at);
DELETE FROM ai_budget_days WHERE workload = 'assistant';
UPDATE ai_budget_reservations SET workload = 'analysis' WHERE workload = 'assistant';

ALTER TABLE ai_workload_profiles
    DROP CONSTRAINT ai_workload_profiles_workload_check;
ALTER TABLE ai_workload_profiles
    ADD CONSTRAINT ai_workload_profiles_workload_check
    CHECK (workload = 'analysis');

ALTER TABLE ai_budget_days
    DROP CONSTRAINT ai_budget_days_workload_check;
ALTER TABLE ai_budget_days
    ADD CONSTRAINT ai_budget_days_workload_check
    CHECK (workload = 'analysis');

ALTER TABLE ai_budget_reservations
    DROP CONSTRAINT ai_budget_reservations_workload_check;
ALTER TABLE ai_budget_reservations
    ADD CONSTRAINT ai_budget_reservations_workload_check
    CHECK (workload = 'analysis');

ALTER TABLE ai_provider_connections
    DROP CONSTRAINT ai_provider_connection_backup_models;

-- Rebuild the backup mapping rather than merely deleting Assistant. The old
-- constraint required both supported keys but allowed arbitrary extra keys;
-- canonicalising here makes every previously valid backup satisfy the tighter
-- Analysis-only shape while retaining its configured Analysis model.
UPDATE ai_provider_connections
SET backup_models = jsonb_build_object(
    'analysis', backup_models->>'analysis'
)
WHERE is_backup;

UPDATE ai_provider_connections
SET backup_models = '{}'::jsonb
WHERE NOT is_backup;

ALTER TABLE ai_provider_connections
    ADD CONSTRAINT ai_provider_connection_backup_models CHECK (
        jsonb_typeof(backup_models) = 'object'
        AND (backup_models - 'analysis') = '{}'::jsonb
        AND (is_backup OR backup_models = '{}'::jsonb)
        AND (NOT is_backup OR
            nullif(trim(backup_models->>'analysis'), '') IS NOT NULL)
    );
