package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

type ToolInput struct {
	OrganisationID             string
	ProductID                  string
	Scope                      string
	OwnerIntegrationID         string
	RuntimeServiceConnectionID string
	HTTPPath                   string
	Namespace                  string
	Name                       string
	Description                string
	InputSchema                json.RawMessage
	OutputSchema               json.RawMessage
	Endpoint                   string
	HTTPMethod                 string
	UpstreamAuth               json.RawMessage
	Credential                 string
	RequestMapping             json.RawMessage
	ResponseMapping            json.RawMessage
	RequestExample             json.RawMessage
	ResponseExample            json.RawMessage
	AuthorizationPolicy        json.RawMessage
	TimeoutMS                  int
}

func (s *Service) normalizeToolOwnership(ctx context.Context, product model.Product, scope, ownerIntegrationID string) (string, string, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	ownerIntegrationID = strings.TrimSpace(ownerIntegrationID)
	if scope == "" {
		scope = model.ToolScopeCommon
	}
	switch scope {
	case model.ToolScopeCommon:
		if ownerIntegrationID != "" {
			return "", "", errors.New("common tools cannot have an owner integration")
		}
		return scope, "", nil
	case model.ToolScopeAPI:
		if ownerIntegrationID == "" {
			return "", "", errors.New("api tools require owner_integration_id")
		}
		integration, err := s.store.Integration(ctx, product.ID, ownerIntegrationID)
		if err != nil || integration.OrganisationID != product.OrganisationID {
			return "", "", errors.New("api tool owner must be an integration in the same deployment")
		}
		return scope, ownerIntegrationID, nil
	default:
		return "", "", errors.New("tool scope must be common or api")
	}
}

func (s *Service) CreateTool(ctx context.Context, input ToolInput, actor Actor) (model.Tool, error) {
	product, err := s.store.Product(ctx, input.ProductID)
	if err != nil {
		return model.Tool{}, err
	}
	input.OrganisationID = product.OrganisationID
	input.Scope, input.OwnerIntegrationID, err = s.normalizeToolOwnership(ctx, product, input.Scope, input.OwnerIntegrationID)
	if err != nil {
		return model.Tool{}, err
	}
	input.Namespace, input.Name = strings.ToLower(strings.TrimSpace(input.Namespace)), strings.ToLower(strings.TrimSpace(input.Name))
	input.Description, input.HTTPMethod, input.Endpoint = strings.TrimSpace(input.Description), strings.ToUpper(strings.TrimSpace(input.HTTPMethod)), strings.TrimSpace(input.Endpoint)
	input.RuntimeServiceConnectionID, input.HTTPPath = strings.TrimSpace(input.RuntimeServiceConnectionID), strings.TrimSpace(input.HTTPPath)
	if !toolNamePattern.MatchString(input.Namespace) || !toolNamePattern.MatchString(input.Name) || input.Description == "" || len(input.Description) > 500 {
		return model.Tool{}, errors.New("tool namespace, name, and description are invalid")
	}
	if err := toolruntime.ValidateSchema(input.InputSchema); err != nil {
		return model.Tool{}, err
	}
	if len(input.OutputSchema) == 0 {
		input.OutputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`)
	}
	if err := toolruntime.ValidateSchema(input.OutputSchema); err != nil {
		return model.Tool{}, fmt.Errorf("output schema: %w", err)
	}
	methods := map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
	if !methods[input.HTTPMethod] {
		return model.Tool{}, errors.New("tool must use an allowed HTTP method")
	}
	var parsed *url.URL
	var auth ToolUpstreamAuth
	if input.RuntimeServiceConnectionID != "" {
		if input.Endpoint != "" || strings.TrimSpace(input.Credential) != "" || len(bytes.TrimSpace(input.UpstreamAuth)) != 0 {
			return model.Tool{}, errors.New("API runtime tools use their service connection for endpoint and authentication")
		}
		input.Endpoint, input.UpstreamAuth, err = s.validateRuntimeToolConnection(ctx, product, input)
		if err != nil {
			return model.Tool{}, err
		}
		_ = json.Unmarshal(input.UpstreamAuth, &auth)
		parsed, _ = url.Parse(input.Endpoint)
	} else {
		parsed, err = url.Parse(input.Endpoint)
		if err != nil || !validToolEndpoint(input.Endpoint) {
			return model.Tool{}, errors.New("tool endpoint must be a fixed credential-free public HTTPS URL or HTTP localhost URL and use an allowed HTTP method")
		}
		var upstreamAuth json.RawMessage
		upstreamAuth, auth, _, err = normalizeToolUpstreamAuth(input.UpstreamAuth, nil, "", input.Credential)
		if err != nil {
			return model.Tool{}, err
		}
		input.UpstreamAuth = upstreamAuth
	}
	if auth.Type == "delegated_oauth" {
		provider, providerErr := s.store.IdentityProvider(ctx, input.ProductID)
		if providerErr != nil || provider.State != "active" || provider.DelegatedAPIOrigin == "" {
			return model.Tool{}, errors.New("configure an active identity provider authorization API origin before creating a delegated OAuth tool")
		}
		vendorOrigin, originErr := url.Parse(provider.DelegatedAPIOrigin)
		if originErr != nil || !strings.EqualFold(parsed.Scheme, vendorOrigin.Scheme) || !strings.EqualFold(parsed.Host, vendorOrigin.Host) {
			return model.Tool{}, errors.New("delegated OAuth tool endpoint must use the configured vendor API origin")
		}
	}
	input.RequestMapping, _, err = normalizeToolRequestMapping(input.RequestMapping)
	if err != nil {
		return model.Tool{}, err
	}
	input.ResponseMapping, _, err = normalizeToolResponseMapping(input.ResponseMapping)
	if err != nil {
		return model.Tool{}, err
	}
	if err := validateToolMappings(input.InputSchema, input.Endpoint, input.HTTPMethod, input.RequestMapping); err != nil {
		return model.Tool{}, err
	}
	input.RequestExample, err = normalizeToolExample(input.RequestExample, input.InputSchema, "request")
	if err != nil {
		return model.Tool{}, err
	}
	input.ResponseExample, err = normalizeToolExample(input.ResponseExample, input.OutputSchema, "response")
	if err != nil {
		return model.Tool{}, err
	}
	if input.TimeoutMS == 0 {
		input.TimeoutMS = 10_000
	}
	if input.TimeoutMS < 100 || input.TimeoutMS > 60_000 {
		return model.Tool{}, errors.New("tool timeout must be between 100 and 60000 milliseconds")
	}
	policy, _, err := normalizeToolPolicy(input.AuthorizationPolicy, input.HTTPMethod)
	if err != nil {
		return model.Tool{}, err
	}
	input.AuthorizationPolicy = policy
	input, err = s.validateCanonicalToolInput(ctx, input, credentialRequired(auth.Type) && (input.RuntimeServiceConnectionID != "" || input.Credential != ""))
	if err != nil {
		return model.Tool{}, err
	}
	toolID, err := randomUUID()
	if err != nil {
		return model.Tool{}, err
	}
	connectionID := ""
	if input.RuntimeServiceConnectionID == "" {
		connectionID, err = randomUUID()
		if err != nil {
			return model.Tool{}, err
		}
	}
	credentialID, credentialFingerprint := "", ""
	if input.Credential != "" {
		credentialID, credentialFingerprint, err = s.saveToolCredential(ctx, input.OrganisationID, connectionID, input.Credential)
		if err != nil {
			return model.Tool{}, err
		}
	}
	baseURL := input.Endpoint
	if input.RuntimeServiceConnectionID != "" {
		baseURL = ""
	}
	value, err := s.store.CreateTool(ctx, model.Tool{ID: toolID, OrganisationID: input.OrganisationID, ProductID: input.ProductID, Scope: input.Scope, OwnerIntegrationID: input.OwnerIntegrationID, RuntimeServiceConnectionID: input.RuntimeServiceConnectionID, HTTPPath: input.HTTPPath, Namespace: input.Namespace, Name: input.Name, Description: input.Description, InputSchema: input.InputSchema, OutputSchema: input.OutputSchema, APIConnectionID: connectionID, BaseURL: baseURL, HTTPMethod: input.HTTPMethod, UpstreamAuth: input.UpstreamAuth, CredentialID: credentialID, CredentialFingerprint: credentialFingerprint, RequestMapping: input.RequestMapping, ResponseMapping: input.ResponseMapping, RequestExample: input.RequestExample, ResponseExample: input.ResponseExample, AuthorizationPolicy: input.AuthorizationPolicy, TimeoutMS: input.TimeoutMS, BackendKind: "http", UpstreamAnnotations: json.RawMessage(`{}`)})
	if err != nil {
		return model.Tool{}, s.cleanupFailedToolCredential(ctx, input.OrganisationID, credentialID, err)
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: input.OrganisationID, ProductID: input.ProductID, ActorID: actor.ID, Action: "tool.created", TargetType: "tool", TargetID: toolID, Current: map[string]any{"name": input.Namespace + "." + input.Name, "scope": input.Scope, "owner_integration_id": input.OwnerIntegrationID, "method": input.HTTPMethod, "authentication": auth.Type, "credential_stored": credentialID != "", "state": "draft"}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

// cleanupFailedToolCredential makes a best effort to undo secret creation even
// when the request context has already been cancelled. The original operation
// error and any cleanup error are both retained so callers can distinguish the
// failed write from an orphaned-secret condition that needs operator attention.
func (s *Service) cleanupFailedToolCredential(ctx context.Context, organisationID, credentialID string, operationErr error) error {
	if credentialID == "" {
		return operationErr
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := s.store.DeleteSecret(cleanupCtx, organisationID, credentialID); err != nil {
		return errors.Join(operationErr, fmt.Errorf("stored tool credential cleanup failed: %w", err))
	}
	return operationErr
}

func (s *Service) PublishTool(ctx context.Context, productID, toolID string, revision int64, actor Actor) (model.Tool, error) {
	current, err := s.store.Tool(ctx, productID, toolID)
	if err != nil {
		return model.Tool{}, err
	}
	if (current.BackendKind == "mcp" || current.BackendKind == "native") && current.UpstreamDrifted {
		return model.Tool{}, ErrToolDrifted
	}
	if current.BackendKind == "native" {
		if s.nativeTools == nil {
			return model.Tool{}, errors.New("native plugin manager is unavailable")
		}
		if err := s.nativeTools.ValidateNativeTool(current); err != nil {
			return model.Tool{}, fmt.Errorf("native tool requires source review before publication: %w", err)
		}
	} else if err := s.validateStoredHTTPTool(ctx, current); err != nil {
		return model.Tool{}, fmt.Errorf("tool requires review before publication: %w", err)
	}
	if err := s.validateToolGrantRegistry(ctx, productID, current); err != nil {
		return model.Tool{}, err
	}
	updated, err := s.store.PublishTool(ctx, productID, toolID, revision, actor.ID)
	if err != nil {
		return model.Tool{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "tool.published", TargetType: "tool", TargetID: toolID, Prior: map[string]any{"state": current.State}, Current: map[string]any{"state": "published", "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, err
}

func (s *Service) SetPublicMCP(ctx context.Context, productID string, enabled, acknowledged bool, expectedRevision int64, actor Actor) (model.Product, error) {
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.Product{}, err
	}
	if product.PublicMCPEnabled == enabled {
		return product, nil
	}
	if enabled && !acknowledged {
		return model.Product{}, ErrConfirmationRequired
	}

	prior := product.PublicMCPEnabled
	product.PublicMCPEnabled = enabled
	updated, err := s.store.UpdateProduct(ctx, product, expectedRevision)
	if err != nil {
		return model.Product{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{
		ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.ID,
		ActorID: actor.ID, Action: "product.public_mcp.changed", TargetType: "product", TargetID: updated.ID,
		Prior: map[string]any{"public_mcp_enabled": prior}, Current: map[string]any{"public_mcp_enabled": enabled},
		RequestID: actor.RequestID, CreatedAt: s.now(),
	}); err != nil {
		return model.Product{}, err
	}
	return updated, nil
}

func (s *Service) SetSourceVisibility(ctx context.Context, productID, sourceID string, visibility model.Visibility, acknowledged bool, expectedRevision int64, actor Actor) (model.Source, error) {
	if !visibility.Valid() {
		return model.Source{}, ErrInvalidVisibility
	}
	source, err := s.store.Source(ctx, productID, sourceID)
	if err != nil {
		return model.Source{}, err
	}
	if source.Visibility == visibility {
		return source, nil
	}
	if visibility == model.VisibilityPublic {
		if !acknowledged {
			return model.Source{}, ErrConfirmationRequired
		}
		if source.Quarantined {
			return model.Source{}, fmt.Errorf("%w: quarantined sources cannot be public", ErrUnsafeForPublic)
		}
	}

	prior := source.Visibility
	source.Visibility = visibility
	updated, err := s.store.UpdateSource(ctx, source, expectedRevision)
	if err != nil {
		return model.Source{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{
		ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.ProductID,
		ActorID: actor.ID, Action: "source.visibility.changed", TargetType: "source", TargetID: updated.ID,
		Prior: map[string]any{"visibility": prior}, Current: map[string]any{"visibility": visibility},
		RequestID: actor.RequestID, CreatedAt: s.now(),
	}); err != nil {
		return model.Source{}, err
	}
	return updated, nil
}
