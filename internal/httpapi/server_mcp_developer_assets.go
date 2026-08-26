package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type mcpDeveloperAssetResource struct {
	URI         string
	Name        string
	Title       string
	Description string
	MIMEType    string
	Text        string
	Meta        map[string]any
}

func developerAssetResourceURI(parts ...string) string {
	return "dokosoko://developer-assets/" + strings.Join(parts, "/")
}

func firstNonEmptyMCPDeveloperAsset(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func appendMCPDeveloperAssetResource(resources *[]mcpDeveloperAssetResource, seen map[string]bool, value mcpDeveloperAssetResource) {
	if value.URI == "" || seen[value.URI] {
		return
	}
	seen[value.URI] = true
	*resources = append(*resources, value)
}

func publicDeveloperAssetVisibilityAllowed(public bool, values ...model.Visibility) bool {
	if !public {
		return true
	}
	for _, value := range values {
		if value != model.VisibilityPublic {
			return false
		}
	}
	return true
}

func parseDeveloperAssetResourceURI(uri string) ([]string, bool) {
	const prefix = "dokosoko://developer-assets/"
	if !strings.HasPrefix(uri, prefix) {
		return nil, false
	}
	path := strings.TrimPrefix(uri, prefix)
	if path == "" || strings.ContainsAny(path, "?#\\") {
		return nil, false
	}
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.TrimSpace(part) != part {
			return nil, false
		}
	}
	return parts, true
}

func exactMCPDeveloperAssetResource(resources []mcpDeveloperAssetResource, uri string) (mcpDeveloperAssetResource, error) {
	for _, resource := range resources {
		if resource.URI == uri {
			return resource, nil
		}
	}
	return mcpDeveloperAssetResource{}, store.ErrNotFound
}

func (s *Server) exactAPIDeveloperAssetPublicationSnapshot(ctx context.Context, publication model.APIDeveloperAssetPublication) (string, model.Visibility, error) {
	revisions, err := s.service.Store().IntegrationRevisions(ctx, publication.APIID)
	if err != nil {
		return "", "", err
	}
	for _, revision := range revisions {
		if revision.ID != publication.APIRevisionID {
			continue
		}
		if revision.IntegrationID != publication.APIID || revision.State != "published" || revision.PublishedAt == nil {
			return "", "", errors.New("API developer-asset publication does not reference an exact published API revision")
		}
		var snapshot struct {
			DisplayName string           `json:"display_name"`
			Visibility  model.Visibility `json:"visibility"`
		}
		if err := json.Unmarshal(revision.Snapshot, &snapshot); err != nil {
			return "", "", fmt.Errorf("API revision snapshot is invalid: %w", err)
		}
		if snapshot.Visibility == "" {
			snapshot.Visibility = model.VisibilityPrivate
		}
		if !snapshot.Visibility.Valid() {
			return "", "", errors.New("API revision snapshot has invalid visibility")
		}
		if strings.TrimSpace(snapshot.DisplayName) == "" {
			snapshot.DisplayName = "API " + publication.APIID
		}
		return snapshot.DisplayName, snapshot.Visibility, nil
	}
	return "", "", errors.New("API developer-asset publication references an unknown API revision")
}

func mcpDeveloperAssetJSONValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func mcpDeveloperAssetEvidenceText(unit model.KnowledgeUnit, resourceURI, wrapperKind, wrapperID string) (string, error) {
	var citation any
	if len(unit.Citation) != 0 {
		if err := json.Unmarshal(unit.Citation, &citation); err != nil {
			return "", errors.New("developer-asset evidence has an invalid citation")
		}
	}
	citationJSON, err := json.MarshalIndent(citation, "", "  ")
	if err != nil {
		return "", err
	}
	title := firstNonEmptyMCPDeveloperAsset(unit.Title, unit.SourceEntityID, unit.ID)
	var text strings.Builder
	fmt.Fprintf(&text, "# %s\n\n- Evidence URI: `%s`\n- Exact %s publication: `%s`\n- Knowledge unit: `%s`\n- Evidence kind: `%s`\n- Source publication: `%s/%s`\n- Source entity: `%s`\n- Content hash: `%s`\n",
		title, resourceURI, wrapperKind, wrapperID, unit.ID, unit.Kind, unit.SourcePublicationKind, unit.SourcePublicationID, unit.SourceEntityID, unit.ContentHash)
	if len(unit.Breadcrumb) != 0 {
		fmt.Fprintf(&text, "- Breadcrumb: %s\n", strings.Join(unit.Breadcrumb, " / "))
	}
	text.WriteString("\n## Evidence\n\n")
	text.WriteString(strings.TrimSpace(unit.Content))
	text.WriteString("\n\n## Immutable citation\n\n```json\n")
	text.Write(citationJSON)
	text.WriteString("\n```\n")
	return text.String(), nil
}

// publicationScopedDeveloperAssetEvidence projects only the immutable units in
// one ready generation. API generations are additionally required to carry one
// matching API-scope row per unit, so a raw source revision or another API's
// selector can never be reached by changing an MCP URI.
func publicationScopedDeveloperAssetEvidence(record store.SearchIndexGenerationRecord, uriPrefix []string, apiID string, public bool) ([]mcpDeveloperAssetResource, string, error) {
	generation := record.Generation
	if !generation.Valid() || generation.State != "ready" || generation.UnitCount != len(record.Units) || generation.DeploymentID == "" {
		return nil, "", errors.New("developer-asset evidence generation is inconsistent")
	}
	units := append([]model.KnowledgeUnit(nil), record.Units...)
	sort.Slice(units, func(i, j int) bool {
		if units[i].Ordinal == units[j].Ordinal {
			return units[i].ID < units[j].ID
		}
		return units[i].Ordinal < units[j].Ordinal
	})
	unitIDs := make(map[string]bool, len(units))
	scopesByUnit := make(map[string][]model.KnowledgeUnitAPIScope, len(record.APIScopes))
	for _, scope := range record.APIScopes {
		if scope.DeploymentID != generation.DeploymentID || scope.KnowledgeUnitID == "" || scope.APIID == "" || scope.ScopeKind == "" {
			return nil, "", errors.New("developer-asset evidence has an invalid API scope")
		}
		scopesByUnit[scope.KnowledgeUnitID] = append(scopesByUnit[scope.KnowledgeUnitID], scope)
	}
	resources := make([]mcpDeveloperAssetResource, 0, len(units))
	var toc strings.Builder
	toc.WriteString("## Publication-scoped evidence\n\n")
	if len(units) == 0 {
		toc.WriteString("No evidence units were selected for this publication.\n")
	}
	for _, unit := range units {
		if unit.ID == "" || unitIDs[unit.ID] || unit.DeploymentID != generation.DeploymentID ||
			unit.SearchIndexGenerationID != generation.ID || unit.SourcePublicationKind == "" ||
			unit.SourcePublicationID == "" || unit.SourceEntityID == "" || strings.TrimSpace(unit.Content) == "" ||
			unit.ContentHash == "" || len(unit.Citation) == 0 || !unit.Visibility.Valid() {
			return nil, "", errors.New("developer-asset evidence unit is inconsistent")
		}
		unitIDs[unit.ID] = true
		if public && unit.Visibility != model.VisibilityPublic {
			return nil, "", errors.New("public developer-asset publication contains private evidence")
		}
		unitScopes := scopesByUnit[unit.ID]
		var scope *model.KnowledgeUnitAPIScope
		if apiID == "" {
			if len(unitScopes) != 0 {
				return nil, "", errors.New("global developer-asset evidence unexpectedly has an API scope")
			}
		} else {
			if len(unitScopes) != 1 || unitScopes[0].APIID != apiID {
				return nil, "", errors.New("API developer-asset evidence is outside its exact API scope")
			}
			scope = &unitScopes[0]
		}
		metadata := make(map[string]any)
		if len(unit.Metadata) != 0 {
			if err := json.Unmarshal(unit.Metadata, &metadata); err != nil {
				return nil, "", errors.New("developer-asset evidence has invalid metadata")
			}
			if metadata == nil {
				return nil, "", errors.New("developer-asset evidence has invalid metadata")
			}
		}
		citation, err := mcpDeveloperAssetJSONValue(unit.Citation)
		if err != nil {
			return nil, "", errors.New("developer-asset evidence has an invalid citation")
		}
		if _, ok := citation.(map[string]any); !ok {
			return nil, "", errors.New("developer-asset evidence has an invalid citation")
		}
		assetKind, _ := metadata["asset_kind"].(string)
		assetKind = firstNonEmptyMCPDeveloperAsset(assetKind, unit.SourcePublicationKind)
		parts := append(append([]string(nil), uriPrefix...), "evidence", unit.ID)
		uri := developerAssetResourceURI(parts...)
		metadata["knowledge_unit_id"] = unit.ID
		metadata["search_index_generation_id"] = generation.ID
		metadata["index_builder_version"] = generation.BuilderVersion
		metadata["retrieval_profile_version"] = generation.RetrievalProfileVersion
		metadata["source_publication_kind"] = unit.SourcePublicationKind
		metadata["source_publication_id"] = unit.SourcePublicationID
		metadata["source_entity_id"] = unit.SourceEntityID
		metadata["content_hash"] = unit.ContentHash
		metadata["citation"] = citation
		if scope != nil {
			metadata["api_id"] = scope.APIID
			metadata["api_scope_kind"] = scope.ScopeKind
			metadata["api_selector_hash"] = scope.SelectorHash
			metadata["api_sdk_binding_id"] = scope.APISDKBindingID
		}
		text, err := mcpDeveloperAssetEvidenceText(unit, uri, generation.PublicationKind, generation.PublicationID)
		if err != nil {
			return nil, "", err
		}
		title := firstNonEmptyMCPDeveloperAsset(unit.Title, unit.SourceEntityID, unit.ID)
		resources = append(resources, mcpDeveloperAssetResource{
			URI: uri, Name: "developer-asset-evidence-" + unit.ID, Title: title,
			Description: "Immutable " + assetKind + " evidence selected for one exact published scope.",
			MIMEType:    "text/markdown", Text: text, Meta: metadata,
		})
		breadcrumb := strings.Join(unit.Breadcrumb, " / ")
		if breadcrumb != "" {
			breadcrumb = " — " + breadcrumb
		}
		fmt.Fprintf(&toc, "- `%s` — %s%s — `%s`\n", unit.Kind, title, breadcrumb, uri)
	}
	for unitID := range scopesByUnit {
		if !unitIDs[unitID] {
			return nil, "", errors.New("developer-asset API scope references unknown evidence")
		}
	}
	return resources, toc.String(), nil
}

func (s *Server) exactGlobalDocumentationPublicationResources(ctx context.Context, deploymentID string, publication model.DeploymentDocumentationPublication, public bool) ([]mcpDeveloperAssetResource, error) {
	if publication.DeploymentID != deploymentID || !publicDeveloperAssetVisibilityAllowed(public, publication.Visibility) {
		return nil, store.ErrNotFound
	}
	record, err := s.service.ReadyDeveloperAssetSearchIndex(ctx, "global_documentation", publication.ID)
	if err != nil {
		return nil, err
	}
	evidence, toc, err := publicationScopedDeveloperAssetEvidence(record, []string{"global-documentation", publication.ID}, "", public)
	if err != nil {
		return nil, err
	}
	var text strings.Builder
	fmt.Fprintf(&text, "# Global Documentation Publication\n\n- Exact publication: `%s`\n- Revision: `%d`\n- Snapshot hash: `%s`\n- Exact index generation: `%s`\n\n%s", publication.ID, publication.Revision, publication.SnapshotHash, record.Generation.ID, toc)
	root := mcpDeveloperAssetResource{
		URI: developerAssetResourceURI("global-documentation", publication.ID), Name: "global-documentation-" + publication.ID,
		Title: "Global documentation publication", Description: "The exact deployment-wide documentation snapshot and its publication-scoped evidence.",
		MIMEType: "text/markdown", Text: text.String(), Meta: map[string]any{
			"asset_kind": "documentation", "global_documentation_publication_id": publication.ID,
			"revision": publication.Revision, "snapshot_hash": publication.SnapshotHash,
			"search_index_generation_id": record.Generation.ID,
		},
	}
	return append(evidence, root), nil
}

func (s *Server) exactGlobalDocumentationPublicationResource(ctx context.Context, deploymentID string, publication model.DeploymentDocumentationPublication, public bool) (mcpDeveloperAssetResource, error) {
	resources, err := s.exactGlobalDocumentationPublicationResources(ctx, deploymentID, publication, public)
	if err != nil {
		return mcpDeveloperAssetResource{}, err
	}
	return exactMCPDeveloperAssetResource(resources, developerAssetResourceURI("global-documentation", publication.ID))
}

func (s *Server) exactAPIDeveloperAssetPublicationResources(ctx context.Context, deploymentID, apiID string, publication model.APIDeveloperAssetPublication, public bool) ([]mcpDeveloperAssetResource, error) {
	if publication.DeploymentID != deploymentID || publication.APIID != apiID {
		return nil, store.ErrNotFound
	}
	displayName, apiVisibility, err := s.exactAPIDeveloperAssetPublicationSnapshot(ctx, publication)
	if err != nil {
		return nil, err
	}
	if !publicDeveloperAssetVisibilityAllowed(public, apiVisibility) {
		return nil, store.ErrNotFound
	}
	record, err := s.service.ReadyDeveloperAssetSearchIndex(ctx, "api", publication.ID)
	if err != nil {
		return nil, err
	}
	evidence, toc, err := publicationScopedDeveloperAssetEvidence(record, []string{"apis", publication.APIID, "publications", publication.ID}, publication.APIID, public)
	if err != nil {
		return nil, err
	}
	var text strings.Builder
	fmt.Fprintf(&text, "# %s Developer Assets\n\n- API ID: `%s`\n- Exact asset publication: `%s`\n- Snapshot hash: `%s`\n- Global documentation publication: `%s`\n- Exact index generation: `%s`\n\n%s", displayName, publication.APIID, publication.ID, publication.SnapshotHash, publication.DeploymentDocumentationPublicationID, record.Generation.ID, toc)
	meta := map[string]any{
		"api_id": publication.APIID, "api_developer_asset_publication_id": publication.ID,
		"api_snapshot_hash": publication.SnapshotHash, "search_index_generation_id": record.Generation.ID,
	}
	root := mcpDeveloperAssetResource{
		URI: developerAssetResourceURI("apis", publication.APIID, "publications", publication.ID), Name: "api-assets-" + publication.ID,
		Title: displayName + " developer assets", Description: "Exact API-scoped documentation, contracts, and SDK evidence selected by this publication.",
		MIMEType: "text/markdown", Text: text.String(), Meta: meta,
	}
	return append(evidence, root), nil
}

func (s *Server) exactAPIDeveloperAssetPublicationResource(ctx context.Context, deploymentID, apiID string, publication model.APIDeveloperAssetPublication, public bool) (mcpDeveloperAssetResource, error) {
	resources, err := s.exactAPIDeveloperAssetPublicationResources(ctx, deploymentID, apiID, publication, public)
	if err != nil {
		return mcpDeveloperAssetResource{}, err
	}
	return exactMCPDeveloperAssetResource(resources, developerAssetResourceURI("apis", publication.APIID, "publications", publication.ID))
}

// exactPublishedDeveloperAssetResource resolves immutable resource identifiers
// directly. Discovery intentionally remains current-only, while an exact URI
// that a client retained continues to resolve against its historical
// publication's activated, ready index generation.
func (s *Server) exactPublishedDeveloperAssetResource(ctx context.Context, deploymentID, uri string, public bool) (mcpDeveloperAssetResource, error) {
	parts, ok := parseDeveloperAssetResourceURI(uri)
	if !ok {
		return mcpDeveloperAssetResource{}, store.ErrNotFound
	}
	switch {
	case len(parts) == 2 && parts[0] == "global-documentation":
		publication, err := s.service.Store().DeploymentDocumentationPublication(ctx, deploymentID, parts[1])
		if err != nil {
			return mcpDeveloperAssetResource{}, err
		}
		return s.exactGlobalDocumentationPublicationResource(ctx, deploymentID, publication, public)
	case len(parts) == 4 && parts[0] == "apis" && parts[2] == "publications":
		publication, err := s.service.Store().APIDeveloperAssetPublication(ctx, deploymentID, parts[3])
		if err != nil {
			return mcpDeveloperAssetResource{}, err
		}
		return s.exactAPIDeveloperAssetPublicationResource(ctx, deploymentID, parts[1], publication, public)
	case len(parts) == 4 && parts[0] == "global-documentation" && parts[2] == "evidence":
		publication, err := s.service.Store().DeploymentDocumentationPublication(ctx, deploymentID, parts[1])
		if err != nil {
			return mcpDeveloperAssetResource{}, err
		}
		resources, err := s.exactGlobalDocumentationPublicationResources(ctx, deploymentID, publication, public)
		if err != nil {
			return mcpDeveloperAssetResource{}, err
		}
		return exactMCPDeveloperAssetResource(resources, uri)
	case len(parts) == 6 && parts[0] == "apis" && parts[2] == "publications" && parts[4] == "evidence":
		publication, err := s.service.Store().APIDeveloperAssetPublication(ctx, deploymentID, parts[3])
		if err != nil {
			return mcpDeveloperAssetResource{}, err
		}
		resources, err := s.exactAPIDeveloperAssetPublicationResources(ctx, deploymentID, parts[1], publication, public)
		if err != nil {
			return mcpDeveloperAssetResource{}, err
		}
		return exactMCPDeveloperAssetResource(resources, uri)
	default:
		return mcpDeveloperAssetResource{}, store.ErrNotFound
	}
}

func (s *Server) publishedDeveloperAssetResources(ctx context.Context, productID string, public bool, manifest model.ProductManifest) ([]mcpDeveloperAssetResource, error) {
	deploymentID := manifest.DeploymentID
	if deploymentID == "" {
		deploymentID = productID
	}
	resources := make([]mcpDeveloperAssetResource, 0)
	seen := make(map[string]bool)
	if global, err := s.service.ReadyDeploymentDocumentationPublication(ctx); err == nil {
		if publicDeveloperAssetVisibilityAllowed(public, global.Visibility) {
			values, lookupErr := s.exactGlobalDocumentationPublicationResources(ctx, deploymentID, global, public)
			if lookupErr != nil {
				return nil, lookupErr
			}
			for _, value := range values {
				appendMCPDeveloperAssetResource(&resources, seen, value)
			}
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	for _, api := range manifest.Integrations {
		if public && api.Visibility != model.VisibilityPublic {
			continue
		}
		publication, err := s.service.ReadyAPIDeveloperAssetPublication(ctx, api.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return nil, err
		}
		values, lookupErr := s.exactAPIDeveloperAssetPublicationResources(ctx, deploymentID, api.ID, publication, public)
		if lookupErr != nil {
			return nil, lookupErr
		}
		for _, value := range values {
			appendMCPDeveloperAssetResource(&resources, seen, value)
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].URI < resources[j].URI })
	return resources, nil
}

func mcpDeveloperAssetSearchInputSchema() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"query":               map[string]any{"type": "string", "minLength": 1, "maxLength": 500},
			"scope":               map[string]any{"type": "string", "enum": []string{"global", "api", "combined"}},
			"api_id":              map[string]any{"type": "string", "minLength": 1, "maxLength": 100},
			"asset_kinds":         map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{"type": "string", "enum": []string{"documentation", "contract", "sdk"}}},
			"languages":           map[string]any{"type": "array", "maxItems": 20, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 100}},
			"ecosystems":          map[string]any{"type": "array", "maxItems": 20, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 100}},
			"sdk_release_ids":     map[string]any{"type": "array", "maxItems": 50, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 100}},
			"exact_versions":      map[string]any{"type": "array", "maxItems": 50, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 120}},
			"limit":               map[string]any{"type": "integer", "minimum": 1, "maximum": 20},
			"context_token_limit": map[string]any{"type": "integer", "minimum": 256, "maximum": 16000},
		},
		"required": []string{"query"},
	}
}

func manifestAllowsDeveloperAssetAPI(manifest model.ProductManifest, apiID string, public bool) bool {
	for _, api := range manifest.Integrations {
		if api.ID == apiID && (!public || api.Visibility == model.VisibilityPublic) {
			return true
		}
	}
	return false
}

func (s *Server) callDeveloperAssetSearch(ctx context.Context, input platform.DeveloperAssetQueryLabInput, public bool, manifest model.ProductManifest) (platform.DeveloperAssetQueryLabResponse, error) {
	input.DeploymentDocumentationPublicationID = ""
	input.APIDeveloperAssetPublicationID = ""
	input.APIID = strings.TrimSpace(input.APIID)
	input.Scope = strings.ToLower(strings.TrimSpace(input.Scope))
	if input.Scope == "api" || input.Scope == "combined" || input.APIID != "" {
		if input.APIID == "" || !manifestAllowsDeveloperAssetAPI(manifest, input.APIID, public) {
			return platform.DeveloperAssetQueryLabResponse{}, errors.New("selected API is not available in this MCP catalog scope")
		}
	}
	effectiveScope := input.Scope
	if effectiveScope == "" {
		if input.APIID == "" {
			effectiveScope = "global"
		} else {
			effectiveScope = "combined"
		}
	}
	// Public combined search uses the independently published active global
	// documentation snapshot as well as the selected API snapshot. Check both
	// sides of that scope before Query Lab resolves IDs so a private global head
	// can never leak through an otherwise-public API.
	if public && (effectiveScope == "global" || effectiveScope == "combined") {
		deploymentID := manifest.DeploymentID
		if deploymentID == "" {
			deploymentID = manifest.ProductID
		}
		global, err := s.service.ReadyDeploymentDocumentationPublication(ctx)
		if err != nil || global.Visibility != model.VisibilityPublic {
			return platform.DeveloperAssetQueryLabResponse{}, errors.New("public global documentation is unavailable")
		}
	}
	return s.service.RunDeveloperAssetQueryLab(ctx, input)
}
