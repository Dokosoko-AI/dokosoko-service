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
	fmt.Fprintf(&builder, "Description: %s\nBackend: %s\nMethod: %s", evidenceText(value.Description), value.BackendKind, value.HTTPMethod)
	if value.BackendKind == "http" && strings.TrimSpace(value.BaseURL) != "" {
		fmt.Fprintf(&builder, "\nFixed endpoint: %s", value.BaseURL)
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

func automaticToolEvidence(integration model.Integration, name, description, inputSchema, details, version string) model.IntegrationEvidence {
	excerpt := "Description: " + evidenceText(description) + "\nInput schema: " + inputSchema
	if details != "" {
		excerpt += "\n" + details
	}
	resourceID := "automatic-tool:" + integration.ID + ":" + name
	return model.IntegrationEvidence{Kind: "automatic_tool", ResourceID: resourceID, Label: name, Excerpt: truncateRunes(excerpt, maxAnalysisToolItem), Version: version, Visibility: model.VisibilityPrivate, Fingerprint: evidenceFingerprint("automatic_tool", resourceID, version, excerpt)}
}

func accessOperationNames(raw json.RawMessage) map[string]bool {
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	result := make(map[string]bool, len(values))
	for key := range values {
		result[key] = true
	}
	return result
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

func namespaceIntegrationRecipes(plan model.IntegrationPlan, product model.Product, integration model.Integration) model.IntegrationPlan {
	prefix := integrationRecipePrefix(product, integration)
	defaultSlug := "connect-" + prefix + "-to-mcp"
	productDefault := "connect-" + slugify(product.Slug) + "-to-mcp"
	for index := range plan.Recipes {
		slug := slugify(plan.Recipes[index].Slug)
		switch {
		case slug == "", slug == productDefault, slug == "connect-to-mcp":
			plan.Recipes[index].Slug = defaultSlug
			plan.Recipes[index].Title = "Connect " + integration.DisplayName + " to MCP"
		case slug == defaultSlug, strings.HasPrefix(slug, prefix+"-"):
			plan.Recipes[index].Slug = slug
		default:
			plan.Recipes[index].Slug = prefix + "-" + slug
		}
	}
	return plan
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
	oauthExcerpt := "Private MCP endpoint: POST /mcp\nAn unauthenticated MCP request returns HTTP 401 with a WWW-Authenticate resource_metadata URL for the exact /mcp resource.\nThe protected-resource metadata advertises the DokoSoko authorization server; its metadata advertises authorization, token, and dynamic client-registration endpoints.\nDynamic registration accepts an exact loopback callback for a public client.\nAuthorization Code uses PKCE method S256 and the exact MCP resource parameter; the code exchange repeats the same client, callback, verifier, and resource.\nMCP protocol: Stateless MCPv2 2026-07-28\nThe MCP client authenticates to DokoSoko; DokoSoko brokers the configured upstream OIDC provider and keeps the upstream token out of the MCP client."
	values = append(values, model.IntegrationEvidence{Kind: "mcp_oauth", ResourceID: "mcp-oauth-contract-v1", Label: "Private MCP OAuth contract", Excerpt: oauthExcerpt, Version: "1", Visibility: model.VisibilityPrivate, Fingerprint: evidenceFingerprint("mcp_oauth", "1", oauthExcerpt)})
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
		toolVersion := strconv.FormatInt(binding.ToolRevision, 10)
		label := binding.ToolID
		excerpt := "Exact bound tool revision: " + toolVersion
		if binding.Tool != nil {
			label = recipeToolLabel(*binding.Tool, &integration)
			if binding.Tool.Revision == binding.ToolRevision {
				excerpt += "\n" + toolCatalogExcerpt(*binding.Tool, min(maxAnalysisToolItem, remaining))
			}
		}
		excerpt = truncateRunes(excerpt, min(maxAnalysisToolItem, remaining))
		toolRunes += len([]rune(excerpt))
		values = append(values, model.IntegrationEvidence{Kind: "tool", ResourceID: binding.ToolID, Label: label, Excerpt: excerpt, Version: toolVersion, Visibility: model.VisibilityPrivate, Fingerprint: evidenceFingerprint("tool_binding", integration.ID, binding.ToolID, toolVersion, excerpt)})
	}
	hasDocumentation := false
	for _, link := range integration.Resources {
		if link.Kind == "documentation" && link.ResolvedRevision != nil {
			hasDocumentation = true
			break
		}
	}
	if hasDocumentation {
		values = append(values, automaticToolEvidence(integration, integration.FamilyKey+".knowledge.search", "Search only the reviewed documentation pinned by this published API revision.", `{"type":"object","additionalProperties":false,"properties":{"query":{"type":"string","minLength":1,"maxLength":2000}},"required":["query"]}`, "Read-only. No confirmation or idempotency key is required.", version))
	}
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
	plan := model.IntegrationPlan{Summary: "Expose " + subjectName + " through one discoverable MCP endpoint, with private identity only where customer data or actions require it."}
	provider, err := s.store.IdentityProvider(ctx, product.ID)
	if err == nil && provider.State == "active" {
		plan.Identity = model.IntegrationIdentityPlan{Mode: "oauth2", Issuer: provider.Issuer, Audience: provider.Audience, Explanation: "DokoSoko brokers customer sign-in through the configured OIDC provider and keeps vendor access tokens out of MCP clients."}
	} else {
		plan.Identity = model.IntegrationIdentityPlan{Mode: "none", Explanation: "Public discovery can work without identity. Configure OIDC before exposing customer-specific data or actions."}
	}
	plan.Endpoints = []model.IntegrationEndpointPlan{
		{Name: "mcp", Method: "POST", Path: "/mcp", Purpose: "Private MCP discovery and tool execution.", Identity: "oauth2", Evidence: evidenceIDs(evidence)},
	}
	recipeEndpointIDs := []string{"mcp"}
	if product.PublicMCPEnabled {
		plan.Endpoints = append(plan.Endpoints, model.IntegrationEndpointPlan{Name: "public-mcp", Method: "POST", Path: "/mcp/public", Purpose: "Anonymous access to explicitly public recipes and knowledge.", Identity: "none", Evidence: evidenceIDs(evidence)})
		recipeEndpointIDs = append(recipeEndpointIDs, "public-mcp")
	}
	if plan.Identity.Mode == "oauth2" {
		plan.Endpoints = append(plan.Endpoints, model.IntegrationEndpointPlan{Name: "access-evaluation", Method: "POST", Path: "/v1/access/evaluations", Purpose: "Resolve the authenticated customer to bounded grants before private authorization.", Identity: "oauth2", Evidence: evidenceIDs(evidence)})
	}
	plan.Recipes = []model.RecipeSeed{{Slug: "connect-" + slugify(product.Slug) + "-to-mcp", Title: "Connect " + product.Name + " to MCP", Outcome: "An MCP client can discover the connector and verify access.", Audience: "developer", EndpointIDs: recipeEndpointIDs, EvidenceIDs: evidenceIDs(evidence)}}
	if integration != nil {
		plan = namespaceIntegrationRecipes(plan, product, *integration)
	}
	unknowns := make([]model.IntegrationUnknown, 0)
	if plan.Identity.Mode == "none" {
		unknowns = append(unknowns, model.IntegrationUnknown{ID: "private-access", Question: "Will developers access customer-specific data or perform actions?", Why: "Private operations require an identity boundary and explicit grants; public MCP must remain read-only and deliberately published.", Blocking: false})
	}
	if len(evidence) == 0 {
		unknowns = append(unknowns, model.IntegrationUnknown{ID: "source-of-truth", Question: "Which API specification or documentation is the source of truth?", Why: "DokoSoko cannot produce trustworthy endpoints or implementation steps without evidence.", Blocking: true})
	}
	return plan, unknowns
}

func evidenceIDs(evidence []model.IntegrationEvidence) []string {
	result := make([]string, 0, len(evidence))
	seen := make(map[string]bool, len(evidence))
	for _, item := range evidence {
		if seen[item.ResourceID] {
			continue
		}
		seen[item.ResourceID] = true
		result = append(result, item.ResourceID)
	}
	return result
}

func normalizeIntegrationPlan(plan model.IntegrationPlan, fallback model.IntegrationPlan, evidence []model.IntegrationEvidence) model.IntegrationPlan {
	// Transport and identity are platform security policy, not model-authored
	// content. The model may improve the narrative and propose recipes, but it
	// cannot add an endpoint, change a method/path, downgrade authentication, or
	// invent issuer/audience/grant values.
	allowedEvidence := make(map[string]bool, len(evidence))
	for _, item := range evidence {
		allowedEvidence[item.ResourceID] = item.ResourceID != ""
	}
	if strings.TrimSpace(plan.Summary) == "" || len(plan.Summary) > 1000 {
		plan.Summary = fallback.Summary
	}
	plan.Identity = fallback.Identity
	plan.Endpoints = append([]model.IntegrationEndpointPlan(nil), fallback.Endpoints...)
	allowedEndpoints := make(map[string]bool, len(fallback.Endpoints))
	for _, endpoint := range fallback.Endpoints {
		allowedEndpoints[endpoint.Name] = true
	}
	cleanRecipes := make([]model.RecipeSeed, 0, len(plan.Recipes))
	seenRecipe := make(map[string]bool)
	for _, seed := range plan.Recipes {
		seed.Slug = slugify(seed.Slug)
		seed.Title, seed.Outcome, seed.Audience = strings.TrimSpace(seed.Title), strings.TrimSpace(seed.Outcome), strings.TrimSpace(seed.Audience)
		endpointIDs := make([]string, 0, len(seed.EndpointIDs))
		seenEndpointID := make(map[string]bool, len(seed.EndpointIDs))
		for _, rawEndpointID := range seed.EndpointIDs {
			endpointID := strings.TrimSpace(rawEndpointID)
			if allowedEndpoints[endpointID] && !seenEndpointID[endpointID] {
				endpointIDs = append(endpointIDs, endpointID)
				seenEndpointID[endpointID] = true
			}
		}
		seed.EndpointIDs = endpointIDs
		endpointEvidence := make(map[string]bool)
		for _, endpoint := range fallback.Endpoints {
			if !seenEndpointID[endpoint.Name] {
				continue
			}
			for _, evidenceID := range endpoint.Evidence {
				endpointEvidence[evidenceID] = true
			}
		}
		evidenceIDs := make([]string, 0, len(seed.EvidenceIDs))
		seenEvidenceID := make(map[string]bool, len(seed.EvidenceIDs))
		validEvidenceSelection := true
		for _, rawEvidenceID := range seed.EvidenceIDs {
			evidenceID := strings.TrimSpace(rawEvidenceID)
			if evidenceID == "" || !allowedEvidence[evidenceID] || !endpointEvidence[evidenceID] || seenEvidenceID[evidenceID] {
				validEvidenceSelection = false
				break
			}
			evidenceIDs = append(evidenceIDs, evidenceID)
			seenEvidenceID[evidenceID] = true
		}
		seed.EvidenceIDs = evidenceIDs
		if seed.Slug == "" || seenRecipe[seed.Slug] || seed.Title == "" || seed.Outcome == "" || seed.Audience == "" || len(seed.Title) > 160 || len(seed.Outcome) > 1000 || len(seed.Audience) > 80 || len(seed.EndpointIDs) == 0 || !validEvidenceSelection {
			continue
		}
		seenRecipe[seed.Slug] = true
		cleanRecipes = append(cleanRecipes, seed)
		if len(cleanRecipes) == 12 {
			break
		}
	}
	if len(cleanRecipes) == 0 {
		cleanRecipes = fallback.Recipes
	}
	plan.Recipes = cleanRecipes
	return plan
}
