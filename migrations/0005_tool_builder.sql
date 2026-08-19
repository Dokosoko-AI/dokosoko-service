ALTER TABLE tool_definitions
    ADD COLUMN api_connection_id uuid REFERENCES api_connections(id) ON DELETE RESTRICT,
    ADD COLUMN http_method text NOT NULL DEFAULT 'POST' CHECK (http_method IN ('GET', 'POST', 'PUT', 'PATCH', 'DELETE')),
    ADD COLUMN output_schema jsonb NOT NULL DEFAULT '{"type":"object"}'::jsonb,
    ADD COLUMN authorization_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN timeout_ms integer NOT NULL DEFAULT 10000 CHECK (timeout_ms BETWEEN 100 AND 60000);

