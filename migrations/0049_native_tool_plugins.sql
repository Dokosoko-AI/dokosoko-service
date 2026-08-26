-- Trusted native tool plugins are source packages compiled into DokoSoko.
-- Definitions remain governed by the normal draft/review/publication flow and
-- releases pin the exact source and contract that was reviewed.

ALTER TABLE tool_definitions
    DROP CONSTRAINT tool_definitions_backend_kind_check,
    DROP CONSTRAINT tool_backend_configuration,
    DROP CONSTRAINT tool_definitions_http_method_check,
    ADD COLUMN effect text,
    ADD COLUMN idempotency_mode text NOT NULL DEFAULT 'none',
    ADD COLUMN identity_requirement text NOT NULL DEFAULT 'none',
    ADD COLUMN state_scope text NOT NULL DEFAULT 'none',
    ADD COLUMN max_concurrency integer NOT NULL DEFAULT 0,
    ADD COLUMN max_result_bytes bigint NOT NULL DEFAULT 1048576,
    ADD COLUMN native_plugin_id text NOT NULL DEFAULT '',
    ADD COLUMN native_tool_id text NOT NULL DEFAULT '',
    ADD COLUMN native_plugin_version text NOT NULL DEFAULT '',
    ADD COLUMN native_sdk_version integer NOT NULL DEFAULT 0,
    ADD COLUMN native_manifest_hash text NOT NULL DEFAULT '',
    ADD COLUMN native_contract_hash text NOT NULL DEFAULT '';

UPDATE tool_definitions
SET effect = CASE
        WHEN backend_kind = 'http' AND http_method = 'GET' THEN 'read'
        WHEN backend_kind = 'http' AND http_method = 'DELETE' THEN 'destructive'
        ELSE 'write'
    END,
    idempotency_mode = CASE
        WHEN backend_kind = 'http' AND http_method = 'GET' THEN 'supported'
        WHEN backend_kind = 'http' THEN 'required'
        ELSE 'none'
    END;

ALTER TABLE tool_definitions
    ALTER COLUMN effect SET NOT NULL,
    ADD CONSTRAINT tool_definitions_backend_kind_check CHECK (backend_kind IN ('http', 'mcp', 'native')),
    ADD CONSTRAINT tool_definitions_effect_check CHECK (effect IN ('read', 'write', 'destructive')),
    ADD CONSTRAINT tool_definitions_idempotency_check CHECK (idempotency_mode IN ('none', 'supported', 'required')),
    ADD CONSTRAINT tool_definitions_identity_check CHECK (identity_requirement IN ('none', 'optional', 'actor_required', 'customer_required', 'actor_and_customer_required', 'installation_required')),
    ADD CONSTRAINT tool_definitions_state_scope_check CHECK (state_scope IN ('none', 'plugin', 'actor', 'customer', 'installation')),
    ADD CONSTRAINT tool_definitions_native_limits_check CHECK (max_concurrency BETWEEN 0 AND 64 AND max_result_bytes BETWEEN 1 AND 1048576),
    ADD CONSTRAINT tool_definitions_http_method_check CHECK (
        (backend_kind = 'http' AND http_method IN ('GET', 'POST', 'PUT', 'PATCH', 'DELETE'))
        OR (backend_kind = 'mcp' AND http_method = 'MCP')
        OR (backend_kind = 'native' AND http_method = 'NATIVE')
    ),
    ADD CONSTRAINT tool_backend_configuration CHECK (
        (backend_kind = 'http' AND mcp_connection_id IS NULL AND native_plugin_id = '' AND native_tool_id = '' AND ((api_connection_id IS NOT NULL) <> (runtime_service_connection_id IS NOT NULL)))
        OR (backend_kind = 'mcp' AND api_connection_id IS NULL AND runtime_service_connection_id IS NULL AND mcp_connection_id IS NOT NULL AND upstream_tool_name <> '' AND upstream_schema_hash <> '' AND native_plugin_id = '' AND native_tool_id = '')
        OR (backend_kind = 'native' AND api_connection_id IS NULL AND runtime_service_connection_id IS NULL AND mcp_connection_id IS NULL AND native_plugin_id <> '' AND native_tool_id <> '' AND native_plugin_version <> '' AND native_sdk_version > 0 AND native_manifest_hash <> '' AND native_contract_hash <> '' AND max_concurrency BETWEEN 1 AND 64 AND (effect = 'read' OR idempotency_mode = 'required'))
    );

CREATE INDEX tool_definitions_native_plugin_idx
    ON tool_definitions(product_id, native_plugin_id, state)
    WHERE backend_kind = 'native';

ALTER TABLE tool_releases
    DROP CONSTRAINT tool_releases_backend_kind_check,
    DROP CONSTRAINT tool_release_backend_configuration,
    ADD COLUMN effect text,
    ADD COLUMN idempotency_mode text NOT NULL DEFAULT 'none',
    ADD COLUMN identity_requirement text NOT NULL DEFAULT 'none',
    ADD COLUMN state_scope text NOT NULL DEFAULT 'none',
    ADD COLUMN max_concurrency integer NOT NULL DEFAULT 0,
    ADD COLUMN max_result_bytes bigint NOT NULL DEFAULT 1048576,
    ADD COLUMN native_plugin_id text NOT NULL DEFAULT '',
    ADD COLUMN native_tool_id text NOT NULL DEFAULT '',
    ADD COLUMN native_plugin_version text NOT NULL DEFAULT '',
    ADD COLUMN native_sdk_version integer NOT NULL DEFAULT 0,
    ADD COLUMN native_manifest_hash text NOT NULL DEFAULT '',
    ADD COLUMN native_contract_hash text NOT NULL DEFAULT '';

UPDATE tool_releases release
SET effect = definition.effect,
    idempotency_mode = definition.idempotency_mode,
    identity_requirement = definition.identity_requirement,
    state_scope = definition.state_scope,
    max_concurrency = definition.max_concurrency,
    max_result_bytes = definition.max_result_bytes
FROM tool_definitions definition
WHERE definition.id = release.tool_definition_id;

ALTER TABLE tool_releases
    ALTER COLUMN effect SET NOT NULL,
    ADD CONSTRAINT tool_releases_backend_kind_check CHECK (backend_kind IN ('http', 'mcp', 'native')),
    ADD CONSTRAINT tool_releases_effect_check CHECK (effect IN ('read', 'write', 'destructive')),
    ADD CONSTRAINT tool_releases_idempotency_check CHECK (idempotency_mode IN ('none', 'supported', 'required')),
    ADD CONSTRAINT tool_releases_identity_check CHECK (identity_requirement IN ('none', 'optional', 'actor_required', 'customer_required', 'actor_and_customer_required', 'installation_required')),
    ADD CONSTRAINT tool_releases_state_scope_check CHECK (state_scope IN ('none', 'plugin', 'actor', 'customer', 'installation')),
    ADD CONSTRAINT tool_releases_native_limits_check CHECK (max_concurrency BETWEEN 0 AND 64 AND max_result_bytes BETWEEN 1 AND 1048576),
    ADD CONSTRAINT tool_release_backend_configuration CHECK (
        (backend_kind = 'http' AND mcp_connection_id IS NULL AND native_plugin_id = '' AND native_tool_id = '' AND ((api_connection_id IS NOT NULL) <> (runtime_service_connection_id IS NOT NULL)))
        OR (backend_kind = 'mcp' AND api_connection_id IS NULL AND runtime_service_connection_id IS NULL AND mcp_connection_id IS NOT NULL AND upstream_tool_name <> '' AND upstream_schema_hash <> '' AND native_plugin_id = '' AND native_tool_id = '')
        OR (backend_kind = 'native' AND api_connection_id IS NULL AND runtime_service_connection_id IS NULL AND mcp_connection_id IS NULL AND native_plugin_id <> '' AND native_tool_id <> '' AND native_plugin_version <> '' AND native_sdk_version > 0 AND native_manifest_hash <> '' AND native_contract_hash <> '' AND max_concurrency BETWEEN 1 AND 64 AND (effect = 'read' OR idempotency_mode = 'required'))
    );

CREATE TABLE native_plugin_state (
    plugin_id text NOT NULL,
    scope_kind text NOT NULL CHECK (scope_kind IN ('plugin', 'actor', 'customer', 'installation')),
    scope_id text NOT NULL DEFAULT '',
    state_key text NOT NULL CHECK (state_key <> '' AND octet_length(state_key) <= 128),
    value jsonb NOT NULL CHECK (octet_length(value::text) <= 65536),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (plugin_id, scope_kind, scope_id, state_key),
    CHECK ((scope_kind = 'plugin') OR scope_id <> '')
);

CREATE INDEX native_plugin_state_expiry_idx
    ON native_plugin_state(expires_at)
    WHERE expires_at IS NOT NULL;
