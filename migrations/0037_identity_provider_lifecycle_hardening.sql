-- Existing providers have not passed the lifecycle test introduced by 0036.
-- Force each active provider through draft -> test -> activate and advance its
-- revision so every previously issued OAuth artifact is stale.
ALTER TABLE identity_providers
  DROP CONSTRAINT identity_provider_api_shape,
  ADD CONSTRAINT identity_provider_api_shape CHECK (
    (
      delegated_api_origin ~ '^https://[^/?#:@]+$'
      OR delegated_api_origin ~ '^http://(([A-Za-z0-9-]+[.])*localhost|127[.]0[.]0[.]1|\[::1\])(:[0-9]{1,5})?$'
    )
    AND (
      oauth_resource = ''
      OR oauth_resource ~ '^[A-Za-z][A-Za-z0-9+.-]*:[^#[:space:]]+$'
    )
  );

UPDATE identity_providers
SET state = 'disabled', revision = revision + 1, updated_at = now()
WHERE state = 'active';

-- The forced revision change makes every pre-lifecycle OAuth transaction and
-- token permanently unusable. Remove those artifacts in this transaction and
-- then remove every broker-owned delegated bearer secret, including any
-- pre-existing crash-window orphan. Snapshotting first is required because
-- both artifact tables protect their secrets with ON DELETE RESTRICT.
CREATE TEMP TABLE stale_identity_delegated_secrets (
  id uuid PRIMARY KEY
) ON COMMIT DROP;
INSERT INTO stale_identity_delegated_secrets(id)
SELECT upstream_access_secret_id FROM oauth_authorization_codes
UNION
SELECT upstream_access_secret_id FROM oauth_access_tokens;
INSERT INTO stale_identity_delegated_secrets(id)
SELECT id FROM secrets WHERE purpose = 'vendor_delegated_access'
ON CONFLICT (id) DO NOTHING;

DELETE FROM oauth_states;
DELETE FROM oauth_authorization_codes;
DELETE FROM oauth_access_tokens;
DELETE FROM secrets secret
USING stale_identity_delegated_secrets stale
WHERE secret.id = stale.id
  AND secret.purpose = 'vendor_delegated_access'
  AND NOT EXISTS (
    SELECT 1 FROM oauth_authorization_codes code
    WHERE code.upstream_access_secret_id = secret.id
  )
  AND NOT EXISTS (
    SELECT 1 FROM oauth_access_tokens token
    WHERE token.upstream_access_secret_id = secret.id
  );

ALTER TABLE identity_provider_tests
  DROP CONSTRAINT identity_provider_tests_status_check,
  DROP CONSTRAINT identity_provider_tests_check1,
  ADD COLUMN callback_claimed_at timestamptz,
  ADD CONSTRAINT identity_provider_tests_status_check
    CHECK (status IN ('pending', 'passed', 'failed', 'expired')),
  ADD CONSTRAINT identity_provider_tests_check1 CHECK (
    (status = 'pending' AND completed_at IS NULL AND upstream_verifier <> '' AND nonce <> '')
    OR
    (status IN ('passed', 'failed', 'expired') AND completed_at IS NOT NULL AND upstream_verifier = '' AND nonce = '')
  );

ALTER TABLE oauth_authorization_codes
  ADD COLUMN provider_organisation_id uuid REFERENCES organisations(id) ON DELETE CASCADE;
UPDATE oauth_authorization_codes code
SET provider_organisation_id = product.organisation_id
FROM products product
WHERE product.id = code.product_id;
ALTER TABLE oauth_authorization_codes
  ALTER COLUMN provider_organisation_id SET NOT NULL;
