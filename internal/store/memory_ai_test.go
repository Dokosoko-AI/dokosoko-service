package store

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestAIBudgetReservationsAreConcurrencySafe(t *testing.T) {
	memory := NewMemory()
	day := time.Now().UTC().Truncate(24 * time.Hour)
	var accepted atomic.Int64
	var group sync.WaitGroup
	for index := 0; index < 20; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			ok, err := memory.ReserveAIBudget(context.Background(), model.AIBudgetReservation{ID: fmt.Sprintf("reservation-%d", index), ProductID: "prod_acme", Workload: "authoring", Day: day, ReservedTokens: 100, ExpiresAt: time.Now().UTC().Add(time.Minute)}, 1000)
			if err != nil {
				t.Errorf("reserve: %v", err)
				return
			}
			if ok {
				accepted.Add(1)
			}
		}(index)
	}
	group.Wait()
	if accepted.Load() != 10 {
		t.Fatalf("accepted reservations = %d, want 10", accepted.Load())
	}
}
