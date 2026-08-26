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

type developerAssetDocumentationBuildOptions struct {
	assetOrdinal           int
	visibility             model.Visibility
	outerSelector          developerAssetSelector
	outerSelectorHash      string
	wrapperPublicationKind string
	wrapperPublicationID   string
	apiID                  string
	bindingID              string
	collectionIdentity     *model.DocumentationCollection
}

func documentationCollectionHistoricalIdentity(revision model.DocumentationCollectionRevision) (model.DocumentationCollection, error) {
	if !revision.HasHistoricalIdentity() {
		return model.DocumentationCollection{}, errors.New("documentation collection revision is missing its historical root identity")
	}
	return model.DocumentationCollection{
		ID: revision.DocumentationCollectionID, DeploymentID: revision.DeploymentID,
		Name: revision.DocumentationCollectionName, Slug: revision.DocumentationCollectionSlug,
		Description: revision.DocumentationCollectionDescription, Visibility: revision.Visibility,
	}, nil
}

func documentationPublicationHistoricalIdentity(asset model.APIPublicationDocumentationAsset, revision model.DocumentationCollectionRevision) (model.DocumentationCollection, error) {
	if !asset.MatchesRevisionIdentity(revision) {
		return model.DocumentationCollection{}, errors.New("API documentation asset root identity does not match its exact revision")
	}
	return model.DocumentationCollection{
		ID: asset.DocumentationCollectionID, DeploymentID: revision.DeploymentID,
		Name: asset.DocumentationCollectionName, Slug: asset.DocumentationCollectionSlug,
		Description: asset.DocumentationCollectionDescription, Visibility: asset.Visibility,
	}, nil
}

func (s *Service) buildGlobalDocumentationIndex(ctx context.Context, deploymentID string, publication model.DeploymentDocumentationPublication) ([]developerAssetIndexDraft, error) {
	if publication.DeploymentID != deploymentID || !publication.Visibility.Valid() || !validDeveloperAssetContentHash(publication.SnapshotHash) {
		return nil, errors.New("global documentation publication is invalid")
	}
	members := append([]model.DeploymentDocumentationPublicationMember(nil), publication.Members...)
	sort.Slice(members, func(i, j int) bool {
		if members[i].Ordinal == members[j].Ordinal {
			return members[i].DocumentationCollectionRevisionID < members[j].DocumentationCollectionRevisionID
		}
		return members[i].Ordinal < members[j].Ordinal
	})
	seenOrdinals := make(map[int]bool, len(members))
	result := make([]developerAssetIndexDraft, 0)
	for _, member := range members {
		if seenOrdinals[member.Ordinal] || !member.Visibility.Valid() || !validDeveloperAssetContentHash(member.ContentHash) {
			return nil, errors.New("global documentation publication has invalid members")
		}
		seenOrdinals[member.Ordinal] = true
		record, err := s.store.DocumentationCollectionRevision(ctx, deploymentID, member.DocumentationCollectionRevisionID)
		if err != nil {
			return nil, err
		}
		if record.Revision.ContentHash != member.ContentHash || record.Revision.Visibility != member.Visibility {
			return nil, errors.New("global documentation member no longer matches its exact revision")
		}
		if publication.Visibility == model.VisibilityPublic && member.Visibility != model.VisibilityPublic {
			return nil, errors.New("public global documentation contains a private revision")
		}
		visibility, err := developerAssetVisibility(publication.Visibility, member.Visibility, record.Revision.Visibility)
		if err != nil {
			return nil, err
		}
		drafts, err := s.buildDocumentationRevisionIndex(ctx, deploymentID, record, developerAssetDocumentationBuildOptions{
			assetOrdinal: member.Ordinal, visibility: visibility,
			outerSelector:          developerAssetSelector{values: map[string]map[string]bool{}, present: map[string]bool{}},
			wrapperPublicationKind: "global_documentation", wrapperPublicationID: publication.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("documentation revision %s: %w", record.Revision.ID, err)
		}
		result = append(result, drafts...)
	}
	return result, nil
}

func (s *Service) buildDocumentationRevisionIndex(ctx context.Context, deploymentID string, record store.DocumentationCollectionRevisionRecord, options developerAssetDocumentationBuildOptions) ([]developerAssetIndexDraft, error) {
	revision := record.Revision
	if revision.DeploymentID != deploymentID || !revision.Visibility.Valid() || !validDeveloperAssetContentHash(revision.ContentHash) {
		return nil, errors.New("documentation collection revision is invalid")
	}
	collection, err := documentationCollectionHistoricalIdentity(revision)
	if err != nil {
		return nil, err
	}
	if options.collectionIdentity != nil {
		collection = *options.collectionIdentity
	}
	result := make([]developerAssetIndexDraft, 0)
	if record.Map == nil || record.Map.DocumentationCollectionRevisionID != revision.ID || record.Map.DeploymentID != deploymentID || !validDeveloperAssetContentHash(record.Map.ContentHash) {
		return nil, errors.New("published documentation revision is missing its exact Documentation Map")
	}

	members := append([]model.DocumentationCollectionMember(nil), record.Members...)
	sort.Slice(members, func(i, j int) bool {
		if members[i].Ordinal == members[j].Ordinal {
			return members[i].ID < members[j].ID
		}
		return members[i].Ordinal < members[j].Ordinal
	})
	seenOrdinals := make(map[int]bool, len(members))
	for _, member := range members {
		if member.DocumentationCollectionRevisionID != revision.ID || seenOrdinals[member.Ordinal] {
			return nil, errors.New("documentation collection has invalid member ordinals")
		}
		seenOrdinals[member.Ordinal] = true
		memberSelector, _, err := parseDeveloperAssetSelector(member.Selector, developerAssetDocumentationSelector)
		if err != nil {
			return nil, fmt.Errorf("member %s: %w", member.ID, err)
		}
		switch member.Kind {
		case "source_publication":
			publication, err := s.store.SourcePublication(ctx, deploymentID, member.SourcePublicationID)
			if err != nil {
				return nil, err
			}
			review, err := s.store.SourcePublicationDocumentationReview(ctx, deploymentID, publication.ID)
			if err != nil {
				return nil, err
			}
			selections := append([]model.SourcePublicationDocumentSelection(nil), review.Selections...)
			sort.Slice(selections, func(i, j int) bool {
				left, right := int(^uint(0)>>1), int(^uint(0)>>1)
				if selections[i].Ordinal != nil {
					left = *selections[i].Ordinal
				}
				if selections[j].Ordinal != nil {
					right = *selections[j].Ordinal
				}
				if left == right {
					return selections[i].DocumentationDocumentID < selections[j].DocumentationDocumentID
				}
				return left < right
			})
			for selectionOrdinal, selection := range selections {
				if selection.SourcePublicationID != publication.ID || selection.DeploymentID != deploymentID {
					return nil, errors.New("source publication review is inconsistent")
				}
				switch selection.Decision {
				case "included":
					if selection.Ordinal == nil || strings.TrimSpace(selection.Reason) != "" {
						return nil, errors.New("included source publication document selection is invalid")
					}
				case "excluded", "quarantined":
					if selection.Ordinal != nil || strings.TrimSpace(selection.Reason) == "" {
						return nil, errors.New("excluded source publication document selection is invalid")
					}
					continue
				default:
					return nil, errors.New("source publication review decision is invalid")
				}
				candidate, err := s.store.DocumentationCandidateDocument(ctx, deploymentID, selection.DocumentationDocumentID)
				if err != nil {
					return nil, err
				}
				if candidate.Document.ContentHash != selection.ContentHash || !validDeveloperAssetContentHash(selection.ContentHash) {
					return nil, errors.New("included source publication document hash does not match")
				}
				if options.visibility == model.VisibilityPublic && (publication.Visibility != model.VisibilityPublic || candidate.Document.Visibility != model.VisibilityPublic) {
					return nil, errors.New("public documentation publication contains private source evidence")
				}
				visibility, err := developerAssetVisibility(options.visibility, publication.Visibility, candidate.Document.Visibility)
				if err != nil {
					return nil, err
				}
				drafts, err := buildDocumentationCandidateIndex(candidate, revision, collection, member, memberSelector, options, visibility, selectionOrdinal, "source_publication", publication.ID)
				if err != nil {
					return nil, err
				}
				result = append(result, drafts...)
			}
		case "document":
			candidate, err := s.store.DocumentationCandidateDocument(ctx, deploymentID, member.DocumentationDocumentID)
			if err != nil {
				return nil, err
			}
			if options.visibility == model.VisibilityPublic && candidate.Document.Visibility != model.VisibilityPublic {
				return nil, errors.New("public documentation publication contains a private document")
			}
			visibility, err := developerAssetVisibility(options.visibility, candidate.Document.Visibility)
			if err != nil {
				return nil, err
			}
			drafts, err := buildDocumentationCandidateIndex(candidate, revision, collection, member, memberSelector, options, visibility, candidate.Document.Ordinal, "document", "")
			if err != nil {
				return nil, err
			}
			result = append(result, drafts...)
		case "section":
			section, candidate, err := s.store.DocumentationCandidateSection(ctx, deploymentID, member.DocumentationSectionID)
			if err != nil {
				return nil, err
			}
			if options.visibility == model.VisibilityPublic && candidate.Document.Visibility != model.VisibilityPublic {
				return nil, errors.New("public documentation publication contains a private section parent")
			}
			visibility, err := developerAssetVisibility(options.visibility, candidate.Document.Visibility)
			if err != nil {
				return nil, err
			}
			drafts, err := buildDocumentationSectionMemberIndex(candidate, section, revision, collection, member, memberSelector, options, visibility)
			if err != nil {
				return nil, err
			}
			result = append(result, drafts...)
		default:
			return nil, fmt.Errorf("unsupported documentation member kind %q", member.Kind)
		}
	}
	if options.outerSelector.matches(developerAssetSelectorCandidate{kind: "map", contentKind: "map", isMap: true}) {
		mapDraft, err := newDocumentationMapDraft(record, revision, collection, options, result)
		if err != nil {
			return nil, err
		}
		result = append(result, mapDraft)
	}
	return result, nil
}

func newDocumentationMapDraft(
	record store.DocumentationCollectionRevisionRecord,
	revision model.DocumentationCollectionRevision,
	collection model.DocumentationCollection,
	options developerAssetDocumentationBuildOptions,
	selectedDrafts []developerAssetIndexDraft,
) (developerAssetIndexDraft, error) {
	if options.visibility == model.VisibilityPublic && record.Map.Visibility != model.VisibilityPublic {
		return developerAssetIndexDraft{}, errors.New("public documentation publication contains a private Documentation Map")
	}
	mapBody := record.Map.Map
	agentMarkdown := record.Map.AgentMarkdown
	mapEntityID := record.Map.ID
	mapContentHash := record.Map.ContentHash
	if options.outerSelector.restricted() {
		selectedDocumentIDs := make(map[string]bool)
		selectedSectionIDs := make(map[string]bool)
		for _, draft := range selectedDrafts {
			switch draft.unit.Kind {
			case "document":
				selectedDocumentIDs[draft.unit.SourceEntityID] = true
			case "section":
				selectedSectionIDs[draft.unit.SourceEntityID] = true
			}
		}
		mapBody = scopeDocumentationMapBody(mapBody, documentationMapScope{
			documentIDs: selectedDocumentIDs, categoryDocumentIDs: selectedDocumentIDs,
			sectionIDs: selectedSectionIDs, knownIDs: documentationKnownIDs(nil, record.Map.Map),
		})
		// The stored agent Markdown and overview describe the entire collection.
		// A selected API binding gets a fresh immutable projection so those broad
		// routing summaries cannot reintroduce evidence excluded by its selector.
		mapBody.Overview = collection.Description
		agentMarkdown = "# " + documentationMapLine(collection.Name)
		if description := documentationMapLine(collection.Description); description != "" {
			agentMarkdown += "\n\n" + description
		}
	}
	content, err := developerAssetMapContent(agentMarkdown, mapBody)
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	if options.outerSelector.restricted() {
		mapContentHash = contentHash([]byte(content))
		mapEntityID = deterministicDeveloperAssetUUID(strings.Join([]string{
			record.Map.ID, options.outerSelectorHash, mapContentHash,
		}, "\x00"))
	}
	visibility, err := developerAssetVisibility(options.visibility, revision.Visibility, record.Map.Visibility)
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	citation, err := marshalDeveloperAssetObject(map[string]any{
		"publication_kind": "documentation_collection", "publication_id": revision.ID,
		"index_publication_kind": options.wrapperPublicationKind, "index_publication_id": options.wrapperPublicationID,
		"api_documentation_binding_id":         options.bindingID,
		"documentation_collection_revision_id": revision.ID, "documentation_map_id": record.Map.ID,
		"documentation_map_content_hash": record.Map.ContentHash, "map_version": record.Map.MapVersion,
		"selector_hash": options.outerSelectorHash, "content_hash": mapContentHash,
	})
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	metadata, err := marshalDeveloperAssetObject(map[string]any{
		"asset_kind": "documentation", "documentation_collection_id": collection.ID,
		"documentation_collection_name": collection.Name, "documentation_collection_slug": collection.Slug,
		"documentation_collection_description": collection.Description,
		"api_documentation_binding_id":         options.bindingID,
		"documentation_collection_revision_id": revision.ID, "collection_revision": revision.Revision,
		"documentation_map_id": record.Map.ID, "documentation_map_content_hash": record.Map.ContentHash,
		"map_version": record.Map.MapVersion, "selector_hash": options.outerSelectorHash,
	})
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	draft := developerAssetIndexDraft{unit: model.KnowledgeUnit{
		Kind: "map", SourcePublicationKind: "documentation_collection", SourcePublicationID: revision.ID,
		SourceEntityID: mapEntityID, Title: collection.Name + " documentation map",
		Breadcrumb: []string{collection.Name}, Content: content, Identifiers: []string{mapEntityID, record.Map.ID, collection.ID, collection.Slug},
		Visibility: visibility, Citation: citation, Metadata: metadata, ContentHash: mapContentHash,
	}, order: developerAssetIndexOrder{assetOrdinal: options.assetOrdinal, memberOrdinal: -1, kindRank: 0, tieBreaker: mapEntityID}}
	draft.scope = developerAssetDocumentationScope(options)
	return draft, nil
}

func developerAssetDocumentationScope(options developerAssetDocumentationBuildOptions) *model.KnowledgeUnitAPIScope {
	if options.apiID == "" {
		return nil
	}
	return &model.KnowledgeUnitAPIScope{APIID: options.apiID, ScopeKind: developerAssetScopeKind(options.outerSelector), SelectorHash: options.outerSelectorHash}
}

func buildDocumentationCandidateIndex(candidate store.DocumentationCandidateRecord, revision model.DocumentationCollectionRevision, collection model.DocumentationCollection, member model.DocumentationCollectionMember, memberSelector developerAssetSelector, options developerAssetDocumentationBuildOptions, visibility model.Visibility, entityOrdinal int, memberKind, sourcePublicationID string) ([]developerAssetIndexDraft, error) {
	result := make([]developerAssetIndexDraft, 0, len(candidate.Sections)+1)
	document := candidate.Document
	documentSelector := documentationDocumentSelectorCandidate(document)
	if memberSelector.matches(documentSelector) && options.outerSelector.matches(documentSelector) {
		draft, err := newDocumentationDocumentDraft(document, revision, collection, member, options, visibility, entityOrdinal, memberKind, sourcePublicationID)
		if err != nil {
			return nil, err
		}
		result = append(result, draft)
	}
	if !member.IncludeDescendants {
		return result, nil
	}
	sections := append([]model.DocumentationSection(nil), candidate.Sections...)
	sort.Slice(sections, func(i, j int) bool {
		if sections[i].Ordinal == sections[j].Ordinal {
			return sections[i].ID < sections[j].ID
		}
		return sections[i].Ordinal < sections[j].Ordinal
	})
	for _, section := range sections {
		sectionSelector := documentationSectionSelectorCandidate(document, section)
		if !memberSelector.matches(sectionSelector) || !options.outerSelector.matches(sectionSelector) {
			continue
		}
		draft, err := newDocumentationSectionDraft(document, section, revision, collection, member, options, visibility, memberKind, sourcePublicationID)
		if err != nil {
			return nil, err
		}
		result = append(result, draft)
	}
	return result, nil
}

func buildDocumentationSectionMemberIndex(candidate store.DocumentationCandidateRecord, selected model.DocumentationSection, revision model.DocumentationCollectionRevision, collection model.DocumentationCollection, member model.DocumentationCollectionMember, memberSelector developerAssetSelector, options developerAssetDocumentationBuildOptions, visibility model.Visibility) ([]developerAssetIndexDraft, error) {
	sectionsByID := make(map[string]model.DocumentationSection, len(candidate.Sections))
	sections := append([]model.DocumentationSection(nil), candidate.Sections...)
	for _, section := range sections {
		sectionsByID[section.ID] = section
	}
	sort.Slice(sections, func(i, j int) bool {
		if sections[i].Ordinal == sections[j].Ordinal {
			return sections[i].ID < sections[j].ID
		}
		return sections[i].Ordinal < sections[j].Ordinal
	})
	result := make([]developerAssetIndexDraft, 0)
	for _, section := range sections {
		if section.ID != selected.ID && (!member.IncludeDescendants || !developerAssetSectionDescendsFrom(section, selected.ID, sectionsByID)) {
			continue
		}
		candidateSelector := documentationSectionSelectorCandidate(candidate.Document, section)
		if !memberSelector.matches(candidateSelector) || !options.outerSelector.matches(candidateSelector) {
			continue
		}
		draft, err := newDocumentationSectionDraft(candidate.Document, section, revision, collection, member, options, visibility, "section", "")
		if err != nil {
			return nil, err
		}
		result = append(result, draft)
	}
	return result, nil
}

func documentationDocumentSelectorCandidate(document model.DocumentationDocument) developerAssetSelectorCandidate {
	return developerAssetSelectorCandidate{
		kind: "document", documentID: document.ID, sourcePath: document.SourcePath,
		language: document.Language, contentKind: document.Kind,
		identifiers: []string{document.ID, document.SourcePath, document.CanonicalURL, document.Title},
	}
}

func documentationSectionSelectorCandidate(document model.DocumentationDocument, section model.DocumentationSection) developerAssetSelectorCandidate {
	return developerAssetSelectorCandidate{
		kind: "section", documentID: document.ID, sectionID: section.ID, sourcePath: document.SourcePath,
		language: developerAssetFirstNonEmpty(section.CodeLanguage, document.Language), contentKind: section.ContentKind,
		identifiers: append([]string{section.ID, section.Anchor, section.Heading, document.SourcePath}, section.Breadcrumb...),
	}
}

func newDocumentationDocumentDraft(document model.DocumentationDocument, revision model.DocumentationCollectionRevision, collection model.DocumentationCollection, member model.DocumentationCollectionMember, options developerAssetDocumentationBuildOptions, visibility model.Visibility, entityOrdinal int, memberKind, sourcePublicationID string) (developerAssetIndexDraft, error) {
	sourceMetadata, err := sourceMetadataValue(document.Metadata)
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	citation, err := marshalDeveloperAssetObject(map[string]any{
		"publication_kind": "documentation_collection", "publication_id": revision.ID,
		"index_publication_kind": options.wrapperPublicationKind, "index_publication_id": options.wrapperPublicationID,
		"api_documentation_binding_id":         options.bindingID,
		"documentation_collection_revision_id": revision.ID, "documentation_document_id": document.ID,
		"documentation_source_publication_id": sourcePublicationID, "source_path": document.SourcePath,
		"canonical_url": document.CanonicalURL, "content_hash": document.ContentHash,
	})
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	metadata, err := marshalDeveloperAssetObject(map[string]any{
		"asset_kind": "documentation", "documentation_collection_id": collection.ID,
		"documentation_collection_name": collection.Name, "documentation_collection_slug": collection.Slug,
		"documentation_collection_description": collection.Description,
		"api_documentation_binding_id":         options.bindingID,
		"documentation_collection_revision_id": revision.ID, "collection_revision": revision.Revision,
		"documentation_member_id": member.ID, "documentation_member_kind": memberKind,
		"document_kind": document.Kind, "media_type": document.MediaType,
		"ingestion_run_id": document.IngestionRunID, "selector_hash": options.outerSelectorHash,
		"source_metadata": sourceMetadata,
	})
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	draft := developerAssetIndexDraft{unit: model.KnowledgeUnit{
		Kind: "document", SourcePublicationKind: "documentation_collection", SourcePublicationID: revision.ID,
		SourceEntityID: document.ID, Title: document.Title, Breadcrumb: []string{collection.Name, document.Title},
		Content: developerAssetFirstNonEmpty(strings.TrimSpace(document.NormalizedMarkdown), document.Title), Language: document.Language,
		Identifiers: []string{document.ID, document.SourcePath, document.CanonicalURL, document.Title},
		Visibility:  visibility, Citation: citation, Metadata: metadata, ContentHash: document.ContentHash,
	}, order: developerAssetIndexOrder{assetOrdinal: options.assetOrdinal, memberOrdinal: member.Ordinal, entityOrdinal: entityOrdinal, kindRank: 1, tieBreaker: document.ID}}
	draft.scope = developerAssetDocumentationScope(options)
	return draft, nil
}

func newDocumentationSectionDraft(document model.DocumentationDocument, section model.DocumentationSection, revision model.DocumentationCollectionRevision, collection model.DocumentationCollection, member model.DocumentationCollectionMember, options developerAssetDocumentationBuildOptions, visibility model.Visibility, memberKind, sourcePublicationID string) (developerAssetIndexDraft, error) {
	sourceMetadata, err := sourceMetadataValue(section.Metadata)
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	citation, err := marshalDeveloperAssetObject(map[string]any{
		"publication_kind": "documentation_collection", "publication_id": revision.ID,
		"index_publication_kind": options.wrapperPublicationKind, "index_publication_id": options.wrapperPublicationID,
		"api_documentation_binding_id":         options.bindingID,
		"documentation_collection_revision_id": revision.ID, "documentation_document_id": document.ID,
		"documentation_section_id": section.ID, "documentation_source_publication_id": sourcePublicationID,
		"source_path": document.SourcePath, "canonical_url": document.CanonicalURL, "anchor": section.Anchor,
		"source_start": developerAssetInteger(section.SourceStart), "source_end": developerAssetInteger(section.SourceEnd),
		"content_hash": section.ContentHash,
	})
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	metadata, err := marshalDeveloperAssetObject(map[string]any{
		"asset_kind": "documentation", "documentation_collection_id": collection.ID,
		"documentation_collection_name": collection.Name, "documentation_collection_slug": collection.Slug,
		"documentation_collection_description": collection.Description,
		"api_documentation_binding_id":         options.bindingID,
		"documentation_collection_revision_id": revision.ID, "collection_revision": revision.Revision,
		"documentation_member_id": member.ID, "documentation_member_kind": memberKind,
		"content_kind": section.ContentKind, "token_estimate": section.TokenEstimate,
		"ingestion_run_id": document.IngestionRunID, "selector_hash": options.outerSelectorHash,
		"source_metadata": sourceMetadata,
	})
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	breadcrumb := append([]string(nil), section.Breadcrumb...)
	if len(breadcrumb) == 0 || breadcrumb[0] != document.Title {
		breadcrumb = append([]string{document.Title}, breadcrumb...)
	}
	breadcrumb = append([]string{collection.Name}, breadcrumb...)
	title := developerAssetFirstNonEmpty(section.Heading, document.Title)
	draft := developerAssetIndexDraft{unit: model.KnowledgeUnit{
		Kind: "section", SourcePublicationKind: "documentation_collection", SourcePublicationID: revision.ID,
		SourceEntityID: section.ID, ParentSourceEntityID: developerAssetFirstNonEmpty(section.ParentSectionID, document.ID),
		Title: title, Breadcrumb: breadcrumb, Content: developerAssetFirstNonEmpty(strings.TrimSpace(section.NormalizedText), title),
		Language:    developerAssetFirstNonEmpty(section.CodeLanguage, document.Language),
		Identifiers: append([]string{section.ID, section.Anchor, section.Heading, document.SourcePath}, section.Breadcrumb...),
		Visibility:  visibility, Citation: citation, Metadata: metadata, ContentHash: section.ContentHash,
	}, order: developerAssetIndexOrder{assetOrdinal: options.assetOrdinal, memberOrdinal: member.Ordinal, entityOrdinal: section.Ordinal, kindRank: 2, tieBreaker: section.ID}}
	draft.scope = developerAssetDocumentationScope(options)
	return draft, nil
}

func developerAssetFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Service) apiDeveloperAssetPublicationVisibility(
	ctx context.Context,
	publication model.APIDeveloperAssetPublication,
) (model.Visibility, error) {
	revisions, err := s.store.IntegrationRevisions(ctx, publication.APIID)
	if err != nil {
		return "", err
	}
	for _, revision := range revisions {
		if revision.ID != publication.APIRevisionID {
			continue
		}
		if revision.IntegrationID != publication.APIID || revision.State != "published" || revision.PublishedAt == nil {
			return "", errors.New("API developer-asset publication does not reference an exact published API revision")
		}
		var snapshot struct {
			Visibility model.Visibility `json:"visibility"`
		}
		if err := json.Unmarshal(revision.Snapshot, &snapshot); err != nil {
			return "", fmt.Errorf("API revision snapshot is invalid: %w", err)
		}
		// Pre-developer-asset snapshots did not always carry visibility. They
		// are conservatively private rather than inheriting mutable API state.
		if snapshot.Visibility == "" {
			snapshot.Visibility = model.VisibilityPrivate
		}
		if !snapshot.Visibility.Valid() {
			return "", errors.New("API revision snapshot has invalid visibility")
		}
		return snapshot.Visibility, nil
	}
	return "", errors.New("API developer-asset publication references an unknown API revision")
}

func (s *Service) buildAPIDeveloperAssetIndex(ctx context.Context, deploymentID string, publication model.APIDeveloperAssetPublication) ([]developerAssetIndexDraft, error) {
	if publication.DeploymentID != deploymentID || publication.APIID == "" || !validDeveloperAssetContentHash(publication.SnapshotHash) {
		return nil, errors.New("API developer-asset publication is invalid")
	}
	apiVisibility, err := s.apiDeveloperAssetPublicationVisibility(ctx, publication)
	if err != nil {
		return nil, err
	}
	result := make([]developerAssetIndexDraft, 0)

	documentation := append([]model.APIPublicationDocumentationAsset(nil), publication.Documentation...)
	sort.Slice(documentation, func(i, j int) bool {
		if documentation[i].Ordinal == documentation[j].Ordinal {
			return documentation[i].BindingID < documentation[j].BindingID
		}
		return documentation[i].Ordinal < documentation[j].Ordinal
	})
	seenOrdinals := make(map[int]bool, len(documentation))
	for _, asset := range documentation {
		if seenOrdinals[asset.Ordinal] {
			return nil, errors.New("API documentation assets have duplicate ordinals")
		}
		seenOrdinals[asset.Ordinal] = true
		selector, _, err := parseDeveloperAssetSelector(asset.Selector, developerAssetDocumentationSelector)
		if err != nil {
			return nil, fmt.Errorf("API documentation binding %s: %w", asset.BindingID, err)
		}
		// Documentation selector hashes are copied from the immutable API
		// publication. PostgreSQL owns their jsonb-text canonicalization, which
		// intentionally differs from encoding/json for non-empty objects.
		if !validDeveloperAssetContentHash(asset.SelectorHash) {
			return nil, errors.New("API documentation selector hash is invalid")
		}
		record, err := s.store.DocumentationCollectionRevision(ctx, deploymentID, asset.DocumentationCollectionRevisionID)
		if err != nil {
			return nil, err
		}
		if record.Revision.ContentHash != asset.ContentHash || record.Revision.Visibility == model.VisibilityPrivate && asset.Visibility == model.VisibilityPublic {
			return nil, errors.New("API documentation asset does not match its exact revision")
		}
		collectionIdentity, err := documentationPublicationHistoricalIdentity(asset, record.Revision)
		if err != nil {
			return nil, err
		}
		if apiVisibility == model.VisibilityPublic && (asset.Visibility != model.VisibilityPublic || record.Revision.Visibility != model.VisibilityPublic) {
			return nil, errors.New("public API publication contains private documentation")
		}
		visibility, err := developerAssetVisibility(apiVisibility, asset.Visibility, record.Revision.Visibility)
		if err != nil {
			return nil, err
		}
		drafts, err := s.buildDocumentationRevisionIndex(ctx, deploymentID, record, developerAssetDocumentationBuildOptions{
			assetOrdinal: asset.Ordinal, visibility: visibility, outerSelector: selector,
			outerSelectorHash: asset.SelectorHash, wrapperPublicationKind: "api", wrapperPublicationID: publication.ID,
			apiID: publication.APIID, bindingID: asset.BindingID, collectionIdentity: &collectionIdentity,
		})
		if err != nil {
			return nil, fmt.Errorf("API documentation binding %s: %w", asset.BindingID, err)
		}
		result = append(result, drafts...)
	}

	contracts := append([]model.APIPublicationContractAsset(nil), publication.Contracts...)
	sort.Slice(contracts, func(i, j int) bool {
		if contracts[i].Ordinal == contracts[j].Ordinal {
			return contracts[i].BindingID < contracts[j].BindingID
		}
		return contracts[i].Ordinal < contracts[j].Ordinal
	})
	seenOrdinals = make(map[int]bool, len(contracts))
	for _, asset := range contracts {
		if seenOrdinals[asset.Ordinal] {
			return nil, errors.New("API contract assets have duplicate ordinals")
		}
		seenOrdinals[asset.Ordinal] = true
		drafts, err := s.buildAPIContractAssetIndex(ctx, deploymentID, publication, apiVisibility, asset, 1_000_000+asset.Ordinal)
		if err != nil {
			return nil, fmt.Errorf("API contract binding %s: %w", asset.BindingID, err)
		}
		result = append(result, drafts...)
	}

	sdks := append([]model.APIPublicationSDKAsset(nil), publication.SDKs...)
	sort.Slice(sdks, func(i, j int) bool {
		if sdks[i].Ordinal == sdks[j].Ordinal {
			return sdks[i].BindingID < sdks[j].BindingID
		}
		return sdks[i].Ordinal < sdks[j].Ordinal
	})
	seenOrdinals = make(map[int]bool, len(sdks))
	for _, asset := range sdks {
		if seenOrdinals[asset.Ordinal] {
			return nil, errors.New("API SDK assets have duplicate ordinals")
		}
		seenOrdinals[asset.Ordinal] = true
		drafts, err := s.buildAPISDKAssetIndex(ctx, deploymentID, publication, apiVisibility, asset, 2_000_000+asset.Ordinal)
		if err != nil {
			return nil, fmt.Errorf("API SDK binding %s: %w", asset.BindingID, err)
		}
		result = append(result, drafts...)
	}
	return result, nil
}

func (s *Service) buildAPIContractAssetIndex(ctx context.Context, deploymentID string, apiPublication model.APIDeveloperAssetPublication, apiVisibility model.Visibility, asset model.APIPublicationContractAsset, assetOrdinal int) ([]developerAssetIndexDraft, error) {
	revision, err := s.store.APIContractRevision(ctx, deploymentID, asset.APIContractRevisionID)
	if err != nil {
		return nil, err
	}
	if revision.ContentHash != asset.ContentHash || !validDeveloperAssetContentHash(revision.ContentHash) {
		return nil, errors.New("contract revision hash does not match the API snapshot")
	}
	record, err := s.store.APIContractCandidate(ctx, deploymentID, revision.APIContractCandidateID)
	if err != nil {
		return nil, err
	}
	if record.Candidate.APIContractID != revision.APIContractID || record.Candidate.ContentHash != revision.ContentHash {
		return nil, errors.New("contract revision does not resolve to its exact candidate")
	}
	if !asset.MatchesRevisionIdentity(revision) {
		return nil, errors.New("API contract asset root identity does not match its exact revision")
	}
	contract := model.APIContract{
		ID: asset.APIContractID, DeploymentID: revision.DeploymentID, Name: asset.APIContractName,
		Slug: asset.APIContractSlug, Description: asset.APIContractDescription, Kind: asset.APIContractKind,
		Visibility: asset.Visibility,
	}
	if apiVisibility == model.VisibilityPublic && (asset.Visibility != model.VisibilityPublic || revision.Visibility != model.VisibilityPublic || record.Candidate.Visibility != model.VisibilityPublic) {
		return nil, errors.New("public API publication contains a private contract")
	}
	visibility, err := developerAssetVisibility(apiVisibility, asset.Visibility, revision.Visibility, record.Candidate.Visibility)
	if err != nil {
		return nil, err
	}
	baseMetadata := map[string]any{
		"asset_kind": "contract", "api_developer_asset_publication_id": apiPublication.ID,
		"api_contract_id": revision.APIContractID, "api_contract_revision_id": revision.ID,
		"api_contract_name": contract.Name, "api_contract_slug": contract.Slug,
		"api_contract_description":  contract.Description,
		"api_contract_candidate_id": record.Candidate.ID, "contract_revision": revision.Revision,
		"contract_kind": contract.Kind, "primary": asset.Primary, "api_contract_binding_id": asset.BindingID,
	}
	result := make([]developerAssetIndexDraft, 0, len(record.Operations)+len(record.Schemas)+len(record.Examples)+1)
	scope := &model.KnowledgeUnitAPIScope{APIID: apiPublication.APIID, ScopeKind: "attached"}
	if record.Map == nil || record.Map.APIContractCandidateID != record.Candidate.ID || !validDeveloperAssetContentHash(record.Map.ContentHash) {
		return nil, errors.New("published contract is missing its exact Contract Map")
	}
	mapContent, err := developerAssetMapContent(record.Map.AgentMarkdown, record.Map.Map)
	if err != nil {
		return nil, err
	}
	mapCitation, err := marshalDeveloperAssetObject(map[string]any{
		"publication_kind": "contract", "publication_id": revision.ID,
		"index_publication_kind": "api", "index_publication_id": apiPublication.ID,
		"api_contract_revision_id": revision.ID, "api_contract_candidate_id": record.Candidate.ID,
		"api_contract_map_id": record.Map.ID, "map_version": record.Map.MapVersion, "content_hash": record.Map.ContentHash,
	})
	if err != nil {
		return nil, err
	}
	mapMetadata := cloneDeveloperAssetMetadata(baseMetadata)
	mapMetadata["map_version"] = record.Map.MapVersion
	mapEncoded, err := marshalDeveloperAssetObject(mapMetadata)
	if err != nil {
		return nil, err
	}
	result = append(result, developerAssetIndexDraft{unit: model.KnowledgeUnit{
		Kind: "map", SourcePublicationKind: "contract", SourcePublicationID: revision.ID,
		SourceEntityID: record.Map.ID, Title: contract.Name + " contract map", Breadcrumb: []string{contract.Name},
		Content: mapContent, Identifiers: []string{record.Map.ID, contract.ID, contract.Slug}, Visibility: visibility,
		Citation: mapCitation, Metadata: mapEncoded, ContentHash: record.Map.ContentHash,
	}, order: developerAssetIndexOrder{assetOrdinal: assetOrdinal, memberOrdinal: -1, kindRank: 0, tieBreaker: record.Map.ID}, scope: scope})

	operations := append([]model.APIContractOperation(nil), record.Operations...)
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].Ordinal == operations[j].Ordinal {
			return operations[i].ID < operations[j].ID
		}
		return operations[i].Ordinal < operations[j].Ordinal
	})
	operationIDs := make(map[string]bool, len(operations))
	for _, operation := range operations {
		if operation.APIContractCandidateID != record.Candidate.ID || operationIDs[operation.ID] || !validDeveloperAssetContentHash(operation.ContentHash) {
			return nil, errors.New("contract operation is inconsistent")
		}
		operationIDs[operation.ID] = true
		content, err := developerAssetContractOperationContent(operation)
		if err != nil {
			return nil, err
		}
		citation, err := marshalDeveloperAssetObject(map[string]any{
			"publication_kind": "contract", "publication_id": revision.ID,
			"index_publication_kind": "api", "index_publication_id": apiPublication.ID,
			"api_contract_revision_id": revision.ID, "api_contract_candidate_id": record.Candidate.ID,
			"api_contract_operation_id": operation.ID, "operation_key": operation.OperationKey,
			"method": operation.Method, "path_template": operation.PathTemplate, "content_hash": operation.ContentHash,
		})
		if err != nil {
			return nil, err
		}
		metadata := cloneDeveloperAssetMetadata(baseMetadata)
		metadata["operation_key"], metadata["operation_id"] = operation.OperationKey, operation.OperationID
		metadata["method"], metadata["path_template"], metadata["tags"] = operation.Method, operation.PathTemplate, operation.Tags
		encoded, err := marshalDeveloperAssetObject(metadata)
		if err != nil {
			return nil, err
		}
		title := developerAssetFirstNonEmpty(operation.Summary, operation.OperationID, operation.OperationKey, strings.ToUpper(operation.Method)+" "+operation.PathTemplate)
		result = append(result, developerAssetIndexDraft{unit: model.KnowledgeUnit{
			Kind: "contract_operation", SourcePublicationKind: "contract", SourcePublicationID: revision.ID,
			SourceEntityID: operation.ID, Title: title, Breadcrumb: []string{contract.Name, strings.ToUpper(operation.Method) + " " + operation.PathTemplate},
			Content: content, Identifiers: append([]string{operation.ID, operation.OperationKey, operation.OperationID, operation.PathTemplate, strings.ToUpper(operation.Method) + " " + operation.PathTemplate}, operation.Tags...),
			Visibility: visibility, Citation: citation, Metadata: encoded, ContentHash: operation.ContentHash,
		}, order: developerAssetIndexOrder{assetOrdinal: assetOrdinal, entityOrdinal: operation.Ordinal, kindRank: 1, tieBreaker: operation.ID}, scope: scope})
	}

	schemas := append([]model.APIContractSchema(nil), record.Schemas...)
	sort.Slice(schemas, func(i, j int) bool {
		if schemas[i].SchemaKey == schemas[j].SchemaKey {
			return schemas[i].ID < schemas[j].ID
		}
		return schemas[i].SchemaKey < schemas[j].SchemaKey
	})
	for index, schema := range schemas {
		if schema.APIContractCandidateID != record.Candidate.ID || !validDeveloperAssetContentHash(schema.ContentHash) {
			return nil, errors.New("contract schema is inconsistent")
		}
		content, err := compactDeveloperAssetJSON(schema.Schema)
		if err != nil {
			return nil, err
		}
		citation, err := marshalDeveloperAssetObject(map[string]any{
			"publication_kind": "contract", "publication_id": revision.ID,
			"index_publication_kind": "api", "index_publication_id": apiPublication.ID,
			"api_contract_revision_id": revision.ID, "api_contract_candidate_id": record.Candidate.ID,
			"api_contract_schema_id": schema.ID, "schema_key": schema.SchemaKey, "content_hash": schema.ContentHash,
		})
		if err != nil {
			return nil, err
		}
		metadata := cloneDeveloperAssetMetadata(baseMetadata)
		metadata["schema_key"] = schema.SchemaKey
		encoded, err := marshalDeveloperAssetObject(metadata)
		if err != nil {
			return nil, err
		}
		result = append(result, developerAssetIndexDraft{unit: model.KnowledgeUnit{
			Kind: "contract_schema", SourcePublicationKind: "contract", SourcePublicationID: revision.ID,
			SourceEntityID: schema.ID, Title: schema.SchemaKey, Breadcrumb: []string{contract.Name, "Schemas", schema.SchemaKey},
			Content: content, Identifiers: []string{schema.ID, schema.SchemaKey}, Visibility: visibility,
			Citation: citation, Metadata: encoded, ContentHash: schema.ContentHash,
		}, order: developerAssetIndexOrder{assetOrdinal: assetOrdinal, entityOrdinal: index, kindRank: 2, tieBreaker: schema.ID}, scope: scope})
	}

	examples := append([]model.APIContractExample(nil), record.Examples...)
	sort.Slice(examples, func(i, j int) bool {
		left := examples[i].APIContractOperationID + "\x00" + examples[i].Name + "\x00" + examples[i].ID
		right := examples[j].APIContractOperationID + "\x00" + examples[j].Name + "\x00" + examples[j].ID
		return left < right
	})
	for index, example := range examples {
		if example.APIContractCandidateID != record.Candidate.ID || !validDeveloperAssetContentHash(example.ContentHash) || (example.APIContractOperationID != "" && !operationIDs[example.APIContractOperationID]) {
			return nil, errors.New("contract example is inconsistent")
		}
		content, err := compactDeveloperAssetJSON(example.Value)
		if err != nil {
			return nil, err
		}
		citation, err := marshalDeveloperAssetObject(map[string]any{
			"publication_kind": "contract", "publication_id": revision.ID,
			"index_publication_kind": "api", "index_publication_id": apiPublication.ID,
			"api_contract_revision_id": revision.ID, "api_contract_candidate_id": record.Candidate.ID,
			"api_contract_example_id": example.ID, "api_contract_operation_id": example.APIContractOperationID,
			"name": example.Name, "content_hash": example.ContentHash,
		})
		if err != nil {
			return nil, err
		}
		metadata := cloneDeveloperAssetMetadata(baseMetadata)
		metadata["example_kind"], metadata["media_type"], metadata["status_code"] = example.Kind, example.MediaType, example.StatusCode
		encoded, err := marshalDeveloperAssetObject(metadata)
		if err != nil {
			return nil, err
		}
		result = append(result, developerAssetIndexDraft{unit: model.KnowledgeUnit{
			Kind: "contract_example", SourcePublicationKind: "contract", SourcePublicationID: revision.ID,
			SourceEntityID: example.ID, ParentSourceEntityID: example.APIContractOperationID,
			Title: example.Name, Breadcrumb: []string{contract.Name, "Examples", example.Name}, Content: content,
			Identifiers: []string{example.ID, example.Name, example.StatusCode, example.MediaType}, Visibility: visibility,
			Citation: citation, Metadata: encoded, ContentHash: example.ContentHash,
		}, order: developerAssetIndexOrder{assetOrdinal: assetOrdinal, entityOrdinal: index, kindRank: 3, tieBreaker: example.ID}, scope: scope})
	}
	return result, nil
}

func developerAssetContractOperationContent(operation model.APIContractOperation) (string, error) {
	security, err := compactDeveloperAssetJSON(operation.Security)
	if err != nil {
		return "", err
	}
	lines := []string{strings.ToUpper(operation.Method) + " " + operation.PathTemplate}
	for _, value := range []struct{ label, content string }{
		{"Operation", operation.OperationID}, {"Summary", operation.Summary}, {"Description", operation.Description},
		{"Tags", strings.Join(operation.Tags, ", ")}, {"Request schemas", strings.Join(operation.RequestSchemaRefs, ", ")},
		{"Response schemas", strings.Join(operation.ResponseSchemaRefs, ", ")}, {"Security", security},
	} {
		if strings.TrimSpace(value.content) != "" && value.content != "{}" {
			lines = append(lines, value.label+": "+strings.TrimSpace(value.content))
		}
	}
	return strings.Join(lines, "\n"), nil
}

func cloneDeveloperAssetMetadata(value map[string]any) map[string]any {
	result := make(map[string]any, len(value)+4)
	for key, item := range value {
		result[key] = item
	}
	return result
}

func (s *Service) buildAPISDKAssetIndex(ctx context.Context, deploymentID string, apiPublication model.APIDeveloperAssetPublication, apiVisibility model.Visibility, asset model.APIPublicationSDKAsset, assetOrdinal int) ([]developerAssetIndexDraft, error) {
	selector, canonicalSelector, err := parseDeveloperAssetSelector(asset.Selector, developerAssetSDKSelector)
	if err != nil {
		return nil, err
	}
	if !validDeveloperAssetContentHash(asset.SelectorHash) || contentHash(canonicalSelector) != asset.SelectorHash {
		return nil, errors.New("API SDK selector hash does not match its exact selector")
	}
	if asset.SDKContentPublicationID == "" {
		return nil, errors.New("API SDK asset has no reviewed content publication")
	}
	packageValue := model.SDKPackage{
		ID: asset.SDKPackageID, DeploymentID: deploymentID, Ecosystem: asset.SDKPackageEcosystem,
		CanonicalCoordinate: asset.SDKPackageCoordinate, DisplayCoordinate: asset.SDKPackageDisplayCoordinate,
		Name: asset.SDKPackageDisplayName, Language: asset.SDKPackageLanguage, Platform: asset.SDKPackagePlatform,
		Visibility: asset.Visibility,
	}
	if strings.TrimSpace(packageValue.ID) == "" || strings.TrimSpace(packageValue.Ecosystem) == "" ||
		strings.TrimSpace(packageValue.CanonicalCoordinate) == "" || strings.TrimSpace(packageValue.DisplayCoordinate) == "" ||
		strings.TrimSpace(packageValue.Name) == "" {
		return nil, errors.New("API SDK asset is missing its immutable package metadata snapshot")
	}
	release, err := s.store.SDKRelease(ctx, deploymentID, asset.SDKReleaseID)
	if err != nil {
		return nil, err
	}
	publication, err := s.store.SDKContentPublication(ctx, deploymentID, asset.SDKContentPublicationID)
	if err != nil {
		return nil, err
	}
	if release.SDKPackageID != packageValue.ID || publication.Publication.SDKReleaseID != release.ID ||
		publication.Publication.SDKContentCandidateID == "" || strings.TrimSpace(release.ExactVersion) == "" ||
		strings.EqualFold(strings.TrimSpace(release.ExactVersion), "latest") {
		return nil, errors.New("API SDK asset does not resolve to one exact package release and content publication")
	}
	record, err := s.store.SDKContentCandidate(ctx, deploymentID, publication.Publication.SDKContentCandidateID)
	if err != nil {
		return nil, err
	}
	if err := store.ValidateSDKContentCandidateGraph(record); err != nil {
		return nil, fmt.Errorf("SDK candidate graph is inconsistent: %w", err)
	}
	if record.Candidate.SDKReleaseID != release.ID || record.Candidate.ID != publication.Publication.SDKContentCandidateID ||
		record.Candidate.ContentHash != publication.Publication.ContentHash ||
		!validDeveloperAssetContentHash(record.Candidate.ContentHash) || !validDeveloperAssetContentHash(release.ReleaseHash) {
		return nil, errors.New("SDK content publication does not resolve to its exact candidate")
	}
	canonicalAssetHash, err := json.Marshal(map[string]any{
		"release_hash": release.ReleaseHash, "content_hash": publication.Publication.ContentHash,
		"selector_hash": asset.SelectorHash, "compatibility_assertion_id": asset.CompatibilityAssertionID,
	})
	if err != nil {
		return nil, err
	}
	expectedAssetHash := contentHash(canonicalAssetHash)
	if !validDeveloperAssetContentHash(asset.ContentHash) || (asset.ContentHash != expectedAssetHash && asset.ContentHash != publication.Publication.ContentHash) {
		return nil, errors.New("API SDK snapshot content hash is not tied to the exact release, publication, and selector")
	}
	if apiVisibility == model.VisibilityPublic && (asset.Visibility != model.VisibilityPublic || release.Visibility != model.VisibilityPublic || publication.Publication.Visibility != model.VisibilityPublic || record.Candidate.Visibility != model.VisibilityPublic) {
		return nil, errors.New("public API publication contains private SDK content")
	}
	visibility, err := developerAssetVisibility(apiVisibility, asset.Visibility, release.Visibility, publication.Publication.Visibility, record.Candidate.Visibility)
	if err != nil {
		return nil, err
	}

	files := make(map[string]model.SDKPublicationFile, len(record.Files))
	for _, file := range record.Files {
		if file.SDKContentCandidateID != record.Candidate.ID || files[file.ID].ID != "" || !validDeveloperAssetContentHash(file.ContentHash) {
			return nil, errors.New("SDK candidate contains an inconsistent file")
		}
		files[file.ID] = file
	}
	includedFiles := make(map[string]int)
	seenFileSelections := make(map[string]bool, len(publication.FileSelections))
	for _, selection := range publication.FileSelections {
		file, ok := files[selection.SDKPublicationFileID]
		if !ok || seenFileSelections[file.ID] || selection.DeploymentID != deploymentID || selection.SDKContentPublicationID != publication.Publication.ID ||
			selection.SDKContentCandidateID != record.Candidate.ID || selection.ContentHash != file.ContentHash {
			return nil, errors.New("SDK file selection does not match the exact candidate")
		}
		seenFileSelections[file.ID] = true
		switch selection.Decision {
		case "included":
			if selection.Ordinal == nil {
				return nil, errors.New("included SDK file has no publication ordinal")
			}
			includedFiles[file.ID] = *selection.Ordinal
		case "excluded", "quarantined":
			if selection.Ordinal != nil || strings.TrimSpace(selection.Reason) == "" {
				return nil, errors.New("excluded SDK file selection is invalid")
			}
		default:
			return nil, errors.New("SDK file selection decision is invalid")
		}
	}
	if len(seenFileSelections) != len(files) {
		return nil, errors.New("SDK publication does not decide every candidate file")
	}

	baseMetadata := map[string]any{
		"asset_kind": "sdk", "api_developer_asset_publication_id": apiPublication.ID,
		"api_sdk_binding_id": asset.BindingID, "sdk_package_id": packageValue.ID,
		"sdk_release_id": release.ID, "sdk_content_publication_id": publication.Publication.ID,
		"sdk_content_candidate_id": record.Candidate.ID, "sdk_content_publication_revision": publication.Publication.Revision,
		"ecosystem": packageValue.Ecosystem, "coordinate": packageValue.CanonicalCoordinate,
		"display_coordinate": packageValue.DisplayCoordinate, "exact_version": release.ExactVersion,
		"install_command": release.InstallCommand, "release_hash": release.ReleaseHash,
		"sdk_content_hash": publication.Publication.ContentHash, "api_snapshot_asset_content_hash": asset.ContentHash,
		"selector_hash": asset.SelectorHash, "compatibility_assertion_id": asset.CompatibilityAssertionID,
	}
	scope := &model.KnowledgeUnitAPIScope{
		APIID: apiPublication.APIID, APISDKBindingID: asset.BindingID,
		ScopeKind: developerAssetScopeKind(selector), SelectorHash: asset.SelectorHash,
	}
	result := make([]developerAssetIndexDraft, 0, len(record.Sections)+len(record.Symbols)+len(record.Samples)+1)
	if err := store.ValidateReviewedSDKPublicationMap(packageValue, release, record, publication); err != nil {
		return nil, fmt.Errorf("published SDK content has a non-canonical review projection: %w", err)
	}
	publishedMap := publication.PublishedMap
	if publication.Map == nil || publishedMap == nil || publication.Map.SDKContentPublicationID != publication.Publication.ID ||
		publication.Map.SDKContentCandidateID != record.Candidate.ID || publishedMap.SDKContentCandidateID != record.Candidate.ID ||
		publication.Map.SDKMapID != publishedMap.ID || publication.Map.ContentHash != publishedMap.ContentHash ||
		!validDeveloperAssetContentHash(publishedMap.ContentHash) {
		return nil, errors.New("published SDK content is missing its exact SDK Map")
	}
	if selector.matches(developerAssetSelectorCandidate{
		kind: "map", contentKind: "map", identifiers: []string{publishedMap.ID, packageValue.ID, release.ID, packageValue.CanonicalCoordinate, release.ExactVersion}, isMap: true,
	}) {
		mapContent, err := developerAssetMapContent(publishedMap.AgentMarkdown, publishedMap.Map)
		if err != nil {
			return nil, err
		}
		citation, err := marshalDeveloperAssetObject(map[string]any{
			"publication_kind": "sdk", "publication_id": publication.Publication.ID,
			"index_publication_kind": "api", "index_publication_id": apiPublication.ID,
			"sdk_content_publication_id": publication.Publication.ID, "sdk_release_id": release.ID,
			"sdk_package_id": packageValue.ID, "exact_version": release.ExactVersion,
			"sdk_map_id": publishedMap.ID, "map_version": publishedMap.MapVersion, "content_hash": publishedMap.ContentHash,
		})
		if err != nil {
			return nil, err
		}
		metadata := cloneDeveloperAssetMetadata(baseMetadata)
		metadata["map_version"] = publishedMap.MapVersion
		encoded, err := marshalDeveloperAssetObject(metadata)
		if err != nil {
			return nil, err
		}
		result = append(result, developerAssetIndexDraft{unit: model.KnowledgeUnit{
			Kind: "map", SourcePublicationKind: "sdk", SourcePublicationID: publication.Publication.ID,
			SourceEntityID: publishedMap.ID, Title: packageValue.Name + " SDK map",
			Breadcrumb: []string{packageValue.Name, release.ExactVersion}, Content: mapContent,
			Language: packageValue.Language, Ecosystem: packageValue.Ecosystem,
			Identifiers: []string{publishedMap.ID, packageValue.ID, release.ID, packageValue.CanonicalCoordinate, packageValue.DisplayCoordinate, release.ExactVersion},
			Visibility:  visibility, Citation: citation, Metadata: encoded, ContentHash: publishedMap.ContentHash,
		}, order: developerAssetIndexOrder{assetOrdinal: assetOrdinal, memberOrdinal: -1, kindRank: 0, tieBreaker: publishedMap.ID}, scope: scope})
	}

	sections := append([]model.SDKSection(nil), record.Sections...)
	sort.Slice(sections, func(i, j int) bool {
		leftFile, leftOK := includedFiles[sections[i].SDKPublicationFileID]
		rightFile, rightOK := includedFiles[sections[j].SDKPublicationFileID]
		if leftOK != rightOK {
			return leftOK
		}
		if leftFile != rightFile {
			return leftFile < rightFile
		}
		if sections[i].Ordinal != sections[j].Ordinal {
			return sections[i].Ordinal < sections[j].Ordinal
		}
		return sections[i].ID < sections[j].ID
	})
	sectionsByID := make(map[string]model.SDKSection, len(sections))
	for _, section := range sections {
		fileOrdinal, included := includedFiles[section.SDKPublicationFileID]
		file, fileExists := files[section.SDKPublicationFileID]
		if !fileExists || section.SDKContentCandidateID != record.Candidate.ID || !validDeveloperAssetContentHash(section.ContentHash) {
			return nil, errors.New("SDK section is inconsistent")
		}
		sectionsByID[section.ID] = section
		if !included {
			continue
		}
		candidate := developerAssetSelectorCandidate{
			kind: "sdk_section", fileID: file.ID, sectionID: section.ID, sourcePath: file.SourcePath,
			language: developerAssetFirstNonEmpty(section.CodeLanguage, file.Language, packageValue.Language), contentKind: section.ContentKind,
			module: developerAssetSDKFileModule(file), identifiers: append([]string{section.ID, section.Anchor, section.Heading, file.SourcePath}, section.Breadcrumb...),
		}
		if !selector.matches(candidate) {
			continue
		}
		draft, err := newSDKSectionDraft(section, file, packageValue, release, publication.Publication, apiPublication, baseMetadata, visibility, assetOrdinal, fileOrdinal, scope)
		if err != nil {
			return nil, err
		}
		result = append(result, draft)
	}

	symbols := append([]model.SDKSymbol(nil), record.Symbols...)
	sort.Slice(symbols, func(i, j int) bool {
		left := symbols[i].QualifiedName + "\x00" + symbols[i].ID
		right := symbols[j].QualifiedName + "\x00" + symbols[j].ID
		return left < right
	})
	for index, symbol := range symbols {
		if symbol.SDKContentCandidateID != record.Candidate.ID || !validDeveloperAssetContentHash(symbol.ContentHash) {
			return nil, errors.New("SDK symbol is inconsistent")
		}
		fileID := symbol.SDKPublicationFileID
		if symbol.SDKSectionID != "" {
			section, ok := sectionsByID[symbol.SDKSectionID]
			if !ok {
				return nil, errors.New("SDK symbol references an unknown section")
			}
			if fileID != "" && fileID != section.SDKPublicationFileID {
				return nil, errors.New("SDK symbol file does not match its section ancestry")
			}
			fileID = section.SDKPublicationFileID
		}
		file, hasFile := files[fileID]
		fileOrdinal, fileIncluded := includedFiles[fileID]
		if !hasFile || !fileIncluded {
			continue
		}
		module := developerAssetSDKModule(symbol.QualifiedName, developerAssetSDKFileModule(file))
		candidate := developerAssetSelectorCandidate{
			kind: "sdk_symbol", fileID: file.ID, sectionID: symbol.SDKSectionID, symbolID: symbol.ID,
			sourcePath: file.SourcePath, language: developerAssetFirstNonEmpty(symbol.Language, file.Language, packageValue.Language),
			contentKind: symbol.Kind, module: module,
			identifiers: append([]string{symbol.ID, symbol.QualifiedName, symbol.DisplayName, symbol.Signature, file.SourcePath}, symbol.Identifiers...),
		}
		if !selector.matches(candidate) {
			continue
		}
		draft, err := newSDKSymbolDraft(symbol, file, packageValue, release, publication.Publication, apiPublication, baseMetadata, visibility, assetOrdinal, fileOrdinal*1_000_000+index, scope)
		if err != nil {
			return nil, err
		}
		result = append(result, draft)
	}

	samples := make(map[string]model.SDKCodeSample, len(record.Samples))
	for _, sample := range record.Samples {
		if sample.SDKContentCandidateID != record.Candidate.ID || samples[sample.ID].ID != "" || !validDeveloperAssetContentHash(sample.ContentHash) {
			return nil, errors.New("SDK sample is inconsistent")
		}
		samples[sample.ID] = sample
	}
	selections := append([]model.SDKContentPublicationSampleSelection(nil), publication.SampleSelections...)
	sort.Slice(selections, func(i, j int) bool {
		left, right := int(^uint(0)>>1), int(^uint(0)>>1)
		if selections[i].Ordinal != nil {
			left = *selections[i].Ordinal
		}
		if selections[j].Ordinal != nil {
			right = *selections[j].Ordinal
		}
		if left == right {
			return selections[i].SDKCodeSampleID < selections[j].SDKCodeSampleID
		}
		return left < right
	})
	seenSamples := make(map[string]bool, len(selections))
	for _, selection := range selections {
		sample, ok := samples[selection.SDKCodeSampleID]
		if !ok || seenSamples[sample.ID] || selection.DeploymentID != deploymentID || selection.SDKContentPublicationID != publication.Publication.ID || !selection.ValidFor(sample) {
			return nil, errors.New("SDK sample selection does not match the exact candidate")
		}
		seenSamples[sample.ID] = true
		if selection.Decision != "approved" {
			continue
		}
		if visibility == model.VisibilityPublic && sample.Visibility != model.VisibilityPublic {
			return nil, errors.New("public SDK publication contains a private approved sample")
		}
		fileID := sample.SDKPublicationFileID
		if sample.SDKSectionID != "" {
			section, ok := sectionsByID[sample.SDKSectionID]
			if !ok {
				return nil, errors.New("SDK sample references an unknown section")
			}
			if fileID != "" && fileID != section.SDKPublicationFileID {
				return nil, errors.New("SDK sample file does not match its section ancestry")
			}
			fileID = section.SDKPublicationFileID
		}
		file := files[fileID]
		if fileID != "" {
			if file.ID == "" {
				return nil, errors.New("SDK sample references an unknown file")
			}
			if _, included := includedFiles[fileID]; !included {
				return nil, errors.New("approved SDK sample belongs to a file that was not included")
			}
		}
		candidate := developerAssetSelectorCandidate{
			kind: "sdk_sample", fileID: file.ID, sectionID: sample.SDKSectionID, sampleID: sample.ID,
			sourcePath: developerAssetFirstNonEmpty(sample.SourcePath, file.SourcePath), language: sample.Language,
			contentKind: "sample", module: developerAssetSDKFileModule(file),
			identifiers: append([]string{sample.ID, sample.Title, sample.Intent, sample.SourcePath}, sample.Imports...),
		}
		if !selector.matches(candidate) {
			continue
		}
		sampleVisibility, err := developerAssetVisibility(visibility, sample.Visibility)
		if err != nil {
			return nil, err
		}
		draft, err := newSDKSampleDraft(sample, file, packageValue, release, publication.Publication, apiPublication, baseMetadata, sampleVisibility, assetOrdinal, *selection.Ordinal, scope)
		if err != nil {
			return nil, err
		}
		result = append(result, draft)
	}
	if len(seenSamples) != len(samples) {
		return nil, errors.New("SDK publication does not decide every candidate sample")
	}
	return result, nil
}

func newSDKSectionDraft(section model.SDKSection, file model.SDKPublicationFile, packageValue model.SDKPackage, release model.SDKRelease, publication model.SDKContentPublication, apiPublication model.APIDeveloperAssetPublication, baseMetadata map[string]any, visibility model.Visibility, assetOrdinal, fileOrdinal int, scope *model.KnowledgeUnitAPIScope) (developerAssetIndexDraft, error) {
	sourceMetadata, err := sourceMetadataValue(section.Metadata)
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	citation, err := marshalDeveloperAssetObject(map[string]any{
		"publication_kind": "sdk", "publication_id": publication.ID,
		"index_publication_kind": "api", "index_publication_id": apiPublication.ID,
		"sdk_content_publication_id": publication.ID, "sdk_release_id": release.ID, "sdk_package_id": packageValue.ID,
		"exact_version": release.ExactVersion, "sdk_section_id": section.ID, "sdk_publication_file_id": file.ID,
		"source_path": file.SourcePath, "anchor": section.Anchor,
		"source_start": developerAssetInteger(section.SourceStart), "source_end": developerAssetInteger(section.SourceEnd),
		"line_range": developerAssetLineRange(section.SourceStart, section.SourceEnd), "content_hash": section.ContentHash,
	})
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	metadata := cloneDeveloperAssetMetadata(baseMetadata)
	metadata["sdk_publication_file_id"], metadata["source_path"], metadata["file_role"] = file.ID, file.SourcePath, file.Role
	metadata["content_kind"], metadata["token_estimate"], metadata["source_metadata"] = section.ContentKind, section.TokenEstimate, sourceMetadata
	encoded, err := marshalDeveloperAssetObject(metadata)
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	title := developerAssetFirstNonEmpty(section.Heading, file.SourcePath)
	breadcrumb := append([]string{packageValue.Name, release.ExactVersion}, section.Breadcrumb...)
	return developerAssetIndexDraft{unit: model.KnowledgeUnit{
		Kind: "sdk_section", SourcePublicationKind: "sdk", SourcePublicationID: publication.ID,
		SourceEntityID: section.ID, ParentSourceEntityID: developerAssetFirstNonEmpty(section.ParentSectionID, file.ID), Title: title,
		Breadcrumb: breadcrumb, Content: developerAssetFirstNonEmpty(strings.TrimSpace(section.NormalizedText), title),
		Language: developerAssetFirstNonEmpty(section.CodeLanguage, file.Language, packageValue.Language), Ecosystem: packageValue.Ecosystem,
		Identifiers: append([]string{section.ID, section.Anchor, section.Heading, file.SourcePath, packageValue.CanonicalCoordinate, release.ExactVersion}, section.Breadcrumb...),
		Visibility:  visibility, Citation: citation, Metadata: encoded, ContentHash: section.ContentHash,
	}, order: developerAssetIndexOrder{assetOrdinal: assetOrdinal, memberOrdinal: fileOrdinal, entityOrdinal: section.Ordinal, kindRank: 1, tieBreaker: section.ID}, scope: scope}, nil
}

func newSDKSymbolDraft(symbol model.SDKSymbol, file model.SDKPublicationFile, packageValue model.SDKPackage, release model.SDKRelease, publication model.SDKContentPublication, apiPublication model.APIDeveloperAssetPublication, baseMetadata map[string]any, visibility model.Visibility, assetOrdinal, entityOrdinal int, scope *model.KnowledgeUnitAPIScope) (developerAssetIndexDraft, error) {
	sourceMetadata, err := sourceMetadataValue(symbol.Metadata)
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	citation, err := marshalDeveloperAssetObject(map[string]any{
		"publication_kind": "sdk", "publication_id": publication.ID,
		"index_publication_kind": "api", "index_publication_id": apiPublication.ID,
		"sdk_content_publication_id": publication.ID, "sdk_release_id": release.ID, "sdk_package_id": packageValue.ID,
		"exact_version": release.ExactVersion, "sdk_symbol_id": symbol.ID, "sdk_section_id": symbol.SDKSectionID,
		"sdk_publication_file_id": file.ID, "source_path": file.SourcePath,
		"source_start": developerAssetInteger(symbol.SourceStart), "source_end": developerAssetInteger(symbol.SourceEnd),
		"line_range": developerAssetLineRange(symbol.SourceStart, symbol.SourceEnd), "content_hash": symbol.ContentHash,
	})
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	metadata := cloneDeveloperAssetMetadata(baseMetadata)
	metadata["sdk_publication_file_id"], metadata["sdk_section_id"], metadata["source_path"] = file.ID, symbol.SDKSectionID, file.SourcePath
	metadata["symbol_kind"], metadata["qualified_name"], metadata["signature"] = symbol.Kind, symbol.QualifiedName, symbol.Signature
	metadata["source_metadata"] = sourceMetadata
	encoded, err := marshalDeveloperAssetObject(metadata)
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	content := strings.TrimSpace(strings.Join(canonicalStringSet([]string{symbol.Signature, symbol.Documentation}), "\n"))
	if content == "" {
		content = developerAssetFirstNonEmpty(symbol.QualifiedName, symbol.DisplayName)
	}
	return developerAssetIndexDraft{unit: model.KnowledgeUnit{
		Kind: "sdk_symbol", SourcePublicationKind: "sdk", SourcePublicationID: publication.ID,
		SourceEntityID: symbol.ID, ParentSourceEntityID: developerAssetFirstNonEmpty(symbol.SDKSectionID, file.ID),
		Title: developerAssetFirstNonEmpty(symbol.DisplayName, symbol.QualifiedName), Breadcrumb: []string{packageValue.Name, release.ExactVersion, file.SourcePath, symbol.QualifiedName},
		Content: content, Language: developerAssetFirstNonEmpty(symbol.Language, file.Language, packageValue.Language), Ecosystem: packageValue.Ecosystem,
		Identifiers: append([]string{symbol.ID, symbol.QualifiedName, symbol.DisplayName, symbol.Signature, packageValue.CanonicalCoordinate, release.ExactVersion}, symbol.Identifiers...),
		Visibility:  visibility, Citation: citation, Metadata: encoded, ContentHash: symbol.ContentHash,
	}, order: developerAssetIndexOrder{assetOrdinal: assetOrdinal, entityOrdinal: entityOrdinal, kindRank: 2, tieBreaker: symbol.ID}, scope: scope}, nil
}

func newSDKSampleDraft(sample model.SDKCodeSample, file model.SDKPublicationFile, packageValue model.SDKPackage, release model.SDKRelease, publication model.SDKContentPublication, apiPublication model.APIDeveloperAssetPublication, baseMetadata map[string]any, visibility model.Visibility, assetOrdinal, sampleOrdinal int, scope *model.KnowledgeUnitAPIScope) (developerAssetIndexDraft, error) {
	validationEvidence, err := sourceMetadataValue(sample.ValidationEvidence)
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	citation, err := marshalDeveloperAssetObject(map[string]any{
		"publication_kind": "sdk", "publication_id": publication.ID,
		"index_publication_kind": "api", "index_publication_id": apiPublication.ID,
		"sdk_content_publication_id": publication.ID, "sdk_release_id": release.ID, "sdk_package_id": packageValue.ID,
		"exact_version": release.ExactVersion, "sdk_sample_id": sample.ID, "sdk_section_id": sample.SDKSectionID,
		"sdk_publication_file_id": sample.SDKPublicationFileID, "source_uri": sample.SourceURI,
		"source_revision": sample.SourceRevision, "source_path": developerAssetFirstNonEmpty(sample.SourcePath, file.SourcePath),
		"source_start": developerAssetInteger(sample.SourceStart), "source_end": developerAssetInteger(sample.SourceEnd),
		"line_range": developerAssetLineRange(sample.SourceStart, sample.SourceEnd), "content_hash": sample.ContentHash,
	})
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	metadata := cloneDeveloperAssetMetadata(baseMetadata)
	metadata["sdk_publication_file_id"], metadata["sdk_section_id"] = sample.SDKPublicationFileID, sample.SDKSectionID
	metadata["sample_origin"], metadata["validation_status"] = sample.Origin, sample.ValidationStatus
	metadata["validation_evidence"], metadata["license_expression"], metadata["attribution"] = validationEvidence, sample.LicenseExpression, sample.Attribution
	encoded, err := marshalDeveloperAssetObject(metadata)
	if err != nil {
		return developerAssetIndexDraft{}, err
	}
	lines := []string{"Intent: " + sample.Intent}
	if len(sample.Prerequisites) != 0 {
		lines = append(lines, "Prerequisites: "+strings.Join(sample.Prerequisites, "; "))
	}
	if len(sample.Imports) != 0 {
		lines = append(lines, "Imports: "+strings.Join(sample.Imports, ", "))
	}
	lines = append(lines, sample.Code)
	return developerAssetIndexDraft{unit: model.KnowledgeUnit{
		Kind: "sdk_sample", SourcePublicationKind: "sdk", SourcePublicationID: publication.ID,
		SourceEntityID: sample.ID, ParentSourceEntityID: developerAssetFirstNonEmpty(sample.SDKSectionID, sample.SDKPublicationFileID),
		Title: sample.Title, Breadcrumb: []string{packageValue.Name, release.ExactVersion, "Samples", sample.Title},
		Content: strings.TrimSpace(strings.Join(lines, "\n")), Language: sample.Language, Ecosystem: packageValue.Ecosystem,
		Identifiers: append([]string{sample.ID, sample.Title, sample.Intent, packageValue.CanonicalCoordinate, release.ExactVersion}, sample.Imports...),
		Visibility:  visibility, Citation: citation, Metadata: encoded, ContentHash: sample.ContentHash,
	}, order: developerAssetIndexOrder{assetOrdinal: assetOrdinal, entityOrdinal: sampleOrdinal, kindRank: 3, tieBreaker: sample.ID}, scope: scope}, nil
}

func developerAssetSDKModule(qualifiedName, sourcePath string) string {
	qualifiedName = strings.TrimSpace(qualifiedName)
	for _, separator := range []string{"::", ".", "#"} {
		if index := strings.LastIndex(qualifiedName, separator); index > 0 {
			return qualifiedName[:index]
		}
	}
	return strings.TrimSpace(sourcePath)
}

func developerAssetSDKFileModule(file model.SDKPublicationFile) string {
	var metadata map[string]any
	if json.Unmarshal(file.Metadata, &metadata) == nil {
		for _, key := range []string{"module", "module_id", "module_name", "package"} {
			if value, ok := metadata[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return strings.TrimSpace(file.SourcePath)
}
