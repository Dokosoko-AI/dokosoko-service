-- Promote the most common OpenAI-compatible AI platforms to first-class
-- providers. Their origins stay fixed so a saved provider name cannot be used
-- to redirect credentials to an arbitrary host.
ALTER TABLE ai_provider_connections
    DROP CONSTRAINT IF EXISTS ai_provider_connections_provider_check;

ALTER TABLE ai_provider_connections
    ADD CONSTRAINT ai_provider_connections_provider_check
    CHECK (provider IN (
        'openai',
        'google',
        'anthropic',
        'digitalocean',
        'xai',
        'deepseek',
        'openai-compatible'
    ));
