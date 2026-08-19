ALTER TABLE vendor_identity_providers
    ADD COLUMN product_id uuid REFERENCES products(id) ON DELETE CASCADE,
    ADD COLUMN audience text NOT NULL DEFAULT '',
    ADD COLUMN organisation_claim text NOT NULL DEFAULT 'org_id',
    ADD COLUMN entitlement_hook_url text NOT NULL DEFAULT '',
    ADD COLUMN allowed_redirect_uris text[] NOT NULL DEFAULT '{}';
ALTER TABLE vendor_identity_providers DROP CONSTRAINT IF EXISTS vendor_identity_providers_organisation_id_issuer_key;
CREATE UNIQUE INDEX vendor_identity_product_idx ON vendor_identity_providers(product_id) WHERE product_id IS NOT NULL;

CREATE TABLE oauth_states (
    state_digest bytea PRIMARY KEY,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    client_id text NOT NULL,
    redirect_uri text NOT NULL,
    downstream_state text NOT NULL,
    downstream_challenge text NOT NULL,
    upstream_verifier text NOT NULL,
    nonce text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_authorization_codes (
    code_digest bytea PRIMARY KEY,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    client_id text NOT NULL,
    redirect_uri text NOT NULL,
    downstream_challenge text NOT NULL,
    issuer text NOT NULL,
    subject text NOT NULL,
    email text NOT NULL DEFAULT '',
    display_name text NOT NULL DEFAULT '',
    vendor_organisation_id text NOT NULL DEFAULT '',
    entitlements jsonb NOT NULL DEFAULT '{}'::jsonb,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_access_tokens (
    token_digest bytea PRIMARY KEY,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    client_id text NOT NULL,
    issuer text NOT NULL,
    subject text NOT NULL,
    email text NOT NULL DEFAULT '',
    display_name text NOT NULL DEFAULT '',
    vendor_organisation_id text NOT NULL DEFAULT '',
    entitlements jsonb NOT NULL DEFAULT '{}'::jsonb,
    scopes text[] NOT NULL DEFAULT '{}',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
CREATE INDEX oauth_access_tokens_subject_idx ON oauth_access_tokens(product_id, issuer, subject, expires_at DESC);
