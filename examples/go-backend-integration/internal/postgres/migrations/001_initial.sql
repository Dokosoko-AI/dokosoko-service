CREATE TABLE IF NOT EXISTS support_submissions (
    receipt_id text PRIMARY KEY,
    submission_id text NOT NULL UNIQUE,
    kind text NOT NULL CHECK (kind IN ('bug', 'feedback')),
    payload jsonb NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS support_submissions_received_at_idx
    ON support_submissions(received_at DESC, receipt_id DESC);

CREATE TABLE IF NOT EXISTS dokosoko_idempotency_results (
    idempotency_key text PRIMARY KEY
        CHECK (char_length(idempotency_key) BETWEEN 16 AND 200),
    request_sha256 bytea NOT NULL CHECK (octet_length(request_sha256) = 32),
    response_status smallint NOT NULL,
    response_body bytea NOT NULL,
    first_request_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    retain_until timestamptz NOT NULL
);

COMMENT ON COLUMN dokosoko_idempotency_results.retain_until IS
    'Earliest safe deletion time. Keeping records longer is valid and safer for delayed retries.';

-- Early versions of this example used jsonb here, which is semantically
-- equivalent but cannot replay the exact original response bytes.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'dokosoko_idempotency_results'
          AND column_name = 'response_body'
          AND data_type = 'jsonb'
    ) THEN
        ALTER TABLE dokosoko_idempotency_results
            ALTER COLUMN response_body TYPE bytea
            USING convert_to(response_body::text, 'UTF8');
    END IF;
END
$$;
