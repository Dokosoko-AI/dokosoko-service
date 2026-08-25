-- API-owned HTTP tools reference a logical runtime service and keep only a
-- relative path. Releases pin immutable endpoint/auth configuration per
-- environment, while credential versions remain deliberately unpinned.

ALTER TABLE runtime_service_connections
    ADD CONSTRAINT runtime_service_connections_id_integration_unique
    UNIQUE (id, integration_id);

ALTER TABLE runtime_service_connection_revisions
    ADD CONSTRAINT runtime_service_connection_revisions_identity_unique
    UNIQUE (id, connection_id, environment_id);

ALTER TABLE tool_definitions
    ADD COLUMN runtime_service_connection_id uuid,
    ADD COLUMN http_path text NOT NULL DEFAULT '',
    ADD CONSTRAINT tool_definitions_runtime_owner_fk
        FOREIGN KEY (runtime_service_connection_id, owner_integration_id)
        REFERENCES runtime_service_connections(id, integration_id) ON DELETE RESTRICT,
    ADD CONSTRAINT tool_definitions_runtime_shape_check CHECK (
        runtime_service_connection_id IS NULL
        OR (scope = 'api' AND backend_kind = 'http' AND api_connection_id IS NULL AND http_path LIKE '/%')
    );

ALTER TABLE tool_releases
    ADD COLUMN runtime_service_connection_id uuid,
    ADD COLUMN http_path text NOT NULL DEFAULT '',
    ADD CONSTRAINT tool_releases_runtime_owner_fk
        FOREIGN KEY (runtime_service_connection_id, owner_integration_id)
        REFERENCES runtime_service_connections(id, integration_id) ON DELETE RESTRICT,
    ADD CONSTRAINT tool_releases_runtime_shape_check CHECK (
        runtime_service_connection_id IS NULL
        OR (scope = 'api' AND backend_kind = 'http' AND api_connection_id IS NULL AND http_path LIKE '/%')
    ),
    ADD CONSTRAINT tool_releases_id_runtime_connection_unique
        UNIQUE (id, runtime_service_connection_id);

ALTER TABLE tool_release_runtime_targets
    ADD COLUMN runtime_service_connection_id uuid;

UPDATE tool_release_runtime_targets target
SET runtime_service_connection_id = revision.connection_id
FROM runtime_service_connection_revisions revision
WHERE revision.id = target.connection_revision_id;

ALTER TABLE tool_release_runtime_targets
    ALTER COLUMN runtime_service_connection_id SET NOT NULL,
    ADD CONSTRAINT tool_release_runtime_targets_release_connection_fk
        FOREIGN KEY (tool_release_id, runtime_service_connection_id)
        REFERENCES tool_releases(id, runtime_service_connection_id) ON DELETE CASCADE,
    ADD CONSTRAINT tool_release_runtime_targets_revision_identity_fk
        FOREIGN KEY (connection_revision_id, runtime_service_connection_id, environment_id)
        REFERENCES runtime_service_connection_revisions(id, connection_id, environment_id) ON DELETE RESTRICT;

