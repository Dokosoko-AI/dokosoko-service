package platform

import (
	"context"
	"time"
)

// DefaultIdentityOAuthRetentionInterval bounds how long a cold deployment can
// retain expired provider-test PKCE material or unusable delegated bearer
// tokens without receiving identity traffic.
const DefaultIdentityOAuthRetentionInterval = 15 * time.Minute

// RunIdentityOAuthRetentionJanitor performs an immediate startup sweep and
// repeats it independently of request traffic. Each store call is bounded;
// one sweep drains all batches that were stale at its fixed cutoff.
func (s *Service) RunIdentityOAuthRetentionJanitor(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultIdentityOAuthRetentionInterval
	}
	s.drainStaleIdentityOAuthArtifacts(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.drainStaleIdentityOAuthArtifacts(ctx)
		}
	}
}

func (s *Service) drainStaleIdentityOAuthArtifacts(ctx context.Context) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return
	}
	cutoff := s.now()
	expireCtx, cancelExpire := context.WithTimeout(ctx, 2*time.Second)
	_ = s.store.ExpireIdentityProviderTests(expireCtx, deployment.ID, cutoff)
	cancelExpire()
	for ctx.Err() == nil {
		cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		deleted, cleanupErr := s.store.DeleteStaleOAuthArtifacts(cleanupCtx, deployment.ID, cutoff, identityOAuthCleanupBatch)
		cancel()
		if cleanupErr != nil || deleted == 0 {
			return
		}
	}
}
