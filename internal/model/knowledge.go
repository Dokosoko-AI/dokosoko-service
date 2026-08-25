package model

import (
	"encoding/json"
	"time"
)

type Source struct {
	ID             string     `json:"id"`
	OrganisationID string     `json:"organisation_id"`
	ProductID      string     `json:"product_id"`
	Name           string     `json:"name"`
	Kind           string     `json:"kind"`
	Location       string     `json:"location"`
	Visibility     Visibility `json:"visibility"`
	Published      bool       `json:"published"`
	Quarantined    bool       `json:"quarantined"`
	Revision       int64      `json:"revision"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CrawlJob struct {
	ID              string     `json:"id"`
	OrganisationID  string     `json:"organisation_id"`
	ProductID       string     `json:"product_id"`
	SourceID        string     `json:"source_id"`
	State           string     `json:"state"`
	Attempt         int        `json:"attempt"`
	DiscoveredCount int        `json:"discovered_count"`
	FetchedCount    int        `json:"fetched_count"`
	ChangedCount    int        `json:"changed_count"`
	ErrorCode       string     `json:"error_code,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	QueuedAt        time.Time  `json:"queued_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

// CrawlReviewDocument is an immutable document candidate produced by one
// specific crawl generation. Reused snapshots are linked to every generation
// that observed them so a reviewer always approves a complete, exact set.
type CrawlReviewDocument struct {
	ID                  string          `json:"id"`
	CrawlJobID          string          `json:"crawl_job_id"`
	SnapshotID          string          `json:"snapshot_id"`
	Title               string          `json:"title"`
	CanonicalURL        string          `json:"canonical_url"`
	State               string          `json:"state"`
	TrustLevel          int             `json:"trust_level"`
	InjectionIndicators json.RawMessage `json:"injection_indicators"`
	ContentHash         string          `json:"content_hash"`
	Changed             bool            `json:"changed"`
}

// SourcePublication pins the reviewed crawl generation and the selected
// immutable documents which may be referenced by Integration resource sets.
type SourcePublication struct {
	ID             string     `json:"id"`
	OrganisationID string     `json:"organisation_id"`
	ProductID      string     `json:"product_id"`
	SourceID       string     `json:"source_id"`
	CrawlJobID     string     `json:"crawl_job_id"`
	Revision       int64      `json:"revision"`
	Visibility     Visibility `json:"visibility"`
	ContentHash    string     `json:"content_hash"`
	DocumentCount  int        `json:"document_count"`
	ReviewedBy     string     `json:"reviewed_by"`
	ReviewedAt     time.Time  `json:"reviewed_at"`
	PublishedAt    time.Time  `json:"published_at"`
}

type SourceReview struct {
	Source      Source                `json:"source"`
	CrawlJob    CrawlJob              `json:"crawl_job"`
	Documents   []CrawlReviewDocument `json:"documents"`
	Publication *SourcePublication    `json:"publication,omitempty"`
}

type Secret struct {
	ID             string
	OrganisationID string
	Name           string
	Purpose        string
	Ciphertext     []byte
	Nonce          []byte
	KeyVersion     int
	Fingerprint    string
	CreatedAt      time.Time
}
