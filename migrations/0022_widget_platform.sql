-- Authenticated widget delivery. Raw widget credentials and session tokens are
-- never persisted; only SHA-256 digests and non-secret fingerprints are kept.

-- The original public/private snippet experiment never shipped as a runtime
-- dependency. Remove its parallel configuration model before installing the
-- authenticated widget platform so there is only one source of truth.
DROP TABLE IF EXISTS widget_configurations;

CREATE TABLE widgets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft','active','disabled')),
    allowed_origins text[] NOT NULL DEFAULT '{}',
    integration_ids uuid[] NOT NULL DEFAULT '{}',
    appearance jsonb NOT NULL DEFAULT '{"theme":"auto","launcherPosition":"right"}'::jsonb,
    revision bigint NOT NULL DEFAULT 1,
    activated_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(deployment_id, name)
);

CREATE INDEX widgets_deployment_state_idx ON widgets(deployment_id, state, created_at);

CREATE TABLE widget_secrets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    widget_id uuid NOT NULL REFERENCES widgets(id) ON DELETE CASCADE,
    secret_digest bytea NOT NULL UNIQUE,
    fingerprint text NOT NULL,
    last_used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX widget_secrets_active_idx ON widget_secrets(widget_id, created_at DESC) WHERE revoked_at IS NULL;

CREATE TABLE widget_bootstrap_tokens (
    token_digest bytea PRIMARY KEY,
    widget_id uuid NOT NULL REFERENCES widgets(id) ON DELETE CASCADE,
    user_id text NOT NULL CHECK (length(user_id) BETWEEN 1 AND 255),
    customer_organisation_id text NOT NULL DEFAULT '' CHECK (length(customer_organisation_id) <= 255),
    origin text NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX widget_bootstrap_expiry_idx ON widget_bootstrap_tokens(expires_at) WHERE used_at IS NULL;

CREATE TABLE widget_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    widget_id uuid NOT NULL REFERENCES widgets(id) ON DELETE CASCADE,
    token_digest bytea NOT NULL UNIQUE,
    user_id text NOT NULL,
    customer_organisation_id text NOT NULL DEFAULT '',
    origin text NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    last_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX widget_sessions_widget_expiry_idx ON widget_sessions(widget_id, expires_at DESC);
CREATE INDEX widget_sessions_expiry_idx ON widget_sessions(expires_at) WHERE revoked_at IS NULL;
