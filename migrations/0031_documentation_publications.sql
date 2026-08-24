-- Documentation is reviewed and published as an exact crawl generation. A
-- snapshot can be reused by later crawls, so the generation/document link is
-- deliberately separate from source_snapshots.crawl_job_id.
CREATE UNIQUE INDEX knowledge_documents_snapshot_unique_idx
    ON knowledge_documents(snapshot_id);

CREATE TABLE crawl_job_documents (
    crawl_job_id uuid NOT NULL REFERENCES crawl_jobs(id) ON DELETE CASCADE,
    knowledge_document_id uuid NOT NULL REFERENCES knowledge_documents(id) ON DELETE RESTRICT,
    changed boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (crawl_job_id, knowledge_document_id)
);
CREATE INDEX crawl_job_documents_document_idx
    ON crawl_job_documents(knowledge_document_id, crawl_job_id);

-- Preserve the original generation link for snapshots created before this
-- migration. Existing published sources still require a fresh explicit review
-- before they can be pinned by a new Integration revision.
INSERT INTO crawl_job_documents(crawl_job_id, knowledge_document_id, changed)
SELECT ss.crawl_job_id, kd.id, true
FROM knowledge_documents kd
JOIN source_snapshots ss ON ss.id = kd.snapshot_id
ON CONFLICT DO NOTHING;

CREATE TABLE source_publications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    crawl_job_id uuid NOT NULL REFERENCES crawl_jobs(id) ON DELETE RESTRICT,
    revision bigint NOT NULL CHECK (revision > 0),
    visibility visibility NOT NULL,
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    document_count integer NOT NULL CHECK (document_count > 0),
    reviewed_by text NOT NULL,
    reviewed_at timestamptz NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_id, revision),
    UNIQUE (source_id, crawl_job_id),
    UNIQUE (id, product_id)
);
CREATE INDEX source_publications_product_source_idx
    ON source_publications(product_id, source_id, revision DESC);

CREATE TABLE source_publication_documents (
    source_publication_id uuid NOT NULL REFERENCES source_publications(id) ON DELETE CASCADE,
    knowledge_document_id uuid NOT NULL REFERENCES knowledge_documents(id) ON DELETE RESTRICT,
    PRIMARY KEY (source_publication_id, knowledge_document_id)
);
CREATE INDEX source_publication_documents_document_idx
    ON source_publication_documents(knowledge_document_id, source_publication_id);
