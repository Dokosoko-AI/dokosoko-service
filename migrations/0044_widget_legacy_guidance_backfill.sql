-- 0043 could only pin recipes that were still current at upgrade time. An
-- already-active widget may legitimately predate later evidence drift: in that
-- case its closest migration equivalent is the last explicitly published
-- immutable recipe revision, not an empty guidance snapshot. New activations
-- still require current reviewed guidance in application code.

UPDATE widgets widget
SET knowledge_bindings = coalesce((
    SELECT jsonb_agg(
        jsonb_build_object(
            'recipe_id', recipe.id::text,
            'recipe_revision_id', revision.id::text,
            'recipe_revision', revision.revision,
            'integration_ids', recipe_scope.integration_ids,
            'title', recipe.title,
            'outcome', recipe.outcome,
            'audience', recipe.audience,
            'stable_uri', recipe.stable_uri,
            'markdown', revision.markdown,
            'references', revision.reference_items,
            'content_hash', 'sha256:' || encode(digest(revision.markdown || revision.reference_items::text, 'sha256'), 'hex'),
            'bound_at', coalesce(widget.activated_at, now())
        )
        ORDER BY recipe.slug
    )
    FROM recipes recipe
    JOIN recipe_revisions revision ON revision.id = recipe.current_revision_id
    JOIN LATERAL (
        SELECT jsonb_agg(resource_id ORDER BY resource_id) AS integration_ids
        FROM (
            SELECT DISTINCT dependency ->> 'resource_id' AS resource_id
            FROM jsonb_array_elements(recipe.dependencies) dependency
            WHERE dependency ->> 'kind' IN ('integration', 'integration_scope')
              AND dependency ->> 'resource_id' = ANY(widget.integration_ids::text[])
        ) scoped_integrations
    ) recipe_scope ON recipe_scope.integration_ids IS NOT NULL
    WHERE recipe.product_id = widget.deployment_id
      AND recipe.published_at IS NOT NULL
      AND recipe.state IN ('published', 'outdated')
), '[]'::jsonb)
WHERE widget.state = 'active'
  AND jsonb_array_length(widget.knowledge_bindings) = 0;
