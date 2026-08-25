-- The widget is a delivery surface for the same reviewed implementation
-- guidance exposed through DokoSoko's MCP resources. Pin exact recipe
-- revisions at activation and keep bounded conversation context per session.

ALTER TABLE widgets
    ADD COLUMN knowledge_bindings jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD CONSTRAINT widgets_knowledge_bindings_array
        CHECK (jsonb_typeof(knowledge_bindings) = 'array');

-- Existing active widgets receive the published, current recipes already
-- scoped to one of their pinned Integrations. Later recipe publications remain
-- opt-in: reactivating the widget creates a new exact knowledge snapshot.
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
            'bound_at', now()
        )
        ORDER BY recipe.slug
    )
    FROM recipes recipe
    JOIN recipe_revisions revision ON revision.id = recipe.current_revision_id
    JOIN LATERAL (
        SELECT jsonb_agg(dependency ->> 'resource_id' ORDER BY dependency ->> 'resource_id') AS integration_ids
        FROM jsonb_array_elements(recipe.dependencies) dependency
        WHERE dependency ->> 'kind' IN ('integration', 'integration_scope')
          AND dependency ->> 'resource_id' = ANY(widget.integration_ids::text[])
    ) recipe_scope ON recipe_scope.integration_ids IS NOT NULL
    WHERE recipe.product_id = widget.deployment_id
      AND recipe.state = 'published'
      AND NOT recipe.needs_attention
), '[]'::jsonb)
WHERE widget.state = 'active';

CREATE TABLE widget_agent_messages (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES widget_sessions(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('user', 'assistant')),
    content text NOT NULL CHECK (length(content) BETWEEN 1 AND 12000),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX widget_agent_messages_session_created_idx
    ON widget_agent_messages(session_id, created_at DESC, id DESC);
