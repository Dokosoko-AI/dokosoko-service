-- Developer-asset discovery requires a durable activation audit in addition
-- to a ready index generation. Existing immutable publications predate that
-- marker, so record a deterministic migration-owned activation event. A
-- publication without a ready current-version index remains undiscoverable.
INSERT INTO audit_events(
    event_key, organisation_id, product_id, actor_id, actor_kind, action,
    target_type, target_id, prior, current, request_id, outcome, created_at
)
SELECT
    'developer-asset-activation:global_documentation:' || publication.id::text,
    deployment.organisation_id,
    publication.deployment_id,
    publication.published_by,
    'root',
    'developer_asset.publication_activated',
    'global_documentation',
    publication.id::text,
    '{}'::jsonb,
    jsonb_build_object(
        'revision', publication.revision,
        'snapshot_hash', publication.snapshot_hash,
        'migration_backfill', true
    ),
    'migration:0063',
    'success',
    publication.published_at
FROM deployment_documentation_publications publication
JOIN deployments deployment ON deployment.id = publication.deployment_id
ON CONFLICT (event_key) DO NOTHING;

INSERT INTO audit_events(
    event_key, organisation_id, product_id, actor_id, actor_kind, action,
    target_type, target_id, prior, current, request_id, outcome, created_at
)
SELECT
    'developer-asset-activation:api:' || publication.id::text,
    deployment.organisation_id,
    publication.deployment_id,
    publication.published_by,
    'root',
    'developer_asset.publication_activated',
    'api',
    publication.id::text,
    '{}'::jsonb,
    jsonb_build_object(
        'integration_id', publication.integration_id,
        'integration_revision_id', publication.integration_revision_id,
        'snapshot_hash', publication.snapshot_hash,
        'migration_backfill', true
    ),
    'migration:0063',
    'success',
    publication.published_at
FROM api_developer_asset_publications publication
JOIN deployments deployment ON deployment.id = publication.deployment_id
ON CONFLICT (event_key) DO NOTHING;
