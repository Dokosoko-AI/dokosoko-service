package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/docreview"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	secretvault "github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

var (
	ErrConfirmationRequired = errors.New("public access confirmation required")
	ErrUnsafeForPublic      = errors.New("resource is not safe for public access")
	ErrInvalidVisibility    = errors.New("invalid visibility")
	ErrToolDrifted          = errors.New("imported tool schema drift requires review")
	ErrSourceReviewRequired = errors.New("source publication requires a completed, reviewable crawl with fetched evidence")
)

type Actor struct {
	ID        string
	RequestID string
}

type Service struct {
	store                    store.Store
	vault                    *secretvault.Vault
	aiRuntime                airuntime.Runtime
	aiEnvironmentCredentials map[string]string
	now                      func() time.Time
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func New(storage store.Store) *Service {
	return &Service{store: storage, aiRuntime: newAIRuntime(nil), aiEnvironmentCredentials: make(map[string]string), now: func() time.Time { return time.Now().UTC() }}
}

func NewWithVault(storage store.Store, vault *secretvault.Vault) *Service {
	return &Service{store: storage, vault: vault, aiRuntime: newAIRuntime(nil), aiEnvironmentCredentials: make(map[string]string), now: func() time.Time { return time.Now().UTC() }}
}

func NewWithVaultAndProductBuilderDoer(storage store.Store, vault *secretvault.Vault, doer ProductBuilderDoer) *Service {
	return &Service{store: storage, vault: vault, aiRuntime: newAIRuntime(doer), aiEnvironmentCredentials: make(map[string]string), now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Store() store.Store { return s.store }

type IdentityInput struct {
	DeploymentID       string
	Issuer             string
	ClientID           string
	ClientSecret       string
	Scopes             []string
	Audience           string
	OAuthResource      string
	OrganisationClaim  string
	InstallationClaim  string
	DelegatedAPIOrigin string
	State              string
	Revision           int64
}

func validHTTPSOrigin(raw string) bool {
	parsed, err := url.Parse(raw)
	local := err == nil && identity.IsLocalDevelopmentHostname(parsed.Hostname())
	return err == nil && (parsed.Scheme == "https" && parsed.Port() == "" || parsed.Scheme == "http" && local) && parsed.Host != "" && parsed.User == nil && parsed.Fragment == "" && parsed.RawQuery == ""
}

func validHTTPSBaseOrigin(raw string) bool {
	parsed, err := url.Parse(raw)
	local := err == nil && identity.IsLocalDevelopmentHostname(parsed.Hostname())
	return err == nil && (parsed.Scheme == "https" && parsed.Port() == "" || parsed.Scheme == "http" && local) && parsed.Host != "" && parsed.User == nil && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

func validHTTPSURI(raw string) bool {
	parsed, err := url.Parse(raw)
	local := err == nil && identity.IsLocalDevelopmentHostname(parsed.Hostname())
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http" && local) && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

func validOAuthRedirect(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	return parsed.Scheme == "https" || (parsed.Scheme == "http" && identity.IsLocalDevelopmentHostname(parsed.Hostname()))
}

func validToolEndpoint(raw string) bool {
	parsed, err := url.Parse(raw)
	local := err == nil && identity.IsLocalDevelopmentHostname(parsed.Hostname())
	return err == nil && (parsed.Scheme == "https" && parsed.Port() == "" || parsed.Scheme == "http" && local) && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

func (s *Service) ConfigureIdentity(ctx context.Context, input IdentityInput, actor Actor) (identity.ProviderConfig, error) {
	input.DeploymentID = strings.TrimSpace(input.DeploymentID)
	input.Issuer, input.ClientID, input.ClientSecret = strings.TrimRight(strings.TrimSpace(input.Issuer), "/"), strings.TrimSpace(input.ClientID), strings.TrimSpace(input.ClientSecret)
	input.Audience, input.OAuthResource = strings.TrimSpace(input.Audience), strings.TrimSpace(input.OAuthResource)
	input.OrganisationClaim, input.InstallationClaim = strings.TrimSpace(input.OrganisationClaim), strings.TrimSpace(input.InstallationClaim)
	input.DelegatedAPIOrigin = strings.TrimRight(strings.TrimSpace(input.DelegatedAPIOrigin), "/")
	input.State = strings.ToLower(strings.TrimSpace(input.State))
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return identity.ProviderConfig{}, err
	}
	if input.DeploymentID != deployment.ID || input.ClientID == "" || !validHTTPSOrigin(input.Issuer) {
		return identity.ProviderConfig{}, errors.New("deployment, HTTPS OIDC issuer, and client ID are required")
	}
	if !validHTTPSBaseOrigin(input.DelegatedAPIOrigin) || (input.OAuthResource != "" && !validHTTPSURI(input.OAuthResource)) {
		return identity.ProviderConfig{}, errors.New("delegated API origin and OAuth resource must be credential-free HTTPS URLs")
	}
	if input.State == "" {
		input.State = "active"
	}
	if input.State != "active" && input.State != "disabled" {
		return identity.ProviderConfig{}, errors.New("identity provider state must be active or disabled")
	}
	if input.OrganisationClaim == "" {
		input.OrganisationClaim = "org_id"
	}
	if len(input.Scopes) == 0 {
		input.Scopes = []string{"openid", "profile", "email"}
	}
	seenScopes := map[string]bool{}
	scopes := make([]string, 0, len(input.Scopes)+1)
	for _, scope := range input.Scopes {
		scope = strings.TrimSpace(scope)
		if scope != "" && !seenScopes[scope] {
			seenScopes[scope] = true
			scopes = append(scopes, scope)
		}
	}
	if !seenScopes["openid"] {
		scopes = append([]string{"openid"}, scopes...)
	}
	current, currentErr := s.store.IdentityProvider(ctx, input.DeploymentID)
	if currentErr != nil && !errors.Is(currentErr, store.ErrNotFound) {
		return identity.ProviderConfig{}, currentErr
	}
	config := identity.ProviderConfig{OrganisationID: deployment.OrganisationID, DeploymentID: input.DeploymentID, Issuer: input.Issuer, ClientID: input.ClientID, Scopes: scopes, Audience: input.Audience, OAuthResource: input.OAuthResource, OrganisationClaim: input.OrganisationClaim, InstallationClaim: input.InstallationClaim, DelegatedAPIOrigin: input.DelegatedAPIOrigin, State: input.State, Revision: input.Revision}
	if currentErr == nil {
		if input.Revision != current.Revision {
			return identity.ProviderConfig{}, store.ErrConflict
		}
		config.ID, config.ClientSecretID = current.ID, current.ClientSecretID
		if (current.Issuer != config.Issuer || current.ClientID != config.ClientID) && input.ClientSecret == "" {
			return identity.ProviderConfig{}, errors.New("changing the OIDC issuer or client ID requires a new client secret")
		}
	} else {
		if input.Revision != 0 {
			return identity.ProviderConfig{}, store.ErrConflict
		}
		config.ID, err = randomUUID()
		if err != nil {
			return identity.ProviderConfig{}, err
		}
	}
	if input.ClientSecret != "" {
		if s.vault == nil {
			return identity.ProviderConfig{}, errors.New("identity credential encryption is not configured")
		}
		secretID, err := randomUUID()
		if err != nil {
			return identity.ProviderConfig{}, err
		}
		encrypted, err := s.vault.Encrypt([]byte(input.ClientSecret), deployment.OrganisationID+":idp:"+secretID)
		if err != nil {
			return identity.ProviderConfig{}, err
		}
		if _, err := s.store.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: deployment.OrganisationID, Name: "identity-provider-oidc-" + input.DeploymentID + "-" + secretID, Purpose: "identity_provider_oidc_client", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint}); err != nil {
			return identity.ProviderConfig{}, err
		}
		config.ClientSecretID = secretID
	}
	if config.ClientSecretID == "" {
		return identity.ProviderConfig{}, errors.New("OIDC client secret is required for initial configuration")
	}
	updated, err := s.store.SaveIdentityProvider(ctx, config)
	if err != nil {
		return identity.ProviderConfig{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: input.DeploymentID, ActorID: actor.ID, Action: "identity_provider.configured", TargetType: "identity_provider", TargetID: updated.ID, Current: map[string]any{"issuer": updated.Issuer, "scopes": updated.Scopes, "oauth_resource": updated.OAuthResource, "delegated_api_origin": updated.DelegatedAPIOrigin, "state": updated.State, "credential_rotated": input.ClientSecret != ""}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func (s *Service) UpdateCustomerAccountState(ctx context.Context, productID, accountID, state string, revision int64, actor Actor) (identity.CustomerAccount, error) {
	state = strings.ToLower(strings.TrimSpace(state))
	if state != "active" && state != "suspended" {
		return identity.CustomerAccount{}, errors.New("customer account state must be active or suspended")
	}
	current, err := s.store.CustomerAccount(ctx, productID, accountID)
	if err != nil {
		return identity.CustomerAccount{}, err
	}
	if current.State == state {
		if current.Revision != revision {
			return identity.CustomerAccount{}, store.ErrConflict
		}
		return current, nil
	}
	prior := current.State
	current.State = state
	updated, err := s.store.UpdateCustomerAccount(ctx, current, revision)
	if err != nil {
		return identity.CustomerAccount{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "customer_account.state.changed", TargetType: "customer_account", TargetID: updated.ID, Prior: map[string]any{"state": prior}, Current: map[string]any{"state": state}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func randomID(prefix string) string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buffer)
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

func validateNameSlug(name, slug string) error {
	if strings.TrimSpace(name) == "" || len(strings.TrimSpace(name)) > 120 {
		return errors.New("name must be between 1 and 120 characters")
	}
	if !slugPattern.MatchString(slug) || len(slug) > 63 {
		return errors.New("slug must use lower-case letters, numbers, and single hyphens")
	}
	return nil
}

func (s *Service) CreateOrganisation(ctx context.Context, name, slug string, actor Actor) (model.Organisation, error) {
	name, slug = strings.TrimSpace(name), strings.TrimSpace(slug)
	if err := validateNameSlug(name, slug); err != nil {
		return model.Organisation{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.Organisation{}, err
	}
	value, err := s.store.CreateOrganisation(ctx, model.Organisation{ID: id, Name: name, Slug: slug})
	if err != nil {
		return model.Organisation{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: value.ID, ActorID: actor.ID, Action: "organisation.created", TargetType: "organisation", TargetID: value.ID, Current: map[string]any{"name": value.Name, "slug": value.Slug}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func (s *Service) CreateProduct(ctx context.Context, organisationID, name, slug string, actor Actor) (model.Product, error) {
	name, slug = strings.TrimSpace(name), strings.TrimSpace(slug)
	if err := validateNameSlug(name, slug); err != nil {
		return model.Product{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.Product{}, err
	}
	value, err := s.store.CreateProduct(ctx, model.Product{ID: id, OrganisationID: organisationID, Name: name, Slug: slug, DefaultVersionPolicy: "latest"})
	if err != nil {
		return model.Product{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: organisationID, ProductID: value.ID, ActorID: actor.ID, Action: "product.created", TargetType: "product", TargetID: value.ID, Current: map[string]any{"name": value.Name, "slug": value.Slug, "public_mcp_enabled": false}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func (s *Service) CreateEnvironment(ctx context.Context, organisationID, productID, name, slug string, production bool, actor Actor) (model.Environment, error) {
	name, slug = strings.TrimSpace(name), strings.TrimSpace(slug)
	if err := validateNameSlug(name, slug); err != nil {
		return model.Environment{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.Environment{}, err
	}
	value, err := s.store.CreateEnvironment(ctx, model.Environment{ID: id, OrganisationID: organisationID, ProductID: productID, Name: name, Slug: slug, IsProduction: production})
	if err != nil {
		return model.Environment{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: organisationID, ProductID: productID, ActorID: actor.ID, Action: "environment.created", TargetType: "environment", TargetID: value.ID, Current: map[string]any{"name": value.Name, "slug": value.Slug, "is_production": value.IsProduction}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func (s *Service) CreateSource(ctx context.Context, organisationID, productID, name, kind, location string, actor Actor) (model.Source, error) {
	name, kind, location = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(kind)), strings.TrimSpace(location)
	if name == "" || len(name) > 120 || location == "" || len(location) > 2048 {
		return model.Source{}, errors.New("source name and location are required")
	}
	allowedKinds := map[string]bool{"website": true, "openapi": true, "git": true, "upload": true}
	if !allowedKinds[kind] {
		return model.Source{}, errors.New("unsupported source kind")
	}
	if kind == "website" || kind == "openapi" {
		parsed, err := url.Parse(location)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return model.Source{}, errors.New("web source must use an absolute http(s) URL without embedded credentials")
		}
	}
	id, err := randomUUID()
	if err != nil {
		return model.Source{}, err
	}
	value, err := s.store.CreateSource(ctx, model.Source{ID: id, OrganisationID: organisationID, ProductID: productID, Name: name, Kind: kind, Location: location, Visibility: model.VisibilityPrivate})
	if err != nil {
		return model.Source{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: organisationID, ProductID: productID, ActorID: actor.ID, Action: "source.created", TargetType: "source", TargetID: value.ID, Current: map[string]any{"name": value.Name, "kind": value.Kind, "visibility": model.VisibilityPrivate}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func (s *Service) QueueCrawl(ctx context.Context, productID, sourceID string, actor Actor) (model.CrawlJob, error) {
	source, err := s.store.Source(ctx, productID, sourceID)
	if err != nil {
		return model.CrawlJob{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.CrawlJob{}, err
	}
	job, err := s.store.CreateCrawlJob(ctx, model.CrawlJob{ID: id, OrganisationID: source.OrganisationID, ProductID: productID, SourceID: sourceID, State: "queued"})
	if err != nil {
		return model.CrawlJob{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: source.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "source.crawl.queued", TargetType: "crawl_job", TargetID: job.ID, Current: map[string]any{"source_id": sourceID, "state": "queued"}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return job, err
}

type ToolInput struct {
	OrganisationID      string
	ProductID           string
	Namespace           string
	Name                string
	Description         string
	InputSchema         json.RawMessage
	OutputSchema        json.RawMessage
	Endpoint            string
	HTTPMethod          string
	AuthorizationPolicy json.RawMessage
	TimeoutMS           int
}

type ProviderInput struct {
	OrganisationID string
	ProductID      string
	Name           string
	BaseURL        string
	Credential     string
	RequiredGrants []string
	MaxTTLSeconds  int
}

func (s *Service) CreateProvider(ctx context.Context, input ProviderInput, actor Actor) (model.Provider, error) {
	input.Name, input.BaseURL, input.Credential = strings.TrimSpace(input.Name), strings.TrimRight(strings.TrimSpace(input.BaseURL), "/"), strings.TrimSpace(input.Credential)
	if input.Name == "" || len(input.Name) > 120 || !validHTTPSOrigin(input.BaseURL) || input.Credential == "" || s.vault == nil {
		return model.Provider{}, errors.New("provider name, fixed HTTPS base URL, and encrypted API credential are required")
	}
	if input.MaxTTLSeconds == 0 {
		input.MaxTTLSeconds = 3600
	}
	if input.MaxTTLSeconds < 300 || input.MaxTTLSeconds > 86400 {
		return model.Provider{}, errors.New("provider maximum TTL must be between 300 and 86400 seconds")
	}
	providerID, err := randomUUID()
	if err != nil {
		return model.Provider{}, err
	}
	secretID, err := randomUUID()
	if err != nil {
		return model.Provider{}, err
	}
	encrypted, err := s.vault.Encrypt([]byte(input.Credential), input.OrganisationID+":provider:"+secretID)
	if err != nil {
		return model.Provider{}, err
	}
	if _, err := s.store.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: input.OrganisationID, Name: "provider-" + providerID, Purpose: "provider_api", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint}); err != nil {
		return model.Provider{}, err
	}
	grants := make([]string, 0, len(input.RequiredGrants))
	seen := map[string]bool{}
	for _, grant := range input.RequiredGrants {
		grant = strings.TrimSpace(grant)
		if grant != "" && !seen[grant] {
			seen[grant] = true
			grants = append(grants, grant)
		}
	}
	config, _ := json.Marshal(map[string]any{"contract_version": "2026-08-01", "authorize_path": "/v1/authorize", "project_path": "/v1/projects", "credential_path": "/v1/credentials", "revoke_path": "/v1/credentials/{credential_id}/revoke", "required_grants": grants, "max_ttl_seconds": input.MaxTTLSeconds})
	value, err := s.store.CreateProvider(ctx, model.Provider{ID: providerID, OrganisationID: input.OrganisationID, ProductID: input.ProductID, Name: input.Name, Kind: "remote", BaseURL: input.BaseURL, CredentialID: secretID, Config: config})
	if err != nil {
		return model.Provider{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: input.OrganisationID, ProductID: input.ProductID, ActorID: actor.ID, Action: "provider.created", TargetType: "provider", TargetID: value.ID, Current: map[string]any{"name": value.Name, "kind": value.Kind, "contract_version": "2026-08-01", "credential_stored": true}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, nil
}

type LLMProfileInput struct {
	OrganisationID      string
	ProductID           string
	Role                string
	Provider            string
	Endpoint            string
	Model               string
	Credential          string
	EmbeddingDimensions int
	MaxInputTokens      int
	MaxOutputTokens     int
	DailyTokenBudget    int64
	Enabled             bool
}

func (s *Service) SaveLLMProfile(ctx context.Context, input LLMProfileInput, actor Actor) (model.LLMProfile, error) {
	input.Role, input.Provider = strings.ToLower(strings.TrimSpace(input.Role)), strings.ToLower(strings.TrimSpace(input.Provider))
	input.Endpoint, input.Model, input.Credential = strings.TrimSpace(input.Endpoint), strings.TrimSpace(input.Model), strings.TrimSpace(input.Credential)
	roles := map[string]bool{"embedding": true, "extraction": true, "reranking": true, "evaluation": true, "assistant": true}
	if !roles[input.Role] || input.Provider == "" || input.Model == "" || !validHTTPSBaseOrigin(input.Endpoint) {
		return model.LLMProfile{}, errors.New("LLM role, provider, model, and fixed HTTPS endpoint are required")
	}
	if input.Role == "embedding" && (input.EmbeddingDimensions < 64 || input.EmbeddingDimensions > 8192) {
		return model.LLMProfile{}, errors.New("embedding dimensions must be between 64 and 8192")
	}
	if input.Role != "embedding" {
		input.EmbeddingDimensions = 0
	}
	if input.MaxInputTokens < 256 || input.MaxInputTokens > 1_000_000 || input.MaxOutputTokens < 1 || input.MaxOutputTokens > 32_768 || input.DailyTokenBudget < 0 || input.DailyTokenBudget > 10_000_000_000 {
		return model.LLMProfile{}, errors.New("LLM token limits or daily budget are outside supported bounds")
	}
	profiles, _ := s.store.LLMProfiles(ctx, input.ProductID)
	var current model.LLMProfile
	for _, profile := range profiles {
		if profile.Role == input.Role {
			current = profile
			break
		}
	}
	if current.ID != "" && current.Provider != input.Provider && input.Credential == "" {
		return model.LLMProfile{}, errors.New("changing the AI provider requires a new credential")
	}
	profileID, credentialID := current.ID, current.CredentialID
	var err error
	if profileID == "" {
		profileID, err = randomUUID()
		if err != nil {
			return model.LLMProfile{}, err
		}
	}
	if input.Credential != "" {
		if s.vault == nil {
			return model.LLMProfile{}, errors.New("LLM credential encryption is not configured")
		}
		credentialID, err = randomUUID()
		if err != nil {
			return model.LLMProfile{}, err
		}
		encrypted, err := s.vault.Encrypt([]byte(input.Credential), input.OrganisationID+":llm:"+credentialID)
		if err != nil {
			return model.LLMProfile{}, err
		}
		if _, err := s.store.CreateSecret(ctx, model.Secret{ID: credentialID, OrganisationID: input.OrganisationID, Name: "llm-" + input.Role + "-" + credentialID, Purpose: "llm_provider", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint}); err != nil {
			return model.LLMProfile{}, err
		}
	}
	if input.Enabled && credentialID == "" {
		return model.LLMProfile{}, errors.New("an encrypted provider credential is required before enabling an LLM profile")
	}
	hardening := json.RawMessage(`{"context_is_untrusted":true,"tool_calls_disabled":true,"authorization_disabled":true,"require_citations":true,"no_answer_on_low_confidence":true}`)
	value, err := s.store.SaveLLMProfile(ctx, model.LLMProfile{ID: profileID, OrganisationID: input.OrganisationID, ProductID: input.ProductID, Role: input.Role, Provider: input.Provider, Endpoint: input.Endpoint, Model: input.Model, CredentialID: credentialID, EmbeddingDimensions: input.EmbeddingDimensions, MaxInputTokens: input.MaxInputTokens, MaxOutputTokens: input.MaxOutputTokens, DailyTokenBudget: input.DailyTokenBudget, Hardening: hardening, Enabled: input.Enabled})
	if err != nil {
		return model.LLMProfile{}, err
	}
	workloads := map[string]string{"extraction": "analysis", "evaluation": "analysis", "assistant": "assistant"}
	if workload := workloads[input.Role]; workload != "" {
		connections, connectionErr := s.store.AIProviderConnections(ctx, input.ProductID)
		if connectionErr != nil && !errors.Is(connectionErr, store.ErrNotFound) {
			return model.LLMProfile{}, connectionErr
		}
		var connection model.AIProviderConnection
		for _, candidate := range connections {
			if candidate.Provider == input.Provider {
				connection = candidate
				break
			}
		}
		connectionRevision := connection.Revision
		if connection.ID == "" {
			connection.ID, err = randomUUID()
			if err != nil {
				return model.LLMProfile{}, err
			}
		}
		if input.Credential == "" && connection.CredentialID != "" {
			credentialID = connection.CredentialID
		}
		connection, err = s.store.SaveAIProviderConnection(ctx, model.AIProviderConnection{ID: connection.ID, OrganisationID: input.OrganisationID, DeploymentID: input.ProductID, Provider: input.Provider, Endpoint: input.Endpoint, CredentialID: credentialID, ManagedBy: "console", Enabled: true, BackupModels: json.RawMessage(`{}`), LastTestedAt: connection.LastTestedAt, LastErrorCode: connection.LastErrorCode}, connectionRevision)
		if err != nil {
			return model.LLMProfile{}, err
		}
		currentAIProfile, profileErr := s.store.AIWorkloadProfile(ctx, input.ProductID, workload)
		if profileErr != nil && !errors.Is(profileErr, store.ErrNotFound) {
			return model.LLMProfile{}, profileErr
		}
		aiProfileID, aiProfileRevision := currentAIProfile.ID, currentAIProfile.Revision
		if aiProfileID == "" {
			aiProfileID, err = randomUUID()
			if err != nil {
				return model.LLMProfile{}, err
			}
		}
		if _, err = s.store.SaveAIWorkloadProfile(ctx, model.AIWorkloadProfile{ID: aiProfileID, OrganisationID: input.OrganisationID, ProductID: input.ProductID, Workload: workload, ProviderConnectionID: connection.ID, Model: input.Model, MaxInputTokens: input.MaxInputTokens, MaxOutputTokens: input.MaxOutputTokens, DailyTokenBudget: input.DailyTokenBudget, Hardening: hardening, Enabled: input.Enabled}, aiProfileRevision); err != nil {
			return model.LLMProfile{}, err
		}
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: input.OrganisationID, ProductID: input.ProductID, ActorID: actor.ID, Action: "llm.profile.saved", TargetType: "llm_profile", TargetID: value.ID, Current: map[string]any{"role": value.Role, "provider": value.Provider, "model": value.Model, "enabled": value.Enabled, "credential_rotated": input.Credential != "", "hardening": map[string]bool{"context_is_untrusted": true, "tool_calls_disabled": true, "authorization_disabled": true, "require_citations": true}}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, nil
}

func (s *Service) CreateTool(ctx context.Context, input ToolInput, actor Actor) (model.Tool, error) {
	input.Namespace, input.Name = strings.ToLower(strings.TrimSpace(input.Namespace)), strings.ToLower(strings.TrimSpace(input.Name))
	input.Description, input.HTTPMethod, input.Endpoint = strings.TrimSpace(input.Description), strings.ToUpper(strings.TrimSpace(input.HTTPMethod)), strings.TrimSpace(input.Endpoint)
	if !toolNamePattern.MatchString(input.Namespace) || !toolNamePattern.MatchString(input.Name) || input.Description == "" || len(input.Description) > 500 {
		return model.Tool{}, errors.New("tool namespace, name, and description are invalid")
	}
	if err := toolruntime.ValidateSchema(input.InputSchema); err != nil {
		return model.Tool{}, err
	}
	if len(input.OutputSchema) == 0 {
		input.OutputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`)
	}
	if err := toolruntime.ValidateSchema(input.OutputSchema); err != nil {
		return model.Tool{}, fmt.Errorf("output schema: %w", err)
	}
	methods := map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
	parsed, err := url.Parse(input.Endpoint)
	if err != nil || !validToolEndpoint(input.Endpoint) || !methods[input.HTTPMethod] {
		return model.Tool{}, errors.New("tool endpoint must be a fixed credential-free HTTPS URL on the default port and use an allowed HTTP method")
	}
	provider, err := s.store.IdentityProvider(ctx, input.ProductID)
	if err != nil || provider.State != "active" || provider.DelegatedAPIOrigin == "" {
		return model.Tool{}, errors.New("configure an active identity provider delegated API origin before creating an HTTP tool")
	}
	vendorOrigin, err := url.Parse(provider.DelegatedAPIOrigin)
	if err != nil || !strings.EqualFold(parsed.Scheme, vendorOrigin.Scheme) || !strings.EqualFold(parsed.Host, vendorOrigin.Host) {
		return model.Tool{}, errors.New("tool endpoint must use the configured vendor API origin")
	}
	if input.TimeoutMS == 0 {
		input.TimeoutMS = 10_000
	}
	if input.TimeoutMS < 100 || input.TimeoutMS > 60_000 {
		return model.Tool{}, errors.New("tool timeout must be between 100 and 60000 milliseconds")
	}
	policy, _, err := normalizeToolPolicy(input.AuthorizationPolicy, input.HTTPMethod)
	if err != nil {
		return model.Tool{}, err
	}
	input.AuthorizationPolicy = policy
	toolID, err := randomUUID()
	if err != nil {
		return model.Tool{}, err
	}
	connectionID, err := randomUUID()
	if err != nil {
		return model.Tool{}, err
	}
	value, err := s.store.CreateTool(ctx, model.Tool{ID: toolID, OrganisationID: input.OrganisationID, ProductID: input.ProductID, Namespace: input.Namespace, Name: input.Name, Description: input.Description, InputSchema: input.InputSchema, OutputSchema: input.OutputSchema, APIConnectionID: connectionID, BaseURL: input.Endpoint, HTTPMethod: input.HTTPMethod, AuthorizationPolicy: input.AuthorizationPolicy, TimeoutMS: input.TimeoutMS, BackendKind: "http", UpstreamAnnotations: json.RawMessage(`{}`)})
	if err != nil {
		return model.Tool{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: input.OrganisationID, ProductID: input.ProductID, ActorID: actor.ID, Action: "tool.created", TargetType: "tool", TargetID: toolID, Current: map[string]any{"name": input.Namespace + "." + input.Name, "method": input.HTTPMethod, "authentication": "delegated_vendor_oauth", "state": "draft"}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func (s *Service) PublishTool(ctx context.Context, productID, toolID string, revision int64, actor Actor) (model.Tool, error) {
	current, err := s.store.Tool(ctx, productID, toolID)
	if err != nil {
		return model.Tool{}, err
	}
	if current.BackendKind == "mcp" && current.UpstreamDrifted {
		return model.Tool{}, ErrToolDrifted
	}
	if err := s.validateToolGrantRegistry(ctx, productID, current); err != nil {
		return model.Tool{}, err
	}
	updated, err := s.store.PublishTool(ctx, productID, toolID, revision, actor.ID)
	if err != nil {
		return model.Tool{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "tool.published", TargetType: "tool", TargetID: toolID, Prior: map[string]any{"state": current.State}, Current: map[string]any{"state": "published", "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, err
}

func (s *Service) SetPublicMCP(ctx context.Context, productID string, enabled, acknowledged bool, expectedRevision int64, actor Actor) (model.Product, error) {
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.Product{}, err
	}
	if product.PublicMCPEnabled == enabled {
		return product, nil
	}
	if enabled && !acknowledged {
		return model.Product{}, ErrConfirmationRequired
	}

	prior := product.PublicMCPEnabled
	product.PublicMCPEnabled = enabled
	updated, err := s.store.UpdateProduct(ctx, product, expectedRevision)
	if err != nil {
		return model.Product{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{
		ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.ID,
		ActorID: actor.ID, Action: "product.public_mcp.changed", TargetType: "product", TargetID: updated.ID,
		Prior: map[string]any{"public_mcp_enabled": prior}, Current: map[string]any{"public_mcp_enabled": enabled},
		RequestID: actor.RequestID, CreatedAt: s.now(),
	}); err != nil {
		return model.Product{}, err
	}
	return updated, nil
}

func (s *Service) SetSourceVisibility(ctx context.Context, productID, sourceID string, visibility model.Visibility, acknowledged bool, expectedRevision int64, actor Actor) (model.Source, error) {
	if !visibility.Valid() {
		return model.Source{}, ErrInvalidVisibility
	}
	source, err := s.store.Source(ctx, productID, sourceID)
	if err != nil {
		return model.Source{}, err
	}
	if source.Visibility == visibility {
		return source, nil
	}
	if visibility == model.VisibilityPublic {
		if !acknowledged {
			return model.Source{}, ErrConfirmationRequired
		}
		if source.Quarantined {
			return model.Source{}, fmt.Errorf("%w: quarantined sources cannot be public", ErrUnsafeForPublic)
		}
	}

	prior := source.Visibility
	source.Visibility = visibility
	updated, err := s.store.UpdateSource(ctx, source, expectedRevision)
	if err != nil {
		return model.Source{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{
		ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.ProductID,
		ActorID: actor.ID, Action: "source.visibility.changed", TargetType: "source", TargetID: updated.ID,
		Prior: map[string]any{"visibility": prior}, Current: map[string]any{"visibility": visibility},
		RequestID: actor.RequestID, CreatedAt: s.now(),
	}); err != nil {
		return model.Source{}, err
	}
	return updated, nil
}

type SourcePublicationInput struct {
	Revision            int64
	CrawlJobID          string
	DocumentIDs         []string
	AcknowledgeReviewed bool
}

func (s *Service) SourceReview(ctx context.Context, productID, sourceID, crawlJobID string) (model.SourceReview, error) {
	review, err := s.store.SourceReview(ctx, productID, sourceID, strings.TrimSpace(crawlJobID))
	if err != nil {
		return model.SourceReview{}, err
	}
	if review.CrawlJob.ProductID != productID || review.CrawlJob.SourceID != sourceID {
		return model.SourceReview{}, store.ErrNotFound
	}
	return review, nil
}

func (s *Service) PublishSource(ctx context.Context, productID, sourceID string, input SourcePublicationInput, actor Actor) (model.Source, model.SourcePublication, error) {
	current, err := s.store.Source(ctx, productID, sourceID)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	if current.Quarantined {
		return model.Source{}, model.SourcePublication{}, fmt.Errorf("%w: quarantined sources require remediation and a clean crawl", ErrUnsafeForPublic)
	}
	if !input.AcknowledgeReviewed {
		return model.Source{}, model.SourcePublication{}, ErrSourceReviewRequired
	}
	crawls, err := s.store.CrawlJobs(ctx, productID, sourceID)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	input.CrawlJobID = strings.TrimSpace(input.CrawlJobID)
	latestReviewable := len(crawls) > 0 && crawls[0].ID == input.CrawlJobID && crawls[0].FinishedAt != nil && (crawls[0].State == "review" || crawls[0].State == "succeeded")
	if !latestReviewable || crawls[0].FetchedCount == 0 || input.Revision != current.Revision {
		return model.Source{}, model.SourcePublication{}, ErrSourceReviewRequired
	}
	review, err := s.store.SourceReview(ctx, productID, sourceID, input.CrawlJobID)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	if review.Publication != nil || len(review.Documents) == 0 {
		return model.Source{}, model.SourcePublication{}, ErrSourceReviewRequired
	}
	documents := make(map[string]model.CrawlReviewDocument, len(review.Documents))
	for _, document := range review.Documents {
		documents[document.ID] = document
	}
	selected := make([]model.CrawlReviewDocument, 0, len(input.DocumentIDs))
	selectedIDs := make([]string, 0, len(input.DocumentIDs))
	seen := make(map[string]bool, len(input.DocumentIDs))
	for _, documentID := range input.DocumentIDs {
		documentID = strings.TrimSpace(documentID)
		document, ok := documents[documentID]
		if documentID == "" || seen[documentID] || !ok || !docreview.SafeAssessment(document.State, document.InjectionIndicators) {
			return model.Source{}, model.SourcePublication{}, ErrSourceReviewRequired
		}
		seen[documentID] = true
		selectedIDs = append(selectedIDs, documentID)
		selected = append(selected, document)
	}
	if len(selected) == 0 {
		return model.Source{}, model.SourcePublication{}, ErrSourceReviewRequired
	}
	publicationHash, err := docreview.PublicationContentHash(selected)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	now := s.now()
	publication := model.SourcePublication{ID: id, OrganisationID: current.OrganisationID, ProductID: productID, SourceID: sourceID, CrawlJobID: input.CrawlJobID, Visibility: current.Visibility, ContentHash: publicationHash, DocumentCount: len(selectedIDs), ReviewedBy: actor.ID, ReviewedAt: now, PublishedAt: now}
	updated, publication, err := s.store.PublishSource(ctx, productID, sourceID, input.Revision, publication, selectedIDs)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "source.publication.created", TargetType: "source_publication", TargetID: publication.ID, Prior: map[string]any{"source_revision": current.Revision}, Current: map[string]any{"source_id": sourceID, "source_revision": updated.Revision, "crawl_job_id": publication.CrawlJobID, "publication_revision": publication.Revision, "content_hash": publication.ContentHash, "document_count": publication.DocumentCount, "visibility": updated.Visibility}, RequestID: actor.RequestID, CreatedAt: now})
	return updated, publication, err
}

func (s *Service) StartIntegrationRun(ctx context.Context, productID, environmentID, requestedOutcome string, actor Actor) (model.IntegrationRun, error) {
	requestedOutcome = strings.TrimSpace(requestedOutcome)
	if requestedOutcome == "" || len(requestedOutcome) > 500 {
		return model.IntegrationRun{}, errors.New("requested outcome must be between 1 and 500 characters")
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.IntegrationRun{}, err
	}
	environments, err := s.store.Environments(ctx, productID)
	if err != nil {
		return model.IntegrationRun{}, err
	}
	validEnvironment := false
	for _, environment := range environments {
		if environment.ID == environmentID && environment.OrganisationID == product.OrganisationID {
			validEnvironment = true
			break
		}
	}
	if !validEnvironment {
		return model.IntegrationRun{}, errors.New("environment does not belong to the product")
	}
	id, err := randomUUID()
	if err != nil {
		return model.IntegrationRun{}, err
	}
	actorPseudonym := pseudonymActor(productID, actor.ID)
	if actorPseudonym == "" {
		return model.IntegrationRun{}, errors.New("an authenticated run owner is required")
	}
	value, err := s.store.CreateIntegrationRun(ctx, model.IntegrationRun{ID: id, OrganisationID: product.OrganisationID, ProductID: productID, EnvironmentID: environmentID, ActorPseudonym: actorPseudonym, RequestedOutcome: requestedOutcome, State: "running", StartedAt: s.now()})
	if err != nil {
		return model.IntegrationRun{}, err
	}
	_ = s.store.AppendAnalytics(ctx, model.AnalyticsEvent{OrganisationID: product.OrganisationID, ProductID: productID, EventName: "run_started", ActorKind: "vendor_user", ActorPseudonym: actorPseudonym, IntegrationRunID: value.ID, CreatedAt: s.now()})
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: product.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "integration_run.started", TargetType: "integration_run", TargetID: value.ID, Current: map[string]any{"environment_id": environmentID, "state": value.State}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func (s *Service) CompleteIntegrationRun(ctx context.Context, productID, runID string, reportedSuccess, validatedSuccess *bool, failureCode string, actor Actor) (model.IntegrationRun, error) {
	if validatedSuccess == nil {
		return model.IntegrationRun{}, errors.New("a deterministic validation result is required")
	}
	failureCode = strings.TrimSpace(failureCode)
	if !*validatedSuccess && (failureCode == "" || len(failureCode) > 120) {
		return model.IntegrationRun{}, errors.New("a failure code is required when validation fails")
	}
	if *validatedSuccess {
		failureCode = ""
	}
	current, err := s.store.IntegrationRun(ctx, productID, runID)
	if err != nil {
		return model.IntegrationRun{}, err
	}
	actorPseudonym := pseudonymActor(productID, actor.ID)
	if actorPseudonym == "" || current.ActorPseudonym != actorPseudonym {
		return model.IntegrationRun{}, errors.New("integration run is not owned by this principal")
	}
	value, err := s.store.CompleteIntegrationRun(ctx, productID, runID, reportedSuccess, validatedSuccess, failureCode, s.now())
	if err != nil {
		return model.IntegrationRun{}, err
	}
	_ = s.store.AppendAnalytics(ctx, model.AnalyticsEvent{OrganisationID: value.OrganisationID, ProductID: productID, EventName: "implementation_validated", ActorKind: "vendor_user", ActorPseudonym: actorPseudonym, IntegrationRunID: value.ID, Dimensions: map[string]any{"success": *validatedSuccess}, CreatedAt: s.now()})
	if reportedSuccess != nil {
		_ = s.store.AppendAnalytics(ctx, model.AnalyticsEvent{OrganisationID: value.OrganisationID, ProductID: productID, EventName: "success_reported", ActorKind: "vendor_user", ActorPseudonym: actorPseudonym, IntegrationRunID: value.ID, Dimensions: map[string]any{"success": *reportedSuccess}, CreatedAt: s.now()})
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: value.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "integration_run.completed", TargetType: "integration_run", TargetID: runID, Prior: map[string]any{"state": current.State}, Current: map[string]any{"state": value.State, "reported_success": reportedSuccess, "validated_success": validatedSuccess, "failure_code": failureCode}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func pseudonymActor(productID, actorID string) string {
	if actorID == "" || actorID == "anonymous" {
		return ""
	}
	digest := sha256.Sum256([]byte(productID + "\x00" + actorID))
	return hex.EncodeToString(digest[:16])
}
