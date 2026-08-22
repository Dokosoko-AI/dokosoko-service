package identity

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"golang.org/x/oauth2"
)

var (
	ErrInvalidOAuth     = errors.New("invalid or expired OAuth transaction")
	ErrIdentityDisabled = errors.New("vendor identity is not configured")
	grantPattern        = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{0,127}$`)
	pkcePattern         = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)
)

const (
	accessEvaluationPath = "/v1/access/evaluations"
	privateMCPScope      = "mcp:private"
)

type Repository interface {
	IdentityProvider(context.Context, string) (ProviderConfig, error)
	CustomerAccount(context.Context, string, string) (CustomerAccount, error)
	ResolveCustomerAccount(context.Context, CustomerAccount) (CustomerAccount, error)
	ProductInstallationByExternalID(context.Context, string, string) (model.ProductInstallation, error)
	CreateOAuthState(context.Context, OAuthState) error
	ConsumeOAuthState(context.Context, []byte) (OAuthState, error)
	CreateOAuthCode(context.Context, OAuthCode) error
	ConsumeOAuthCode(context.Context, []byte) (OAuthCode, error)
	CreateAccessToken(context.Context, AccessToken) error
	AccessTokenByDigest(context.Context, []byte) (AccessToken, error)
	CreateSecret(context.Context, model.Secret) (model.Secret, error)
	Secret(context.Context, string, string) (model.Secret, error)
}

type Upstream interface {
	AuthorizationURL(context.Context, ProviderConfig, string, string, string, string) (string, error)
	ExchangeAndVerify(context.Context, ProviderConfig, string, string, string, string) (UpstreamIdentity, error)
}

type AccessEvaluator interface {
	Resolve(context.Context, ProviderConfig, UpstreamIdentity) (AccessEvaluation, error)
}

type ClientMetadataResolver interface {
	Resolve(context.Context, string) (ClientMetadata, error)
}

type ClientRegistry interface {
	OAuthClient(context.Context, string, string) (OAuthClient, error)
}

type Broker struct {
	repository      Repository
	vault           *secrets.Vault
	publicURL       string
	upstream        Upstream
	accessEvaluator AccessEvaluator
	clients         ClientMetadataResolver
	now             func() time.Time
}

type AuthorizationRequest struct {
	ProductID     string
	ClientID      string
	RedirectURI   string
	Resource      string
	Scope         string
	State         string
	CodeChallenge string
}

type CallbackResult struct{ RedirectURI string }

type TokenResult struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int       `json:"expires_in"`
	Scope       string    `json:"scope"`
	Principal   Principal `json:"-"`
}

func NewBroker(repository Repository, vault *secrets.Vault, publicURL string, upstream Upstream, accessEvaluator AccessEvaluator, clients ClientMetadataResolver) *Broker {
	if upstream == nil {
		upstream = &OIDCUpstream{repository: repository, vault: vault}
	}
	if accessEvaluator == nil {
		accessEvaluator = &HTTPAccessEvaluation{}
	}
	if clients == nil {
		clients = &HTTPClientMetadataResolver{}
	}
	return &Broker{repository: repository, vault: vault, publicURL: strings.TrimRight(publicURL, "/"), upstream: upstream, accessEvaluator: accessEvaluator, clients: clients, now: func() time.Time { return time.Now().UTC() }}
}

func randomToken(bytesCount int) (string, error) {
	buffer := make([]byte, bytesCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func randomUUID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	buffer[6] = buffer[6]&0x0f | 0x40
	buffer[8] = buffer[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buffer[:4], buffer[4:6], buffer[6:8], buffer[8:10], buffer[10:]), nil
}

func requestID() string {
	buffer := make([]byte, 16)
	_, _ = rand.Read(buffer)
	return "req_" + hex.EncodeToString(buffer)
}

func digest(value string) []byte {
	result := sha256.Sum256([]byte(value))
	return result[:]
}

func pkce(value string) string { return base64.RawURLEncoding.EncodeToString(digest(value)) }

func validRedirect(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Fragment != "" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	return parsed.Scheme == "https" || (parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost"))
}

// ValidRedirectURI applies the redirect policy shared by CIMD and dynamically
// registered public MCP clients.
func ValidRedirectURI(value string) bool { return validRedirect(value) }

func normalizeScopes(raw string) ([]string, bool) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		fields = []string{privateMCPScope}
	}
	seen := make(map[string]bool, len(fields))
	for _, scope := range fields {
		if scope != privateMCPScope || seen[scope] {
			return nil, false
		}
		seen[scope] = true
	}
	return []string{privateMCPScope}, true
}

func (b *Broker) canonicalResource(_ string) string {
	return b.publicURL + "/mcp"
}

func (b *Broker) Begin(ctx context.Context, request AuthorizationRequest) (string, error) {
	scopes, scopesOK := normalizeScopes(request.Scope)
	if request.ProductID == "" || request.ClientID == "" || request.State == "" || len(request.State) > 2048 || !pkcePattern.MatchString(request.CodeChallenge) || !validRedirect(request.RedirectURI) || !scopesOK {
		return "", ErrInvalidOAuth
	}
	if request.Resource != b.canonicalResource(request.ProductID) {
		return "", ErrInvalidOAuth
	}
	metadata, err := b.resolveClient(ctx, request.ProductID, request.ClientID)
	if err != nil || metadata.ClientID != request.ClientID || !contains(metadata.RedirectURIs, request.RedirectURI) {
		return "", ErrInvalidOAuth
	}
	config, err := b.repository.IdentityProvider(ctx, request.ProductID)
	if err != nil || config.State != "active" {
		return "", ErrIdentityDisabled
	}
	rawState, err := randomToken(32)
	if err != nil {
		return "", err
	}
	nonce, err := randomToken(24)
	if err != nil {
		return "", err
	}
	upstreamVerifier, err := randomToken(48)
	if err != nil {
		return "", err
	}
	if err := b.repository.CreateOAuthState(ctx, OAuthState{Digest: digest(rawState), ProductID: request.ProductID, ClientID: request.ClientID, RedirectURI: request.RedirectURI, Resource: request.Resource, Scopes: scopes, DownstreamState: request.State, DownstreamChallenge: request.CodeChallenge, UpstreamVerifier: upstreamVerifier, Nonce: nonce, ExpiresAt: b.now().Add(10 * time.Minute)}); err != nil {
		return "", err
	}
	callback := b.publicURL + "/oauth/callback"
	return b.upstream.AuthorizationURL(ctx, config, rawState, nonce, pkce(upstreamVerifier), callback)
}

func (b *Broker) resolveClient(ctx context.Context, productID, clientID string) (ClientMetadata, error) {
	if registry, ok := b.repository.(ClientRegistry); ok {
		registered, err := registry.OAuthClient(ctx, productID, clientID)
		if err == nil {
			return ClientMetadata{ClientID: registered.ClientID, ClientName: registered.ClientName, RedirectURIs: registered.RedirectURIs}, nil
		}
		if strings.HasPrefix(clientID, "mcp_client_") {
			return ClientMetadata{}, ErrInvalidOAuth
		}
	}
	return b.clients.Resolve(ctx, clientID)
}

func minTime(values ...time.Time) time.Time {
	var result time.Time
	for _, value := range values {
		if value.IsZero() {
			continue
		}
		if result.IsZero() || value.Before(result) {
			result = value
		}
	}
	return result
}

func (b *Broker) saveDelegatedToken(ctx context.Context, config ProviderConfig, token string) (string, error) {
	if b.vault == nil || token == "" {
		return "", errors.New("delegated vendor access token is unavailable")
	}
	id, err := randomUUID()
	if err != nil {
		return "", err
	}
	encrypted, err := b.vault.Encrypt([]byte(token), config.OrganisationID+":delegated:"+id)
	if err != nil {
		return "", err
	}
	_, err = b.repository.CreateSecret(ctx, model.Secret{ID: id, OrganisationID: config.OrganisationID, Name: "vendor-delegated-access-" + id, Purpose: "vendor_delegated_access", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint})
	return id, err
}

func grantsMap(grants []string) map[string]bool {
	result := make(map[string]bool, len(grants))
	for _, grant := range grants {
		result[grant] = true
	}
	return result
}

func (b *Broker) Callback(ctx context.Context, rawState, code string) (CallbackResult, error) {
	state, err := b.repository.ConsumeOAuthState(ctx, digest(rawState))
	if err != nil || state.ProductID == "" || b.now().After(state.ExpiresAt) || code == "" {
		return CallbackResult{}, ErrInvalidOAuth
	}
	productID := state.ProductID
	config, err := b.repository.IdentityProvider(ctx, productID)
	if err != nil || config.State != "active" {
		return CallbackResult{}, ErrIdentityDisabled
	}
	callback := b.publicURL + "/oauth/callback"
	upstream, err := b.upstream.ExchangeAndVerify(ctx, config, code, state.UpstreamVerifier, state.Nonce, callback)
	if err != nil || upstream.Claims.Subject == "" || upstream.Claims.Issuer != config.Issuer || upstream.Claims.ExternalCustomerID == "" {
		return CallbackResult{}, ErrInvalidOAuth
	}
	accountID, err := randomUUID()
	if err != nil {
		return CallbackResult{}, err
	}
	account, err := b.repository.ResolveCustomerAccount(ctx, CustomerAccount{ID: accountID, OrganisationID: config.OrganisationID, ProductID: productID, Issuer: upstream.Claims.Issuer, ExternalID: upstream.Claims.ExternalCustomerID, State: "active", LastAuthenticatedAt: b.now()})
	if err != nil || account.State != "active" {
		return CallbackResult{}, ErrInvalidOAuth
	}
	if upstream.Claims.InstallationID != "" {
		installation, installationErr := b.repository.ProductInstallationByExternalID(ctx, productID, upstream.Claims.InstallationID)
		if installationErr != nil || installation.State != "active" || installation.CustomerAccountID != account.ID {
			return CallbackResult{}, ErrInvalidOAuth
		}
	}
	upstream.AccessEvaluationKey = "aeval_" + hex.EncodeToString(state.Digest)[:32]
	evaluation, err := b.accessEvaluator.Resolve(ctx, config, upstream)
	if err != nil {
		return CallbackResult{}, fmt.Errorf("vendor access evaluation failed closed: %w", err)
	}
	secretID, err := b.saveDelegatedToken(ctx, config, upstream.AccessToken)
	if err != nil {
		return CallbackResult{}, err
	}
	accessExpiresAt := minTime(b.now().Add(time.Hour), upstream.ExpiresAt, evaluation.ExpiresAt)
	if accessExpiresAt.IsZero() || !accessExpiresAt.After(b.now()) {
		return CallbackResult{}, ErrInvalidOAuth
	}
	rawCode, err := randomToken(32)
	if err != nil {
		return CallbackResult{}, err
	}
	value := OAuthCode{Digest: digest(rawCode), ProductID: productID, ClientID: state.ClientID, RedirectURI: state.RedirectURI, Resource: state.Resource, Scopes: state.Scopes, DownstreamChallenge: state.DownstreamChallenge, Issuer: upstream.Claims.Issuer, Subject: upstream.Claims.Subject, Email: upstream.Claims.Email, DisplayName: upstream.Claims.DisplayName, CustomerAccountID: account.ID, ExternalCustomerID: account.ExternalID, InstallationID: upstream.Claims.InstallationID, Grants: grantsMap(evaluation.Grants), AccessEvaluationID: evaluation.ID, PolicyVersion: evaluation.PolicyVersion, UpstreamAccessSecretID: secretID, AccessExpiresAt: accessExpiresAt, ExpiresAt: b.now().Add(5 * time.Minute)}
	if err := b.repository.CreateOAuthCode(ctx, value); err != nil {
		return CallbackResult{}, err
	}
	redirect, _ := url.Parse(state.RedirectURI)
	query := redirect.Query()
	query.Set("code", rawCode)
	query.Set("state", state.DownstreamState)
	redirect.RawQuery = query.Encode()
	return CallbackResult{RedirectURI: redirect.String()}, nil
}

func (b *Broker) Exchange(ctx context.Context, rawCode, verifier, clientID, redirectURI, resource string) (TokenResult, error) {
	code, err := b.repository.ConsumeOAuthCode(ctx, digest(rawCode))
	if err != nil || b.now().After(code.ExpiresAt) || !code.AccessExpiresAt.After(b.now()) || code.ClientID != clientID || code.RedirectURI != redirectURI || code.Resource != resource || !hmac.Equal([]byte(code.DownstreamChallenge), []byte(pkce(verifier))) {
		return TokenResult{}, ErrInvalidOAuth
	}
	rawToken, err := randomToken(32)
	if err != nil {
		return TokenResult{}, err
	}
	now := b.now()
	record := AccessToken{Digest: digest(rawToken), ProductID: code.ProductID, ClientID: code.ClientID, Resource: code.Resource, Issuer: code.Issuer, Subject: code.Subject, Email: code.Email, DisplayName: code.DisplayName, CustomerAccountID: code.CustomerAccountID, ExternalCustomerID: code.ExternalCustomerID, InstallationID: code.InstallationID, Grants: code.Grants, AccessEvaluationID: code.AccessEvaluationID, PolicyVersion: code.PolicyVersion, UpstreamAccessSecretID: code.UpstreamAccessSecretID, Scopes: code.Scopes, ExpiresAt: code.AccessExpiresAt, CreatedAt: now}
	if err := b.repository.CreateAccessToken(ctx, record); err != nil {
		return TokenResult{}, err
	}
	principal := Principal{ProductID: record.ProductID, ClientID: record.ClientID, Resource: record.Resource, Issuer: record.Issuer, Subject: record.Subject, Email: record.Email, DisplayName: record.DisplayName, CustomerAccountID: record.CustomerAccountID, ExternalCustomerID: record.ExternalCustomerID, InstallationID: record.InstallationID, Grants: record.Grants, AccessEvaluationID: record.AccessEvaluationID, PolicyVersion: record.PolicyVersion, Scopes: record.Scopes}
	return TokenResult{AccessToken: "doko_at_" + rawToken, TokenType: "Bearer", ExpiresIn: max(1, int(record.ExpiresAt.Sub(now).Seconds())), Scope: strings.Join(record.Scopes, " "), Principal: principal}, nil
}

func (b *Broker) Authenticate(ctx context.Context, token string) (Principal, error) {
	if !strings.HasPrefix(token, "doko_at_") {
		return Principal{}, ErrInvalidOAuth
	}
	record, err := b.repository.AccessTokenByDigest(ctx, digest(strings.TrimPrefix(token, "doko_at_")))
	if err != nil || record.RevokedAt != nil || b.now().After(record.ExpiresAt) || record.Resource != b.canonicalResource(record.ProductID) {
		return Principal{}, ErrInvalidOAuth
	}
	if b.vault == nil || record.UpstreamAccessSecretID == "" {
		return Principal{}, ErrInvalidOAuth
	}
	config, err := b.repository.IdentityProvider(ctx, record.ProductID)
	if err != nil || config.State != "active" {
		return Principal{}, ErrInvalidOAuth
	}
	account, err := b.repository.CustomerAccount(ctx, record.ProductID, record.CustomerAccountID)
	if err != nil || account.State != "active" || account.ExternalID != record.ExternalCustomerID {
		return Principal{}, ErrInvalidOAuth
	}
	stored, err := b.repository.Secret(ctx, config.OrganisationID, record.UpstreamAccessSecretID)
	if err != nil {
		return Principal{}, ErrInvalidOAuth
	}
	plaintext, err := b.vault.Decrypt(secrets.Encrypted{Ciphertext: stored.Ciphertext, Nonce: stored.Nonce, KeyVersion: stored.KeyVersion, Fingerprint: stored.Fingerprint}, config.OrganisationID+":delegated:"+record.UpstreamAccessSecretID)
	if err != nil {
		return Principal{}, ErrInvalidOAuth
	}
	return Principal{ProductID: record.ProductID, ClientID: record.ClientID, Resource: record.Resource, Issuer: record.Issuer, Subject: record.Subject, Email: record.Email, DisplayName: record.DisplayName, CustomerAccountID: record.CustomerAccountID, ExternalCustomerID: record.ExternalCustomerID, InstallationID: record.InstallationID, Grants: record.Grants, AccessEvaluationID: record.AccessEvaluationID, PolicyVersion: record.PolicyVersion, DelegatedAPIOrigin: config.DelegatedAPIOrigin, UpstreamAccessToken: string(plaintext), Scopes: record.Scopes}, nil
}

type OIDCUpstream struct {
	repository Repository
	vault      *secrets.Vault
	Client     *http.Client
	Resolver   IPResolver
}

func sameOrigin(left, right string) bool {
	a, errA := url.Parse(left)
	b, errB := url.Parse(right)
	return errA == nil && errB == nil && strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func (u *OIDCUpstream) oauthConfig(ctx context.Context, config ProviderConfig, callback string) (context.Context, *oidc.Provider, oauth2.Config, error) {
	issuer, err := url.Parse(config.Issuer)
	if err != nil {
		return ctx, nil, oauth2.Config{}, err
	}
	client, err := SafeOutboundClient(ctx, issuer, u.Client, u.Resolver)
	if err != nil {
		return ctx, nil, oauth2.Config{}, err
	}
	providerCtx := oidc.ClientContext(ctx, client)
	provider, err := oidc.NewProvider(providerCtx, config.Issuer)
	if err != nil {
		return ctx, nil, oauth2.Config{}, err
	}
	endpoint := provider.Endpoint()
	if !sameOrigin(config.Issuer, endpoint.AuthURL) || !sameOrigin(config.Issuer, endpoint.TokenURL) {
		return ctx, nil, oauth2.Config{}, errors.New("OIDC endpoints must share the configured issuer origin")
	}
	secret, err := u.repository.Secret(ctx, config.OrganisationID, config.ClientSecretID)
	if err != nil {
		return ctx, nil, oauth2.Config{}, err
	}
	plaintext, err := u.vault.Decrypt(secrets.Encrypted{Ciphertext: secret.Ciphertext, Nonce: secret.Nonce, KeyVersion: secret.KeyVersion, Fingerprint: secret.Fingerprint}, config.OrganisationID+":idp:"+config.ClientSecretID)
	if err != nil {
		return ctx, nil, oauth2.Config{}, err
	}
	oauthCtx := context.WithValue(providerCtx, oauth2.HTTPClient, client)
	return oauthCtx, provider, oauth2.Config{ClientID: config.ClientID, ClientSecret: string(plaintext), Endpoint: endpoint, RedirectURL: callback, Scopes: config.Scopes}, nil
}

func (u *OIDCUpstream) AuthorizationURL(ctx context.Context, config ProviderConfig, state, nonce, challenge, callback string) (string, error) {
	_, _, oauthConfig, err := u.oauthConfig(ctx, config, callback)
	if err != nil {
		return "", err
	}
	options := []oauth2.AuthCodeOption{oidc.Nonce(nonce), oauth2.SetAuthURLParam("code_challenge", challenge), oauth2.SetAuthURLParam("code_challenge_method", "S256")}
	if config.Audience != "" {
		options = append(options, oauth2.SetAuthURLParam("audience", config.Audience))
	}
	if config.OAuthResource != "" {
		options = append(options, oauth2.SetAuthURLParam("resource", config.OAuthResource))
	}
	return oauthConfig.AuthCodeURL(state, options...), nil
}

func (u *OIDCUpstream) ExchangeAndVerify(ctx context.Context, config ProviderConfig, code, verifier, nonce, callback string) (UpstreamIdentity, error) {
	oauthCtx, provider, oauthConfig, err := u.oauthConfig(ctx, config, callback)
	if err != nil {
		return UpstreamIdentity{}, err
	}
	options := []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("code_verifier", verifier)}
	if config.OAuthResource != "" {
		options = append(options, oauth2.SetAuthURLParam("resource", config.OAuthResource))
	}
	token, err := oauthConfig.Exchange(oauthCtx, code, options...)
	if err != nil {
		return UpstreamIdentity{}, err
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return UpstreamIdentity{}, errors.New("vendor IdP omitted id_token")
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: config.ClientID}).Verify(oauthCtx, rawIDToken)
	if err != nil || idToken.Nonce != nonce {
		return UpstreamIdentity{}, ErrInvalidOAuth
	}
	claims := map[string]any{}
	if err := idToken.Claims(&claims); err != nil {
		return UpstreamIdentity{}, err
	}
	organisation, _ := claims[config.OrganisationClaim].(string)
	installation, _ := claims[config.InstallationClaim].(string)
	name, _ := claims["name"].(string)
	email, _ := claims["email"].(string)
	expiresAt := token.Expiry
	if expiresAt.IsZero() {
		expiresAt = time.Now().UTC().Add(time.Hour)
	}
	return UpstreamIdentity{Claims: Claims{Issuer: idToken.Issuer, Subject: idToken.Subject, Email: email, DisplayName: name, ExternalCustomerID: organisation, InstallationID: installation}, AccessToken: token.AccessToken, ExpiresAt: expiresAt}, nil
}

type IPResolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if hmac.Equal([]byte(value), []byte(expected)) {
			return true
		}
	}
	return false
}

func unsafeIP(address net.IP) bool {
	if address == nil || address.IsUnspecified() || address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() {
		return true
	}
	for _, raw := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "2001:db8::/32", "fc00::/7", "fe80::/10"} {
		_, block, _ := net.ParseCIDR(raw)
		if block.Contains(address) {
			return true
		}
	}
	return false
}

func safeClient(ctx context.Context, parsed *url.URL, provided *http.Client, resolver IPResolver) (*http.Client, error) {
	if parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Port() != "" && parsed.Port() != "443") {
		return nil, errors.New("destination must be credential-free HTTPS on port 443")
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIP(ctx, "ip", parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("destination did not resolve safely")
	}
	for _, address := range addresses {
		if unsafeIP(address) {
			return nil, errors.New("destination resolves to a non-public address")
		}
	}
	if provided != nil {
		return provided, nil
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{Proxy: nil, DisableCompression: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: parsed.Hostname()}, ResponseHeaderTimeout: 10 * time.Second, DialContext: func(dialContext context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(dialContext, network, net.JoinHostPort(addresses[0].String(), "443"))
	}}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

// SafeOutboundClient returns a DNS-pinned, redirect-disabled HTTPS client for every
// vendor-controlled or client-metadata destination.
func SafeOutboundClient(ctx context.Context, parsed *url.URL, client *http.Client, resolver IPResolver) (*http.Client, error) {
	return safeClient(ctx, parsed, client, resolver)
}

type HTTPClientMetadataResolver struct {
	Client   *http.Client
	Resolver IPResolver
}

func (r *HTTPClientMetadataResolver) Resolve(ctx context.Context, clientID string) (ClientMetadata, error) {
	parsed, err := url.Parse(clientID)
	if err != nil || parsed.Scheme != "https" || parsed.Path == "" || parsed.Path == "/" || parsed.RawQuery != "" {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	client, err := SafeOutboundClient(ctx, parsed, r.Client, r.Resolver)
	if err != nil {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, clientID, nil)
	if err != nil {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	var metadata ClientMetadata
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&metadata); err != nil || decoder.Decode(&struct{}{}) != io.EOF || metadata.ClientID != clientID || len(metadata.RedirectURIs) == 0 || len(metadata.RedirectURIs) > 20 {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	for _, redirect := range metadata.RedirectURIs {
		if !validRedirect(redirect) {
			return ClientMetadata{}, ErrInvalidOAuth
		}
	}
	return metadata, nil
}

type HTTPAccessEvaluation struct {
	Client   *http.Client
	Resolver IPResolver
	Now      func() time.Time
}

func retryDelay(header string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return min(time.Duration(seconds)*time.Second, 250*time.Millisecond)
	}
	return 100 * time.Millisecond
}

func (h *HTTPAccessEvaluation) Resolve(ctx context.Context, config ProviderConfig, upstream UpstreamIdentity) (AccessEvaluation, error) {
	now := time.Now().UTC
	if h.Now != nil {
		now = h.Now
	}
	if config.DelegatedAPIOrigin == "" {
		return AccessEvaluation{}, errors.New("vendor access evaluation is not configured")
	}
	parsed, err := url.Parse(config.DelegatedAPIOrigin + accessEvaluationPath)
	if err != nil {
		return AccessEvaluation{}, err
	}
	client, err := SafeOutboundClient(ctx, parsed, h.Client, h.Resolver)
	if err != nil {
		return AccessEvaluation{}, err
	}
	body := []byte(`{}`)
	idempotencyKey := upstream.AccessEvaluationKey
	if idempotencyKey == "" {
		return AccessEvaluation{}, errors.New("access evaluation key is unavailable")
	}
	var response *http.Response
	for attempt := 0; attempt < 2; attempt++ {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
		if requestErr != nil {
			return AccessEvaluation{}, requestErr
		}
		request.Header.Set("Authorization", "Bearer "+upstream.AccessToken)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Idempotency-Key", idempotencyKey)
		request.Header.Set("X-DokoSoko-Request-ID", requestID())
		response, err = client.Do(request)
		if err == nil && response.StatusCode != http.StatusRequestTimeout && response.StatusCode != http.StatusTooManyRequests && response.StatusCode < 500 {
			break
		}
		if response != nil {
			response.Body.Close()
		}
		if attempt == 0 {
			delay := 100 * time.Millisecond
			if response != nil {
				delay = retryDelay(response.Header.Get("Retry-After"))
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return AccessEvaluation{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if err != nil {
		return AccessEvaluation{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return AccessEvaluation{}, fmt.Errorf("access evaluation returned %s", response.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return AccessEvaluation{}, errors.New("access evaluation response is too large")
	}
	var value AccessEvaluation
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&value); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return AccessEvaluation{}, errors.New("invalid access evaluation response")
	}
	if value.ID == "" || len(value.ID) > 200 || value.ExpiresAt.Before(now().Add(-time.Second)) || value.ExpiresAt.After(now().Add(24*time.Hour)) || len(value.PolicyVersion) > 200 || len(value.Grants) > 500 {
		return AccessEvaluation{}, errors.New("invalid access evaluation response")
	}
	seen := make(map[string]bool, len(value.Grants))
	for _, grant := range value.Grants {
		if !grantPattern.MatchString(grant) || seen[grant] {
			return AccessEvaluation{}, errors.New("invalid access evaluation grants")
		}
		seen[grant] = true
	}
	sort.Strings(value.Grants)
	return value, nil
}
