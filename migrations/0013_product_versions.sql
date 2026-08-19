ALTER TABLE products
    ADD COLUMN description text NOT NULL DEFAULT '',
    ADD COLUMN default_version_policy text NOT NULL DEFAULT 'latest'
        CHECK (default_version_policy IN ('latest','lts'));

ALTER TABLE connector_releases
    ADD COLUMN display_version text,
    ADD COLUMN profile_id text NOT NULL DEFAULT '',
    ADD COLUMN profile_name text NOT NULL DEFAULT '',
    ADD COLUMN definition_revision bigint NOT NULL DEFAULT 0,
    ADD COLUMN is_latest boolean NOT NULL DEFAULT false,
    ADD COLUMN is_lts boolean NOT NULL DEFAULT false,
    ADD COLUMN deprecated_at timestamptz,
    ADD COLUMN deprecation_message text NOT NULL DEFAULT '',
    ADD COLUMN replacement_version text NOT NULL DEFAULT '',
    ADD COLUMN sunset_at timestamptz,
    ADD COLUMN revision bigint NOT NULL DEFAULT 1,
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

UPDATE connector_releases SET display_version = version::text WHERE display_version IS NULL;
ALTER TABLE connector_releases ALTER COLUMN display_version SET NOT NULL;
ALTER TABLE connector_releases ADD CONSTRAINT connector_releases_display_version_unique UNIQUE(product_id, display_version);
ALTER TABLE connector_releases ADD CONSTRAINT connector_releases_pin_target_unique UNIQUE(id, product_id, organisation_id);
ALTER TABLE connector_releases ADD CONSTRAINT connector_release_display_version_valid CHECK (
    display_version ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'
);
ALTER TABLE connector_releases ADD CONSTRAINT connector_release_lifecycle_valid CHECK (
    (deprecated_at IS NULL OR (NOT is_latest AND NOT is_lts AND deprecation_message <> ''))
    AND replacement_version <> display_version
);
CREATE UNIQUE INDEX connector_releases_one_latest_idx ON connector_releases(product_id) WHERE is_latest AND deprecated_at IS NULL;
CREATE INDEX connector_releases_product_lifecycle_idx ON connector_releases(product_id, is_latest DESC, is_lts DESC, published_at DESC);

CREATE TABLE product_version_pins (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    customer_id text NOT NULL,
    connector_release_id uuid NOT NULL,
    reason text NOT NULL DEFAULT '',
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(product_id, customer_id),
    FOREIGN KEY(connector_release_id, product_id, organisation_id)
        REFERENCES connector_releases(id, product_id, organisation_id) ON DELETE RESTRICT
);
CREATE INDEX product_version_pins_product_idx ON product_version_pins(product_id, customer_id);
