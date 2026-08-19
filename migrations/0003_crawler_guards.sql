CREATE UNIQUE INDEX crawl_jobs_one_active_per_source_idx
    ON crawl_jobs(source_id)
    WHERE state IN ('queued', 'running');

CREATE INDEX knowledge_documents_public_lookup_idx
    ON knowledge_documents(product_id, visibility, state, updated_at DESC);
