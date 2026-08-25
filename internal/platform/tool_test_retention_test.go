package platform_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type toolTestRetentionCall struct {
	deleted int64
	limit   int
	err     error
}

type toolTestRetentionTrackingStore struct {
	store.Store
	calls chan toolTestRetentionCall
}

func (s *toolTestRetentionTrackingStore) DeleteExpiredToolTestData(ctx context.Context, now time.Time, limit int) (int64, error) {
	deleted, err := s.Store.DeleteExpiredToolTestData(ctx, now, limit)
	s.calls <- toolTestRetentionCall{deleted: deleted, limit: limit, err: err}
	return deleted, err
}

func appendRetentionRun(t *testing.T, memory *store.Memory, toolID string, revision int64, id string, expiresAt time.Time) {
	t.Helper()
	err := memory.AppendToolTestRun(context.Background(), model.ToolTestRun{
		ID:             id,
		OrganisationID: "org_acme",
		ProductID:      "prod_acme",
		ToolID:         toolID,
		ToolRevision:   revision,
		ArgumentHash:   make([]byte, 32),
		ExpiresAt:      expiresAt,
		CreatedAt:      expiresAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func createRetentionTool(t *testing.T, memory *store.Memory) (string, int64) {
	t.Helper()
	tool, err := memory.CreateTool(context.Background(), model.Tool{
		ID:             "tool-retention",
		OrganisationID: "org_acme",
		ProductID:      "prod_acme",
		Namespace:      "retention",
		Name:           "janitor",
		BackendKind:    "http",
	})
	if err != nil {
		t.Fatal(err)
	}
	return tool.ID, tool.Revision
}

func nextToolTestRetentionCall(t *testing.T, calls <-chan toolTestRetentionCall) toolTestRetentionCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tool-test retention cleanup")
		return toolTestRetentionCall{}
	}
}

func stopToolTestRetentionJanitor(t *testing.T, cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("tool-test retention janitor did not stop after cancellation")
	}
}

func TestToolTestRetentionJanitorImmediatelyDrainsBoundedBatches(t *testing.T) {
	memory := store.NewMemory()
	tracking := &toolTestRetentionTrackingStore{Store: memory, calls: make(chan toolTestRetentionCall, 16)}
	service := platform.New(tracking)
	toolID, revision := createRetentionTool(t, memory)
	now := time.Now().UTC()
	for index := 0; index < 205; index++ {
		appendRetentionRun(t, memory, toolID, revision, fmt.Sprintf("expired-run-%03d", index), now.Add(-time.Hour))
	}
	appendRetentionRun(t, memory, toolID, revision, "live-run", now.Add(time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		service.RunToolTestRetentionJanitor(ctx, time.Hour)
		close(done)
	}()

	for index, wantDeleted := range []int64{100, 100, 5, 0} {
		call := nextToolTestRetentionCall(t, tracking.calls)
		if call.err != nil || call.limit != 100 || call.deleted != wantDeleted {
			t.Fatalf("cleanup call %d = {deleted:%d limit:%d err:%v}, want deleted=%d limit=100", index, call.deleted, call.limit, call.err, wantDeleted)
		}
	}
	stopToolTestRetentionJanitor(t, cancel, done)

	if _, err := memory.ToolTestRun(context.Background(), "prod_acme", toolID, "expired-run-204", now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expired run remains after drain: %v", err)
	}
	if run, err := memory.ToolTestRun(context.Background(), "prod_acme", toolID, "live-run", now); err != nil || run.ID != "live-run" {
		t.Fatalf("live run = %#v, err=%v", run, err)
	}
}

func TestToolTestRetentionJanitorRunsAgainOnInterval(t *testing.T) {
	memory := store.NewMemory()
	tracking := &toolTestRetentionTrackingStore{Store: memory, calls: make(chan toolTestRetentionCall, 32)}
	service := platform.New(tracking)
	toolID, revision := createRetentionTool(t, memory)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		service.RunToolTestRetentionJanitor(ctx, 10*time.Millisecond)
		close(done)
	}()
	first := nextToolTestRetentionCall(t, tracking.calls)
	if first.err != nil || first.limit != 100 || first.deleted != 0 {
		t.Fatalf("immediate cleanup = {deleted:%d limit:%d err:%v}", first.deleted, first.limit, first.err)
	}

	now := time.Now().UTC()
	appendRetentionRun(t, memory, toolID, revision, "scheduled-expired-run", now.Add(-time.Hour))
	foundDelete := false
	for !foundDelete {
		call := nextToolTestRetentionCall(t, tracking.calls)
		if call.err != nil || call.limit != 100 {
			t.Fatalf("scheduled cleanup = {deleted:%d limit:%d err:%v}", call.deleted, call.limit, call.err)
		}
		foundDelete = call.deleted == 1
	}
	final := nextToolTestRetentionCall(t, tracking.calls)
	if final.err != nil || final.limit != 100 || final.deleted != 0 {
		t.Fatalf("drain completion = {deleted:%d limit:%d err:%v}", final.deleted, final.limit, final.err)
	}
	stopToolTestRetentionJanitor(t, cancel, done)
}
