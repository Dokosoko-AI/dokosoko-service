-- A shared runtime credential set is the reusable Authorization profile used
-- by API service connections. Keep non-secret provider configuration and the
-- operator's rotation destination with the profile, alongside its environment
-- binding, instead of copying those fields onto every API.

ALTER TABLE runtime_credential_sets
    ADD COLUMN auth_config jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(auth_config) = 'object'),
    ADD COLUMN key_management_url text NOT NULL DEFAULT '',
    ADD COLUMN access_evaluation_url text NOT NULL DEFAULT '',
    ADD COLUMN usage_url text NOT NULL DEFAULT '';

COMMENT ON COLUMN runtime_credential_sets.auth_config IS
    'Non-secret structural authentication settings owned by this reusable Authorization profile.';
COMMENT ON COLUMN runtime_credential_sets.key_management_url IS
    'Credential-free operator URL for managing or rotating this Authorization profile; never fetched by DokoSoko.';
COMMENT ON COLUMN runtime_credential_sets.access_evaluation_url IS
    'Synchronous fail-closed access decision endpoint owned by this Authorization.';
COMMENT ON COLUMN runtime_credential_sets.usage_url IS
    'Asynchronous usage event endpoint owned by this Authorization.';

CREATE TABLE authorization_usage_events (
    id uuid PRIMARY KEY,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    authorization_id uuid NOT NULL REFERENCES runtime_credential_sets(id) ON DELETE RESTRICT,
    url text NOT NULL,
    authentication_type text NOT NULL,
    header_name text NOT NULL DEFAULT '',
    auth_config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(auth_config) = 'object'),
    credential_version_id uuid NOT NULL REFERENCES runtime_credential_versions(id) ON DELETE RESTRICT,
    credential_secret_id uuid NOT NULL REFERENCES secrets(id) ON DELETE RESTRICT,
    credential_fingerprint text NOT NULL,
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'delivering', 'delivered')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text NOT NULL DEFAULT '',
    leased_until timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX authorization_usage_events_delivery_idx
    ON authorization_usage_events(state, available_at, created_at)
    WHERE state = 'queued';

CREATE INDEX authorization_usage_events_lease_idx
    ON authorization_usage_events(leased_until)
    WHERE state = 'delivering';

COMMENT ON TABLE authorization_usage_events IS
    'Value-free durable outbox for asynchronous Authorization usage delivery. Arguments, results, tokens, and plaintext credentials are never stored.';
