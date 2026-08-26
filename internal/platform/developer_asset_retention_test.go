package platform

import (
	"context"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/store"
)

type developerAssetRetentionCall struct {
	before time.Time
	limit  int
}

type developerAssetRetentionStore struct {
	store.Store
	calls   chan developerAssetRetentionCall
	deleted []int64
}

func (s *developerAssetRetentionStore) DeleteExpiredRetrievalQueryTraces(_ context.Context, before time.Time, limit int) (int64, error) {
	s.calls <- developerAssetRetentionCall{before: before, limit: limit}
	if len(s.deleted) == 0 {
		return 0, nil
	}
	value := s.deleted[0]
	s.deleted = s.deleted[1:]
	return value, nil
}

func TestDeveloperAssetRetrievalRetentionJanitorImmediatelyDrainsBoundedBatches(t *testing.T) {
	t.Parallel()
	tracking := &developerAssetRetentionStore{
		Store: store.NewMemory(), calls: make(chan developerAssetRetentionCall, 4),
		deleted: []int64{developerAssetRetrievalCleanupBatch, 1, 0},
	}
	service := New(tracking)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.RunDeveloperAssetRetrievalRetentionJanitor(ctx, time.Hour) }()
	var first time.Time
	for index := 0; index < 3; index++ {
		select {
		case call := <-tracking.calls:
			if call.limit != developerAssetRetrievalCleanupBatch {
				t.Fatalf("cleanup limit = %d", call.limit)
			}
			if index == 0 {
				first = call.before
			} else if !call.before.Equal(first) {
				t.Fatal("one sweep changed its fixed retention cutoff")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for retrieval cleanup")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("retrieval retention janitor did not stop")
	}
}
