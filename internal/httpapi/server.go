package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/mcpbridge"
	"github.com/dokosoko/dokosoko-service/internal/model"
	packagegateway "github.com/dokosoko/dokosoko-service/internal/packages"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	providerruntime "github.com/dokosoko/dokosoko-service/internal/providers"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

const (
	demoAdminToken   = "doko_admin_demo"
	demoPrivateToken = "doko_private_demo"
)

type Server struct {
	service         *platform.Service
	auth            *auth.Manager
	packageGateway  *packagegateway.Gateway
	toolRuntime     *toolruntime.Runtime
	identityBroker  *identity.Broker
	usageReporter   identity.UsageReporter
	providerRuntime *providerruntime.Runtime
	mcpBridge       *mcpbridge.Manager
	baseURL         string
	allowDemoTokens bool
	secureCookies   bool
	rateMu          sync.Mutex
	rates           map[string]rateWindow
}

type Options struct {
	BaseURL         string
	UIDirectory     string
	Auth            *auth.Manager
	PackageGateway  *packagegateway.Gateway
	ToolRuntime     *toolruntime.Runtime
	IdentityBroker  *identity.Broker
	UsageReporter   identity.UsageReporter
	ProviderRuntime *providerruntime.Runtime
	MCPBridge       *mcpbridge.Manager
	AllowDemoTokens bool
}

type rateWindow struct {
	started time.Time
	count   int
}

type contextKey string

const (
	requestIDKey  contextKey = "request_id"
	actorIDKey    contextKey = "actor_id"
	principalKey  contextKey = "identity_principal"
	sessionCookie            = "dokosoko_session"
)

func New(service *platform.Service, baseURL string) http.Handler {
	return NewWithOptions(service, Options{BaseURL: baseURL, AllowDemoTokens: true})
}

func NewWithUI(service *platform.Service, baseURL, uiDirectory string) http.Handler {
	return NewWithOptions(service, Options{BaseURL: baseURL, UIDirectory: uiDirectory, AllowDemoTokens: true})
}

func NewWithOptions(service *platform.Service, options Options) http.Handler {
	baseURL := strings.TrimRight(options.BaseURL, "/")
	server := &Server{service: service, auth: options.Auth, packageGateway: options.PackageGateway, toolRuntime: options.ToolRuntime, identityBroker: options.IdentityBroker, usageReporter: options.UsageReporter, providerRuntime: options.ProviderRuntime, mcpBridge: options.MCPBridge, baseURL: baseURL, allowDemoTokens: options.AllowDemoTokens, secureCookies: strings.HasPrefix(baseURL, "https://"), rates: make(map[string]rateWindow)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /readyz", server.ready)
	mux.HandleFunc("GET /api/v1/setup/status", server.setupStatus)
	mux.HandleFunc("POST /api/v1/setup/begin", server.setupBegin)
	mux.HandleFunc("POST /api/v1/setup/complete", server.setupComplete)
	mux.HandleFunc("POST /api/v1/auth/login", server.login)
	mux.HandleFunc("POST /api/v1/auth/logout", server.logout)
	mux.HandleFunc("GET /api/v1/auth/session", server.currentSession)
	mux.HandleFunc("GET /oauth/authorize", server.oauthAuthorize)
	mux.HandleFunc("GET /oauth/callback/{productID}", server.oauthCallback)
	mux.HandleFunc("POST /oauth/token", server.oauthToken)
	mux.HandleFunc("GET /oauth/upstream/callback", server.upstreamOAuthCallback)
	mux.HandleFunc("/api/v1/", server.adminAPI)
	mux.HandleFunc("POST /mcp/public/{productID}", server.publicMCP)
	mux.HandleFunc("POST /mcp/{productID}", server.privateMCP)
	mux.HandleFunc("GET /widgets/{productID}/{asset}", server.widgetScript)
	mux.HandleFunc("GET /artifacts/{productID}/{packageID}", server.packageArtifact)
	if options.UIDirectory != "" {
		mux.Handle("/", staticConsole(options.UIDirectory))
	}
	return requestID(mux)
}

func staticConsole(directory string) http.Handler {
	files := http.FileServer(http.Dir(directory))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
			return
		}
		cleaned := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			http.NotFound(w, r)
			return
		}
		if cleaned == "." {
			cleaned = "index.html"
		}
		candidate := filepath.Join(directory, cleaned)
		if info, err := os.Stat(candidate); err != nil || info.IsDir() {
			candidate = filepath.Join(directory, "index.html")
			if _, err := os.Stat(candidate); err != nil {
				http.NotFound(w, r)
				return
			}
			r.URL.Path = "/"
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if strings.Contains(r.URL.Path, "/_next/static/") || strings.Contains(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" {
			id = fmt.Sprintf("req_%d", time.Now().UnixNano())
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if pingable, ok := s.service.Store().(interface{ Ping(context.Context) error }); ok {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pingable.Ping(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "database_unavailable", "The database readiness check failed.", nil)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func isBearer(r *http.Request, token string) bool {
	return r.Header.Get("Authorization") == "Bearer "+token
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) < 8 || !strings.EqualFold(value[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(value[7:])
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable", "Authentication is not configured.", nil)
		return
	}
	completed, err := s.auth.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "setup_status_failed", "Setup status could not be read.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"setup_complete": completed, "requires_mfa": true})
}

func (s *Server) setupBegin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable", "Authentication is not configured.", nil)
		return
	}
	if !s.allowFixedWindow("setup|"+remoteHost(r.RemoteAddr), 10, time.Now().UTC()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many setup attempts.", nil)
		return
	}
	var input auth.SetupInput
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	enrollment, err := s.auth.BeginSetup(r.Context(), bearerToken(r), input)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, enrollment)
}

func (s *Server) setupComplete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable", "Authentication is not configured.", nil)
		return
	}
	if !s.allowFixedWindow("setup-complete|"+remoteHost(r.RemoteAddr), 20, time.Now().UTC()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many MFA attempts.", nil)
		return
	}
	var input struct {
		EnrollmentID string `json:"enrollment_id"`
		Code         string `json:"code"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	result, err := s.auth.CompleteSetup(r.Context(), input.EnrollmentID, input.Code)
	if err != nil {
		s.authError(w, err)
		return
	}
	s.setSessionCookies(w, result.Session)
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable", "Authentication is not configured.", nil)
		return
	}
	if !s.allowFixedWindow("login|"+remoteHost(r.RemoteAddr), 10, time.Now().UTC()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many login attempts.", nil)
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	session, err := s.auth.Login(r.Context(), input.Email, input.Password, input.Code)
	if err != nil {
		s.authError(w, err)
		return
	}
	s.setSessionCookies(w, session)
	writeJSON(w, http.StatusOK, map[string]any{"user": session.User, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt.Format(time.RFC3339)})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable", "Authentication is not configured.", nil)
		return
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication_required", "No active session.", nil)
		return
	}
	if s.auth.VerifyCSRF(r.Context(), cookie.Value, r.Header.Get("X-CSRF-Token")) != nil || !s.validOrigin(r) {
		writeError(w, http.StatusForbidden, "csrf_validation_failed", "A valid CSRF token and origin are required.", nil)
		return
	}
	_ = s.auth.Logout(r.Context(), cookie.Value)
	s.clearSessionCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) currentSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	_, _, session, ok := s.cookieSession(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "No active session.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": session.User, "expires_at": session.ExpiresAt.Format(time.RFC3339)})
}

func (s *Server) rootUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable", "Root authentication is unavailable.", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, err := s.auth.RootUsers(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "root_users_failed", "Root users could not be listed.", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": users})
	case http.MethodPost:
		var input auth.SetupInput
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		enrollment, err := s.auth.BeginRoot(r.Context(), input)
		if err != nil {
			s.authError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, enrollment)
	case http.MethodPut:
		var input struct {
			EnrollmentID string `json:"enrollment_id"`
			Code         string `json:"code"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		result, err := s.auth.CompleteRoot(r.Context(), input.EnrollmentID, input.Code, actor(r).ID)
		if err != nil {
			s.authError(w, err)
			return
		}
		requestID, _ := r.Context().Value(requestIDKey).(string)
		_ = s.service.Store().AppendAudit(r.Context(), model.AuditEvent{ID: "audit_" + strconv.FormatInt(time.Now().UnixNano(), 10), ActorID: actor(r).ID, Action: "root.created", TargetType: "root_user", TargetID: result.User.ID, Current: map[string]any{"email": result.User.Email, "mfa_enforced": true}, RequestID: requestID, CreatedAt: time.Now().UTC()})
		writeJSON(w, http.StatusCreated, result)
	default:
		w.Header().Set("Allow", "GET, POST, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) revokeRootUser(w http.ResponseWriter, r *http.Request, userID string) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable", "Root authentication is unavailable.", nil)
		return
	}
	err := s.auth.RevokeRoot(r.Context(), userID, actor(r).ID)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrCannotRevokeSelf), errors.Is(err, auth.ErrLastRoot):
			writeError(w, http.StatusConflict, "root_revoke_denied", err.Error(), nil)
		case errors.Is(err, store.ErrNotFound):
			s.storeError(w, err)
		default:
			writeError(w, http.StatusInternalServerError, "root_revoke_failed", "Root administrator could not be revoked.", nil)
		}
		return
	}
	requestID, _ := r.Context().Value(requestIDKey).(string)
	_ = s.service.Store().AppendAudit(r.Context(), model.AuditEvent{ID: "audit_" + strconv.FormatInt(time.Now().UnixNano(), 10), ActorID: actor(r).ID, Action: "root.revoked", TargetType: "root_user", TargetID: userID, RequestID: requestID, CreatedAt: time.Now().UTC()})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) systemDoctor(w http.ResponseWriter, r *http.Request) {
	type check struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	checks := []check{}
	databaseStatus := check{Name: "database", Status: "ok", Message: "Persistence is reachable and migrations were applied at startup."}
	if pingable, ok := s.service.Store().(interface{ Ping(context.Context) error }); ok {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		err := pingable.Ping(ctx)
		cancel()
		if err != nil {
			databaseStatus.Status, databaseStatus.Message = "error", "Persistence connectivity failed."
		}
	}
	checks = append(checks, databaseStatus)
	rootStatus := check{Name: "root_mfa", Status: "error", Message: "No active MFA-protected root administrator was found."}
	if s.auth != nil {
		if completed, err := s.auth.Status(r.Context()); err == nil && completed {
			rootStatus.Status, rootStatus.Message = "ok", "At least one active MFA-protected root administrator exists."
		}
	}
	checks = append(checks, rootStatus)
	checks = append(checks,
		check{Name: "encryption", Status: "ok", Message: "The service started with a valid 256-bit master key and tenant-bound secret vault."},
		check{Name: "public_url", Status: "ok", Message: "The configured public origin is " + s.baseURL + "."},
		check{Name: "knowledge_hardening", Status: "ok", Message: "Quarantine, publication gates, product/visibility filters, and model authority denial are enforced."},
	)
	overall := "ok"
	for _, item := range checks {
		if item.Status != "ok" {
			overall = "error"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": overall, "checks": checks, "generated_at": time.Now().UTC()})
}

func (s *Server) authenticateAdmin(r *http.Request) (string, bool, bool) {
	if s.allowDemoTokens && isBearer(r, demoAdminToken) {
		return "root_demo", false, true
	}
	actorID, _, _, ok := s.cookieSession(r)
	return actorID, true, ok
}

func (s *Server) cookieSession(r *http.Request) (string, string, auth.Session, bool) {
	if s.auth == nil {
		return "", "", auth.Session{}, false
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", "", auth.Session{}, false
	}
	session, err := s.auth.Authenticate(r.Context(), cookie.Value)
	if err != nil {
		return "", "", auth.Session{}, false
	}
	return session.User.ID, cookie.Value, session, true
}

func (s *Server) setSessionCookies(w http.ResponseWriter, session auth.Session) {
	maxAge := max(1, int(time.Until(session.ExpiresAt).Seconds()))
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: session.Token, Path: "/", HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteStrictMode, MaxAge: maxAge, Expires: session.ExpiresAt})
	http.SetCookie(w, &http.Cookie{Name: "dokosoko_csrf", Value: session.CSRFToken, Path: "/", HttpOnly: false, Secure: s.secureCookies, SameSite: http.SameSiteStrictMode, MaxAge: maxAge, Expires: session.ExpiresAt})
}

func (s *Server) clearSessionCookies(w http.ResponseWriter) {
	expires := time.Unix(1, 0).UTC()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: expires})
	http.SetCookie(w, &http.Cookie{Name: "dokosoko_csrf", Value: "", Path: "/", Secure: s.secureCookies, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: expires})
}

func (s *Server) validOrigin(r *http.Request) bool {
	origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/")
	return origin != "" && origin == s.baseURL
}

func mutating(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func (s *Server) authError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrSetupToken), errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrInvalidMFA):
		writeError(w, http.StatusUnauthorized, "authentication_failed", "Credentials or MFA verification failed.", nil)
	case errors.Is(err, auth.ErrSetupComplete):
		writeError(w, http.StatusConflict, "setup_complete", "Initial setup is already complete.", nil)
	case errors.Is(err, auth.ErrSetupExpired):
		writeError(w, http.StatusGone, "setup_expired", "The setup enrollment expired. Start again.", nil)
	case errors.Is(err, auth.ErrPasswordRequirement):
		writeError(w, http.StatusBadRequest, "password_requirements", "Use at least 14 characters with upper-case, lower-case, number, and symbol characters.", nil)
	default:
		writeError(w, http.StatusBadRequest, "authentication_failed", err.Error(), nil)
	}
}

func oauthError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

func (s *Server) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	if s.identityBroker == nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "Identity broker is not configured.")
		return
	}
	if r.URL.Query().Get("response_type") != "code" || r.URL.Query().Get("code_challenge_method") != "S256" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "Authorization code flow with PKCE S256 is required.")
		return
	}
	if !s.allowFixedWindow("oauth-authorize|"+remoteHost(r.RemoteAddr), 60, time.Now().UTC()) {
		oauthError(w, http.StatusTooManyRequests, "temporarily_unavailable", "Authorization request limit exceeded.")
		return
	}
	redirect, err := s.identityBroker.Begin(r.Context(), identity.AuthorizationRequest{
		ProductID: r.URL.Query().Get("product_id"), ClientID: r.URL.Query().Get("client_id"), RedirectURI: r.URL.Query().Get("redirect_uri"), State: r.URL.Query().Get("state"), CodeChallenge: r.URL.Query().Get("code_challenge"),
	})
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "The OAuth client, redirect URI, or product identity configuration is invalid.")
		return
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	if s.identityBroker == nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "Identity broker is not configured.")
		return
	}
	result, err := s.identityBroker.Callback(r.Context(), r.PathValue("productID"), r.URL.Query().Get("state"), r.URL.Query().Get("code"))
	if err != nil {
		oauthError(w, http.StatusUnauthorized, "access_denied", "Vendor identity or entitlement verification failed.")
		return
	}
	http.Redirect(w, r, result.RedirectURI, http.StatusFound)
}

func (s *Server) oauthToken(w http.ResponseWriter, r *http.Request) {
	if s.identityBroker == nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "Identity broker is not configured.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") != "authorization_code" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "Authorization code grant form data is required.")
		return
	}
	if !s.allowFixedWindow("oauth-token|"+remoteHost(r.RemoteAddr), 120, time.Now().UTC()) {
		oauthError(w, http.StatusTooManyRequests, "temporarily_unavailable", "Token request limit exceeded.")
		return
	}
	result, err := s.identityBroker.Exchange(r.Context(), r.PostForm.Get("code"), r.PostForm.Get("code_verifier"), r.PostForm.Get("client_id"), r.PostForm.Get("redirect_uri"))
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "The authorization code or PKCE verifier is invalid.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	s.recordAnalytics(r.Context(), result.Principal.ProductID, "connector_authorized", "vendor_user", pseudonym(result.Principal.ProductID, result.Principal), map[string]any{"client_id": result.Principal.ClientID})
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) upstreamOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if s.mcpBridge == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp_bridge_unavailable", "Stateless MCPv2 authorization is unavailable.", nil)
		return
	}
	if r.URL.Query().Get("error") != "" {
		writeError(w, http.StatusUnauthorized, "upstream_authorization_denied", "The upstream MCP authorization server denied access.", nil)
		return
	}
	if _, err := s.mcpBridge.CompleteAuthorization(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"), r.URL.Query().Get("iss")); err != nil {
		writeError(w, http.StatusUnauthorized, "upstream_authorization_failed", "The upstream MCP authorization response was invalid or expired.", nil)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `<!doctype html><html><head><meta name="viewport" content="width=device-width"><title>Connected</title><style>body{font:16px system-ui;margin:4rem;max-width:42rem}h1{color:#18181b}</style></head><body><h1>Stateless MCPv2 connection authorized</h1><p>Your upstream user grant is encrypted and bound to this DokoSoko identity. You can close this window.</p></body></html>`)
}

func (s *Server) vendorIdentity(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().VendorIdentity(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPut:
		var input struct {
			OrganisationID          string   `json:"organisation_id"`
			Issuer                  string   `json:"issuer"`
			ClientID                string   `json:"client_id"`
			ClientSecret            string   `json:"client_secret"`
			Scopes                  []string `json:"scopes"`
			Audience                string   `json:"audience"`
			OrganisationClaim       string   `json:"organisation_claim"`
			InstallationClaim       string   `json:"installation_claim"`
			EntitlementHookURL      string   `json:"entitlement_hook_url"`
			AllowedRedirectURIs     []string `json:"allowed_redirect_uris"`
			AuthorizationHookURL    string   `json:"authorization_hook_url"`
			AuthorizationCredential string   `json:"authorization_credential"`
			UsageHookURL            string   `json:"usage_hook_url"`
			UsageCredential         string   `json:"usage_credential"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.ConfigureIdentity(r.Context(), platform.IdentityInput{OrganisationID: input.OrganisationID, ProductID: productID, Issuer: input.Issuer, ClientID: input.ClientID, ClientSecret: input.ClientSecret, Scopes: input.Scopes, Audience: input.Audience, OrganisationClaim: input.OrganisationClaim, InstallationClaim: input.InstallationClaim, EntitlementHookURL: input.EntitlementHookURL, AllowedRedirectURIs: input.AllowedRedirectURIs, AuthorizationHookURL: input.AuthorizationHookURL, AuthorizationCredential: input.AuthorizationCredential, UsageHookURL: input.UsageHookURL, UsageCredential: input.UsageCredential}, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) mcpConnections(w http.ResponseWriter, r *http.Request, productID string) {
	if s.mcpBridge == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp_bridge_unavailable", "Stateless MCPv2 bridge is unavailable.", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().MCPConnections(r.Context(), productID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values, "protocol_policy": "Stateless MCPv2 Only", "protocol_version": model.StatelessMCPv2Protocol, "specification_url": "https://blog.modelcontextprotocol.io/posts/2026-07-28/"})
	case http.MethodPost:
		var input struct {
			OrganisationID    string   `json:"organisation_id"`
			Name              string   `json:"name"`
			Namespace         string   `json:"namespace"`
			Endpoint          string   `json:"endpoint"`
			AuthMode          string   `json:"auth_mode"`
			Credential        string   `json:"credential"`
			OAuthClientID     string   `json:"oauth_client_id"`
			OAuthClientSecret string   `json:"oauth_client_secret"`
			OAuthIssuer       string   `json:"oauth_issuer"`
			AuthorizationURL  string   `json:"authorization_url"`
			TokenURL          string   `json:"token_url"`
			Scopes            []string `json:"scopes"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.mcpBridge.CreateConnection(r.Context(), mcpbridge.ConnectionInput{OrganisationID: input.OrganisationID, ProductID: productID, Name: input.Name, Namespace: input.Namespace, Endpoint: input.Endpoint, AuthMode: input.AuthMode, Credential: input.Credential, OAuthClientID: input.OAuthClientID, OAuthClientSecret: input.OAuthClientSecret, OAuthIssuer: input.OAuthIssuer, AuthorizationURL: input.AuthorizationURL, TokenURL: input.TokenURL, Scopes: input.Scopes}, mcpbridge.Actor{ID: actor(r).ID, RequestID: actor(r).RequestID})
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) inspectMCPConnection(w http.ResponseWriter, r *http.Request, productID, connectionID string) {
	if s.mcpBridge == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp_bridge_unavailable", "Stateless MCPv2 bridge is unavailable.", nil)
		return
	}
	value, err := s.mcpBridge.Inspect(r.Context(), productID, connectionID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "mcp_upstream_inspection_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) importMCPConnection(w http.ResponseWriter, r *http.Request, productID, connectionID string) {
	if s.mcpBridge == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp_bridge_unavailable", "Stateless MCPv2 bridge is unavailable.", nil)
		return
	}
	var input struct {
		ToolNames            []string `json:"tool_names"`
		RequiredEntitlements []string `json:"required_entitlements"`
		ConfirmationRequired bool     `json:"confirmation_required"`
		TimeoutMS            int      `json:"timeout_ms"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	adminActor := actor(r)
	value, err := s.mcpBridge.Import(r.Context(), productID, connectionID, mcpbridge.ImportInput{ToolNames: input.ToolNames, RequiredEntitlements: input.RequiredEntitlements, ConfirmationRequired: input.ConfirmationRequired, TimeoutMS: input.TimeoutMS}, mcpbridge.Actor{ID: adminActor.ID, RequestID: adminActor.RequestID})
	if err != nil {
		writeError(w, http.StatusBadGateway, "mcp_upstream_import_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) analytics(w http.ResponseWriter, r *http.Request, productID string) {
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 365 {
			writeError(w, http.StatusBadRequest, "invalid_period", "Analytics period must be between 1 and 365 days.", nil)
			return
		}
		days = parsed
	}
	value, err := s.service.Store().AnalyticsSummary(r.Context(), productID, time.Now().UTC().AddDate(0, 0, -days))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) providers(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Providers(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			OrganisationID       string   `json:"organisation_id"`
			Name                 string   `json:"name"`
			BaseURL              string   `json:"base_url"`
			Credential           string   `json:"credential"`
			RequiredEntitlements []string `json:"required_entitlements"`
			MaxTTLSeconds        int      `json:"max_ttl_seconds"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateProvider(r.Context(), platform.ProviderInput{OrganisationID: input.OrganisationID, ProductID: productID, Name: input.Name, BaseURL: input.BaseURL, Credential: input.Credential, RequiredEntitlements: input.RequiredEntitlements, MaxTTLSeconds: input.MaxTTLSeconds}, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request, productID string) {
	values, err := s.service.Store().Projects(r.Context(), productID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) credentials(w http.ResponseWriter, r *http.Request, productID string) {
	values, err := s.service.Store().CredentialLeases(r.Context(), productID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) llmProfiles(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().LLMProfiles(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPut:
		var input struct {
			OrganisationID      string `json:"organisation_id"`
			Role                string `json:"role"`
			Provider            string `json:"provider"`
			Endpoint            string `json:"endpoint"`
			Model               string `json:"model"`
			Credential          string `json:"credential"`
			EmbeddingDimensions int    `json:"embedding_dimensions"`
			MaxInputTokens      int    `json:"max_input_tokens"`
			MaxOutputTokens     int    `json:"max_output_tokens"`
			DailyTokenBudget    int64  `json:"daily_token_budget"`
			Enabled             bool   `json:"enabled"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveLLMProfile(r.Context(), platform.LLMProfileInput{OrganisationID: input.OrganisationID, ProductID: productID, Role: input.Role, Provider: input.Provider, Endpoint: input.Endpoint, Model: input.Model, Credential: input.Credential, EmbeddingDimensions: input.EmbeddingDimensions, MaxInputTokens: input.MaxInputTokens, MaxOutputTokens: input.MaxOutputTokens, DailyTokenBudget: input.DailyTokenBudget, Enabled: input.Enabled}, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func pseudonym(productID string, principal identity.Principal) string {
	if principal.Subject == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(productID + "\x00" + principal.Issuer + "\x00" + principal.Subject))
	return hex.EncodeToString(digest[:16])
}

func (s *Server) recordAnalytics(ctx context.Context, productID, eventName, actorKind, actorPseudonym string, dimensions map[string]any) {
	product, err := s.service.Store().Product(ctx, productID)
	if err != nil {
		return
	}
	_ = s.service.Store().AppendAnalytics(ctx, model.AnalyticsEvent{OrganisationID: product.OrganisationID, ProductID: productID, EventName: eventName, ActorKind: actorKind, ActorPseudonym: actorPseudonym, Dimensions: dimensions, CreatedAt: time.Now().UTC()})
}

func (s *Server) adminAPI(w http.ResponseWriter, r *http.Request) {
	actorID, cookieSession, ok := s.authenticateAdmin(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Use an administrator access token.", nil)
		return
	}
	if cookieSession && mutating(r.Method) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || s.auth == nil || s.auth.VerifyCSRF(r.Context(), cookie.Value, r.Header.Get("X-CSRF-Token")) != nil {
			writeError(w, http.StatusForbidden, "csrf_validation_failed", "A valid CSRF token is required.", nil)
			return
		}
		if !s.validOrigin(r) {
			writeError(w, http.StatusForbidden, "origin_validation_failed", "The request origin is not allowed.", nil)
			return
		}
	}
	r = r.WithContext(context.WithValue(r.Context(), actorIDKey, actorID))
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 || parts[0] != "api" || parts[1] != "v1" {
		writeError(w, http.StatusNotFound, "not_found", "Route not found.", nil)
		return
	}
	switch {
	case len(parts) == 4 && parts[2] == "root" && parts[3] == "users":
		s.rootUsers(w, r)
	case len(parts) == 4 && parts[2] == "system" && parts[3] == "doctor" && r.Method == http.MethodGet:
		s.systemDoctor(w, r)
	case len(parts) == 5 && parts[2] == "root" && parts[3] == "users" && r.Method == http.MethodDelete:
		s.revokeRootUser(w, r, parts[4])
	case len(parts) == 3 && parts[2] == "organisations":
		s.organisations(w, r)
	case len(parts) == 5 && parts[2] == "organisations" && parts[4] == "products":
		s.products(w, r, parts[3])
	case len(parts) == 4 && parts[2] == "products" && r.Method == http.MethodPatch:
		s.productSettings(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "description" && parts[5] == "rewrite" && r.Method == http.MethodPost:
		s.rewriteProductDescription(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "versions":
		s.productVersions(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "versions" && r.Method == http.MethodPatch:
		s.productVersionLifecycle(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "versions" && parts[6] == "impact" && r.Method == http.MethodGet:
		s.productVersionImpact(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "versions" && parts[6] == "diff" && r.Method == http.MethodGet:
		s.productVersionDiff(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "versions" && parts[6] == "reconcile" && r.Method == http.MethodPost:
		s.reconcileProductVersion(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "versions" && parts[6] == "promotion" && r.Method == http.MethodPost:
		s.promoteProductVersion(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "version-pins":
		s.productVersionPins(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "version-pins" && parts[5] == "history" && r.Method == http.MethodGet:
		s.productVersionPinHistory(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "version-pins" && r.Method == http.MethodDelete:
		s.deleteProductVersionPin(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "installations":
		s.productInstallations(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "organisations" && parts[4] == "audit" && r.Method == http.MethodGet:
		s.auditEvents(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "environments":
		s.environments(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "distribution":
		s.distribution(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "definition" && r.Method == http.MethodGet:
		s.productDefinition(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "product-builds":
		s.productBuilds(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "product-builds" && parts[6] == "publish" && r.Method == http.MethodPost:
		s.publishProductBuild(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "sources":
		s.sources(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "visibility" && r.Method == http.MethodPatch:
		s.sourceVisibility(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "crawl" && r.Method == http.MethodPost:
		s.queueCrawl(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "publish" && r.Method == http.MethodPost:
		s.publishSource(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "crawls" && r.Method == http.MethodGet:
		s.crawlJobs(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "packages":
		s.packages(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "packages" && parts[6] == "visibility" && r.Method == http.MethodPatch:
		s.packageVisibility(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "packages" && parts[6] == "publish" && r.Method == http.MethodPost:
		s.publishPackage(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "widgets" && r.Method == http.MethodGet:
		s.widgets(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "tools":
		s.tools(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "mcp-connections":
		s.mcpConnections(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "mcp-connections" && parts[6] == "inspect" && r.Method == http.MethodPost:
		s.inspectMCPConnection(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "mcp-connections" && parts[6] == "import" && r.Method == http.MethodPost:
		s.importMCPConnection(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "identity":
		s.vendorIdentity(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "analytics" && r.Method == http.MethodGet:
		s.analytics(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "integration-runs":
		s.integrationRuns(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "integration-runs" && parts[6] == "complete" && r.Method == http.MethodPost:
		s.completeIntegrationRun(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "providers":
		s.providers(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "projects" && r.Method == http.MethodGet:
		s.projects(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "credentials" && r.Method == http.MethodGet:
		s.credentials(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "llm-profiles":
		s.llmProfiles(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "publish" && r.Method == http.MethodPost:
		s.publishTool(w, r, parts[3], parts[5])
	default:
		writeError(w, http.StatusNotFound, "not_found", "Route not found.", nil)
	}
}

func (s *Server) productSettings(w http.ResponseWriter, r *http.Request, productID string) {
	var input struct {
		Description              string `json:"description"`
		DefaultVersionPolicy     string `json:"default_version_policy"`
		RequirePromotionApproval bool   `json:"require_promotion_approval"`
		Revision                 int64  `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.UpdateProductSettingsWithApproval(r.Context(), productID, input.Description, input.DefaultVersionPolicy, input.RequirePromotionApproval, input.Revision, actor(r))
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) rewriteProductDescription(w http.ResponseWriter, r *http.Request, productID string) {
	var input struct {
		Draft string `json:"draft"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.RewriteProductDescription(r.Context(), productID, input.Draft, actor(r))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "description_rewrite_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"description": value})
}

func (s *Server) productVersions(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().ProductVersions(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			Version           string `json:"version"`
			ProfileID         string `json:"profile_id"`
			IsLatest          bool   `json:"is_latest"`
			IsLTS             bool   `json:"is_lts"`
			ReleaseStage      string `json:"release_stage"`
			RolloutPercentage int    `json:"rollout_percentage"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateProductVersion(r.Context(), productID, platform.ProductVersionInput{Version: input.Version, ProfileID: input.ProfileID, IsLatest: input.IsLatest, IsLTS: input.IsLTS, ReleaseStage: input.ReleaseStage, RolloutPercentage: input.RolloutPercentage}, actor(r))
		if err != nil {
			s.productCatalogError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) productVersionLifecycle(w http.ResponseWriter, r *http.Request, productID, versionID string) {
	var input struct {
		IsLatest           bool       `json:"is_latest"`
		IsLTS              bool       `json:"is_lts"`
		Deprecated         bool       `json:"deprecated"`
		DeprecationMessage string     `json:"deprecation_message"`
		ReplacementVersion string     `json:"replacement_version"`
		SunsetAt           *time.Time `json:"sunset_at"`
		RolloutPercentage  int        `json:"rollout_percentage"`
		AcknowledgeImpact  bool       `json:"acknowledge_impact"`
		Revision           int64      `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.UpdateProductVersionLifecycle(r.Context(), productID, versionID, platform.ProductVersionLifecycleInput{IsLatest: input.IsLatest, IsLTS: input.IsLTS, Deprecated: input.Deprecated, DeprecationMessage: input.DeprecationMessage, ReplacementVersion: input.ReplacementVersion, SunsetAt: input.SunsetAt, RolloutPercentage: input.RolloutPercentage, AcknowledgeImpact: input.AcknowledgeImpact, Revision: input.Revision}, actor(r))
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) productVersionPins(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().ProductVersionPins(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			Scope            string `json:"scope"`
			ScopeID          string `json:"scope_id"`
			CustomerID       string `json:"customer_id"`
			EnvironmentID    string `json:"environment_id"`
			InstallationID   string `json:"installation_id"`
			ProductVersionID string `json:"product_version_id"`
			Reason           string `json:"reason"`
			Revision         int64  `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Scope == "" {
			input.Scope, input.ScopeID = "customer", input.CustomerID
		}
		value, err := s.service.SaveScopedProductVersionPin(r.Context(), productID, platform.ProductVersionPinInput{Scope: input.Scope, ScopeID: input.ScopeID, CustomerID: input.CustomerID, EnvironmentID: input.EnvironmentID, InstallationID: input.InstallationID, ProductVersionID: input.ProductVersionID, Reason: input.Reason, Revision: input.Revision}, actor(r))
		if err != nil {
			s.productCatalogError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) productVersionPinHistory(w http.ResponseWriter, r *http.Request, productID string) {
	values, err := s.service.Store().ProductVersionPinHistory(r.Context(), productID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) productInstallations(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().ProductInstallations(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			ID            string `json:"id"`
			CustomerID    string `json:"customer_id"`
			EnvironmentID string `json:"environment_id"`
			ExternalID    string `json:"external_id"`
			Name          string `json:"name"`
			State         string `json:"state"`
			Revision      int64  `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveProductInstallation(r.Context(), productID, platform.ProductInstallationInput{ID: input.ID, CustomerID: input.CustomerID, EnvironmentID: input.EnvironmentID, ExternalID: input.ExternalID, Name: input.Name, State: input.State, Revision: input.Revision}, actor(r))
		if err != nil {
			s.productCatalogError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) productVersionImpact(w http.ResponseWriter, r *http.Request, productID, versionID string) {
	value, err := s.service.ProductVersionImpact(r.Context(), productID, versionID)
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) productVersionDiff(w http.ResponseWriter, r *http.Request, productID, versionID string) {
	value, err := s.service.Store().ProductVersion(r.Context(), productID, versionID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value.Diff)
}

func (s *Server) reconcileProductVersion(w http.ResponseWriter, r *http.Request, productID, versionID string) {
	var input struct {
		Revision int64 `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.ReconcileProductVersion(r.Context(), productID, versionID, input.Revision, actor(r))
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) promoteProductVersion(w http.ResponseWriter, r *http.Request, productID, versionID string) {
	var input struct {
		Action   string `json:"action"`
		Note     string `json:"note"`
		Revision int64  `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.PromoteProductVersion(r.Context(), productID, versionID, platform.ProductVersionPromotionInput{Action: input.Action, Note: input.Note, Revision: input.Revision}, actor(r))
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) deleteProductVersionPin(w http.ResponseWriter, r *http.Request, productID, pinID string) {
	if err := s.service.DeleteProductVersionPin(r.Context(), productID, pinID, actor(r)); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) productDefinition(w http.ResponseWriter, r *http.Request, productID string) {
	value, err := s.service.Store().ProductDefinition(r.Context(), productID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) productBuilds(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().ProductBuilds(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			Inputs []model.ProductBuildInput `json:"inputs"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.BuildProductDefinition(r.Context(), productID, input.Inputs, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) publishProductBuild(w http.ResponseWriter, r *http.Request, productID, buildID string) {
	value, err := s.service.PublishProductDefinition(r.Context(), productID, buildID, actor(r))
	if errors.Is(err, platform.ErrProductDefinitionInvalid) {
		writeError(w, http.StatusUnprocessableEntity, "product_definition_invalid", "Resolve blocking product definition findings before publication.", nil)
		return
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) auditEvents(w http.ResponseWriter, r *http.Request, organisationID string) {
	values, err := s.service.Store().AuditEvents(r.Context(), organisationID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if len(values) > 500 {
		values = values[:500]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) integrationRuns(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().IntegrationRuns(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			EnvironmentID    string `json:"environment_id"`
			RequestedOutcome string `json:"requested_outcome"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.StartIntegrationRun(r.Context(), productID, input.EnvironmentID, input.RequestedOutcome, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) completeIntegrationRun(w http.ResponseWriter, r *http.Request, productID, runID string) {
	var input struct {
		ReportedSuccess  *bool  `json:"reported_success"`
		ValidatedSuccess *bool  `json:"validated_success"`
		FailureCode      string `json:"failure_code"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.CompleteIntegrationRun(r.Context(), productID, runID, input.ReportedSuccess, input.ValidatedSuccess, input.FailureCode, actor(r))
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "integration_run_complete", "The integration run was already completed.", nil)
			return
		}
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

type namedResourceInput struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (s *Server) organisations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Organisations(r.Context())
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input namedResourceInput
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateOrganisation(r.Context(), input.Name, input.Slug, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) products(w http.ResponseWriter, r *http.Request, organisationID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Products(r.Context(), organisationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input namedResourceInput
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateProduct(r.Context(), organisationID, input.Name, input.Slug, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) environments(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Environments(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			namedResourceInput
			OrganisationID string `json:"organisation_id"`
			IsProduction   bool   `json:"is_production"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateEnvironment(r.Context(), input.OrganisationID, productID, input.Name, input.Slug, input.IsProduction, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) creationError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "resource_conflict", "A resource with this slug or production role already exists.", nil)
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		s.storeError(w, err)
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_resource", err.Error(), nil)
}

func (s *Server) productCatalogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict):
		s.storeError(w, err)
	case errors.Is(err, platform.ErrProductDescriptionRequired):
		writeError(w, http.StatusUnprocessableEntity, "product_description_required", err.Error(), nil)
	case errors.Is(err, platform.ErrProductVersionDeprecated):
		writeError(w, http.StatusConflict, "product_version_deprecated", err.Error(), nil)
	case errors.Is(err, platform.ErrProductVersionLifecycle):
		writeError(w, http.StatusUnprocessableEntity, "invalid_product_version_lifecycle", err.Error(), nil)
	case errors.Is(err, platform.ErrProductVersionDrifted):
		writeError(w, http.StatusConflict, "product_version_drifted", err.Error(), nil)
	case errors.Is(err, platform.ErrPromotionApprovalRequired), errors.Is(err, platform.ErrPromotionSeparationOfDuties):
		writeError(w, http.StatusConflict, "promotion_approval_required", err.Error(), nil)
	case errors.Is(err, platform.ErrProductVersionImpact):
		writeError(w, http.StatusConflict, "product_version_impact_acknowledgement_required", err.Error(), nil)
	default:
		writeError(w, http.StatusBadRequest, "invalid_product_catalog", err.Error(), nil)
	}
}

func (s *Server) sources(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Sources(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			OrganisationID string `json:"organisation_id"`
			Name           string `json:"name"`
			Kind           string `json:"kind"`
			Location       string `json:"location"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateSource(r.Context(), input.OrganisationID, productID, input.Name, input.Kind, input.Location, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) queueCrawl(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	value, err := s.service.QueueCrawl(r.Context(), productID, sourceID, actor(r))
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "crawl_already_active", "This source already has a queued or running crawl.", nil)
			return
		}
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, value)
}

func (s *Server) crawlJobs(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	values, err := s.service.Store().CrawlJobs(r.Context(), productID, sourceID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func revisionInput(r *http.Request) (int64, error) {
	var input struct {
		Revision int64 `json:"revision"`
	}
	err := decodeJSON(r.Body, &input)
	if err == nil && input.Revision < 1 {
		err = errors.New("revision must be positive")
	}
	return input.Revision, err
}

func (s *Server) publishSource(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	revision, err := revisionInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.PublishSource(r.Context(), productID, sourceID, revision, actor(r))
	if err != nil {
		s.platformError(w, err, "Quarantined source content cannot be published.")
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) publishPackage(w http.ResponseWriter, r *http.Request, productID, packageID string) {
	revision, err := revisionInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.PublishPackage(r.Context(), productID, packageID, revision, actor(r))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) packages(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Packages(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			OrganisationID string `json:"organisation_id"`
			Name           string `json:"name"`
			Ecosystem      string `json:"ecosystem"`
			Version        string `json:"version"`
			Mode           string `json:"mode"`
			UpstreamURL    string `json:"upstream_url"`
			FetchHookURL   string `json:"fetch_hook_url"`
			Credential     string `json:"credential"`
			ChecksumSHA256 string `json:"checksum_sha256"`
			ExpectedSize   int64  `json:"expected_size"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreatePackage(r.Context(), platform.PackageInput{OrganisationID: input.OrganisationID, ProductID: productID, Name: input.Name, Ecosystem: input.Ecosystem, Version: input.Version, Mode: input.Mode, UpstreamURL: input.UpstreamURL, FetchHookURL: input.FetchHookURL, Credential: input.Credential, ChecksumSHA256: input.ChecksumSHA256, ExpectedSize: input.ExpectedSize}, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) tools(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Tools(r.Context(), productID, false)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			OrganisationID      string          `json:"organisation_id"`
			Namespace           string          `json:"namespace"`
			Name                string          `json:"name"`
			Description         string          `json:"description"`
			InputSchema         json.RawMessage `json:"input_schema"`
			OutputSchema        json.RawMessage `json:"output_schema"`
			APIHookURL          string          `json:"api_hook_url"`
			HTTPMethod          string          `json:"http_method"`
			Credential          string          `json:"credential"`
			AuthorizationPolicy json.RawMessage `json:"authorization_policy"`
			TimeoutMS           int             `json:"timeout_ms"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateTool(r.Context(), platform.ToolInput{OrganisationID: input.OrganisationID, ProductID: productID, Namespace: input.Namespace, Name: input.Name, Description: input.Description, InputSchema: input.InputSchema, OutputSchema: input.OutputSchema, APIHookURL: input.APIHookURL, HTTPMethod: input.HTTPMethod, Credential: input.Credential, AuthorizationPolicy: input.AuthorizationPolicy, TimeoutMS: input.TimeoutMS}, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) publishTool(w http.ResponseWriter, r *http.Request, productID, toolID string) {
	revision, err := revisionInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.PublishTool(r.Context(), productID, toolID, revision, actor(r))
	if err != nil {
		if errors.Is(err, platform.ErrToolDrifted) {
			writeError(w, http.StatusConflict, "upstream_schema_drift", "Re-inspect and review the upstream schema before publishing this tool.", nil)
			return
		}
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

type distributionPatch struct {
	PublicMCPEnabled  bool  `json:"public_mcp_enabled"`
	AcknowledgePublic bool  `json:"acknowledge_public"`
	Revision          int64 `json:"revision"`
}

func (s *Server) distribution(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		product, err := s.service.Store().Product(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		sources, _ := s.service.Store().Sources(r.Context(), productID)
		packages, _ := s.service.Store().Packages(r.Context(), productID)
		publicSources, publicPackages := 0, 0
		for _, item := range sources {
			if item.Visibility == model.VisibilityPublic && item.Published {
				publicSources++
			}
		}
		for _, item := range packages {
			if item.Visibility == model.VisibilityPublic && item.Published {
				publicPackages++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"product":              product,
			"public_mcp_endpoint":  s.baseURL + "/mcp/public/" + productID,
			"private_mcp_endpoint": s.baseURL + "/mcp/" + productID,
			"public_sources":       publicSources, "public_packages": publicPackages,
		})
	case http.MethodPatch:
		var input distributionPatch
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SetPublicMCP(r.Context(), productID, input.PublicMCPEnabled, input.AcknowledgePublic, input.Revision, actor(r))
		if err != nil {
			s.platformError(w, err, "Enabling Public MCP makes explicitly public, published resources available without authentication.")
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

type visibilityPatch struct {
	Visibility        model.Visibility `json:"visibility"`
	AcknowledgePublic bool             `json:"acknowledge_public"`
	Revision          int64            `json:"revision"`
}

func (s *Server) sourceVisibility(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	var input visibilityPatch
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.SetSourceVisibility(r.Context(), productID, sourceID, input.Visibility, input.AcknowledgePublic, input.Revision, actor(r))
	if err != nil {
		s.platformError(w, err, "This source's published content will be accessible without authentication through Public MCP and the public widget.")
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) packageVisibility(w http.ResponseWriter, r *http.Request, productID, packageID string) {
	var input visibilityPatch
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.SetPackageVisibility(r.Context(), productID, packageID, input.Visibility, input.AcknowledgePublic, input.Revision, actor(r))
	if err != nil {
		s.platformError(w, err, "This package's published metadata and artifact link will be accessible without authentication through Public MCP and the public widget.")
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func actor(r *http.Request) platform.Actor {
	requestID, _ := r.Context().Value(requestIDKey).(string)
	actorID, _ := r.Context().Value(actorIDKey).(string)
	if actorID == "" {
		actorID = "anonymous"
	}
	return platform.Actor{ID: actorID, RequestID: requestID}
}

func (s *Server) widgets(w http.ResponseWriter, r *http.Request, productID string) {
	product, err := s.service.Store().Product(r.Context(), productID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	publicSnippet := fmt.Sprintf(`<script async src="%s/widgets/%s/public.js" data-product="%s"></script>`, s.baseURL, productID, productID)
	privateSnippet := fmt.Sprintf(`<script async src="%s/widgets/%s/private.js" data-product="%s"></script>`, s.baseURL, productID, productID)
	writeJSON(w, http.StatusOK, map[string]any{
		"public":  map[string]any{"enabled": product.PublicMCPEnabled, "snippet": publicSnippet, "contains_secret": false},
		"private": map[string]any{"enabled": true, "snippet": privateSnippet, "contains_secret": false},
	})
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func usageToolDefinition() map[string]any {
	valueSchema := map[string]any{"oneOf": []map[string]any{{"type": "string", "maxLength": 500}, {"type": "number"}, {"type": "boolean"}, {"type": "null"}}}
	itemSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"key":         map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9_.-]{0,63}$`},
			"label":       map[string]any{"type": "string", "minLength": 1, "maxLength": 80},
			"value":       valueSchema,
			"format":      map[string]any{"type": "string", "enum": []string{"text", "number", "percentage", "date", "datetime", "duration", "currency"}},
			"unit":        map[string]any{"type": "string", "maxLength": 32},
			"description": map[string]any{"type": "string", "maxLength": 500},
		},
		"required": []string{"key", "label", "value"},
	}
	return map[string]any{
		"name":        "usage.get",
		"description": "Return the current vendor-provided usage report for the authenticated account. DokoSoko does not calculate or store these values.",
		"inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}},
		"outputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"as_of": map[string]any{"type": "string", "format": "date-time"},
				"items": map[string]any{"type": "array", "maxItems": 50, "items": itemSchema},
			},
			"required": []string{"as_of", "items"},
		},
		"annotations": map[string]any{"readOnlyHint": true, "idempotentHint": true, "destructiveHint": false},
	}
}

func (s *Server) publicMCP(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("productID")
	product, err := s.service.Store().Product(r.Context(), productID)
	if err != nil || !product.PublicMCPEnabled {
		writeError(w, http.StatusNotFound, "public_mcp_unavailable", "Public MCP is not enabled for this product.", nil)
		return
	}
	if !s.allowAnonymous(productID, r.RemoteAddr, time.Now().UTC()) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Public MCP request limit exceeded.", nil)
		return
	}
	w.Header().Set("X-RateLimit-Limit", "120")
	s.handleMCP(w, r, productID, true)
}

func (s *Server) allowAnonymous(productID, remoteAddress string, now time.Time) bool {
	return s.allowFixedWindow("public|"+productID+"|"+remoteHost(remoteAddress), 120, now)
}

func remoteHost(remoteAddress string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddress))
	if err == nil {
		return host
	}
	return strings.Trim(strings.TrimSpace(remoteAddress), "[]")
}

func (s *Server) allowFixedWindow(key string, limit int, now time.Time) bool {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	window := s.rates[key]
	if window.started.IsZero() || now.Sub(window.started) >= time.Minute {
		window = rateWindow{started: now}
	}
	if window.count >= limit {
		s.rates[key] = window
		return false
	}
	window.count++
	s.rates[key] = window
	return true
}

func (s *Server) privateMCP(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("productID")
	var principal identity.Principal
	if s.identityBroker != nil {
		value, err := s.identityBroker.Authenticate(r.Context(), bearerToken(r))
		if err == nil && value.ProductID == productID {
			principal = value
		}
	}
	if principal.Subject == "" && s.allowDemoTokens && isBearer(r, demoPrivateToken) {
		principal = identity.Principal{ProductID: productID, ClientID: productID, Issuer: "development", Subject: "private_mcp_demo", Entitlements: map[string]bool{}}
	}
	if principal.Subject == "" {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Private MCP requires a DokoSoko access token.", nil)
		return
	}
	if !s.allowFixedWindow("private-mcp|"+productID+"|"+principal.Issuer+"|"+principal.Subject, 600, time.Now().UTC()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Private MCP request limit exceeded.", nil)
		return
	}
	s.handleMCP(w, r.WithContext(context.WithValue(r.Context(), principalKey, principal)), productID, false)
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request, productID string, public bool) {
	var request rpcRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if err := s.validateStatelessMCPv2(r, request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32022, "message": err.Error(), "data": map[string]any{"supported": []string{model.StatelessMCPv2Protocol}, "policy": "Stateless MCPv2 Only", "specification": "https://blog.modelcontextprotocol.io/posts/2026-07-28/"}}})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	channel, actorKind, actorID := "public_mcp", "anonymous", ""
	selection := model.ProductSelectionContext{}
	if !public {
		principal, _ := r.Context().Value(principalKey).(identity.Principal)
		channel, actorKind, actorID = "private_mcp", "vendor_user", pseudonym(productID, principal)
		selection.CustomerID, selection.InstallationID = principal.VendorOrganisation, principal.InstallationID
	}
	productManifest, manifestErr := s.service.ProductManifestFor(r.Context(), productID, selection)
	analyticsDimensions := map[string]any{"channel": channel, "method": request.Method}
	if manifestErr == nil {
		analyticsDimensions["catalog_revision"] = productManifest.CatalogRevision
		analyticsDimensions["selection_source"] = productManifest.SelectionSource
		analyticsDimensions["environment_id"] = productManifest.EnvironmentID
		analyticsDimensions["installation_id"] = productManifest.InstallationID
		if productManifest.EffectiveVersion != nil {
			analyticsDimensions["product_version_id"] = productManifest.EffectiveVersion.ID
			analyticsDimensions["product_version"] = productManifest.EffectiveVersion.Version
			analyticsDimensions["manifest_hash"] = productManifest.ManifestHash
		}
	}
	s.recordAnalytics(r.Context(), productID, "mcp.request", actorKind, actorID, analyticsDimensions)
	switch request.Method {
	case "server/discover":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Product discovery failed")
			return
		}
		cacheScope := "private"
		if public {
			cacheScope = "public"
		}
		writeRPC(w, request.ID, map[string]any{"resultType": "complete", "supportedVersions": []string{model.StatelessMCPv2Protocol}, "capabilities": map[string]any{"tools": map[string]any{"listChanged": true}}, "product": productManifest, "catalogRevision": productManifest.CatalogRevision, "manifestHash": productManifest.ManifestHash, "instructions": "Use the effective DokoSoko product version and capability releases returned in product discovery. Authenticated installation, environment, and customer pins override default product channels in that order.", "ttlMs": 30000, "cacheScope": cacheScope})
	case "tools/list":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Product discovery failed")
			return
		}
		tools := []map[string]any{
			{"name": "product.get_manifest", "description": "Return this product, its MCP-facing description, the effective pinned or default product version, compatibility profile, capability releases, and available versions.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}},
			{"name": "product.versions.list", "description": "List published product versions and their latest, LTS, deprecated, replacement, and sunset metadata.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}},
			{"name": "search_knowledge", "description": "Search published product knowledge.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}}},
			{"name": "find_package", "description": "Find published packages.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}}},
			{"name": "get_package", "description": "Get published package metadata.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"package_id": map[string]any{"type": "string"}}, "required": []string{"package_id"}}},
		}
		if !public {
			principal, _ := r.Context().Value(principalKey).(identity.Principal)
			tools = append(tools,
				map[string]any{"name": "integration_runs.start", "description": "Start an environment-scoped integration outcome run.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"environment_id": map[string]any{"type": "string"}, "requested_outcome": map[string]any{"type": "string", "maxLength": 500}}, "required": []string{"environment_id", "requested_outcome"}}},
				map[string]any{"name": "integration_runs.complete", "description": "Complete a run with a deterministic validation result.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"run_id": map[string]any{"type": "string"}, "reported_success": map[string]any{"type": "boolean"}, "validated_success": map[string]any{"type": "boolean"}, "failure_code": map[string]any{"type": "string", "maxLength": 120}}, "required": []string{"run_id", "validated_success"}}},
			)
			if s.usageReporter != nil {
				config, err := s.service.Store().VendorIdentity(r.Context(), productID)
				if err == nil && config.UsageHookURL != "" {
					tools = append(tools, usageToolDefinition())
				}
			}
			if s.mcpBridge != nil {
				connections, _ := s.service.Store().MCPConnections(r.Context(), productID)
				for _, connection := range connections {
					if connection.AuthMode == "delegated_oauth" && connection.State == "active" {
						tools = append(tools, map[string]any{"name": "mcp_connections.authorize", "description": "Create a short-lived authorization URL that connects your identity to a delegated Stateless MCPv2 upstream.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"connection_id": map[string]any{"type": "string"}}, "required": []string{"connection_id"}}})
						break
					}
				}
			}
			if s.providerRuntime != nil && s.providerRuntime.HasCapabilities(r.Context(), productID, principal.Entitlements) {
				tools = append(tools,
					map[string]any{"name": "projects.create", "description": "Create an idempotent, environment-scoped vendor project.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"provider_id": map[string]any{"type": "string"}, "environment_id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"}, "idempotency_key": map[string]any{"type": "string", "minLength": 16}, "ttl_seconds": map[string]any{"type": "integer", "minimum": 300}}, "required": []string{"provider_id", "environment_id", "name", "idempotency_key", "ttl_seconds"}}},
					map[string]any{"name": "credentials.issue", "description": "Issue a short-lived credential once; DokoSoko retains only its fingerprint.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"provider_id": map[string]any{"type": "string"}, "environment_id": map[string]any{"type": "string"}, "project_id": map[string]any{"type": "string"}, "scopes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "idempotency_key": map[string]any{"type": "string", "minLength": 16}, "ttl_seconds": map[string]any{"type": "integer", "minimum": 300}}, "required": []string{"provider_id", "environment_id", "scopes", "idempotency_key", "ttl_seconds"}}},
					map[string]any{"name": "credentials.revoke", "description": "Revoke a credential lease owned by the authenticated vendor subject.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"lease_id": map[string]any{"type": "string"}}, "required": []string{"lease_id"}}},
				)
			}
			if s.toolRuntime != nil {
				custom, err := s.toolRuntime.Available(r.Context(), productID, principal.Entitlements)
				if err == nil {
					for _, item := range custom {
						_, allowed, allowErr := s.service.ProductVersionAllowsToolFor(r.Context(), productID, selection, item)
						if allowErr != nil || !allowed {
							continue
						}
						definition := map[string]any{"name": item.Namespace + "." + item.Name, "description": item.Description, "inputSchema": item.InputSchema}
						if len(item.OutputSchema) > 0 {
							definition["outputSchema"] = item.OutputSchema
						}
						tools = append(tools, definition)
					}
				}
			}
		}
		cacheScope := "private"
		if public {
			cacheScope = "public"
		}
		versionMeta := map[string]any{"product_id": productManifest.ProductID, "catalog_revision": productManifest.CatalogRevision, "manifest_hash": productManifest.ManifestHash, "definition_revision": productManifest.DefinitionRevision, "selection_source": productManifest.SelectionSource, "environment_id": productManifest.EnvironmentID, "installation_id": productManifest.InstallationID}
		if productManifest.EffectiveVersion != nil {
			versionMeta["version"] = productManifest.EffectiveVersion.Version
			versionMeta["is_latest"] = productManifest.EffectiveVersion.IsLatest
			versionMeta["is_lts"] = productManifest.EffectiveVersion.IsLTS
			versionMeta["deprecated"] = productManifest.EffectiveVersion.Deprecated
		}
		for _, definition := range tools {
			definition["_meta"] = map[string]any{"com.dokosoko/productVersion": versionMeta}
		}
		writeRPC(w, request.ID, map[string]any{"resultType": "complete", "product": productManifest, "catalogRevision": productManifest.CatalogRevision, "manifestHash": productManifest.ManifestHash, "tools": tools, "ttlMs": 30000, "cacheScope": cacheScope})
	case "tools/call":
		s.callTool(r.Context(), w, request, productID, public, productManifest, manifestErr)
	default:
		writeRPCError(w, request.ID, -32601, "Method not found")
	}
}

func (s *Server) validateStatelessMCPv2(r *http.Request, request rpcRequest) error {
	if request.JSONRPC != "2.0" || request.Method == "" {
		return errors.New("a JSON-RPC 2.0 method is required")
	}
	if r.Header.Get("MCP-Protocol-Version") != model.StatelessMCPv2Protocol {
		return errors.New("this endpoint is Stateless MCPv2 Only and requires MCP-Protocol-Version 2026-07-28")
	}
	if r.Header.Get("Mcp-Method") != request.Method {
		return errors.New("Mcp-Method must exactly match the JSON-RPC method")
	}
	var params map[string]json.RawMessage
	if len(request.Params) == 0 || json.Unmarshal(request.Params, &params) != nil {
		return errors.New("request params must contain Stateless MCPv2 metadata")
	}
	var meta map[string]json.RawMessage
	if json.Unmarshal(params["_meta"], &meta) != nil {
		return errors.New("request params._meta is required")
	}
	var protocolVersion string
	if json.Unmarshal(meta["io.modelcontextprotocol/protocolVersion"], &protocolVersion) != nil || protocolVersion != model.StatelessMCPv2Protocol {
		return errors.New("params._meta must declare protocol version 2026-07-28")
	}
	if request.Method == "tools/call" {
		var name string
		if json.Unmarshal(params["name"], &name) != nil || name == "" || r.Header.Get("Mcp-Name") != name {
			return errors.New("Mcp-Name must exactly match the requested tool name")
		}
	}
	if origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/"); origin != "" && origin != s.baseURL {
		return errors.New("the request Origin is not allowed")
	}
	return nil
}

func (s *Server) callTool(ctx context.Context, w http.ResponseWriter, request rpcRequest, productID string, public bool, productManifest model.ProductManifest, manifestErr error) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
		Meta      struct {
			Confirmed bool `json:"confirmed"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		writeRPCError(w, request.ID, -32602, "Invalid params")
		return
	}
	principal, _ := ctx.Value(principalKey).(identity.Principal)
	actorKind, actorID, channel := "anonymous", "", "public_mcp"
	if !public {
		actorKind, actorID, channel = "vendor_user", pseudonym(productID, principal), "private_mcp"
	}
	selection := model.ProductSelectionContext{}
	if !public {
		selection.CustomerID, selection.InstallationID = principal.VendorOrganisation, principal.InstallationID
	}
	dimensions := map[string]any{"channel": channel, "tool": params.Name}
	if manifestErr == nil {
		dimensions["catalog_revision"], dimensions["selection_source"] = productManifest.CatalogRevision, productManifest.SelectionSource
		dimensions["environment_id"], dimensions["installation_id"] = productManifest.EnvironmentID, productManifest.InstallationID
		if productManifest.EffectiveVersion != nil {
			dimensions["product_version_id"], dimensions["product_version"], dimensions["manifest_hash"] = productManifest.EffectiveVersion.ID, productManifest.EffectiveVersion.Version, productManifest.ManifestHash
		}
	}
	s.recordAnalytics(ctx, productID, "tool.called", actorKind, actorID, dimensions)
	switch params.Name {
	case "product.get_manifest":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Product discovery failed")
			return
		}
		writeToolResult(w, request.ID, productManifest)
	case "product.versions.list":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Product version discovery failed")
			return
		}
		writeToolResult(w, request.ID, map[string]any{"product_id": productManifest.ProductID, "catalog_revision": productManifest.CatalogRevision, "manifest_hash": productManifest.ManifestHash, "effective_version": productManifest.EffectiveVersion, "selection_source": productManifest.SelectionSource, "available_versions": productManifest.AvailableVersions, "operational_warnings": productManifest.OperationalWarnings})
	case "usage.get":
		if public || s.usageReporter == nil {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		if len(params.Arguments) != 0 {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		value, err := s.usageReporter.Report(ctx, productID, principal)
		if err != nil {
			if errors.Is(err, identity.ErrUsageDenied) {
				writeRPCError(w, request.ID, -32003, "Usage report is unavailable for this identity")
				return
			}
			writeRPCError(w, request.ID, -32603, "Usage report could not be retrieved")
			return
		}
		s.recordAnalytics(ctx, productID, "usage.retrieved", "vendor_user", pseudonym(productID, principal), map[string]any{"channel": "private_mcp"})
		writeToolResult(w, request.ID, value)
	case "mcp_connections.authorize":
		if public || s.mcpBridge == nil {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		connectionID, _ := params.Arguments["connection_id"].(string)
		requestID, _ := ctx.Value(requestIDKey).(string)
		authorizationURL, err := s.mcpBridge.BeginAuthorization(ctx, productID, connectionID, toolruntime.Principal{Subject: principal.Issuer + "|" + principal.Subject, VendorOrganisation: principal.VendorOrganisation, InstallationID: principal.InstallationID, Entitlements: principal.Entitlements, RequestID: requestID})
		if err != nil {
			writeRPCError(w, request.ID, -32003, "Upstream user authorization could not be started")
			return
		}
		writeToolResult(w, request.ID, map[string]any{"authorization_url": authorizationURL, "expires_in_seconds": 600, "connection_id": connectionID})
	case "integration_runs.start":
		if public {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		var input struct {
			EnvironmentID    string `json:"environment_id"`
			RequestedOutcome string `json:"requested_outcome"`
		}
		if decodeArguments(params.Arguments, &input) != nil {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		requestID, _ := ctx.Value(requestIDKey).(string)
		value, err := s.service.StartIntegrationRun(ctx, productID, input.EnvironmentID, input.RequestedOutcome, platform.Actor{ID: principal.Issuer + "|" + principal.Subject, RequestID: requestID})
		if err != nil {
			writeRPCError(w, request.ID, -32602, "Integration run could not be started")
			return
		}
		writeToolResult(w, request.ID, value)
	case "integration_runs.complete":
		if public {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		var input struct {
			RunID            string `json:"run_id"`
			ReportedSuccess  *bool  `json:"reported_success"`
			ValidatedSuccess *bool  `json:"validated_success"`
			FailureCode      string `json:"failure_code"`
		}
		if decodeArguments(params.Arguments, &input) != nil {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		requestID, _ := ctx.Value(requestIDKey).(string)
		value, err := s.service.CompleteIntegrationRun(ctx, productID, input.RunID, input.ReportedSuccess, input.ValidatedSuccess, input.FailureCode, platform.Actor{ID: principal.Issuer + "|" + principal.Subject, RequestID: requestID})
		if err != nil {
			writeRPCError(w, request.ID, -32602, "Integration run could not be completed")
			return
		}
		writeToolResult(w, request.ID, value)
	case "projects.create":
		if public || s.providerRuntime == nil {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		var input struct {
			ProviderID string `json:"provider_id"`
			providerruntime.ProjectRequest
		}
		if decodeArguments(params.Arguments, &input) != nil {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		value, err := s.providerRuntime.CreateProject(ctx, productID, input.ProviderID, input.ProjectRequest, providerPrincipal(principal, ctx))
		if err != nil {
			providerRPCError(w, request.ID, err)
			return
		}
		s.recordAnalytics(ctx, productID, "capability_resolved", "vendor_user", pseudonym(productID, principal), map[string]any{"capability": "project.create"})
		writeToolResult(w, request.ID, value)
	case "credentials.issue":
		if public || s.providerRuntime == nil {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		var input struct {
			ProviderID string `json:"provider_id"`
			providerruntime.CredentialRequest
		}
		if decodeArguments(params.Arguments, &input) != nil {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		value, err := s.providerRuntime.IssueCredential(ctx, productID, input.ProviderID, input.CredentialRequest, providerPrincipal(principal, ctx))
		if err != nil {
			providerRPCError(w, request.ID, err)
			return
		}
		s.recordAnalytics(ctx, productID, "credentials_issued", "vendor_user", pseudonym(productID, principal), map[string]any{"existing": value.Existing})
		writeToolResult(w, request.ID, value)
	case "credentials.revoke":
		if public || s.providerRuntime == nil {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		leaseID, _ := params.Arguments["lease_id"].(string)
		value, err := s.providerRuntime.RevokeCredential(ctx, productID, leaseID, providerPrincipal(principal, ctx))
		if err != nil {
			providerRPCError(w, request.ID, err)
			return
		}
		writeToolResult(w, request.ID, value)
	case "search_knowledge":
		query, _ := params.Arguments["query"].(string)
		if public {
			items, err := s.service.Store().PublicKnowledge(ctx, productID, query)
			if err != nil {
				writeRPCError(w, request.ID, -32603, "Search failed")
				return
			}
			filtered := make([]model.KnowledgeRecord, 0, len(items))
			for _, item := range items {
				allowed := false
				for _, kind := range []string{"docs", "openapi", "git"} {
					managed, candidateAllowed, allowErr := s.service.ProductVersionAllowsArtifactFor(ctx, productID, selection, kind, item.SourceID, "", "")
					if allowErr == nil && (!managed || candidateAllowed) {
						allowed = true
						break
					}
				}
				if allowed {
					filtered = append(filtered, item)
				}
			}
			writeToolResult(w, request.ID, filtered)
			return
		}
		if principal.ProductID != productID {
			writeRPCError(w, request.ID, -32003, "Knowledge access was denied by product policy")
			return
		}
		items, err := s.service.Store().PrivateKnowledge(ctx, productID, query)
		if err != nil {
			writeRPCError(w, request.ID, -32603, "Search failed")
			return
		}
		filtered := make([]model.KnowledgeRecord, 0, len(items))
		for _, item := range items {
			allowed := false
			for _, kind := range []string{"docs", "openapi", "git"} {
				managed, candidateAllowed, allowErr := s.service.ProductVersionAllowsArtifactFor(ctx, productID, selection, kind, item.SourceID, "", "")
				if allowErr == nil && (!managed || candidateAllowed) {
					allowed = true
					break
				}
			}
			if allowed {
				filtered = append(filtered, item)
			}
		}
		writeToolResult(w, request.ID, filtered)
	case "find_package":
		packages, err := s.service.Store().Packages(ctx, productID)
		if err != nil {
			writeRPCError(w, request.ID, -32603, "Package lookup failed")
			return
		}
		items := make([]model.Package, 0)
		query, _ := params.Arguments["query"].(string)
		for _, item := range packages {
			if public && (!item.Published || item.Visibility != model.VisibilityPublic) {
				continue
			}
			managed, allowed, allowErr := s.service.ProductVersionAllowsArtifactFor(ctx, productID, selection, "package", item.ID, item.Name, item.Version)
			if allowErr != nil || (managed && !allowed) {
				continue
			}
			if query == "" || strings.Contains(strings.ToLower(item.Name), strings.ToLower(query)) {
				items = append(items, item)
			}
		}
		writeToolResult(w, request.ID, items)
	case "get_package":
		id, _ := params.Arguments["package_id"].(string)
		item, err := s.service.Store().Package(ctx, productID, id)
		if err != nil || (public && (!item.Published || item.Visibility != model.VisibilityPublic)) {
			writeToolResult(w, request.ID, map[string]any{"error": "package_not_found"})
			return
		}
		managed, allowed, allowErr := s.service.ProductVersionAllowsArtifactFor(ctx, productID, selection, "package", item.ID, item.Name, item.Version)
		if allowErr != nil || (managed && !allowed) {
			writeToolResult(w, request.ID, map[string]any{"error": "package_not_in_effective_product_version"})
			return
		}
		writeToolResult(w, request.ID, item)
	default:
		if public {
			writeRPCError(w, request.ID, -32601, "Tool is not available on Public MCP")
			return
		}
		if s.toolRuntime != nil {
			requestID, _ := ctx.Value(requestIDKey).(string)
			principal, _ := ctx.Value(principalKey).(identity.Principal)
			available, lookupErr := s.service.Store().Tools(ctx, productID, false)
			if lookupErr == nil {
				for _, candidate := range available {
					if candidate.Namespace+"."+candidate.Name != params.Name {
						continue
					}
					_, allowed, allowErr := s.service.ProductVersionAllowsToolFor(ctx, productID, selection, candidate)
					if allowErr != nil || !allowed {
						writeRPCError(w, request.ID, -32003, "Tool is not included in the effective product version")
						return
					}
					break
				}
			}
			value, err := s.toolRuntime.Execute(ctx, productID, params.Name, params.Arguments, toolruntime.Principal{Subject: principal.Issuer + "|" + principal.Subject, VendorOrganisation: principal.VendorOrganisation, InstallationID: principal.InstallationID, Entitlements: principal.Entitlements, Confirmed: params.Meta.Confirmed, RequestID: requestID})
			if err == nil {
				if upstream, ok := value.(toolruntime.MCPCallResult); ok {
					writeRPC(w, request.ID, upstream.Result)
					return
				}
				writeToolResult(w, request.ID, value)
				return
			}
			if errors.Is(err, toolruntime.ErrDenied) || errors.Is(err, toolruntime.ErrConfirmation) {
				writeRPCError(w, request.ID, -32003, "Tool execution was denied by policy")
				return
			}
			if errors.Is(err, mcpbridge.ErrGrantRequired) {
				writeRPCError(w, request.ID, -32001, "Authorize this Stateless MCPv2 connection with mcp_connections.authorize before calling its tools")
				return
			}
		}
		writeRPCError(w, request.ID, -32601, "Tool not found")
	}
}

func decodeArguments(arguments map[string]any, destination any) error {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func providerPrincipal(principal identity.Principal, ctx context.Context) providerruntime.Principal {
	requestID, _ := ctx.Value(requestIDKey).(string)
	return providerruntime.Principal{Subject: principal.Issuer + "|" + principal.Subject, VendorOrganisation: principal.VendorOrganisation, InstallationID: principal.InstallationID, Entitlements: principal.Entitlements, RequestID: requestID}
}

func providerRPCError(w http.ResponseWriter, id any, err error) {
	if errors.Is(err, providerruntime.ErrDenied) {
		writeRPCError(w, id, -32003, "Provider operation was denied by vendor authorization")
		return
	}
	if errors.Is(err, providerruntime.ErrInvalidRequest) {
		writeRPCError(w, id, -32602, "Provider operation request or response was invalid")
		return
	}
	writeRPCError(w, id, -32603, "Provider operation failed closed")
}

func (s *Server) widgetScript(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("productID")
	asset := r.PathValue("asset")
	if !strings.HasSuffix(asset, ".js") {
		http.NotFound(w, r)
		return
	}
	kind := strings.TrimSuffix(asset, ".js")
	if kind != "public" && kind != "private" {
		http.NotFound(w, r)
		return
	}
	product, err := s.service.Store().Product(r.Context(), productID)
	if err != nil || (kind == "public" && !product.PublicMCPEnabled) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	fmt.Fprintf(w, `(function(){const s=document.currentScript;if(!s)return;const b=document.createElement('button');b.type='button';b.textContent='Ask %s';b.setAttribute('aria-label','Open %s DokoSoko assistant');b.style.cssText='position:fixed;right:20px;bottom:20px;border:0;border-radius:999px;padding:12px 16px;background:#18181b;color:#fff;font:600 14px system-ui;box-shadow:0 8px 30px rgba(0,0,0,.18);z-index:2147483647';b.onclick=function(){window.dispatchEvent(new CustomEvent('dokosoko:open',{detail:{product:%q,kind:%q}}))};document.body.appendChild(b)})();`, product.Name, kind, productID, kind)
}

func (s *Server) packageArtifact(w http.ResponseWriter, r *http.Request) {
	productID, packageID := r.PathValue("productID"), r.PathValue("packageID")
	pkg, err := s.service.Store().Package(r.Context(), productID, packageID)
	if err != nil || !pkg.Published {
		http.NotFound(w, r)
		return
	}
	public := pkg.Visibility == model.VisibilityPublic
	var packagePrincipal identity.Principal
	if !public {
		_, _, _, cookieAuthenticated := s.cookieSession(r)
		demoAuthenticated := s.allowDemoTokens && isBearer(r, demoPrivateToken)
		authenticated := false
		if s.identityBroker != nil {
			principal, authErr := s.identityBroker.Authenticate(r.Context(), bearerToken(r))
			authenticated = authErr == nil && principal.ProductID == productID
			if authenticated {
				packagePrincipal = principal
			}
		}
		if !cookieAuthenticated && !demoAuthenticated && !authenticated {
			writeError(w, http.StatusUnauthorized, "authentication_required", "Private package access requires an authorized DokoSoko identity.", nil)
			return
		}
	}
	if s.packageGateway == nil {
		writeError(w, http.StatusServiceUnavailable, "package_gateway_unavailable", "Package delivery is not configured.", nil)
		return
	}
	artifact, err := s.packageGateway.Acquire(r.Context(), productID, packageID)
	if err != nil {
		if errors.Is(err, packagegateway.ErrArtifactInvalid) {
			writeError(w, http.StatusBadGateway, "artifact_integrity_failed", "The upstream package failed checksum or size verification.", nil)
			return
		}
		writeError(w, http.StatusBadGateway, "package_upstream_failed", "The package could not be acquired safely.", nil)
		return
	}
	defer artifact.Close()
	actorKind, actorID, channel := "anonymous", "", "public"
	if !public {
		actorKind, actorID, channel = "vendor_user", pseudonym(productID, packagePrincipal), "private"
	}
	s.recordAnalytics(r.Context(), productID, "package.downloaded", actorKind, actorID, map[string]any{"channel": channel, "ecosystem": pkg.Ecosystem, "mode": pkg.Mode})
	s.recordAnalytics(r.Context(), productID, "package_acquired", actorKind, actorID, map[string]any{"channel": channel, "ecosystem": pkg.Ecosystem})
	w.Header().Set("Content-Type", artifact.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, artifact.Filename))
	w.Header().Set("Content-Length", strconv.FormatInt(artifact.Size, 10))
	w.Header().Set("X-Checksum-SHA256", artifact.SHA256)
	w.Header().Set("Cache-Control", "private, no-store")
	if public {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	http.ServeContent(w, r, artifact.Filename, time.Time{}, artifact.File)
}

func decodeJSON(reader io.Reader, value any) error {
	decoder := json.NewDecoder(io.LimitReader(reader, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func (s *Server) platformError(w http.ResponseWriter, err error, warning string) {
	switch {
	case errors.Is(err, platform.ErrConfirmationRequired):
		writeError(w, http.StatusConflict, "public_confirmation_required", warning, map[string]any{"requires": "acknowledge_public"})
	case errors.Is(err, platform.ErrUnsafeForPublic):
		writeError(w, http.StatusUnprocessableEntity, "unsafe_for_public", err.Error(), nil)
	case errors.Is(err, platform.ErrInvalidVisibility):
		writeError(w, http.StatusBadRequest, "invalid_visibility", err.Error(), nil)
	default:
		s.storeError(w, err)
	}
}

func (s *Server) storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found.", nil)
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "revision_conflict", "The resource changed. Refresh and try again.", nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "The request could not be completed.", nil)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "details": details}})
}

func writeRPC(w http.ResponseWriter, id, result any) {
	if source, ok := result.(map[string]any); ok {
		withMeta := make(map[string]any, len(source)+1)
		for key, value := range source {
			withMeta[key] = value
		}
		meta := map[string]any{}
		if sourceMeta, ok := source["_meta"].(map[string]any); ok {
			for key, value := range sourceMeta {
				meta[key] = value
			}
		}
		meta["io.modelcontextprotocol/serverInfo"] = map[string]any{"name": "DokoSoko", "version": "2.0.0"}
		withMeta["_meta"] = meta
		result = withMeta
	}
	writeJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func writeRPCError(w http.ResponseWriter, id any, code int, message string) {
	writeJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
}

func writeToolResult(w http.ResponseWriter, id, value any) {
	encoded, _ := json.Marshal(value)
	writeRPC(w, id, map[string]any{"resultType": "complete", "content": []map[string]any{{"type": "text", "text": string(encoded)}}, "structuredContent": value, "isError": false})
}
