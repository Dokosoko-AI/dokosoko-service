// Package lifecycle coordinates long-running service workers.
package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Worker func(context.Context) error

type Supervisor struct {
	ctx          context.Context
	logf         func(string, ...any)
	restartDelay time.Duration
	wg           sync.WaitGroup
}

func NewSupervisor(ctx context.Context, logf func(string, ...any)) *Supervisor {
	if ctx == nil {
		ctx = context.Background()
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Supervisor{ctx: ctx, logf: logf, restartDelay: 5 * time.Second}
}

// Start runs and restarts a worker until the supervisor context is cancelled.
// Both returned errors and panics are visible through the configured logger.
func (s *Supervisor) Start(name string, worker Worker) {
	if worker == nil {
		panic("lifecycle worker must not be nil")
	}
	if name == "" {
		name = "unnamed"
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for s.ctx.Err() == nil {
			err := runWorker(s.ctx, worker)
			if s.ctx.Err() != nil {
				return
			}
			if err == nil {
				err = fmt.Errorf("stopped unexpectedly")
			}
			s.logf("background worker %s failed: %v; restarting", name, err)
			timer := time.NewTimer(s.restartDelay)
			select {
			case <-s.ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
		}
	}()
}

func runWorker(ctx context.Context, worker Worker) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic: %v", recovered)
		}
	}()
	return worker(ctx)
}

func (s *Supervisor) Wait() {
	s.wg.Wait()
}
