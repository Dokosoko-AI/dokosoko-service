package platform

import (
	"context"
	"time"
)

// DefaultToolTestRetentionInterval is the interval between independent
// retention sweeps. Each sweep drains all evidence that was expired when the
// sweep began, using the store's bounded cleanup operation.
const DefaultToolTestRetentionInterval = 15 * time.Minute

// RunToolTestRetentionJanitor removes expired tool-test confirmations and
// sanitized evidence independently of API traffic. It performs an immediate
// sweep so expired data is not retained for a full interval after startup.
func (s *Service) RunToolTestRetentionJanitor(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultToolTestRetentionInterval
	}

	s.drainExpiredToolTestData(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.drainExpiredToolTestData(ctx)
		}
	}
}

func (s *Service) drainExpiredToolTestData(ctx context.Context) {
	cutoff := s.now()
	for ctx.Err() == nil {
		cleanupCtx, cancel := context.WithTimeout(ctx, toolTestCleanupTimeout)
		deleted, err := s.store.DeleteExpiredToolTestData(cleanupCtx, cutoff, toolTestCleanupBatch)
		cancel()
		if err != nil || deleted == 0 {
			return
		}
	}
}
