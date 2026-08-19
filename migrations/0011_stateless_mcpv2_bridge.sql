CREATE TABLE mcp_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name text NOT NULL,
    namespace text NOT NULL,
    endpoint text NOT NULL,
    protocol_version text NOT NULL DEFAULT '2026-07-28' CHECK (protocol_version = '2026-07-28'),
    auth_mode text NOT NULL CHECK (auth_mode IN ('none', 'service', 'delegated_oauth')),
    credential_secret_id uuid REFERENCES secrets(id) ON DELETE RESTRICT,
    oauth_client_id text NOT NULL DEFAULT '',
    oauth_client_secret_id uuid REFERENCES secrets(id) ON DELETE RESTRICT,
    oauth_issuer text NOT NULL DEFAULT '',
    authorization_url text NOT NULL DEFAULT '',
    token_url text NOT NULL DEFAULT '',
    scopes text[] NOT NULL DEFAULT '{}',
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'disabled')),
    last_synced_at timestamptz,
    last_catalog_hash text NOT NULL DEFAULT '',
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id, name),
    UNIQUE (product_id, namespace),
    UNIQUE (product_id, endpoint)
);

CREATE TABLE mcp_user_grants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    connection_id uuid NOT NULL REFERENCES mcp_connections(id) ON DELETE CASCADE,
    subject_id text NOT NULL,
    upstream_subject text NOT NULL DEFAULT '',
    access_secret_id uuid NOT NULL REFERENCES secrets(id) ON DELETE RESTRICT,
    refresh_secret_id uuid REFERENCES secrets(id) ON DELETE RESTRICT,
    scopes text[] NOT NULL DEFAULT '{}',
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (connection_id, subject_id)
);

CREATE TABLE mcp_authorization_states (
    digest bytea PRIMARY KEY,
    connection_id uuid NOT NULL REFERENCES mcp_connections(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    subject_id text NOT NULL,
    code_verifier text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE tool_definitions
    ADD COLUMN backend_kind text NOT NULL DEFAULT 'http' CHECK (backend_kind IN ('http', 'mcp')),
    ADD COLUMN mcp_connection_id uuid REFERENCES mcp_connections(id) ON DELETE RESTRICT,
    ADD COLUMN upstream_tool_name text NOT NULL DEFAULT '',
    ADD COLUMN upstream_schema_hash text NOT NULL DEFAULT '',
    ADD COLUMN upstream_annotations jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN upstream_drifted boolean NOT NULL DEFAULT false;

ALTER TABLE tool_releases
    ALTER COLUMN api_connection_id DROP NOT NULL,
    ADD COLUMN backend_kind text NOT NULL DEFAULT 'http' CHECK (backend_kind IN ('http', 'mcp')),
    ADD COLUMN mcp_connection_id uuid REFERENCES mcp_connections(id) ON DELETE RESTRICT,
    ADD COLUMN upstream_tool_name text NOT NULL DEFAULT '',
    ADD COLUMN upstream_schema_hash text NOT NULL DEFAULT '';

ALTER TABLE tool_definitions
    ADD CONSTRAINT tool_backend_configuration CHECK (
        (backend_kind = 'http' AND api_connection_id IS NOT NULL AND mcp_connection_id IS NULL)
        OR
        (backend_kind = 'mcp' AND api_connection_id IS NULL AND mcp_connection_id IS NOT NULL AND upstream_tool_name <> '' AND upstream_schema_hash <> '')
    );

ALTER TABLE tool_releases
    ADD CONSTRAINT tool_release_backend_configuration CHECK (
        (backend_kind = 'http' AND api_connection_id IS NOT NULL AND mcp_connection_id IS NULL)
        OR
        (backend_kind = 'mcp' AND api_connection_id IS NULL AND mcp_connection_id IS NOT NULL AND upstream_tool_name <> '' AND upstream_schema_hash <> '')
    );

CREATE INDEX mcp_user_grants_subject_idx ON mcp_user_grants(connection_id, subject_id) WHERE revoked_at IS NULL;
CREATE INDEX mcp_authorization_states_expiry_idx ON mcp_authorization_states(expires_at);
CREATE INDEX tool_definitions_mcp_connection_idx ON tool_definitions(mcp_connection_id) WHERE backend_kind = 'mcp';
