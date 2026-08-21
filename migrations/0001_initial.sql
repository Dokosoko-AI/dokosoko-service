CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TYPE visibility AS ENUM ('private', 'public');
CREATE TYPE lifecycle_state AS ENUM ('draft', 'validated', 'published', 'quarantined', 'retired');
CREATE TYPE package_mode AS ENUM ('public', 'proxy', 'download');

CREATE TABLE platform_config (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    public_url text NOT NULL,
    storage_driver text NOT NULL DEFAULT 'filesystem',
    storage_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    encryption_key_version integer NOT NULL DEFAULT 1,
    setup_completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issuer text NOT NULL,
    subject text NOT NULL,
    email citext,
    display_name text NOT NULL,
    disabled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issuer, subject)
);

CREATE TABLE root_users (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
    mfa_enforced boolean NOT NULL DEFAULT true CHECK (mfa_enforced),
    recovery_codes_digest bytea,
    created_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE TABLE organisations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    name text NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('owner', 'admin', 'editor', 'viewer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organisation_id, user_id)
);

CREATE TABLE products (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    slug text NOT NULL,
    name text NOT NULL,
    public_mcp_enabled boolean NOT NULL DEFAULT false,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, slug)
);

CREATE TABLE environments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    slug text NOT NULL,
    name text NOT NULL,
    is_production boolean NOT NULL DEFAULT false,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id, slug)
);

CREATE TABLE connector_releases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    version bigint NOT NULL,
    state lifecycle_state NOT NULL DEFAULT 'draft',
    manifest jsonb NOT NULL DEFAULT '{}'::jsonb,
    published_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id, version)
);

CREATE TABLE secrets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid REFERENCES organisations(id) ON DELETE CASCADE,
    name text NOT NULL,
    purpose text NOT NULL,
    ciphertext bytea NOT NULL,
    nonce bytea NOT NULL,
    key_version integer NOT NULL,
    fingerprint text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    rotated_at timestamptz,
    UNIQUE (organisation_id, name)
);

CREATE TABLE sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    environment_id uuid REFERENCES environments(id) ON DELETE CASCADE,
    name text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('website', 'openapi', 'git', 'upload', 'sdk', 'package_metadata')),
    location text NOT NULL,
    visibility visibility NOT NULL DEFAULT 'private',
    state lifecycle_state NOT NULL DEFAULT 'draft',
    credential_secret_id uuid REFERENCES secrets(id) ON DELETE SET NULL,
    crawl_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sources_product_visibility_idx ON sources(product_id, visibility, state);

CREATE TABLE crawl_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    state text NOT NULL CHECK (state IN ('queued', 'running', 'review', 'succeeded', 'failed', 'cancelled')),
    attempt integer NOT NULL DEFAULT 1,
    discovered_count integer NOT NULL DEFAULT 0,
    fetched_count integer NOT NULL DEFAULT 0,
    changed_count integer NOT NULL DEFAULT 0,
    error_code text,
    error_message text,
    queued_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz
);
CREATE INDEX crawl_jobs_source_created_idx ON crawl_jobs(source_id, queued_at DESC);

CREATE TABLE source_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    crawl_job_id uuid NOT NULL REFERENCES crawl_jobs(id) ON DELETE CASCADE,
    canonical_url text NOT NULL,
    object_key text NOT NULL,
    content_sha256 bytea NOT NULL,
    etag text,
    last_modified timestamptz,
    content_type text,
    response_status integer,
    trust_indicators jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_id, canonical_url, content_sha256)
);

CREATE TABLE knowledge_documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    environment_id uuid REFERENCES environments(id) ON DELETE CASCADE,
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    snapshot_id uuid NOT NULL REFERENCES source_snapshots(id) ON DELETE RESTRICT,
    connector_release_id uuid REFERENCES connector_releases(id) ON DELETE SET NULL,
    title text NOT NULL,
    canonical_url text NOT NULL,
    body text NOT NULL,
    body_tsv tsvector GENERATED ALWAYS AS (to_tsvector('english', coalesce(title, '') || ' ' || body)) STORED,
    embedding vector(1536),
    visibility visibility NOT NULL DEFAULT 'private',
    state lifecycle_state NOT NULL DEFAULT 'draft',
    trust_level smallint NOT NULL DEFAULT 0 CHECK (trust_level BETWEEN 0 AND 100),
    injection_indicators jsonb NOT NULL DEFAULT '[]'::jsonb,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX knowledge_documents_scope_idx ON knowledge_documents(organisation_id, product_id, environment_id, visibility, state);
CREATE INDEX knowledge_documents_fts_idx ON knowledge_documents USING gin(body_tsv);

CREATE TABLE packages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    environment_id uuid REFERENCES environments(id) ON DELETE CASCADE,
    ecosystem text NOT NULL CHECK (ecosystem IN ('npm', 'go', 'git', 'maven', 'android', 'swift', 'nuget')),
    name text NOT NULL,
    version text NOT NULL,
    external_package_id text,
    mode package_mode NOT NULL,
    visibility visibility NOT NULL DEFAULT 'private',
    state lifecycle_state NOT NULL DEFAULT 'draft',
    upstream_url text,
    download_url text,
    credential_secret_id uuid REFERENCES secrets(id) ON DELETE RESTRICT,
    checksum_sha256 bytea,
    expected_size bigint,
    anonymous_download_budget bigint,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT packages_delivery_configuration CHECK (
        (mode = 'public' AND upstream_url IS NOT NULL AND download_url IS NULL AND credential_secret_id IS NULL AND external_package_id IS NULL) OR
        (mode = 'proxy' AND upstream_url IS NOT NULL AND download_url IS NULL AND credential_secret_id IS NOT NULL AND external_package_id IS NULL) OR
        (mode = 'download' AND upstream_url IS NULL AND download_url IS NOT NULL AND download_url ~ '^https://[^/?#:@]+(:443)?/v1/package/download$' AND credential_secret_id IS NOT NULL AND external_package_id IS NOT NULL AND char_length(btrim(external_package_id)) BETWEEN 1 AND 200)
    ),
    CONSTRAINT packages_checksum_sha256_length CHECK (checksum_sha256 IS NULL OR octet_length(checksum_sha256) = 32),
    CONSTRAINT packages_expected_size_range CHECK (expected_size IS NULL OR expected_size BETWEEN 1 AND 1073741824),
    UNIQUE (product_id, environment_id, ecosystem, name, version)
);
CREATE INDEX packages_product_visibility_idx ON packages(product_id, visibility, state);

CREATE TABLE vendor_identity_providers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    issuer text NOT NULL,
    client_id text NOT NULL,
    client_secret_id uuid REFERENCES secrets(id) ON DELETE RESTRICT,
    scopes text[] NOT NULL DEFAULT ARRAY['openid'],
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, issuer)
);

CREATE TABLE vendor_grants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    environment_id uuid REFERENCES environments(id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(id) ON DELETE CASCADE,
    vendor_subject text,
    vendor_organisation_id text NOT NULL,
    scopes text[] NOT NULL DEFAULT '{}',
    consent_revision bigint NOT NULL,
    delegated_secret_id uuid REFERENCES secrets(id) ON DELETE SET NULL,
    expires_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE entitlement_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(id) ON DELETE CASCADE,
    vendor_organisation_id text NOT NULL,
    entitlements jsonb NOT NULL,
    source_revision text,
    valid_until timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX entitlement_snapshot_lookup_idx ON entitlement_snapshots(product_id, user_id, valid_until DESC);

CREATE TABLE providers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('builtin', 'remote', 'proxy')),
    base_url text,
    credential_secret_id uuid REFERENCES secrets(id) ON DELETE SET NULL,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    provider_id uuid NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    owner_type text NOT NULL CHECK (owner_type IN ('organisation', 'user', 'integration_workspace')),
    owner_id text NOT NULL,
    external_id text NOT NULL,
    idempotency_key text NOT NULL,
    state text NOT NULL,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, idempotency_key)
);

CREATE TABLE credential_leases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    project_id uuid REFERENCES projects(id) ON DELETE CASCADE,
    provider_id uuid NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    subject_id text NOT NULL,
    scopes text[] NOT NULL DEFAULT '{}',
    secret_fingerprint text NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE api_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name text NOT NULL,
    base_url text NOT NULL,
    allowed_hosts text[] NOT NULL,
    credential_secret_id uuid REFERENCES secrets(id) ON DELETE SET NULL,
    tls_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tool_definitions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    namespace text NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    input_schema jsonb NOT NULL,
    state lifecycle_state NOT NULL DEFAULT 'draft',
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id, namespace, name)
);

CREATE TABLE tool_releases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    tool_definition_id uuid NOT NULL REFERENCES tool_definitions(id) ON DELETE CASCADE,
    api_connection_id uuid NOT NULL REFERENCES api_connections(id) ON DELETE RESTRICT,
    version bigint NOT NULL,
    request_mapping jsonb NOT NULL,
    output_schema jsonb NOT NULL,
    response_mapping jsonb NOT NULL,
    authorization_policy jsonb NOT NULL,
    timeout_ms integer NOT NULL CHECK (timeout_ms BETWEEN 100 AND 60000),
    rate_limit jsonb NOT NULL,
    published_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tool_definition_id, version)
);

CREATE TABLE widget_configurations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('public', 'private')),
    enabled boolean NOT NULL DEFAULT false,
    allowed_origins text[] NOT NULL DEFAULT '{}',
    theme jsonb NOT NULL DEFAULT '{}'::jsonb,
    locale text NOT NULL DEFAULT 'en',
    privacy_notice_url text,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id, kind)
);

CREATE TABLE integration_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    connector_release_id uuid REFERENCES connector_releases(id) ON DELETE SET NULL,
    requested_outcome text NOT NULL,
    state text NOT NULL,
    reported_success boolean,
    validated_success boolean,
    failure_code text,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);

CREATE TABLE analytics_events (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL,
    product_id uuid,
    environment_id uuid,
    event_name text NOT NULL,
    actor_kind text NOT NULL,
    actor_pseudonym text,
    integration_run_id uuid,
    dimensions jsonb NOT NULL DEFAULT '{}'::jsonb,
    value numeric,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
CREATE TABLE analytics_events_default PARTITION OF analytics_events DEFAULT;
CREATE INDEX analytics_events_default_scope_idx ON analytics_events_default(organisation_id, product_id, event_name, created_at DESC);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid REFERENCES organisations(id) ON DELETE RESTRICT,
    product_id uuid REFERENCES products(id) ON DELETE RESTRICT,
    actor_id text NOT NULL,
    actor_kind text NOT NULL,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id text NOT NULL,
    prior jsonb,
    current jsonb,
    policy_decision jsonb,
    request_id text NOT NULL,
    outcome text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_scope_idx ON audit_events(organisation_id, product_id, created_at DESC);

CREATE FUNCTION prevent_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only';
END;
$$;
CREATE TRIGGER audit_events_no_update BEFORE UPDATE OR DELETE ON audit_events
FOR EACH ROW EXECUTE FUNCTION prevent_audit_mutation();

CREATE FUNCTION prevent_analytics_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'analytics_events is append-only';
END;
$$;
CREATE TRIGGER analytics_events_no_update BEFORE UPDATE OR DELETE ON analytics_events
FOR EACH ROW EXECUTE FUNCTION prevent_analytics_mutation();
