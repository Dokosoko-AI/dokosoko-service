-- Recover exact contract/set creation after a lost response without repeating
-- creation, its first immutable revision, or its audit event.
CREATE TABLE developer_asset_creation_requests (
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    resource_kind text NOT NULL CHECK (resource_kind IN ('api_contract', 'documentation_collection')),
    request_digest text NOT NULL CHECK (request_digest ~ '^[a-f0-9]{64}$'),
    input_digest text NOT NULL CHECK (input_digest ~ '^[a-f0-9]{64}$'),
    api_contract_id uuid,
    documentation_collection_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (deployment_id, resource_kind, request_digest),
    FOREIGN KEY (api_contract_id, deployment_id) REFERENCES api_contracts(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (documentation_collection_id, deployment_id) REFERENCES documentation_collections(id, deployment_id) ON DELETE RESTRICT,
    CHECK ((resource_kind = 'api_contract' AND api_contract_id IS NOT NULL AND documentation_collection_id IS NULL)
        OR (resource_kind = 'documentation_collection' AND documentation_collection_id IS NOT NULL AND api_contract_id IS NULL))
);
