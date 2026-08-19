ALTER TABLE products
    ADD COLUMN catalog_revision bigint NOT NULL DEFAULT 1,
    ADD COLUMN require_promotion_approval boolean NOT NULL DEFAULT false;

ALTER TABLE connector_releases
    ADD COLUMN manifest_hash text NOT NULL DEFAULT '',
    ADD COLUMN release_diff jsonb NOT NULL DEFAULT '{"generated_at":"0001-01-01T00:00:00Z","summary":"Initial product release","added":[],"removed":[],"changed":[]}'::jsonb,
    ADD COLUMN release_stage text NOT NULL DEFAULT 'active'
        CHECK (release_stage IN ('preview','active')),
    ADD COLUMN rollout_percentage integer NOT NULL DEFAULT 100
        CHECK (rollout_percentage BETWEEN 0 AND 100),
    ADD COLUMN promotion_state text NOT NULL DEFAULT 'not_required'
        CHECK (promotion_state IN ('not_required','pending','approved','rejected')),
    ADD COLUMN promotion_note text NOT NULL DEFAULT '',
    ADD COLUMN requested_latest boolean NOT NULL DEFAULT false,
    ADD COLUMN requested_lts boolean NOT NULL DEFAULT false,
    ADD COLUMN publisher_actor_id text NOT NULL DEFAULT '',
    ADD COLUMN promotion_requested_by text NOT NULL DEFAULT '',
    ADD COLUMN approved_by text NOT NULL DEFAULT '',
    ADD COLUMN approved_at timestamptz,
    ADD COLUMN drift_status text NOT NULL DEFAULT 'healthy'
        CHECK (drift_status IN ('unchecked','healthy','drifted')),
    ADD COLUMN drift_details jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN drift_checked_at timestamptz;

-- Existing immutable release manifests also need an integrity identity. New
-- releases use the canonical application serializer; legacy releases receive
-- a deterministic hash of their stored JSON representation during migration.
UPDATE connector_releases
SET manifest_hash = 'sha256:' || encode(digest(convert_to(manifest::text, 'UTF8'), 'sha256'), 'hex')
WHERE manifest_hash = '';

CREATE INDEX connector_releases_operational_idx
    ON connector_releases(product_id, release_stage, drift_status, is_latest DESC, published_at DESC);

CREATE TABLE product_installations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    customer_id text NOT NULL,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE RESTRICT,
    external_id text NOT NULL,
    name text NOT NULL,
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active','paused')),
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(product_id, external_id),
    UNIQUE(id, product_id, organisation_id)
);
CREATE INDEX product_installations_resolution_idx
    ON product_installations(product_id, customer_id, external_id) WHERE state='active';

ALTER TABLE product_version_pins
    DROP CONSTRAINT product_version_pins_product_id_customer_id_key,
    ADD COLUMN scope text NOT NULL DEFAULT 'customer'
        CHECK (scope IN ('customer','environment','installation')),
    ADD COLUMN scope_id text NOT NULL DEFAULT '',
    ADD COLUMN environment_id uuid REFERENCES environments(id) ON DELETE CASCADE,
    ADD COLUMN installation_id uuid REFERENCES product_installations(id) ON DELETE CASCADE;

UPDATE product_version_pins SET scope_id=customer_id WHERE scope_id='';

ALTER TABLE product_version_pins
    ADD CONSTRAINT product_version_pins_scope_unique UNIQUE(product_id, scope, scope_id),
    ADD CONSTRAINT product_version_pins_scope_shape CHECK (
        (scope='customer' AND customer_id<>'' AND environment_id IS NULL AND installation_id IS NULL)
        OR (scope='environment' AND environment_id IS NOT NULL AND installation_id IS NULL)
        OR (scope='installation' AND installation_id IS NOT NULL)
    );

CREATE TABLE product_version_pin_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    pin_id uuid NOT NULL,
    scope text NOT NULL CHECK (scope IN ('customer','environment','installation')),
    scope_id text NOT NULL,
    prior_version text NOT NULL DEFAULT '',
    product_version text NOT NULL DEFAULT '',
    action text NOT NULL CHECK (action IN ('created','updated','deleted')),
    reason text NOT NULL DEFAULT '',
    actor_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX product_version_pin_history_scope_idx
    ON product_version_pin_history(product_id, created_at DESC);

ALTER TABLE vendor_identity_providers
    ADD COLUMN installation_claim text NOT NULL DEFAULT '';
ALTER TABLE oauth_authorization_codes
    ADD COLUMN installation_id text NOT NULL DEFAULT '';
ALTER TABLE oauth_access_tokens
    ADD COLUMN installation_id text NOT NULL DEFAULT '';
