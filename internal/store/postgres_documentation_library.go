package store

import "context"

func (p *Postgres) DocumentationLibrary(ctx context.Context, query DocumentationLibraryQuery) (DocumentationLibraryPage, error) {
	if query.Reviewed {
		return p.reviewedDocumentationLibrary(ctx, query)
	}
	result := DocumentationLibraryPage{Items: []DocumentationLibraryItem{}}
	limit := boundedDeveloperAssetResultLimit(query.Limit)
	if limit == 0 {
		return result, nil
	}
	// Rank before searching: an old matching paragraph must never make a
	// superseded document appear in the current library. Two bounded queries
	// replace the explorer's per-document detail hydration.
	filter := `WITH ranked AS (
 SELECT document.id,document.ingestion_run_id,document.source_path,document.title,document.content_hash,document.visibility,document.normalized_markdown,coalesce(run.source_id::text,'') AS source_id,run.queued_at,
 row_number() OVER version_order AS position,
 lead(document.id::text,1,'') OVER version_order AS previous_document_id
 FROM documentation_documents document JOIN developer_asset_ingestion_runs run ON run.id=document.ingestion_run_id AND run.deployment_id=document.deployment_id
 WHERE document.deployment_id=$1 AND ($2='' OR run.source_id::text=$2)
 WINDOW version_order AS (PARTITION BY coalesce(run.source_id::text,run.target_key),document.source_path ORDER BY run.queued_at DESC,run.id DESC,document.id)
), selected AS (
 SELECT * FROM ranked document WHERE ($3 OR position=1) AND ($4='' OR strpos(lower(title || ' ' || source_path || ' ' || normalized_markdown),lower($4))>0)
 AND ($5='' OR EXISTS(SELECT 1 FROM source_publication_document_selections selection JOIN source_publications publication ON publication.id=selection.source_publication_id AND publication.product_id=selection.deployment_id
 WHERE selection.documentation_document_id=document.id AND selection.deployment_id=$1 AND publication.id::text=$5 AND publication.crawl_job_id=document.ingestion_run_id AND publication.source_id::text=document.source_id AND selection.content_hash=document.content_hash AND selection.decision='included'))
) `
	args := []any{query.DeploymentID, query.SourceID, query.History, query.Query, query.SourcePublicationID}
	if err := p.pool.QueryRow(ctx, filter+`SELECT count(*) FROM selected`, args...).Scan(&result.Total); err != nil {
		return result, databaseError(err)
	}
	if query.Offset >= result.Total {
		return result, nil
	}
	args = append(args, limit, max(query.Offset, 0))
	rows, err := p.pool.Query(ctx, filter+`SELECT document.id::text,document.source_id,document.ingestion_run_id::text,document.source_path,document.title,document.content_hash,document.visibility,document.previous_document_id,document.queued_at,
 coalesce((SELECT selection.decision FROM source_publication_document_selections selection JOIN source_publications publication ON publication.id=selection.source_publication_id AND publication.product_id=selection.deployment_id WHERE selection.documentation_document_id=document.id AND selection.deployment_id=$1 AND publication.crawl_job_id=document.ingestion_run_id AND publication.source_id::text=document.source_id AND selection.content_hash=document.content_hash AND ($5='' OR publication.id::text=$5) ORDER BY publication.revision DESC LIMIT 1),'unreviewed')
 FROM selected document ORDER BY document.queued_at DESC,document.ingestion_run_id DESC,document.id LIMIT $6 OFFSET $7`, args...)
	if err != nil {
		return result, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var item DocumentationLibraryItem
		if err := rows.Scan(&item.ID, &item.SourceID, &item.IngestionRunID, &item.SourcePath, &item.Title, &item.ContentHash, &item.Visibility, &item.PreviousDocumentID, &item.QueuedAt, &item.Decision); err != nil {
			return result, databaseError(err)
		}
		result.Items = append(result.Items, item)
	}
	result.HasMore = max(query.Offset, 0)+len(result.Items) < result.Total
	return result, rows.Err()
}

func (p *Postgres) reviewedDocumentationLibrary(ctx context.Context, query DocumentationLibraryQuery) (DocumentationLibraryPage, error) {
	result := DocumentationLibraryPage{Items: []DocumentationLibraryItem{}}
	// Limit the library to the latest publication before joining its documents.
	// Previous-content lookup then stays within each displayed source's history;
	// it must not repeatedly scan a materialized deployment-wide history.
	filter := `WITH latest AS (
 SELECT DISTINCT ON (source_id) * FROM source_publications
 WHERE product_id=$1 AND ($2='' OR source_id::text=$2)
 ORDER BY source_id,revision DESC
), selected AS (
 SELECT document.id,document.source_path,document.title,document.content_hash,document.normalized_markdown,
 publication.source_id,publication.crawl_job_id,publication.visibility,publication.revision,publication.published_at,run.queued_at,
 source.kind='upload' AND (SELECT count(*) FROM documentation_documents imported WHERE imported.ingestion_run_id=run.id AND imported.deployment_id=run.deployment_id)=1 AS single_upload
 FROM latest publication
 JOIN sources source ON source.id=publication.source_id AND source.product_id=publication.product_id
 JOIN developer_asset_ingestion_runs run ON run.id=publication.crawl_job_id AND run.deployment_id=publication.product_id AND run.source_id=publication.source_id
 JOIN source_publication_document_selections selection ON selection.source_publication_id=publication.id AND selection.deployment_id=publication.product_id AND selection.decision='included'
 JOIN documentation_documents document ON document.id=selection.documentation_document_id AND document.deployment_id=publication.product_id AND document.content_hash=selection.content_hash AND document.ingestion_run_id=publication.crawl_job_id
 WHERE ($3='' OR strpos(lower(document.title || ' ' || document.source_path || ' ' || document.normalized_markdown),lower($3))>0)
) `
	args := []any{query.DeploymentID, query.SourceID, query.Query}
	if err := p.pool.QueryRow(ctx, filter+`SELECT count(*) FROM selected`, args...).Scan(&result.Total); err != nil {
		return result, databaseError(err)
	}
	limit, offset := boundedDeveloperAssetResultLimit(query.Limit), max(query.Offset, 0)
	if offset >= result.Total || limit == 0 {
		return result, nil
	}
	rows, err := p.pool.Query(ctx, filter+`SELECT document.id::text,document.source_id::text,document.crawl_job_id::text,document.source_path,document.title,document.content_hash,document.visibility,
 coalesce((SELECT previous.id::text FROM source_publications publication
 JOIN developer_asset_ingestion_runs run ON run.id=publication.crawl_job_id AND run.deployment_id=publication.product_id AND run.source_id=publication.source_id
 JOIN source_publication_document_selections selection ON selection.source_publication_id=publication.id AND selection.deployment_id=publication.product_id AND selection.decision='included'
 JOIN documentation_documents previous ON previous.id=selection.documentation_document_id AND previous.deployment_id=publication.product_id AND previous.content_hash=selection.content_hash AND previous.ingestion_run_id=publication.crawl_job_id
 WHERE publication.product_id=$1 AND publication.source_id=document.source_id AND publication.revision<document.revision
 AND (run.queued_at,run.id)<(document.queued_at,document.crawl_job_id)
 AND (previous.source_path=document.source_path OR (document.single_upload AND (SELECT count(*) FROM documentation_documents imported WHERE imported.ingestion_run_id=run.id AND imported.deployment_id=run.deployment_id)=1))
 ORDER BY publication.revision DESC,previous.id LIMIT 1),''),document.queued_at
 FROM selected document ORDER BY document.queued_at DESC,document.source_id,document.id LIMIT $4 OFFSET $5`, append(args, limit, offset)...)
	if err != nil {
		return result, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var item DocumentationLibraryItem
		if err := rows.Scan(&item.ID, &item.SourceID, &item.IngestionRunID, &item.SourcePath, &item.Title, &item.ContentHash, &item.Visibility, &item.PreviousDocumentID, &item.QueuedAt); err != nil {
			return result, err
		}
		item.Decision = "included"
		result.Items = append(result.Items, item)
	}
	result.HasMore = offset+len(result.Items) < result.Total
	return result, rows.Err()
}
