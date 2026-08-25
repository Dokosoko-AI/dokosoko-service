-- A widget is an authenticated product surface, not only a documentation
-- viewer. Let the customer's trusted backend attach a small, display-ready
-- snapshot of the view the user is already on. The model receives these facts,
-- never the opaque customer identity values used to bind the session.

ALTER TABLE widget_bootstrap_tokens
    ADD COLUMN session_context jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT widget_bootstrap_tokens_session_context_object
        CHECK (jsonb_typeof(session_context) = 'object');

ALTER TABLE widget_sessions
    ADD COLUMN session_context jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT widget_sessions_session_context_object
        CHECK (jsonb_typeof(session_context) = 'object');
