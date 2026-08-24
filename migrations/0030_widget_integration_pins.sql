-- Widgets must never follow mutable Integration rows. Activation pins the
-- complete immutable publication (including its documentation, API contracts,
-- packages, authorization points, and exact tool revisions) into the widget
-- configuration updated under the widget's optimistic revision lock.

ALTER TABLE widgets
    ADD COLUMN integration_bindings jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD CONSTRAINT widgets_integration_bindings_array
        CHECK (jsonb_typeof(integration_bindings) = 'array');

-- Preserve active installations by pinning every selected Integration to the
-- latest immutable publication that existed at migration time. Widgets with an
-- unresolved selection fail closed and must be reviewed and reactivated.
UPDATE widgets widget
SET integration_bindings = coalesce((
    SELECT jsonb_agg(
        jsonb_build_object(
            'integration_id', selected.integration_id::text,
            'integration_revision_id', publication.id::text,
            'integration_revision', publication.revision,
            'manifest_hash', publication.manifest_hash,
            'snapshot', publication.snapshot,
            'bound_at', now()
        )
        ORDER BY selected.position
    )
    FROM unnest(widget.integration_ids) WITH ORDINALITY
        AS selected(integration_id, position)
    JOIN LATERAL (
        SELECT revision.*
        FROM integration_revisions revision
        WHERE revision.integration_id = selected.integration_id
          AND revision.state = 'published'
        ORDER BY revision.revision DESC
        LIMIT 1
    ) publication ON true
), '[]'::jsonb);

UPDATE widgets
SET state = 'disabled',
    updated_at = now(),
    revision = revision + 1
WHERE state = 'active'
  AND (
      cardinality(integration_ids) = 0
      OR jsonb_array_length(integration_bindings) <> cardinality(integration_ids)
      OR EXISTS (
          SELECT 1
          FROM jsonb_array_elements(integration_bindings) binding
          WHERE coalesce(binding -> 'snapshot' ->> 'visibility', 'private') <> 'public'
            AND NOT (
                coalesce(jsonb_typeof(binding -> 'snapshot' -> 'resource_sets') = 'array', false)
                AND EXISTS (
                    SELECT 1
                    FROM jsonb_array_elements(CASE
                        WHEN jsonb_typeof(binding -> 'snapshot' -> 'resource_sets') = 'array'
                        THEN binding -> 'snapshot' -> 'resource_sets'
                        ELSE '[]'::jsonb
                    END) resource
                    WHERE resource ->> 'kind' = 'documentation'
                      AND CASE WHEN jsonb_typeof(resource -> 'source_publications') = 'array' THEN jsonb_array_length(resource -> 'source_publications') > 0 ELSE false END
                )
                AND EXISTS (
                    SELECT 1
                    FROM jsonb_array_elements(CASE
                        WHEN jsonb_typeof(binding -> 'snapshot' -> 'resource_sets') = 'array'
                        THEN binding -> 'snapshot' -> 'resource_sets'
                        ELSE '[]'::jsonb
                    END) resource
                    WHERE resource ->> 'kind' = 'api'
                )
                AND CASE WHEN jsonb_typeof(binding -> 'snapshot' -> 'authorization_points') = 'array' THEN jsonb_array_length(binding -> 'snapshot' -> 'authorization_points') > 0 ELSE false END
                AND CASE WHEN jsonb_typeof(binding -> 'snapshot' -> 'tools') = 'array' THEN jsonb_array_length(binding -> 'snapshot' -> 'tools') > 0 ELSE false END
                AND CASE WHEN jsonb_typeof(binding -> 'snapshot' -> 'access_connections') = 'array' THEN jsonb_array_length(binding -> 'snapshot' -> 'access_connections') > 0 ELSE false END
            )
      )
  );
