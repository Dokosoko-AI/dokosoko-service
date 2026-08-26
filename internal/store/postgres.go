package store

import (
	"context"
	"errors"
	"strings"

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
		case "23505", "23503", "23514", "23502", "55000":
			return ErrConflict
		case "22P02":
			return ErrNotFound
		}
	}
	return err
}

func scanProduct(row pgx.Row) (model.Product, error) {
	var value model.Product
	err := row.Scan(&value.ID, &value.OrganisationID, &value.Name, &value.Slug, &value.Description, &value.CatalogRevision, &value.PublicMCPEnabled, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const productSelect = `SELECT id::text, organisation_id::text, name, slug, description, catalog_revision, public_mcp_enabled, revision, created_at, updated_at FROM products`

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
	return scanProduct(p.pool.QueryRow(ctx, `INSERT INTO products(id, organisation_id, name, slug, description) VALUES ($1, $2, $3, $4, $5) RETURNING `+productSelect[len("SELECT "):len(productSelect)-len(" FROM products")], value.ID, value.OrganisationID, value.Name, value.Slug, value.Description))
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
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Product{}, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var deploymentRevision, deploymentCatalogRevision int64
	deploymentErr := tx.QueryRow(ctx, `SELECT revision,catalog_revision FROM deployments WHERE id=$1 FOR UPDATE`, value.ID).Scan(&deploymentRevision, &deploymentCatalogRevision)
	if deploymentErr != nil && !errors.Is(deploymentErr, pgx.ErrNoRows) {
		return model.Product{}, databaseError(deploymentErr)
	}
	if deploymentErr == nil {
		var productRevision, productCatalogRevision int64
		if err := tx.QueryRow(ctx, `SELECT revision,catalog_revision FROM products WHERE id=$1 FOR UPDATE`, value.ID).Scan(&productRevision, &productCatalogRevision); err != nil {
			return model.Product{}, databaseError(err)
		}
		if productRevision != expected || deploymentRevision != expected || productRevision != deploymentRevision || productCatalogRevision != deploymentCatalogRevision {
			return model.Product{}, ErrConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE deployments SET description=$2,public_mcp_enabled=$3,revision=revision+1,catalog_revision=catalog_revision+1,updated_at=now() WHERE id=$1`, value.ID, value.Description, value.PublicMCPEnabled); err != nil {
			return model.Product{}, databaseError(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE products SET description=$2,public_mcp_enabled=$3,revision=revision+1,catalog_revision=catalog_revision+1,updated_at=now() WHERE id=$1`, value.ID, value.Description, value.PublicMCPEnabled); err != nil {
			return model.Product{}, databaseError(err)
		}
	} else {
		result, updateErr := tx.Exec(ctx, `UPDATE products SET description=$2,public_mcp_enabled=$3,revision=revision+1,catalog_revision=catalog_revision+1,updated_at=now() WHERE id=$1 AND revision=$4`, value.ID, value.Description, value.PublicMCPEnabled, expected)
		if updateErr != nil {
			return model.Product{}, databaseError(updateErr)
		}
		if result.RowsAffected() != 1 {
			if _, lookupErr := scanProduct(tx.QueryRow(ctx, productSelect+` WHERE id=$1`, value.ID)); lookupErr == nil {
				return model.Product{}, ErrConflict
			}
			return model.Product{}, ErrNotFound
		}
	}
	updated, err := scanProduct(tx.QueryRow(ctx, productSelect+` WHERE id=$1`, value.ID))
	if err != nil {
		return model.Product{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Product{}, databaseError(err)
	}
	return updated, nil
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
