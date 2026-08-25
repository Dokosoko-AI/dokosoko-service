package providers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/netpolicy"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
)

var (
	ErrDenied            = errors.New("provider operation denied")
	ErrInvalidRequest    = errors.New("invalid provider operation request")
	ErrUnsafeDestination = errors.New("provider destination is unsafe")
)

type Store interface {
	Providers(context.Context, string) ([]model.Provider, error)
	Provider(context.Context, string, string) (model.Provider, error)
	Projects(context.Context, string) ([]model.Project, error)
	Project(context.Context, string, string) (model.Project, error)
	CreateProject(context.Context, model.Project) (model.Project, error)
	CredentialLeases(context.Context, string) ([]model.CredentialLease, error)
	CredentialLease(context.Context, string, string) (model.CredentialLease, error)
	CreateCredentialLease(context.Context, model.CredentialLease) (model.CredentialLease, error)
	RevokeCredentialLease(context.Context, string, string, time.Time) (model.CredentialLease, error)
	Secret(context.Context, string, string) (model.Secret, error)
	AppendAudit(context.Context, model.AuditEvent) error
}

type Resolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type Runtime struct {
	store    Store
	vault    *secrets.Vault
	resolver Resolver
	doer     Doer
	now      func() time.Time
}

type Principal struct {
	Subject            string
	ExternalCustomerID string
	InstallationID     string
	Grants             map[string]bool
	RequestID          string
}

type ProjectRequest struct {
	EnvironmentID  string `json:"environment_id"`
	Name           string `json:"name"`
	IdempotencyKey string `json:"idempotency_key"`
	TTLSeconds     int    `json:"ttl_seconds"`
}

type CredentialRequest struct {
	EnvironmentID  string   `json:"environment_id"`
	ProjectID      string   `json:"project_id"`
	Scopes         []string `json:"scopes"`
	IdempotencyKey string   `json:"idempotency_key"`
	TTLSeconds     int      `json:"ttl_seconds"`
}

type CredentialResult struct {
	Lease      model.CredentialLease `json:"lease"`
	Credential string                `json:"credential,omitempty"`
	Existing   bool                  `json:"existing"`
}

type providerConfig struct {
	ContractVersion string   `json:"contract_version"`
	AuthorizePath   string   `json:"authorize_path"`
	ProjectPath     string   `json:"project_path"`
	CredentialPath  string   `json:"credential_path"`
	RevokePath      string   `json:"revoke_path"`
	RequiredGrants  []string `json:"required_grants"`
	MaxTTLSeconds   int      `json:"max_ttl_seconds"`
}

func New(store Store, vault *secrets.Vault, resolver Resolver, doer Doer) *Runtime {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &Runtime{store: store, vault: vault, resolver: resolver, doer: doer, now: func() time.Time { return time.Now().UTC() }}
}

func (r *Runtime) HasCapabilities(ctx context.Context, productID string, grants map[string]bool) bool {
	values, err := r.store.Providers(ctx, productID)
	if err != nil {
		return false
	}
	for _, value := range values {
		cfg, err := config(value)
		if err == nil && allow(cfg, Principal{Subject: "discovery", Grants: grants}) {
			return true
		}
	}
	return false
}

func randomUUID() string {
	value := make([]byte, 16)
	_, _ = rand.Read(value)
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}

func config(value model.Provider) (providerConfig, error) {
	var result providerConfig
	if err := json.Unmarshal(value.Config, &result); err != nil {
		return result, err
	}
	if result.AuthorizePath == "" {
		result.AuthorizePath = "/v1/authorize"
	}
	if result.ProjectPath == "" {
		result.ProjectPath = "/v1/projects"
	}
	if result.CredentialPath == "" {
		result.CredentialPath = "/v1/credentials"
	}
	if result.RevokePath == "" {
		result.RevokePath = "/v1/credentials/{credential_id}/revoke"
	}
	if result.MaxTTLSeconds == 0 {
		result.MaxTTLSeconds = 3600
	}
	return result, nil
}

func allow(config providerConfig, principal Principal) bool {
	if principal.Subject == "" {
		return false
	}
	for _, required := range config.RequiredGrants {
		if !principal.Grants[required] {
			return false
		}
	}
	return true
}

func (r *Runtime) destination(ctx context.Context, provider model.Provider, path string) (*url.URL, Doer, error) {
	base, err := url.Parse(provider.BaseURL)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil || base.Port() != "" || !strings.HasPrefix(path, "/") {
		return nil, nil, ErrUnsafeDestination
	}
	addresses, err := r.resolver.LookupIP(ctx, "ip", base.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, nil, ErrUnsafeDestination
	}
	for _, address := range addresses {
		if netpolicy.UnsafeIP(address) {
			return nil, nil, ErrUnsafeDestination
		}
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
	base.RawQuery, base.Fragment = "", ""
	if r.doer != nil {
		return base, r.doer, nil
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: base.Hostname()}, DisableCompression: true, ResponseHeaderTimeout: 10 * time.Second, DialContext: func(dialContext context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(dialContext, network, net.JoinHostPort(addresses[0].String(), "443"))
	}}
	return base, &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func (r *Runtime) credential(ctx context.Context, provider model.Provider) (string, error) {
	if r.vault == nil || provider.CredentialID == "" {
		return "", errors.New("provider credential unavailable")
	}
	value, err := r.store.Secret(ctx, provider.OrganisationID, provider.CredentialID)
	if err != nil {
		return "", err
	}
	plaintext, err := r.vault.Decrypt(secrets.Encrypted{Ciphertext: value.Ciphertext, Nonce: value.Nonce, KeyVersion: value.KeyVersion, Fingerprint: value.Fingerprint}, provider.OrganisationID+":provider:"+provider.CredentialID)
	return string(plaintext), err
}

func (r *Runtime) post(ctx context.Context, provider model.Provider, path string, input, output any) error {
	destination, client, err := r.destination(ctx, provider, path)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, destination.String(), bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	credential, err := r.credential(ctx, provider)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("provider returned %s", response.Status)
	}
	if output == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output)
}

func (r *Runtime) authorize(ctx context.Context, provider model.Provider, cfg providerConfig, principal Principal, operation string, details map[string]any) error {
	if !allow(cfg, principal) {
		return ErrDenied
	}
	var response struct {
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reason"`
	}
	err := r.post(ctx, provider, cfg.AuthorizePath, map[string]any{"operation": operation, "subject": principal.Subject, "external_customer_id": principal.ExternalCustomerID, "installation_id": principal.InstallationID, "product_id": provider.ProductID, "details": details}, &response)
	if err != nil || !response.Allowed {
		return ErrDenied
	}
	return nil
}

func validTTL(value, maximum int) bool { return value >= 300 && value <= maximum }

func (r *Runtime) CreateProject(ctx context.Context, productID, providerID string, request ProjectRequest, principal Principal) (model.Project, error) {
	provider, err := r.store.Provider(ctx, productID, providerID)
	if err != nil {
		return model.Project{}, err
	}
	cfg, err := config(provider)
	if err != nil || request.EnvironmentID == "" || request.Name == "" || len(request.IdempotencyKey) < 16 || !validTTL(request.TTLSeconds, cfg.MaxTTLSeconds) {
		return model.Project{}, ErrInvalidRequest
	}
	projects, err := r.store.Projects(ctx, productID)
	if err != nil {
		return model.Project{}, err
	}
	for _, existing := range projects {
		if existing.ProviderID == providerID && existing.IdempotencyKey == request.IdempotencyKey {
			return existing, nil
		}
	}
	if err := r.authorize(ctx, provider, cfg, principal, "project.create", map[string]any{"environment_id": request.EnvironmentID, "ttl_seconds": request.TTLSeconds}); err != nil {
		return model.Project{}, err
	}
	var response struct {
		ProjectID string     `json:"project_id"`
		State     string     `json:"state"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	err = r.post(ctx, provider, cfg.ProjectPath, map[string]any{"product_id": productID, "environment_id": request.EnvironmentID, "name": request.Name, "owner": map[string]string{"type": "user", "id": principal.Subject}, "idempotency_key": request.IdempotencyKey, "ttl_seconds": request.TTLSeconds}, &response)
	if err != nil || response.ProjectID == "" {
		return model.Project{}, ErrInvalidRequest
	}
	if response.State == "" {
		response.State = "active"
	}
	value, err := r.store.CreateProject(ctx, model.Project{ID: randomUUID(), OrganisationID: provider.OrganisationID, ProductID: productID, EnvironmentID: request.EnvironmentID, ProviderID: providerID, OwnerType: "user", OwnerID: principal.Subject, ExternalID: response.ProjectID, IdempotencyKey: request.IdempotencyKey, State: response.State, ExpiresAt: response.ExpiresAt})
	if err == nil {
		if err := r.store.AppendAudit(ctx, model.AuditEvent{ID: "audit_" + randomUUID(), OrganisationID: provider.OrganisationID, ProductID: productID, ActorID: principal.Subject, Action: "project.created", TargetType: "project", TargetID: value.ID, Current: map[string]any{"provider_id": providerID, "state": value.State}, RequestID: principal.RequestID, CreatedAt: r.now()}); err != nil {
			return model.Project{}, err
		}
	}
	return value, err
}

func (r *Runtime) IssueCredential(ctx context.Context, productID, providerID string, request CredentialRequest, principal Principal) (CredentialResult, error) {
	provider, err := r.store.Provider(ctx, productID, providerID)
	if err != nil {
		return CredentialResult{}, err
	}
	cfg, err := config(provider)
	if err != nil || request.EnvironmentID == "" || len(request.IdempotencyKey) < 16 || !validTTL(request.TTLSeconds, cfg.MaxTTLSeconds) || len(request.Scopes) > 20 {
		return CredentialResult{}, ErrInvalidRequest
	}
	leases, err := r.store.CredentialLeases(ctx, productID)
	if err != nil {
		return CredentialResult{}, err
	}
	for _, existing := range leases {
		if existing.ProviderID == providerID && existing.IdempotencyKey == request.IdempotencyKey {
			return CredentialResult{Lease: existing, Existing: true}, nil
		}
	}
	if request.ProjectID != "" {
		project, err := r.store.Project(ctx, productID, request.ProjectID)
		if err != nil || project.ProviderID != providerID || project.OwnerID != principal.Subject {
			return CredentialResult{}, ErrDenied
		}
	}
	if err := r.authorize(ctx, provider, cfg, principal, "credential.issue", map[string]any{"environment_id": request.EnvironmentID, "project_id": request.ProjectID, "scopes": request.Scopes, "ttl_seconds": request.TTLSeconds}); err != nil {
		return CredentialResult{}, err
	}
	var response struct {
		CredentialID string    `json:"credential_id"`
		Credential   string    `json:"credential"`
		ExpiresAt    time.Time `json:"expires_at"`
	}
	err = r.post(ctx, provider, cfg.CredentialPath, map[string]any{"product_id": productID, "environment_id": request.EnvironmentID, "project_id": request.ProjectID, "subject": principal.Subject, "scopes": request.Scopes, "idempotency_key": request.IdempotencyKey, "ttl_seconds": request.TTLSeconds}, &response)
	if err != nil || response.CredentialID == "" || response.Credential == "" || !response.ExpiresAt.After(r.now()) || response.ExpiresAt.After(r.now().Add(time.Duration(cfg.MaxTTLSeconds)*time.Second)) {
		return CredentialResult{}, ErrInvalidRequest
	}
	fingerprint := sha256.Sum256([]byte(response.Credential))
	lease, err := r.store.CreateCredentialLease(ctx, model.CredentialLease{ID: randomUUID(), OrganisationID: provider.OrganisationID, ProductID: productID, EnvironmentID: request.EnvironmentID, ProjectID: request.ProjectID, ProviderID: providerID, SubjectID: principal.Subject, ExternalID: response.CredentialID, IdempotencyKey: request.IdempotencyKey, Scopes: request.Scopes, SecretFingerprint: hex.EncodeToString(fingerprint[:]), ExpiresAt: response.ExpiresAt})
	if err != nil {
		return CredentialResult{}, err
	}
	if err := r.store.AppendAudit(ctx, model.AuditEvent{ID: "audit_" + randomUUID(), OrganisationID: provider.OrganisationID, ProductID: productID, ActorID: principal.Subject, Action: "credential.issued", TargetType: "credential_lease", TargetID: lease.ID, Current: map[string]any{"provider_id": providerID, "scopes": lease.Scopes, "expires_at": lease.ExpiresAt}, RequestID: principal.RequestID, CreatedAt: r.now()}); err != nil {
		return CredentialResult{}, err
	}
	return CredentialResult{Lease: lease, Credential: response.Credential}, nil
}

func (r *Runtime) RevokeCredential(ctx context.Context, productID, leaseID string, principal Principal) (model.CredentialLease, error) {
	lease, err := r.store.CredentialLease(ctx, productID, leaseID)
	if err != nil || lease.SubjectID != principal.Subject {
		return model.CredentialLease{}, ErrDenied
	}
	provider, err := r.store.Provider(ctx, productID, lease.ProviderID)
	if err != nil {
		return model.CredentialLease{}, err
	}
	cfg, err := config(provider)
	if err != nil {
		return model.CredentialLease{}, err
	}
	if err := r.authorize(ctx, provider, cfg, principal, "credential.revoke", map[string]any{"credential_id": lease.ExternalID}); err != nil {
		return model.CredentialLease{}, err
	}
	path := strings.ReplaceAll(cfg.RevokePath, "{credential_id}", url.PathEscape(lease.ExternalID))
	if err := r.post(ctx, provider, path, map[string]any{"product_id": productID, "subject": principal.Subject}, nil); err != nil {
		return model.CredentialLease{}, err
	}
	updated, err := r.store.RevokeCredentialLease(ctx, productID, leaseID, r.now())
	if err == nil {
		if err := r.store.AppendAudit(ctx, model.AuditEvent{ID: "audit_" + randomUUID(), OrganisationID: provider.OrganisationID, ProductID: productID, ActorID: principal.Subject, Action: "credential.revoked", TargetType: "credential_lease", TargetID: leaseID, RequestID: principal.RequestID, CreatedAt: r.now()}); err != nil {
			return model.CredentialLease{}, err
		}
	}
	return updated, err
}
