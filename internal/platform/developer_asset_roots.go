package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type APIContractInput struct {
	Name        string
	Slug        string
	Description string
	Visibility  model.Visibility
	Lifecycle   string
	Revision    int64
}

func (s *Service) SaveAPIContract(ctx context.Context, contractID string, input APIContractInput, actor Actor) (model.APIContract, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.APIContract{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Description = strings.TrimSpace(input.Description)
	if err := validateNameSlug(input.Name, input.Slug); err != nil {
		return model.APIContract{}, err
	}
	if len(input.Description) > 4000 {
		return model.APIContract{}, errors.New("contract description must be no more than 4000 characters")
	}
	if input.Visibility == "" {
		input.Visibility = model.VisibilityPrivate
	}
	if !input.Visibility.Valid() {
		return model.APIContract{}, ErrInvalidVisibility
	}
	if input.Lifecycle == "" {
		input.Lifecycle = "active"
	}
	if input.Lifecycle != "active" && input.Lifecycle != "archived" {
		return model.APIContract{}, errors.New("contract lifecycle must be active or archived")
	}
	contractID = strings.TrimSpace(contractID)
	if contractID == "" {
		contractID, err = randomUUID()
		if err != nil {
			return model.APIContract{}, err
		}
	}
	value, err := s.store.SaveAPIContract(ctx, model.APIContract{
		ID: contractID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID,
		Name: input.Name, Slug: input.Slug, Description: input.Description, Kind: "openapi",
		Visibility: input.Visibility, Lifecycle: input.Lifecycle,
	}, input.Revision)
	if err != nil {
		return model.APIContract{}, err
	}
	if err := s.appendDeveloperAssetAudit(ctx, deployment, actor, "api_contract.saved", "api_contract", value.ID, map[string]any{
		"name": value.Name, "slug": value.Slug, "visibility": value.Visibility, "lifecycle": value.Lifecycle, "revision": value.Revision,
	}); err != nil {
		return model.APIContract{}, err
	}
	return value, nil
}

type APIContractSourceInput struct {
	SourceID   string
	SourceRole string
	Revision   int64
}

func (s *Service) SaveAPIContractSource(ctx context.Context, contractID, associationID string, input APIContractSourceInput, actor Actor) (model.APIContractSource, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.APIContractSource{}, err
	}
	contract, err := s.store.APIContract(ctx, deployment.ID, strings.TrimSpace(contractID))
	if err != nil {
		return model.APIContractSource{}, err
	}
	source, err := s.store.Source(ctx, deployment.ID, strings.TrimSpace(input.SourceID))
	if err != nil {
		return model.APIContractSource{}, err
	}
	if source.Kind != "openapi" && source.Kind != "upload" && source.Kind != "repository" {
		return model.APIContractSource{}, errors.New("contract sources must be OpenAPI, upload, or repository sources")
	}
	if contract.Visibility == model.VisibilityPublic && source.Visibility != model.VisibilityPublic {
		return model.APIContractSource{}, errors.New("a public contract cannot use a private source")
	}
	input.SourceRole = strings.TrimSpace(input.SourceRole)
	if input.SourceRole == "" {
		input.SourceRole = "primary"
	}
	if input.SourceRole != "primary" && input.SourceRole != "supplemental" {
		return model.APIContractSource{}, errors.New("contract source role must be primary or supplemental")
	}
	associationID = strings.TrimSpace(associationID)
	if associationID == "" {
		associationID, err = randomUUID()
		if err != nil {
			return model.APIContractSource{}, err
		}
	}
	value := model.APIContractSource{
		ID: associationID, DeploymentID: deployment.ID, APIContractID: contract.ID, SourceID: source.ID,
		SourceRole: input.SourceRole, Lifecycle: "attached", Revision: max(input.Revision, 1), CreatedBy: actor.ID,
	}
	value, err = s.store.SaveAPIContractSource(ctx, value, input.Revision)
	if err != nil {
		return model.APIContractSource{}, err
	}
	if err := s.appendDeveloperAssetAudit(ctx, deployment, actor, "api_contract.source_saved", "api_contract_source", value.ID, map[string]any{
		"api_contract_id": contract.ID, "source_id": source.ID, "source_role": value.SourceRole, "revision": value.Revision,
	}); err != nil {
		return model.APIContractSource{}, err
	}
	return value, nil
}

func (s *Service) DetachAPIContractSource(ctx context.Context, associationID string, revision int64, actor Actor) (model.APIContractSource, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.APIContractSource{}, err
	}
	value, err := s.store.DetachAPIContractSource(ctx, deployment.ID, strings.TrimSpace(associationID), revision)
	if err != nil {
		return model.APIContractSource{}, err
	}
	err = s.appendDeveloperAssetAudit(ctx, deployment, actor, "api_contract.source_detached", "api_contract_source", value.ID, map[string]any{
		"api_contract_id": value.APIContractID, "source_id": value.SourceID, "revision": value.Revision,
	})
	return value, err
}

type DocumentationCollectionMemberInput struct {
	Kind               string
	ID                 string
	IncludeDescendants bool
	Selector           json.RawMessage
}

type DocumentationCollectionInput struct {
	Name                string
	Slug                string
	Description         string
	Visibility          model.Visibility
	Lifecycle           string
	Revision            int64
	Members             []DocumentationCollectionMemberInput
	AcknowledgeReviewed bool
}

type resolvedDocumentationMember struct {
	Member      model.DocumentationCollectionMember
	Title       string
	ContentHash string
	Visibility  model.Visibility
	EvidenceID  string
	MapBody     model.DocumentationMapBody
}

func (s *Service) documentHasIncludedPublication(ctx context.Context, deploymentID string, record store.DocumentationCandidateRecord) bool {
	if record.DocumentationMap == nil {
		return false
	}
	publications, err := s.store.SourcePublications(ctx, deploymentID, record.Run.SourceID)
	if err != nil {
		return false
	}
	for _, publication := range publications {
		review, reviewErr := s.store.SourcePublicationDocumentationReview(ctx, deploymentID, publication.ID)
		if reviewErr != nil {
			continue
		}
		if review.MapLink == nil || review.MapLink.DocumentationMapID != record.DocumentationMap.ID || review.MapLink.ContentHash != record.DocumentationMap.ContentHash {
			continue
		}
		for _, selection := range review.Selections {
			if selection.DocumentationDocumentID == record.Document.ID && selection.Decision == "included" && selection.ContentHash == record.Document.ContentHash {
				return true
			}
		}
	}
	return false
}

type documentationMapScope struct {
	documentIDs         map[string]bool
	categoryDocumentIDs map[string]bool
	sectionIDs          map[string]bool
	knownIDs            map[string]bool
	mapLevel            bool
}

func documentationMapEvidenceID(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"document:", "section:"} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return value
}

func (scope documentationMapScope) selected(value string) bool {
	value = documentationMapEvidenceID(value)
	return scope.documentIDs[value] || scope.sectionIDs[value]
}

func (scope documentationMapScope) known(value string) bool {
	return scope.knownIDs[documentationMapEvidenceID(value)]
}

func scopeDocumentationMapEntry(entry model.KnowledgeMapEntry, scope documentationMapScope) (model.KnowledgeMapEntry, bool) {
	selected := scope.selected(entry.ID)
	aliases := make([]string, 0, len(entry.Aliases))
	for _, alias := range entry.Aliases {
		if scope.selected(alias) {
			selected = true
		}
		if scope.known(alias) && !scope.selected(alias) {
			continue
		}
		aliases = append(aliases, alias)
	}
	children := make([]model.KnowledgeMapEntry, 0, len(entry.Children))
	for _, child := range entry.Children {
		if scoped, ok := scopeDocumentationMapEntry(child, scope); ok {
			children = append(children, scoped)
		}
	}
	if !selected && len(children) == 0 {
		return model.KnowledgeMapEntry{}, false
	}
	entry.Aliases = aliases
	entry.Children = children
	return entry, true
}

func scopeDocumentationMapEntries(values []model.KnowledgeMapEntry, scope documentationMapScope) []model.KnowledgeMapEntry {
	result := make([]model.KnowledgeMapEntry, 0, len(values))
	for _, value := range values {
		if scoped, ok := scopeDocumentationMapEntry(value, scope); ok {
			result = append(result, scoped)
		}
	}
	return result
}

func scopeDocumentationMapBody(value model.DocumentationMapBody, scope documentationMapScope) model.DocumentationMapBody {
	categoryScope := scope
	if scope.categoryDocumentIDs != nil {
		categoryScope.documentIDs = scope.categoryDocumentIDs
	}
	result := model.DocumentationMapBody{
		Overview: value.Overview, Documents: scopeDocumentationMapEntries(value.Documents, scope),
		Topics: scopeDocumentationMapEntries(value.Topics, categoryScope), Workflows: scopeDocumentationMapEntries(value.Workflows, categoryScope),
		Authentication: scopeDocumentationMapEntries(value.Authentication, categoryScope), Errors: scopeDocumentationMapEntries(value.Errors, categoryScope),
		Examples: scopeDocumentationMapEntries(value.Examples, categoryScope),
	}
	// These fields have no evidence IDs, so they cannot be safely attributed to
	// a document/section subset. Retain them only while projecting an exact
	// reviewed map boundary; a narrower member or API selector must fail closed.
	if scope.mapLevel {
		result.Versions = append([]string(nil), value.Versions...)
		result.Languages = append([]string(nil), value.Languages...)
		result.QualityWarnings = append([]string(nil), value.QualityWarnings...)
	}
	for _, gap := range value.Gaps {
		evidenceIDs := make([]string, 0, len(gap.EvidenceIDs))
		for _, evidenceID := range gap.EvidenceIDs {
			if categoryScope.selected(evidenceID) || (categoryScope.mapLevel && !categoryScope.known(evidenceID)) {
				evidenceIDs = append(evidenceIDs, evidenceID)
			}
		}
		if len(gap.EvidenceIDs) == 0 {
			if scope.mapLevel {
				result.Gaps = append(result.Gaps, gap)
			}
			continue
		}
		if len(evidenceIDs) > 0 {
			gap.EvidenceIDs = evidenceIDs
			result.Gaps = append(result.Gaps, gap)
		}
	}
	if scope.mapLevel {
		result.ExcludedSourceIDs = append([]string(nil), value.ExcludedSourceIDs...)
	}
	return result
}

func documentationSectionIDs(sections []model.DocumentationSection, rootID string, includeDescendants bool) map[string]bool {
	selected := make(map[string]bool)
	if rootID == "" {
		if includeDescendants {
			for _, section := range sections {
				selected[section.ID] = true
			}
		}
		return selected
	}
	selected[rootID] = true
	if !includeDescendants {
		return selected
	}
	for changed := true; changed; {
		changed = false
		for _, section := range sections {
			if !selected[section.ID] && selected[section.ParentSectionID] {
				selected[section.ID] = true
				changed = true
			}
		}
	}
	return selected
}

func addDocumentationMapEntryIDs(result map[string]bool, entries []model.KnowledgeMapEntry) {
	for _, entry := range entries {
		result[documentationMapEvidenceID(entry.ID)] = true
		addDocumentationMapEntryIDs(result, entry.Children)
	}
}

func documentationKnownIDs(records []store.DocumentationCandidateRecord, mapBodies ...model.DocumentationMapBody) map[string]bool {
	result := make(map[string]bool)
	for _, record := range records {
		result[record.Document.ID] = true
		for _, section := range record.Sections {
			result[section.ID] = true
		}
	}
	for _, body := range mapBodies {
		addDocumentationMapEntryIDs(result, body.Documents)
	}
	return result
}

func scopeDocumentationMapForSelector(
	value model.DocumentationMapBody,
	records []store.DocumentationCandidateRecord,
	allowedDocumentIDs map[string]bool,
	allowedSectionIDs map[string]bool,
	selector developerAssetSelector,
) model.DocumentationMapBody {
	narrowsMapEvidence := selector.restricted() || selector.includeMap != nil && !*selector.includeMap
	if !narrowsMapEvidence {
		return value
	}
	selectedDocumentIDs := make(map[string]bool)
	selectedSectionIDs := make(map[string]bool)
	for _, record := range records {
		if allowedDocumentIDs[record.Document.ID] && selector.matches(documentationDocumentSelectorCandidate(record.Document)) {
			selectedDocumentIDs[record.Document.ID] = true
		}
		for _, section := range record.Sections {
			if allowedSectionIDs[section.ID] && selector.matches(documentationSectionSelectorCandidate(record.Document, section)) {
				selectedSectionIDs[section.ID] = true
			}
		}
	}
	return scopeDocumentationMapBody(value, documentationMapScope{
		documentIDs: selectedDocumentIDs, categoryDocumentIDs: selectedDocumentIDs,
		sectionIDs: selectedSectionIDs, knownIDs: documentationKnownIDs(records, value),
	})
}

func (s *Service) sourcePublicationDocumentationMap(ctx context.Context, deploymentID string, publication model.SourcePublication, includeDescendants bool, selector developerAssetSelector) (model.DocumentationMapBody, error) {
	review, err := s.store.SourcePublicationDocumentationReview(ctx, deploymentID, publication.ID)
	if err != nil {
		return model.DocumentationMapBody{}, err
	}
	if review.MapLink == nil {
		return model.DocumentationMapBody{}, errors.New("source publication does not pin a persisted Documentation Map")
	}
	records := make([]store.DocumentationCandidateRecord, 0, len(review.Selections))
	selectedDocuments := make(map[string]bool)
	excludedDocuments := make([]string, 0)
	var exactMap *model.DocumentationMap
	for _, selection := range review.Selections {
		if selection.SourcePublicationID != publication.ID || selection.DeploymentID != deploymentID {
			return model.DocumentationMapBody{}, errors.New("source publication review is inconsistent")
		}
		record, lookupErr := s.store.DocumentationCandidateDocument(ctx, deploymentID, selection.DocumentationDocumentID)
		if lookupErr != nil {
			return model.DocumentationMapBody{}, lookupErr
		}
		if record.Document.ContentHash != selection.ContentHash || record.DocumentationMap == nil {
			return model.DocumentationMapBody{}, errors.New("source publication review does not match exact persisted documentation evidence")
		}
		if record.DocumentationMap.ID != review.MapLink.DocumentationMapID || record.DocumentationMap.ContentHash != review.MapLink.ContentHash {
			return model.DocumentationMapBody{}, errors.New("source publication Documentation Map pin does not match its candidate evidence")
		}
		if exactMap == nil {
			value := *record.DocumentationMap
			exactMap = &value
		} else if exactMap.ID != record.DocumentationMap.ID || exactMap.ContentHash != record.DocumentationMap.ContentHash {
			return model.DocumentationMapBody{}, errors.New("source publication spans more than one Documentation Map")
		}
		records = append(records, record)
		switch selection.Decision {
		case "included":
			if selection.Ordinal == nil || strings.TrimSpace(selection.Reason) != "" {
				return model.DocumentationMapBody{}, errors.New("included source publication document selection is invalid")
			}
			selectedDocuments[record.Document.ID] = true
		case "excluded", "quarantined":
			if selection.Ordinal != nil || strings.TrimSpace(selection.Reason) == "" {
				return model.DocumentationMapBody{}, errors.New("excluded source publication document selection is invalid")
			}
			excludedDocuments = append(excludedDocuments, record.Document.ID)
		default:
			return model.DocumentationMapBody{}, errors.New("source publication review decision is invalid")
		}
	}
	if exactMap == nil || len(selectedDocuments) == 0 {
		return model.DocumentationMapBody{}, errors.New("source publication has no included Documentation Map evidence")
	}
	selectedSections := make(map[string]bool)
	if includeDescendants {
		for _, record := range records {
			if selectedDocuments[record.Document.ID] {
				for _, section := range record.Sections {
					selectedSections[section.ID] = true
				}
			}
		}
	}
	body := scopeDocumentationMapBody(exactMap.Map, documentationMapScope{
		documentIDs: selectedDocuments, categoryDocumentIDs: selectedDocuments, sectionIDs: selectedSections, knownIDs: documentationKnownIDs(records, exactMap.Map), mapLevel: true,
	})
	body.ExcludedSourceIDs = sortedUniqueDocumentationMapStrings(append(body.ExcludedSourceIDs, excludedDocuments...))
	body = scopeDocumentationMapForSelector(body, records, selectedDocuments, selectedSections, selector)
	return body, nil
}

func sortedUniqueDocumentationMapStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func mergeDocumentationMapEntries(groups ...[]model.KnowledgeMapEntry) []model.KnowledgeMapEntry {
	byValue := make(map[string]model.KnowledgeMapEntry)
	for _, values := range groups {
		for _, value := range values {
			encoded, err := json.Marshal(value)
			if err == nil {
				byValue[string(encoded)] = value
			}
		}
	}
	keys := make([]string, 0, len(byValue))
	for key := range byValue {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := byValue[keys[i]], byValue[keys[j]]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.ID != right.ID {
			return left.ID < right.ID
		}
		return keys[i] < keys[j]
	})
	result := make([]model.KnowledgeMapEntry, 0, len(keys))
	for _, key := range keys {
		result = append(result, byValue[key])
	}
	return result
}

func mergeDocumentationMapGaps(groups ...[]model.KnowledgeMapGap) []model.KnowledgeMapGap {
	byValue := make(map[string]model.KnowledgeMapGap)
	for _, values := range groups {
		for _, value := range values {
			value.EvidenceIDs = sortedUniqueDocumentationMapStrings(value.EvidenceIDs)
			encoded, err := json.Marshal(value)
			if err == nil {
				byValue[string(encoded)] = value
			}
		}
	}
	keys := make([]string, 0, len(byValue))
	for key := range byValue {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := byValue[keys[i]], byValue[keys[j]]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Description != right.Description {
			return left.Description < right.Description
		}
		return keys[i] < keys[j]
	})
	result := make([]model.KnowledgeMapGap, 0, len(keys))
	for _, key := range keys {
		result = append(result, byValue[key])
	}
	return result
}

func mergeDocumentationMemberMaps(overview string, members []resolvedDocumentationMember, memberEntries []model.KnowledgeMapEntry) model.DocumentationMapBody {
	result := model.DocumentationMapBody{Overview: overview, Documents: append([]model.KnowledgeMapEntry(nil), memberEntries...), Topics: []model.KnowledgeMapEntry{}, Workflows: []model.KnowledgeMapEntry{}}
	for _, member := range members {
		result.Topics = mergeDocumentationMapEntries(result.Topics, member.MapBody.Topics)
		result.Workflows = mergeDocumentationMapEntries(result.Workflows, member.MapBody.Workflows)
		result.Authentication = mergeDocumentationMapEntries(result.Authentication, member.MapBody.Authentication)
		result.Errors = mergeDocumentationMapEntries(result.Errors, member.MapBody.Errors)
		result.Examples = mergeDocumentationMapEntries(result.Examples, member.MapBody.Examples)
		result.Versions = sortedUniqueDocumentationMapStrings(append(result.Versions, member.MapBody.Versions...))
		result.Languages = sortedUniqueDocumentationMapStrings(append(result.Languages, member.MapBody.Languages...))
		result.Gaps = mergeDocumentationMapGaps(result.Gaps, member.MapBody.Gaps)
		result.QualityWarnings = sortedUniqueDocumentationMapStrings(append(result.QualityWarnings, member.MapBody.QualityWarnings...))
		result.ExcludedSourceIDs = sortedUniqueDocumentationMapStrings(append(result.ExcludedSourceIDs, member.MapBody.ExcludedSourceIDs...))
	}
	return result
}

func (s *Service) resolveDocumentationMember(ctx context.Context, deploymentID, revisionID string, ordinal int, input DocumentationCollectionMemberInput) (resolvedDocumentationMember, error) {
	input.Kind = strings.TrimSpace(input.Kind)
	input.ID = strings.TrimSpace(input.ID)
	if input.ID == "" {
		return resolvedDocumentationMember{}, errors.New("documentation member ID is required")
	}
	selector, err := normalizeJSONObject(input.Selector)
	if err != nil {
		return resolvedDocumentationMember{}, err
	}
	memberSelector, selector, err := parseDeveloperAssetSelector(selector, developerAssetDocumentationSelector)
	if err != nil {
		return resolvedDocumentationMember{}, err
	}
	memberID, err := randomUUID()
	if err != nil {
		return resolvedDocumentationMember{}, err
	}
	result := resolvedDocumentationMember{Member: model.DocumentationCollectionMember{
		ID: memberID, DocumentationCollectionRevisionID: revisionID, Kind: input.Kind,
		Ordinal: ordinal, IncludeDescendants: input.IncludeDescendants, Selector: selector,
	}}
	switch input.Kind {
	case "source_publication":
		publication, lookupErr := s.store.SourcePublication(ctx, deploymentID, input.ID)
		if lookupErr != nil {
			return resolvedDocumentationMember{}, lookupErr
		}
		mapBody, mapErr := s.sourcePublicationDocumentationMap(ctx, deploymentID, publication, input.IncludeDescendants, memberSelector)
		if mapErr != nil {
			return resolvedDocumentationMember{}, mapErr
		}
		result.Member.SourcePublicationID = publication.ID
		result.Title, result.ContentHash, result.Visibility = publication.SourceID, publication.ContentHash, publication.Visibility
		result.EvidenceID = "source-publication:" + publication.ID
		result.MapBody = mapBody
	case "document":
		record, lookupErr := s.store.DocumentationCandidateDocument(ctx, deploymentID, input.ID)
		if lookupErr != nil {
			return resolvedDocumentationMember{}, lookupErr
		}
		if !s.documentHasIncludedPublication(ctx, deploymentID, record) {
			return resolvedDocumentationMember{}, errors.New("documentation document has not been included by a reviewed source publication")
		}
		if record.DocumentationMap == nil {
			return resolvedDocumentationMember{}, errors.New("documentation document does not have a persisted Documentation Map")
		}
		knownIDs := documentationKnownIDs([]store.DocumentationCandidateRecord{record}, record.DocumentationMap.Map)
		allowedDocumentIDs := map[string]bool{record.Document.ID: true}
		allowedSectionIDs := documentationSectionIDs(record.Sections, "", input.IncludeDescendants)
		result.MapBody = scopeDocumentationMapBody(record.DocumentationMap.Map, documentationMapScope{
			documentIDs: allowedDocumentIDs, categoryDocumentIDs: allowedDocumentIDs,
			sectionIDs: allowedSectionIDs, knownIDs: knownIDs,
		})
		result.MapBody = scopeDocumentationMapForSelector(result.MapBody, []store.DocumentationCandidateRecord{record}, allowedDocumentIDs, allowedSectionIDs, memberSelector)
		result.Member.DocumentationDocumentID = record.Document.ID
		result.Title, result.ContentHash, result.Visibility = record.Document.Title, record.Document.ContentHash, record.Document.Visibility
		result.EvidenceID = "document:" + record.Document.ID
	case "section":
		section, record, lookupErr := s.store.DocumentationCandidateSection(ctx, deploymentID, input.ID)
		if lookupErr != nil {
			return resolvedDocumentationMember{}, lookupErr
		}
		if !s.documentHasIncludedPublication(ctx, deploymentID, record) {
			return resolvedDocumentationMember{}, errors.New("documentation section belongs to a document that has not been included by a reviewed source publication")
		}
		if record.DocumentationMap == nil {
			return resolvedDocumentationMember{}, errors.New("documentation section does not have a persisted Documentation Map")
		}
		knownIDs := documentationKnownIDs([]store.DocumentationCandidateRecord{record}, record.DocumentationMap.Map)
		allowedSectionIDs := documentationSectionIDs(record.Sections, section.ID, input.IncludeDescendants)
		result.MapBody = scopeDocumentationMapBody(record.DocumentationMap.Map, documentationMapScope{
			documentIDs: map[string]bool{record.Document.ID: true}, categoryDocumentIDs: map[string]bool{},
			sectionIDs: allowedSectionIDs, knownIDs: knownIDs,
		})
		result.MapBody = scopeDocumentationMapForSelector(result.MapBody, []store.DocumentationCandidateRecord{record}, map[string]bool{}, allowedSectionIDs, memberSelector)
		result.Member.DocumentationSectionID = section.ID
		result.Title, result.ContentHash, result.Visibility = section.Heading, section.ContentHash, record.Document.Visibility
		result.EvidenceID = "section:" + section.ID
	default:
		return resolvedDocumentationMember{}, errors.New("documentation member kind must be source_publication, document, or section")
	}
	return result, nil
}

func (s *Service) SaveDocumentationCollection(ctx context.Context, collectionID string, input DocumentationCollectionInput, actor Actor) (model.DocumentationCollection, error) {
	if !input.AcknowledgeReviewed {
		return model.DocumentationCollection{}, ErrSourceReviewRequired
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.DocumentationCollection{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Description = strings.TrimSpace(input.Description)
	if err := validateNameSlug(input.Name, input.Slug); err != nil {
		return model.DocumentationCollection{}, err
	}
	if len(input.Description) > 4000 || len(input.Members) == 0 || len(input.Members) > 1000 {
		return model.DocumentationCollection{}, errors.New("documentation collection requires 1-1000 members and a description no longer than 4000 characters")
	}
	if input.Visibility == "" {
		input.Visibility = model.VisibilityPrivate
	}
	if !input.Visibility.Valid() {
		return model.DocumentationCollection{}, ErrInvalidVisibility
	}
	if input.Lifecycle == "" {
		input.Lifecycle = "active"
	}
	if input.Lifecycle != "active" && input.Lifecycle != "archived" {
		return model.DocumentationCollection{}, errors.New("documentation collection lifecycle must be active or archived")
	}
	collectionID = strings.TrimSpace(collectionID)
	creating := collectionID == ""
	if creating {
		collectionID, err = randomUUID()
		if err != nil {
			return model.DocumentationCollection{}, err
		}
	}
	revisionID, err := randomUUID()
	if err != nil {
		return model.DocumentationCollection{}, err
	}
	revisionNumber := int64(1)
	if !creating {
		current, lookupErr := s.store.DocumentationCollection(ctx, deployment.ID, collectionID)
		if lookupErr != nil {
			return model.DocumentationCollection{}, lookupErr
		}
		if current.Revision != input.Revision {
			return model.DocumentationCollection{}, store.ErrConflict
		}
		revisionNumber = input.Revision + 1
	}
	resolved := make([]resolvedDocumentationMember, 0, len(input.Members))
	seen := make(map[string]bool, len(input.Members))
	for index, member := range input.Members {
		value, resolveErr := s.resolveDocumentationMember(ctx, deployment.ID, revisionID, index, member)
		if resolveErr != nil {
			return model.DocumentationCollection{}, fmt.Errorf("documentation member %d: %w", index+1, resolveErr)
		}
		key := value.Member.Kind + "\x00" + member.ID
		if seen[key] {
			return model.DocumentationCollection{}, errors.New("documentation collection cannot contain the same member twice")
		}
		seen[key] = true
		if input.Visibility == model.VisibilityPublic && value.Visibility != model.VisibilityPublic {
			return model.DocumentationCollection{}, errors.New("public documentation collection cannot include private evidence")
		}
		resolved = append(resolved, value)
	}
	manifestValues := make([]map[string]any, 0, len(resolved))
	mapEntries := make([]model.KnowledgeMapEntry, 0, len(resolved))
	memberValues := make([]model.DocumentationCollectionMember, 0, len(resolved))
	var markdown strings.Builder
	fmt.Fprintf(&markdown, "# %s\n\n%s\n\n## Contents\n", documentationMapLine(input.Name), documentationMapLine(input.Description))
	for _, value := range resolved {
		memberValues = append(memberValues, value.Member)
		manifestValues = append(manifestValues, map[string]any{
			"kind": value.Member.Kind, "evidence_id": value.EvidenceID, "content_hash": value.ContentHash,
			"include_descendants": value.Member.IncludeDescendants, "selector": value.Member.Selector,
		})
		mapEntries = append(mapEntries, model.KnowledgeMapEntry{
			ID: value.EvidenceID, Kind: value.Member.Kind, Title: value.Title,
			Summary: "Exact reviewed collection member.", Children: value.MapBody.Documents,
		})
		fmt.Fprintf(&markdown, "- %s — `%s` (`%s`)\n", documentationMapLine(value.Title), value.EvidenceID, value.ContentHash)
	}
	manifest, err := json.Marshal(manifestValues)
	if err != nil {
		return model.DocumentationCollection{}, err
	}
	mapBody := mergeDocumentationMemberMaps(input.Description, resolved, mapEntries)
	mapJSON, err := json.Marshal(map[string]any{"map_version": "documentation-map-v1", "map": mapBody, "agent_markdown": markdown.String()})
	if err != nil {
		return model.DocumentationCollection{}, err
	}
	now := s.now()
	revision := model.DocumentationCollectionRevision{
		ID: revisionID, DeploymentID: deployment.ID, DocumentationCollectionID: collectionID, Revision: revisionNumber,
		DocumentationCollectionName: input.Name, DocumentationCollectionSlug: input.Slug,
		DocumentationCollectionDescription: input.Description,
		Visibility:                         input.Visibility, ContentHash: contentHash(manifest), SelectionManifest: manifest,
		ReviewedBy: actor.ID, ReviewedAt: now, PublishedAt: now,
	}
	mapID, err := randomUUID()
	if err != nil {
		return model.DocumentationCollection{}, err
	}
	documentationMap := model.DocumentationMap{
		ID: mapID, DeploymentID: deployment.ID, DocumentationCollectionRevisionID: revisionID,
		MapVersion: "documentation-map-v1", Map: mapBody, AgentMarkdown: markdown.String(),
		ContentHash: contentHash(mapJSON), Visibility: input.Visibility,
	}
	collection := model.DocumentationCollection{
		ID: collectionID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID,
		Name: input.Name, Slug: input.Slug, Description: input.Description,
		Visibility: input.Visibility, Lifecycle: input.Lifecycle,
	}
	record := store.DocumentationCollectionRevisionRecord{Revision: revision, Members: memberValues, Map: &documentationMap}
	if creating {
		collection, err = s.store.CreateDocumentationCollection(ctx, collection, record)
	} else {
		collection, err = s.store.ReviseDocumentationCollection(ctx, collection, input.Revision, record)
	}
	if err != nil {
		return model.DocumentationCollection{}, err
	}
	if err := s.appendDeveloperAssetAudit(ctx, deployment, actor, "documentation_collection.revision_saved", "documentation_collection", collection.ID, map[string]any{
		"name": collection.Name, "revision": collection.Revision, "revision_id": revision.ID,
		"content_hash": revision.ContentHash, "member_count": len(memberValues), "visibility": collection.Visibility,
	}); err != nil {
		return model.DocumentationCollection{}, err
	}
	return collection, nil
}

func documentationMapLine(value string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(value), " "), "`", "'")
}

type DeploymentDocumentationPublicationInput struct {
	CollectionRevisionIDs []string
	Visibility            model.Visibility
	ExpectedHeadRevision  int64
	AcknowledgeReviewed   bool
}

func (s *Service) PublishDeploymentDocumentation(ctx context.Context, input DeploymentDocumentationPublicationInput, actor Actor) (model.DeploymentDocumentationPublication, error) {
	if !input.AcknowledgeReviewed {
		return model.DeploymentDocumentationPublication{}, ErrSourceReviewRequired
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.DeploymentDocumentationPublication{}, err
	}
	if input.Visibility == "" {
		input.Visibility = model.VisibilityPrivate
	}
	if !input.Visibility.Valid() {
		return model.DeploymentDocumentationPublication{}, ErrInvalidVisibility
	}
	if len(input.CollectionRevisionIDs) == 0 || len(input.CollectionRevisionIDs) > 500 {
		return model.DeploymentDocumentationPublication{}, errors.New("global documentation publication requires 1-500 exact collection revisions")
	}
	members := make([]model.DeploymentDocumentationPublicationMember, 0, len(input.CollectionRevisionIDs))
	seen := make(map[string]bool, len(input.CollectionRevisionIDs))
	for ordinal, id := range input.CollectionRevisionIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return model.DeploymentDocumentationPublication{}, errors.New("global documentation publication revision IDs must be non-empty and unique")
		}
		seen[id] = true
		record, lookupErr := s.store.DocumentationCollectionRevision(ctx, deployment.ID, id)
		if lookupErr != nil {
			return model.DeploymentDocumentationPublication{}, lookupErr
		}
		if input.Visibility == model.VisibilityPublic && record.Revision.Visibility != model.VisibilityPublic {
			return model.DeploymentDocumentationPublication{}, errors.New("public global documentation cannot include a private collection revision")
		}
		members = append(members, model.DeploymentDocumentationPublicationMember{
			DocumentationCollectionRevisionID: record.Revision.ID, Ordinal: ordinal,
			ContentHash: record.Revision.ContentHash, Visibility: record.Revision.Visibility,
		})
	}
	canonical, err := json.Marshal(map[string]any{
		"schema_version": "deployment-documentation-v1", "visibility": input.Visibility, "members": members,
	})
	if err != nil {
		return model.DeploymentDocumentationPublication{}, err
	}
	snapshotHash := contentHash(canonical)
	id, err := randomUUID()
	if err != nil {
		return model.DeploymentDocumentationPublication{}, err
	}
	now := s.now()
	publication := model.DeploymentDocumentationPublication{
		ID: id, DeploymentID: deployment.ID, Revision: input.ExpectedHeadRevision + 1,
		Visibility: input.Visibility, SnapshotSchemaVersion: "deployment-documentation-v1",
		SnapshotHash: snapshotHash, Members: members, PublishedBy: actor.ID, PublishedAt: now,
	}
	publication, err = s.store.PublishDeploymentDocumentation(ctx, publication, input.ExpectedHeadRevision)
	if err != nil {
		// A publication and its search index are separate durable records. If a
		// previous request committed the publication but failed while building
		// its index or audit event, make an exact retry recover that immutable
		// publication instead of forcing a duplicate revision.
		if !errors.Is(err, store.ErrConflict) {
			return model.DeploymentDocumentationPublication{}, err
		}
		head, headRevision, lookupErr := s.store.ActiveDeploymentDocumentationPublication(ctx, deployment.ID)
		if lookupErr != nil || headRevision != input.ExpectedHeadRevision+1 || head.SnapshotHash != snapshotHash || head.Visibility != input.Visibility {
			return model.DeploymentDocumentationPublication{}, err
		}
		publication = head
	}
	if err := s.activateDeploymentDocumentationPublication(ctx, deployment, publication, actor); err != nil {
		return model.DeploymentDocumentationPublication{}, err
	}
	return publication, nil
}
