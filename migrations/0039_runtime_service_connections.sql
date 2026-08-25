-- Runtime delivery configuration is separate from management-plane access
-- connections and from legacy per-tool api_connections. Connections are
-- API-owned; credentials may be dedicated to an API or shared deliberately.

CREATE TABLE runtime_service_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active','disabled')),
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (integration_id, name),
    UNIQUE (id, deployment_id, organisation_id)
);
CREATE INDEX runtime_service_connections_integration_idx
    ON runtime_service_connections(integration_id, state, name);

CREATE TABLE runtime_credential_sets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    scope text NOT NULL CHECK (scope IN ('dedicated','shared')),
    owner_integration_id uuid REFERENCES integrations(id) ON DELETE CASCADE,
    name text NOT NULL,
    environment_variable text NOT NULL,
    authentication_type text NOT NULL CHECK (
        authentication_type IN (
            'bearer',
            'authorization_scheme',
            'api_key_header',
            'api_key_query',
            'basic',
            'oauth_client_credentials',
            'custom_header'
        )
    ),
    header_name text NOT NULL DEFAULT '',
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active','disabled')),
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT runtime_credential_sets_scope_owner_check CHECK (
        (scope = 'shared' AND owner_integration_id IS NULL)
        OR (scope = 'dedicated' AND owner_integration_id IS NOT NULL)
    ),
    UNIQUE (deployment_id, environment_id, name),
    UNIQUE (deployment_id, environment_id, environment_variable),
    UNIQUE (id, deployment_id, organisation_id)
);
CREATE INDEX runtime_credential_sets_owner_idx
    ON runtime_credential_sets(deployment_id, environment_id, owner_integration_id, scope);

CREATE TABLE runtime_credential_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    credential_set_id uuid NOT NULL REFERENCES runtime_credential_sets(id) ON DELETE CASCADE,
    secret_id uuid NOT NULL REFERENCES secrets(id) ON DELETE RESTRICT,
    fingerprint text NOT NULL,
    state text NOT NULL CHECK (state IN ('pending','active','retiring','revoked','expired')),
    created_by uuid REFERENCES root_users(user_id) ON DELETE SET NULL,
    activated_at timestamptz,
    retires_at timestamptz,
    revoked_at timestamptz,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT runtime_credential_versions_state_shape CHECK (
        (state = 'active' AND activated_at IS NOT NULL AND revoked_at IS NULL)
        OR (state = 'revoked' AND revoked_at IS NOT NULL)
        OR state IN ('pending','retiring','expired')
    )
);
CREATE UNIQUE INDEX runtime_credential_versions_one_active_idx
    ON runtime_credential_versions(credential_set_id) WHERE state = 'active';
CREATE INDEX runtime_credential_versions_set_created_idx
    ON runtime_credential_versions(credential_set_id, created_at DESC);

CREATE TABLE runtime_service_connection_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    connection_id uuid NOT NULL REFERENCES runtime_service_connections(id) ON DELETE CASCADE,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    base_url text NOT NULL,
    authentication_type text NOT NULL CHECK (
        authentication_type IN (
            'none',
            'delegated_oauth',
            'bearer',
            'authorization_scheme',
            'api_key_header',
            'api_key_query',
            'basic',
            'oauth_client_credentials',
            'custom_header'
        )
    ),
    credential_set_id uuid REFERENCES runtime_credential_sets(id) ON DELETE RESTRICT,
    auth_config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(auth_config) = 'object'),
    content_hash text NOT NULL,
    revision bigint NOT NULL,
    is_current boolean NOT NULL DEFAULT true,
    created_by uuid REFERENCES root_users(user_id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT runtime_service_connection_revisions_credential_check CHECK (
        authentication_type IN ('none','delegated_oauth') OR credential_set_id IS NOT NULL
    ),
    UNIQUE (connection_id, environment_id, revision),
    UNIQUE (connection_id, environment_id, content_hash)
);
CREATE UNIQUE INDEX runtime_service_connection_revisions_one_current_idx
    ON runtime_service_connection_revisions(connection_id, environment_id) WHERE is_current;
CREATE INDEX runtime_service_connection_revisions_created_idx
    ON runtime_service_connection_revisions(connection_id, environment_id, revision DESC);

-- A published tool release pins configuration, never secret material. Runtime
-- credential rotation therefore does not require republishing a tool.
CREATE TABLE tool_release_runtime_targets (
    tool_release_id uuid NOT NULL REFERENCES tool_releases(id) ON DELETE CASCADE,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    connection_revision_id uuid NOT NULL REFERENCES runtime_service_connection_revisions(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tool_release_id, environment_id)
);
