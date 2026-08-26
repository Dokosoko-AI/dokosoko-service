package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

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
