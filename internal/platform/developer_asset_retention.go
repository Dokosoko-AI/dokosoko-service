package platform

import (
	"context"
	"fmt"
	"time"
)

const (
	// DefaultDeveloperAssetRetrievalRetentionInterval bounds how long a cold
	// deployment can retain expired Query Lab/MCP retrieval traces.
	DefaultDeveloperAssetRetrievalRetentionInterval = 15 * time.Minute
	developerAssetRetrievalCleanupBatch             = 200
	developerAssetRetrievalCleanupTimeout           = 2 * time.Second
)

// RunDeveloperAssetRetrievalRetentionJanitor performs an immediate bounded
// sweep and repeats independently of query traffic. Publication evidence and
// knowledge units are immutable and are not part of this ephemeral trace
// retention policy.
func (s *Service) RunDeveloperAssetRetrievalRetentionJanitor(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultDeveloperAssetRetrievalRetentionInterval
	}
	if err := s.drainExpiredDeveloperAssetRetrievalTraces(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.drainExpiredDeveloperAssetRetrievalTraces(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *Service) drainExpiredDeveloperAssetRetrievalTraces(ctx context.Context) error {
	cutoff := s.now()
	for ctx.Err() == nil {
		cleanupCtx, cancel := context.WithTimeout(ctx, developerAssetRetrievalCleanupTimeout)
		deleted, err := s.store.DeleteExpiredRetrievalQueryTraces(cleanupCtx, cutoff, developerAssetRetrievalCleanupBatch)
		cancel()
		if err != nil {
			return fmt.Errorf("delete expired developer-asset retrieval traces: %w", err)
		}
		if deleted == 0 {
			return nil
		}
	}
	return ctx.Err()
}
