package packages_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko/v2/internal/model"
	"github.com/dokosoko/dokosoko/v2/internal/packages"
	"github.com/dokosoko/dokosoko/v2/internal/secrets"
	"github.com/dokosoko/dokosoko/v2/internal/store"
)

type resolver struct{ address net.IP }

func (r resolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{r.address}, nil
}

type doerFunc func(*http.Request) (*http.Response, error)

func (function doerFunc) Do(request *http.Request) (*http.Response, error) { return function(request) }

func response(status int, body string, headers map[string]string) *http.Response {
	value := &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), ContentLength: int64(len(body))}
	for key, header := range headers {
		value.Header.Set(key, header)
	}
	return value
}

func packageFixture(t *testing.T, mode string, body []byte) (*store.Memory, *secrets.Vault, model.Package) {
	t.Helper()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x45}, 32))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	credentialID := "credential-id"
	encrypted, err := vault.Encrypt([]byte("vendor-token"), "org_acme:package:"+credentialID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = memory.CreateSecret(context.Background(), model.Secret{ID: credentialID, OrganisationID: "org_acme", Name: "gateway-test", Purpose: "package_upstream", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := memory.CreatePackage(context.Background(), model.Package{ID: "package-id", OrganisationID: "org_acme", ProductID: "prod_acme", Name: "@acme/sdk", Version: "1.2.3", Ecosystem: "npm", Mode: mode, Location: "https://packages.example.test/sdk.tgz", FetchHookURL: "https://api.example.test/fetch", CredentialID: credentialID, ChecksumSHA256: digest[:], ExpectedSize: int64(len(body))})
	if err != nil {
		t.Fatal(err)
	}
	pkg.Published = true
	pkg, err = memory.UpdatePackage(context.Background(), pkg, pkg.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return memory, vault, pkg
}

func TestProxyModeUsesCredentialAndVerifiesArtifact(t *testing.T) {
	t.Parallel()
	body := []byte("verified package bytes")
	memory, vault, pkg := packageFixture(t, "proxy", body)
	var authorization string
	gateway := packages.New(memory, vault, packages.Config{DataDirectory: t.TempDir(), Resolver: resolver{net.ParseIP("8.8.8.8")}, Doer: doerFunc(func(request *http.Request) (*http.Response, error) {
		authorization = request.Header.Get("Authorization")
		return response(http.StatusOK, string(body), map[string]string{"Content-Type": "application/gzip"}), nil
	})})
	artifact, err := gateway.Acquire(context.Background(), pkg.ProductID, pkg.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	read, _ := io.ReadAll(artifact.File)
	if string(read) != string(body) || authorization != "Bearer vendor-token" || artifact.SHA256 != hex.EncodeToString(pkg.ChecksumSHA256) {
		t.Fatalf("artifact=%q auth=%q hash=%q", read, authorization, artifact.SHA256)
	}
}

func TestFetchModeDoesNotForwardHookCredentialToSignedURL(t *testing.T) {
	t.Parallel()
	body := []byte("fetch package bytes")
	memory, vault, pkg := packageFixture(t, "fetch", body)
	calls := 0
	gateway := packages.New(memory, vault, packages.Config{DataDirectory: t.TempDir(), Resolver: resolver{net.ParseIP("1.1.1.1")}, Doer: doerFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if request.Header.Get("Authorization") != "Bearer vendor-token" || request.Method != http.MethodPost {
				t.Fatalf("hook auth=%q method=%s", request.Header.Get("Authorization"), request.Method)
			}
			digest := sha256.Sum256(body)
			payload := `{"url":"https://signed.example.test/artifact","sha256":"` + hex.EncodeToString(digest[:]) + `","size":` + fmt.Sprint(len(body)) + `,"expires_at":"` + time.Now().UTC().Add(time.Minute).Format(time.RFC3339) + `"}`
			return response(http.StatusOK, payload, map[string]string{"Content-Type": "application/json"}), nil
		}
		if request.Header.Get("Authorization") != "" {
			t.Fatalf("hook credential leaked to signed URL: %q", request.Header.Get("Authorization"))
		}
		return response(http.StatusOK, string(body), nil), nil
	})})
	artifact, err := gateway.Acquire(context.Background(), pkg.ProductID, pkg.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestGatewayRejectsPrivateNetworksAndChecksumMismatch(t *testing.T) {
	t.Parallel()
	body := []byte("expected")
	memory, vault, pkg := packageFixture(t, "proxy", body)
	gateway := packages.New(memory, vault, packages.Config{DataDirectory: t.TempDir(), Resolver: resolver{net.ParseIP("169.254.169.254")}, Doer: doerFunc(func(*http.Request) (*http.Response, error) { return response(http.StatusOK, string(body), nil), nil })})
	if _, err := gateway.Acquire(context.Background(), pkg.ProductID, pkg.ID); !errors.Is(err, packages.ErrUnsafeURL) {
		t.Fatalf("private network error = %v", err)
	}
	gateway = packages.New(memory, vault, packages.Config{DataDirectory: t.TempDir(), Resolver: resolver{net.ParseIP("8.8.4.4")}, Doer: doerFunc(func(*http.Request) (*http.Response, error) { return response(http.StatusOK, "tampered", nil), nil })})
	if _, err := gateway.Acquire(context.Background(), pkg.ProductID, pkg.ID); !errors.Is(err, packages.ErrArtifactInvalid) {
		t.Fatalf("checksum error = %v", err)
	}
}
