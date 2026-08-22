CREATE TABLE integration_analyses (
  id uuid PRIMARY KEY,
  organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  schema_version integer NOT NULL DEFAULT 1 CHECK (schema_version > 0),
  state text NOT NULL CHECK (state IN ('running','review','failed')),
  generated_by text NOT NULL CHECK (generated_by IN ('deterministic','ai_assisted')),
  evidence jsonb NOT NULL DEFAULT '[]'::jsonb,
  plan jsonb NOT NULL DEFAULT '{}'::jsonb,
  unknowns jsonb NOT NULL DEFAULT '[]'::jsonb,
  error_code text NOT NULL DEFAULT '',
  revision bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);

CREATE INDEX integration_analyses_product_created_idx ON integration_analyses(product_id, created_at DESC);

CREATE TABLE recipes (
  id uuid PRIMARY KEY,
  organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  analysis_id uuid REFERENCES integration_analyses(id) ON DELETE SET NULL,
  slug text NOT NULL,
  title text NOT NULL,
  outcome text NOT NULL,
  audience text NOT NULL,
  state text NOT NULL CHECK (state IN ('draft','review','approved','published','outdated')),
  generated boolean NOT NULL DEFAULT true,
  needs_attention boolean NOT NULL DEFAULT true,
  visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('public','private')),
  dependencies jsonb NOT NULL DEFAULT '[]'::jsonb,
  current_revision_id uuid,
  stable_uri text NOT NULL,
  approved_by text NOT NULL DEFAULT '',
  approved_at timestamptz,
  published_at timestamptz,
  revision bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(product_id, slug),
  UNIQUE(product_id, stable_uri)
);

CREATE INDEX recipes_product_state_idx ON recipes(product_id, state, updated_at DESC);
CREATE INDEX recipes_attention_idx ON recipes(product_id, needs_attention) WHERE needs_attention;

CREATE TABLE recipe_revisions (
  id uuid PRIMARY KEY,
  recipe_id uuid NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  revision integer NOT NULL CHECK (revision > 0),
  markdown text NOT NULL,
  reference_items jsonb NOT NULL DEFAULT '[]'::jsonb,
  validation jsonb NOT NULL DEFAULT '[]'::jsonb,
  review text NOT NULL DEFAULT '',
  generated_by text NOT NULL CHECK (generated_by IN ('ai','human','deterministic')),
  model text NOT NULL DEFAULT '',
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(recipe_id, revision)
);

ALTER TABLE recipes
  ADD CONSTRAINT recipes_current_revision_fk
  FOREIGN KEY (current_revision_id) REFERENCES recipe_revisions(id) ON DELETE SET NULL
  DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE ai_jobs (
  id uuid PRIMARY KEY,
  organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
  product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('integration_analysis','recipe_generation','recipe_rework','recipe_review')),
  target_id uuid,
  state text NOT NULL CHECK (state IN ('queued','running','succeeded','failed')),
  attempt integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
  input jsonb NOT NULL DEFAULT '{}'::jsonb,
  output jsonb NOT NULL DEFAULT '{}'::jsonb,
  error_code text NOT NULL DEFAULT '',
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  finished_at timestamptz
);

CREATE INDEX ai_jobs_product_created_idx ON ai_jobs(product_id, created_at DESC);
CREATE INDEX ai_jobs_pending_idx ON ai_jobs(state, created_at) WHERE state IN ('queued','running');
