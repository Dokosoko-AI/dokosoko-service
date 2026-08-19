CREATE TABLE reporting_configs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    bug_reports_enabled boolean NOT NULL DEFAULT false,
    feedback_enabled boolean NOT NULL DEFAULT false,
    bug_hook_url text NOT NULL DEFAULT '',
    bug_hook_credential_id uuid REFERENCES secrets(id) ON DELETE RESTRICT,
    feedback_hook_url text NOT NULL DEFAULT '',
    feedback_hook_credential_id uuid REFERENCES secrets(id) ON DELETE RESTRICT,
    retention_days integer NOT NULL DEFAULT 30 CHECK (retention_days BETWEEN 1 AND 365),
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id)
);

CREATE TABLE report_submissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('bug', 'feedback')),
    state text NOT NULL CHECK (state IN ('held', 'pending', 'delivering', 'delivered', 'failed')),
    actor_pseudonym text NOT NULL,
    idempotency_digest bytea NOT NULL,
    payload_ciphertext bytea NOT NULL,
    payload_nonce bytea NOT NULL,
    payload_key_version integer NOT NULL,
    payload_fingerprint text NOT NULL,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz,
    delivery_started_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    external_id text NOT NULL DEFAULT '',
    external_url text NOT NULL DEFAULT '',
    delivered_at timestamptz,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id, actor_pseudonym, kind, idempotency_digest)
);

CREATE INDEX report_submissions_product_created_idx ON report_submissions(product_id, created_at DESC);
CREATE INDEX report_submissions_delivery_idx ON report_submissions(state, next_attempt_at, delivery_started_at) WHERE state IN ('pending', 'delivering');
CREATE INDEX report_submissions_expiry_idx ON report_submissions(expires_at);
