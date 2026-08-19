package auth_test

import (
	"bytes"
	"context"
	"encoding/base32"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func newManager(t *testing.T) (*auth.Manager, *store.Memory) {
	t.Helper()
	memory := store.NewMemory()
	manager, err := auth.New(memory, auth.Config{SetupToken: "one-time-setup-token", MasterKey: []byte("0123456789abcdef0123456789abcdef"), PublicURL: "https://dokosoko.example", SessionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return manager, memory
}

func TestPasswordHashingAndRequirements(t *testing.T) {
	t.Parallel()
	if _, err := auth.HashPassword("short"); !errors.Is(err, auth.ErrPasswordRequirement) {
		t.Fatalf("weak password error = %v", err)
	}
	hash, err := auth.HashPassword("Long-and-Safe-Password-42!")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, "Long-and-Safe") {
		t.Fatal("password hash contains plaintext")
	}
	if !auth.VerifyPassword(hash, "Long-and-Safe-Password-42!") {
		t.Fatal("valid password did not verify")
	}
	if auth.VerifyPassword(hash, "Wrong-and-Safe-Password-42!") {
		t.Fatal("invalid password verified")
	}
}

func TestTOTPMatchesRFCVectorTruncatedToSixDigits(t *testing.T) {
	t.Parallel()
	secret := []byte("12345678901234567890")
	value := auth.TOTP(secret, time.Unix(59, 0).UTC())
	if value != "287082" {
		t.Fatalf("TOTP = %q, want 287082", value)
	}
	if !auth.VerifyTOTP(secret, value, time.Unix(59, 0).UTC()) {
		t.Fatal("generated TOTP did not verify")
	}
}

func TestFirstRunSetupRequiresTokenMFAAndCreatesSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager, memory := newManager(t)
	input := auth.SetupInput{Email: "Root@Example.com", DisplayName: "Root Operator", Password: "Long-and-Safe-Password-42!"}

	if _, err := manager.BeginSetup(ctx, "wrong", input); !errors.Is(err, auth.ErrSetupToken) {
		t.Fatalf("wrong setup token error = %v", err)
	}
	enrollment, err := manager.BeginSetup(ctx, "one-time-setup-token", input)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CompleteSetup(ctx, enrollment.ID, "000000"); !errors.Is(err, auth.ErrInvalidMFA) {
		t.Fatalf("invalid MFA error = %v", err)
	}
	result, err := manager.CompleteSetup(ctx, enrollment.ID, auth.TOTP(secret, time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	if result.User.Role != "root" || result.User.Email != "root@example.com" || len(result.RecoveryCodes) != 10 {
		t.Fatalf("setup result = %#v", result)
	}
	completed, err := manager.Status(ctx)
	if err != nil || !completed {
		t.Fatalf("setup completed = %v, err = %v", completed, err)
	}
	if _, err := manager.BeginSetup(ctx, "one-time-setup-token", input); !errors.Is(err, auth.ErrSetupComplete) {
		t.Fatalf("repeated setup error = %v", err)
	}

	stored, err := memory.RootByEmail(ctx, "root@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored.TOTPSecretCiphertext, secret) {
		t.Fatal("stored MFA secret contains plaintext")
	}
	if err := manager.VerifyCSRF(ctx, result.Session.Token, result.CSRFToken); err != nil {
		t.Fatalf("valid CSRF token error = %v", err)
	}
	if err := manager.VerifyCSRF(ctx, result.Session.Token, "wrong"); !errors.Is(err, auth.ErrInvalidCSRF) {
		t.Fatalf("invalid CSRF error = %v", err)
	}
	session, err := manager.Authenticate(ctx, result.Session.Token)
	if err != nil || session.User.ID != result.User.ID {
		t.Fatalf("authenticated session = %#v, err = %v", session, err)
	}
	if err := manager.Logout(ctx, result.Session.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Authenticate(ctx, result.Session.Token); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("logged-out session error = %v", err)
	}
}

func TestRootLoginRequiresPasswordAndMFA(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager, _ := newManager(t)
	input := auth.SetupInput{Email: "root@example.com", DisplayName: "Root Operator", Password: "Long-and-Safe-Password-42!"}
	enrollment, err := manager.BeginSetup(ctx, "one-time-setup-token", input)
	if err != nil {
		t.Fatal(err)
	}
	secret, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if _, err := manager.CompleteSetup(ctx, enrollment.ID, auth.TOTP(secret, time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Login(ctx, input.Email, "wrong", auth.TOTP(secret, time.Now().UTC())); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v", err)
	}
	if _, err := manager.Login(ctx, input.Email, input.Password, "000000"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("wrong MFA error = %v", err)
	}
	session, err := manager.Login(ctx, input.Email, input.Password, auth.TOTP(secret, time.Now().UTC()))
	if err != nil || session.Token == "" || session.CSRFToken == "" {
		t.Fatalf("login session = %#v, err = %v", session, err)
	}
}

func TestAdditionalRootRequiresOwnMFAAndCanBeRevoked(t *testing.T) {
	ctx := context.Background()
	manager, _ := newManager(t)
	first := auth.SetupInput{Email: "root@example.com", DisplayName: "Root Operator", Password: "Long-and-Safe-Password-42!"}
	enrollment, err := manager.BeginSetup(ctx, "one-time-setup-token", first)
	if err != nil {
		t.Fatal(err)
	}
	secret, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	created, err := manager.CompleteSetup(ctx, enrollment.ID, auth.TOTP(secret, time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	second := auth.SetupInput{Email: "backup@example.com", DisplayName: "Backup Root", Password: "Another-Safe-Password-42!"}
	secondEnrollment, err := manager.BeginRoot(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	secondSecret, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secondEnrollment.Secret)
	secondCreated, err := manager.CompleteRoot(ctx, secondEnrollment.ID, auth.TOTP(secondSecret, time.Now().UTC()), created.User.ID)
	if err != nil || len(secondCreated.RecoveryCodes) != 10 {
		t.Fatalf("additional root = %#v, err = %v", secondCreated, err)
	}
	if err := manager.RevokeRoot(ctx, created.User.ID, created.User.ID); !errors.Is(err, auth.ErrCannotRevokeSelf) {
		t.Fatalf("self revoke error = %v", err)
	}
	if err := manager.RevokeRoot(ctx, secondCreated.User.ID, created.User.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Login(ctx, second.Email, second.Password, auth.TOTP(secondSecret, time.Now().UTC())); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("revoked root login error = %v", err)
	}
	if err := manager.RevokeRoot(ctx, created.User.ID, secondCreated.User.ID); !errors.Is(err, auth.ErrLastRoot) {
		t.Fatalf("last root revoke error = %v", err)
	}
}
