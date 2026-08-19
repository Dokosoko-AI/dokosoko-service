CREATE TABLE product_builds (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    state text NOT NULL CHECK (state IN ('running', 'review', 'published', 'failed')),
    analysis_mode text NOT NULL CHECK (analysis_mode IN ('automatic','ai_assisted')),
    inputs jsonb NOT NULL DEFAULT '[]'::jsonb,
    proposed_definition jsonb NOT NULL DEFAULT '{}'::jsonb,
    unresolved jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE INDEX product_builds_product_created_idx ON product_builds(product_id, created_at DESC);

CREATE TABLE product_definitions (
    product_id uuid PRIMARY KEY REFERENCES products(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    id uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    state text NOT NULL CHECK (state IN ('draft', 'published')),
    generated_by text NOT NULL,
    source_build_id uuid REFERENCES product_builds(id) ON DELETE SET NULL,
    definition jsonb NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
