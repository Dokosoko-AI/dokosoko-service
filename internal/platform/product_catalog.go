package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var ErrDescriptionRewrite = errors.New("product description could not be rewritten")

func validProductDescription(value string) bool {
	return value != "" && len(value) <= 1000 && strings.IndexFunc(value, unicode.IsControl) < 0 && !containsAISecretText(value)
}

// UpdateProductSettings keeps the deployment description current for MCP
// discovery. Product-level release channels and promotion policy no longer
// exist; API publications are the immutable delivery boundary.
func (s *Service) UpdateProductSettings(ctx context.Context, productID, description string, expected int64, actor Actor) (model.Product, error) {
	description = strings.TrimSpace(description)
	if !validProductDescription(description) {
		return model.Product{}, errors.New("product description must be 1 to 1000 printable characters")
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.Product{}, err
	}
	priorDescription := product.Description
	product.Description = description
	updated, err := s.store.UpdateProduct(ctx, product, expected)
	if err != nil {
		return model.Product{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.ID, ActorID: actor.ID, Action: "product.discovery.updated", TargetType: "product", TargetID: updated.ID, Prior: map[string]any{"description_changed": priorDescription != description}, Current: map[string]any{"description_length": len(description), "catalog_revision": updated.CatalogRevision}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, err
}

func (s *Service) ProductManifest(ctx context.Context, productID, _ string) (model.ProductManifest, error) {
	return s.ProductManifestFor(ctx, productID, model.CatalogScope{})
}

// ProductManifestFor builds discovery directly from the latest immutable
// publication of every active API. There is no customer pin, channel,
// promotion, rollout, or installation selection layer.
func (s *Service) ProductManifestFor(ctx context.Context, productID string, scope model.CatalogScope) (model.ProductManifest, error) {
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.ProductManifest{}, err
	}
	manifest := model.ProductManifest{
		DeploymentID: product.ID, DeploymentSlug: product.Slug, DeploymentName: product.Name,
		ProductID: product.ID, ProductSlug: product.Slug, ProductName: product.Name,
		Description: product.Description, CatalogRevision: product.CatalogRevision,
		Integrations: []model.IntegrationManifest{},
	}
	if deployment, deploymentErr := s.store.Deployment(ctx); deploymentErr == nil && deployment.ID == productID {
		manifest.DeploymentID, manifest.DeploymentSlug, manifest.DeploymentName = deployment.ID, deployment.Slug, deployment.Name
		manifest.CatalogRevision = deployment.CatalogRevision
	}
	integrations, err := s.store.Integrations(ctx, productID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return model.ProductManifest{}, err
	}
	for _, integration := range integrations {
		if integration.Lifecycle != "active" && integration.Lifecycle != "deprecated" {
			continue
		}
		revisions, revisionErr := s.store.IntegrationRevisions(ctx, integration.ID)
		if revisionErr != nil {
			return model.ProductManifest{}, revisionErr
		}
		var published model.IntegrationRevision
		for _, revision := range revisions {
			if revision.State == "published" && revision.Revision > published.Revision {
				published = revision
			}
		}
		if published.ID == "" {
			continue
		}
		var snapshot integrationSnapshot
		if err := json.Unmarshal(published.Snapshot, &snapshot); err != nil {
			return model.ProductManifest{}, fmt.Errorf("integration %s has an invalid published snapshot: %w", integration.ID, err)
		}
		if snapshot.Visibility == "" {
			snapshot.Visibility = model.VisibilityPrivate
		}
		if snapshot.Tools != nil {
			manifest.ManagedIntegrationTools = true
		}
		if scope.Public && (integration.Visibility != model.VisibilityPublic || snapshot.Visibility != model.VisibilityPublic) {
			continue
		}
		entry := model.IntegrationManifest{ID: integration.ID, FamilyKey: snapshot.FamilyKey, VersionKey: snapshot.VersionKey, DisplayName: snapshot.DisplayName, Description: snapshot.Description, Visibility: snapshot.Visibility, Lifecycle: snapshot.Lifecycle, ReplacementIntegrationID: snapshot.ReplacementIntegrationID, SunsetAt: snapshot.SunsetAt, Revision: published.Revision, ManifestHash: published.ManifestHash, Resources: []model.IntegrationManifestResource{}}
		for _, resource := range snapshot.Resources {
			name := resource.Name
			if name == "" {
				name = resource.SetID
			}
			item := model.IntegrationManifestResource{ResourceSetID: resource.SetID, Kind: resource.Kind, Name: name, Revision: resource.Revision, ContentHash: resource.ContentHash}
			for _, publication := range resource.SourcePublications {
				item.SourcePublications = append(item.SourcePublications, model.IntegrationManifestSourcePublication{ID: publication.SourcePublicationID, SourceID: publication.SourceID, Revision: publication.Revision, ContentHash: publication.ContentHash})
			}
			entry.Resources = append(entry.Resources, item)
		}
		sort.SliceStable(entry.Resources, func(i, j int) bool {
			if entry.Resources[i].Kind == entry.Resources[j].Kind {
				return entry.Resources[i].Name < entry.Resources[j].Name
			}
			return entry.Resources[i].Kind < entry.Resources[j].Kind
		})
		for _, sdk := range snapshot.SDKs {
			entry.SDKs = append(entry.SDKs, model.IntegrationManifestSDK{ID: sdk.ID, Ecosystem: sdk.Ecosystem, Coordinate: sdk.Coordinate, ExactVersion: sdk.ExactVersion, InstallCommand: sdk.InstallCommand, DocumentationURL: sdk.DocumentationURL, SourceURL: sdk.SourceURL, Checksum: sdk.Checksum, Visibility: sdk.Visibility, Revision: sdk.Revision, ContentHash: sdk.ContentHash})
		}
		for _, point := range snapshot.AuthorizationPoints {
			entry.AuthorizationPoints = append(entry.AuthorizationPoints, model.IntegrationManifestAuthorizationPoint{ID: point.ID, Key: point.Key, Name: point.Name, ActionType: point.ActionType, RequiredGrants: append([]string(nil), point.RequiredGrants...), ConfirmationRequired: point.ConfirmationRequired, DecisionTTLSeconds: point.DecisionTTLSeconds, Revision: point.Revision})
		}
		for _, tool := range snapshot.Tools {
			entry.Tools = append(entry.Tools, model.IntegrationManifestTool{ToolID: tool.ToolID, ToolRevision: tool.ToolRevision, AuthorizationPointID: tool.AuthorizationPointID, AuthorizationPointRevision: tool.AuthorizationPointRevision, RuntimeServiceConnectionID: tool.RuntimeServiceConnectionID, Namespace: tool.Namespace, Name: tool.Name, BackendKind: tool.BackendKind, Effect: tool.Effect, IdempotencyMode: tool.IdempotencyMode, IdentityRequirement: tool.IdentityRequirement, StateScope: tool.StateScope, MaxConcurrency: tool.MaxConcurrency, MaxResultBytes: tool.MaxResultBytes, ContentHash: tool.ContentHash, UpstreamSchemaHash: tool.UpstreamSchemaHash, NativePluginID: tool.NativePluginID, NativeToolID: tool.NativeToolID, NativePluginVersion: tool.NativePluginVersion, NativeSDKVersion: tool.NativeSDKVersion, NativeManifestHash: tool.NativeManifestHash, NativeContractHash: tool.NativeContractHash})
		}
		for _, connection := range snapshot.ServiceConnections {
			item := model.IntegrationManifestServiceConnection{ConnectionID: connection.ConnectionID, ConnectionRevision: connection.ConnectionRevision, Name: connection.Name, Description: connection.Description, State: connection.State, CurrentRevisions: []model.IntegrationManifestServiceConnectionRevision{}}
			for _, revision := range connection.CurrentRevisions {
				item.CurrentRevisions = append(item.CurrentRevisions, model.IntegrationManifestServiceConnectionRevision{RevisionID: revision.RevisionID, Revision: revision.Revision, EnvironmentID: revision.EnvironmentID, BaseURL: revision.BaseURL, AuthenticationType: revision.AuthenticationType, CredentialSetID: revision.CredentialSetID, AuthConfig: append(json.RawMessage(nil), revision.AuthConfig...), ContentHash: revision.ContentHash, Current: revision.Current, CredentialReady: revision.CredentialReady})
			}
			entry.ServiceConnections = append(entry.ServiceConnections, item)
		}
		manifest.Integrations = append(manifest.Integrations, entry)
	}
	sort.SliceStable(manifest.Integrations, func(i, j int) bool {
		if manifest.Integrations[i].FamilyKey == manifest.Integrations[j].FamilyKey {
			return manifest.Integrations[i].VersionKey < manifest.Integrations[j].VersionKey
		}
		return manifest.Integrations[i].FamilyKey < manifest.Integrations[j].FamilyKey
	})
	return manifest, nil
}

// CatalogAllowsTool checks an exact published tool revision against the
// current immutable API publications.
func (s *Service) CatalogAllowsTool(ctx context.Context, productID string, scope model.CatalogScope, tool model.Tool) (bool, bool, error) {
	manifest, err := s.ProductManifestFor(ctx, productID, scope)
	if err != nil {
		return false, false, err
	}
	if !manifest.ManagedIntegrationTools {
		return false, true, nil
	}
	for _, integration := range manifest.Integrations {
		for _, candidate := range integration.Tools {
			if candidate.ToolID == tool.ID && candidate.ToolRevision == tool.Revision {
				return true, true, nil
			}
		}
	}
	return true, false, nil
}

func (s *Service) CatalogAllowsArtifact(ctx context.Context, productID string, scope model.CatalogScope, kind, referenceID string) (bool, bool, error) {
	manifest, err := s.ProductManifestFor(ctx, productID, scope)
	if err != nil {
		return false, false, err
	}
	managed := false
	for _, integration := range manifest.Integrations {
		for _, resource := range integration.Resources {
			if resource.Kind != kind {
				continue
			}
			managed = true
			for _, publication := range resource.SourcePublications {
				if publication.SourceID == referenceID {
					return true, true, nil
				}
			}
		}
	}
	return managed, !managed, nil
}

func (s *Service) RewriteProductDescription(ctx context.Context, productID, draft string, actor Actor) (string, error) {
	draft = strings.TrimSpace(draft)
	if draft == "" || len(draft) > 2000 || strings.IndexFunc(draft, unicode.IsControl) >= 0 {
		return "", errors.New("description draft must be 1 to 2000 printable characters")
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return "", err
	}
	const promptVersion = "mcp-product-description-v2"
	prompt, _ := json.Marshal(map[string]string{"product_name": product.Name, "draft": draft})
	descriptionSchema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"description":{"type":"string","minLength":1,"maxLength":2000}},"required":["description"]}`)
	system := aiCommonUntrustedInputPolicy + "\n\nRewrite a product description for an AI agent discovering a DokoSoko product. Preserve only supplied facts; never invent capabilities, versions, claims, URLs, or credentials. Use 1 to 3 concise sentences explaining what the product enables, who it serves, and important scope boundaries. Avoid marketing superlatives and implementation detail."
	completion, err := s.generateAIStructured(ctx, aiInvocation{Product: product, Workload: airuntime.WorkloadAnalysis, Action: "product_description_rewrite", PromptVersion: promptVersion, System: system, User: string(prompt), SchemaName: "product_description", Schema: descriptionSchema, MaxOutput: 512, Temperature: 0.2})
	if err != nil {
		switch airuntime.Code(err) {
		case airuntime.ErrorBudgetExhausted:
			return "", errors.New("Analysis daily token budget is exhausted")
		case airuntime.ErrorInvalidCredential:
			return "", errors.New("the Analysis provider rejected its credential; update the provider connection")
		case airuntime.ErrorUnsupportedModel:
			return "", errors.New("the Analysis model is unavailable; choose a supported model")
		case airuntime.ErrorRateLimited, airuntime.ErrorQuotaExhausted, airuntime.ErrorProviderUnavailable, airuntime.ErrorTimeout:
			return "", errors.New("the Analysis provider is temporarily unavailable")
		case airuntime.ErrorUnsafeInput:
			return "", errors.New("the description contains credential-like material and cannot be sent to Analysis")
		}
		return "", errors.New("enable the Analysis workload before using AI rewrite")
	}
	var result struct {
		Description string `json:"description"`
	}
	if decodeStrictAIResult(completion.JSON, &result) != nil {
		return "", ErrDescriptionRewrite
	}
	result.Description = strings.TrimSpace(result.Description)
	if !validProductDescription(result.Description) {
		return "", fmt.Errorf("%w: model output was invalid", ErrDescriptionRewrite)
	}
	totalTokens := completion.InputTokens + completion.OutputTokens
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: product.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "product.description.rewritten", TargetType: "product", TargetID: productID, Current: map[string]any{"model": completion.ResolvedModel, "prompt_version": promptVersion, "input_length": len(draft), "output_length": len(result.Description), "tokens": totalTokens, "saved": false}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return "", err
	}
	return result.Description, nil
}
