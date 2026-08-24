-- A crawl review must retain the assessment that the reviewer saw even when a
-- later generation reuses and reassesses the same immutable document.
ALTER TABLE crawl_job_documents
    ADD COLUMN assessment_state lifecycle_state,
    ADD COLUMN assessment_trust_level smallint,
    ADD COLUMN assessment_injection_indicators jsonb;

UPDATE crawl_job_documents AS cjd
SET assessment_state = kd.state,
    assessment_trust_level = kd.trust_level,
    assessment_injection_indicators = kd.injection_indicators
FROM knowledge_documents AS kd
WHERE kd.id = cjd.knowledge_document_id;

ALTER TABLE crawl_job_documents
    ALTER COLUMN assessment_state SET NOT NULL,
    ALTER COLUMN assessment_trust_level SET NOT NULL,
    ALTER COLUMN assessment_injection_indicators SET NOT NULL,
    ADD CONSTRAINT crawl_job_documents_assessment_trust_level_check
        CHECK (assessment_trust_level BETWEEN 0 AND 100);

-- Pages may be retried while their job is running. Once the job leaves that
-- state, its review evidence (including `changed`, which is hashed) is frozen.
CREATE FUNCTION guard_frozen_crawl_job_document_assessment()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    job_state text;
BEGIN
    IF TG_OP = 'DELETE' THEN
        SELECT state INTO job_state
        FROM crawl_jobs
        WHERE id = OLD.crawl_job_id;

        -- A missing parent means this is the job/source ON DELETE CASCADE.
        IF job_state IS NOT NULL AND job_state IS DISTINCT FROM 'running' THEN
            RAISE EXCEPTION 'crawl job document assessment is frozen once its job is not running'
                USING ERRCODE = '55000';
        END IF;
        RETURN OLD;
    END IF;

    IF TG_OP = 'UPDATE' THEN
        SELECT state INTO job_state
        FROM crawl_jobs
        WHERE id = OLD.crawl_job_id;
        IF job_state IS DISTINCT FROM 'running' THEN
            RAISE EXCEPTION 'crawl job document assessment is frozen once its job is not running'
                USING ERRCODE = '55000';
        END IF;
    END IF;

    SELECT state INTO job_state
    FROM crawl_jobs
    WHERE id = NEW.crawl_job_id;

    IF job_state IS DISTINCT FROM 'running' THEN
        RAISE EXCEPTION 'crawl job document assessment is frozen once its job is not running'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER crawl_job_documents_frozen_assessment_trigger
BEFORE INSERT OR UPDATE OR DELETE
ON crawl_job_documents
FOR EACH ROW
EXECUTE FUNCTION guard_frozen_crawl_job_document_assessment();
