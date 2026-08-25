// Package ratelimit provides bounded, fail-closed in-process rate limiters.
package ratelimit

import (
	"sync"
	"time"
)

type window struct {
	started time.Time
	count   int
}

// FixedWindow limits distinct keys as well as requests per key. The distinct
// key cap is part of the safety contract: attacker-controlled identifiers can
// never grow the backing map without bound.
type FixedWindow struct {
	mu         sync.Mutex
	duration   time.Duration
	maxEntries int
	windows    map[string]window
}

func NewFixedWindow(duration time.Duration, maxEntries int) *FixedWindow {
	if duration <= 0 {
		panic("rate-limit window duration must be positive")
	}
	if maxEntries <= 0 {
		panic("rate-limit entry cap must be positive")
	}
	return &FixedWindow{duration: duration, maxEntries: maxEntries, windows: make(map[string]window)}
}

// Allow consumes one request from key. New keys fail closed when every entry
// is still active; expired entries are pruned before enforcing the map cap.
func (l *FixedWindow) Allow(key string, limit int, now time.Time) bool {
	if l == nil || key == "" || limit <= 0 || now.IsZero() {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	current, exists := l.windows[key]
	if !exists {
		l.pruneExpired(now)
		if len(l.windows) >= l.maxEntries {
			return false
		}
	}
	if current.started.IsZero() || now.Before(current.started) || now.Sub(current.started) >= l.duration {
		current = window{started: now}
	}
	if current.count >= limit {
		l.windows[key] = current
		return false
	}
	current.count++
	l.windows[key] = current
	return true
}

func (l *FixedWindow) pruneExpired(now time.Time) {
	for key, current := range l.windows {
		if current.started.IsZero() || now.Before(current.started) || now.Sub(current.started) >= l.duration {
			delete(l.windows, key)
		}
	}
}

func (l *FixedWindow) Len() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.windows)
}
