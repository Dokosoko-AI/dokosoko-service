package postgres

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"time"

	"github.com/dokosoko/examples/go-backend-integration/internal/backend"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	config.MaxConns = 10
	config.MinConns = 1
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL pool: %w", err)
	}
	store := &Store{pool: pool}
	if err := store.Ready(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	return store, nil
}

func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) Ready(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(584786140220260822)`); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files)
	for _, name := range files {
		contents, err := migrations.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(contents)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) Accept(ctx context.Context, input backend.AcceptInput) (backend.AcceptResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return backend.AcceptResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serializes all attempts for one idempotency key, including the first insert.
	// A 64-bit advisory hash collision can only reduce concurrency; it cannot mix data.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, input.IdempotencyKey); err != nil {
		return backend.AcceptResult{}, err
	}
	var storedHash, storedBody []byte
	var storedStatus int
	err = tx.QueryRow(ctx, `
		SELECT request_sha256, response_status, response_body
		FROM dokosoko_idempotency_results
		WHERE idempotency_key = $1
	`, input.IdempotencyKey).Scan(&storedHash, &storedStatus, &storedBody)
	switch {
	case err == nil:
		if !bytes.Equal(storedHash, input.RequestSHA256) {
			return backend.AcceptResult{}, backend.ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return backend.AcceptResult{}, err
		}
		return backend.AcceptResult{StatusCode: storedStatus, Body: storedBody, Replayed: true}, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return backend.AcceptResult{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO support_submissions(receipt_id, submission_id, kind, payload)
		VALUES ($1, $2, $3, $4::jsonb)
	`, input.ReceiptID, input.SubmissionID, input.Kind, input.CanonicalBody); err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" && postgresError.ConstraintName == "support_submissions_submission_id_key" {
			return backend.AcceptResult{}, backend.ErrSubmissionConflict
		}
		return backend.AcceptResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO dokosoko_idempotency_results(
			idempotency_key, request_sha256, response_status, response_body,
			first_request_id, retain_until
		) VALUES ($1, $2, $3, $4, $5, now() + interval '24 hours')
	`, input.IdempotencyKey, input.RequestSHA256, http.StatusAccepted, input.ReceiptBody, input.RequestID); err != nil {
		return backend.AcceptResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return backend.AcceptResult{}, err
	}
	return backend.AcceptResult{StatusCode: http.StatusAccepted, Body: input.ReceiptBody}, nil
}
