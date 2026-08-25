-- Tool test consent and evidence are intentionally separate from published
-- execution history. Nonces are stored only as digests, consumption is an
-- append-only row with a unique confirmation, and evidence contains only
-- bounded value-free JSON shapes and fixed diagnostic findings.
CREATE TABLE tool_test_confirmations (
    id uuid PRIMARY KEY,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    tool_id uuid NOT NULL REFERENCES tool_definitions(id) ON DELETE CASCADE,
    tool_revision bigint NOT NULL CHECK (tool_revision > 0),
    argument_hash bytea NOT NULL CHECK (octet_length(argument_hash) = 32),
    nonce_digest bytea NOT NULL UNIQUE CHECK (octet_length(nonce_digest) = 32),
    actor_id text NOT NULL CHECK (actor_id <> ''),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);

CREATE INDEX tool_test_confirmations_expiry_idx
    ON tool_test_confirmations(expires_at);

CREATE TABLE tool_test_confirmation_consumptions (
    id uuid PRIMARY KEY,
    confirmation_id uuid NOT NULL UNIQUE REFERENCES tool_test_confirmations(id) ON DELETE CASCADE,
    consumed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tool_test_runs (
    id uuid PRIMARY KEY,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    tool_id uuid NOT NULL REFERENCES tool_definitions(id) ON DELETE CASCADE,
    tool_revision bigint NOT NULL CHECK (tool_revision > 0),
    tool_name text NOT NULL CHECK (tool_name <> ''),
    actor_id text NOT NULL CHECK (actor_id <> ''),
    request_id text NOT NULL DEFAULT '',
    argument_hash bytea NOT NULL CHECK (octet_length(argument_hash) = 32),
    method text NOT NULL CHECK (method IN ('GET', 'POST', 'PUT', 'PATCH', 'DELETE')),
    authentication_type text NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('success', 'failure')),
    phase text NOT NULL,
    network_call_performed boolean NOT NULL DEFAULT false,
    upstream_status_code integer CHECK (upstream_status_code IS NULL OR upstream_status_code BETWEEN 100 AND 599),
    response_bytes bigint CHECK (response_bytes IS NULL OR response_bytes >= 0),
    request_shape jsonb NOT NULL CHECK (jsonb_typeof(request_shape) = 'object'),
    response_shape jsonb CHECK (response_shape IS NULL OR jsonb_typeof(response_shape) = 'object'),
    findings jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(findings) = 'array'),
    duration_ms bigint NOT NULL CHECK (duration_ms >= 0),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);

CREATE INDEX tool_test_runs_product_tool_created_idx
    ON tool_test_runs(product_id, tool_id, created_at DESC);
CREATE INDEX tool_test_runs_expiry_idx
    ON tool_test_runs(expires_at);
