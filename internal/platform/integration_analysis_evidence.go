package platform

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type integrationSourceExcerpt struct {
	Text       string
	References []model.RecipeReference
}

// evidenceText keeps operator-authored prose inside one canonical evidence
// field. Deterministic recipe code parses only server-emitted field lines; a
// newline in a description must never be able to introduce a forged field such
// as "Fixed endpoint:" or "Required grants:" ahead of the authoritative one.
func evidenceText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func recipeReferenceKind(label, location string) string {
	if strings.Contains(strings.ToLower(label), "code") || strings.Contains(strings.ToLower(label), "sample") || strings.Contains(strings.ToLower(location), "github.com") {
		return "code"
	}
	return "documentation"
}

func integrationSourceExcerpts(records []model.KnowledgeRecord) map[string]integrationSourceExcerpt {
	sort.Slice(records, func(i, j int) bool {
		if records[i].SourceID == records[j].SourceID {
			if records[i].Title == records[j].Title {
				return records[i].ID < records[j].ID
			}
			return records[i].Title < records[j].Title
		}
		return records[i].SourceID < records[j].SourceID
	})
	result := make(map[string]integrationSourceExcerpt)
	documentsBySource := make(map[string]int)
	totalRunes := 0
	for _, record := range records {
		if !record.Published || record.SourceID == "" || documentsBySource[record.SourceID] >= maxAnalysisDocumentsPerSource || totalRunes >= maxAnalysisKnowledgeRunes {
			continue
		}
		separator := ""
		current := result[record.SourceID]
		if current.Text != "" {
			separator = "\n\n"
		}
		separatorRunes := len([]rune(separator))
		remainingSource := maxAnalysisSourceExcerptRunes - len([]rune(current.Text)) - separatorRunes
		remainingTotal := maxAnalysisKnowledgeRunes - totalRunes - separatorRunes
		limit := min(maxAnalysisDocumentRunes, remainingSource, remainingTotal)
		if limit <= 0 {
			continue
		}
		header := "Document: " + truncateRunes(record.Title, 240)
		if record.URL != "" {
			header += "\nCanonical URL: " + truncateRunes(record.URL, 500)
		}
		chunk := truncateRunes(header+"\nExcerpt:\n"+record.Text, limit)
		if chunk == "" {
			continue
		}
		current.Text += separator + chunk
		if parsed, err := url.Parse(record.URL); err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil {
			current.References = append(current.References, model.RecipeReference{Label: firstNonEmpty(strings.TrimSpace(record.Title), record.URL), URL: record.URL, Kind: recipeReferenceKind(record.Title, record.URL), ResourceID: record.ID})
		}
		result[record.SourceID] = current
		documentsBySource[record.SourceID]++
		totalRunes += separatorRunes + len([]rune(chunk))
	}
	return result
}

func integrationCatalogExcerpt(value model.Integration, limit int) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Family: %s\nVersion: %s\nLifecycle: %s", value.FamilyKey, value.VersionKey, value.Lifecycle)
	if strings.TrimSpace(value.Description) != "" {
		fmt.Fprintf(&builder, "\nDescription: %s", evidenceText(value.Description))
	}
	for _, resource := range value.Resources {
		fmt.Fprintf(&builder, "\n\nResource: %s (%s)", resource.Name, resource.Kind)
		if resource.ResolvedRevision != nil && len(resource.ResolvedRevision.Manifest) > 0 {
			fmt.Fprintf(&builder, "\nManifest: %s", resource.ResolvedRevision.Manifest)
		}
	}
	return truncateRunes(builder.String(), limit)
}

func toolCatalogExcerpt(value model.Tool, limit int) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Description: %s\nScope: %s", evidenceText(value.Description), value.Scope)
	if value.OwnerIntegrationID != "" {
		fmt.Fprintf(&builder, "\nOwner integration ID: %s", value.OwnerIntegrationID)
	}
	fmt.Fprintf(&builder, "\nBackend: %s\nUpstream drifted: %t\nMethod: %s", value.BackendKind, value.UpstreamDrifted, value.HTTPMethod)
	if endpoint := integrationRecipeToolEndpoint(value); endpoint != "" {
		fmt.Fprintf(&builder, "\nFixed endpoint: %s", endpoint)
	}
	if len(value.InputSchema) > 0 {
		fmt.Fprintf(&builder, "\nInput schema: %s", value.InputSchema)
	}
	if len(value.OutputSchema) > 0 {
		fmt.Fprintf(&builder, "\nOutput schema: %s", value.OutputSchema)
	}
	if len(value.AuthorizationPolicy) > 0 {
		fmt.Fprintf(&builder, "\nAuthorization policy: %s", value.AuthorizationPolicy)
	}
	return truncateRunes(builder.String(), limit)
}

// integrationRecipeToolEndpoint returns an endpoint only when the exact
// published tool revision establishes one unambiguous product endpoint. A
// runtime-backed tool pins one target revision per environment; choosing the
// first of several different endpoints would turn an environment detail into
// a false product fact, so those tools require AI-authored, environment-aware
// instructions (or a reviewed API contract) instead of a deterministic seed.
func integrationRecipeToolEndpoint(value model.Tool) string {
	if value.BackendKind != "http" {
		return ""
	}
	if endpoint := strings.TrimSpace(value.BaseURL); endpoint != "" {
		return endpoint
	}
	if value.RuntimeServiceConnectionID == "" || value.HTTPPath == "" || len(value.RuntimeTargets) == 0 {
		return ""
	}
	endpoint := ""
	for _, target := range value.RuntimeTargets {
		if target.RuntimeServiceConnectionID != value.RuntimeServiceConnectionID || target.ConnectionRevisionID == "" {
			return ""
		}
		candidate, err := composeRuntimeToolEndpoint(target.BaseURL, value.HTTPPath)
		if err != nil || endpoint != "" && candidate != endpoint {
			return ""
		}
		endpoint = candidate
	}
	return endpoint
}

func recipeToolLabel(value model.Tool, integration *model.Integration) string {
	switch value.Scope {
	case model.ToolScopeCommon:
		return "common." + value.Name
	case model.ToolScopeAPI:
		if integration != nil && integration.ID == value.OwnerIntegrationID {
			return integration.FamilyKey + ".custom." + value.Name
		}
	}
	return value.Namespace + "." + value.Name
}

func integrationScopeID(evidence []model.IntegrationEvidence) (string, bool) {
	for _, item := range evidence {
		if item.Kind == integrationScopeEvidenceKind && strings.TrimSpace(item.ResourceID) != "" {
			return item.ResourceID, true
		}
	}
	return "", false
}

func integrationRecipePrefix(product model.Product, integration model.Integration) string {
	prefix := slugify(strings.Join([]string{product.Slug, integration.FamilyKey, integration.VersionKey}, "-"))
	if prefix == "" {
		prefix = slugify(integration.ID)
	}
	return prefix
}

func scopedResourceExcerpt(link model.IntegrationResourceLink, limit int) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Kind: %s\nBinding: %s", link.Kind, map[bool]string{true: "follow latest", false: "pinned exact revision"}[link.FollowLatest])
	if link.ResolvedRevision != nil {
		fmt.Fprintf(&builder, "\nRevision: %d\nRevision ID: %s\nContent hash: %s", link.ResolvedRevision.Revision, link.ResolvedRevision.ID, link.ResolvedRevision.ContentHash)
		if len(link.ResolvedRevision.Manifest) > 0 {
			fmt.Fprintf(&builder, "\nManifest: %s", link.ResolvedRevision.Manifest)
		}
	}
	return truncateRunes(builder.String(), limit)
}

func scopedSDKExcerpt(reference model.SDKReference, limit int) string {
	return truncateRunes(fmt.Sprintf("Ecosystem: %s\nCoordinate: %s\nExact version: %s\nInstall: %s\nDocumentation: %s\nSource: %s\nChecksum: %s", reference.Ecosystem, reference.Coordinate, reference.ExactVersion, reference.InstallCommand, reference.DocumentationURL, reference.SourceURL, reference.Checksum), limit)
}

func scopedAuthorizationExcerpt(point model.AuthorizationPoint, limit int) string {
	return truncateRunes(fmt.Sprintf("Description: %s\nAction: %s\nRequired grants: %s\nConfirmation required: %t\nDecision TTL seconds: %d\nState: %s", evidenceText(point.Description), point.ActionType, strings.Join(point.RequiredGrants, ", "), point.ConfirmationRequired, point.DecisionTTLSeconds, point.State), limit)
}

func (s *Service) scopedIntegrationEvidence(ctx context.Context, product model.Product, integrationID string) ([]model.IntegrationEvidence, model.Integration, error) {
	integrationID = strings.TrimSpace(integrationID)
	integration, err := s.store.Integration(ctx, product.ID, integrationID)
	if err != nil {
		return nil, model.Integration{}, err
	}
	if integration.Lifecycle == "retired" {
		return nil, model.Integration{}, ErrRecipeNeedsInput
	}
	manifest, err := s.ProductManifestFor(ctx, product.ID, model.CatalogScope{})
	if err != nil {
		return nil, model.Integration{}, err
	}
	version := strconv.FormatInt(integration.Revision, 10)
	values := []model.IntegrationEvidence{
		{Kind: integrationScopeEvidenceKind, ResourceID: integration.ID, Label: integration.DisplayName, Version: version, Visibility: model.VisibilityPrivate, Fingerprint: evidenceFingerprint(integrationScopeEvidenceKind, integration.ID, version)},
		{Kind: "integration", ResourceID: integration.ID, Label: integration.DisplayName, Excerpt: integrationCatalogExcerpt(integration, maxAnalysisIntegrationItem), Version: version, Visibility: integration.Visibility, Fingerprint: evidenceFingerprint("integration", integration.ID, version, integration.FamilyKey, integration.VersionKey, integration.Description)},
	}
	provider, providerErr := s.store.IdentityProvider(ctx, product.ID)
	if providerErr == nil && provider.State == "active" {
		providerVersion := strconv.FormatInt(provider.Revision, 10)
		providerExcerpt := truncateRunes(fmt.Sprintf("Issuer: %s\nAudience: %s\nOAuth resource: %s\nScopes: %s\nCustomer account claim: %s\nInstallation claim: %s\nAuthorization API origin: %s\nState: %s", provider.Issuer, provider.Audience, provider.OAuthResource, strings.Join(provider.Scopes, ", "), provider.OrganisationClaim, provider.InstallationClaim, provider.DelegatedAPIOrigin, provider.State), maxAnalysisToolItem)
		values = append(values, model.IntegrationEvidence{Kind: "identity_provider", ResourceID: provider.ID, Label: "Customer identity boundary", Excerpt: providerExcerpt, Version: providerVersion, Visibility: model.VisibilityPrivate, Fingerprint: evidenceFingerprint("identity_provider", provider.ID, providerVersion, providerExcerpt)})
	} else if providerErr != nil && !errors.Is(providerErr, store.ErrNotFound) {
		return nil, model.Integration{}, providerErr
	}
	resourceRunes := 0
	publicationNames := make(map[string]string)
	for _, link := range integration.Resources {
		remaining := maxAnalysisIntegrationRunes - resourceRunes
		if remaining <= 0 {
			break
		}
		excerpt := scopedResourceExcerpt(link, min(maxAnalysisIntegrationItem, remaining))
		resourceRunes += len([]rune(excerpt))
		resolvedVersion := "unresolved"
		fingerprintValues := []string{"resource_set", integration.ID, link.ResourceSetID, link.PinnedRevisionID, strconv.FormatBool(link.FollowLatest), excerpt}
		if link.ResolvedRevision != nil {
			resolvedVersion = strconv.FormatInt(link.ResolvedRevision.Revision, 10)
			fingerprintValues = append(fingerprintValues, link.ResolvedRevision.ID, link.ResolvedRevision.ContentHash)
		}
		values = append(values, model.IntegrationEvidence{Kind: "resource_set", ResourceID: link.ResourceSetID, Label: link.Name, Excerpt: excerpt, Version: resolvedVersion, Visibility: integration.Visibility, Fingerprint: evidenceFingerprint(fingerprintValues...)})
		if link.Kind == "documentation" && link.ResolvedRevision != nil {
			entries, parseErr := parseDocumentationManifest(link.ResolvedRevision.Manifest)
			if parseErr != nil {
				return nil, model.Integration{}, fmt.Errorf("resolve reviewed documentation evidence: %w", parseErr)
			}
			for _, entry := range entries {
				publicationNames[entry.SourcePublicationID] = entry.Name
			}
		}
	}
	publicationIDs := make([]string, 0, len(publicationNames))
	for publicationID := range publicationNames {
		publicationIDs = append(publicationIDs, publicationID)
	}
	sort.Strings(publicationIDs)
	documentRunes := 0
	for _, publicationID := range publicationIDs {
		remaining := maxAnalysisScopedDocumentRunes - documentRunes
		if remaining <= 0 {
			break
		}
		publication, publicationErr := s.store.SourcePublication(ctx, product.ID, publicationID)
		if publicationErr != nil {
			return nil, model.Integration{}, publicationErr
		}
		records, knowledgeErr := s.store.PrivateKnowledge(ctx, product.ID, []string{publication.ID}, "")
		if knowledgeErr != nil && !errors.Is(knowledgeErr, store.ErrNotFound) {
			return nil, model.Integration{}, knowledgeErr
		}
		excerpt := integrationSourceExcerpts(records)[publication.SourceID]
		excerpt.Text = truncateRunes(excerpt.Text, remaining)
		documentRunes += len([]rune(excerpt.Text))
		label, location, visibility := publicationNames[publication.ID], "", publication.Visibility
		if source, sourceErr := s.store.Source(ctx, product.ID, publication.SourceID); sourceErr == nil {
			label, location = firstNonEmpty(label, source.Name), source.Location
			if source.Visibility == model.VisibilityPrivate {
				visibility = model.VisibilityPrivate
			}
		} else if !errors.Is(sourceErr, store.ErrNotFound) {
			return nil, model.Integration{}, sourceErr
		}
		values = append(values, model.IntegrationEvidence{Kind: "source_publication", ResourceID: publication.ID, Label: firstNonEmpty(label, publication.SourceID), Location: location, Excerpt: excerpt.Text, References: excerpt.References, Version: strconv.FormatInt(publication.Revision, 10), Visibility: visibility, Fingerprint: evidenceFingerprint("source_publication", publication.ID, publication.SourceID, publication.ContentHash, excerpt.Text)})
	}
	for _, reference := range integration.SDKs {
		excerpt := scopedSDKExcerpt(reference, maxAnalysisToolItem)
		values = append(values, model.IntegrationEvidence{Kind: "sdk", ResourceID: reference.ID, Label: reference.Coordinate, Excerpt: excerpt, Version: reference.ExactVersion, Visibility: reference.Visibility, Fingerprint: evidenceFingerprint("sdk", integration.ID, reference.ID, strconv.FormatInt(reference.Revision, 10), excerpt)})
	}
	points, err := s.store.AuthorizationPoints(ctx, integration.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, model.Integration{}, err
	}
	for _, point := range points {
		pointVersion := strconv.FormatInt(point.Revision, 10)
		excerpt := scopedAuthorizationExcerpt(point, maxAnalysisToolItem)
		values = append(values, model.IntegrationEvidence{Kind: "authorization_point", ResourceID: point.ID, Label: point.Key, Excerpt: excerpt, Version: pointVersion, Visibility: model.VisibilityPrivate, Fingerprint: evidenceFingerprint("authorization_point", integration.ID, point.ID, pointVersion, excerpt)})
	}
	grants, err := s.store.GrantDefinitions(ctx, product.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, model.Integration{}, err
	}
	bindings, err := s.store.IntegrationToolBindings(ctx, integration.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, model.Integration{}, err
	}
	toolRunes := 0
	for _, binding := range bindings {
		remaining := maxAnalysisToolRunes - toolRunes
		if remaining <= 0 {
			break
		}
		if binding.Tool == nil || binding.Tool.Revision != binding.ToolRevision || binding.Tool.Scope != model.ToolScopeAPI || binding.Tool.OwnerIntegrationID != integration.ID {
			continue
		}
		tool, lookupErr := s.store.Tool(ctx, product.ID, binding.ToolID)
		if lookupErr != nil {
			if errors.Is(lookupErr, store.ErrNotFound) {
				continue
			}
			return nil, model.Integration{}, lookupErr
		}
		if tool.Revision != binding.ToolRevision || tool.Scope != model.ToolScopeAPI || tool.OwnerIntegrationID != integration.ID {
			continue
		}
		toolVersion := strconv.FormatInt(binding.ToolRevision, 10)
		label := recipeToolLabel(tool, &integration)
		toolFacts := "Exact bound tool revision: " + toolVersion + "\n" + toolCatalogExcerpt(tool, min(maxAnalysisToolItem, remaining))
		if tool.BackendKind == "mcp" {
			if identity, exposed := canonicalMCPToolIdentityFor(manifest, tool); exposed && currentMCPToolAuthorization(identity, points, grants) {
				toolFacts = "Exact bound tool revision: " + toolVersion + "\nMCP tool name: " + identity.name + "\n" + toolCatalogExcerpt(tool, min(maxAnalysisToolItem, remaining))
			}
		}
		excerpt := truncateRunes(toolFacts, min(maxAnalysisToolItem, remaining))
		toolRunes += len([]rune(excerpt))
		values = append(values, model.IntegrationEvidence{Kind: "tool", ResourceID: binding.ToolID, Label: label, Excerpt: excerpt, Version: toolVersion, Visibility: integration.Visibility, Fingerprint: evidenceFingerprint("tool_binding", integration.ID, binding.ToolID, toolVersion, string(integration.Visibility), excerpt)})
	}
	developerAssets, err := s.scopedRecipeDeveloperAssetEvidence(ctx, integration, scopedRecipeDeveloperAssetQuery(integration, values))
	if err != nil {
		return nil, model.Integration{}, err
	}
	values = append(values, developerAssets...)
	sort.Slice(values, func(i, j int) bool {
		if values[i].Kind == values[j].Kind {
			if values[i].Label == values[j].Label {
				return values[i].ResourceID < values[j].ResourceID
			}
			return values[i].Label < values[j].Label
		}
		return values[i].Kind < values[j].Kind
	})
	return values, integration, nil
}

func (s *Service) integrationEvidence(ctx context.Context, product model.Product) ([]model.IntegrationEvidence, error) {
	values := make([]model.IntegrationEvidence, 0)
	publicationIDs, err := s.latestSourcePublicationIDs(ctx, product.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	knowledge, err := s.store.PrivateKnowledge(ctx, product.ID, publicationIDs, "")
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	excerpts := integrationSourceExcerpts(knowledge)
	sources, err := s.store.Sources(ctx, product.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	for _, source := range sources {
		version := strconv.FormatInt(source.Revision, 10)
		excerpt := excerpts[source.ID]
		values = append(values, model.IntegrationEvidence{Kind: "source", ResourceID: source.ID, Label: source.Name, Location: source.Location, Excerpt: excerpt.Text, References: excerpt.References, Version: version, Visibility: source.Visibility, Fingerprint: evidenceFingerprint("source", source.ID, version, source.Location, excerpt.Text)})
	}
	integrations, err := s.store.Integrations(ctx, product.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	integrationRunes := 0
	for _, integration := range integrations {
		version := strconv.FormatInt(integration.Revision, 10)
		excerpt := integrationCatalogExcerpt(integration, min(maxAnalysisIntegrationItem, maxAnalysisIntegrationRunes-integrationRunes))
		integrationRunes += len([]rune(excerpt))
		values = append(values, model.IntegrationEvidence{Kind: "integration", ResourceID: integration.ID, Label: integration.DisplayName, Excerpt: excerpt, Version: version, Visibility: integration.Visibility, Fingerprint: evidenceFingerprint("integration", integration.ID, version, excerpt)})
	}
	tools, err := s.store.Tools(ctx, product.ID, false)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	toolRunes := 0
	for _, tool := range tools {
		version := strconv.FormatInt(tool.Revision, 10)
		excerpt := toolCatalogExcerpt(tool, min(maxAnalysisToolItem, maxAnalysisToolRunes-toolRunes))
		toolRunes += len([]rune(excerpt))
		values = append(values, model.IntegrationEvidence{Kind: "tool", ResourceID: tool.ID, Label: recipeToolLabel(tool, nil), Excerpt: excerpt, Version: version, Visibility: model.VisibilityPrivate, Fingerprint: evidenceFingerprint("tool", tool.ID, version, excerpt)})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Kind == values[j].Kind {
			return values[i].Label < values[j].Label
		}
		return values[i].Kind < values[j].Kind
	})
	return values, nil
}

func (s *Service) latestSourcePublicationIDs(ctx context.Context, productID string) ([]string, error) {
	sources, err := s.store.Sources(ctx, productID)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(sources))
	for _, source := range sources {
		publications, publicationErr := s.store.SourcePublications(ctx, productID, source.ID)
		if publicationErr != nil {
			if errors.Is(publicationErr, store.ErrNotFound) {
				continue
			}
			return nil, publicationErr
		}
		if len(publications) > 0 {
			result = append(result, publications[0].ID)
		}
	}
	sort.Strings(result)
	return result, nil
}

func evidenceSourcePublicationIDs(evidence []model.IntegrationEvidence) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, item := range evidence {
		if item.Kind != "source_publication" {
			continue
		}
		id := strings.TrimSpace(item.ResourceID)
		if id != "" && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}

func (s *Service) deterministicIntegrationPlan(ctx context.Context, product model.Product, evidence []model.IntegrationEvidence, integration *model.Integration) (model.IntegrationPlan, []model.IntegrationUnknown) {
	subjectName := product.Name
	if integration != nil {
		subjectName = integration.DisplayName
	}
	plan := model.IntegrationPlan{Summary: "Review the exact product evidence for " + subjectName + " and identify independently implementable capabilities."}
	provider, err := s.store.IdentityProvider(ctx, product.ID)
	if err == nil && provider.State == "active" {
		plan.Identity = model.IntegrationIdentityPlan{Mode: "oauth2", Issuer: provider.Issuer, Audience: provider.Audience, Explanation: "DokoSoko brokers customer sign-in through the configured OIDC provider and keeps vendor access tokens out of MCP clients."}
	} else {
		plan.Identity = model.IntegrationIdentityPlan{Mode: "none", Explanation: "Public discovery can work without identity. Configure OIDC before exposing customer-specific data or actions."}
	}
	plan.Endpoints = []model.IntegrationEndpointPlan{
		{Name: "mcp", Method: "POST", Path: "/mcp", Purpose: "Private MCP discovery and tool execution.", Identity: "oauth2", Evidence: evidenceIDs(evidence)},
	}
	if product.PublicMCPEnabled {
		plan.Endpoints = append(plan.Endpoints, model.IntegrationEndpointPlan{Name: "public-mcp", Method: "POST", Path: "/mcp/public", Purpose: "Anonymous access to explicitly public recipes and knowledge.", Identity: "none", Evidence: evidenceIDs(evidence)})
	}
	if plan.Identity.Mode == "oauth2" {
		plan.Endpoints = append(plan.Endpoints, model.IntegrationEndpointPlan{Name: "access-evaluation", Method: "POST", Path: "/v1/access/evaluations", Purpose: "Resolve the authenticated customer to bounded grants before private authorization.", Identity: "oauth2", Evidence: evidenceIDs(evidence)})
	}
	if integration != nil {
		plan.Recipes = deterministicIntegrationRecipeSeeds(product, *integration, evidence)
		if len(plan.Recipes) > 0 {
			plan.Summary = fmt.Sprintf("Reviewed evidence supports %d independently implementable product capability recipe candidate(s) for %s.", len(plan.Recipes), subjectName)
		}
	}
	unknowns := make([]model.IntegrationUnknown, 0)
	if plan.Identity.Mode == "none" {
		unknowns = append(unknowns, model.IntegrationUnknown{ID: "private-access", Question: "Will developers access customer-specific data or perform actions?", Why: "Private operations require an identity boundary and explicit grants; public MCP must remain read-only and deliberately published.", Blocking: false})
	}
	if integration == nil {
		unknowns = append(unknowns, model.IntegrationUnknown{ID: "integration-scope", Question: "Which product API should the recipe implement?", Why: "A product-integration recipe must be bound to one selected API and its exact reviewed evidence.", Blocking: true})
	} else if len(plan.Recipes) == 0 {
		unknowns = append(unknowns, model.IntegrationUnknown{ID: "product-capability", Question: "Which exact product operation should the recipe implement?", Why: "No revision-exact API-owned tool currently supplies one callable, schema-bound operation for a tangible recipe.", Blocking: true})
	}
	return plan, unknowns
}

func integrationRecipeCapabilityViable(item model.IntegrationEvidence, integrationID string) bool {
	if integrationID == "" || strings.TrimSpace(item.ResourceID) == "" || strings.TrimSpace(item.Label) == "" || !recipeCapabilityEvidence(item) {
		return false
	}
	switch item.Kind {
	case "tool":
		if recipeEvidenceField(item.Excerpt, "Exact bound tool revision") != item.Version || recipeEvidenceField(item.Excerpt, "Scope") != model.ToolScopeAPI || recipeEvidenceField(item.Excerpt, "Owner integration ID") != integrationID || recipeEvidenceField(item.Excerpt, "Backend") == "" || recipeEvidenceField(item.Excerpt, "Upstream drifted") != "false" {
			return false
		}
		if recipeEvidenceField(item.Excerpt, "Input schema") == "" || recipeEvidenceField(item.Excerpt, "Output schema") == "" {
			return false
		}
		switch recipeEvidenceField(item.Excerpt, "Backend") {
		case "http":
			return recipeEvidenceField(item.Excerpt, "Method") != "" && recipeEvidenceField(item.Excerpt, "Fixed endpoint") != ""
		case "mcp":
			return recipeEvidenceField(item.Excerpt, "MCP tool name") != ""
		default:
			return false
		}
	default:
		return false
	}
}

func integrationRecipeSelection(evidence []model.IntegrationEvidence, seed model.RecipeSeed) (map[string]model.IntegrationEvidence, bool) {
	integrationID, scoped := integrationScopeID(evidence)
	if !scoped || len(seed.EndpointIDs) != 0 || len(seed.CapabilityIDs) != 1 || len(seed.EvidenceIDs) == 0 || len(seed.EvidenceIDs) > 24 {
		return nil, false
	}
	selected, ok := recipeResolveProductSelection(model.IntegrationAnalysis{Evidence: evidence}, seed)
	if !ok {
		return nil, false
	}
	byID, ambiguous := recipeUniqueEvidenceByID(selected)
	if len(ambiguous) != 0 {
		return nil, false
	}
	capabilityID := seed.CapabilityIDs[0]
	capability, ok := byID[capabilityID]
	if !ok || !integrationRecipeCapabilityViable(capability, integrationID) {
		return nil, false
	}
	if seed.SDKID != "" {
		return nil, false
	}
	for id, item := range byID {
		if recipeCapabilityEvidence(item) && id != capabilityID || item.Kind == "sdk" {
			return nil, false
		}
	}
	return byID, true
}

func boundedIntegrationRecipeSlug(value string, limit int) string {
	value = strings.Trim(slugify(value), "-")
	if len(value) <= limit {
		return value
	}
	return strings.Trim(value[:limit], "-")
}

func deterministicIntegrationRecipeSeeds(product model.Product, integration model.Integration, evidence []model.IntegrationEvidence) []model.RecipeSeed {
	productEvidence := recipeProductEvidence(evidence)
	byID, ambiguous := recipeUniqueEvidenceByID(productEvidence)
	toolCapabilityIDs := make([]string, 0)
	for _, capabilityID := range recipeProductCapabilityIDs(productEvidence) {
		item, exists := byID[capabilityID]
		if !exists || ambiguous[capabilityID] || !integrationRecipeCapabilityViable(item, integration.ID) {
			continue
		}
		toolCapabilityIDs = append(toolCapabilityIDs, capabilityID)
	}
	capabilityIDs := toolCapabilityIDs
	prefix := integrationRecipePrefix(product, integration)
	result := make([]model.RecipeSeed, 0, min(len(capabilityIDs), 12))
	seenSlugs := make(map[string]bool)
	for _, capabilityID := range capabilityIDs {
		item, exists := byID[capabilityID]
		if !exists {
			continue
		}
		label := evidenceText(item.Label)
		slug := boundedIntegrationRecipeSlug(prefix+"-"+label, 160)
		if slug == "" || seenSlugs[slug] {
			suffix := evidenceFingerprint(item.ResourceID)[:8]
			slug = boundedIntegrationRecipeSlug(prefix+"-"+label, 151) + "-" + suffix
		}
		if slug == "" || seenSlugs[slug] {
			continue
		}
		seed := model.RecipeSeed{
			Slug:          slug,
			Title:         truncateRunes("Implement "+label, 160),
			Outcome:       truncateRunes("The consuming project implements and verifies the reviewed "+label+" product capability.", 1000),
			Audience:      "coding_agent",
			CapabilityIDs: []string{item.ResourceID},
			EvidenceIDs:   []string{item.ResourceID},
		}
		if _, valid := integrationRecipeSelection(evidence, seed); !valid {
			continue
		}
		seenSlugs[slug] = true
		result = append(result, seed)
		if len(result) == 12 {
			break
		}
	}
	return result
}

func evidenceIDs(evidence []model.IntegrationEvidence) []string {
	result := make([]string, 0, len(evidence))
	seen := make(map[string]bool, len(evidence))
	for _, item := range evidence {
		id := strings.TrimSpace(item.ResourceID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}

func normalizeIntegrationPlan(plan model.IntegrationPlan, fallback model.IntegrationPlan, evidence []model.IntegrationEvidence) model.IntegrationPlan {
	// Transport and identity are platform security policy, not model-authored
	// content. The model may improve the narrative and propose recipes, but it
	// cannot add an endpoint, change a method/path, downgrade authentication, or
	// invent issuer/audience/grant values.
	if strings.TrimSpace(plan.Summary) == "" || len(plan.Summary) > 1000 {
		plan.Summary = fallback.Summary
	}
	plan.Identity = fallback.Identity
	plan.Endpoints = append([]model.IntegrationEndpointPlan(nil), fallback.Endpoints...)
	cleanRecipes := make([]model.RecipeSeed, 0, len(plan.Recipes))
	seenRecipe := make(map[string]bool)
	for _, seed := range plan.Recipes {
		seed.Slug = slugify(seed.Slug)
		seed.Title, seed.Outcome = strings.TrimSpace(seed.Title), strings.TrimSpace(seed.Outcome)
		seed.Audience = "coding_agent"
		seed.SDKID = strings.TrimSpace(seed.SDKID)
		seed.EndpointIDs = nil
		for index := range seed.CapabilityIDs {
			seed.CapabilityIDs[index] = strings.TrimSpace(seed.CapabilityIDs[index])
		}
		for index := range seed.EvidenceIDs {
			seed.EvidenceIDs[index] = strings.TrimSpace(seed.EvidenceIDs[index])
		}
		if seed.Slug == "" || seenRecipe[seed.Slug] || seed.Title == "" || seed.Outcome == "" || len(seed.Slug) > 160 || len(seed.Title) > 160 || len(seed.Outcome) > 1000 {
			continue
		}
		if _, valid := integrationRecipeSelection(evidence, seed); !valid {
			continue
		}
		seenRecipe[seed.Slug] = true
		cleanRecipes = append(cleanRecipes, seed)
		if len(cleanRecipes) == 12 {
			break
		}
	}
	plan.Recipes = cleanRecipes
	return plan
}
