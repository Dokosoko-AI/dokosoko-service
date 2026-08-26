package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const (
	developerAssetAIMaxEvidenceItems = 160
	developerAssetAIMaxPromptBytes   = 96 << 10
	developerAssetAIMaxEvidenceBytes = 56 << 10
)

type DeveloperAssetAIAdvisoryInput struct {
	PromptKey                      string `json:"prompt_key"`
	SourcePublicationID            string `json:"source_publication_id,omitempty"`
	SDKContentPublicationID        string `json:"sdk_content_publication_id,omitempty"`
	APIID                          string `json:"api_id,omitempty"`
	APIDeveloperAssetPublicationID string `json:"api_developer_asset_publication_id,omitempty"`
	APISDKBindingID                string `json:"api_sdk_binding_id,omitempty"`
	SDKCodeSampleID                string `json:"sdk_code_sample_id,omitempty"`
}

type developerAssetAIEvidence struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Excerpt     string `json:"excerpt,omitempty"`
	ContentHash string `json:"content_hash"`
	Truncated   bool   `json:"truncated,omitempty"`
	Mandatory   bool   `json:"-"`
}

type developerAssetAIWorkflowScope struct {
	product           model.Product
	promptKey         string
	schemaName        string
	schema            json.RawMessage
	artifact          model.DeveloperAssetAIAdvisoryRun
	evidence          []developerAssetAIEvidence
	evidenceIDs       []string
	allowedEvidence   map[string]bool
	allowedSelectors  map[string]map[string]bool
	approvedSamples   map[string]bool
	mandatoryEvidence map[string]bool
	payloadScope      map[string]any
	mapKinds          map[string]bool
	truncated         bool
	stageRunID        string
	action            string
	maxOutput         int
}

func normalizeDeveloperAssetAIAdvisoryInput(input DeveloperAssetAIAdvisoryInput) DeveloperAssetAIAdvisoryInput {
	input.PromptKey = strings.TrimSpace(input.PromptKey)
	input.SourcePublicationID = strings.TrimSpace(input.SourcePublicationID)
	input.SDKContentPublicationID = strings.TrimSpace(input.SDKContentPublicationID)
	input.APIID = strings.TrimSpace(input.APIID)
	input.APIDeveloperAssetPublicationID = strings.TrimSpace(input.APIDeveloperAssetPublicationID)
	input.APISDKBindingID = strings.TrimSpace(input.APISDKBindingID)
	input.SDKCodeSampleID = strings.TrimSpace(input.SDKCodeSampleID)
	return input
}

func validateDeveloperAssetAIAdvisoryInput(input DeveloperAssetAIAdvisoryInput) error {
	missing := func(values ...string) bool {
		for _, value := range values {
			if value == "" {
				return true
			}
		}
		return false
	}
	switch input.PromptKey {
	case AIPromptKeyDocumentationMap:
		if missing(input.SourcePublicationID) || input.SDKContentPublicationID != "" || input.APIID != "" ||
			input.APIDeveloperAssetPublicationID != "" || input.APISDKBindingID != "" || input.SDKCodeSampleID != "" {
			return fmt.Errorf("%w: documentation enrichment requires only source_publication_id", ErrDeveloperAssetAIAdvisoryInvalid)
		}
	case AIPromptKeySDKMap:
		if missing(input.SDKContentPublicationID) || input.SourcePublicationID != "" || input.APIID != "" ||
			input.APIDeveloperAssetPublicationID != "" || input.APISDKBindingID != "" || input.SDKCodeSampleID != "" {
			return fmt.Errorf("%w: SDK map enrichment requires only sdk_content_publication_id", ErrDeveloperAssetAIAdvisoryInvalid)
		}
	case AIPromptKeySDKApplicability:
		if missing(input.SDKContentPublicationID, input.APIID, input.APIDeveloperAssetPublicationID, input.APISDKBindingID) ||
			input.SourcePublicationID != "" || input.SDKCodeSampleID != "" {
			return fmt.Errorf("%w: SDK applicability requires one exact API publication binding scope", ErrDeveloperAssetAIAdvisoryInvalid)
		}
	case AIPromptKeySDKSampleReview:
		if missing(input.SDKContentPublicationID, input.APIID, input.APIDeveloperAssetPublicationID, input.APISDKBindingID, input.SDKCodeSampleID) ||
			input.SourcePublicationID != "" {
			return fmt.Errorf("%w: SDK sample review requires one exact published sample and API binding scope", ErrDeveloperAssetAIAdvisoryInvalid)
		}
	default:
		return fmt.Errorf("%w: unsupported prompt_key", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	return nil
}

func newDeveloperAssetAIWorkflowScope(product model.Product, promptKey string) developerAssetAIWorkflowScope {
	return developerAssetAIWorkflowScope{
		product: product, promptKey: promptKey, allowedEvidence: make(map[string]bool),
		mandatoryEvidence: make(map[string]bool),
		approvedSamples:   make(map[string]bool),
		allowedSelectors:  map[string]map[string]bool{"module": {}, "operation": {}, "sample": {}},
		artifact:          model.DeveloperAssetAIAdvisoryRun{DeploymentID: product.ID, PromptKey: promptKey},
	}
}

func (scope *developerAssetAIWorkflowScope) addEvidence(id, kind, label, excerpt, hash string, maximum int) {
	id, kind, label = strings.TrimSpace(id), strings.TrimSpace(kind), strings.TrimSpace(label)
	if id == "" || len(id) > 200 || scope.allowedEvidence[id] {
		return
	}
	if len(scope.evidence) >= developerAssetAIMaxEvidenceItems-8 {
		scope.truncated = true
		return
	}
	if maximum < 1 {
		maximum = 1200
	}
	excerpt = strings.TrimSpace(excerpt)
	truncated := len([]rune(excerpt)) > maximum
	excerpt = truncateRunes(excerpt, maximum)
	label = truncateRunes(strings.Join(strings.Fields(label), " "), 240)
	if label == "" {
		label = kind
	}
	if !validDeveloperAssetContentHash(hash) {
		hash = contentHash([]byte(kind + "\x00" + label + "\x00" + excerpt))
	}
	scope.evidence = append(scope.evidence, developerAssetAIEvidence{
		ID: id, Kind: kind, Label: label, Excerpt: excerpt, ContentHash: hash, Truncated: truncated,
	})
	scope.allowedEvidence[id] = true
	scope.truncated = scope.truncated || truncated
}

func (scope *developerAssetAIWorkflowScope) addMandatoryEvidence(id, kind, label, excerpt, hash string, maximum int) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	scope.mandatoryEvidence[id] = true
	if scope.allowedEvidence[id] {
		for index := range scope.evidence {
			if scope.evidence[index].ID == id {
				scope.evidence[index].Mandatory = true
				return
			}
		}
	}
	// Mandatory lineage/map evidence uses reserved capacity and is never
	// displaced by a large corpus. finalizeEvidence applies the cumulative
	// byte budget with mandatory records first.
	currentLimit := developerAssetAIMaxEvidenceItems
	if len(scope.evidence) < currentLimit {
		if maximum < 1 {
			maximum = 1200
		}
		excerpt = strings.TrimSpace(excerpt)
		truncated := len([]rune(excerpt)) > maximum
		excerpt = truncateRunes(excerpt, maximum)
		label = truncateRunes(strings.Join(strings.Fields(strings.TrimSpace(label)), " "), 240)
		if label == "" {
			label = kind
		}
		if !validDeveloperAssetContentHash(hash) {
			hash = contentHash([]byte(kind + "\x00" + label + "\x00" + excerpt))
		}
		scope.evidence = append(scope.evidence, developerAssetAIEvidence{ID: id, Kind: kind, Label: label, Excerpt: excerpt, ContentHash: hash, Truncated: truncated, Mandatory: true})
		scope.allowedEvidence[id] = true
		scope.truncated = scope.truncated || truncated
	}
}

func (scope *developerAssetAIWorkflowScope) finalizeEvidence() error {
	if len(scope.evidence) == 0 {
		return fmt.Errorf("%w: exact reviewed scope has no bounded evidence", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	sort.Slice(scope.evidence, func(i, j int) bool {
		if scope.evidence[i].Mandatory != scope.evidence[j].Mandatory {
			return scope.evidence[i].Mandatory
		}
		if scope.evidence[i].Kind == scope.evidence[j].Kind {
			return scope.evidence[i].ID < scope.evidence[j].ID
		}
		return scope.evidence[i].Kind < scope.evidence[j].Kind
	})
	bounded := make([]developerAssetAIEvidence, 0, len(scope.evidence))
	usedBytes := 2
	for _, item := range scope.evidence {
		encoded, marshalErr := json.Marshal(item)
		if marshalErr != nil {
			return marshalErr
		}
		cost := len(encoded) + 1
		if len(bounded) >= developerAssetAIMaxEvidenceItems || usedBytes+cost > developerAssetAIMaxEvidenceBytes {
			if item.Mandatory {
				return fmt.Errorf("%w: mandatory advisory lineage exceeds the evidence budget", ErrDeveloperAssetAIAdvisoryInvalid)
			}
			scope.truncated = true
			continue
		}
		bounded, usedBytes = append(bounded, item), usedBytes+cost
	}
	scope.evidence = bounded
	scope.allowedEvidence = make(map[string]bool, len(scope.evidence))
	scope.evidenceIDs = make([]string, 0, len(scope.evidence))
	for _, item := range scope.evidence {
		scope.evidenceIDs = append(scope.evidenceIDs, item.ID)
		scope.allowedEvidence[item.ID] = true
	}
	for id := range scope.mandatoryEvidence {
		if !scope.allowedEvidence[id] {
			return fmt.Errorf("%w: mandatory advisory lineage exceeds the evidence budget", ErrDeveloperAssetAIAdvisoryInvalid)
		}
	}
	for _, kind := range []string{"operation", "sample"} {
		for value := range scope.allowedSelectors[kind] {
			if !scope.allowedEvidence[value] {
				delete(scope.allowedSelectors[kind], value)
			}
		}
	}
	sort.Strings(scope.evidenceIDs)
	encoded, err := json.Marshal(scope.evidence)
	if err != nil {
		return err
	}
	scope.artifact.AllowedEvidenceIDs = append([]string(nil), scope.evidenceIDs...)
	scope.artifact.EvidenceHash = contentHash(encoded)
	return nil
}

func (s *Service) documentationAIWorkflowScope(ctx context.Context, product model.Product, input DeveloperAssetAIAdvisoryInput) (developerAssetAIWorkflowScope, error) {
	scope := newDeveloperAssetAIWorkflowScope(product, input.PromptKey)
	publication, err := s.store.SourcePublication(ctx, product.ID, input.SourcePublicationID)
	if err != nil {
		return scope, err
	}
	if publication.ProductID != product.ID || publication.ReviewedBy == "" || publication.ReviewedAt.IsZero() || publication.PublishedAt.IsZero() ||
		!validDeveloperAssetContentHash(publication.ContentHash) {
		return scope, fmt.Errorf("%w: source publication is not one immutable reviewed publication", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	review, err := s.store.SourcePublicationDocumentationReview(ctx, product.ID, publication.ID)
	if err != nil || review.MapLink == nil || review.MapLink.SourcePublicationID != publication.ID ||
		review.MapLink.DeploymentID != product.ID || !validDeveloperAssetContentHash(review.MapLink.ContentHash) {
		return scope, fmt.Errorf("%w: reviewed documentation publication is missing its exact deterministic map", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	selections := append([]model.SourcePublicationDocumentSelection(nil), review.Selections...)
	sort.Slice(selections, func(i, j int) bool {
		if selections[i].Ordinal == nil {
			return false
		}
		if selections[j].Ordinal == nil {
			return true
		}
		return *selections[i].Ordinal < *selections[j].Ordinal
	})
	var exactRun model.DeveloperAssetIngestionRun
	var exactMap *model.DocumentationMap
	included := 0
	visibility := publication.Visibility
	for _, selection := range selections {
		if selection.SourcePublicationID != publication.ID || selection.DeploymentID != product.ID || selection.ReviewedBy == "" || selection.ReviewedAt.IsZero() {
			return scope, fmt.Errorf("%w: documentation selection escapes its reviewed publication", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		if selection.Decision != "included" {
			continue
		}
		if selection.Ordinal == nil || selection.Reason != "" || !validDeveloperAssetContentHash(selection.ContentHash) {
			return scope, fmt.Errorf("%w: included documentation selection is incomplete", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		record, lookupErr := s.store.DocumentationCandidateDocument(ctx, product.ID, selection.DocumentationDocumentID)
		if lookupErr != nil || record.Document.ID != selection.DocumentationDocumentID || record.Document.ContentHash != selection.ContentHash ||
			record.Run.DeploymentID != product.ID || record.Run.AssetKind != model.DeveloperAssetDocumentation || record.Document.IngestionRunID != record.Run.ID {
			return scope, fmt.Errorf("%w: documentation selection does not resolve to its exact candidate run", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		if record.Run.State != model.DeveloperAssetIngestionReviewReady && record.Run.State != model.DeveloperAssetIngestionPublished {
			return scope, fmt.Errorf("%w: documentation candidate run is not review-complete", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		if record.Document.Visibility.Valid() {
			visibility, err = developerAssetVisibility(visibility, record.Document.Visibility)
		} else {
			return scope, ErrInvalidVisibility
		}
		if exactRun.ID == "" {
			exactRun = record.Run
			exactMap = record.DocumentationMap
		} else if exactRun.ID != record.Run.ID {
			return scope, fmt.Errorf("%w: documentation publication mixes candidate runs", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		if record.DocumentationMap == nil || record.DocumentationMap.ID != review.MapLink.DocumentationMapID ||
			record.DocumentationMap.ContentHash != review.MapLink.ContentHash || record.DocumentationMap.IngestionRunID != record.Run.ID {
			return scope, fmt.Errorf("%w: documentation publication map does not match its exact run", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		scope.addEvidence(record.Document.ID, "document", record.Document.Title, record.Document.NormalizedMarkdown, record.Document.ContentHash, 1000)
		sections := append([]model.DocumentationSection(nil), record.Sections...)
		sort.Slice(sections, func(i, j int) bool { return sections[i].Ordinal < sections[j].Ordinal })
		for _, section := range sections {
			if section.DeploymentID != product.ID || section.DocumentationDocumentID != record.Document.ID || !validDeveloperAssetContentHash(section.ContentHash) {
				return scope, fmt.Errorf("%w: documentation section escapes its exact document", ErrDeveloperAssetAIAdvisoryInvalid)
			}
			scope.addEvidence(section.ID, "section", firstNonEmpty(section.Heading, record.Document.Title), section.NormalizedText, section.ContentHash, 1400)
		}
		included++
	}
	if included == 0 || exactMap == nil || exactRun.ID == "" || exactMap.MapVersion == "" || exactMap.AgentMarkdown == "" {
		return scope, fmt.Errorf("%w: reviewed documentation publication has no included deterministic-map evidence", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	visibility, err = developerAssetVisibility(visibility, exactMap.Visibility)
	if err != nil {
		return scope, err
	}
	scope.addMandatoryEvidence(exactMap.ID, "documentation_map", "Deterministic Documentation Map", exactMap.Map.Overview, exactMap.ContentHash, 600)
	scope.schemaName, scope.schema = "documentation_map_advisory", documentationMapAdvisorySchema
	scope.mapKinds = map[string]bool{"topic": true, "workflow": true, "authentication": true, "error": true, "example": true, "version": true, "language": true}
	scope.action, scope.maxOutput, scope.stageRunID = "documentation_map_enrichment", 4096, exactRun.ID
	scope.artifact.ScopeKind, scope.artifact.ScopeID, scope.artifact.ScopeVisibility = "documentation_publication", publication.ID, visibility
	scope.artifact.IngestionRunID, scope.artifact.SourcePublicationID = exactRun.ID, publication.ID
	scope.payloadScope = map[string]any{
		"kind": "documentation_publication", "source_publication_id": publication.ID, "source_id": publication.SourceID,
		"publication_content_hash": publication.ContentHash, "map_id": exactMap.ID, "map_version": exactMap.MapVersion,
		"map_content_hash": exactMap.ContentHash, "visibility": visibility,
	}
	if err := scope.finalizeEvidence(); err != nil {
		return scope, err
	}
	return scope, nil
}

func (s *Service) sdkAIWorkflowScope(ctx context.Context, product model.Product, input DeveloperAssetAIAdvisoryInput) (developerAssetAIWorkflowScope, store.SDKContentCandidateRecord, store.SDKContentPublicationRecord, error) {
	scope := newDeveloperAssetAIWorkflowScope(product, input.PromptKey)
	publication, err := s.store.SDKContentPublication(ctx, product.ID, input.SDKContentPublicationID)
	if err != nil {
		return scope, store.SDKContentCandidateRecord{}, publication, err
	}
	release, err := s.store.SDKRelease(ctx, product.ID, publication.Publication.SDKReleaseID)
	if err != nil {
		return scope, store.SDKContentCandidateRecord{}, publication, err
	}
	packageValue, err := s.store.SDKPackage(ctx, product.ID, release.SDKPackageID)
	if err != nil {
		return scope, store.SDKContentCandidateRecord{}, publication, err
	}
	candidate, err := s.store.SDKContentCandidate(ctx, product.ID, publication.Publication.SDKContentCandidateID)
	if err != nil {
		return scope, candidate, publication, err
	}
	if publication.Publication.DeploymentID != product.ID || publication.Publication.ID != input.SDKContentPublicationID ||
		publication.Publication.SDKReleaseID != release.ID || release.SDKPackageID != packageValue.ID ||
		publication.Publication.SDKContentCandidateID != candidate.Candidate.ID || candidate.Candidate.SDKReleaseID != release.ID ||
		candidate.Candidate.ContentHash != publication.Publication.ContentHash || publication.Publication.ReviewedBy == "" ||
		publication.Publication.ReviewedAt.IsZero() || publication.Publication.PublishedAt.IsZero() {
		return scope, candidate, publication, fmt.Errorf("%w: SDK content is not one exact immutable reviewed publication", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	if err := store.ValidateReviewedSDKPublicationMap(packageValue, release, candidate, publication); err != nil {
		return scope, candidate, publication, fmt.Errorf("%w: reviewed SDK publication map is not its canonical decision projection: %v", ErrDeveloperAssetAIAdvisoryInvalid, err)
	}
	publishedMap := publication.PublishedMap
	if publishedMap == nil || publication.Map == nil || publication.Map.SDKContentPublicationID != publication.Publication.ID ||
		publication.Map.SDKContentCandidateID != candidate.Candidate.ID || publishedMap.SDKContentCandidateID != candidate.Candidate.ID ||
		publication.Map.SDKMapID != publishedMap.ID || publication.Map.ContentHash != publishedMap.ContentHash ||
		publishedMap.MapVersion == "" || publishedMap.AgentMarkdown == "" {
		return scope, candidate, publication, fmt.Errorf("%w: reviewed SDK publication is missing its exact deterministic map", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	run, err := s.store.DeveloperAssetIngestionRun(ctx, product.ID, candidate.Candidate.IngestionRunID)
	if err != nil || run.AssetKind != model.DeveloperAssetSDK || run.TargetID != release.ID ||
		(run.State != model.DeveloperAssetIngestionReviewReady && run.State != model.DeveloperAssetIngestionPublished) {
		return scope, candidate, publication, fmt.Errorf("%w: SDK publication does not resolve to its exact review-complete run", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	visibility, err := developerAssetVisibility(packageValue.Visibility, release.Visibility, candidate.Candidate.Visibility, publication.Publication.Visibility)
	if err != nil {
		return scope, candidate, publication, err
	}
	files := make(map[string]model.SDKPublicationFile, len(candidate.Files))
	for _, file := range candidate.Files {
		files[file.ID] = file
	}
	selectedFiles := make(map[string]bool)
	for _, selection := range publication.FileSelections {
		file, exists := files[selection.SDKPublicationFileID]
		if !exists || selection.DeploymentID != product.ID || selection.SDKContentPublicationID != publication.Publication.ID ||
			selection.SDKContentCandidateID != candidate.Candidate.ID || selection.ContentHash != file.ContentHash {
			return scope, candidate, publication, fmt.Errorf("%w: SDK file selection escapes its reviewed publication", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		if selection.Decision == "included" {
			if selection.Ordinal == nil || selection.Reason != "" {
				return scope, candidate, publication, fmt.Errorf("%w: included SDK file selection is incomplete", ErrDeveloperAssetAIAdvisoryInvalid)
			}
			selectedFiles[file.ID] = true
			scope.addEvidence(file.ID, "sdk_file", file.SourcePath, file.NormalizedContent, file.ContentHash, 1000)
		}
	}
	for _, section := range candidate.Sections {
		if selectedFiles[section.SDKPublicationFileID] {
			scope.addEvidence(section.ID, "sdk_section", firstNonEmpty(section.Heading, files[section.SDKPublicationFileID].SourcePath), section.NormalizedText, section.ContentHash, 1400)
		}
	}
	for _, symbol := range candidate.Symbols {
		if selectedFiles[symbol.SDKPublicationFileID] {
			scope.addEvidence(symbol.ID, "sdk_symbol", symbol.QualifiedName, strings.TrimSpace(symbol.Signature+"\n"+symbol.Documentation), symbol.ContentHash, 800)
		}
	}
	samples := make(map[string]model.SDKCodeSample, len(candidate.Samples))
	for _, sample := range candidate.Samples {
		samples[sample.ID] = sample
	}
	for _, selection := range publication.SampleSelections {
		sample, exists := samples[selection.SDKCodeSampleID]
		if !exists || selection.SDKContentPublicationID != publication.Publication.ID || !selection.ValidFor(sample) {
			return scope, candidate, publication, fmt.Errorf("%w: SDK sample selection escapes its reviewed publication", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		if selection.Decision == "approved" {
			scope.approvedSamples[sample.ID] = true
			visibility, err = developerAssetVisibility(visibility, sample.Visibility)
			if err != nil {
				return scope, candidate, publication, err
			}
			if input.PromptKey == AIPromptKeySDKSampleReview && sample.ID == input.SDKCodeSampleID {
				scope.addMandatoryEvidence(sample.ID, "sdk_sample", sample.Title, strings.TrimSpace(sample.Intent+"\n"+sample.Code), sample.ContentHash, 1600)
			} else {
				scope.addEvidence(sample.ID, "sdk_sample", sample.Title, strings.TrimSpace(sample.Intent+"\n"+sample.Code), sample.ContentHash, 1600)
			}
		}
	}
	if len(selectedFiles) == 0 {
		return scope, candidate, publication, fmt.Errorf("%w: reviewed SDK publication has no included files", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	scope.addMandatoryEvidence(publishedMap.ID, "sdk_map", "Deterministic reviewed SDK Map", publishedMap.Map.Overview, publishedMap.ContentHash, 600)
	scope.artifact.ScopeKind, scope.artifact.ScopeID, scope.artifact.ScopeVisibility = "sdk_content_publication", publication.Publication.ID, visibility
	scope.artifact.IngestionRunID, scope.artifact.SDKPackageID, scope.artifact.SDKReleaseID = run.ID, packageValue.ID, release.ID
	scope.artifact.SDKContentCandidateID, scope.artifact.SDKContentPublicationID = candidate.Candidate.ID, publication.Publication.ID
	scope.stageRunID = run.ID
	scope.payloadScope = map[string]any{
		"kind": "sdk_content_publication", "sdk_package_id": packageValue.ID, "ecosystem": packageValue.Ecosystem,
		"coordinate": packageValue.CanonicalCoordinate, "sdk_release_id": release.ID, "exact_version": release.ExactVersion,
		"release_hash": release.ReleaseHash, "sdk_content_candidate_id": candidate.Candidate.ID,
		"sdk_content_publication_id": publication.Publication.ID, "publication_content_hash": publication.Publication.ContentHash,
		"map_id": publishedMap.ID, "map_version": publishedMap.MapVersion, "map_content_hash": publishedMap.ContentHash,
		"visibility": visibility,
	}
	return scope, candidate, publication, nil
}

func exactAPIRevisionVisibility(revisions []model.IntegrationRevision, publication model.APIDeveloperAssetPublication) (model.Visibility, error) {
	for _, revision := range revisions {
		if revision.ID != publication.APIRevisionID {
			continue
		}
		if revision.IntegrationID != publication.APIID || revision.State != "published" || revision.PublishedAt == nil {
			return "", fmt.Errorf("%w: API publication does not reference an exact published revision", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		var snapshot struct {
			Visibility model.Visibility `json:"visibility"`
		}
		if json.Unmarshal(revision.Snapshot, &snapshot) != nil {
			return "", fmt.Errorf("%w: API revision snapshot is invalid", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		if snapshot.Visibility == "" {
			snapshot.Visibility = model.VisibilityPrivate
		}
		if !snapshot.Visibility.Valid() {
			return "", ErrInvalidVisibility
		}
		return snapshot.Visibility, nil
	}
	return "", fmt.Errorf("%w: API publication revision was not found", ErrDeveloperAssetAIAdvisoryInvalid)
}

func (s *Service) extendSDKAPIAIWorkflowScope(ctx context.Context, scope *developerAssetAIWorkflowScope, input DeveloperAssetAIAdvisoryInput, candidate store.SDKContentCandidateRecord) error {
	publication, err := s.store.APIDeveloperAssetPublication(ctx, scope.product.ID, input.APIDeveloperAssetPublicationID)
	if err != nil {
		return err
	}
	if publication.APIID != input.APIID || publication.DeploymentID != scope.product.ID || publication.PublishedAt.IsZero() {
		return fmt.Errorf("%w: API publication does not belong to the selected API", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	ready, err := s.developerAssetPublicationReady(ctx, scope.product.ID, "api", publication.ID)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("%w: API developer-asset publication is not activation and index ready", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	var sdkAsset *model.APIPublicationSDKAsset
	for index := range publication.SDKs {
		asset := &publication.SDKs[index]
		if asset.BindingID == input.APISDKBindingID {
			if sdkAsset != nil {
				return fmt.Errorf("%w: API publication contains an ambiguous SDK binding", ErrDeveloperAssetAIAdvisoryInvalid)
			}
			sdkAsset = asset
		}
	}
	if sdkAsset == nil || sdkAsset.SDKPackageID != scope.artifact.SDKPackageID || sdkAsset.SDKReleaseID != scope.artifact.SDKReleaseID ||
		sdkAsset.SDKContentPublicationID != scope.artifact.SDKContentPublicationID || sdkAsset.ContentHash == "" {
		return fmt.Errorf("%w: SDK binding is not the exact immutable asset in the selected API publication", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	revisions, err := s.store.IntegrationRevisions(ctx, publication.APIID)
	if err != nil {
		return err
	}
	apiVisibility, err := exactAPIRevisionVisibility(revisions, publication)
	if err != nil {
		return err
	}
	scope.artifact.ScopeVisibility, err = developerAssetVisibility(scope.artifact.ScopeVisibility, apiVisibility, sdkAsset.Visibility)
	if err != nil {
		return err
	}
	scope.artifact.APIID, scope.artifact.APIDeveloperAssetPublicationID, scope.artifact.APISDKBindingID = publication.APIID, publication.ID, sdkAsset.BindingID
	scope.addMandatoryEvidence(publication.ID, "api_publication", "Exact API publication", publication.SnapshotHash, publication.SnapshotHash, 200)
	scope.addMandatoryEvidence(sdkAsset.BindingID, "api_sdk_binding_snapshot", "Exact SDK binding snapshot", string(sdkAsset.Selector), sdkAsset.SelectorHash, 800)
	operations := make(map[string]model.APIContractOperation)
	operationKeys := make(map[string]string)
	exactContractRevisions := make(map[string]bool)
	for _, asset := range publication.Contracts {
		revision, lookupErr := s.store.APIContractRevision(ctx, scope.product.ID, asset.APIContractRevisionID)
		if lookupErr != nil || !asset.MatchesRevisionIdentity(revision) || revision.ContentHash != asset.ContentHash {
			return fmt.Errorf("%w: API publication contract does not resolve to its exact revision", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		record, lookupErr := s.store.APIContractCandidate(ctx, scope.product.ID, revision.APIContractCandidateID)
		if lookupErr != nil || record.Candidate.ID != revision.APIContractCandidateID {
			return fmt.Errorf("%w: API contract revision candidate was not found", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		scope.artifact.ScopeVisibility, err = developerAssetVisibility(scope.artifact.ScopeVisibility, asset.Visibility, revision.Visibility, record.Candidate.Visibility)
		if err != nil {
			return err
		}
		exactContractRevisions[revision.ID] = true
		for _, operation := range record.Operations {
			operations[operation.ID] = operation
			operationKeys[operation.OperationKey] = operation.ID
		}
	}
	for _, reference := range candidate.SampleRefs {
		if !scope.approvedSamples[reference.SDKCodeSampleID] || !scope.allowedEvidence[reference.SDKCodeSampleID] ||
			reference.DeploymentID != scope.product.ID || reference.SDKContentCandidateID != candidate.Candidate.ID || reference.APIID != publication.APIID ||
			reference.APISDKBindingID != "" && reference.APISDKBindingID != sdkAsset.BindingID ||
			reference.APIContractRevisionID != "" && !exactContractRevisions[reference.APIContractRevisionID] {
			continue
		}
		if reference.APIContractOperationID != "" {
			operation, exists := operations[reference.APIContractOperationID]
			if !exists || reference.APIContractCandidateID != "" && operation.APIContractCandidateID != reference.APIContractCandidateID {
				continue
			}
			scope.allowedSelectors["operation"][operation.ID] = true
			scope.addEvidence(operation.ID, "api_operation", operation.Method+" "+operation.PathTemplate, strings.TrimSpace(operation.Summary+"\n"+operation.Description), operation.ContentHash, 1000)
		}
		scope.allowedSelectors["sample"][reference.SDKCodeSampleID] = true
		if input.PromptKey == AIPromptKeySDKSampleReview && reference.SDKCodeSampleID == input.SDKCodeSampleID {
			scope.addMandatoryEvidence(reference.ID, "sdk_api_reference", reference.ReferenceKind, "Reviewed reference from sample "+reference.SDKCodeSampleID+" to API "+publication.APIID, "", 400)
		} else {
			scope.addEvidence(reference.ID, "sdk_api_reference", reference.ReferenceKind, "Reviewed reference from sample "+reference.SDKCodeSampleID+" to API "+publication.APIID, "", 400)
		}
	}
	if sdkAsset.CompatibilityAssertionID != "" {
		assertion, lookupErr := s.store.SDKCompatibilityAssertion(ctx, scope.product.ID, sdkAsset.CompatibilityAssertionID)
		if lookupErr != nil || assertion.APIID != publication.APIID || assertion.SDKReleaseID != scope.artifact.SDKReleaseID || assertion.State != "active" ||
			assertion.APIContractRevisionID != "" && !exactContractRevisions[assertion.APIContractRevisionID] {
			return fmt.Errorf("%w: compatibility assertion escapes the exact publication scope", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		for _, module := range assertion.ApplicableModules {
			module = strings.TrimSpace(module)
			if safeDeveloperAssetAIText(module, 240) {
				scope.allowedSelectors["module"][module] = true
			}
		}
		for _, key := range assertion.ApplicableOperationKeys {
			if operationID := operationKeys[key]; operationID != "" {
				scope.allowedSelectors["operation"][operationID] = true
				operation := operations[operationID]
				scope.addEvidence(operation.ID, "api_operation", operation.Method+" "+operation.PathTemplate, strings.TrimSpace(operation.Summary+"\n"+operation.Description), operation.ContentHash, 1000)
			}
		}
		scope.addMandatoryEvidence(assertion.ID, "compatibility_assertion", "Reviewed compatibility assertion", strings.Join(append(append([]string{}, assertion.ApplicableModules...), assertion.KnownGaps...), "\n"), assertion.ContentHash, 1000)
	}
	scope.payloadScope["api_id"] = publication.APIID
	scope.payloadScope["api_developer_asset_publication_id"] = publication.ID
	scope.payloadScope["api_revision_id"] = publication.APIRevisionID
	scope.payloadScope["api_snapshot_hash"] = publication.SnapshotHash
	scope.payloadScope["api_sdk_binding_id"] = sdkAsset.BindingID
	scope.payloadScope["api_sdk_selector_hash"] = sdkAsset.SelectorHash
	scope.payloadScope["visibility"] = scope.artifact.ScopeVisibility
	return nil
}

func (s *Service) loadDeveloperAssetAIWorkflowScope(ctx context.Context, product model.Product, input DeveloperAssetAIAdvisoryInput) (developerAssetAIWorkflowScope, error) {
	if input.PromptKey == AIPromptKeyDocumentationMap {
		return s.documentationAIWorkflowScope(ctx, product, input)
	}
	scope, candidate, publication, err := s.sdkAIWorkflowScope(ctx, product, input)
	if err != nil {
		return scope, err
	}
	switch input.PromptKey {
	case AIPromptKeySDKMap:
		scope.schemaName, scope.schema = "sdk_map_advisory", sdkMapAdvisorySchema
		scope.mapKinds = map[string]bool{"installation": true, "initialization": true, "authentication": true, "module": true, "symbol": true, "workflow": true, "sample": true, "error": true, "pagination": true, "retry": true, "webhook": true, "deprecation": true, "migration": true}
		scope.action, scope.maxOutput = "sdk_map_enrichment", 4096
	case AIPromptKeySDKApplicability, AIPromptKeySDKSampleReview:
		if err := s.extendSDKAPIAIWorkflowScope(ctx, &scope, input, candidate); err != nil {
			return scope, err
		}
		if input.PromptKey == AIPromptKeySDKApplicability {
			scope.schemaName, scope.schema = "sdk_applicability_advisory", sdkApplicabilityAdvisorySchema
			scope.action, scope.maxOutput, scope.stageRunID = "sdk_applicability_suggestion", 2048, ""
			scope.artifact.ScopeKind, scope.artifact.ScopeID = "sdk_api_binding", input.APISDKBindingID
		} else {
			sample, found := model.SDKCodeSample{}, false
			for _, value := range candidate.Samples {
				if value.ID == input.SDKCodeSampleID {
					sample, found = value, true
					break
				}
			}
			approved := false
			for _, selection := range publication.SampleSelections {
				approved = approved || selection.SDKCodeSampleID == input.SDKCodeSampleID && selection.Decision == "approved" && selection.ValidFor(sample)
			}
			if !found || !approved || !scope.allowedSelectors["sample"][sample.ID] {
				return scope, fmt.Errorf("%w: sample is not an approved, explicitly API-referenced member of the exact publication", ErrDeveloperAssetAIAdvisoryInvalid)
			}
			scope.artifact.ScopeVisibility, err = developerAssetVisibility(scope.artifact.ScopeVisibility, sample.Visibility)
			if err != nil {
				return scope, err
			}
			// Replace the map-sized excerpt with the largest bounded static review
			// evidence. It remains data inside JSON and is never executed.
			for index := range scope.evidence {
				if scope.evidence[index].ID == sample.ID {
					excerpt := strings.TrimSpace("Intent: " + sample.Intent + "\nImports: " + strings.Join(sample.Imports, ", ") + "\nPrerequisites: " + strings.Join(sample.Prerequisites, "; ") + "\nCode:\n" + sample.Code)
					truncated := len([]rune(excerpt)) > 8000
					scope.evidence[index].Excerpt = truncateRunes(excerpt, 8000)
					scope.evidence[index].Truncated = truncated
					scope.truncated = scope.truncated || truncated
					scope.evidence[index].Mandatory = true
					scope.mandatoryEvidence[sample.ID] = true
				}
			}
			for _, reference := range candidate.SampleRefs {
				if reference.SDKCodeSampleID == sample.ID && scope.allowedEvidence[reference.ID] {
					scope.mandatoryEvidence[reference.ID] = true
					for index := range scope.evidence {
						if scope.evidence[index].ID == reference.ID {
							scope.evidence[index].Mandatory = true
						}
					}
				}
			}
			scope.schemaName, scope.schema = "sdk_sample_review_advisory", sdkSampleReviewAdvisorySchema
			scope.action, scope.maxOutput, scope.stageRunID = "sdk_sample_review", 2048, ""
			scope.artifact.ScopeKind, scope.artifact.ScopeID, scope.artifact.SDKCodeSampleID = "sdk_sample", sample.ID, sample.ID
			scope.payloadScope["sdk_code_sample_id"] = sample.ID
			scope.payloadScope["sample_content_hash"] = sample.ContentHash
			scope.payloadScope["validation_status"] = sample.ValidationStatus
		}
	default:
		return scope, fmt.Errorf("%w: unsupported prompt_key", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	if err := scope.finalizeEvidence(); err != nil {
		return scope, err
	}
	return scope, nil
}

func developerAssetAISelectorPayload(values map[string]map[string]bool) map[string][]string {
	result := make(map[string][]string)
	for kind, selectors := range values {
		for value := range selectors {
			result[kind] = append(result[kind], value)
		}
		sort.Strings(result[kind])
	}
	return result
}

func developerAssetAIStageErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrAIUnavailable):
		return "ai_unavailable"
	case errors.Is(err, ErrDeveloperAssetAIAdvisoryInvalid):
		return "invalid_advisory"
	default:
		if code := string(airuntime.Code(err)); code != "" {
			return code
		}
		return "workflow_failed"
	}
}

func (s *Service) startDeveloperAssetAIStage(ctx context.Context, runID, promptKey, inputHash string) (model.DeveloperAssetIngestionStage, error) {
	stages, err := s.store.DeveloperAssetIngestionStages(ctx, runID)
	if err != nil {
		return model.DeveloperAssetIngestionStage{}, err
	}
	attempt := 1
	for _, stage := range stages {
		if stage.Name == model.IngestionStageAIEnrich && stage.Attempt >= attempt {
			attempt = stage.Attempt + 1
		}
	}
	now := s.now()
	for retries := 0; retries < 4; retries++ {
		id, idErr := randomUUID()
		if idErr != nil {
			return model.DeveloperAssetIngestionStage{}, idErr
		}
		checkpoint, _ := json.Marshal(map[string]any{"prompt_key": promptKey, "input_hash": inputHash})
		stage := model.DeveloperAssetIngestionStage{
			ID: id, IngestionRunID: runID, Name: model.IngestionStageAIEnrich, Attempt: attempt + retries,
			State: "running", InputHash: inputHash, Checkpoint: checkpoint, Diagnostics: json.RawMessage(`{"state":"running"}`), StartedAt: &now,
		}
		created, createErr := s.store.SaveDeveloperAssetIngestionStage(ctx, stage, "")
		if !errors.Is(createErr, store.ErrConflict) {
			return created, createErr
		}
	}
	return model.DeveloperAssetIngestionStage{}, store.ErrConflict
}

func (s *Service) finishDeveloperAssetAIStage(ctx context.Context, stage model.DeveloperAssetIngestionStage, advisoryID, outputHash string, runErr error) {
	if stage.ID == "" {
		return
	}
	now := s.now()
	stage.FinishedAt, stage.OutputHash, stage.ErrorMessage = &now, outputHash, ""
	if runErr == nil {
		stage.State = "succeeded"
		stage.Checkpoint, _ = json.Marshal(map[string]any{"advisory_run_id": advisoryID, "result_hash": outputHash})
		stage.Diagnostics = json.RawMessage(`{"state":"succeeded"}`)
	} else {
		stage.State, stage.OutputHash, stage.ErrorCode = "failed", "", developerAssetAIStageErrorCode(runErr)
		stage.Checkpoint = json.RawMessage(`{}`)
		stage.Diagnostics, _ = json.Marshal(map[string]any{"state": "failed", "error_code": stage.ErrorCode})
	}
	_, _ = s.store.SaveDeveloperAssetIngestionStage(ctx, stage, "running")
}

func developerAssetAICachedRunMatches(cached, expected model.DeveloperAssetAIAdvisoryRun, promptVersion, inputHash string) bool {
	if !cached.Valid() || cached.PromptVersion != promptVersion || cached.InputHash != inputHash ||
		cached.ResultHash != contentHash(cached.Result) || cached.EvidenceHash != expected.EvidenceHash ||
		cached.DeploymentID != expected.DeploymentID || cached.PromptKey != expected.PromptKey ||
		cached.ScopeKind != expected.ScopeKind || cached.ScopeID != expected.ScopeID ||
		cached.ScopeVisibility != expected.ScopeVisibility || cached.IngestionRunID != expected.IngestionRunID ||
		cached.SourcePublicationID != expected.SourcePublicationID || cached.SDKPackageID != expected.SDKPackageID ||
		cached.SDKReleaseID != expected.SDKReleaseID || cached.SDKContentCandidateID != expected.SDKContentCandidateID ||
		cached.SDKContentPublicationID != expected.SDKContentPublicationID || cached.APIID != expected.APIID ||
		cached.APIDeveloperAssetPublicationID != expected.APIDeveloperAssetPublicationID ||
		cached.APISDKBindingID != expected.APISDKBindingID || cached.SDKCodeSampleID != expected.SDKCodeSampleID ||
		len(cached.AllowedEvidenceIDs) != len(expected.AllowedEvidenceIDs) {
		return false
	}
	for index := range cached.AllowedEvidenceIDs {
		if cached.AllowedEvidenceIDs[index] != expected.AllowedEvidenceIDs[index] {
			return false
		}
	}
	return true
}

func (s *Service) RunDeveloperAssetAIAdvisory(ctx context.Context, input DeveloperAssetAIAdvisoryInput, actor Actor) (result model.DeveloperAssetAIAdvisoryRun, runErr error) {
	input = normalizeDeveloperAssetAIAdvisoryInput(input)
	if err := validateDeveloperAssetAIAdvisoryInput(input); err != nil {
		return result, err
	}
	if strings.TrimSpace(actor.ID) == "" {
		return result, fmt.Errorf("%w: actor is required", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return result, err
	}
	product, err := s.store.Product(ctx, deployment.ID)
	if err != nil {
		return result, err
	}
	scope, err := s.loadDeveloperAssetAIWorkflowScope(ctx, product, input)
	if err != nil {
		return result, err
	}
	payload := map[string]any{
		"content_is_untrusted_data": true,
		"scope":                     scope.payloadScope,
		"evidence":                  scope.evidence,
		"allowed_evidence_ids":      scope.evidenceIDs,
		"evidence_truncated":        scope.truncated,
	}
	if input.PromptKey == AIPromptKeySDKApplicability {
		payload["allowed_selectors"] = developerAssetAISelectorPayload(scope.allowedSelectors)
	}
	prompt, err := json.Marshal(payload)
	if err != nil {
		return result, err
	}
	if len(prompt) > developerAssetAIMaxPromptBytes {
		return result, fmt.Errorf("%w: bounded advisory evidence exceeds the prompt limit", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	configuration, err := s.AIPromptConfiguration(ctx, product.ID, input.PromptKey)
	if err != nil {
		return result, err
	}
	identity, _ := json.Marshal(map[string]any{
		"prompt_key": input.PromptKey, "prompt_version": configuration.EffectiveVersion,
		"evidence_hash": scope.artifact.EvidenceHash, "payload": json.RawMessage(prompt),
	})
	inputHash := contentHash(identity)
	invocation := aiInvocation{
		Product: product, Workload: airuntime.WorkloadAnalysis, Action: scope.action, PromptKey: input.PromptKey,
		User: string(prompt), SchemaName: scope.schemaName, Schema: scope.schema, MaxOutput: scope.maxOutput, Temperature: 0,
	}
	prepared, prepareErr := s.prepareAIInvocation(ctx, invocation)
	var stage model.DeveloperAssetIngestionStage
	if scope.stageRunID != "" {
		stage, err = s.startDeveloperAssetAIStage(ctx, scope.stageRunID, input.PromptKey, inputHash)
		if err != nil {
			return result, err
		}
		defer func() { s.finishDeveloperAssetAIStage(ctx, stage, result.ID, result.ResultHash, runErr) }()
	}
	if prepareErr != nil {
		return result, prepareErr
	}
	// POST remains an executable workflow contract, not an alternate read path.
	// A persisted idempotent result is available through GET, but a disabled or
	// unconfigured Analysis workload must still fail explicitly before a POST
	// can return a cached artifact.
	if _, _, targetErr := s.aiWorkloadTarget(ctx, product, airuntime.WorkloadAnalysis); targetErr != nil {
		return result, targetErr
	}
	if cached, lookupErr := s.store.DeveloperAssetAIAdvisoryRunByInputHash(ctx, product.ID, input.PromptKey, inputHash); lookupErr == nil {
		if !developerAssetAICachedRunMatches(cached, scope.artifact, prepared.PromptVersion, inputHash) {
			return result, fmt.Errorf("%w: cached advisory artifact is invalid", ErrDeveloperAssetAIAdvisoryInvalid)
		}
		return cached, nil
	} else if !errors.Is(lookupErr, store.ErrNotFound) {
		return result, lookupErr
	}
	completion, err := s.generateAIStructured(ctx, prepared)
	if err != nil {
		return result, err
	}
	var canonical json.RawMessage
	switch input.PromptKey {
	case AIPromptKeyDocumentationMap, AIPromptKeySDKMap:
		canonical, err = validateDeveloperAssetAIMapResult(completion.JSON, scope.allowedEvidence, scope.mapKinds)
	case AIPromptKeySDKApplicability:
		canonical, err = validateDeveloperAssetAIApplicabilityResult(completion.JSON, scope.allowedEvidence, scope.allowedSelectors)
	case AIPromptKeySDKSampleReview:
		canonical, err = validateDeveloperAssetAISampleReviewResult(completion.JSON, scope.allowedEvidence)
	}
	if err != nil {
		return result, err
	}
	id, err := randomUUID()
	if err != nil {
		return result, err
	}
	scope.artifact.ID, scope.artifact.PromptVersion = id, prepared.PromptVersion
	scope.artifact.InputHash, scope.artifact.Result, scope.artifact.ResultHash = inputHash, canonical, contentHash(canonical)
	scope.artifact.CreatedBy, scope.artifact.CreatedAt = actor.ID, s.now()
	if !scope.artifact.Valid() {
		return result, fmt.Errorf("%w: validated advisory artifact has an invalid scope", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	result, err = s.store.CreateDeveloperAssetAIAdvisoryRun(ctx, scope.artifact)
	if err != nil {
		return model.DeveloperAssetAIAdvisoryRun{}, err
	}
	return result, nil
}

func (s *Service) DeveloperAssetAIAdvisoryRuns(ctx context.Context, promptKey, scopeID string, limit int) ([]model.DeveloperAssetAIAdvisoryRun, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return nil, err
	}
	promptKey, scopeID = strings.TrimSpace(promptKey), strings.TrimSpace(scopeID)
	if promptKey != "" {
		switch promptKey {
		case AIPromptKeyDocumentationMap, AIPromptKeySDKMap, AIPromptKeySDKApplicability, AIPromptKeySDKSampleReview:
		default:
			return nil, fmt.Errorf("%w: unsupported prompt_key", ErrDeveloperAssetAIAdvisoryInvalid)
		}
	}
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 200", ErrDeveloperAssetAIAdvisoryInvalid)
	}
	return s.store.DeveloperAssetAIAdvisoryRuns(ctx, store.DeveloperAssetAIAdvisoryQuery{DeploymentID: deployment.ID, PromptKey: promptKey, ScopeID: scopeID, Limit: limit})
}

func (s *Service) DeveloperAssetAIAdvisoryRun(ctx context.Context, id string) (model.DeveloperAssetAIAdvisoryRun, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.DeveloperAssetAIAdvisoryRun{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return model.DeveloperAssetAIAdvisoryRun{}, store.ErrNotFound
	}
	return s.store.DeveloperAssetAIAdvisoryRun(ctx, deployment.ID, id)
}
