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
