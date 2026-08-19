ALTER TABLE credential_leases
    ADD COLUMN external_id text NOT NULL DEFAULT '',
    ADD COLUMN idempotency_key text NOT NULL DEFAULT '';
CREATE INDEX credential_leases_product_created_idx ON credential_leases(product_id, created_at DESC);
CREATE UNIQUE INDEX credential_leases_provider_idempotency_idx ON credential_leases(provider_id, idempotency_key) WHERE idempotency_key <> '';
