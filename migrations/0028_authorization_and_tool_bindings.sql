-- Declarative authorization and exact Integration-to-tool compatibility.
-- Authorization points contain policy only: they can never supply a network
-- destination or replace the fixed access-evaluation contract.

CREATE TABLE grant_definitions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    key text NOT NULL,
    display_name text NOT NULL,
    description text NOT NULL DEFAULT '',
    risk text NOT NULL DEFAULT 'low'
        CHECK (risk IN ('low','medium','high','critical')),
    state text NOT NULL DEFAULT 'active'
        CHECK (state IN ('active','deprecated')),
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (deployment_id, key),
    CHECK (key ~ '^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+$'),
    CHECK (char_length(display_name) BETWEEN 1 AND 120),
    CHECK (char_length(description) <= 1000)
);
CREATE INDEX grant_definitions_deployment_state_idx
    ON grant_definitions(deployment_id, state, key);

CREATE TABLE authorization_points (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    key text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    action_type text NOT NULL
        CHECK (action_type IN ('read','write','destructive')),
    required_grants text[] NOT NULL DEFAULT '{}',
    confirmation_required boolean NOT NULL DEFAULT false,
    decision_ttl_seconds integer NOT NULL DEFAULT 300
        CHECK (decision_ttl_seconds BETWEEN 0 AND 3600),
    state text NOT NULL DEFAULT 'draft'
        CHECK (state IN ('draft','active','deprecated')),
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (integration_id, key),
    CHECK (key ~ '^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+$'),
    CHECK (char_length(name) BETWEEN 1 AND 120),
    CHECK (char_length(description) <= 1000),
    CHECK (cardinality(required_grants) <= 32),
    CHECK (action_type <> 'destructive' OR confirmation_required)
);
CREATE INDEX authorization_points_integration_state_idx
    ON authorization_points(integration_id, state, key);

CREATE TABLE integration_tool_bindings (
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    tool_id uuid NOT NULL REFERENCES tool_definitions(id) ON DELETE CASCADE,
    tool_revision bigint NOT NULL,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (integration_id, tool_id),
    FOREIGN KEY (tool_id, tool_revision)
        REFERENCES tool_releases(tool_definition_id, version) ON DELETE RESTRICT
);
CREATE INDEX integration_tool_bindings_tool_idx
    ON integration_tool_bindings(tool_id, tool_revision);

-- Existing published tools were historically written as release version 1
-- regardless of the definition revision. Preserve the release and make it
-- refer to the exact published definition revision before bindings are added.
UPDATE tool_releases release
SET version = definition.revision
FROM tool_definitions definition
WHERE release.tool_definition_id = definition.id
  AND definition.state = 'published'
  AND release.version <> definition.revision
  AND NOT EXISTS (
      SELECT 1 FROM tool_releases conflict
      WHERE conflict.tool_definition_id = release.tool_definition_id
        AND conflict.version = definition.revision
  );

-- Seed the registry from existing published tool policies. Operators can add
-- descriptions and risk classifications in the console before republishing.
INSERT INTO grant_definitions(
    deployment_id, organisation_id, key, display_name, description, risk, state
)
SELECT DISTINCT
    definition.product_id,
    definition.organisation_id,
    grant_key,
    initcap(replace(grant_key, '.', ' ')),
    'Imported from an existing published tool policy.',
    'medium',
    'active'
FROM tool_definitions definition
CROSS JOIN LATERAL jsonb_array_elements_text(
    coalesce(definition.authorization_policy->'required_grants', '[]'::jsonb)
) grant_key
WHERE definition.state = 'published'
  AND grant_key ~ '^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+$'
ON CONFLICT (deployment_id, key) DO NOTHING;
