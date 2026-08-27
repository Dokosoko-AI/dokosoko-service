package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func validateRuntimeHTTPPath(value string) error {
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || len(value) > 2048 || strings.ContainsAny(value, "?#\\\r\n\x00") {
		return errors.New("runtime tool HTTP path must be one relative absolute-path without a query or fragment")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." || strings.EqualFold(segment, "%2e") || strings.EqualFold(segment, "%2e%2e") {
			return errors.New("runtime tool HTTP path cannot contain dot segments")
		}
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil {
		return errors.New("runtime tool HTTP path is invalid")
	}
	return nil
}

func composeRuntimeToolEndpoint(baseURL, httpPath string) (string, error) {
	if err := validateRuntimeHTTPPath(httpPath); err != nil {
		return "", err
	}
	value := strings.TrimRight(strings.TrimSpace(baseURL), "/") + httpPath
	if !validToolEndpoint(value) {
		return "", errors.New("runtime service endpoint and tool path do not form a safe fixed URL")
	}
	return value, nil
}

func runtimeTargetAuth(authenticationType string, authConfig json.RawMessage, headerName string) (json.RawMessage, ToolUpstreamAuth, error) {
	object := map[string]any{}
	if len(authConfig) > 0 && string(authConfig) != "null" {
		if err := json.Unmarshal(authConfig, &object); err != nil || object == nil {
			return nil, ToolUpstreamAuth{}, errors.New("runtime service authentication configuration is invalid")
		}
	}
	object["type"] = authenticationType
	if authenticationType == "api_key_header" || authenticationType == "custom_header" {
		object["header_name"] = headerName
	}
	raw, err := json.Marshal(object)
	if err != nil {
		return nil, ToolUpstreamAuth{}, err
	}
	canonical, auth, _, err := normalizeToolUpstreamAuth(raw, raw, "runtime-credential", "")
	return canonical, auth, err
}

func (s *Service) validateRuntimeToolConnection(ctx context.Context, product model.Product, input ToolInput) (string, json.RawMessage, error) {
	if input.Scope != model.ToolScopeAPI || input.OwnerIntegrationID == "" {
		return "", nil, errors.New("runtime service connections can only be used by API-owned tools")
	}
	if err := validateRuntimeHTTPPath(input.HTTPPath); err != nil {
		return "", nil, err
	}
	connection, err := s.store.RuntimeServiceConnection(ctx, product.ID, input.RuntimeServiceConnectionID)
	if err != nil || connection.OrganisationID != product.OrganisationID || connection.IntegrationID != input.OwnerIntegrationID || connection.State != "active" {
		return "", nil, errors.New("runtime service connection must be active and owned by the tool's API")
	}
	environments, err := s.store.Environments(ctx, product.ID)
	if err != nil {
		return "", nil, err
	}
	knownEnvironments := make(map[string]bool, len(environments))
	for _, environment := range environments {
		knownEnvironments[environment.ID] = true
	}
	if len(connection.CurrentRevisions) == 0 {
		return "", nil, errors.New("runtime service connection has no current environment configuration")
	}
	var representativeEndpoint string
	var representativeAuth json.RawMessage
	for _, revision := range connection.CurrentRevisions {
		if !knownEnvironments[revision.EnvironmentID] || revision.ConnectionID != connection.ID {
			return "", nil, errors.New("runtime service connection contains a cross-environment configuration")
		}
		endpoint, composeErr := composeRuntimeToolEndpoint(revision.BaseURL, input.HTTPPath)
		if composeErr != nil {
			return "", nil, composeErr
		}
		headerName := ""
		authConfig := revision.AuthConfig
		if runtimeAuthenticationNeedsCredential(revision.AuthenticationType) {
			credentialSet, credentialErr := s.store.RuntimeCredentialSet(ctx, product.ID, revision.CredentialSetID)
			if credentialErr != nil || credentialSet.State != "active" || credentialSet.EnvironmentID != revision.EnvironmentID || credentialSet.AuthenticationType != revision.AuthenticationType || credentialSet.Scope == "dedicated" && credentialSet.OwnerIntegrationID != connection.IntegrationID || credentialSet.Scope == "shared" && credentialSet.OwnerIntegrationID != "" || !credentialSet.CredentialPresent {
				return "", nil, errors.New("runtime service credential is not active or eligible for this API and environment")
			}
			headerName = credentialSet.HeaderName
			authConfig = credentialSet.AuthConfig
		}
		authRaw, _, authErr := runtimeTargetAuth(revision.AuthenticationType, authConfig, headerName)
		if authErr != nil {
			return "", nil, fmt.Errorf("runtime service authentication: %w", authErr)
		}
		if representativeEndpoint == "" {
			representativeEndpoint, representativeAuth = endpoint, authRaw
		}
	}
	return representativeEndpoint, representativeAuth, nil
}

func runtimeValidationInput(tool model.Tool) (ToolInput, bool, error) {
	if tool.RuntimeServiceConnectionID == "" {
		return ToolInput{}, false, nil
	}
	if len(tool.RuntimeTargets) == 0 {
		return ToolInput{}, true, errors.New("runtime service connection has no pinned or current target")
	}
	target := tool.RuntimeTargets[0]
	endpoint, err := composeRuntimeToolEndpoint(target.BaseURL, tool.HTTPPath)
	if err != nil {
		return ToolInput{}, true, err
	}
	authRaw, auth, err := runtimeTargetAuth(target.AuthenticationType, target.AuthConfig, target.HeaderName)
	if err != nil {
		return ToolInput{}, true, err
	}
	for _, candidate := range tool.RuntimeTargets {
		if credentialRequired(candidate.AuthenticationType) && candidate.CredentialSecretID == "" {
			return ToolInput{}, true, errors.New("runtime service credential is unavailable")
		}
	}
	return ToolInput{
		OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, Scope: tool.Scope, OwnerIntegrationID: tool.OwnerIntegrationID,
		RuntimeServiceConnectionID: tool.RuntimeServiceConnectionID, HTTPPath: tool.HTTPPath,
		Namespace: tool.Namespace, Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema, OutputSchema: tool.OutputSchema,
		Endpoint: endpoint, HTTPMethod: tool.HTTPMethod, UpstreamAuth: authRaw, RequestMapping: tool.RequestMapping, ResponseMapping: tool.ResponseMapping,
		RequestExample: tool.RequestExample, ResponseExample: tool.ResponseExample, AuthorizationPolicy: tool.AuthorizationPolicy, TimeoutMS: tool.TimeoutMS,
	}, credentialRequired(auth.Type), nil
}
