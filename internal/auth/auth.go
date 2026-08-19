package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	ErrSetupComplete       = errors.New("initial setup is already complete")
	ErrSetupToken          = errors.New("invalid setup token")
	ErrSetupExpired        = errors.New("setup enrollment expired")
	ErrInvalidMFA          = errors.New("invalid MFA code")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidSession      = errors.New("invalid session")
	ErrInvalidCSRF         = errors.New("invalid CSRF token")
	ErrPasswordRequirement = errors.New("password does not meet requirements")
	ErrCannotRevokeSelf    = errors.New("a root administrator cannot revoke their own account")
	ErrLastRoot            = errors.New("the last active root administrator cannot be revoked")
)

type RootAccount struct {
	UserID               string
	Email                string
	DisplayName          string
	PasswordHash         string
	TOTPSecretCiphertext []byte
	RecoveryCodeDigests  [][]byte
	CreatedAt            time.Time
	RevokedAt            *time.Time
	CreatedBy            string
}

type SessionRecord struct {
	TokenDigest []byte
	UserID      string
	CSRFDigest  []byte
	ExpiresAt   time.Time
	CreatedAt   time.Time
	LastSeenAt  time.Time
}

type Store interface {
	SetupCompleted(context.Context) (bool, error)
	CreateInitialRoot(context.Context, RootAccount) error
	CreateRoot(context.Context, RootAccount) error
	RevokeRoot(context.Context, string, time.Time) error
	RootByEmail(context.Context, string) (RootAccount, error)
	RootByID(context.Context, string) (RootAccount, error)
	RootAccounts(context.Context) ([]RootAccount, error)
	CreateSession(context.Context, SessionRecord) error
	SessionByDigest(context.Context, []byte) (SessionRecord, error)
	DeleteSession(context.Context, []byte) error
}

type Config struct {
	SetupToken string
	MasterKey  []byte
	PublicURL  string
	SessionTTL time.Duration
}

type Manager struct {
	store          Store
	setupTokenHash [32]byte
	masterKey      []byte
	publicURL      string
	sessionTTL     time.Duration
	now            func() time.Time
	mu             sync.Mutex
	pending        map[string]pendingSetup
}

type pendingSetup struct {
	ID            string
	Email         string
	DisplayName   string
	PasswordHash  string
	TOTPSecret    []byte
	RecoveryCodes []string
	ExpiresAt     time.Time
}

type SetupInput struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type Enrollment struct {
	ID              string `json:"enrollment_id"`
	Secret          string `json:"totp_secret"`
	ProvisioningURI string `json:"provisioning_uri"`
	ExpiresAt       string `json:"expires_at"`
}

type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

type Session struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
	User      User
}

type SetupResult struct {
	Session       Session  `json:"-"`
	User          User     `json:"user"`
	CSRFToken     string   `json:"csrf_token"`
	RecoveryCodes []string `json:"recovery_codes"`
}

func New(store Store, config Config) (*Manager, error) {
	if strings.TrimSpace(config.SetupToken) == "" {
		return nil, errors.New("setup token is required")
	}
	if len(config.MasterKey) != 32 {
		return nil, errors.New("master key must be exactly 32 bytes")
	}
	if config.SessionTTL <= 0 {
		config.SessionTTL = 8 * time.Hour
	}
	return &Manager{
		store: store, setupTokenHash: sha256.Sum256([]byte(config.SetupToken)), masterKey: append([]byte(nil), config.MasterKey...),
		publicURL: strings.TrimRight(config.PublicURL, "/"), sessionTTL: config.SessionTTL,
		now: func() time.Time { return time.Now().UTC() }, pending: make(map[string]pendingSetup),
	}, nil
}

func (m *Manager) Status(ctx context.Context) (bool, error) { return m.store.SetupCompleted(ctx) }

func (m *Manager) BeginSetup(ctx context.Context, setupToken string, input SetupInput) (Enrollment, error) {
	provided := sha256.Sum256([]byte(setupToken))
	if !hmac.Equal(provided[:], m.setupTokenHash[:]) {
		return Enrollment{}, ErrSetupToken
	}
	completed, err := m.store.SetupCompleted(ctx)
	if err != nil {
		return Enrollment{}, err
	}
	if completed {
		return Enrollment{}, ErrSetupComplete
	}
	return m.beginEnrollment(input)
}

func (m *Manager) beginEnrollment(input SetupInput) (Enrollment, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	displayName := strings.TrimSpace(input.DisplayName)
	if !validEmail(email) || displayName == "" || len(displayName) > 120 {
		return Enrollment{}, errors.New("valid email and display name are required")
	}
	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return Enrollment{}, err
	}
	totpSecret, err := randomBytes(20)
	if err != nil {
		return Enrollment{}, err
	}
	recoveryCodes, err := generateRecoveryCodes(10)
	if err != nil {
		return Enrollment{}, err
	}
	id, err := randomToken(18)
	if err != nil {
		return Enrollment{}, err
	}
	expiresAt := m.now().Add(15 * time.Minute)
	m.mu.Lock()
	m.pending[id] = pendingSetup{ID: id, Email: email, DisplayName: displayName, PasswordHash: passwordHash, TOTPSecret: totpSecret, RecoveryCodes: recoveryCodes, ExpiresAt: expiresAt}
	m.mu.Unlock()

	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(totpSecret)
	issuer := "DokoSoko"
	label := url.PathEscape(issuer + ":" + email)
	values := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return Enrollment{ID: id, Secret: secret, ProvisioningURI: "otpauth://totp/" + label + "?" + values.Encode(), ExpiresAt: expiresAt.Format(time.RFC3339)}, nil
}

func (m *Manager) BeginRoot(ctx context.Context, input SetupInput) (Enrollment, error) {
	if _, err := m.store.RootByEmail(ctx, strings.ToLower(strings.TrimSpace(input.Email))); err == nil {
		return Enrollment{}, errors.New("a root administrator with this email already exists")
	}
	return m.beginEnrollment(input)
}

func (m *Manager) CompleteRoot(ctx context.Context, enrollmentID, code, createdBy string) (SetupResult, error) {
	m.mu.Lock()
	pending, ok := m.pending[enrollmentID]
	if !ok || m.now().After(pending.ExpiresAt) {
		delete(m.pending, enrollmentID)
		m.mu.Unlock()
		return SetupResult{}, ErrSetupExpired
	}
	if !VerifyTOTP(pending.TOTPSecret, code, m.now()) {
		m.mu.Unlock()
		return SetupResult{}, ErrInvalidMFA
	}
	delete(m.pending, enrollmentID)
	m.mu.Unlock()
	ciphertext, err := m.encrypt(pending.TOTPSecret)
	if err != nil {
		return SetupResult{}, err
	}
	digests := make([][]byte, 0, len(pending.RecoveryCodes))
	for _, recoveryCode := range pending.RecoveryCodes {
		digest := sha256.Sum256([]byte(strings.ToUpper(recoveryCode)))
		digests = append(digests, digest[:])
	}
	userID, err := randomUUID()
	if err != nil {
		return SetupResult{}, err
	}
	account := RootAccount{UserID: userID, Email: pending.Email, DisplayName: pending.DisplayName, PasswordHash: pending.PasswordHash, TOTPSecretCiphertext: ciphertext, RecoveryCodeDigests: digests, CreatedAt: m.now(), CreatedBy: createdBy}
	if err := m.store.CreateRoot(ctx, account); err != nil {
		return SetupResult{}, err
	}
	return SetupResult{User: publicUser(account), RecoveryCodes: pending.RecoveryCodes}, nil
}

func (m *Manager) RevokeRoot(ctx context.Context, userID, actorID string) error {
	if userID == actorID {
		return ErrCannotRevokeSelf
	}
	return m.store.RevokeRoot(ctx, userID, m.now())
}

func (m *Manager) CompleteSetup(ctx context.Context, enrollmentID, code string) (SetupResult, error) {
	m.mu.Lock()
	pending, ok := m.pending[enrollmentID]
	if !ok || m.now().After(pending.ExpiresAt) {
		delete(m.pending, enrollmentID)
		m.mu.Unlock()
		return SetupResult{}, ErrSetupExpired
	}
	if !VerifyTOTP(pending.TOTPSecret, code, m.now()) {
		m.mu.Unlock()
		return SetupResult{}, ErrInvalidMFA
	}
	delete(m.pending, enrollmentID)
	m.mu.Unlock()
	ciphertext, err := m.encrypt(pending.TOTPSecret)
	if err != nil {
		return SetupResult{}, err
	}
	digests := make([][]byte, 0, len(pending.RecoveryCodes))
	for _, code := range pending.RecoveryCodes {
		digest := sha256.Sum256([]byte(strings.ToUpper(code)))
		digests = append(digests, digest[:])
	}
	userID, err := randomUUID()
	if err != nil {
		return SetupResult{}, err
	}
	account := RootAccount{UserID: userID, Email: pending.Email, DisplayName: pending.DisplayName, PasswordHash: pending.PasswordHash, TOTPSecretCiphertext: ciphertext, RecoveryCodeDigests: digests, CreatedAt: m.now()}
	if err := m.store.CreateInitialRoot(ctx, account); err != nil {
		return SetupResult{}, err
	}
	session, err := m.newSession(ctx, account)
	if err != nil {
		return SetupResult{}, err
	}
	return SetupResult{Session: session, User: session.User, CSRFToken: session.CSRFToken, RecoveryCodes: pending.RecoveryCodes}, nil
}

func (m *Manager) Login(ctx context.Context, email, password, code string) (Session, error) {
	account, err := m.store.RootByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil || account.RevokedAt != nil || !VerifyPassword(account.PasswordHash, password) {
		return Session{}, ErrInvalidCredentials
	}
	secret, err := m.decrypt(account.TOTPSecretCiphertext)
	if err != nil || !VerifyTOTP(secret, code, m.now()) {
		return Session{}, ErrInvalidCredentials
	}
	return m.newSession(ctx, account)
}

func (m *Manager) Authenticate(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrInvalidSession
	}
	digest := sha256.Sum256([]byte(token))
	record, err := m.store.SessionByDigest(ctx, digest[:])
	if err != nil || m.now().After(record.ExpiresAt) {
		if err == nil {
			_ = m.store.DeleteSession(ctx, digest[:])
		}
		return Session{}, ErrInvalidSession
	}
	account, err := m.store.RootByID(ctx, record.UserID)
	if err != nil || account.RevokedAt != nil {
		return Session{}, ErrInvalidSession
	}
	return Session{Token: token, ExpiresAt: record.ExpiresAt, User: publicUser(account)}, nil
}

func (m *Manager) VerifyCSRF(ctx context.Context, sessionToken, csrfToken string) error {
	sessionDigest := sha256.Sum256([]byte(sessionToken))
	record, err := m.store.SessionByDigest(ctx, sessionDigest[:])
	if err != nil || m.now().After(record.ExpiresAt) {
		return ErrInvalidSession
	}
	csrfDigest := sha256.Sum256([]byte(csrfToken))
	if !hmac.Equal(record.CSRFDigest, csrfDigest[:]) {
		return ErrInvalidCSRF
	}
	return nil
}

func (m *Manager) Logout(ctx context.Context, token string) error {
	digest := sha256.Sum256([]byte(token))
	return m.store.DeleteSession(ctx, digest[:])
}

func (m *Manager) RootUsers(ctx context.Context) ([]User, error) {
	accounts, err := m.store.RootAccounts(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]User, 0, len(accounts))
	for _, account := range accounts {
		result = append(result, publicUser(account))
	}
	return result, nil
}

func (m *Manager) newSession(ctx context.Context, account RootAccount) (Session, error) {
	token, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	csrfToken, err := randomToken(24)
	if err != nil {
		return Session{}, err
	}
	tokenDigest := sha256.Sum256([]byte(token))
	csrfDigest := sha256.Sum256([]byte(csrfToken))
	now := m.now()
	expiresAt := now.Add(m.sessionTTL)
	if err := m.store.CreateSession(ctx, SessionRecord{TokenDigest: tokenDigest[:], UserID: account.UserID, CSRFDigest: csrfDigest[:], ExpiresAt: expiresAt, CreatedAt: now, LastSeenAt: now}); err != nil {
		return Session{}, err
	}
	return Session{Token: token, CSRFToken: csrfToken, ExpiresAt: expiresAt, User: publicUser(account)}, nil
}

func publicUser(account RootAccount) User {
	return User{ID: account.UserID, Email: account.Email, DisplayName: account.DisplayName, Role: "root", RevokedAt: account.RevokedAt}
}

func (m *Manager) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(m.masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, gcm.Seal(nil, nonce, plaintext, []byte("dokosoko-root-totp-v1"))...), nil
}

func (m *Manager) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(m.masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("invalid encrypted value")
	}
	nonce := ciphertext[:gcm.NonceSize()]
	return gcm.Open(nil, nonce, ciphertext[gcm.NonceSize():], []byte("dokosoko-root-totp-v1"))
}

func validEmail(value string) bool {
	at := strings.LastIndex(value, "@")
	return at > 0 && at < len(value)-3 && strings.Contains(value[at+1:], ".") && len(value) <= 254
}

func randomBytes(size int) ([]byte, error) {
	buffer := make([]byte, size)
	_, err := rand.Read(buffer)
	return buffer, err
}

func randomToken(size int) (string, error) {
	buffer, err := randomBytes(size)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func randomUUID() (string, error) {
	value, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:]), nil
}

func generateRecoveryCodes(count int) ([]string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	result := make([]string, 0, count)
	for range count {
		buffer, err := randomBytes(12)
		if err != nil {
			return nil, err
		}
		characters := make([]byte, 12)
		for index, value := range buffer {
			characters[index] = alphabet[int(value)%len(alphabet)]
		}
		result = append(result, fmt.Sprintf("%s-%s-%s", characters[:4], characters[4:8], characters[8:]))
	}
	return result, nil
}
