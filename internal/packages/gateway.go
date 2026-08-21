package packages

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
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
)

var (
	ErrUnsafeURL       = errors.New("package endpoint resolves to a disallowed network")
	ErrArtifactInvalid = errors.New("package artifact failed integrity validation")
)

const (
	packageDownloadPath      = "/v1/package/download"
	maxDownloadResponseBytes = 1 << 20
	minDownloadLifetime      = 5 * time.Minute
	maxDownloadLifetime      = time.Hour
	maxDownloadAttempts      = 2
	defaultMaxArtifactBytes  = 1 << 30
)

type Store interface {
	Package(context.Context, string, string) (model.Package, error)
	Secret(context.Context, string, string) (model.Secret, error)
}

type Resolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type Config struct {
	DataDirectory string
	MaxBytes      int64
	Resolver      Resolver
	Doer          Doer
}

type Gateway struct {
	store    Store
	vault    *secrets.Vault
	dataDir  string
	maxBytes int64
	resolver Resolver
	doer     Doer
}

type Artifact struct {
	File        *os.File
	Size        int64
	SHA256      string
	ContentType string
	Filename    string
}

func (a *Artifact) Close() error {
	name := a.File.Name()
	err := a.File.Close()
	_ = os.Remove(name)
	return err
}

func New(store Store, vault *secrets.Vault, config Config) *Gateway {
	if config.DataDirectory == "" {
		config.DataDirectory = os.TempDir()
	}
	if config.MaxBytes <= 0 {
		config.MaxBytes = defaultMaxArtifactBytes
	}
	if config.Resolver == nil {
		config.Resolver = net.DefaultResolver
	}
	return &Gateway{store: store, vault: vault, dataDir: config.DataDirectory, maxBytes: config.MaxBytes, resolver: config.Resolver, doer: config.Doer}
}

func deniedIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	address, err := netip.ParseAddr(ip.String())
	if err != nil {
		return true
	}
	for _, prefix := range []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("198.18.0.0/15")} {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func (g *Gateway) safeEndpoint(ctx context.Context, value string) (*url.URL, net.IP, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || (parsed.Port() != "" && parsed.Port() != "443") {
		return nil, nil, ErrUnsafeURL
	}
	addresses, err := g.resolver.LookupIP(ctx, "ip", parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, nil, ErrUnsafeURL
	}
	for _, address := range addresses {
		if deniedIP(address) {
			return nil, nil, ErrUnsafeURL
		}
	}
	return parsed, addresses[0], nil
}

func (g *Gateway) clientFor(parsed *url.URL, address net.IP) Doer {
	if g.doer != nil {
		return g.doer
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}
	transport := &http.Transport{
		Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: parsed.Hostname()},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), "443"))
		},
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, DisableCompression: true,
	}
	return &http.Client{Transport: transport, Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (g *Gateway) request(ctx context.Context, method, endpoint, credential string, body io.Reader) (*http.Response, error) {
	current := endpoint
	for redirects := 0; redirects <= 5; redirects++ {
		parsed, address, err := g.safeEndpoint(ctx, current)
		if err != nil {
			return nil, err
		}
		request, err := http.NewRequestWithContext(ctx, method, parsed.String(), body)
		if err != nil {
			return nil, err
		}
		request.Header.Set("User-Agent", "DokoSokoPackageGateway/2.0")
		request.Header.Set("Accept", "application/octet-stream, application/json")
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		if credential != "" {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
		response, err := g.clientFor(parsed, address).Do(request)
		if err != nil {
			return nil, err
		}
		if response.StatusCode < 300 || response.StatusCode >= 400 {
			return response, nil
		}
		location := response.Header.Get("Location")
		_ = response.Body.Close()
		if location == "" || redirects == 5 {
			return nil, errors.New("invalid package redirect")
		}
		reference, err := url.Parse(location)
		if err != nil {
			return nil, errors.New("invalid package redirect")
		}
		next := parsed.ResolveReference(reference)
		if !strings.EqualFold(next.Hostname(), parsed.Hostname()) {
			credential = ""
		}
		current, method, body = next.String(), http.MethodGet, nil
	}
	return nil, errors.New("package redirect limit exceeded")
}

func (g *Gateway) decryptCredential(ctx context.Context, pkg model.Package) (string, error) {
	if pkg.CredentialID == "" {
		return "", nil
	}
	if g.vault == nil {
		return "", errors.New("secret vault is unavailable")
	}
	value, err := g.store.Secret(ctx, pkg.OrganisationID, pkg.CredentialID)
	if err != nil {
		return "", err
	}
	plaintext, err := g.vault.Decrypt(secrets.Encrypted{Ciphertext: value.Ciphertext, Nonce: value.Nonce, Fingerprint: value.Fingerprint, KeyVersion: value.KeyVersion}, pkg.OrganisationID+":package:"+pkg.CredentialID)
	return string(plaintext), err
}

type downloadResponse struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	SizeBytes *int64 `json:"size_bytes"`
	ExpiresAt string `json:"expires_at"`
}

func packageDownloadIdentifier(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value), nil
}

func packageDownloadRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func packageDownloadRetryDelay(response *http.Response, attempt int) time.Duration {
	if response != nil {
		if raw := strings.TrimSpace(response.Header.Get("Retry-After")); raw != "" {
			if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
				delay := time.Duration(seconds) * time.Second
				if delay > 2*time.Second {
					return 2 * time.Second
				}
				return delay
			}
			if at, err := http.ParseTime(raw); err == nil {
				delay := time.Until(at)
				if delay < 0 {
					return 0
				}
				if delay > 2*time.Second {
					return 2 * time.Second
				}
				return delay
			}
		}
	}
	return time.Duration(attempt+1) * 50 * time.Millisecond
}

func waitForPackageDownloadRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (g *Gateway) requestPackageDownload(ctx context.Context, pkg model.Package, credential, idempotencyKey, requestID string, body []byte) (*http.Response, error) {
	parsed, address, err := g.safeEndpoint(ctx, pkg.DownloadURL)
	if err != nil || parsed.Path != packageDownloadPath || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, ErrUnsafeURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	request.Header.Set("X-DokoSoko-Request-ID", requestID)
	request.Header.Set("User-Agent", "DokoSokoPackageGateway/3.0")
	return g.clientFor(parsed, address).Do(request)
}

func (g *Gateway) validatePackageDownload(pkg model.Package, response downloadResponse) (string, string, int64, error) {
	if response.ID == "" || len(response.ID) > 200 {
		return "", "", 0, errors.New("package download response has an invalid id")
	}
	digest, err := hex.DecodeString(response.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return "", "", 0, errors.New("package download response has an invalid sha256")
	}
	if response.SizeBytes == nil || *response.SizeBytes < 0 || *response.SizeBytes > g.maxBytes {
		return "", "", 0, errors.New("package download response has an invalid size_bytes")
	}
	artifactURL, err := url.Parse(response.URL)
	if err != nil || artifactURL.Scheme != "https" || artifactURL.Hostname() == "" || artifactURL.User != nil || artifactURL.Fragment != "" || (artifactURL.Port() != "" && artifactURL.Port() != "443") {
		return "", "", 0, errors.New("package download response has an invalid url")
	}
	expiresAt, err := time.Parse(time.RFC3339, response.ExpiresAt)
	now := time.Now().UTC()
	if err != nil || expiresAt.Before(now.Add(minDownloadLifetime)) || expiresAt.After(now.Add(maxDownloadLifetime)) {
		return "", "", 0, errors.New("package download response has an invalid expires_at")
	}
	if len(pkg.ChecksumSHA256) > 0 && !strings.EqualFold(hex.EncodeToString(pkg.ChecksumSHA256), response.SHA256) {
		return "", "", 0, fmt.Errorf("%w: package download checksum does not match the published package", ErrArtifactInvalid)
	}
	if pkg.ExpectedSize > 0 && pkg.ExpectedSize != *response.SizeBytes {
		return "", "", 0, fmt.Errorf("%w: package download size does not match the published package", ErrArtifactInvalid)
	}
	return response.URL, strings.ToLower(response.SHA256), *response.SizeBytes, nil
}

func (g *Gateway) createPackageDownload(ctx context.Context, pkg model.Package, credential string) (string, string, int64, error) {
	if strings.TrimSpace(pkg.ExternalPackageID) == "" {
		return "", "", 0, errors.New("download package is missing external_package_id")
	}
	idempotencyKey, err := packageDownloadIdentifier("pkgdl_")
	if err != nil {
		return "", "", 0, err
	}
	payload, err := json.Marshal(map[string]string{"external_package_id": pkg.ExternalPackageID})
	if err != nil {
		return "", "", 0, err
	}
	for attempt := 0; attempt < maxDownloadAttempts; attempt++ {
		requestID, err := packageDownloadIdentifier("req_")
		if err != nil {
			return "", "", 0, err
		}
		response, requestErr := g.requestPackageDownload(ctx, pkg, credential, idempotencyKey, requestID, payload)
		if requestErr != nil {
			if attempt+1 < maxDownloadAttempts {
				if err := waitForPackageDownloadRetry(ctx, packageDownloadRetryDelay(nil, attempt)); err != nil {
					return "", "", 0, err
				}
				continue
			}
			return "", "", 0, requestErr
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxDownloadResponseBytes))
			_ = response.Body.Close()
			if attempt+1 < maxDownloadAttempts && packageDownloadRetryable(response.StatusCode) {
				if err := waitForPackageDownloadRetry(ctx, packageDownloadRetryDelay(response, attempt)); err != nil {
					return "", "", 0, err
				}
				continue
			}
			return "", "", 0, fmt.Errorf("package download endpoint returned %s", response.Status)
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxDownloadResponseBytes+1))
		_ = response.Body.Close()
		if readErr != nil || len(raw) > maxDownloadResponseBytes {
			return "", "", 0, errors.New("package download response is too large")
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		var value downloadResponse
		if err := decoder.Decode(&value); err != nil {
			return "", "", 0, fmt.Errorf("invalid package download response: %w", err)
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return "", "", 0, errors.New("invalid package download response: trailing JSON content")
		}
		return g.validatePackageDownload(pkg, value)
	}
	return "", "", 0, errors.New("package download endpoint retry limit exceeded")
}

func (g *Gateway) resolve(ctx context.Context, pkg model.Package, credential string) (string, string, int64, error) {
	switch pkg.Mode {
	case "public":
		return pkg.Location, hex.EncodeToString(pkg.ChecksumSHA256), pkg.ExpectedSize, nil
	case "proxy":
		return pkg.Location, hex.EncodeToString(pkg.ChecksumSHA256), pkg.ExpectedSize, nil
	case "download":
		return g.createPackageDownload(ctx, pkg, credential)
	default:
		return "", "", 0, errors.New("unsupported package mode")
	}
}

func (g *Gateway) Acquire(ctx context.Context, productID, packageID string) (*Artifact, error) {
	pkg, err := g.store.Package(ctx, productID, packageID)
	if err != nil {
		return nil, err
	}
	if !pkg.Published {
		return nil, errors.New("package is not published")
	}
	credential, err := g.decryptCredential(ctx, pkg)
	if err != nil {
		return nil, err
	}
	endpoint, expectedHash, expectedSize, err := g.resolve(ctx, pkg, credential)
	if err != nil {
		return nil, err
	}
	artifactCredential := credential
	if pkg.Mode == "download" {
		artifactCredential = ""
	}
	response, err := g.request(ctx, http.MethodGet, endpoint, artifactCredential, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("package upstream returned %s", response.Status)
	}
	if response.ContentLength > g.maxBytes || (expectedSize > 0 && response.ContentLength > 0 && response.ContentLength != expectedSize) {
		return nil, ErrArtifactInvalid
	}
	if err := os.MkdirAll(filepath.Join(g.dataDir, "package-tmp"), 0o700); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(filepath.Join(g.dataDir, "package-tmp"), "artifact-*")
	if err != nil {
		return nil, err
	}
	cleanup := func() { name := file.Name(); _ = file.Close(); _ = os.Remove(name) }
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, g.maxBytes+1))
	actualHash := hex.EncodeToString(hash.Sum(nil))
	if err != nil || written > g.maxBytes || (expectedSize > 0 && written != expectedSize) || (expectedHash != "" && !strings.EqualFold(expectedHash, actualHash)) {
		cleanup()
		return nil, ErrArtifactInvalid
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, err
	}
	filename := pkg.Name + "-" + pkg.Version
	if disposition := response.Header.Get("Content-Disposition"); disposition != "" {
		if _, parameters, err := mime.ParseMediaType(disposition); err == nil && parameters["filename"] != "" {
			filename = parameters["filename"]
		}
	}
	filename = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r < 32 {
			return '-'
		}
		return r
	}, filename)
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return &Artifact{File: file, Size: written, SHA256: actualHash, ContentType: contentType, Filename: filename}, nil
}
