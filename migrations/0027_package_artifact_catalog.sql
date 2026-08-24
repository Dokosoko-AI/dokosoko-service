-- SDK/package catalogue metadata only. Registry delivery remains external;
-- these tables intentionally contain no package payload, proxy, credential,
-- entitlement hook, or download hook configuration.

CREATE TABLE package_artifacts (
    id uuid PRIMARY KEY,
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    organisation_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    ecosystem text NOT NULL,
    coordinate text NOT NULL,
    purl text NOT NULL CHECK (purl ~ '^pkg:'),
    registry_url text NOT NULL,
    source_url text NOT NULL DEFAULT '',
    language text NOT NULL DEFAULT '',
    platform text NOT NULL DEFAULT '',
    visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','public')),
    lifecycle text NOT NULL DEFAULT 'draft' CHECK (lifecycle IN ('draft','active','deprecated','retired')),
    replacement_package_artifact_id uuid REFERENCES package_artifacts(id) ON DELETE RESTRICT,
    deprecation_message text NOT NULL DEFAULT '',
    sunset_at timestamptz,
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (deployment_id, ecosystem, coordinate),
    UNIQUE (id, deployment_id),
    CHECK (replacement_package_artifact_id IS NULL OR replacement_package_artifact_id <> id),
    CHECK ((lifecycle IN ('deprecated','retired')) OR (replacement_package_artifact_id IS NULL AND deprecation_message = '' AND sunset_at IS NULL))
);
CREATE INDEX package_artifacts_deployment_lifecycle_idx
    ON package_artifacts(deployment_id, lifecycle, ecosystem, coordinate);

CREATE TABLE package_releases (
    id uuid PRIMARY KEY,
    package_artifact_id uuid NOT NULL REFERENCES package_artifacts(id) ON DELETE RESTRICT,
    artifact_name text NOT NULL,
    ecosystem text NOT NULL,
    coordinate text NOT NULL,
    version text NOT NULL,
    purl text NOT NULL CHECK (purl ~ '^pkg:'),
    registry_url text NOT NULL,
    source_url text NOT NULL DEFAULT '',
    language text NOT NULL DEFAULT '',
    platform text NOT NULL DEFAULT '',
    install_command text NOT NULL,
    digest text NOT NULL CHECK (digest ~ '^(sha256|sha384|sha512):[0-9a-f]+$'),
    provenance_url text NOT NULL DEFAULT '',
    sbom_url text NOT NULL DEFAULT '',
    visibility text NOT NULL CHECK (visibility IN ('private','public')),
    content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
    published_by text NOT NULL DEFAULT '',
    published_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (package_artifact_id, version),
    UNIQUE (package_artifact_id, content_hash),
    UNIQUE (id, package_artifact_id)
);
CREATE INDEX package_releases_artifact_published_idx
    ON package_releases(package_artifact_id, published_at DESC);

ALTER TABLE integrations
    ADD CONSTRAINT integrations_id_deployment_package_key UNIQUE (id, deployment_id);

CREATE TABLE integration_package_bindings (
    id uuid PRIMARY KEY,
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    integration_id uuid NOT NULL,
    package_artifact_id uuid NOT NULL,
    package_release_id uuid NOT NULL,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (integration_id, package_artifact_id),
    FOREIGN KEY (integration_id, deployment_id)
        REFERENCES integrations(id, deployment_id) ON DELETE CASCADE,
    FOREIGN KEY (package_artifact_id, deployment_id)
        REFERENCES package_artifacts(id, deployment_id) ON DELETE RESTRICT,
    FOREIGN KEY (package_release_id, package_artifact_id)
        REFERENCES package_releases(id, package_artifact_id) ON DELETE RESTRICT
);
CREATE INDEX integration_package_bindings_release_idx
    ON integration_package_bindings(package_release_id, integration_id);
