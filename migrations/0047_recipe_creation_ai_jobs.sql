-- The console's manual "Add recipe" flow records a distinct creation job so
-- it can be audited separately from bulk evidence refreshes.
ALTER TABLE ai_jobs
    DROP CONSTRAINT ai_jobs_kind_check;

ALTER TABLE ai_jobs
    ADD CONSTRAINT ai_jobs_kind_check
    CHECK (kind IN ('integration_analysis','recipe_creation','recipe_generation','recipe_rework','recipe_review'));
