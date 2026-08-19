ALTER TABLE vendor_identity_providers
    ADD COLUMN usage_hook_url text NOT NULL DEFAULT '',
    ADD COLUMN usage_credential_id uuid REFERENCES secrets(id) ON DELETE RESTRICT;
