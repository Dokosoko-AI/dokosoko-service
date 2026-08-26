package store

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

func formatPGVector(values []float32) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	parts := make([]string, len(values))
	for index, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return "", ErrConflict
		}
		parts[index] = strconv.FormatFloat(float64(value), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}

func parsePGVector(value string) ([]float32, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if len(value) < 2 || value[0] != '[' || value[len(value)-1] != ']' {
		return nil, ErrConflict
	}
	parts := strings.Split(value[1:len(value)-1], ",")
	result := make([]float32, len(parts))
	for index, part := range parts {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return nil, err
		}
		result[index] = float32(parsed)
	}
	return result, nil
}

const searchIndexGenerationSelect = `SELECT id::text,deployment_id::text,publication_kind,publication_id::text,asset_kind,builder_version,retrieval_profile_version,embedding_model,embedding_dimensions,state,unit_count,content_hash,diagnostics,started_at,ready_at,created_at FROM search_index_generations`

func scanSearchIndexGeneration(row pgx.Row) (model.SearchIndexGeneration, error) {
	var value model.SearchIndexGeneration
	err := row.Scan(&value.ID, &value.DeploymentID, &value.PublicationKind, &value.PublicationID, &value.AssetKind,
		&value.BuilderVersion, &value.RetrievalProfileVersion, &value.EmbeddingModel, &value.EmbeddingDimensions,
		&value.State, &value.UnitCount, &value.ContentHash, &value.Diagnostics, &value.StartedAt, &value.ReadyAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) SearchIndexGenerations(ctx context.Context, deploymentID, publicationKind, publicationID string) ([]model.SearchIndexGeneration, error) {
	query := searchIndexGenerationSelect + ` WHERE deployment_id=$1`
	args := []any{deploymentID}
	if publicationKind != "" {
		args = append(args, publicationKind)
		query += ` AND publication_kind=$` + postgresArgument(len(args))
	}
	if publicationID != "" {
		args = append(args, publicationID)
		query += ` AND publication_id=$` + postgresArgument(len(args))
	}
	query += ` ORDER BY created_at DESC`
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.SearchIndexGeneration, 0)
	for rows.Next() {
		value, scanErr := scanSearchIndexGeneration(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func scanKnowledgeUnit(row pgx.Row) (model.KnowledgeUnit, error) {
	var value model.KnowledgeUnit
	var embedding string
	err := row.Scan(&value.ID, &value.SearchIndexGenerationID, &value.DeploymentID, &value.Kind, &value.SourcePublicationKind,
		&value.SourcePublicationID, &value.SourceEntityID, &value.ParentSourceEntityID, &value.Title, &value.Breadcrumb,
		&value.Content, &value.Language, &value.Ecosystem, &value.Identifiers, &value.Visibility, &value.Citation,
		&value.Metadata, &value.ContentHash, &value.Ordinal, &embedding)
	if err != nil {
		return value, databaseError(err)
	}
	value.Embedding, err = parsePGVector(embedding)
	return value, err
}

func (p *Postgres) SearchIndexGeneration(ctx context.Context, deploymentID, id string) (SearchIndexGenerationRecord, error) {
	var result SearchIndexGenerationRecord
	value, err := scanSearchIndexGeneration(p.pool.QueryRow(ctx, searchIndexGenerationSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return result, err
	}
	result.Generation = value
	rows, err := p.pool.Query(ctx, `SELECT id::text,search_index_generation_id::text,deployment_id::text,unit_kind,source_publication_kind,source_publication_id::text,source_entity_id::text,coalesce(parent_source_entity_id::text,''),title,breadcrumb,content,language,ecosystem,identifiers,visibility,citation,metadata,content_hash,ordinal,coalesce(embedding::text,'') FROM knowledge_units WHERE deployment_id=$1 AND search_index_generation_id=$2 ORDER BY ordinal`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		item, scanErr := scanKnowledgeUnit(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.Units = append(result.Units, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = p.pool.Query(ctx, `SELECT scope.knowledge_unit_id::text,scope.deployment_id::text,scope.integration_id::text,coalesce(scope.api_sdk_binding_id::text,''),scope.scope_kind,scope.selector_hash,scope.created_at
		FROM knowledge_unit_api_scopes scope JOIN knowledge_units unit ON unit.id=scope.knowledge_unit_id WHERE scope.deployment_id=$1 AND unit.search_index_generation_id=$2 ORDER BY scope.knowledge_unit_id,scope.integration_id`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		var item model.KnowledgeUnitAPIScope
		if err := rows.Scan(&item.KnowledgeUnitID, &item.DeploymentID, &item.APIID, &item.APISDKBindingID,
			&item.ScopeKind, &item.SelectorHash, &item.CreatedAt); err != nil {
			rows.Close()
			return result, databaseError(err)
		}
		result.APIScopes = append(result.APIScopes, item)
	}
	return result, rows.Err()
}

func (p *Postgres) CreateSearchIndexGeneration(ctx context.Context, value model.SearchIndexGeneration) (model.SearchIndexGeneration, error) {
	return scanSearchIndexGeneration(p.pool.QueryRow(ctx, `INSERT INTO search_index_generations(id,deployment_id,publication_kind,publication_id,asset_kind,builder_version,retrieval_profile_version,embedding_model,embedding_dimensions,state,unit_count,content_hash,diagnostics,started_at,ready_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id::text,deployment_id::text,publication_kind,publication_id::text,asset_kind,builder_version,retrieval_profile_version,embedding_model,embedding_dimensions,state,unit_count,content_hash,diagnostics,started_at,ready_at,created_at`,
		value.ID, value.DeploymentID, value.PublicationKind, value.PublicationID, value.AssetKind, value.BuilderVersion,
		value.RetrievalProfileVersion, value.EmbeddingModel, value.EmbeddingDimensions, value.State, value.UnitCount,
		value.ContentHash, value.Diagnostics, value.StartedAt, value.ReadyAt))
}

func (p *Postgres) CompleteSearchIndexGeneration(ctx context.Context, value SearchIndexGenerationRecord, expectedState string) (model.SearchIndexGeneration, error) {
	if value.Generation.State != "ready" || value.Generation.ReadyAt == nil || value.Generation.UnitCount != len(value.Units) {
		return model.SearchIndexGeneration{}, ErrConflict
	}
	unitIDs := make(map[string]bool, len(value.Units))
	ordinals := make(map[int]bool, len(value.Units))
	for _, item := range value.Units {
		if (value.Generation.EmbeddingDimensions == nil && len(item.Embedding) != 0) ||
			(value.Generation.EmbeddingDimensions != nil && len(item.Embedding) != *value.Generation.EmbeddingDimensions) ||
			item.SearchIndexGenerationID != value.Generation.ID || item.DeploymentID != value.Generation.DeploymentID ||
			unitIDs[item.ID] || ordinals[item.Ordinal] {
			return model.SearchIndexGeneration{}, ErrConflict
		}
		unitIDs[item.ID], ordinals[item.Ordinal] = true, true
	}
	scopeKeys := make(map[string]bool, len(value.APIScopes))
	for _, item := range value.APIScopes {
		key := item.KnowledgeUnitID + "\x00" + item.APIID
		if !unitIDs[item.KnowledgeUnitID] || item.DeploymentID != value.Generation.DeploymentID || scopeKeys[key] {
			return model.SearchIndexGeneration{}, ErrConflict
		}
		scopeKeys[key] = true
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.SearchIndexGeneration{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanSearchIndexGeneration(tx.QueryRow(ctx, `UPDATE search_index_generations SET state=$3,unit_count=$4,content_hash=$5,diagnostics=$6,started_at=$7,ready_at=$8
		WHERE deployment_id=$1 AND id=$2 AND state=$9 RETURNING id::text,deployment_id::text,publication_kind,publication_id::text,asset_kind,builder_version,retrieval_profile_version,embedding_model,embedding_dimensions,state,unit_count,content_hash,diagnostics,started_at,ready_at,created_at`,
		value.Generation.DeploymentID, value.Generation.ID, value.Generation.State, value.Generation.UnitCount,
		value.Generation.ContentHash, value.Generation.Diagnostics, value.Generation.StartedAt, value.Generation.ReadyAt, expectedState))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			var exists bool
			if lookupErr := tx.QueryRow(ctx, `SELECT true FROM search_index_generations WHERE deployment_id=$1 AND id=$2`, value.Generation.DeploymentID, value.Generation.ID).Scan(&exists); lookupErr == nil {
				return model.SearchIndexGeneration{}, ErrConflict
			}
		}
		return model.SearchIndexGeneration{}, err
	}
	for _, item := range value.Units {
		embedding, formatErr := formatPGVector(item.Embedding)
		if formatErr != nil {
			return model.SearchIndexGeneration{}, formatErr
		}
		_, err = tx.Exec(ctx, `INSERT INTO knowledge_units(id,search_index_generation_id,deployment_id,unit_kind,source_publication_kind,source_publication_id,source_entity_id,parent_source_entity_id,title,breadcrumb,content,language,ecosystem,identifiers,visibility,citation,metadata,content_hash,ordinal,embedding)
			VALUES($1,$2,$3,$4,$5,$6,$7,nullif($8,'')::uuid,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,nullif($20,'')::vector)`,
			item.ID, item.SearchIndexGenerationID, item.DeploymentID, item.Kind, item.SourcePublicationKind,
			item.SourcePublicationID, item.SourceEntityID, item.ParentSourceEntityID, item.Title, item.Breadcrumb, item.Content,
			item.Language, item.Ecosystem, item.Identifiers, item.Visibility, item.Citation, item.Metadata, item.ContentHash, item.Ordinal, embedding)
		if err != nil {
			return model.SearchIndexGeneration{}, databaseError(err)
		}
	}
	for _, item := range value.APIScopes {
		_, err = tx.Exec(ctx, `INSERT INTO knowledge_unit_api_scopes(knowledge_unit_id,deployment_id,integration_id,api_sdk_binding_id,scope_kind,selector_hash)
			VALUES($1,$2,$3,nullif($4,'')::uuid,$5,$6)`, item.KnowledgeUnitID, item.DeploymentID, item.APIID,
			item.APISDKBindingID, item.ScopeKind, item.SelectorHash)
		if err != nil {
			return model.SearchIndexGeneration{}, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.SearchIndexGeneration{}, err
	}
	return updated, nil
}

func (p *Postgres) FailSearchIndexGeneration(ctx context.Context, value model.SearchIndexGeneration, expectedState string) (model.SearchIndexGeneration, error) {
	if value.State != "failed" {
		return model.SearchIndexGeneration{}, ErrConflict
	}
	updated, err := scanSearchIndexGeneration(p.pool.QueryRow(ctx, `UPDATE search_index_generations SET state=$3,diagnostics=$4,started_at=$5,ready_at=$6 WHERE deployment_id=$1 AND id=$2 AND state=$7
		RETURNING id::text,deployment_id::text,publication_kind,publication_id::text,asset_kind,builder_version,retrieval_profile_version,embedding_model,embedding_dimensions,state,unit_count,content_hash,diagnostics,started_at,ready_at,created_at`,
		value.DeploymentID, value.ID, value.State, value.Diagnostics, value.StartedAt, value.ReadyAt, expectedState))
	if errors.Is(err, ErrNotFound) {
		var exists bool
		if lookupErr := p.pool.QueryRow(ctx, `SELECT true FROM search_index_generations WHERE deployment_id=$1 AND id=$2`, value.DeploymentID, value.ID).Scan(&exists); lookupErr == nil {
			return model.SearchIndexGeneration{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) RetrieveDeveloperAssetKnowledge(ctx context.Context, query DeveloperAssetKnowledgeQuery) ([]DeveloperAssetKnowledgeResult, error) {
	builderVersion := strings.TrimSpace(query.BuilderVersion)
	retrievalProfileVersion := strings.TrimSpace(query.RetrievalProfileVersion)
	if builderVersion == "" || retrievalProfileVersion == "" {
		return nil, ErrConflict
	}
	if query.DeploymentDocumentationPublicationID == "" && query.APIDeveloperAssetPublicationID == "" {
		return nil, ErrConflict
	}
	if query.APIDeveloperAssetPublicationID != "" && query.APIID == "" {
		return nil, ErrConflict
	}
	limit := boundedDeveloperAssetResultLimit(query.Limit)
	if limit == 0 {
		return []DeveloperAssetKnowledgeResult{}, nil
	}
	embedding, err := formatPGVector(query.QueryEmbedding)
	if err != nil {
		return nil, err
	}
	if len(query.QueryEmbedding) > 0 {
		rows, dimensionErr := p.pool.Query(ctx, `SELECT DISTINCT embedding_dimensions FROM search_index_generations WHERE deployment_id=$1 AND state='ready'
			AND builder_version=$4 AND retrieval_profile_version=$5 AND (
			($2<>'' AND publication_kind='global_documentation' AND publication_id=$2::uuid) OR
			($3<>'' AND publication_kind='api' AND publication_id=$3::uuid))`, query.DeploymentID,
			query.DeploymentDocumentationPublicationID, query.APIDeveloperAssetPublicationID,
			builderVersion, retrievalProfileVersion)
		if dimensionErr != nil {
			return nil, databaseError(dimensionErr)
		}
		for rows.Next() {
			var dimension *int
			if scanErr := rows.Scan(&dimension); scanErr != nil {
				rows.Close()
				return nil, databaseError(scanErr)
			}
			if dimension == nil || *dimension != len(query.QueryEmbedding) {
				rows.Close()
				return nil, ErrConflict
			}
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return nil, rowsErr
		}
		rows.Close()
	}
	rows, err := p.pool.Query(ctx, `WITH requested_query AS (
		SELECT CASE WHEN btrim($10)='' THEN NULL ELSE websearch_to_tsquery('english',$10) END AS text_value,
		       nullif($12,'')::vector AS embedding_value
	), scored AS (
		SELECT unit.id,unit.search_index_generation_id,unit.deployment_id,unit.unit_kind,unit.source_publication_kind,
			unit.source_publication_id,unit.source_entity_id,unit.parent_source_entity_id,unit.title,unit.breadcrumb,
			unit.content,unit.language,unit.ecosystem,unit.identifiers,unit.visibility,unit.citation,unit.metadata,
			unit.content_hash,unit.ordinal,unit.embedding,
			coalesce(ts_rank_cd(unit.content_tsv,requested_query.text_value),0) + CASE WHEN EXISTS (
				SELECT 1 FROM unnest(unit.identifiers) identifier WHERE lower(identifier)=lower($10)
			) THEN 1 ELSE 0 END AS lexical_score,
			CASE WHEN requested_query.embedding_value IS NULL THEN 0
			     WHEN unit.embedding IS NULL THEN -1
			     ELSE 1 - (unit.embedding <=> requested_query.embedding_value) END AS semantic_score
		FROM knowledge_units unit
		JOIN search_index_generations generation ON generation.id=unit.search_index_generation_id AND generation.state='ready'
			AND generation.builder_version=$13 AND generation.retrieval_profile_version=$14
		CROSS JOIN requested_query
		WHERE unit.deployment_id=$1
		  AND (
			($2<>'' AND generation.publication_kind='global_documentation' AND generation.publication_id=$2::uuid)
			OR ($3<>'' AND generation.publication_kind='api' AND generation.publication_id=$3::uuid AND EXISTS (
				SELECT 1 FROM knowledge_unit_api_scopes scope WHERE scope.knowledge_unit_id=unit.id AND scope.integration_id=$4::uuid
			))
		  )
		  AND (coalesce(array_length($5::text[],1),0)=0 OR
		       coalesce(nullif(unit.metadata->>'asset_kind',''),generation.asset_kind)=ANY($5::text[]))
		  AND (coalesce(array_length($6::text[],1),0)=0 OR unit.language=ANY($6::text[]))
		  AND (coalesce(array_length($7::text[],1),0)=0 OR unit.ecosystem=ANY($7::text[]))
		  AND (coalesce(array_length($8::text[],1),0)=0 OR unit.metadata->>'sdk_release_id'=ANY($8::text[]))
		  AND (coalesce(array_length($9::text[],1),0)=0 OR unit.metadata->>'exact_version'=ANY($9::text[]))
		  AND (requested_query.embedding_value IS NOT NULL OR requested_query.text_value IS NULL OR unit.content_tsv @@ requested_query.text_value OR EXISTS (
			SELECT 1 FROM unnest(unit.identifiers) identifier WHERE lower(identifier)=lower($10)
		  ))
	) SELECT id::text,search_index_generation_id::text,deployment_id::text,unit_kind,source_publication_kind,
		source_publication_id::text,source_entity_id::text,coalesce(parent_source_entity_id::text,''),title,breadcrumb,
		content,language,ecosystem,identifiers,visibility,citation,metadata,content_hash,ordinal,coalesce(embedding::text,''),
		lexical_score,semantic_score,
		CASE WHEN $12='' THEN lexical_score ELSE
			0.45 * (lexical_score / (1 + lexical_score)) +
			0.55 * greatest(0,least(1,(semantic_score + 1) / 2)) END AS fused_score
	FROM scored ORDER BY fused_score DESC,ordinal,id LIMIT $11`, query.DeploymentID,
		query.DeploymentDocumentationPublicationID, query.APIDeveloperAssetPublicationID, query.APIID,
		query.AssetKinds, query.Languages, query.Ecosystems, query.SDKReleaseIDs, query.ExactVersions,
		query.QueryText, limit, embedding, builderVersion, retrievalProfileVersion)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]DeveloperAssetKnowledgeResult, 0)
	for rows.Next() {
		var item DeveloperAssetKnowledgeResult
		var embeddingValue string
		if err := rows.Scan(&item.Unit.ID, &item.Unit.SearchIndexGenerationID, &item.Unit.DeploymentID, &item.Unit.Kind,
			&item.Unit.SourcePublicationKind, &item.Unit.SourcePublicationID, &item.Unit.SourceEntityID,
			&item.Unit.ParentSourceEntityID, &item.Unit.Title, &item.Unit.Breadcrumb, &item.Unit.Content,
			&item.Unit.Language, &item.Unit.Ecosystem, &item.Unit.Identifiers, &item.Unit.Visibility,
			&item.Unit.Citation, &item.Unit.Metadata, &item.Unit.ContentHash, &item.Unit.Ordinal, &embeddingValue,
			&item.LexicalScore, &item.SemanticScore, &item.FusedScore); err != nil {
			return nil, databaseError(err)
		}
		item.Unit.Embedding, err = parsePGVector(embeddingValue)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

const retrievalQueryTraceSelect = `SELECT id::text,deployment_id::text,coalesce(deployment_documentation_publication_id::text,''),coalesce(api_developer_asset_publication_id::text,''),retrieval_profile_version,query_text,query_hash,requested_filters,resolved_scope,routing_decision,state,candidate_count,result_count,context_tokens,latency_ms,diagnostics,expires_at,created_at FROM retrieval_query_traces`

func scanRetrievalQueryTrace(row pgx.Row) (model.RetrievalQueryTrace, error) {
	var value model.RetrievalQueryTrace
	err := row.Scan(&value.ID, &value.DeploymentID, &value.DeploymentDocumentationPublicationID,
		&value.APIDeveloperAssetPublicationID, &value.RetrievalProfileVersion, &value.QueryText, &value.QueryHash,
		&value.RequestedFilters, &value.ResolvedScope, &value.RoutingDecision, &value.State, &value.CandidateCount,
		&value.ResultCount, &value.ContextTokens, &value.LatencyMS, &value.Diagnostics, &value.ExpiresAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) RetrievalQueryTraces(ctx context.Context, deploymentID string, since time.Time, limit int) ([]model.RetrievalQueryTrace, error) {
	limit = boundedDeveloperAssetResultLimit(limit)
	if limit == 0 {
		return []model.RetrievalQueryTrace{}, nil
	}
	rows, err := p.pool.Query(ctx, retrievalQueryTraceSelect+` WHERE deployment_id=$1 AND created_at >= $2 ORDER BY created_at DESC LIMIT $3`, deploymentID, since, limit)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.RetrievalQueryTrace, 0)
	for rows.Next() {
		value, scanErr := scanRetrievalQueryTrace(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) RetrievalQueryTrace(ctx context.Context, deploymentID, id string) (RetrievalQueryTraceRecord, error) {
	var result RetrievalQueryTraceRecord
	value, err := scanRetrievalQueryTrace(p.pool.QueryRow(ctx, retrievalQueryTraceSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return result, err
	}
	result.Trace = value
	rows, err := p.pool.Query(ctx, `SELECT retrieval_query_trace_id::text,rank,coalesce(knowledge_unit_id::text,''),source_publication_kind,source_publication_id::text,source_entity_id::text,lexical_score,semantic_score,rerank_score,selected,exclusion_reason,citation,excerpt,content_hash FROM retrieval_query_trace_results WHERE deployment_id=$1 AND retrieval_query_trace_id=$2 ORDER BY rank`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var item model.RetrievalQueryTraceResult
		if err := rows.Scan(&item.RetrievalQueryTraceID, &item.Rank, &item.KnowledgeUnitID, &item.SourcePublicationKind,
			&item.SourcePublicationID, &item.SourceEntityID, &item.LexicalScore, &item.SemanticScore, &item.RerankScore,
			&item.Selected, &item.ExclusionReason, &item.Citation, &item.Excerpt, &item.ContentHash); err != nil {
			return result, databaseError(err)
		}
		result.Results = append(result.Results, item)
	}
	return result, rows.Err()
}

func (p *Postgres) AppendRetrievalQueryTrace(ctx context.Context, value RetrievalQueryTraceRecord) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	trace := value.Trace
	_, err = tx.Exec(ctx, `INSERT INTO retrieval_query_traces(id,deployment_id,deployment_documentation_publication_id,api_developer_asset_publication_id,retrieval_profile_version,query_text,query_hash,requested_filters,resolved_scope,routing_decision,state,candidate_count,result_count,context_tokens,latency_ms,diagnostics,expires_at)
		VALUES($1,$2,nullif($3,'')::uuid,nullif($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		trace.ID, trace.DeploymentID, trace.DeploymentDocumentationPublicationID, trace.APIDeveloperAssetPublicationID,
		trace.RetrievalProfileVersion, trace.QueryText, trace.QueryHash, trace.RequestedFilters, trace.ResolvedScope,
		trace.RoutingDecision, trace.State, trace.CandidateCount, trace.ResultCount, trace.ContextTokens, trace.LatencyMS,
		trace.Diagnostics, trace.ExpiresAt)
	if err != nil {
		return databaseError(err)
	}
	for _, item := range value.Results {
		_, err = tx.Exec(ctx, `INSERT INTO retrieval_query_trace_results(retrieval_query_trace_id,deployment_id,rank,knowledge_unit_id,source_publication_kind,source_publication_id,source_entity_id,lexical_score,semantic_score,rerank_score,selected,exclusion_reason,citation,excerpt,content_hash)
			VALUES($1,$2,$3,nullif($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, trace.ID, trace.DeploymentID,
			item.Rank, item.KnowledgeUnitID, item.SourcePublicationKind, item.SourcePublicationID, item.SourceEntityID,
			item.LexicalScore, item.SemanticScore, item.RerankScore, item.Selected, item.ExclusionReason, item.Citation,
			item.Excerpt, item.ContentHash)
		if err != nil {
			return databaseError(err)
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) DeleteExpiredRetrievalQueryTraces(ctx context.Context, before time.Time, limit int) (int64, error) {
	limit = boundedDeveloperAssetResultLimit(limit)
	if limit == 0 {
		return 0, nil
	}
	result, err := p.pool.Exec(ctx, `WITH expired AS MATERIALIZED (
		SELECT trace.id FROM retrieval_query_traces trace
		WHERE trace.expires_at IS NOT NULL AND trace.expires_at <= $1
		  AND NOT EXISTS (
			SELECT 1 FROM retrieval_evaluation_case_results evaluation
			WHERE evaluation.retrieval_query_trace_id=trace.id
		  )
		ORDER BY trace.expires_at LIMIT $2 FOR UPDATE SKIP LOCKED
	), deleted_results AS (
		DELETE FROM retrieval_query_trace_results result USING expired
		WHERE result.retrieval_query_trace_id=expired.id RETURNING result.retrieval_query_trace_id
	)
	DELETE FROM retrieval_query_traces trace USING expired WHERE trace.id=expired.id`, before, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	return result.RowsAffected(), nil
}

const retrievalEvaluationSetSelect = `SELECT id::text,deployment_id::text,name,description,lifecycle,revision,created_by,created_at,updated_at FROM retrieval_evaluation_sets`

func scanRetrievalEvaluationSet(row pgx.Row) (model.RetrievalEvaluationSet, error) {
	var value model.RetrievalEvaluationSet
	err := row.Scan(&value.ID, &value.DeploymentID, &value.Name, &value.Description, &value.Lifecycle, &value.Revision,
		&value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) RetrievalEvaluationSets(ctx context.Context, deploymentID string) ([]model.RetrievalEvaluationSet, error) {
	rows, err := p.pool.Query(ctx, retrievalEvaluationSetSelect+` WHERE deployment_id=$1 ORDER BY name`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.RetrievalEvaluationSet, 0)
	for rows.Next() {
		value, scanErr := scanRetrievalEvaluationSet(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) RetrievalEvaluationSet(ctx context.Context, deploymentID, id string) (model.RetrievalEvaluationSet, error) {
	return scanRetrievalEvaluationSet(p.pool.QueryRow(ctx, retrievalEvaluationSetSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func insertRetrievalEvaluationRevisionTx(ctx context.Context, tx pgx.Tx, deploymentID string, record RetrievalEvaluationSetRevisionRecord) error {
	value := record.Revision
	_, err := tx.Exec(ctx, `INSERT INTO retrieval_evaluation_set_revisions(id,deployment_id,retrieval_evaluation_set_id,revision,content_hash,created_by) VALUES($1,$2,$3,$4,$5,$6)`,
		value.ID, deploymentID, value.RetrievalEvaluationSetID, value.Revision, value.ContentHash, value.CreatedBy)
	if err != nil {
		return databaseError(err)
	}
	for _, item := range record.Cases {
		_, err = tx.Exec(ctx, `INSERT INTO retrieval_evaluation_cases(id,deployment_id,retrieval_evaluation_set_revision_id,case_key,query,requested_filters,expected_evidence,forbidden_evidence,expect_no_results)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, item.ID, deploymentID, item.RetrievalEvaluationSetRevisionID,
			item.CaseKey, item.Query, item.RequestedFilters, item.ExpectedEvidence, item.ForbiddenEvidence, item.ExpectNoResults)
		if err != nil {
			return databaseError(err)
		}
	}
	return nil
}

func (p *Postgres) CreateRetrievalEvaluationSet(ctx context.Context, value model.RetrievalEvaluationSet, record RetrievalEvaluationSetRevisionRecord) (model.RetrievalEvaluationSet, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.RetrievalEvaluationSet{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanRetrievalEvaluationSet(tx.QueryRow(ctx, `INSERT INTO retrieval_evaluation_sets(id,deployment_id,name,description,lifecycle,created_by) VALUES($1,$2,$3,$4,$5,$6)
		RETURNING id::text,deployment_id::text,name,description,lifecycle,revision,created_by,created_at,updated_at`,
		value.ID, value.DeploymentID, value.Name, value.Description, value.Lifecycle, value.CreatedBy))
	if err != nil {
		return model.RetrievalEvaluationSet{}, err
	}
	record.Revision.Revision = created.Revision
	if err := insertRetrievalEvaluationRevisionTx(ctx, tx, value.DeploymentID, record); err != nil {
		return model.RetrievalEvaluationSet{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.RetrievalEvaluationSet{}, err
	}
	return created, nil
}

func (p *Postgres) ReviseRetrievalEvaluationSet(ctx context.Context, value model.RetrievalEvaluationSet, expected int64, record RetrievalEvaluationSetRevisionRecord) (model.RetrievalEvaluationSet, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.RetrievalEvaluationSet{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanRetrievalEvaluationSet(tx.QueryRow(ctx, `UPDATE retrieval_evaluation_sets SET name=$3,description=$4,lifecycle=$5,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id=$2 AND revision=$6
		RETURNING id::text,deployment_id::text,name,description,lifecycle,revision,created_by,created_at,updated_at`, value.DeploymentID, value.ID, value.Name, value.Description, value.Lifecycle, expected))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			var exists bool
			if lookupErr := tx.QueryRow(ctx, `SELECT true FROM retrieval_evaluation_sets WHERE deployment_id=$1 AND id=$2`, value.DeploymentID, value.ID).Scan(&exists); lookupErr == nil {
				return model.RetrievalEvaluationSet{}, ErrConflict
			}
		}
		return model.RetrievalEvaluationSet{}, err
	}
	record.Revision.Revision = updated.Revision
	if err := insertRetrievalEvaluationRevisionTx(ctx, tx, value.DeploymentID, record); err != nil {
		return model.RetrievalEvaluationSet{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.RetrievalEvaluationSet{}, err
	}
	return updated, nil
}

func (p *Postgres) RetrievalEvaluationSetRevisions(ctx context.Context, deploymentID, setID string) ([]model.RetrievalEvaluationSetRevision, error) {
	rows, err := p.pool.Query(ctx, `SELECT revision.id::text,revision.retrieval_evaluation_set_id::text,revision.revision,revision.content_hash,revision.created_by,revision.created_at FROM retrieval_evaluation_set_revisions revision WHERE revision.deployment_id=$1 AND revision.retrieval_evaluation_set_id=$2 ORDER BY revision.revision DESC`, deploymentID, setID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.RetrievalEvaluationSetRevision, 0)
	for rows.Next() {
		var value model.RetrievalEvaluationSetRevision
		if err := rows.Scan(&value.ID, &value.RetrievalEvaluationSetID, &value.Revision, &value.ContentHash, &value.CreatedBy, &value.CreatedAt); err != nil {
			return nil, databaseError(err)
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) RetrievalEvaluationSetRevision(ctx context.Context, deploymentID, id string) (RetrievalEvaluationSetRevisionRecord, error) {
	var result RetrievalEvaluationSetRevisionRecord
	err := p.pool.QueryRow(ctx, `SELECT id::text,retrieval_evaluation_set_id::text,revision,content_hash,created_by,created_at FROM retrieval_evaluation_set_revisions WHERE deployment_id=$1 AND id=$2`, deploymentID, id).
		Scan(&result.Revision.ID, &result.Revision.RetrievalEvaluationSetID, &result.Revision.Revision,
			&result.Revision.ContentHash, &result.Revision.CreatedBy, &result.Revision.CreatedAt)
	if err != nil {
		return result, databaseError(err)
	}
	rows, err := p.pool.Query(ctx, `SELECT id::text,retrieval_evaluation_set_revision_id::text,case_key,query,requested_filters,expected_evidence,forbidden_evidence,expect_no_results FROM retrieval_evaluation_cases WHERE deployment_id=$1 AND retrieval_evaluation_set_revision_id=$2 ORDER BY case_key`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var item model.RetrievalEvaluationCase
		if err := rows.Scan(&item.ID, &item.RetrievalEvaluationSetRevisionID, &item.CaseKey, &item.Query,
			&item.RequestedFilters, &item.ExpectedEvidence, &item.ForbiddenEvidence, &item.ExpectNoResults); err != nil {
			return result, databaseError(err)
		}
		result.Cases = append(result.Cases, item)
	}
	return result, rows.Err()
}

func scanRetrievalEvaluationRun(row pgx.Row) (model.RetrievalEvaluationRun, error) {
	var value model.RetrievalEvaluationRun
	err := row.Scan(&value.ID, &value.DeploymentID, &value.RetrievalEvaluationSetRevisionID,
		&value.DeploymentDocumentationPublicationID, &value.APIDeveloperAssetPublicationID, &value.RetrievalProfileVersion,
		&value.State, &value.Metrics, &value.StartedAt, &value.FinishedAt, &value.CreatedBy, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) RetrievalEvaluationRuns(ctx context.Context, deploymentID, revisionID string) ([]model.RetrievalEvaluationRun, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text,deployment_id::text,retrieval_evaluation_set_revision_id::text,coalesce(deployment_documentation_publication_id::text,''),coalesce(api_developer_asset_publication_id::text,''),retrieval_profile_version,state,metrics,started_at,finished_at,created_by,created_at FROM retrieval_evaluation_runs WHERE deployment_id=$1 AND retrieval_evaluation_set_revision_id=$2 ORDER BY created_at DESC`, deploymentID, revisionID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.RetrievalEvaluationRun, 0)
	for rows.Next() {
		value, scanErr := scanRetrievalEvaluationRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) RetrievalEvaluationRun(ctx context.Context, deploymentID, id string) (RetrievalEvaluationRunRecord, error) {
	var result RetrievalEvaluationRunRecord
	value, err := scanRetrievalEvaluationRun(p.pool.QueryRow(ctx, `SELECT id::text,deployment_id::text,retrieval_evaluation_set_revision_id::text,coalesce(deployment_documentation_publication_id::text,''),coalesce(api_developer_asset_publication_id::text,''),retrieval_profile_version,state,metrics,started_at,finished_at,created_by,created_at FROM retrieval_evaluation_runs WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return result, err
	}
	result.Run = value
	rows, err := p.pool.Query(ctx, `SELECT retrieval_evaluation_run_id::text,deployment_id::text,retrieval_evaluation_set_revision_id::text,retrieval_evaluation_case_id::text,coalesce(retrieval_query_trace_id::text,''),passed,metrics,failure_reason,created_at FROM retrieval_evaluation_case_results WHERE deployment_id=$1 AND retrieval_evaluation_run_id=$2 ORDER BY retrieval_evaluation_case_id`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var item model.RetrievalEvaluationCaseResult
		if err := rows.Scan(&item.RetrievalEvaluationRunID, &item.DeploymentID, &item.RetrievalEvaluationSetRevisionID, &item.RetrievalEvaluationCaseID,
			&item.RetrievalQueryTraceID, &item.Passed, &item.Metrics, &item.FailureReason, &item.CreatedAt); err != nil {
			return result, databaseError(err)
		}
		result.Results = append(result.Results, item)
	}
	return result, rows.Err()
}

func (p *Postgres) CreateRetrievalEvaluationRun(ctx context.Context, value model.RetrievalEvaluationRun) (model.RetrievalEvaluationRun, error) {
	return scanRetrievalEvaluationRun(p.pool.QueryRow(ctx, `INSERT INTO retrieval_evaluation_runs(id,deployment_id,retrieval_evaluation_set_revision_id,deployment_documentation_publication_id,api_developer_asset_publication_id,retrieval_profile_version,state,metrics,started_at,finished_at,created_by)
		VALUES($1,$2,$3,nullif($4,'')::uuid,nullif($5,'')::uuid,$6,$7,$8,$9,$10,$11)
		RETURNING id::text,deployment_id::text,retrieval_evaluation_set_revision_id::text,coalesce(deployment_documentation_publication_id::text,''),coalesce(api_developer_asset_publication_id::text,''),retrieval_profile_version,state,metrics,started_at,finished_at,created_by,created_at`,
		value.ID, value.DeploymentID, value.RetrievalEvaluationSetRevisionID, value.DeploymentDocumentationPublicationID,
		value.APIDeveloperAssetPublicationID, value.RetrievalProfileVersion, value.State, value.Metrics,
		value.StartedAt, value.FinishedAt, value.CreatedBy))
}

func (p *Postgres) CompleteRetrievalEvaluationRun(ctx context.Context, value RetrievalEvaluationRunRecord, expectedState string) (model.RetrievalEvaluationRun, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.RetrievalEvaluationRun{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanRetrievalEvaluationRun(tx.QueryRow(ctx, `UPDATE retrieval_evaluation_runs SET state=$3,metrics=$4,started_at=$5,finished_at=$6 WHERE deployment_id=$1 AND id=$2 AND state=$7
		RETURNING id::text,deployment_id::text,retrieval_evaluation_set_revision_id::text,coalesce(deployment_documentation_publication_id::text,''),coalesce(api_developer_asset_publication_id::text,''),retrieval_profile_version,state,metrics,started_at,finished_at,created_by,created_at`,
		value.Run.DeploymentID, value.Run.ID, value.Run.State, value.Run.Metrics, value.Run.StartedAt, value.Run.FinishedAt, expectedState))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			var exists bool
			if lookupErr := tx.QueryRow(ctx, `SELECT true FROM retrieval_evaluation_runs WHERE deployment_id=$1 AND id=$2`, value.Run.DeploymentID, value.Run.ID).Scan(&exists); lookupErr == nil {
				return model.RetrievalEvaluationRun{}, ErrConflict
			}
		}
		return model.RetrievalEvaluationRun{}, err
	}
	for _, item := range value.Results {
		_, err = tx.Exec(ctx, `INSERT INTO retrieval_evaluation_case_results(retrieval_evaluation_run_id,deployment_id,retrieval_evaluation_set_revision_id,retrieval_evaluation_case_id,retrieval_query_trace_id,passed,metrics,failure_reason)
			VALUES($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,$8)`, updated.ID, updated.DeploymentID, updated.RetrievalEvaluationSetRevisionID, item.RetrievalEvaluationCaseID,
			item.RetrievalQueryTraceID, item.Passed, item.Metrics, item.FailureReason)
		if err != nil {
			return model.RetrievalEvaluationRun{}, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.RetrievalEvaluationRun{}, err
	}
	return updated, nil
}
