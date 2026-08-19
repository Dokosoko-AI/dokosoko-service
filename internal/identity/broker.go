package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
)

type Repository interface {
	VendorIdentity(context.Context, string) (VendorConfig, error)
	CreateOAuthState(context.Context, OAuthState) error
	ConsumeOAuthState(context.Context, []byte) (OAuthState, error)
	CreateOAuthCode(context.Context, OAuthCode) error
	ConsumeOAuthCode(context.Context, []byte) (OAuthCode, error)
	CreateAccessToken(context.Context, AccessToken) error
	AccessTokenByDigest(context.Context, []byte) (AccessToken, error)
	Secret(context.Context, string, string) (model.Secret, error)
}

type Upstream interface {
	AuthorizationURL(context.Context, VendorConfig, string, string, string, string) (string, error)
	ExchangeAndVerify(context.Context, VendorConfig, string, string, string, string, string) (Claims, string, error)
}

type EntitlementResolver interface {
	Resolve(context.Context, VendorConfig, Claims, string) (map[string]bool, error)
}

type Broker struct {
	repository   Repository
	vault        *secrets.Vault
	publicURL    string
	upstream     Upstream
	entitlements EntitlementResolver
	now          func() time.Time
}

type AuthorizationRequest struct {
	ProductID     string
	ClientID      string
	RedirectURI   string
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

func NewBroker(repository Repository, vault *secrets.Vault, publicURL string, upstream Upstream, entitlements EntitlementResolver) *Broker {
	if upstream == nil {
		upstream = &OIDCUpstream{repository: repository, vault: vault}
	}
	if entitlements == nil {
		entitlements = &HookEntitlements{}
	}
	return &Broker{repository: repository, vault: vault, publicURL: strings.TrimRight(publicURL, "/"), upstream: upstream, entitlements: entitlements, now: func() time.Time { return time.Now().UTC() }}
}

func randomToken(bytesCount int) (string, error) {
	buffer := make([]byte, bytesCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func digest(value string) []byte {
	result := sha256.Sum256([]byte(value))
	return result[:]
}

func pkce(value string) string { return base64.RawURLEncoding.EncodeToString(digest(value)) }

func validRedirect(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Fragment != "" || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "https" || (parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost"))
}

func (b *Broker) Begin(ctx context.Context, request AuthorizationRequest) (string, error) {
	if request.ProductID == "" || request.ClientID == "" || request.State == "" || len(request.CodeChallenge) < 43 || !validRedirect(request.RedirectURI) {
		return "", ErrInvalidOAuth
	}
	config, err := b.repository.VendorIdentity(ctx, request.ProductID)
	if err != nil {
		return "", ErrIdentityDisabled
	}
	if request.ClientID != request.ProductID || !contains(config.AllowedRedirectURIs, request.RedirectURI) {
		return "", ErrInvalidOAuth
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
	if err := b.repository.CreateOAuthState(ctx, OAuthState{Digest: digest(rawState), ProductID: request.ProductID, ClientID: request.ClientID, RedirectURI: request.RedirectURI, DownstreamState: request.State, DownstreamChallenge: request.CodeChallenge, UpstreamVerifier: upstreamVerifier, Nonce: nonce, ExpiresAt: b.now().Add(10 * time.Minute)}); err != nil {
		return "", err
	}
	callback := b.publicURL + "/oauth/callback/" + url.PathEscape(request.ProductID)
	return b.upstream.AuthorizationURL(ctx, config, rawState, nonce, pkce(upstreamVerifier), callback)
}

func (b *Broker) Callback(ctx context.Context, productID, rawState, code string) (CallbackResult, error) {
	state, err := b.repository.ConsumeOAuthState(ctx, digest(rawState))
	if err != nil || state.ProductID != productID || b.now().After(state.ExpiresAt) || code == "" {
		return CallbackResult{}, ErrInvalidOAuth
	}
	config, err := b.repository.VendorIdentity(ctx, productID)
	if err != nil {
		return CallbackResult{}, ErrIdentityDisabled
	}
	callback := b.publicURL + "/oauth/callback/" + url.PathEscape(productID)
	claims, vendorAccessToken, err := b.upstream.ExchangeAndVerify(ctx, config, code, state.UpstreamVerifier, state.Nonce, callback, config.Audience)
	if err != nil || claims.Subject == "" || claims.Issuer != config.Issuer {
		return CallbackResult{}, ErrInvalidOAuth
	}
	entitlements, err := b.entitlements.Resolve(ctx, config, claims, vendorAccessToken)
	if err != nil {
		return CallbackResult{}, fmt.Errorf("vendor entitlement resolution failed closed: %w", err)
	}
	rawCode, err := randomToken(32)
	if err != nil {
		return CallbackResult{}, err
	}
	if err := b.repository.CreateOAuthCode(ctx, OAuthCode{Digest: digest(rawCode), ProductID: productID, ClientID: state.ClientID, RedirectURI: state.RedirectURI, DownstreamChallenge: state.DownstreamChallenge, Issuer: claims.Issuer, Subject: claims.Subject, Email: claims.Email, DisplayName: claims.DisplayName, VendorOrganisation: claims.VendorOrganisation, Entitlements: entitlements, ExpiresAt: b.now().Add(5 * time.Minute)}); err != nil {
		return CallbackResult{}, err
	}
	redirect, _ := url.Parse(state.RedirectURI)
	query := redirect.Query()
	query.Set("code", rawCode)
	query.Set("state", state.DownstreamState)
	redirect.RawQuery = query.Encode()
	return CallbackResult{RedirectURI: redirect.String()}, nil
}

func (b *Broker) Exchange(ctx context.Context, rawCode, verifier, clientID, redirectURI string) (TokenResult, error) {
	code, err := b.repository.ConsumeOAuthCode(ctx, digest(rawCode))
	if err != nil || b.now().After(code.ExpiresAt) || code.ClientID != clientID || code.RedirectURI != redirectURI || !hmac.Equal([]byte(code.DownstreamChallenge), []byte(pkce(verifier))) {
		return TokenResult{}, ErrInvalidOAuth
	}
	rawToken, err := randomToken(32)
	if err != nil {
		return TokenResult{}, err
	}
	now := b.now()
	scopes := []string{"mcp:private"}
	record := AccessToken{Digest: digest(rawToken), ProductID: code.ProductID, ClientID: code.ClientID, Issuer: code.Issuer, Subject: code.Subject, Email: code.Email, DisplayName: code.DisplayName, VendorOrganisation: code.VendorOrganisation, Entitlements: code.Entitlements, Scopes: scopes, ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	if err := b.repository.CreateAccessToken(ctx, record); err != nil {
		return TokenResult{}, err
	}
	return TokenResult{AccessToken: "doko_at_" + rawToken, TokenType: "Bearer", ExpiresIn: 3600, Scope: strings.Join(scopes, " "), Principal: Principal{ProductID: record.ProductID, ClientID: record.ClientID, Issuer: record.Issuer, Subject: record.Subject, Email: record.Email, DisplayName: record.DisplayName, VendorOrganisation: record.VendorOrganisation, Entitlements: record.Entitlements, Scopes: record.Scopes}}, nil
}

func (b *Broker) Authenticate(ctx context.Context, token string) (Principal, error) {
	if !strings.HasPrefix(token, "doko_at_") {
		return Principal{}, ErrInvalidOAuth
	}
	record, err := b.repository.AccessTokenByDigest(ctx, digest(strings.TrimPrefix(token, "doko_at_")))
	if err != nil || record.RevokedAt != nil || b.now().After(record.ExpiresAt) {
		return Principal{}, ErrInvalidOAuth
	}
	return Principal{ProductID: record.ProductID, ClientID: record.ClientID, Issuer: record.Issuer, Subject: record.Subject, Email: record.Email, DisplayName: record.DisplayName, VendorOrganisation: record.VendorOrganisation, Entitlements: record.Entitlements, Scopes: record.Scopes}, nil
}

type OIDCUpstream struct {
	repository Repository
	vault      *secrets.Vault
}

func (u *OIDCUpstream) oauthConfig(ctx context.Context, config VendorConfig, callback string) (*oidc.Provider, oauth2.Config, error) {
	provider, err := oidc.NewProvider(ctx, config.Issuer)
	if err != nil {
		return nil, oauth2.Config{}, err
	}
	secret, err := u.repository.Secret(ctx, config.OrganisationID, config.ClientSecretID)
	if err != nil {
		return nil, oauth2.Config{}, err
	}
	plaintext, err := u.vault.Decrypt(secrets.Encrypted{Ciphertext: secret.Ciphertext, Nonce: secret.Nonce, KeyVersion: secret.KeyVersion, Fingerprint: secret.Fingerprint}, config.OrganisationID+":idp:"+config.ClientSecretID)
	if err != nil {
		return nil, oauth2.Config{}, err
	}
	return provider, oauth2.Config{ClientID: config.ClientID, ClientSecret: string(plaintext), Endpoint: provider.Endpoint(), RedirectURL: callback, Scopes: config.Scopes}, nil
}

func (u *OIDCUpstream) AuthorizationURL(ctx context.Context, config VendorConfig, state, nonce, challenge, callback string) (string, error) {
	_, oauthConfig, err := u.oauthConfig(ctx, config, callback)
	if err != nil {
		return "", err
	}
	options := []oauth2.AuthCodeOption{oidc.Nonce(nonce), oauth2.SetAuthURLParam("code_challenge", challenge), oauth2.SetAuthURLParam("code_challenge_method", "S256")}
	if config.Audience != "" {
		options = append(options, oauth2.SetAuthURLParam("audience", config.Audience))
	}
	return oauthConfig.AuthCodeURL(state, options...), nil
}

func (u *OIDCUpstream) ExchangeAndVerify(ctx context.Context, config VendorConfig, code, verifier, nonce, callback, _ string) (Claims, string, error) {
	provider, oauthConfig, err := u.oauthConfig(ctx, config, callback)
	if err != nil {
		return Claims{}, "", err
	}
	token, err := oauthConfig.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return Claims{}, "", err
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return Claims{}, "", errors.New("vendor IdP omitted id_token")
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: config.ClientID}).Verify(ctx, rawIDToken)
	if err != nil || idToken.Nonce != nonce {
		return Claims{}, "", ErrInvalidOAuth
	}
	claims := map[string]any{}
	if err := idToken.Claims(&claims); err != nil {
		return Claims{}, "", err
	}
	organisation, _ := claims[config.OrganisationClaim].(string)
	name, _ := claims["name"].(string)
	email, _ := claims["email"].(string)
	return Claims{Issuer: idToken.Issuer, Subject: idToken.Subject, Email: email, DisplayName: name, VendorOrganisation: organisation}, token.AccessToken, nil
}

type IPResolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}

type HookEntitlements struct {
	Client   *http.Client
	Resolver IPResolver
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

func (h *HookEntitlements) clientFor(ctx context.Context, parsed *url.URL) (*http.Client, error) {
	resolver := h.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIP(ctx, "ip", parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("entitlement hook host did not resolve safely")
	}
	for _, address := range addresses {
		if unsafeIP(address) {
			return nil, errors.New("entitlement hook resolves to a non-public address")
		}
	}
	if h.Client != nil {
		return h.Client, nil
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{Proxy: nil, DisableCompression: true, ResponseHeaderTimeout: 10 * time.Second, DialContext: func(dialContext context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(dialContext, network, net.JoinHostPort(addresses[0].String(), "443"))
	}}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func (h *HookEntitlements) Resolve(ctx context.Context, config VendorConfig, claims Claims, vendorAccessToken string) (map[string]bool, error) {
	if config.EntitlementHookURL == "" {
		return map[string]bool{}, nil
	}
	parsed, err := url.Parse(config.EntitlementHookURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("unsafe entitlement hook URL")
	}
	encoded, err := json.Marshal(map[string]string{"subject": claims.Subject, "vendor_organisation_id": claims.VendorOrganisation, "product_id": config.ProductID})
	if err != nil {
		return nil, err
	}
	body := strings.NewReader(string(encoded))
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+vendorAccessToken)
	client, err := h.clientFor(ctx, parsed)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("entitlement hook returned %s", response.Status)
	}
	var value struct {
		Entitlements map[string]bool `json:"entitlements"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&value); err != nil {
		return nil, err
	}
	return value.Entitlements, nil
}
