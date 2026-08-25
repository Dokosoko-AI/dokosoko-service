package ratelimit

import (
	"fmt"
	"testing"
	"time"
)

func TestFixedWindowLimitsPrunesAndBoundsKeys(t *testing.T) {
	start := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	limiter := NewFixedWindow(time.Minute, 2)
	if !limiter.Allow("a", 2, start) || !limiter.Allow("a", 2, start) || limiter.Allow("a", 2, start) {
		t.Fatal("per-key limit was not enforced")
	}
	if !limiter.Allow("b", 1, start) || limiter.Allow("c", 1, start) {
		t.Fatal("distinct-key cap did not fail closed")
	}
	if !limiter.Allow("c", 1, start.Add(time.Minute)) || limiter.Len() != 1 {
		t.Fatalf("expired windows were not pruned: entries=%d", limiter.Len())
	}
	if limiter.Allow("", 1, start) || limiter.Allow("bad", 0, start) || limiter.Allow("bad", 1, time.Time{}) {
		t.Fatal("invalid limiter inputs must fail closed")
	}
}

func TestFixedWindowNeverExceedsEntryCap(t *testing.T) {
	now := time.Now().UTC()
	limiter := NewFixedWindow(time.Hour, 64)
	for index := 0; index < 10_000; index++ {
		limiter.Allow(fmt.Sprintf("attacker-%d", index), 1, now)
	}
	if limiter.Len() != 64 {
		t.Fatalf("entries=%d, want 64", limiter.Len())
	}
}
