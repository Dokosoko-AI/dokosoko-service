package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func oauthError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

func (s *Server) oauthAuthorizationServerMetadata(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                s.baseURL,
		"authorization_endpoint":                s.baseURL + "/oauth/authorize",
		"token_endpoint":                        s.baseURL + "/oauth/token",
		"registration_endpoint":                 s.baseURL + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"mcp:private"},
		"resource_parameter_supported":          true,
		"client_id_metadata_document_supported": true,
	})
}

type oauthRegistrationRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
}

func onlyRegistrationValue(values []string, expected string) bool {
	return len(values) == 0 || (len(values) == 1 && values[0] == expected)
}

func (s *Server) oauthRegister(w http.ResponseWriter, r *http.Request) {
	if s.identityBroker == nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "Private MCP identity is not available.")
		return
	}
	if !s.allowFixedWindow("oauth-register|"+remoteHost(r.RemoteAddr), 30, time.Now().UTC()) {
		w.Header().Set("Retry-After", "60")
		oauthError(w, http.StatusTooManyRequests, "temporarily_unavailable", "Client registration request limit exceeded.")
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		oauthError(w, http.StatusNotFound, "invalid_client_metadata", "Private MCP is not configured.")
		return
	}
	provider, err := s.service.Store().IdentityProvider(r.Context(), deployment.ID)
	if err != nil || provider.State != "active" {
		oauthError(w, http.StatusNotFound, "invalid_client_metadata", "Private MCP identity is not available.")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "Client metadata is too large.")
		return
	}
	var input oauthRegistrationRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "Client metadata must be one JSON object.")
		return
	}
	input.ClientName = strings.TrimSpace(input.ClientName)
	if len(input.RedirectURIs) == 0 || len(input.RedirectURIs) > 20 || utf8.RuneCountInString(input.ClientName) > 200 || (input.TokenEndpointAuthMethod != "" && input.TokenEndpointAuthMethod != "none") || !onlyRegistrationValue(input.GrantTypes, "authorization_code") || !onlyRegistrationValue(input.ResponseTypes, "code") || (input.Scope != "" && input.Scope != "mcp:private") {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "Only public authorization-code clients using PKCE and the mcp:private scope are supported.")
		return
	}
	redirects := append([]string(nil), input.RedirectURIs...)
	sort.Strings(redirects)
	for index, redirect := range redirects {
		if !identity.ValidRedirectURI(redirect) || (index > 0 && redirects[index-1] == redirect) {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "Redirect URIs must be unique HTTPS or loopback HTTP URLs without fragments.")
			return
		}
	}
	fingerprint := sha256.Sum256([]byte(deployment.ID + "\x00" + input.ClientName + "\x00" + strings.Join(redirects, "\x00")))
	clientID := "mcp_client_" + base64.RawURLEncoding.EncodeToString(fingerprint[:24])
	client, err := s.service.Store().CreateOAuthClient(r.Context(), identity.OAuthClient{ClientID: clientID, DeploymentID: deployment.ID, ClientName: input.ClientName, RedirectURIs: redirects})
	if err != nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "Client registration could not be completed. Retrying the same request is safe.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  client.ClientID,
		"client_id_issued_at":        client.CreatedAt.Unix(),
		"client_name":                client.ClientName,
		"redirect_uris":              client.RedirectURIs,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"scope":                      "mcp:private",
	})
}

func (s *Server) oauthProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if s.identityBroker == nil {
		http.NotFound(w, r)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	provider, err := s.service.Store().IdentityProvider(r.Context(), deployment.ID)
	if err != nil || provider.State != "active" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 s.baseURL + "/mcp",
		"authorization_servers":    []string{s.baseURL},
		"scopes_supported":         []string{"mcp:private"},
		"bearer_methods_supported": []string{"header"},
	})
}

func (s *Server) productFromMCPResource(ctx context.Context, raw string) (string, bool) {
	resource, err := url.Parse(raw)
	base, baseErr := url.Parse(s.baseURL)
	if err != nil || baseErr != nil || resource.RawQuery != "" || resource.Fragment != "" || resource.User != nil || !strings.EqualFold(resource.Scheme, base.Scheme) || !strings.EqualFold(resource.Host, base.Host) {
		return "", false
	}
	expectedPath := strings.TrimRight(base.EscapedPath(), "/") + "/mcp"
	if resource.EscapedPath() != expectedPath {
		return "", false
	}
	deployment, err := s.service.Store().Deployment(ctx)
	if err != nil {
		return "", false
	}
	return deployment.ID, true
}

func (s *Server) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	if s.identityBroker == nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "Identity broker is not configured.")
		return
	}
	if r.URL.Query().Get("response_type") != "code" || r.URL.Query().Get("code_challenge_method") != "S256" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "Authorization code flow with PKCE S256 is required.")
		return
	}
	if !s.allowFixedWindow("oauth-authorize|"+remoteHost(r.RemoteAddr), 60, time.Now().UTC()) {
		oauthError(w, http.StatusTooManyRequests, "temporarily_unavailable", "Authorization request limit exceeded.")
		return
	}
	resource := r.URL.Query().Get("resource")
	productID, ok := s.productFromMCPResource(r.Context(), resource)
	if !ok {
		oauthError(w, http.StatusBadRequest, "invalid_target", "The resource must identify a DokoSoko MCP endpoint.")
		return
	}
	redirect, err := s.identityBroker.Begin(r.Context(), identity.AuthorizationRequest{
		ProductID: productID, ClientID: r.URL.Query().Get("client_id"), RedirectURI: r.URL.Query().Get("redirect_uri"), Resource: resource, Scope: r.URL.Query().Get("scope"), State: r.URL.Query().Get("state"), CodeChallenge: r.URL.Query().Get("code_challenge"),
	})
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "The OAuth client, redirect URI, or product identity configuration is invalid.")
		return
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	if s.identityBroker == nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "Identity broker is not configured.")
		return
	}
	rawState := r.URL.Query().Get("state")
	if identity.IsProviderTestState(rawState) {
		test, err := s.identityBroker.CompleteProviderTest(r.Context(), rawState, r.URL.Query().Get("code"), r.URL.Query().Get("error"))
		if err != nil {
			http.Redirect(w, r, s.baseURL+"/identity?identity_test_error=invalid_or_expired", http.StatusSeeOther)
			return
		}
		query := url.Values{"identity_test_id": {test.ID}}
		http.Redirect(w, r, s.baseURL+"/identity?"+query.Encode(), http.StatusSeeOther)
		return
	}
	if r.URL.Query().Get("error") != "" {
		oauthError(w, http.StatusUnauthorized, "access_denied", "The vendor authorization server denied access.")
		return
	}
	result, err := s.identityBroker.Callback(r.Context(), rawState, r.URL.Query().Get("code"))
	if err != nil {
		oauthError(w, http.StatusUnauthorized, "access_denied", "Vendor identity or access verification failed.")
		return
	}
	http.Redirect(w, r, result.RedirectURI, http.StatusFound)
}

func (s *Server) oauthToken(w http.ResponseWriter, r *http.Request) {
	if s.identityBroker == nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "Identity broker is not configured.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") != "authorization_code" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "Authorization code grant form data is required.")
		return
	}
	if !s.allowFixedWindow("oauth-token|"+remoteHost(r.RemoteAddr), 120, time.Now().UTC()) {
		oauthError(w, http.StatusTooManyRequests, "temporarily_unavailable", "Token request limit exceeded.")
		return
	}
	result, err := s.identityBroker.Exchange(r.Context(), r.PostForm.Get("code"), r.PostForm.Get("code_verifier"), r.PostForm.Get("client_id"), r.PostForm.Get("redirect_uri"), r.PostForm.Get("resource"))
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "The authorization code or PKCE verifier is invalid.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	s.recordAnalytics(r.Context(), result.Principal.ProductID, "connector_authorized", "vendor_user", pseudonym(result.Principal.ProductID, result.Principal), map[string]any{"client_id": result.Principal.ClientID})
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) upstreamOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if s.mcpBridge == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp_bridge_unavailable", "Stateless MCPv2 authorization is unavailable.", nil)
		return
	}
	if r.URL.Query().Get("error") != "" {
		writeError(w, http.StatusUnauthorized, "upstream_authorization_denied", "The upstream MCP authorization server denied access.", nil)
		return
	}
	if _, err := s.mcpBridge.CompleteAuthorization(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"), r.URL.Query().Get("iss")); err != nil {
		writeError(w, http.StatusUnauthorized, "upstream_authorization_failed", "The upstream MCP authorization response was invalid or expired.", nil)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `<!doctype html><html><head><meta name="viewport" content="width=device-width"><title>Connected</title><style>body{font:16px system-ui;margin:4rem;max-width:42rem}h1{color:#18181b}</style></head><body><h1>Stateless MCPv2 connection authorized</h1><p>Your upstream user grant is encrypted and bound to this DokoSoko identity. You can close this window.</p></body></html>`)
}

func (s *Server) identityProvider(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().IdentityProvider(r.Context(), deployment.ID)
		if errors.Is(err, store.ErrNotFound) {
			s.writeIdentityProvider(w, r, identity.ProviderConfig{OrganisationID: deployment.OrganisationID, DeploymentID: deployment.ID}, false)
			return
		}
		if err != nil {
			s.storeError(w, err)
			return
		}
		s.writeIdentityProvider(w, r, value, true)
	case http.MethodPut:
		var input struct {
			Provider           string   `json:"provider"`
			Issuer             string   `json:"issuer"`
			ClientID           string   `json:"client_id"`
			ClientSecret       string   `json:"client_secret"`
			Scopes             []string `json:"scopes"`
			Audience           string   `json:"audience"`
			OAuthResource      string   `json:"oauth_resource"`
			OrganisationClaim  string   `json:"customer_account_claim"`
			InstallationClaim  string   `json:"installation_claim"`
			DelegatedAPIOrigin string   `json:"authorization_api_origin"`
			Revision           *int64   `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Provider != "oidc" || input.Issuer == "" || input.ClientID == "" || input.OrganisationClaim == "" || input.DelegatedAPIOrigin == "" || input.Revision == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "provider oidc, issuer, client_id, customer_account_claim, authorization_api_origin, and revision are required.", nil)
			return
		}
		value, err := s.service.ConfigureIdentity(r.Context(), platform.IdentityInput{DeploymentID: deployment.ID, Issuer: input.Issuer, ClientID: input.ClientID, ClientSecret: input.ClientSecret, Scopes: input.Scopes, Audience: input.Audience, OAuthResource: input.OAuthResource, OrganisationClaim: input.OrganisationClaim, InstallationClaim: input.InstallationClaim, DelegatedAPIOrigin: input.DelegatedAPIOrigin, Revision: *input.Revision}, actor(r))
		if err != nil {
			s.identityConfigurationError(w, err)
			return
		}
		s.writeIdentityProvider(w, r, value, true)
	case http.MethodDelete:
		var input struct {
			Revision *int64 `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil || input.Revision == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		if _, err := s.service.DisconnectIdentityProvider(r.Context(), deployment.ID, *input.Revision, actor(r)); err != nil {
			s.identityDisconnectError(w, err)
			return
		}
		s.writeIdentityProvider(w, r, identity.ProviderConfig{OrganisationID: deployment.OrganisationID, DeploymentID: deployment.ID}, false)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}
