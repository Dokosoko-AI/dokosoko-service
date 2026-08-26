package store

import (
	"encoding/json"
	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

func cloneRoot(account auth.RootAccount) auth.RootAccount {
	account.TOTPSecretCiphertext = append([]byte(nil), account.TOTPSecretCiphertext...)
	account.RecoveryCodeDigests = make([][]byte, len(account.RecoveryCodeDigests))
	for index, digest := range account.RecoveryCodeDigests {
		account.RecoveryCodeDigests[index] = append([]byte(nil), digest...)
	}
	return account
}

func cloneSession(session auth.SessionRecord) auth.SessionRecord {
	session.TokenDigest = append([]byte(nil), session.TokenDigest...)
	session.CSRFDigest = append([]byte(nil), session.CSRFDigest...)
	return session
}

func cloneSecret(value model.Secret) model.Secret {
	value.Ciphertext = append([]byte(nil), value.Ciphertext...)
	value.Nonce = append([]byte(nil), value.Nonce...)
	return value
}

func cloneTool(value model.Tool) model.Tool {
	value.CredentialPresent = value.CredentialID != ""
	value.InputSchema = append([]byte(nil), value.InputSchema...)
	value.OutputSchema = append([]byte(nil), value.OutputSchema...)
	value.UpstreamAuth = append([]byte(nil), value.UpstreamAuth...)
	value.RequestMapping = append([]byte(nil), value.RequestMapping...)
	value.ResponseMapping = append([]byte(nil), value.ResponseMapping...)
	value.RequestExample = append([]byte(nil), value.RequestExample...)
	value.ResponseExample = append([]byte(nil), value.ResponseExample...)
	value.AuthorizationPolicy = append([]byte(nil), value.AuthorizationPolicy...)
	value.UpstreamAnnotations = append([]byte(nil), value.UpstreamAnnotations...)
	value.RuntimeTargets = cloneToolRuntimeTargets(value.RuntimeTargets)
	return value
}

func cloneToolRuntimeTargets(values []model.ToolRuntimeTarget) []model.ToolRuntimeTarget {
	result := make([]model.ToolRuntimeTarget, len(values))
	copy(result, values)
	for index := range result {
		result[index].AuthConfig = append(json.RawMessage(nil), result[index].AuthConfig...)
	}
	return result
}

func cloneMCPConnection(value model.MCPConnection) model.MCPConnection {
	value.Config = append([]byte(nil), value.Config...)
	if value.LastSyncedAt != nil {
		lastSynced := *value.LastSyncedAt
		value.LastSyncedAt = &lastSynced
	}
	return value
}

func cloneIdentityProviderTest(value identity.ProviderTest) identity.ProviderTest {
	value.StateDigest = append([]byte(nil), value.StateDigest...)
	if value.CompletedAt != nil {
		completed := *value.CompletedAt
		value.CompletedAt = &completed
	}
	if value.CallbackClaimedAt != nil {
		claimed := *value.CallbackClaimedAt
		value.CallbackClaimedAt = &claimed
	}
	return value
}

func cloneOAuthState(value identity.OAuthState) identity.OAuthState {
	value.Digest = append([]byte(nil), value.Digest...)
	value.Scopes = append([]string(nil), value.Scopes...)
	return value
}

func cloneOAuthCode(value identity.OAuthCode) identity.OAuthCode {
	value.Digest = append([]byte(nil), value.Digest...)
	value.Scopes = append([]string(nil), value.Scopes...)
	value.Grants = cloneGrants(value.Grants)
	return value
}

func cloneAccessToken(value identity.AccessToken) identity.AccessToken {
	value.Digest = append([]byte(nil), value.Digest...)
	value.Grants = cloneGrants(value.Grants)
	value.Scopes = append([]string(nil), value.Scopes...)
	return value
}

func cloneGrants(value map[string]bool) map[string]bool {
	result := make(map[string]bool, len(value))
	for key, enabled := range value {
		result[key] = enabled
	}
	return result
}

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
