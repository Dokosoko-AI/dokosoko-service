package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOAuthLoopbackLoginCompletesWithExactCallbackAndPrivateToken(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	var valuesMu sync.Mutex
	var registeredRedirect string
	var expectedChallenge string
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/oauth-authorization-server":
			writeJSONTest(writer, map[string]any{
				"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize",
				"token_endpoint": server.URL + "/token", "registration_endpoint": server.URL + "/register",
				"code_challenge_methods_supported":      []string{"S256"},
				"token_endpoint_auth_methods_supported": []string{"none"},
			})
		case "/register":
			var registration struct {
				RedirectURIs []string `json:"redirect_uris"`
			}
			if err := json.NewDecoder(request.Body).Decode(&registration); err != nil || len(registration.RedirectURIs) != 1 {
				t.Error("invalid dynamic registration request")
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			valuesMu.Lock()
			registeredRedirect = registration.RedirectURIs[0]
			valuesMu.Unlock()
			writer.WriteHeader(http.StatusCreated)
			writeJSONTest(writer, map[string]any{"client_id": "loopback-client"})
		case "/token":
			_ = request.ParseForm()
			verifier := request.Form.Get("code_verifier")
			digest := sha256.Sum256([]byte(verifier))
			valuesMu.Lock()
			challenge := expectedChallenge
			valuesMu.Unlock()
			if request.Form.Get("code") != "browser-code" || base64.RawURLEncoding.EncodeToString(digest[:]) != challenge {
				t.Error("invalid loopback token exchange")
			}
			writeJSONTest(writer, map[string]any{
				"access_token": "loopback-secret-token", "token_type": "Bearer", "expires_in": 600, "scope": "mcp:private",
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	directory := t.TempDir()
	stateFile := filepath.Join(directory, "state.json")
	tokenFile := filepath.Join(directory, "token.json")
	var browserStatus int
	var browserHeaders http.Header
	var browserBody string
	var authorizationState string
	result, err := LoginOAuth(context.Background(), OAuthLoginConfig{
		Start: OAuthStartConfig{
			Endpoint: server.URL + "/mcp", AllowedLoopbackHTTP: testLoopbackHTTP(t, server.URL),
			StateFile: stateFile, TokenFile: tokenFile,
		},
		ListenAddress: "127.0.0.1:0",
		CallbackPath:  "/oauth/callback",
		Timeout:       2 * time.Second,
		OnAuthorizationURL: func(rawAuthorizationURL string) {
			authorizationURL, parseErr := url.Parse(rawAuthorizationURL)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			redirectURI := authorizationURL.Query().Get("redirect_uri")
			redirect, parseErr := url.Parse(redirectURI)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			if redirect.Scheme != "http" || redirect.Hostname() != "127.0.0.1" || redirect.Port() == "" || redirect.Port() == "0" || redirect.Path != "/oauth/callback" {
				t.Errorf("unsafe or invalid generated redirect URI %q", redirectURI)
			}
			valuesMu.Lock()
			if registeredRedirect != redirectURI {
				t.Errorf("registered redirect %q did not match authorization redirect %q", registeredRedirect, redirectURI)
			}
			expectedChallenge = authorizationURL.Query().Get("code_challenge")
			valuesMu.Unlock()
			authorizationState = authorizationURL.Query().Get("state")

			wrongResponse, getErr := http.Get(redirect.Scheme + "://" + redirect.Host + "/wrong-path")
			if getErr != nil {
				t.Fatal(getErr)
			}
			_ = wrongResponse.Body.Close()
			if wrongResponse.StatusCode != http.StatusNotFound {
				t.Errorf("wrong callback path status = %d", wrongResponse.StatusCode)
			}

			callback := redirectURI + "?code=browser-code&state=" + url.QueryEscape(authorizationState)
			response, getErr := http.Get(callback)
			if getErr != nil {
				t.Fatal(getErr)
			}
			defer response.Body.Close()
			body, readErr := io.ReadAll(response.Body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			browserStatus = response.StatusCode
			browserHeaders = response.Header.Clone()
			browserBody = string(body)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TokenFile != tokenFile || result.ExpiresIn != 600 {
		t.Fatalf("login result = %#v", result)
	}
	if browserStatus != http.StatusOK || !strings.Contains(browserBody, "Authorization complete") {
		t.Fatalf("browser response: status=%d body=%q", browserStatus, browserBody)
	}
	if browserHeaders.Get("Cache-Control") != "no-store" || browserHeaders.Get("Content-Security-Policy") == "" {
		t.Fatalf("browser security headers = %#v", browserHeaders)
	}
	for _, secret := range []string{"browser-code", authorizationState, "loopback-secret-token"} {
		if strings.Contains(browserBody, secret) {
			t.Errorf("browser response exposed OAuth value %q", secret)
		}
	}
	assertPrivateMode(t, tokenFile)
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("consumed state file still exists: %v", err)
	}
	loaded, err := LoadToken(tokenFile, "")
	if err != nil || loaded != "loopback-secret-token" {
		t.Fatalf("loaded token mismatch: value length=%d err=%v", len(loaded), err)
	}
}

func TestOAuthLoopbackLoginRejectsMismatchedStateWithoutExchangeOrDisclosure(t *testing.T) {
	t.Parallel()
	var tokenCalls atomic.Int32
	server := newOAuthLoginTestServer(t, func(http.ResponseWriter, *http.Request) {
		tokenCalls.Add(1)
	})
	defer server.Close()

	directory := t.TempDir()
	stateFile := filepath.Join(directory, "state.json")
	tokenFile := filepath.Join(directory, "token.json")
	const wrongState = "attacker-state-value"
	var browserStatus int
	var browserBody string
	_, err := LoginOAuth(context.Background(), OAuthLoginConfig{
		Start: OAuthStartConfig{
			Endpoint: server.URL + "/mcp", AllowedLoopbackHTTP: testLoopbackHTTP(t, server.URL),
			ClientID: "public-client", StateFile: stateFile, TokenFile: tokenFile,
		},
		ListenAddress: "127.0.0.1:0",
		CallbackPath:  "/callback",
		Timeout:       2 * time.Second,
		OnAuthorizationURL: func(rawAuthorizationURL string) {
			authorizationURL, parseErr := url.Parse(rawAuthorizationURL)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			callback := authorizationURL.Query().Get("redirect_uri") + "?code=sensitive-code&state=" + wrongState
			response, getErr := http.Get(callback)
			if getErr != nil {
				t.Fatal(getErr)
			}
			defer response.Body.Close()
			body, readErr := io.ReadAll(response.Body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			browserStatus = response.StatusCode
			browserBody = string(body)
		},
	})
	if err == nil || !strings.Contains(err.Error(), "state did not match") {
		t.Fatalf("error = %v", err)
	}
	if tokenCalls.Load() != 0 {
		t.Fatal("token endpoint was called for a mismatched state")
	}
	if browserStatus != http.StatusBadRequest || !strings.Contains(browserBody, "Return to the terminal") {
		t.Fatalf("browser response: status=%d body=%q", browserStatus, browserBody)
	}
	for _, secret := range []string{"sensitive-code", wrongState, "state did not match"} {
		if strings.Contains(browserBody, secret) {
			t.Errorf("browser response exposed OAuth detail %q", secret)
		}
	}
	assertPrivateMode(t, stateFile)
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
		t.Fatalf("token file exists after rejected callback: %v", err)
	}
}

func TestOAuthLoopbackLoginTimesOutAndLeavesResumeState(t *testing.T) {
	t.Parallel()
	server := newOAuthLoginTestServer(t, nil)
	defer server.Close()

	directory := t.TempDir()
	stateFile := filepath.Join(directory, "state.json")
	tokenFile := filepath.Join(directory, "token.json")
	started := time.Now()
	_, err := LoginOAuth(context.Background(), OAuthLoginConfig{
		Start: OAuthStartConfig{
			Endpoint: server.URL + "/mcp", AllowedLoopbackHTTP: testLoopbackHTTP(t, server.URL),
			ClientID: "public-client", StateFile: stateFile, TokenFile: tokenFile,
		},
		ListenAddress: "127.0.0.1:0",
		CallbackPath:  "/callback",
		Timeout:       100 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bounded callback wait took %s", elapsed)
	}
	assertPrivateMode(t, stateFile)
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
		t.Fatalf("token file exists after timeout: %v", err)
	}
}

func TestOAuthLoopbackLoginRejectsUnsafeListenerBeforeOAuthRequests(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	_, err := LoginOAuth(context.Background(), OAuthLoginConfig{
		Start: OAuthStartConfig{
			Endpoint: server.URL + "/mcp", AllowedLoopbackHTTP: testLoopbackHTTP(t, server.URL),
			StateFile: filepath.Join(t.TempDir(), "state.json"),
		},
		ListenAddress: "0.0.0.0:0",
		CallbackPath:  "/callback",
		Timeout:       time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "must bind") {
		t.Fatalf("error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatal("OAuth request occurred before unsafe listener rejection")
	}
}

func TestListenLoopbackPreservesLocalhostInRedirectURI(t *testing.T) {
	t.Parallel()
	listener, redirectURI, err := listenLoopback("localhost:0", "/callback")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	parsed, err := url.Parse(redirectURI)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Hostname() != "localhost" || parsed.Port() == "" || parsed.Port() == "0" || parsed.Path != "/callback" {
		t.Fatalf("redirect URI = %q", redirectURI)
	}
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok || !tcpAddress.IP.IsLoopback() {
		t.Fatalf("listener address = %v", listener.Addr())
	}
}

func TestValidateOAuthCallbackPath(t *testing.T) {
	t.Parallel()
	for _, invalid := range []string{"callback", "//callback", "/a/../callback", "/callback?next=/", "/callback#fragment", "/callback%2fother"} {
		if _, err := validateCallbackPath(invalid); err == nil {
			t.Errorf("callback path %q was accepted", invalid)
		}
	}
	if value, err := validateCallbackPath(""); err != nil || value != "/callback" {
		t.Fatalf("default callback path = %q, %v", value, err)
	}
}

func newOAuthLoginTestServer(t *testing.T, tokenHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/oauth-authorization-server":
			writeJSONTest(writer, map[string]any{
				"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize",
				"token_endpoint":                        server.URL + "/token",
				"code_challenge_methods_supported":      []string{"S256"},
				"token_endpoint_auth_methods_supported": []string{"none"},
			})
		case "/token":
			if tokenHandler == nil {
				writeJSONTest(writer, map[string]any{"access_token": "unused", "token_type": "Bearer"})
				return
			}
			tokenHandler(writer, request)
		default:
			http.NotFound(writer, request)
		}
	}))
	return server
}
