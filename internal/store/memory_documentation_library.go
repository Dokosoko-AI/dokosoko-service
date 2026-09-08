package store

import (
	"context"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) DocumentationLibrary(_ context.Context, query DocumentationLibraryQuery) (DocumentationLibraryPage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if query.Reviewed {
		return m.reviewedDocumentationLibraryLocked(query), nil
	}
	type candidate struct {
		item         DocumentationLibraryItem
		key, content string
		included     bool
	}
	candidates := []candidate{}
	for runID, output := range m.developerAssets.documentationOutputs {
		run := m.developerAssets.ingestionRuns[runID]
		if run.DeploymentID != query.DeploymentID || (query.SourceID != "" && run.SourceID != query.SourceID) {
			continue
		}
		for _, document := range output.Documents {
			if document.DeploymentID != query.DeploymentID {
				continue
			}
			decision := "unreviewed"
			includedInPublication := query.SourcePublicationID == ""
			var latestRevision int64
			for publicationID, review := range m.developerAssets.sourcePublicationReviews {
				publication, ok := m.sourcePublications[query.DeploymentID][publicationID]
				if !ok || publication.ProductID != query.DeploymentID || publication.SourceID != run.SourceID || publication.CrawlJobID != run.ID || (query.SourcePublicationID != "" && publicationID != query.SourcePublicationID) || publication.Revision <= latestRevision {
					continue
				}
				for _, selection := range review.Selections {
					if selection.DocumentationDocumentID == document.ID && selection.ContentHash == document.ContentHash {
						decision, latestRevision = selection.Decision, publication.Revision
						if publicationID == query.SourcePublicationID && selection.Decision == "included" {
							includedInPublication = true
						}
					}
				}
			}
			identity := run.SourceID
			if identity == "" {
				identity = run.TargetKey
			}
			candidates = append(candidates, candidate{item: DocumentationLibraryItem{ID: document.ID, SourceID: run.SourceID, IngestionRunID: runID, SourcePath: document.SourcePath, Title: document.Title, ContentHash: document.ContentHash, Visibility: document.Visibility, Decision: decision, QueuedAt: run.QueuedAt}, included: includedInPublication, key: identity + "\x00" + document.SourcePath, content: document.NormalizedMarkdown})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i].item, candidates[j].item
		if !a.QueuedAt.Equal(b.QueuedAt) {
			return a.QueuedAt.After(b.QueuedAt)
		}
		if a.IngestionRunID != b.IngestionRunID {
			return a.IngestionRunID > b.IngestionRunID
		}
		return a.ID < b.ID
	})
	previous := map[string]string{}
	for i := len(candidates) - 1; i >= 0; i-- {
		candidates[i].item.PreviousDocumentID = previous[candidates[i].key]
		previous[candidates[i].key] = candidates[i].item.ID
	}
	seen := map[string]bool{}
	items := []DocumentationLibraryItem{}
	needle := strings.ToLower(strings.TrimSpace(query.Query))
	for _, candidate := range candidates {
		if !query.History && seen[candidate.key] {
			continue
		}
		seen[candidate.key] = true
		if !candidate.included {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(candidate.item.Title+" "+candidate.item.SourcePath+" "+candidate.content), needle) {
			continue
		}
		items = append(items, candidate.item)
	}
	total, offset, limit := len(items), max(query.Offset, 0), boundedDeveloperAssetResultLimit(query.Limit)
	if offset >= total || limit == 0 {
		return DocumentationLibraryPage{Items: []DocumentationLibraryItem{}, Total: total}, nil
	}
	end := min(offset+limit, total)
	return DocumentationLibraryPage{Items: items[offset:end], Total: total, HasMore: end < total}, nil
}

func (m *Memory) reviewedDocumentationLibraryLocked(query DocumentationLibraryQuery) DocumentationLibraryPage {
	type reviewed struct {
		item         DocumentationLibraryItem
		revision     int64
		text         string
		singleUpload bool
	}
	latest := map[string]int64{}
	publications := []model.SourcePublication{}
	for _, publication := range m.sourcePublications[query.DeploymentID] {
		if publication.ProductID != query.DeploymentID || (query.SourceID != "" && publication.SourceID != query.SourceID) {
			continue
		}
		publications = append(publications, publication)
		latest[publication.SourceID] = max(latest[publication.SourceID], publication.Revision)
	}
	sort.Slice(publications, func(i, j int) bool { return publications[i].Revision < publications[j].Revision })
	values := []reviewed{}
	bySource := map[string][]reviewed{}
	for _, publication := range publications {
		run := m.developerAssets.ingestionRuns[publication.CrawlJobID]
		if run.DeploymentID != query.DeploymentID || run.SourceID != publication.SourceID {
			continue
		}
		review := m.developerAssets.sourcePublicationReviews[publication.ID]
		output := m.developerAssets.documentationOutputs[publication.CrawlJobID]
		singleUpload := m.sources[query.DeploymentID][publication.SourceID].Kind == "upload" && len(output.Documents) == 1
		for _, selection := range review.Selections {
			if selection.Decision != "included" || selection.DeploymentID != query.DeploymentID {
				continue
			}
			for _, document := range output.Documents {
				if document.ID != selection.DocumentationDocumentID || document.DeploymentID != query.DeploymentID || document.IngestionRunID != publication.CrawlJobID || document.ContentHash != selection.ContentHash {
					continue
				}
				item := DocumentationLibraryItem{ID: document.ID, SourceID: publication.SourceID, IngestionRunID: publication.CrawlJobID, SourcePath: document.SourcePath, Title: document.Title, ContentHash: document.ContentHash, Visibility: publication.Visibility, Decision: "included", QueuedAt: run.QueuedAt}
				value := reviewed{item: item, revision: publication.Revision, text: document.NormalizedMarkdown, singleUpload: singleUpload}
				values = append(values, value)
				bySource[publication.SourceID] = append(bySource[publication.SourceID], value)
			}
		}
	}
	items := []DocumentationLibraryItem{}
	needle := strings.ToLower(strings.TrimSpace(query.Query))
	for _, value := range values {
		if value.revision != latest[value.item.SourceID] || needle != "" && !strings.Contains(strings.ToLower(value.item.Title+" "+value.item.SourcePath+" "+value.text), needle) {
			continue
		}
		var previousRevision int64
		for _, previous := range bySource[value.item.SourceID] {
			older := previous.item.QueuedAt.Before(value.item.QueuedAt) || previous.item.QueuedAt.Equal(value.item.QueuedAt) && previous.item.IngestionRunID < value.item.IngestionRunID
			matches := previous.item.SourcePath == value.item.SourcePath || previous.singleUpload && value.singleUpload
			if previous.item.SourceID != value.item.SourceID || previous.revision >= value.revision || !older || !matches {
				continue
			}
			if previous.revision > previousRevision || previous.revision == previousRevision && previous.item.ID < value.item.PreviousDocumentID {
				value.item.PreviousDocumentID, previousRevision = previous.item.ID, previous.revision
			}
		}
		items = append(items, value.item)
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].QueuedAt.Equal(items[j].QueuedAt) {
			return items[i].QueuedAt.After(items[j].QueuedAt)
		}
		if items[i].SourceID != items[j].SourceID {
			return items[i].SourceID < items[j].SourceID
		}
		return items[i].ID < items[j].ID
	})
	result := DocumentationLibraryPage{Items: []DocumentationLibraryItem{}, Total: len(items)}
	limit, offset := boundedDeveloperAssetResultLimit(query.Limit), max(query.Offset, 0)
	if offset >= len(items) || limit == 0 {
		return result
	}
	end := min(offset+limit, len(items))
	result.Items, result.HasMore = items[offset:end], end < len(items)
	return result
}
