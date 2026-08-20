-- A DokoSoko installation is one deployable connector catalog. The legacy
-- products table remains during the compatibility window, but a deployment is
-- deliberately a singleton and reuses the existing product UUID.
DO $$
BEGIN
    IF (SELECT count(*) FROM products) > 1 THEN
        RAISE EXCEPTION 'DokoSoko 0017 requires zero or one legacy product; split multi-product data before migrating';
    END IF;
END $$;

CREATE TABLE deployments (
    id uuid PRIMARY KEY,
    singleton boolean NOT NULL DEFAULT true UNIQUE CHECK (singleton),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE RESTRICT,
    name text NOT NULL,
    slug text NOT NULL,
    description text NOT NULL DEFAULT '',
    public_mcp_enabled boolean NOT NULL DEFAULT false,
    default_release_policy text NOT NULL DEFAULT 'latest'
        CHECK (default_release_policy IN ('latest','lts')),
    require_promotion_approval boolean NOT NULL DEFAULT false,
    catalog_revision bigint NOT NULL DEFAULT 1,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organisation_id, slug)
);

INSERT INTO deployments(
    id, organisation_id, name, slug, description, public_mcp_enabled,
    default_release_policy, require_promotion_approval, catalog_revision,
    revision, created_at, updated_at
)
SELECT
    id, organisation_id, name, slug, description, public_mcp_enabled,
    default_version_policy, require_promotion_approval, catalog_revision,
    revision, created_at, updated_at
FROM products
ORDER BY created_at
LIMIT 1;

CREATE TABLE integrations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    family_key text NOT NULL,
    version_key text NOT NULL,
    display_name text NOT NULL,
    description text NOT NULL DEFAULT '',
    lifecycle text NOT NULL DEFAULT 'draft'
        CHECK (lifecycle IN ('draft','active','deprecated','retired')),
    replacement_integration_id uuid REFERENCES integrations(id) ON DELETE SET NULL,
    sunset_at timestamptz,
    legacy_component_id text NOT NULL DEFAULT '',
    legacy_release_id text NOT NULL DEFAULT '',
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (deployment_id, family_key, version_key)
);
CREATE INDEX integrations_lifecycle_idx
    ON integrations(deployment_id, lifecycle, display_name, version_key);
CREATE UNIQUE INDEX integrations_legacy_release_idx
    ON integrations(deployment_id, legacy_component_id, legacy_release_id)
    WHERE legacy_component_id <> '' AND legacy_release_id <> '';

CREATE TABLE integration_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    revision bigint NOT NULL,
    state text NOT NULL CHECK (state IN ('draft','published')),
    snapshot jsonb NOT NULL,
    manifest_hash text NOT NULL,
    published_by text NOT NULL DEFAULT '',
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (integration_id, revision),
    UNIQUE (integration_id, manifest_hash)
);

CREATE TABLE resource_sets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('documentation','package','hook')),
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active','archived')),
    legacy_key text NOT NULL DEFAULT '',
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (deployment_id, kind, name)
);
CREATE UNIQUE INDEX resource_sets_legacy_key_idx
    ON resource_sets(deployment_id, legacy_key)
    WHERE legacy_key <> '';

CREATE TABLE resource_set_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_set_id uuid NOT NULL REFERENCES resource_sets(id) ON DELETE CASCADE,
    revision bigint NOT NULL,
    manifest jsonb NOT NULL DEFAULT '[]'::jsonb,
    content_hash text NOT NULL,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (resource_set_id, revision),
    UNIQUE (resource_set_id, content_hash)
);

CREATE TABLE integration_resource_bindings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    resource_set_id uuid NOT NULL REFERENCES resource_sets(id) ON DELETE RESTRICT,
    follow_latest boolean NOT NULL DEFAULT true,
    pinned_revision_id uuid REFERENCES resource_set_revisions(id) ON DELETE RESTRICT,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((follow_latest AND pinned_revision_id IS NULL) OR
           (NOT follow_latest AND pinned_revision_id IS NOT NULL)),
    UNIQUE (integration_id, resource_set_id)
);

-- Promote each legacy API component/release pair into one first-class
-- Integration. The legacy identifiers make this migration idempotent and keep
-- catalog-build evidence traceable.
INSERT INTO integrations(
    deployment_id, organisation_id, family_key, version_key, display_name,
    description, lifecycle, legacy_component_id, legacy_release_id, revision
)
SELECT
    pd.product_id,
    pd.organisation_id,
    component.value->>'slug',
    release.value->>'version',
    component.value->>'name',
    coalesce(component.value->>'description', ''),
    CASE WHEN release.value->>'state' = 'published' THEN 'active' ELSE 'draft' END,
    component.value->>'id',
    release.value->>'id',
    greatest(pd.revision, 1)
FROM product_definitions pd
CROSS JOIN LATERAL jsonb_array_elements(coalesce(pd.definition->'components', '[]'::jsonb)) component(value)
CROSS JOIN LATERAL jsonb_array_elements(coalesce(component.value->'releases', '[]'::jsonb)) release(value)
WHERE component.value->>'slug' <> ''
  AND release.value->>'version' <> ''
ON CONFLICT DO NOTHING;

-- Legacy bindings are not silently shared: one set per Integration and kind is
-- created. Administrators can deliberately attach or duplicate sets later.
WITH legacy_sets AS (
    SELECT
        i.id AS integration_id,
        i.deployment_id,
        i.organisation_id,
        i.display_name || ' ' || i.version_key || ' ' ||
            CASE
                WHEN binding.value->>'kind' IN ('openapi','docs','git') THEN 'documentation'
                WHEN binding.value->>'kind' = 'package' THEN 'packages'
                ELSE 'hooks'
            END AS set_name,
        CASE
            WHEN binding.value->>'kind' IN ('openapi','docs','git') THEN 'documentation'
            WHEN binding.value->>'kind' = 'package' THEN 'package'
            ELSE 'hook'
        END AS set_kind
    FROM product_definitions pd
    CROSS JOIN LATERAL jsonb_array_elements(coalesce(pd.definition->'components', '[]'::jsonb)) component(value)
    CROSS JOIN LATERAL jsonb_array_elements(coalesce(component.value->'releases', '[]'::jsonb)) release(value)
    CROSS JOIN LATERAL jsonb_array_elements(coalesce(release.value->'bindings', '[]'::jsonb)) binding(value)
    JOIN integrations i
      ON i.deployment_id = pd.product_id
     AND i.legacy_component_id = component.value->>'id'
     AND i.legacy_release_id = release.value->>'id'
    WHERE binding.value->>'kind' IN ('openapi','docs','git','package','mcp','tool')
    GROUP BY i.id, i.deployment_id, i.organisation_id, i.display_name, i.version_key,
             CASE
                 WHEN binding.value->>'kind' IN ('openapi','docs','git') THEN 'documentation'
                 WHEN binding.value->>'kind' = 'package' THEN 'package'
                 ELSE 'hook'
             END,
             CASE
                 WHEN binding.value->>'kind' IN ('openapi','docs','git') THEN 'documentation'
                 WHEN binding.value->>'kind' = 'package' THEN 'packages'
                 ELSE 'hooks'
             END
)
INSERT INTO resource_sets(deployment_id, organisation_id, kind, name, legacy_key)
SELECT deployment_id, organisation_id, set_kind, set_name,
       integration_id::text || ':' || set_kind
FROM legacy_sets
ON CONFLICT DO NOTHING;

WITH grouped_bindings AS (
    SELECT
        i.id AS integration_id,
        CASE
            WHEN binding.value->>'kind' IN ('openapi','docs','git') THEN 'documentation'
            WHEN binding.value->>'kind' = 'package' THEN 'package'
            ELSE 'hook'
        END AS set_kind,
        jsonb_agg(binding.value ORDER BY binding.value->>'name') AS manifest
    FROM product_definitions pd
    CROSS JOIN LATERAL jsonb_array_elements(coalesce(pd.definition->'components', '[]'::jsonb)) component(value)
    CROSS JOIN LATERAL jsonb_array_elements(coalesce(component.value->'releases', '[]'::jsonb)) release(value)
    CROSS JOIN LATERAL jsonb_array_elements(coalesce(release.value->'bindings', '[]'::jsonb)) binding(value)
    JOIN integrations i
      ON i.deployment_id = pd.product_id
     AND i.legacy_component_id = component.value->>'id'
     AND i.legacy_release_id = release.value->>'id'
    WHERE binding.value->>'kind' IN ('openapi','docs','git','package','mcp','tool')
    GROUP BY i.id,
             CASE
                 WHEN binding.value->>'kind' IN ('openapi','docs','git') THEN 'documentation'
                 WHEN binding.value->>'kind' = 'package' THEN 'package'
                 ELSE 'hook'
             END
)
INSERT INTO resource_set_revisions(resource_set_id, revision, manifest, content_hash)
SELECT rs.id, 1, grouped.manifest,
       'sha256:' || encode(digest(convert_to(grouped.manifest::text, 'UTF8'), 'sha256'), 'hex')
FROM grouped_bindings grouped
JOIN resource_sets rs
  ON rs.legacy_key = grouped.integration_id::text || ':' || grouped.set_kind
ON CONFLICT (resource_set_id, revision) DO NOTHING;

INSERT INTO integration_resource_bindings(integration_id, resource_set_id)
SELECT i.id, rs.id
FROM integrations i
JOIN resource_sets rs ON rs.legacy_key LIKE i.id::text || ':%'
ON CONFLICT (integration_id, resource_set_id) DO NOTHING;

INSERT INTO integration_revisions(
    integration_id, revision, state, snapshot, manifest_hash, published_at
)
SELECT
    i.id,
    i.revision,
    CASE WHEN i.lifecycle = 'draft' THEN 'draft' ELSE 'published' END,
    jsonb_build_object(
        'family_key', i.family_key,
        'version_key', i.version_key,
        'display_name', i.display_name,
        'description', i.description,
        'lifecycle', i.lifecycle,
        'resource_sets', coalesce((
            SELECT jsonb_agg(jsonb_build_object(
                'id', rs.id,
                'kind', rs.kind,
                'revision_id', rsr.id,
                'revision', rsr.revision,
                'content_hash', rsr.content_hash
            ) ORDER BY rs.kind, rs.name)
            FROM integration_resource_bindings irb
            JOIN resource_sets rs ON rs.id = irb.resource_set_id
            LEFT JOIN LATERAL (
                SELECT value.* FROM resource_set_revisions value
                WHERE value.resource_set_id = rs.id
                ORDER BY value.revision DESC LIMIT 1
            ) rsr ON true
            WHERE irb.integration_id = i.id
        ), '[]'::jsonb)
    ),
    '',
    CASE WHEN i.lifecycle = 'draft' THEN NULL ELSE now() END
FROM integrations i;

UPDATE integration_revisions
SET manifest_hash = 'sha256:' || encode(digest(convert_to(snapshot::text, 'UTF8'), 'sha256'), 'hex')
WHERE manifest_hash = '';
