package access

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/netpolicy"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
)

func validOperationPath(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(parsed.Path, "/") || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	cleaned := path.Clean(parsed.Path)
	return cleaned == parsed.Path && !strings.Contains(parsed.Path, "..")
}

func (r *Runtime) clientForURL(ctx context.Context, destination *url.URL) (Doer, error) {
	localDevelopment := r.privateLocalDevelopmentURL(destination)
	if (destination.Scheme != "https" && !localDevelopment) || destination.Hostname() == "" || destination.User != nil || (destination.Scheme == "https" && destination.Port() != "") {
		return nil, ErrUnsafeDestination
	}
	addresses, err := r.resolver.LookupIP(ctx, "ip", destination.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, ErrUnsafeDestination
	}
	for _, address := range addresses {
		if netpolicy.UnsafeIP(address) && !localDevelopment {
			return nil, ErrUnsafeDestination
		}
	}
	if r.doer != nil {
		return r.doer, nil
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	port := destination.Port()
	if port == "" {
		port = "443"
		if destination.Scheme == "http" {
			port = "80"
		}
	}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: destination.Hostname()}, DisableCompression: true, ResponseHeaderTimeout: 10 * time.Second, DialContext: func(dialContext context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(dialContext, network, net.JoinHostPort(addresses[0].String(), port))
	}}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func (r *Runtime) destination(ctx context.Context, connection model.AccessConnection, operationPath string) (*url.URL, Doer, error) {
	base, err := url.Parse(connection.BaseURL)
	if err != nil || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || !validOperationPath(operationPath) {
		return nil, nil, ErrUnsafeDestination
	}
	base.Path = strings.TrimRight(base.Path, "/") + operationPath
	client, err := r.clientForURL(ctx, base)
	return base, client, err
}

func (r *Runtime) managementSecret(ctx context.Context, connection model.AccessConnection) ([]byte, error) {
	if r.vault == nil || connection.ManagementSecretID == "" {
		return nil, errors.New("access management credential unavailable")
	}
	value, err := r.store.Secret(ctx, connection.OrganisationID, connection.ManagementSecretID)
	if err != nil {
		return nil, err
	}
	encrypted := secrets.Encrypted{Ciphertext: value.Ciphertext, Nonce: value.Nonce, KeyVersion: value.KeyVersion, Fingerprint: value.Fingerprint}
	plaintext, err := r.vault.Decrypt(encrypted, connection.OrganisationID+":access:"+connection.ManagementSecretID)
	if err != nil && connection.LegacyProviderID != "" {
		plaintext, err = r.vault.Decrypt(encrypted, connection.OrganisationID+":provider:"+connection.ManagementSecretID)
	}
	return plaintext, err
}

func (r *Runtime) oauthToken(ctx context.Context, connection model.AccessConnection) (string, error) {
	r.tokenMu.Lock()
	if cached := r.tokens[connection.ID]; cached.Token != "" && cached.ExpiresAt.After(r.now().Add(30*time.Second)) {
		r.tokenMu.Unlock()
		return cached.Token, nil
	}
	r.tokenMu.Unlock()
	var cfg connectionConfig
	if err := json.Unmarshal(connection.Config, &cfg); err != nil || cfg.TokenURL == "" {
		return "", errors.New("OAuth management token endpoint is not configured")
	}
	tokenURL, err := url.Parse(cfg.TokenURL)
	if err != nil {
		return "", ErrUnsafeDestination
	}
	client, err := r.clientForURL(ctx, tokenURL)
	if err != nil {
		return "", err
	}
	credential, err := r.managementSecret(ctx, connection)
	if err != nil {
		return "", err
	}
	defer func() { clear(credential) }()
	var pair struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if json.Unmarshal(credential, &pair) != nil || pair.ClientID == "" || pair.ClientSecret == "" {
		return "", errors.New("OAuth management credential must contain client_id and client_secret")
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	if len(cfg.Scopes) > 0 {
		form.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	if cfg.Audience != "" {
		form.Set("audience", cfg.Audience)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.SetBasicAuth(pair.ClientID, pair.ClientSecret)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("management OAuth endpoint returned %s", response.Status)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result) != nil || result.AccessToken == "" {
		return "", errors.New("management OAuth endpoint returned an invalid token response")
	}
	if result.ExpiresIn <= 0 || result.ExpiresIn > 86400 {
		result.ExpiresIn = 3600
	}
	r.tokenMu.Lock()
	r.tokens[connection.ID] = tokenCacheEntry{Token: result.AccessToken, ExpiresAt: r.now().Add(time.Duration(result.ExpiresIn) * time.Second)}
	r.tokenMu.Unlock()
	return result.AccessToken, nil
}

func (r *Runtime) authenticate(ctx context.Context, request *http.Request, connection model.AccessConnection, definition model.AccessDefinition) error {
	switch definition.ManagementAuthType {
	case "none":
		return nil
	case "bearer":
		credential, err := r.managementSecret(ctx, connection)
		if err != nil {
			return err
		}
		defer clear(credential)
		request.Header.Set("Authorization", "Bearer "+string(credential))
		return nil
	case "api_key":
		credential, err := r.managementSecret(ctx, connection)
		if err != nil {
			return err
		}
		defer clear(credential)
		var cfg connectionConfig
		_ = json.Unmarshal(connection.Config, &cfg)
		if cfg.APIKeyHeader == "" {
			cfg.APIKeyHeader = "X-API-Key"
		}
		if strings.EqualFold(cfg.APIKeyHeader, "Authorization") || !httpgutsValidHeaderName(cfg.APIKeyHeader) {
			return ErrInvalidRequest
		}
		request.Header.Set(cfg.APIKeyHeader, string(credential))
		return nil
	case "oauth2_client_credentials":
		token, err := r.oauthToken(ctx, connection)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		return nil
	default:
		return ErrUnsupported
	}
}

func httpgutsValidHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", character)) {
			return false
		}
	}
	return true
}

func (r *Runtime) call(ctx context.Context, connection model.AccessConnection, definition model.AccessDefinition, operation operation, input, output any) error {
	destination, client, err := r.destination(ctx, connection, operation.Path)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, operation.Method, destination.String(), bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	if err := r.authenticate(ctx, request, connection, definition); err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return fmt.Errorf("access provider returned %s", response.Status)
	}
	if output == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output)
}

func (r *Runtime) authorize(ctx context.Context, connection model.AccessConnection, definition model.AccessDefinition, cfg definitionConfig, principal Principal, operationName string, details map[string]any) error {
	if !allowed(cfg, principal) {
		return ErrDenied
	}
	authorize, configured := cfg.Operations["authorize"]
	if !configured {
		return nil
	}
	var response struct {
		Allowed bool `json:"allowed"`
	}
	err := r.call(ctx, connection, definition, authorize, map[string]any{"operation": operationName, "subject": principal.Subject, "external_customer_id": principal.ExternalCustomerID, "installation_id": principal.InstallationID, "deployment_id": connection.DeploymentID, "details": details}, &response)
	if errors.Is(err, ErrUnsafeDestination) {
		return err
	}
	if err != nil || !response.Allowed {
		return ErrDenied
	}
	return nil
}

func validTTL(value, maximum int) bool {
	if value == 0 {
		return true
	}
	return value >= 300 && (maximum == 0 || value <= maximum)
}
