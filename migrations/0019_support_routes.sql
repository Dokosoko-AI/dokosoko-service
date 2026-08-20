CREATE TABLE support_routes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    name text NOT NULL,
    is_default boolean NOT NULL DEFAULT false,
    bug_reports_enabled boolean NOT NULL DEFAULT false,
    feedback_enabled boolean NOT NULL DEFAULT false,
    bug_hook_url text NOT NULL DEFAULT '',
    bug_hook_credential_id uuid REFERENCES secrets(id) ON DELETE RESTRICT,
    feedback_hook_url text NOT NULL DEFAULT '',
    feedback_hook_credential_id uuid REFERENCES secrets(id) ON DELETE RESTRICT,
    retention_days integer NOT NULL DEFAULT 30 CHECK (retention_days BETWEEN 1 AND 365),
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active','archived')),
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (deployment_id, name)
);
CREATE UNIQUE INDEX support_routes_one_default_idx
    ON support_routes(deployment_id) WHERE is_default AND state = 'active';

CREATE TABLE integration_support_bindings (
    integration_id uuid PRIMARY KEY REFERENCES integrations(id) ON DELETE CASCADE,
    support_route_id uuid NOT NULL REFERENCES support_routes(id) ON DELETE RESTRICT,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE report_submissions
    ADD COLUMN integration_id uuid REFERENCES integrations(id) ON DELETE SET NULL,
    ADD COLUMN integration_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN support_route_id uuid REFERENCES support_routes(id) ON DELETE SET NULL;

CREATE INDEX report_submissions_integration_created_idx
    ON report_submissions(integration_id, created_at DESC)
    WHERE integration_id IS NOT NULL;

INSERT INTO support_routes(
    id, deployment_id, organisation_id, name, is_default,
    bug_reports_enabled, feedback_enabled, bug_hook_url,
    bug_hook_credential_id, feedback_hook_url,
    feedback_hook_credential_id, retention_days, revision,
    created_at, updated_at
)
SELECT
    config.id,
    config.product_id,
    config.organisation_id,
    'Default support route',
    true,
    config.bug_reports_enabled,
    config.feedback_enabled,
    config.bug_hook_url,
    config.bug_hook_credential_id,
    config.feedback_hook_url,
    config.feedback_hook_credential_id,
    config.retention_days,
    config.revision,
    config.created_at,
    config.updated_at
FROM reporting_configs config
ON CONFLICT (id) DO NOTHING;

UPDATE report_submissions submission
SET support_route_id = route.id
FROM support_routes route
WHERE route.deployment_id = submission.product_id
  AND route.is_default
  AND submission.support_route_id IS NULL;
