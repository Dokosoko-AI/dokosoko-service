ALTER TABLE packages
    ADD COLUMN fetch_hook_url text,
    ADD CONSTRAINT packages_delivery_configuration CHECK (
        (mode = 'public') OR
        (mode = 'proxy' AND upstream_url IS NOT NULL) OR
        (mode = 'fetch' AND fetch_hook_url IS NOT NULL)
    );

