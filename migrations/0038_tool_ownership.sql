-- Tool ownership is explicit. Namespace remains the stable MCP-facing name,
-- while scope and owner determine where the definition may be managed and
-- attached. Existing tools remain common for compatibility; new API-owned
-- tools set owner_integration_id and are constrained to that API by the
-- service layer.
ALTER TABLE tool_definitions
    ADD COLUMN scope text NOT NULL DEFAULT 'common'
        CHECK (scope IN ('common', 'api')),
    ADD COLUMN owner_integration_id uuid REFERENCES integrations(id) ON DELETE RESTRICT,
    ADD CONSTRAINT tool_definitions_scope_owner_check CHECK (
        (scope = 'common' AND owner_integration_id IS NULL)
        OR (scope = 'api' AND owner_integration_id IS NOT NULL)
    );

CREATE INDEX tool_definitions_owner_idx
    ON tool_definitions(product_id, owner_integration_id, state)
    WHERE owner_integration_id IS NOT NULL;

-- Releases keep the ownership decision that was reviewed at publication.
-- This prevents a later metadata change from rewriting historical intent.
ALTER TABLE tool_releases
    ADD COLUMN scope text NOT NULL DEFAULT 'common'
        CHECK (scope IN ('common', 'api')),
    ADD COLUMN owner_integration_id uuid REFERENCES integrations(id) ON DELETE RESTRICT,
    ADD CONSTRAINT tool_releases_scope_owner_check CHECK (
        (scope = 'common' AND owner_integration_id IS NULL)
        OR (scope = 'api' AND owner_integration_id IS NOT NULL)
    );

UPDATE tool_releases release
SET scope = definition.scope,
    owner_integration_id = definition.owner_integration_id
FROM tool_definitions definition
WHERE definition.id = release.tool_definition_id;
