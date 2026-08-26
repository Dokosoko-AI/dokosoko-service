-- Historical developer-asset rendering must never dereference mutable root
-- records. Existing rows predate these snapshots, so the only conservative
-- backfill available is the root identity visible at migration time. Every new
-- revision and API publication captures the exact identity at creation.

-- These four evidence tables are immutable. Temporarily remove both mutation
-- and visibility triggers because PostgreSQL fires UPDATE triggers even when
-- only newly-added snapshot columns change; historical bindings/visibility may
-- legitimately differ from today's mutable roots.
DROP TRIGGER developer_asset_immutable_guard ON documentation_collection_revisions;
DROP TRIGGER developer_asset_visibility_guard ON documentation_collection_revisions;
DROP TRIGGER developer_asset_immutable_guard ON api_contract_revisions;
DROP TRIGGER developer_asset_visibility_guard ON api_contract_revisions;
DROP TRIGGER developer_asset_immutable_guard ON api_publication_documentation_assets;
DROP TRIGGER developer_asset_visibility_guard ON api_publication_documentation_assets;
DROP TRIGGER developer_asset_immutable_guard ON api_publication_contract_assets;
DROP TRIGGER developer_asset_visibility_guard ON api_publication_contract_assets;

ALTER TABLE documentation_collection_revisions
    ADD COLUMN documentation_collection_name text,
    ADD COLUMN documentation_collection_slug text,
    ADD COLUMN documentation_collection_description text;

UPDATE documentation_collection_revisions revision
   SET documentation_collection_name = collection.name,
       documentation_collection_slug = collection.slug,
       documentation_collection_description = collection.description
  FROM documentation_collections collection
 WHERE collection.id = revision.documentation_collection_id;

ALTER TABLE documentation_collection_revisions
    ALTER COLUMN documentation_collection_name SET NOT NULL,
    ALTER COLUMN documentation_collection_slug SET NOT NULL,
    ALTER COLUMN documentation_collection_description SET NOT NULL,
    ADD CONSTRAINT documentation_collection_revisions_snapshot_name_check
        CHECK (btrim(documentation_collection_name) <> ''),
    ADD CONSTRAINT documentation_collection_revisions_snapshot_slug_check
        CHECK (documentation_collection_slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$');

ALTER TABLE api_contract_revisions
    ADD COLUMN api_contract_name text,
    ADD COLUMN api_contract_slug text,
    ADD COLUMN api_contract_description text,
    ADD COLUMN api_contract_kind text;

UPDATE api_contract_revisions revision
   SET api_contract_name = contract.name,
       api_contract_slug = contract.slug,
       api_contract_description = contract.description,
       api_contract_kind = contract.contract_kind
  FROM api_contracts contract
 WHERE contract.id = revision.api_contract_id;

ALTER TABLE api_contract_revisions
    ALTER COLUMN api_contract_name SET NOT NULL,
    ALTER COLUMN api_contract_slug SET NOT NULL,
    ALTER COLUMN api_contract_description SET NOT NULL,
    ALTER COLUMN api_contract_kind SET NOT NULL,
    ADD CONSTRAINT api_contract_revisions_snapshot_name_check
        CHECK (btrim(api_contract_name) <> ''),
    ADD CONSTRAINT api_contract_revisions_snapshot_slug_check
        CHECK (api_contract_slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'),
    ADD CONSTRAINT api_contract_revisions_snapshot_kind_check
        CHECK (api_contract_kind = 'openapi');

ALTER TABLE api_publication_documentation_assets
    ADD COLUMN documentation_collection_name text,
    ADD COLUMN documentation_collection_slug text,
    ADD COLUMN documentation_collection_description text;

UPDATE api_publication_documentation_assets asset
   SET documentation_collection_name = revision.documentation_collection_name,
       documentation_collection_slug = revision.documentation_collection_slug,
       documentation_collection_description = revision.documentation_collection_description
  FROM documentation_collection_revisions revision
 WHERE revision.id = asset.documentation_collection_revision_id;

ALTER TABLE api_publication_documentation_assets
    ALTER COLUMN documentation_collection_name SET NOT NULL,
    ALTER COLUMN documentation_collection_slug SET NOT NULL,
    ALTER COLUMN documentation_collection_description SET NOT NULL,
    ADD CONSTRAINT api_publication_documentation_assets_snapshot_name_check
        CHECK (btrim(documentation_collection_name) <> ''),
    ADD CONSTRAINT api_publication_documentation_assets_snapshot_slug_check
        CHECK (documentation_collection_slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$');

ALTER TABLE api_publication_contract_assets
    ADD COLUMN api_contract_name text,
    ADD COLUMN api_contract_slug text,
    ADD COLUMN api_contract_description text,
    ADD COLUMN api_contract_kind text;

UPDATE api_publication_contract_assets asset
   SET api_contract_name = revision.api_contract_name,
       api_contract_slug = revision.api_contract_slug,
       api_contract_description = revision.api_contract_description,
       api_contract_kind = revision.api_contract_kind
  FROM api_contract_revisions revision
 WHERE revision.id = asset.api_contract_revision_id;

ALTER TABLE api_publication_contract_assets
    ALTER COLUMN api_contract_name SET NOT NULL,
    ALTER COLUMN api_contract_slug SET NOT NULL,
    ALTER COLUMN api_contract_description SET NOT NULL,
    ALTER COLUMN api_contract_kind SET NOT NULL,
    ADD CONSTRAINT api_publication_contract_assets_snapshot_name_check
        CHECK (btrim(api_contract_name) <> ''),
    ADD CONSTRAINT api_publication_contract_assets_snapshot_slug_check
        CHECK (api_contract_slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'),
    ADD CONSTRAINT api_publication_contract_assets_snapshot_kind_check
        CHECK (api_contract_kind = 'openapi');

CREATE TRIGGER developer_asset_immutable_guard
BEFORE UPDATE OR DELETE ON documentation_collection_revisions
FOR EACH ROW EXECUTE FUNCTION reject_developer_asset_immutable_mutation();
CREATE TRIGGER developer_asset_visibility_guard
BEFORE INSERT OR UPDATE ON documentation_collection_revisions
FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_visibility();

CREATE TRIGGER developer_asset_immutable_guard
BEFORE UPDATE OR DELETE ON api_contract_revisions
FOR EACH ROW EXECUTE FUNCTION reject_developer_asset_immutable_mutation();
CREATE TRIGGER developer_asset_visibility_guard
BEFORE INSERT OR UPDATE ON api_contract_revisions
FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_visibility();

CREATE TRIGGER developer_asset_immutable_guard
BEFORE UPDATE OR DELETE ON api_publication_documentation_assets
FOR EACH ROW EXECUTE FUNCTION reject_developer_asset_immutable_mutation();
CREATE TRIGGER developer_asset_visibility_guard
BEFORE INSERT OR UPDATE ON api_publication_documentation_assets
FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_visibility();

CREATE TRIGGER developer_asset_immutable_guard
BEFORE UPDATE OR DELETE ON api_publication_contract_assets
FOR EACH ROW EXECUTE FUNCTION reject_developer_asset_immutable_mutation();
CREATE TRIGGER developer_asset_visibility_guard
BEFORE INSERT OR UPDATE ON api_publication_contract_assets
FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_visibility();

CREATE FUNCTION guard_documentation_collection_revision_identity_snapshot()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    expected documentation_collections%ROWTYPE;
BEGIN
    SELECT * INTO expected
      FROM documentation_collections
     WHERE id = NEW.documentation_collection_id
       AND deployment_id = NEW.deployment_id;
    IF NOT FOUND
       OR NEW.documentation_collection_name IS DISTINCT FROM expected.name
       OR NEW.documentation_collection_slug IS DISTINCT FROM expected.slug
       OR NEW.documentation_collection_description IS DISTINCT FROM expected.description THEN
        RAISE EXCEPTION 'documentation revision identity snapshot must match its collection'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER documentation_collection_revision_identity_snapshot_guard
BEFORE INSERT ON documentation_collection_revisions
FOR EACH ROW EXECUTE FUNCTION guard_documentation_collection_revision_identity_snapshot();

CREATE FUNCTION guard_api_contract_revision_identity_snapshot()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    expected api_contracts%ROWTYPE;
BEGIN
    SELECT * INTO expected
      FROM api_contracts
     WHERE id = NEW.api_contract_id
       AND deployment_id = NEW.deployment_id;
    IF NOT FOUND
       OR NEW.api_contract_name IS DISTINCT FROM expected.name
       OR NEW.api_contract_slug IS DISTINCT FROM expected.slug
       OR NEW.api_contract_description IS DISTINCT FROM expected.description
       OR NEW.api_contract_kind IS DISTINCT FROM expected.contract_kind THEN
        RAISE EXCEPTION 'API contract revision identity snapshot must match its contract'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER api_contract_revision_identity_snapshot_guard
BEFORE INSERT ON api_contract_revisions
FOR EACH ROW EXECUTE FUNCTION guard_api_contract_revision_identity_snapshot();

CREATE FUNCTION guard_api_publication_documentation_identity_snapshot()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    expected documentation_collection_revisions%ROWTYPE;
BEGIN
    SELECT * INTO expected
      FROM documentation_collection_revisions
     WHERE id = NEW.documentation_collection_revision_id
       AND documentation_collection_id = NEW.documentation_collection_id;
    IF NOT FOUND
       OR NEW.documentation_collection_name IS DISTINCT FROM expected.documentation_collection_name
       OR NEW.documentation_collection_slug IS DISTINCT FROM expected.documentation_collection_slug
       OR NEW.documentation_collection_description IS DISTINCT FROM expected.documentation_collection_description THEN
        RAISE EXCEPTION 'API documentation asset identity snapshot must match its exact revision'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER api_publication_documentation_identity_snapshot_guard
BEFORE INSERT ON api_publication_documentation_assets
FOR EACH ROW EXECUTE FUNCTION guard_api_publication_documentation_identity_snapshot();

CREATE FUNCTION guard_api_publication_contract_identity_snapshot()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    expected api_contract_revisions%ROWTYPE;
BEGIN
    SELECT * INTO expected
      FROM api_contract_revisions
     WHERE id = NEW.api_contract_revision_id
       AND api_contract_id = NEW.api_contract_id;
    IF NOT FOUND
       OR NEW.api_contract_name IS DISTINCT FROM expected.api_contract_name
       OR NEW.api_contract_slug IS DISTINCT FROM expected.api_contract_slug
       OR NEW.api_contract_description IS DISTINCT FROM expected.api_contract_description
       OR NEW.api_contract_kind IS DISTINCT FROM expected.api_contract_kind THEN
        RAISE EXCEPTION 'API contract asset identity snapshot must match its exact revision'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER api_publication_contract_identity_snapshot_guard
BEFORE INSERT ON api_publication_contract_assets
FOR EACH ROW EXECUTE FUNCTION guard_api_publication_contract_identity_snapshot();
