package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) sourceInputReplacementLocked(productID, sourceID, requestDigest string) (SourceInputReplacementResult, error) {
	source, ok := m.sources[productID][sourceID]
	if !ok || source.ProductID != productID {
		return SourceInputReplacementResult{}, ErrNotFound
	}
	record, ok := m.sourceInputReplacements[productID+":"+sourceID+":"+requestDigest]
	if !ok {
		return SourceInputReplacementResult{}, ErrNotFound
	}
	for _, job := range m.crawls[sourceID] {
		if job.ID == record.CrawlJobID && job.ProductID == productID && job.SourceID == sourceID {
			return memoryClone(SourceInputReplacementResult{Source: source, CrawlJob: job}), nil
		}
	}
	return SourceInputReplacementResult{}, ErrNotFound
}

func (m *Memory) SourceInputReplacement(_ context.Context, productID, sourceID, requestDigest string) (SourceInputReplacementResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sourceInputReplacementLocked(productID, sourceID, requestDigest)
}

func (m *Memory) ReplaceSourceInput(_ context.Context, input SourceInputReplacement) (SourceInputReplacementResult, error) {
	if err := validateSourceInputReplacement(input); err != nil {
		return SourceInputReplacementResult{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	source, ok := m.sources[input.ProductID][input.SourceID]
	if !ok || source.ProductID != input.ProductID || source.OrganisationID != input.Audit.OrganisationID {
		return SourceInputReplacementResult{}, ErrNotFound
	}
	key := input.ProductID + ":" + input.SourceID + ":" + input.RequestDigest
	if record, ok := m.sourceInputReplacements[key]; ok {
		if record.InputDigest != input.InputDigest || record.ExpectedRevision != input.ExpectedRevision {
			return SourceInputReplacementResult{}, ErrSourceInputChanged
		}
		return m.sourceInputReplacementLocked(input.ProductID, input.SourceID, input.RequestDigest)
	}
	if source.Kind != "upload" || source.Revision != input.ExpectedRevision {
		return SourceInputReplacementResult{}, ErrSourceInputChanged
	}
	for _, job := range m.crawls[source.ID] {
		if job.State == "queued" || job.State == "running" {
			return SourceInputReplacementResult{}, ErrSourceImportActive
		}
		if job.ID == input.CrawlJobID {
			return SourceInputReplacementResult{}, ErrConflict
		}
	}
	for _, event := range m.audit {
		if event.ID == input.Audit.ID {
			return SourceInputReplacementResult{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	source.Location = input.Location
	source.Revision++
	source.UpdatedAt = now
	job := model.CrawlJob{ID: input.CrawlJobID, OrganisationID: source.OrganisationID, ProductID: source.ProductID, SourceID: source.ID, State: "queued", Attempt: 1, PipelineVersion: "developer-assets-v2", Diagnostics: json.RawMessage(`{}`), QueuedAt: now}
	if m.sourceInputReplacements == nil {
		m.sourceInputReplacements = map[string]sourceInputReplacementRecord{}
	}
	m.sourceInputReplacements[key] = sourceInputReplacementRecord{InputDigest: input.InputDigest, ExpectedRevision: input.ExpectedRevision, CrawlJobID: job.ID}
	m.sources[source.ProductID][source.ID] = source
	m.crawls[source.ID] = append(m.crawls[source.ID], job)
	event := input.Audit
	event.Prior = map[string]any{"source_revision": input.ExpectedRevision}
	event.Current = map[string]any{"source_revision": source.Revision, "crawl_job_id": job.ID, "visibility": source.Visibility}
	event.Outcome = "success"
	m.audit = append(m.audit, memoryClone(event))
	return memoryClone(SourceInputReplacementResult{Source: source, CrawlJob: job}), nil
}
