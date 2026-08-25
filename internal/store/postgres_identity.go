package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/jackc/pgx/v5"
	"time"
)

func scanIdentityProvider(row interface{ Scan(...any) error }) (identity.ProviderConfig, error) {
	var value identity.ProviderConfig
	err := row.Scan(&value.ID, &value.OrganisationID, &value.DeploymentID, &value.Issuer, &value.ClientID, &value.ClientSecretID, &value.Scopes, &value.Audience, &value.OAuthResource, &value.OrganisationClaim, &value.InstallationClaim, &value.DelegatedAPIOrigin, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const identityProviderSelect = `SELECT id::text, organisation_id::text, product_id::text, issuer, client_id, coalesce(client_secret_id::text, ''), scopes, audience, oauth_resource, organisation_claim, installation_claim, delegated_api_origin, state, revision, created_at, updated_at FROM identity_providers`

func (p *Postgres) IdentityProvider(ctx context.Context, deploymentID string) (identity.ProviderConfig, error) {
	return scanIdentityProvider(p.pool.QueryRow(ctx, identityProviderSelect+` WHERE product_id = $1`, deploymentID))
}

func (p *Postgres) SaveIdentityProvider(ctx context.Context, value identity.ProviderConfig) (identity.ProviderConfig, error) {
	var updated identity.ProviderConfig
	var err error
	if value.Revision == 0 {
		updated, err = scanIdentityProvider(p.pool.QueryRow(ctx, `INSERT INTO identity_providers(id, organisation_id, product_id, issuer, client_id, client_secret_id, scopes, audience, oauth_resource, organisation_claim, installation_claim, delegated_api_origin, state) VALUES ($1,$2,$3,$4,$5,nullif($6,'')::uuid,$7,$8,$9,$10,$11,$12,$13) RETURNING id::text, organisation_id::text, product_id::text, issuer, client_id, coalesce(client_secret_id::text, ''), scopes, audience, oauth_resource, organisation_claim, installation_claim, delegated_api_origin, state, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.DeploymentID, value.Issuer, value.ClientID, value.ClientSecretID, value.Scopes, value.Audience, value.OAuthResource, value.OrganisationClaim, value.InstallationClaim, value.DelegatedAPIOrigin, value.State))
	} else {
		updated, err = scanIdentityProvider(p.pool.QueryRow(ctx, `UPDATE identity_providers SET issuer=$2,client_id=$3,client_secret_id=nullif($4,'')::uuid,scopes=$5,audience=$6,oauth_resource=$7,organisation_claim=$8,installation_claim=$9,delegated_api_origin=$10,state=$11,revision=revision+1,updated_at=now() WHERE product_id=$1 AND revision=$12 RETURNING id::text, organisation_id::text, product_id::text, issuer, client_id, coalesce(client_secret_id::text, ''), scopes, audience, oauth_resource, organisation_claim, installation_claim, delegated_api_origin, state, revision, created_at, updated_at`, value.DeploymentID, value.Issuer, value.ClientID, value.ClientSecretID, value.Scopes, value.Audience, value.OAuthResource, value.OrganisationClaim, value.InstallationClaim, value.DelegatedAPIOrigin, value.State, value.Revision))
	}
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.IdentityProvider(ctx, value.DeploymentID); lookupErr == nil {
			return identity.ProviderConfig{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) DeleteIdentityProvider(ctx context.Context, deploymentID string, expectedRevision int64) (identity.ProviderConfig, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return identity.ProviderConfig{}, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := scanIdentityProvider(tx.QueryRow(ctx, identityProviderSelect+` WHERE product_id=$1 AND revision=$2 AND state='disabled' FOR UPDATE`, deploymentID, expectedRevision))
	if errors.Is(err, ErrNotFound) {
		var exists bool
		if lookupErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity_providers WHERE product_id=$1)`, deploymentID).Scan(&exists); lookupErr != nil {
			return identity.ProviderConfig{}, databaseError(lookupErr)
		}
		if exists {
			return identity.ProviderConfig{}, ErrConflict
		}
		return identity.ProviderConfig{}, ErrNotFound
	}
	if err != nil {
		return identity.ProviderConfig{}, err
	}
	secretIDs := map[string]bool{}
	if current.ClientSecretID != "" {
		secretIDs[current.ClientSecretID] = true
	}
	if _, err := tx.Exec(ctx, `DELETE FROM oauth_states WHERE product_id=$1`, deploymentID); err != nil {
		return identity.ProviderConfig{}, databaseError(err)
	}
	for _, table := range []string{"oauth_authorization_codes", "oauth_access_tokens"} {
		rows, queryErr := tx.Query(ctx, `DELETE FROM `+table+` WHERE product_id=$1 RETURNING upstream_access_secret_id::text`, deploymentID)
		if queryErr != nil {
			return identity.ProviderConfig{}, databaseError(queryErr)
		}
		for rows.Next() {
			var secretID string
			if scanErr := rows.Scan(&secretID); scanErr != nil {
				rows.Close()
				return identity.ProviderConfig{}, databaseError(scanErr)
			}
			secretIDs[secretID] = true
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return identity.ProviderConfig{}, databaseError(rowsErr)
		}
		rows.Close()
	}
	if _, err := tx.Exec(ctx, `DELETE FROM identity_provider_tests WHERE product_id=$1`, deploymentID); err != nil {
		return identity.ProviderConfig{}, databaseError(err)
	}
	if result, err := tx.Exec(ctx, `DELETE FROM identity_providers WHERE product_id=$1 AND revision=$2 AND state='disabled'`, deploymentID, expectedRevision); err != nil {
		return identity.ProviderConfig{}, databaseError(err)
	} else if result.RowsAffected() != 1 {
		return identity.ProviderConfig{}, ErrConflict
	}
	if len(secretIDs) > 0 {
		ids := make([]string, 0, len(secretIDs))
		for id := range secretIDs {
			ids = append(ids, id)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM secrets secret WHERE secret.id=ANY($1::uuid[]) AND NOT EXISTS (SELECT 1 FROM identity_providers provider WHERE provider.client_secret_id=secret.id) AND NOT EXISTS (SELECT 1 FROM oauth_authorization_codes code WHERE code.upstream_access_secret_id=secret.id) AND NOT EXISTS (SELECT 1 FROM oauth_access_tokens token WHERE token.upstream_access_secret_id=secret.id)`, ids); err != nil {
			return identity.ProviderConfig{}, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return identity.ProviderConfig{}, databaseError(err)
	}
	return current, nil
}

func scanIdentityProviderTest(row interface{ Scan(...any) error }) (identity.ProviderTest, error) {
	var value identity.ProviderTest
	err := row.Scan(&value.ID, &value.OrganisationID, &value.DeploymentID, &value.ConfigurationRevision, &value.StateDigest, &value.UpstreamVerifier, &value.Nonce, &value.Status, &value.FailureCode, &value.Issuer, &value.Subject, &value.CustomerID, &value.CallbackClaimedAt, &value.CreatedAt, &value.ExpiresAt, &value.CompletedAt)
	return value, databaseError(err)
}

const identityProviderTestSelect = `SELECT id::text,organisation_id::text,product_id::text,configuration_revision,state_digest,upstream_verifier,nonce,status,failure_code,issuer,subject,customer_id,callback_claimed_at,created_at,expires_at,completed_at FROM identity_provider_tests`

func (p *Postgres) CreateIdentityProviderTest(ctx context.Context, value identity.ProviderTest) error {
	result, err := p.pool.Exec(ctx, `INSERT INTO identity_provider_tests(id,organisation_id,product_id,configuration_revision,state_digest,upstream_verifier,nonce,status,failure_code,issuer,subject,customer_id,callback_claimed_at,created_at,expires_at,completed_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16 FROM identity_providers provider WHERE provider.product_id=$3 AND provider.revision=$4 AND provider.state IN ('disabled','active') FOR UPDATE OF provider`, value.ID, value.OrganisationID, value.DeploymentID, value.ConfigurationRevision, value.StateDigest, value.UpstreamVerifier, value.Nonce, value.Status, value.FailureCode, value.Issuer, value.Subject, value.CustomerID, value.CallbackClaimedAt, value.CreatedAt, value.ExpiresAt, value.CompletedAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func (p *Postgres) IdentityProviderTest(ctx context.Context, deploymentID, id string) (identity.ProviderTest, error) {
	return scanIdentityProviderTest(p.pool.QueryRow(ctx, identityProviderTestSelect+` WHERE product_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) ClaimIdentityProviderTestByStateDigest(ctx context.Context, digest []byte, now time.Time) (identity.ProviderTest, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return identity.ProviderTest{}, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	expired, err := tx.Exec(ctx, `UPDATE identity_provider_tests SET status='expired',failure_code='test_expired',upstream_verifier='',nonce='',subject='',customer_id='',completed_at=$2 WHERE state_digest=$1 AND status='pending' AND callback_claimed_at IS NULL AND expires_at<=$2`, digest, now)
	if err != nil {
		return identity.ProviderTest{}, databaseError(err)
	}
	if expired.RowsAffected() > 0 {
		if err := tx.Commit(ctx); err != nil {
			return identity.ProviderTest{}, databaseError(err)
		}
		return identity.ProviderTest{}, ErrConflict
	}
	claimed, err := scanIdentityProviderTest(tx.QueryRow(ctx, `UPDATE identity_provider_tests SET callback_claimed_at=$2 WHERE state_digest=$1 AND status='pending' AND callback_claimed_at IS NULL AND expires_at>$2 RETURNING id::text,organisation_id::text,product_id::text,configuration_revision,state_digest,upstream_verifier,nonce,status,failure_code,issuer,subject,customer_id,callback_claimed_at,created_at,expires_at,completed_at`, digest, now))
	if err != nil {
		return identity.ProviderTest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return identity.ProviderTest{}, databaseError(err)
	}
	return claimed, nil
}

func (p *Postgres) LatestIdentityProviderTest(ctx context.Context, deploymentID string) (identity.ProviderTest, error) {
	return scanIdentityProviderTest(p.pool.QueryRow(ctx, identityProviderTestSelect+` WHERE product_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, deploymentID))
}

func (p *Postgres) CompleteIdentityProviderTest(ctx context.Context, value identity.ProviderTest) (identity.ProviderTest, error) {
	updated, err := scanIdentityProviderTest(p.pool.QueryRow(ctx, `UPDATE identity_provider_tests SET status=$4,failure_code=$5,issuer=$6,subject=$7,customer_id=$8,upstream_verifier='',nonce='',completed_at=$9 WHERE id=$1 AND product_id=$2 AND configuration_revision=$3 AND status='pending' RETURNING id::text,organisation_id::text,product_id::text,configuration_revision,state_digest,upstream_verifier,nonce,status,failure_code,issuer,subject,customer_id,callback_claimed_at,created_at,expires_at,completed_at`, value.ID, value.DeploymentID, value.ConfigurationRevision, value.Status, value.FailureCode, value.Issuer, value.Subject, value.CustomerID, value.CompletedAt))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.IdentityProviderTest(ctx, value.DeploymentID, value.ID); lookupErr == nil {
			return identity.ProviderTest{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) ExpireIdentityProviderTests(ctx context.Context, deploymentID string, now time.Time) error {
	_, err := p.pool.Exec(ctx, `UPDATE identity_provider_tests SET status=CASE WHEN status='pending' THEN 'expired' ELSE status END,failure_code=CASE WHEN status='pending' THEN 'test_expired' ELSE failure_code END,upstream_verifier='',nonce='',subject='',customer_id='',completed_at=CASE WHEN status='pending' THEN $2 ELSE completed_at END WHERE product_id=$1 AND expires_at<=$2 AND (status<>'pending' OR callback_claimed_at IS NULL OR callback_claimed_at <= $2 - interval '2 minutes')`, deploymentID, now)
	return databaseError(err)
}

func scanOAuthClient(row interface{ Scan(...any) error }) (identity.OAuthClient, error) {
	var value identity.OAuthClient
	err := row.Scan(&value.ClientID, &value.DeploymentID, &value.ClientName, &value.RedirectURIs, &value.CreatedAt)
	return value, databaseError(err)
}

const oauthClientSelect = `SELECT client_id, deployment_id::text, client_name, redirect_uris, created_at FROM mcp_oauth_clients`

func (p *Postgres) OAuthClient(ctx context.Context, deploymentID, clientID string) (identity.OAuthClient, error) {
	return scanOAuthClient(p.pool.QueryRow(ctx, oauthClientSelect+` WHERE deployment_id=$1 AND client_id=$2`, deploymentID, clientID))
}

func (p *Postgres) CreateOAuthClient(ctx context.Context, value identity.OAuthClient) (identity.OAuthClient, error) {
	created, err := scanOAuthClient(p.pool.QueryRow(ctx, `INSERT INTO mcp_oauth_clients(client_id,deployment_id,client_name,redirect_uris) VALUES($1,$2,$3,$4) ON CONFLICT(client_id) DO UPDATE SET client_id=excluded.client_id WHERE mcp_oauth_clients.deployment_id=excluded.deployment_id AND mcp_oauth_clients.client_name=excluded.client_name AND mcp_oauth_clients.redirect_uris=excluded.redirect_uris RETURNING client_id,deployment_id::text,client_name,redirect_uris,created_at`, value.ClientID, value.DeploymentID, value.ClientName, value.RedirectURIs))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.OAuthClient(ctx, value.DeploymentID, value.ClientID); lookupErr == nil {
			return identity.OAuthClient{}, ErrConflict
		}
	}
	return created, err
}

func scanCustomerAccount(row interface{ Scan(...any) error }) (identity.CustomerAccount, error) {
	var value identity.CustomerAccount
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Issuer, &value.ExternalID, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt, &value.LastAuthenticatedAt)
	return value, databaseError(err)
}

const customerAccountSelect = `SELECT id::text,organisation_id::text,product_id::text,issuer,external_id,state,revision,created_at,updated_at,last_authenticated_at FROM customer_accounts`

func (p *Postgres) ResolveCustomerAccount(ctx context.Context, value identity.CustomerAccount) (identity.CustomerAccount, error) {
	return scanCustomerAccount(p.pool.QueryRow(ctx, `INSERT INTO customer_accounts(id,organisation_id,product_id,issuer,external_id,state,last_authenticated_at) VALUES ($1,$2,$3,$4,$5,'active',$6) ON CONFLICT(product_id,issuer,external_id) DO UPDATE SET last_authenticated_at=excluded.last_authenticated_at,updated_at=excluded.last_authenticated_at RETURNING id::text,organisation_id::text,product_id::text,issuer,external_id,state,revision,created_at,updated_at,last_authenticated_at`, value.ID, value.OrganisationID, value.ProductID, value.Issuer, value.ExternalID, value.LastAuthenticatedAt))
}

func (p *Postgres) CustomerAccounts(ctx context.Context, productID, startingAfter string, limit int) ([]identity.CustomerAccount, bool, error) {
	if _, err := p.Product(ctx, productID); err != nil {
		return nil, false, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := customerAccountSelect + ` WHERE product_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2`
	arguments := []any{productID, limit + 1}
	if startingAfter != "" {
		var cursorCreatedAt time.Time
		if err := p.pool.QueryRow(ctx, `SELECT created_at FROM customer_accounts WHERE product_id=$1 AND id::text=$2`, productID, startingAfter).Scan(&cursorCreatedAt); err != nil {
			return nil, false, databaseError(err)
		}
		query = customerAccountSelect + ` WHERE product_id=$1 AND (created_at,id::text)<($3,$4) ORDER BY created_at DESC,id DESC LIMIT $2`
		arguments = append(arguments, cursorCreatedAt, startingAfter)
	}
	rows, err := p.pool.Query(ctx, query, arguments...)
	if err != nil {
		return nil, false, databaseError(err)
	}
	defer rows.Close()
	result := make([]identity.CustomerAccount, 0)
	for rows.Next() {
		value, scanErr := scanCustomerAccount(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

func (p *Postgres) CustomerAccount(ctx context.Context, productID, id string) (identity.CustomerAccount, error) {
	return scanCustomerAccount(p.pool.QueryRow(ctx, customerAccountSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) UpdateCustomerAccount(ctx context.Context, value identity.CustomerAccount, expected int64) (identity.CustomerAccount, error) {
	updated, err := scanCustomerAccount(p.pool.QueryRow(ctx, `UPDATE customer_accounts SET state=$3,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$4 RETURNING id::text,organisation_id::text,product_id::text,issuer,external_id,state,revision,created_at,updated_at,last_authenticated_at`, value.ProductID, value.ID, value.State, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.CustomerAccount(ctx, value.ProductID, value.ID); lookupErr == nil {
			return identity.CustomerAccount{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) CreateOAuthState(ctx context.Context, value identity.OAuthState) error {
	result, err := p.pool.Exec(ctx, `INSERT INTO oauth_states(state_digest, product_id, provider_revision, client_id, redirect_uri, resource, scopes, downstream_state, downstream_challenge, upstream_verifier, nonce, expires_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12 FROM identity_providers provider WHERE provider.product_id=$2 AND provider.state='active' AND provider.revision=$3 FOR UPDATE OF provider`, value.Digest, value.ProductID, value.ProviderRevision, value.ClientID, value.RedirectURI, value.Resource, value.Scopes, value.DownstreamState, value.DownstreamChallenge, value.UpstreamVerifier, value.Nonce, value.ExpiresAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func scanOAuthState(row pgx.Row) (identity.OAuthState, error) {
	var value identity.OAuthState
	err := row.Scan(&value.Digest, &value.ProductID, &value.ProviderRevision, &value.ClientID, &value.RedirectURI, &value.Resource, &value.Scopes, &value.DownstreamState, &value.DownstreamChallenge, &value.UpstreamVerifier, &value.Nonce, &value.ExpiresAt)
	return value, databaseError(err)
}

func (p *Postgres) ConsumeOAuthState(ctx context.Context, digest []byte) (identity.OAuthState, error) {
	return scanOAuthState(p.pool.QueryRow(ctx, `DELETE FROM oauth_states WHERE state_digest = $1 RETURNING state_digest, product_id::text, provider_revision, client_id, redirect_uri, resource, scopes, downstream_state, downstream_challenge, upstream_verifier, nonce, expires_at`, digest))
}

func (p *Postgres) CreateOAuthCode(ctx context.Context, value identity.OAuthCode) error {
	grants, _ := json.Marshal(value.Grants)
	result, err := p.pool.Exec(ctx, `INSERT INTO oauth_authorization_codes(code_digest,product_id,provider_organisation_id,provider_revision,client_id,redirect_uri,resource,scopes,downstream_challenge,issuer,subject,email,display_name,customer_account_id,external_customer_id,installation_id,grants,access_evaluation_id,access_evaluated_at,policy_version,upstream_access_secret_id,access_expires_at,expires_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23 FROM identity_providers provider WHERE provider.product_id=$2 AND provider.state='active' AND provider.revision=$4 FOR UPDATE OF provider`, value.Digest, value.ProductID, value.OrganisationID, value.ProviderRevision, value.ClientID, value.RedirectURI, value.Resource, value.Scopes, value.DownstreamChallenge, value.Issuer, value.Subject, value.Email, value.DisplayName, value.CustomerAccountID, value.ExternalCustomerID, value.InstallationID, grants, value.AccessEvaluationID, value.AccessEvaluatedAt, value.PolicyVersion, value.UpstreamAccessSecretID, value.AccessExpiresAt, value.ExpiresAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func scanOAuthCode(row pgx.Row) (identity.OAuthCode, error) {
	var value identity.OAuthCode
	var grants []byte
	err := row.Scan(&value.Digest, &value.ProductID, &value.OrganisationID, &value.ProviderRevision, &value.ClientID, &value.RedirectURI, &value.Resource, &value.Scopes, &value.DownstreamChallenge, &value.Issuer, &value.Subject, &value.Email, &value.DisplayName, &value.CustomerAccountID, &value.ExternalCustomerID, &value.InstallationID, &grants, &value.AccessEvaluationID, &value.AccessEvaluatedAt, &value.PolicyVersion, &value.UpstreamAccessSecretID, &value.AccessExpiresAt, &value.ExpiresAt)
	if err == nil {
		err = json.Unmarshal(grants, &value.Grants)
	}
	return value, databaseError(err)
}

func (p *Postgres) ConsumeOAuthCode(ctx context.Context, digest []byte) (identity.OAuthCode, error) {
	return scanOAuthCode(p.pool.QueryRow(ctx, `DELETE FROM oauth_authorization_codes WHERE code_digest = $1 RETURNING code_digest,product_id::text,provider_organisation_id::text,provider_revision,client_id,redirect_uri,resource,scopes,downstream_challenge,issuer,subject,email,display_name,customer_account_id::text,external_customer_id,installation_id,grants,access_evaluation_id,access_evaluated_at,policy_version,upstream_access_secret_id::text,access_expires_at,expires_at`, digest))
}

func (p *Postgres) CreateAccessToken(ctx context.Context, value identity.AccessToken) error {
	grants, _ := json.Marshal(value.Grants)
	result, err := p.pool.Exec(ctx, `INSERT INTO oauth_access_tokens(token_digest,product_id,provider_revision,client_id,resource,issuer,subject,email,display_name,customer_account_id,external_customer_id,installation_id,grants,access_evaluation_id,access_evaluated_at,policy_version,upstream_access_secret_id,scopes,expires_at,created_at,revoked_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21 FROM identity_providers provider WHERE provider.product_id=$2 AND provider.state='active' AND provider.revision=$3 FOR UPDATE OF provider`, value.Digest, value.ProductID, value.ProviderRevision, value.ClientID, value.Resource, value.Issuer, value.Subject, value.Email, value.DisplayName, value.CustomerAccountID, value.ExternalCustomerID, value.InstallationID, grants, value.AccessEvaluationID, value.AccessEvaluatedAt, value.PolicyVersion, value.UpstreamAccessSecretID, value.Scopes, value.ExpiresAt, value.CreatedAt, value.RevokedAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func scanAccessToken(row pgx.Row) (identity.AccessToken, error) {
	var value identity.AccessToken
	var grants []byte
	err := row.Scan(&value.Digest, &value.ProductID, &value.ProviderRevision, &value.ClientID, &value.Resource, &value.Issuer, &value.Subject, &value.Email, &value.DisplayName, &value.CustomerAccountID, &value.ExternalCustomerID, &value.InstallationID, &grants, &value.AccessEvaluationID, &value.AccessEvaluatedAt, &value.PolicyVersion, &value.UpstreamAccessSecretID, &value.Scopes, &value.ExpiresAt, &value.CreatedAt, &value.RevokedAt)
	if err == nil {
		err = json.Unmarshal(grants, &value.Grants)
	}
	return value, databaseError(err)
}

func (p *Postgres) AccessTokenByDigest(ctx context.Context, digest []byte) (identity.AccessToken, error) {
	return scanAccessToken(p.pool.QueryRow(ctx, `SELECT token_digest,product_id::text,provider_revision,client_id,resource,issuer,subject,email,display_name,customer_account_id::text,external_customer_id,installation_id,grants,access_evaluation_id,access_evaluated_at,policy_version,upstream_access_secret_id::text,scopes,expires_at,created_at,revoked_at FROM oauth_access_tokens WHERE token_digest = $1`, digest))
}

func (p *Postgres) DeleteStaleOAuthArtifacts(ctx context.Context, productID string, now time.Time, limit int) (int64, error) {
	limit = boundedOAuthArtifactCleanupLimit(limit)
	if limit == 0 {
		return 0, nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stateResult, err := tx.Exec(ctx, `
		WITH doomed AS (
			SELECT state_digest
			FROM oauth_states
			WHERE product_id=$1
			  AND (expires_at <= $2 OR NOT EXISTS (
				SELECT 1 FROM identity_providers provider
				WHERE provider.product_id=$1 AND provider.state='active' AND provider.revision=oauth_states.provider_revision
			  ))
			ORDER BY expires_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM oauth_states state
		USING doomed
		WHERE state.state_digest = doomed.state_digest`, productID, now, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	deleted := stateResult.RowsAffected()
	secretIDs := make(map[string]bool)

	codeRows, err := tx.Query(ctx, `
		WITH doomed AS (
			SELECT code_digest
			FROM oauth_authorization_codes
			WHERE product_id=$1
			  AND (expires_at <= $2 OR access_expires_at <= $2 OR NOT EXISTS (
				SELECT 1 FROM identity_providers provider
				WHERE provider.product_id=$1 AND provider.state='active' AND provider.revision=oauth_authorization_codes.provider_revision
			  ))
			ORDER BY expires_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM oauth_authorization_codes code
		USING doomed
		WHERE code.code_digest = doomed.code_digest
		RETURNING code.upstream_access_secret_id::text`, productID, now, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	for codeRows.Next() {
		var secretID string
		if err := codeRows.Scan(&secretID); err != nil {
			codeRows.Close()
			return 0, databaseError(err)
		}
		secretIDs[secretID] = true
		deleted++
	}
	if err := codeRows.Err(); err != nil {
		codeRows.Close()
		return 0, databaseError(err)
	}
	codeRows.Close()

	tokenRows, err := tx.Query(ctx, `
		WITH doomed AS (
			SELECT token_digest
			FROM oauth_access_tokens
			WHERE product_id=$1
			  AND (expires_at <= $2 OR revoked_at IS NOT NULL OR NOT EXISTS (
				SELECT 1 FROM identity_providers provider
				WHERE provider.product_id=$1 AND provider.state='active' AND provider.revision=oauth_access_tokens.provider_revision
			  ))
			ORDER BY expires_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM oauth_access_tokens token
		USING doomed
		WHERE token.token_digest = doomed.token_digest
		RETURNING token.upstream_access_secret_id::text`, productID, now, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	for tokenRows.Next() {
		var secretID string
		if err := tokenRows.Scan(&secretID); err != nil {
			tokenRows.Close()
			return 0, databaseError(err)
		}
		secretIDs[secretID] = true
		deleted++
	}
	if err := tokenRows.Err(); err != nil {
		tokenRows.Close()
		return 0, databaseError(err)
	}
	tokenRows.Close()

	if len(secretIDs) > 0 {
		ids := make([]string, 0, len(secretIDs))
		for id := range secretIDs {
			ids = append(ids, id)
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM secrets secret
			WHERE secret.id = ANY($1::uuid[])
			  AND secret.purpose = 'vendor_delegated_access'
			  AND NOT EXISTS (
				SELECT 1 FROM oauth_authorization_codes code
				WHERE code.upstream_access_secret_id = secret.id
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM oauth_access_tokens token
				WHERE token.upstream_access_secret_id = secret.id
			  )`, ids); err != nil {
			return 0, databaseError(err)
		}
	}
	orphanResult, err := tx.Exec(ctx, `
		WITH doomed AS (
			SELECT secret.id
			FROM secrets secret
			WHERE secret.organisation_id = (SELECT organisation_id FROM products WHERE id=$1)
			  AND secret.created_at <= $2
			  AND (
				(secret.purpose = 'vendor_delegated_access'
				  AND NOT EXISTS (SELECT 1 FROM oauth_authorization_codes code WHERE code.upstream_access_secret_id = secret.id)
				  AND NOT EXISTS (SELECT 1 FROM oauth_access_tokens token WHERE token.upstream_access_secret_id = secret.id))
				OR
				(secret.purpose = 'identity_provider_oidc_client'
				  AND NOT EXISTS (SELECT 1 FROM identity_providers provider WHERE provider.client_secret_id = secret.id))
			  )
			ORDER BY secret.created_at, secret.id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM secrets secret
		USING doomed
		WHERE secret.id = doomed.id`, productID, now.Add(-identitySecretOrphanGrace), limit)
	if err != nil {
		return 0, databaseError(err)
	}
	deleted += orphanResult.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return 0, databaseError(err)
	}
	return deleted, nil
}
