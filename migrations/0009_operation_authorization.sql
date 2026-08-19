ALTER TABLE vendor_identity_providers
    ADD COLUMN authorization_hook_url text NOT NULL DEFAULT '',
    ADD COLUMN authorization_credential_id uuid REFERENCES secrets(id) ON DELETE RESTRICT;
