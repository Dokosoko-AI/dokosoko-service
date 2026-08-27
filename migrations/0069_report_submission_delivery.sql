ALTER TABLE report_submissions
    DROP CONSTRAINT IF EXISTS report_submissions_state_check,
    ADD COLUMN delivery_url text NOT NULL DEFAULT '',
    ADD COLUMN attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    ADD COLUMN available_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN lease_owner text NOT NULL DEFAULT '',
    ADD COLUMN leased_until timestamptz,
    ADD COLUMN last_error text NOT NULL DEFAULT '',
    ADD COLUMN delivered_at timestamptz;

UPDATE report_submissions AS submission
SET delivery_url = CASE submission.kind
        WHEN 'bug' THEN deployment.error_submission_url
        WHEN 'feedback' THEN deployment.feedback_submission_url
        ELSE ''
    END
FROM deployments AS deployment
WHERE deployment.id = submission.product_id;

UPDATE report_submissions
SET state = 'failed', last_error = 'delivery destination is not configured'
WHERE state = 'queued' AND delivery_url = '';

ALTER TABLE report_submissions
    ADD CONSTRAINT report_submissions_state_check
    CHECK (state IN ('queued', 'delivering', 'delivered', 'failed'));

CREATE INDEX report_submissions_delivery_idx
    ON report_submissions (available_at, created_at, id)
    WHERE state IN ('queued', 'delivering');

COMMENT ON COLUMN report_submissions.delivery_url IS
    'Immutable root-configured destination snapshot for this plaintext submission.';
COMMENT ON COLUMN report_submissions.last_error IS
    'Bounded, credential-free delivery failure category.';
