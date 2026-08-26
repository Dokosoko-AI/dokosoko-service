-- Historical API publications must not be re-rendered from mutable SDK
-- package display metadata. Snapshot the delivery identity used by indexing
-- and MCP resources at the immutable API-publication boundary.
ALTER TABLE api_publication_sdk_assets
    ADD COLUMN sdk_package_ecosystem text NOT NULL DEFAULT '',
    ADD COLUMN sdk_package_coordinate text NOT NULL DEFAULT '',
    ADD COLUMN sdk_package_display_coordinate text NOT NULL DEFAULT '',
    ADD COLUMN sdk_package_display_name text NOT NULL DEFAULT '',
    ADD COLUMN sdk_package_language text NOT NULL DEFAULT '',
    ADD COLUMN sdk_package_platform text NOT NULL DEFAULT '';

-- This is a one-time immutable-evidence schema backfill. Remove only the
-- table's mutation guard for the duration of this transaction and restore it
-- immediately after populating the new snapshot columns.
DROP TRIGGER developer_asset_immutable_guard ON api_publication_sdk_assets;
DROP TRIGGER developer_asset_visibility_guard ON api_publication_sdk_assets;

UPDATE api_publication_sdk_assets asset
   SET sdk_package_ecosystem = package.ecosystem,
       sdk_package_coordinate = package.canonical_coordinate,
       sdk_package_display_coordinate = package.display_coordinate,
       sdk_package_display_name = package.name,
       sdk_package_language = package.language,
       sdk_package_platform = package.platform
  FROM sdk_packages package
 WHERE package.id = asset.sdk_package_id
   AND package.deployment_id = asset.deployment_id;

CREATE TRIGGER developer_asset_immutable_guard
BEFORE UPDATE OR DELETE ON api_publication_sdk_assets
FOR EACH ROW EXECUTE FUNCTION reject_developer_asset_immutable_mutation();

CREATE TRIGGER developer_asset_visibility_guard
BEFORE INSERT OR UPDATE ON api_publication_sdk_assets
FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_visibility();

ALTER TABLE api_publication_sdk_assets
    ALTER COLUMN sdk_package_ecosystem DROP DEFAULT,
    ALTER COLUMN sdk_package_coordinate DROP DEFAULT,
    ALTER COLUMN sdk_package_display_coordinate DROP DEFAULT,
    ALTER COLUMN sdk_package_display_name DROP DEFAULT,
    ALTER COLUMN sdk_package_language DROP DEFAULT,
    ALTER COLUMN sdk_package_platform DROP DEFAULT,
    ADD CONSTRAINT api_publication_sdk_assets_package_snapshot_check CHECK (
        btrim(sdk_package_ecosystem) <> ''
        AND btrim(sdk_package_coordinate) <> ''
        AND btrim(sdk_package_display_coordinate) <> ''
        AND btrim(sdk_package_display_name) <> ''
    );

CREATE FUNCTION guard_api_publication_sdk_package_snapshot()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    expected sdk_packages%ROWTYPE;
BEGIN
    SELECT * INTO expected
      FROM sdk_packages
     WHERE id = NEW.sdk_package_id
       AND deployment_id = NEW.deployment_id;

    IF NOT FOUND
       OR NEW.sdk_package_ecosystem IS DISTINCT FROM expected.ecosystem
       OR NEW.sdk_package_coordinate IS DISTINCT FROM expected.canonical_coordinate
       OR NEW.sdk_package_display_coordinate IS DISTINCT FROM expected.display_coordinate
       OR NEW.sdk_package_display_name IS DISTINCT FROM expected.name
       OR NEW.sdk_package_language IS DISTINCT FROM expected.language
       OR NEW.sdk_package_platform IS DISTINCT FROM expected.platform THEN
        RAISE EXCEPTION 'API SDK asset package snapshot must match the selected package'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER api_publication_sdk_package_snapshot_guard
BEFORE INSERT ON api_publication_sdk_assets
FOR EACH ROW EXECUTE FUNCTION guard_api_publication_sdk_package_snapshot();
