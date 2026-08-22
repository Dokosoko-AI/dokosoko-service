-- Public OAuth clients created through RFC 7591 dynamic client registration.
-- These rows contain no credential. Exact redirect matching and PKCE are
-- enforced when the client starts an authorization transaction.

CREATE TABLE mcp_oauth_clients (
    client_id text PRIMARY KEY CHECK (client_id ~ '^mcp_client_[A-Za-z0-9_-]{32,64}$'),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    client_name text NOT NULL DEFAULT '' CHECK (length(client_name) <= 200),
    redirect_uris text[] NOT NULL CHECK (cardinality(redirect_uris) BETWEEN 1 AND 20),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX mcp_oauth_clients_deployment_created_idx
    ON mcp_oauth_clients(deployment_id, created_at DESC);
