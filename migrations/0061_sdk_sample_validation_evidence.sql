-- New ingestion distinguishes unsupported or inconclusive static checks from
-- legacy unvalidated rows. Historical values remain readable; new candidates
-- default to the explicit fail-closed state.
ALTER TABLE sdk_code_samples
    DROP CONSTRAINT sdk_code_samples_validation_status_check,
    ALTER COLUMN validation_status SET DEFAULT 'not_checked',
    ADD CONSTRAINT sdk_code_samples_validation_status_check CHECK (validation_status IN (
        'not_checked','unvalidated','syntax_checked','compiled','contract_tested','executed'
    ));

-- An unsupported sample may only be approved when a reviewer records the
-- evidence they used. Machine-validated samples may leave this empty because
-- their immutable validation_evidence is stored on sdk_code_samples.
ALTER TABLE sdk_content_publication_sample_selections
    ADD COLUMN review_evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT sdk_content_publication_sample_review_evidence_object_check
        CHECK (jsonb_typeof(review_evidence) = 'object'),
    ADD CONSTRAINT sdk_content_publication_sample_review_evidence_length_check
        CHECK (octet_length(review_evidence::text) <= 8000);

CREATE FUNCTION guard_sdk_sample_approval_evidence()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    sample_status text;
    sample_evidence jsonb;
    machine_passed boolean;
    review_recorded boolean;
BEGIN
    IF NEW.decision <> 'approved' THEN
        RETURN NEW;
    END IF;

    SELECT validation_status, validation_evidence
      INTO sample_status, sample_evidence
      FROM sdk_code_samples
     WHERE id = NEW.sdk_code_sample_id
       AND sdk_content_candidate_id = NEW.sdk_content_candidate_id
       AND deployment_id = NEW.deployment_id;

    machine_passed := FOUND
        AND sample_status NOT IN ('not_checked', 'unvalidated')
        AND (sample_evidence->>'validated' = 'true' OR sample_evidence->>'passed' = 'true')
        AND (btrim(coalesce(sample_evidence->>'validator', '')) <> ''
             OR btrim(coalesce(sample_evidence->>'evidence_id', '')) <> '');
    review_recorded := jsonb_typeof(NEW.review_evidence) = 'object'
        AND btrim(coalesce(NEW.review_evidence->>'summary', '')) <> '';

    IF NOT machine_passed AND NOT review_recorded THEN
        RAISE EXCEPTION 'SDK sample approval requires positive machine validation evidence or explicit review evidence'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER sdk_sample_approval_evidence_guard
BEFORE INSERT OR UPDATE ON sdk_content_publication_sample_selections
FOR EACH ROW EXECUTE FUNCTION guard_sdk_sample_approval_evidence();
