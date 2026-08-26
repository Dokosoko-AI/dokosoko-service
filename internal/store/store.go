package store

import (
	"errors"
	"time"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("revision conflict")
	ErrCatalogConflict = errors.New("catalog revision conflict")
)

const identitySecretOrphanGrace = 15 * time.Minute

func boundedToolTestCleanupLimit(limit int) int {
	if limit < 1 {
		return 0
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func boundedOAuthArtifactCleanupLimit(limit int) int {
	if limit < 1 {
		return 0
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

// Store is the complete persistence contract used by the application composition root.
// Domain services should depend on the narrow capability interfaces in contracts.go when possible.
type Store interface {
	DeploymentCatalogStore
	RuntimeServiceStore
	IntegrationPolicyStore
	ProductCatalogStore
	KnowledgeStore
	ToolStore
	NativePluginStateStore
	MCPStore
	ReportingStore
	AIRecipeStore
	IdentityStore
	ObservabilityStore
	DeveloperAssetStore
}
