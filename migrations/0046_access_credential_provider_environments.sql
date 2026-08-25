-- API Admin credentials use the provider's environment identifier. Requiring a
-- legacy DokoSoko environment UUID couples the reusable access contract to an
-- unrelated catalog record and prevents deployments with no legacy environments
-- from persisting otherwise successful provider issuance.
ALTER TABLE access_credentials
    DROP CONSTRAINT access_credentials_environment_id_fkey;

ALTER TABLE access_credentials
    ALTER COLUMN environment_id TYPE text USING environment_id::text;

ALTER TABLE access_credentials
    ADD CONSTRAINT access_credentials_environment_id_valid
    CHECK (environment_id = btrim(environment_id) AND length(environment_id) BETWEEN 1 AND 200);
