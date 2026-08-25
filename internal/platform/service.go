package platform

import (
	"errors"
	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	secretvault "github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"regexp"
	"time"
)

var (
	ErrConfirmationRequired  = errors.New("public access confirmation required")
	ErrUnsafeForPublic       = errors.New("resource is not safe for public access")
	ErrInvalidVisibility     = errors.New("invalid visibility")
	ErrToolDrifted           = errors.New("imported tool schema drift requires review")
	ErrSourceReviewRequired  = errors.New("source publication requires a completed, reviewable crawl with fetched evidence")
	ErrIdentityDraftRequired = errors.New("identity provider must be a disabled draft")
	ErrIdentityTestRequired  = errors.New("a passing, unexpired identity test for this exact configuration revision is required")
	ErrIdentityConfigInvalid = errors.New("identity provider configuration is invalid")
	ErrIdentityCredential    = errors.New("identity provider client credential is required")
	ErrIdentityDisableFirst  = errors.New("identity provider must be disabled before disconnecting")
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

const identityOAuthCleanupBatch = 100

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
