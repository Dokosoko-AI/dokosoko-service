-- HTTP tool authentication is an upstream connection concern.  The
-- connection keeps only non-secret configuration and an encrypted secret
-- reference; tool definitions keep declarative request/response mappings.
ALTER TABLE api_connections
    ADD COLUMN authentication_type text NOT NULL DEFAULT 'delegated_oauth',
    ADD COLUMN auth_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN credential_secret_id uuid REFERENCES secrets(id) ON DELETE RESTRICT,
    ADD CONSTRAINT api_connections_authentication_type_check CHECK (
        authentication_type IN (
            'delegated_oauth',
            'none',
            'bearer',
            'authorization_scheme',
            'api_key_header',
            'api_key_query',
            'basic',
            'oauth_client_credentials',
            'custom_header'
        )
    ),
    ADD CONSTRAINT api_connections_auth_config_object_check CHECK (jsonb_typeof(auth_config) = 'object'),
    ADD CONSTRAINT api_connections_credential_check CHECK (
        authentication_type IN ('delegated_oauth', 'none') OR credential_secret_id IS NOT NULL
    );

ALTER TABLE tool_definitions
    ADD COLUMN request_mapping jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN response_mapping jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN request_example jsonb,
    ADD COLUMN response_example jsonb,
    ADD CONSTRAINT tool_definitions_request_mapping_object_check CHECK (jsonb_typeof(request_mapping) = 'object'),
    ADD CONSTRAINT tool_definitions_response_mapping_object_check CHECK (jsonb_typeof(response_mapping) = 'object'),
    ADD CONSTRAINT tool_definitions_request_example_object_check CHECK (request_example IS NULL OR jsonb_typeof(request_example) = 'object'),
    ADD CONSTRAINT tool_definitions_response_example_object_check CHECK (response_example IS NULL OR jsonb_typeof(response_example) = 'object');

-- Migration 0011 introduced MCP-backed definitions after the original HTTP
-- method check, but did not update that check. Keep the method and backend
-- discriminators consistent so MCP imports work on PostgreSQL as they do in
-- the in-memory store.
ALTER TABLE tool_definitions
    DROP CONSTRAINT tool_definitions_http_method_check,
    ADD CONSTRAINT tool_definitions_http_method_check CHECK (
        (backend_kind = 'http' AND http_method IN ('GET', 'POST', 'PUT', 'PATCH', 'DELETE'))
        OR (backend_kind = 'mcp' AND http_method = 'MCP')
    );

-- Existing HTTP tools already execute with the authenticated customer's
-- delegated access token.  Preserve that exact behavior through the new
-- explicit connection contract.
UPDATE api_connections
SET authentication_type = 'delegated_oauth',
    auth_config = '{}'::jsonb
WHERE authentication_type = 'delegated_oauth';
