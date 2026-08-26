-- The AI safety policy is executable server-owned behavior, not mutable or
-- descriptive profile state. Keeping a JSON copy on each workload profile made
-- the API imply that these booleans controlled enforcement when they did not.
ALTER TABLE ai_workload_profiles
    DROP COLUMN hardening;
