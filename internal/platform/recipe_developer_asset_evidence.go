package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const (
	recipeDeveloperAssetGlobalPublicationKind = "developer_asset_global_publication"
	recipeDeveloperAssetAPIPublicationKind    = "developer_asset_api_publication"
	recipeDeveloperAssetDocumentationKind     = "developer_asset_documentation"
	recipeDeveloperAssetContractKind          = "developer_asset_contract"
	recipeDeveloperAssetSDKKind               = "developer_asset_sdk"

	maxRecipeDeveloperAssetEvidence       = 12
	maxRecipeDeveloperAssetGlobalEvidence = 4
	maxRecipeDeveloperAssetEvidenceRunes  = 8_000
	maxRecipeDeveloperAssetExcerptRunes   = 1_200
)

type recipeDeveloperAssetScope struct {
	global *model.DeploymentDocumentationPublication
	api    *model.APIDeveloperAssetPublication
}

type recipeDeveloperAssetUnitContext struct {
	scopeKind             string
	apiID                 string
	indexPublicationID    string
	indexPublicationHash  string
	sourcePublicationHash string
	assetContentHash      string
}

func recipeDeveloperAssetSupportingKind(kind string) bool {
	switch kind {
	case recipeDeveloperAssetDocumentationKind, recipeDeveloperAssetContractKind, recipeDeveloperAssetSDKKind:
		return true
	default:
		return false
	}
}

func recipeDeveloperAssetEvidenceKind(assetKind string) string {
	switch assetKind {
	case "documentation":
		return recipeDeveloperAssetDocumentationKind
	case "contract":
		return recipeDeveloperAssetContractKind
	case "sdk":
		return recipeDeveloperAssetSDKKind
	default:
		return ""
	}
}

func developerAssetJSONStrings(raw json.RawMessage) map[string]string {
	var object map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&object) != nil {
		return nil
	}
	result := make(map[string]string, len(object))
	for key, value := range object {
		switch typed := value.(type) {
		case string:
			result[key] = strings.TrimSpace(typed)
		case json.Number:
			if _, err := strconv.ParseInt(typed.String(), 10, 64); err == nil {
				result[key] = typed.String()
			}
		}
	}
	return result
}

func recipeDeveloperAssetSourceRevision(unit model.KnowledgeUnit, metadata map[string]string) string {
	switch metadata["asset_kind"] {
	case "documentation":
		return firstNonEmpty(metadata["collection_revision"], unit.SourcePublicationID)
	case "contract":
		return firstNonEmpty(metadata["contract_revision"], unit.SourcePublicationID)
	case "sdk":
		revision := firstNonEmpty(metadata["sdk_content_publication_revision"], unit.SourcePublicationID)
		if exactVersion := metadata["exact_version"]; exactVersion != "" {
			revision += "@" + exactVersion
		}
		return revision
	default:
		return unit.SourcePublicationID
	}
}

func recipeDeveloperAssetReference(resourceID string, citation map[string]string) (string, []model.RecipeReference) {
	location := firstNonEmpty(citation["canonical_url"], citation["source_uri"])
	parsed, err := url.Parse(location)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return "", nil
	}
	label := firstNonEmpty(citation["source_path"], location)
	return location, []model.RecipeReference{{
		Label: label, URL: location, Kind: recipeReferenceKind(label, location),
		ResourceID: resourceID, Anchor: citation["anchor"],
	}}
}

// recipeDeveloperAssetEvidenceFromUnit converts one already publication- and
// API-scoped retrieval unit into a stable recipe fact. The current aggregate
// publication is retained in the evidence excerpt for auditability, while the
// fingerprint binds only the selected nested publication/revision/content.
// Consequently, adding an unrelated asset to a later aggregate publication
// does not invalidate a recipe which still selects the same exact fact.
func recipeDeveloperAssetEvidenceFromUnit(unit model.KnowledgeUnit, context recipeDeveloperAssetUnitContext) (model.IntegrationEvidence, error) {
	metadata := developerAssetJSONStrings(unit.Metadata)
	citation := developerAssetJSONStrings(unit.Citation)
	kind := recipeDeveloperAssetEvidenceKind(metadata["asset_kind"])
	if kind == "" || citation == nil || context.indexPublicationID == "" || context.indexPublicationHash == "" ||
		context.sourcePublicationHash == "" || unit.SourcePublicationID == "" || unit.SourceEntityID == "" || unit.ContentHash == "" {
		return model.IntegrationEvidence{}, errors.New("retrieved developer-asset unit is missing exact publication provenance")
	}
	if citation["index_publication_kind"] != context.scopeKind || citation["index_publication_id"] != context.indexPublicationID ||
		citation["publication_id"] != unit.SourcePublicationID || citation["content_hash"] != unit.ContentHash {
		return model.IntegrationEvidence{}, errors.New("retrieved developer-asset unit escaped its exact publication scope")
	}
	sourceRevision := recipeDeveloperAssetSourceRevision(unit, metadata)
	resourceID := strings.Join([]string{
		"developer_asset", context.scopeKind, context.apiID,
		unit.SourcePublicationKind, unit.SourcePublicationID, unit.SourceEntityID,
	}, ":")
	version := strings.Join([]string{
		unit.SourcePublicationID, sourceRevision, context.sourcePublicationHash,
		context.assetContentHash, unit.ContentHash,
	}, "@")
	fingerprint := evidenceFingerprint(
		kind, resourceID, context.scopeKind, context.apiID,
		unit.SourcePublicationKind, unit.SourcePublicationID, sourceRevision,
		context.sourcePublicationHash, context.assetContentHash, unit.SourceEntityID, unit.ContentHash,
	)
	header := strings.Join([]string{
		"Developer asset scope: " + context.scopeKind,
		"API ID: " + context.apiID,
		"Index publication ID: " + context.indexPublicationID,
		"Index publication snapshot hash: " + context.indexPublicationHash,
		"Source publication kind: " + unit.SourcePublicationKind,
		"Source publication ID: " + unit.SourcePublicationID,
		"Source revision: " + sourceRevision,
		"Source publication content hash: " + context.sourcePublicationHash,
		"API asset content hash: " + context.assetContentHash,
		"Source entity ID: " + unit.SourceEntityID,
		"Source entity content hash: " + unit.ContentHash,
	}, "\n")
	contentLimit := maxRecipeDeveloperAssetExcerptRunes - len([]rune(header)) - len([]rune("\nContent:\n"))
	if contentLimit < 1 {
		return model.IntegrationEvidence{}, errors.New("retrieved developer-asset provenance exceeds the evidence bound")
	}
	location, references := recipeDeveloperAssetReference(resourceID, citation)
	return model.IntegrationEvidence{
		Kind: kind, ResourceID: resourceID,
		Label:    firstNonEmpty(strings.TrimSpace(unit.Title), strings.Join(unit.Breadcrumb, " / "), unit.SourceEntityID),
		Location: location, Excerpt: header + "\nContent:\n" + truncateRunes(unit.Content, contentLimit),
		References: references, Version: version, Visibility: unit.Visibility, Fingerprint: fingerprint,
	}, nil
}

func recipeDeveloperAssetGlobalContext(publication model.DeploymentDocumentationPublication, unit model.KnowledgeUnit) (recipeDeveloperAssetUnitContext, bool) {
	if unit.SourcePublicationKind != "documentation_collection" {
		return recipeDeveloperAssetUnitContext{}, false
	}
	for _, member := range publication.Members {
		if member.DocumentationCollectionRevisionID == unit.SourcePublicationID && member.ContentHash != "" {
			return recipeDeveloperAssetUnitContext{
				scopeKind: "global_documentation", indexPublicationID: publication.ID,
				indexPublicationHash: publication.SnapshotHash, sourcePublicationHash: member.ContentHash,
				assetContentHash: member.ContentHash,
			}, true
		}
	}
	return recipeDeveloperAssetUnitContext{}, false
}

func recipeDeveloperAssetAPIContext(publication model.APIDeveloperAssetPublication, unit model.KnowledgeUnit) (recipeDeveloperAssetUnitContext, bool) {
	base := recipeDeveloperAssetUnitContext{
		scopeKind: "api", apiID: publication.APIID, indexPublicationID: publication.ID,
		indexPublicationHash: publication.SnapshotHash,
	}
	switch unit.SourcePublicationKind {
	case "documentation_collection":
		for _, asset := range publication.Documentation {
			if asset.DocumentationCollectionRevisionID == unit.SourcePublicationID {
				base.sourcePublicationHash, base.assetContentHash = asset.ContentHash, asset.ContentHash
				return base, true
			}
		}
	case "contract":
		for _, asset := range publication.Contracts {
			if asset.APIContractRevisionID == unit.SourcePublicationID {
				base.sourcePublicationHash, base.assetContentHash = asset.ContentHash, asset.ContentHash
				return base, true
			}
		}
	case "sdk":
		metadata := developerAssetJSONStrings(unit.Metadata)
		for _, asset := range publication.SDKs {
			if asset.SDKContentPublicationID == unit.SourcePublicationID {
				base.sourcePublicationHash = metadata["sdk_content_hash"]
				base.assetContentHash = asset.ContentHash
				return base, base.sourcePublicationHash != ""
			}
		}
	}
	return recipeDeveloperAssetUnitContext{}, false
}

func recipeDeveloperAssetSortResults(query string, values []store.DeveloperAssetKnowledgeResult) {
	sort.SliceStable(values, func(i, j int) bool {
		left, right := queryLabRerankScore(query, values[i]), queryLabRerankScore(query, values[j])
		if left == right {
			return values[i].Unit.ID < values[j].Unit.ID
		}
		return left > right
	})
}

func (s *Service) retrieveRecipeDeveloperAssetScope(ctx context.Context, integration model.Integration, scope recipeDeveloperAssetScope, scopeKind, query string, limit int) ([]model.IntegrationEvidence, error) {
	publicationID := ""
	knowledgeQuery := store.DeveloperAssetKnowledgeQuery{
		DeploymentID: integration.DeploymentID, QueryText: query,
		BuilderVersion: DeveloperAssetIndexBuilderVersion, RetrievalProfileVersion: DeveloperAssetRetrievalProfileVersion,
		QueryEmbedding: localDeveloperAssetEmbedding(query), Limit: max(limit*4, 24),
	}
	switch scopeKind {
	case "global_documentation":
		if scope.global == nil {
			return nil, nil
		}
		publicationID = scope.global.ID
		knowledgeQuery.DeploymentDocumentationPublicationID = publicationID
	case "api":
		if scope.api == nil {
			return nil, nil
		}
		publicationID = scope.api.ID
		knowledgeQuery.APIDeveloperAssetPublicationID = publicationID
		knowledgeQuery.APIID = integration.ID
	default:
		return nil, errors.New("unsupported recipe developer-asset scope")
	}
	if _, err := s.BuildDeveloperAssetSearchIndex(ctx, scopeKind, publicationID); err != nil {
		return nil, err
	}
	values, err := s.store.RetrieveDeveloperAssetKnowledge(ctx, knowledgeQuery)
	if err != nil {
		return nil, err
	}
	recipeDeveloperAssetSortResults(query, values)
	result := make([]model.IntegrationEvidence, 0, min(limit, len(values)))
	runes := 0
	for _, value := range values {
		var provenance recipeDeveloperAssetUnitContext
		var exact bool
		if scopeKind == "global_documentation" {
			provenance, exact = recipeDeveloperAssetGlobalContext(*scope.global, value.Unit)
		} else {
			provenance, exact = recipeDeveloperAssetAPIContext(*scope.api, value.Unit)
		}
		if !exact {
			return nil, errors.New("retrieval returned developer-asset content outside the selected publication")
		}
		evidence, evidenceErr := recipeDeveloperAssetEvidenceFromUnit(value.Unit, provenance)
		if evidenceErr != nil {
			return nil, evidenceErr
		}
		itemRunes := len([]rune(evidence.Excerpt))
		if len(result) == limit || runes+itemRunes > maxRecipeDeveloperAssetEvidenceRunes {
			break
		}
		result = append(result, evidence)
		runes += itemRunes
	}
	return result, nil
}

func (s *Service) retrieveRecipeDeveloperAssetEvidence(ctx context.Context, integration model.Integration, scope recipeDeveloperAssetScope, query string) ([]model.IntegrationEvidence, error) {
	query = truncateRunes(strings.Join(strings.Fields(query), " "), 500)
	if query == "" {
		query = firstNonEmpty(integration.DisplayName, integration.FamilyKey, integration.ID)
	}
	global, err := s.retrieveRecipeDeveloperAssetScope(ctx, integration, scope, "global_documentation", query, maxRecipeDeveloperAssetGlobalEvidence)
	if err != nil {
		return nil, err
	}
	api, err := s.retrieveRecipeDeveloperAssetScope(ctx, integration, scope, "api", query, maxRecipeDeveloperAssetEvidence-maxRecipeDeveloperAssetGlobalEvidence)
	if err != nil {
		return nil, err
	}
	candidates := append(global, api...)
	result := make([]model.IntegrationEvidence, 0, min(maxRecipeDeveloperAssetEvidence, len(candidates)))
	runes := 0
	for _, item := range candidates {
		itemRunes := len([]rune(item.Excerpt))
		if len(result) == maxRecipeDeveloperAssetEvidence || runes+itemRunes > maxRecipeDeveloperAssetEvidenceRunes {
			break
		}
		result = append(result, item)
		runes += itemRunes
	}
	return result, nil
}

func recipeDeveloperAssetPublicationEvidence(scope recipeDeveloperAssetScope) []model.IntegrationEvidence {
	result := make([]model.IntegrationEvidence, 0, 2)
	if scope.global != nil {
		version := strconv.FormatInt(scope.global.Revision, 10)
		excerpt := fmt.Sprintf("Publication ID: %s\nRevision: %s\nSnapshot hash: %s", scope.global.ID, version, scope.global.SnapshotHash)
		result = append(result, model.IntegrationEvidence{
			Kind: recipeDeveloperAssetGlobalPublicationKind, ResourceID: scope.global.ID,
			Label: "Global developer documentation publication", Excerpt: excerpt, Version: version,
			Visibility:  scope.global.Visibility,
			Fingerprint: evidenceFingerprint(recipeDeveloperAssetGlobalPublicationKind, scope.global.ID, version, scope.global.SnapshotHash),
		})
	}
	if scope.api != nil {
		excerpt := fmt.Sprintf("Publication ID: %s\nAPI ID: %s\nAPI revision ID: %s\nSnapshot hash: %s\nGlobal documentation publication ID: %s", scope.api.ID, scope.api.APIID, scope.api.APIRevisionID, scope.api.SnapshotHash, scope.api.DeploymentDocumentationPublicationID)
		result = append(result, model.IntegrationEvidence{
			Kind: recipeDeveloperAssetAPIPublicationKind, ResourceID: scope.api.ID,
			Label: "Selected API developer-asset publication", Excerpt: excerpt, Version: scope.api.APIRevisionID,
			Visibility:  model.VisibilityPrivate,
			Fingerprint: evidenceFingerprint(recipeDeveloperAssetAPIPublicationKind, scope.api.ID, scope.api.APIRevisionID, scope.api.SnapshotHash),
		})
	}
	return result
}

func (s *Service) latestRecipeDeveloperAssetScope(ctx context.Context, integration model.Integration) (recipeDeveloperAssetScope, error) {
	publications, err := s.store.APIDeveloperAssetPublications(ctx, integration.DeploymentID, integration.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return recipeDeveloperAssetScope{}, nil
		}
		return recipeDeveloperAssetScope{}, err
	}
	if len(publications) == 0 {
		return recipeDeveloperAssetScope{}, nil
	}
	revisions, err := s.store.IntegrationRevisions(ctx, integration.ID)
	if err != nil {
		return recipeDeveloperAssetScope{}, err
	}
	publicationByRevision := make(map[string]*model.APIDeveloperAssetPublication, len(publications))
	for index := range publications {
		publication := &publications[index]
		if publication.APIID != integration.ID || publication.DeploymentID != integration.DeploymentID {
			return recipeDeveloperAssetScope{}, errors.New("API developer-asset publication escaped its selected API")
		}
		ready, readyErr := s.developerAssetPublicationReady(ctx, integration.DeploymentID, "api", publication.ID)
		if readyErr != nil {
			return recipeDeveloperAssetScope{}, readyErr
		}
		if ready {
			publicationByRevision[publication.APIRevisionID] = publication
		}
	}
	var selected *model.APIDeveloperAssetPublication
	selectedRevision := int64(-1)
	for _, revision := range revisions {
		publication := publicationByRevision[revision.ID]
		if revision.State == "published" && publication != nil && revision.Revision > selectedRevision {
			selected = publication
			selectedRevision = revision.Revision
		}
	}
	if selected == nil {
		return recipeDeveloperAssetScope{}, errors.New("API developer-asset publications are not tied to a published API revision")
	}
	selectedCopy := *selected
	scope := recipeDeveloperAssetScope{api: &selectedCopy}
	if selected.DeploymentDocumentationPublicationID != "" {
		global, lookupErr := s.store.DeploymentDocumentationPublication(ctx, integration.DeploymentID, selected.DeploymentDocumentationPublicationID)
		if lookupErr != nil {
			return recipeDeveloperAssetScope{}, lookupErr
		}
		ready, readyErr := s.developerAssetPublicationReady(ctx, integration.DeploymentID, "global_documentation", global.ID)
		if readyErr != nil {
			return recipeDeveloperAssetScope{}, readyErr
		}
		if ready {
			scope.global = &global
		}
	}
	return scope, nil
}

func scopedRecipeDeveloperAssetQuery(integration model.Integration, evidence []model.IntegrationEvidence) string {
	parts := []string{integration.DisplayName, integration.FamilyKey, integration.VersionKey, integration.Description}
	for _, item := range evidence {
		if item.Kind == "tool" {
			parts = append(parts, item.Label, item.Excerpt)
		}
	}
	return truncateRunes(strings.Join(parts, "\n"), 500)
}

func (s *Service) scopedRecipeDeveloperAssetEvidence(ctx context.Context, integration model.Integration, query string) ([]model.IntegrationEvidence, error) {
	scope, err := s.latestRecipeDeveloperAssetScope(ctx, integration)
	if err != nil || scope.api == nil {
		return nil, err
	}
	values := recipeDeveloperAssetPublicationEvidence(scope)
	retrieved, err := s.retrieveRecipeDeveloperAssetEvidence(ctx, integration, scope, query)
	if err != nil {
		return nil, err
	}
	return append(values, retrieved...), nil
}

func (s *Service) recipeDeveloperAssetScopeFromEvidence(ctx context.Context, deploymentID, apiID string, evidence []model.IntegrationEvidence) (recipeDeveloperAssetScope, error) {
	var globalMarker, apiMarker *model.IntegrationEvidence
	for _, item := range evidence {
		switch item.Kind {
		case recipeDeveloperAssetGlobalPublicationKind:
			if globalMarker != nil {
				return recipeDeveloperAssetScope{}, errors.New("stored analysis has ambiguous global developer-asset publications")
			}
			copy := item
			globalMarker = &copy
		case recipeDeveloperAssetAPIPublicationKind:
			if apiMarker != nil {
				return recipeDeveloperAssetScope{}, errors.New("stored analysis has ambiguous API developer-asset publications")
			}
			copy := item
			apiMarker = &copy
		}
	}
	if apiMarker == nil {
		return recipeDeveloperAssetScope{}, nil
	}
	apiPublication, err := s.store.APIDeveloperAssetPublication(ctx, deploymentID, apiMarker.ResourceID)
	if err != nil || apiPublication.APIID != apiID {
		if err == nil {
			err = errors.New("stored analysis developer-asset publication does not belong to its selected API")
		}
		return recipeDeveloperAssetScope{}, err
	}
	expectedAPIFingerprint := evidenceFingerprint(recipeDeveloperAssetAPIPublicationKind, apiPublication.ID, apiPublication.APIRevisionID, apiPublication.SnapshotHash)
	if apiMarker.Version != apiPublication.APIRevisionID || apiMarker.Fingerprint != expectedAPIFingerprint ||
		recipeEvidenceField(apiMarker.Excerpt, "Publication ID") != apiPublication.ID ||
		recipeEvidenceField(apiMarker.Excerpt, "API ID") != apiPublication.APIID ||
		recipeEvidenceField(apiMarker.Excerpt, "API revision ID") != apiPublication.APIRevisionID ||
		recipeEvidenceField(apiMarker.Excerpt, "Snapshot hash") != apiPublication.SnapshotHash ||
		recipeEvidenceField(apiMarker.Excerpt, "Global documentation publication ID") != apiPublication.DeploymentDocumentationPublicationID {
		return recipeDeveloperAssetScope{}, errors.New("stored analysis API developer-asset publication fingerprint is inexact")
	}
	scope := recipeDeveloperAssetScope{api: &apiPublication}
	if globalMarker != nil {
		global, lookupErr := s.store.DeploymentDocumentationPublication(ctx, deploymentID, globalMarker.ResourceID)
		if lookupErr != nil {
			return recipeDeveloperAssetScope{}, lookupErr
		}
		if apiPublication.DeploymentDocumentationPublicationID != global.ID {
			return recipeDeveloperAssetScope{}, errors.New("stored analysis global documentation does not match its API publication")
		}
		version := strconv.FormatInt(global.Revision, 10)
		expectedGlobalFingerprint := evidenceFingerprint(recipeDeveloperAssetGlobalPublicationKind, global.ID, version, global.SnapshotHash)
		if globalMarker.Version != version || globalMarker.Fingerprint != expectedGlobalFingerprint ||
			recipeEvidenceField(globalMarker.Excerpt, "Publication ID") != global.ID ||
			recipeEvidenceField(globalMarker.Excerpt, "Revision") != version ||
			recipeEvidenceField(globalMarker.Excerpt, "Snapshot hash") != global.SnapshotHash {
			return recipeDeveloperAssetScope{}, errors.New("stored analysis global developer-asset publication fingerprint is inexact")
		}
		scope.global = &global
	} else if apiPublication.DeploymentDocumentationPublicationID != "" {
		return recipeDeveloperAssetScope{}, errors.New("stored analysis omitted its API publication's global documentation")
	}
	return scope, nil
}

func selectedRecipeDeveloperAssetEvidence(evidence []model.IntegrationEvidence, evidenceIDs []string) []model.IntegrationEvidence {
	selectedIDs := make(map[string]bool, len(evidenceIDs))
	for _, evidenceID := range evidenceIDs {
		selectedIDs[evidenceID] = true
	}
	selected := make([]model.IntegrationEvidence, 0, len(selectedIDs))
	for _, item := range evidence {
		if recipeDeveloperAssetSupportingKind(item.Kind) && selectedIDs[item.ResourceID] {
			selected = append(selected, item)
		}
	}
	return selected
}

// prioritizeRecipeDeveloperAssetEvidence keeps explicit citations before
// filling the remainder of the same small count and context budget with fresh
// retrieval results.
func prioritizeRecipeDeveloperAssetEvidence(evidence, priority []model.IntegrationEvidence) []model.IntegrationEvidence {
	values := make([]model.IntegrationEvidence, 0, len(evidence)+len(priority))
	for _, item := range evidence {
		if !recipeDeveloperAssetSupportingKind(item.Kind) {
			values = append(values, item)
		}
	}
	seen := make(map[string]bool, len(priority)+len(evidence))
	runes := 0
	for _, item := range append(priority, evidence...) {
		if !recipeDeveloperAssetSupportingKind(item.Kind) {
			continue
		}
		itemRunes := len([]rune(item.Excerpt))
		if seen[item.ResourceID] || len(seen) == maxRecipeDeveloperAssetEvidence || runes+itemRunes > maxRecipeDeveloperAssetEvidenceRunes {
			continue
		}
		values = append(values, item)
		seen[item.ResourceID] = true
		runes += itemRunes
	}
	return values
}

func (s *Service) relevantRecipeDeveloperAssetAnalysis(ctx context.Context, product model.Product, analysis model.IntegrationAnalysis, outcome string) (model.IntegrationAnalysis, error) {
	apiID, scoped := integrationScopeID(analysis.Evidence)
	if !scoped {
		return analysis, nil
	}
	scope, err := s.recipeDeveloperAssetScopeFromEvidence(ctx, product.ID, apiID, analysis.Evidence)
	if err != nil || scope.api == nil {
		return analysis, err
	}
	integration, err := s.store.Integration(ctx, product.ID, apiID)
	if err != nil {
		return analysis, err
	}
	retrieved, err := s.retrieveRecipeDeveloperAssetEvidence(ctx, integration, scope, outcome)
	if err != nil {
		return analysis, err
	}
	// The integration-analysis model may have deliberately selected one of the
	// initially retrieved supporting facts for a recipe. Give those exact facts
	// priority when the outcome-specific rerank is assembled so a rank change
	// cannot silently erase an otherwise valid citation before the recipe is
	// materialized.
	selectedIDs := make([]string, 0)
	for _, recipe := range analysis.Plan.Recipes {
		selectedIDs = append(selectedIDs, recipe.EvidenceIDs...)
	}
	selected := selectedRecipeDeveloperAssetEvidence(analysis.Evidence, selectedIDs)
	values := make([]model.IntegrationEvidence, 0, len(analysis.Evidence)+len(retrieved))
	for _, item := range analysis.Evidence {
		if !recipeDeveloperAssetSupportingKind(item.Kind) {
			values = append(values, item)
		}
	}
	values = append(values, retrieved...)
	analysis.Evidence = prioritizeRecipeDeveloperAssetEvidence(values, selected)
	return analysis, nil
}

func parseRecipeDeveloperAssetResourceID(resourceID string) (scopeKind, apiID, sourcePublicationID, sourceEntityID string, ok bool) {
	parts := strings.Split(resourceID, ":")
	if len(parts) != 6 || parts[0] != "developer_asset" || (parts[1] != "global_documentation" && parts[1] != "api") ||
		parts[4] == "" || parts[5] == "" || (parts[1] == "api" && parts[2] == "") {
		return "", "", "", "", false
	}
	return parts[1], parts[2], parts[4], parts[5], true
}

// restoreRecipeDeveloperAssetDependencies performs bounded exact-identifier
// lookups for selected historical dependencies which a fresh relevance ranking
// did not happen to return. Ranking changes caused by a newly attached but
// unrelated asset therefore cannot make a recipe stale; the selected nested
// publication fact must actually disappear or change.
func (s *Service) restoreRecipeDeveloperAssetDependencies(ctx context.Context, integration model.Integration, evidence []model.IntegrationEvidence, dependencies []model.RecipeDependency) ([]model.IntegrationEvidence, error) {
	scope, err := s.recipeDeveloperAssetScopeFromEvidence(ctx, integration.DeploymentID, integration.ID, evidence)
	if err != nil || scope.api == nil {
		return evidence, err
	}
	existing := make(map[string]bool, len(evidence))
	for _, item := range evidence {
		existing[item.ResourceID] = true
	}
	for _, dependency := range dependencies {
		if !recipeDeveloperAssetSupportingKind(dependency.Kind) || existing[dependency.ResourceID] {
			continue
		}
		scopeKind, apiID, _, entityID, ok := parseRecipeDeveloperAssetResourceID(dependency.ResourceID)
		if !ok || scopeKind == "api" && apiID != integration.ID {
			continue
		}
		candidates, lookupErr := s.retrieveRecipeDeveloperAssetScope(ctx, integration, scope, scopeKind, entityID, 8)
		if lookupErr != nil {
			return evidence, lookupErr
		}
		for _, item := range candidates {
			if item.Kind == dependency.Kind && item.ResourceID == dependency.ResourceID && item.Version == dependency.Version {
				evidence = append(evidence, item)
				existing[item.ResourceID] = true
				break
			}
		}
	}
	return evidence, nil
}

func (s *Service) validatePublicRecipeDeveloperAssetEvidence(ctx context.Context, productID string, recipe model.Recipe, item model.IntegrationEvidence) error {
	if !recipeDeveloperAssetSupportingKind(item.Kind) {
		return nil
	}
	if item.Visibility != model.VisibilityPublic {
		return errPublicRecipeEvidence
	}
	scopeKind := recipeEvidenceField(item.Excerpt, "Developer asset scope")
	indexID := recipeEvidenceField(item.Excerpt, "Index publication ID")
	indexHash := recipeEvidenceField(item.Excerpt, "Index publication snapshot hash")
	sourceID := recipeEvidenceField(item.Excerpt, "Source publication ID")
	sourceHash := recipeEvidenceField(item.Excerpt, "Source publication content hash")
	assetHash := recipeEvidenceField(item.Excerpt, "API asset content hash")
	if indexID == "" || indexHash == "" || sourceID == "" || sourceHash == "" || assetHash == "" {
		return errPublicRecipeEvidence
	}
	switch scopeKind {
	case "global_documentation":
		if item.Kind != recipeDeveloperAssetDocumentationKind {
			return errPublicRecipeEvidence
		}
		publication, err := s.store.DeploymentDocumentationPublication(ctx, productID, indexID)
		if err != nil {
			return err
		}
		if publication.Visibility != model.VisibilityPublic || publication.SnapshotHash != indexHash {
			return errPublicRecipeEvidence
		}
		memberPublic := false
		for _, member := range publication.Members {
			memberPublic = memberPublic || member.DocumentationCollectionRevisionID == sourceID && member.ContentHash == sourceHash && member.ContentHash == assetHash && member.Visibility == model.VisibilityPublic
		}
		if !memberPublic {
			return errPublicRecipeEvidence
		}
		revision, lookupErr := s.store.DocumentationCollectionRevision(ctx, productID, sourceID)
		if lookupErr != nil {
			return lookupErr
		}
		if revision.Revision.Visibility != model.VisibilityPublic || revision.Revision.ContentHash != sourceHash {
			return errPublicRecipeEvidence
		}
	case "api":
		publication, err := s.store.APIDeveloperAssetPublication(ctx, productID, indexID)
		if err != nil {
			return err
		}
		if publication.APIID != recipe.IntegrationID || publication.SnapshotHash != indexHash {
			return errPublicRecipeEvidence
		}
		integration, err := s.store.Integration(ctx, productID, publication.APIID)
		if err != nil {
			return err
		}
		if integration.Visibility != model.VisibilityPublic {
			return errPublicRecipeEvidence
		}
		assetPublic := false
		switch item.Kind {
		case recipeDeveloperAssetDocumentationKind:
			for _, asset := range publication.Documentation {
				assetPublic = assetPublic || asset.DocumentationCollectionRevisionID == sourceID && asset.ContentHash == sourceHash && asset.ContentHash == assetHash && asset.Visibility == model.VisibilityPublic
			}
			if assetPublic {
				revision, lookupErr := s.store.DocumentationCollectionRevision(ctx, productID, sourceID)
				if lookupErr != nil {
					return lookupErr
				}
				assetPublic = revision.Revision.Visibility == model.VisibilityPublic && revision.Revision.ContentHash == sourceHash
			}
		case recipeDeveloperAssetContractKind:
			for _, asset := range publication.Contracts {
				assetPublic = assetPublic || asset.APIContractRevisionID == sourceID && asset.ContentHash == sourceHash && asset.ContentHash == assetHash && asset.Visibility == model.VisibilityPublic
			}
			if assetPublic {
				revision, lookupErr := s.store.APIContractRevision(ctx, productID, sourceID)
				if lookupErr != nil {
					return lookupErr
				}
				assetPublic = revision.Visibility == model.VisibilityPublic && revision.ContentHash == sourceHash
			}
		case recipeDeveloperAssetSDKKind:
			for _, asset := range publication.SDKs {
				if asset.SDKContentPublicationID != sourceID || asset.ContentHash != assetHash || asset.Visibility != model.VisibilityPublic {
					continue
				}
				record, lookupErr := s.store.SDKContentPublication(ctx, productID, sourceID)
				if lookupErr != nil {
					return lookupErr
				}
				if record.Publication.SDKReleaseID != asset.SDKReleaseID {
					continue
				}
				release, lookupErr := s.store.SDKRelease(ctx, productID, record.Publication.SDKReleaseID)
				if lookupErr != nil {
					return lookupErr
				}
				if release.ID != asset.SDKReleaseID || release.SDKPackageID != asset.SDKPackageID {
					continue
				}
				packageValue, lookupErr := s.store.SDKPackage(ctx, productID, release.SDKPackageID)
				if lookupErr != nil {
					return lookupErr
				}
				candidate, lookupErr := s.store.SDKContentCandidate(ctx, productID, record.Publication.SDKContentCandidateID)
				if lookupErr != nil {
					return lookupErr
				}
				assetPublic = record.Publication.ContentHash == sourceHash && record.Publication.Visibility == model.VisibilityPublic &&
					release.Visibility == model.VisibilityPublic && packageValue.Visibility == model.VisibilityPublic &&
					candidate.Candidate.Visibility == model.VisibilityPublic && candidate.Candidate.ContentHash == sourceHash
			}
		}
		if !assetPublic {
			return errPublicRecipeEvidence
		}
	default:
		return errPublicRecipeEvidence
	}
	return nil
}
