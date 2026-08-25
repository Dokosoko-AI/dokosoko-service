package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

func toolBuilderFinding(level, code, field, message string) ToolDraftFinding {
	return ToolDraftFinding{Level: level, Code: code, Field: field, Message: message}
}

func containsToolBuilderSecretText(value string) bool {
	if toolBuilderSecretAssignment.MatchString(value) || toolBuilderJWTValue.MatchString(value) || toolBuilderKnownSecretValue.MatchString(value) || toolBuilderURLUserInfo.MatchString(value) {
		return true
	}
	nonValues := map[string]bool{"authentication": true, "authorization": true, "credential": true, "credentials": true, "token": true, "tokens": true}
	for _, pattern := range []*regexp.Regexp{toolBuilderBearerValue, toolBuilderBasicValue} {
		for _, match := range pattern.FindAllStringSubmatch(value, -1) {
			candidate := ""
			if len(match) > 1 {
				candidate = strings.Trim(strings.ToLower(match[1]), ".,;:!?")
			}
			if candidate != "" && !nonValues[candidate] {
				return true
			}
		}
	}
	return false
}

func containsToolBuilderSecretValue(value any) bool {
	switch current := value.(type) {
	case string:
		return containsToolBuilderSecretText(current)
	case json.RawMessage:
		var decoded any
		if json.Unmarshal(current, &decoded) != nil {
			return containsToolBuilderSecretText(string(current))
		}
		return containsToolBuilderSecretValue(decoded)
	case map[string]any:
		for key, child := range current {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
			if normalized == "authorization" || normalized == "api_key" || normalized == "access_token" || normalized == "refresh_token" || normalized == "client_secret" || normalized == "password" || normalized == "secret" {
				if schema, ok := child.(map[string]any); !ok || schema["type"] == nil {
					return true
				}
			}
			if containsToolBuilderSecretValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range current {
			if containsToolBuilderSecretValue(child) {
				return true
			}
		}
	}
	return false
}

func sanitizeToolBuilderEndpoint(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || containsToolBuilderSecretText(parsed.Path) {
		return "", raw != ""
	}
	changed := parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != ""
	path := strings.NewReplacer("%7B", "{", "%7D", "}", "%7b", "{", "%7d", "}").Replace(parsed.EscapedPath())
	return parsed.Scheme + "://" + parsed.Host + path, changed
}

func sanitizeToolBuilderTokenURL(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return "", false
	}
	return sanitizeToolBuilderEndpoint(raw)
}

func defaultToolDraft() ToolDraft {
	return ToolDraft{
		HTTPMethod:          "GET",
		TimeoutMS:           10_000,
		InputSchema:         append(json.RawMessage(nil), emptyToolBuilderSchema...),
		OutputSchema:        append(json.RawMessage(nil), emptyToolBuilderSchema...),
		UpstreamAuth:        ToolUpstreamAuth{Type: "delegated_oauth"},
		RequestMapping:      ToolRequestMapping{ParameterLocations: map[string]string{}},
		ResponseMapping:     ToolResponseMapping{},
		AuthorizationPolicy: ToolPolicy{RequiredGrants: []string{}, Risk: "low"},
	}
}

func stripIrrelevantToolBuilderAuth(auth ToolUpstreamAuth) ToolUpstreamAuth {
	switch auth.Type {
	case "delegated_oauth", "none":
		return ToolUpstreamAuth{Type: auth.Type}
	case "bearer":
		return ToolUpstreamAuth{Type: auth.Type, Prefix: auth.Prefix}
	case "authorization_scheme":
		return ToolUpstreamAuth{Type: auth.Type, Scheme: auth.Scheme}
	case "api_key_header", "custom_header":
		return ToolUpstreamAuth{Type: auth.Type, HeaderName: auth.HeaderName, Prefix: auth.Prefix}
	case "api_key_query":
		return ToolUpstreamAuth{Type: auth.Type, QueryName: auth.QueryName}
	case "basic":
		return ToolUpstreamAuth{Type: auth.Type, Username: auth.Username}
	case "oauth_client_credentials":
		return ToolUpstreamAuth{Type: auth.Type, ClientID: auth.ClientID, TokenURL: auth.TokenURL, TokenEndpointAuthMethod: auth.TokenEndpointAuthMethod, Scopes: auth.Scopes, Audience: auth.Audience, Resource: auth.Resource}
	default:
		return ToolUpstreamAuth{Type: "none"}
	}
}

// sanitizePartialToolBuilderAuth keeps safe, useful fields from an incomplete
// manual/import/AI draft. Authoritative validation can still mark the draft
// invalid, but it must not erase a discovered OAuth token URL, API-key name,
// or other non-secret metadata merely because the user has not supplied every
// required field yet.
func sanitizePartialToolBuilderAuth(auth ToolUpstreamAuth) ToolUpstreamAuth {
	auth = stripIrrelevantToolBuilderAuth(auth)
	if auth.Scheme != "" && !validAuthorizationScheme(auth.Scheme) {
		auth.Scheme = ""
	}
	if auth.HeaderName != "" && !safeCustomHeader(auth.HeaderName) {
		auth.HeaderName = ""
	}
	if auth.QueryName != "" && !toolQueryNamePattern.MatchString(auth.QueryName) {
		auth.QueryName = ""
	}
	if len(auth.Prefix) > 64 || strings.ContainsAny(auth.Prefix, "\r\n\x00") {
		auth.Prefix = ""
	}
	if len(auth.Username) > 255 || strings.ContainsAny(auth.Username, ":\r\n\x00") {
		auth.Username = ""
	}
	if len(auth.ClientID) > 255 || strings.ContainsAny(auth.ClientID, "\r\n\x00") {
		auth.ClientID = ""
	}
	if auth.TokenEndpointAuthMethod != "" && auth.TokenEndpointAuthMethod != "client_secret_basic" && auth.TokenEndpointAuthMethod != "client_secret_post" {
		auth.TokenEndpointAuthMethod = ""
	}
	if len(auth.Audience) > 500 || strings.ContainsAny(auth.Audience, "\r\n\x00") {
		auth.Audience = ""
	}
	if len(auth.Resource) > 500 || strings.ContainsAny(auth.Resource, "\r\n\x00") {
		auth.Resource = ""
	}
	seenScopes := map[string]bool{}
	scopes := make([]string, 0, len(auth.Scopes))
	for _, scope := range auth.Scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || seenScopes[scope] || len(scope) > 200 || strings.ContainsAny(scope, "\r\n\x00") {
			continue
		}
		seenScopes[scope] = true
		scopes = append(scopes, scope)
		if len(scopes) == 32 {
			break
		}
	}
	sort.Strings(scopes)
	auth.Scopes = scopes
	return auth
}

func toolBuilderJSONSize(value any) (int, error) {
	encoded, err := json.Marshal(value)
	return len(encoded), err
}

func toolBuilderJSONObject(raw json.RawMessage) bool {
	var value map[string]any
	return json.Unmarshal(raw, &value) == nil && value != nil
}

func normalizeToolBuilderDraft(draft ToolDraft) (ToolDraft, []ToolDraftFinding) {
	findings := make([]ToolDraftFinding, 0)
	draft.Namespace = strings.ToLower(strings.TrimSpace(draft.Namespace))
	draft.Name = strings.ToLower(strings.TrimSpace(draft.Name))
	draft.Description = strings.TrimSpace(draft.Description)
	draft.HTTPMethod = strings.ToUpper(strings.TrimSpace(draft.HTTPMethod))
	if containsToolBuilderSecretText(draft.Namespace) || containsToolBuilderSecretText(draft.Name) {
		draft.Namespace, draft.Name = "", ""
		findings = append(findings, toolBuilderFinding("error", "credential_material_removed", "identity", "Credential-like material was removed from the tool identity."))
	}
	if len(draft.Namespace) > 64 || len(draft.Name) > 64 {
		draft.Namespace, draft.Name = "", ""
		findings = append(findings, toolBuilderFinding("error", "invalid_identity", "identity", "Tool namespace and name must each contain at most 64 characters."))
	}
	if containsToolBuilderSecretText(draft.HTTPMethod) || len(draft.HTTPMethod) > 12 {
		draft.HTTPMethod = ""
		findings = append(findings, toolBuilderFinding("error", "invalid_http_method", "http_method", "HTTP method is invalid."))
	}
	if draft.HTTPMethod == "" {
		draft.HTTPMethod = "GET"
	}
	if draft.TimeoutMS == 0 {
		draft.TimeoutMS = 10_000
	}
	if containsToolBuilderSecretText(draft.Description) {
		draft.Description = ""
		findings = append(findings, toolBuilderFinding("error", "credential_material_removed", "description", "Credential-like material was removed from the draft."))
	} else if utf8.RuneCountInString(draft.Description) > 500 {
		draft.Description = ""
		findings = append(findings, toolBuilderFinding("error", "invalid_description", "description", "Description must contain at most 500 characters."))
	}
	var endpointChanged bool
	if len(draft.Endpoint) > 8<<10 {
		draft.Endpoint = ""
		findings = append(findings, toolBuilderFinding("error", "invalid_endpoint", "endpoint", "Endpoint must contain at most 8 KiB."))
	} else {
		draft.Endpoint, endpointChanged = sanitizeToolBuilderEndpoint(draft.Endpoint)
	}
	if endpointChanged {
		findings = append(findings, toolBuilderFinding("error", "endpoint_must_be_credential_free", "endpoint", "Endpoint user information, query values, or fragments are not allowed and were removed."))
	}

	if len(draft.InputSchema) == 0 {
		draft.InputSchema = append(json.RawMessage(nil), emptyToolBuilderSchema...)
	} else if len(draft.InputSchema) > 64<<10 || !json.Valid(draft.InputSchema) || !toolBuilderJSONObject(draft.InputSchema) {
		draft.InputSchema = append(json.RawMessage(nil), emptyToolBuilderSchema...)
		findings = append(findings, toolBuilderFinding("error", "invalid_input_schema", "input_schema", "Input schema must be one JSON object no larger than 64 KiB."))
	}
	if containsToolBuilderSecretValue(draft.InputSchema) {
		draft.InputSchema = append(json.RawMessage(nil), emptyToolBuilderSchema...)
		findings = append(findings, toolBuilderFinding("error", "credential_material_removed", "input_schema", "Credential-like material was removed from the input schema."))
	}
	if len(draft.OutputSchema) == 0 {
		draft.OutputSchema = append(json.RawMessage(nil), emptyToolBuilderSchema...)
	} else if len(draft.OutputSchema) > 64<<10 || !json.Valid(draft.OutputSchema) || !toolBuilderJSONObject(draft.OutputSchema) {
		draft.OutputSchema = append(json.RawMessage(nil), emptyToolBuilderSchema...)
		findings = append(findings, toolBuilderFinding("error", "invalid_output_schema", "output_schema", "Output schema must be one JSON object no larger than 64 KiB."))
	}
	if containsToolBuilderSecretValue(draft.OutputSchema) {
		draft.OutputSchema = append(json.RawMessage(nil), emptyToolBuilderSchema...)
		findings = append(findings, toolBuilderFinding("error", "credential_material_removed", "output_schema", "Credential-like material was removed from the output schema."))
	}

	draft.UpstreamAuth.Type = strings.ToLower(strings.TrimSpace(draft.UpstreamAuth.Type))
	if draft.UpstreamAuth.Type == "" {
		draft.UpstreamAuth.Type = "delegated_oauth"
	}
	draft.UpstreamAuth.HeaderName = strings.TrimSpace(draft.UpstreamAuth.HeaderName)
	draft.UpstreamAuth.Scheme = strings.TrimSpace(draft.UpstreamAuth.Scheme)
	draft.UpstreamAuth.QueryName = strings.TrimSpace(draft.UpstreamAuth.QueryName)
	draft.UpstreamAuth.Prefix = strings.TrimSpace(draft.UpstreamAuth.Prefix)
	draft.UpstreamAuth.Username = strings.TrimSpace(draft.UpstreamAuth.Username)
	draft.UpstreamAuth.ClientID = strings.TrimSpace(draft.UpstreamAuth.ClientID)
	draft.UpstreamAuth.TokenEndpointAuthMethod = strings.ToLower(strings.TrimSpace(draft.UpstreamAuth.TokenEndpointAuthMethod))
	draft.UpstreamAuth.Audience = strings.TrimSpace(draft.UpstreamAuth.Audience)
	draft.UpstreamAuth.Resource = strings.TrimSpace(draft.UpstreamAuth.Resource)
	draft.UpstreamAuth.TokenURL, endpointChanged = sanitizeToolBuilderTokenURL(draft.UpstreamAuth.TokenURL)
	if endpointChanged {
		findings = append(findings, toolBuilderFinding("error", "token_url_must_be_credential_free", "upstream_auth.token_url", "OAuth token URL user information, query values, or fragments are not allowed and were removed."))
	}
	if containsToolBuilderSecretText(draft.UpstreamAuth.Type) || containsToolBuilderSecretText(draft.UpstreamAuth.Scheme) || containsToolBuilderSecretText(draft.UpstreamAuth.HeaderName) || containsToolBuilderSecretText(draft.UpstreamAuth.QueryName) || containsToolBuilderSecretText(draft.UpstreamAuth.Prefix) || containsToolBuilderSecretText(draft.UpstreamAuth.Username) || containsToolBuilderSecretText(draft.UpstreamAuth.ClientID) || containsToolBuilderSecretText(draft.UpstreamAuth.TokenEndpointAuthMethod) || containsToolBuilderSecretText(draft.UpstreamAuth.Audience) || containsToolBuilderSecretText(draft.UpstreamAuth.Resource) || containsToolBuilderSecretText(strings.Join(draft.UpstreamAuth.Scopes, " ")) {
		draft.UpstreamAuth.Type, draft.UpstreamAuth.Scheme, draft.UpstreamAuth.HeaderName, draft.UpstreamAuth.QueryName, draft.UpstreamAuth.Prefix = "none", "", "", "", ""
		draft.UpstreamAuth.Username, draft.UpstreamAuth.ClientID, draft.UpstreamAuth.TokenEndpointAuthMethod, draft.UpstreamAuth.Audience, draft.UpstreamAuth.Resource = "", "", "", "", ""
		draft.UpstreamAuth.Scopes = nil
		findings = append(findings, toolBuilderFinding("error", "credential_material_removed", "upstream_auth", "Credential-like material was removed from authentication configuration."))
	}
	allowedAuth := map[string]bool{"delegated_oauth": true, "none": true, "bearer": true, "authorization_scheme": true, "api_key_header": true, "api_key_query": true, "basic": true, "oauth_client_credentials": true, "custom_header": true}
	if !allowedAuth[draft.UpstreamAuth.Type] {
		draft.UpstreamAuth = ToolUpstreamAuth{Type: "none"}
		findings = append(findings, toolBuilderFinding("error", "invalid_upstream_auth", "upstream_auth.type", "Upstream authentication type is not supported."))
	}
	draft.UpstreamAuth = sanitizePartialToolBuilderAuth(draft.UpstreamAuth)
	authRaw, _ := json.Marshal(draft.UpstreamAuth)
	if len(authRaw) > 8<<10 {
		draft.UpstreamAuth = ToolUpstreamAuth{Type: draft.UpstreamAuth.Type}
		authRaw, _ = json.Marshal(draft.UpstreamAuth)
		findings = append(findings, toolBuilderFinding("error", "invalid_upstream_auth", "upstream_auth", "Upstream authentication configuration is too large."))
	}
	if _, normalized, _, err := normalizeToolUpstreamAuth(authRaw, authRaw, "candidate-credential", ""); err != nil {
		findings = append(findings, toolBuilderFinding("error", "invalid_upstream_auth", "upstream_auth", "Upstream authentication configuration is invalid or incomplete."))
		// Preserve the already-sanitized partial configuration so the user can
		// complete it in the form. It is still invalid and cannot be saved or
		// published until authoritative validation passes.
		draft.UpstreamAuth = sanitizePartialToolBuilderAuth(draft.UpstreamAuth)
	} else {
		draft.UpstreamAuth = stripIrrelevantToolBuilderAuth(normalized)
	}
	if credentialRequired(draft.UpstreamAuth.Type) && !draft.CredentialPresent {
		findings = append(findings, toolBuilderFinding("error", "credential_required", "credential_present", "This authentication mode requires a credential to be stored separately before the tool can be published."))
	}
	if !credentialRequired(draft.UpstreamAuth.Type) && draft.CredentialPresent {
		draft.CredentialPresent = false
		findings = append(findings, toolBuilderFinding("warning", "credential_not_used", "credential_present", "This authentication mode does not use a stored tool credential."))
	}

	if draft.RequestMapping.ParameterLocations == nil {
		draft.RequestMapping.ParameterLocations = map[string]string{}
	}
	for name, location := range draft.RequestMapping.ParameterLocations {
		if containsToolBuilderSecretText(name) || containsToolBuilderSecretText(location) {
			delete(draft.RequestMapping.ParameterLocations, name)
			findings = append(findings, toolBuilderFinding("error", "credential_material_removed", "request_mapping", "Credential-like material was removed from request mapping."))
		}
	}
	requestRaw, _ := json.Marshal(draft.RequestMapping)
	if _, normalized, err := normalizeToolRequestMapping(requestRaw); err != nil {
		findings = append(findings, toolBuilderFinding("error", "invalid_request_mapping", "request_mapping", "Request mapping contains an invalid parameter name or location."))
		draft.RequestMapping = ToolRequestMapping{ParameterLocations: map[string]string{}}
	} else {
		draft.RequestMapping = normalized
	}
	responseRaw, _ := json.Marshal(draft.ResponseMapping)
	if containsToolBuilderSecretText(draft.ResponseMapping.ResultPath) {
		draft.ResponseMapping.ResultPath = ""
		responseRaw = json.RawMessage(`{}`)
		findings = append(findings, toolBuilderFinding("error", "credential_material_removed", "response_mapping", "Credential-like material was removed from response mapping."))
	}
	if _, normalized, err := normalizeToolResponseMapping(responseRaw); err != nil {
		findings = append(findings, toolBuilderFinding("error", "invalid_response_mapping", "response_mapping", "Response mapping is invalid."))
		draft.ResponseMapping = ToolResponseMapping{}
	} else {
		draft.ResponseMapping = normalized
	}
	policyRaw, _ := json.Marshal(draft.AuthorizationPolicy)
	for _, grant := range draft.AuthorizationPolicy.RequiredGrants {
		if containsToolBuilderSecretText(grant) {
			draft.AuthorizationPolicy.RequiredGrants = nil
			policyRaw, _ = json.Marshal(draft.AuthorizationPolicy)
			findings = append(findings, toolBuilderFinding("error", "credential_material_removed", "authorization_policy", "Credential-like material was removed from authorization policy."))
			break
		}
	}
	if _, normalized, err := normalizeToolPolicy(policyRaw, draft.HTTPMethod); err != nil {
		findings = append(findings, toolBuilderFinding("error", "invalid_authorization_policy", "authorization_policy", "Authorization policy is invalid."))
		risk := "low"
		if draft.HTTPMethod != "GET" {
			risk = "medium"
		}
		if draft.HTTPMethod == "DELETE" {
			risk = "critical"
		}
		draft.AuthorizationPolicy = ToolPolicy{RequiredGrants: []string{}, Risk: risk, ConfirmationRequired: draft.HTTPMethod == "DELETE", IdempotencyRequired: draft.HTTPMethod != "GET"}
	} else {
		draft.AuthorizationPolicy = normalized
	}
	if draft.AuthorizationPolicy.RequiredGrants == nil {
		draft.AuthorizationPolicy.RequiredGrants = []string{}
	}

	if size, err := toolBuilderJSONSize(draft.RequestExample); err != nil || size > 64<<10 {
		draft.RequestExample = nil
		findings = append(findings, toolBuilderFinding("error", "invalid_request_example", "request_example", "Request example must be JSON no larger than 64 KiB."))
	} else if containsToolBuilderSecretValue(draft.RequestExample) {
		draft.RequestExample = nil
		findings = append(findings, toolBuilderFinding("error", "credential_material_removed", "request_example", "Credential-like material was removed from the request example."))
	}
	if size, err := toolBuilderJSONSize(draft.ResponseExample); err != nil || size > 64<<10 {
		draft.ResponseExample = nil
		findings = append(findings, toolBuilderFinding("error", "invalid_response_example", "response_example", "Response example must be JSON no larger than 64 KiB."))
	} else if containsToolBuilderSecretValue(draft.ResponseExample) {
		draft.ResponseExample = nil
		findings = append(findings, toolBuilderFinding("error", "credential_material_removed", "response_example", "Credential-like material was removed from the response example."))
	}
	return draft, findings
}

// ValidateToolDraft performs deterministic, local validation only. It never
// resolves DNS or calls either the candidate endpoint or an AI provider.
func (s *Service) validateToolDraft(ctx context.Context, productID string, draft ToolDraft, credentialPresent bool) (ToolDraftValidation, error) {
	product, err := s.store.Product(ctx, strings.TrimSpace(productID))
	if err != nil {
		return ToolDraftValidation{}, err
	}
	// credential_present is a server-derived output bit. Never trust the copy
	// nested in an inbound or AI-authored draft.
	draft.CredentialPresent = credentialPresent
	draft, findings := normalizeToolBuilderDraft(draft)
	add := func(level, code, field, message string) {
		findings = append(findings, toolBuilderFinding(level, code, field, message))
	}
	if !toolNamePattern.MatchString(draft.Namespace) {
		add("error", "invalid_namespace", "namespace", "Namespace must be a lower-case identifier beginning with a letter.")
	}
	if !toolNamePattern.MatchString(draft.Name) {
		add("error", "invalid_name", "name", "Name must be a lower-case identifier beginning with a letter.")
	}
	if draft.Description == "" || utf8.RuneCountInString(draft.Description) > 500 {
		add("error", "invalid_description", "description", "Description is required and must contain at most 500 characters.")
	}
	allowedMethods := map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
	if !allowedMethods[draft.HTTPMethod] {
		add("error", "invalid_http_method", "http_method", "HTTP method must be GET, POST, PUT, PATCH, or DELETE.")
	}
	if draft.TimeoutMS < 100 || draft.TimeoutMS > 60_000 {
		add("error", "invalid_timeout", "timeout_ms", "Timeout must be between 100 and 60000 milliseconds.")
	}
	parsed, parseErr := url.Parse(draft.Endpoint)
	localEndpoint := parseErr == nil && identity.IsLocalDevelopmentHostname(parsed.Hostname())
	if parseErr != nil || !validToolEndpoint(draft.Endpoint) || (localEndpoint && parsed.Scheme != "http") || (!localEndpoint && parsed.Scheme != "https") {
		add("error", "unsafe_endpoint", "endpoint", "Endpoint must be a fixed credential-free HTTPS URL on the default port, or an HTTP localhost URL for development.")
	} else if net.ParseIP(parsed.Hostname()) != nil && !identity.IsLocalDevelopmentHostname(parsed.Hostname()) {
		add("error", "ip_literal_endpoint", "endpoint", "Public tool endpoints must use a DNS hostname rather than an IP literal.")
	}
	if draft.UpstreamAuth.Type == "delegated_oauth" && parseErr == nil && parsed.Host != "" {
		provider, providerErr := s.store.IdentityProvider(ctx, product.ID)
		if providerErr != nil || provider.State != "active" || provider.DelegatedAPIOrigin == "" {
			add("error", "delegated_oauth_unconfigured", "upstream_auth.type", "Configure an active delegated identity provider origin before using delegated OAuth.")
		} else if origin, originErr := url.Parse(provider.DelegatedAPIOrigin); originErr != nil || !strings.EqualFold(origin.Scheme, parsed.Scheme) || !strings.EqualFold(origin.Host, parsed.Host) {
			add("error", "delegated_origin_mismatch", "endpoint", "Delegated OAuth tools must use the configured authorization API origin.")
		}
	}
	if err := toolruntime.ValidateSchema(draft.InputSchema); err != nil {
		add("error", "invalid_input_schema", "input_schema", "Input schema is not a supported strict object JSON schema.")
	} else if toolruntime.SchemaContainsSensitiveFields(draft.InputSchema) {
		add("error", "sensitive_input_field", "input_schema", "Authentication and credential-shaped fields must be configured in the write-only connection, not exposed as tool arguments.")
	}
	if err := toolruntime.ValidateSchema(draft.OutputSchema); err != nil {
		add("error", "invalid_output_schema", "output_schema", "Output schema is not a supported strict object JSON schema.")
	} else if toolruntime.SchemaContainsSensitiveFields(draft.OutputSchema) {
		add("error", "sensitive_output_field", "output_schema", "Authentication and credential-shaped fields must not be exposed in agent-visible tool results.")
	}

	properties := toolBuilderSchemaProperties(draft.InputSchema)
	placeholders := map[string]bool{}
	if parseErr == nil {
		for _, match := range toolBuilderPlaceholderPattern.FindAllStringSubmatch(parsed.Path, -1) {
			placeholders[match[1]] = true
		}
	}
	for placeholder := range placeholders {
		if _, ok := properties[placeholder]; !ok {
			add("error", "path_parameter_missing_schema", "input_schema", "Every endpoint path placeholder must have a matching input-schema property.")
		}
		if draft.RequestMapping.ParameterLocations[placeholder] != "path" {
			add("error", "path_parameter_not_mapped", "request_mapping.parameter_locations", "Every endpoint path placeholder must be mapped to the path location.")
		}
	}
	for name, location := range draft.RequestMapping.ParameterLocations {
		if _, ok := properties[name]; !ok {
			add("error", "mapping_parameter_missing_schema", "request_mapping.parameter_locations", "Every mapped parameter must exist in the input schema.")
		}
		if location == "path" && !placeholders[name] {
			add("error", "path_mapping_missing_placeholder", "request_mapping.parameter_locations", "Every path-mapped parameter must have a matching endpoint placeholder.")
		}
		if draft.HTTPMethod == "GET" && location == "body" {
			add("error", "get_body_mapping", "request_mapping.parameter_locations", "GET tools cannot map input parameters to a request body.")
		}
	}
	if parseErr == nil && validToolEndpoint(draft.Endpoint) && toolruntime.ValidateSchema(draft.InputSchema) == nil {
		mappingRaw, _ := json.Marshal(draft.RequestMapping)
		if err := validateToolMappings(draft.InputSchema, draft.Endpoint, draft.HTTPMethod, mappingRaw); err != nil {
			add("error", "request_mapping_schema_mismatch", "request_mapping.parameter_locations", err.Error())
		}
	}
	if draft.ResponseMapping.ResultPath != "" && !validToolBuilderResultPath(draft.ResponseMapping.ResultPath) {
		add("error", "invalid_result_path", "response_mapping.result_path", "Result path must be a dotted sequence of response property names.")
	}
	if draft.RequestExample != nil {
		if err := toolruntime.ValidateArguments(draft.InputSchema, draft.RequestExample); err != nil {
			add("error", "invalid_request_example", "request_example", "Request example does not match the input schema.")
		}
	}
	if draft.ResponseExample != nil {
		object, ok := draft.ResponseExample.(map[string]any)
		if !ok {
			add("error", "invalid_response_example", "response_example", "Response example must be an object matching the output schema.")
		} else if err := toolruntime.ValidateArguments(draft.OutputSchema, object); err != nil {
			add("error", "invalid_response_example", "response_example", "Response example does not match the output schema.")
		}
	}
	if draft.HTTPMethod != "GET" && !draft.AuthorizationPolicy.IdempotencyRequired {
		add("error", "mutation_not_idempotent", "authorization_policy.idempotency_required", "Mutation tools require idempotency metadata before they can be saved or published.")
	}
	definitions, grantErr := s.store.GrantDefinitions(ctx, product.ID)
	if grantErr != nil && !errors.Is(grantErr, store.ErrNotFound) {
		return ToolDraftValidation{}, grantErr
	}
	if len(missingRegisteredGrants(definitions, draft.AuthorizationPolicy.RequiredGrants)) > 0 {
		add("error", "unregistered_grant", "authorization_policy.required_grants", "One or more required grants are not active in this product.")
	}
	sortToolBuilderFindings(findings)
	valid := true
	for _, finding := range findings {
		if finding.Level == "error" {
			valid = false
			break
		}
	}
	return ToolDraftValidation{Valid: valid, NetworkCallPerformed: false, Findings: findings, NormalizedDraft: draft, CheckedAt: s.now()}, nil
}

// ValidateToolDraft validates a detached draft and therefore treats it as
// having no credential. HTTP/assisted flows must use ValidateToolDraftContext,
// which derives presence from server-side state plus explicit save intent.
func (s *Service) ValidateToolDraft(ctx context.Context, productID string, draft ToolDraft) (ToolDraftValidation, error) {
	return s.validateToolDraft(ctx, productID, draft, false)
}

func toolBuilderSchemaProperties(raw json.RawMessage) map[string]any {
	var schema struct {
		Properties map[string]any `json:"properties"`
	}
	if json.Unmarshal(raw, &schema) != nil || schema.Properties == nil {
		return map[string]any{}
	}
	return schema.Properties
}

func validToolBuilderResultPath(value string) bool {
	if value == "" {
		return true
	}
	if strings.HasPrefix(value, "/") {
		for _, part := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
			part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
			if part == "" || len(part) > 100 || strings.ContainsAny(part, "\r\n\x00") {
				return false
			}
		}
		return true
	}
	for _, part := range strings.Split(value, ".") {
		if !toolQueryNamePattern.MatchString(part) {
			return false
		}
	}
	return true
}

func sortToolBuilderFindings(findings []ToolDraftFinding) {
	rank := map[string]int{"error": 0, "warning": 1, "info": 2}
	sort.SliceStable(findings, func(i, j int) bool {
		if rank[findings[i].Level] != rank[findings[j].Level] {
			return rank[findings[i].Level] < rank[findings[j].Level]
		}
		if findings[i].Field != findings[j].Field {
			return findings[i].Field < findings[j].Field
		}
		return findings[i].Code < findings[j].Code
	})
}

func toolBuilderDraftFingerprint(draft ToolDraft) string {
	encoded, _ := json.Marshal(draft)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func toolBuilderChanges(before, after ToolDraft, rationale string) []ToolDraftChange {
	left, _ := json.Marshal(before)
	right, _ := json.Marshal(after)
	var prior, current map[string]json.RawMessage
	_ = json.Unmarshal(left, &prior)
	_ = json.Unmarshal(right, &current)
	keys := make([]string, 0, len(current))
	for key := range current {
		if !bytes.Equal(prior[key], current[key]) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	changes := make([]ToolDraftChange, 0, len(keys))
	for _, key := range keys {
		changes = append(changes, ToolDraftChange{Field: key, Rationale: rationale, SecuritySensitive: key == "upstream_auth" || key == "endpoint" || key == "authorization_policy"})
	}
	return changes
}

func toolBuilderMethodClass(value string) string {
	if map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}[value] {
		return value
	}
	return "invalid"
}

func toolBuilderAuthClass(value string) string {
	if map[string]bool{"delegated_oauth": true, "none": true, "bearer": true, "authorization_scheme": true, "api_key_header": true, "api_key_query": true, "basic": true, "oauth_client_credentials": true, "custom_header": true}[value] {
		return value
	}
	return "invalid"
}

func (s *Service) appendToolBuilderAudit(ctx context.Context, product model.Product, actor Actor, action string, metadata map[string]any) error {
	return s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: product.OrganisationID, ProductID: product.ID, ActorID: actor.ID, Action: action, TargetType: "tool_builder", TargetID: product.ID, Current: metadata, RequestID: actor.RequestID, CreatedAt: s.now()})
}

// AuditToolDraftValidation records only counts and non-secret classifications;
// the draft, endpoint, schemas and examples are intentionally excluded.
func (s *Service) AuditToolDraftValidation(ctx context.Context, productID string, result ToolDraftValidation, actor Actor) error {
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return err
	}
	return s.appendToolBuilderAudit(ctx, product, actor, "tool_builder.validated", map[string]any{"valid": result.Valid, "finding_count": len(result.Findings), "method": toolBuilderMethodClass(result.NormalizedDraft.HTTPMethod), "authentication": toolBuilderAuthClass(result.NormalizedDraft.UpstreamAuth.Type)})
}

func (s *Service) toolDraftBase(ctx context.Context, productID, toolID string, revision int64) (model.Tool, bool, error) {
	toolID = strings.TrimSpace(toolID)
	if toolID == "" {
		if revision != 0 {
			return model.Tool{}, false, fmt.Errorf("%w: base revision requires a base tool", ErrToolBuilderInvalidInput)
		}
		return model.Tool{}, false, nil
	}
	if revision <= 0 {
		return model.Tool{}, false, fmt.Errorf("%w: base revision is required", ErrToolBuilderInvalidInput)
	}
	tool, err := s.store.Tool(ctx, productID, toolID)
	if err != nil {
		return model.Tool{}, false, err
	}
	if tool.Revision != revision {
		return model.Tool{}, false, store.ErrConflict
	}
	return tool, true, nil
}

// ValidateToolDraftBase prevents an assisted operation from being applied to a
// stale persisted tool. New-tool drafts omit both values.
func (s *Service) ValidateToolDraftBase(ctx context.Context, productID, toolID string, revision int64) error {
	_, _, err := s.toolDraftBase(ctx, productID, toolID, revision)
	return err
}

func storedToolAuthType(tool model.Tool) string {
	if len(tool.UpstreamAuth) == 0 {
		return "delegated_oauth"
	}
	var auth ToolUpstreamAuth
	if json.Unmarshal(tool.UpstreamAuth, &auth) != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(auth.Type))
}

func (s *Service) prepareToolDraftContext(ctx context.Context, productID string, input ToolDraftContext) (ToolDraft, error) {
	base, hasBase, err := s.toolDraftBase(ctx, productID, input.BaseToolID, input.BaseRevision)
	if err != nil {
		return ToolDraft{}, err
	}
	draft := input.Draft
	draft.CredentialPresent = false
	authType := strings.ToLower(strings.TrimSpace(draft.UpstreamAuth.Type))
	if !credentialRequired(authType) {
		return draft, nil
	}
	if input.CredentialWillBeSupplied {
		draft.CredentialPresent = true
		return draft, nil
	}
	if hasBase && base.CredentialID != "" && toolCredentialCanBeReused(base.BaseURL, draft.Endpoint, base.UpstreamAuth, draft.UpstreamAuth) {
		draft.CredentialPresent = true
	}
	return draft, nil
}

// ValidateToolDraftContext derives credential presence exclusively from a
// matching persisted base-tool credential or the caller's explicit intent to
// supply a credential during the final save. draft.credential_present is ignored.
func (s *Service) ValidateToolDraftContext(ctx context.Context, productID string, input ToolDraftContext) (ToolDraftValidation, error) {
	draft, err := s.prepareToolDraftContext(ctx, productID, input)
	if err != nil {
		return ToolDraftValidation{}, err
	}
	return s.validateToolDraft(ctx, productID, draft, draft.CredentialPresent)
}

var toolBuilderProposalOutputSchema = json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "summary":{"type":"string"},
    "reply":{"type":"string"},
    "draft_json":{"type":"string"}
  },
  "required":["summary","reply","draft_json"]
}`)

var toolBuilderAnalysisOutputSchema = json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "summary":{"type":"string"},
    "findings":{
      "type":"array",
      "items":{
        "type":"object",
        "additionalProperties":false,
        "properties":{
          "level":{"type":"string","enum":["warning","info"]},
          "code":{"type":"string"},
          "field":{"type":"string"},
          "message":{"type":"string"},
          "suggestion":{"type":"string"}
        },
        "required":["level","code","field","message","suggestion"]
      }
    }
  },
  "required":["summary","findings"]
}`)
