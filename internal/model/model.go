package model

import (
	"time"
)

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityPublic  Visibility = "public"
)

func (v Visibility) Valid() bool {
	return v == VisibilityPrivate || v == VisibilityPublic
}

type Organisation struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Revision  int64     `json:"revision"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Environment struct {
	ID             string    `json:"id"`
	OrganisationID string    `json:"organisation_id"`
	ProductID      string    `json:"product_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	IsProduction   bool      `json:"is_production"`
	Revision       int64     `json:"revision"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Product struct {
	ID               string    `json:"id"`
	OrganisationID   string    `json:"organisation_id"`
	Name             string    `json:"name"`
	Slug             string    `json:"slug"`
	Description      string    `json:"description"`
	CatalogRevision  int64     `json:"catalog_revision"`
	PublicMCPEnabled bool      `json:"public_mcp_enabled"`
	Revision         int64     `json:"revision"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Deployment is the singleton identity of a DokoSoko installation. Product is
// retained above only while legacy product-scoped clients are migrated.
type Deployment struct {
	ID               string    `json:"id"`
	OrganisationID   string    `json:"organisation_id"`
	Name             string    `json:"name"`
	Slug             string    `json:"slug"`
	Description      string    `json:"description"`
	PublicMCPEnabled bool      `json:"public_mcp_enabled"`
	CatalogRevision  int64     `json:"catalog_revision"`
	Revision         int64     `json:"revision"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
