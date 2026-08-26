package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/internal/tools"
)

var ErrToolCloneRevisionStale = fmt.Errorf("source tool revision is stale: %w", store.ErrConflict)

type toolEditorStore interface {
	// UpdateTool must replace the connection credential reference and delete a
	// superseded purpose-bound secret in the same atomic store operation.
	UpdateTool(context.Context, model.Tool, int64) (model.Tool, error)
	RetireTool(context.Context, string, string, int64) (model.Tool, error)
}

func (s *Service) toolEditor() (toolEditorStore, error) {
	value, ok := s.store.(toolEditorStore)
	if !ok {
		return nil, errors.New("tool editor is unavailable")
	}
	return value, nil
}

type ToolPolicy struct {
	RequiredGrants       []string `json:"required_grants"`
	ConfirmationRequired bool     `json:"confirmation_required"`
	Risk                 string   `json:"risk,omitempty"`
	IdempotencyRequired  bool     `json:"idempotency_required,omitempty"`
}

func normalizeToolPolicy(raw json.RawMessage, method string) (json.RawMessage, ToolPolicy, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var policy ToolPolicy
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return nil, ToolPolicy{}, fmt.Errorf("invalid authorization policy: %w", err)
	}
	seen := make(map[string]bool, len(policy.RequiredGrants))
	grants := make([]string, 0, len(policy.RequiredGrants))
	for _, grant := range policy.RequiredGrants {
		grant = strings.ToLower(strings.TrimSpace(grant))
		if grant == "" || seen[grant] {
			continue
		}
		if !authorizationKeyPattern.MatchString(grant) {
			return nil, ToolPolicy{}, fmt.Errorf("required grant %q is invalid", grant)
		}
		seen[grant] = true
		grants = append(grants, grant)
	}
	if len(grants) > 32 {
		return nil, ToolPolicy{}, errors.New("tool may require at most 32 grants")
	}
	sort.Strings(grants)
	policy.RequiredGrants = grants
	mutation := method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE"
	if policy.Risk == "" {
		policy.Risk = "low"
		if mutation {
			policy.Risk = "medium"
		}
		if method == "DELETE" {
			policy.Risk = "critical"
		}
	}
	if method == "DELETE" {
		policy.Risk = "critical"
		policy.ConfirmationRequired = true
	}
	if policy.Risk != "low" && policy.Risk != "medium" && policy.Risk != "high" && policy.Risk != "critical" {
		return nil, ToolPolicy{}, errors.New("tool risk must be low, medium, high, or critical")
	}
	if policy.Risk == "critical" && !policy.ConfirmationRequired {
		return nil, ToolPolicy{}, errors.New("critical tools require confirmation")
	}
	if mutation && !policy.IdempotencyRequired {
		return nil, ToolPolicy{}, errors.New("mutation tools require idempotency metadata")
	}
	normalized, err := json.Marshal(policy)
	return normalized, policy, err
}

func normalizeToolEditorInput(ctx context.Context, service *Service, current model.Tool, input ToolInput) (ToolInput, error) {
	var err error
	input.Namespace, input.Name = strings.TrimSpace(input.Namespace), strings.TrimSpace(input.Name)
	input.Description, input.Endpoint = strings.TrimSpace(input.Description), strings.TrimSpace(input.Endpoint)
	input.HTTPMethod = strings.ToUpper(strings.TrimSpace(input.HTTPMethod))
	if input.Namespace == "" {
		input.Namespace = current.Namespace
	}
	if input.Name == "" {
		input.Name = current.Name
	}
	if input.Namespace != current.Namespace || input.Name != current.Name {
		return input, errors.New("published tool identity is immutable; clone the tool to rename it")
	}
	if input.Description == "" || len(input.Description) > 500 {
		return input, errors.New("tool description is invalid")
	}
	if err := tools.ValidateSchema(input.InputSchema); err != nil {
		return input, err
	}
	if len(input.OutputSchema) == 0 {
		input.OutputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`)
	}
	if err := tools.ValidateSchema(input.OutputSchema); err != nil {
		return input, fmt.Errorf("output schema: %w", err)
	}
	if len(input.RequestExample) == 0 {
		input.RequestExample = current.RequestExample
	}
	input.RequestExample, err = normalizeToolExample(input.RequestExample, input.InputSchema, "request")
	if err != nil {
		return input, err
	}
	if len(input.ResponseExample) == 0 {
		input.ResponseExample = current.ResponseExample
	}
	input.ResponseExample, err = normalizeToolExample(input.ResponseExample, input.OutputSchema, "response")
	if err != nil {
		return input, err
	}
	if current.BackendKind == "http" {
		methods := map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
		if current.RuntimeServiceConnectionID != "" {
			if !methods[input.HTTPMethod] {
				return input, errors.New("tool must use an allowed HTTP method")
			}
			if strings.TrimSpace(input.Credential) != "" {
				return input, errors.New("runtime service credentials are managed on the API connection")
			}
			input.Scope, input.OwnerIntegrationID = current.Scope, current.OwnerIntegrationID
			if input.RuntimeServiceConnectionID == "" {
				input.RuntimeServiceConnectionID = current.RuntimeServiceConnectionID
			}
			if input.RuntimeServiceConnectionID != current.RuntimeServiceConnectionID {
				return input, errors.New("clone the tool to change its runtime service connection")
			}
			if strings.TrimSpace(input.HTTPPath) == "" {
				input.HTTPPath = current.HTTPPath
			}
			product, productErr := service.store.Product(ctx, input.ProductID)
			if productErr != nil {
				return input, productErr
			}
			input.Endpoint, input.UpstreamAuth, err = service.validateRuntimeToolConnection(ctx, product, input)
			if err != nil {
				return input, err
			}
			if len(input.RequestMapping) == 0 {
				input.RequestMapping = current.RequestMapping
			}
			input.RequestMapping, _, err = normalizeToolRequestMapping(input.RequestMapping)
			if err != nil {
				return input, err
			}
			if len(input.ResponseMapping) == 0 {
				input.ResponseMapping = current.ResponseMapping
			}
			input.ResponseMapping, _, err = normalizeToolResponseMapping(input.ResponseMapping)
			if err != nil {
				return input, err
			}
			if err := validateToolMappings(input.InputSchema, input.Endpoint, input.HTTPMethod, input.RequestMapping); err != nil {
				return input, err
			}
		} else {
			parsed, err := url.Parse(input.Endpoint)
			if err != nil || !validToolEndpoint(input.Endpoint) || !methods[input.HTTPMethod] {
				return input, errors.New("tool endpoint must be a fixed credential-free URL on the configured vendor API origin")
			}
			if len(input.UpstreamAuth) == 0 {
				input.UpstreamAuth = current.UpstreamAuth
			}
			var auth ToolUpstreamAuth
			input.UpstreamAuth, auth, _, err = normalizeToolUpstreamAuth(input.UpstreamAuth, current.UpstreamAuth, current.CredentialID, input.Credential)
			if err != nil {
				return input, err
			}
			if credentialRequired(auth.Type) && input.Credential == "" && !toolCredentialCanBeReused(current.BaseURL, input.Endpoint, current.UpstreamAuth, auth) {
				return input, errors.New("re-enter the upstream credential after changing its destination or authentication configuration")
			}
			if auth.Type == "delegated_oauth" {
				provider, providerErr := service.store.IdentityProvider(ctx, input.ProductID)
				if providerErr != nil || provider.State != "active" || provider.DelegatedAPIOrigin == "" {
					return input, errors.New("configure an active identity provider authorization API origin before editing this tool")
				}
				vendorOrigin, originErr := url.Parse(provider.DelegatedAPIOrigin)
				if originErr != nil || !strings.EqualFold(parsed.Scheme, vendorOrigin.Scheme) || !strings.EqualFold(parsed.Host, vendorOrigin.Host) {
					return input, errors.New("delegated OAuth tool endpoint must use the configured vendor API origin")
				}
			}
			if len(input.RequestMapping) == 0 {
				input.RequestMapping = current.RequestMapping
			}
			input.RequestMapping, _, err = normalizeToolRequestMapping(input.RequestMapping)
			if err != nil {
				return input, err
			}
			if len(input.ResponseMapping) == 0 {
				input.ResponseMapping = current.ResponseMapping
			}
			input.ResponseMapping, _, err = normalizeToolResponseMapping(input.ResponseMapping)
			if err != nil {
				return input, err
			}
			if err := validateToolMappings(input.InputSchema, input.Endpoint, input.HTTPMethod, input.RequestMapping); err != nil {
				return input, err
			}
		}
	} else {
		input.Endpoint = current.BaseURL
		input.HTTPMethod = current.HTTPMethod
		input.UpstreamAuth = current.UpstreamAuth
		input.RequestMapping = current.RequestMapping
		input.ResponseMapping = current.ResponseMapping
	}
	if input.TimeoutMS == 0 {
		input.TimeoutMS = 10_000
	}
	if input.TimeoutMS < 100 || input.TimeoutMS > 60_000 {
		return input, errors.New("tool timeout must be between 100 and 60000 milliseconds")
	}
	policy, _, err := normalizeToolPolicy(input.AuthorizationPolicy, input.HTTPMethod)
	input.AuthorizationPolicy = policy
	return input, err
}

// validateCanonicalToolInput is the authoritative persistence boundary shared
// by direct administration, the Web Interface builder, and publication review.
// Editor normalizers remain useful for exact error messages and credential
// reuse, but they must never be a weaker path around the canonical draft rules.
func (s *Service) validateCanonicalToolInput(ctx context.Context, input ToolInput, credentialPresent bool) (ToolInput, error) {
	if len(input.UpstreamAuth) == 0 {
		input.UpstreamAuth = json.RawMessage(`{"type":"delegated_oauth"}`)
	}
	if len(input.RequestMapping) == 0 {
		input.RequestMapping = json.RawMessage(`{}`)
	}
	if len(input.ResponseMapping) == 0 {
		input.ResponseMapping = json.RawMessage(`{}`)
	}
	if len(input.AuthorizationPolicy) == 0 {
		input.AuthorizationPolicy = json.RawMessage(`{}`)
	}
	var auth ToolUpstreamAuth
	if err := strictJSON(input.UpstreamAuth, &auth); err != nil {
		return input, fmt.Errorf("invalid upstream authentication: %w", err)
	}
	var requestMapping ToolRequestMapping
	if err := strictJSON(input.RequestMapping, &requestMapping); err != nil {
		return input, fmt.Errorf("invalid request mapping: %w", err)
	}
	var responseMapping ToolResponseMapping
	if err := strictJSON(input.ResponseMapping, &responseMapping); err != nil {
		return input, fmt.Errorf("invalid response mapping: %w", err)
	}
	var policy ToolPolicy
	if err := strictJSON(input.AuthorizationPolicy, &policy); err != nil {
		return input, fmt.Errorf("invalid authorization policy: %w", err)
	}
	var requestExample map[string]any
	if len(input.RequestExample) > 0 && string(input.RequestExample) != "null" {
		if err := json.Unmarshal(input.RequestExample, &requestExample); err != nil {
			return input, fmt.Errorf("invalid request example: %w", err)
		}
	}
	var responseExample any
	if len(input.ResponseExample) > 0 && string(input.ResponseExample) != "null" {
		if err := json.Unmarshal(input.ResponseExample, &responseExample); err != nil {
			return input, fmt.Errorf("invalid response example: %w", err)
		}
	}
	draft := ToolDraft{
		Namespace: input.Namespace, Name: input.Name, Description: input.Description,
		HTTPMethod: input.HTTPMethod, Endpoint: input.Endpoint, TimeoutMS: input.TimeoutMS,
		InputSchema: input.InputSchema, OutputSchema: input.OutputSchema, UpstreamAuth: auth,
		RequestMapping: requestMapping, ResponseMapping: responseMapping, AuthorizationPolicy: policy,
		RequestExample: requestExample, ResponseExample: responseExample, CredentialPresent: credentialPresent,
	}
	validation, err := s.validateToolDraft(ctx, input.ProductID, draft, credentialPresent)
	if err != nil {
		return input, err
	}
	if !validation.Valid {
		for _, finding := range validation.Findings {
			if finding.Level == "error" {
				return input, fmt.Errorf("tool draft failed authoritative validation (%s): %s", finding.Code, finding.Message)
			}
		}
		return input, errors.New("tool draft failed authoritative validation")
	}
	normalized := validation.NormalizedDraft
	input.Namespace, input.Name, input.Description = normalized.Namespace, normalized.Name, normalized.Description
	input.HTTPMethod, input.Endpoint, input.TimeoutMS = normalized.HTTPMethod, normalized.Endpoint, normalized.TimeoutMS
	input.InputSchema, input.OutputSchema = normalized.InputSchema, normalized.OutputSchema
	input.UpstreamAuth, err = json.Marshal(normalized.UpstreamAuth)
	if err != nil {
		return input, err
	}
	input.RequestMapping, err = json.Marshal(normalized.RequestMapping)
	if err != nil {
		return input, err
	}
	input.ResponseMapping, err = json.Marshal(normalized.ResponseMapping)
	if err != nil {
		return input, err
	}
	input.AuthorizationPolicy, err = json.Marshal(normalized.AuthorizationPolicy)
	if err != nil {
		return input, err
	}
	input.RequestExample = nil
	if normalized.RequestExample != nil {
		input.RequestExample, err = json.Marshal(normalized.RequestExample)
		if err != nil {
			return input, err
		}
	}
	input.ResponseExample = nil
	if normalized.ResponseExample != nil {
		input.ResponseExample, err = json.Marshal(normalized.ResponseExample)
		if err != nil {
			return input, err
		}
	}
	return input, nil
}

// validateStoredHTTPTool re-runs the same deterministic validation used by the
// editor before a stored draft can be simulated or published. This is
// intentionally read-only: old drafts that relied on superseded schema,
// destination, authentication, or mapping rules must be reviewed and saved
// through the editor instead of being silently rewritten at publication time.
func (s *Service) validateStoredHTTPTool(ctx context.Context, tool model.Tool) error {
	if tool.BackendKind != "http" {
		return nil
	}
	if !toolNamePattern.MatchString(tool.Namespace) || !toolNamePattern.MatchString(tool.Name) {
		return errors.New("stored tool identity is invalid")
	}
	if err := tools.ValidateSchema(tool.OutputSchema); err != nil {
		return fmt.Errorf("output schema: %w", err)
	}
	if tool.RuntimeServiceConnectionID != "" {
		input, credentialPresent, err := runtimeValidationInput(tool)
		if err != nil {
			return err
		}
		_, err = s.validateCanonicalToolInput(ctx, input, credentialPresent)
		return err
	}
	input := ToolInput{
		OrganisationID:      tool.OrganisationID,
		ProductID:           tool.ProductID,
		Namespace:           tool.Namespace,
		Name:                tool.Name,
		Description:         tool.Description,
		InputSchema:         tool.InputSchema,
		OutputSchema:        tool.OutputSchema,
		Endpoint:            tool.BaseURL,
		HTTPMethod:          tool.HTTPMethod,
		UpstreamAuth:        tool.UpstreamAuth,
		RequestMapping:      tool.RequestMapping,
		ResponseMapping:     tool.ResponseMapping,
		RequestExample:      tool.RequestExample,
		ResponseExample:     tool.ResponseExample,
		AuthorizationPolicy: tool.AuthorizationPolicy,
		TimeoutMS:           tool.TimeoutMS,
	}
	if _, err := s.validateCanonicalToolInput(ctx, input, tool.CredentialID != ""); err != nil {
		return err
	}
	normalized, err := normalizeToolEditorInput(ctx, s, tool, input)
	if err != nil {
		return err
	}
	if normalized.Endpoint != tool.BaseURL || normalized.HTTPMethod != tool.HTTPMethod {
		return errors.New("stored endpoint or HTTP method is not canonical; review and save the draft again")
	}
	var auth ToolUpstreamAuth
	if err := json.Unmarshal(normalized.UpstreamAuth, &auth); err != nil {
		return errors.New("stored upstream authentication is invalid")
	}
	var storedAuth ToolUpstreamAuth
	if len(tool.UpstreamAuth) == 0 {
		storedAuth.Type = "delegated_oauth"
	} else if err := strictJSON(tool.UpstreamAuth, &storedAuth); err != nil {
		return errors.New("stored upstream authentication is invalid")
	}
	if storedAuth.Type == "oauth_client_credentials" && storedAuth.TokenEndpointAuthMethod == "" {
		storedAuth.TokenEndpointAuthMethod = "client_secret_basic"
	}
	storedAuthJSON, _ := json.Marshal(storedAuth)
	if !bytes.Equal(storedAuthJSON, normalized.UpstreamAuth) {
		return errors.New("stored upstream authentication is not canonical; review and save the draft again")
	}
	var storedPolicy, normalizedPolicy ToolPolicy
	if err := strictJSON(tool.AuthorizationPolicy, &storedPolicy); err != nil || json.Unmarshal(normalized.AuthorizationPolicy, &normalizedPolicy) != nil {
		return errors.New("stored authorization policy is invalid")
	}
	if !slices.Equal(storedPolicy.RequiredGrants, normalizedPolicy.RequiredGrants) || storedPolicy.ConfirmationRequired != normalizedPolicy.ConfirmationRequired || storedPolicy.IdempotencyRequired != normalizedPolicy.IdempotencyRequired {
		return errors.New("stored authorization policy is not canonical; review and save the draft again")
	}
	if credentialRequired(auth.Type) {
		credential, err := s.ResolveToolCredential(ctx, tool)
		if err != nil {
			return errors.New("stored upstream credential is unavailable")
		}
		for index := range credential {
			credential[index] = 0
		}
	}
	return nil
}

func (s *Service) UpdateTool(ctx context.Context, productID, toolID string, input ToolInput, expected int64, actor Actor) (model.Tool, error) {
	current, err := s.store.Tool(ctx, productID, toolID)
	if err != nil {
		return model.Tool{}, err
	}
	if current.State != "draft" {
		return model.Tool{}, errors.New("published tools are immutable; clone the tool to make a new draft")
	}
	if current.BackendKind == "native" {
		return model.Tool{}, errors.New("native tools are source-managed; change the plugin source and restart DokoSoko")
	}
	input.OrganisationID, input.ProductID = current.OrganisationID, current.ProductID
	input, err = normalizeToolEditorInput(ctx, s, current, input)
	if err != nil {
		return model.Tool{}, err
	}
	var normalizedAuth ToolUpstreamAuth
	if err := strictJSON(input.UpstreamAuth, &normalizedAuth); err != nil {
		return model.Tool{}, err
	}
	credentialPresent := credentialRequired(normalizedAuth.Type) && (input.RuntimeServiceConnectionID != "" || input.Credential != "" || current.CredentialID != "")
	input, err = s.validateCanonicalToolInput(ctx, input, credentialPresent)
	if err != nil {
		return model.Tool{}, err
	}
	editor, err := s.toolEditor()
	if err != nil {
		return model.Tool{}, err
	}
	priorCredentialID := current.CredentialID
	current.Description, current.InputSchema, current.OutputSchema = input.Description, input.InputSchema, input.OutputSchema
	current.RequestExample, current.ResponseExample = input.RequestExample, input.ResponseExample
	current.BaseURL, current.HTTPMethod = input.Endpoint, input.HTTPMethod
	current.RuntimeServiceConnectionID, current.HTTPPath = input.RuntimeServiceConnectionID, input.HTTPPath
	current.UpstreamAuth, current.RequestMapping, current.ResponseMapping = input.UpstreamAuth, input.RequestMapping, input.ResponseMapping
	if current.BackendKind == "http" && current.RuntimeServiceConnectionID == "" {
		var auth ToolUpstreamAuth
		_ = json.Unmarshal(input.UpstreamAuth, &auth)
		if !credentialRequired(auth.Type) {
			current.CredentialID, current.CredentialFingerprint = "", ""
		} else if input.Credential != "" {
			current.CredentialID, current.CredentialFingerprint, err = s.saveToolCredential(ctx, current.OrganisationID, current.APIConnectionID, input.Credential)
			if err != nil {
				return model.Tool{}, err
			}
		}
	}
	current.AuthorizationPolicy, current.TimeoutMS = input.AuthorizationPolicy, input.TimeoutMS
	updated, err := editor.UpdateTool(ctx, current, expected)
	if err != nil {
		if current.CredentialID != "" && current.CredentialID != priorCredentialID {
			err = s.cleanupFailedToolCredential(ctx, current.OrganisationID, current.CredentialID, err)
		}
		return model.Tool{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "tool.updated", TargetType: "tool", TargetID: updated.ID, Prior: map[string]any{"revision": current.Revision}, Current: map[string]any{"revision": updated.Revision, "state": updated.State, "required_grants": input.AuthorizationPolicy}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.Tool{}, err
	}
	return updated, nil
}

type ToolCloneInput struct {
	Namespace  string
	Name       string
	Credential string
	Revision   int64
}

func (s *Service) CloneTool(ctx context.Context, productID, toolID string, input ToolCloneInput, actor Actor) (model.Tool, error) {
	current, err := s.store.Tool(ctx, productID, toolID)
	if err != nil {
		return model.Tool{}, err
	}
	if input.Revision < 1 || current.Revision != input.Revision {
		return model.Tool{}, ErrToolCloneRevisionStale
	}
	if current.State != "draft" && current.State != "published" {
		return model.Tool{}, errors.New("only draft or published tools can be cloned")
	}
	if current.BackendKind == "mcp" {
		return model.Tool{}, errors.New("imported MCP tools cannot be cloned; import a distinct upstream tool through its MCP connection")
	}
	if current.BackendKind == "native" {
		return model.Tool{}, errors.New("native tools are source-managed and cannot be cloned")
	}
	input.Namespace, input.Name = strings.TrimSpace(input.Namespace), strings.TrimSpace(input.Name)
	if !toolNamePattern.MatchString(input.Namespace) || !toolNamePattern.MatchString(input.Name) {
		return model.Tool{}, errors.New("tool namespace and name must be valid lower-case identifiers")
	}
	id, err := randomUUID()
	if err != nil {
		return model.Tool{}, err
	}
	copy := current
	copy.ID, copy.Namespace, copy.Name, copy.State, copy.Revision = id, input.Namespace, input.Name, "draft", 0
	copy.CredentialID, copy.CredentialFingerprint, copy.CredentialPresent = "", "", false
	copy.RuntimeTargets = nil
	copy.CreatedAt, copy.UpdatedAt = s.now(), s.now()
	if copy.RuntimeServiceConnectionID != "" {
		if strings.TrimSpace(input.Credential) != "" {
			return model.Tool{}, errors.New("runtime service credentials are managed on the API connection")
		}
		copy.APIConnectionID, copy.BaseURL, copy.UpstreamAuth = "", "", nil
	} else if copy.BackendKind == "http" {
		copy.APIConnectionID, err = randomUUID()
		if err != nil {
			return model.Tool{}, err
		}
		var auth ToolUpstreamAuth
		copy.UpstreamAuth, auth, _, err = normalizeToolUpstreamAuth(copy.UpstreamAuth, nil, "", input.Credential)
		if err != nil {
			return model.Tool{}, err
		}
		if credentialRequired(auth.Type) {
			copy.CredentialID, copy.CredentialFingerprint, err = s.saveToolCredential(ctx, copy.OrganisationID, copy.APIConnectionID, input.Credential)
			if err != nil {
				return model.Tool{}, err
			}
		}
	}
	created, err := s.store.CreateTool(ctx, copy)
	if err != nil {
		return model.Tool{}, s.cleanupFailedToolCredential(ctx, copy.OrganisationID, copy.CredentialID, err)
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: created.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "tool.cloned", TargetType: "tool", TargetID: created.ID, Current: map[string]any{"source_tool_id": current.ID, "source_revision": current.Revision, "source_state": current.State, "name": created.Namespace + "." + created.Name, "credential_supplied": copy.CredentialID != ""}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.Tool{}, err
	}
	return created, nil
}

func (s *Service) RetireTool(ctx context.Context, productID, toolID string, expected int64, actor Actor) (model.Tool, error) {
	current, err := s.store.Tool(ctx, productID, toolID)
	if err != nil {
		return model.Tool{}, err
	}
	if current.State == "retired" {
		return current, nil
	}
	editor, err := s.toolEditor()
	if err != nil {
		return model.Tool{}, err
	}
	updated, err := editor.RetireTool(ctx, productID, toolID, expected)
	if err != nil {
		return model.Tool{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "tool.retired", TargetType: "tool", TargetID: updated.ID, Prior: map[string]any{"state": current.State}, Current: map[string]any{"state": updated.State, "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.Tool{}, err
	}
	return updated, nil
}

type ToolDryRun struct {
	ToolID               string         `json:"tool_id"`
	Revision             int64          `json:"revision"`
	Valid                bool           `json:"valid"`
	NetworkCallPerformed bool           `json:"network_call_performed"`
	Method               string         `json:"method"`
	DestinationOrigin    string         `json:"destination_origin,omitempty"`
	DestinationPath      string         `json:"destination_path,omitempty"`
	BackendKind          string         `json:"backend_kind"`
	RequiredGrants       []string       `json:"required_grants"`
	ConfirmationRequired bool           `json:"confirmation_required"`
	Risk                 string         `json:"risk"`
	IdempotencyRequired  bool           `json:"idempotency_required"`
	NormalizedArguments  map[string]any `json:"normalized_arguments"`
	Warnings             []string       `json:"warnings"`
}

func (s *Service) DryRunTool(ctx context.Context, productID, toolID string, arguments map[string]any) (ToolDryRun, error) {
	tool, err := s.store.Tool(ctx, productID, toolID)
	if err != nil {
		return ToolDryRun{}, err
	}
	if err := s.validateStoredHTTPTool(ctx, tool); err != nil {
		return ToolDryRun{}, fmt.Errorf("tool requires review before validation: %w", err)
	}
	if err := tools.ValidateArguments(tool.InputSchema, arguments); err != nil {
		return ToolDryRun{}, err
	}
	_, policy, err := normalizeToolPolicy(tool.AuthorizationPolicy, tool.HTTPMethod)
	if err != nil {
		return ToolDryRun{}, err
	}
	result := ToolDryRun{ToolID: tool.ID, Revision: tool.Revision, Valid: true, NetworkCallPerformed: false, Method: tool.HTTPMethod, BackendKind: tool.BackendKind, RequiredGrants: policy.RequiredGrants, ConfirmationRequired: policy.ConfirmationRequired, Risk: policy.Risk, IdempotencyRequired: policy.IdempotencyRequired, NormalizedArguments: arguments, Warnings: []string{}}
	if tool.BackendKind == "http" {
		endpoint := tool.BaseURL
		if tool.RuntimeServiceConnectionID != "" {
			input, _, runtimeErr := runtimeValidationInput(tool)
			if runtimeErr != nil {
				return ToolDryRun{}, runtimeErr
			}
			endpoint = input.Endpoint
		}
		parsed, parseErr := url.Parse(endpoint)
		if parseErr != nil {
			return ToolDryRun{}, errors.New("stored tool destination is invalid")
		}
		result.DestinationOrigin = parsed.Scheme + "://" + parsed.Host
		result.DestinationPath = parsed.EscapedPath()
	}
	if (tool.HTTPMethod == "POST" || tool.HTTPMethod == "PUT" || tool.HTTPMethod == "PATCH" || tool.HTTPMethod == "DELETE") && !policy.IdempotencyRequired {
		result.Warnings = append(result.Warnings, "Mutation is not declared idempotent.")
	}
	return result, nil
}

func (s *Service) validateToolGrantRegistry(ctx context.Context, productID string, tool model.Tool) error {
	catalog, err := s.authorizationCatalog()
	if err != nil {
		return err
	}
	definitions, err := catalog.GrantDefinitions(ctx, productID)
	if err != nil {
		return err
	}
	_, policy, err := normalizeToolPolicy(tool.AuthorizationPolicy, tool.HTTPMethod)
	if err != nil {
		return err
	}
	if missing := missingRegisteredGrants(definitions, policy.RequiredGrants); len(missing) > 0 {
		return fmt.Errorf("register required grants before publishing: %s", strings.Join(missing, ", "))
	}
	return nil
}
