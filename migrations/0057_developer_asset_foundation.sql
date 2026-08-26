-- Deployment-owned developer assets. Package binaries remain external: this
-- schema stores reviewed metadata, normalized documentation, code samples,
-- immutable publication evidence, and rebuildable search projections only.

-- The new enrichment workflows use the same server-owned safety policy as the
-- existing analysis prompts. Only their bounded editorial instructions are
-- configurable.
ALTER TABLE ai_prompt_settings
    DROP CONSTRAINT ai_prompt_settings_prompt_key_check,
    ADD CONSTRAINT ai_prompt_settings_prompt_key_check CHECK (prompt_key IN (
        'integration.analysis',
        'recipe.brief',
        'recipe.authoring',
        'recipe.review',
        'documentation.map_enrichment',
        'sdk.map_enrichment',
        'sdk.applicability_suggestion',
        'sdk.sample_review'
    ));

-- Preserve the current crawl-job state machine and its one-active-job index,
-- while making partial coverage and dead-worker recovery first-class.
ALTER TABLE crawl_jobs
    ADD COLUMN failed_count integer NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
    ADD COLUMN skipped_count integer NOT NULL DEFAULT 0 CHECK (skipped_count >= 0),
    ADD COLUMN redirected_count integer NOT NULL DEFAULT 0 CHECK (redirected_count >= 0),
    ADD COLUMN lease_owner text NOT NULL DEFAULT '',
    ADD COLUMN lease_expires_at timestamptz,
    ADD COLUMN heartbeat_at timestamptz,
    ADD COLUMN pipeline_version text NOT NULL DEFAULT 'legacy-v1'
        CHECK (btrim(pipeline_version) <> ''),
    ADD COLUMN raw_manifest_hash text NOT NULL DEFAULT '' CHECK (
        raw_manifest_hash = '' OR raw_manifest_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    ADD COLUMN diagnostics jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(diagnostics) = 'object'),
    ADD CONSTRAINT crawl_jobs_lease_shape_check CHECK (
        (lease_owner = '' AND lease_expires_at IS NULL)
        OR (lease_owner <> '' AND lease_expires_at IS NOT NULL)
    );
CREATE INDEX crawl_jobs_expired_lease_idx
    ON crawl_jobs(lease_expires_at)
    WHERE state = 'running' AND lease_expires_at IS NOT NULL;

-- Existing reviewed publications get explicit legacy processor identities.
-- Historical hashes and selected documents remain unchanged.
ALTER TABLE source_publications
    ADD COLUMN pipeline_version text NOT NULL DEFAULT 'legacy-v1'
        CHECK (btrim(pipeline_version) <> ''),
    ADD COLUMN parser_version text NOT NULL DEFAULT 'legacy-v1'
        CHECK (btrim(parser_version) <> ''),
    ADD COLUMN map_version text NOT NULL DEFAULT 'legacy-v1'
        CHECK (btrim(map_version) <> ''),
    ADD COLUMN index_version text NOT NULL DEFAULT 'legacy-v1'
        CHECK (btrim(index_version) <> ''),
    ADD COLUMN raw_manifest_hash text NOT NULL DEFAULT '' CHECK (
        raw_manifest_hash = '' OR raw_manifest_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    ADD COLUMN diagnostics jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(diagnostics) = 'object');

-- Composite keys let every new foreign key prove deployment ownership rather
-- than relying on application-side checks.
ALTER TABLE deployments
    ADD CONSTRAINT deployments_id_organisation_developer_assets_key
        UNIQUE (id, organisation_id);
ALTER TABLE sources
    ADD CONSTRAINT sources_id_product_developer_assets_key
        UNIQUE (id, product_id);
ALTER TABLE integrations
    ADD CONSTRAINT integrations_id_deployment_organisation_assets_key
        UNIQUE (id, deployment_id, organisation_id);
ALTER TABLE integration_revisions
    ADD CONSTRAINT integration_revisions_id_integration_assets_key
        UNIQUE (id, integration_id);

CREATE FUNCTION canonical_sdk_coordinate(value_ecosystem text, value_coordinate text)
RETURNS text
LANGUAGE sql
IMMUTABLE
STRICT
AS $$
    SELECT CASE lower(btrim(value_ecosystem))
        WHEN 'pypi' THEN regexp_replace(lower(btrim(value_coordinate)), '[-_.]+', '-', 'g')
        WHEN 'npm' THEN lower(btrim(value_coordinate))
        WHEN 'cargo' THEN lower(btrim(value_coordinate))
        WHEN 'nuget' THEN lower(btrim(value_coordinate))
        ELSE btrim(value_coordinate)
    END
$$;

-- Raw objects are content-addressed evidence. The service stores only their
-- bounded object location and metadata, never package registry credentials.
CREATE TABLE developer_asset_raw_blobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    sha256 text NOT NULL CHECK (sha256 ~ '^sha256:[0-9a-f]{64}$'),
    object_key text NOT NULL CHECK (btrim(object_key) <> ''),
    media_type text NOT NULL DEFAULT 'application/octet-stream',
    byte_size bigint NOT NULL CHECK (byte_size >= 0),
    source_uri text NOT NULL DEFAULT '',
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (deployment_id, sha256),
    UNIQUE (id, deployment_id)
);

CREATE TABLE developer_asset_ingestion_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    organisation_id uuid NOT NULL,
    asset_kind text NOT NULL CHECK (asset_kind IN ('documentation','contract','sdk')),
    target_id uuid,
    target_key text NOT NULL CHECK (btrim(target_key) <> ''),
    source_id uuid,
    resolved_source_uri text NOT NULL DEFAULT '',
    resolved_source_revision text NOT NULL DEFAULT '',
    resolved_source_hash text NOT NULL DEFAULT '' CHECK (
        resolved_source_hash = '' OR resolved_source_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    state text NOT NULL DEFAULT 'queued' CHECK (state IN (
        'queued','running','review_ready','failed','cancelled','published'
    )),
    attempt integer NOT NULL DEFAULT 1 CHECK (attempt > 0),
    pipeline_version text NOT NULL CHECK (btrim(pipeline_version) <> ''),
    parser_version text NOT NULL CHECK (btrim(parser_version) <> ''),
    normalizer_version text NOT NULL CHECK (btrim(normalizer_version) <> ''),
    mapper_version text NOT NULL CHECK (btrim(mapper_version) <> ''),
    raw_manifest jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(raw_manifest) = 'array'),
    raw_manifest_hash text NOT NULL DEFAULT '' CHECK (
        raw_manifest_hash = '' OR raw_manifest_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    diagnostics jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(diagnostics) = 'object'),
    discovered_count integer NOT NULL DEFAULT 0 CHECK (discovered_count >= 0),
    acquired_count integer NOT NULL DEFAULT 0 CHECK (acquired_count >= 0),
    failed_count integer NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
    skipped_count integer NOT NULL DEFAULT 0 CHECK (skipped_count >= 0),
    quarantined_count integer NOT NULL DEFAULT 0 CHECK (quarantined_count >= 0),
    lease_owner text NOT NULL DEFAULT '',
    lease_expires_at timestamptz,
    heartbeat_at timestamptz,
    error_code text NOT NULL DEFAULT '',
    error_message text NOT NULL DEFAULT '',
    queued_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (deployment_id, organisation_id)
        REFERENCES deployments(id, organisation_id) ON DELETE RESTRICT,
    FOREIGN KEY (source_id, deployment_id)
        REFERENCES sources(id, product_id) ON DELETE RESTRICT,
    CHECK (
        (asset_kind = 'documentation' AND source_id IS NOT NULL
            AND target_id IS NOT NULL AND target_id = source_id)
        OR (asset_kind = 'contract' AND source_id IS NOT NULL AND target_id IS NOT NULL)
        OR (asset_kind = 'sdk' AND target_id IS NOT NULL)
    ),
    CHECK (
        (lease_owner = '' AND lease_expires_at IS NULL)
        OR (lease_owner <> '' AND lease_expires_at IS NOT NULL)
    ),
    CHECK (finished_at IS NULL OR started_at IS NOT NULL),
    UNIQUE (id, deployment_id)
);
CREATE UNIQUE INDEX developer_asset_ingestion_one_active_target_idx
    ON developer_asset_ingestion_runs(deployment_id, asset_kind, target_key)
    WHERE state IN ('queued','running');
CREATE INDEX developer_asset_ingestion_lease_idx
    ON developer_asset_ingestion_runs(lease_expires_at)
    WHERE state = 'running' AND lease_expires_at IS NOT NULL;

-- Candidate ownership depends on stable run identity and deterministic
-- processor versions. Operational state, counters, diagnostics, and leases
-- remain mutable while a worker advances the run.
CREATE FUNCTION guard_developer_asset_ingestion_run_update()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF (NEW.deployment_id, NEW.organisation_id, NEW.asset_kind, NEW.target_id,
        NEW.target_key, NEW.source_id, NEW.attempt, NEW.pipeline_version,
        NEW.parser_version, NEW.normalizer_version, NEW.mapper_version)
       IS DISTINCT FROM
       (OLD.deployment_id, OLD.organisation_id, OLD.asset_kind, OLD.target_id,
        OLD.target_key, OLD.source_id, OLD.attempt, OLD.pipeline_version,
        OLD.parser_version, OLD.normalizer_version, OLD.mapper_version) THEN
        RAISE EXCEPTION 'developer-asset ingestion identity and processor versions are immutable'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.state IS DISTINCT FROM OLD.state AND NOT (
        (OLD.state = 'queued' AND NEW.state IN ('running','failed','cancelled'))
        OR (OLD.state = 'running' AND NEW.state IN ('review_ready','failed','cancelled'))
        OR (OLD.state = 'review_ready' AND NEW.state IN ('published','cancelled'))
    ) THEN
        RAISE EXCEPTION 'invalid developer-asset ingestion state transition % -> %', OLD.state, NEW.state
            USING ERRCODE = '23514';
    END IF;
    IF (NEW.resolved_source_uri, NEW.resolved_source_revision,
        NEW.resolved_source_hash, NEW.raw_manifest, NEW.raw_manifest_hash)
       IS DISTINCT FROM
       (OLD.resolved_source_uri, OLD.resolved_source_revision,
        OLD.resolved_source_hash, OLD.raw_manifest, OLD.raw_manifest_hash)
       AND (
            OLD.state IN ('review_ready','failed','cancelled','published')
            OR EXISTS (SELECT 1 FROM documentation_documents WHERE ingestion_run_id = OLD.id)
            OR EXISTS (SELECT 1 FROM api_contract_candidates WHERE ingestion_run_id = OLD.id)
            OR EXISTS (SELECT 1 FROM sdk_content_candidates WHERE ingestion_run_id = OLD.id)
       ) THEN
        RAISE EXCEPTION 'developer-asset resolved input is immutable after candidate output or terminal review'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER developer_asset_ingestion_runs_update_guard_trigger
BEFORE UPDATE ON developer_asset_ingestion_runs
FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_ingestion_run_update();

CREATE TABLE developer_asset_ingestion_stages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ingestion_run_id uuid NOT NULL REFERENCES developer_asset_ingestion_runs(id) ON DELETE CASCADE,
    stage_name text NOT NULL CHECK (stage_name IN (
        'acquire','validate','parse','normalize','segment','extract','map',
        'ai_enrich','quality_check','build_index','review','publish'
    )),
    attempt integer NOT NULL DEFAULT 1 CHECK (attempt > 0),
    state text NOT NULL DEFAULT 'queued' CHECK (state IN (
        'queued','running','succeeded','failed','skipped','cancelled'
    )),
    input_hash text NOT NULL DEFAULT '' CHECK (
        input_hash = '' OR input_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    output_hash text NOT NULL DEFAULT '' CHECK (
        output_hash = '' OR output_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    checkpoint jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(checkpoint) = 'object'),
    diagnostics jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(diagnostics) = 'object'),
    error_code text NOT NULL DEFAULT '',
    error_message text NOT NULL DEFAULT '',
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (finished_at IS NULL OR started_at IS NOT NULL),
    UNIQUE (ingestion_run_id, stage_name, attempt)
);

-- Typed documentation corpus -------------------------------------------------

CREATE TABLE documentation_documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    ingestion_run_id uuid NOT NULL,
    legacy_knowledge_document_id uuid REFERENCES knowledge_documents(id) ON DELETE RESTRICT,
    source_path text NOT NULL CHECK (btrim(source_path) <> ''),
    canonical_url text NOT NULL DEFAULT '',
    title text NOT NULL CHECK (btrim(title) <> ''),
    document_kind text NOT NULL DEFAULT 'guide' CHECK (document_kind IN (
        'guide','reference','tutorial','concept','changelog','example','other'
    )),
    language text NOT NULL DEFAULT '',
    media_type text NOT NULL DEFAULT 'text/markdown',
    normalized_markdown text NOT NULL,
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (ingestion_run_id, deployment_id)
        REFERENCES developer_asset_ingestion_runs(id, deployment_id) ON DELETE RESTRICT,
    UNIQUE (ingestion_run_id, source_path),
    UNIQUE (ingestion_run_id, ordinal),
    UNIQUE (id, deployment_id)
);
CREATE INDEX documentation_documents_publication_idx
    ON documentation_documents(ingestion_run_id, ordinal);

CREATE TABLE documentation_sections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    documentation_document_id uuid NOT NULL,
    parent_section_id uuid,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    heading_level smallint NOT NULL DEFAULT 0 CHECK (heading_level BETWEEN 0 AND 6),
    heading text NOT NULL DEFAULT '',
    anchor text NOT NULL DEFAULT '',
    breadcrumb text[] NOT NULL DEFAULT '{}',
    content_kind text NOT NULL DEFAULT 'prose' CHECK (content_kind IN (
        'prose','code','table','schema','operation','example','warning','mixed'
    )),
    normalized_text text NOT NULL,
    code_language text NOT NULL DEFAULT '',
    token_estimate integer NOT NULL DEFAULT 0 CHECK (token_estimate >= 0),
    source_start integer CHECK (source_start IS NULL OR source_start >= 0),
    source_end integer CHECK (source_end IS NULL OR source_end >= 0),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (documentation_document_id, deployment_id)
        REFERENCES documentation_documents(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (parent_section_id, documentation_document_id)
        REFERENCES documentation_sections(id, documentation_document_id) ON DELETE RESTRICT,
    CHECK (source_end IS NULL OR source_start IS NULL OR source_end >= source_start),
    CHECK ((parent_section_id IS NULL) OR parent_section_id <> id),
    UNIQUE (documentation_document_id, ordinal),
    UNIQUE (id, documentation_document_id),
    UNIQUE (id, deployment_id)
);
CREATE INDEX documentation_sections_heading_idx
    ON documentation_sections(documentation_document_id, heading_level, ordinal);

CREATE TABLE documentation_collections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    organisation_id uuid NOT NULL,
    name text NOT NULL CHECK (btrim(name) <> ''),
    slug text NOT NULL CHECK (slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'),
    description text NOT NULL DEFAULT '',
    visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','public')),
    lifecycle text NOT NULL DEFAULT 'active' CHECK (lifecycle IN ('active','archived')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (deployment_id, organisation_id)
        REFERENCES deployments(id, organisation_id) ON DELETE RESTRICT,
    UNIQUE (deployment_id, slug),
    UNIQUE (id, deployment_id)
);

CREATE TABLE documentation_collection_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    documentation_collection_id uuid NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    selection_manifest jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(selection_manifest) = 'array'),
    reviewed_by text NOT NULL CHECK (btrim(reviewed_by) <> ''),
    reviewed_at timestamptz NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (documentation_collection_id, deployment_id)
        REFERENCES documentation_collections(id, deployment_id) ON DELETE RESTRICT,
    UNIQUE (documentation_collection_id, revision),
    UNIQUE (documentation_collection_id, content_hash),
    UNIQUE (id, documentation_collection_id),
    UNIQUE (id, deployment_id)
);

CREATE TABLE documentation_collection_members (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    documentation_collection_revision_id uuid NOT NULL,
    source_publication_id uuid,
    documentation_document_id uuid,
    documentation_section_id uuid,
    member_kind text NOT NULL CHECK (member_kind IN ('source_publication','document','section')),
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    include_descendants boolean NOT NULL DEFAULT true,
    selector jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(selector) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (documentation_collection_revision_id, deployment_id)
        REFERENCES documentation_collection_revisions(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (source_publication_id, deployment_id)
        REFERENCES source_publications(id, product_id) ON DELETE RESTRICT,
    FOREIGN KEY (documentation_document_id, deployment_id)
        REFERENCES documentation_documents(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (documentation_section_id, deployment_id)
        REFERENCES documentation_sections(id, deployment_id) ON DELETE RESTRICT,
    CHECK (
        (member_kind = 'source_publication' AND source_publication_id IS NOT NULL
            AND documentation_document_id IS NULL AND documentation_section_id IS NULL)
        OR (member_kind = 'document' AND source_publication_id IS NULL
            AND documentation_document_id IS NOT NULL AND documentation_section_id IS NULL)
        OR (member_kind = 'section' AND source_publication_id IS NULL
            AND documentation_document_id IS NULL AND documentation_section_id IS NOT NULL)
    ),
    UNIQUE (documentation_collection_revision_id, ordinal)
);
CREATE UNIQUE INDEX documentation_collection_source_member_idx
    ON documentation_collection_members(documentation_collection_revision_id, source_publication_id)
    WHERE source_publication_id IS NOT NULL;
CREATE UNIQUE INDEX documentation_collection_document_member_idx
    ON documentation_collection_members(documentation_collection_revision_id, documentation_document_id)
    WHERE documentation_document_id IS NOT NULL;
CREATE UNIQUE INDEX documentation_collection_section_member_idx
    ON documentation_collection_members(documentation_collection_revision_id, documentation_section_id)
    WHERE documentation_section_id IS NOT NULL;

-- Publication is an immutable selection over candidate documents. Excluded
-- and quarantined decisions are retained alongside included membership so the
-- review UI can explain the complete acquired corpus without mutating parser
-- output.
CREATE TABLE source_publication_document_selections (
    source_publication_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    documentation_document_id uuid NOT NULL,
    decision text NOT NULL CHECK (decision IN ('included','excluded','quarantined')),
    reason text NOT NULL DEFAULT '',
    ordinal integer CHECK (ordinal IS NULL OR ordinal >= 0),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    reviewed_by text NOT NULL CHECK (btrim(reviewed_by) <> ''),
    reviewed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (source_publication_id, deployment_id)
        REFERENCES source_publications(id, product_id) ON DELETE RESTRICT,
    FOREIGN KEY (documentation_document_id, deployment_id)
        REFERENCES documentation_documents(id, deployment_id) ON DELETE RESTRICT,
    CHECK ((decision = 'included' AND ordinal IS NOT NULL AND reason = '')
        OR (decision <> 'included' AND ordinal IS NULL AND reason <> '')),
    PRIMARY KEY (source_publication_id, documentation_document_id)
);
CREATE UNIQUE INDEX source_publication_document_included_ordinal_idx
    ON source_publication_document_selections(source_publication_id, ordinal)
    WHERE decision = 'included';

CREATE TABLE documentation_maps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    ingestion_run_id uuid,
    documentation_collection_revision_id uuid,
    map_version text NOT NULL CHECK (btrim(map_version) <> ''),
    structured_map jsonb NOT NULL CHECK (jsonb_typeof(structured_map) = 'object'),
    agent_markdown text NOT NULL,
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (ingestion_run_id, deployment_id)
        REFERENCES developer_asset_ingestion_runs(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (documentation_collection_revision_id, deployment_id)
        REFERENCES documentation_collection_revisions(id, deployment_id) ON DELETE RESTRICT,
    CHECK (num_nonnulls(ingestion_run_id, documentation_collection_revision_id) = 1),
    UNIQUE (id, deployment_id)
);
CREATE UNIQUE INDEX documentation_maps_ingestion_run_idx
    ON documentation_maps(ingestion_run_id, map_version)
    WHERE ingestion_run_id IS NOT NULL;
CREATE UNIQUE INDEX documentation_maps_collection_revision_idx
    ON documentation_maps(documentation_collection_revision_id, map_version)
    WHERE documentation_collection_revision_id IS NOT NULL;

CREATE TABLE source_publication_documentation_maps (
    source_publication_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    documentation_map_id uuid NOT NULL,
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (source_publication_id, deployment_id)
        REFERENCES source_publications(id, product_id) ON DELETE RESTRICT,
    FOREIGN KEY (documentation_map_id, deployment_id)
        REFERENCES documentation_maps(id, deployment_id) ON DELETE RESTRICT,
    PRIMARY KEY (source_publication_id, documentation_map_id),
    UNIQUE (source_publication_id)
);

CREATE TABLE deployment_documentation_publications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    revision bigint NOT NULL CHECK (revision > 0),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    snapshot_schema_version text NOT NULL CHECK (btrim(snapshot_schema_version) <> ''),
    snapshot_hash text NOT NULL CHECK (snapshot_hash ~ '^sha256:[0-9a-f]{64}$'),
    published_by text NOT NULL CHECK (btrim(published_by) <> ''),
    published_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (deployment_id, revision),
    UNIQUE (deployment_id, snapshot_hash),
    UNIQUE (id, deployment_id)
);

CREATE TABLE deployment_documentation_publication_members (
    deployment_documentation_publication_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    documentation_collection_revision_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (deployment_documentation_publication_id, deployment_id)
        REFERENCES deployment_documentation_publications(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (documentation_collection_revision_id, deployment_id)
        REFERENCES documentation_collection_revisions(id, deployment_id) ON DELETE RESTRICT,
    PRIMARY KEY (deployment_documentation_publication_id, documentation_collection_revision_id),
    UNIQUE (deployment_documentation_publication_id, ordinal)
);

CREATE TABLE deployment_documentation_heads (
    deployment_id uuid PRIMARY KEY REFERENCES deployments(id) ON DELETE RESTRICT,
    deployment_documentation_publication_id uuid NOT NULL,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_by text NOT NULL CHECK (btrim(updated_by) <> ''),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (deployment_documentation_publication_id, deployment_id)
        REFERENCES deployment_documentation_publications(id, deployment_id) ON DELETE RESTRICT
);

-- Typed API contracts --------------------------------------------------------

CREATE TABLE api_contracts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    organisation_id uuid NOT NULL,
    name text NOT NULL CHECK (btrim(name) <> ''),
    slug text NOT NULL CHECK (slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'),
    description text NOT NULL DEFAULT '',
    contract_kind text NOT NULL DEFAULT 'openapi' CHECK (contract_kind = 'openapi'),
    visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','public')),
    lifecycle text NOT NULL DEFAULT 'active' CHECK (lifecycle IN ('active','archived')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (deployment_id, organisation_id)
        REFERENCES deployments(id, organisation_id) ON DELETE RESTRICT,
    UNIQUE (deployment_id, slug),
    UNIQUE (id, deployment_id)
);

-- A configured OpenAPI/git/upload source resolves to at most one active
-- contract target. Detached history is retained so a later reassignment never
-- changes the source/target identity captured by an older ingestion run.
CREATE TABLE api_contract_sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    api_contract_id uuid NOT NULL,
    source_id uuid NOT NULL,
    source_role text NOT NULL DEFAULT 'primary'
        CHECK (source_role IN ('primary','supplemental')),
    lifecycle text NOT NULL DEFAULT 'attached'
        CHECK (lifecycle IN ('attached','detached')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (api_contract_id, deployment_id)
        REFERENCES api_contracts(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (source_id, deployment_id)
        REFERENCES sources(id, product_id) ON DELETE RESTRICT,
    UNIQUE (api_contract_id, source_id),
    UNIQUE (id, deployment_id)
);
CREATE UNIQUE INDEX api_contract_sources_one_active_target_idx
    ON api_contract_sources(deployment_id, source_id)
    WHERE lifecycle = 'attached';
CREATE UNIQUE INDEX api_contract_sources_one_active_primary_idx
    ON api_contract_sources(api_contract_id)
    WHERE lifecycle = 'attached' AND source_role = 'primary';

CREATE FUNCTION guard_api_contract_source()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    selected_source_kind text;
BEGIN
    IF TG_OP = 'UPDATE' AND
       (NEW.deployment_id, NEW.api_contract_id, NEW.source_id)
       IS DISTINCT FROM
       (OLD.deployment_id, OLD.api_contract_id, OLD.source_id) THEN
        RAISE EXCEPTION 'contract source identity is immutable; detach it and create a new association'
            USING ERRCODE = '23514';
    END IF;
    SELECT kind INTO selected_source_kind FROM sources WHERE id = NEW.source_id;
    IF selected_source_kind NOT IN ('openapi','git','upload') THEN
        RAISE EXCEPTION 'contract source kind % is not supported', selected_source_kind
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER api_contract_sources_guard_trigger
BEFORE INSERT OR UPDATE ON api_contract_sources
FOR EACH ROW EXECUTE FUNCTION guard_api_contract_source();

-- Parsing produces an immutable candidate that admins can inspect and query
-- before approval. A published revision below selects exactly one candidate.
CREATE TABLE api_contract_candidates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    api_contract_id uuid NOT NULL,
    ingestion_run_id uuid NOT NULL,
    openapi_version text NOT NULL CHECK (btrim(openapi_version) <> ''),
    source_format text NOT NULL CHECK (source_format IN ('json','yaml')),
    normalized_contract jsonb NOT NULL CHECK (jsonb_typeof(normalized_contract) = 'object'),
    source_hash text NOT NULL CHECK (source_hash ~ '^sha256:[0-9a-f]{64}$'),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    validation_result jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(validation_result) = 'object'),
    parser_version text NOT NULL CHECK (btrim(parser_version) <> ''),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    diagnostics jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(diagnostics) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (api_contract_id, deployment_id)
        REFERENCES api_contracts(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (ingestion_run_id, deployment_id)
        REFERENCES developer_asset_ingestion_runs(id, deployment_id) ON DELETE RESTRICT,
    UNIQUE (api_contract_id, ingestion_run_id),
    UNIQUE (api_contract_id, content_hash),
    UNIQUE (id, api_contract_id),
    UNIQUE (id, deployment_id)
);

CREATE TABLE api_contract_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    api_contract_id uuid NOT NULL,
    api_contract_candidate_id uuid NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    reviewed_by text NOT NULL CHECK (btrim(reviewed_by) <> ''),
    reviewed_at timestamptz NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (api_contract_id, deployment_id)
        REFERENCES api_contracts(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_contract_candidate_id, api_contract_id)
        REFERENCES api_contract_candidates(id, api_contract_id) ON DELETE RESTRICT,
    UNIQUE (api_contract_id, revision),
    UNIQUE (api_contract_id, content_hash),
    UNIQUE (api_contract_candidate_id),
    UNIQUE (id, api_contract_id),
    UNIQUE (id, api_contract_candidate_id),
    UNIQUE (id, deployment_id)
);

CREATE TABLE api_contract_revision_source_publications (
    api_contract_revision_id uuid PRIMARY KEY,
    deployment_id uuid NOT NULL,
    api_contract_candidate_id uuid NOT NULL,
    source_publication_id uuid NOT NULL,
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (api_contract_revision_id, api_contract_candidate_id)
        REFERENCES api_contract_revisions(id, api_contract_candidate_id) ON DELETE RESTRICT,
    FOREIGN KEY (source_publication_id, deployment_id)
        REFERENCES source_publications(id, product_id) ON DELETE RESTRICT
);

CREATE TABLE api_contract_operations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    api_contract_candidate_id uuid NOT NULL,
    operation_key text NOT NULL CHECK (btrim(operation_key) <> ''),
    operation_id text NOT NULL DEFAULT '',
    method text NOT NULL CHECK (method IN ('GET','POST','PUT','PATCH','DELETE','HEAD','OPTIONS','TRACE')),
    path_template text NOT NULL CHECK (left(path_template, 1) = '/'),
    tags text[] NOT NULL DEFAULT '{}',
    summary text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    security jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(security) = 'array'),
    request_schema_refs text[] NOT NULL DEFAULT '{}',
    response_schema_refs text[] NOT NULL DEFAULT '{}',
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (api_contract_candidate_id, deployment_id)
        REFERENCES api_contract_candidates(id, deployment_id) ON DELETE RESTRICT,
    UNIQUE (api_contract_candidate_id, operation_key),
    UNIQUE (api_contract_candidate_id, method, path_template),
    UNIQUE (api_contract_candidate_id, ordinal),
    UNIQUE (id, api_contract_candidate_id),
    UNIQUE (id, deployment_id)
);
CREATE UNIQUE INDEX api_contract_operations_operation_id_idx
    ON api_contract_operations(api_contract_candidate_id, operation_id)
    WHERE operation_id <> '';

CREATE TABLE api_contract_schemas (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    api_contract_candidate_id uuid NOT NULL,
    schema_key text NOT NULL CHECK (btrim(schema_key) <> ''),
    schema_document jsonb NOT NULL CHECK (jsonb_typeof(schema_document) = 'object'),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (api_contract_candidate_id, deployment_id)
        REFERENCES api_contract_candidates(id, deployment_id) ON DELETE RESTRICT,
    UNIQUE (api_contract_candidate_id, schema_key),
    UNIQUE (id, deployment_id)
);

CREATE TABLE api_contract_examples (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    api_contract_candidate_id uuid NOT NULL,
    api_contract_operation_id uuid,
    name text NOT NULL CHECK (btrim(name) <> ''),
    example_kind text NOT NULL CHECK (example_kind IN ('request','response','webhook')),
    media_type text NOT NULL DEFAULT 'application/json',
    status_code text NOT NULL DEFAULT '',
    value jsonb NOT NULL,
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (api_contract_candidate_id, deployment_id)
        REFERENCES api_contract_candidates(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_contract_operation_id, api_contract_candidate_id)
        REFERENCES api_contract_operations(id, api_contract_candidate_id) ON DELETE RESTRICT,
    UNIQUE (api_contract_candidate_id, name, example_kind, status_code),
    UNIQUE (id, deployment_id)
);

CREATE TABLE api_contract_maps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    api_contract_candidate_id uuid NOT NULL,
    map_version text NOT NULL CHECK (btrim(map_version) <> ''),
    structured_map jsonb NOT NULL CHECK (jsonb_typeof(structured_map) = 'object'),
    agent_markdown text NOT NULL,
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (api_contract_candidate_id, deployment_id)
        REFERENCES api_contract_candidates(id, deployment_id) ON DELETE RESTRICT,
    UNIQUE (api_contract_candidate_id, map_version),
    UNIQUE (id, deployment_id)
);

-- Deployment-owned SDK catalogue --------------------------------------------

CREATE TABLE sdk_packages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    organisation_id uuid NOT NULL,
    ecosystem text NOT NULL CHECK (char_length(ecosystem) BETWEEN 1 AND 40),
    canonical_coordinate text NOT NULL CHECK (
        char_length(canonical_coordinate) BETWEEN 1 AND 240
        AND canonical_coordinate = canonical_sdk_coordinate(ecosystem, canonical_coordinate)
    ),
    display_coordinate text NOT NULL CHECK (char_length(display_coordinate) BETWEEN 1 AND 240),
    name text NOT NULL CHECK (btrim(name) <> ''),
    description text NOT NULL DEFAULT '',
    registry_url text NOT NULL DEFAULT '',
    source_url text NOT NULL DEFAULT '',
    language text NOT NULL DEFAULT '',
    platform text NOT NULL DEFAULT '',
    visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','public')),
    lifecycle text NOT NULL DEFAULT 'active' CHECK (lifecycle IN ('draft','active','deprecated','archived')),
    replacement_sdk_package_id uuid,
    deprecation_message text NOT NULL DEFAULT '',
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (deployment_id, organisation_id)
        REFERENCES deployments(id, organisation_id) ON DELETE RESTRICT,
    FOREIGN KEY (replacement_sdk_package_id, deployment_id)
        REFERENCES sdk_packages(id, deployment_id) ON DELETE RESTRICT,
    CHECK (replacement_sdk_package_id IS NULL OR replacement_sdk_package_id <> id),
    CHECK (lifecycle IN ('deprecated','archived') OR replacement_sdk_package_id IS NULL),
    UNIQUE (deployment_id, ecosystem, canonical_coordinate),
    UNIQUE (id, deployment_id)
);
CREATE INDEX sdk_packages_catalog_idx
    ON sdk_packages(deployment_id, lifecycle, ecosystem, canonical_coordinate);

CREATE TABLE sdk_releases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    sdk_package_id uuid NOT NULL,
    exact_version text NOT NULL CHECK (
        char_length(exact_version) BETWEEN 1 AND 120
        AND lower(exact_version) <> 'latest'
        AND exact_version !~ '[*<>=~^]'
    ),
    install_command text NOT NULL CHECK (char_length(install_command) BETWEEN 1 AND 500),
    documentation_url text NOT NULL DEFAULT '',
    source_url text NOT NULL DEFAULT '',
    resolved_source_revision text NOT NULL DEFAULT '',
    upstream_digest text NOT NULL DEFAULT '' CHECK (
        upstream_digest = '' OR upstream_digest ~ '^(sha256|sha384|sha512):[0-9a-f]+$'
    ),
    identity_assurance text NOT NULL DEFAULT 'metadata_only'
        CHECK (identity_assurance IN ('metadata_only','resolved_source','verified_digest')),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    lifecycle text NOT NULL DEFAULT 'active'
        CHECK (lifecycle IN ('active','deprecated','yanked','archived')),
    release_hash text NOT NULL CHECK (release_hash ~ '^sha256:[0-9a-f]{64}$'),
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_package_id, deployment_id)
        REFERENCES sdk_packages(id, deployment_id) ON DELETE RESTRICT,
    CHECK (identity_assurance <> 'verified_digest' OR upstream_digest <> ''),
    UNIQUE (sdk_package_id, exact_version),
    UNIQUE (sdk_package_id, release_hash),
    UNIQUE (id, sdk_package_id),
    UNIQUE (id, deployment_id)
);
CREATE INDEX sdk_releases_package_version_idx
    ON sdk_releases(sdk_package_id, created_at DESC, exact_version);

-- Registry yanks and administrative deprecations are later facts about an
-- immutable release identity. Record them as append-only events rather than
-- rewriting the exact release used by historical API publications.
CREATE TABLE sdk_release_lifecycle_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    sdk_release_id uuid NOT NULL,
    lifecycle text NOT NULL CHECK (lifecycle IN ('active','deprecated','yanked','archived')),
    reason text NOT NULL DEFAULT '',
    observed_source_uri text NOT NULL DEFAULT '',
    observed_at timestamptz NOT NULL,
    recorded_by text NOT NULL CHECK (btrim(recorded_by) <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_release_id, deployment_id)
        REFERENCES sdk_releases(id, deployment_id) ON DELETE RESTRICT,
    UNIQUE (sdk_release_id, observed_at, lifecycle),
    UNIQUE (id, deployment_id)
);

-- SDK parsing is also candidate-first. Package/release identity is immutable,
-- while reviewed content publication selects one exact candidate.
CREATE TABLE sdk_content_candidates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    sdk_release_id uuid NOT NULL,
    ingestion_run_id uuid NOT NULL,
    pipeline_version text NOT NULL CHECK (btrim(pipeline_version) <> ''),
    parser_version text NOT NULL CHECK (btrim(parser_version) <> ''),
    normalizer_version text NOT NULL CHECK (btrim(normalizer_version) <> ''),
    mapper_version text NOT NULL CHECK (btrim(mapper_version) <> ''),
    map_version text NOT NULL CHECK (btrim(map_version) <> ''),
    source_manifest jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(source_manifest) = 'array'),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    diagnostics jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(diagnostics) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_release_id, deployment_id)
        REFERENCES sdk_releases(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (ingestion_run_id, deployment_id)
        REFERENCES developer_asset_ingestion_runs(id, deployment_id) ON DELETE RESTRICT,
    UNIQUE (sdk_release_id, ingestion_run_id),
    UNIQUE (sdk_release_id, content_hash),
    UNIQUE (id, sdk_release_id),
    UNIQUE (id, deployment_id)
);

CREATE TABLE sdk_content_publications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    sdk_release_id uuid NOT NULL,
    sdk_content_candidate_id uuid NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    reviewed_by text NOT NULL CHECK (btrim(reviewed_by) <> ''),
    reviewed_at timestamptz NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_release_id, deployment_id)
        REFERENCES sdk_releases(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_content_candidate_id, sdk_release_id)
        REFERENCES sdk_content_candidates(id, sdk_release_id) ON DELETE RESTRICT,
    UNIQUE (sdk_release_id, revision),
    UNIQUE (sdk_release_id, content_hash),
    UNIQUE (sdk_content_candidate_id),
    UNIQUE (id, sdk_release_id),
    UNIQUE (id, sdk_content_candidate_id),
    UNIQUE (id, sdk_content_candidate_id, deployment_id),
    UNIQUE (id, deployment_id)
);

CREATE TABLE sdk_publication_files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    sdk_content_candidate_id uuid NOT NULL,
    raw_blob_id uuid,
    source_path text NOT NULL CHECK (btrim(source_path) <> ''),
    file_role text NOT NULL CHECK (file_role IN (
        'readme','guide','reference','example','manifest','source','test','generated','vendor','other'
    )),
    media_type text NOT NULL DEFAULT 'application/octet-stream',
    language text NOT NULL DEFAULT '',
    suggested_disposition text NOT NULL DEFAULT 'included' CHECK (suggested_disposition IN (
        'included','excluded','quarantined','unsupported'
    )),
    exclusion_reason text NOT NULL DEFAULT '',
    normalized_content text NOT NULL DEFAULT '',
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    byte_size bigint NOT NULL DEFAULT 0 CHECK (byte_size >= 0),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_content_candidates(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (raw_blob_id, deployment_id)
        REFERENCES developer_asset_raw_blobs(id, deployment_id) ON DELETE RESTRICT,
    CHECK ((suggested_disposition = 'included') OR exclusion_reason <> ''),
    UNIQUE (sdk_content_candidate_id, source_path),
    UNIQUE (sdk_content_candidate_id, ordinal),
    UNIQUE (id, sdk_content_candidate_id),
    UNIQUE (id, sdk_content_candidate_id, deployment_id),
    UNIQUE (id, deployment_id)
);

CREATE TABLE sdk_sections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    sdk_content_candidate_id uuid NOT NULL,
    sdk_publication_file_id uuid NOT NULL,
    parent_section_id uuid,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    heading text NOT NULL DEFAULT '',
    anchor text NOT NULL DEFAULT '',
    breadcrumb text[] NOT NULL DEFAULT '{}',
    content_kind text NOT NULL DEFAULT 'prose' CHECK (content_kind IN (
        'prose','code','table','symbol','example','warning','mixed'
    )),
    normalized_text text NOT NULL,
    code_language text NOT NULL DEFAULT '',
    token_estimate integer NOT NULL DEFAULT 0 CHECK (token_estimate >= 0),
    source_start integer CHECK (source_start IS NULL OR source_start >= 0),
    source_end integer CHECK (source_end IS NULL OR source_end >= 0),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_content_candidates(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_publication_file_id, sdk_content_candidate_id)
        REFERENCES sdk_publication_files(id, sdk_content_candidate_id) ON DELETE RESTRICT,
    FOREIGN KEY (parent_section_id, sdk_content_candidate_id)
        REFERENCES sdk_sections(id, sdk_content_candidate_id) ON DELETE RESTRICT,
    CHECK (source_end IS NULL OR source_start IS NULL OR source_end >= source_start),
    CHECK ((parent_section_id IS NULL) OR parent_section_id <> id),
    UNIQUE (sdk_content_candidate_id, ordinal),
    UNIQUE (id, sdk_content_candidate_id),
    UNIQUE (id, deployment_id)
);

CREATE TABLE sdk_symbols (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    sdk_content_candidate_id uuid NOT NULL,
    sdk_publication_file_id uuid,
    sdk_section_id uuid,
    language text NOT NULL CHECK (btrim(language) <> ''),
    symbol_kind text NOT NULL CHECK (symbol_kind IN (
        'module','namespace','class','interface','type','function','method','property','constant','error'
    )),
    qualified_name text NOT NULL CHECK (btrim(qualified_name) <> ''),
    display_name text NOT NULL CHECK (btrim(display_name) <> ''),
    signature text NOT NULL DEFAULT '',
    documentation text NOT NULL DEFAULT '',
    identifiers text[] NOT NULL DEFAULT '{}',
    source_start integer CHECK (source_start IS NULL OR source_start >= 0),
    source_end integer CHECK (source_end IS NULL OR source_end >= 0),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_content_candidates(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_publication_file_id, sdk_content_candidate_id)
        REFERENCES sdk_publication_files(id, sdk_content_candidate_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_section_id, sdk_content_candidate_id)
        REFERENCES sdk_sections(id, sdk_content_candidate_id) ON DELETE RESTRICT,
    CHECK (source_end IS NULL OR source_start IS NULL OR source_end >= source_start),
    UNIQUE (sdk_content_candidate_id, language, qualified_name, symbol_kind),
    UNIQUE (id, deployment_id)
);

CREATE TABLE sdk_code_samples (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    sdk_content_candidate_id uuid NOT NULL,
    sdk_publication_file_id uuid,
    sdk_section_id uuid,
    language text NOT NULL CHECK (btrim(language) <> ''),
    title text NOT NULL CHECK (btrim(title) <> ''),
    intent text NOT NULL CHECK (btrim(intent) <> ''),
    code text NOT NULL CHECK (btrim(code) <> ''),
    imports jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(imports) = 'array'),
    prerequisites jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(prerequisites) = 'array'),
    origin text NOT NULL CHECK (origin IN ('extracted','curated','generated')),
    source_uri text NOT NULL DEFAULT '',
    source_revision text NOT NULL DEFAULT '',
    source_path text NOT NULL DEFAULT '',
    source_start integer CHECK (source_start IS NULL OR source_start >= 0),
    source_end integer CHECK (source_end IS NULL OR source_end >= 0),
    attribution text NOT NULL DEFAULT '',
    license_expression text NOT NULL DEFAULT '',
    validation_status text NOT NULL DEFAULT 'unvalidated' CHECK (validation_status IN (
        'unvalidated','syntax_checked','compiled','contract_tested','executed'
    )),
    validation_evidence jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(validation_evidence) = 'object'),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_content_candidates(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_publication_file_id, sdk_content_candidate_id)
        REFERENCES sdk_publication_files(id, sdk_content_candidate_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_section_id, sdk_content_candidate_id)
        REFERENCES sdk_sections(id, sdk_content_candidate_id) ON DELETE RESTRICT,
    CHECK (source_end IS NULL OR source_start IS NULL OR source_end >= source_start),
    CHECK (origin <> 'extracted' OR (source_path <> '' AND source_revision <> '')),
    UNIQUE (sdk_content_candidate_id, content_hash),
    UNIQUE (id, sdk_content_candidate_id),
    UNIQUE (id, sdk_content_candidate_id, deployment_id),
    UNIQUE (id, deployment_id)
);

CREATE TABLE sdk_maps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    sdk_content_candidate_id uuid NOT NULL,
    map_version text NOT NULL CHECK (btrim(map_version) <> ''),
    structured_map jsonb NOT NULL CHECK (jsonb_typeof(structured_map) = 'object'),
    agent_markdown text NOT NULL,
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_content_candidates(id, deployment_id) ON DELETE RESTRICT,
    UNIQUE (sdk_content_candidate_id, map_version),
    UNIQUE (id, sdk_content_candidate_id),
    UNIQUE (id, sdk_content_candidate_id, deployment_id),
    UNIQUE (id, deployment_id)
);

CREATE TABLE sdk_content_publication_file_selections (
    sdk_content_publication_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    sdk_content_candidate_id uuid NOT NULL,
    sdk_publication_file_id uuid NOT NULL,
    decision text NOT NULL CHECK (decision IN ('included','excluded','quarantined')),
    reason text NOT NULL DEFAULT '',
    ordinal integer CHECK (ordinal IS NULL OR ordinal >= 0),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_content_publication_id, sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_content_publications(id, sdk_content_candidate_id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_publication_file_id, sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_publication_files(id, sdk_content_candidate_id, deployment_id) ON DELETE RESTRICT,
    CHECK ((decision = 'included' AND ordinal IS NOT NULL AND reason = '')
        OR (decision <> 'included' AND ordinal IS NULL AND reason <> '')),
    PRIMARY KEY (sdk_content_publication_id, sdk_publication_file_id)
);
CREATE UNIQUE INDEX sdk_content_publication_file_included_ordinal_idx
    ON sdk_content_publication_file_selections(sdk_content_publication_id, ordinal)
    WHERE decision = 'included';

CREATE TABLE sdk_content_publication_sample_selections (
    sdk_content_publication_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    sdk_content_candidate_id uuid NOT NULL,
    sdk_code_sample_id uuid NOT NULL,
    decision text NOT NULL CHECK (decision IN ('approved','excluded','quarantined')),
    reason text NOT NULL DEFAULT '',
    ordinal integer CHECK (ordinal IS NULL OR ordinal >= 0),
    reviewed_by text NOT NULL CHECK (btrim(reviewed_by) <> ''),
    reviewed_at timestamptz NOT NULL,
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_content_publication_id, sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_content_publications(id, sdk_content_candidate_id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_code_sample_id, sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_code_samples(id, sdk_content_candidate_id, deployment_id) ON DELETE RESTRICT,
    CHECK ((decision = 'approved' AND ordinal IS NOT NULL AND reason = '')
        OR (decision <> 'approved' AND ordinal IS NULL AND reason <> '')),
    PRIMARY KEY (sdk_content_publication_id, sdk_code_sample_id)
);
CREATE UNIQUE INDEX sdk_content_publication_sample_approved_ordinal_idx
    ON sdk_content_publication_sample_selections(sdk_content_publication_id, ordinal)
    WHERE decision = 'approved';

CREATE TABLE sdk_content_publication_maps (
    sdk_content_publication_id uuid PRIMARY KEY,
    deployment_id uuid NOT NULL,
    sdk_content_candidate_id uuid NOT NULL,
    sdk_map_id uuid NOT NULL,
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_content_publication_id, sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_content_publications(id, sdk_content_candidate_id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_map_id, sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_maps(id, sdk_content_candidate_id, deployment_id) ON DELETE RESTRICT
);

CREATE TABLE sdk_compatibility_assertions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    sdk_release_id uuid NOT NULL,
    api_contract_revision_id uuid,
    supersedes_assertion_id uuid,
    coverage text NOT NULL CHECK (coverage IN ('full','partial','unknown')),
    assurance text NOT NULL CHECK (assurance IN (
        'related','documented','reviewed','tested','verified'
    )),
    assertion_state text NOT NULL DEFAULT 'active'
        CHECK (assertion_state IN ('active','superseded','withdrawn')),
    applicable_modules text[] NOT NULL DEFAULT '{}',
    applicable_capabilities text[] NOT NULL DEFAULT '{}',
    applicable_operation_keys text[] NOT NULL DEFAULT '{}',
    known_gaps text[] NOT NULL DEFAULT '{}',
    evidence jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(evidence) = 'array'),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    reviewed_by text NOT NULL CHECK (btrim(reviewed_by) <> ''),
    reviewed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (integration_id, deployment_id)
        REFERENCES integrations(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_release_id, deployment_id)
        REFERENCES sdk_releases(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_contract_revision_id, deployment_id)
        REFERENCES api_contract_revisions(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (supersedes_assertion_id, integration_id, sdk_release_id)
        REFERENCES sdk_compatibility_assertions(id, integration_id, sdk_release_id) ON DELETE RESTRICT,
    CHECK (supersedes_assertion_id IS NULL OR supersedes_assertion_id <> id),
    CHECK (assurance NOT IN ('tested','verified') OR jsonb_array_length(evidence) > 0),
    UNIQUE (integration_id, sdk_release_id, content_hash),
    UNIQUE (id, integration_id, sdk_release_id),
    UNIQUE (id, deployment_id)
);

-- Draft API bindings. Documentation and contracts may follow the latest
-- reviewed revision while in draft; SDK bindings are exact only.
CREATE TABLE api_documentation_bindings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    documentation_collection_id uuid NOT NULL,
    follow_latest boolean NOT NULL DEFAULT true,
    pinned_revision_id uuid,
    selector jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(selector) = 'object'),
    visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','public')),
    lifecycle text NOT NULL DEFAULT 'attached' CHECK (lifecycle IN ('attached','detached')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (integration_id, deployment_id)
        REFERENCES integrations(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (documentation_collection_id, deployment_id)
        REFERENCES documentation_collections(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (pinned_revision_id, documentation_collection_id)
        REFERENCES documentation_collection_revisions(id, documentation_collection_id) ON DELETE RESTRICT,
    CHECK ((follow_latest AND pinned_revision_id IS NULL) OR
           (NOT follow_latest AND pinned_revision_id IS NOT NULL)),
    UNIQUE (integration_id, documentation_collection_id),
    UNIQUE (id, integration_id, documentation_collection_id, deployment_id)
);

CREATE TABLE api_contract_bindings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    api_contract_id uuid NOT NULL,
    follow_latest boolean NOT NULL DEFAULT true,
    pinned_revision_id uuid,
    primary_contract boolean NOT NULL DEFAULT false,
    visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','public')),
    lifecycle text NOT NULL DEFAULT 'attached' CHECK (lifecycle IN ('attached','detached')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (integration_id, deployment_id)
        REFERENCES integrations(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_contract_id, deployment_id)
        REFERENCES api_contracts(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (pinned_revision_id, api_contract_id)
        REFERENCES api_contract_revisions(id, api_contract_id) ON DELETE RESTRICT,
    CHECK ((follow_latest AND pinned_revision_id IS NULL) OR
           (NOT follow_latest AND pinned_revision_id IS NOT NULL)),
    UNIQUE (integration_id, api_contract_id),
    UNIQUE (id, integration_id, api_contract_id, deployment_id)
);
CREATE UNIQUE INDEX api_contract_bindings_one_primary_idx
    ON api_contract_bindings(integration_id)
    WHERE primary_contract AND lifecycle = 'attached';

CREATE TABLE api_sdk_bindings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    sdk_package_id uuid NOT NULL,
    sdk_release_id uuid NOT NULL,
    sdk_content_publication_id uuid,
    api_contract_revision_id uuid,
    compatibility_assertion_id uuid,
    binding_state text NOT NULL DEFAULT 'draft' CHECK (binding_state IN (
        'legacy_metadata','draft','ready','detached'
    )),
    coverage text NOT NULL DEFAULT 'unknown' CHECK (coverage IN ('full','partial','unknown')),
    assurance text NOT NULL DEFAULT 'related' CHECK (assurance IN (
        'related','documented','reviewed','tested','verified'
    )),
    applicable_modules text[] NOT NULL DEFAULT '{}',
    applicable_capabilities text[] NOT NULL DEFAULT '{}',
    applicable_operation_keys text[] NOT NULL DEFAULT '{}',
    selector jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(selector) = 'object'),
    selector_hash text NOT NULL CHECK (selector_hash ~ '^sha256:[0-9a-f]{64}$'),
    visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','public')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (integration_id, deployment_id)
        REFERENCES integrations(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_package_id, deployment_id)
        REFERENCES sdk_packages(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_release_id, sdk_package_id)
        REFERENCES sdk_releases(id, sdk_package_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_content_publication_id, sdk_release_id)
        REFERENCES sdk_content_publications(id, sdk_release_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_contract_revision_id, deployment_id)
        REFERENCES api_contract_revisions(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (compatibility_assertion_id, integration_id, sdk_release_id)
        REFERENCES sdk_compatibility_assertions(id, integration_id, sdk_release_id) ON DELETE RESTRICT,
    CHECK (binding_state <> 'ready' OR sdk_content_publication_id IS NOT NULL),
    CHECK (assurance NOT IN ('tested','verified') OR compatibility_assertion_id IS NOT NULL),
    CHECK (selector_hash = 'sha256:' || encode(
        digest(convert_to(selector::text, 'UTF8'), 'sha256'), 'hex'
    )),
    UNIQUE (id, integration_id),
    UNIQUE (id, sdk_content_publication_id),
    UNIQUE (id, compatibility_assertion_id),
    UNIQUE (id, integration_id, sdk_package_id, sdk_release_id, deployment_id)
);
CREATE UNIQUE INDEX api_sdk_bindings_one_active_package_idx
    ON api_sdk_bindings(integration_id, sdk_package_id)
    WHERE binding_state <> 'detached';
CREATE INDEX api_sdk_bindings_release_idx
    ON api_sdk_bindings(sdk_release_id, integration_id);

CREATE TABLE sdk_sample_api_references (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sdk_code_sample_id uuid NOT NULL,
    sdk_content_candidate_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    api_contract_revision_id uuid,
    api_contract_candidate_id uuid,
    api_contract_operation_id uuid,
    api_sdk_binding_id uuid,
    reference_kind text NOT NULL CHECK (reference_kind IN ('api','contract','operation')),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (sdk_code_sample_id, sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_code_samples(id, sdk_content_candidate_id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (integration_id, deployment_id)
        REFERENCES integrations(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_contract_revision_id, api_contract_candidate_id)
        REFERENCES api_contract_revisions(id, api_contract_candidate_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_contract_operation_id, api_contract_candidate_id)
        REFERENCES api_contract_operations(id, api_contract_candidate_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_sdk_binding_id, integration_id)
        REFERENCES api_sdk_bindings(id, integration_id) ON DELETE RESTRICT,
    CHECK (
        (reference_kind = 'api' AND api_contract_revision_id IS NULL
            AND api_contract_candidate_id IS NULL AND api_contract_operation_id IS NULL)
        OR (reference_kind = 'contract' AND api_contract_revision_id IS NOT NULL
            AND api_contract_candidate_id IS NOT NULL AND api_contract_operation_id IS NULL)
        OR (reference_kind = 'operation' AND api_contract_revision_id IS NOT NULL
            AND api_contract_candidate_id IS NOT NULL AND api_contract_operation_id IS NOT NULL)
    ),
    UNIQUE (sdk_code_sample_id, integration_id, reference_kind,
        api_contract_revision_id, api_contract_operation_id)
);
CREATE UNIQUE INDEX sdk_sample_api_only_reference_idx
    ON sdk_sample_api_references(sdk_code_sample_id, integration_id, reference_kind)
    WHERE reference_kind = 'api';
CREATE UNIQUE INDEX sdk_sample_contract_reference_idx
    ON sdk_sample_api_references(
        sdk_code_sample_id, integration_id, api_contract_revision_id, reference_kind
    ) WHERE reference_kind = 'contract';
CREATE UNIQUE INDEX sdk_sample_operation_reference_idx
    ON sdk_sample_api_references(
        sdk_code_sample_id, integration_id, api_contract_operation_id, reference_kind
    ) WHERE reference_kind = 'operation';

-- Exact developer-asset snapshots extend, but never rewrite, an immutable API
-- integration revision.
CREATE TABLE api_developer_asset_publications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    integration_revision_id uuid NOT NULL,
    deployment_documentation_publication_id uuid,
    snapshot_schema_version text NOT NULL CHECK (btrim(snapshot_schema_version) <> ''),
    snapshot_hash text NOT NULL CHECK (snapshot_hash ~ '^sha256:[0-9a-f]{64}$'),
    published_by text NOT NULL CHECK (btrim(published_by) <> ''),
    published_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (integration_id, deployment_id)
        REFERENCES integrations(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (integration_revision_id, integration_id)
        REFERENCES integration_revisions(id, integration_id) ON DELETE RESTRICT,
    FOREIGN KEY (deployment_documentation_publication_id, deployment_id)
        REFERENCES deployment_documentation_publications(id, deployment_id) ON DELETE RESTRICT,
    UNIQUE (integration_revision_id),
    UNIQUE (integration_id, snapshot_hash),
    UNIQUE (id, deployment_id, integration_id),
    UNIQUE (id, deployment_id)
);

CREATE TABLE api_publication_documentation_assets (
    api_developer_asset_publication_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    api_documentation_binding_id uuid NOT NULL,
    documentation_collection_id uuid NOT NULL,
    documentation_collection_revision_id uuid NOT NULL,
    selector jsonb NOT NULL CHECK (jsonb_typeof(selector) = 'object'),
    selector_hash text NOT NULL CHECK (selector_hash ~ '^sha256:[0-9a-f]{64}$'),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (api_developer_asset_publication_id, deployment_id, integration_id)
        REFERENCES api_developer_asset_publications(id, deployment_id, integration_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_documentation_binding_id, integration_id, documentation_collection_id, deployment_id)
        REFERENCES api_documentation_bindings(
            id, integration_id, documentation_collection_id, deployment_id
        ) ON DELETE RESTRICT,
    FOREIGN KEY (documentation_collection_revision_id, documentation_collection_id)
        REFERENCES documentation_collection_revisions(id, documentation_collection_id) ON DELETE RESTRICT,
    PRIMARY KEY (api_developer_asset_publication_id, api_documentation_binding_id),
    UNIQUE (api_developer_asset_publication_id, ordinal)
);

CREATE TABLE api_publication_contract_assets (
    api_developer_asset_publication_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    api_contract_binding_id uuid NOT NULL,
    api_contract_id uuid NOT NULL,
    api_contract_revision_id uuid NOT NULL,
    primary_contract boolean NOT NULL DEFAULT false,
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (api_developer_asset_publication_id, deployment_id, integration_id)
        REFERENCES api_developer_asset_publications(id, deployment_id, integration_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_contract_binding_id, integration_id, api_contract_id, deployment_id)
        REFERENCES api_contract_bindings(id, integration_id, api_contract_id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_contract_revision_id, api_contract_id)
        REFERENCES api_contract_revisions(id, api_contract_id) ON DELETE RESTRICT,
    PRIMARY KEY (api_developer_asset_publication_id, api_contract_binding_id),
    UNIQUE (api_developer_asset_publication_id, ordinal)
);
CREATE UNIQUE INDEX api_publication_contract_one_primary_idx
    ON api_publication_contract_assets(api_developer_asset_publication_id)
    WHERE primary_contract;

CREATE TABLE api_publication_sdk_assets (
    api_developer_asset_publication_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    api_sdk_binding_id uuid NOT NULL,
    sdk_package_id uuid NOT NULL,
    sdk_release_id uuid NOT NULL,
    sdk_content_publication_id uuid,
    compatibility_assertion_id uuid,
    selector jsonb NOT NULL CHECK (jsonb_typeof(selector) = 'object'),
    selector_hash text NOT NULL CHECK (selector_hash ~ '^sha256:[0-9a-f]{64}$'),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (api_developer_asset_publication_id, deployment_id, integration_id)
        REFERENCES api_developer_asset_publications(id, deployment_id, integration_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_sdk_binding_id, integration_id, sdk_package_id, sdk_release_id, deployment_id)
        REFERENCES api_sdk_bindings(
            id, integration_id, sdk_package_id, sdk_release_id, deployment_id
        ) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_package_id, deployment_id)
        REFERENCES sdk_packages(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_release_id, sdk_package_id)
        REFERENCES sdk_releases(id, sdk_package_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_content_publication_id, sdk_release_id)
        REFERENCES sdk_content_publications(id, sdk_release_id) ON DELETE RESTRICT,
    FOREIGN KEY (compatibility_assertion_id, deployment_id)
        REFERENCES sdk_compatibility_assertions(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_sdk_binding_id, sdk_content_publication_id)
        REFERENCES api_sdk_bindings(id, sdk_content_publication_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_sdk_binding_id, compatibility_assertion_id)
        REFERENCES api_sdk_bindings(id, compatibility_assertion_id) ON DELETE RESTRICT,
    PRIMARY KEY (api_developer_asset_publication_id, api_sdk_binding_id),
    UNIQUE (api_developer_asset_publication_id, sdk_package_id),
    UNIQUE (api_developer_asset_publication_id, ordinal)
);

-- Rebuildable retrieval projection and reproducible Query Lab evidence --------

CREATE TABLE search_index_generations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    publication_kind text NOT NULL CHECK (publication_kind IN (
        'source','documentation_collection','global_documentation','contract','sdk','api'
    )),
    publication_id uuid NOT NULL,
    asset_kind text NOT NULL CHECK (asset_kind IN ('documentation','contract','sdk','mixed')),
    builder_version text NOT NULL CHECK (btrim(builder_version) <> ''),
    retrieval_profile_version text NOT NULL CHECK (btrim(retrieval_profile_version) <> ''),
    embedding_model text NOT NULL DEFAULT '',
    embedding_dimensions integer CHECK (embedding_dimensions IS NULL OR embedding_dimensions > 0),
    state text NOT NULL DEFAULT 'queued' CHECK (state IN (
        'queued','building','ready','failed','superseded'
    )),
    unit_count integer NOT NULL DEFAULT 0 CHECK (unit_count >= 0),
    content_hash text NOT NULL DEFAULT '' CHECK (
        content_hash = '' OR content_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    diagnostics jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(diagnostics) = 'object'),
    started_at timestamptz,
    ready_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((embedding_model = '' AND embedding_dimensions IS NULL)
        OR (embedding_model <> '' AND embedding_dimensions IS NOT NULL)),
    CHECK (state <> 'ready' OR (ready_at IS NOT NULL AND content_hash <> '')),
    UNIQUE (publication_kind, publication_id, builder_version, retrieval_profile_version),
    UNIQUE (id, deployment_id)
);
CREATE INDEX search_index_generations_ready_idx
    ON search_index_generations(deployment_id, publication_kind, publication_id, ready_at DESC)
    WHERE state = 'ready';

CREATE TABLE knowledge_units (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    search_index_generation_id uuid NOT NULL,
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    unit_kind text NOT NULL CHECK (unit_kind IN (
        'document','section','contract_operation','contract_schema','contract_example',
        'sdk_section','sdk_symbol','sdk_sample','map'
    )),
    source_publication_kind text NOT NULL CHECK (source_publication_kind IN (
        'source','documentation_collection','global_documentation','contract','sdk','api'
    )),
    source_publication_id uuid NOT NULL,
    source_entity_id uuid NOT NULL,
    parent_source_entity_id uuid,
    title text NOT NULL DEFAULT '',
    breadcrumb text[] NOT NULL DEFAULT '{}',
    content text NOT NULL,
    content_tsv tsvector GENERATED ALWAYS AS (
        to_tsvector('english', coalesce(title, '') || ' ' || content)
    ) STORED,
    embedding vector,
    language text NOT NULL DEFAULT '',
    ecosystem text NOT NULL DEFAULT '',
    identifiers text[] NOT NULL DEFAULT '{}',
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    citation jsonb NOT NULL CHECK (jsonb_typeof(citation) = 'object'),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (search_index_generation_id, deployment_id)
        REFERENCES search_index_generations(id, deployment_id) ON DELETE CASCADE,
    UNIQUE (search_index_generation_id, source_publication_kind, source_entity_id),
    UNIQUE (search_index_generation_id, ordinal),
    UNIQUE (id, deployment_id)
);
CREATE INDEX knowledge_units_fts_idx ON knowledge_units USING gin(content_tsv);
CREATE INDEX knowledge_units_identifiers_idx ON knowledge_units USING gin(identifiers);
CREATE INDEX knowledge_units_scope_idx
    ON knowledge_units(deployment_id, visibility, unit_kind, language, ecosystem);

CREATE TABLE knowledge_unit_api_scopes (
    knowledge_unit_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    api_sdk_binding_id uuid,
    scope_kind text NOT NULL CHECK (scope_kind IN ('global','attached','shared','selected')),
    selector_hash text NOT NULL DEFAULT '' CHECK (
        selector_hash = '' OR selector_hash ~ '^sha256:[0-9a-f]{64}$'
    ),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (knowledge_unit_id, deployment_id)
        REFERENCES knowledge_units(id, deployment_id) ON DELETE CASCADE,
    FOREIGN KEY (integration_id, deployment_id)
        REFERENCES integrations(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_sdk_binding_id, integration_id)
        REFERENCES api_sdk_bindings(id, integration_id) ON DELETE RESTRICT,
    PRIMARY KEY (knowledge_unit_id, integration_id)
);

CREATE TABLE retrieval_query_traces (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    deployment_documentation_publication_id uuid,
    api_developer_asset_publication_id uuid,
    retrieval_profile_version text NOT NULL CHECK (btrim(retrieval_profile_version) <> ''),
    query_text text NOT NULL CHECK (octet_length(query_text) BETWEEN 1 AND 32768),
    query_hash text NOT NULL CHECK (query_hash ~ '^sha256:[0-9a-f]{64}$'),
    requested_filters jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(requested_filters) = 'object'),
    resolved_scope jsonb NOT NULL CHECK (jsonb_typeof(resolved_scope) = 'object'),
    routing_decision jsonb NOT NULL CHECK (jsonb_typeof(routing_decision) = 'object'),
    state text NOT NULL CHECK (state IN ('succeeded','no_results','failed')),
    candidate_count integer NOT NULL DEFAULT 0 CHECK (candidate_count >= 0),
    result_count integer NOT NULL DEFAULT 0 CHECK (result_count >= 0),
    context_tokens integer NOT NULL DEFAULT 0 CHECK (context_tokens >= 0),
    latency_ms integer NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
    diagnostics jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(diagnostics) = 'object'),
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (deployment_documentation_publication_id, deployment_id)
        REFERENCES deployment_documentation_publications(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_developer_asset_publication_id, deployment_id)
        REFERENCES api_developer_asset_publications(id, deployment_id) ON DELETE RESTRICT,
    CHECK (num_nonnulls(deployment_documentation_publication_id, api_developer_asset_publication_id) > 0),
    UNIQUE (id, deployment_id)
);
CREATE INDEX retrieval_query_traces_created_idx
    ON retrieval_query_traces(deployment_id, created_at DESC);

CREATE TABLE retrieval_query_trace_results (
    retrieval_query_trace_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    rank integer NOT NULL CHECK (rank > 0),
    knowledge_unit_id uuid,
    source_publication_kind text NOT NULL,
    source_publication_id uuid NOT NULL,
    source_entity_id uuid NOT NULL,
    lexical_score double precision,
    semantic_score double precision,
    rerank_score double precision,
    selected boolean NOT NULL DEFAULT true,
    exclusion_reason text NOT NULL DEFAULT '',
    citation jsonb NOT NULL CHECK (jsonb_typeof(citation) = 'object'),
    excerpt text NOT NULL DEFAULT '',
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (retrieval_query_trace_id, deployment_id)
        REFERENCES retrieval_query_traces(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (knowledge_unit_id, deployment_id)
        REFERENCES knowledge_units(id, deployment_id) ON DELETE SET NULL (knowledge_unit_id),
    CHECK (selected OR exclusion_reason <> ''),
    PRIMARY KEY (retrieval_query_trace_id, rank)
);

CREATE TABLE retrieval_evaluation_sets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    name text NOT NULL CHECK (btrim(name) <> ''),
    description text NOT NULL DEFAULT '',
    lifecycle text NOT NULL DEFAULT 'active' CHECK (lifecycle IN ('active','archived')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (deployment_id, name),
    UNIQUE (id, deployment_id)
);

CREATE TABLE retrieval_evaluation_set_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    retrieval_evaluation_set_id uuid NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_by text NOT NULL CHECK (btrim(created_by) <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (retrieval_evaluation_set_id, deployment_id)
        REFERENCES retrieval_evaluation_sets(id, deployment_id) ON DELETE RESTRICT,
    UNIQUE (retrieval_evaluation_set_id, revision),
    UNIQUE (retrieval_evaluation_set_id, content_hash),
    UNIQUE (id, deployment_id)
);

CREATE TABLE retrieval_evaluation_cases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    retrieval_evaluation_set_revision_id uuid NOT NULL,
    case_key text NOT NULL CHECK (btrim(case_key) <> ''),
    query text NOT NULL CHECK (octet_length(query) BETWEEN 1 AND 32768),
    requested_filters jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(requested_filters) = 'object'),
    expected_evidence jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(expected_evidence) = 'array'),
    forbidden_evidence jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(forbidden_evidence) = 'array'),
    expect_no_results boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (retrieval_evaluation_set_revision_id, deployment_id)
        REFERENCES retrieval_evaluation_set_revisions(id, deployment_id) ON DELETE RESTRICT,
    CHECK (NOT expect_no_results OR jsonb_array_length(expected_evidence) = 0),
    UNIQUE (retrieval_evaluation_set_revision_id, case_key),
    UNIQUE (id, retrieval_evaluation_set_revision_id, deployment_id),
    UNIQUE (id, deployment_id)
);

CREATE TABLE retrieval_evaluation_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL,
    retrieval_evaluation_set_revision_id uuid NOT NULL,
    deployment_documentation_publication_id uuid,
    api_developer_asset_publication_id uuid,
    retrieval_profile_version text NOT NULL CHECK (btrim(retrieval_profile_version) <> ''),
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','running','passed','failed','cancelled')),
    metrics jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metrics) = 'object'),
    started_at timestamptz,
    finished_at timestamptz,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (retrieval_evaluation_set_revision_id, deployment_id)
        REFERENCES retrieval_evaluation_set_revisions(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (deployment_documentation_publication_id, deployment_id)
        REFERENCES deployment_documentation_publications(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_developer_asset_publication_id, deployment_id)
        REFERENCES api_developer_asset_publications(id, deployment_id) ON DELETE RESTRICT,
    CHECK (num_nonnulls(deployment_documentation_publication_id, api_developer_asset_publication_id) > 0),
    CHECK (finished_at IS NULL OR started_at IS NOT NULL),
    UNIQUE (id, deployment_id),
    UNIQUE (id, retrieval_evaluation_set_revision_id, deployment_id)
);

CREATE TABLE retrieval_evaluation_case_results (
    retrieval_evaluation_run_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    retrieval_evaluation_set_revision_id uuid NOT NULL,
    retrieval_evaluation_case_id uuid NOT NULL,
    retrieval_query_trace_id uuid,
    passed boolean NOT NULL,
    metrics jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metrics) = 'object'),
    failure_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (retrieval_evaluation_run_id, retrieval_evaluation_set_revision_id, deployment_id)
        REFERENCES retrieval_evaluation_runs(
            id, retrieval_evaluation_set_revision_id, deployment_id
        ) ON DELETE RESTRICT,
    FOREIGN KEY (retrieval_evaluation_case_id, retrieval_evaluation_set_revision_id, deployment_id)
        REFERENCES retrieval_evaluation_cases(
            id, retrieval_evaluation_set_revision_id, deployment_id
        ) ON DELETE RESTRICT,
    FOREIGN KEY (retrieval_query_trace_id, deployment_id)
        REFERENCES retrieval_query_traces(id, deployment_id) ON DELETE RESTRICT,
    CHECK (passed OR failure_reason <> ''),
    PRIMARY KEY (retrieval_evaluation_run_id, retrieval_evaluation_case_id)
);

-- Resolve the generic run target once, at enqueue time. The captured target is
-- then protected by the run-update guard even if an administrator later
-- detaches a contract source association.
CREATE FUNCTION guard_developer_asset_ingestion_run_target()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    CASE NEW.asset_kind
    WHEN 'documentation' THEN
        -- The scoped source foreign key and target=source CHECK are sufficient.
        NULL;
    WHEN 'contract' THEN
        IF NOT EXISTS (
            SELECT 1
              FROM api_contract_sources binding
             WHERE binding.deployment_id = NEW.deployment_id
               AND binding.source_id = NEW.source_id
               AND binding.api_contract_id = NEW.target_id
               AND binding.lifecycle = 'attached'
        ) THEN
            RAISE EXCEPTION 'contract ingestion requires one active source-to-contract target'
                USING ERRCODE = '23514';
        END IF;
    WHEN 'sdk' THEN
        IF NOT EXISTS (
            SELECT 1 FROM sdk_releases release
             WHERE release.id = NEW.target_id
               AND release.deployment_id = NEW.deployment_id
        ) THEN
            RAISE EXCEPTION 'SDK ingestion target must be an exact release in the same deployment'
                USING ERRCODE = '23514';
        END IF;
    ELSE
        RAISE EXCEPTION 'unsupported developer-asset ingestion kind %', NEW.asset_kind
            USING ERRCODE = '23514';
    END CASE;
    RETURN NEW;
END;
$$;
CREATE TRIGGER developer_asset_ingestion_runs_target_guard_trigger
BEFORE INSERT ON developer_asset_ingestion_runs
FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_ingestion_run_target();

-- Compatibility-first migration of the current API-owned SDK references. Only
-- groups whose exact release metadata agrees are shared. Conflicts stay on the
-- legacy path and are recorded for deliberate administrator review.
CREATE TABLE legacy_sdk_reference_migration_ledger (
    legacy_sdk_reference_id uuid PRIMARY KEY,
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    integration_id uuid NOT NULL,
    sdk_package_id uuid,
    sdk_release_id uuid,
    api_sdk_binding_id uuid,
    status text NOT NULL CHECK (status IN ('migrated','conflict')),
    conflict_code text NOT NULL DEFAULT '',
    legacy_snapshot jsonb NOT NULL CHECK (jsonb_typeof(legacy_snapshot) = 'object'),
    migrated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (integration_id, deployment_id)
        REFERENCES integrations(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_package_id, deployment_id)
        REFERENCES sdk_packages(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_release_id, deployment_id)
        REFERENCES sdk_releases(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_sdk_binding_id)
        REFERENCES api_sdk_bindings(id) ON DELETE RESTRICT,
    CHECK ((status = 'migrated' AND conflict_code = '' AND api_sdk_binding_id = legacy_sdk_reference_id)
        OR (status = 'conflict' AND conflict_code <> '' AND api_sdk_binding_id IS NULL))
);

INSERT INTO sdk_packages(
    deployment_id, organisation_id, ecosystem, canonical_coordinate,
    display_coordinate, name, visibility, lifecycle, created_at, updated_at
)
SELECT
    reference.deployment_id,
    reference.organisation_id,
    lower(btrim(reference.ecosystem)),
    canonical_sdk_coordinate(reference.ecosystem, reference.coordinate),
    min(reference.coordinate),
    min(reference.coordinate),
    CASE WHEN bool_or(reference.visibility = 'public') THEN 'public' ELSE 'private' END,
    'active',
    min(reference.created_at),
    max(reference.updated_at)
FROM sdk_references reference
GROUP BY reference.deployment_id, reference.organisation_id,
    lower(btrim(reference.ecosystem)),
    canonical_sdk_coordinate(reference.ecosystem, reference.coordinate)
ON CONFLICT (deployment_id, ecosystem, canonical_coordinate) DO NOTHING;

WITH normalized AS (
    SELECT
        reference.*,
        package.id AS sdk_package_id,
        jsonb_build_object(
            'install_command', reference.install_command,
            'documentation_url', reference.documentation_url,
            'source_url', reference.source_url,
            'checksum', reference.checksum,
            'visibility', reference.visibility
        ) AS immutable_metadata
    FROM sdk_references reference
    JOIN sdk_packages package
      ON package.deployment_id = reference.deployment_id
     AND package.ecosystem = lower(btrim(reference.ecosystem))
     AND package.canonical_coordinate = canonical_sdk_coordinate(reference.ecosystem, reference.coordinate)
), safe_releases AS (
    SELECT
        sdk_package_id,
        deployment_id,
        exact_version,
        min(install_command) AS install_command,
        min(documentation_url) AS documentation_url,
        min(source_url) AS source_url,
        min(checksum) AS checksum,
        min(visibility) AS visibility,
        min(created_at) AS created_at
    FROM normalized
    GROUP BY sdk_package_id, deployment_id, exact_version
    HAVING count(DISTINCT immutable_metadata) = 1
)
INSERT INTO sdk_releases(
    deployment_id, sdk_package_id, exact_version, install_command,
    documentation_url, source_url, upstream_digest, identity_assurance,
    visibility, lifecycle, release_hash, created_at
)
SELECT
    safe.deployment_id,
    safe.sdk_package_id,
    safe.exact_version,
    safe.install_command,
    safe.documentation_url,
    safe.source_url,
    safe.checksum,
    CASE WHEN safe.checksum = '' THEN 'metadata_only' ELSE 'verified_digest' END,
    safe.visibility,
    'active',
    'sha256:' || encode(digest(convert_to(jsonb_build_object(
        'package_id', safe.sdk_package_id,
        'version', safe.exact_version,
        'install_command', safe.install_command,
        'documentation_url', safe.documentation_url,
        'source_url', safe.source_url,
        'digest', safe.checksum,
        'visibility', safe.visibility
    )::text, 'UTF8'), 'sha256'), 'hex'),
    safe.created_at
FROM safe_releases safe
ON CONFLICT (sdk_package_id, exact_version) DO NOTHING;

WITH normalized AS (
    SELECT
        reference.*,
        package.id AS sdk_package_id
    FROM sdk_references reference
    JOIN sdk_packages package
      ON package.deployment_id = reference.deployment_id
     AND package.ecosystem = lower(btrim(reference.ecosystem))
     AND package.canonical_coordinate = canonical_sdk_coordinate(reference.ecosystem, reference.coordinate)
)
INSERT INTO api_sdk_bindings(
    id, deployment_id, integration_id, sdk_package_id, sdk_release_id,
    binding_state, coverage, assurance, selector, selector_hash, visibility,
    revision, created_by, created_at, updated_at
)
SELECT
    reference.id,
    reference.deployment_id,
    reference.integration_id,
    reference.sdk_package_id,
    release.id,
    'legacy_metadata',
    'unknown',
    'related',
    '{}'::jsonb,
    'sha256:' || encode(digest(convert_to('{}', 'UTF8'), 'sha256'), 'hex'),
    reference.visibility,
    reference.revision,
    'migration:0057',
    reference.created_at,
    reference.updated_at
FROM normalized reference
JOIN sdk_releases release
  ON release.sdk_package_id = reference.sdk_package_id
 AND release.exact_version = reference.exact_version
 AND release.install_command = reference.install_command
 AND release.documentation_url = reference.documentation_url
 AND release.source_url = reference.source_url
 AND release.upstream_digest = reference.checksum
 AND release.visibility = reference.visibility
ON CONFLICT DO NOTHING;

INSERT INTO legacy_sdk_reference_migration_ledger(
    legacy_sdk_reference_id, deployment_id, integration_id, sdk_package_id,
    sdk_release_id, api_sdk_binding_id, status, conflict_code, legacy_snapshot
)
SELECT
    reference.id,
    reference.deployment_id,
    reference.integration_id,
    package.id,
    release.id,
    binding.id,
    CASE WHEN binding.id IS NOT NULL THEN 'migrated' ELSE 'conflict' END,
    CASE
        WHEN release.id IS NULL THEN 'conflicting_release_metadata'
        WHEN binding.id IS NULL THEN 'duplicate_api_package_binding'
        ELSE ''
    END,
    jsonb_build_object(
        'id', reference.id,
        'ecosystem', reference.ecosystem,
        'coordinate', reference.coordinate,
        'exact_version', reference.exact_version,
        'install_command', reference.install_command,
        'documentation_url', reference.documentation_url,
        'source_url', reference.source_url,
        'checksum', reference.checksum,
        'visibility', reference.visibility,
        'revision', reference.revision
    )
FROM sdk_references reference
JOIN sdk_packages package
  ON package.deployment_id = reference.deployment_id
 AND package.ecosystem = lower(btrim(reference.ecosystem))
 AND package.canonical_coordinate = canonical_sdk_coordinate(reference.ecosystem, reference.coordinate)
LEFT JOIN sdk_releases release
  ON release.sdk_package_id = package.id
 AND release.exact_version = reference.exact_version
 AND release.install_command = reference.install_command
 AND release.documentation_url = reference.documentation_url
 AND release.source_url = reference.source_url
 AND release.upstream_digest = reference.checksum
 AND release.visibility = reference.visibility
LEFT JOIN api_sdk_bindings binding ON binding.id = reference.id;

CREATE FUNCTION guard_developer_asset_candidate_membership()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    selected_hash text;
    run_asset_kind text;
    run_target_id uuid;
    run_source_id uuid;
    run_state text;
    publication_source_id uuid;
    published_candidate_id uuid;
    selected_validation_status text;
    publication_visibility text;
    candidate_visibility text;
BEGIN
    CASE TG_TABLE_NAME
    WHEN 'documentation_documents' THEN
        SELECT asset_kind, source_id, target_id
          INTO run_asset_kind, run_source_id, run_target_id
          FROM developer_asset_ingestion_runs
         WHERE id = NEW.ingestion_run_id;
        IF run_asset_kind IS DISTINCT FROM 'documentation'
           OR run_source_id IS NULL
           OR run_target_id IS DISTINCT FROM run_source_id THEN
            RAISE EXCEPTION 'documentation candidate must belong to a source-backed documentation ingestion run';
        END IF;
    WHEN 'source_publication_document_selections' THEN
        SELECT publication.source_id, document.content_hash,
               run.source_id, run.state, run.asset_kind,
               publication.visibility::text, document.visibility
          INTO publication_source_id, selected_hash,
               run_source_id, run_state, run_asset_kind,
               publication_visibility, candidate_visibility
          FROM source_publications publication
          JOIN documentation_documents document
            ON document.id = NEW.documentation_document_id
          JOIN developer_asset_ingestion_runs run
            ON run.id = document.ingestion_run_id
         WHERE publication.id = NEW.source_publication_id;
        IF run_asset_kind IS DISTINCT FROM 'documentation'
           OR run_source_id IS DISTINCT FROM publication_source_id
           OR run_state NOT IN ('review_ready','published')
           OR NEW.content_hash IS DISTINCT FROM selected_hash
           OR (publication_visibility = 'public' AND candidate_visibility <> 'public') THEN
            RAISE EXCEPTION 'source publication selection must pin reviewed output from the same documentation source';
        END IF;
    WHEN 'source_publication_documentation_maps' THEN
        SELECT publication.source_id, map.content_hash,
               run.source_id, run.state, run.asset_kind,
               publication.visibility::text, map.visibility
          INTO publication_source_id, selected_hash,
               run_source_id, run_state, run_asset_kind,
               publication_visibility, candidate_visibility
          FROM source_publications publication
          JOIN documentation_maps map ON map.id = NEW.documentation_map_id
          JOIN developer_asset_ingestion_runs run ON run.id = map.ingestion_run_id
         WHERE publication.id = NEW.source_publication_id;
        IF run_asset_kind IS DISTINCT FROM 'documentation'
           OR run_source_id IS DISTINCT FROM publication_source_id
           OR run_state NOT IN ('review_ready','published')
           OR NEW.content_hash IS DISTINCT FROM selected_hash
           OR (publication_visibility = 'public' AND candidate_visibility <> 'public') THEN
            RAISE EXCEPTION 'source publication map must pin reviewed output from the same documentation source';
        END IF;
    WHEN 'documentation_maps' THEN
        IF NEW.ingestion_run_id IS NOT NULL THEN
            SELECT asset_kind, source_id, target_id
              INTO run_asset_kind, run_source_id, run_target_id
              FROM developer_asset_ingestion_runs
             WHERE id = NEW.ingestion_run_id;
            IF run_asset_kind IS DISTINCT FROM 'documentation'
               OR run_source_id IS NULL
               OR run_target_id IS DISTINCT FROM run_source_id THEN
                RAISE EXCEPTION 'documentation map candidate must belong to its exact documentation ingestion run';
            END IF;
        END IF;
    WHEN 'documentation_collection_members' THEN
        IF NEW.documentation_document_id IS NOT NULL AND NOT EXISTS (
            SELECT 1 FROM source_publication_document_selections selection
             WHERE selection.documentation_document_id = NEW.documentation_document_id
               AND selection.decision = 'included'
        ) THEN
            RAISE EXCEPTION 'collection document member must have an included source-publication decision';
        END IF;
        IF NEW.documentation_section_id IS NOT NULL AND NOT EXISTS (
            SELECT 1
              FROM documentation_sections section
              JOIN source_publication_document_selections selection
                ON selection.documentation_document_id = section.documentation_document_id
             WHERE section.id = NEW.documentation_section_id
               AND selection.decision = 'included'
        ) THEN
            RAISE EXCEPTION 'collection section member must belong to an included source-publication document';
        END IF;
    WHEN 'api_contract_candidates' THEN
        SELECT asset_kind, target_id
          INTO run_asset_kind, run_target_id
          FROM developer_asset_ingestion_runs
         WHERE id = NEW.ingestion_run_id;
        IF run_asset_kind IS DISTINCT FROM 'contract'
           OR run_target_id IS DISTINCT FROM NEW.api_contract_id THEN
            RAISE EXCEPTION 'contract candidate must belong to an ingestion run for that exact contract';
        END IF;
    WHEN 'api_contract_revisions' THEN
        SELECT run.state INTO run_state
          FROM api_contract_candidates candidate
          JOIN developer_asset_ingestion_runs run ON run.id = candidate.ingestion_run_id
         WHERE candidate.id = NEW.api_contract_candidate_id;
        IF run_state NOT IN ('review_ready','published') THEN
            RAISE EXCEPTION 'contract revision requires a review-ready exact candidate';
        END IF;
    WHEN 'api_contract_revision_source_publications' THEN
        SELECT run.source_id, publication.source_id, publication.content_hash,
               revision.visibility, publication.visibility::text
          INTO run_source_id, publication_source_id, selected_hash,
               candidate_visibility, publication_visibility
          FROM api_contract_revisions revision
          JOIN api_contract_candidates candidate
            ON candidate.id = revision.api_contract_candidate_id
          JOIN developer_asset_ingestion_runs run
            ON run.id = candidate.ingestion_run_id
          JOIN source_publications publication
            ON publication.id = NEW.source_publication_id
         WHERE revision.id = NEW.api_contract_revision_id;
        IF run_source_id IS DISTINCT FROM publication_source_id
           OR NEW.api_contract_candidate_id IS DISTINCT FROM (
                SELECT api_contract_candidate_id FROM api_contract_revisions
                 WHERE id = NEW.api_contract_revision_id
           )
           OR NEW.content_hash IS DISTINCT FROM selected_hash
           OR (candidate_visibility = 'public' AND publication_visibility <> 'public') THEN
            RAISE EXCEPTION 'contract revision source evidence must match its exact candidate source publication';
        END IF;
    WHEN 'sdk_content_candidates' THEN
        SELECT asset_kind, target_id
          INTO run_asset_kind, run_target_id
          FROM developer_asset_ingestion_runs
         WHERE id = NEW.ingestion_run_id;
        IF run_asset_kind IS DISTINCT FROM 'sdk'
           OR run_target_id IS DISTINCT FROM NEW.sdk_release_id THEN
            RAISE EXCEPTION 'SDK content candidate must belong to an ingestion run for that exact release';
        END IF;
    WHEN 'sdk_content_publications' THEN
        SELECT run.state INTO run_state
          FROM sdk_content_candidates candidate
          JOIN developer_asset_ingestion_runs run ON run.id = candidate.ingestion_run_id
         WHERE candidate.id = NEW.sdk_content_candidate_id;
        IF run_state NOT IN ('review_ready','published') THEN
            RAISE EXCEPTION 'SDK content publication requires a review-ready exact candidate';
        END IF;
    WHEN 'sdk_content_publication_file_selections' THEN
        SELECT content_hash INTO selected_hash
          FROM sdk_publication_files WHERE id = NEW.sdk_publication_file_id;
        IF NEW.content_hash IS DISTINCT FROM selected_hash THEN
            RAISE EXCEPTION 'SDK publication file selection hash does not match its candidate file';
        END IF;
    WHEN 'sdk_content_publication_sample_selections' THEN
        SELECT sample.content_hash, sample.validation_status
          INTO selected_hash, selected_validation_status
          FROM sdk_code_samples sample WHERE sample.id = NEW.sdk_code_sample_id;
        IF NEW.content_hash IS DISTINCT FROM selected_hash
           OR (NEW.decision = 'approved' AND selected_validation_status = 'unvalidated') THEN
            RAISE EXCEPTION 'approved SDK sample must pin validated candidate evidence';
        END IF;
    WHEN 'sdk_content_publication_maps' THEN
        SELECT content_hash INTO selected_hash
          FROM sdk_maps WHERE id = NEW.sdk_map_id;
        IF NEW.content_hash IS DISTINCT FROM selected_hash THEN
            RAISE EXCEPTION 'SDK publication map hash does not match its candidate map';
        END IF;
    WHEN 'sdk_sample_api_references' THEN
        IF NEW.api_contract_revision_id IS NOT NULL AND NOT EXISTS (
            SELECT 1
              FROM api_contract_revisions revision
              JOIN api_contract_bindings binding
                ON binding.api_contract_id = revision.api_contract_id
             WHERE revision.id = NEW.api_contract_revision_id
               AND binding.integration_id = NEW.integration_id
               AND binding.lifecycle = 'attached'
        ) THEN
            RAISE EXCEPTION 'SDK sample contract reference must target a contract attached to that API';
        END IF;
        IF NEW.api_sdk_binding_id IS NOT NULL THEN
            SELECT publication.sdk_content_candidate_id
              INTO published_candidate_id
              FROM api_sdk_bindings binding
              JOIN sdk_content_publications publication
                ON publication.id = binding.sdk_content_publication_id
             WHERE binding.id = NEW.api_sdk_binding_id;
            IF published_candidate_id IS DISTINCT FROM NEW.sdk_content_candidate_id THEN
                RAISE EXCEPTION 'SDK sample API reference must use a binding for the sample candidate publication';
            END IF;
        END IF;
    ELSE
        RAISE EXCEPTION 'unsupported developer-asset candidate trigger table %', TG_TABLE_NAME;
    END CASE;
    RETURN NEW;
END;
$$;

DO $$
DECLARE
    relation_name text;
BEGIN
    FOREACH relation_name IN ARRAY ARRAY[
        'documentation_documents',
        'source_publication_document_selections',
        'source_publication_documentation_maps',
        'documentation_maps',
        'documentation_collection_members',
        'api_contract_candidates',
        'api_contract_revisions',
        'api_contract_revision_source_publications',
        'sdk_content_candidates',
        'sdk_content_publications',
        'sdk_content_publication_file_selections',
        'sdk_content_publication_sample_selections',
        'sdk_content_publication_maps',
        'sdk_sample_api_references'
    ]
    LOOP
        EXECUTE format(
            'CREATE TRIGGER %I BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_candidate_membership()',
            'developer_asset_candidate_guard',
            relation_name
        );
    END LOOP;
END $$;

CREATE FUNCTION guard_api_sdk_binding_consistency()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    assertion_contract_revision_id uuid;
    assertion_coverage text;
    assertion_assurance text;
BEGIN
    IF NEW.api_contract_revision_id IS NOT NULL AND NOT EXISTS (
        SELECT 1
          FROM api_contract_revisions revision
          JOIN api_contract_bindings contract_binding
            ON contract_binding.api_contract_id = revision.api_contract_id
         WHERE revision.id = NEW.api_contract_revision_id
           AND contract_binding.integration_id = NEW.integration_id
           AND contract_binding.lifecycle = 'attached'
    ) THEN
        RAISE EXCEPTION 'SDK binding contract revision must belong to a contract attached to that API'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.compatibility_assertion_id IS NOT NULL THEN
        SELECT api_contract_revision_id, coverage, assurance
          INTO assertion_contract_revision_id, assertion_coverage, assertion_assurance
          FROM sdk_compatibility_assertions
         WHERE id = NEW.compatibility_assertion_id;
        IF NEW.api_contract_revision_id IS DISTINCT FROM assertion_contract_revision_id
           OR NEW.coverage IS DISTINCT FROM assertion_coverage
           OR NEW.assurance IS DISTINCT FROM assertion_assurance THEN
            RAISE EXCEPTION 'SDK binding must copy its exact compatibility assertion scope and assurance'
                USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER api_sdk_bindings_consistency_guard_trigger
BEFORE INSERT OR UPDATE ON api_sdk_bindings
FOR EACH ROW EXECUTE FUNCTION guard_api_sdk_binding_consistency();

-- Every projection names an immutable, deployment-scoped publication. Keep
-- the polymorphic key bounded in one audited function rather than accepting
-- arbitrary UUIDs that only application code understands.
CREATE FUNCTION developer_asset_publication_visibility(
    value_deployment_id uuid,
    value_publication_kind text,
    value_publication_id uuid
)
RETURNS text
LANGUAGE plpgsql
STABLE
AS $$
DECLARE
    value_visibility text;
BEGIN
    CASE value_publication_kind
    WHEN 'source' THEN
        SELECT visibility::text INTO value_visibility
          FROM source_publications
         WHERE id = value_publication_id AND product_id = value_deployment_id;
    WHEN 'documentation_collection' THEN
        SELECT visibility INTO value_visibility
          FROM documentation_collection_revisions
         WHERE id = value_publication_id AND deployment_id = value_deployment_id;
    WHEN 'global_documentation' THEN
        SELECT visibility INTO value_visibility
          FROM deployment_documentation_publications
         WHERE id = value_publication_id AND deployment_id = value_deployment_id;
    WHEN 'contract' THEN
        SELECT visibility INTO value_visibility
          FROM api_contract_revisions
         WHERE id = value_publication_id AND deployment_id = value_deployment_id;
    WHEN 'sdk' THEN
        SELECT visibility INTO value_visibility
          FROM sdk_content_publications
         WHERE id = value_publication_id AND deployment_id = value_deployment_id;
    WHEN 'api' THEN
        SELECT integration.visibility INTO value_visibility
          FROM api_developer_asset_publications publication
          JOIN integrations integration ON integration.id = publication.integration_id
         WHERE publication.id = value_publication_id
           AND publication.deployment_id = value_deployment_id;
    ELSE
        RAISE EXCEPTION 'unsupported developer-asset publication kind %', value_publication_kind
            USING ERRCODE = '23514';
    END CASE;
    IF value_visibility IS NULL THEN
        RAISE EXCEPTION 'developer-asset publication %/% does not exist in deployment %',
            value_publication_kind, value_publication_id, value_deployment_id
            USING ERRCODE = '23503';
    END IF;
    RETURN value_visibility;
END;
$$;

CREATE FUNCTION guard_developer_asset_search_projection()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    publication_visibility text;
    expected_asset_kind text;
BEGIN
    CASE TG_TABLE_NAME
    WHEN 'search_index_generations' THEN
        publication_visibility := developer_asset_publication_visibility(
            NEW.deployment_id, NEW.publication_kind, NEW.publication_id
        );
        expected_asset_kind := CASE NEW.publication_kind
            WHEN 'source' THEN 'documentation'
            WHEN 'documentation_collection' THEN 'documentation'
            WHEN 'global_documentation' THEN 'documentation'
            WHEN 'contract' THEN 'contract'
            WHEN 'sdk' THEN 'sdk'
            WHEN 'api' THEN 'mixed'
        END;
        IF NEW.asset_kind IS DISTINCT FROM expected_asset_kind THEN
            RAISE EXCEPTION 'search generation asset kind % does not match publication kind %',
                NEW.asset_kind, NEW.publication_kind USING ERRCODE = '23514';
        END IF;
        IF TG_OP = 'UPDATE' AND
           (NEW.deployment_id, NEW.publication_kind, NEW.publication_id,
            NEW.asset_kind, NEW.builder_version, NEW.retrieval_profile_version,
            NEW.embedding_model, NEW.embedding_dimensions)
           IS DISTINCT FROM
           (OLD.deployment_id, OLD.publication_kind, OLD.publication_id,
            OLD.asset_kind, OLD.builder_version, OLD.retrieval_profile_version,
            OLD.embedding_model, OLD.embedding_dimensions) THEN
            RAISE EXCEPTION 'search generation publication and processor identity are immutable'
                USING ERRCODE = '23514';
        END IF;
    WHEN 'knowledge_units' THEN
        publication_visibility := developer_asset_publication_visibility(
            NEW.deployment_id, NEW.source_publication_kind, NEW.source_publication_id
        );
        IF NEW.visibility = 'public' AND publication_visibility <> 'public' THEN
            RAISE EXCEPTION 'public knowledge unit cannot widen a private publication'
                USING ERRCODE = '23514';
        END IF;
    WHEN 'retrieval_query_trace_results' THEN
        publication_visibility := developer_asset_publication_visibility(
            NEW.deployment_id, NEW.source_publication_kind, NEW.source_publication_id
        );
    ELSE
        RAISE EXCEPTION 'unsupported developer-asset search projection table %', TG_TABLE_NAME;
    END CASE;
    RETURN NEW;
END;
$$;

CREATE TRIGGER search_index_generations_publication_guard_trigger
BEFORE INSERT OR UPDATE ON search_index_generations
FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_search_projection();
CREATE TRIGGER knowledge_units_publication_guard_trigger
BEFORE INSERT ON knowledge_units
FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_search_projection();
CREATE TRIGGER retrieval_query_trace_results_publication_guard_trigger
BEFORE INSERT ON retrieval_query_trace_results
FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_search_projection();

-- Snapshot children copy mutable draft bindings into exact immutable evidence.
-- Verify every copied selector, revision, hash, and optional SDK assertion at
-- the boundary where it stops following draft state.
CREATE FUNCTION guard_developer_asset_publication_member()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    binding_selector jsonb;
    binding_selector_hash text;
    binding_follow_latest boolean;
    binding_pinned_revision_id uuid;
    binding_lifecycle text;
    binding_primary boolean;
    binding_content_publication_id uuid;
    binding_assertion_id uuid;
    selected_content_hash text;
    latest_revision_id uuid;
    expected_content_hash text;
BEGIN
    CASE TG_TABLE_NAME
    WHEN 'deployment_documentation_publication_members' THEN
        SELECT content_hash INTO selected_content_hash
          FROM documentation_collection_revisions
         WHERE id = NEW.documentation_collection_revision_id;
        IF NEW.content_hash IS DISTINCT FROM selected_content_hash THEN
            RAISE EXCEPTION 'global documentation member must pin the exact collection revision hash'
                USING ERRCODE = '23514';
        END IF;
    WHEN 'api_publication_documentation_assets' THEN
        SELECT binding.selector, binding.follow_latest, binding.pinned_revision_id,
               binding.lifecycle, revision.content_hash
          INTO binding_selector, binding_follow_latest, binding_pinned_revision_id,
               binding_lifecycle, selected_content_hash
          FROM api_documentation_bindings binding
          JOIN documentation_collection_revisions revision
            ON revision.id = NEW.documentation_collection_revision_id
         WHERE binding.id = NEW.api_documentation_binding_id;
        IF binding_follow_latest THEN
            SELECT id INTO latest_revision_id
              FROM documentation_collection_revisions
             WHERE documentation_collection_id = NEW.documentation_collection_id
             ORDER BY revision DESC LIMIT 1;
        ELSE
            latest_revision_id := binding_pinned_revision_id;
        END IF;
        IF binding_lifecycle IS DISTINCT FROM 'attached'
           OR NEW.documentation_collection_revision_id IS DISTINCT FROM latest_revision_id
           OR NEW.selector IS DISTINCT FROM binding_selector
           OR NEW.selector_hash IS DISTINCT FROM 'sha256:' || encode(
                digest(convert_to(NEW.selector::text, 'UTF8'), 'sha256'), 'hex'
           )
           OR NEW.content_hash IS DISTINCT FROM selected_content_hash THEN
            RAISE EXCEPTION 'API documentation snapshot must resolve the exact active binding revision and selector'
                USING ERRCODE = '23514';
        END IF;
    WHEN 'api_publication_contract_assets' THEN
        SELECT binding.follow_latest, binding.pinned_revision_id, binding.lifecycle,
               binding.primary_contract, revision.content_hash
          INTO binding_follow_latest, binding_pinned_revision_id, binding_lifecycle,
               binding_primary, selected_content_hash
          FROM api_contract_bindings binding
          JOIN api_contract_revisions revision ON revision.id = NEW.api_contract_revision_id
         WHERE binding.id = NEW.api_contract_binding_id;
        IF binding_follow_latest THEN
            SELECT id INTO latest_revision_id
              FROM api_contract_revisions
             WHERE api_contract_id = NEW.api_contract_id
             ORDER BY revision DESC LIMIT 1;
        ELSE
            latest_revision_id := binding_pinned_revision_id;
        END IF;
        IF binding_lifecycle IS DISTINCT FROM 'attached'
           OR NEW.api_contract_revision_id IS DISTINCT FROM latest_revision_id
           OR NEW.primary_contract IS DISTINCT FROM binding_primary
           OR NEW.content_hash IS DISTINCT FROM selected_content_hash THEN
            RAISE EXCEPTION 'API contract snapshot must resolve the exact active binding revision'
                USING ERRCODE = '23514';
        END IF;
    WHEN 'api_publication_sdk_assets' THEN
        SELECT binding.sdk_content_publication_id, binding.compatibility_assertion_id,
               binding.selector, binding.selector_hash,
               coalesce(publication.content_hash, release.release_hash)
          INTO binding_content_publication_id, binding_assertion_id,
               binding_selector, binding_selector_hash, expected_content_hash
          FROM api_sdk_bindings binding
          JOIN sdk_releases release ON release.id = binding.sdk_release_id
          LEFT JOIN sdk_content_publications publication
            ON publication.id = binding.sdk_content_publication_id
         WHERE binding.id = NEW.api_sdk_binding_id;
        IF NEW.sdk_content_publication_id IS DISTINCT FROM binding_content_publication_id
           OR NEW.compatibility_assertion_id IS DISTINCT FROM binding_assertion_id
           OR NEW.selector IS DISTINCT FROM binding_selector
           OR NEW.selector_hash IS DISTINCT FROM binding_selector_hash
           OR NEW.content_hash IS DISTINCT FROM expected_content_hash THEN
            RAISE EXCEPTION 'API SDK snapshot must copy the exact binding release, content, assertion, and selector'
                USING ERRCODE = '23514';
        END IF;
    ELSE
        RAISE EXCEPTION 'unsupported developer-asset publication member table %', TG_TABLE_NAME;
    END CASE;
    RETURN NEW;
END;
$$;

DO $$
DECLARE
    relation_name text;
BEGIN
    FOREACH relation_name IN ARRAY ARRAY[
        'deployment_documentation_publication_members',
        'api_publication_documentation_assets',
        'api_publication_contract_assets',
        'api_publication_sdk_assets'
    ]
    LOOP
        EXECUTE format(
            'CREATE TRIGGER %I BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_publication_member()',
            'developer_asset_exact_member_guard', relation_name
        );
    END LOOP;
END $$;

-- Immutable publication evidence rejects accidental updates and deletes. Root
-- catalogue records, draft bindings, ingestion state, index generations, and
-- active-head pointers remain intentionally mutable.
CREATE FUNCTION reject_developer_asset_immutable_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% is immutable; create a new revision or publication', TG_TABLE_NAME
        USING ERRCODE = '55000';
END;
$$;

CREATE FUNCTION guard_api_developer_asset_publication()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    revision_state text;
    revision_published_at timestamptz;
    api_visibility text;
    global_visibility text;
BEGIN
    SELECT revision.state, revision.published_at, integration.visibility
      INTO revision_state, revision_published_at, api_visibility
      FROM integration_revisions revision
      JOIN integrations integration ON integration.id = revision.integration_id
     WHERE revision.id = NEW.integration_revision_id
       AND revision.integration_id = NEW.integration_id;
    IF revision_state IS DISTINCT FROM 'published' OR revision_published_at IS NULL THEN
        RAISE EXCEPTION 'developer-asset publication requires an exact published API revision'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.published_at IS DISTINCT FROM revision_published_at THEN
        RAISE EXCEPTION 'developer-asset publication timestamp must match the API revision'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.deployment_documentation_publication_id IS NOT NULL THEN
        SELECT visibility INTO global_visibility
          FROM deployment_documentation_publications
         WHERE id = NEW.deployment_documentation_publication_id
           AND deployment_id = NEW.deployment_id;
        IF api_visibility = 'public' AND global_visibility IS DISTINCT FROM 'public' THEN
            RAISE EXCEPTION 'public API publication cannot select private global documentation'
                USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER api_developer_asset_publication_guard_trigger
BEFORE INSERT ON api_developer_asset_publications
FOR EACH ROW EXECUTE FUNCTION guard_api_developer_asset_publication();

CREATE FUNCTION guard_developer_asset_visibility()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    first_visibility text;
    second_visibility text;
    third_visibility text;
    fourth_visibility text;
    effective_visibility text;
BEGIN
    CASE TG_TABLE_NAME
    WHEN 'documentation_documents' THEN
        SELECT source.visibility::text INTO first_visibility
          FROM developer_asset_ingestion_runs run
          JOIN sources source ON source.id = run.source_id
         WHERE run.id = NEW.ingestion_run_id
           AND run.asset_kind = 'documentation';
        IF NEW.visibility = 'public' AND first_visibility IS DISTINCT FROM 'public' THEN
            RAISE EXCEPTION 'public documentation candidate cannot widen its private source';
        END IF;
    WHEN 'documentation_collection_revisions' THEN
        SELECT visibility INTO first_visibility
          FROM documentation_collections
         WHERE id = NEW.documentation_collection_id;
        IF NEW.visibility = 'public' AND first_visibility IS DISTINCT FROM 'public' THEN
            RAISE EXCEPTION 'public collection revision cannot widen a private collection';
        END IF;
    WHEN 'documentation_collection_members' THEN
        SELECT visibility INTO first_visibility
          FROM documentation_collection_revisions
         WHERE id = NEW.documentation_collection_revision_id;
        IF NEW.source_publication_id IS NOT NULL THEN
            SELECT visibility::text INTO second_visibility
              FROM source_publications WHERE id = NEW.source_publication_id;
        ELSIF NEW.documentation_document_id IS NOT NULL THEN
            SELECT visibility INTO second_visibility
              FROM documentation_documents WHERE id = NEW.documentation_document_id;
        ELSE
            SELECT document.visibility INTO second_visibility
              FROM documentation_sections section
              JOIN documentation_documents document
                ON document.id = section.documentation_document_id
             WHERE section.id = NEW.documentation_section_id;
        END IF;
        IF first_visibility = 'public' AND second_visibility IS DISTINCT FROM 'public' THEN
            RAISE EXCEPTION 'public collection revision cannot select private documentation';
        END IF;
    WHEN 'documentation_maps' THEN
        IF NEW.ingestion_run_id IS NOT NULL THEN
            SELECT source.visibility::text INTO first_visibility
              FROM developer_asset_ingestion_runs run
              JOIN sources source ON source.id = run.source_id
             WHERE run.id = NEW.ingestion_run_id
               AND run.asset_kind = 'documentation';
        ELSE
            SELECT visibility INTO first_visibility
              FROM documentation_collection_revisions
             WHERE id = NEW.documentation_collection_revision_id;
        END IF;
        IF NEW.visibility IS DISTINCT FROM first_visibility THEN
            RAISE EXCEPTION 'documentation map visibility must match its exact candidate or collection revision';
        END IF;
    WHEN 'deployment_documentation_publication_members' THEN
        SELECT visibility INTO first_visibility
          FROM deployment_documentation_publications
         WHERE id = NEW.deployment_documentation_publication_id;
        SELECT visibility INTO second_visibility
          FROM documentation_collection_revisions
         WHERE id = NEW.documentation_collection_revision_id;
        IF NEW.visibility IS DISTINCT FROM second_visibility THEN
            RAISE EXCEPTION 'global documentation member visibility must match its collection revision';
        END IF;
        IF first_visibility = 'public' AND second_visibility IS DISTINCT FROM 'public' THEN
            RAISE EXCEPTION 'public global documentation cannot select a private collection revision';
        END IF;
    WHEN 'api_contract_candidates' THEN
        SELECT contract.visibility, source.visibility::text
          INTO first_visibility, second_visibility
          FROM api_contracts contract
          JOIN developer_asset_ingestion_runs run ON run.id = NEW.ingestion_run_id
          JOIN sources source ON source.id = run.source_id
         WHERE contract.id = NEW.api_contract_id;
        IF NEW.visibility = 'public'
           AND (first_visibility IS DISTINCT FROM 'public' OR second_visibility IS DISTINCT FROM 'public') THEN
            RAISE EXCEPTION 'public contract candidate cannot widen private contract evidence';
        END IF;
    WHEN 'api_contract_revisions' THEN
        SELECT visibility, content_hash
          INTO first_visibility, effective_visibility
          FROM api_contract_candidates
         WHERE id = NEW.api_contract_candidate_id;
        IF NEW.visibility IS DISTINCT FROM first_visibility
           OR NEW.content_hash IS DISTINCT FROM effective_visibility THEN
            RAISE EXCEPTION 'contract revision must pin the exact candidate visibility and hash';
        END IF;
    WHEN 'sdk_releases' THEN
        SELECT visibility INTO first_visibility
          FROM sdk_packages WHERE id = NEW.sdk_package_id;
        IF NEW.visibility = 'public' AND first_visibility IS DISTINCT FROM 'public' THEN
            RAISE EXCEPTION 'public SDK release cannot widen a private SDK package';
        END IF;
    WHEN 'sdk_content_candidates' THEN
        SELECT release.visibility, package.visibility
          INTO first_visibility, second_visibility
          FROM sdk_releases release
          JOIN sdk_packages package ON package.id = release.sdk_package_id
         WHERE release.id = NEW.sdk_release_id;
        IF NEW.visibility = 'public'
           AND (first_visibility IS DISTINCT FROM 'public' OR second_visibility IS DISTINCT FROM 'public') THEN
            RAISE EXCEPTION 'public SDK content cannot widen private release evidence';
        END IF;
    WHEN 'sdk_content_publications' THEN
        SELECT visibility, content_hash
          INTO first_visibility, effective_visibility
          FROM sdk_content_candidates
         WHERE id = NEW.sdk_content_candidate_id;
        IF NEW.visibility IS DISTINCT FROM first_visibility
           OR NEW.content_hash IS DISTINCT FROM effective_visibility THEN
            RAISE EXCEPTION 'SDK content publication must pin the exact candidate visibility and hash';
        END IF;
    WHEN 'sdk_code_samples' THEN
        SELECT visibility INTO first_visibility
          FROM sdk_content_candidates
         WHERE id = NEW.sdk_content_candidate_id;
        IF NEW.visibility = 'public' AND first_visibility IS DISTINCT FROM 'public' THEN
            RAISE EXCEPTION 'public SDK sample cannot widen a private content candidate';
        END IF;
    WHEN 'api_documentation_bindings' THEN
        SELECT visibility INTO first_visibility
          FROM documentation_collections
         WHERE id = NEW.documentation_collection_id;
        IF NEW.pinned_revision_id IS NOT NULL THEN
            SELECT visibility INTO second_visibility
              FROM documentation_collection_revisions
             WHERE id = NEW.pinned_revision_id;
        ELSE
            second_visibility := 'public';
        END IF;
        IF NEW.visibility = 'public'
           AND (first_visibility IS DISTINCT FROM 'public' OR second_visibility IS DISTINCT FROM 'public') THEN
            RAISE EXCEPTION 'public API documentation binding cannot widen private documentation';
        END IF;
    WHEN 'api_contract_bindings' THEN
        SELECT visibility INTO first_visibility
          FROM api_contracts WHERE id = NEW.api_contract_id;
        IF NEW.pinned_revision_id IS NOT NULL THEN
            SELECT visibility INTO second_visibility
              FROM api_contract_revisions WHERE id = NEW.pinned_revision_id;
        ELSE
            second_visibility := 'public';
        END IF;
        IF NEW.visibility = 'public'
           AND (first_visibility IS DISTINCT FROM 'public' OR second_visibility IS DISTINCT FROM 'public') THEN
            RAISE EXCEPTION 'public API contract binding cannot widen a private contract';
        END IF;
    WHEN 'api_sdk_bindings' THEN
        SELECT package.visibility, release.visibility
          INTO first_visibility, second_visibility
          FROM sdk_packages package
          JOIN sdk_releases release ON release.sdk_package_id = package.id
         WHERE package.id = NEW.sdk_package_id AND release.id = NEW.sdk_release_id;
        IF NEW.sdk_content_publication_id IS NOT NULL THEN
            SELECT visibility INTO third_visibility
              FROM sdk_content_publications WHERE id = NEW.sdk_content_publication_id;
        ELSE
            third_visibility := 'public';
        END IF;
        IF NEW.visibility = 'public' AND (
            first_visibility IS DISTINCT FROM 'public'
            OR second_visibility IS DISTINCT FROM 'public'
            OR third_visibility IS DISTINCT FROM 'public'
        ) THEN
            RAISE EXCEPTION 'public API SDK binding cannot widen private SDK evidence';
        END IF;
    WHEN 'api_publication_documentation_assets' THEN
        SELECT integration.visibility, binding.visibility, revision.visibility
          INTO first_visibility, second_visibility, third_visibility
          FROM api_developer_asset_publications publication
          JOIN integrations integration ON integration.id = publication.integration_id
          JOIN api_documentation_bindings binding ON binding.id = NEW.api_documentation_binding_id
          JOIN documentation_collection_revisions revision
            ON revision.id = NEW.documentation_collection_revision_id
         WHERE publication.id = NEW.api_developer_asset_publication_id;
        effective_visibility := CASE
            WHEN second_visibility = 'public' AND third_visibility = 'public' THEN 'public'
            ELSE 'private'
        END;
        IF NEW.visibility IS DISTINCT FROM effective_visibility
           OR (first_visibility = 'public' AND effective_visibility <> 'public') THEN
            RAISE EXCEPTION 'API documentation snapshot has an invalid effective visibility';
        END IF;
    WHEN 'api_publication_contract_assets' THEN
        SELECT integration.visibility, binding.visibility, revision.visibility
          INTO first_visibility, second_visibility, third_visibility
          FROM api_developer_asset_publications publication
          JOIN integrations integration ON integration.id = publication.integration_id
          JOIN api_contract_bindings binding ON binding.id = NEW.api_contract_binding_id
          JOIN api_contract_revisions revision ON revision.id = NEW.api_contract_revision_id
         WHERE publication.id = NEW.api_developer_asset_publication_id;
        effective_visibility := CASE
            WHEN second_visibility = 'public' AND third_visibility = 'public' THEN 'public'
            ELSE 'private'
        END;
        IF NEW.visibility IS DISTINCT FROM effective_visibility
           OR (first_visibility = 'public' AND effective_visibility <> 'public') THEN
            RAISE EXCEPTION 'API contract snapshot has an invalid effective visibility';
        END IF;
    WHEN 'api_publication_sdk_assets' THEN
        SELECT integration.visibility, binding.visibility, package.visibility, release.visibility
          INTO first_visibility, second_visibility, third_visibility, fourth_visibility
          FROM api_developer_asset_publications publication
          JOIN integrations integration ON integration.id = publication.integration_id
          JOIN api_sdk_bindings binding ON binding.id = NEW.api_sdk_binding_id
          JOIN sdk_packages package ON package.id = NEW.sdk_package_id
          JOIN sdk_releases release ON release.id = NEW.sdk_release_id
         WHERE publication.id = NEW.api_developer_asset_publication_id;
        effective_visibility := CASE
            WHEN second_visibility = 'public' AND third_visibility = 'public'
                 AND fourth_visibility = 'public'
                 AND (
                    NEW.sdk_content_publication_id IS NULL
                    OR (SELECT visibility = 'public' FROM sdk_content_publications
                         WHERE id = NEW.sdk_content_publication_id)
                 )
            THEN 'public'
            ELSE 'private'
        END;
        IF NEW.visibility IS DISTINCT FROM effective_visibility
           OR (first_visibility = 'public' AND effective_visibility <> 'public') THEN
            RAISE EXCEPTION 'API SDK snapshot has an invalid effective visibility';
        END IF;
    ELSE
        RAISE EXCEPTION 'unsupported developer-asset visibility trigger table %', TG_TABLE_NAME;
    END CASE;
    RETURN NEW;
END;
$$;

DO $$
DECLARE
    relation_name text;
BEGIN
    FOREACH relation_name IN ARRAY ARRAY[
        'documentation_documents',
        'documentation_collection_revisions',
        'documentation_collection_members',
        'documentation_maps',
        'deployment_documentation_publication_members',
        'api_contract_candidates',
        'api_contract_revisions',
        'sdk_releases',
        'sdk_content_candidates',
        'sdk_content_publications',
        'sdk_code_samples',
        'api_documentation_bindings',
        'api_contract_bindings',
        'api_sdk_bindings',
        'api_publication_documentation_assets',
        'api_publication_contract_assets',
        'api_publication_sdk_assets'
    ]
    LOOP
        EXECUTE format(
            'CREATE TRIGGER %I BEFORE INSERT OR UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_visibility()',
            'developer_asset_visibility_guard',
            relation_name
        );
    END LOOP;
END $$;

DO $$
DECLARE
    relation_name text;
BEGIN
    FOREACH relation_name IN ARRAY ARRAY[
        'developer_asset_raw_blobs',
        'documentation_documents',
        'documentation_sections',
        'documentation_collection_revisions',
        'documentation_collection_members',
        'source_publication_document_selections',
        'documentation_maps',
        'source_publication_documentation_maps',
        'deployment_documentation_publications',
        'deployment_documentation_publication_members',
        'api_contract_candidates',
        'api_contract_revisions',
        'api_contract_revision_source_publications',
        'api_contract_operations',
        'api_contract_schemas',
        'api_contract_examples',
        'api_contract_maps',
        'sdk_releases',
        'sdk_release_lifecycle_events',
        'sdk_content_candidates',
        'sdk_content_publications',
        'sdk_publication_files',
        'sdk_sections',
        'sdk_symbols',
        'sdk_code_samples',
        'sdk_maps',
        'sdk_content_publication_file_selections',
        'sdk_content_publication_sample_selections',
        'sdk_content_publication_maps',
        'sdk_compatibility_assertions',
        'sdk_sample_api_references',
        'api_developer_asset_publications',
        'api_publication_documentation_assets',
        'api_publication_contract_assets',
        'api_publication_sdk_assets',
        'retrieval_evaluation_set_revisions',
        'retrieval_evaluation_cases',
        'retrieval_evaluation_case_results',
        'legacy_sdk_reference_migration_ledger'
    ]
    LOOP
        EXECUTE format(
            'CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION reject_developer_asset_immutable_mutation()',
            'developer_asset_immutable_guard',
            relation_name
        );
    END LOOP;
END $$;

-- Query traces are append-only while retained, but unlike publication
-- evidence they may be deleted by the configured retention policy.
CREATE TRIGGER retrieval_query_traces_no_update_trigger
BEFORE UPDATE ON retrieval_query_traces
FOR EACH ROW EXECUTE FUNCTION reject_developer_asset_immutable_mutation();
CREATE TRIGGER retrieval_query_trace_results_no_update_trigger
BEFORE UPDATE ON retrieval_query_trace_results
FOR EACH ROW EXECUTE FUNCTION reject_developer_asset_immutable_mutation();
CREATE TRIGGER knowledge_units_no_update_trigger
BEFORE UPDATE ON knowledge_units
FOR EACH ROW EXECUTE FUNCTION reject_developer_asset_immutable_mutation();
