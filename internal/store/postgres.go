package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
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

func bumpProductCatalogRevisionTx(ctx context.Context, tx pgx.Tx, productID string) error {
	result, err := tx.Exec(ctx, `UPDATE products SET catalog_revision=catalog_revision+1, updated_at=now() WHERE id=$1`, productID)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func appendProductVersionPinHistoryTx(ctx context.Context, tx pgx.Tx, value model.ProductVersionPinHistory) error {
	_, err := tx.Exec(ctx, `INSERT INTO product_version_pin_history(id,organisation_id,product_id,pin_id,scope,scope_id,prior_version,product_version,action,reason,actor_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, value.ID, value.OrganisationID, value.ProductID, value.PinID, value.Scope, value.ScopeID, value.PriorVersion, value.ProductVersion, value.Action, value.Reason, value.ActorID, value.CreatedAt)
	return databaseError(err)
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
	if err := bumpProductCatalogRevisionTx(ctx, tx, value.ProductID); err != nil {
		return model.ProductVersion{}, err
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
	if err := bumpProductCatalogRevisionTx(ctx, tx, value.ProductID); err != nil {
		return model.ProductVersion{}, err
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

func (p *Postgres) SaveProductVersionPin(ctx context.Context, value model.ProductVersionPin, expected int64, history model.ProductVersionPinHistory) (model.ProductVersionPin, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.ProductVersionPin{}, databaseError(err)
	}
	defer tx.Rollback(ctx)
	var saved model.ProductVersionPin
	if expected == 0 {
		saved, err = scanProductVersionPin(tx.QueryRow(ctx, `INSERT INTO product_version_pins(id,organisation_id,product_id,scope,scope_id,customer_account_id,environment_id,installation_id,connector_release_id,reason) VALUES($1,$2,$3,$4,$5,$6,nullif($7,'')::uuid,nullif($8,'')::uuid,$9,$10) RETURNING id::text,organisation_id::text,product_id::text,scope,scope_id,customer_account_id::text,coalesce(environment_id::text,''),coalesce(installation_id::text,''),connector_release_id::text,(SELECT display_version FROM connector_releases WHERE id=product_version_pins.connector_release_id),reason,revision,created_at,updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Scope, value.ScopeID, value.CustomerAccountID, value.EnvironmentID, value.InstallationID, value.ProductVersionID, value.Reason))
	} else {
		saved, err = scanProductVersionPin(tx.QueryRow(ctx, `UPDATE product_version_pins SET connector_release_id=$4,reason=$5,revision=revision+1,updated_at=now() WHERE product_id=$1 AND scope=$2 AND scope_id=$3 AND revision=$6 RETURNING id::text,organisation_id::text,product_id::text,scope,scope_id,customer_account_id::text,coalesce(environment_id::text,''),coalesce(installation_id::text,''),connector_release_id::text,(SELECT display_version FROM connector_releases WHERE id=product_version_pins.connector_release_id),reason,revision,created_at,updated_at`, value.ProductID, value.Scope, value.ScopeID, value.ProductVersionID, value.Reason, expected))
	}
	if errors.Is(err, ErrNotFound) {
		var exists bool
		if lookupErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_version_pins WHERE product_id=$1 AND scope=$2 AND scope_id=$3)`, value.ProductID, value.Scope, value.ScopeID).Scan(&exists); lookupErr != nil {
			return model.ProductVersionPin{}, databaseError(lookupErr)
		}
		if exists {
			return model.ProductVersionPin{}, ErrConflict
		}
	}
	if err != nil {
		return model.ProductVersionPin{}, err
	}
	history.PinID = saved.ID
	if err := appendProductVersionPinHistoryTx(ctx, tx, history); err != nil {
		return model.ProductVersionPin{}, err
	}
	if err := bumpProductCatalogRevisionTx(ctx, tx, value.ProductID); err != nil {
		return model.ProductVersionPin{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ProductVersionPin{}, databaseError(err)
	}
	return saved, nil
}

func (p *Postgres) DeleteProductVersionPin(ctx context.Context, productID, id string, history model.ProductVersionPinHistory) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return databaseError(err)
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `DELETE FROM product_version_pins WHERE product_id=$1 AND id=$2`, productID, id)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := appendProductVersionPinHistoryTx(ctx, tx, history); err != nil {
		return err
	}
	if err := bumpProductCatalogRevisionTx(ctx, tx, productID); err != nil {
		return err
	}
	return databaseError(tx.Commit(ctx))
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
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.ProductInstallation{}, databaseError(err)
	}
	defer tx.Rollback(ctx)
	var saved model.ProductInstallation
	if expected == 0 {
		saved, err = scanProductInstallation(tx.QueryRow(ctx, `INSERT INTO product_installations(id,organisation_id,product_id,customer_account_id,environment_id,external_id,name,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text,organisation_id::text,product_id::text,customer_account_id::text,environment_id::text,external_id,name,state,revision,created_at,updated_at`, value.ID, value.OrganisationID, value.ProductID, value.CustomerAccountID, value.EnvironmentID, value.ExternalID, value.Name, value.State))
	} else {
		saved, err = scanProductInstallation(tx.QueryRow(ctx, `UPDATE product_installations SET customer_account_id=$3,environment_id=$4,external_id=$5,name=$6,state=$7,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$8 RETURNING id::text,organisation_id::text,product_id::text,customer_account_id::text,environment_id::text,external_id,name,state,revision,created_at,updated_at`, value.ProductID, value.ID, value.CustomerAccountID, value.EnvironmentID, value.ExternalID, value.Name, value.State, expected))
	}
	if errors.Is(err, ErrNotFound) {
		var exists bool
		if lookupErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_installations WHERE product_id=$1 AND id=$2)`, value.ProductID, value.ID).Scan(&exists); lookupErr != nil {
			return model.ProductInstallation{}, databaseError(lookupErr)
		}
		if exists {
			return model.ProductInstallation{}, ErrConflict
		}
	}
	if err != nil {
		return model.ProductInstallation{}, err
	}
	if err := bumpProductCatalogRevisionTx(ctx, tx, value.ProductID); err != nil {
		return model.ProductInstallation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ProductInstallation{}, databaseError(err)
	}
	return saved, nil
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
