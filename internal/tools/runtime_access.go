package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (r *Runtime) SetPrivateLocalhostHosts(destinations []string) {
	r.privateLocalhostDestinations = make(map[string]struct{}, len(destinations))
	for _, destination := range destinations {
		hostname, port := localDevelopmentDestination(destination)
		if hostname != "" {
			r.privateLocalhostDestinations[net.JoinHostPort(hostname, port)] = struct{}{}
		}
	}
}

func localDevelopmentDestination(raw string) (string, string) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || strings.Contains(raw, "://") || strings.ContainsAny(raw, "/?#@") {
		return "", ""
	}
	hostname, port, err := net.SplitHostPort(raw)
	if err != nil {
		hostname, port = raw, "80"
	}
	hostname = strings.TrimSuffix(strings.Trim(hostname, "[]"), ".")
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort < 1 || parsedPort > 65535 || !identity.IsLocalDevelopmentHostname(hostname) {
		return "", ""
	}
	return hostname, strconv.Itoa(parsedPort)
}

func (r *Runtime) Published(ctx context.Context, productID string) ([]model.Tool, error) {
	return r.store.Tools(ctx, productID, true)
}

func (r *Runtime) Available(ctx context.Context, productID string, grants map[string]bool) ([]model.Tool, error) {
	values, err := r.Published(ctx, productID)
	if err != nil {
		return nil, err
	}
	result := make([]model.Tool, 0, len(values))
	for _, value := range values {
		_, executable := r.executor(value)
		if !value.UpstreamDrifted && executable && grantsAllow(value, grants) {
			result = append(result, value)
		}
	}
	return result, nil
}

func (r *Runtime) authorizeBound(ctx context.Context, tool model.Tool, binding BoundAuthorization, principal Principal, enforceConfirmation bool) error {
	publishedPoint := binding.AuthorizationPoint
	if binding.IntegrationID == "" || binding.ToolID != tool.ID || binding.ToolRevision != tool.Revision || publishedPoint.ID == "" || publishedPoint.IntegrationID != binding.IntegrationID || publishedPoint.Revision != binding.AuthorizationPointRevision || publishedPoint.State != "active" {
		return ErrDenied
	}
	point, err := r.store.AuthorizationPoint(ctx, binding.IntegrationID, publishedPoint.ID)
	if err != nil || point.ID != publishedPoint.ID || point.IntegrationID != binding.IntegrationID || point.Revision != binding.AuthorizationPointRevision || point.State != "active" {
		return ErrDenied
	}
	definitions, err := r.store.GrantDefinitions(ctx, point.DeploymentID)
	if err != nil {
		return ErrDenied
	}
	active := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		active[definition.Key] = definition.State == "active"
	}
	var toolPolicy struct {
		RequiredGrants []string `json:"required_grants"`
	}
	if err := json.Unmarshal(tool.AuthorizationPolicy, &toolPolicy); err != nil {
		return ErrDenied
	}
	requiredGrants := append(append([]string(nil), point.RequiredGrants...), toolPolicy.RequiredGrants...)
	for _, required := range requiredGrants {
		if !active[required] || !principal.Grants[required] {
			return ErrDenied
		}
	}
	if point.DecisionTTLSeconds <= 0 || strings.TrimSpace(principal.AccessEvaluationID) == "" || principal.AccessEvaluatedAt.IsZero() {
		return ErrDenied
	}
	age := r.now().Sub(principal.AccessEvaluatedAt)
	if age < 0 || age > time.Duration(point.DecisionTTLSeconds)*time.Second {
		return ErrDenied
	}
	if enforceConfirmation && point.ConfirmationRequired && !principal.Confirmed {
		return ErrConfirmation
	}
	return nil
}

// AvailableBound returns only tools whose legacy tool policy and exact
// Integration authorization action both allow discovery. Confirmation is
// advertised to clients but is enforced only on execution.
func (r *Runtime) AvailableBound(ctx context.Context, productID string, bindings []BoundAuthorization, principal Principal) ([]model.Tool, error) {
	values, err := r.Published(ctx, productID)
	if err != nil {
		return nil, err
	}
	byTool := make(map[string][]BoundAuthorization, len(bindings))
	for _, binding := range bindings {
		byTool[binding.ToolID] = append(byTool[binding.ToolID], binding)
	}
	result := make([]model.Tool, 0, len(values))
	for _, value := range values {
		candidates := byTool[value.ID]
		_, executable := r.executor(value)
		if value.UpstreamDrifted || !executable || len(candidates) != 1 || !grantsAllow(value, principal.Grants) || r.authorizeBound(ctx, value, candidates[0], principal, false) != nil {
			continue
		}
		result = append(result, value)
	}
	return result, nil
}

func (r *Runtime) find(ctx context.Context, productID, fullName string) (model.Tool, error) {
	values, err := r.Published(ctx, productID)
	if err != nil {
		return model.Tool{}, err
	}
	for _, value := range values {
		_, executable := r.executor(value)
		if executable && value.Namespace+"."+value.Name == fullName {
			return value, nil
		}
	}
	return model.Tool{}, errors.New("published tool not found")
}

func authorize(tool model.Tool, principal Principal) error {
	var policy struct {
		RequiredGrants       []string `json:"required_grants"`
		ConfirmationRequired bool     `json:"confirmation_required"`
	}
	if err := json.Unmarshal(tool.AuthorizationPolicy, &policy); err != nil {
		return ErrDenied
	}
	for _, required := range policy.RequiredGrants {
		if !principal.Grants[required] {
			return ErrDenied
		}
	}
	if policy.ConfirmationRequired && !principal.Confirmed {
		return ErrConfirmation
	}
	return nil
}

func grantsAllow(tool model.Tool, grants map[string]bool) bool {
	var policy struct {
		RequiredGrants []string `json:"required_grants"`
	}
	if err := json.Unmarshal(tool.AuthorizationPolicy, &policy); err != nil {
		return false
	}
	for _, required := range policy.RequiredGrants {
		if !grants[required] {
			return false
		}
	}
	return true
}

func (r *Runtime) ExecuteBound(ctx context.Context, productID, fullName string, arguments map[string]any, principal Principal, binding BoundAuthorization) (any, error) {
	tool, err := r.find(ctx, productID, fullName)
	if err != nil {
		return nil, err
	}
	if tool.UpstreamDrifted {
		return nil, ErrDenied
	}
	if err := ValidateArguments(tool.InputSchema, arguments); err != nil {
		return nil, err
	}
	if err := authorize(tool, principal); err != nil {
		return nil, err
	}
	if err := r.authorizeBound(ctx, tool, binding, principal, true); err != nil {
		return nil, err
	}
	decisionID := ""
	if tool.RuntimeServiceConnectionID != "" {
		prepared, prepareErr := prepareRuntimeTool(tool, principal.EnvironmentID)
		if prepareErr != nil {
			return nil, ErrDenied
		}
		decisionID, err = r.evaluateAuthorization(ctx, productID, fullName, prepared, principal, binding)
		if err != nil {
			return nil, ErrDenied
		}
	}
	result, err := r.executeAuthorized(ctx, productID, fullName, tool, arguments, principal)
	if err != nil {
		return nil, err
	}
	if tool.RuntimeServiceConnectionID != "" {
		prepared, prepareErr := prepareRuntimeTool(tool, principal.EnvironmentID)
		if prepareErr == nil {
			r.enqueueAuthorizationUsage(ctx, fullName, decisionID, prepared, principal, binding)
		}
	}
	return result, nil
}

func sameOrigin(left, right string) bool {
	a, errA := url.Parse(left)
	b, errB := url.Parse(right)
	return errA == nil && errB == nil && strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
