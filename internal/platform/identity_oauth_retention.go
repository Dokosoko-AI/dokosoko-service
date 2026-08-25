package platform

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/store"
)

// DefaultIdentityOAuthRetentionInterval bounds how long a cold deployment can
// retain expired provider-test PKCE material or unusable delegated bearer
// tokens without receiving identity traffic.
const DefaultIdentityOAuthRetentionInterval = 15 * time.Minute

// RunIdentityOAuthRetentionJanitor performs an immediate startup sweep and
// repeats it independently of request traffic. Each store call is bounded;
// one sweep drains all batches that were stale at its fixed cutoff.
func (s *Service) RunIdentityOAuthRetentionJanitor(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultIdentityOAuthRetentionInterval
	}
	if err := s.drainStaleIdentityOAuthArtifacts(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.drainStaleIdentityOAuthArtifacts(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *Service) drainStaleIdentityOAuthArtifacts(ctx context.Context) error {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("load deployment for identity retention: %w", err)
	}
	cutoff := s.now()
	expireCtx, cancelExpire := context.WithTimeout(ctx, 2*time.Second)
	expireErr := s.store.ExpireIdentityProviderTests(expireCtx, deployment.ID, cutoff)
	cancelExpire()
	if expireErr != nil {
		return fmt.Errorf("expire identity provider tests: %w", expireErr)
	}
	for ctx.Err() == nil {
		cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		deleted, cleanupErr := s.store.DeleteStaleOAuthArtifacts(cleanupCtx, deployment.ID, cutoff, identityOAuthCleanupBatch)
		cancel()
		if cleanupErr != nil {
			return fmt.Errorf("delete stale OAuth artifacts: %w", cleanupErr)
		}
		if deleted == 0 {
			return nil
		}
	}
	return ctx.Err()
}
