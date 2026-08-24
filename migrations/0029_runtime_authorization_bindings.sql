-- Bind every Integration tool action to one exact authorization-point
-- revision and retain when the vendor access decision was evaluated. Existing
-- bindings are backfilled only when the Integration has one unambiguous active
-- point. Any unresolved draft binding is removed so the database invariant is
-- exact and future callers must explicitly choose an action before publishing.

ALTER TABLE integration_tool_bindings
    ADD COLUMN authorization_point_id uuid
        REFERENCES authorization_points(id) ON DELETE RESTRICT,
    ADD COLUMN authorization_point_revision bigint;

UPDATE integration_tool_bindings binding
SET authorization_point_id = point.id,
    authorization_point_revision = point.revision
FROM authorization_points point
WHERE point.integration_id = binding.integration_id
  AND point.state = 'active'
  AND NOT EXISTS (
      SELECT 1
      FROM authorization_points other
      WHERE other.integration_id = binding.integration_id
        AND other.state = 'active'
        AND other.id <> point.id
  );

DELETE FROM integration_tool_bindings
WHERE authorization_point_id IS NULL
   OR authorization_point_revision IS NULL;

ALTER TABLE integration_tool_bindings
    ALTER COLUMN authorization_point_id SET NOT NULL,
    ALTER COLUMN authorization_point_revision SET NOT NULL,
    ADD CONSTRAINT integration_tool_bindings_authorization_pair_check
    CHECK (authorization_point_revision > 0);

CREATE INDEX integration_tool_bindings_authorization_idx
    ON integration_tool_bindings(
        integration_id,
        authorization_point_id,
        authorization_point_revision
    );

ALTER TABLE oauth_authorization_codes
    ADD COLUMN access_evaluated_at timestamptz;

-- Existing rows did not retain the evaluator timestamp. Use an explicitly
-- expired sentinel so an unknown-age decision can never authorize a managed
-- Integration action; the user must complete a new access evaluation.
UPDATE oauth_authorization_codes
SET access_evaluated_at = '1970-01-01T00:00:00Z'::timestamptz
WHERE access_evaluated_at IS NULL;

ALTER TABLE oauth_authorization_codes
    ALTER COLUMN access_evaluated_at SET NOT NULL;

ALTER TABLE oauth_access_tokens
    ADD COLUMN access_evaluated_at timestamptz;

-- Tokens created before this migration likewise fail closed for managed
-- Integration actions while remaining usable for legacy, unbound tools.
UPDATE oauth_access_tokens
SET access_evaluated_at = '1970-01-01T00:00:00Z'::timestamptz
WHERE access_evaluated_at IS NULL;

ALTER TABLE oauth_access_tokens
    ALTER COLUMN access_evaluated_at SET NOT NULL;
