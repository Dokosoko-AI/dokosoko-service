package platform

import (
	"context"
	"fmt"
	"time"
)

// DefaultToolTestRetentionInterval is the interval between independent
// retention sweeps. Each sweep drains all evidence that was expired when the
// sweep began, using the store's bounded cleanup operation.
const DefaultToolTestRetentionInterval = 15 * time.Minute

// RunToolTestRetentionJanitor removes expired tool-test confirmations and
// sanitized evidence independently of API traffic. It performs an immediate
// sweep so expired data is not retained for a full interval after startup.
func (s *Service) RunToolTestRetentionJanitor(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultToolTestRetentionInterval
	}

	if err := s.drainExpiredToolTestData(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.drainExpiredToolTestData(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *Service) drainExpiredToolTestData(ctx context.Context) error {
	cutoff := s.now()
	for ctx.Err() == nil {
		cleanupCtx, cancel := context.WithTimeout(ctx, toolTestCleanupTimeout)
		deleted, err := s.store.DeleteExpiredToolTestData(cleanupCtx, cutoff, toolTestCleanupBatch)
		cancel()
		if err != nil {
			return fmt.Errorf("delete expired tool-test data: %w", err)
		}
		if deleted == 0 {
			return nil
		}
	}
	return ctx.Err()
}
