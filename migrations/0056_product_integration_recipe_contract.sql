-- Recipes are implementation guidance for one exact product Integration. MCP
-- is only their delivery mechanism and is not part of the recipe contract.
-- Keep legacy recipe revisions intact for audit/history, but withdraw every
-- legacy recipe until it has been regenerated and explicitly reviewed under
-- the product-integration contract.
ALTER TABLE recipes
    ADD COLUMN integration_id uuid,
    ADD COLUMN contract_version text NOT NULL DEFAULT 'legacy-mcp-v1',
    ADD CONSTRAINT recipes_contract_version_check CHECK (
        contract_version IN ('legacy-mcp-v1', 'product-integration-v2')
    ),
    ADD CONSTRAINT recipes_integration_deployment_fk
        FOREIGN KEY (integration_id, product_id)
        REFERENCES integrations(id, deployment_id) ON DELETE CASCADE,
    ADD CONSTRAINT recipes_contract_integration_check CHECK (
        (contract_version = 'legacy-mcp-v1' AND integration_id IS NULL)
        OR
        (contract_version = 'product-integration-v2' AND integration_id IS NOT NULL)
    );

UPDATE recipes
SET state = 'outdated',
    needs_attention = true,
    approved_by = '',
    approved_at = NULL,
    published_at = NULL,
    revision = revision + 1,
    updated_at = now()
WHERE contract_version = 'legacy-mcp-v1';

ALTER TABLE recipe_revisions
    ADD COLUMN spec_version integer NOT NULL DEFAULT 1,
    ADD COLUMN spec jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN integration_revision_id uuid
        REFERENCES integration_revisions(id) ON DELETE RESTRICT,
    ADD COLUMN integration_manifest_hash text NOT NULL DEFAULT '',
    ADD COLUMN prompt_version text NOT NULL DEFAULT '',
    ADD COLUMN prompt_hash text NOT NULL DEFAULT '',
    ADD CONSTRAINT recipe_revisions_spec_version_check CHECK (spec_version > 0),
    ADD CONSTRAINT recipe_revisions_spec_object_check CHECK (
        jsonb_typeof(spec) = 'object'
    ),
    ADD CONSTRAINT recipe_revisions_integration_binding_check CHECK (
        (integration_revision_id IS NULL AND integration_manifest_hash = '')
        OR
        (integration_revision_id IS NOT NULL
            AND integration_manifest_hash ~ '^sha256:[0-9a-f]{64}$')
    ),
    ADD CONSTRAINT recipe_revisions_prompt_hash_check CHECK (
        prompt_hash = '' OR prompt_hash ~ '^sha256:[0-9a-f]{64}$'
    );

CREATE INDEX recipes_integration_state_idx
    ON recipes(integration_id, state, updated_at DESC)
    WHERE integration_id IS NOT NULL;

CREATE INDEX recipe_revisions_integration_revision_idx
    ON recipe_revisions(integration_revision_id)
    WHERE integration_revision_id IS NOT NULL;
