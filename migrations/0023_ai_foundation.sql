-- Provider credentials belong to a deployment connection. Product workloads
-- reference the connection instead of storing the same credential repeatedly.
CREATE TABLE ai_provider_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    provider text NOT NULL CHECK (provider IN ('openai','google','anthropic','openai-compatible')),
    endpoint text NOT NULL,
    credential_secret_id uuid REFERENCES secrets(id) ON DELETE RESTRICT,
    managed_by text NOT NULL DEFAULT 'console' CHECK (managed_by IN ('console','environment')),
    enabled boolean NOT NULL DEFAULT true,
    last_tested_at timestamptz,
    last_error_code text NOT NULL DEFAULT '',
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (deployment_id, provider),
    CONSTRAINT ai_provider_connection_endpoint CHECK (
        endpoint ~ '^https://[^/?#:@]+$'
    ),
    CONSTRAINT ai_provider_connection_credential CHECK (
        managed_by = 'environment' OR credential_secret_id IS NOT NULL
    )
);

CREATE TABLE ai_workload_profiles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    workload text NOT NULL CHECK (workload IN ('extraction','authoring','review','support')),
    provider_connection_id uuid NOT NULL REFERENCES ai_provider_connections(id) ON DELETE RESTRICT,
    model text NOT NULL,
    max_input_tokens integer NOT NULL CHECK (max_input_tokens BETWEEN 256 AND 1000000),
    max_output_tokens integer NOT NULL CHECK (max_output_tokens BETWEEN 1 AND 32768),
    daily_token_budget bigint NOT NULL CHECK (daily_token_budget BETWEEN 0 AND 10000000000),
    hardening jsonb NOT NULL DEFAULT '{"context_is_untrusted":true,"tool_calls_disabled":true,"authorization_disabled":true,"require_citations":true,"no_answer_on_low_confidence":true}'::jsonb,
    enabled boolean NOT NULL DEFAULT false,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id, workload)
);

CREATE TABLE ai_budget_days (
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    workload text NOT NULL CHECK (workload IN ('extraction','authoring','review','support')),
    day date NOT NULL,
    used_tokens bigint NOT NULL DEFAULT 0 CHECK (used_tokens >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, workload, day)
);

CREATE TABLE ai_budget_reservations (
    id uuid PRIMARY KEY,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    workload text NOT NULL CHECK (workload IN ('extraction','authoring','review','support')),
    day date NOT NULL,
    reserved_tokens bigint NOT NULL CHECK (reserved_tokens > 0),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_budget_reservations_active_idx
    ON ai_budget_reservations(product_id, workload, day, expires_at);

CREATE TABLE ai_usage_events (
    id uuid PRIMARY KEY,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    workload text NOT NULL CHECK (workload IN ('extraction','authoring','review','support')),
    action text NOT NULL,
    provider text NOT NULL,
    requested_model text NOT NULL,
    resolved_model text NOT NULL DEFAULT '',
    provider_request_id text NOT NULL DEFAULT '',
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    duration_ms bigint NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    outcome text NOT NULL CHECK (outcome IN ('succeeded','failed')),
    error_code text NOT NULL DEFAULT '',
    prompt_version text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_usage_events_product_created_idx
    ON ai_usage_events(product_id, created_at DESC);
CREATE INDEX ai_usage_events_workload_created_idx
    ON ai_usage_events(product_id, workload, created_at DESC);

-- Preserve existing configuration while moving to the clearer workload
-- vocabulary. Embedding and reranking profiles were not consumed by the
-- runtime and intentionally remain only in the compatibility table.
INSERT INTO ai_provider_connections(
    organisation_id, deployment_id, provider, endpoint,
    credential_secret_id, managed_by, enabled
)
SELECT DISTINCT ON (product_id, provider)
    organisation_id, product_id, provider, endpoint,
    credential_secret_id, 'console', true
FROM llm_profiles
WHERE provider IN ('openai','openai-compatible')
  AND credential_secret_id IS NOT NULL
ORDER BY product_id, provider, updated_at DESC
ON CONFLICT (deployment_id, provider) DO NOTHING;

INSERT INTO ai_workload_profiles(
    organisation_id, product_id, workload, provider_connection_id, model,
    max_input_tokens, max_output_tokens, daily_token_budget, hardening, enabled
)
SELECT
    profile.organisation_id,
    profile.product_id,
    CASE profile.role
        WHEN 'assistant' THEN 'support'
        WHEN 'evaluation' THEN 'review'
        ELSE profile.role
    END,
    connection.id,
    profile.model,
    profile.max_input_tokens,
    profile.max_output_tokens,
    profile.daily_token_budget,
    profile.hardening,
    profile.enabled
FROM llm_profiles profile
JOIN ai_provider_connections connection
  ON connection.deployment_id = profile.product_id
 AND connection.provider = profile.provider
WHERE profile.role IN ('extraction','evaluation','assistant')
ON CONFLICT (product_id, workload) DO NOTHING;
