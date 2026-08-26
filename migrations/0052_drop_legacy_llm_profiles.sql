-- Role-based LLM profiles were replaced by provider connections and the
-- Analysis workload. Credential secrets remain referenced by migrated provider
-- connections and keep their historical authenticated-encryption context.

DROP TABLE llm_profiles;
