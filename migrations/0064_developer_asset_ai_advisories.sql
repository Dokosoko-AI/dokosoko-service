-- AI enrichment remains advisory. This append-only table stores only a closed,
-- validated suggestion plus exact scope/evidence identities and safe hashes;
-- source text, code, system prompts, and raw provider responses are never
-- duplicated here.
CREATE TABLE developer_asset_ai_advisory_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE RESTRICT,
    prompt_key text NOT NULL CHECK (prompt_key IN (
        'documentation.map_enrichment',
        'sdk.map_enrichment',
        'sdk.applicability_suggestion',
        'sdk.sample_review'
    )),
    prompt_version text NOT NULL CHECK (btrim(prompt_version) <> ''),
    scope_kind text NOT NULL CHECK (scope_kind IN (
        'documentation_publication','sdk_content_publication','sdk_api_binding','sdk_sample'
    )),
    scope_id uuid NOT NULL,
    scope_visibility text NOT NULL CHECK (scope_visibility IN ('private','public')),
    ingestion_run_id uuid,
    source_publication_id uuid,
    sdk_package_id uuid,
    sdk_release_id uuid,
    sdk_content_candidate_id uuid,
    sdk_content_publication_id uuid,
    integration_id uuid,
    api_developer_asset_publication_id uuid,
    api_sdk_binding_id uuid,
    sdk_code_sample_id uuid,
    allowed_evidence_ids jsonb NOT NULL CHECK (
        jsonb_typeof(allowed_evidence_ids) = 'array'
        AND jsonb_array_length(allowed_evidence_ids) BETWEEN 1 AND 512
    ),
    evidence_hash text NOT NULL CHECK (evidence_hash ~ '^sha256:[0-9a-f]{64}$'),
    input_hash text NOT NULL CHECK (input_hash ~ '^sha256:[0-9a-f]{64}$'),
    -- json (rather than jsonb) preserves the exact server-canonical bytes
    -- committed by result_hash across a database round trip.
    result json NOT NULL CHECK (json_typeof(result) = 'object'),
    result_hash text NOT NULL CHECK (result_hash ~ '^sha256:[0-9a-f]{64}$'),
    created_by text NOT NULL CHECK (btrim(created_by) <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (ingestion_run_id, deployment_id)
        REFERENCES developer_asset_ingestion_runs(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (source_publication_id, deployment_id)
        REFERENCES source_publications(id, product_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_package_id, deployment_id)
        REFERENCES sdk_packages(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_release_id, sdk_package_id)
        REFERENCES sdk_releases(id, sdk_package_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_content_candidate_id, sdk_release_id)
        REFERENCES sdk_content_candidates(id, sdk_release_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_content_publication_id, sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_content_publications(id, sdk_content_candidate_id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_developer_asset_publication_id, deployment_id, integration_id)
        REFERENCES api_developer_asset_publications(id, deployment_id, integration_id) ON DELETE RESTRICT,
    FOREIGN KEY (api_sdk_binding_id, integration_id, sdk_package_id, sdk_release_id, deployment_id)
        REFERENCES api_sdk_bindings(id, integration_id, sdk_package_id, sdk_release_id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (sdk_code_sample_id, sdk_content_candidate_id, deployment_id)
        REFERENCES sdk_code_samples(id, sdk_content_candidate_id, deployment_id) ON DELETE RESTRICT,
    CHECK (
        (prompt_key = 'documentation.map_enrichment'
            AND scope_kind = 'documentation_publication' AND scope_id = source_publication_id
            AND ingestion_run_id IS NOT NULL AND source_publication_id IS NOT NULL
            AND sdk_package_id IS NULL AND sdk_release_id IS NULL
            AND sdk_content_candidate_id IS NULL AND sdk_content_publication_id IS NULL
            AND integration_id IS NULL AND api_developer_asset_publication_id IS NULL
            AND api_sdk_binding_id IS NULL AND sdk_code_sample_id IS NULL)
        OR
        (prompt_key = 'sdk.map_enrichment'
            AND scope_kind = 'sdk_content_publication' AND scope_id = sdk_content_publication_id
            AND ingestion_run_id IS NOT NULL AND source_publication_id IS NULL
            AND sdk_package_id IS NOT NULL AND sdk_release_id IS NOT NULL
            AND sdk_content_candidate_id IS NOT NULL AND sdk_content_publication_id IS NOT NULL
            AND integration_id IS NULL AND api_developer_asset_publication_id IS NULL
            AND api_sdk_binding_id IS NULL AND sdk_code_sample_id IS NULL)
        OR
        (prompt_key = 'sdk.applicability_suggestion'
            AND scope_kind = 'sdk_api_binding' AND scope_id = api_sdk_binding_id
            AND ingestion_run_id IS NOT NULL AND source_publication_id IS NULL
            AND sdk_package_id IS NOT NULL AND sdk_release_id IS NOT NULL
            AND sdk_content_candidate_id IS NOT NULL AND sdk_content_publication_id IS NOT NULL
            AND integration_id IS NOT NULL AND api_developer_asset_publication_id IS NOT NULL
            AND api_sdk_binding_id IS NOT NULL AND sdk_code_sample_id IS NULL)
        OR
        (prompt_key = 'sdk.sample_review'
            AND scope_kind = 'sdk_sample' AND scope_id = sdk_code_sample_id
            AND ingestion_run_id IS NOT NULL AND source_publication_id IS NULL
            AND sdk_package_id IS NOT NULL AND sdk_release_id IS NOT NULL
            AND sdk_content_candidate_id IS NOT NULL AND sdk_content_publication_id IS NOT NULL
            AND integration_id IS NOT NULL AND api_developer_asset_publication_id IS NOT NULL
            AND api_sdk_binding_id IS NOT NULL AND sdk_code_sample_id IS NOT NULL)
    ),
    CHECK (result_hash = 'sha256:' || encode(
        digest(convert_to(result::text, 'UTF8'), 'sha256'), 'hex'
    )),
    UNIQUE (deployment_id, prompt_key, input_hash),
    UNIQUE (id, deployment_id)
);
CREATE INDEX developer_asset_ai_advisory_scope_idx
    ON developer_asset_ai_advisory_runs(deployment_id, scope_kind, scope_id, created_at DESC);
CREATE INDEX developer_asset_ai_advisory_prompt_idx
    ON developer_asset_ai_advisory_runs(deployment_id, prompt_key, created_at DESC);

-- Recheck every caller-supplied lineage column and the effective visibility
-- at the database boundary. The service performs the same checks before any
-- provider call; this trigger prevents a future writer from bypassing them.
CREATE FUNCTION guard_developer_asset_ai_advisory_lineage()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    expected_visibility text;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM jsonb_array_elements(NEW.allowed_evidence_ids) item
        WHERE jsonb_typeof(item) <> 'string'
           OR length(btrim(item #>> '{}')) NOT BETWEEN 1 AND 200
    ) OR (
        SELECT count(*)
        FROM jsonb_array_elements_text(NEW.allowed_evidence_ids)
    ) <> (
        SELECT count(DISTINCT item)
        FROM jsonb_array_elements_text(NEW.allowed_evidence_ids) item
    ) THEN
        RAISE EXCEPTION 'developer-asset AI advisory evidence IDs are invalid'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.prompt_key = 'documentation.map_enrichment' THEN
        SELECT CASE WHEN publication.visibility::text = 'private'
                    OR map.visibility = 'private'
                    OR EXISTS (
                        SELECT 1
                        FROM source_publication_document_selections selection
                        JOIN documentation_documents document
                          ON document.id = selection.documentation_document_id
                         AND document.deployment_id = selection.deployment_id
                        WHERE selection.source_publication_id = publication.id
                          AND selection.decision = 'included'
                          AND document.visibility = 'private'
                    )
                    THEN 'private' ELSE 'public' END
          INTO expected_visibility
          FROM source_publications publication
          JOIN source_publication_documentation_maps map_link
            ON map_link.source_publication_id = publication.id
           AND map_link.deployment_id = publication.product_id
          JOIN documentation_maps map
            ON map.id = map_link.documentation_map_id
           AND map.deployment_id = publication.product_id
           AND map.ingestion_run_id = NEW.ingestion_run_id
         WHERE publication.id = NEW.source_publication_id
           AND publication.product_id = NEW.deployment_id
           AND map.content_hash = map_link.content_hash;
    ELSE
        SELECT CASE WHEN package.visibility = 'private'
                    OR release.visibility = 'private'
                    OR candidate.visibility = 'private'
                    OR publication.visibility = 'private'
                    OR EXISTS (
                        SELECT 1
                        FROM sdk_content_publication_sample_selections selection
                        JOIN sdk_code_samples sample
                          ON sample.id = selection.sdk_code_sample_id
                         AND sample.sdk_content_candidate_id = selection.sdk_content_candidate_id
                        WHERE selection.sdk_content_publication_id = publication.id
                          AND selection.decision = 'approved'
                          AND sample.visibility = 'private'
                    )
                    THEN 'private' ELSE 'public' END
          INTO expected_visibility
          FROM sdk_packages package
          JOIN sdk_releases release
            ON release.sdk_package_id = package.id
           AND release.deployment_id = package.deployment_id
          JOIN sdk_content_candidates candidate
            ON candidate.sdk_release_id = release.id
           AND candidate.deployment_id = release.deployment_id
           AND candidate.ingestion_run_id = NEW.ingestion_run_id
          JOIN sdk_content_publications publication
            ON publication.sdk_content_candidate_id = candidate.id
           AND publication.sdk_release_id = release.id
          JOIN sdk_content_publication_maps map_link
            ON map_link.sdk_content_publication_id = publication.id
           AND map_link.sdk_content_candidate_id = candidate.id
          JOIN sdk_maps map
            ON map.id = map_link.sdk_map_id
           AND map.sdk_content_candidate_id = candidate.id
           AND map.content_hash = map_link.content_hash
         WHERE package.id = NEW.sdk_package_id
           AND package.deployment_id = NEW.deployment_id
           AND release.id = NEW.sdk_release_id
           AND candidate.id = NEW.sdk_content_candidate_id
           AND publication.id = NEW.sdk_content_publication_id;

        IF FOUND AND NEW.prompt_key IN ('sdk.applicability_suggestion','sdk.sample_review') THEN
            SELECT CASE WHEN expected_visibility = 'private'
                        OR asset.visibility = 'private'
                        OR COALESCE(NULLIF(revision.snapshot->>'visibility',''),'private') = 'private'
                        OR EXISTS (
                            SELECT 1
                            FROM api_publication_contract_assets contract_asset
                            JOIN api_contract_revisions contract_revision
                              ON contract_revision.id = contract_asset.api_contract_revision_id
                             AND contract_revision.api_contract_id = contract_asset.api_contract_id
                            JOIN api_contract_candidates contract_candidate
                              ON contract_candidate.id = contract_revision.api_contract_candidate_id
                             AND contract_candidate.deployment_id = contract_revision.deployment_id
                            WHERE contract_asset.api_developer_asset_publication_id = api_publication.id
                              AND (contract_asset.visibility = 'private'
                                   OR contract_revision.visibility = 'private'
                                   OR contract_candidate.visibility = 'private')
                        )
                        OR (NEW.prompt_key = 'sdk.sample_review' AND sample.visibility = 'private')
                        THEN 'private' ELSE 'public' END
              INTO expected_visibility
              FROM api_developer_asset_publications api_publication
              JOIN integration_revisions revision
                ON revision.id = api_publication.integration_revision_id
               AND revision.integration_id = api_publication.integration_id
               AND revision.state = 'published'
               AND revision.published_at IS NOT NULL
              JOIN api_publication_sdk_assets asset
                ON asset.api_developer_asset_publication_id = api_publication.id
               AND asset.integration_id = api_publication.integration_id
               AND asset.api_sdk_binding_id = NEW.api_sdk_binding_id
               AND asset.sdk_package_id = NEW.sdk_package_id
               AND asset.sdk_release_id = NEW.sdk_release_id
               AND asset.sdk_content_publication_id = NEW.sdk_content_publication_id
              LEFT JOIN sdk_code_samples sample
                ON sample.id = NEW.sdk_code_sample_id
               AND sample.sdk_content_candidate_id = NEW.sdk_content_candidate_id
               AND sample.deployment_id = NEW.deployment_id
             WHERE api_publication.id = NEW.api_developer_asset_publication_id
               AND api_publication.deployment_id = NEW.deployment_id
               AND api_publication.integration_id = NEW.integration_id
               AND (NEW.prompt_key <> 'sdk.sample_review' OR EXISTS (
                    SELECT 1
                    FROM sdk_content_publication_sample_selections selection
                    WHERE selection.sdk_content_publication_id = NEW.sdk_content_publication_id
                      AND selection.sdk_content_candidate_id = NEW.sdk_content_candidate_id
                      AND selection.sdk_code_sample_id = NEW.sdk_code_sample_id
                      AND selection.decision = 'approved'
               ));
        END IF;
    END IF;

    IF NOT FOUND OR expected_visibility IS NULL OR expected_visibility <> NEW.scope_visibility THEN
        RAISE EXCEPTION 'developer-asset AI advisory lineage or visibility is invalid'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER developer_asset_ai_advisory_lineage_guard
BEFORE INSERT ON developer_asset_ai_advisory_runs
FOR EACH ROW EXECUTE FUNCTION guard_developer_asset_ai_advisory_lineage();

CREATE TRIGGER developer_asset_immutable_guard
BEFORE UPDATE OR DELETE ON developer_asset_ai_advisory_runs
FOR EACH ROW EXECUTE FUNCTION reject_developer_asset_immutable_mutation();
