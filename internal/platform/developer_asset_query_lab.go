package platform

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type DeveloperAssetQueryLabInput struct {
	Scope                                string   `json:"scope"`
	APIID                                string   `json:"api_id,omitempty"`
	DeploymentDocumentationPublicationID string   `json:"deployment_documentation_publication_id,omitempty"`
	APIDeveloperAssetPublicationID       string   `json:"api_developer_asset_publication_id,omitempty"`
	Query                                string   `json:"query"`
	AssetKinds                           []string `json:"asset_kinds,omitempty"`
	Languages                            []string `json:"languages,omitempty"`
	Ecosystems                           []string `json:"ecosystems,omitempty"`
	SDKReleaseIDs                        []string `json:"sdk_release_ids,omitempty"`
	ExactVersions                        []string `json:"exact_versions,omitempty"`
	Limit                                int      `json:"limit,omitempty"`
	ContextTokenLimit                    int      `json:"context_token_limit,omitempty"`
}

type DeveloperAssetQueryLabResult struct {
	Rank          int                 `json:"rank"`
	Unit          model.KnowledgeUnit `json:"unit"`
	Excerpt       string              `json:"excerpt"`
	LexicalScore  float64             `json:"lexical_score"`
	// SemanticScore retains the public v1 field name, but currently contains
	// cosine similarity from local-feature-hash-v1, not a learned embedding.
	SemanticScore float64 `json:"semantic_score"`
	RerankScore   float64             `json:"rerank_score"`
	Selected      bool                `json:"selected"`
}

type DeveloperAssetQueryLabResponse struct {
	TraceID       string                         `json:"trace_id"`
	ResolvedScope json.RawMessage                `json:"resolved_scope"`
	Results       []DeveloperAssetQueryLabResult `json:"results"`
	ContextTokens int                            `json:"context_tokens"`
	Diagnostics   json.RawMessage                `json:"diagnostics"`
}

func normalizeQueryLabFilters(values []string, maximum int) ([]string, error) {
	return normalizeStringSet(values, maximum)
}

func (s *Service) resolveQueryLabScope(ctx context.Context, deployment model.Deployment, input *DeveloperAssetQueryLabInput) (string, string, json.RawMessage, error) {
	input.Scope = strings.ToLower(strings.TrimSpace(input.Scope))
	input.APIID = strings.TrimSpace(input.APIID)
	if input.Scope == "" {
		if input.APIID == "" {
			input.Scope = "global"
		} else {
			input.Scope = "combined"
		}
	}
	if input.Scope != "global" && input.Scope != "api" && input.Scope != "combined" {
		return "", "", nil, errors.New("query scope must be global, api, or combined")
	}
	globalID := strings.TrimSpace(input.DeploymentDocumentationPublicationID)
	apiPublicationID := strings.TrimSpace(input.APIDeveloperAssetPublicationID)
	if input.Scope == "global" || input.Scope == "combined" {
		if globalID != "" {
			publication, err := s.store.DeploymentDocumentationPublication(ctx, deployment.ID, globalID)
			if err != nil {
				return "", "", nil, err
			}
			ready, readyErr := s.developerAssetPublicationReady(ctx, deployment.ID, "global_documentation", publication.ID)
			if readyErr != nil {
				return "", "", nil, readyErr
			}
			if !ready {
				return "", "", nil, errors.New("selected global documentation publication is not ready for retrieval")
			}
			globalID = publication.ID
		} else {
			publication, err := s.readyDeploymentDocumentationPublication(ctx, deployment.ID)
			if err == nil {
				globalID = publication.ID
			} else if input.Scope == "global" || !errors.Is(err, store.ErrNotFound) {
				return "", "", nil, err
			}
		}
	}
	if input.Scope == "api" || input.Scope == "combined" {
		if input.APIID == "" {
			return "", "", nil, errors.New("api_id is required for api or combined query scope")
		}
		if _, err := s.store.Integration(ctx, deployment.ID, input.APIID); err != nil {
			return "", "", nil, err
		}
		if apiPublicationID != "" {
			publication, err := s.store.APIDeveloperAssetPublication(ctx, deployment.ID, apiPublicationID)
			if err != nil || publication.APIID != input.APIID {
				return "", "", nil, errors.New("API developer-asset publication does not belong to the selected API")
			}
			ready, readyErr := s.developerAssetPublicationReady(ctx, deployment.ID, "api", publication.ID)
			if readyErr != nil {
				return "", "", nil, readyErr
			}
			if !ready {
				return "", "", nil, errors.New("selected API developer-asset publication is not ready for retrieval")
			}
			apiPublicationID = publication.ID
		} else {
			publication, err := s.readyAPIDeveloperAssetPublication(ctx, deployment.ID, input.APIID)
			if errors.Is(err, store.ErrNotFound) {
				return "", "", nil, errors.New("selected API has no immutable developer-asset publication")
			}
			if err != nil {
				return "", "", nil, err
			}
			apiPublicationID = publication.ID
		}
	}
	if globalID == "" && apiPublicationID == "" {
		return "", "", nil, errors.New("query scope resolves to no published developer assets")
	}
	resolved, _ := json.Marshal(map[string]any{
		"scope": input.Scope, "api_id": input.APIID,
		"deployment_documentation_publication_id": globalID,
		"api_developer_asset_publication_id":      apiPublicationID,
	})
	return globalID, apiPublicationID, resolved, nil
}

func queryLabRerankScore(query string, result store.DeveloperAssetKnowledgeResult) float64 {
	score := result.FusedScore
	if result.Unit.Kind == "map" {
		score += 0.08
	}
	if strings.EqualFold(strings.TrimSpace(result.Unit.Title), strings.TrimSpace(query)) {
		score += 0.15
	}
	for _, identifier := range result.Unit.Identifiers {
		if strings.EqualFold(strings.TrimSpace(identifier), strings.TrimSpace(query)) {
			score += 0.2
			break
		}
	}
	return score
}

func queryLabExcerpt(value string, maximumRunes int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= maximumRunes {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maximumRunes])) + "…"
}

func estimatedDeveloperAssetTokens(value string) int {
	count := utf8.RuneCountInString(value)
	if count == 0 {
		return 0
	}
	return (count + 3) / 4
}

func (s *Service) RunDeveloperAssetQueryLab(ctx context.Context, input DeveloperAssetQueryLabInput) (DeveloperAssetQueryLabResponse, error) {
	started := time.Now()
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return DeveloperAssetQueryLabResponse{}, err
	}
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" || len(input.Query) > 500 {
		return DeveloperAssetQueryLabResponse{}, errors.New("query must be between 1 and 500 characters")
	}
	if containsAISecretText(input.Query) {
		return DeveloperAssetQueryLabResponse{}, errors.New("query contains secret-like material and was not retained")
	}
	if input.Limit == 0 {
		input.Limit = 10
	}
	if input.Limit < 1 || input.Limit > 50 {
		return DeveloperAssetQueryLabResponse{}, errors.New("result limit must be between 1 and 50")
	}
	if input.ContextTokenLimit == 0 {
		input.ContextTokenLimit = 4000
	}
	if input.ContextTokenLimit < 256 || input.ContextTokenLimit > 32000 {
		return DeveloperAssetQueryLabResponse{}, errors.New("context_token_limit must be between 256 and 32000")
	}
	for target, maximum := range map[*[]string]int{
		&input.AssetKinds: 3, &input.Languages: 50, &input.Ecosystems: 20,
		&input.SDKReleaseIDs: 100, &input.ExactVersions: 100,
	} {
		values, normalizeErr := normalizeQueryLabFilters(*target, maximum)
		if normalizeErr != nil {
			return DeveloperAssetQueryLabResponse{}, normalizeErr
		}
		*target = values
	}
	for _, kind := range input.AssetKinds {
		if kind != string(model.DeveloperAssetDocumentation) && kind != string(model.DeveloperAssetContract) && kind != string(model.DeveloperAssetSDK) {
			return DeveloperAssetQueryLabResponse{}, errors.New("asset_kinds may contain only documentation, contract, or sdk")
		}
	}
	globalID, apiPublicationID, resolvedScope, err := s.resolveQueryLabScope(ctx, deployment, &input)
	if err != nil {
		return DeveloperAssetQueryLabResponse{}, err
	}
	candidateLimit := input.Limit * 4
	if candidateLimit < 40 {
		candidateLimit = 40
	}
	if candidateLimit > 200 {
		candidateLimit = 200
	}
	queryEmbedding := localDeveloperAssetEmbedding(input.Query)
	candidates, err := s.store.RetrieveDeveloperAssetKnowledge(ctx, store.DeveloperAssetKnowledgeQuery{
		DeploymentID: deployment.ID, DeploymentDocumentationPublicationID: globalID,
		APIDeveloperAssetPublicationID: apiPublicationID, APIID: input.APIID,
		BuilderVersion: DeveloperAssetIndexBuilderVersion, RetrievalProfileVersion: DeveloperAssetRetrievalProfileVersion,
		AssetKinds: input.AssetKinds, Languages: input.Languages, Ecosystems: input.Ecosystems,
		SDKReleaseIDs: input.SDKReleaseIDs, ExactVersions: input.ExactVersions,
		QueryText: input.Query, QueryEmbedding: queryEmbedding, Limit: candidateLimit,
	})
	if err != nil {
		return DeveloperAssetQueryLabResponse{}, err
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := queryLabRerankScore(input.Query, candidates[i]), queryLabRerankScore(input.Query, candidates[j])
		if left == right {
			return candidates[i].Unit.ID < candidates[j].Unit.ID
		}
		return left > right
	})
	results := make([]DeveloperAssetQueryLabResult, 0, input.Limit)
	traceResults := make([]model.RetrievalQueryTraceResult, 0, len(candidates))
	contextTokens := 0
	for index, candidate := range candidates {
		excerpt := queryLabExcerpt(candidate.Unit.Content, 1200)
		tokens := estimatedDeveloperAssetTokens(candidate.Unit.Title + "\n" + excerpt)
		selected := len(results) < input.Limit && contextTokens+tokens <= input.ContextTokenLimit
		rerank := queryLabRerankScore(input.Query, candidate)
		if selected {
			contextTokens += tokens
			results = append(results, DeveloperAssetQueryLabResult{
				Rank: len(results) + 1, Unit: candidate.Unit, Excerpt: excerpt,
				LexicalScore: candidate.LexicalScore, SemanticScore: candidate.SemanticScore,
				RerankScore: rerank, Selected: true,
			})
		}
		lexical, semantic, rerankCopy := candidate.LexicalScore, candidate.SemanticScore, rerank
		exclusionReason := ""
		if !selected {
			exclusionReason = "rank_or_context_budget"
		}
		traceResults = append(traceResults, model.RetrievalQueryTraceResult{
			Rank: index + 1, KnowledgeUnitID: candidate.Unit.ID,
			SourcePublicationKind: candidate.Unit.SourcePublicationKind, SourcePublicationID: candidate.Unit.SourcePublicationID,
			SourceEntityID: candidate.Unit.SourceEntityID, LexicalScore: &lexical, SemanticScore: &semantic,
			RerankScore: &rerankCopy, Selected: selected, ExclusionReason: exclusionReason,
			Citation: candidate.Unit.Citation, Excerpt: excerpt, ContentHash: candidate.Unit.ContentHash,
		})
	}
	traceID, err := randomUUID()
	if err != nil {
		return DeveloperAssetQueryLabResponse{}, err
	}
	requestedFilters, _ := json.Marshal(map[string]any{
		"asset_kinds": input.AssetKinds, "languages": input.Languages, "ecosystems": input.Ecosystems,
		"sdk_release_ids": input.SDKReleaseIDs, "exact_versions": input.ExactVersions,
		"limit": input.Limit, "context_token_limit": input.ContextTokenLimit,
	})
	routing, _ := json.Marshal(map[string]any{
		"lexical": "postgres_fts", "feature_hash": developerAssetEmbeddingModel,
		"fusion": "weighted_lexical_feature_hash_cosine", "map_boost": 0.08, "exact_identifier_boost": 0.2,
	})
	diagnostics, _ := json.Marshal(map[string]any{"candidate_pool_limit": candidateLimit, "truncated_by_context": len(results) < min(input.Limit, len(candidates))})
	now := s.now()
	expiresAt := now.Add(30 * 24 * time.Hour)
	trace := model.RetrievalQueryTrace{
		ID: traceID, DeploymentID: deployment.ID, DeploymentDocumentationPublicationID: globalID,
		APIDeveloperAssetPublicationID: apiPublicationID, RetrievalProfileVersion: DeveloperAssetRetrievalProfileVersion,
		QueryText: input.Query, QueryHash: contentHash([]byte(strings.ToLower(input.Query))),
		RequestedFilters: requestedFilters, ResolvedScope: resolvedScope, RoutingDecision: routing,
		State: "succeeded", CandidateCount: len(candidates), ResultCount: len(results), ContextTokens: contextTokens,
		LatencyMS: int(time.Since(started).Milliseconds()), Diagnostics: diagnostics, ExpiresAt: &expiresAt, CreatedAt: now,
	}
	for index := range traceResults {
		traceResults[index].RetrievalQueryTraceID = trace.ID
	}
	if err := s.store.AppendRetrievalQueryTrace(ctx, store.RetrievalQueryTraceRecord{Trace: trace, Results: traceResults}); err != nil {
		return DeveloperAssetQueryLabResponse{}, err
	}
	return DeveloperAssetQueryLabResponse{TraceID: trace.ID, ResolvedScope: resolvedScope, Results: results, ContextTokens: contextTokens, Diagnostics: diagnostics}, nil
}
