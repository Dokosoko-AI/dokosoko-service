package acceptance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const oauthStateVersion = 2

type OAuthStartConfig struct {
	Issuer      string
	MetadataURL string
	Endpoint    string
	Resource    string
	// AllowedLoopbackHTTP contains exact host:port authorities that may use
	// plain HTTP during local development. It is persisted in resume state so
	// the token exchange cannot silently widen the transport exception.
	AllowedLoopbackHTTP []string
	RedirectURI         string
	Scope               string
	ClientName          string
	ClientID            string
	StateFile           string
	TokenFile           string
	HTTPClient          *http.Client
	Now                 func() time.Time
}

type OAuthStartResult struct {
	AuthorizationURL string
	StateFile        string
	ResumeCommand    string
	DCRPerformed     bool
}

type OAuthFinishConfig struct {
	StateFile     string
	TokenFile     string
	CallbackURL   string
	Code          string
	ReturnedState string
	HTTPClient    *http.Client
	Now           func() time.Time
}

type OAuthFinishResult struct {
	TokenFile string
	ExpiresIn int64
	Scope     string
}

type authorizationMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

type oauthState struct {
	Version               int       `json:"version"`
	CreatedAt             time.Time `json:"created_at"`
	ExpiresAt             time.Time `json:"expires_at"`
	AuthorizationEndpoint string    `json:"authorization_endpoint"`
	TokenEndpoint         string    `json:"token_endpoint"`
	ClientID              string    `json:"client_id"`
	RedirectURI           string    `json:"redirect_uri"`
	Resource              string    `json:"resource"`
	AllowedLoopbackHTTP   []string  `json:"allowed_loopback_http,omitempty"`
	Scope                 string    `json:"scope"`
	State                 string    `json:"state"`
	CodeVerifier          string    `json:"code_verifier"`
}

type oauthTokenFile struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int64     `json:"expires_in"`
	Scope       string    `json:"scope,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Resource    string    `json:"resource"`
}

func StartOAuth(ctx context.Context, config OAuthStartConfig) (OAuthStartResult, error) {
	now := time.Now
	if config.Now != nil {
		now = config.Now
	}
	if config.Scope == "" {
		config.Scope = "mcp:private"
	}
	if config.ClientName == "" {
		config.ClientName = "MCP acceptance client"
	}
	if err := validateSecureEndpoint(config.Endpoint, config.AllowedLoopbackHTTP); err != nil {
		return OAuthStartResult{}, fmt.Errorf("endpoint: %w", err)
	}
	if config.Resource == "" {
		config.Resource = config.Endpoint
	}
	if err := validateSecureEndpoint(config.Resource, config.AllowedLoopbackHTTP); err != nil {
		return OAuthStartResult{}, fmt.Errorf("resource: %w", err)
	}
	if err := validateRedirect(config.RedirectURI); err != nil {
		return OAuthStartResult{}, err
	}
	if config.StateFile == "" {
		return OAuthStartResult{}, errors.New("state file is required")
	}
	metadataURL := strings.TrimSpace(config.MetadataURL)
	expectedIssuer := strings.TrimRight(strings.TrimSpace(config.Issuer), "/")
	if metadataURL == "" {
		if expectedIssuer == "" {
			parsed, _ := url.Parse(config.Endpoint)
			expectedIssuer = parsed.Scheme + "://" + parsed.Host
		}
		derivedMetadataURL, deriveErr := authorizationServerMetadataURL(expectedIssuer)
		if deriveErr != nil {
			return OAuthStartResult{}, fmt.Errorf("issuer: %w", deriveErr)
		}
		metadataURL = derivedMetadataURL
	} else if expectedIssuer == "" {
		parsed, _ := url.Parse(config.Endpoint)
		expectedIssuer = parsed.Scheme + "://" + parsed.Host
	}
	if err := validateSecureEndpoint(metadataURL, config.AllowedLoopbackHTTP); err != nil {
		return OAuthStartResult{}, fmt.Errorf("metadata URL: %w", err)
	}
	client := clientWithoutRedirects(config.HTTPClient, 20*time.Second)
	metadata, err := fetchMetadata(ctx, client, metadataURL, expectedIssuer, config.AllowedLoopbackHTTP)
	if err != nil {
		return OAuthStartResult{}, err
	}
	clientID := strings.TrimSpace(config.ClientID)
	dcr := false
	if clientID == "" {
		if metadata.RegistrationEndpoint == "" {
			return OAuthStartResult{}, errors.New("authorization server metadata did not advertise dynamic client registration; provide --client-id")
		}
		clientID, err = registerClient(ctx, client, metadata.RegistrationEndpoint, config.ClientName, config.RedirectURI, config.Scope)
		if err != nil {
			return OAuthStartResult{}, err
		}
		dcr = true
	}
	verifierBytes, err := randomBytes(48)
	if err != nil {
		return OAuthStartResult{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	csrfState, err := randomID("oauth_")
	if err != nil {
		return OAuthStartResult{}, err
	}
	challenge := sha256.Sum256([]byte(verifier))
	values := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {config.RedirectURI},
		"resource":              {config.Resource},
		"scope":                 {config.Scope},
		"state":                 {csrfState},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	authorizationURL, err := addQuery(metadata.AuthorizationEndpoint, values)
	if err != nil {
		return OAuthStartResult{}, err
	}
	created := now().UTC()
	state := oauthState{
		Version: oauthStateVersion, CreatedAt: created, ExpiresAt: created.Add(20 * time.Minute),
		AuthorizationEndpoint: metadata.AuthorizationEndpoint, TokenEndpoint: metadata.TokenEndpoint,
		ClientID: clientID, RedirectURI: config.RedirectURI, Resource: config.Resource,
		AllowedLoopbackHTTP: append([]string(nil), config.AllowedLoopbackHTTP...),
		Scope:               config.Scope, State: csrfState, CodeVerifier: verifier,
	}
	if err := writePrivateNew(config.StateFile, state); err != nil {
		return OAuthStartResult{}, fmt.Errorf("write OAuth state: %w", err)
	}
	tokenFile := config.TokenFile
	if tokenFile == "" {
		tokenFile = "mcp-token.json"
	}
	resume := fmt.Sprintf("mcp-acceptance oauth finish --state-file %q --callback-url %q --token-file %q", config.StateFile, "PASTE_FULL_CALLBACK_URL_HERE", tokenFile)
	return OAuthStartResult{AuthorizationURL: authorizationURL, StateFile: config.StateFile, ResumeCommand: resume, DCRPerformed: dcr}, nil
}

func FinishOAuth(ctx context.Context, config OAuthFinishConfig) (OAuthFinishResult, error) {
	if config.StateFile == "" || config.TokenFile == "" {
		return OAuthFinishResult{}, errors.New("state file and token file are required")
	}
	now := time.Now
	if config.Now != nil {
		now = config.Now
	}
	var state oauthState
	if err := readPrivateJSON(config.StateFile, &state); err != nil {
		return OAuthFinishResult{}, fmt.Errorf("read OAuth state: %w", err)
	}
	if state.Version != oauthStateVersion || state.ClientID == "" || state.CodeVerifier == "" || state.State == "" {
		return OAuthFinishResult{}, errors.New("OAuth state file is invalid")
	}
	if now().UTC().After(state.ExpiresAt) {
		return OAuthFinishResult{}, errors.New("OAuth state has expired; start again")
	}
	code, returnedState, err := callbackValues(config)
	if err != nil {
		return OAuthFinishResult{}, err
	}
	if returnedState != state.State {
		return OAuthFinishResult{}, errors.New("OAuth callback state did not match")
	}
	if err := validateSecureEndpoint(state.TokenEndpoint, state.AllowedLoopbackHTTP); err != nil {
		return OAuthFinishResult{}, fmt.Errorf("OAuth token endpoint: %w", err)
	}
	if err := validateSecureEndpoint(state.Resource, state.AllowedLoopbackHTTP); err != nil {
		return OAuthFinishResult{}, fmt.Errorf("OAuth resource: %w", err)
	}
	if _, err := os.Stat(config.TokenFile); err == nil {
		return OAuthFinishResult{}, errors.New("token file already exists; choose a new path")
	} else if !errors.Is(err, os.ErrNotExist) {
		return OAuthFinishResult{}, err
	}
	client := clientWithoutRedirects(config.HTTPClient, 20*time.Second)
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {state.CodeVerifier},
		"client_id":     {state.ClientID},
		"redirect_uri":  {state.RedirectURI},
		"resource":      {state.Resource},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, state.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return OAuthFinishResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return OAuthFinishResult{}, errors.New("OAuth token exchange failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return OAuthFinishResult{}, fmt.Errorf("OAuth token endpoint returned HTTP %d", response.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
		Scope       string `json:"scope"`
	}
	if json.Unmarshal(body, &token) != nil || token.AccessToken == "" || !strings.EqualFold(token.TokenType, "Bearer") {
		return OAuthFinishResult{}, errors.New("OAuth token response was invalid")
	}
	stored := oauthTokenFile{AccessToken: token.AccessToken, TokenType: "Bearer", ExpiresIn: token.ExpiresIn, Scope: token.Scope, CreatedAt: now().UTC(), Resource: state.Resource}
	if err := writePrivateNew(config.TokenFile, stored); err != nil {
		return OAuthFinishResult{}, fmt.Errorf("write token file: %w", err)
	}
	if err := os.Remove(config.StateFile); err != nil {
		return OAuthFinishResult{}, errors.New("token was stored, but consumed OAuth state could not be removed")
	}
	return OAuthFinishResult{TokenFile: config.TokenFile, ExpiresIn: token.ExpiresIn, Scope: token.Scope}, nil
}

func LoadToken(file, environment string) (string, error) {
	if file != "" && environment != "" && os.Getenv(environment) != "" {
		return "", errors.New("configure a token file or token environment variable, not both")
	}
	if file == "" {
		return strings.TrimSpace(os.Getenv(environment)), nil
	}
	info, err := os.Stat(file)
	if err != nil {
		return "", err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("token file permissions are too broad; expected mode 0600")
	}
	value, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	var token oauthTokenFile
	if json.Unmarshal(value, &token) == nil && token.AccessToken != "" {
		return token.AccessToken, nil
	}
	raw := strings.TrimSpace(string(value))
	if raw == "" {
		return "", errors.New("token file was empty")
	}
	return raw, nil
}

func fetchMetadata(ctx context.Context, client *http.Client, endpoint, expectedIssuer string, allowedLoopbackHTTP []string) (authorizationMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return authorizationMetadata{}, err
	}
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return authorizationMetadata{}, errors.New("authorization server metadata request failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode != http.StatusOK {
		return authorizationMetadata{}, fmt.Errorf("authorization server metadata returned HTTP %d", response.StatusCode)
	}
	var metadata authorizationMetadata
	if json.Unmarshal(body, &metadata) != nil || metadata.AuthorizationEndpoint == "" || metadata.TokenEndpoint == "" {
		return authorizationMetadata{}, errors.New("authorization server metadata was invalid")
	}
	if strings.TrimRight(metadata.Issuer, "/") != strings.TrimRight(expectedIssuer, "/") {
		return authorizationMetadata{}, errors.New("authorization server metadata issuer did not match the configured issuer")
	}
	if !contains(metadata.CodeChallengeMethodsSupported, "S256") || !contains(metadata.TokenEndpointAuthMethodsSupported, "none") {
		return authorizationMetadata{}, errors.New("authorization server does not advertise public PKCE S256 clients")
	}
	for _, endpoint := range []string{metadata.Issuer, metadata.AuthorizationEndpoint, metadata.TokenEndpoint} {
		if err := validateSecureEndpoint(endpoint, allowedLoopbackHTTP); err != nil {
			return authorizationMetadata{}, errors.New("authorization server metadata contained an unsafe endpoint")
		}
	}
	if metadata.RegistrationEndpoint != "" {
		if err := validateSecureEndpoint(metadata.RegistrationEndpoint, allowedLoopbackHTTP); err != nil {
			return authorizationMetadata{}, errors.New("authorization server metadata contained an unsafe registration endpoint")
		}
	}
	return metadata, nil
}

func registerClient(ctx context.Context, client *http.Client, endpoint, name, redirectURI, scope string) (string, error) {
	payload, _ := json.Marshal(map[string]any{
		"redirect_uris":              []string{redirectURI},
		"client_name":                name,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"scope":                      scope,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return "", errors.New("dynamic client registration failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || (response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK) {
		return "", fmt.Errorf("dynamic client registration returned HTTP %d", response.StatusCode)
	}
	var registration struct {
		ClientID string `json:"client_id"`
	}
	if json.Unmarshal(body, &registration) != nil || registration.ClientID == "" {
		return "", errors.New("dynamic client registration response was invalid")
	}
	return registration.ClientID, nil
}

func callbackValues(config OAuthFinishConfig) (string, string, error) {
	if config.CallbackURL != "" {
		parsed, err := url.Parse(config.CallbackURL)
		if err != nil {
			return "", "", errors.New("callback URL was invalid")
		}
		if parsed.Query().Get("error") != "" {
			return "", "", errors.New("authorization server returned an OAuth error")
		}
		if parsed.Query().Get("code") == "" || parsed.Query().Get("state") == "" {
			return "", "", errors.New("callback URL must contain code and state")
		}
		return parsed.Query().Get("code"), parsed.Query().Get("state"), nil
	}
	if config.Code == "" || config.ReturnedState == "" {
		return "", "", errors.New("provide --callback-url or both --code and --returned-state")
	}
	return config.Code, config.ReturnedState, nil
}

func addQuery(raw string, values url.Values) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	for key, entries := range values {
		for _, value := range entries {
			query.Set(key, value)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func authorizationServerMetadataURL(issuer string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(issuer, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("issuer must be an absolute HTTP(S) URL without user information, a query, or a fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("issuer must use http or https")
	}
	issuerPath := strings.TrimPrefix(parsed.EscapedPath(), "/")
	parsed.Path = "/.well-known/oauth-authorization-server"
	parsed.RawPath = ""
	if issuerPath != "" {
		parsed.RawPath = parsed.Path + "/" + issuerPath
		unescaped, unescapeErr := url.PathUnescape(parsed.RawPath)
		if unescapeErr != nil {
			return "", errors.New("issuer path was invalid")
		}
		parsed.Path = unescaped
	}
	return parsed.String(), nil
}

func validateRedirect(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.Fragment != "" || parsed.User != nil {
		return errors.New("redirect URI must be an absolute URL without user information or a fragment")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme != "http" || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "[::1]" && parsed.Hostname() != "::1" && parsed.Hostname() != "localhost") {
		return errors.New("redirect URI must use HTTPS or loopback HTTP")
	}
	return nil
}

func writePrivateNew(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Clean(path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func readPrivateJSON(path string, destination any) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("file permissions are too broad; expected mode 0600")
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(value, destination)
}
