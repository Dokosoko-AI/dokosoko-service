-- Optional provider-owned destinations for feedback and error submissions.
-- These are Authorization metadata only. No delivery worker consumes them yet.

ALTER TABLE runtime_credential_sets
    ADD COLUMN feedback_submission_url text NOT NULL DEFAULT '',
    ADD COLUMN error_submission_url text NOT NULL DEFAULT '';

COMMENT ON COLUMN runtime_credential_sets.feedback_submission_url IS
    'Optional provider endpoint for future feedback submission delivery; currently stored as Authorization metadata only.';
COMMENT ON COLUMN runtime_credential_sets.error_submission_url IS
    'Optional provider endpoint for future error submission delivery; currently stored as Authorization metadata only.';
