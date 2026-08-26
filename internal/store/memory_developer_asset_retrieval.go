package store

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func cloneKnowledgeUnit(value model.KnowledgeUnit) model.KnowledgeUnit {
	embedding := append([]float32(nil), value.Embedding...)
	value.Embedding = nil
	cloned := memoryClone(value)
	cloned.Embedding = embedding
	return cloned
}

func cloneSearchIndexGenerationRecord(value SearchIndexGenerationRecord) SearchIndexGenerationRecord {
	cloned := SearchIndexGenerationRecord{
		Generation: memoryClone(value.Generation),
		APIScopes:  memoryClone(value.APIScopes),
		Units:      make([]model.KnowledgeUnit, len(value.Units)),
	}
	for index, unit := range value.Units {
		cloned.Units[index] = cloneKnowledgeUnit(unit)
	}
	return cloned
}

func (m *Memory) SearchIndexGenerations(_ context.Context, deploymentID, publicationKind, publicationID string) ([]model.SearchIndexGeneration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.SearchIndexGeneration, 0)
	for _, record := range m.developerAssets.indexGenerations {
		value := record.Generation
		if value.DeploymentID == deploymentID && (publicationKind == "" || value.PublicationKind == publicationKind) && (publicationID == "" || value.PublicationID == publicationID) {
			result = append(result, memoryClone(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) SearchIndexGeneration(_ context.Context, deploymentID, id string) (SearchIndexGenerationRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.indexGenerations[id]
	if !ok || value.Generation.DeploymentID != deploymentID {
		return SearchIndexGenerationRecord{}, ErrNotFound
	}
	return cloneSearchIndexGenerationRecord(value), nil
}

func (m *Memory) CreateSearchIndexGeneration(_ context.Context, value model.SearchIndexGeneration) (model.SearchIndexGeneration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID {
		return model.SearchIndexGeneration{}, ErrNotFound
	}
	if _, exists := m.developerAssets.indexGenerations[value.ID]; exists {
		return model.SearchIndexGeneration{}, ErrConflict
	}
	for _, current := range m.developerAssets.indexGenerations {
		generation := current.Generation
		if generation.PublicationKind == value.PublicationKind && generation.PublicationID == value.PublicationID && generation.BuilderVersion == value.BuilderVersion && generation.RetrievalProfileVersion == value.RetrievalProfileVersion {
			return model.SearchIndexGeneration{}, ErrConflict
		}
	}
	if value.State == "" {
		value.State = "queued"
	}
	value.CreatedAt = time.Now().UTC()
	m.developerAssets.indexGenerations[value.ID] = SearchIndexGenerationRecord{Generation: memoryClone(value)}
	return value, nil
}

func (m *Memory) CompleteSearchIndexGeneration(_ context.Context, value SearchIndexGenerationRecord, expectedState string) (model.SearchIndexGeneration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.developerAssets.indexGenerations[value.Generation.ID]
	if !ok || current.Generation.DeploymentID != value.Generation.DeploymentID {
		return model.SearchIndexGeneration{}, ErrNotFound
	}
	if current.Generation.State != expectedState {
		return model.SearchIndexGeneration{}, ErrConflict
	}
	if value.Generation.State != "ready" || value.Generation.ReadyAt == nil || value.Generation.UnitCount != len(value.Units) {
		return model.SearchIndexGeneration{}, ErrConflict
	}
	unitIDs := make(map[string]bool, len(value.Units))
	ordinals := make(map[int]bool, len(value.Units))
	for _, unit := range value.Units {
		if (value.Generation.EmbeddingDimensions == nil && len(unit.Embedding) != 0) ||
			(value.Generation.EmbeddingDimensions != nil && len(unit.Embedding) != *value.Generation.EmbeddingDimensions) {
			return model.SearchIndexGeneration{}, ErrConflict
		}
		if unit.SearchIndexGenerationID != value.Generation.ID || unit.DeploymentID != value.Generation.DeploymentID || unitIDs[unit.ID] || ordinals[unit.Ordinal] {
			return model.SearchIndexGeneration{}, ErrConflict
		}
		unitIDs[unit.ID], ordinals[unit.Ordinal] = true, true
	}
	scopeKeys := make(map[string]bool, len(value.APIScopes))
	for _, scope := range value.APIScopes {
		key := scope.KnowledgeUnitID + "\x00" + scope.APIID
		if !unitIDs[scope.KnowledgeUnitID] || scope.DeploymentID != value.Generation.DeploymentID || scopeKeys[key] {
			return model.SearchIndexGeneration{}, ErrConflict
		}
		scopeKeys[key] = true
	}
	value.Generation.CreatedAt = current.Generation.CreatedAt
	m.developerAssets.indexGenerations[value.Generation.ID] = cloneSearchIndexGenerationRecord(value)
	return value.Generation, nil
}

func (m *Memory) FailSearchIndexGeneration(_ context.Context, value model.SearchIndexGeneration, expectedState string) (model.SearchIndexGeneration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.developerAssets.indexGenerations[value.ID]
	if !ok || current.Generation.DeploymentID != value.DeploymentID {
		return model.SearchIndexGeneration{}, ErrNotFound
	}
	if current.Generation.State != expectedState || value.State != "failed" {
		return model.SearchIndexGeneration{}, ErrConflict
	}
	value.CreatedAt = current.Generation.CreatedAt
	m.developerAssets.indexGenerations[value.ID] = SearchIndexGenerationRecord{Generation: memoryClone(value)}
	return value, nil
}

func stringFilterContains(values []string, candidate string) bool {
	if len(values) == 0 {
		return true
	}
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func cosineSimilarity(left, right []float32) (float64, bool) {
	if len(left) == 0 || len(left) != len(right) {
		return 0, false
	}
	var dot, leftNorm, rightNorm float64
	for index := range left {
		l, r := float64(left[index]), float64(right[index])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, false
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm)), true
}

func (m *Memory) RetrieveDeveloperAssetKnowledge(_ context.Context, query DeveloperAssetKnowledgeQuery) ([]DeveloperAssetKnowledgeResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
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
	needle := strings.ToLower(strings.TrimSpace(query.QueryText))
	result := make([]DeveloperAssetKnowledgeResult, 0)
	for _, record := range m.developerAssets.indexGenerations {
		generation := record.Generation
		if generation.DeploymentID != query.DeploymentID || generation.State != "ready" ||
			generation.BuilderVersion != builderVersion || generation.RetrievalProfileVersion != retrievalProfileVersion {
			continue
		}
		global := query.DeploymentDocumentationPublicationID != "" && generation.PublicationKind == "global_documentation" && generation.PublicationID == query.DeploymentDocumentationPublicationID
		api := query.APIDeveloperAssetPublicationID != "" && generation.PublicationKind == "api" && generation.PublicationID == query.APIDeveloperAssetPublicationID
		if !global && !api {
			continue
		}
		if len(query.QueryEmbedding) > 0 && (generation.EmbeddingDimensions == nil || *generation.EmbeddingDimensions != len(query.QueryEmbedding)) {
			return nil, ErrConflict
		}
		scopedUnits := make(map[string]bool)
		if api {
			for _, scope := range record.APIScopes {
				if scope.APIID == query.APIID {
					scopedUnits[scope.KnowledgeUnitID] = true
				}
			}
		}
		for _, unit := range record.Units {
			if api && !scopedUnits[unit.ID] {
				continue
			}
			if !stringFilterContains(query.Languages, unit.Language) || !stringFilterContains(query.Ecosystems, unit.Ecosystem) {
				continue
			}
			var metadata map[string]any
			_ = json.Unmarshal(unit.Metadata, &metadata)
			assetKind, _ := metadata["asset_kind"].(string)
			if assetKind == "" {
				assetKind = generation.AssetKind
			}
			sdkReleaseID, _ := metadata["sdk_release_id"].(string)
			exactVersion, _ := metadata["exact_version"].(string)
			if !stringFilterContains(query.AssetKinds, assetKind) || !stringFilterContains(query.SDKReleaseIDs, sdkReleaseID) || !stringFilterContains(query.ExactVersions, exactVersion) {
				continue
			}
			lexicalScore := 0.0
			identifierMatch := false
			for _, identifier := range unit.Identifiers {
				if needle != "" && strings.EqualFold(identifier, needle) {
					lexicalScore, identifierMatch = lexicalScore+1, true
				}
			}
			haystack := strings.ToLower(unit.Title + " " + unit.Content)
			if needle != "" {
				for _, token := range strings.Fields(needle) {
					lexicalScore += float64(strings.Count(haystack, token))
				}
				if len(query.QueryEmbedding) == 0 && lexicalScore == 0 && !identifierMatch {
					continue
				}
			}
			semanticScore := 0.0
			fusedScore := lexicalScore
			if len(query.QueryEmbedding) > 0 {
				semanticScore = -1
				if similarity, ok := cosineSimilarity(unit.Embedding, query.QueryEmbedding); ok {
					semanticScore = similarity
				}
				lexicalNormalized := lexicalScore / (1 + lexicalScore)
				semanticNormalized := math.Max(0, math.Min(1, (semanticScore+1)/2))
				// Fixed normalized fusion keeps memory and PostgreSQL retrieval
				// deterministic: lexical 45%, cosine similarity 55%.
				fusedScore = 0.45*lexicalNormalized + 0.55*semanticNormalized
			}
			result = append(result, DeveloperAssetKnowledgeResult{
				Unit: cloneKnowledgeUnit(unit), LexicalScore: lexicalScore,
				SemanticScore: semanticScore, FusedScore: fusedScore,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FusedScore == result[j].FusedScore {
			return result[i].Unit.Ordinal < result[j].Unit.Ordinal
		}
		return result[i].FusedScore > result[j].FusedScore
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func boundedDeveloperAssetResultLimit(limit int) int {
	if limit < 1 {
		return 0
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func (m *Memory) RetrievalQueryTraces(_ context.Context, deploymentID string, since time.Time, limit int) ([]model.RetrievalQueryTrace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	limit = boundedDeveloperAssetResultLimit(limit)
	result := make([]model.RetrievalQueryTrace, 0)
	for _, record := range m.developerAssets.queryTraces {
		if record.Trace.DeploymentID == deploymentID && !record.Trace.CreatedAt.Before(since) {
			result = append(result, memoryClone(record.Trace))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (m *Memory) RetrievalQueryTrace(_ context.Context, deploymentID, id string) (RetrievalQueryTraceRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.queryTraces[id]
	if !ok || value.Trace.DeploymentID != deploymentID {
		return RetrievalQueryTraceRecord{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) AppendRetrievalQueryTrace(_ context.Context, value RetrievalQueryTraceRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.Trace.DeploymentID != m.deployment.ID {
		return ErrNotFound
	}
	if _, exists := m.developerAssets.queryTraces[value.Trace.ID]; exists {
		return ErrConflict
	}
	if value.Trace.DeploymentDocumentationPublicationID != "" {
		publication, ok := m.developerAssets.documentationPublications[value.Trace.DeploymentDocumentationPublicationID]
		if !ok || publication.DeploymentID != value.Trace.DeploymentID {
			return ErrNotFound
		}
	}
	if value.Trace.APIDeveloperAssetPublicationID != "" {
		publication, ok := m.developerAssets.apiPublications[value.Trace.APIDeveloperAssetPublicationID]
		if !ok || publication.DeploymentID != value.Trace.DeploymentID {
			return ErrNotFound
		}
	}
	if value.Trace.DeploymentDocumentationPublicationID == "" && value.Trace.APIDeveloperAssetPublicationID == "" {
		return ErrConflict
	}
	ranks := make(map[int]bool, len(value.Results))
	for _, result := range value.Results {
		if result.RetrievalQueryTraceID != value.Trace.ID || result.Rank < 1 || ranks[result.Rank] {
			return ErrConflict
		}
		ranks[result.Rank] = true
	}
	if value.Trace.CreatedAt.IsZero() {
		value.Trace.CreatedAt = time.Now().UTC()
	}
	m.developerAssets.queryTraces[value.Trace.ID] = memoryClone(value)
	return nil
}

func (m *Memory) DeleteExpiredRetrievalQueryTraces(_ context.Context, before time.Time, limit int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	limit = boundedDeveloperAssetResultLimit(limit)
	if limit == 0 {
		return 0, nil
	}
	type candidate struct {
		id string
		at time.Time
	}
	candidates := make([]candidate, 0)
	for id, value := range m.developerAssets.queryTraces {
		if value.Trace.ExpiresAt != nil && !value.Trace.ExpiresAt.After(before) {
			candidates = append(candidates, candidate{id: id, at: *value.Trace.ExpiresAt})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].at.Before(candidates[j].at) })
	if limit < len(candidates) {
		candidates = candidates[:limit]
	}
	for _, value := range candidates {
		delete(m.developerAssets.queryTraces, value.id)
	}
	return int64(len(candidates)), nil
}
