-- A replacement and its queued import commit together. Retain the exact import
-- identity so a lost response can be recovered without replacing another input.
CREATE TABLE source_input_replacements (
    product_id uuid NOT NULL,
    source_id uuid NOT NULL,
    request_digest text NOT NULL CHECK (request_digest ~ '^[a-f0-9]{64}$'),
    input_digest text NOT NULL CHECK (input_digest ~ '^[a-f0-9]{64}$'),
    expected_revision bigint NOT NULL CHECK (expected_revision > 0),
    crawl_job_id uuid NOT NULL UNIQUE REFERENCES crawl_jobs(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, source_id, request_digest),
    FOREIGN KEY (source_id, product_id) REFERENCES sources(id, product_id) ON DELETE RESTRICT
);
