-- Keep mutable documentation binding selectors and immutable API publication
-- evidence on one database-owned canonical hash. jsonb::text is stable inside
-- PostgreSQL, while client JSON encoders may choose different whitespace.

ALTER TABLE api_documentation_bindings
    ADD COLUMN selector_hash text;

UPDATE api_documentation_bindings
   SET selector_hash = 'sha256:' || encode(
       digest(convert_to(selector::text, 'UTF8'), 'sha256'), 'hex'
   );

ALTER TABLE api_documentation_bindings
    ALTER COLUMN selector_hash SET NOT NULL,
    ADD CONSTRAINT api_documentation_bindings_selector_hash_check
        CHECK (selector_hash ~ '^sha256:[0-9a-f]{64}$');

CREATE FUNCTION set_api_documentation_binding_selector_hash()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.selector_hash := 'sha256:' || encode(
        digest(convert_to(NEW.selector::text, 'UTF8'), 'sha256'), 'hex'
    );
    RETURN NEW;
END;
$$;

CREATE TRIGGER api_documentation_bindings_selector_hash_trigger
BEFORE INSERT OR UPDATE ON api_documentation_bindings
FOR EACH ROW EXECUTE FUNCTION set_api_documentation_binding_selector_hash();

-- The original guard recomputed the publication child's hash. Compare the
-- child to the mutable binding's exact selector and database-owned hash, just
-- as SDK publication evidence already does.
CREATE OR REPLACE FUNCTION guard_developer_asset_publication_member()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    binding_selector jsonb;
    binding_selector_hash text;
    binding_follow_latest boolean;
    binding_pinned_revision_id uuid;
    binding_lifecycle text;
    binding_primary boolean;
    binding_content_publication_id uuid;
    binding_assertion_id uuid;
    selected_content_hash text;
    latest_revision_id uuid;
    expected_content_hash text;
BEGIN
    CASE TG_TABLE_NAME
    WHEN 'deployment_documentation_publication_members' THEN
        SELECT content_hash INTO selected_content_hash
          FROM documentation_collection_revisions
         WHERE id = NEW.documentation_collection_revision_id;
        IF NEW.content_hash IS DISTINCT FROM selected_content_hash THEN
            RAISE EXCEPTION 'global documentation member must pin the exact collection revision hash'
                USING ERRCODE = '23514';
        END IF;
    WHEN 'api_publication_documentation_assets' THEN
        SELECT binding.selector, binding.selector_hash, binding.follow_latest,
               binding.pinned_revision_id, binding.lifecycle, revision.content_hash
          INTO binding_selector, binding_selector_hash, binding_follow_latest,
               binding_pinned_revision_id, binding_lifecycle, selected_content_hash
          FROM api_documentation_bindings binding
          JOIN documentation_collection_revisions revision
            ON revision.id = NEW.documentation_collection_revision_id
         WHERE binding.id = NEW.api_documentation_binding_id;
        IF binding_follow_latest THEN
            SELECT id INTO latest_revision_id
              FROM documentation_collection_revisions
             WHERE documentation_collection_id = NEW.documentation_collection_id
             ORDER BY revision DESC LIMIT 1;
        ELSE
            latest_revision_id := binding_pinned_revision_id;
        END IF;
        IF binding_lifecycle IS DISTINCT FROM 'attached'
           OR NEW.documentation_collection_revision_id IS DISTINCT FROM latest_revision_id
           OR NEW.selector IS DISTINCT FROM binding_selector
           OR NEW.selector_hash IS DISTINCT FROM binding_selector_hash
           OR NEW.content_hash IS DISTINCT FROM selected_content_hash THEN
            RAISE EXCEPTION 'API documentation snapshot must resolve the exact active binding revision and selector'
                USING ERRCODE = '23514';
        END IF;
    WHEN 'api_publication_contract_assets' THEN
        SELECT binding.follow_latest, binding.pinned_revision_id, binding.lifecycle,
               binding.primary_contract, revision.content_hash
          INTO binding_follow_latest, binding_pinned_revision_id, binding_lifecycle,
               binding_primary, selected_content_hash
          FROM api_contract_bindings binding
          JOIN api_contract_revisions revision ON revision.id = NEW.api_contract_revision_id
         WHERE binding.id = NEW.api_contract_binding_id;
        IF binding_follow_latest THEN
            SELECT id INTO latest_revision_id
              FROM api_contract_revisions
             WHERE api_contract_id = NEW.api_contract_id
             ORDER BY revision DESC LIMIT 1;
        ELSE
            latest_revision_id := binding_pinned_revision_id;
        END IF;
        IF binding_lifecycle IS DISTINCT FROM 'attached'
           OR NEW.api_contract_revision_id IS DISTINCT FROM latest_revision_id
           OR NEW.primary_contract IS DISTINCT FROM binding_primary
           OR NEW.content_hash IS DISTINCT FROM selected_content_hash THEN
            RAISE EXCEPTION 'API contract snapshot must resolve the exact active binding revision'
                USING ERRCODE = '23514';
        END IF;
    WHEN 'api_publication_sdk_assets' THEN
        SELECT binding.sdk_content_publication_id, binding.compatibility_assertion_id,
               binding.selector, binding.selector_hash,
               coalesce(publication.content_hash, release.release_hash)
          INTO binding_content_publication_id, binding_assertion_id,
               binding_selector, binding_selector_hash, expected_content_hash
          FROM api_sdk_bindings binding
          JOIN sdk_releases release ON release.id = binding.sdk_release_id
          LEFT JOIN sdk_content_publications publication
            ON publication.id = binding.sdk_content_publication_id
         WHERE binding.id = NEW.api_sdk_binding_id;
        IF NEW.sdk_content_publication_id IS DISTINCT FROM binding_content_publication_id
           OR NEW.compatibility_assertion_id IS DISTINCT FROM binding_assertion_id
           OR NEW.selector IS DISTINCT FROM binding_selector
           OR NEW.selector_hash IS DISTINCT FROM binding_selector_hash
           OR NEW.content_hash IS DISTINCT FROM expected_content_hash THEN
            RAISE EXCEPTION 'API SDK snapshot must copy the exact binding release, content, assertion, and selector'
                USING ERRCODE = '23514';
        END IF;
    ELSE
        RAISE EXCEPTION 'unsupported developer-asset publication member table %', TG_TABLE_NAME;
    END CASE;
    RETURN NEW;
END;
$$;
