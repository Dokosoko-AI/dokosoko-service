package platform

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

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
