package tools

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func auditID() string {
	buffer := make([]byte, 16)
	_, _ = rand.Read(buffer)
	return "audit_" + hex.EncodeToString(buffer)
}

func (r *Runtime) Execute(ctx context.Context, productID, fullName string, arguments map[string]any, principal Principal) (any, error) {
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
	return r.executeAuthorized(ctx, productID, fullName, tool, arguments, principal)
}

func toolUpstreamAuth(tool model.Tool) (upstreamAuth, error) {
	if len(tool.UpstreamAuth) == 0 {
		return upstreamAuth{Type: "delegated_oauth"}, nil
	}
	var value upstreamAuth
	if err := json.Unmarshal(tool.UpstreamAuth, &value); err != nil || value.Type == "" {
		return upstreamAuth{}, ErrDenied
	}
	if value.Type == "authorization_scheme" && !validAuthorizationScheme(value.Scheme) {
		return upstreamAuth{}, ErrDenied
	}
	if value.Type == "api_key_header" || value.Type == "custom_header" {
		if len(value.Prefix) > 64 || strings.ContainsAny(value.Prefix, "\r\n\x00") {
			return upstreamAuth{}, ErrDenied
		}
		value.Prefix = strings.TrimSpace(value.Prefix)
	}
	return value, nil
}

func prefixedCredential(prefix string, credential []byte) string {
	if prefix == "" {
		return string(credential)
	}
	return prefix + " " + string(credential)
}

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

func toolRequestMapping(tool model.Tool) (requestMapping, error) {
	if len(tool.RequestMapping) == 0 {
		return requestMapping{}, nil
	}
	var value requestMapping
	if err := json.Unmarshal(tool.RequestMapping, &value); err != nil {
		return requestMapping{}, ErrDenied
	}
	return value, nil
}

func toolResponseMapping(tool model.Tool) (responseMapping, error) {
	if len(tool.ResponseMapping) == 0 {
		return responseMapping{}, nil
	}
	var value responseMapping
	if err := json.Unmarshal(tool.ResponseMapping, &value); err != nil {
		return responseMapping{}, ErrDenied
	}
	return value, nil
}

func wipe(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func (r *Runtime) toolCredential(ctx context.Context, tool model.Tool) ([]byte, error) {
	if r.credentials == nil || tool.CredentialID == "" {
		return nil, ErrDenied
	}
	value, err := r.credentials.ResolveToolCredential(ctx, tool)
	if err != nil || len(value) == 0 {
		wipe(value)
		return nil, ErrDenied
	}
	return value, nil
}

func (r *Runtime) purgeExpiredOAuthTokensLocked(now time.Time) {
	for key, cached := range r.tokens {
		if now.Add(oauthTokenRefreshSkew).Before(cached.ExpiresAt) {
			continue
		}
		wipe(cached.AccessToken)
		delete(r.tokens, key)
	}
}

func (r *Runtime) cacheOAuthTokenLocked(cacheKey, tokenType, accessToken string, expiresAt time.Time) {
	if r.tokens == nil {
		r.tokens = make(map[string]cachedOAuthToken)
	}
	if previous, ok := r.tokens[cacheKey]; ok {
		wipe(previous.AccessToken)
	} else {
		for len(r.tokens) >= maxOAuthTokenCacheEntries {
			var evictionKey string
			var eviction cachedOAuthToken
			for key, cached := range r.tokens {
				if evictionKey == "" || cached.ExpiresAt.Before(eviction.ExpiresAt) || cached.ExpiresAt.Equal(eviction.ExpiresAt) && key < evictionKey {
					evictionKey = key
					eviction = cached
				}
			}
			wipe(eviction.AccessToken)
			delete(r.tokens, evictionKey)
		}
	}
	r.tokens[cacheKey] = cachedOAuthToken{AccessToken: []byte(accessToken), TokenType: tokenType, ExpiresAt: expiresAt}
}

func (r *Runtime) exchangeOAuthClientToken(ctx context.Context, tool model.Tool, auth upstreamAuth, trace *executionTrace) (string, string, time.Time, error) {
	credential, err := r.toolCredential(ctx, tool)
	if err != nil {
		return "", "", time.Time{}, err
	}
	defer wipe(credential)
	parsed, address, err := r.safeDestination(ctx, auth.TokenURL)
	if err != nil {
		return "", "", time.Time{}, err
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	if len(auth.Scopes) > 0 {
		form.Set("scope", strings.Join(auth.Scopes, " "))
	}
	if auth.Audience != "" {
		form.Set("audience", auth.Audience)
	}
	if auth.Resource != "" {
		form.Set("resource", auth.Resource)
	}
	if auth.TokenEndpointAuthMethod == "" {
		auth.TokenEndpointAuthMethod = "client_secret_basic"
	}
	if auth.TokenEndpointAuthMethod == "client_secret_post" {
		form.Set("client_id", auth.ClientID)
		form.Set("client_secret", string(credential))
	} else if auth.TokenEndpointAuthMethod != "client_secret_basic" {
		return "", "", time.Time{}, ErrDenied
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", time.Time{}, errors.New("upstream OAuth token request is invalid")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if auth.TokenEndpointAuthMethod == "client_secret_basic" {
		request.SetBasicAuth(url.QueryEscape(auth.ClientID), url.QueryEscape(string(credential)))
	}
	if trace != nil {
		trace.NetworkCallPerformed = true
		trace.Phase = "token_exchange"
	}
	response, err := r.client(parsed, address, min(time.Duration(tool.TimeoutMS)*time.Millisecond, 15*time.Second)).Do(request)
	if err != nil {
		return "", "", time.Time{}, errors.New("upstream OAuth token exchange failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", "", time.Time{}, errors.New("upstream OAuth token exchange failed")
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, 64<<10+1))
	if err != nil || len(encoded) > 64<<10 {
		return "", "", time.Time{}, errors.New("upstream OAuth token response is invalid")
	}
	defer wipe(encoded)
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	if err := decoder.Decode(&token); err != nil || strings.TrimSpace(token.AccessToken) == "" || strings.ContainsAny(token.AccessToken, "\r\n\x00") {
		return "", "", time.Time{}, errors.New("upstream OAuth token response is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", "", time.Time{}, errors.New("upstream OAuth token response is invalid")
	}
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	if !strings.EqualFold(token.TokenType, "Bearer") {
		return "", "", time.Time{}, errors.New("upstream OAuth token type is unsupported")
	}
	if token.ExpiresIn < 1 || token.ExpiresIn > 86400 {
		token.ExpiresIn = 300
	}
	expiresAt := r.now().Add(time.Duration(token.ExpiresIn) * time.Second)
	return "Bearer", token.AccessToken, expiresAt, nil
}

func (r *Runtime) oauthClientToken(ctx context.Context, tool model.Tool, auth upstreamAuth) (string, string, error) {
	return r.oauthClientTokenTraced(ctx, tool, auth, nil)
}

func (r *Runtime) oauthClientTokenTraced(ctx context.Context, tool model.Tool, auth upstreamAuth, trace *executionTrace) (string, string, error) {
	cacheKey := tool.APIConnectionID + "\x00" + tool.CredentialID
	if tool.RuntimeServiceConnectionID != "" {
		cacheKey = tool.RuntimeServiceConnectionID + "\x00" + tool.RuntimeCredentialVersionID
	}
	for {
		r.tokenMu.Lock()
		r.purgeExpiredOAuthTokensLocked(r.now())
		if cached, ok := r.tokens[cacheKey]; ok {
			tokenType, accessToken := cached.TokenType, string(cached.AccessToken)
			r.tokenMu.Unlock()
			return tokenType, accessToken, nil
		}
		if flight, ok := r.tokenFlights[cacheKey]; ok {
			r.tokenMu.Unlock()
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-flight.done:
				if flight.err != nil {
					return "", "", flight.err
				}
				continue
			}
		}
		if r.tokenFlights == nil {
			r.tokenFlights = make(map[string]*oauthTokenFlight)
		}
		flight := &oauthTokenFlight{done: make(chan struct{})}
		r.tokenFlights[cacheKey] = flight
		r.tokenMu.Unlock()

		tokenType, accessToken, expiresAt, err := r.exchangeOAuthClientToken(ctx, tool, auth, trace)
		r.tokenMu.Lock()
		if err == nil {
			r.purgeExpiredOAuthTokensLocked(r.now())
			r.cacheOAuthTokenLocked(cacheKey, tokenType, accessToken, expiresAt)
		}
		flight.err = err
		delete(r.tokenFlights, cacheKey)
		close(flight.done)
		r.tokenMu.Unlock()
		return tokenType, accessToken, err
	}
}
