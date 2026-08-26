-- Persist only product-specific edits to the four server-owned AI workflow
-- prompts. NULL instructions select the server default while the row retains
-- its revision, preventing a reset from making stale revision-1 writes valid
-- again. The immutable safety policy and default prompt bodies stay in code.

CREATE TABLE ai_prompt_settings (
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    prompt_key text NOT NULL CHECK (prompt_key IN (
        'integration.analysis',
        'recipe.brief',
        'recipe.authoring',
        'recipe.review'
    )),
    instructions text CHECK (
        instructions IS NULL OR (
            octet_length(instructions) BETWEEN 1 AND 32768
            AND btrim(instructions) <> ''
        )
    ),
    revision bigint NOT NULL DEFAULT 2 CHECK (revision >= 2),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, prompt_key)
);
