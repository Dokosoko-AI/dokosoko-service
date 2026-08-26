-- Collapse the historical product-suite schema into DokoSoko's core:
-- documentation, exact SDK references, runtime credentials, recipes, reviewed
-- HTTP/native tools, and MCP delivery.

-- Package catalogue entries become exact, API-owned SDK references. Package
-- bytes, provenance workflows, replacement chains, and release lifecycle are
-- deliberately not carried forward.
CREATE TABLE sdk_references (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    integration_id uuid NOT NULL,
    ecosystem text NOT NULL CHECK (char_length(ecosystem) BETWEEN 1 AND 40),
    coordinate text NOT NULL CHECK (char_length(coordinate) BETWEEN 1 AND 240),
    exact_version text NOT NULL CHECK (
        char_length(exact_version) BETWEEN 1 AND 120
        AND lower(exact_version) <> 'latest'
        AND exact_version !~ '[*<>=~^]'
    ),
    install_command text NOT NULL CHECK (char_length(install_command) BETWEEN 1 AND 500),
    documentation_url text NOT NULL DEFAULT '',
    source_url text NOT NULL DEFAULT '',
    checksum text NOT NULL DEFAULT '' CHECK (
        checksum = '' OR checksum ~ '^(sha256|sha384|sha512):[a-f0-9]+$'
    ),
    visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','public')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (integration_id, ecosystem, coordinate),
    FOREIGN KEY (integration_id, deployment_id)
        REFERENCES integrations(id, deployment_id) ON DELETE CASCADE
);
CREATE INDEX sdk_references_integration_visibility_idx
    ON sdk_references(integration_id, visibility, ecosystem, coordinate);

INSERT INTO sdk_references(
    id, deployment_id, organisation_id, integration_id, ecosystem, coordinate,
    exact_version, install_command, documentation_url, source_url, checksum,
    visibility, created_at, updated_at
)
SELECT
    binding.id,
    binding.deployment_id,
    artifact.organisation_id,
    binding.integration_id,
    release.ecosystem,
    release.coordinate,
    release.version,
    release.install_command,
    release.registry_url,
    release.source_url,
    release.digest,
    release.visibility,
    binding.created_at,
    binding.updated_at
FROM integration_package_bindings binding
JOIN package_artifacts artifact ON artifact.id = binding.package_artifact_id
JOIN package_releases release ON release.id = binding.package_release_id
ON CONFLICT (integration_id, ecosystem, coordinate) DO NOTHING;

DROP TABLE integration_package_bindings;
DROP TABLE package_releases;
DROP TABLE package_artifacts;

-- Reporting is now a simple consent-based plaintext outbox. Historical rows
-- used envelope encryption and cannot be converted without retaining the
-- removed cryptographic delivery service, so invalidate them explicitly.
DELETE FROM report_submissions;
DROP INDEX IF EXISTS report_submissions_delivery_idx;
ALTER TABLE report_submissions
    DROP CONSTRAINT report_submissions_state_check,
    ADD COLUMN payload jsonb,
    DROP COLUMN support_route_id,
    DROP COLUMN integration_snapshot,
    DROP COLUMN payload_ciphertext,
    DROP COLUMN payload_nonce,
    DROP COLUMN payload_key_version,
    DROP COLUMN payload_fingerprint,
    DROP COLUMN attempts,
    DROP COLUMN next_attempt_at,
    DROP COLUMN delivery_started_at,
    DROP COLUMN last_error,
    DROP COLUMN external_id,
    DROP COLUMN external_url,
    DROP COLUMN delivered_at,
    ALTER COLUMN state SET DEFAULT 'queued',
    ADD CONSTRAINT report_submissions_state_check CHECK (state = 'queued');
ALTER TABLE report_submissions ALTER COLUMN payload SET NOT NULL;

DROP TABLE integration_support_bindings;
DROP TABLE support_routes;
DROP TABLE backend_connections;

-- Upstream MCP imports use one service access token. Existing service-token
-- connections remain valid. Delegated/anonymous imports and the MCP tools
-- generated from them are removed because they have no valid token-only
-- representation.
DELETE FROM integration_tool_bindings binding
USING tool_definitions definition, mcp_connections connection
WHERE binding.tool_id = definition.id
  AND definition.mcp_connection_id = connection.id
  AND connection.auth_mode <> 'service';

DELETE FROM tool_definitions definition
USING mcp_connections connection
WHERE definition.mcp_connection_id = connection.id
  AND connection.auth_mode <> 'service';

DELETE FROM mcp_connections WHERE auth_mode <> 'service';
DROP TABLE mcp_authorization_states;
DROP TABLE mcp_user_grants;

ALTER TABLE mcp_connections
    DROP CONSTRAINT mcp_connections_auth_mode_check,
    ADD COLUMN forward_user_identity boolean NOT NULL DEFAULT false;
UPDATE mcp_connections SET auth_mode = 'access_token' WHERE auth_mode = 'service';
ALTER TABLE mcp_connections
    DROP COLUMN oauth_client_id,
    DROP COLUMN oauth_client_secret_id,
    DROP COLUMN oauth_issuer,
    DROP COLUMN authorization_url,
    DROP COLUMN token_url,
    DROP COLUMN scopes,
    ALTER COLUMN credential_secret_id SET NOT NULL,
    ADD CONSTRAINT mcp_connections_auth_mode_check CHECK (auth_mode = 'access_token');

-- Remove embedded widget state. A future widget is an external plugin and has
-- no tables or runtime endpoints in dokosoko-service.
DROP TABLE widget_agent_messages;
DROP TABLE widget_sessions;
DROP TABLE widget_bootstrap_tokens;
DROP TABLE widget_secrets;
DROP TABLE widgets;

-- Remove provider-owned resource provisioning. Runtime credential sets and
-- fixed runtime service connections remain the supported key-management path.
DROP TABLE access_instance_integration_bindings;
DROP TABLE access_credentials;
DROP TABLE access_instances;
DROP TABLE integration_access_bindings;
DROP TABLE access_connections;
DROP TABLE access_definition_revisions;
DROP TABLE access_definitions;
DROP TABLE credential_leases;
DROP TABLE projects;
DROP TABLE providers;

-- Product build/release governance, installations, pins, channels, rollout,
-- promotion, and drift are no longer product concepts. Immutable Integration
-- publications remain the delivery boundary.
DROP TABLE product_version_pin_history;
DROP TABLE product_version_pins;
DROP TABLE product_installations;
DROP TABLE product_definitions;
DROP TABLE product_builds;

ALTER TABLE knowledge_documents DROP COLUMN connector_release_id;
ALTER TABLE analytics_events DROP COLUMN integration_run_id;
DROP TABLE integration_runs;
DROP TABLE connector_releases;

ALTER TABLE products
    DROP COLUMN default_version_policy,
    DROP COLUMN require_promotion_approval;
ALTER TABLE deployments
    DROP COLUMN default_release_policy,
    DROP COLUMN require_promotion_approval;
