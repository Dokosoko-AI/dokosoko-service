package lifecycle

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSupervisorRestartsErrorsAndRecoversPanics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var attempts atomic.Int32
	var mu sync.Mutex
	var messages []string
	supervisor := NewSupervisor(ctx, func(format string, values ...any) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, format)
	})
	supervisor.restartDelay = time.Millisecond
	supervisor.Start("retention", func(ctx context.Context) error {
		switch attempts.Add(1) {
		case 1:
			return errors.New("database unavailable")
		case 2:
			panic("corrupt cursor")
		default:
			<-ctx.Done()
			return ctx.Err()
		}
	})
	deadline := time.Now().Add(time.Second)
	for attempts.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	supervisor.Wait()
	if attempts.Load() < 3 {
		t.Fatalf("attempts=%d, want at least 3", attempts.Load())
	}
	mu.Lock()
	joined := strings.Join(messages, "\n")
	mu.Unlock()
	if !strings.Contains(joined, "background worker %s failed") || len(messages) != 2 {
		t.Fatalf("messages=%q", messages)
	}
}
