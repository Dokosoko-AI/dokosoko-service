package acceptance

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OAuthLoginConfig configures an interactive OAuth flow whose callback is
// received by a temporary HTTP listener bound only to the local loopback
// interface. Start.RedirectURI is ignored and replaced with the listener URI.
type OAuthLoginConfig struct {
	Start              OAuthStartConfig
	ListenAddress      string
	CallbackPath       string
	Timeout            time.Duration
	OnAuthorizationURL func(string)
}

type oauthLoginCompletion struct {
	result OAuthFinishResult
	err    error
}

// LoginOAuth starts a loopback callback listener, prepares the existing
// authorization-code flow, presents its authorization URL, and waits for one
// exact callback. FinishOAuth performs state validation and the token exchange.
func LoginOAuth(ctx context.Context, config OAuthLoginConfig) (OAuthFinishResult, error) {
	if config.Timeout <= 0 {
		return OAuthFinishResult{}, errors.New("OAuth login timeout must be positive")
	}
	callbackPath, err := validateCallbackPath(config.CallbackPath)
	if err != nil {
		return OAuthFinishResult{}, err
	}
	listener, redirectURI, err := listenLoopback(config.ListenAddress, callbackPath)
	if err != nil {
		return OAuthFinishResult{}, err
	}
	defer listener.Close()

	startConfig := config.Start
	startConfig.RedirectURI = redirectURI
	if startConfig.TokenFile == "" {
		startConfig.TokenFile = "mcp-token.json"
	}
	started, err := StartOAuth(ctx, startConfig)
	if err != nil {
		return OAuthFinishResult{}, err
	}

	waitContext, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	completion := make(chan oauthLoginCompletion, 1)
	var handleOnce sync.Once
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setOAuthBrowserHeaders(writer.Header())
		if request.URL.Path != callbackPath {
			http.Error(writer, "Not found.", http.StatusNotFound)
			return
		}
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			http.Error(writer, "Method not allowed.", http.StatusMethodNotAllowed)
			return
		}
		handled := false
		handleOnce.Do(func() {
			handled = true
			callbackURL := redirectURI
			if request.URL.RawQuery != "" {
				callbackURL += "?" + request.URL.RawQuery
			}
			result, finishErr := FinishOAuth(waitContext, OAuthFinishConfig{
				StateFile:   startConfig.StateFile,
				TokenFile:   startConfig.TokenFile,
				CallbackURL: callbackURL,
				HTTPClient:  startConfig.HTTPClient,
				Now:         startConfig.Now,
			})
			if finishErr != nil {
				writeOAuthBrowserResult(writer, false)
			} else {
				writeOAuthBrowserResult(writer, true)
			}
			completion <- oauthLoginCompletion{result: result, err: finishErr}
		})
		if !handled {
			http.Error(writer, "OAuth callback was already handled.", http.StatusConflict)
		}
	})
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       5 * time.Second,
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErr := server.Serve(listener)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			serveErrors <- serveErr
		}
	}()
	defer func() {
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownContext)
	}()

	if config.OnAuthorizationURL != nil {
		config.OnAuthorizationURL(started.AuthorizationURL)
	}
	select {
	case finished := <-completion:
		return finished.result, finished.err
	case serveErr := <-serveErrors:
		return OAuthFinishResult{}, fmt.Errorf("OAuth loopback callback listener failed: %w", serveErr)
	case <-waitContext.Done():
		if errors.Is(waitContext.Err(), context.DeadlineExceeded) {
			return OAuthFinishResult{}, fmt.Errorf("OAuth callback timed out after %s", config.Timeout)
		}
		return OAuthFinishResult{}, fmt.Errorf("OAuth login canceled: %w", waitContext.Err())
	}
}

func validateCallbackPath(value string) (string, error) {
	if value == "" {
		value = "/callback"
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#") || parsed.Path != value || parsed.RawQuery != "" || parsed.Fragment != "" || path.Clean(value) != value {
		return "", errors.New("callback path must be a clean absolute URL path without escaping, a query, or a fragment")
	}
	return value, nil
}

func listenLoopback(address, callbackPath string) (net.Listener, string, error) {
	if address == "" {
		address = "127.0.0.1:0"
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return nil, "", errors.New("listen address must be localhost, 127.0.0.1, or [::1] with a port")
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return nil, "", errors.New("listen address port must be a number from 0 through 65535")
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || (!ip.Equal(net.IPv4(127, 0, 0, 1)) && !ip.Equal(net.IPv6loopback)) {
			return nil, "", errors.New("OAuth callback listener must bind to localhost, 127.0.0.1, or ::1")
		}
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, "", fmt.Errorf("start OAuth loopback callback listener: %w", err)
	}
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok || !tcpAddress.IP.IsLoopback() {
		_ = listener.Close()
		return nil, "", errors.New("OAuth callback listener did not bind to a loopback address")
	}
	redirectHost := tcpAddress.IP.String()
	if host == "localhost" {
		redirectHost = "localhost"
	}
	redirect := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(redirectHost, strconv.Itoa(tcpAddress.Port)),
		Path:   callbackPath,
	}
	return listener, redirect.String(), nil
}

func setOAuthBrowserHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}

func writeOAuthBrowserResult(writer http.ResponseWriter, succeeded bool) {
	if succeeded {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("<!doctype html><html><head><meta charset=\"utf-8\"><title>OAuth complete</title></head><body><main>Authorization complete. You can close this tab and return to the terminal.</main></body></html>"))
		return
	}
	writer.WriteHeader(http.StatusBadRequest)
	_, _ = writer.Write([]byte("<!doctype html><html><head><meta charset=\"utf-8\"><title>OAuth failed</title></head><body><main>Authorization could not be completed. Return to the terminal for details.</main></body></html>"))
}
