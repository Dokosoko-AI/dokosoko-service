package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dokosoko/examples/go-backend-integration/internal/backend"
)

func TestAcceptIsTransactionalAcrossConcurrentAttempts(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	key := "integration-test-key-" + suffix
	otherKey := "integration-test-other-key-" + suffix
	submissionID := "integration_test_submission_" + suffix
	defer func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM dokosoko_idempotency_results WHERE idempotency_key = ANY($1)`, []string{key, otherKey})
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM support_submissions WHERE submission_id = $1`, submissionID)
	}()

	canonical := []byte(`{"submission_id":"` + submissionID + `"}`)
	digest := sha256.Sum256(canonical)
	inputs := []backend.AcceptInput{
		acceptInput(key, submissionID, "receipt_a_"+suffix, canonical, digest[:]),
		acceptInput(key, submissionID, "receipt_b_"+suffix, canonical, digest[:]),
	}
	results := make([]backend.AcceptResult, len(inputs))
	errorsByIndex := make([]error, len(inputs))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range inputs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			results[index], errorsByIndex[index] = store.Accept(ctx, inputs[index])
		}(index)
	}
	close(start)
	wait.Wait()
	for index, err := range errorsByIndex {
		if err != nil {
			t.Fatalf("accept %d: %v", index, err)
		}
	}
	if results[0].StatusCode != http.StatusAccepted || results[1].StatusCode != http.StatusAccepted || string(results[0].Body) != string(results[1].Body) {
		t.Fatalf("concurrent results differ: %#v %#v", results[0], results[1])
	}
	if results[0].Replayed == results[1].Replayed {
		t.Fatalf("expected one original and one replay: %#v %#v", results[0], results[1])
	}

	changed := sha256.Sum256([]byte(`{"different":true}`))
	conflict := acceptInput(key, submissionID, "receipt_c_"+suffix, canonical, changed[:])
	if _, err := store.Accept(ctx, conflict); !errors.Is(err, backend.ErrIdempotencyConflict) {
		t.Fatalf("different payload error = %v", err)
	}
	duplicateSubmission := acceptInput(otherKey, submissionID, "receipt_d_"+suffix, canonical, digest[:])
	if _, err := store.Accept(ctx, duplicateSubmission); !errors.Is(err, backend.ErrSubmissionConflict) {
		t.Fatalf("duplicate submission error = %v", err)
	}
}

func acceptInput(key, submissionID, receiptID string, canonical, digest []byte) backend.AcceptInput {
	receipt, _ := json.Marshal(backend.SupportSubmissionReceipt{ID: receiptID, Status: "accepted", ExternalID: submissionID})
	return backend.AcceptInput{
		IdempotencyKey: key,
		RequestID:      "req_11111111111111111111111111111111",
		RequestSHA256:  append([]byte(nil), digest...),
		CanonicalBody:  append([]byte(nil), canonical...),
		ReceiptBody:    receipt,
		ReceiptID:      receiptID,
		SubmissionID:   submissionID,
		Kind:           "bug",
	}
}
