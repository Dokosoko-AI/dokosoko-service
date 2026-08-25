-- Identity configuration is activated only after one exact revision completes
-- a real, short-lived OIDC authorization-code test. Test transactions retain
-- no upstream token or client credential.
ALTER TABLE identity_providers
  DROP CONSTRAINT IF EXISTS identity_provider_api_shape,
  DROP COLUMN IF EXISTS config,
  ADD CONSTRAINT identity_provider_api_shape CHECK (
    (
      delegated_api_origin ~ '^https://[^/?#:@]+$'
      OR delegated_api_origin ~ '^http://(([A-Za-z0-9-]+[.])*localhost|127[.]0[.]0[.]1|\[::1\])(:[0-9]{1,5})?$'
    )
    AND (
      oauth_resource = ''
      OR oauth_resource ~ '^https://'
      OR oauth_resource ~ '^http://(([A-Za-z0-9-]+[.])*localhost|127[.]0[.]0[.]1|\[::1\])(:[0-9]{1,5})?([/?].*)?$'
    )
  );

CREATE TABLE identity_provider_tests (
  id uuid PRIMARY KEY,
  organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  configuration_revision bigint NOT NULL CHECK (configuration_revision > 0),
  state_digest bytea NOT NULL UNIQUE CHECK (octet_length(state_digest) = 32),
  upstream_verifier text NOT NULL DEFAULT '',
  nonce text NOT NULL DEFAULT '',
  status text NOT NULL CHECK (status IN ('pending', 'passed', 'failed')),
  failure_code text NOT NULL DEFAULT '',
  issuer text NOT NULL DEFAULT '',
  subject text NOT NULL DEFAULT '',
  customer_id text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  completed_at timestamptz,
  CHECK (expires_at > created_at),
  CHECK (
    (status = 'pending' AND completed_at IS NULL AND upstream_verifier <> '' AND nonce <> '')
    OR
    (status IN ('passed', 'failed') AND completed_at IS NOT NULL AND upstream_verifier = '' AND nonce = '')
  )
);

CREATE INDEX identity_provider_tests_product_created_idx
  ON identity_provider_tests(product_id, created_at DESC, id DESC);

-- Revision zero marks every artifact created before revision binding existed.
-- Current providers start at revision one, so those legacy artifacts fail
-- closed as soon as the new binary evaluates them.
ALTER TABLE oauth_states
  ADD COLUMN provider_revision bigint NOT NULL DEFAULT 0 CHECK (provider_revision >= 0);
ALTER TABLE oauth_authorization_codes
  ADD COLUMN provider_revision bigint NOT NULL DEFAULT 0 CHECK (provider_revision >= 0);
ALTER TABLE oauth_access_tokens
  ADD COLUMN provider_revision bigint NOT NULL DEFAULT 0 CHECK (provider_revision >= 0);
