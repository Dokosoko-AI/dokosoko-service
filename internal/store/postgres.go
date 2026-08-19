package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	pool      *pgxpool.Pool
	publicURL string
}

func NewPostgres(pool *pgxpool.Pool, publicURL string) *Postgres {
	return &Postgres{pool: pool, publicURL: strings.TrimRight(publicURL, "/")}
}

func databaseError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23503":
			return ErrConflict
		case "22P02":
			return ErrNotFound
		}
	}
	return err
}

func scanProduct(row pgx.Row) (model.Product, error) {
	var value model.Product
	err := row.Scan(&value.ID, &value.OrganisationID, &value.Name, &value.Slug, &value.PublicMCPEnabled, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func scanOrganisation(row interface{ Scan(...any) error }) (model.Organisation, error) {
	var value model.Organisation
	err := row.Scan(&value.ID, &value.Name, &value.Slug, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) Organisations(ctx context.Context) ([]model.Organisation, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text, name, slug, revision, created_at, updated_at FROM organisations ORDER BY name`)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.Organisation, 0)
	for rows.Next() {
		value, err := scanOrganisation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) CreateOrganisation(ctx context.Context, value model.Organisation) (model.Organisation, error) {
	return scanOrganisation(p.pool.QueryRow(ctx, `INSERT INTO organisations(id, name, slug) VALUES ($1, $2, $3) RETURNING id::text, name, slug, revision, created_at, updated_at`, value.ID, value.Name, value.Slug))
}

func (p *Postgres) Products(ctx context.Context, organisationID string) ([]model.Product, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text, organisation_id::text, name, slug, public_mcp_enabled, revision, created_at, updated_at FROM products WHERE organisation_id = $1 ORDER BY name`, organisationID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.Product, 0)
	for rows.Next() {
		value, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) CreateProduct(ctx context.Context, value model.Product) (model.Product, error) {
	return scanProduct(p.pool.QueryRow(ctx, `INSERT INTO products(id, organisation_id, name, slug) VALUES ($1, $2, $3, $4) RETURNING id::text, organisation_id::text, name, slug, public_mcp_enabled, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.Name, value.Slug))
}

func scanEnvironment(row interface{ Scan(...any) error }) (model.Environment, error) {
	var value model.Environment
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Name, &value.Slug, &value.IsProduction, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) Environments(ctx context.Context, productID string) ([]model.Environment, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text, organisation_id::text, product_id::text, name, slug, is_production, revision, created_at, updated_at FROM environments WHERE product_id = $1 ORDER BY name`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.Environment, 0)
	for rows.Next() {
		value, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) CreateEnvironment(ctx context.Context, value model.Environment) (model.Environment, error) {
	return scanEnvironment(p.pool.QueryRow(ctx, `INSERT INTO environments(id, organisation_id, product_id, name, slug, is_production) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id::text, organisation_id::text, product_id::text, name, slug, is_production, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Name, value.Slug, value.IsProduction))
}

func (p *Postgres) Product(ctx context.Context, id string) (model.Product, error) {
	return scanProduct(p.pool.QueryRow(ctx, `SELECT id::text, organisation_id::text, name, slug, public_mcp_enabled, revision, created_at, updated_at FROM products WHERE id = $1`, id))
}

func (p *Postgres) UpdateProduct(ctx context.Context, value model.Product, expected int64) (model.Product, error) {
	updated, err := scanProduct(p.pool.QueryRow(ctx, `UPDATE products SET public_mcp_enabled = $2, revision = revision + 1, updated_at = now() WHERE id = $1 AND revision = $3 RETURNING id::text, organisation_id::text, name, slug, public_mcp_enabled, revision, created_at, updated_at`, value.ID, value.PublicMCPEnabled, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.Product(ctx, value.ID); lookupErr == nil {
			return model.Product{}, ErrConflict
		}
	}
	return updated, err
}

func scanSource(row interface{ Scan(...any) error }) (model.Source, error) {
	var value model.Source
	var state string
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Name, &value.Kind, &value.Location, &value.Visibility, &state, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	value.Published = state == "published"
	value.Quarantined = state == "quarantined"
	return value, databaseError(err)
}

func (p *Postgres) Sources(ctx context.Context, productID string) ([]model.Source, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at FROM sources WHERE product_id = $1 ORDER BY name`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.Source, 0)
	for rows.Next() {
		value, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) Source(ctx context.Context, productID, id string) (model.Source, error) {
	return scanSource(p.pool.QueryRow(ctx, `SELECT id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at FROM sources WHERE product_id = $1 AND id = $2`, productID, id))
}

func (p *Postgres) CreateSource(ctx context.Context, value model.Source) (model.Source, error) {
	return scanSource(p.pool.QueryRow(ctx, `INSERT INTO sources(id, organisation_id, product_id, name, kind, location, visibility, state) VALUES ($1, $2, $3, $4, $5, $6, 'private', 'draft') RETURNING id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Name, value.Kind, value.Location))
}

func (p *Postgres) UpdateSource(ctx context.Context, value model.Source, expected int64) (model.Source, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Source{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanSource(tx.QueryRow(ctx, `UPDATE sources SET visibility = $3, revision = revision + 1, updated_at = now() WHERE product_id = $1 AND id = $2 AND revision = $4 RETURNING id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at`, value.ProductID, value.ID, value.Visibility, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.Source(ctx, value.ProductID, value.ID); lookupErr == nil {
			return model.Source{}, ErrConflict
		}
	}
	if err != nil {
		return model.Source{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE knowledge_documents SET visibility = $3, revision = revision + 1, updated_at = now() WHERE product_id = $1 AND source_id = $2 AND state = 'published'`, value.ProductID, value.ID, value.Visibility); err != nil {
		return model.Source{}, err
	}
	return updated, tx.Commit(ctx)
}

func (p *Postgres) PublishSource(ctx context.Context, productID, sourceID string, expected int64) (model.Source, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Source{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanSource(tx.QueryRow(ctx, `UPDATE sources SET state = 'published', revision = revision + 1, updated_at = now() WHERE product_id = $1 AND id = $2 AND revision = $3 AND state <> 'quarantined' RETURNING id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at`, productID, sourceID, expected))
	if err != nil {
		return model.Source{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE knowledge_documents SET state = 'published', visibility = $3, revision = revision + 1, updated_at = now() WHERE product_id = $1 AND source_id = $2 AND state = 'validated'`, productID, sourceID, updated.Visibility); err != nil {
		return model.Source{}, err
	}
	return updated, tx.Commit(ctx)
}

func scanCrawlJob(row interface{ Scan(...any) error }) (model.CrawlJob, error) {
	var value model.CrawlJob
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.SourceID, &value.State, &value.Attempt, &value.DiscoveredCount, &value.FetchedCount, &value.ChangedCount, &value.ErrorCode, &value.ErrorMessage, &value.QueuedAt, &value.StartedAt, &value.FinishedAt)
	return value, databaseError(err)
}

const crawlJobSelect = `SELECT id::text, organisation_id::text, product_id::text, source_id::text, state, attempt, discovered_count, fetched_count, changed_count, coalesce(error_code, ''), coalesce(error_message, ''), queued_at, started_at, finished_at FROM crawl_jobs`

func (p *Postgres) CrawlJobs(ctx context.Context, productID, sourceID string) ([]model.CrawlJob, error) {
	rows, err := p.pool.Query(ctx, crawlJobSelect+` WHERE product_id = $1 AND source_id = $2 ORDER BY queued_at DESC LIMIT 50`, productID, sourceID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.CrawlJob, 0)
	for rows.Next() {
		value, err := scanCrawlJob(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) CreateCrawlJob(ctx context.Context, value model.CrawlJob) (model.CrawlJob, error) {
	return scanCrawlJob(p.pool.QueryRow(ctx, `INSERT INTO crawl_jobs(id, organisation_id, product_id, source_id, state) VALUES ($1, $2, $3, $4, 'queued') RETURNING id::text, organisation_id::text, product_id::text, source_id::text, state, attempt, discovered_count, fetched_count, changed_count, coalesce(error_code, ''), coalesce(error_message, ''), queued_at, started_at, finished_at`, value.ID, value.OrganisationID, value.ProductID, value.SourceID))
}

func scanPackage(row interface{ Scan(...any) error }) (model.Package, error) {
	var value model.Package
	var state string
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Name, &value.Ecosystem, &value.Version, &value.Mode, &value.Location, &value.FetchHookURL, &value.CredentialID, &value.ChecksumSHA256, &value.ExpectedSize, &value.Visibility, &state, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	value.Published = state == "published"
	return value, databaseError(err)
}

func (p *Postgres) Packages(ctx context.Context, productID string) ([]model.Package, error) {
	rows, err := p.pool.Query(ctx, packageSelect+` WHERE product_id = $1 ORDER BY name`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.Package, 0)
	for rows.Next() {
		value, err := scanPackage(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) Package(ctx context.Context, productID, id string) (model.Package, error) {
	return scanPackage(p.pool.QueryRow(ctx, packageSelect+` WHERE product_id = $1 AND id = $2`, productID, id))
}

const packageSelect = `SELECT id::text, organisation_id::text, product_id::text, name, ecosystem, version, mode::text, coalesce(upstream_url, ''), coalesce(fetch_hook_url, ''), coalesce(credential_secret_id::text, ''), coalesce(checksum_sha256, ''::bytea), coalesce(expected_size, 0), visibility::text, state::text, revision, created_at, updated_at FROM packages`

func (p *Postgres) CreatePackage(ctx context.Context, value model.Package) (model.Package, error) {
	return scanPackage(p.pool.QueryRow(ctx, `INSERT INTO packages(id, organisation_id, product_id, ecosystem, name, version, mode, visibility, state, upstream_url, fetch_hook_url, credential_secret_id, checksum_sha256, expected_size) VALUES ($1,$2,$3,$4,$5,$6,$7,'private','draft',nullif($8,''),nullif($9,''),nullif($10,'')::uuid,nullif($11,''::bytea),nullif($12,0)) RETURNING id::text, organisation_id::text, product_id::text, name, ecosystem, version, mode::text, coalesce(upstream_url, ''), coalesce(fetch_hook_url, ''), coalesce(credential_secret_id::text, ''), coalesce(checksum_sha256, ''::bytea), coalesce(expected_size, 0), visibility::text, state::text, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Ecosystem, value.Name, value.Version, value.Mode, value.Location, value.FetchHookURL, value.CredentialID, value.ChecksumSHA256, value.ExpectedSize))
}

func (p *Postgres) UpdatePackage(ctx context.Context, value model.Package, expected int64) (model.Package, error) {
	updated, err := scanPackage(p.pool.QueryRow(ctx, `UPDATE packages SET visibility = $3, revision = revision + 1, updated_at = now() WHERE product_id = $1 AND id = $2 AND revision = $4 RETURNING id::text, organisation_id::text, product_id::text, name, ecosystem, version, mode::text, coalesce(upstream_url, ''), coalesce(fetch_hook_url, ''), coalesce(credential_secret_id::text, ''), coalesce(checksum_sha256, ''::bytea), coalesce(expected_size, 0), visibility::text, state::text, revision, created_at, updated_at`, value.ProductID, value.ID, value.Visibility, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.Package(ctx, value.ProductID, value.ID); lookupErr == nil {
			return model.Package{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) PublishPackage(ctx context.Context, productID, packageID string, expected int64) (model.Package, error) {
	updated, err := scanPackage(p.pool.QueryRow(ctx, `UPDATE packages SET state = 'published', revision = revision + 1, updated_at = now() WHERE product_id = $1 AND id = $2 AND revision = $3 AND state <> 'quarantined' RETURNING id::text, organisation_id::text, product_id::text, name, ecosystem, version, mode::text, coalesce(upstream_url, ''), coalesce(fetch_hook_url, ''), coalesce(credential_secret_id::text, ''), coalesce(checksum_sha256, ''::bytea), coalesce(expected_size, 0), visibility::text, state::text, revision, created_at, updated_at`, productID, packageID, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.Package(ctx, productID, packageID); lookupErr == nil {
			return model.Package{}, ErrConflict
		}
	}
	return updated, err
}

func scanSecret(row pgx.Row) (model.Secret, error) {
	var value model.Secret
	err := row.Scan(&value.ID, &value.OrganisationID, &value.Name, &value.Purpose, &value.Ciphertext, &value.Nonce, &value.KeyVersion, &value.Fingerprint, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) CreateSecret(ctx context.Context, value model.Secret) (model.Secret, error) {
	return scanSecret(p.pool.QueryRow(ctx, `INSERT INTO secrets(id, organisation_id, name, purpose, ciphertext, nonce, key_version, fingerprint) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text, organisation_id::text, name, purpose, ciphertext, nonce, key_version, fingerprint, created_at`, value.ID, value.OrganisationID, value.Name, value.Purpose, value.Ciphertext, value.Nonce, value.KeyVersion, value.Fingerprint))
}

func (p *Postgres) Secret(ctx context.Context, organisationID, id string) (model.Secret, error) {
	return scanSecret(p.pool.QueryRow(ctx, `SELECT id::text, organisation_id::text, name, purpose, ciphertext, nonce, key_version, fingerprint, created_at FROM secrets WHERE organisation_id = $1 AND id = $2`, organisationID, id))
}

const toolSelect = `SELECT t.id::text, t.organisation_id::text, t.product_id::text, t.namespace, t.name, t.description, t.input_schema, t.output_schema, t.state::text, t.revision, coalesce(t.api_connection_id::text, ''), coalesce(c.base_url, ''), t.http_method, coalesce(c.credential_secret_id::text, ''), t.authorization_policy, t.timeout_ms, t.backend_kind, coalesce(t.mcp_connection_id::text, ''), t.upstream_tool_name, t.upstream_schema_hash, t.upstream_annotations, t.upstream_drifted, t.created_at, t.updated_at FROM tool_definitions t LEFT JOIN api_connections c ON c.id = t.api_connection_id`

func scanTool(row interface{ Scan(...any) error }) (model.Tool, error) {
	var value model.Tool
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Namespace, &value.Name, &value.Description, &value.InputSchema, &value.OutputSchema, &value.State, &value.Revision, &value.APIConnectionID, &value.BaseURL, &value.HTTPMethod, &value.CredentialID, &value.AuthorizationPolicy, &value.TimeoutMS, &value.BackendKind, &value.MCPConnectionID, &value.UpstreamToolName, &value.UpstreamSchemaHash, &value.UpstreamAnnotations, &value.UpstreamDrifted, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) Tools(ctx context.Context, productID string, publishedOnly bool) ([]model.Tool, error) {
	query := toolSelect + ` WHERE t.product_id = $1`
	if publishedOnly {
		query += ` AND t.state = 'published'`
	}
	query += ` ORDER BY t.namespace, t.name`
	rows, err := p.pool.Query(ctx, query, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.Tool, 0)
	for rows.Next() {
		value, err := scanTool(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) Tool(ctx context.Context, productID, id string) (model.Tool, error) {
	return scanTool(p.pool.QueryRow(ctx, toolSelect+` WHERE t.product_id = $1 AND t.id = $2`, productID, id))
}

func (p *Postgres) CreateTool(ctx context.Context, value model.Tool) (model.Tool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Tool{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if value.BackendKind == "" {
		value.BackendKind = "http"
	}
	if value.BackendKind == "http" {
		parsedBase := value.BaseURL
		allowedHost := ""
		if parsed, parseErr := url.Parse(value.BaseURL); parseErr == nil {
			allowedHost = parsed.Hostname()
		}
		if _, err := tx.Exec(ctx, `INSERT INTO api_connections(id, organisation_id, product_id, name, base_url, allowed_hosts, credential_secret_id) VALUES ($1,$2,$3,$4,$5,$6,nullif($7,'')::uuid)`, value.APIConnectionID, value.OrganisationID, value.ProductID, value.Namespace+"."+value.Name, parsedBase, []string{allowedHost}, value.CredentialID); err != nil {
			return model.Tool{}, databaseError(err)
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO tool_definitions(id, organisation_id, product_id, namespace, name, description, input_schema, output_schema, state, api_connection_id, http_method, authorization_policy, timeout_ms, backend_kind, mcp_connection_id, upstream_tool_name, upstream_schema_hash, upstream_annotations, upstream_drifted) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'draft',nullif($9,'')::uuid,$10,$11,$12,$13,nullif($14,'')::uuid,$15,$16,$17,$18)`, value.ID, value.OrganisationID, value.ProductID, value.Namespace, value.Name, value.Description, value.InputSchema, value.OutputSchema, value.APIConnectionID, value.HTTPMethod, value.AuthorizationPolicy, value.TimeoutMS, value.BackendKind, value.MCPConnectionID, value.UpstreamToolName, value.UpstreamSchemaHash, value.UpstreamAnnotations, value.UpstreamDrifted)
	if err != nil {
		return model.Tool{}, databaseError(err)
	}
	created, err := scanTool(tx.QueryRow(ctx, toolSelect+` WHERE t.id = $1`, value.ID))
	if err != nil {
		return model.Tool{}, err
	}
	return created, tx.Commit(ctx)
}

func (p *Postgres) UpdateImportedTool(ctx context.Context, value model.Tool, expected int64) (model.Tool, error) {
	updated, err := scanTool(p.pool.QueryRow(ctx, `UPDATE tool_definitions SET description=$4, input_schema=$5, output_schema=$6, authorization_policy=$7, timeout_ms=$8, upstream_schema_hash=$9, upstream_annotations=$10, upstream_drifted=$11, revision=revision+1, updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$3 AND backend_kind='mcp' AND state='draft' RETURNING id::text, organisation_id::text, product_id::text, namespace, name, description, input_schema, output_schema, state::text, revision, coalesce(api_connection_id::text, ''), '', http_method, '', authorization_policy, timeout_ms, backend_kind, coalesce(mcp_connection_id::text, ''), upstream_tool_name, upstream_schema_hash, upstream_annotations, upstream_drifted, created_at, updated_at`, value.ProductID, value.ID, expected, value.Description, value.InputSchema, value.OutputSchema, value.AuthorizationPolicy, value.TimeoutMS, value.UpstreamSchemaHash, value.UpstreamAnnotations, value.UpstreamDrifted))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.Tool(ctx, value.ProductID, value.ID); lookupErr == nil {
			return model.Tool{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) MarkImportedToolDrift(ctx context.Context, productID, id string, drifted bool) (model.Tool, error) {
	query := `WITH updated AS (UPDATE tool_definitions SET upstream_drifted=$3,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2 AND backend_kind='mcp' RETURNING id) ` + toolSelect + ` WHERE t.id IN (SELECT id FROM updated)`
	return scanTool(p.pool.QueryRow(ctx, query, productID, id, drifted))
}

func (p *Postgres) PublishTool(ctx context.Context, productID, id string, expected int64, actorID string) (model.Tool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Tool{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var organisationID, backendKind, connectionID, mcpConnectionID, upstreamToolName, upstreamSchemaHash string
	var outputSchema, authorizationPolicy []byte
	var timeoutMS int
	if err := tx.QueryRow(ctx, `SELECT organisation_id::text, backend_kind, coalesce(api_connection_id::text,''), coalesce(mcp_connection_id::text,''), upstream_tool_name, upstream_schema_hash, output_schema, authorization_policy, timeout_ms FROM tool_definitions WHERE product_id = $1 AND id = $2 AND revision = $3 FOR UPDATE`, productID, id, expected).Scan(&organisationID, &backendKind, &connectionID, &mcpConnectionID, &upstreamToolName, &upstreamSchemaHash, &outputSchema, &authorizationPolicy, &timeoutMS); err != nil {
		return model.Tool{}, databaseError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tool_releases(organisation_id, product_id, tool_definition_id, api_connection_id, version, request_mapping, output_schema, response_mapping, authorization_policy, timeout_ms, rate_limit, published_by, published_at, backend_kind, mcp_connection_id, upstream_tool_name, upstream_schema_hash) VALUES ($1,$2,$3,nullif($4,'')::uuid,1,'{}',$5,'{}',$6,$7,'{"requests":60,"window_seconds":60}',nullif($8,'')::uuid,now(),$9,nullif($10,'')::uuid,$11,$12) ON CONFLICT (tool_definition_id, version) DO UPDATE SET output_schema = excluded.output_schema, authorization_policy = excluded.authorization_policy, timeout_ms = excluded.timeout_ms, published_by = excluded.published_by, published_at = now(), backend_kind=excluded.backend_kind, mcp_connection_id=excluded.mcp_connection_id, upstream_tool_name=excluded.upstream_tool_name, upstream_schema_hash=excluded.upstream_schema_hash`, organisationID, productID, id, connectionID, outputSchema, authorizationPolicy, timeoutMS, actorID, backendKind, mcpConnectionID, upstreamToolName, upstreamSchemaHash); err != nil {
		return model.Tool{}, databaseError(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE tool_definitions SET state = 'published', revision = revision + 1, updated_at = now() WHERE id = $1`, id); err != nil {
		return model.Tool{}, err
	}
	updated, err := scanTool(tx.QueryRow(ctx, toolSelect+` WHERE t.id = $1`, id))
	if err != nil {
		return model.Tool{}, err
	}
	return updated, tx.Commit(ctx)
}

const mcpConnectionSelect = `SELECT id::text, organisation_id::text, product_id::text, name, namespace, endpoint, protocol_version, auth_mode, coalesce(credential_secret_id::text,''), oauth_client_id, coalesce(oauth_client_secret_id::text,''), oauth_issuer, authorization_url, token_url, scopes, state, last_synced_at, last_catalog_hash, config, revision, created_at, updated_at FROM mcp_connections`

func scanMCPConnection(row interface{ Scan(...any) error }) (model.MCPConnection, error) {
	var value model.MCPConnection
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Name, &value.Namespace, &value.Endpoint, &value.ProtocolVersion, &value.AuthMode, &value.CredentialID, &value.OAuthClientID, &value.OAuthClientSecretID, &value.OAuthIssuer, &value.AuthorizationURL, &value.TokenURL, &value.Scopes, &value.State, &value.LastSyncedAt, &value.LastCatalogHash, &value.Config, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) MCPConnections(ctx context.Context, productID string) ([]model.MCPConnection, error) {
	rows, err := p.pool.Query(ctx, mcpConnectionSelect+` WHERE product_id=$1 ORDER BY name`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.MCPConnection, 0)
	for rows.Next() {
		value, err := scanMCPConnection(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) MCPConnection(ctx context.Context, productID, id string) (model.MCPConnection, error) {
	return scanMCPConnection(p.pool.QueryRow(ctx, mcpConnectionSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateMCPConnection(ctx context.Context, value model.MCPConnection) (model.MCPConnection, error) {
	return scanMCPConnection(p.pool.QueryRow(ctx, `INSERT INTO mcp_connections(id,organisation_id,product_id,name,namespace,endpoint,protocol_version,auth_mode,credential_secret_id,oauth_client_id,oauth_client_secret_id,oauth_issuer,authorization_url,token_url,scopes,config) VALUES($1,$2,$3,$4,$5,$6,'2026-07-28',$7,nullif($8,'')::uuid,$9,nullif($10,'')::uuid,$11,$12,$13,$14,$15) RETURNING id::text, organisation_id::text, product_id::text, name, namespace, endpoint, protocol_version, auth_mode, coalesce(credential_secret_id::text,''), oauth_client_id, coalesce(oauth_client_secret_id::text,''), oauth_issuer, authorization_url, token_url, scopes, state, last_synced_at, last_catalog_hash, config, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Name, value.Namespace, value.Endpoint, value.AuthMode, value.CredentialID, value.OAuthClientID, value.OAuthClientSecretID, value.OAuthIssuer, value.AuthorizationURL, value.TokenURL, value.Scopes, value.Config))
}

func (p *Postgres) UpdateMCPConnectionSync(ctx context.Context, productID, id, catalogHash string, syncedAt time.Time) (model.MCPConnection, error) {
	return scanMCPConnection(p.pool.QueryRow(ctx, `UPDATE mcp_connections SET last_synced_at=$3,last_catalog_hash=$4,revision=revision+1,updated_at=$3 WHERE product_id=$1 AND id=$2 RETURNING id::text, organisation_id::text, product_id::text, name, namespace, endpoint, protocol_version, auth_mode, coalesce(credential_secret_id::text,''), oauth_client_id, coalesce(oauth_client_secret_id::text,''), oauth_issuer, authorization_url, token_url, scopes, state, last_synced_at, last_catalog_hash, config, revision, created_at, updated_at`, productID, id, syncedAt, catalogHash))
}

const mcpGrantSelect = `SELECT id::text,organisation_id::text,product_id::text,connection_id::text,subject_id,upstream_subject,access_secret_id::text,coalesce(refresh_secret_id::text,''),scopes,expires_at,revoked_at,created_at,updated_at FROM mcp_user_grants`

func scanMCPGrant(row interface{ Scan(...any) error }) (model.MCPUserGrant, error) {
	var value model.MCPUserGrant
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.ConnectionID, &value.SubjectID, &value.UpstreamSubject, &value.AccessSecretID, &value.RefreshSecretID, &value.Scopes, &value.ExpiresAt, &value.RevokedAt, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) MCPUserGrant(ctx context.Context, connectionID, subjectID string) (model.MCPUserGrant, error) {
	return scanMCPGrant(p.pool.QueryRow(ctx, mcpGrantSelect+` WHERE connection_id=$1 AND subject_id=$2 AND revoked_at IS NULL`, connectionID, subjectID))
}

func (p *Postgres) SaveMCPUserGrant(ctx context.Context, value model.MCPUserGrant) (model.MCPUserGrant, error) {
	return scanMCPGrant(p.pool.QueryRow(ctx, `INSERT INTO mcp_user_grants(id,organisation_id,product_id,connection_id,subject_id,upstream_subject,access_secret_id,refresh_secret_id,scopes,expires_at,revoked_at) VALUES($1,$2,$3,$4,$5,$6,$7,nullif($8,'')::uuid,$9,$10,null) ON CONFLICT(connection_id,subject_id) DO UPDATE SET upstream_subject=excluded.upstream_subject,access_secret_id=excluded.access_secret_id,refresh_secret_id=excluded.refresh_secret_id,scopes=excluded.scopes,expires_at=excluded.expires_at,revoked_at=null,updated_at=now() RETURNING id::text,organisation_id::text,product_id::text,connection_id::text,subject_id,upstream_subject,access_secret_id::text,coalesce(refresh_secret_id::text,''),scopes,expires_at,revoked_at,created_at,updated_at`, value.ID, value.OrganisationID, value.ProductID, value.ConnectionID, value.SubjectID, value.UpstreamSubject, value.AccessSecretID, value.RefreshSecretID, value.Scopes, value.ExpiresAt))
}

func (p *Postgres) CreateMCPAuthorizationState(ctx context.Context, value model.MCPAuthorizationState) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO mcp_authorization_states(digest,connection_id,product_id,subject_id,code_verifier,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, value.Digest, value.ConnectionID, value.ProductID, value.SubjectID, value.CodeVerifier, value.ExpiresAt)
	return databaseError(err)
}

func (p *Postgres) ConsumeMCPAuthorizationState(ctx context.Context, digest []byte) (model.MCPAuthorizationState, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.MCPAuthorizationState{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var value model.MCPAuthorizationState
	err = tx.QueryRow(ctx, `DELETE FROM mcp_authorization_states WHERE digest=$1 AND expires_at>now() RETURNING digest,connection_id::text,product_id::text,subject_id,code_verifier,expires_at`, digest).Scan(&value.Digest, &value.ConnectionID, &value.ProductID, &value.SubjectID, &value.CodeVerifier, &value.ExpiresAt)
	if err != nil {
		return model.MCPAuthorizationState{}, databaseError(err)
	}
	return value, tx.Commit(ctx)
}

func scanProvider(row pgx.Row) (model.Provider, error) {
	var value model.Provider
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Name, &value.Kind, &value.BaseURL, &value.CredentialID, &value.Config, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const providerSelect = `SELECT id::text, organisation_id::text, product_id::text, name, kind, coalesce(base_url,''), coalesce(credential_secret_id::text,''), config, revision, created_at, updated_at FROM providers`

func (p *Postgres) Providers(ctx context.Context, productID string) ([]model.Provider, error) {
	rows, err := p.pool.Query(ctx, providerSelect+` WHERE product_id=$1 ORDER BY name`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.Provider, 0)
	for rows.Next() {
		value, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) Provider(ctx context.Context, productID, id string) (model.Provider, error) {
	return scanProvider(p.pool.QueryRow(ctx, providerSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateProvider(ctx context.Context, value model.Provider) (model.Provider, error) {
	return scanProvider(p.pool.QueryRow(ctx, `INSERT INTO providers(id,organisation_id,product_id,name,kind,base_url,credential_secret_id,config) VALUES ($1,$2,$3,$4,$5,$6,nullif($7,'')::uuid,$8) RETURNING id::text, organisation_id::text, product_id::text, name, kind, coalesce(base_url,''), coalesce(credential_secret_id::text,''), config, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Name, value.Kind, value.BaseURL, value.CredentialID, value.Config))
}

func scanProject(row pgx.Row) (model.Project, error) {
	var value model.Project
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.EnvironmentID, &value.ProviderID, &value.OwnerType, &value.OwnerID, &value.ExternalID, &value.IdempotencyKey, &value.State, &value.ExpiresAt, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const projectSelect = `SELECT id::text, organisation_id::text, product_id::text, environment_id::text, provider_id::text, owner_type, owner_id, external_id, idempotency_key, state, expires_at, created_at, updated_at FROM projects`

func (p *Postgres) Projects(ctx context.Context, productID string) ([]model.Project, error) {
	rows, err := p.pool.Query(ctx, projectSelect+` WHERE product_id=$1 ORDER BY created_at DESC`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.Project, 0)
	for rows.Next() {
		value, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) Project(ctx context.Context, productID, id string) (model.Project, error) {
	return scanProject(p.pool.QueryRow(ctx, projectSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateProject(ctx context.Context, value model.Project) (model.Project, error) {
	return scanProject(p.pool.QueryRow(ctx, `INSERT INTO projects(id,organisation_id,product_id,environment_id,provider_id,owner_type,owner_id,external_id,idempotency_key,state,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (provider_id,idempotency_key) DO UPDATE SET updated_at=projects.updated_at RETURNING id::text, organisation_id::text, product_id::text, environment_id::text, provider_id::text, owner_type, owner_id, external_id, idempotency_key, state, expires_at, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.EnvironmentID, value.ProviderID, value.OwnerType, value.OwnerID, value.ExternalID, value.IdempotencyKey, value.State, value.ExpiresAt))
}

func scanCredentialLease(row pgx.Row) (model.CredentialLease, error) {
	var value model.CredentialLease
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.EnvironmentID, &value.ProjectID, &value.ProviderID, &value.SubjectID, &value.ExternalID, &value.IdempotencyKey, &value.Scopes, &value.SecretFingerprint, &value.ExpiresAt, &value.RevokedAt, &value.CreatedAt)
	return value, databaseError(err)
}

const credentialLeaseSelect = `SELECT id::text, organisation_id::text, product_id::text, environment_id::text, coalesce(project_id::text,''), provider_id::text, subject_id, external_id, idempotency_key, scopes, secret_fingerprint, expires_at, revoked_at, created_at FROM credential_leases`

func (p *Postgres) CredentialLeases(ctx context.Context, productID string) ([]model.CredentialLease, error) {
	rows, err := p.pool.Query(ctx, credentialLeaseSelect+` WHERE product_id=$1 ORDER BY created_at DESC`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.CredentialLease, 0)
	for rows.Next() {
		value, err := scanCredentialLease(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) CredentialLease(ctx context.Context, productID, id string) (model.CredentialLease, error) {
	return scanCredentialLease(p.pool.QueryRow(ctx, credentialLeaseSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateCredentialLease(ctx context.Context, value model.CredentialLease) (model.CredentialLease, error) {
	return scanCredentialLease(p.pool.QueryRow(ctx, `INSERT INTO credential_leases(id,organisation_id,product_id,environment_id,project_id,provider_id,subject_id,external_id,idempotency_key,scopes,secret_fingerprint,expires_at) VALUES ($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (provider_id,idempotency_key) WHERE idempotency_key <> '' DO UPDATE SET idempotency_key=credential_leases.idempotency_key RETURNING id::text, organisation_id::text, product_id::text, environment_id::text, coalesce(project_id::text,''), provider_id::text, subject_id, external_id, idempotency_key, scopes, secret_fingerprint, expires_at, revoked_at, created_at`, value.ID, value.OrganisationID, value.ProductID, value.EnvironmentID, value.ProjectID, value.ProviderID, value.SubjectID, value.ExternalID, value.IdempotencyKey, value.Scopes, value.SecretFingerprint, value.ExpiresAt))
}

func (p *Postgres) RevokeCredentialLease(ctx context.Context, productID, id string, revokedAt time.Time) (model.CredentialLease, error) {
	return scanCredentialLease(p.pool.QueryRow(ctx, `UPDATE credential_leases SET revoked_at=$3 WHERE product_id=$1 AND id=$2 AND revoked_at IS NULL RETURNING id::text, organisation_id::text, product_id::text, environment_id::text, coalesce(project_id::text,''), provider_id::text, subject_id, external_id, idempotency_key, scopes, secret_fingerprint, expires_at, revoked_at, created_at`, productID, id, revokedAt))
}

func scanIntegrationRun(row interface{ Scan(...any) error }) (model.IntegrationRun, error) {
	var value model.IntegrationRun
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.EnvironmentID, &value.UserID, &value.ActorPseudonym, &value.RequestedOutcome, &value.State, &value.ReportedSuccess, &value.ValidatedSuccess, &value.FailureCode, &value.StartedAt, &value.FinishedAt)
	return value, databaseError(err)
}

const integrationRunSelect = `SELECT id::text, organisation_id::text, product_id::text, environment_id::text, coalesce(user_id::text,''), actor_pseudonym, requested_outcome, state, reported_success, validated_success, coalesce(failure_code,''), started_at, finished_at FROM integration_runs`

func (p *Postgres) IntegrationRuns(ctx context.Context, productID string) ([]model.IntegrationRun, error) {
	rows, err := p.pool.Query(ctx, integrationRunSelect+` WHERE product_id=$1 ORDER BY started_at DESC LIMIT 500`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.IntegrationRun, 0)
	for rows.Next() {
		value, err := scanIntegrationRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) IntegrationRun(ctx context.Context, productID, id string) (model.IntegrationRun, error) {
	return scanIntegrationRun(p.pool.QueryRow(ctx, integrationRunSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateIntegrationRun(ctx context.Context, value model.IntegrationRun) (model.IntegrationRun, error) {
	return scanIntegrationRun(p.pool.QueryRow(ctx, `INSERT INTO integration_runs(id,organisation_id,product_id,environment_id,user_id,actor_pseudonym,requested_outcome,state,started_at) VALUES ($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,'running',$8) RETURNING id::text, organisation_id::text, product_id::text, environment_id::text, coalesce(user_id::text,''), actor_pseudonym, requested_outcome, state, reported_success, validated_success, coalesce(failure_code,''), started_at, finished_at`, value.ID, value.OrganisationID, value.ProductID, value.EnvironmentID, value.UserID, value.ActorPseudonym, value.RequestedOutcome, value.StartedAt))
}

func (p *Postgres) CompleteIntegrationRun(ctx context.Context, productID, id string, reported, validated *bool, failureCode string, finishedAt time.Time) (model.IntegrationRun, error) {
	state := "failed"
	if validated != nil && *validated {
		state = "succeeded"
	}
	value, err := scanIntegrationRun(p.pool.QueryRow(ctx, `UPDATE integration_runs SET state=$3, reported_success=$4, validated_success=$5, failure_code=nullif($6,''), finished_at=$7 WHERE product_id=$1 AND id=$2 AND finished_at IS NULL RETURNING id::text, organisation_id::text, product_id::text, environment_id::text, coalesce(user_id::text,''), actor_pseudonym, requested_outcome, state, reported_success, validated_success, coalesce(failure_code,''), started_at, finished_at`, productID, id, state, reported, validated, failureCode, finishedAt))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.IntegrationRun(ctx, productID, id); lookupErr == nil {
			return model.IntegrationRun{}, ErrConflict
		}
	}
	return value, err
}

func scanLLMProfile(row pgx.Row) (model.LLMProfile, error) {
	var value model.LLMProfile
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Role, &value.Provider, &value.Endpoint, &value.Model, &value.CredentialID, &value.EmbeddingDimensions, &value.MaxInputTokens, &value.MaxOutputTokens, &value.DailyTokenBudget, &value.Hardening, &value.Enabled, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const llmProfileSelect = `SELECT id::text, organisation_id::text, product_id::text, role, provider, endpoint, model, coalesce(credential_secret_id::text,''), coalesce(embedding_dimensions,0), max_input_tokens, max_output_tokens, daily_token_budget, hardening, enabled, revision, created_at, updated_at FROM llm_profiles`

func (p *Postgres) LLMProfiles(ctx context.Context, productID string) ([]model.LLMProfile, error) {
	rows, err := p.pool.Query(ctx, llmProfileSelect+` WHERE product_id=$1 ORDER BY role`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.LLMProfile, 0)
	for rows.Next() {
		value, err := scanLLMProfile(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) SaveLLMProfile(ctx context.Context, value model.LLMProfile) (model.LLMProfile, error) {
	return scanLLMProfile(p.pool.QueryRow(ctx, `INSERT INTO llm_profiles(id,organisation_id,product_id,role,provider,endpoint,model,credential_secret_id,embedding_dimensions,max_input_tokens,max_output_tokens,daily_token_budget,hardening,enabled) VALUES ($1,$2,$3,$4,$5,$6,$7,nullif($8,'')::uuid,nullif($9,0),$10,$11,$12,$13,$14) ON CONFLICT (product_id,role) DO UPDATE SET provider=excluded.provider,endpoint=excluded.endpoint,model=excluded.model,credential_secret_id=excluded.credential_secret_id,embedding_dimensions=excluded.embedding_dimensions,max_input_tokens=excluded.max_input_tokens,max_output_tokens=excluded.max_output_tokens,daily_token_budget=excluded.daily_token_budget,hardening=excluded.hardening,enabled=excluded.enabled,revision=llm_profiles.revision+1,updated_at=now() RETURNING id::text, organisation_id::text, product_id::text, role, provider, endpoint, model, coalesce(credential_secret_id::text,''), coalesce(embedding_dimensions,0), max_input_tokens, max_output_tokens, daily_token_budget, hardening, enabled, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Role, value.Provider, value.Endpoint, value.Model, value.CredentialID, value.EmbeddingDimensions, value.MaxInputTokens, value.MaxOutputTokens, value.DailyTokenBudget, value.Hardening, value.Enabled))
}

func scanVendorIdentity(row interface{ Scan(...any) error }) (identity.VendorConfig, error) {
	var value identity.VendorConfig
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Issuer, &value.ClientID, &value.ClientSecretID, &value.Scopes, &value.Audience, &value.OrganisationClaim, &value.EntitlementHookURL, &value.AllowedRedirectURIs, &value.AuthorizationHookURL, &value.AuthorizationCredentialID, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const vendorIdentitySelect = `SELECT id::text, organisation_id::text, product_id::text, issuer, client_id, coalesce(client_secret_id::text, ''), scopes, audience, organisation_claim, entitlement_hook_url, allowed_redirect_uris, authorization_hook_url, coalesce(authorization_credential_id::text,''), revision, created_at, updated_at FROM vendor_identity_providers`

func (p *Postgres) VendorIdentity(ctx context.Context, productID string) (identity.VendorConfig, error) {
	return scanVendorIdentity(p.pool.QueryRow(ctx, vendorIdentitySelect+` WHERE product_id = $1`, productID))
}

func (p *Postgres) SaveVendorIdentity(ctx context.Context, value identity.VendorConfig) (identity.VendorConfig, error) {
	return scanVendorIdentity(p.pool.QueryRow(ctx, `INSERT INTO vendor_identity_providers(id, organisation_id, product_id, issuer, client_id, client_secret_id, scopes, audience, organisation_claim, entitlement_hook_url, allowed_redirect_uris, authorization_hook_url, authorization_credential_id) VALUES ($1,$2,$3,$4,$5,nullif($6,'')::uuid,$7,$8,$9,$10,$11,$12,nullif($13,'')::uuid) ON CONFLICT (product_id) WHERE product_id IS NOT NULL DO UPDATE SET issuer=excluded.issuer, client_id=excluded.client_id, client_secret_id=excluded.client_secret_id, scopes=excluded.scopes, audience=excluded.audience, organisation_claim=excluded.organisation_claim, entitlement_hook_url=excluded.entitlement_hook_url, allowed_redirect_uris=excluded.allowed_redirect_uris, authorization_hook_url=excluded.authorization_hook_url, authorization_credential_id=excluded.authorization_credential_id, revision=vendor_identity_providers.revision+1, updated_at=now() RETURNING id::text, organisation_id::text, product_id::text, issuer, client_id, coalesce(client_secret_id::text, ''), scopes, audience, organisation_claim, entitlement_hook_url, allowed_redirect_uris, authorization_hook_url, coalesce(authorization_credential_id::text,''), revision, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Issuer, value.ClientID, value.ClientSecretID, value.Scopes, value.Audience, value.OrganisationClaim, value.EntitlementHookURL, value.AllowedRedirectURIs, value.AuthorizationHookURL, value.AuthorizationCredentialID))
}

func (p *Postgres) CreateOAuthState(ctx context.Context, value identity.OAuthState) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO oauth_states(state_digest, product_id, client_id, redirect_uri, downstream_state, downstream_challenge, upstream_verifier, nonce, expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, value.Digest, value.ProductID, value.ClientID, value.RedirectURI, value.DownstreamState, value.DownstreamChallenge, value.UpstreamVerifier, value.Nonce, value.ExpiresAt)
	return databaseError(err)
}

func scanOAuthState(row pgx.Row) (identity.OAuthState, error) {
	var value identity.OAuthState
	err := row.Scan(&value.Digest, &value.ProductID, &value.ClientID, &value.RedirectURI, &value.DownstreamState, &value.DownstreamChallenge, &value.UpstreamVerifier, &value.Nonce, &value.ExpiresAt)
	return value, databaseError(err)
}

func (p *Postgres) ConsumeOAuthState(ctx context.Context, digest []byte) (identity.OAuthState, error) {
	return scanOAuthState(p.pool.QueryRow(ctx, `DELETE FROM oauth_states WHERE state_digest = $1 RETURNING state_digest, product_id::text, client_id, redirect_uri, downstream_state, downstream_challenge, upstream_verifier, nonce, expires_at`, digest))
}

func (p *Postgres) CreateOAuthCode(ctx context.Context, value identity.OAuthCode) error {
	entitlements, _ := json.Marshal(value.Entitlements)
	_, err := p.pool.Exec(ctx, `INSERT INTO oauth_authorization_codes(code_digest, product_id, client_id, redirect_uri, downstream_challenge, issuer, subject, email, display_name, vendor_organisation_id, entitlements, expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, value.Digest, value.ProductID, value.ClientID, value.RedirectURI, value.DownstreamChallenge, value.Issuer, value.Subject, value.Email, value.DisplayName, value.VendorOrganisation, entitlements, value.ExpiresAt)
	return databaseError(err)
}

func scanOAuthCode(row pgx.Row) (identity.OAuthCode, error) {
	var value identity.OAuthCode
	var entitlements []byte
	err := row.Scan(&value.Digest, &value.ProductID, &value.ClientID, &value.RedirectURI, &value.DownstreamChallenge, &value.Issuer, &value.Subject, &value.Email, &value.DisplayName, &value.VendorOrganisation, &entitlements, &value.ExpiresAt)
	if err == nil {
		err = json.Unmarshal(entitlements, &value.Entitlements)
	}
	return value, databaseError(err)
}

func (p *Postgres) ConsumeOAuthCode(ctx context.Context, digest []byte) (identity.OAuthCode, error) {
	return scanOAuthCode(p.pool.QueryRow(ctx, `DELETE FROM oauth_authorization_codes WHERE code_digest = $1 RETURNING code_digest, product_id::text, client_id, redirect_uri, downstream_challenge, issuer, subject, email, display_name, vendor_organisation_id, entitlements, expires_at`, digest))
}

func (p *Postgres) CreateAccessToken(ctx context.Context, value identity.AccessToken) error {
	entitlements, _ := json.Marshal(value.Entitlements)
	_, err := p.pool.Exec(ctx, `INSERT INTO oauth_access_tokens(token_digest, product_id, client_id, issuer, subject, email, display_name, vendor_organisation_id, entitlements, scopes, expires_at, created_at, revoked_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, value.Digest, value.ProductID, value.ClientID, value.Issuer, value.Subject, value.Email, value.DisplayName, value.VendorOrganisation, entitlements, value.Scopes, value.ExpiresAt, value.CreatedAt, value.RevokedAt)
	return databaseError(err)
}

func scanAccessToken(row pgx.Row) (identity.AccessToken, error) {
	var value identity.AccessToken
	var entitlements []byte
	err := row.Scan(&value.Digest, &value.ProductID, &value.ClientID, &value.Issuer, &value.Subject, &value.Email, &value.DisplayName, &value.VendorOrganisation, &entitlements, &value.Scopes, &value.ExpiresAt, &value.CreatedAt, &value.RevokedAt)
	if err == nil {
		err = json.Unmarshal(entitlements, &value.Entitlements)
	}
	return value, databaseError(err)
}

func (p *Postgres) AccessTokenByDigest(ctx context.Context, digest []byte) (identity.AccessToken, error) {
	return scanAccessToken(p.pool.QueryRow(ctx, `SELECT token_digest, product_id::text, client_id, issuer, subject, email, display_name, vendor_organisation_id, entitlements, scopes, expires_at, created_at, revoked_at FROM oauth_access_tokens WHERE token_digest = $1`, digest))
}

func (p *Postgres) PublicKnowledge(ctx context.Context, productID, query string) ([]model.KnowledgeRecord, error) {
	pattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := p.pool.Query(ctx, `SELECT id::text, product_id::text, source_id::text, title, body, canonical_url, visibility::text FROM knowledge_documents WHERE product_id = $1 AND visibility = 'public' AND state = 'published' AND ($2 = '%%' OR title ILIKE $2 OR body ILIKE $2) ORDER BY updated_at DESC LIMIT 20`, productID, pattern)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.KnowledgeRecord, 0)
	for rows.Next() {
		var value model.KnowledgeRecord
		if err := rows.Scan(&value.ID, &value.ProductID, &value.SourceID, &value.Title, &value.Text, &value.URL, &value.Visibility); err != nil {
			return nil, err
		}
		value.Published = true
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) PrivateKnowledge(ctx context.Context, productID, query string) ([]model.KnowledgeRecord, error) {
	pattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := p.pool.Query(ctx, `SELECT id::text, product_id::text, source_id::text, title, body, canonical_url, visibility::text FROM knowledge_documents WHERE product_id = $1 AND state = 'published' AND ($2 = '%%' OR title ILIKE $2 OR body ILIKE $2) ORDER BY updated_at DESC LIMIT 20`, productID, pattern)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.KnowledgeRecord, 0)
	for rows.Next() {
		var value model.KnowledgeRecord
		if err := rows.Scan(&value.ID, &value.ProductID, &value.SourceID, &value.Title, &value.Text, &value.URL, &value.Visibility); err != nil {
			return nil, err
		}
		value.Published = true
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) AppendAnalytics(ctx context.Context, event model.AnalyticsEvent) error {
	dimensions, _ := json.Marshal(event.Dimensions)
	_, err := p.pool.Exec(ctx, `INSERT INTO analytics_events(organisation_id, product_id, event_name, actor_kind, actor_pseudonym, integration_run_id, dimensions, value, created_at) VALUES ($1,$2,$3,$4,nullif($5,''),nullif($6,'')::uuid,$7,nullif($8,0),$9)`, event.OrganisationID, event.ProductID, event.EventName, event.ActorKind, event.ActorPseudonym, event.IntegrationRunID, dimensions, event.Value, event.CreatedAt)
	return databaseError(err)
}

func (p *Postgres) AnalyticsSummary(ctx context.Context, productID string, since time.Time) (model.AnalyticsSummary, error) {
	value := model.AnalyticsSummary{Since: since, GeneratedAt: time.Now().UTC(), Channels: map[string]int64{"private_mcp": 0, "public_mcp": 0, "private_widget": 0, "public_widget": 0}, Funnel: map[string]int64{"connector_authorized": 0, "run_started": 0, "capability_resolved": 0, "package_acquired": 0, "credentials_issued": 0, "implementation_validated": 0, "success_reported": 0}}
	err := p.pool.QueryRow(ctx, `SELECT count(DISTINCT actor_pseudonym) FILTER (WHERE actor_pseudonym IS NOT NULL), count(*) FILTER (WHERE event_name='mcp.request'), count(*) FILTER (WHERE event_name='tool.called'), count(*) FILTER (WHERE event_name='package.downloaded') FROM analytics_events WHERE product_id=$1 AND created_at >= $2`, productID, since).Scan(&value.ActiveDevelopers, &value.MCPRequests, &value.ToolCalls, &value.PackageDownloads)
	if err != nil {
		return value, databaseError(err)
	}
	if err := p.pool.QueryRow(ctx, `SELECT count(DISTINCT issuer || E'\\000' || subject) FROM oauth_access_tokens WHERE product_id=$1 AND created_at >= $2`, productID, since).Scan(&value.AuthorizedUsers); err != nil {
		return value, databaseError(err)
	}
	if err := p.pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE validated_success IS NOT NULL), count(*) FILTER (WHERE validated_success=true) FROM integration_runs WHERE product_id=$1 AND started_at >= $2`, productID, since).Scan(&value.IntegrationRuns, &value.ValidatedRuns, &value.ValidatedSuccess); err != nil {
		return value, databaseError(err)
	}
	if value.ValidatedRuns > 0 {
		value.FirstPassRate = float64(value.ValidatedSuccess) * 100 / float64(value.ValidatedRuns)
	}
	rows, err := p.pool.Query(ctx, `SELECT coalesce(dimensions->>'channel','unknown'), count(*) FROM analytics_events WHERE product_id=$1 AND created_at >= $2 AND event_name='mcp.request' GROUP BY 1`, productID, since)
	if err != nil {
		return value, databaseError(err)
	}
	for rows.Next() {
		var channel string
		var count int64
		if err := rows.Scan(&channel, &count); err != nil {
			rows.Close()
			return value, err
		}
		value.Channels[channel] = count
	}
	rows.Close()
	rows, err = p.pool.Query(ctx, `SELECT event_name, count(*) FROM analytics_events WHERE product_id=$1 AND created_at >= $2 AND event_name = ANY($3) GROUP BY event_name`, productID, since, []string{"connector_authorized", "run_started", "capability_resolved", "package_acquired", "credentials_issued", "implementation_validated", "success_reported"})
	if err != nil {
		return value, databaseError(err)
	}
	for rows.Next() {
		var name string
		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			rows.Close()
			return value, err
		}
		value.Funnel[name] = count
	}
	rows.Close()
	rows, err = p.pool.Query(ctx, `SELECT created_at::date::text, count(*) FROM analytics_events WHERE product_id=$1 AND created_at >= $2 AND event_name='mcp.request' GROUP BY created_at::date ORDER BY created_at::date`, productID, since)
	if err != nil {
		return value, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var point model.AnalyticsPoint
		if err := rows.Scan(&point.Date, &point.Count); err != nil {
			return value, err
		}
		value.DailyRequests = append(value.DailyRequests, point)
	}
	return value, rows.Err()
}

func (p *Postgres) AppendAudit(ctx context.Context, event model.AuditEvent) error {
	prior, _ := json.Marshal(event.Prior)
	current, _ := json.Marshal(event.Current)
	_, err := p.pool.Exec(ctx, `INSERT INTO audit_events(organisation_id, product_id, actor_id, actor_kind, action, target_type, target_id, prior, current, request_id, outcome, created_at) VALUES (nullif($1, '')::uuid, nullif($2, '')::uuid, $3, 'root', $4, $5, $6, $7, $8, $9, 'success', $10)`, event.OrganisationID, event.ProductID, event.ActorID, event.Action, event.TargetType, event.TargetID, prior, current, event.RequestID, event.CreatedAt)
	return databaseError(err)
}

func (p *Postgres) AuditEvents(ctx context.Context, organisationID string) ([]model.AuditEvent, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text, coalesce(organisation_id::text, ''), coalesce(product_id::text, ''), actor_id, action, target_type, target_id, coalesce(prior, '{}'::jsonb), coalesce(current, '{}'::jsonb), request_id, created_at FROM audit_events WHERE organisation_id = $1 ORDER BY created_at DESC`, organisationID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.AuditEvent, 0)
	for rows.Next() {
		var value model.AuditEvent
		var prior, current []byte
		if err := rows.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.ActorID, &value.Action, &value.TargetType, &value.TargetID, &prior, &current, &value.RequestID, &value.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(prior, &value.Prior)
		_ = json.Unmarshal(current, &value.Current)
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) SetupCompleted(ctx context.Context) (bool, error) {
	var completed bool
	err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM root_users WHERE revoked_at IS NULL)`).Scan(&completed)
	return completed, err
}

func (p *Postgres) CreateInitialRoot(ctx context.Context, account auth.RootAccount) error {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(2811042001)`); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM root_users)`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrConflict
	}
	if _, err := tx.Exec(ctx, `INSERT INTO platform_config(singleton, public_url, setup_completed_at) VALUES (true, $1, $2) ON CONFLICT (singleton) DO UPDATE SET public_url = excluded.public_url, setup_completed_at = excluded.setup_completed_at, updated_at = now()`, p.publicURL, account.CreatedAt); err != nil {
		return databaseError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO users(id, issuer, subject, email, display_name, created_at, updated_at) VALUES ($1, 'dokosoko:root', $1::text, $2, $3, $4, $4)`, account.UserID, account.Email, account.DisplayName, account.CreatedAt); err != nil {
		return databaseError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO root_users(user_id, password_hash, totp_secret_ciphertext, recovery_code_digests, created_at, revoked_at) VALUES ($1, $2, $3, $4, $5, $6)`, account.UserID, account.PasswordHash, account.TOTPSecretCiphertext, account.RecoveryCodeDigests, account.CreatedAt, account.RevokedAt); err != nil {
		return databaseError(err)
	}
	return tx.Commit(ctx)
}

func (p *Postgres) CreateRoot(ctx context.Context, account auth.RootAccount) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO users(id, issuer, subject, email, display_name, created_at, updated_at) VALUES ($1, 'dokosoko:root', $1::text, $2, $3, $4, $4)`, account.UserID, account.Email, account.DisplayName, account.CreatedAt); err != nil {
		return databaseError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO root_users(user_id, password_hash, totp_secret_ciphertext, recovery_code_digests, created_by, created_at) VALUES ($1,$2,$3,$4,nullif($5,'')::uuid,$6)`, account.UserID, account.PasswordHash, account.TOTPSecretCiphertext, account.RecoveryCodeDigests, account.CreatedBy, account.CreatedAt); err != nil {
		return databaseError(err)
	}
	return tx.Commit(ctx)
}

func (p *Postgres) RevokeRoot(ctx context.Context, userID string, revokedAt time.Time) error {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(2811042002)`); err != nil {
		return err
	}
	var active int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM root_users WHERE revoked_at IS NULL`).Scan(&active); err != nil {
		return err
	}
	if active <= 1 {
		return auth.ErrLastRoot
	}
	command, err := tx.Exec(ctx, `UPDATE root_users SET revoked_at=$2 WHERE user_id=$1 AND revoked_at IS NULL`, userID, revokedAt)
	if err != nil {
		return databaseError(err)
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM root_sessions WHERE user_id=$1`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanRoot(row pgx.Row) (auth.RootAccount, error) {
	var value auth.RootAccount
	err := row.Scan(&value.UserID, &value.Email, &value.DisplayName, &value.PasswordHash, &value.TOTPSecretCiphertext, &value.RecoveryCodeDigests, &value.CreatedAt, &value.RevokedAt)
	return value, databaseError(err)
}

const rootSelect = `SELECT u.id::text, u.email::text, u.display_name, r.password_hash, r.totp_secret_ciphertext, r.recovery_code_digests, r.created_at, r.revoked_at FROM root_users r JOIN users u ON u.id = r.user_id`

func (p *Postgres) RootByEmail(ctx context.Context, email string) (auth.RootAccount, error) {
	return scanRoot(p.pool.QueryRow(ctx, rootSelect+` WHERE u.email = $1`, strings.ToLower(email)))
}

func (p *Postgres) RootByID(ctx context.Context, id string) (auth.RootAccount, error) {
	return scanRoot(p.pool.QueryRow(ctx, rootSelect+` WHERE u.id = $1`, id))
}

func (p *Postgres) RootAccounts(ctx context.Context) ([]auth.RootAccount, error) {
	rows, err := p.pool.Query(ctx, rootSelect+` ORDER BY r.created_at`)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]auth.RootAccount, 0)
	for rows.Next() {
		value, err := scanRoot(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) CreateSession(ctx context.Context, session auth.SessionRecord) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO root_sessions(token_digest, user_id, csrf_digest, expires_at, created_at, last_seen_at) VALUES ($1, $2, $3, $4, $5, $6)`, session.TokenDigest, session.UserID, session.CSRFDigest, session.ExpiresAt, session.CreatedAt, session.LastSeenAt)
	return databaseError(err)
}

func (p *Postgres) SessionByDigest(ctx context.Context, digest []byte) (auth.SessionRecord, error) {
	var value auth.SessionRecord
	err := p.pool.QueryRow(ctx, `SELECT token_digest, user_id::text, csrf_digest, expires_at, created_at, last_seen_at FROM root_sessions WHERE token_digest = $1`, digest).Scan(&value.TokenDigest, &value.UserID, &value.CSRFDigest, &value.ExpiresAt, &value.CreatedAt, &value.LastSeenAt)
	return value, databaseError(err)
}

func (p *Postgres) DeleteSession(ctx context.Context, digest []byte) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM root_sessions WHERE token_digest = $1`, digest)
	return err
}

func (p *Postgres) Ping(ctx context.Context) error {
	if err := p.pool.Ping(ctx); err != nil {
		return fmt.Errorf("database ping: %w", err)
	}
	return nil
}

func (p *Postgres) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM root_sessions WHERE expires_at <= $1`, now)
	return err
}
