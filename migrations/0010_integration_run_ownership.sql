ALTER TABLE integration_runs
    ADD COLUMN actor_pseudonym text NOT NULL DEFAULT '';

CREATE INDEX integration_runs_owner_idx
    ON integration_runs(product_id, actor_pseudonym, started_at DESC);
