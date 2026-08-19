ALTER TABLE root_users
    ADD COLUMN password_hash text NOT NULL,
    ADD COLUMN totp_secret_ciphertext bytea NOT NULL,
    ADD COLUMN recovery_code_digests bytea[] NOT NULL DEFAULT '{}';

CREATE TABLE root_sessions (
    token_digest bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES root_users(user_id) ON DELETE CASCADE,
    csrf_digest bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX root_sessions_user_expiry_idx ON root_sessions(user_id, expires_at DESC);

