package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) PublicKnowledge(ctx context.Context, productID string, publicationIDs []string, query string) ([]model.KnowledgeRecord, error) {
	pattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := p.pool.Query(ctx, `
		SELECT DISTINCT kd.id::text, kd.product_id::text, kd.source_id::text, kd.title, kd.body, kd.canonical_url, s.visibility::text
		FROM source_publications sp
		JOIN source_publication_documents spd ON spd.source_publication_id = sp.id
		JOIN knowledge_documents kd ON kd.id = spd.knowledge_document_id
		JOIN sources s ON s.id = sp.source_id AND s.product_id = sp.product_id
		WHERE sp.product_id = $1 AND sp.id = ANY($2::uuid[])
		  AND sp.visibility = 'public' AND s.visibility = 'public'
		  AND s.state = 'published' AND kd.state = 'published'
		  AND kd.injection_indicators = '[]'::jsonb
		  AND ($3 = '%%' OR kd.title ILIKE $3 OR kd.body ILIKE $3)
		ORDER BY kd.id::text LIMIT 20`, productID, publicationIDs, pattern)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.KnowledgeRecord, 0)
	for rows.Next() {
		var value model.KnowledgeRecord
		if err := rows.Scan(&value.ID, &value.ProductID, &value.SourceID, &value.Title, &value.Text, &value.URL, &value.Visibility); err != nil {
			return nil, err
		}
		value.Published = true
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) PrivateKnowledge(ctx context.Context, productID string, publicationIDs []string, query string) ([]model.KnowledgeRecord, error) {
	pattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := p.pool.Query(ctx, `
		SELECT DISTINCT kd.id::text, kd.product_id::text, kd.source_id::text, kd.title, kd.body, kd.canonical_url, s.visibility::text
		FROM source_publications sp
		JOIN source_publication_documents spd ON spd.source_publication_id = sp.id
		JOIN knowledge_documents kd ON kd.id = spd.knowledge_document_id
		JOIN sources s ON s.id = sp.source_id AND s.product_id = sp.product_id
		WHERE sp.product_id = $1 AND sp.id = ANY($2::uuid[])
		  AND s.state = 'published' AND kd.state = 'published'
		  AND kd.injection_indicators = '[]'::jsonb
		  AND ($3 = '%%' OR kd.title ILIKE $3 OR kd.body ILIKE $3)
		ORDER BY kd.id::text LIMIT 20`, productID, publicationIDs, pattern)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.KnowledgeRecord, 0)
	for rows.Next() {
		var value model.KnowledgeRecord
		if err := rows.Scan(&value.ID, &value.ProductID, &value.SourceID, &value.Title, &value.Text, &value.URL, &value.Visibility); err != nil {
			return nil, err
		}
		value.Published = true
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) AppendAudit(ctx context.Context, event model.AuditEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		return errors.New("audit event ID is required")
	}
	prior, err := json.Marshal(event.Prior)
	if err != nil {
		return fmt.Errorf("marshal audit prior state: %w", err)
	}
	current, err := json.Marshal(event.Current)
	if err != nil {
		return fmt.Errorf("marshal audit current state: %w", err)
	}
	outcome := event.Outcome
	if outcome == "" {
		outcome = "success"
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	var persistErr error
	for attempt := 0; attempt < 3; attempt++ {
		_, persistErr = p.pool.Exec(persistCtx, `INSERT INTO audit_events(event_key, organisation_id, product_id, actor_id, actor_kind, action, target_type, target_id, prior, current, request_id, outcome, created_at) VALUES ($1, nullif($2, '')::uuid, nullif($3, '')::uuid, $4, 'root', $5, $6, $7, $8, $9, $10, $11, $12) ON CONFLICT (event_key) DO NOTHING`, event.ID, event.OrganisationID, event.ProductID, event.ActorID, event.Action, event.TargetType, event.TargetID, prior, current, event.RequestID, outcome, event.CreatedAt)
		if persistErr == nil {
			return nil
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * 25 * time.Millisecond)
			select {
			case <-persistCtx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return databaseError(errors.Join(persistErr, persistCtx.Err()))
			case <-timer.C:
			}
		}
	}
	return databaseError(persistErr)
}

const auditEventSelect = `SELECT event_key, coalesce(organisation_id::text, ''), coalesce(product_id::text, ''), actor_id, action, target_type, target_id, coalesce(prior, '{}'::jsonb), coalesce(current, '{}'::jsonb), request_id, outcome, created_at FROM audit_events`

func scanAuditEvent(row pgx.Row) (model.AuditEvent, error) {
	var value model.AuditEvent
	var prior, current []byte
	if err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.ActorID, &value.Action,
		&value.TargetType, &value.TargetID, &prior, &current, &value.RequestID, &value.Outcome, &value.CreatedAt); err != nil {
		return model.AuditEvent{}, databaseError(err)
	}
	if err := json.Unmarshal(prior, &value.Prior); err != nil {
		return model.AuditEvent{}, fmt.Errorf("decode audit prior state: %w", err)
	}
	if err := json.Unmarshal(current, &value.Current); err != nil {
		return model.AuditEvent{}, fmt.Errorf("decode audit current state: %w", err)
	}
	return value, nil
}

func (p *Postgres) AuditEvent(ctx context.Context, id string) (model.AuditEvent, error) {
	return scanAuditEvent(p.pool.QueryRow(ctx, auditEventSelect+` WHERE event_key=$1`, id))
}

func (p *Postgres) AuditEvents(ctx context.Context, organisationID string) ([]model.AuditEvent, error) {
	rows, err := p.pool.Query(ctx, auditEventSelect+` WHERE organisation_id = $1 ORDER BY created_at DESC`, organisationID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.AuditEvent, 0)
	for rows.Next() {
		value, scanErr := scanAuditEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
