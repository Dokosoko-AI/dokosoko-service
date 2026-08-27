-- Promote recipes from single-API-owned records to deployment-owned reusable
-- assets. Existing v2 rows keep their immutable singular binding; v3 rows use
-- many-to-many current attachments and freeze exact API publications per
-- recipe revision.
ALTER TABLE recipes
    DROP CONSTRAINT recipes_contract_version_check,
    DROP CONSTRAINT recipes_contract_integration_check,
    ADD CONSTRAINT recipes_contract_version_check CHECK (
        contract_version IN (
            'legacy-mcp-v1',
            'product-integration-v2',
            'deployment-recipe-v3'
        )
    ),
    ADD CONSTRAINT recipes_contract_integration_check CHECK (
        (contract_version = 'legacy-mcp-v1' AND integration_id IS NULL)
        OR
        (contract_version = 'product-integration-v2' AND integration_id IS NOT NULL)
        OR
        (contract_version = 'deployment-recipe-v3' AND integration_id IS NULL)
    ),
    ADD CONSTRAINT recipes_id_product_unique UNIQUE (id, product_id);

ALTER TABLE recipe_revisions
    ADD CONSTRAINT recipe_revisions_id_recipe_unique UNIQUE (id, recipe_id);

CREATE TABLE recipe_api_attachments (
    recipe_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (recipe_id, integration_id),
    FOREIGN KEY (recipe_id, deployment_id)
        REFERENCES recipes(id, product_id) ON DELETE CASCADE,
    FOREIGN KEY (integration_id, deployment_id)
        REFERENCES integrations(id, deployment_id) ON DELETE CASCADE
);

CREATE INDEX recipe_api_attachments_integration_idx
    ON recipe_api_attachments(integration_id, recipe_id);

CREATE TABLE recipe_revision_api_bindings (
    recipe_revision_id uuid NOT NULL,
    recipe_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    integration_revision_id uuid NOT NULL,
    integration_manifest_hash text NOT NULL CHECK (
        integration_manifest_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    PRIMARY KEY (recipe_revision_id, integration_id),
    FOREIGN KEY (recipe_revision_id, recipe_id)
        REFERENCES recipe_revisions(id, recipe_id) ON DELETE CASCADE,
    FOREIGN KEY (recipe_id, deployment_id)
        REFERENCES recipes(id, product_id) ON DELETE CASCADE,
    FOREIGN KEY (integration_id, deployment_id)
        REFERENCES integrations(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (integration_revision_id, integration_id)
        REFERENCES integration_revisions(id, integration_id) ON DELETE RESTRICT
);

CREATE INDEX recipe_revision_api_bindings_integration_revision_idx
    ON recipe_revision_api_bindings(integration_revision_id);

CREATE OR REPLACE FUNCTION validate_recipe_revision_api_binding()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    published_state text;
    published_hash text;
BEGIN
    SELECT state, manifest_hash
      INTO published_state, published_hash
      FROM integration_revisions
     WHERE id = NEW.integration_revision_id
       AND integration_id = NEW.integration_id;

    IF published_state IS DISTINCT FROM 'published'
       OR published_hash IS DISTINCT FROM NEW.integration_manifest_hash THEN
        RAISE EXCEPTION 'recipe API binding must reference the exact published Integration manifest';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER recipe_revision_api_binding_validate
BEFORE INSERT OR UPDATE ON recipe_revision_api_bindings
FOR EACH ROW EXECUTE FUNCTION validate_recipe_revision_api_binding();

-- Uniformly expose historical v2 recipes through the attachment projection
-- without changing their contract, singular columns, or revision contents.
INSERT INTO recipe_api_attachments(recipe_id, deployment_id, integration_id, created_by)
SELECT id, product_id, integration_id, 'migration:0065'
  FROM recipes
 WHERE contract_version = 'product-integration-v2'
   AND integration_id IS NOT NULL
ON CONFLICT DO NOTHING;

INSERT INTO recipe_revision_api_bindings(
    recipe_revision_id,
    recipe_id,
    deployment_id,
    integration_id,
    integration_revision_id,
    integration_manifest_hash
)
SELECT revision.id,
       recipe.id,
       recipe.product_id,
       recipe.integration_id,
       revision.integration_revision_id,
       revision.integration_manifest_hash
  FROM recipe_revisions revision
  JOIN recipes recipe ON recipe.id = revision.recipe_id
 WHERE recipe.contract_version = 'product-integration-v2'
   AND recipe.integration_id IS NOT NULL
   AND revision.integration_revision_id IS NOT NULL
ON CONFLICT DO NOTHING;
