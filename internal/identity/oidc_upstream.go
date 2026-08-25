package identity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"golang.org/x/oauth2"
)

type OIDCUpstream struct {
	repository Repository
	vault      *secrets.Vault
	Client     *http.Client
	Resolver   IPResolver
}

func NewOIDCUpstream(repository Repository, vault *secrets.Vault, client *http.Client, resolver IPResolver) *OIDCUpstream {
	return &OIDCUpstream{repository: repository, vault: vault, Client: client, Resolver: resolver}
}

func (u *OIDCUpstream) oauthConfig(ctx context.Context, config ProviderConfig, callback string) (context.Context, *oidc.Provider, oauth2.Config, error) {
	issuer, err := url.Parse(config.Issuer)
	if err != nil {
		return ctx, nil, oauth2.Config{}, err
	}
	client, localBoundary, err := safeOIDCClient(ctx, issuer, u.Client, u.Resolver)
	if err != nil {
		return ctx, nil, oauth2.Config{}, err
	}
	providerCtx := oidc.ClientContext(ctx, client)
	provider, err := oidc.NewProvider(providerCtx, config.Issuer)
	if err != nil {
		return ctx, nil, oauth2.Config{}, err
	}
	endpoint := provider.Endpoint()
	issuerLocal := IsLocalDevelopmentHostname(issuer.Hostname())
	var metadata struct {
		JWKSURL                           string   `json:"jwks_uri"`
		TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return ctx, nil, oauth2.Config{}, err
	}
	for name, raw := range map[string]string{"authorization endpoint": endpoint.AuthURL, "token endpoint": endpoint.TokenURL, "JWKS endpoint": metadata.JWKSURL} {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil {
			return ctx, nil, oauth2.Config{}, fmt.Errorf("invalid OIDC %s: %w", name, parseErr)
		}
		if _, destinationErr := resolveSafeOIDCDestination(ctx, parsed, u.Resolver, issuerLocal, localBoundary); destinationErr != nil {
			return ctx, nil, oauth2.Config{}, fmt.Errorf("unsafe OIDC %s: %w", name, destinationErr)
		}
	}
	endpoint.AuthStyle = oauth2.AuthStyleInHeader
	if len(metadata.TokenEndpointAuthMethodsSupported) > 0 {
		supportsBasic, supportsPost := false, false
		for _, method := range metadata.TokenEndpointAuthMethodsSupported {
			supportsBasic = supportsBasic || method == "client_secret_basic"
			supportsPost = supportsPost || method == "client_secret_post"
		}
		switch {
		case supportsBasic:
			endpoint.AuthStyle = oauth2.AuthStyleInHeader
		case supportsPost:
			endpoint.AuthStyle = oauth2.AuthStyleInParams
		default:
			return ctx, nil, oauth2.Config{}, ErrClientAuthenticationUnsupported
		}
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
		return UpstreamIdentity{}, errors.New("OIDC provider omitted id_token")
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
