-- Generated MCP operations (for example API-scoped credential rotation) do
-- not have a tool_definitions row. Keep their one-time confirmations in a
-- dedicated table so mutation confirmation remains server-issued, exact,
-- replay-safe, and independent of client attestation.
CREATE TABLE managed_operation_confirmations (
    id uuid PRIMARY KEY,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    operation_key text NOT NULL CHECK (operation_key <> ''),
    argument_hash bytea NOT NULL CHECK (octet_length(argument_hash) = 32),
    nonce_digest bytea NOT NULL UNIQUE CHECK (octet_length(nonce_digest) = 32),
    actor_id text NOT NULL CHECK (actor_id <> ''),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);

CREATE INDEX managed_operation_confirmations_expiry_idx
    ON managed_operation_confirmations(expires_at);

CREATE TABLE managed_operation_confirmation_consumptions (
    id uuid PRIMARY KEY,
    confirmation_id uuid NOT NULL UNIQUE REFERENCES managed_operation_confirmations(id) ON DELETE CASCADE,
    consumed_at timestamptz NOT NULL DEFAULT now()
);
