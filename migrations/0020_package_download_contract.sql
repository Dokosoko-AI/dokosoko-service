ALTER TABLE packages
    DROP CONSTRAINT packages_delivery_configuration;

ALTER TYPE package_mode RENAME VALUE 'fetch' TO 'download';

ALTER TABLE packages
    RENAME COLUMN fetch_hook_url TO download_url;

ALTER TABLE packages
    ADD CONSTRAINT packages_delivery_configuration CHECK (
        (mode = 'public') OR
        (mode = 'proxy' AND upstream_url IS NOT NULL) OR
        (mode = 'download' AND download_url IS NOT NULL)
    );
