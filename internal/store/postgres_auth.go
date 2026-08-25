package store

import (
	"context"
	"fmt"
	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

func (p *Postgres) SetupCompleted(ctx context.Context) (bool, error) {
	var completed bool
	err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM root_users WHERE revoked_at IS NULL)`).Scan(&completed)
	return completed, err
}

func (p *Postgres) CreateInitialRoot(ctx context.Context, account auth.RootAccount) error {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(2811042001)`); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM root_users)`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrConflict
	}
	if _, err := tx.Exec(ctx, `INSERT INTO platform_config(singleton, public_url, setup_completed_at) VALUES (true, $1, $2) ON CONFLICT (singleton) DO UPDATE SET public_url = excluded.public_url, setup_completed_at = excluded.setup_completed_at, updated_at = now()`, p.publicURL, account.CreatedAt); err != nil {
		return databaseError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO users(id, issuer, subject, email, display_name, created_at, updated_at) VALUES ($1::uuid, 'dokosoko:root', $1::uuid::text, $2, $3, $4, $4)`, account.UserID, account.Email, account.DisplayName, account.CreatedAt); err != nil {
		return databaseError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO root_users(user_id, password_hash, totp_secret_ciphertext, recovery_code_digests, created_at, revoked_at) VALUES ($1, $2, $3, $4, $5, $6)`, account.UserID, account.PasswordHash, account.TOTPSecretCiphertext, account.RecoveryCodeDigests, account.CreatedAt, account.RevokedAt); err != nil {
		return databaseError(err)
	}
	return tx.Commit(ctx)
}

func (p *Postgres) CreateRoot(ctx context.Context, account auth.RootAccount) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO users(id, issuer, subject, email, display_name, created_at, updated_at) VALUES ($1::uuid, 'dokosoko:root', $1::uuid::text, $2, $3, $4, $4)`, account.UserID, account.Email, account.DisplayName, account.CreatedAt); err != nil {
		return databaseError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO root_users(user_id, password_hash, totp_secret_ciphertext, recovery_code_digests, created_by, created_at) VALUES ($1,$2,$3,$4,nullif($5,'')::uuid,$6)`, account.UserID, account.PasswordHash, account.TOTPSecretCiphertext, account.RecoveryCodeDigests, account.CreatedBy, account.CreatedAt); err != nil {
		return databaseError(err)
	}
	return tx.Commit(ctx)
}

func (p *Postgres) RevokeRoot(ctx context.Context, userID string, revokedAt time.Time) error {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(2811042002)`); err != nil {
		return err
	}
	var active int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM root_users WHERE revoked_at IS NULL`).Scan(&active); err != nil {
		return err
	}
	if active <= 1 {
		return auth.ErrLastRoot
	}
	command, err := tx.Exec(ctx, `UPDATE root_users SET revoked_at=$2 WHERE user_id=$1 AND revoked_at IS NULL`, userID, revokedAt)
	if err != nil {
		return databaseError(err)
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM root_sessions WHERE user_id=$1`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanRoot(row pgx.Row) (auth.RootAccount, error) {
	var value auth.RootAccount
	err := row.Scan(&value.UserID, &value.Email, &value.DisplayName, &value.PasswordHash, &value.TOTPSecretCiphertext, &value.RecoveryCodeDigests, &value.CreatedAt, &value.RevokedAt)
	return value, databaseError(err)
}

const rootSelect = `SELECT u.id::text, u.email::text, u.display_name, r.password_hash, r.totp_secret_ciphertext, r.recovery_code_digests, r.created_at, r.revoked_at FROM root_users r JOIN users u ON u.id = r.user_id`

func (p *Postgres) RootByEmail(ctx context.Context, email string) (auth.RootAccount, error) {
	return scanRoot(p.pool.QueryRow(ctx, rootSelect+` WHERE u.email = $1`, strings.ToLower(email)))
}

func (p *Postgres) RootByID(ctx context.Context, id string) (auth.RootAccount, error) {
	return scanRoot(p.pool.QueryRow(ctx, rootSelect+` WHERE u.id = $1`, id))
}

func (p *Postgres) RootAccounts(ctx context.Context) ([]auth.RootAccount, error) {
	rows, err := p.pool.Query(ctx, rootSelect+` ORDER BY r.created_at`)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]auth.RootAccount, 0)
	for rows.Next() {
		value, err := scanRoot(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) CreateSession(ctx context.Context, session auth.SessionRecord) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO root_sessions(token_digest, user_id, csrf_digest, expires_at, created_at, last_seen_at) VALUES ($1, $2, $3, $4, $5, $6)`, session.TokenDigest, session.UserID, session.CSRFDigest, session.ExpiresAt, session.CreatedAt, session.LastSeenAt)
	return databaseError(err)
}

func (p *Postgres) SessionByDigest(ctx context.Context, digest []byte) (auth.SessionRecord, error) {
	var value auth.SessionRecord
	err := p.pool.QueryRow(ctx, `SELECT token_digest, user_id::text, csrf_digest, expires_at, created_at, last_seen_at FROM root_sessions WHERE token_digest = $1`, digest).Scan(&value.TokenDigest, &value.UserID, &value.CSRFDigest, &value.ExpiresAt, &value.CreatedAt, &value.LastSeenAt)
	return value, databaseError(err)
}

func (p *Postgres) DeleteSession(ctx context.Context, digest []byte) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM root_sessions WHERE token_digest = $1`, digest)
	return err
}

func (p *Postgres) Ping(ctx context.Context) error {
	if err := p.pool.Ping(ctx); err != nil {
		return fmt.Errorf("database ping: %w", err)
	}
	return nil
}

func (p *Postgres) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM root_sessions WHERE expires_at <= $1`, now)
	return err
}
