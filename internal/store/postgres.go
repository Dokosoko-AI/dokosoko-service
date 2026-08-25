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
	"github.com/dokosoko/dokosoko-service/internal/docreview"
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
	err := row.Scan(&value.ID, &value.OrganisationID, &value.Name, &value.Slug, &value.Description, &value.DefaultVersionPolicy, &value.CatalogRevision, &value.RequirePromotionApproval, &value.PublicMCPEnabled, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const productSelect = `SELECT id::text, organisation_id::text, name, slug, description, default_version_policy, catalog_revision, require_promotion_approval, public_mcp_enabled, revision, created_at, updated_at FROM products`

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
	rows, err := p.pool.Query(ctx, productSelect+` WHERE organisation_id = $1 ORDER BY name`, organisationID)
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
	return scanProduct(p.pool.QueryRow(ctx, `INSERT INTO products(id, organisation_id, name, slug, description, default_version_policy, require_promotion_approval) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id::text, organisation_id::text, name, slug, description, default_version_policy, catalog_revision, require_promotion_approval, public_mcp_enabled, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.Name, value.Slug, value.Description, value.DefaultVersionPolicy, value.RequirePromotionApproval))
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
	return scanProduct(p.pool.QueryRow(ctx, productSelect+` WHERE id = $1`, id))
}

func (p *Postgres) UpdateProduct(ctx context.Context, value model.Product, expected int64) (model.Product, error) {
	updated, err := scanProduct(p.pool.QueryRow(ctx, `UPDATE products SET description=$2, default_version_policy=$3, require_promotion_approval=$4, public_mcp_enabled=$5, revision=revision+1, catalog_revision=catalog_revision+1, updated_at=now() WHERE id=$1 AND revision=$6 RETURNING id::text, organisation_id::text, name, slug, description, default_version_policy, catalog_revision, require_promotion_approval, public_mcp_enabled, revision, created_at, updated_at`, value.ID, value.Description, value.DefaultVersionPolicy, value.RequirePromotionApproval, value.PublicMCPEnabled, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.Product(ctx, value.ID); lookupErr == nil {
			return model.Product{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) BumpProductCatalogRevision(ctx context.Context, productID string) (int64, error) {
	var revision int64
	err := p.pool.QueryRow(ctx, `UPDATE products SET catalog_revision=catalog_revision+1, updated_at=now() WHERE id=$1 RETURNING catalog_revision`, productID).Scan(&revision)
	return revision, databaseError(err)
}

func scanProductVersion(row interface{ Scan(...any) error }) (model.ProductVersion, error) {
	var value model.ProductVersion
	var manifest, releaseDiff, driftDetails []byte
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Version, &value.ProfileID, &value.ProfileName, &value.DefinitionRevision, &value.ManifestHash, &releaseDiff, &value.ReleaseStage, &value.RolloutPercentage, &value.PromotionState, &value.PromotionNote, &value.RequestedLatest, &value.RequestedLTS, &value.PublisherActorID, &value.PromotionRequestedBy, &value.ApprovedBy, &value.ApprovedAt, &value.DriftStatus, &driftDetails, &value.DriftCheckedAt, &value.IsLatest, &value.IsLTS, &value.DeprecatedAt, &value.DeprecationMessage, &value.ReplacementVersion, &value.SunsetAt, &value.Revision, &value.PublishedAt, &value.CreatedAt, &value.UpdatedAt, &manifest)
	if err != nil {
		return model.ProductVersion{}, databaseError(err)
	}
	if err := json.Unmarshal(manifest, &value.Manifest); err != nil {
		return model.ProductVersion{}, err
	}
	if err := json.Unmarshal(releaseDiff, &value.Diff); err != nil {
		return model.ProductVersion{}, err
	}
	if err := json.Unmarshal(driftDetails, &value.DriftDetails); err != nil {
		return model.ProductVersion{}, err
	}
	return value, nil
}

const productVersionColumns = `id::text, organisation_id::text, product_id::text, display_version, profile_id, profile_name, definition_revision, manifest_hash, release_diff, release_stage, rollout_percentage, promotion_state, promotion_note, requested_latest, requested_lts, publisher_actor_id, promotion_requested_by, approved_by, approved_at, drift_status, drift_details, drift_checked_at, is_latest, is_lts, deprecated_at, deprecation_message, replacement_version, sunset_at, revision, coalesce(published_at,created_at), created_at, updated_at, manifest`
const productVersionSelect = `SELECT ` + productVersionColumns + ` FROM connector_releases`

func (p *Postgres) ProductVersions(ctx context.Context, productID string) ([]model.ProductVersion, error) {
	rows, err := p.pool.Query(ctx, productVersionSelect+` WHERE product_id=$1 AND state='published' ORDER BY published_at DESC, created_at DESC`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.ProductVersion, 0)
	for rows.Next() {
		value, err := scanProductVersion(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (p *Postgres) ProductVersion(ctx context.Context, productID, id string) (model.ProductVersion, error) {
	return scanProductVersion(p.pool.QueryRow(ctx, productVersionSelect+` WHERE product_id=$1 AND id=$2 AND state='published'`, productID, id))
}

func (p *Postgres) CreateProductVersion(ctx context.Context, value model.ProductVersion) (model.ProductVersion, error) {
	manifest, err := json.Marshal(value.Manifest)
	if err != nil {
		return model.ProductVersion{}, err
	}
	releaseDiff, err := json.Marshal(value.Diff)
	if err != nil {
		return model.ProductVersion{}, err
	}
	driftDetails, err := json.Marshal(value.DriftDetails)
	if err != nil {
		return model.ProductVersion{}, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.ProductVersion{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT 1 FROM products WHERE id=$1 FOR UPDATE`, value.ProductID); err != nil {
		return model.ProductVersion{}, databaseError(err)
	}
	if value.IsLatest {
		if _, err := tx.Exec(ctx, `UPDATE connector_releases SET is_latest=false, revision=revision+1, updated_at=now() WHERE product_id=$1 AND is_latest`, value.ProductID); err != nil {
			return model.ProductVersion{}, databaseError(err)
		}
	}
	created, err := scanProductVersion(tx.QueryRow(ctx, `INSERT INTO connector_releases(id,organisation_id,product_id,version,state,manifest,published_at,display_version,profile_id,profile_name,definition_revision,manifest_hash,release_diff,release_stage,rollout_percentage,promotion_state,promotion_note,requested_latest,requested_lts,publisher_actor_id,promotion_requested_by,approved_by,approved_at,drift_status,drift_details,drift_checked_at,is_latest,is_lts,deprecated_at,deprecation_message,replacement_version,sunset_at) SELECT $1,$2,$3,coalesce(max(version),0)+1,'published',$4,now(),$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29 FROM connector_releases WHERE product_id=$3 RETURNING `+productVersionColumns, value.ID, value.OrganisationID, value.ProductID, manifest, value.Version, value.ProfileID, value.ProfileName, value.DefinitionRevision, value.ManifestHash, releaseDiff, value.ReleaseStage, value.RolloutPercentage, value.PromotionState, value.PromotionNote, value.RequestedLatest, value.RequestedLTS, value.PublisherActorID, value.PromotionRequestedBy, value.ApprovedBy, value.ApprovedAt, value.DriftStatus, driftDetails, value.DriftCheckedAt, value.IsLatest, value.IsLTS, value.DeprecatedAt, value.DeprecationMessage, value.ReplacementVersion, value.SunsetAt))
	if err != nil {
		return model.ProductVersion{}, databaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ProductVersion{}, databaseError(err)
	}
	return created, nil
}

func (p *Postgres) UpdateProductVersion(ctx context.Context, value model.ProductVersion, expected int64) (model.ProductVersion, error) {
	releaseDiff, err := json.Marshal(value.Diff)
	if err != nil {
		return model.ProductVersion{}, err
	}
	driftDetails, err := json.Marshal(value.DriftDetails)
	if err != nil {
		return model.ProductVersion{}, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.ProductVersion{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT 1 FROM products WHERE id=$1 FOR UPDATE`, value.ProductID); err != nil {
		return model.ProductVersion{}, databaseError(err)
	}
	if value.IsLatest {
		if _, err := tx.Exec(ctx, `UPDATE connector_releases SET is_latest=false, revision=revision+1, updated_at=now() WHERE product_id=$1 AND id<>$2 AND is_latest`, value.ProductID, value.ID); err != nil {
			return model.ProductVersion{}, databaseError(err)
		}
	}
	updated, err := scanProductVersion(tx.QueryRow(ctx, `UPDATE connector_releases SET release_diff=$3,release_stage=$4,rollout_percentage=$5,promotion_state=$6,promotion_note=$7,requested_latest=$8,requested_lts=$9,promotion_requested_by=$10,approved_by=$11,approved_at=$12,drift_status=$13,drift_details=$14,drift_checked_at=$15,is_latest=$16,is_lts=$17,deprecated_at=$18,deprecation_message=$19,replacement_version=$20,sunset_at=$21,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$22 AND state='published' RETURNING `+productVersionColumns, value.ProductID, value.ID, releaseDiff, value.ReleaseStage, value.RolloutPercentage, value.PromotionState, value.PromotionNote, value.RequestedLatest, value.RequestedLTS, value.PromotionRequestedBy, value.ApprovedBy, value.ApprovedAt, value.DriftStatus, driftDetails, value.DriftCheckedAt, value.IsLatest, value.IsLTS, value.DeprecatedAt, value.DeprecationMessage, value.ReplacementVersion, value.SunsetAt, expected))
	if err != nil {
		return model.ProductVersion{}, databaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ProductVersion{}, databaseError(err)
	}
	return updated, nil
}

func scanProductVersionPin(row interface{ Scan(...any) error }) (model.ProductVersionPin, error) {
	var value model.ProductVersionPin
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Scope, &value.ScopeID, &value.CustomerAccountID, &value.EnvironmentID, &value.InstallationID, &value.ProductVersionID, &value.ProductVersion, &value.Reason, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const productVersionPinSelect = `SELECT p.id::text,p.organisation_id::text,p.product_id::text,p.scope,p.scope_id,p.customer_account_id::text,coalesce(p.environment_id::text,''),coalesce(p.installation_id::text,''),p.connector_release_id::text,r.display_version,p.reason,p.revision,p.created_at,p.updated_at FROM product_version_pins p JOIN connector_releases r ON r.id=p.connector_release_id`

func (p *Postgres) ProductVersionPins(ctx context.Context, productID string) ([]model.ProductVersionPin, error) {
	rows, err := p.pool.Query(ctx, productVersionPinSelect+` WHERE p.product_id=$1 ORDER BY p.scope,p.scope_id`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.ProductVersionPin, 0)
	for rows.Next() {
		value, err := scanProductVersionPin(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (p *Postgres) ProductVersionPin(ctx context.Context, productID, scope, scopeID string) (model.ProductVersionPin, error) {
	return scanProductVersionPin(p.pool.QueryRow(ctx, productVersionPinSelect+` WHERE p.product_id=$1 AND p.scope=$2 AND p.scope_id=$3`, productID, scope, scopeID))
}

func (p *Postgres) SaveProductVersionPin(ctx context.Context, value model.ProductVersionPin, expected int64) (model.ProductVersionPin, error) {
	if expected == 0 {
		created, err := scanProductVersionPin(p.pool.QueryRow(ctx, `INSERT INTO product_version_pins(id,organisation_id,product_id,scope,scope_id,customer_account_id,environment_id,installation_id,connector_release_id,reason) VALUES($1,$2,$3,$4,$5,$6,nullif($7,'')::uuid,nullif($8,'')::uuid,$9,$10) RETURNING id::text,organisation_id::text,product_id::text,scope,scope_id,customer_account_id::text,coalesce(environment_id::text,''),coalesce(installation_id::text,''),connector_release_id::text,(SELECT display_version FROM connector_releases WHERE id=product_version_pins.connector_release_id),reason,revision,created_at,updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Scope, value.ScopeID, value.CustomerAccountID, value.EnvironmentID, value.InstallationID, value.ProductVersionID, value.Reason))
		return created, err
	}
	updated, err := scanProductVersionPin(p.pool.QueryRow(ctx, `UPDATE product_version_pins SET connector_release_id=$4,reason=$5,revision=revision+1,updated_at=now() WHERE product_id=$1 AND scope=$2 AND scope_id=$3 AND revision=$6 RETURNING id::text,organisation_id::text,product_id::text,scope,scope_id,customer_account_id::text,coalesce(environment_id::text,''),coalesce(installation_id::text,''),connector_release_id::text,(SELECT display_version FROM connector_releases WHERE id=product_version_pins.connector_release_id),reason,revision,created_at,updated_at`, value.ProductID, value.Scope, value.ScopeID, value.ProductVersionID, value.Reason, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.ProductVersionPin(ctx, value.ProductID, value.Scope, value.ScopeID); lookupErr == nil {
			return model.ProductVersionPin{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) DeleteProductVersionPin(ctx context.Context, productID, id string) error {
	result, err := p.pool.Exec(ctx, `DELETE FROM product_version_pins WHERE product_id=$1 AND id=$2`, productID, id)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanProductVersionPinHistory(row interface{ Scan(...any) error }) (model.ProductVersionPinHistory, error) {
	var value model.ProductVersionPinHistory
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.PinID, &value.Scope, &value.ScopeID, &value.PriorVersion, &value.ProductVersion, &value.Action, &value.Reason, &value.ActorID, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) ProductVersionPinHistory(ctx context.Context, productID string) ([]model.ProductVersionPinHistory, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text,organisation_id::text,product_id::text,pin_id::text,scope,scope_id,prior_version,product_version,action,reason,actor_id,created_at FROM product_version_pin_history WHERE product_id=$1 ORDER BY created_at DESC LIMIT 500`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.ProductVersionPinHistory, 0)
	for rows.Next() {
		value, scanErr := scanProductVersionPinHistory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (p *Postgres) AppendProductVersionPinHistory(ctx context.Context, value model.ProductVersionPinHistory) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO product_version_pin_history(id,organisation_id,product_id,pin_id,scope,scope_id,prior_version,product_version,action,reason,actor_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, value.ID, value.OrganisationID, value.ProductID, value.PinID, value.Scope, value.ScopeID, value.PriorVersion, value.ProductVersion, value.Action, value.Reason, value.ActorID, value.CreatedAt)
	return databaseError(err)
}

func scanProductInstallation(row interface{ Scan(...any) error }) (model.ProductInstallation, error) {
	var value model.ProductInstallation
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.CustomerAccountID, &value.EnvironmentID, &value.ExternalID, &value.Name, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const productInstallationSelect = `SELECT id::text,organisation_id::text,product_id::text,customer_account_id::text,environment_id::text,external_id,name,state,revision,created_at,updated_at FROM product_installations`

func (p *Postgres) ProductInstallations(ctx context.Context, productID string) ([]model.ProductInstallation, error) {
	rows, err := p.pool.Query(ctx, productInstallationSelect+` WHERE product_id=$1 ORDER BY name`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.ProductInstallation, 0)
	for rows.Next() {
		value, scanErr := scanProductInstallation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (p *Postgres) ProductInstallation(ctx context.Context, productID, id string) (model.ProductInstallation, error) {
	return scanProductInstallation(p.pool.QueryRow(ctx, productInstallationSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) ProductInstallationByExternalID(ctx context.Context, productID, externalID string) (model.ProductInstallation, error) {
	return scanProductInstallation(p.pool.QueryRow(ctx, productInstallationSelect+` WHERE product_id=$1 AND external_id=$2`, productID, externalID))
}

func (p *Postgres) SaveProductInstallation(ctx context.Context, value model.ProductInstallation, expected int64) (model.ProductInstallation, error) {
	if expected == 0 {
		return scanProductInstallation(p.pool.QueryRow(ctx, `INSERT INTO product_installations(id,organisation_id,product_id,customer_account_id,environment_id,external_id,name,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text,organisation_id::text,product_id::text,customer_account_id::text,environment_id::text,external_id,name,state,revision,created_at,updated_at`, value.ID, value.OrganisationID, value.ProductID, value.CustomerAccountID, value.EnvironmentID, value.ExternalID, value.Name, value.State))
	}
	updated, err := scanProductInstallation(p.pool.QueryRow(ctx, `UPDATE product_installations SET customer_account_id=$3,environment_id=$4,external_id=$5,name=$6,state=$7,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$8 RETURNING id::text,organisation_id::text,product_id::text,customer_account_id::text,environment_id::text,external_id,name,state,revision,created_at,updated_at`, value.ProductID, value.ID, value.CustomerAccountID, value.EnvironmentID, value.ExternalID, value.Name, value.State, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.ProductInstallation(ctx, value.ProductID, value.ID); lookupErr == nil {
			return model.ProductInstallation{}, ErrConflict
		}
	}
	return updated, err
}

func scanProductDefinition(row interface{ Scan(...any) error }) (model.ProductDefinition, error) {
	var value model.ProductDefinition
	var raw []byte
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.State, &value.GeneratedBy, &value.SourceBuildID, &raw, &value.Revision, &value.PublishedAt, &value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		return model.ProductDefinition{}, databaseError(err)
	}
	metadata := value
	if err := json.Unmarshal(raw, &value); err != nil {
		return model.ProductDefinition{}, err
	}
	value.ID, value.OrganisationID, value.ProductID = metadata.ID, metadata.OrganisationID, metadata.ProductID
	value.State, value.GeneratedBy, value.SourceBuildID = metadata.State, metadata.GeneratedBy, metadata.SourceBuildID
	value.Revision, value.PublishedAt, value.CreatedAt, value.UpdatedAt = metadata.Revision, metadata.PublishedAt, metadata.CreatedAt, metadata.UpdatedAt
	return value, nil
}

const productDefinitionSelect = `SELECT id::text, organisation_id::text, product_id::text, state, generated_by, coalesce(source_build_id::text,''), definition, revision, published_at, created_at, updated_at FROM product_definitions`

func (p *Postgres) ProductDefinition(ctx context.Context, productID string) (model.ProductDefinition, error) {
	return scanProductDefinition(p.pool.QueryRow(ctx, productDefinitionSelect+` WHERE product_id=$1`, productID))
}

func (p *Postgres) SaveProductDefinition(ctx context.Context, value model.ProductDefinition, expected int64) (model.ProductDefinition, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return model.ProductDefinition{}, err
	}
	if expected == 0 {
		return scanProductDefinition(p.pool.QueryRow(ctx, `INSERT INTO product_definitions(id,organisation_id,product_id,state,generated_by,source_build_id,definition,published_at) VALUES ($1,$2,$3,$4,$5,nullif($6,'')::uuid,$7,$8) RETURNING id::text, organisation_id::text, product_id::text, state, generated_by, coalesce(source_build_id::text,''), definition, revision, published_at, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.State, value.GeneratedBy, value.SourceBuildID, raw, value.PublishedAt))
	}
	updated, err := scanProductDefinition(p.pool.QueryRow(ctx, `UPDATE product_definitions SET state=$2,generated_by=$3,source_build_id=nullif($4,'')::uuid,definition=$5,published_at=$6,revision=revision+1,updated_at=now() WHERE product_id=$1 AND revision=$7 RETURNING id::text, organisation_id::text, product_id::text, state, generated_by, coalesce(source_build_id::text,''), definition, revision, published_at, created_at, updated_at`, value.ProductID, value.State, value.GeneratedBy, value.SourceBuildID, raw, value.PublishedAt, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.ProductDefinition(ctx, value.ProductID); lookupErr == nil {
			return model.ProductDefinition{}, ErrConflict
		}
	}
	return updated, err
}

func scanProductBuild(row interface{ Scan(...any) error }) (model.ProductBuild, error) {
	var value model.ProductBuild
	var inputs, proposal, unresolved []byte
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.State, &value.AnalysisMode, &inputs, &proposal, &unresolved, &value.CreatedAt, &value.CompletedAt)
	if err != nil {
		return model.ProductBuild{}, databaseError(err)
	}
	if err := json.Unmarshal(inputs, &value.Inputs); err != nil {
		return model.ProductBuild{}, err
	}
	if err := json.Unmarshal(proposal, &value.Proposal); err != nil {
		return model.ProductBuild{}, err
	}
	if err := json.Unmarshal(unresolved, &value.Unresolved); err != nil {
		return model.ProductBuild{}, err
	}
	return value, nil
}

const productBuildSelect = `SELECT id::text, organisation_id::text, product_id::text, state, analysis_mode, inputs, proposed_definition, unresolved, created_at, completed_at FROM product_builds`

func (p *Postgres) ProductBuilds(ctx context.Context, productID string) ([]model.ProductBuild, error) {
	rows, err := p.pool.Query(ctx, productBuildSelect+` WHERE product_id=$1 ORDER BY created_at DESC LIMIT 50`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.ProductBuild, 0)
	for rows.Next() {
		value, err := scanProductBuild(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) ProductBuild(ctx context.Context, productID, id string) (model.ProductBuild, error) {
	return scanProductBuild(p.pool.QueryRow(ctx, productBuildSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateProductBuild(ctx context.Context, value model.ProductBuild) (model.ProductBuild, error) {
	inputs, err := json.Marshal(value.Inputs)
	if err != nil {
		return model.ProductBuild{}, err
	}
	proposal, err := json.Marshal(value.Proposal)
	if err != nil {
		return model.ProductBuild{}, err
	}
	unresolved, err := json.Marshal(value.Unresolved)
	if err != nil {
		return model.ProductBuild{}, err
	}
	return scanProductBuild(p.pool.QueryRow(ctx, `INSERT INTO product_builds(id,organisation_id,product_id,state,analysis_mode,inputs,proposed_definition,unresolved,created_at,completed_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id::text, organisation_id::text, product_id::text, state, analysis_mode, inputs, proposed_definition, unresolved, created_at, completed_at`, value.ID, value.OrganisationID, value.ProductID, value.State, value.AnalysisMode, inputs, proposal, unresolved, value.CreatedAt, value.CompletedAt))
}

func (p *Postgres) MarkProductBuildPublished(ctx context.Context, productID, id string) (model.ProductBuild, error) {
	return scanProductBuild(p.pool.QueryRow(ctx, `UPDATE product_builds SET state='published' WHERE product_id=$1 AND id=$2 AND state='review' RETURNING id::text, organisation_id::text, product_id::text, state, analysis_mode, inputs, proposed_definition, unresolved, created_at, completed_at`, productID, id))
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

func scanSourcePublication(row interface{ Scan(...any) error }) (model.SourcePublication, error) {
	var value model.SourcePublication
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.SourceID, &value.CrawlJobID, &value.Revision, &value.Visibility, &value.ContentHash, &value.DocumentCount, &value.ReviewedBy, &value.ReviewedAt, &value.PublishedAt)
	return value, databaseError(err)
}

const sourcePublicationSelect = `SELECT id::text, organisation_id::text, product_id::text, source_id::text, crawl_job_id::text, revision, visibility::text, content_hash, document_count, reviewed_by, reviewed_at, published_at FROM source_publications`

func (p *Postgres) SourcePublications(ctx context.Context, productID, sourceID string) ([]model.SourcePublication, error) {
	rows, err := p.pool.Query(ctx, sourcePublicationSelect+` WHERE product_id = $1 AND source_id = $2 ORDER BY revision DESC`, productID, sourceID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.SourcePublication, 0)
	for rows.Next() {
		value, scanErr := scanSourcePublication(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) SourcePublication(ctx context.Context, productID, publicationID string) (model.SourcePublication, error) {
	return scanSourcePublication(p.pool.QueryRow(ctx, sourcePublicationSelect+` WHERE product_id = $1 AND id = $2`, productID, publicationID))
}

func (p *Postgres) SourceReview(ctx context.Context, productID, sourceID, crawlJobID string) (model.SourceReview, error) {
	source, err := p.Source(ctx, productID, sourceID)
	if err != nil {
		return model.SourceReview{}, err
	}
	jobQuery := crawlJobSelect + ` WHERE product_id = $1 AND source_id = $2`
	args := []any{productID, sourceID}
	if strings.TrimSpace(crawlJobID) != "" {
		jobQuery += ` AND id = $3`
		args = append(args, crawlJobID)
	}
	jobQuery += ` ORDER BY queued_at DESC, id DESC LIMIT 1`
	job, err := scanCrawlJob(p.pool.QueryRow(ctx, jobQuery, args...))
	if err != nil {
		return model.SourceReview{}, err
	}
	rows, err := p.pool.Query(ctx, `
		SELECT kd.id::text, cjd.crawl_job_id::text, kd.snapshot_id::text, kd.title,
		       kd.canonical_url, cjd.assessment_state::text, cjd.assessment_trust_level,
		       cjd.assessment_injection_indicators,
		       'sha256:' || encode(ss.content_sha256, 'hex'), cjd.changed
		FROM crawl_job_documents cjd
		JOIN knowledge_documents kd ON kd.id = cjd.knowledge_document_id
		JOIN source_snapshots ss ON ss.id = kd.snapshot_id
		WHERE cjd.crawl_job_id = $1 AND kd.product_id = $2 AND kd.source_id = $3
		ORDER BY kd.canonical_url, kd.id`, job.ID, productID, sourceID)
	if err != nil {
		return model.SourceReview{}, databaseError(err)
	}
	defer rows.Close()
	documents := make([]model.CrawlReviewDocument, 0)
	for rows.Next() {
		var value model.CrawlReviewDocument
		if scanErr := rows.Scan(&value.ID, &value.CrawlJobID, &value.SnapshotID, &value.Title, &value.CanonicalURL, &value.State, &value.TrustLevel, &value.InjectionIndicators, &value.ContentHash, &value.Changed); scanErr != nil {
			return model.SourceReview{}, scanErr
		}
		documents = append(documents, value)
	}
	if err := rows.Err(); err != nil {
		return model.SourceReview{}, err
	}
	review := model.SourceReview{Source: source, CrawlJob: job, Documents: documents}
	publication, err := scanSourcePublication(p.pool.QueryRow(ctx, sourcePublicationSelect+` WHERE product_id = $1 AND source_id = $2 AND crawl_job_id = $3`, productID, sourceID, job.ID))
	if err == nil {
		review.Publication = &publication
	} else if !errors.Is(err, ErrNotFound) {
		return model.SourceReview{}, err
	}
	return review, nil
}

func (p *Postgres) PublishSource(ctx context.Context, productID, sourceID string, expected int64, publication model.SourcePublication, documentIDs []string) (model.Source, model.SourcePublication, error) {
	if len(documentIDs) == 0 {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// All crawler paths that can create or reassess evidence take this row
	// first. Holding it through commit makes the checks below one serial view.
	lockedSource, err := scanSource(tx.QueryRow(ctx, `
		SELECT id::text, organisation_id::text, product_id::text, name, kind, location,
		       visibility::text, state::text, revision, created_at, updated_at
		FROM sources
		WHERE product_id = $1 AND id = $2
		FOR UPDATE`, productID, sourceID))
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	if lockedSource.Revision != expected || lockedSource.Quarantined {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}

	latestJob, err := scanCrawlJob(tx.QueryRow(ctx, crawlJobSelect+`
		WHERE product_id = $1 AND source_id = $2
		ORDER BY queued_at DESC, id DESC
		LIMIT 1
		FOR UPDATE`, productID, sourceID))
	if errors.Is(err, ErrNotFound) {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	if latestJob.ID != publication.CrawlJobID || latestJob.FinishedAt == nil ||
		(latestJob.State != "review" && latestJob.State != "succeeded") ||
		latestJob.FetchedCount == 0 {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}

	rows, err := tx.Query(ctx, `
		SELECT kd.id::text, cjd.crawl_job_id::text, kd.snapshot_id::text, kd.title,
		       kd.canonical_url, cjd.assessment_state::text, cjd.assessment_trust_level,
		       cjd.assessment_injection_indicators,
		       'sha256:' || encode(ss.content_sha256, 'hex'), cjd.changed,
		       kd.state::text, kd.injection_indicators
		FROM crawl_job_documents cjd
		JOIN knowledge_documents kd ON kd.id = cjd.knowledge_document_id
		JOIN source_snapshots ss ON ss.id = kd.snapshot_id
		WHERE cjd.crawl_job_id = $1
		  AND kd.product_id = $2 AND kd.source_id = $3
		  AND kd.id = ANY($4::uuid[])
		ORDER BY kd.id
		FOR UPDATE OF cjd, kd`, publication.CrawlJobID, productID, sourceID, documentIDs)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, databaseError(err)
	}
	lockedDocuments := make([]model.CrawlReviewDocument, 0, len(documentIDs))
	for rows.Next() {
		var document model.CrawlReviewDocument
		var liveState string
		var liveIndicators json.RawMessage
		if scanErr := rows.Scan(
			&document.ID, &document.CrawlJobID, &document.SnapshotID, &document.Title,
			&document.CanonicalURL, &document.State, &document.TrustLevel,
			&document.InjectionIndicators, &document.ContentHash, &document.Changed,
			&liveState, &liveIndicators,
		); scanErr != nil {
			rows.Close()
			return model.Source{}, model.SourcePublication{}, scanErr
		}
		if !docreview.SafeAssessment(document.State, document.InjectionIndicators) ||
			!docreview.SafeAssessment(liveState, liveIndicators) {
			rows.Close()
			return model.Source{}, model.SourcePublication{}, ErrConflict
		}
		lockedDocuments = append(lockedDocuments, document)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return model.Source{}, model.SourcePublication{}, rowsErr
	}
	if len(lockedDocuments) != len(documentIDs) {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	lockedHash, err := docreview.PublicationContentHash(lockedDocuments)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	if publication.ContentHash != lockedHash {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}

	updated, err := scanSource(tx.QueryRow(ctx, `UPDATE sources SET state = 'published', revision = revision + 1, updated_at = now() WHERE product_id = $1 AND id = $2 AND revision = $3 AND state <> 'quarantined' RETURNING id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at`, productID, sourceID, expected))
	if errors.Is(err, ErrNotFound) {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	publication, err = scanSourcePublication(tx.QueryRow(ctx, `
		INSERT INTO source_publications(id, organisation_id, product_id, source_id, crawl_job_id, revision, visibility, content_hash, document_count, reviewed_by, reviewed_at, published_at)
		SELECT $1, $2, $3, $4, $5, coalesce(max(revision), 0) + 1, $6, $7, $8, $9, $10, $11
		FROM source_publications WHERE source_id = $4
		RETURNING id::text, organisation_id::text, product_id::text, source_id::text, crawl_job_id::text, revision, visibility::text, content_hash, document_count, reviewed_by, reviewed_at, published_at`, publication.ID, updated.OrganisationID, productID, sourceID, publication.CrawlJobID, updated.Visibility, publication.ContentHash, len(documentIDs), publication.ReviewedBy, publication.ReviewedAt, publication.PublishedAt))
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO source_publication_documents(source_publication_id, knowledge_document_id)
		SELECT $1, value::uuid FROM unnest($2::text[]) AS selected(value)`, publication.ID, documentIDs)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, databaseError(err)
	}
	if result.RowsAffected() != int64(len(documentIDs)) {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE knowledge_documents SET state = 'published', visibility = $3, revision = revision + 1, updated_at = now() WHERE product_id = $1 AND source_id = $2 AND id = ANY($4::uuid[]) AND state = 'validated' AND injection_indicators = '[]'::jsonb`, productID, sourceID, updated.Visibility, documentIDs); err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	return updated, publication, nil
}

func scanCrawlJob(row interface{ Scan(...any) error }) (model.CrawlJob, error) {
	var value model.CrawlJob
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.SourceID, &value.State, &value.Attempt, &value.DiscoveredCount, &value.FetchedCount, &value.ChangedCount, &value.ErrorCode, &value.ErrorMessage, &value.QueuedAt, &value.StartedAt, &value.FinishedAt)
	return value, databaseError(err)
}

const crawlJobSelect = `SELECT id::text, organisation_id::text, product_id::text, source_id::text, state, attempt, discovered_count, fetched_count, changed_count, coalesce(error_code, ''), coalesce(error_message, ''), queued_at, started_at, finished_at FROM crawl_jobs`

func (p *Postgres) CrawlJobs(ctx context.Context, productID, sourceID string) ([]model.CrawlJob, error) {
	rows, err := p.pool.Query(ctx, crawlJobSelect+` WHERE product_id = $1 AND source_id = $2 ORDER BY queued_at DESC, id DESC LIMIT 50`, productID, sourceID)
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

type pgxRowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func createCrawlJobWithSourceLock(ctx context.Context, query pgxRowQuerier, value model.CrawlJob) (model.CrawlJob, error) {
	var lockedSourceID string
	err := query.QueryRow(ctx, `
		SELECT id::text FROM sources
		WHERE id = $1 AND organisation_id = $2 AND product_id = $3
		FOR UPDATE`, value.SourceID, value.OrganisationID, value.ProductID).Scan(&lockedSourceID)
	if err != nil {
		return model.CrawlJob{}, databaseError(err)
	}
	return scanCrawlJob(query.QueryRow(ctx, `INSERT INTO crawl_jobs(id, organisation_id, product_id, source_id, state) VALUES ($1, $2, $3, $4, 'queued') RETURNING id::text, organisation_id::text, product_id::text, source_id::text, state, attempt, discovered_count, fetched_count, changed_count, coalesce(error_code, ''), coalesce(error_message, ''), queued_at, started_at, finished_at`, value.ID, value.OrganisationID, value.ProductID, value.SourceID))
}

func (p *Postgres) CreateCrawlJob(ctx context.Context, value model.CrawlJob) (model.CrawlJob, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.CrawlJob{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := createCrawlJobWithSourceLock(ctx, tx, value)
	if err != nil {
		return model.CrawlJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.CrawlJob{}, err
	}
	return created, nil
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

func (p *Postgres) DeleteSecret(ctx context.Context, organisationID, id string) error {
	result, err := p.pool.Exec(ctx, `DELETE FROM secrets WHERE organisation_id=$1 AND id=$2`, organisationID, id)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

const toolSelect = `SELECT t.id::text, t.organisation_id::text, t.product_id::text, t.scope, coalesce(t.owner_integration_id::text, ''), coalesce(t.runtime_service_connection_id::text,''), t.http_path, t.namespace, t.name, t.description, t.input_schema, t.output_schema, t.state::text, t.revision, coalesce(t.api_connection_id::text, ''), coalesce(c.base_url, ''), t.http_method, CASE WHEN t.backend_kind = 'mcp' THEN '{"type":"none"}'::jsonb ELSE coalesce(c.auth_config || jsonb_build_object('type', c.authentication_type), '{"type":"delegated_oauth"}'::jsonb) END, coalesce(c.credential_secret_id::text, ''), coalesce(secret.fingerprint, ''), t.request_mapping, t.response_mapping, t.request_example, t.response_example, t.authorization_policy, t.timeout_ms, t.backend_kind, coalesce(t.mcp_connection_id::text, ''), t.upstream_tool_name, t.upstream_schema_hash, t.upstream_annotations, t.upstream_drifted, t.created_at, t.updated_at FROM tool_definitions t LEFT JOIN api_connections c ON c.id = t.api_connection_id LEFT JOIN secrets secret ON secret.id = c.credential_secret_id`

func scanTool(row interface{ Scan(...any) error }) (model.Tool, error) {
	var value model.Tool
	var requestExample, responseExample []byte
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Scope, &value.OwnerIntegrationID, &value.RuntimeServiceConnectionID, &value.HTTPPath, &value.Namespace, &value.Name, &value.Description, &value.InputSchema, &value.OutputSchema, &value.State, &value.Revision, &value.APIConnectionID, &value.BaseURL, &value.HTTPMethod, &value.UpstreamAuth, &value.CredentialID, &value.CredentialFingerprint, &value.RequestMapping, &value.ResponseMapping, &requestExample, &responseExample, &value.AuthorizationPolicy, &value.TimeoutMS, &value.BackendKind, &value.MCPConnectionID, &value.UpstreamToolName, &value.UpstreamSchemaHash, &value.UpstreamAnnotations, &value.UpstreamDrifted, &value.CreatedAt, &value.UpdatedAt)
	value.RequestExample = append(json.RawMessage(nil), requestExample...)
	value.ResponseExample = append(json.RawMessage(nil), responseExample...)
	value.CredentialPresent = value.CredentialID != ""
	return value, databaseError(err)
}

func nullableToolExample(value json.RawMessage) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return value
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
	if err := rows.Err(); err != nil {
		return nil, databaseError(err)
	}
	for index := range result {
		result[index], err = p.enrichToolRuntimeTargets(ctx, result[index])
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (p *Postgres) Tool(ctx context.Context, productID, id string) (model.Tool, error) {
	value, err := scanTool(p.pool.QueryRow(ctx, toolSelect+` WHERE t.product_id = $1 AND t.id = $2`, productID, id))
	if err != nil {
		return model.Tool{}, err
	}
	return p.enrichToolRuntimeTargets(ctx, value)
}

func (p *Postgres) CreateTool(ctx context.Context, value model.Tool) (model.Tool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Tool{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	value.Scope, value.OwnerIntegrationID, err = normalizeToolOwnership(value.Scope, value.OwnerIntegrationID)
	if err != nil {
		return model.Tool{}, err
	}
	if value.Scope == model.ToolScopeAPI {
		var ownerExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE id=$1 AND deployment_id=$2 AND organisation_id=$3)`, value.OwnerIntegrationID, value.ProductID, value.OrganisationID).Scan(&ownerExists); err != nil {
			return model.Tool{}, databaseError(err)
		}
		if !ownerExists {
			return model.Tool{}, ErrConflict
		}
	}
	if value.BackendKind == "" {
		value.BackendKind = "http"
	}
	if value.BackendKind == "http" && value.RuntimeServiceConnectionID == "" {
		authenticationType := "delegated_oauth"
		var upstreamAuth struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(value.UpstreamAuth, &upstreamAuth) == nil && upstreamAuth.Type != "" {
			authenticationType = upstreamAuth.Type
		}
		parsedBase := value.BaseURL
		allowedHost := ""
		if parsed, parseErr := url.Parse(value.BaseURL); parseErr == nil {
			allowedHost = parsed.Hostname()
		}
		if _, err := tx.Exec(ctx, `INSERT INTO api_connections(id, organisation_id, product_id, name, base_url, allowed_hosts, authentication_type, auth_config, credential_secret_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,nullif($9,'')::uuid)`, value.APIConnectionID, value.OrganisationID, value.ProductID, value.Namespace+"."+value.Name, parsedBase, []string{allowedHost}, authenticationType, value.UpstreamAuth, value.CredentialID); err != nil {
			return model.Tool{}, databaseError(err)
		}
	}
	if value.RuntimeServiceConnectionID != "" {
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runtime_service_connections WHERE id=$1 AND deployment_id=$2 AND organisation_id=$3 AND integration_id=$4 AND state='active' AND EXISTS(SELECT 1 FROM runtime_service_connection_revisions WHERE connection_id=$1 AND is_current))`, value.RuntimeServiceConnectionID, value.ProductID, value.OrganisationID, value.OwnerIntegrationID).Scan(&valid); err != nil {
			return model.Tool{}, databaseError(err)
		}
		if !valid || value.Scope != model.ToolScopeAPI || value.APIConnectionID != "" || value.HTTPPath == "" {
			return model.Tool{}, ErrConflict
		}
	}
	requestMapping, responseMapping := value.RequestMapping, value.ResponseMapping
	if len(requestMapping) == 0 {
		requestMapping = json.RawMessage(`{}`)
	}
	if len(responseMapping) == 0 {
		responseMapping = json.RawMessage(`{}`)
	}
	_, err = tx.Exec(ctx, `INSERT INTO tool_definitions(id, organisation_id, product_id, scope, owner_integration_id, runtime_service_connection_id, http_path, namespace, name, description, input_schema, output_schema, state, api_connection_id, http_method, request_mapping, response_mapping, request_example, response_example, authorization_policy, timeout_ms, backend_kind, mcp_connection_id, upstream_tool_name, upstream_schema_hash, upstream_annotations, upstream_drifted) VALUES ($1,$2,$3,$4,nullif($5,'')::uuid,nullif($6,'')::uuid,$7,$8,$9,$10,$11,$12,'draft',nullif($13,'')::uuid,$14,$15,$16,$17,$18,$19,$20,$21,nullif($22,'')::uuid,$23,$24,$25,$26)`, value.ID, value.OrganisationID, value.ProductID, value.Scope, value.OwnerIntegrationID, value.RuntimeServiceConnectionID, value.HTTPPath, value.Namespace, value.Name, value.Description, value.InputSchema, value.OutputSchema, value.APIConnectionID, value.HTTPMethod, requestMapping, responseMapping, nullableToolExample(value.RequestExample), nullableToolExample(value.ResponseExample), value.AuthorizationPolicy, value.TimeoutMS, value.BackendKind, value.MCPConnectionID, value.UpstreamToolName, value.UpstreamSchemaHash, value.UpstreamAnnotations, value.UpstreamDrifted)
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
	updated, err := scanTool(p.pool.QueryRow(ctx, `UPDATE tool_definitions SET description=$4, input_schema=$5, output_schema=$6, authorization_policy=$7, timeout_ms=$8, upstream_schema_hash=$9, upstream_annotations=$10, upstream_drifted=$11, revision=revision+1, updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$3 AND backend_kind='mcp' AND state='draft' RETURNING id::text, organisation_id::text, product_id::text, scope, coalesce(owner_integration_id::text, ''), '', '', namespace, name, description, input_schema, output_schema, state::text, revision, coalesce(api_connection_id::text, ''), '', http_method, '{"type":"none"}'::jsonb, '', '', request_mapping, response_mapping, request_example, response_example, authorization_policy, timeout_ms, backend_kind, coalesce(mcp_connection_id::text, ''), upstream_tool_name, upstream_schema_hash, upstream_annotations, upstream_drifted, created_at, updated_at`, value.ProductID, value.ID, expected, value.Description, value.InputSchema, value.OutputSchema, value.AuthorizationPolicy, value.TimeoutMS, value.UpstreamSchemaHash, value.UpstreamAnnotations, value.UpstreamDrifted))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.Tool(ctx, value.ProductID, value.ID); lookupErr == nil {
			return model.Tool{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) MarkImportedToolDrift(ctx context.Context, productID, id string, drifted bool) (model.Tool, error) {
	query := `WITH updated AS (UPDATE tool_definitions SET upstream_drifted=$3,updated_at=now() WHERE product_id=$1 AND id=$2 AND backend_kind='mcp' RETURNING id) ` + toolSelect + ` WHERE t.id IN (SELECT id FROM updated)`
	return scanTool(p.pool.QueryRow(ctx, query, productID, id, drifted))
}

func (p *Postgres) PublishTool(ctx context.Context, productID, id string, expected int64, actorID string) (model.Tool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Tool{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var organisationID, scope, ownerIntegrationID, backendKind, connectionID, mcpConnectionID, upstreamToolName, upstreamSchemaHash, runtimeConnectionID, httpPath string
	var outputSchema, requestMapping, responseMapping, authorizationPolicy []byte
	var timeoutMS int
	if err := tx.QueryRow(ctx, `SELECT organisation_id::text, scope, coalesce(owner_integration_id::text,''), backend_kind, coalesce(api_connection_id::text,''), coalesce(mcp_connection_id::text,''), upstream_tool_name, upstream_schema_hash, output_schema, request_mapping, response_mapping, authorization_policy, timeout_ms, coalesce(runtime_service_connection_id::text,''), http_path FROM tool_definitions WHERE product_id = $1 AND id = $2 AND revision = $3 AND state='draft' FOR UPDATE`, productID, id, expected).Scan(&organisationID, &scope, &ownerIntegrationID, &backendKind, &connectionID, &mcpConnectionID, &upstreamToolName, &upstreamSchemaHash, &outputSchema, &requestMapping, &responseMapping, &authorizationPolicy, &timeoutMS, &runtimeConnectionID, &httpPath); err != nil {
		return model.Tool{}, databaseError(err)
	}
	if runtimeConnectionID != "" {
		var lockedOwner, state string
		if err := tx.QueryRow(ctx, `SELECT integration_id::text,state FROM runtime_service_connections WHERE id=$1 AND deployment_id=$2 AND organisation_id=$3 FOR SHARE`, runtimeConnectionID, productID, organisationID).Scan(&lockedOwner, &state); err != nil {
			return model.Tool{}, databaseError(err)
		}
		if scope != model.ToolScopeAPI || lockedOwner != ownerIntegrationID || state != "active" || connectionID != "" {
			return model.Tool{}, ErrConflict
		}
	}
	var releaseID string
	if err := tx.QueryRow(ctx, `INSERT INTO tool_releases(organisation_id,product_id,tool_definition_id,api_connection_id,version,request_mapping,output_schema,response_mapping,authorization_policy,timeout_ms,rate_limit,published_by,published_at,backend_kind,mcp_connection_id,upstream_tool_name,upstream_schema_hash,scope,owner_integration_id,runtime_service_connection_id,http_path)
		VALUES($1,$2,$3,nullif($4,'')::uuid,$5,$6,$7,$8,$9,$10,'{"requests":60,"window_seconds":60}',nullif($11,'')::uuid,now(),$12,nullif($13,'')::uuid,$14,$15,$16,nullif($17,'')::uuid,nullif($18,'')::uuid,$19)
		RETURNING id::text`, organisationID, productID, id, connectionID, expected+1, requestMapping, outputSchema, responseMapping, authorizationPolicy, timeoutMS, actorID, backendKind, mcpConnectionID, upstreamToolName, upstreamSchemaHash, scope, ownerIntegrationID, runtimeConnectionID, httpPath).Scan(&releaseID); err != nil {
		return model.Tool{}, databaseError(err)
	}
	if runtimeConnectionID != "" {
		result, err := tx.Exec(ctx, `INSERT INTO tool_release_runtime_targets(tool_release_id,environment_id,connection_revision_id,runtime_service_connection_id)
			SELECT $1,environment_id,id,connection_id FROM runtime_service_connection_revisions WHERE connection_id=$2 AND is_current`, releaseID, runtimeConnectionID)
		if err != nil {
			return model.Tool{}, databaseError(err)
		}
		if result.RowsAffected() == 0 {
			return model.Tool{}, ErrConflict
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE tool_definitions SET state = 'published', revision = revision + 1, updated_at = now() WHERE product_id=$1 AND id=$2 AND revision=$3 AND state='draft'`, productID, id, expected); err != nil {
		return model.Tool{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Tool{}, err
	}
	return p.Tool(ctx, productID, id)
}

func (p *Postgres) CreateToolTestConfirmation(ctx context.Context, value model.ToolTestConfirmation) error {
	result, err := p.pool.Exec(ctx, `INSERT INTO tool_test_confirmations(id,organisation_id,product_id,tool_id,tool_revision,argument_hash,nonce_digest,actor_id,expires_at,created_at)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10
		FROM tool_definitions
		WHERE product_id=$3 AND id=$4 AND revision=$5`, value.ID, value.OrganisationID, value.ProductID, value.ToolID, value.ToolRevision, value.ArgumentHash, value.NonceDigest, value.ActorID, value.ExpiresAt, value.CreatedAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (p *Postgres) ConsumeToolTestConfirmation(ctx context.Context, digest []byte, productID, toolID string, revision int64, argumentHash []byte, actorID, consumptionID string, now time.Time) (model.ToolTestConfirmation, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.ToolTestConfirmation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var value model.ToolTestConfirmation
	err = tx.QueryRow(ctx, `SELECT c.id::text,c.organisation_id::text,c.product_id::text,c.tool_id::text,c.tool_revision,c.argument_hash,c.nonce_digest,c.actor_id,c.expires_at,c.created_at
		FROM tool_test_confirmations c
		JOIN tool_definitions t ON t.product_id=c.product_id AND t.id=c.tool_id AND t.revision=c.tool_revision
		WHERE c.nonce_digest=$1 AND c.product_id=$2 AND c.tool_id=$3 AND c.tool_revision=$4 AND c.argument_hash=$5 AND c.actor_id=$6 AND c.expires_at>$7
		FOR UPDATE OF c`, digest, productID, toolID, revision, argumentHash, actorID, now).Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.ToolID, &value.ToolRevision, &value.ArgumentHash, &value.NonceDigest, &value.ActorID, &value.ExpiresAt, &value.CreatedAt)
	if err != nil {
		return model.ToolTestConfirmation{}, databaseError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tool_test_confirmation_consumptions(id,confirmation_id,consumed_at) VALUES($1,$2,$3)`, consumptionID, value.ID, now); err != nil {
		return model.ToolTestConfirmation{}, databaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ToolTestConfirmation{}, err
	}
	return value, nil
}

func (p *Postgres) CreateManagedOperationConfirmation(ctx context.Context, value model.ManagedOperationConfirmation) error {
	result, err := p.pool.Exec(ctx, `INSERT INTO managed_operation_confirmations(id,organisation_id,product_id,operation_key,argument_hash,nonce_digest,actor_id,expires_at,created_at)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9
		FROM products
		WHERE id=$3 AND organisation_id=$2`, value.ID, value.OrganisationID, value.ProductID, value.OperationKey, value.ArgumentHash, value.NonceDigest, value.ActorID, value.ExpiresAt, value.CreatedAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (p *Postgres) ConsumeManagedOperationConfirmation(ctx context.Context, digest []byte, productID, operationKey string, argumentHash []byte, actorID, consumptionID string, now time.Time) (model.ManagedOperationConfirmation, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.ManagedOperationConfirmation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var value model.ManagedOperationConfirmation
	err = tx.QueryRow(ctx, `SELECT id::text,organisation_id::text,product_id::text,operation_key,argument_hash,nonce_digest,actor_id,expires_at,created_at
		FROM managed_operation_confirmations
		WHERE nonce_digest=$1 AND product_id=$2 AND operation_key=$3 AND argument_hash=$4 AND actor_id=$5 AND expires_at>$6
		FOR UPDATE`, digest, productID, operationKey, argumentHash, actorID, now).Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.OperationKey, &value.ArgumentHash, &value.NonceDigest, &value.ActorID, &value.ExpiresAt, &value.CreatedAt)
	if err != nil {
		return model.ManagedOperationConfirmation{}, databaseError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO managed_operation_confirmation_consumptions(id,confirmation_id,consumed_at) VALUES($1,$2,$3)`, consumptionID, value.ID, now); err != nil {
		return model.ManagedOperationConfirmation{}, databaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ManagedOperationConfirmation{}, err
	}
	return value, nil
}

func (p *Postgres) DeleteExpiredToolTestData(ctx context.Context, now time.Time, limit int) (int64, error) {
	limit = boundedToolTestCleanupLimit(limit)
	if limit == 0 {
		return 0, nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	confirmationResult, err := tx.Exec(ctx, `WITH expired AS (
		SELECT id FROM tool_test_confirmations WHERE expires_at<=$1 ORDER BY expires_at,id LIMIT $2 FOR UPDATE SKIP LOCKED
	) DELETE FROM tool_test_confirmations AS target USING expired WHERE target.id=expired.id`, now, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	managedConfirmationResult, err := tx.Exec(ctx, `WITH expired AS (
		SELECT id FROM managed_operation_confirmations WHERE expires_at<=$1 ORDER BY expires_at,id LIMIT $2 FOR UPDATE SKIP LOCKED
	) DELETE FROM managed_operation_confirmations AS target USING expired WHERE target.id=expired.id`, now, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	runResult, err := tx.Exec(ctx, `WITH expired AS (
		SELECT id FROM tool_test_runs WHERE expires_at<=$1 ORDER BY expires_at,id LIMIT $2 FOR UPDATE SKIP LOCKED
	) DELETE FROM tool_test_runs AS target USING expired WHERE target.id=expired.id`, now, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return confirmationResult.RowsAffected() + managedConfirmationResult.RowsAffected() + runResult.RowsAffected(), nil
}

func (p *Postgres) AppendToolTestRun(ctx context.Context, value model.ToolTestRun) error {
	requestShape, err := json.Marshal(value.RequestShape)
	if err != nil {
		return err
	}
	responseShape, err := json.Marshal(value.ResponseShape)
	if err != nil {
		return err
	}
	findings, err := json.Marshal(value.Findings)
	if err != nil {
		return err
	}
	result, err := p.pool.Exec(ctx, `INSERT INTO tool_test_runs(id,organisation_id,product_id,tool_id,tool_revision,tool_name,actor_id,request_id,argument_hash,method,authentication_type,outcome,phase,network_call_performed,upstream_status_code,response_bytes,request_shape,response_shape,findings,duration_ms,expires_at,created_at)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,nullif($15,0),$16,$17,CASE WHEN $18::jsonb='null'::jsonb THEN NULL ELSE $18::jsonb END,$19,$20,$21,$22
		FROM tool_definitions
		WHERE product_id=$3 AND id=$4`, value.ID, value.OrganisationID, value.ProductID, value.ToolID, value.ToolRevision, value.ToolName, value.ActorID, value.RequestID, value.ArgumentHash, value.Method, value.AuthenticationType, value.Outcome, value.Phase, value.NetworkCallPerformed, value.UpstreamStatusCode, value.ResponseBytes, requestShape, responseShape, findings, value.DurationMS, value.ExpiresAt, value.CreatedAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func scanToolTestRun(row interface{ Scan(...any) error }) (model.ToolTestRun, error) {
	var value model.ToolTestRun
	var requestShape, responseShape, findings []byte
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.ToolID, &value.ToolRevision, &value.ToolName, &value.ActorID, &value.RequestID, &value.ArgumentHash, &value.Method, &value.AuthenticationType, &value.Outcome, &value.Phase, &value.NetworkCallPerformed, &value.UpstreamStatusCode, &value.ResponseBytes, &requestShape, &responseShape, &findings, &value.DurationMS, &value.ExpiresAt, &value.CreatedAt)
	if err != nil {
		return model.ToolTestRun{}, databaseError(err)
	}
	if err := json.Unmarshal(requestShape, &value.RequestShape); err != nil {
		return model.ToolTestRun{}, err
	}
	if len(responseShape) > 0 && string(responseShape) != "null" {
		var shape model.JSONShape
		if err := json.Unmarshal(responseShape, &shape); err != nil {
			return model.ToolTestRun{}, err
		}
		value.ResponseShape = &shape
	}
	if err := json.Unmarshal(findings, &value.Findings); err != nil {
		return model.ToolTestRun{}, err
	}
	return value, nil
}

func (p *Postgres) ToolTestRuns(ctx context.Context, productID, toolID string, now time.Time) ([]model.ToolTestRun, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text,organisation_id::text,product_id::text,tool_id::text,tool_revision,tool_name,actor_id,request_id,argument_hash,method,authentication_type,outcome,phase,network_call_performed,coalesce(upstream_status_code,0),coalesce(response_bytes,0),request_shape,coalesce(response_shape,'null'::jsonb),findings,duration_ms,expires_at,created_at
		FROM tool_test_runs WHERE product_id=$1 AND ($2='' OR tool_id::text=$2) AND expires_at>$3 ORDER BY created_at DESC LIMIT 100`, productID, toolID, now)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.ToolTestRun, 0)
	for rows.Next() {
		value, err := scanToolTestRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) ToolTestRun(ctx context.Context, productID, toolID, runID string, now time.Time) (model.ToolTestRun, error) {
	return scanToolTestRun(p.pool.QueryRow(ctx, `SELECT id::text,organisation_id::text,product_id::text,tool_id::text,tool_revision,tool_name,actor_id,request_id,argument_hash,method,authentication_type,outcome,phase,network_call_performed,coalesce(upstream_status_code,0),coalesce(response_bytes,0),request_shape,coalesce(response_shape,'null'::jsonb),findings,duration_ms,expires_at,created_at
		FROM tool_test_runs WHERE product_id=$1 AND tool_id=$2 AND id=$3 AND expires_at>$4`, productID, toolID, runID, now))
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

func scanReportSubmission(row interface{ Scan(...any) error }) (model.ReportSubmission, error) {
	var value model.ReportSubmission
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.IntegrationID, &value.IntegrationSnapshot, &value.SupportRouteID, &value.Kind, &value.State, &value.ActorPseudonym, &value.IdempotencyDigest, &value.PayloadCiphertext, &value.PayloadNonce, &value.PayloadKeyVersion, &value.PayloadFingerprint, &value.Attempts, &value.NextAttemptAt, &value.DeliveryStartedAt, &value.LastError, &value.ExternalID, &value.ExternalURL, &value.DeliveredAt, &value.ExpiresAt, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const reportSubmissionColumns = `id::text, organisation_id::text, product_id::text, coalesce(integration_id::text,''), integration_snapshot, coalesce(support_route_id::text,''), kind, state, actor_pseudonym, idempotency_digest, payload_ciphertext, payload_nonce, payload_key_version, payload_fingerprint, attempts, next_attempt_at, delivery_started_at, last_error, external_id, external_url, delivered_at, expires_at, created_at, updated_at`
const reportSubmissionSelect = `SELECT ` + reportSubmissionColumns + ` FROM report_submissions`

func (p *Postgres) ReportSubmissions(ctx context.Context, productID, startingAfter string, limit int) ([]model.ReportSubmission, bool, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := reportSubmissionSelect + ` WHERE product_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2`
	arguments := []any{productID, limit + 1}
	if startingAfter != "" {
		var cursorCreatedAt time.Time
		if err := p.pool.QueryRow(ctx, `SELECT created_at FROM report_submissions WHERE product_id=$1 AND id::text=$2`, productID, startingAfter).Scan(&cursorCreatedAt); err != nil {
			return nil, false, databaseError(err)
		}
		query = reportSubmissionSelect + ` WHERE product_id=$1 AND (created_at,id::text)<($3,$4) ORDER BY created_at DESC,id DESC LIMIT $2`
		arguments = append(arguments, cursorCreatedAt, startingAfter)
	}
	rows, err := p.pool.Query(ctx, query, arguments...)
	if err != nil {
		return nil, false, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.ReportSubmission, 0)
	for rows.Next() {
		value, scanErr := scanReportSubmission(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

func (p *Postgres) ReportSubmission(ctx context.Context, productID, id string) (model.ReportSubmission, error) {
	return scanReportSubmission(p.pool.QueryRow(ctx, reportSubmissionSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateReportSubmission(ctx context.Context, value model.ReportSubmission) (model.ReportSubmission, error) {
	return scanReportSubmission(p.pool.QueryRow(ctx, `INSERT INTO report_submissions(id,organisation_id,product_id,integration_id,integration_snapshot,support_route_id,kind,state,actor_pseudonym,idempotency_digest,payload_ciphertext,payload_nonce,payload_key_version,payload_fingerprint,next_attempt_at,expires_at)
		VALUES ($1,$2,$3,nullif($4,'')::uuid,$5,nullif($6,'')::uuid,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (product_id,actor_pseudonym,kind,idempotency_digest) DO UPDATE SET updated_at=report_submissions.updated_at
		RETURNING `+reportSubmissionColumns, value.ID, value.OrganisationID, value.ProductID, value.IntegrationID, value.IntegrationSnapshot, value.SupportRouteID, value.Kind, value.State, value.ActorPseudonym, value.IdempotencyDigest, value.PayloadCiphertext, value.PayloadNonce, value.PayloadKeyVersion, value.PayloadFingerprint, value.NextAttemptAt, value.ExpiresAt))
}

func (p *Postgres) ActivateHeldReportSubmissions(ctx context.Context, productID, routeID, kind string, now time.Time) error {
	_, err := p.pool.Exec(ctx, `UPDATE report_submissions SET state='pending', next_attempt_at=$4, updated_at=$4 WHERE product_id=$1 AND support_route_id=$2 AND kind=$3 AND state='held' AND expires_at>$4`, productID, routeID, kind, now)
	return databaseError(err)
}

func (p *Postgres) ClaimReportSubmissions(ctx context.Context, now time.Time, limit int) ([]model.ReportSubmission, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := p.pool.Query(ctx, `WITH ready AS (
		SELECT id FROM report_submissions
		WHERE expires_at>$1 AND ((state='pending' AND (next_attempt_at IS NULL OR next_attempt_at<=$1)) OR (state='delivering' AND delivery_started_at<$1-interval '5 minutes'))
		ORDER BY coalesce(next_attempt_at,created_at), created_at
		FOR UPDATE SKIP LOCKED LIMIT $2
	) UPDATE report_submissions s SET state='delivering', attempts=s.attempts+1, delivery_started_at=$1, updated_at=$1 FROM ready WHERE s.id=ready.id
	RETURNING s.id::text, s.organisation_id::text, s.product_id::text, coalesce(s.integration_id::text,''), s.integration_snapshot, coalesce(s.support_route_id::text,''), s.kind, s.state, s.actor_pseudonym, s.idempotency_digest, s.payload_ciphertext, s.payload_nonce, s.payload_key_version, s.payload_fingerprint, s.attempts, s.next_attempt_at, s.delivery_started_at, s.last_error, s.external_id, s.external_url, s.delivered_at, s.expires_at, s.created_at, s.updated_at`, now, limit)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.ReportSubmission, 0)
	for rows.Next() {
		value, scanErr := scanReportSubmission(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) UpdateReportSubmissionDelivery(ctx context.Context, value model.ReportSubmission) (model.ReportSubmission, error) {
	return scanReportSubmission(p.pool.QueryRow(ctx, `UPDATE report_submissions SET state=$3, attempts=$4, next_attempt_at=$5, delivery_started_at=$6, last_error=$7, external_id=$8, external_url=$9, delivered_at=$10, updated_at=now() WHERE product_id=$1 AND id=$2
	RETURNING `+reportSubmissionColumns, value.ProductID, value.ID, value.State, value.Attempts, value.NextAttemptAt, value.DeliveryStartedAt, value.LastError, value.ExternalID, value.ExternalURL, value.DeliveredAt))
}

func (p *Postgres) RetryReportSubmission(ctx context.Context, productID, id string, now time.Time) (model.ReportSubmission, error) {
	value, err := scanReportSubmission(p.pool.QueryRow(ctx, `UPDATE report_submissions SET state='pending', next_attempt_at=$3, delivery_started_at=NULL, last_error='', updated_at=$3 WHERE product_id=$1 AND id=$2 AND state IN ('held','failed') AND expires_at>$3
	RETURNING `+reportSubmissionColumns, productID, id, now))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.ReportSubmission(ctx, productID, id); lookupErr == nil {
			return model.ReportSubmission{}, ErrConflict
		}
	}
	return value, err
}

func (p *Postgres) DeleteExpiredReportSubmissions(ctx context.Context, now time.Time) (int64, error) {
	result, err := p.pool.Exec(ctx, `DELETE FROM report_submissions WHERE expires_at<=$1`, now)
	if err != nil {
		return 0, databaseError(err)
	}
	return result.RowsAffected(), nil
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

func scanIdentityProvider(row interface{ Scan(...any) error }) (identity.ProviderConfig, error) {
	var value identity.ProviderConfig
	err := row.Scan(&value.ID, &value.OrganisationID, &value.DeploymentID, &value.Issuer, &value.ClientID, &value.ClientSecretID, &value.Scopes, &value.Audience, &value.OAuthResource, &value.OrganisationClaim, &value.InstallationClaim, &value.DelegatedAPIOrigin, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const identityProviderSelect = `SELECT id::text, organisation_id::text, product_id::text, issuer, client_id, coalesce(client_secret_id::text, ''), scopes, audience, oauth_resource, organisation_claim, installation_claim, delegated_api_origin, state, revision, created_at, updated_at FROM identity_providers`

func (p *Postgres) IdentityProvider(ctx context.Context, deploymentID string) (identity.ProviderConfig, error) {
	return scanIdentityProvider(p.pool.QueryRow(ctx, identityProviderSelect+` WHERE product_id = $1`, deploymentID))
}

func (p *Postgres) SaveIdentityProvider(ctx context.Context, value identity.ProviderConfig) (identity.ProviderConfig, error) {
	var updated identity.ProviderConfig
	var err error
	if value.Revision == 0 {
		updated, err = scanIdentityProvider(p.pool.QueryRow(ctx, `INSERT INTO identity_providers(id, organisation_id, product_id, issuer, client_id, client_secret_id, scopes, audience, oauth_resource, organisation_claim, installation_claim, delegated_api_origin, state) VALUES ($1,$2,$3,$4,$5,nullif($6,'')::uuid,$7,$8,$9,$10,$11,$12,$13) RETURNING id::text, organisation_id::text, product_id::text, issuer, client_id, coalesce(client_secret_id::text, ''), scopes, audience, oauth_resource, organisation_claim, installation_claim, delegated_api_origin, state, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.DeploymentID, value.Issuer, value.ClientID, value.ClientSecretID, value.Scopes, value.Audience, value.OAuthResource, value.OrganisationClaim, value.InstallationClaim, value.DelegatedAPIOrigin, value.State))
	} else {
		updated, err = scanIdentityProvider(p.pool.QueryRow(ctx, `UPDATE identity_providers SET issuer=$2,client_id=$3,client_secret_id=nullif($4,'')::uuid,scopes=$5,audience=$6,oauth_resource=$7,organisation_claim=$8,installation_claim=$9,delegated_api_origin=$10,state=$11,revision=revision+1,updated_at=now() WHERE product_id=$1 AND revision=$12 RETURNING id::text, organisation_id::text, product_id::text, issuer, client_id, coalesce(client_secret_id::text, ''), scopes, audience, oauth_resource, organisation_claim, installation_claim, delegated_api_origin, state, revision, created_at, updated_at`, value.DeploymentID, value.Issuer, value.ClientID, value.ClientSecretID, value.Scopes, value.Audience, value.OAuthResource, value.OrganisationClaim, value.InstallationClaim, value.DelegatedAPIOrigin, value.State, value.Revision))
	}
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.IdentityProvider(ctx, value.DeploymentID); lookupErr == nil {
			return identity.ProviderConfig{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) DeleteIdentityProvider(ctx context.Context, deploymentID string, expectedRevision int64) (identity.ProviderConfig, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return identity.ProviderConfig{}, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := scanIdentityProvider(tx.QueryRow(ctx, identityProviderSelect+` WHERE product_id=$1 AND revision=$2 AND state='disabled' FOR UPDATE`, deploymentID, expectedRevision))
	if errors.Is(err, ErrNotFound) {
		var exists bool
		if lookupErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity_providers WHERE product_id=$1)`, deploymentID).Scan(&exists); lookupErr != nil {
			return identity.ProviderConfig{}, databaseError(lookupErr)
		}
		if exists {
			return identity.ProviderConfig{}, ErrConflict
		}
		return identity.ProviderConfig{}, ErrNotFound
	}
	if err != nil {
		return identity.ProviderConfig{}, err
	}
	secretIDs := map[string]bool{}
	if current.ClientSecretID != "" {
		secretIDs[current.ClientSecretID] = true
	}
	if _, err := tx.Exec(ctx, `DELETE FROM oauth_states WHERE product_id=$1`, deploymentID); err != nil {
		return identity.ProviderConfig{}, databaseError(err)
	}
	for _, table := range []string{"oauth_authorization_codes", "oauth_access_tokens"} {
		rows, queryErr := tx.Query(ctx, `DELETE FROM `+table+` WHERE product_id=$1 RETURNING upstream_access_secret_id::text`, deploymentID)
		if queryErr != nil {
			return identity.ProviderConfig{}, databaseError(queryErr)
		}
		for rows.Next() {
			var secretID string
			if scanErr := rows.Scan(&secretID); scanErr != nil {
				rows.Close()
				return identity.ProviderConfig{}, databaseError(scanErr)
			}
			secretIDs[secretID] = true
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return identity.ProviderConfig{}, databaseError(rowsErr)
		}
		rows.Close()
	}
	if _, err := tx.Exec(ctx, `DELETE FROM identity_provider_tests WHERE product_id=$1`, deploymentID); err != nil {
		return identity.ProviderConfig{}, databaseError(err)
	}
	if result, err := tx.Exec(ctx, `DELETE FROM identity_providers WHERE product_id=$1 AND revision=$2 AND state='disabled'`, deploymentID, expectedRevision); err != nil {
		return identity.ProviderConfig{}, databaseError(err)
	} else if result.RowsAffected() != 1 {
		return identity.ProviderConfig{}, ErrConflict
	}
	if len(secretIDs) > 0 {
		ids := make([]string, 0, len(secretIDs))
		for id := range secretIDs {
			ids = append(ids, id)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM secrets secret WHERE secret.id=ANY($1::uuid[]) AND NOT EXISTS (SELECT 1 FROM identity_providers provider WHERE provider.client_secret_id=secret.id) AND NOT EXISTS (SELECT 1 FROM oauth_authorization_codes code WHERE code.upstream_access_secret_id=secret.id) AND NOT EXISTS (SELECT 1 FROM oauth_access_tokens token WHERE token.upstream_access_secret_id=secret.id)`, ids); err != nil {
			return identity.ProviderConfig{}, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return identity.ProviderConfig{}, databaseError(err)
	}
	return current, nil
}

func scanIdentityProviderTest(row interface{ Scan(...any) error }) (identity.ProviderTest, error) {
	var value identity.ProviderTest
	err := row.Scan(&value.ID, &value.OrganisationID, &value.DeploymentID, &value.ConfigurationRevision, &value.StateDigest, &value.UpstreamVerifier, &value.Nonce, &value.Status, &value.FailureCode, &value.Issuer, &value.Subject, &value.CustomerID, &value.CallbackClaimedAt, &value.CreatedAt, &value.ExpiresAt, &value.CompletedAt)
	return value, databaseError(err)
}

const identityProviderTestSelect = `SELECT id::text,organisation_id::text,product_id::text,configuration_revision,state_digest,upstream_verifier,nonce,status,failure_code,issuer,subject,customer_id,callback_claimed_at,created_at,expires_at,completed_at FROM identity_provider_tests`

func (p *Postgres) CreateIdentityProviderTest(ctx context.Context, value identity.ProviderTest) error {
	result, err := p.pool.Exec(ctx, `INSERT INTO identity_provider_tests(id,organisation_id,product_id,configuration_revision,state_digest,upstream_verifier,nonce,status,failure_code,issuer,subject,customer_id,callback_claimed_at,created_at,expires_at,completed_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16 FROM identity_providers provider WHERE provider.product_id=$3 AND provider.revision=$4 AND provider.state IN ('disabled','active') FOR UPDATE OF provider`, value.ID, value.OrganisationID, value.DeploymentID, value.ConfigurationRevision, value.StateDigest, value.UpstreamVerifier, value.Nonce, value.Status, value.FailureCode, value.Issuer, value.Subject, value.CustomerID, value.CallbackClaimedAt, value.CreatedAt, value.ExpiresAt, value.CompletedAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func (p *Postgres) IdentityProviderTest(ctx context.Context, deploymentID, id string) (identity.ProviderTest, error) {
	return scanIdentityProviderTest(p.pool.QueryRow(ctx, identityProviderTestSelect+` WHERE product_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) ClaimIdentityProviderTestByStateDigest(ctx context.Context, digest []byte, now time.Time) (identity.ProviderTest, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return identity.ProviderTest{}, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	expired, err := tx.Exec(ctx, `UPDATE identity_provider_tests SET status='expired',failure_code='test_expired',upstream_verifier='',nonce='',subject='',customer_id='',completed_at=$2 WHERE state_digest=$1 AND status='pending' AND callback_claimed_at IS NULL AND expires_at<=$2`, digest, now)
	if err != nil {
		return identity.ProviderTest{}, databaseError(err)
	}
	if expired.RowsAffected() > 0 {
		if err := tx.Commit(ctx); err != nil {
			return identity.ProviderTest{}, databaseError(err)
		}
		return identity.ProviderTest{}, ErrConflict
	}
	claimed, err := scanIdentityProviderTest(tx.QueryRow(ctx, `UPDATE identity_provider_tests SET callback_claimed_at=$2 WHERE state_digest=$1 AND status='pending' AND callback_claimed_at IS NULL AND expires_at>$2 RETURNING id::text,organisation_id::text,product_id::text,configuration_revision,state_digest,upstream_verifier,nonce,status,failure_code,issuer,subject,customer_id,callback_claimed_at,created_at,expires_at,completed_at`, digest, now))
	if err != nil {
		return identity.ProviderTest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return identity.ProviderTest{}, databaseError(err)
	}
	return claimed, nil
}

func (p *Postgres) LatestIdentityProviderTest(ctx context.Context, deploymentID string) (identity.ProviderTest, error) {
	return scanIdentityProviderTest(p.pool.QueryRow(ctx, identityProviderTestSelect+` WHERE product_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, deploymentID))
}

func (p *Postgres) CompleteIdentityProviderTest(ctx context.Context, value identity.ProviderTest) (identity.ProviderTest, error) {
	updated, err := scanIdentityProviderTest(p.pool.QueryRow(ctx, `UPDATE identity_provider_tests SET status=$4,failure_code=$5,issuer=$6,subject=$7,customer_id=$8,upstream_verifier='',nonce='',completed_at=$9 WHERE id=$1 AND product_id=$2 AND configuration_revision=$3 AND status='pending' RETURNING id::text,organisation_id::text,product_id::text,configuration_revision,state_digest,upstream_verifier,nonce,status,failure_code,issuer,subject,customer_id,callback_claimed_at,created_at,expires_at,completed_at`, value.ID, value.DeploymentID, value.ConfigurationRevision, value.Status, value.FailureCode, value.Issuer, value.Subject, value.CustomerID, value.CompletedAt))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.IdentityProviderTest(ctx, value.DeploymentID, value.ID); lookupErr == nil {
			return identity.ProviderTest{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) ExpireIdentityProviderTests(ctx context.Context, deploymentID string, now time.Time) error {
	_, err := p.pool.Exec(ctx, `UPDATE identity_provider_tests SET status=CASE WHEN status='pending' THEN 'expired' ELSE status END,failure_code=CASE WHEN status='pending' THEN 'test_expired' ELSE failure_code END,upstream_verifier='',nonce='',subject='',customer_id='',completed_at=CASE WHEN status='pending' THEN $2 ELSE completed_at END WHERE product_id=$1 AND expires_at<=$2 AND (status<>'pending' OR callback_claimed_at IS NULL OR callback_claimed_at <= $2 - interval '2 minutes')`, deploymentID, now)
	return databaseError(err)
}

func scanOAuthClient(row interface{ Scan(...any) error }) (identity.OAuthClient, error) {
	var value identity.OAuthClient
	err := row.Scan(&value.ClientID, &value.DeploymentID, &value.ClientName, &value.RedirectURIs, &value.CreatedAt)
	return value, databaseError(err)
}

const oauthClientSelect = `SELECT client_id, deployment_id::text, client_name, redirect_uris, created_at FROM mcp_oauth_clients`

func (p *Postgres) OAuthClient(ctx context.Context, deploymentID, clientID string) (identity.OAuthClient, error) {
	return scanOAuthClient(p.pool.QueryRow(ctx, oauthClientSelect+` WHERE deployment_id=$1 AND client_id=$2`, deploymentID, clientID))
}

func (p *Postgres) CreateOAuthClient(ctx context.Context, value identity.OAuthClient) (identity.OAuthClient, error) {
	created, err := scanOAuthClient(p.pool.QueryRow(ctx, `INSERT INTO mcp_oauth_clients(client_id,deployment_id,client_name,redirect_uris) VALUES($1,$2,$3,$4) ON CONFLICT(client_id) DO UPDATE SET client_id=excluded.client_id WHERE mcp_oauth_clients.deployment_id=excluded.deployment_id AND mcp_oauth_clients.client_name=excluded.client_name AND mcp_oauth_clients.redirect_uris=excluded.redirect_uris RETURNING client_id,deployment_id::text,client_name,redirect_uris,created_at`, value.ClientID, value.DeploymentID, value.ClientName, value.RedirectURIs))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.OAuthClient(ctx, value.DeploymentID, value.ClientID); lookupErr == nil {
			return identity.OAuthClient{}, ErrConflict
		}
	}
	return created, err
}

func scanCustomerAccount(row interface{ Scan(...any) error }) (identity.CustomerAccount, error) {
	var value identity.CustomerAccount
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Issuer, &value.ExternalID, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt, &value.LastAuthenticatedAt)
	return value, databaseError(err)
}

const customerAccountSelect = `SELECT id::text,organisation_id::text,product_id::text,issuer,external_id,state,revision,created_at,updated_at,last_authenticated_at FROM customer_accounts`

func (p *Postgres) ResolveCustomerAccount(ctx context.Context, value identity.CustomerAccount) (identity.CustomerAccount, error) {
	return scanCustomerAccount(p.pool.QueryRow(ctx, `INSERT INTO customer_accounts(id,organisation_id,product_id,issuer,external_id,state,last_authenticated_at) VALUES ($1,$2,$3,$4,$5,'active',$6) ON CONFLICT(product_id,issuer,external_id) DO UPDATE SET last_authenticated_at=excluded.last_authenticated_at,updated_at=excluded.last_authenticated_at RETURNING id::text,organisation_id::text,product_id::text,issuer,external_id,state,revision,created_at,updated_at,last_authenticated_at`, value.ID, value.OrganisationID, value.ProductID, value.Issuer, value.ExternalID, value.LastAuthenticatedAt))
}

func (p *Postgres) CustomerAccounts(ctx context.Context, productID, startingAfter string, limit int) ([]identity.CustomerAccount, bool, error) {
	if _, err := p.Product(ctx, productID); err != nil {
		return nil, false, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := customerAccountSelect + ` WHERE product_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2`
	arguments := []any{productID, limit + 1}
	if startingAfter != "" {
		var cursorCreatedAt time.Time
		if err := p.pool.QueryRow(ctx, `SELECT created_at FROM customer_accounts WHERE product_id=$1 AND id::text=$2`, productID, startingAfter).Scan(&cursorCreatedAt); err != nil {
			return nil, false, databaseError(err)
		}
		query = customerAccountSelect + ` WHERE product_id=$1 AND (created_at,id::text)<($3,$4) ORDER BY created_at DESC,id DESC LIMIT $2`
		arguments = append(arguments, cursorCreatedAt, startingAfter)
	}
	rows, err := p.pool.Query(ctx, query, arguments...)
	if err != nil {
		return nil, false, databaseError(err)
	}
	defer rows.Close()
	result := make([]identity.CustomerAccount, 0)
	for rows.Next() {
		value, scanErr := scanCustomerAccount(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

func (p *Postgres) CustomerAccount(ctx context.Context, productID, id string) (identity.CustomerAccount, error) {
	return scanCustomerAccount(p.pool.QueryRow(ctx, customerAccountSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) UpdateCustomerAccount(ctx context.Context, value identity.CustomerAccount, expected int64) (identity.CustomerAccount, error) {
	updated, err := scanCustomerAccount(p.pool.QueryRow(ctx, `UPDATE customer_accounts SET state=$3,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$4 RETURNING id::text,organisation_id::text,product_id::text,issuer,external_id,state,revision,created_at,updated_at,last_authenticated_at`, value.ProductID, value.ID, value.State, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.CustomerAccount(ctx, value.ProductID, value.ID); lookupErr == nil {
			return identity.CustomerAccount{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) CreateOAuthState(ctx context.Context, value identity.OAuthState) error {
	result, err := p.pool.Exec(ctx, `INSERT INTO oauth_states(state_digest, product_id, provider_revision, client_id, redirect_uri, resource, scopes, downstream_state, downstream_challenge, upstream_verifier, nonce, expires_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12 FROM identity_providers provider WHERE provider.product_id=$2 AND provider.state='active' AND provider.revision=$3 FOR UPDATE OF provider`, value.Digest, value.ProductID, value.ProviderRevision, value.ClientID, value.RedirectURI, value.Resource, value.Scopes, value.DownstreamState, value.DownstreamChallenge, value.UpstreamVerifier, value.Nonce, value.ExpiresAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func scanOAuthState(row pgx.Row) (identity.OAuthState, error) {
	var value identity.OAuthState
	err := row.Scan(&value.Digest, &value.ProductID, &value.ProviderRevision, &value.ClientID, &value.RedirectURI, &value.Resource, &value.Scopes, &value.DownstreamState, &value.DownstreamChallenge, &value.UpstreamVerifier, &value.Nonce, &value.ExpiresAt)
	return value, databaseError(err)
}

func (p *Postgres) ConsumeOAuthState(ctx context.Context, digest []byte) (identity.OAuthState, error) {
	return scanOAuthState(p.pool.QueryRow(ctx, `DELETE FROM oauth_states WHERE state_digest = $1 RETURNING state_digest, product_id::text, provider_revision, client_id, redirect_uri, resource, scopes, downstream_state, downstream_challenge, upstream_verifier, nonce, expires_at`, digest))
}

func (p *Postgres) CreateOAuthCode(ctx context.Context, value identity.OAuthCode) error {
	grants, _ := json.Marshal(value.Grants)
	result, err := p.pool.Exec(ctx, `INSERT INTO oauth_authorization_codes(code_digest,product_id,provider_organisation_id,provider_revision,client_id,redirect_uri,resource,scopes,downstream_challenge,issuer,subject,email,display_name,customer_account_id,external_customer_id,installation_id,grants,access_evaluation_id,access_evaluated_at,policy_version,upstream_access_secret_id,access_expires_at,expires_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23 FROM identity_providers provider WHERE provider.product_id=$2 AND provider.state='active' AND provider.revision=$4 FOR UPDATE OF provider`, value.Digest, value.ProductID, value.OrganisationID, value.ProviderRevision, value.ClientID, value.RedirectURI, value.Resource, value.Scopes, value.DownstreamChallenge, value.Issuer, value.Subject, value.Email, value.DisplayName, value.CustomerAccountID, value.ExternalCustomerID, value.InstallationID, grants, value.AccessEvaluationID, value.AccessEvaluatedAt, value.PolicyVersion, value.UpstreamAccessSecretID, value.AccessExpiresAt, value.ExpiresAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func scanOAuthCode(row pgx.Row) (identity.OAuthCode, error) {
	var value identity.OAuthCode
	var grants []byte
	err := row.Scan(&value.Digest, &value.ProductID, &value.OrganisationID, &value.ProviderRevision, &value.ClientID, &value.RedirectURI, &value.Resource, &value.Scopes, &value.DownstreamChallenge, &value.Issuer, &value.Subject, &value.Email, &value.DisplayName, &value.CustomerAccountID, &value.ExternalCustomerID, &value.InstallationID, &grants, &value.AccessEvaluationID, &value.AccessEvaluatedAt, &value.PolicyVersion, &value.UpstreamAccessSecretID, &value.AccessExpiresAt, &value.ExpiresAt)
	if err == nil {
		err = json.Unmarshal(grants, &value.Grants)
	}
	return value, databaseError(err)
}

func (p *Postgres) ConsumeOAuthCode(ctx context.Context, digest []byte) (identity.OAuthCode, error) {
	return scanOAuthCode(p.pool.QueryRow(ctx, `DELETE FROM oauth_authorization_codes WHERE code_digest = $1 RETURNING code_digest,product_id::text,provider_organisation_id::text,provider_revision,client_id,redirect_uri,resource,scopes,downstream_challenge,issuer,subject,email,display_name,customer_account_id::text,external_customer_id,installation_id,grants,access_evaluation_id,access_evaluated_at,policy_version,upstream_access_secret_id::text,access_expires_at,expires_at`, digest))
}

func (p *Postgres) CreateAccessToken(ctx context.Context, value identity.AccessToken) error {
	grants, _ := json.Marshal(value.Grants)
	result, err := p.pool.Exec(ctx, `INSERT INTO oauth_access_tokens(token_digest,product_id,provider_revision,client_id,resource,issuer,subject,email,display_name,customer_account_id,external_customer_id,installation_id,grants,access_evaluation_id,access_evaluated_at,policy_version,upstream_access_secret_id,scopes,expires_at,created_at,revoked_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21 FROM identity_providers provider WHERE provider.product_id=$2 AND provider.state='active' AND provider.revision=$3 FOR UPDATE OF provider`, value.Digest, value.ProductID, value.ProviderRevision, value.ClientID, value.Resource, value.Issuer, value.Subject, value.Email, value.DisplayName, value.CustomerAccountID, value.ExternalCustomerID, value.InstallationID, grants, value.AccessEvaluationID, value.AccessEvaluatedAt, value.PolicyVersion, value.UpstreamAccessSecretID, value.Scopes, value.ExpiresAt, value.CreatedAt, value.RevokedAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func scanAccessToken(row pgx.Row) (identity.AccessToken, error) {
	var value identity.AccessToken
	var grants []byte
	err := row.Scan(&value.Digest, &value.ProductID, &value.ProviderRevision, &value.ClientID, &value.Resource, &value.Issuer, &value.Subject, &value.Email, &value.DisplayName, &value.CustomerAccountID, &value.ExternalCustomerID, &value.InstallationID, &grants, &value.AccessEvaluationID, &value.AccessEvaluatedAt, &value.PolicyVersion, &value.UpstreamAccessSecretID, &value.Scopes, &value.ExpiresAt, &value.CreatedAt, &value.RevokedAt)
	if err == nil {
		err = json.Unmarshal(grants, &value.Grants)
	}
	return value, databaseError(err)
}

func (p *Postgres) AccessTokenByDigest(ctx context.Context, digest []byte) (identity.AccessToken, error) {
	return scanAccessToken(p.pool.QueryRow(ctx, `SELECT token_digest,product_id::text,provider_revision,client_id,resource,issuer,subject,email,display_name,customer_account_id::text,external_customer_id,installation_id,grants,access_evaluation_id,access_evaluated_at,policy_version,upstream_access_secret_id::text,scopes,expires_at,created_at,revoked_at FROM oauth_access_tokens WHERE token_digest = $1`, digest))
}

func (p *Postgres) DeleteStaleOAuthArtifacts(ctx context.Context, productID string, now time.Time, limit int) (int64, error) {
	limit = boundedOAuthArtifactCleanupLimit(limit)
	if limit == 0 {
		return 0, nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stateResult, err := tx.Exec(ctx, `
		WITH doomed AS (
			SELECT state_digest
			FROM oauth_states
			WHERE product_id=$1
			  AND (expires_at <= $2 OR NOT EXISTS (
				SELECT 1 FROM identity_providers provider
				WHERE provider.product_id=$1 AND provider.state='active' AND provider.revision=oauth_states.provider_revision
			  ))
			ORDER BY expires_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM oauth_states state
		USING doomed
		WHERE state.state_digest = doomed.state_digest`, productID, now, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	deleted := stateResult.RowsAffected()
	secretIDs := make(map[string]bool)

	codeRows, err := tx.Query(ctx, `
		WITH doomed AS (
			SELECT code_digest
			FROM oauth_authorization_codes
			WHERE product_id=$1
			  AND (expires_at <= $2 OR access_expires_at <= $2 OR NOT EXISTS (
				SELECT 1 FROM identity_providers provider
				WHERE provider.product_id=$1 AND provider.state='active' AND provider.revision=oauth_authorization_codes.provider_revision
			  ))
			ORDER BY expires_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM oauth_authorization_codes code
		USING doomed
		WHERE code.code_digest = doomed.code_digest
		RETURNING code.upstream_access_secret_id::text`, productID, now, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	for codeRows.Next() {
		var secretID string
		if err := codeRows.Scan(&secretID); err != nil {
			codeRows.Close()
			return 0, databaseError(err)
		}
		secretIDs[secretID] = true
		deleted++
	}
	if err := codeRows.Err(); err != nil {
		codeRows.Close()
		return 0, databaseError(err)
	}
	codeRows.Close()

	tokenRows, err := tx.Query(ctx, `
		WITH doomed AS (
			SELECT token_digest
			FROM oauth_access_tokens
			WHERE product_id=$1
			  AND (expires_at <= $2 OR revoked_at IS NOT NULL OR NOT EXISTS (
				SELECT 1 FROM identity_providers provider
				WHERE provider.product_id=$1 AND provider.state='active' AND provider.revision=oauth_access_tokens.provider_revision
			  ))
			ORDER BY expires_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM oauth_access_tokens token
		USING doomed
		WHERE token.token_digest = doomed.token_digest
		RETURNING token.upstream_access_secret_id::text`, productID, now, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	for tokenRows.Next() {
		var secretID string
		if err := tokenRows.Scan(&secretID); err != nil {
			tokenRows.Close()
			return 0, databaseError(err)
		}
		secretIDs[secretID] = true
		deleted++
	}
	if err := tokenRows.Err(); err != nil {
		tokenRows.Close()
		return 0, databaseError(err)
	}
	tokenRows.Close()

	if len(secretIDs) > 0 {
		ids := make([]string, 0, len(secretIDs))
		for id := range secretIDs {
			ids = append(ids, id)
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM secrets secret
			WHERE secret.id = ANY($1::uuid[])
			  AND secret.purpose = 'vendor_delegated_access'
			  AND NOT EXISTS (
				SELECT 1 FROM oauth_authorization_codes code
				WHERE code.upstream_access_secret_id = secret.id
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM oauth_access_tokens token
				WHERE token.upstream_access_secret_id = secret.id
			  )`, ids); err != nil {
			return 0, databaseError(err)
		}
	}
	orphanResult, err := tx.Exec(ctx, `
		WITH doomed AS (
			SELECT secret.id
			FROM secrets secret
			WHERE secret.organisation_id = (SELECT organisation_id FROM products WHERE id=$1)
			  AND secret.created_at <= $2
			  AND (
				(secret.purpose = 'vendor_delegated_access'
				  AND NOT EXISTS (SELECT 1 FROM oauth_authorization_codes code WHERE code.upstream_access_secret_id = secret.id)
				  AND NOT EXISTS (SELECT 1 FROM oauth_access_tokens token WHERE token.upstream_access_secret_id = secret.id))
				OR
				(secret.purpose = 'identity_provider_oidc_client'
				  AND NOT EXISTS (SELECT 1 FROM identity_providers provider WHERE provider.client_secret_id = secret.id))
			  )
			ORDER BY secret.created_at, secret.id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM secrets secret
		USING doomed
		WHERE secret.id = doomed.id`, productID, now.Add(-identitySecretOrphanGrace), limit)
	if err != nil {
		return 0, databaseError(err)
	}
	deleted += orphanResult.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return 0, databaseError(err)
	}
	return deleted, nil
}

func (p *Postgres) PublicKnowledge(ctx context.Context, productID string, publicationIDs []string, query string) ([]model.KnowledgeRecord, error) {
	pattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := p.pool.Query(ctx, `
		SELECT DISTINCT kd.id::text, kd.product_id::text, kd.source_id::text, kd.title, kd.body, kd.canonical_url, s.visibility::text
		FROM source_publications sp
		JOIN source_publication_documents spd ON spd.source_publication_id = sp.id
		JOIN knowledge_documents kd ON kd.id = spd.knowledge_document_id
		JOIN sources s ON s.id = sp.source_id AND s.product_id = sp.product_id
		WHERE sp.product_id = $1 AND sp.id = ANY($2::uuid[])
		  AND sp.visibility = 'public' AND s.visibility = 'public'
		  AND s.state = 'published' AND kd.state = 'published'
		  AND kd.injection_indicators = '[]'::jsonb
		  AND ($3 = '%%' OR kd.title ILIKE $3 OR kd.body ILIKE $3)
		ORDER BY kd.id::text LIMIT 20`, productID, publicationIDs, pattern)
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

func (p *Postgres) PrivateKnowledge(ctx context.Context, productID string, publicationIDs []string, query string) ([]model.KnowledgeRecord, error) {
	pattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := p.pool.Query(ctx, `
		SELECT DISTINCT kd.id::text, kd.product_id::text, kd.source_id::text, kd.title, kd.body, kd.canonical_url, s.visibility::text
		FROM source_publications sp
		JOIN source_publication_documents spd ON spd.source_publication_id = sp.id
		JOIN knowledge_documents kd ON kd.id = spd.knowledge_document_id
		JOIN sources s ON s.id = sp.source_id AND s.product_id = sp.product_id
		WHERE sp.product_id = $1 AND sp.id = ANY($2::uuid[])
		  AND s.state = 'published' AND kd.state = 'published'
		  AND kd.injection_indicators = '[]'::jsonb
		  AND ($3 = '%%' OR kd.title ILIKE $3 OR kd.body ILIKE $3)
		ORDER BY kd.id::text LIMIT 20`, productID, publicationIDs, pattern)
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

func (p *Postgres) ProductVersionActivity(ctx context.Context, productID, versionID string, since time.Time) (model.ProductVersionActivity, error) {
	var value model.ProductVersionActivity
	err := p.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE event_name='mcp.request'), count(*) FILTER (WHERE event_name='tool.called') FROM analytics_events WHERE product_id=$1 AND created_at >= $2 AND dimensions->>'product_version_id'=$3`, productID, since, versionID).Scan(&value.Requests, &value.ToolCalls)
	return value, databaseError(err)
}

func (p *Postgres) LLMTokensUsed(ctx context.Context, productID, role string, since time.Time) (int64, error) {
	var total int64
	err := p.pool.QueryRow(ctx, `SELECT coalesce(sum(value),0)::bigint FROM analytics_events WHERE product_id=$1 AND created_at >= $2 AND event_name='llm.tokens' AND dimensions->>'role'=$3`, productID, since, role).Scan(&total)
	return total, databaseError(err)
}

func (p *Postgres) AnalyticsSummary(ctx context.Context, productID string, since time.Time) (model.AnalyticsSummary, error) {
	value := model.AnalyticsSummary{Since: since, GeneratedAt: time.Now().UTC(), Channels: map[string]int64{"private_mcp": 0, "public_mcp": 0, "widget": 0}, Versions: map[string]int64{}, Funnel: map[string]int64{"connector_authorized": 0, "run_started": 0, "capability_resolved": 0, "credentials_issued": 0, "implementation_validated": 0, "success_reported": 0}}
	err := p.pool.QueryRow(ctx, `SELECT count(DISTINCT actor_pseudonym) FILTER (WHERE actor_pseudonym IS NOT NULL), count(*) FILTER (WHERE event_name='mcp.request'), count(*) FILTER (WHERE event_name='tool.called') FROM analytics_events WHERE product_id=$1 AND created_at >= $2`, productID, since).Scan(&value.ActiveDevelopers, &value.MCPRequests, &value.ToolCalls)
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
	rows, err = p.pool.Query(ctx, `SELECT dimensions->>'product_version', count(*) FROM analytics_events WHERE product_id=$1 AND created_at >= $2 AND event_name='mcp.request' AND coalesce(dimensions->>'product_version','')<>'' GROUP BY 1`, productID, since)
	if err != nil {
		return value, databaseError(err)
	}
	for rows.Next() {
		var version string
		var count int64
		if err := rows.Scan(&version, &count); err != nil {
			rows.Close()
			return value, err
		}
		value.Versions[version] = count
	}
	rows.Close()
	rows, err = p.pool.Query(ctx, `SELECT event_name, count(*) FROM analytics_events WHERE product_id=$1 AND created_at >= $2 AND event_name = ANY($3) GROUP BY event_name`, productID, since, []string{"connector_authorized", "run_started", "capability_resolved", "credentials_issued", "implementation_validated", "success_reported"})
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

func (p *Postgres) RecipePopularity(ctx context.Context, productID string, since time.Time) ([]model.RecipePopularity, error) {
	rows, err := p.pool.Query(ctx, `SELECT dimensions->>'recipe_id', max(dimensions->>'recipe_slug'), count(*) FILTER (WHERE event_name='recipe.view'), count(*) FILTER (WHERE event_name='recipe.plan_selected')
		FROM analytics_events
		WHERE product_id=$1 AND created_at >= $2 AND event_name = ANY($3) AND coalesce(dimensions->>'recipe_id','')<>''
		GROUP BY dimensions->>'recipe_id'
		ORDER BY count(*) FILTER (WHERE event_name='recipe.plan_selected') DESC, count(*) FILTER (WHERE event_name='recipe.view') DESC, max(dimensions->>'recipe_slug')`, productID, since, []string{"recipe.view", "recipe.plan_selected"})
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.RecipePopularity, 0)
	for rows.Next() {
		var value model.RecipePopularity
		if err := rows.Scan(&value.RecipeID, &value.RecipeSlug, &value.Views, &value.PlanSelections); err != nil {
			return nil, databaseError(err)
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) AppendAudit(ctx context.Context, event model.AuditEvent) error {
	prior, _ := json.Marshal(event.Prior)
	current, _ := json.Marshal(event.Current)
	outcome := event.Outcome
	if outcome == "" {
		outcome = "success"
	}
	_, err := p.pool.Exec(ctx, `INSERT INTO audit_events(organisation_id, product_id, actor_id, actor_kind, action, target_type, target_id, prior, current, request_id, outcome, created_at) VALUES (nullif($1, '')::uuid, nullif($2, '')::uuid, $3, 'root', $4, $5, $6, $7, $8, $9, $10, $11)`, event.OrganisationID, event.ProductID, event.ActorID, event.Action, event.TargetType, event.TargetID, prior, current, event.RequestID, outcome, event.CreatedAt)
	return databaseError(err)
}

func (p *Postgres) AuditEvents(ctx context.Context, organisationID string) ([]model.AuditEvent, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text, coalesce(organisation_id::text, ''), coalesce(product_id::text, ''), actor_id, action, target_type, target_id, coalesce(prior, '{}'::jsonb), coalesce(current, '{}'::jsonb), request_id, outcome, created_at FROM audit_events WHERE organisation_id = $1 ORDER BY created_at DESC`, organisationID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.AuditEvent, 0)
	for rows.Next() {
		var value model.AuditEvent
		var prior, current []byte
		if err := rows.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.ActorID, &value.Action, &value.TargetType, &value.TargetID, &prior, &current, &value.RequestID, &value.Outcome, &value.CreatedAt); err != nil {
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
	if _, err := tx.Exec(ctx, `INSERT INTO users(id, issuer, subject, email, display_name, created_at, updated_at) VALUES ($1::uuid, 'dokosoko:root', $1::uuid::text, $2, $3, $4, $4)`, account.UserID, account.Email, account.DisplayName, account.CreatedAt); err != nil {
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
	if _, err := tx.Exec(ctx, `INSERT INTO users(id, issuer, subject, email, display_name, created_at, updated_at) VALUES ($1::uuid, 'dokosoko:root', $1::uuid::text, $2, $3, $4, $4)`, account.UserID, account.Email, account.DisplayName, account.CreatedAt); err != nil {
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
