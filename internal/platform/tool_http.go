package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	secretvault "github.com/dokosoko/dokosoko-service/internal/secrets"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

const maxToolCredentialBytes = 16 << 10

var toolQueryNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)
var toolPathParameterPattern = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_.-]{0,63})\}`)

// ToolUpstreamAuth is public, non-secret connection configuration. Credential
// material is submitted separately, encrypted immediately, and never included
// in this object or returned by an API.
type ToolUpstreamAuth struct {
	Type                    string   `json:"type"`
	Scheme                  string   `json:"scheme,omitempty"`
	HeaderName              string   `json:"header_name,omitempty"`
	QueryName               string   `json:"query_name,omitempty"`
	Prefix                  string   `json:"prefix,omitempty"`
	Username                string   `json:"username,omitempty"`
	ClientID                string   `json:"client_id,omitempty"`
	TokenURL                string   `json:"token_url,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	Scopes                  []string `json:"scopes,omitempty"`
	Audience                string   `json:"audience,omitempty"`
	Resource                string   `json:"resource,omitempty"`
}

type ToolRequestMapping struct {
	ParameterLocations map[string]string `json:"parameter_locations,omitempty"`
}

type ToolResponseMapping struct {
	ResultPath string `json:"result_path,omitempty"`
}

func normalizeToolExample(raw, schema json.RawMessage, label string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	if len(trimmed) > 64<<10 {
		return nil, fmt.Errorf("%s example is too large", label)
	}
	var example map[string]any
	if err := strictJSON(trimmed, &example); err != nil || example == nil {
		return nil, fmt.Errorf("%s example must be one JSON object", label)
	}
	if containsToolBuilderSecretValue(example) {
		return nil, fmt.Errorf("%s example must not contain credential material", label)
	}
	if err := toolruntime.ValidateArguments(schema, example); err != nil {
		return nil, fmt.Errorf("%s example does not match its schema: %w", label, err)
	}
	encoded, err := json.Marshal(example)
	if err != nil {
		return nil, fmt.Errorf("%s example is invalid: %w", label, err)
	}
	return encoded, nil
}

func strictJSON(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func validHTTPHeaderName(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(char)) {
			continue
		}
		return false
	}
	return true
}

// validAuthorizationScheme accepts the RFC 9110 token grammar used by the
// Authorization field's auth-scheme component. Keeping this separate from
// custom headers lets vendors use schemes such as Token, ApiKey, or SSWS
// without allowing an arbitrary caller-controlled Authorization header.
func validAuthorizationScheme(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(char)) {
			continue
		}
		return false
	}
	return true
}

func safeCustomHeader(value string) bool {
	if !validHTTPHeaderName(value) {
		return false
	}
	switch strings.ToLower(value) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "host", "content-length", "transfer-encoding", "connection", "upgrade", "te", "trailer", "forwarded", "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto", "x-forwarded-uri", "x-http-method", "x-http-method-override", "x-method-override", "x-original-url", "x-original-uri", "x-rewrite-url", "x-envoy-original-path":
		return false
	default:
		return true
	}
}

func credentialRequired(authType string) bool {
	switch authType {
	case "bearer", "authorization_scheme", "api_key_header", "api_key_query", "basic", "oauth_client_credentials", "custom_header":
		return true
	default:
		return false
	}
}

func toolCredentialCanBeReused(currentEndpoint, nextEndpoint string, currentRaw json.RawMessage, next ToolUpstreamAuth) bool {
	currentURL, currentErr := url.Parse(currentEndpoint)
	nextURL, nextErr := url.Parse(nextEndpoint)
	if currentErr != nil || nextErr != nil || !strings.EqualFold(currentURL.Scheme, nextURL.Scheme) || !strings.EqualFold(currentURL.Host, nextURL.Host) {
		return false
	}
	var current ToolUpstreamAuth
	if strictJSON(currentRaw, &current) != nil {
		return false
	}
	currentEncoded, currentErr := json.Marshal(current)
	nextEncoded, nextErr := json.Marshal(next)
	return currentErr == nil && nextErr == nil && bytes.Equal(currentEncoded, nextEncoded)
}

func normalizeToolUpstreamAuth(raw json.RawMessage, currentRaw json.RawMessage, existingCredentialID, credential string) (json.RawMessage, ToolUpstreamAuth, bool, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{"type":"delegated_oauth"}`)
	}
	if len(raw) > 8<<10 {
		return nil, ToolUpstreamAuth{}, false, errors.New("upstream authentication configuration is too large")
	}
	var auth ToolUpstreamAuth
	if err := strictJSON(raw, &auth); err != nil {
		return nil, ToolUpstreamAuth{}, false, fmt.Errorf("invalid upstream authentication: %w", err)
	}
	auth.Type = strings.ToLower(strings.TrimSpace(auth.Type))
	auth.Scheme = strings.TrimSpace(auth.Scheme)
	auth.HeaderName, auth.QueryName = strings.TrimSpace(auth.HeaderName), strings.TrimSpace(auth.QueryName)
	if len(auth.Prefix) > 64 || strings.ContainsAny(auth.Prefix, "\r\n\x00") {
		return nil, ToolUpstreamAuth{}, false, errors.New("authentication header prefix is invalid")
	}
	auth.Prefix = strings.TrimSpace(auth.Prefix)
	auth.Username, auth.ClientID = strings.TrimSpace(auth.Username), strings.TrimSpace(auth.ClientID)
	auth.TokenURL, auth.TokenEndpointAuthMethod = strings.TrimSpace(auth.TokenURL), strings.ToLower(strings.TrimSpace(auth.TokenEndpointAuthMethod))
	auth.Audience, auth.Resource = strings.TrimSpace(auth.Audience), strings.TrimSpace(auth.Resource)
	allowed := map[string]bool{"delegated_oauth": true, "none": true, "bearer": true, "authorization_scheme": true, "api_key_header": true, "api_key_query": true, "basic": true, "oauth_client_credentials": true, "custom_header": true}
	if !allowed[auth.Type] {
		return nil, ToolUpstreamAuth{}, false, errors.New("upstream authentication type is not supported")
	}
	if (auth.Type == "api_key_header" || auth.Type == "custom_header") && !safeCustomHeader(auth.HeaderName) {
		return nil, ToolUpstreamAuth{}, false, errors.New("choose a safe custom authentication header name")
	}
	if auth.Type == "authorization_scheme" && !validAuthorizationScheme(auth.Scheme) {
		return nil, ToolUpstreamAuth{}, false, errors.New("authorization scheme is invalid")
	}
	if auth.Type == "api_key_query" && !toolQueryNamePattern.MatchString(auth.QueryName) {
		return nil, ToolUpstreamAuth{}, false, errors.New("API key query parameter name is invalid")
	}
	if auth.Type == "basic" && (auth.Username == "" || len(auth.Username) > 255 || strings.ContainsAny(auth.Username, ":\r\n\x00")) {
		return nil, ToolUpstreamAuth{}, false, errors.New("basic authentication username is invalid")
	}
	if auth.Type == "oauth_client_credentials" {
		if auth.TokenEndpointAuthMethod == "" {
			auth.TokenEndpointAuthMethod = "client_secret_basic"
		}
		if auth.TokenEndpointAuthMethod != "client_secret_basic" && auth.TokenEndpointAuthMethod != "client_secret_post" {
			return nil, ToolUpstreamAuth{}, false, errors.New("OAuth token endpoint authentication method is not supported")
		}
		if auth.ClientID == "" || len(auth.ClientID) > 255 || strings.ContainsAny(auth.ClientID, "\r\n\x00") {
			return nil, ToolUpstreamAuth{}, false, errors.New("OAuth client ID is invalid")
		}
		parsed, err := url.Parse(auth.TokenURL)
		if err != nil || !validToolEndpoint(auth.TokenURL) || parsed.RawQuery != "" || parsed.User != nil || parsed.Fragment != "" {
			return nil, ToolUpstreamAuth{}, false, errors.New("OAuth token URL must be a fixed credential-free public HTTPS URL or HTTP localhost URL")
		}
		if len(auth.Audience) > 500 || strings.ContainsAny(auth.Audience, "\r\n\x00") {
			return nil, ToolUpstreamAuth{}, false, errors.New("OAuth audience is invalid")
		}
		if len(auth.Resource) > 500 || strings.ContainsAny(auth.Resource, "\r\n\x00") {
			return nil, ToolUpstreamAuth{}, false, errors.New("OAuth resource is invalid")
		}
	}
	seenScopes := map[string]bool{}
	scopes := make([]string, 0, len(auth.Scopes))
	for _, scope := range auth.Scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || seenScopes[scope] {
			continue
		}
		if len(scope) > 200 || strings.ContainsAny(scope, "\r\n\x00") {
			return nil, ToolUpstreamAuth{}, false, errors.New("OAuth scope is invalid")
		}
		seenScopes[scope] = true
		scopes = append(scopes, scope)
	}
	if len(scopes) > 32 {
		return nil, ToolUpstreamAuth{}, false, errors.New("at most 32 OAuth scopes are allowed")
	}
	sort.Strings(scopes)
	auth.Scopes = scopes
	switch auth.Type {
	case "delegated_oauth", "none", "bearer":
		auth.HeaderName, auth.QueryName, auth.Prefix, auth.Username, auth.ClientID, auth.TokenURL, auth.TokenEndpointAuthMethod, auth.Audience, auth.Resource = "", "", "", "", "", "", "", "", ""
		auth.Scopes = nil
	case "authorization_scheme":
		auth.HeaderName, auth.QueryName, auth.Prefix, auth.Username, auth.ClientID, auth.TokenURL, auth.TokenEndpointAuthMethod, auth.Audience, auth.Resource = "", "", "", "", "", "", "", "", ""
		auth.Scopes = nil
	case "api_key_header":
		auth.QueryName, auth.Username, auth.ClientID, auth.TokenURL, auth.TokenEndpointAuthMethod, auth.Audience, auth.Resource = "", "", "", "", "", "", ""
		auth.Scopes = nil
	case "api_key_query":
		auth.HeaderName, auth.Prefix, auth.Username, auth.ClientID, auth.TokenURL, auth.TokenEndpointAuthMethod, auth.Audience, auth.Resource = "", "", "", "", "", "", "", ""
		auth.Scopes = nil
	case "basic":
		auth.HeaderName, auth.QueryName, auth.Prefix, auth.ClientID, auth.TokenURL, auth.TokenEndpointAuthMethod, auth.Audience, auth.Resource = "", "", "", "", "", "", "", ""
		auth.Scopes = nil
	case "oauth_client_credentials":
		auth.HeaderName, auth.QueryName, auth.Prefix, auth.Username = "", "", "", ""
	case "custom_header":
		auth.QueryName, auth.Username, auth.ClientID, auth.TokenURL, auth.TokenEndpointAuthMethod, auth.Audience, auth.Resource = "", "", "", "", "", "", ""
		auth.Scopes = nil
	}
	if auth.Type != "authorization_scheme" {
		auth.Scheme = ""
	}

	var current ToolUpstreamAuth
	_ = json.Unmarshal(currentRaw, &current)
	credentialChanged := credential != ""
	if credentialChanged && (len(credential) > maxToolCredentialBytes || strings.TrimSpace(credential) == "" || strings.ContainsAny(credential, "\r\n\x00")) {
		return nil, ToolUpstreamAuth{}, false, errors.New("upstream credential is invalid")
	}
	if !credentialRequired(auth.Type) && credentialChanged {
		return nil, ToolUpstreamAuth{}, false, errors.New("this authentication type does not accept a stored credential")
	}
	if credentialRequired(auth.Type) && !credentialChanged && (existingCredentialID == "" || current.Type != auth.Type) {
		return nil, ToolUpstreamAuth{}, false, errors.New("an encrypted upstream credential is required for this authentication type")
	}
	encoded, err := json.Marshal(auth)
	return encoded, auth, credentialChanged, err
}

func normalizeToolRequestMapping(raw json.RawMessage) (json.RawMessage, ToolRequestMapping, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if len(raw) > 16<<10 {
		return nil, ToolRequestMapping{}, errors.New("request mapping is too large")
	}
	var mapping ToolRequestMapping
	if err := strictJSON(raw, &mapping); err != nil {
		return nil, ToolRequestMapping{}, fmt.Errorf("invalid request mapping: %w", err)
	}
	if len(mapping.ParameterLocations) > 64 {
		return nil, ToolRequestMapping{}, errors.New("request mapping may contain at most 64 parameters")
	}
	for name, location := range mapping.ParameterLocations {
		if !toolQueryNamePattern.MatchString(name) {
			return nil, ToolRequestMapping{}, fmt.Errorf("request parameter %q is invalid", name)
		}
		if location != "path" && location != "query" && location != "header" && location != "body" {
			return nil, ToolRequestMapping{}, fmt.Errorf("request parameter %q has an unsupported location", name)
		}
		if location == "header" && !safeCustomHeader(strings.ReplaceAll(name, "_", "-")) {
			return nil, ToolRequestMapping{}, fmt.Errorf("request parameter %q maps to an unsafe header", name)
		}
	}
	encoded, err := json.Marshal(mapping)
	return encoded, mapping, err
}

func normalizeToolResponseMapping(raw json.RawMessage) (json.RawMessage, ToolResponseMapping, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if len(raw) > 4<<10 {
		return nil, ToolResponseMapping{}, errors.New("response mapping is too large")
	}
	var mapping ToolResponseMapping
	if err := strictJSON(raw, &mapping); err != nil {
		return nil, ToolResponseMapping{}, fmt.Errorf("invalid response mapping: %w", err)
	}
	mapping.ResultPath = strings.TrimSpace(mapping.ResultPath)
	if len(mapping.ResultPath) > 500 || strings.ContainsAny(mapping.ResultPath, "\r\n\x00") {
		return nil, ToolResponseMapping{}, errors.New("response result path is invalid")
	}
	encoded, err := json.Marshal(mapping)
	return encoded, mapping, err
}

func validateToolMappings(inputSchema json.RawMessage, endpoint, method string, raw json.RawMessage) error {
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(inputSchema, &schema); err != nil {
		return errors.New("input schema is invalid")
	}
	var mapping ToolRequestMapping
	if err := json.Unmarshal(raw, &mapping); err != nil {
		return errors.New("request mapping is invalid")
	}
	for name, location := range mapping.ParameterLocations {
		property, ok := schema.Properties[name]
		if !ok {
			return fmt.Errorf("request mapping references unknown input %q", name)
		}
		if method == "GET" && location == "body" {
			return fmt.Errorf("GET input %q cannot be mapped to a request body", name)
		}
		if location != "body" && !toolRequestScalarSchema(property) {
			return fmt.Errorf("request input %q must use a scalar schema when mapped to %s", name, location)
		}
	}
	if method == "GET" {
		for name, property := range schema.Properties {
			if mapping.ParameterLocations[name] == "" && !toolRequestScalarSchema(property) {
				return fmt.Errorf("GET input %q must use a scalar schema when implicitly mapped to query", name)
			}
		}
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return errors.New("tool endpoint is invalid")
	}
	pathParameters := map[string]bool{}
	requiredParameters := make(map[string]bool, len(schema.Required))
	for _, name := range schema.Required {
		requiredParameters[name] = true
	}
	remainingPath := parsed.Path
	for _, match := range toolPathParameterPattern.FindAllStringSubmatch(parsed.Path, -1) {
		pathParameters[match[1]] = true
		if mapping.ParameterLocations[match[1]] != "path" {
			return fmt.Errorf("endpoint path parameter %q must be explicitly mapped", match[1])
		}
		if !requiredParameters[match[1]] {
			return fmt.Errorf("endpoint path parameter %q must be required by the input schema", match[1])
		}
		remainingPath = strings.ReplaceAll(remainingPath, match[0], "")
	}
	if strings.ContainsAny(remainingPath, "{}") {
		return errors.New("endpoint contains an invalid path parameter placeholder")
	}
	for name, location := range mapping.ParameterLocations {
		if location == "path" && !pathParameters[name] {
			return fmt.Errorf("path input %q has no matching endpoint placeholder", name)
		}
	}
	return nil
}

func toolRequestScalarSchema(raw json.RawMessage) bool {
	var schema struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &schema) != nil {
		return false
	}
	return schema.Type == "string" || schema.Type == "boolean" || schema.Type == "integer" || schema.Type == "number"
}

func (s *Service) saveToolCredential(ctx context.Context, organisationID, connectionID, credential string) (string, string, error) {
	if s.vault == nil {
		return "", "", errors.New("tool credential encryption is not configured")
	}
	secretID, err := randomUUID()
	if err != nil {
		return "", "", err
	}
	encrypted, err := s.vault.Encrypt([]byte(credential), organisationID+":tool-connection:"+connectionID)
	if err != nil {
		return "", "", err
	}
	_, err = s.store.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: organisationID, Name: "tool-connection-" + connectionID + "-" + secretID, Purpose: "tool_upstream", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint})
	if err != nil {
		return "", "", err
	}
	return secretID, encrypted.Fingerprint, nil
}

// ResolveToolCredential implements tools.CredentialResolver without exposing
// secret material through the public Tool representation.
func (s *Service) ResolveToolCredential(ctx context.Context, tool model.Tool) ([]byte, error) {
	if tool.CredentialID == "" || s.vault == nil {
		return nil, errors.New("tool credential is unavailable")
	}
	if tool.RuntimeServiceConnectionID != "" {
		if tool.RuntimeCredentialSetID == "" || tool.RuntimeCredentialVersionID == "" {
			return nil, errors.New("runtime tool credential is unavailable")
		}
		versions, err := s.store.RuntimeCredentialVersions(ctx, tool.RuntimeCredentialSetID)
		if err != nil {
			return nil, errors.New("runtime tool credential is unavailable")
		}
		active := false
		for _, version := range versions {
			if version.ID == tool.RuntimeCredentialVersionID && version.SecretID == tool.CredentialID && version.State == "active" && (version.ExpiresAt == nil || version.ExpiresAt.After(s.now())) {
				active = true
				break
			}
		}
		if !active {
			return nil, errors.New("runtime tool credential is unavailable")
		}
		secret, err := s.store.Secret(ctx, tool.OrganisationID, tool.CredentialID)
		if err != nil || secret.Purpose != "runtime_service_credential" {
			return nil, errors.New("runtime tool credential is unavailable")
		}
		return decryptRuntimeCredential(s.vault, secret)
	}
	secret, err := s.store.Secret(ctx, tool.OrganisationID, tool.CredentialID)
	if err != nil || secret.Purpose != "tool_upstream" {
		return nil, errors.New("tool credential is unavailable")
	}
	return s.vault.Decrypt(secretvault.Encrypted{Ciphertext: secret.Ciphertext, Nonce: secret.Nonce, Fingerprint: secret.Fingerprint, KeyVersion: secret.KeyVersion}, tool.OrganisationID+":tool-connection:"+tool.APIConnectionID)
}
