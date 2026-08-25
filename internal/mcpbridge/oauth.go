package mcpbridge

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

func subjectID(principal toolruntime.Principal) string {
	return principal.Subject
}

func (m *Manager) BeginAuthorization(ctx context.Context, productID, connectionID string, principal toolruntime.Principal) (string, error) {
	connection, err := m.store.MCPConnection(ctx, productID, connectionID)
	if err != nil {
		return "", err
	}
	if connection.AuthMode != "delegated_oauth" || principal.Subject == "" {
		return "", ErrUnsupportedAuth
	}
	state, err := randomToken(32)
	if err != nil {
		return "", err
	}
	verifier, err := randomToken(48)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(state))
	if err := m.store.CreateMCPAuthorizationState(ctx, model.MCPAuthorizationState{Digest: digest[:], ConnectionID: connectionID, ProductID: productID, SubjectID: subjectID(principal), CodeVerifier: verifier, ExpiresAt: m.now().Add(10 * time.Minute)}); err != nil {
		return "", err
	}
	challenge := sha256.Sum256([]byte(verifier))
	values := url.Values{
		"response_type":         {"code"},
		"client_id":             {connection.OAuthClientID},
		"redirect_uri":          {m.baseURL + "/oauth/upstream/callback"},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
		"resource":              {connection.Endpoint},
	}
	if len(connection.Scopes) > 0 {
		values.Set("scope", strings.Join(connection.Scopes, " "))
	}
	parsed, _ := url.Parse(connection.AuthorizationURL)
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

func (m *Manager) oauthToken(ctx context.Context, connection model.MCPConnection, values url.Values) (tokenResponse, error) {
	parsed, address, err := m.safeDestination(ctx, connection.TokenURL)
	if err != nil {
		return tokenResponse{}, err
	}
	clientSecret, err := m.decryptSecret(ctx, connection.OrganisationID, connection.OAuthClientSecretID, connection.OrganisationID+":mcp-connection:"+connection.ID+":oauth-client:")
	if err != nil {
		return tokenResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), strings.NewReader(values.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.SetBasicAuth(connection.OAuthClientID, clientSecret)
	response, err := m.client(parsed, address, 15*time.Second).Do(request)
	if err != nil {
		return tokenResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return tokenResponse{}, fmt.Errorf("upstream authorization server returned %s", response.Status)
	}
	var token tokenResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&token); err != nil || token.AccessToken == "" || (token.TokenType != "" && !strings.EqualFold(token.TokenType, "Bearer")) {
		return tokenResponse{}, errors.New("upstream authorization server returned an invalid token")
	}
	return token, nil
}

func (m *Manager) saveGrant(ctx context.Context, connection model.MCPConnection, subject string, token tokenResponse) (model.MCPUserGrant, error) {
	id, err := randomUUID()
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	accessID, err := m.saveSecret(ctx, connection.OrganisationID, "mcp-user-access-"+connection.Namespace, "mcp_upstream_user_access", token.AccessToken, connection.OrganisationID+":mcp-grant:"+connection.ID+":"+subject+":access:")
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	refreshID, err := m.saveSecret(ctx, connection.OrganisationID, "mcp-user-refresh-"+connection.Namespace, "mcp_upstream_user_refresh", token.RefreshToken, connection.OrganisationID+":mcp-grant:"+connection.ID+":"+subject+":refresh:")
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	expiresIn := token.ExpiresIn
	if expiresIn <= 0 || expiresIn > int64((30*24*time.Hour).Seconds()) {
		expiresIn = 3600
	}
	scopes := connection.Scopes
	if token.Scope != "" {
		scopes = strings.Fields(token.Scope)
	}
	return m.store.SaveMCPUserGrant(ctx, model.MCPUserGrant{ID: id, OrganisationID: connection.OrganisationID, ProductID: connection.ProductID, ConnectionID: connection.ID, SubjectID: subject, AccessSecretID: accessID, RefreshSecretID: refreshID, Scopes: normalizeScopes(scopes), ExpiresAt: m.now().Add(time.Duration(expiresIn) * time.Second)})
}

func (m *Manager) CompleteAuthorization(ctx context.Context, state, code, issuer string) (model.MCPUserGrant, error) {
	digest := sha256.Sum256([]byte(strings.TrimSpace(state)))
	stored, err := m.store.ConsumeMCPAuthorizationState(ctx, digest[:])
	if err != nil || strings.TrimSpace(code) == "" {
		return model.MCPUserGrant{}, errors.New("upstream authorization state is invalid or expired")
	}
	connection, err := m.store.MCPConnection(ctx, stored.ProductID, stored.ConnectionID)
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	if strings.TrimSpace(issuer) != connection.OAuthIssuer {
		return model.MCPUserGrant{}, errors.New("upstream authorization issuer did not match the pinned issuer")
	}
	token, err := m.oauthToken(ctx, connection, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {m.baseURL + "/oauth/upstream/callback"}, "code_verifier": {stored.CodeVerifier}, "resource": {connection.Endpoint}})
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	grant, err := m.saveGrant(ctx, connection, stored.SubjectID, token)
	if err == nil {
		if err := m.store.AppendAudit(ctx, model.AuditEvent{ID: auditID(), OrganisationID: connection.OrganisationID, ProductID: connection.ProductID, ActorID: stored.SubjectID, Action: "mcp.user_grant.connected", TargetType: "mcp_connection", TargetID: connection.ID, Current: map[string]any{"auth_mode": connection.AuthMode, "scopes": grant.Scopes, "expires_at": grant.ExpiresAt}, CreatedAt: m.now()}); err != nil {
			return model.MCPUserGrant{}, err
		}
	}
	return grant, err
}

func (m *Manager) refreshGrant(ctx context.Context, connection model.MCPConnection, grant model.MCPUserGrant) (model.MCPUserGrant, error) {
	if grant.RefreshSecretID == "" {
		return model.MCPUserGrant{}, ErrGrantRequired
	}
	refresh, err := m.decryptSecret(ctx, connection.OrganisationID, grant.RefreshSecretID, connection.OrganisationID+":mcp-grant:"+connection.ID+":"+grant.SubjectID+":refresh:")
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	token, err := m.oauthToken(ctx, connection, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "resource": {connection.Endpoint}, "scope": {strings.Join(grant.Scopes, " ")}})
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	if token.RefreshToken == "" {
		token.RefreshToken = refresh
	}
	return m.saveGrant(ctx, connection, grant.SubjectID, token)
}

func (m *Manager) delegatedBearer(ctx context.Context, connection model.MCPConnection, principal toolruntime.Principal) (string, error) {
	grant, err := m.store.MCPUserGrant(ctx, connection.ID, subjectID(principal))
	if err != nil {
		return "", ErrGrantRequired
	}
	if m.now().Add(30 * time.Second).After(grant.ExpiresAt) {
		grant, err = m.refreshGrant(ctx, connection, grant)
		if err != nil {
			return "", ErrGrantRequired
		}
	}
	return m.decryptSecret(ctx, connection.OrganisationID, grant.AccessSecretID, connection.OrganisationID+":mcp-grant:"+connection.ID+":"+grant.SubjectID+":access:")
}
