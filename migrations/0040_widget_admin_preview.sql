-- Administrator dogfooding uses the real widget exchange and chat runtime,
-- while remaining visibly separate from customer sessions and analytics.

ALTER TABLE widget_bootstrap_tokens
    ADD COLUMN kind text NOT NULL DEFAULT 'customer'
    CHECK (kind IN ('customer', 'admin_preview'));

ALTER TABLE widget_sessions
    ADD COLUMN kind text NOT NULL DEFAULT 'customer'
    CHECK (kind IN ('customer', 'admin_preview'));

CREATE INDEX widget_sessions_kind_idx ON widget_sessions(widget_id, kind, created_at DESC);
