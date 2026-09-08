-- SDK attachments are mutable selections. Publications and AI advisories pin
-- immutable release/content/assertion rows, rather than the attachment's current
-- selection. The original composite foreign keys prevented an attachment from
-- advancing after its first publication (or applicability advisory).

ALTER TABLE api_sdk_bindings
    ADD CONSTRAINT api_sdk_binding_stable_identity
        UNIQUE (id, integration_id, sdk_package_id, deployment_id);

ALTER TABLE api_publication_sdk_assets
    DROP CONSTRAINT api_publication_sdk_assets_api_sdk_binding_id_integration__fkey,
    DROP CONSTRAINT api_publication_sdk_assets_api_sdk_binding_id_sdk_content__fkey,
    DROP CONSTRAINT api_publication_sdk_assets_api_sdk_binding_id_compatibilit_fkey,
    ADD CONSTRAINT api_publication_sdk_binding_identity
        FOREIGN KEY (api_sdk_binding_id, integration_id, sdk_package_id, deployment_id)
        REFERENCES api_sdk_bindings(id, integration_id, sdk_package_id, deployment_id)
        ON DELETE RESTRICT,
    ADD CONSTRAINT api_publication_sdk_assertion_identity
        FOREIGN KEY (compatibility_assertion_id, integration_id, sdk_release_id)
        REFERENCES sdk_compatibility_assertions(id, integration_id, sdk_release_id)
        ON DELETE RESTRICT;

ALTER TABLE developer_asset_ai_advisory_runs
    DROP CONSTRAINT developer_asset_ai_advisory_r_api_sdk_binding_id_integrati_fkey,
    ADD CONSTRAINT developer_asset_advisory_binding_identity
        FOREIGN KEY (api_sdk_binding_id, integration_id, sdk_package_id, deployment_id)
        REFERENCES api_sdk_bindings(id, integration_id, sdk_package_id, deployment_id)
        ON DELETE RESTRICT;

-- Preserve exact selection validation at insertion time. Lock the mutable
-- attachment until the snapshot transaction completes so an update cannot race
-- the release/content/assertion/selector check. Existing immutable publication
-- and advisory lineage guards and direct immutable-asset foreign keys remain.
CREATE FUNCTION guard_sdk_binding_publication_snapshot()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    binding api_sdk_bindings%ROWTYPE;
BEGIN
    SELECT * INTO binding FROM api_sdk_bindings
     WHERE id = NEW.api_sdk_binding_id
     FOR SHARE;
    IF NOT FOUND
       OR NEW.integration_id IS DISTINCT FROM binding.integration_id
       OR NEW.deployment_id IS DISTINCT FROM binding.deployment_id
       OR NEW.sdk_package_id IS DISTINCT FROM binding.sdk_package_id
       OR NEW.sdk_release_id IS DISTINCT FROM binding.sdk_release_id
       OR NEW.sdk_content_publication_id IS DISTINCT FROM binding.sdk_content_publication_id
       OR NEW.compatibility_assertion_id IS DISTINCT FROM binding.compatibility_assertion_id
       OR NEW.selector IS DISTINCT FROM binding.selector
       OR NEW.selector_hash IS DISTINCT FROM binding.selector_hash THEN
        RAISE EXCEPTION 'API SDK snapshot must match the exact current attachment selection'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

-- Trigger names run alphabetically; lock before the existing member hash guard.
CREATE TRIGGER developer_asset_00_sdk_binding_snapshot_guard
BEFORE INSERT ON api_publication_sdk_assets
FOR EACH ROW EXECUTE FUNCTION guard_sdk_binding_publication_snapshot();
