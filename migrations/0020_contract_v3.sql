-- Break the legacy package/hook contract forward without rewriting the
-- checksum-protected migration history. OAuth artifacts are intentionally
-- invalidated because their identity, resource, and grant bindings changed.

CREATE TABLE customer_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    issuer text NOT NULL,
    external_id text NOT NULL,
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active','suspended')),
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    last_authenticated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(product_id, issuer, external_id),
    UNIQUE(id, product_id, organisation_id)
);
CREATE INDEX customer_accounts_product_state_idx
    ON customer_accounts(product_id, state, external_id);
CREATE INDEX customer_accounts_product_created_idx
    ON customer_accounts(product_id, created_at DESC, id DESC);

-- Preserve legacy installation and customer-pin ownership as durable account
-- rows. A configured issuer is retained; otherwise the sentinel makes the
-- migrated ownership explicit until an operator reassigns it.
INSERT INTO customer_accounts(organisation_id, product_id, issuer, external_id)
SELECT DISTINCT
    product.organisation_id,
    legacy.product_id,
    coalesce(identity_provider.issuer, 'https://migration.invalid'),
    CASE WHEN btrim(legacy.customer_id) = '' THEN 'legacy-unassigned' ELSE legacy.customer_id END
FROM (
    SELECT product_id, customer_id FROM product_installations
    UNION
    SELECT product_id, customer_id FROM product_version_pins
) legacy
JOIN products product ON product.id = legacy.product_id
LEFT JOIN LATERAL (
    SELECT issuer
    FROM vendor_identity_providers
    WHERE product_id = legacy.product_id
    ORDER BY created_at DESC
    LIMIT 1
) identity_provider ON true
ON CONFLICT (product_id, issuer, external_id) DO NOTHING;

DROP INDEX product_installations_resolution_idx;
ALTER TABLE product_installations
    ADD COLUMN customer_account_id uuid REFERENCES customer_accounts(id) ON DELETE CASCADE;
UPDATE product_installations installation
SET customer_account_id = account.id
FROM customer_accounts account
WHERE account.product_id = installation.product_id
  AND account.external_id = CASE WHEN btrim(installation.customer_id) = '' THEN 'legacy-unassigned' ELSE installation.customer_id END;
ALTER TABLE product_installations
    ALTER COLUMN customer_account_id SET NOT NULL,
    DROP COLUMN customer_id;
CREATE INDEX product_installations_resolution_idx
    ON product_installations(product_id, customer_account_id, external_id) WHERE state='active';

DROP INDEX product_version_pins_product_idx;
ALTER TABLE product_version_pins
    DROP CONSTRAINT product_version_pins_scope_shape,
    ADD COLUMN customer_account_id uuid REFERENCES customer_accounts(id) ON DELETE CASCADE;
UPDATE product_version_pins pin
SET customer_account_id = account.id
FROM customer_accounts account
WHERE account.product_id = pin.product_id
  AND account.external_id = CASE WHEN btrim(pin.customer_id) = '' THEN 'legacy-unassigned' ELSE pin.customer_id END;
UPDATE product_version_pins SET scope_id = customer_account_id::text WHERE scope='customer';
ALTER TABLE product_version_pins
    ALTER COLUMN customer_account_id SET NOT NULL,
    DROP COLUMN customer_id,
    ADD CONSTRAINT product_version_pins_scope_shape CHECK (
        (scope='customer' AND customer_account_id IS NOT NULL AND environment_id IS NULL AND installation_id IS NULL)
        OR (scope='environment' AND environment_id IS NOT NULL AND installation_id IS NULL)
        OR (scope='installation' AND installation_id IS NOT NULL)
    );
CREATE INDEX product_version_pins_product_idx
    ON product_version_pins(product_id, customer_account_id);

DROP TABLE oauth_access_tokens;
DROP TABLE oauth_authorization_codes;
DROP TABLE oauth_states;

-- Identity is an optional delegated-user boundary, independent of service
-- delivery. Existing rows cannot supply the delegated API origin safely, so
-- require explicit reconfiguration and invalidate authorization artifacts.
DELETE FROM vendor_identity_providers;
ALTER TABLE vendor_identity_providers
    RENAME TO identity_providers;
DROP INDEX vendor_identity_product_idx;
ALTER TABLE identity_providers
    ADD COLUMN oauth_resource text NOT NULL DEFAULT '',
    ADD COLUMN delegated_api_origin text,
    ADD COLUMN state text NOT NULL DEFAULT 'active' CHECK (state IN ('active','disabled')),
    DROP COLUMN entitlement_hook_url,
    DROP COLUMN allowed_redirect_uris,
    DROP COLUMN authorization_hook_url,
    DROP COLUMN authorization_credential_id,
    DROP COLUMN usage_hook_url,
    DROP COLUMN usage_credential_id,
    ALTER COLUMN product_id SET NOT NULL,
    ALTER COLUMN delegated_api_origin SET NOT NULL,
    ADD CONSTRAINT identity_provider_api_shape CHECK (
        delegated_api_origin ~ '^https://[^/?#:@]+$'
        AND (oauth_resource = '' OR oauth_resource ~ '^https://')
    );
CREATE UNIQUE INDEX identity_provider_deployment_idx ON identity_providers(product_id);

CREATE TABLE oauth_states (
    state_digest bytea PRIMARY KEY,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    client_id text NOT NULL,
    redirect_uri text NOT NULL,
    resource text NOT NULL,
    scopes text[] NOT NULL,
    downstream_state text NOT NULL,
    downstream_challenge text NOT NULL,
    upstream_verifier text NOT NULL,
    nonce text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_authorization_codes (
    code_digest bytea PRIMARY KEY,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    client_id text NOT NULL,
    redirect_uri text NOT NULL,
    resource text NOT NULL,
    scopes text[] NOT NULL,
    downstream_challenge text NOT NULL,
    issuer text NOT NULL,
    subject text NOT NULL,
    email text NOT NULL DEFAULT '',
    display_name text NOT NULL DEFAULT '',
    customer_account_id uuid NOT NULL REFERENCES customer_accounts(id) ON DELETE CASCADE,
    external_customer_id text NOT NULL,
    installation_id text NOT NULL DEFAULT '',
    grants jsonb NOT NULL DEFAULT '{}'::jsonb,
    access_evaluation_id text NOT NULL,
    policy_version text NOT NULL DEFAULT '',
    upstream_access_secret_id uuid NOT NULL REFERENCES secrets(id) ON DELETE RESTRICT,
    access_expires_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_access_tokens (
    token_digest bytea PRIMARY KEY,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    client_id text NOT NULL,
    resource text NOT NULL,
    issuer text NOT NULL,
    subject text NOT NULL,
    email text NOT NULL DEFAULT '',
    display_name text NOT NULL DEFAULT '',
    customer_account_id uuid NOT NULL REFERENCES customer_accounts(id) ON DELETE CASCADE,
    external_customer_id text NOT NULL,
    installation_id text NOT NULL DEFAULT '',
    grants jsonb NOT NULL DEFAULT '{}'::jsonb,
    access_evaluation_id text NOT NULL,
    policy_version text NOT NULL DEFAULT '',
    upstream_access_secret_id uuid NOT NULL REFERENCES secrets(id) ON DELETE RESTRICT,
    scopes text[] NOT NULL DEFAULT '{}',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
CREATE INDEX oauth_access_tokens_subject_idx
    ON oauth_access_tokens(product_id, issuer, subject, expires_at DESC);

-- Service-to-service delivery has its own connection and authentication
-- lifecycle. Only bearer authentication is exposed until another mechanism is
-- fully implemented end to end.
CREATE TABLE backend_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    name text NOT NULL,
    base_url text NOT NULL,
    authentication_type text NOT NULL CHECK (authentication_type = 'bearer'),
    credential_secret_id uuid REFERENCES secrets(id) ON DELETE RESTRICT,
    state text NOT NULL DEFAULT 'disabled' CHECK (state IN ('active','disabled')),
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (deployment_id, name),
    CONSTRAINT backend_connection_origin CHECK (base_url ~ '^https://[^/?#:@]+$'),
    CONSTRAINT backend_connection_activation CHECK (state = 'disabled' OR credential_secret_id IS NOT NULL)
);
CREATE INDEX backend_connections_deployment_state_idx
    ON backend_connections(deployment_id, state, name);

INSERT INTO backend_connections(
    id, deployment_id, organisation_id, name, base_url,
    authentication_type, credential_secret_id, state, created_at, updated_at
)
-- Legacy hook secrets were encrypted for a different purpose/context and are
-- intentionally not rebound. Operators must create a fresh credential before
-- activating the migrated connection.
SELECT
    route.id, route.deployment_id, route.organisation_id,
    route.name || ' delivery', 'https://migration.invalid', 'bearer',
    NULL,
    'disabled', route.created_at, route.updated_at
FROM support_routes route
WHERE coalesce(route.bug_hook_credential_id, route.feedback_hook_credential_id) IS NOT NULL
ON CONFLICT (id) DO NOTHING;

ALTER TABLE support_routes
    ADD COLUMN backend_connection_id uuid REFERENCES backend_connections(id) ON DELETE RESTRICT;
UPDATE support_routes route
SET backend_connection_id = connection.id,
    bug_reports_enabled = false,
    feedback_enabled = false
FROM backend_connections connection
WHERE connection.id = route.id;
ALTER TABLE support_routes
    DROP COLUMN bug_hook_url,
    DROP COLUMN bug_hook_credential_id,
    DROP COLUMN feedback_hook_url,
    DROP COLUMN feedback_hook_credential_id;

INSERT INTO support_routes(
    deployment_id, organisation_id, name, is_default,
    bug_reports_enabled, feedback_enabled, retention_days, state
)
SELECT submission.product_id, submission.organisation_id, 'Migrated support archive', false, false, false, 30, 'archived'
FROM report_submissions submission
WHERE submission.support_route_id IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM support_routes route WHERE route.deployment_id = submission.product_id
  )
GROUP BY submission.product_id, submission.organisation_id
ON CONFLICT (deployment_id, name) DO NOTHING;

UPDATE report_submissions submission
SET support_route_id = (
    SELECT candidate.id
    FROM support_routes candidate
    WHERE candidate.deployment_id = submission.product_id
    ORDER BY (candidate.is_default AND candidate.state='active') DESC, candidate.created_at
    LIMIT 1
)
WHERE submission.support_route_id IS NULL;

ALTER TABLE report_submissions
    DROP CONSTRAINT report_submissions_support_route_id_fkey,
    ALTER COLUMN support_route_id SET NOT NULL,
    ADD CONSTRAINT report_submissions_support_route_id_fkey
        FOREIGN KEY (support_route_id) REFERENCES support_routes(id) ON DELETE RESTRICT;
DROP TABLE reporting_configs;

-- Visibility is an integration-level publishing decision. Default every
-- migrated integration to private; exposing it publicly always requires a
-- deliberate post-migration acknowledgement.
ALTER TABLE integrations
    ADD COLUMN visibility text NOT NULL DEFAULT 'private'
        CHECK (visibility IN ('private','public'));

-- Resource sets now contain documentation or API contracts only. Package and
-- open-ended hook sets have no faithful v3 representation and are removed.
DELETE FROM resource_sets WHERE kind IN ('package', 'hook');
ALTER TABLE resource_sets
    DROP CONSTRAINT resource_sets_kind_check,
    ADD CONSTRAINT resource_sets_kind_check CHECK (kind IN ('documentation','api'));

ALTER TABLE access_definitions RENAME COLUMN hook_set_id TO api_resource_set_id;
UPDATE access_definitions SET api_resource_set_id = NULL;
UPDATE access_definitions
SET operations = (operations - 'required_entitlements')
    || jsonb_build_object('required_grants', coalesce(operations->'required_grants', operations->'required_entitlements', '[]'::jsonb));
UPDATE providers
SET config = (config - 'required_entitlements')
    || jsonb_build_object('required_grants', coalesce(config->'required_grants', config->'required_entitlements', '[]'::jsonb));

DELETE FROM sources WHERE kind IN ('sdk', 'package_metadata');
ALTER TABLE sources
    DROP CONSTRAINT sources_kind_check,
    ADD CONSTRAINT sources_kind_check CHECK (kind IN ('website', 'openapi', 'git', 'upload'));
DROP TABLE packages;
DROP TYPE package_mode;
DROP TABLE vendor_grants;
DROP TABLE entitlement_snapshots;
ALTER TABLE api_connections DROP COLUMN credential_secret_id;
