package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

// These values identify the immutable projection contract. A change to unit
// shape, selection semantics, or retrieval behaviour must use a new version so
// an existing ready generation is never silently reinterpreted.
const (
	DeveloperAssetIndexBuilderVersion     = "published-developer-assets-v2"
	DeveloperAssetRetrievalProfileVersion = "hybrid-v1"
)

const developerAssetIndexManifestVersion = "developer-asset-index-manifest-v1"

type developerAssetIndexOrder struct {
	assetOrdinal  int
	memberOrdinal int
	entityOrdinal int
	kindRank      int
	tieBreaker    string
}

type developerAssetIndexDraft struct {
	unit  model.KnowledgeUnit
	order developerAssetIndexOrder
	scope *model.KnowledgeUnitAPIScope
}

type developerAssetIndexTarget struct {
	publicationKind string
	publicationID   string
	assetKind       string
	drafts          []developerAssetIndexDraft
}

// BuildDeveloperAssetSearchIndex materializes retrieval units only from an
// exact immutable publication. It currently accepts the two aggregate
// publication kinds consumed by Query Lab: global_documentation and api.
// Calls are idempotent for the immutable builder/profile identity and reuse an
// already-ready generation.
func (s *Service) BuildDeveloperAssetSearchIndex(ctx context.Context, publicationKind, publicationID string) (model.SearchIndexGeneration, error) {
	publicationKind = strings.TrimSpace(publicationKind)
	publicationID = strings.TrimSpace(publicationID)
	if publicationID == "" || (publicationKind != "global_documentation" && publicationKind != "api") {
		return model.SearchIndexGeneration{}, errors.New("developer-asset index requires an exact global_documentation or api publication")
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.SearchIndexGeneration{}, err
	}

	// Resolve the polymorphic publication before creating a generation. This
	// avoids leaving a generation that cannot satisfy the database publication
	// guard at all.
	var target developerAssetIndexTarget
	switch publicationKind {
	case "global_documentation":
		publication, lookupErr := s.store.DeploymentDocumentationPublication(ctx, deployment.ID, publicationID)
		if lookupErr != nil {
			return model.SearchIndexGeneration{}, lookupErr
		}
		target = developerAssetIndexTarget{publicationKind: publicationKind, publicationID: publication.ID, assetKind: "documentation"}
	case "api":
		publication, lookupErr := s.store.APIDeveloperAssetPublication(ctx, deployment.ID, publicationID)
		if lookupErr != nil {
			return model.SearchIndexGeneration{}, lookupErr
		}
		target = developerAssetIndexTarget{publicationKind: publicationKind, publicationID: publication.ID, assetKind: "mixed"}
	}

	existing, err := s.store.SearchIndexGenerations(ctx, deployment.ID, publicationKind, target.publicationID)
	if err != nil {
		return model.SearchIndexGeneration{}, err
	}
	var generation model.SearchIndexGeneration
	expectedState := "building"
	for _, value := range existing {
		if value.BuilderVersion != DeveloperAssetIndexBuilderVersion || value.RetrievalProfileVersion != DeveloperAssetRetrievalProfileVersion {
			continue
		}
		if value.AssetKind != target.assetKind || value.EmbeddingModel != developerAssetEmbeddingModel || value.EmbeddingDimensions == nil || *value.EmbeddingDimensions != developerAssetEmbeddingDimensions {
			return model.SearchIndexGeneration{}, errors.New("developer-asset index generation processor identity does not match the immutable builder version")
		}
		if value.State == "ready" {
			return value, nil
		}
		if value.State != "queued" && value.State != "building" && value.State != "failed" {
			return model.SearchIndexGeneration{}, fmt.Errorf("developer-asset index generation is %s", value.State)
		}
		generation, expectedState = value, value.State
		break
	}
	if generation.ID == "" {
		now := s.now()
		dimensions := developerAssetEmbeddingDimensions
		generation = model.SearchIndexGeneration{
			ID: deterministicDeveloperAssetUUID(strings.Join([]string{
				deployment.ID, publicationKind, target.publicationID,
				DeveloperAssetIndexBuilderVersion, DeveloperAssetRetrievalProfileVersion,
			}, "\x00")),
			DeploymentID: deployment.ID, PublicationKind: publicationKind, PublicationID: target.publicationID,
			AssetKind: target.assetKind, BuilderVersion: DeveloperAssetIndexBuilderVersion,
			RetrievalProfileVersion: DeveloperAssetRetrievalProfileVersion,
			EmbeddingModel:          developerAssetEmbeddingModel, EmbeddingDimensions: &dimensions,
			State: "building", Diagnostics: json.RawMessage(`{}`), StartedAt: &now,
		}
		generation, err = s.store.CreateSearchIndexGeneration(ctx, generation)
		if err != nil {
			if !errors.Is(err, store.ErrConflict) {
				return model.SearchIndexGeneration{}, err
			}
			// Another worker may have won the deterministic create. Re-read and
			// either reuse its ready value or build against its current state.
			record, lookupErr := s.store.SearchIndexGeneration(ctx, deployment.ID, generation.ID)
			if lookupErr != nil {
				return model.SearchIndexGeneration{}, err
			}
			if record.Generation.BuilderVersion != DeveloperAssetIndexBuilderVersion ||
				record.Generation.RetrievalProfileVersion != DeveloperAssetRetrievalProfileVersion ||
				record.Generation.EmbeddingModel != developerAssetEmbeddingModel ||
				record.Generation.EmbeddingDimensions == nil || *record.Generation.EmbeddingDimensions != developerAssetEmbeddingDimensions {
				return model.SearchIndexGeneration{}, errors.New("conflicting developer-asset generation has a different processor identity")
			}
			if record.Generation.State == "ready" {
				return record.Generation, nil
			}
			generation, expectedState = record.Generation, record.Generation.State
		}
	}
	if generation.StartedAt == nil {
		startedAt := s.now()
		generation.StartedAt = &startedAt
	}

	switch publicationKind {
	case "global_documentation":
		publication, lookupErr := s.store.DeploymentDocumentationPublication(ctx, deployment.ID, target.publicationID)
		if lookupErr == nil {
			target.drafts, lookupErr = s.buildGlobalDocumentationIndex(ctx, deployment.ID, publication)
		}
		err = lookupErr
	case "api":
		publication, lookupErr := s.store.APIDeveloperAssetPublication(ctx, deployment.ID, target.publicationID)
		if lookupErr == nil {
			target.drafts, lookupErr = s.buildAPIDeveloperAssetIndex(ctx, deployment.ID, publication)
		}
		err = lookupErr
	}
	if err != nil {
		return s.failDeveloperAssetIndexGeneration(ctx, generation, expectedState, err)
	}

	record, err := finalizeDeveloperAssetIndexGeneration(generation, target.drafts, s.now())
	if err != nil {
		return s.failDeveloperAssetIndexGeneration(ctx, generation, expectedState, err)
	}
	completed, err := s.store.CompleteSearchIndexGeneration(ctx, record, expectedState)
	if err == nil {
		return completed, nil
	}
	if errors.Is(err, store.ErrConflict) {
		current, lookupErr := s.store.SearchIndexGeneration(ctx, deployment.ID, generation.ID)
		if lookupErr == nil && current.Generation.State == "ready" {
			return current.Generation, nil
		}
	}
	_, _ = s.failDeveloperAssetIndexGeneration(ctx, generation, expectedState, err)
	return model.SearchIndexGeneration{}, err
}

func (s *Service) failDeveloperAssetIndexGeneration(ctx context.Context, generation model.SearchIndexGeneration, expectedState string, cause error) (model.SearchIndexGeneration, error) {
	diagnostics, _ := json.Marshal(map[string]any{
		"builder_version": DeveloperAssetIndexBuilderVersion,
		"error":           cause.Error(),
	})
	generation.State = "failed"
	generation.Diagnostics = diagnostics
	generation.ReadyAt = nil
	failed, err := s.store.FailSearchIndexGeneration(ctx, generation, expectedState)
	if err == nil {
		return failed, cause
	}
	if errors.Is(err, store.ErrConflict) {
		current, lookupErr := s.store.SearchIndexGeneration(ctx, generation.DeploymentID, generation.ID)
		if lookupErr == nil && current.Generation.State == "ready" {
			return current.Generation, nil
		}
	}
	return model.SearchIndexGeneration{}, errors.Join(cause, err)
}

func finalizeDeveloperAssetIndexGeneration(generation model.SearchIndexGeneration, drafts []developerAssetIndexDraft, readyAtTime time.Time) (store.SearchIndexGenerationRecord, error) {
	sort.SliceStable(drafts, func(i, j int) bool { return developerAssetIndexOrderLess(drafts[i], drafts[j]) })

	// The database uniqueness boundary is publication kind + source entity.
	// Resolve duplicates before assigning ordinals and require duplicate source
	// evidence to be byte-for-byte equivalent rather than picking arbitrarily.
	deduplicated := make([]developerAssetIndexDraft, 0, len(drafts))
	seen := make(map[string]int, len(drafts))
	for _, draft := range drafts {
		key := draft.unit.SourcePublicationKind + "\x00" + draft.unit.SourceEntityID
		if priorIndex, ok := seen[key]; ok {
			prior := &deduplicated[priorIndex]
			if prior.unit.ContentHash != draft.unit.ContentHash || prior.unit.Kind != draft.unit.Kind || prior.unit.Content != draft.unit.Content {
				return store.SearchIndexGenerationRecord{}, fmt.Errorf("conflicting duplicate source entity %s", draft.unit.SourceEntityID)
			}
			prior.unit.Identifiers = canonicalStringSet(prior.unit.Identifiers, draft.unit.Identifiers)
			continue
		}
		seen[key] = len(deduplicated)
		deduplicated = append(deduplicated, draft)
	}

	record := store.SearchIndexGenerationRecord{Generation: generation}
	manifestUnits := make([]map[string]any, 0, len(deduplicated))
	manifestScopes := make([]map[string]any, 0, len(deduplicated))
	for ordinal := range deduplicated {
		draft := &deduplicated[ordinal]
		unit := draft.unit
		unit.ID = deterministicDeveloperAssetUUID(generation.ID + "\x00" + unit.SourcePublicationKind + "\x00" + unit.SourceEntityID)
		unit.SearchIndexGenerationID = generation.ID
		unit.DeploymentID = generation.DeploymentID
		unit.Ordinal = ordinal
		unit.Identifiers = canonicalStringSet(unit.Identifiers)
		if !validDeveloperAssetContentHash(unit.ContentHash) || strings.TrimSpace(unit.Content) == "" || !unit.Visibility.Valid() {
			return store.SearchIndexGenerationRecord{}, fmt.Errorf("invalid knowledge unit %s", unit.SourceEntityID)
		}
		if len(unit.Citation) == 0 || len(unit.Metadata) == 0 {
			return store.SearchIndexGenerationRecord{}, fmt.Errorf("knowledge unit %s is missing citation or metadata", unit.SourceEntityID)
		}
		unit.Embedding = localDeveloperAssetEmbedding(developerAssetIndexText(unit.Title, unit.Content))
		record.Units = append(record.Units, unit)
		manifestUnits = append(manifestUnits, map[string]any{
			"ordinal": ordinal, "kind": unit.Kind,
			"source_publication_kind": unit.SourcePublicationKind,
			"source_publication_id":   unit.SourcePublicationID,
			"source_entity_id":        unit.SourceEntityID,
			"parent_source_entity_id": unit.ParentSourceEntityID,
			"title":                   unit.Title, "breadcrumb": unit.Breadcrumb, "content": unit.Content,
			"language": unit.Language, "ecosystem": unit.Ecosystem, "identifiers": unit.Identifiers,
			"visibility": unit.Visibility, "citation": json.RawMessage(unit.Citation),
			"metadata": json.RawMessage(unit.Metadata), "content_hash": unit.ContentHash,
		})
		if draft.scope != nil {
			scope := *draft.scope
			scope.KnowledgeUnitID = unit.ID
			scope.DeploymentID = generation.DeploymentID
			record.APIScopes = append(record.APIScopes, scope)
			manifestScopes = append(manifestScopes, map[string]any{
				"unit_source_publication_kind": unit.SourcePublicationKind,
				"unit_source_entity_id":        unit.SourceEntityID,
				"api_id":                       scope.APIID, "api_sdk_binding_id": scope.APISDKBindingID,
				"scope_kind": scope.ScopeKind, "selector_hash": scope.SelectorHash,
			})
		}
	}
	sort.Slice(manifestScopes, func(i, j int) bool {
		left, _ := json.Marshal(manifestScopes[i])
		right, _ := json.Marshal(manifestScopes[j])
		return string(left) < string(right)
	})
	manifest, err := json.Marshal(map[string]any{
		"manifest_version": developerAssetIndexManifestVersion,
		"publication_kind": generation.PublicationKind, "publication_id": generation.PublicationID,
		"builder_version":           DeveloperAssetIndexBuilderVersion,
		"retrieval_profile_version": DeveloperAssetRetrievalProfileVersion,
		"embedding_model":           developerAssetEmbeddingModel,
		"embedding_dimensions":      developerAssetEmbeddingDimensions,
		"units":                     manifestUnits, "api_scopes": manifestScopes,
	})
	if err != nil {
		return store.SearchIndexGenerationRecord{}, err
	}
	diagnostics, err := json.Marshal(map[string]any{
		"manifest_version":          developerAssetIndexManifestVersion,
		"builder_version":           DeveloperAssetIndexBuilderVersion,
		"retrieval_profile_version": DeveloperAssetRetrievalProfileVersion,
		"embedding_model":           developerAssetEmbeddingModel,
		"unit_count":                len(record.Units),
	})
	if err != nil {
		return store.SearchIndexGenerationRecord{}, err
	}
	readyAt := readyAtTime.UTC()
	record.Generation.State = "ready"
	record.Generation.UnitCount = len(record.Units)
	record.Generation.ContentHash = contentHash(manifest)
	record.Generation.Diagnostics = diagnostics
	record.Generation.ReadyAt = &readyAt
	return record, nil
}

func deterministicDeveloperAssetUUID(seed string) string {
	digest := sha256.Sum256([]byte(seed))
	digest[6] = digest[6]&0x0f | 0x50
	digest[8] = digest[8]&0x3f | 0x80
	hexValue := hex.EncodeToString(digest[:16])
	return hexValue[0:8] + "-" + hexValue[8:12] + "-" + hexValue[12:16] + "-" + hexValue[16:20] + "-" + hexValue[20:32]
}

func developerAssetIndexOrderLess(left, right developerAssetIndexDraft) bool {
	if left.order.assetOrdinal != right.order.assetOrdinal {
		return left.order.assetOrdinal < right.order.assetOrdinal
	}
	if left.order.memberOrdinal != right.order.memberOrdinal {
		return left.order.memberOrdinal < right.order.memberOrdinal
	}
	if left.order.kindRank != right.order.kindRank {
		return left.order.kindRank < right.order.kindRank
	}
	if left.order.entityOrdinal != right.order.entityOrdinal {
		return left.order.entityOrdinal < right.order.entityOrdinal
	}
	if left.order.tieBreaker != right.order.tieBreaker {
		return left.order.tieBreaker < right.order.tieBreaker
	}
	if left.unit.SourcePublicationKind != right.unit.SourcePublicationKind {
		return left.unit.SourcePublicationKind < right.unit.SourcePublicationKind
	}
	if left.unit.SourcePublicationID != right.unit.SourcePublicationID {
		return left.unit.SourcePublicationID < right.unit.SourcePublicationID
	}
	return left.unit.SourceEntityID < right.unit.SourceEntityID
}

func canonicalStringSet(values ...[]string) []string {
	seen := make(map[string]bool)
	for _, list := range values {
		for _, value := range list {
			value = strings.TrimSpace(value)
			if value != "" {
				seen[value] = true
			}
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func validDeveloperAssetContentHash(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func developerAssetVisibility(values ...model.Visibility) (model.Visibility, error) {
	result := model.VisibilityPublic
	for _, value := range values {
		if !value.Valid() {
			return "", ErrInvalidVisibility
		}
		if value == model.VisibilityPrivate {
			result = model.VisibilityPrivate
		}
	}
	return result, nil
}

func canonicalJSONObject(raw json.RawMessage) (json.RawMessage, map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, nil, errors.New("selector must be a JSON object")
	}
	canonical, err := json.Marshal(value)
	return canonical, value, err
}

func sourceMetadataValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, errors.New("source metadata must be valid JSON")
	}
	return value, nil
}

func marshalDeveloperAssetObject(value map[string]any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil || object == nil {
		return nil, errors.New("developer-asset index value must be a JSON object")
	}
	return encoded, nil
}

func compactDeveloperAssetJSON(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "{}", nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(value)
	return string(encoded), err
}

func developerAssetMapContent(agentMarkdown string, body any) (string, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	agentMarkdown = strings.TrimSpace(agentMarkdown)
	if agentMarkdown == "" {
		return string(encoded), nil
	}
	return agentMarkdown + "\n\nStructured map: " + string(encoded), nil
}

func developerAssetIndexText(title, content string) string {
	if strings.TrimSpace(title) == "" {
		return strings.TrimSpace(content)
	}
	return strings.TrimSpace(title) + "\n" + strings.TrimSpace(content)
}

func developerAssetInteger(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func developerAssetOrdinalKey(values ...int) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = fmt.Sprintf("%010d", value)
	}
	return strings.Join(parts, "/")
}

func developerAssetLineRange(start, end *int) string {
	if start == nil && end == nil {
		return ""
	}
	if end == nil || (start != nil && *start == *end) {
		if start == nil {
			return strconv.Itoa(*end)
		}
		return strconv.Itoa(*start)
	}
	if start == nil {
		return "-" + strconv.Itoa(*end)
	}
	return strconv.Itoa(*start) + "-" + strconv.Itoa(*end)
}

type developerAssetSelectorDomain string

const (
	developerAssetDocumentationSelector developerAssetSelectorDomain = "documentation"
	developerAssetSDKSelector           developerAssetSelectorDomain = "sdk"
)

type developerAssetSelector struct {
	values     map[string]map[string]bool
	present    map[string]bool
	scopeOnly  map[string]bool
	includeMap *bool
	selectAll  bool
	selectNone bool
}

type developerAssetSelectorCandidate struct {
	kind        string
	documentID  string
	fileID      string
	sectionID   string
	symbolID    string
	sampleID    string
	sourcePath  string
	language    string
	contentKind string
	module      string
	identifiers []string
	isMap       bool
}

var developerAssetSelectorAliases = map[string]string{
	"document_id": "document_ids", "document_ids": "document_ids", "documentation_document_ids": "document_ids", "documents": "document_ids",
	"file_id": "file_ids", "file_ids": "file_ids", "sdk_file_ids": "file_ids", "files": "file_ids",
	"section_id": "section_ids", "section_ids": "section_ids", "documentation_section_ids": "section_ids", "sdk_section_ids": "section_ids", "sections": "section_ids",
	"symbol_id": "symbol_ids", "symbol_ids": "symbol_ids", "sdk_symbol_ids": "symbol_ids", "symbols": "symbol_ids",
	"sample_id": "sample_ids", "sample_ids": "sample_ids", "sdk_sample_ids": "sample_ids", "samples": "sample_ids",
	"source_path": "source_paths", "source_paths": "source_paths", "paths": "source_paths",
	"language": "languages", "languages": "languages", "content_kind": "content_kinds", "content_kinds": "content_kinds", "kinds": "content_kinds",
	"module": "modules", "module_id": "modules", "module_ids": "modules", "module_names": "modules", "modules": "modules",
	"identifier": "identifiers", "identifiers": "identifiers",
	"api_ids": "api_ids", "contract_revision_ids": "contract_revision_ids",
	"contract_operation_ids": "contract_operation_ids", "operation_ids": "contract_operation_ids",
	"operation_keys": "operation_keys", "capabilities": "capabilities", "capability_ids": "capabilities",
	"evidence_ids": "evidence_ids",
}

func parseDeveloperAssetSelector(raw json.RawMessage, domain developerAssetSelectorDomain) (developerAssetSelector, json.RawMessage, error) {
	canonical, object, err := canonicalJSONObject(raw)
	if err != nil {
		return developerAssetSelector{}, nil, err
	}
	result := developerAssetSelector{values: make(map[string]map[string]bool), present: make(map[string]bool), scopeOnly: make(map[string]bool)}
	allowed := map[string]bool{
		"section_ids": true, "source_paths": true, "languages": true,
		"content_kinds": true, "identifiers": true,
	}
	if domain == developerAssetDocumentationSelector {
		allowed["document_ids"] = true
	} else {
		allowed["file_ids"], allowed["symbol_ids"], allowed["sample_ids"], allowed["modules"] = true, true, true, true
		for _, key := range []string{"api_ids", "contract_revision_ids", "contract_operation_ids", "operation_keys", "capabilities", "evidence_ids"} {
			allowed[key] = true
		}
	}
	for originalKey, encoded := range object {
		key := strings.ToLower(strings.TrimSpace(originalKey))
		if key == "include_map" || key == "all" {
			var value bool
			if err := json.Unmarshal(encoded, &value); err != nil {
				return developerAssetSelector{}, nil, fmt.Errorf("selector %s must be a boolean", originalKey)
			}
			if key == "include_map" {
				copy := value
				result.includeMap = &copy
			} else if value {
				result.selectAll = true
			} else {
				result.selectNone = true
			}
			continue
		}
		canonicalKey, ok := developerAssetSelectorAliases[key]
		if !ok || !allowed[canonicalKey] {
			return developerAssetSelector{}, nil, fmt.Errorf("unsupported %s selector field %q", domain, originalKey)
		}
		var values []string
		if err := json.Unmarshal(encoded, &values); err != nil {
			var single string
			if singleErr := json.Unmarshal(encoded, &single); singleErr != nil {
				return developerAssetSelector{}, nil, fmt.Errorf("selector %s must be a string or string array", originalKey)
			}
			values = []string{single}
		}
		if domain == developerAssetSDKSelector && (canonicalKey == "api_ids" || canonicalKey == "contract_revision_ids" || canonicalKey == "contract_operation_ids" || canonicalKey == "operation_keys" || canonicalKey == "capabilities" || canonicalKey == "evidence_ids") {
			result.scopeOnly[canonicalKey] = true
		} else {
			result.present[canonicalKey] = true
		}
		if result.values[canonicalKey] == nil {
			result.values[canonicalKey] = make(map[string]bool)
		}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" || len(value) > 1000 {
				return developerAssetSelector{}, nil, fmt.Errorf("selector %s contains an empty or overlong value", originalKey)
			}
			result.values[canonicalKey][value] = true
		}
	}
	if result.selectAll && (result.selectNone || len(result.present) != 0 || len(result.scopeOnly) != 0) {
		return developerAssetSelector{}, nil, errors.New("selector all=true cannot be combined with restrictive fields")
	}
	return result, canonical, nil
}

func (selector developerAssetSelector) restricted() bool {
	return selector.selectNone || len(selector.present) != 0
}

func developerAssetSelectorValueMatches(values map[string]bool, candidates ...string) bool {
	for _, candidate := range candidates {
		if values[strings.TrimSpace(candidate)] && strings.TrimSpace(candidate) != "" {
			return true
		}
	}
	return false
}

func (selector developerAssetSelector) matches(candidate developerAssetSelectorCandidate) bool {
	if selector.selectNone {
		return false
	}
	if candidate.isMap {
		if selector.includeMap != nil {
			return *selector.includeMap
		}
		return !selector.restricted()
	}
	if selector.includeMap != nil && len(selector.present) == 0 {
		// include_map is additive; by itself it does not remove normal units.
		return true
	}

	identifierFields := []string{"document_ids", "file_ids", "section_ids", "symbol_ids", "sample_ids"}
	identifierConstraint := false
	identifierMatch := false
	for _, field := range identifierFields {
		if !selector.present[field] {
			continue
		}
		identifierConstraint = true
		var values []string
		switch field {
		case "document_ids":
			values = []string{candidate.documentID}
		case "file_ids":
			values = []string{candidate.fileID}
		case "section_ids":
			values = []string{candidate.sectionID}
		case "symbol_ids":
			values = []string{candidate.symbolID}
		case "sample_ids":
			values = []string{candidate.sampleID}
		}
		if developerAssetSelectorValueMatches(selector.values[field], values...) {
			identifierMatch = true
		}
	}
	if identifierConstraint && !identifierMatch {
		return false
	}
	for field, candidates := range map[string][]string{
		"source_paths":  {candidate.sourcePath},
		"languages":     {candidate.language},
		"content_kinds": {candidate.contentKind, candidate.kind},
		"modules":       {candidate.module},
		"identifiers":   candidate.identifiers,
	} {
		if selector.present[field] && !developerAssetSelectorValueMatches(selector.values[field], candidates...) {
			return false
		}
	}
	return true
}

func developerAssetScopeKind(selector developerAssetSelector) string {
	if selector.restricted() || len(selector.scopeOnly) != 0 || selector.includeMap != nil {
		return "selected"
	}
	return "attached"
}

func developerAssetSectionDescendsFrom(section model.DocumentationSection, ancestorID string, byID map[string]model.DocumentationSection) bool {
	for parentID := section.ParentSectionID; parentID != ""; {
		if parentID == ancestorID {
			return true
		}
		parent, ok := byID[parentID]
		if !ok || parent.ParentSectionID == parentID {
			return false
		}
		parentID = parent.ParentSectionID
	}
	return false
}

func developerAssetFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
