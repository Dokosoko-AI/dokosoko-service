package store

import (
	"context"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

// RelevantPrivateKnowledge uses the existing indexed document tsvector. The
// operator outcome is converted to bounded lexemes by PostgreSQL rather than
// interpreted as tsquery syntax. Results are interleaved by source rank so one
// verbose documentation source cannot crowd every other reviewed source out of
// the prompt boundary.
func (p *Postgres) RelevantPrivateKnowledge(ctx context.Context, productID string, publicationIDs []string, outcome string, limit int) ([]model.KnowledgeRecord, error) {
	limit = boundedRelevantKnowledgeLimit(limit)
	outcome = boundedRelevantKnowledgeQuery(outcome)
	if limit == 0 || len(relevantKnowledgeTerms(outcome)) == 0 {
		return []model.KnowledgeRecord{}, nil
	}

	rows, err := p.pool.Query(ctx, `
		WITH search_terms AS (
			SELECT lexeme
			FROM unnest(tsvector_to_array(to_tsvector('english', $3))) AS term(lexeme)
			LIMIT 32
		), search_query AS (
			SELECT CASE
				WHEN count(*) = 0 THEN NULL::tsquery
				ELSE to_tsquery('english', string_agg(quote_literal(lexeme), ' | ' ORDER BY lexeme))
			END AS value
			FROM search_terms
		), eligible AS (
			SELECT DISTINCT kd.id, kd.product_id, kd.source_id, kd.title, kd.body,
			       kd.canonical_url, kd.visibility, kd.body_tsv
			FROM source_publications sp
			JOIN source_publication_documents spd ON spd.source_publication_id = sp.id
			JOIN knowledge_documents kd
			  ON kd.id = spd.knowledge_document_id
			 AND kd.product_id = sp.product_id
			 AND kd.source_id = sp.source_id
			JOIN sources s
			  ON s.id = sp.source_id
			 AND s.product_id = sp.product_id
			CROSS JOIN search_query query
			WHERE sp.product_id = $1 AND sp.id = ANY($2::uuid[])
			  AND s.state = 'published' AND kd.state = 'published'
			  AND kd.injection_indicators = '[]'::jsonb
			  AND query.value IS NOT NULL AND kd.body_tsv @@ query.value
		), ranked AS (
			SELECT eligible.*,
			       ts_rank_cd(body_tsv, query.value, 32)
			       + 4 * ts_rank_cd(to_tsvector('english', title), query.value, 32)
			       + CASE WHEN position(lower($3) in lower(title)) > 0 THEN 2 ELSE 0 END
			       + CASE WHEN position(lower($3) in lower(body)) > 0 THEN 0.5 ELSE 0 END AS relevance
			FROM eligible
			CROSS JOIN search_query query
		), diverse AS (
			SELECT ranked.*,
			       row_number() OVER (PARTITION BY source_id ORDER BY relevance DESC, id) AS source_rank
			FROM ranked
		)
		SELECT id::text, product_id::text, source_id::text, title, body,
		       canonical_url, visibility::text
		FROM diverse
		ORDER BY source_rank, relevance DESC, id
		LIMIT $4`, productID, publicationIDs, outcome, limit)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.KnowledgeRecord, 0, limit)
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
