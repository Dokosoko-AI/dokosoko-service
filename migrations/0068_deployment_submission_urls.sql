-- Feedback and error submission are deployment-wide destinations. Migrate the
-- values introduced in 0067 before removing their per-Authorization storage.

ALTER TABLE deployments
    ADD COLUMN feedback_submission_url text NOT NULL DEFAULT '',
    ADD COLUMN error_submission_url text NOT NULL DEFAULT '';

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM runtime_credential_sets
        GROUP BY deployment_id
        HAVING count(DISTINCT NULLIF(feedback_submission_url, '')) > 1
            OR count(DISTINCT NULLIF(error_submission_url, '')) > 1
    ) THEN
        RAISE EXCEPTION 'cannot migrate conflicting Authorization submission URLs to deployment settings';
    END IF;
END
$$;

UPDATE deployments AS deployment
SET feedback_submission_url = COALESCE((
        SELECT max(NULLIF(credential.feedback_submission_url, ''))
        FROM runtime_credential_sets AS credential
        WHERE credential.deployment_id = deployment.id
    ), ''),
    error_submission_url = COALESCE((
        SELECT max(NULLIF(credential.error_submission_url, ''))
        FROM runtime_credential_sets AS credential
        WHERE credential.deployment_id = deployment.id
    ), '');

ALTER TABLE runtime_credential_sets
    DROP COLUMN feedback_submission_url,
    DROP COLUMN error_submission_url;

COMMENT ON COLUMN deployments.feedback_submission_url IS
    'Deployment-wide destination for feedback submissions. Empty disables delivery.';
COMMENT ON COLUMN deployments.error_submission_url IS
    'Deployment-wide destination for error submissions. Empty disables delivery.';
