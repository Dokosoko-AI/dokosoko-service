package store

import (
	"bytes"
	"context"
	"encoding/hex"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"slices"
	"sort"
	"time"
)

func (m *Memory) IdentityProvider(_ context.Context, productID string) (identity.ProviderConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.idps[productID]
	if !ok {
		return identity.ProviderConfig{}, ErrNotFound
	}
	value.Scopes = append([]string(nil), value.Scopes...)
	return value, nil
}

func (m *Memory) SaveIdentityProvider(_ context.Context, value identity.ProviderConfig) (identity.ProviderConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.DeploymentID]; !ok {
		return identity.ProviderConfig{}, ErrNotFound
	}
	if current, ok := m.idps[value.DeploymentID]; ok {
		if value.Revision != current.Revision {
			return identity.ProviderConfig{}, ErrConflict
		}
		value.ID, value.CreatedAt, value.Revision = current.ID, current.CreatedAt, current.Revision+1
	} else {
		if value.Revision != 0 {
			return identity.ProviderConfig{}, ErrConflict
		}
		value.Revision = 1
		value.CreatedAt = time.Now().UTC()
	}
	value.UpdatedAt = time.Now().UTC()
	value.Scopes = append([]string(nil), value.Scopes...)
	m.idps[value.DeploymentID] = value
	return value, nil
}

func (m *Memory) DeleteIdentityProvider(_ context.Context, deploymentID string, expectedRevision int64) (identity.ProviderConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.idps[deploymentID]
	if !ok {
		return identity.ProviderConfig{}, ErrNotFound
	}
	if current.Revision != expectedRevision || current.State != "disabled" {
		return identity.ProviderConfig{}, ErrConflict
	}
	secretIDs := map[string]bool{current.ClientSecretID: true}
	for key, value := range m.oauthState {
		if value.ProductID == deploymentID {
			delete(m.oauthState, key)
		}
	}
	for key, value := range m.oauthCodes {
		if value.ProductID == deploymentID {
			secretIDs[value.UpstreamAccessSecretID] = true
			delete(m.oauthCodes, key)
		}
	}
	for key, value := range m.accessTokens {
		if value.ProductID == deploymentID {
			secretIDs[value.UpstreamAccessSecretID] = true
			delete(m.accessTokens, key)
		}
	}
	for id, value := range m.identityTests {
		if value.DeploymentID == deploymentID {
			delete(m.identityTests, id)
		}
	}
	delete(m.idps, deploymentID)
	for secretID := range secretIDs {
		if secretID == "" {
			continue
		}
		referenced := false
		for _, provider := range m.idps {
			if provider.ClientSecretID == secretID {
				referenced = true
				break
			}
		}
		for _, code := range m.oauthCodes {
			if code.UpstreamAccessSecretID == secretID {
				referenced = true
				break
			}
		}
		for _, token := range m.accessTokens {
			if token.UpstreamAccessSecretID == secretID {
				referenced = true
				break
			}
		}
		if !referenced {
			delete(m.secrets, secretID)
		}
	}
	return current, nil
}

func (m *Memory) CreateIdentityProviderTest(_ context.Context, value identity.ProviderTest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID {
		return ErrNotFound
	}
	provider, ok := m.idps[value.DeploymentID]
	if !ok || provider.Revision != value.ConfigurationRevision || (provider.State != "disabled" && provider.State != "active") {
		return ErrConflict
	}
	if _, exists := m.identityTests[value.ID]; exists {
		return ErrConflict
	}
	for _, current := range m.identityTests {
		if bytes.Equal(current.StateDigest, value.StateDigest) {
			return ErrConflict
		}
	}
	m.identityTests[value.ID] = cloneIdentityProviderTest(value)
	return nil
}

func (m *Memory) IdentityProviderTest(_ context.Context, deploymentID, id string) (identity.ProviderTest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.identityTests[id]
	if !ok || value.DeploymentID != deploymentID {
		return identity.ProviderTest{}, ErrNotFound
	}
	return cloneIdentityProviderTest(value), nil
}

func (m *Memory) ClaimIdentityProviderTestByStateDigest(_ context.Context, digest []byte, now time.Time) (identity.ProviderTest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, value := range m.identityTests {
		if !bytes.Equal(value.StateDigest, digest) {
			continue
		}
		if value.Status != "pending" || value.CallbackClaimedAt != nil {
			return identity.ProviderTest{}, ErrConflict
		}
		if !value.ExpiresAt.After(now) {
			completedAt := now
			value.Status = "expired"
			value.FailureCode = "test_expired"
			value.UpstreamVerifier = ""
			value.Nonce = ""
			value.Subject = ""
			value.CustomerID = ""
			value.CompletedAt = &completedAt
			m.identityTests[id] = cloneIdentityProviderTest(value)
			return identity.ProviderTest{}, ErrConflict
		}
		claimedAt := now
		value.CallbackClaimedAt = &claimedAt
		m.identityTests[id] = cloneIdentityProviderTest(value)
		return cloneIdentityProviderTest(value), nil
	}
	return identity.ProviderTest{}, ErrNotFound
}

func (m *Memory) LatestIdentityProviderTest(_ context.Context, deploymentID string) (identity.ProviderTest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var latest identity.ProviderTest
	found := false
	for _, value := range m.identityTests {
		if value.DeploymentID != deploymentID {
			continue
		}
		if !found || value.CreatedAt.After(latest.CreatedAt) || value.CreatedAt.Equal(latest.CreatedAt) && value.ID > latest.ID {
			latest, found = value, true
		}
	}
	if !found {
		return identity.ProviderTest{}, ErrNotFound
	}
	return cloneIdentityProviderTest(latest), nil
}

func (m *Memory) CompleteIdentityProviderTest(_ context.Context, value identity.ProviderTest) (identity.ProviderTest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.identityTests[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return identity.ProviderTest{}, ErrNotFound
	}
	if current.Status != "pending" || current.ConfigurationRevision != value.ConfigurationRevision {
		return identity.ProviderTest{}, ErrConflict
	}
	current.Status = value.Status
	current.FailureCode = value.FailureCode
	current.Issuer = value.Issuer
	current.Subject = value.Subject
	current.CustomerID = value.CustomerID
	current.CompletedAt = value.CompletedAt
	current.UpstreamVerifier = ""
	current.Nonce = ""
	m.identityTests[current.ID] = cloneIdentityProviderTest(current)
	return cloneIdentityProviderTest(current), nil
}

func (m *Memory) ExpireIdentityProviderTests(_ context.Context, deploymentID string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, current := range m.identityTests {
		if current.DeploymentID != deploymentID || current.ExpiresAt.After(now) {
			continue
		}
		claimActive := current.Status == "pending" && current.CallbackClaimedAt != nil && current.CallbackClaimedAt.Add(2*time.Minute).After(now)
		if claimActive {
			continue
		}
		if current.Status == "pending" {
			completedAt := now
			current.Status = "expired"
			current.FailureCode = "test_expired"
			current.UpstreamVerifier = ""
			current.Nonce = ""
			current.CompletedAt = &completedAt
		}
		current.Subject = ""
		current.CustomerID = ""
		m.identityTests[id] = cloneIdentityProviderTest(current)
	}
	return nil
}

func (m *Memory) OAuthClient(_ context.Context, deploymentID, clientID string) (identity.OAuthClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.oauthClients[clientID]
	if !ok || value.DeploymentID != deploymentID {
		return identity.OAuthClient{}, ErrNotFound
	}
	value.RedirectURIs = append([]string(nil), value.RedirectURIs...)
	return value, nil
}

func (m *Memory) CreateOAuthClient(_ context.Context, value identity.OAuthClient) (identity.OAuthClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID {
		return identity.OAuthClient{}, ErrNotFound
	}
	if current, ok := m.oauthClients[value.ClientID]; ok {
		if current.DeploymentID != value.DeploymentID || current.ClientName != value.ClientName || !slices.Equal(current.RedirectURIs, value.RedirectURIs) {
			return identity.OAuthClient{}, ErrConflict
		}
		current.RedirectURIs = append([]string(nil), current.RedirectURIs...)
		return current, nil
	}
	value.CreatedAt = time.Now().UTC()
	value.RedirectURIs = append([]string(nil), value.RedirectURIs...)
	m.oauthClients[value.ClientID] = value
	return value, nil
}

func (m *Memory) ResolveCustomerAccount(_ context.Context, value identity.CustomerAccount) (identity.CustomerAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, current := range m.customerAccounts {
		if current.ProductID == value.ProductID && current.Issuer == value.Issuer && current.ExternalID == value.ExternalID {
			current.LastAuthenticatedAt = value.LastAuthenticatedAt
			current.UpdatedAt = value.LastAuthenticatedAt
			m.customerAccounts[id] = current
			return current, nil
		}
	}
	if _, ok := m.products[value.ProductID]; !ok {
		return identity.CustomerAccount{}, ErrNotFound
	}
	value.Revision = 1
	value.CreatedAt, value.UpdatedAt = value.LastAuthenticatedAt, value.LastAuthenticatedAt
	m.customerAccounts[value.ID] = value
	return value, nil
}

func (m *Memory) CustomerAccounts(_ context.Context, productID, startingAfter string, limit int) ([]identity.CustomerAccount, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.products[productID]; !ok {
		return nil, false, ErrNotFound
	}
	result := make([]identity.CustomerAccount, 0)
	for _, value := range m.customerAccounts {
		if value.ProductID == productID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	start := 0
	if startingAfter != "" {
		start = -1
		for index := range result {
			if result[index].ID == startingAfter {
				start = index + 1
				break
			}
		}
		if start < 0 {
			return nil, false, ErrNotFound
		}
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	result = result[start:]
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

func (m *Memory) CustomerAccount(_ context.Context, productID, id string) (identity.CustomerAccount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.customerAccounts[id]
	if !ok || value.ProductID != productID {
		return identity.CustomerAccount{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) UpdateCustomerAccount(_ context.Context, value identity.CustomerAccount, expected int64) (identity.CustomerAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.customerAccounts[value.ID]
	if !ok || current.ProductID != value.ProductID {
		return identity.CustomerAccount{}, ErrNotFound
	}
	if current.Revision != expected {
		return identity.CustomerAccount{}, ErrConflict
	}
	current.State, current.Revision, current.UpdatedAt = value.State, current.Revision+1, time.Now().UTC()
	m.customerAccounts[value.ID] = current
	return current, nil
}

func (m *Memory) CreateOAuthState(_ context.Context, value identity.OAuthState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, ok := m.idps[value.ProductID]
	if !ok || provider.State != "active" || provider.Revision != value.ProviderRevision {
		return ErrConflict
	}
	m.oauthState[hex.EncodeToString(value.Digest)] = cloneOAuthState(value)
	return nil
}

func (m *Memory) ConsumeOAuthState(_ context.Context, digest []byte) (identity.OAuthState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hex.EncodeToString(digest)
	value, ok := m.oauthState[key]
	if !ok {
		return identity.OAuthState{}, ErrNotFound
	}
	delete(m.oauthState, key)
	return cloneOAuthState(value), nil
}

func (m *Memory) CreateOAuthCode(_ context.Context, value identity.OAuthCode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, ok := m.idps[value.ProductID]
	if !ok || provider.State != "active" || provider.Revision != value.ProviderRevision {
		return ErrConflict
	}
	m.oauthCodes[hex.EncodeToString(value.Digest)] = cloneOAuthCode(value)
	return nil
}

func (m *Memory) ConsumeOAuthCode(_ context.Context, digest []byte) (identity.OAuthCode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hex.EncodeToString(digest)
	value, ok := m.oauthCodes[key]
	if !ok {
		return identity.OAuthCode{}, ErrNotFound
	}
	delete(m.oauthCodes, key)
	return cloneOAuthCode(value), nil
}

func (m *Memory) CreateAccessToken(_ context.Context, value identity.AccessToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, ok := m.idps[value.ProductID]
	if !ok || provider.State != "active" || provider.Revision != value.ProviderRevision {
		return ErrConflict
	}
	m.accessTokens[hex.EncodeToString(value.Digest)] = cloneAccessToken(value)
	return nil
}

func (m *Memory) AccessTokenByDigest(_ context.Context, digest []byte) (identity.AccessToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.accessTokens[hex.EncodeToString(digest)]
	if !ok {
		return identity.AccessToken{}, ErrNotFound
	}
	return cloneAccessToken(value), nil
}

func (m *Memory) DeleteStaleOAuthArtifacts(_ context.Context, productID string, now time.Time, limit int) (int64, error) {
	limit = boundedOAuthArtifactCleanupLimit(limit)
	if limit == 0 {
		return 0, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	activeProviderRevision := int64(0)
	if provider, ok := m.idps[productID]; ok && provider.State == "active" {
		activeProviderRevision = provider.Revision
	}

	var deleted int64
	statesDeleted := 0
	for key, value := range m.oauthState {
		if statesDeleted >= limit {
			break
		}
		if value.ProductID == productID && (!value.ExpiresAt.After(now) || activeProviderRevision <= 0 || value.ProviderRevision != activeProviderRevision) {
			delete(m.oauthState, key)
			statesDeleted++
			deleted++
		}
	}

	secretIDs := make(map[string]bool)
	codesDeleted := 0
	for key, value := range m.oauthCodes {
		if codesDeleted >= limit {
			break
		}
		if value.ProductID == productID && (!value.ExpiresAt.After(now) || !value.AccessExpiresAt.After(now) || activeProviderRevision <= 0 || value.ProviderRevision != activeProviderRevision) {
			delete(m.oauthCodes, key)
			secretIDs[value.UpstreamAccessSecretID] = true
			codesDeleted++
			deleted++
		}
	}

	tokensDeleted := 0
	for key, value := range m.accessTokens {
		if tokensDeleted >= limit {
			break
		}
		if value.ProductID == productID && (!value.ExpiresAt.After(now) || value.RevokedAt != nil || activeProviderRevision <= 0 || value.ProviderRevision != activeProviderRevision) {
			delete(m.accessTokens, key)
			secretIDs[value.UpstreamAccessSecretID] = true
			tokensDeleted++
			deleted++
		}
	}

	for secretID := range secretIDs {
		if secretID == "" {
			continue
		}
		referenced := false
		for _, value := range m.oauthCodes {
			if value.UpstreamAccessSecretID == secretID {
				referenced = true
				break
			}
		}
		if !referenced {
			for _, value := range m.accessTokens {
				if value.UpstreamAccessSecretID == secretID {
					referenced = true
					break
				}
			}
		}
		if secret, ok := m.secrets[secretID]; ok && !referenced && secret.Purpose == "vendor_delegated_access" {
			delete(m.secrets, secretID)
		}
	}

	product, ok := m.products[productID]
	if !ok {
		return deleted, ErrNotFound
	}
	orphansDeleted := 0
	orphanCutoff := now.Add(-identitySecretOrphanGrace)
	for secretID, secret := range m.secrets {
		if orphansDeleted >= limit || secret.OrganisationID != product.OrganisationID || secret.CreatedAt.After(orphanCutoff) {
			continue
		}
		referencedByOAuth := false
		for _, value := range m.oauthCodes {
			if value.UpstreamAccessSecretID == secretID {
				referencedByOAuth = true
				break
			}
		}
		if !referencedByOAuth {
			for _, value := range m.accessTokens {
				if value.UpstreamAccessSecretID == secretID {
					referencedByOAuth = true
					break
				}
			}
		}
		referencedByProvider := false
		for _, value := range m.idps {
			if value.ClientSecretID == secretID {
				referencedByProvider = true
				break
			}
		}
		vendorOrphan := secret.Purpose == "vendor_delegated_access" && !referencedByOAuth
		providerOrphan := secret.Purpose == "identity_provider_oidc_client" && !referencedByProvider
		if vendorOrphan || providerOrphan {
			delete(m.secrets, secretID)
			orphansDeleted++
			deleted++
		}
	}
	return deleted, nil
}
