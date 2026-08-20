CREATE TABLE access_definitions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    service_key text NOT NULL,
    name text NOT NULL,
    instance_cardinality text NOT NULL CHECK (instance_cardinality IN ('one','many')),
    instance_label_singular text NOT NULL,
    instance_label_plural text NOT NULL,
    credential_scope text NOT NULL CHECK (credential_scope IN ('connection','instance')),
    management_auth_type text NOT NULL DEFAULT 'bearer',
    hook_set_id uuid REFERENCES resource_sets(id) ON DELETE SET NULL,
    operations jsonb NOT NULL DEFAULT '{}'::jsonb,
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('draft','active','archived')),
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (deployment_id, service_key)
);

CREATE TABLE access_definition_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    access_definition_id uuid NOT NULL REFERENCES access_definitions(id) ON DELETE CASCADE,
    revision bigint NOT NULL,
    snapshot jsonb NOT NULL,
    content_hash text NOT NULL,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (access_definition_id, revision),
    UNIQUE (access_definition_id, content_hash)
);

CREATE TABLE access_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    access_definition_id uuid NOT NULL REFERENCES access_definitions(id) ON DELETE RESTRICT,
    environment_id uuid REFERENCES environments(id) ON DELETE SET NULL,
    name text NOT NULL,
    region text NOT NULL DEFAULT '',
    base_url text NOT NULL DEFAULT '',
    management_secret_id uuid REFERENCES secrets(id) ON DELETE SET NULL,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active','disabled','error')),
    legacy_provider_id uuid UNIQUE,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX access_connections_deployment_idx
    ON access_connections(deployment_id, state, name);

CREATE TABLE integration_access_bindings (
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    access_connection_id uuid NOT NULL REFERENCES access_connections(id) ON DELETE CASCADE,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (integration_id, access_connection_id)
);

CREATE TABLE access_instances (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    access_connection_id uuid NOT NULL REFERENCES access_connections(id) ON DELETE RESTRICT,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE RESTRICT,
    owner_type text NOT NULL CHECK (owner_type IN ('organisation','user','installation')),
    owner_id text NOT NULL,
    external_id text NOT NULL,
    display_name text NOT NULL DEFAULT '',
    idempotency_key text NOT NULL,
    state text NOT NULL,
    provider_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (access_connection_id, idempotency_key),
    UNIQUE (access_connection_id, external_id)
);
CREATE INDEX access_instances_connection_created_idx
    ON access_instances(access_connection_id, created_at DESC);

CREATE TABLE access_instance_integration_bindings (
    access_instance_id uuid NOT NULL REFERENCES access_instances(id) ON DELETE CASCADE,
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (access_instance_id, integration_id)
);

CREATE TABLE access_credentials (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    access_connection_id uuid NOT NULL REFERENCES access_connections(id) ON DELETE RESTRICT,
    access_instance_id uuid REFERENCES access_instances(id) ON DELETE CASCADE,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE RESTRICT,
    subject_id text NOT NULL,
    external_id text NOT NULL DEFAULT '',
    idempotency_key text NOT NULL DEFAULT '',
    scopes text[] NOT NULL DEFAULT '{}',
    secret_fingerprint text NOT NULL,
    storage_mode text NOT NULL DEFAULT 'one_time'
        CHECK (storage_mode IN ('one_time','managed','reference')),
    encrypted_secret_id uuid REFERENCES secrets(id) ON DELETE SET NULL,
    state text NOT NULL DEFAULT 'active'
        CHECK (state IN ('active','retiring','revoked','expired')),
    expires_at timestamptz,
    rotated_from_id uuid REFERENCES access_credentials(id) ON DELETE SET NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((storage_mode <> 'managed') OR encrypted_secret_id IS NOT NULL)
);
CREATE UNIQUE INDEX access_credentials_connection_idempotency_idx
    ON access_credentials(access_connection_id, idempotency_key)
    WHERE idempotency_key <> '';
CREATE INDEX access_credentials_connection_created_idx
    ON access_credentials(access_connection_id, created_at DESC);

-- A legacy provider mixed the reusable contract and its configured account.
-- Split it into one private definition and one connection while preserving the
-- original provider ID on the definition and as trace metadata on the
-- connection.
INSERT INTO access_definitions(
    id, deployment_id, organisation_id, service_key, name,
    instance_cardinality, instance_label_singular, instance_label_plural,
    credential_scope, management_auth_type, operations, state, revision,
    created_at, updated_at
)
SELECT
    p.id,
    p.product_id,
    p.organisation_id,
    'legacy-' || p.id::text,
    p.name,
    'many',
    'Project',
    'Projects',
    'instance',
    'bearer',
    jsonb_build_object(
        'contract_version', coalesce(p.config->>'contract_version', '2026-08-01'),
        'authorize', jsonb_build_object('method', 'POST', 'path', coalesce(p.config->>'authorize_path', '/v1/authorize')),
        'instances.create', jsonb_build_object('method', 'POST', 'path', coalesce(p.config->>'project_path', '/v1/projects')),
        'credentials.create', jsonb_build_object('method', 'POST', 'path', coalesce(p.config->>'credential_path', '/v1/credentials')),
        'credentials.revoke', jsonb_build_object('method', 'POST', 'path', coalesce(p.config->>'revoke_path', '/v1/credentials/{credential_id}/revoke')),
        'required_entitlements', coalesce(p.config->'required_entitlements', '[]'::jsonb),
        'max_ttl_seconds', coalesce((p.config->>'max_ttl_seconds')::integer, 3600)
    ),
    'active',
    p.revision,
    p.created_at,
    p.updated_at
FROM providers p
ON CONFLICT (id) DO NOTHING;

INSERT INTO access_definition_revisions(
    access_definition_id, revision, snapshot, content_hash
)
SELECT
    value.id,
    value.revision,
    jsonb_build_object(
        'service_key', value.service_key,
        'name', value.name,
        'instance_cardinality', value.instance_cardinality,
        'instance_label_singular', value.instance_label_singular,
        'instance_label_plural', value.instance_label_plural,
        'credential_scope', value.credential_scope,
        'management_auth_type', value.management_auth_type,
        'operations', value.operations
    ),
    ''
FROM access_definitions value
ON CONFLICT (access_definition_id, revision) DO NOTHING;

UPDATE access_definition_revisions
SET content_hash = 'sha256:' || encode(digest(convert_to(snapshot::text, 'UTF8'), 'sha256'), 'hex')
WHERE content_hash = '';

INSERT INTO access_connections(
    deployment_id, organisation_id, access_definition_id, name, base_url,
    management_secret_id, config, state, legacy_provider_id, revision,
    created_at, updated_at
)
SELECT
    p.product_id,
    p.organisation_id,
    p.id,
    p.name,
    coalesce(p.base_url, ''),
    p.credential_secret_id,
    p.config,
    'active',
    p.id,
    p.revision,
    p.created_at,
    p.updated_at
FROM providers p
ON CONFLICT (legacy_provider_id) DO NOTHING;

INSERT INTO integration_access_bindings(integration_id, access_connection_id)
SELECT i.id, connection.id
FROM integrations i
JOIN access_connections connection ON connection.deployment_id = i.deployment_id
ON CONFLICT DO NOTHING;

INSERT INTO access_instances(
    id, deployment_id, organisation_id, access_connection_id, environment_id,
    owner_type, owner_id, external_id, display_name, idempotency_key, state,
    expires_at, created_at, updated_at
)
SELECT
    project.id,
    project.product_id,
    project.organisation_id,
    connection.id,
    project.environment_id,
    CASE WHEN project.owner_type = 'integration_workspace' THEN 'installation' ELSE project.owner_type END,
    project.owner_id,
    project.external_id,
    project.external_id,
    project.idempotency_key,
    project.state,
    project.expires_at,
    project.created_at,
    project.updated_at
FROM projects project
JOIN access_connections connection ON connection.legacy_provider_id = project.provider_id
ON CONFLICT (id) DO NOTHING;

INSERT INTO access_instance_integration_bindings(access_instance_id, integration_id)
SELECT instance.id, integration.id
FROM access_instances instance
JOIN integrations integration ON integration.deployment_id = instance.deployment_id
ON CONFLICT DO NOTHING;

INSERT INTO access_credentials(
    id, deployment_id, organisation_id, access_connection_id,
    access_instance_id, environment_id, subject_id, external_id,
    idempotency_key, scopes, secret_fingerprint, storage_mode, state,
    expires_at, revoked_at, created_at
)
SELECT
    lease.id,
    lease.product_id,
    lease.organisation_id,
    connection.id,
    lease.project_id,
    lease.environment_id,
    lease.subject_id,
    lease.external_id,
    lease.idempotency_key,
    lease.scopes,
    lease.secret_fingerprint,
    'one_time',
    CASE
        WHEN lease.revoked_at IS NOT NULL THEN 'revoked'
        WHEN lease.expires_at <= now() THEN 'expired'
        ELSE 'active'
    END,
    lease.expires_at,
    lease.revoked_at,
    lease.created_at
FROM credential_leases lease
JOIN access_connections connection ON connection.legacy_provider_id = lease.provider_id
ON CONFLICT (id) DO NOTHING;
