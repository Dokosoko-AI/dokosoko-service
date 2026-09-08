package store

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func documentationAttentionStatus(source model.Source, job *model.CrawlJob, publication bool, saved bool) string {
	if job == nil {
		return "not_imported"
	}
	if job.State == "queued" || job.State == "running" {
		return "importing"
	}
	if job.State == "failed" || job.State == "cancelled" {
		return "import_failed"
	}
	if source.Quarantined || job.FailedCount > 0 || job.SkippedCount > 0 {
		return "blocked"
	}
	if !publication {
		return "review"
	}
	if !saved {
		return "save_version"
	}
	return ""
}

func (m *Memory) DocumentationAttention(_ context.Context, deploymentID, sourceID string, limit, offset int) (DocumentationAttentionPage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := []DocumentationAttentionItem{}
	for _, source := range m.sources[deploymentID] {
		if source.ProductID != deploymentID || (sourceID != "" && source.ID != sourceID) || (source.Kind != "website" && source.Kind != "upload") {
			continue
		}
		contract := false
		for _, binding := range m.developerAssets.contractSources {
			if binding.DeploymentID == deploymentID && binding.SourceID == source.ID && binding.Lifecycle == "attached" {
				contract = true
				break
			}
		}
		if contract {
			continue
		}
		var latest *model.CrawlJob
		for _, job := range m.crawls[source.ID] {
			if job.ProductID != deploymentID || job.SourceID != source.ID {
				continue
			}
			if latest == nil || job.QueuedAt.After(latest.QueuedAt) || job.QueuedAt.Equal(latest.QueuedAt) && job.ID > latest.ID {
				copy := job
				latest = &copy
			}
		}
		var publication *model.SourcePublication
		if latest != nil {
			for _, value := range m.sourcePublications[deploymentID] {
				if value.ProductID == deploymentID && value.SourceID == source.ID && value.CrawlJobID == latest.ID && (publication == nil || value.Revision > publication.Revision) {
					copy := value
					publication = &copy
				}
			}
		}
		saved := false
		if publication != nil {
			for _, record := range m.developerAssets.documentationRevisions {
				root := m.developerAssets.documentationCollections[record.Revision.DocumentationCollectionID]
				if record.Revision.DeploymentID != deploymentID || root.DeploymentID != deploymentID || root.Lifecycle != "active" || record.Revision.Visibility != publication.Visibility {
					continue
				}
				for _, member := range record.Members {
					if member.Kind == "source_publication" && member.SourcePublicationID == publication.ID && member.IncludeDescendants && documentationSelectorsEqual(member.Selector, json.RawMessage(`{}`)) {
						saved = true
						break
					}
				}
				if saved {
					break
				}
			}
		}
		status := documentationAttentionStatus(source, latest, publication != nil, saved)
		if status == "" {
			continue
		}
		item := DocumentationAttentionItem{SourceID: source.ID, Name: source.Name, Kind: source.Kind, Visibility: source.Visibility, Status: status, UpdatedAt: source.UpdatedAt}
		if latest != nil {
			item.CrawlJobID, item.FetchedCount, item.ChangedCount, item.FailedCount, item.SkippedCount = latest.ID, latest.FetchedCount, latest.ChangedCount, latest.FailedCount, latest.SkippedCount
			updated := latest.QueuedAt
			if latest.StartedAt != nil {
				updated = *latest.StartedAt
			}
			if latest.FinishedAt != nil {
				updated = *latest.FinishedAt
			}
			if updated.After(item.UpdatedAt) {
				item.UpdatedAt = updated
			}
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].SourceID < items[j].SourceID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	result := DocumentationAttentionPage{Items: []DocumentationAttentionItem{}, Total: len(items)}
	limit, offset = boundedDeveloperAssetResultLimit(limit), max(offset, 0)
	if offset >= len(items) || limit == 0 {
		return result, nil
	}
	end := min(offset+limit, len(items))
	result.Items, result.HasMore = items[offset:end], end < len(items)
	return result, nil
}

func (p *Postgres) DocumentationAttention(ctx context.Context, deploymentID, sourceID string, limit, offset int) (DocumentationAttentionPage, error) {
	result := DocumentationAttentionPage{Items: []DocumentationAttentionItem{}}
	filter := `WITH summaries AS (
 SELECT source.id,source.name,source.kind,source.visibility,coalesce(job.id::text,'') AS crawl_job_id,
 coalesce(job.fetched_count,0) AS fetched_count,coalesce(job.changed_count,0) AS changed_count,coalesce(job.failed_count,0) AS failed_count,coalesce(job.skipped_count,0) AS skipped_count,
 greatest(source.updated_at,coalesce(job.finished_at,job.started_at,job.queued_at,source.updated_at)) AS updated_at,
 CASE WHEN job.id IS NULL THEN 'not_imported'
 WHEN job.state IN ('queued','running') THEN 'importing'
 WHEN job.state IN ('failed','cancelled') THEN 'import_failed'
 WHEN source.state='quarantined' OR job.failed_count>0 OR job.skipped_count>0 THEN 'blocked'
 WHEN publication.id IS NULL THEN 'review'
 WHEN NOT EXISTS (
 SELECT 1 FROM documentation_collection_members member
 JOIN documentation_collection_revisions revision ON revision.id=member.documentation_collection_revision_id AND revision.deployment_id=member.deployment_id
 JOIN documentation_collections collection ON collection.id=revision.documentation_collection_id AND collection.deployment_id=revision.deployment_id
 WHERE member.deployment_id=$1 AND member.source_publication_id=publication.id AND member.member_kind='source_publication' AND member.include_descendants AND member.selector='{}'::jsonb AND revision.visibility=publication.visibility::text AND collection.lifecycle='active'
 ) THEN 'save_version' ELSE '' END AS status
 FROM sources source
 LEFT JOIN LATERAL (SELECT * FROM crawl_jobs WHERE product_id=$1 AND source_id=source.id ORDER BY queued_at DESC,id DESC LIMIT 1) job ON true
 LEFT JOIN LATERAL (SELECT id,visibility FROM source_publications WHERE product_id=$1 AND source_id=source.id AND crawl_job_id=job.id ORDER BY revision DESC LIMIT 1) publication ON true
 WHERE source.product_id=$1 AND source.kind IN ('website','upload') AND ($2='' OR source.id::text=$2)
 AND NOT EXISTS (SELECT 1 FROM api_contract_sources binding WHERE binding.deployment_id=$1 AND binding.source_id=source.id AND binding.lifecycle='attached')
), selected AS (SELECT * FROM summaries WHERE status<>'') `
	args := []any{deploymentID, sourceID}
	if err := p.pool.QueryRow(ctx, filter+`SELECT count(*) FROM selected`, args...).Scan(&result.Total); err != nil {
		return result, databaseError(err)
	}
	limit, offset = boundedDeveloperAssetResultLimit(limit), max(offset, 0)
	if offset >= result.Total || limit == 0 {
		return result, nil
	}
	rows, err := p.pool.Query(ctx, filter+`SELECT id::text,name,kind,visibility,crawl_job_id,status,fetched_count,changed_count,failed_count,skipped_count,updated_at FROM selected ORDER BY updated_at DESC,id LIMIT $3 OFFSET $4`, append(args, limit, offset)...)
	if err != nil {
		return result, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var item DocumentationAttentionItem
		if err := rows.Scan(&item.SourceID, &item.Name, &item.Kind, &item.Visibility, &item.CrawlJobID, &item.Status, &item.FetchedCount, &item.ChangedCount, &item.FailedCount, &item.SkippedCount, &item.UpdatedAt); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	result.HasMore = offset+len(result.Items) < result.Total
	return result, rows.Err()
}
