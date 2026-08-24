package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/tools"
)

type toolEditorStore interface {
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
	if policy.Risk == "" {
		policy.Risk = "low"
		if method != "GET" {
			policy.Risk = "medium"
		}
		if method == "DELETE" {
			policy.Risk = "critical"
		}
	}
	if policy.Risk != "low" && policy.Risk != "medium" && policy.Risk != "high" && policy.Risk != "critical" {
		return nil, ToolPolicy{}, errors.New("tool risk must be low, medium, high, or critical")
	}
	if policy.Risk == "critical" && !policy.ConfirmationRequired {
		return nil, ToolPolicy{}, errors.New("critical tools require confirmation")
	}
	if method != "GET" && policy.IdempotencyRequired == false {
		// Mutations may explicitly remain non-idempotent, but that fact is retained
		// in policy and becomes a publication-readiness warning.
		policy.IdempotencyRequired = false
	}
	normalized, err := json.Marshal(policy)
	return normalized, policy, err
}

func normalizeToolEditorInput(ctx context.Context, service *Service, current model.Tool, input ToolInput) (ToolInput, error) {
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
	if current.BackendKind == "http" {
		methods := map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
		parsed, err := url.Parse(input.Endpoint)
		if err != nil || !validToolEndpoint(input.Endpoint) || !methods[input.HTTPMethod] {
			return input, errors.New("tool endpoint must be a fixed credential-free URL on the configured vendor API origin")
		}
		provider, err := service.store.IdentityProvider(ctx, input.ProductID)
		if err != nil || provider.State != "active" || provider.DelegatedAPIOrigin == "" {
			return input, errors.New("configure an active identity provider delegated API origin before editing an HTTP tool")
		}
		vendorOrigin, err := url.Parse(provider.DelegatedAPIOrigin)
		if err != nil || !strings.EqualFold(parsed.Scheme, vendorOrigin.Scheme) || !strings.EqualFold(parsed.Host, vendorOrigin.Host) {
			return input, errors.New("tool endpoint must use the configured vendor API origin")
		}
	} else {
		input.Endpoint = current.BaseURL
		input.HTTPMethod = current.HTTPMethod
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

func (s *Service) UpdateTool(ctx context.Context, productID, toolID string, input ToolInput, expected int64, actor Actor) (model.Tool, error) {
	current, err := s.store.Tool(ctx, productID, toolID)
	if err != nil {
		return model.Tool{}, err
	}
	if current.State != "draft" {
		return model.Tool{}, errors.New("published tools are immutable; clone the tool to make a new draft")
	}
	input.OrganisationID, input.ProductID = current.OrganisationID, current.ProductID
	input, err = normalizeToolEditorInput(ctx, s, current, input)
	if err != nil {
		return model.Tool{}, err
	}
	editor, err := s.toolEditor()
	if err != nil {
		return model.Tool{}, err
	}
	current.Description, current.InputSchema, current.OutputSchema = input.Description, input.InputSchema, input.OutputSchema
	current.BaseURL, current.HTTPMethod = input.Endpoint, input.HTTPMethod
	current.AuthorizationPolicy, current.TimeoutMS = input.AuthorizationPolicy, input.TimeoutMS
	updated, err := editor.UpdateTool(ctx, current, expected)
	if err != nil {
		return model.Tool{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "tool.updated", TargetType: "tool", TargetID: updated.ID, Prior: map[string]any{"revision": current.Revision}, Current: map[string]any{"revision": updated.Revision, "state": updated.State, "required_grants": input.AuthorizationPolicy}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

type ToolCloneInput struct {
	Namespace string
	Name      string
}

func (s *Service) CloneTool(ctx context.Context, productID, toolID string, input ToolCloneInput, actor Actor) (model.Tool, error) {
	current, err := s.store.Tool(ctx, productID, toolID)
	if err != nil {
		return model.Tool{}, err
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
	copy.CreatedAt, copy.UpdatedAt = s.now(), s.now()
	if copy.BackendKind == "http" {
		copy.APIConnectionID, err = randomUUID()
		if err != nil {
			return model.Tool{}, err
		}
	}
	created, err := s.store.CreateTool(ctx, copy)
	if err != nil {
		return model.Tool{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: created.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "tool.cloned", TargetType: "tool", TargetID: created.ID, Current: map[string]any{"source_tool_id": current.ID, "name": created.Namespace + "." + created.Name}, RequestID: actor.RequestID, CreatedAt: s.now()})
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
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "tool.retired", TargetType: "tool", TargetID: updated.ID, Prior: map[string]any{"state": current.State}, Current: map[string]any{"state": updated.State, "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()})
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
	if err := tools.ValidateArguments(tool.InputSchema, arguments); err != nil {
		return ToolDryRun{}, err
	}
	_, policy, err := normalizeToolPolicy(tool.AuthorizationPolicy, tool.HTTPMethod)
	if err != nil {
		return ToolDryRun{}, err
	}
	result := ToolDryRun{ToolID: tool.ID, Revision: tool.Revision, Valid: true, NetworkCallPerformed: false, Method: tool.HTTPMethod, BackendKind: tool.BackendKind, RequiredGrants: policy.RequiredGrants, ConfirmationRequired: policy.ConfirmationRequired, Risk: policy.Risk, IdempotencyRequired: policy.IdempotencyRequired, NormalizedArguments: arguments, Warnings: []string{}}
	if tool.BackendKind == "http" {
		parsed, parseErr := url.Parse(tool.BaseURL)
		if parseErr != nil {
			return ToolDryRun{}, errors.New("stored tool destination is invalid")
		}
		result.DestinationOrigin = parsed.Scheme + "://" + parsed.Host
		result.DestinationPath = parsed.EscapedPath()
	}
	if tool.HTTPMethod != "GET" && !policy.IdempotencyRequired {
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
