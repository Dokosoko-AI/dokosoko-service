-- API developer-asset publications are exact companions to API revisions.
-- Identical asset selections may legitimately appear in multiple API
-- revisions (for example, when visibility changes), so the asset hash is not
-- an identity key across revisions.
ALTER TABLE api_developer_asset_publications
    DROP CONSTRAINT IF EXISTS api_developer_asset_publications_integration_id_snapshot_hash_key;

-- Publication visibility must be validated against the immutable API revision
-- snapshot. Consulting the mutable integrations row makes historical retries
-- and index rebuilds dependent on today's draft state.
CREATE OR REPLACE FUNCTION guard_api_developer_asset_publication()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    revision_state text;
    revision_published_at timestamptz;
    api_visibility text;
    global_visibility text;
BEGIN
    SELECT revision.state, revision.published_at,
           coalesce(nullif(revision.snapshot->>'visibility', ''), 'private')
      INTO revision_state, revision_published_at, api_visibility
      FROM integration_revisions revision
     WHERE revision.id = NEW.integration_revision_id
       AND revision.integration_id = NEW.integration_id;
    IF revision_state IS DISTINCT FROM 'published' OR revision_published_at IS NULL THEN
        RAISE EXCEPTION 'developer-asset publication requires an exact published API revision'
            USING ERRCODE = '23514';
    END IF;
    IF api_visibility NOT IN ('private', 'public') THEN
        RAISE EXCEPTION 'developer-asset publication requires a valid immutable API visibility'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.published_at IS DISTINCT FROM revision_published_at THEN
        RAISE EXCEPTION 'developer-asset publication timestamp must match the API revision'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.deployment_documentation_publication_id IS NOT NULL THEN
        SELECT visibility INTO global_visibility
          FROM deployment_documentation_publications
         WHERE id = NEW.deployment_documentation_publication_id
           AND deployment_id = NEW.deployment_id;
        IF api_visibility = 'public' AND global_visibility IS DISTINCT FROM 'public' THEN
            RAISE EXCEPTION 'public API publication cannot select private global documentation'
                USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

-- Search projections use the same immutable visibility boundary. This keeps a
-- historical generation valid when the API root is edited later.
CREATE OR REPLACE FUNCTION developer_asset_publication_visibility(
    value_deployment_id uuid,
    value_publication_kind text,
    value_publication_id uuid
)
RETURNS text
LANGUAGE plpgsql
STABLE
AS $$
DECLARE
    value_visibility text;
BEGIN
    CASE value_publication_kind
    WHEN 'source' THEN
        SELECT visibility::text INTO value_visibility
          FROM source_publications
         WHERE id = value_publication_id AND product_id = value_deployment_id;
    WHEN 'documentation_collection' THEN
        SELECT visibility INTO value_visibility
          FROM documentation_collection_revisions
         WHERE id = value_publication_id AND deployment_id = value_deployment_id;
    WHEN 'global_documentation' THEN
        SELECT visibility INTO value_visibility
          FROM deployment_documentation_publications
         WHERE id = value_publication_id AND deployment_id = value_deployment_id;
    WHEN 'contract' THEN
        SELECT visibility INTO value_visibility
          FROM api_contract_revisions
         WHERE id = value_publication_id AND deployment_id = value_deployment_id;
    WHEN 'sdk' THEN
        SELECT visibility INTO value_visibility
          FROM sdk_content_publications
         WHERE id = value_publication_id AND deployment_id = value_deployment_id;
    WHEN 'api' THEN
        SELECT coalesce(nullif(revision.snapshot->>'visibility', ''), 'private')
          INTO value_visibility
          FROM api_developer_asset_publications publication
          JOIN integration_revisions revision
            ON revision.id = publication.integration_revision_id
           AND revision.integration_id = publication.integration_id
         WHERE publication.id = value_publication_id
           AND publication.deployment_id = value_deployment_id;
    ELSE
        RAISE EXCEPTION 'unsupported developer-asset publication kind %', value_publication_kind
            USING ERRCODE = '23514';
    END CASE;
    IF value_visibility IS NULL THEN
        RAISE EXCEPTION 'developer-asset publication %/% does not exist in deployment %',
            value_publication_kind, value_publication_id, value_deployment_id
            USING ERRCODE = '23503';
    END IF;
    RETURN value_visibility;
END;
$$;
