-- Retain one source identity for retries of the same bounded creation request.
-- Request keys and input are stored as digests; imported content stays in the
-- existing credential-free source/upload stores.
CREATE TABLE source_creation_requests (
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    request_digest text NOT NULL CHECK (request_digest ~ '^[a-f0-9]{64}$'),
    input_digest text NOT NULL CHECK (input_digest ~ '^[a-f0-9]{64}$'),
    source_id uuid NOT NULL REFERENCES sources(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, request_digest)
);
