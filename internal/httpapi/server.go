package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	accessruntime "github.com/dokosoko/dokosoko-service/internal/access"
	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/mcpbridge"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	providerruntime "github.com/dokosoko/dokosoko-service/internal/providers"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

const (
	demoAdminToken                    = "doko_admin_demo"
	demoPrivateToken                  = "doko_private_demo"
	managedToolConfirmationTTL        = 2 * time.Minute
	managedToolConfirmationMetaField  = "confirmation_challenge"
	managedToolConfirmationDomain     = "dokosoko-managed-tool-confirmation-v1"
	managedToolConfirmationNonceBytes = 32
)

type Server struct {
	service         *platform.Service
	auth            *auth.Manager
	toolRuntime     *toolruntime.Runtime
	identityBroker  *identity.Broker
	accessRuntime   *accessruntime.Runtime
	providerRuntime *providerruntime.Runtime
	mcpBridge       *mcpbridge.Manager
	reporting       *reporting.Service
	baseURL         string
	uploadDirectory string
	uploadMaxBytes  int64
	allowDemoTokens bool
	widgetsEnabled  bool
	secureCookies   bool
	rateMu          sync.Mutex
	rates           map[string]rateWindow
}

type Options struct {
	BaseURL         string
	UIDirectory     string
	Auth            *auth.Manager
	ToolRuntime     *toolruntime.Runtime
	IdentityBroker  *identity.Broker
	AccessRuntime   *accessruntime.Runtime
	ProviderRuntime *providerruntime.Runtime
	MCPBridge       *mcpbridge.Manager
	Reporting       *reporting.Service
	UploadDirectory string
	UploadMaxBytes  int64
	AllowDemoTokens bool
	WidgetsEnabled  bool
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
	uploadMaxBytes := options.UploadMaxBytes
	if uploadMaxBytes <= 0 {
		uploadMaxBytes = defaultSourceUploadMaxBytes
	}
	server := &Server{service: service, auth: options.Auth, toolRuntime: options.ToolRuntime, identityBroker: options.IdentityBroker, accessRuntime: options.AccessRuntime, providerRuntime: options.ProviderRuntime, mcpBridge: options.MCPBridge, reporting: options.Reporting, baseURL: baseURL, uploadDirectory: strings.TrimSpace(options.UploadDirectory), uploadMaxBytes: uploadMaxBytes, allowDemoTokens: options.AllowDemoTokens, widgetsEnabled: options.WidgetsEnabled, secureCookies: strings.HasPrefix(baseURL, "https://"), rates: make(map[string]rateWindow)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /readyz", server.ready)
	mux.HandleFunc("GET /api/v1/setup/status", server.setupStatus)
	mux.HandleFunc("POST /api/v1/setup/begin", server.setupBegin)
	mux.HandleFunc("POST /api/v1/setup/complete", server.setupComplete)
	mux.HandleFunc("POST /api/v1/auth/login", server.login)
	mux.HandleFunc("POST /api/v1/auth/logout", server.logout)
	mux.HandleFunc("GET /api/v1/auth/session", server.currentSession)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", server.oauthAuthorizationServerMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", server.oauthProtectedResourceMetadata)
	mux.HandleFunc("POST /oauth/register", server.oauthRegister)
	mux.HandleFunc("GET /oauth/authorize", server.oauthAuthorize)
	mux.HandleFunc("GET /oauth/callback", server.oauthCallback)
	mux.HandleFunc("POST /oauth/token", server.oauthToken)
	mux.HandleFunc("GET /oauth/upstream/callback", server.upstreamOAuthCallback)
	mux.HandleFunc("/api/v1/", server.adminAPI)
	mux.HandleFunc("POST /mcp/public", server.publicMCP)
	mux.HandleFunc("POST /mcp", server.privateMCP)
	mux.HandleFunc("GET /agent-setup/{kind}/prompt.md", server.agentSetupPrompt)
	if options.WidgetsEnabled {
		mux.HandleFunc("GET /v1/widgets/{widgetID}/configuration", server.widgetConfiguration)
		mux.HandleFunc("POST /v1/widget-sessions", server.createWidgetSession)
		mux.HandleFunc("POST /v1/widget-sessions/exchange", server.exchangeWidgetSession)
		mux.HandleFunc("GET /v1/widget-session", server.currentWidgetSession)
		mux.HandleFunc("POST /v1/widget-chat", server.widgetChat)
	} else {
		widgetUnavailable := func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			writeError(w, http.StatusNotFound, "not_found", "Route not found.", nil)
		}
		mux.HandleFunc("/v1/widgets/{widgetID}/configuration", widgetUnavailable)
		mux.HandleFunc("/v1/widget-sessions", widgetUnavailable)
		mux.HandleFunc("/v1/widget-sessions/exchange", widgetUnavailable)
		mux.HandleFunc("/v1/widget-session", widgetUnavailable)
		mux.HandleFunc("/v1/widget-chat", widgetUnavailable)
	}
	if options.UIDirectory != "" {
		mux.Handle("/", staticConsole(options.UIDirectory))
	}
	return requestID(mux)
}

func staticConsole(directory string) http.Handler {
	files := http.FileServer(http.Dir(directory))
	contentSecurityPolicy := staticConsoleCSP(directory)
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
		rscRequest := r.Header.Get("RSC") == "1" || strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/x-component")
		serveRootHTML := false
		if rscRequest {
			switch {
			case cleaned == ".":
				cleaned = "index.rsc"
			case !strings.HasSuffix(strings.ToLower(cleaned), ".rsc"):
				cleaned = strings.TrimSuffix(cleaned, ".html") + ".rsc"
			}
		} else if cleaned == "." {
			cleaned = "index.html"
			serveRootHTML = true
		}
		candidate := filepath.Join(directory, cleaned)
		if info, err := os.Stat(candidate); err != nil || info.IsDir() {
			if rscRequest {
				http.NotFound(w, r)
				return
			}
			cleaned = "index.html"
			serveRootHTML = true
			candidate = filepath.Join(directory, "index.html")
			if _, err := os.Stat(candidate); err != nil {
				http.NotFound(w, r)
				return
			}
		}
		if serveRootHTML {
			r.URL.Path = "/"
		} else {
			r.URL.Path = "/" + filepath.ToSlash(cleaned)
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		if extension := strings.ToLower(filepath.Ext(cleaned)); extension == ".html" || extension == ".rsc" {
			w.Header().Set("Vary", "RSC, Accept")
		}
		if strings.HasSuffix(strings.ToLower(cleaned), ".rsc") {
			w.Header().Set("Content-Type", "text/x-component")
			// An older deployment may have cached index.html at the same RSC URL.
			// Always return the component payload so a conditional request cannot
			// receive 304 and reuse that stale HTML response.
			r.Header.Del("If-Modified-Since")
			r.Header.Del("If-None-Match")
		}
		if strings.Contains(r.URL.Path, "/_next/static/") || strings.Contains(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if extension := strings.ToLower(filepath.Ext(cleaned)); extension == ".html" || extension == ".rsc" {
			w.Header().Set("Cache-Control", "no-store")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func staticConsoleCSP(directory string) string {
	scriptSources := []string{"'self'"}
	html, err := os.ReadFile(filepath.Join(directory, "index.html"))
	if err == nil {
		remaining := html
		for {
			start := bytes.Index(bytes.ToLower(remaining), []byte("<script"))
			if start < 0 {
				break
			}
			remaining = remaining[start:]
			openEnd := bytes.IndexByte(remaining, '>')
			if openEnd < 0 {
				break
			}
			closeStart := bytes.Index(bytes.ToLower(remaining[openEnd+1:]), []byte("</script>"))
			if closeStart < 0 {
				break
			}
			closeStart += openEnd + 1
			openTag := strings.ToLower(string(remaining[:openEnd+1]))
			content := remaining[openEnd+1 : closeStart]
			if !strings.Contains(openTag, " src=") && len(bytes.TrimSpace(content)) > 0 {
				digest := sha256.Sum256(content)
				scriptSources = append(scriptSources, "'sha256-"+base64.StdEncoding.EncodeToString(digest[:])+"'")
			}
			remaining = remaining[closeStart+len("</script>"):]
		}
	}
	return "default-src 'self'; script-src " + strings.Join(scriptSources, " ") + "; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
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
		writeError(w, http.StatusBadRequest, "password_requirements", "Use at least 14 characters with upper-case, lower-case, and number characters.", nil)
	default:
		writeError(w, http.StatusBadRequest, "authentication_failed", err.Error(), nil)
	}
}

func oauthError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

func (s *Server) oauthAuthorizationServerMetadata(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                s.baseURL,
		"authorization_endpoint":                s.baseURL + "/oauth/authorize",
		"token_endpoint":                        s.baseURL + "/oauth/token",
		"registration_endpoint":                 s.baseURL + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"mcp:private"},
		"resource_parameter_supported":          true,
		"client_id_metadata_document_supported": true,
	})
}

type oauthRegistrationRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
}

func onlyRegistrationValue(values []string, expected string) bool {
	return len(values) == 0 || (len(values) == 1 && values[0] == expected)
}

func (s *Server) oauthRegister(w http.ResponseWriter, r *http.Request) {
	if s.identityBroker == nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "Private MCP identity is not available.")
		return
	}
	if !s.allowFixedWindow("oauth-register|"+remoteHost(r.RemoteAddr), 30, time.Now().UTC()) {
		w.Header().Set("Retry-After", "60")
		oauthError(w, http.StatusTooManyRequests, "temporarily_unavailable", "Client registration request limit exceeded.")
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		oauthError(w, http.StatusNotFound, "invalid_client_metadata", "Private MCP is not configured.")
		return
	}
	provider, err := s.service.Store().IdentityProvider(r.Context(), deployment.ID)
	if err != nil || provider.State != "active" {
		oauthError(w, http.StatusNotFound, "invalid_client_metadata", "Private MCP identity is not available.")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "Client metadata is too large.")
		return
	}
	var input oauthRegistrationRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "Client metadata must be one JSON object.")
		return
	}
	input.ClientName = strings.TrimSpace(input.ClientName)
	if len(input.RedirectURIs) == 0 || len(input.RedirectURIs) > 20 || utf8.RuneCountInString(input.ClientName) > 200 || (input.TokenEndpointAuthMethod != "" && input.TokenEndpointAuthMethod != "none") || !onlyRegistrationValue(input.GrantTypes, "authorization_code") || !onlyRegistrationValue(input.ResponseTypes, "code") || (input.Scope != "" && input.Scope != "mcp:private") {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "Only public authorization-code clients using PKCE and the mcp:private scope are supported.")
		return
	}
	redirects := append([]string(nil), input.RedirectURIs...)
	sort.Strings(redirects)
	for index, redirect := range redirects {
		if !identity.ValidRedirectURI(redirect) || (index > 0 && redirects[index-1] == redirect) {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "Redirect URIs must be unique HTTPS or loopback HTTP URLs without fragments.")
			return
		}
	}
	fingerprint := sha256.Sum256([]byte(deployment.ID + "\x00" + input.ClientName + "\x00" + strings.Join(redirects, "\x00")))
	clientID := "mcp_client_" + base64.RawURLEncoding.EncodeToString(fingerprint[:24])
	client, err := s.service.Store().CreateOAuthClient(r.Context(), identity.OAuthClient{ClientID: clientID, DeploymentID: deployment.ID, ClientName: input.ClientName, RedirectURIs: redirects})
	if err != nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "Client registration could not be completed. Retrying the same request is safe.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  client.ClientID,
		"client_id_issued_at":        client.CreatedAt.Unix(),
		"client_name":                client.ClientName,
		"redirect_uris":              client.RedirectURIs,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"scope":                      "mcp:private",
	})
}

func (s *Server) oauthProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if s.identityBroker == nil {
		http.NotFound(w, r)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	provider, err := s.service.Store().IdentityProvider(r.Context(), deployment.ID)
	if err != nil || provider.State != "active" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 s.baseURL + "/mcp",
		"authorization_servers":    []string{s.baseURL},
		"scopes_supported":         []string{"mcp:private"},
		"bearer_methods_supported": []string{"header"},
	})
}

func (s *Server) productFromMCPResource(ctx context.Context, raw string) (string, bool) {
	resource, err := url.Parse(raw)
	base, baseErr := url.Parse(s.baseURL)
	if err != nil || baseErr != nil || resource.RawQuery != "" || resource.Fragment != "" || resource.User != nil || !strings.EqualFold(resource.Scheme, base.Scheme) || !strings.EqualFold(resource.Host, base.Host) {
		return "", false
	}
	expectedPath := strings.TrimRight(base.EscapedPath(), "/") + "/mcp"
	if resource.EscapedPath() != expectedPath {
		return "", false
	}
	deployment, err := s.service.Store().Deployment(ctx)
	if err != nil {
		return "", false
	}
	return deployment.ID, true
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
	resource := r.URL.Query().Get("resource")
	productID, ok := s.productFromMCPResource(r.Context(), resource)
	if !ok {
		oauthError(w, http.StatusBadRequest, "invalid_target", "The resource must identify a DokoSoko MCP endpoint.")
		return
	}
	redirect, err := s.identityBroker.Begin(r.Context(), identity.AuthorizationRequest{
		ProductID: productID, ClientID: r.URL.Query().Get("client_id"), RedirectURI: r.URL.Query().Get("redirect_uri"), Resource: resource, Scope: r.URL.Query().Get("scope"), State: r.URL.Query().Get("state"), CodeChallenge: r.URL.Query().Get("code_challenge"),
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
	rawState := r.URL.Query().Get("state")
	if identity.IsProviderTestState(rawState) {
		test, err := s.identityBroker.CompleteProviderTest(r.Context(), rawState, r.URL.Query().Get("code"), r.URL.Query().Get("error"))
		if err != nil {
			http.Redirect(w, r, s.baseURL+"/identity?identity_test_error=invalid_or_expired", http.StatusSeeOther)
			return
		}
		query := url.Values{"identity_test_id": {test.ID}}
		http.Redirect(w, r, s.baseURL+"/identity?"+query.Encode(), http.StatusSeeOther)
		return
	}
	if r.URL.Query().Get("error") != "" {
		oauthError(w, http.StatusUnauthorized, "access_denied", "The vendor authorization server denied access.")
		return
	}
	result, err := s.identityBroker.Callback(r.Context(), rawState, r.URL.Query().Get("code"))
	if err != nil {
		oauthError(w, http.StatusUnauthorized, "access_denied", "Vendor identity or access verification failed.")
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
	result, err := s.identityBroker.Exchange(r.Context(), r.PostForm.Get("code"), r.PostForm.Get("code_verifier"), r.PostForm.Get("client_id"), r.PostForm.Get("redirect_uri"), r.PostForm.Get("resource"))
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

func (s *Server) identityProvider(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().IdentityProvider(r.Context(), deployment.ID)
		if errors.Is(err, store.ErrNotFound) {
			s.writeIdentityProvider(w, r, identity.ProviderConfig{OrganisationID: deployment.OrganisationID, DeploymentID: deployment.ID}, false)
			return
		}
		if err != nil {
			s.storeError(w, err)
			return
		}
		s.writeIdentityProvider(w, r, value, true)
	case http.MethodPut:
		var input struct {
			Provider           string   `json:"provider"`
			Issuer             string   `json:"issuer"`
			ClientID           string   `json:"client_id"`
			ClientSecret       string   `json:"client_secret"`
			Scopes             []string `json:"scopes"`
			Audience           string   `json:"audience"`
			OAuthResource      string   `json:"oauth_resource"`
			OrganisationClaim  string   `json:"customer_account_claim"`
			InstallationClaim  string   `json:"installation_claim"`
			DelegatedAPIOrigin string   `json:"authorization_api_origin"`
			Revision           *int64   `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Provider != "oidc" || input.Issuer == "" || input.ClientID == "" || input.OrganisationClaim == "" || input.DelegatedAPIOrigin == "" || input.Revision == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "provider oidc, issuer, client_id, customer_account_claim, authorization_api_origin, and revision are required.", nil)
			return
		}
		value, err := s.service.ConfigureIdentity(r.Context(), platform.IdentityInput{DeploymentID: deployment.ID, Issuer: input.Issuer, ClientID: input.ClientID, ClientSecret: input.ClientSecret, Scopes: input.Scopes, Audience: input.Audience, OAuthResource: input.OAuthResource, OrganisationClaim: input.OrganisationClaim, InstallationClaim: input.InstallationClaim, DelegatedAPIOrigin: input.DelegatedAPIOrigin, Revision: *input.Revision}, actor(r))
		if err != nil {
			s.identityConfigurationError(w, err)
			return
		}
		s.writeIdentityProvider(w, r, value, true)
	case http.MethodDelete:
		var input struct {
			Revision *int64 `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil || input.Revision == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		if _, err := s.service.DisconnectIdentityProvider(r.Context(), deployment.ID, *input.Revision, actor(r)); err != nil {
			s.identityDisconnectError(w, err)
			return
		}
		s.writeIdentityProvider(w, r, identity.ProviderConfig{OrganisationID: deployment.OrganisationID, DeploymentID: deployment.ID}, false)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
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
		RequiredGrants       []string `json:"required_grants"`
		ConfirmationRequired bool     `json:"confirmation_required"`
		TimeoutMS            int      `json:"timeout_ms"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	adminActor := actor(r)
	value, err := s.mcpBridge.Import(r.Context(), productID, connectionID, mcpbridge.ImportInput{ToolNames: input.ToolNames, RequiredGrants: input.RequiredGrants, ConfirmationRequired: input.ConfirmationRequired, TimeoutMS: input.TimeoutMS}, mcpbridge.Actor{ID: adminActor.ID, RequestID: adminActor.RequestID})
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
			OrganisationID string   `json:"organisation_id"`
			Name           string   `json:"name"`
			BaseURL        string   `json:"base_url"`
			Credential     string   `json:"credential"`
			RequiredGrants []string `json:"required_grants"`
			MaxTTLSeconds  int      `json:"max_ttl_seconds"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateProvider(r.Context(), platform.ProviderInput{OrganisationID: input.OrganisationID, ProductID: productID, Name: input.Name, BaseURL: input.BaseURL, Credential: input.Credential, RequiredGrants: input.RequiredGrants, MaxTTLSeconds: input.MaxTTLSeconds}, actor(r))
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
	digest := sha256.Sum256([]byte(productID + "\x00" + vendorActorID(principal)))
	return hex.EncodeToString(digest[:16])
}

func vendorActorID(principal identity.Principal) string {
	if principal.Subject == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(principal.Issuer)) + "." + base64.RawURLEncoding.EncodeToString([]byte(principal.Subject))
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
	case len(parts) == 4 && parts[2] == "ai" && parts[3] == "connections":
		s.aiProviderConnections(w, r)
	case len(parts) == 6 && parts[2] == "ai" && parts[3] == "connections" && parts[5] == "test" && r.Method == http.MethodPost:
		s.testAIProviderConnection(w, r, parts[4])
	case len(parts) == 5 && parts[2] == "root" && parts[3] == "users" && r.Method == http.MethodDelete:
		s.revokeRootUser(w, r, parts[4])
	case len(parts) == 3 && parts[2] == "organisations":
		s.organisations(w, r)
	case len(parts) == 3 && parts[2] == "deployment":
		s.deployment(w, r)
	case len(parts) == 3 && parts[2] == "environments":
		s.deploymentEnvironments(w, r)
	case len(parts) == 3 && parts[2] == "integrations":
		s.integrations(w, r)
	case s.widgetsEnabled && len(parts) == 3 && parts[2] == "widgets":
		s.adminWidgets(w, r)
	case s.widgetsEnabled && len(parts) == 4 && parts[2] == "widgets":
		s.adminWidget(w, r, parts[3])
	case s.widgetsEnabled && len(parts) == 5 && parts[2] == "widgets" && parts[4] == "activate" && r.Method == http.MethodPost:
		s.setAdminWidgetState(w, r, parts[3], "active")
	case s.widgetsEnabled && len(parts) == 5 && parts[2] == "widgets" && parts[4] == "disable" && r.Method == http.MethodPost:
		s.setAdminWidgetState(w, r, parts[3], "disabled")
	case s.widgetsEnabled && len(parts) == 5 && parts[2] == "widgets" && parts[4] == "secrets":
		s.adminWidgetSecrets(w, r, parts[3])
	case s.widgetsEnabled && len(parts) == 6 && parts[2] == "widgets" && parts[4] == "secrets" && r.Method == http.MethodDelete:
		s.revokeAdminWidgetSecret(w, r, parts[3], parts[5])
	case s.widgetsEnabled && len(parts) == 5 && parts[2] == "widgets" && parts[4] == "sessions" && r.Method == http.MethodGet:
		s.adminWidgetSessions(w, r, parts[3])
	case s.widgetsEnabled && len(parts) == 5 && parts[2] == "widgets" && parts[4] == "preview-session" && r.Method == http.MethodPost:
		s.createAdminWidgetPreviewSession(w, r, parts[3])
	case s.widgetsEnabled && len(parts) == 6 && parts[2] == "widgets" && parts[4] == "sessions" && r.Method == http.MethodDelete:
		s.revokeAdminWidgetSession(w, r, parts[3], parts[5])
	case len(parts) == 4 && parts[2] == "integrations":
		s.integration(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "publish" && r.Method == http.MethodPost:
		s.publishIntegration(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "preflight" && r.Method == http.MethodPost:
		s.preflightIntegration(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "runtime-setup":
		s.integrationRuntimeSetup(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "runtime-connections":
		s.integrationRuntimeConnections(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "runtime-credential-sets" && r.Method == http.MethodPost:
		s.createIntegrationRuntimeCredentialSet(w, r, parts[3])
	case len(parts) == 4 && parts[2] == "runtime-credential-sets" && r.Method == http.MethodGet:
		s.runtimeCredentialSet(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "runtime-credential-sets" && parts[4] == "usage" && r.Method == http.MethodGet:
		s.runtimeCredentialUsage(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "runtime-credential-sets" && parts[4] == "rotate" && r.Method == http.MethodPost:
		s.rotateRuntimeCredential(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "runtime-service-connections" && parts[4] == "check" && r.Method == http.MethodPost:
		s.checkRuntimeServiceConnection(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "runtime-credential-sets" && parts[4] == "versions" && parts[6] == "revoke" && r.Method == http.MethodPost:
		s.revokeRuntimeCredential(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "access-connections" && r.Method == http.MethodPut:
		s.integrationAccessConnections(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "support-route" && r.Method == http.MethodPut:
		s.integrationSupportRoute(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "authorization-points":
		s.authorizationPoints(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "integrations" && parts[4] == "authorization-points":
		s.authorizationPoint(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "tools":
		s.integrationTools(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "resource-sets" && r.Method == http.MethodPost:
		s.attachResourceSet(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "integrations" && parts[4] == "resource-sets" && r.Method == http.MethodDelete:
		s.detachResourceSet(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "packages":
		s.integrationPackages(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "integrations" && parts[4] == "packages":
		s.integrationPackage(w, r, parts[3], parts[5])
	case len(parts) == 3 && parts[2] == "package-artifacts":
		s.packageArtifacts(w, r)
	case len(parts) == 4 && parts[2] == "package-artifacts":
		s.packageArtifact(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "package-artifacts" && parts[4] == "releases" && r.Method == http.MethodGet:
		s.packageReleases(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "package-artifacts" && parts[4] == "publish" && r.Method == http.MethodPost:
		s.publishPackageArtifact(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "package-artifacts" && parts[4] == "deprecate" && r.Method == http.MethodPost:
		s.deprecatePackageArtifact(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "package-artifacts" && parts[4] == "retire" && r.Method == http.MethodPost:
		s.retirePackageArtifact(w, r, parts[3])
	case len(parts) == 3 && parts[2] == "resource-sets":
		s.resourceSets(w, r)
	case len(parts) == 3 && parts[2] == "grant-definitions":
		s.grantDefinitions(w, r)
	case len(parts) == 4 && parts[2] == "grant-definitions":
		s.grantDefinition(w, r, parts[3])
	case len(parts) == 4 && parts[2] == "resource-sets":
		s.resourceSet(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "resource-sets" && parts[4] == "duplicate" && r.Method == http.MethodPost:
		s.duplicateResourceSet(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "resource-sets" && parts[4] == "revisions" && r.Method == http.MethodGet:
		s.resourceSetRevisions(w, r, parts[3])
	case len(parts) == 3 && parts[2] == "access-definitions":
		s.accessDefinitions(w, r)
	case len(parts) == 4 && parts[2] == "access-definitions":
		s.accessDefinition(w, r, parts[3])
	case len(parts) == 3 && parts[2] == "access-connections":
		s.accessConnections(w, r)
	case len(parts) == 3 && parts[2] == "backend-connections":
		s.backendConnections(w, r)
	case len(parts) == 4 && parts[2] == "backend-connections":
		s.backendConnection(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "backend-connections" && parts[4] == "credentials" && r.Method == http.MethodPost:
		s.createBackendConnectionCredential(w, r, parts[3])
	case len(parts) == 3 && parts[2] == "support-routes":
		s.supportRoutes(w, r)
	case len(parts) == 4 && parts[2] == "support-routes":
		s.supportRoute(w, r, parts[3])
	case len(parts) == 3 && parts[2] == "support-submissions" && r.Method == http.MethodGet:
		s.supportSubmissions(w, r)
	case len(parts) == 4 && parts[2] == "support-submissions" && r.Method == http.MethodGet:
		s.supportSubmission(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "support-submissions" && parts[4] == "delivery-attempts" && r.Method == http.MethodPost:
		s.createSupportDeliveryAttempt(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "access-connections" && parts[4] == "instances" && r.Method == http.MethodGet:
		s.accessInstances(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "access-connections" && parts[4] == "credentials" && r.Method == http.MethodGet:
		s.accessCredentials(w, r, parts[3], "")
	case len(parts) == 5 && parts[2] == "access-instances" && parts[4] == "credentials" && r.Method == http.MethodGet:
		s.accessCredentials(w, r, "", parts[3])
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
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "customer-accounts" && r.Method == http.MethodGet:
		s.customerAccounts(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "customer-accounts" && r.Method == http.MethodPatch:
		s.customerAccount(w, r, parts[3], parts[5])
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
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "sources" && parts[5] == "upload":
		s.uploadSource(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "visibility" && r.Method == http.MethodPatch:
		s.sourceVisibility(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "crawl" && r.Method == http.MethodPost:
		s.queueCrawl(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "publish" && r.Method == http.MethodPost:
		s.publishSource(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "review" && r.Method == http.MethodGet:
		s.sourceReview(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "publications" && r.Method == http.MethodGet:
		s.sourcePublications(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "crawls" && r.Method == http.MethodGet:
		s.crawlJobs(w, r, parts[3], parts[5])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "tool-builder":
		s.toolBuilder(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "tools":
		s.tools(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "tools":
		s.toolEditorResource(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "clone" && r.Method == http.MethodPost:
		s.cloneTool(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "dry-run" && r.Method == http.MethodPost:
		s.dryRunTool(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "test-confirmations" && r.Method == http.MethodPost:
		s.createToolTestConfirmation(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "test-runs":
		s.toolTestRuns(w, r, parts[3], parts[5])
	case len(parts) == 9 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "test-runs" && parts[8] == "analyse":
		s.analyseToolTestRun(w, r, parts[3], parts[5], parts[7])
	case len(parts) == 8 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "test-runs" && r.Method == http.MethodGet:
		s.toolTestRun(w, r, parts[3], parts[5], parts[7])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "retire" && r.Method == http.MethodPost:
		s.retireTool(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "mcp-connections":
		s.mcpConnections(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "mcp-connections" && parts[6] == "inspect" && r.Method == http.MethodPost:
		s.inspectMCPConnection(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "mcp-connections" && parts[6] == "import" && r.Method == http.MethodPost:
		s.importMCPConnection(w, r, parts[3], parts[5])
	case len(parts) == 3 && parts[2] == "identity-provider":
		s.identityProvider(w, r)
	case len(parts) == 4 && parts[2] == "identity-provider" && parts[3] == "tests":
		s.identityProviderTests(w, r)
	case len(parts) == 5 && parts[2] == "identity-provider" && parts[3] == "tests":
		s.identityProviderTest(w, r, parts[4])
	case len(parts) == 4 && parts[2] == "identity-provider" && parts[3] == "activate":
		s.activateIdentityProvider(w, r)
	case len(parts) == 4 && parts[2] == "identity-provider" && parts[3] == "disable":
		s.disableIdentityProvider(w, r)
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
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "ai-profiles" && r.Method == http.MethodGet:
		s.aiWorkloadProfiles(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "ai-profiles" && r.Method == http.MethodPut:
		s.aiWorkloadProfile(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "analyses":
		s.integrationAnalyses(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "analyses":
		s.integrationAnalysis(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "analyses" && parts[6] == "recipes" && r.Method == http.MethodPost:
		s.generateRecipes(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "recipes":
		s.recipes(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "recipes":
		s.recipe(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "recipes" && parts[6] == "rework" && r.Method == http.MethodPost:
		s.reworkRecipe(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "recipes" && parts[6] == "approve" && r.Method == http.MethodPost:
		s.approveRecipe(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "recipes" && parts[6] == "publish" && r.Method == http.MethodPost:
		s.publishRecipe(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "ai-jobs" && r.Method == http.MethodGet:
		s.aiJobs(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "attention" && r.Method == http.MethodGet:
		s.attention(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "recipe-analytics" && r.Method == http.MethodGet:
		s.recipeAnalytics(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "ai-usage" && r.Method == http.MethodGet:
		s.aiUsage(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "publish" && r.Method == http.MethodPost:
		s.publishTool(w, r, parts[3], parts[5])
	default:
		writeError(w, http.StatusNotFound, "not_found", "Route not found.", nil)
	}
}

func (s *Server) supportSubmissions(w http.ResponseWriter, r *http.Request) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Reporting is unavailable.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsed < 1 || parsed > 200 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be an integer between 1 and 200.", nil)
			return
		}
		limit = parsed
	}
	startingAfter := r.URL.Query().Get("starting_after")
	if len(startingAfter) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_request", "starting_after is invalid.", nil)
		return
	}
	values, hasMore, err := s.reporting.Submissions(r.Context(), deployment.ID, startingAfter, limit)
	if startingAfter != "" && errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "invalid_request", "starting_after does not identify a support submission in this deployment.", nil)
		return
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values, "has_more": hasMore})
}

func (s *Server) supportSubmission(w http.ResponseWriter, r *http.Request, submissionID string) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Reporting is unavailable.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.reporting.Submission(r.Context(), deployment.ID, submissionID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) createSupportDeliveryAttempt(w http.ResponseWriter, r *http.Request, submissionID string) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Reporting is unavailable.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.reporting.Retry(r.Context(), deployment.ID, submissionID)
	if errors.Is(err, reporting.ErrDeliveryUnavailable) {
		writeError(w, http.StatusConflict, "reporting_delivery_unavailable", "Configure support delivery before retrying.", nil)
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "submission_not_retryable", "Only unexpired held or failed submissions can be retried.", nil)
		return
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	currentActor := actor(r)
	requestID, _ := r.Context().Value(requestIDKey).(string)
	_ = s.service.Store().AppendAudit(r.Context(), model.AuditEvent{ID: "audit_" + strconv.FormatInt(time.Now().UnixNano(), 10), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: currentActor.ID, Action: "support_submission.delivery_attempt_created", TargetType: "support_submission", TargetID: submissionID, Current: map[string]any{"kind": value.Kind, "state": value.State}, RequestID: requestID, CreatedAt: time.Now().UTC()})
	writeJSON(w, http.StatusAccepted, value)
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
			Scope             string `json:"scope"`
			ScopeID           string `json:"scope_id"`
			CustomerAccountID string `json:"customer_account_id"`
			EnvironmentID     string `json:"environment_id"`
			InstallationID    string `json:"installation_id"`
			ProductVersionID  string `json:"product_version_id"`
			Reason            string `json:"reason"`
			Revision          int64  `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Scope == "" {
			input.Scope, input.ScopeID = "customer", input.CustomerAccountID
		}
		value, err := s.service.SaveScopedProductVersionPin(r.Context(), productID, platform.ProductVersionPinInput{Scope: input.Scope, ScopeID: input.ScopeID, CustomerAccountID: input.CustomerAccountID, EnvironmentID: input.EnvironmentID, InstallationID: input.InstallationID, ProductVersionID: input.ProductVersionID, Reason: input.Reason, Revision: input.Revision}, actor(r))
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
			ID                string `json:"id"`
			CustomerAccountID string `json:"customer_account_id"`
			EnvironmentID     string `json:"environment_id"`
			ExternalID        string `json:"external_id"`
			Name              string `json:"name"`
			State             string `json:"state"`
			Revision          int64  `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveProductInstallation(r.Context(), productID, platform.ProductInstallationInput{ID: input.ID, CustomerAccountID: input.CustomerAccountID, EnvironmentID: input.EnvironmentID, ExternalID: input.ExternalID, Name: input.Name, State: input.State, Revision: input.Revision}, actor(r))
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

func (s *Server) customerAccounts(w http.ResponseWriter, r *http.Request, productID string) {
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsed < 1 || parsed > 200 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be an integer between 1 and 200.", nil)
			return
		}
		limit = parsed
	}
	startingAfter := r.URL.Query().Get("starting_after")
	if len(startingAfter) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_request", "starting_after is invalid.", nil)
		return
	}
	values, hasMore, err := s.service.Store().CustomerAccounts(r.Context(), productID, startingAfter, limit)
	if startingAfter != "" && errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "invalid_request", "starting_after does not identify a customer account in this product.", nil)
		return
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values, "has_more": hasMore})
}

func (s *Server) customerAccount(w http.ResponseWriter, r *http.Request, productID, accountID string) {
	var input struct {
		State    string `json:"state"`
		Revision int64  `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.UpdateCustomerAccountState(r.Context(), productID, accountID, input.State, input.Revision, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
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
		if strings.EqualFold(strings.TrimSpace(input.Kind), "upload") {
			writeError(w, http.StatusBadRequest, "source_upload_requires_multipart", "Create uploaded sources with the reviewed multipart upload endpoint.", nil)
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

func (s *Server) sourceReview(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	value, err := s.service.SourceReview(r.Context(), productID, sourceID, strings.TrimSpace(r.URL.Query().Get("crawl_job_id")))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) sourcePublications(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	values, err := s.service.Store().SourcePublications(r.Context(), productID, sourceID)
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
	var input struct {
		Revision            int64    `json:"revision"`
		CrawlJobID          string   `json:"crawl_job_id"`
		DocumentIDs         []string `json:"document_ids"`
		AcknowledgeReviewed bool     `json:"acknowledge_reviewed"`
	}
	if err := decodeJSON(r.Body, &input); err != nil || input.Revision < 1 {
		if err == nil {
			err = errors.New("revision must be positive")
		}
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, publication, err := s.service.PublishSource(r.Context(), productID, sourceID, platform.SourcePublicationInput{Revision: input.Revision, CrawlJobID: input.CrawlJobID, DocumentIDs: input.DocumentIDs, AcknowledgeReviewed: input.AcknowledgeReviewed}, actor(r))
	if err != nil {
		s.platformError(w, err, "Quarantined source content cannot be published.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": value, "publication": publication})
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
			OrganisationID             string          `json:"organisation_id,omitempty"`
			Scope                      string          `json:"scope"`
			OwnerIntegrationID         string          `json:"owner_integration_id"`
			RuntimeServiceConnectionID string          `json:"runtime_service_connection_id"`
			HTTPPath                   string          `json:"http_path"`
			Namespace                  string          `json:"namespace"`
			Name                       string          `json:"name"`
			Description                string          `json:"description"`
			InputSchema                json.RawMessage `json:"input_schema"`
			OutputSchema               json.RawMessage `json:"output_schema"`
			Endpoint                   string          `json:"endpoint"`
			HTTPMethod                 string          `json:"http_method"`
			UpstreamAuth               json.RawMessage `json:"upstream_auth"`
			Credential                 string          `json:"credential"`
			RequestMapping             json.RawMessage `json:"request_mapping"`
			ResponseMapping            json.RawMessage `json:"response_mapping"`
			RequestExample             json.RawMessage `json:"request_example"`
			ResponseExample            json.RawMessage `json:"response_example"`
			AuthorizationPolicy        json.RawMessage `json:"authorization_policy"`
			TimeoutMS                  int             `json:"timeout_ms"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateTool(r.Context(), platform.ToolInput{ProductID: productID, Scope: input.Scope, OwnerIntegrationID: input.OwnerIntegrationID, RuntimeServiceConnectionID: input.RuntimeServiceConnectionID, HTTPPath: input.HTTPPath, Namespace: input.Namespace, Name: input.Name, Description: input.Description, InputSchema: input.InputSchema, OutputSchema: input.OutputSchema, Endpoint: input.Endpoint, HTTPMethod: input.HTTPMethod, UpstreamAuth: input.UpstreamAuth, Credential: input.Credential, RequestMapping: input.RequestMapping, ResponseMapping: input.ResponseMapping, RequestExample: input.RequestExample, ResponseExample: input.ResponseExample, AuthorizationPolicy: input.AuthorizationPolicy, TimeoutMS: input.TimeoutMS}, actor(r))
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
		s.creationError(w, err)
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
		publicSources := 0
		for _, item := range sources {
			if item.Visibility == model.VisibilityPublic && item.Published {
				publicSources++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"product":              product,
			"public_mcp_endpoint":  s.baseURL + "/mcp/public",
			"private_mcp_endpoint": s.baseURL + "/mcp",
			"public_sources":       publicSources,
			"agent_setup":          s.agentSetupLinks(r.Context(), product),
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
		s.platformError(w, err, "This source's published content will be accessible without authentication through Public MCP.")
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

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

const reportingAgentInstructions = " When a likely connector-specific defect is found, offer to prepare a bug report but do not submit automatically. Before using a support reporting tool, show the user a concise preview of what will be shared and obtain explicit approval. Explain that DokoSoko adds the authenticated subject, account or installation, applicable Integration, current connector release, and request metadata; contact name and email are added only when allow_contact is approved. Submit only relevant, sanitized context; never include credentials, tokens, unrelated conversation, complete files, or unapproved personal data. For feedback, preserve the user's meaning and never invent ratings, sentiment, or claims."

func reportOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"submission_id": map[string]any{"type": "string"},
			"status":        map[string]any{"type": "string", "enum": []string{"held", "pending", "delivering", "delivered", "failed"}},
			"external_id":   map[string]any{"type": "string"},
			"external_url":  map[string]any{"type": "string", "format": "uri"},
		},
		"required": []string{"submission_id", "status"},
	}
}

func mergeMetadata(current any, additions map[string]any) map[string]any {
	result := make(map[string]any)
	if existing, ok := current.(map[string]any); ok {
		for key, value := range existing {
			result[key] = value
		}
	}
	for key, value := range additions {
		result[key] = value
	}
	return result
}

func bugReportToolDefinition() map[string]any {
	return map[string]any{
		"name":        "support.report_bug",
		"description": "Prepare and submit a connector bug report only after explicit user confirmation. First show the user a concise preview of the exact report content and disclose that trusted authenticated-account, installation, product-version, and request metadata will be added. Contact details are added only when allow_contact is approved. Include relevant reproduction details and sanitized diagnostics only; never include secrets, credentials, unrelated conversation, complete files, or unapproved personal data.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"integration_id":     map[string]any{"type": "string", "description": "The affected Integration ID from com.dokosoko/supportCapabilities metadata. Omit only for a legacy connector with no Integration catalog."},
				"summary":            map[string]any{"type": "string", "minLength": 1, "maxLength": 160, "description": "A concise user-approved title for the defect."},
				"description":        map[string]any{"type": "string", "minLength": 1, "maxLength": 10000, "description": "What happened and why it appears related to this connector."},
				"reproduction_steps": map[string]any{"type": "array", "maxItems": 20, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 1000}},
				"expected_behavior":  map[string]any{"type": "string", "maxLength": 4000},
				"actual_behavior":    map[string]any{"type": "string", "maxLength": 4000},
				"error_code":         map[string]any{"type": "string", "maxLength": 120},
				"error_message":      map[string]any{"type": "string", "maxLength": 8000},
				"stack_trace":        map[string]any{"type": "string", "maxLength": 16000, "description": "Sanitized relevant stack frames only."},
				"diagnostic_context": map[string]any{"type": "string", "maxLength": 20000, "description": "A bounded, sanitized context summary; do not send full files or conversations."},
				"related_tool":       map[string]any{"type": "string", "pattern": `^[A-Za-z0-9_.-]{1,160}$`},
				"integration_run_id": map[string]any{"type": "string", "maxLength": 160},
				"severity":           map[string]any{"type": "string", "enum": []string{"unknown", "low", "medium", "high", "critical"}, "default": "unknown"},
				"allow_contact":      map[string]any{"type": "boolean", "default": false, "description": "Share the authenticated user's contact details for follow-up only when explicitly approved."},
				"idempotency_key":    map[string]any{"type": "string", "minLength": 16, "maxLength": 200, "description": "A stable unique key for this exact approved report."},
			},
			"required": []string{"summary", "description", "idempotency_key"},
		},
		"outputSchema": reportOutputSchema(),
		"annotations":  map[string]any{"readOnlyHint": false, "idempotentHint": true, "destructiveHint": false, "openWorldHint": true},
		"_meta":        map[string]any{"com.dokosoko/confirmationRequired": true, "com.dokosoko/dataHandling": "encrypted-held-sanitized-user-approved"},
	}
}

func feedbackToolDefinition() map[string]any {
	return map[string]any{
		"name":        "support.submit_feedback",
		"description": "Submit connector feedback expressed by the user only after explicit confirmation. First show the user a concise preview and disclose that trusted authenticated-account, installation, product-version, and request metadata will be added. Contact details are added only when allow_contact is approved. Preserve the user's meaning and distinguish it from agent-generated context; never invent ratings, sentiment, claims, or personal details.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"integration_id":     map[string]any{"type": "string", "description": "The Integration ID the experience relates to, from com.dokosoko/supportCapabilities metadata."},
				"message":            map[string]any{"type": "string", "minLength": 1, "maxLength": 10000, "description": "The user's feedback, faithfully summarized or quoted with approval."},
				"category":           map[string]any{"type": "string", "enum": []string{"general", "usability", "documentation", "performance", "feature_request", "other"}, "default": "general"},
				"rating":             map[string]any{"type": "integer", "minimum": 1, "maximum": 5, "description": "Include only when the user explicitly supplied or approved the rating."},
				"related_tool":       map[string]any{"type": "string", "pattern": `^[A-Za-z0-9_.-]{1,160}$`},
				"integration_run_id": map[string]any{"type": "string", "maxLength": 160},
				"allow_contact":      map[string]any{"type": "boolean", "default": false, "description": "Share the authenticated user's contact details for follow-up only when explicitly approved."},
				"idempotency_key":    map[string]any{"type": "string", "minLength": 16, "maxLength": 200, "description": "A stable unique key for this exact approved feedback."},
			},
			"required": []string{"message", "idempotency_key"},
		},
		"outputSchema": reportOutputSchema(),
		"annotations":  map[string]any{"readOnlyHint": false, "idempotentHint": true, "destructiveHint": false, "openWorldHint": true},
		"_meta":        map[string]any{"com.dokosoko/confirmationRequired": true, "com.dokosoko/dataHandling": "encrypted-held-sanitized-user-approved"},
	}
}

func (s *Server) publicMCP(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("productID")
	if productID == "" {
		deployment, err := s.service.Store().Deployment(r.Context())
		if err != nil {
			writeError(w, http.StatusNotFound, "public_mcp_unavailable", "Public MCP is not configured for this deployment.", nil)
			return
		}
		productID = deployment.ID
	}
	product, err := s.service.Store().Product(r.Context(), productID)
	if err != nil || !product.PublicMCPEnabled {
		writeError(w, http.StatusNotFound, "public_mcp_unavailable", "Public MCP is not enabled for this deployment.", nil)
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
	if productID == "" {
		deployment, err := s.service.Store().Deployment(r.Context())
		if err != nil {
			writeError(w, http.StatusNotFound, "mcp_unavailable", "Private MCP is not configured for this deployment.", nil)
			return
		}
		productID = deployment.ID
	}
	var principal identity.Principal
	if s.identityBroker != nil {
		value, err := s.identityBroker.Authenticate(r.Context(), bearerToken(r))
		if err == nil && value.ProductID == productID {
			principal = value
		}
	}
	if principal.Subject == "" && s.allowDemoTokens && isBearer(r, demoPrivateToken) {
		principal = identity.Principal{ProductID: productID, ClientID: productID, Issuer: "development", Subject: "private_mcp_demo", Grants: map[string]bool{}, AccessEvaluatedAt: time.Now().UTC()}
	}
	if principal.Subject == "" {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata=%q, scope=%q`, s.baseURL+"/.well-known/oauth-protected-resource/mcp", "mcp:private"))
		writeError(w, http.StatusUnauthorized, "authentication_required", "Private MCP requires a DokoSoko access token.", nil)
		return
	}
	if !s.allowFixedWindow("private-mcp|"+productID+"|"+vendorActorID(principal), 600, time.Now().UTC()) {
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
	selection := model.ProductSelectionContext{Public: public}
	if !public {
		principal, _ := r.Context().Value(principalKey).(identity.Principal)
		channel, actorKind, actorID = "private_mcp", "vendor_user", pseudonym(productID, principal)
		selection.CustomerAccountID, selection.InstallationID = principal.CustomerAccountID, principal.InstallationID
	}
	productManifest, manifestErr := s.service.ProductManifestFor(r.Context(), productID, selection)
	if manifestErr != nil {
		writeRPCError(w, request.ID, -32603, "Deployment context could not be resolved")
		return
	}
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
			writeRPCError(w, request.ID, -32603, "Deployment discovery failed")
			return
		}
		cacheScope := "private"
		if public {
			cacheScope = "public"
		}
		instructions := "Use the effective DokoSoko connector release and Integration revisions returned in discovery. Authenticated installation, environment, and customer pins override default deployment channels in that order."
		if !public && s.reporting != nil {
			capabilities, _ := s.reporting.Capabilities(r.Context(), productID)
			reportingEnabled := false
			for _, capability := range capabilities {
				reportingEnabled = reportingEnabled || capability.BugReportsEnabled || capability.FeedbackEnabled
			}
			if reportingEnabled {
				instructions += reportingAgentInstructions
			}
		}
		writeRPC(w, request.ID, map[string]any{"resultType": "complete", "supportedVersions": []string{model.StatelessMCPv2Protocol}, "capabilities": map[string]any{"tools": map[string]any{"listChanged": true}, "resources": map[string]any{"listChanged": true}}, "deployment": productManifest, "product": productManifest, "catalogRevision": productManifest.CatalogRevision, "manifestHash": productManifest.ManifestHash, "instructions": instructions, "ttlMs": 30000, "cacheScope": cacheScope})
	case "resources/list":
		values, err := s.publishedRecipes(r.Context(), productID, public)
		if err != nil {
			writeRPCError(w, request.ID, -32603, "Recipe resources could not be listed")
			return
		}
		resources := make([]map[string]any, 0, len(values))
		for _, recipe := range values {
			resources = append(resources, map[string]any{"uri": recipe.StableURI, "name": recipe.Slug, "title": recipe.Title, "description": recipe.Outcome, "mimeType": "text/markdown", "_meta": map[string]any{"generated": recipe.Generated, "state": recipe.State, "revision_id": recipe.CurrentRevisionID}})
		}
		writeRPC(w, request.ID, map[string]any{"resources": resources})
	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if json.Unmarshal(request.Params, &params) != nil || params.URI == "" {
			writeRPCError(w, request.ID, -32602, "A recipe URI is required")
			return
		}
		recipe, err := s.publishedRecipeByURI(r.Context(), productID, params.URI, public)
		if err != nil || recipe.CurrentRevision == nil {
			writeRPCError(w, request.ID, -32004, "Recipe resource not found")
			return
		}
		s.recordAnalytics(r.Context(), productID, "recipe.view", actorKind, actorID, map[string]any{"recipe_id": recipe.ID, "recipe_slug": recipe.Slug, "channel": channel})
		writeRPC(w, request.ID, map[string]any{"contents": []map[string]any{{"uri": recipe.StableURI, "mimeType": "text/markdown", "text": recipe.CurrentRevision.Markdown, "_meta": map[string]any{"revision_id": recipe.CurrentRevisionID, "published_at": recipe.PublishedAt}}}})
	case "tools/list":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Deployment discovery failed")
			return
		}
		tools := []map[string]any{
			{"name": "deployment.get_manifest", "description": "Return this DokoSoko deployment, its applicable Integration revisions, effective pinned or default connector release, and available releases.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}},
			{"name": "deployment.releases.list", "description": "List published connector releases and their latest, LTS, deprecated, replacement, and sunset metadata.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}},
			{"name": "integration.recipes.list", "description": "List published implementation recipes and their stable MCP resource URIs.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}},
			{"name": "integration.plan", "description": "Choose the closest published recipe for a requested integration outcome. This returns a plan reference, not a claim that work was completed.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"outcome": map[string]any{"type": "string", "maxLength": 500}}, "required": []string{"outcome"}}},
			{"name": "integration.check", "description": "Check whether a published recipe URI is current or needs attention before implementation.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"recipe_uri": map[string]any{"type": "string", "maxLength": 500}}, "required": []string{"recipe_uri"}}},
		}
		principal, _ := r.Context().Value(principalKey).(identity.Principal)
		if len(productManifest.Integrations) == 0 {
			tools = append(tools, map[string]any{"name": "search_knowledge", "description": "Search the latest reviewed documentation for a legacy deployment without an Integration catalog.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}}})
		} else {
			generated, _ := s.apiDefaultToolDefinitions(r.Context(), productID, productManifest, principal, public)
			tools = append(tools, generated...)
		}
		if !public {
			tools = append(tools,
				map[string]any{"name": "integration_runs.start", "description": "Start an environment-scoped integration outcome run.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"environment_id": map[string]any{"type": "string"}, "requested_outcome": map[string]any{"type": "string", "maxLength": 500}}, "required": []string{"environment_id", "requested_outcome"}}},
				map[string]any{"name": "integration_runs.complete", "description": "Complete a run with a deterministic validation result.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"run_id": map[string]any{"type": "string"}, "reported_success": map[string]any{"type": "boolean"}, "validated_success": map[string]any{"type": "boolean"}, "failure_code": map[string]any{"type": "string", "maxLength": 120}}, "required": []string{"run_id", "validated_success"}}},
			)
			if s.reporting != nil {
				capabilities, _ := s.reporting.Capabilities(r.Context(), productID)
				bugEnabled, feedbackEnabled := false, false
				for _, capability := range capabilities {
					bugEnabled = bugEnabled || capability.BugReportsEnabled
					feedbackEnabled = feedbackEnabled || capability.FeedbackEnabled
				}
				metadata := map[string]any{"com.dokosoko/supportCapabilities": capabilities}
				if bugEnabled {
					definition := bugReportToolDefinition()
					definition["_meta"] = mergeMetadata(definition["_meta"], metadata)
					tools = append(tools, definition)
				}
				if feedbackEnabled {
					definition := feedbackToolDefinition()
					definition["_meta"] = mergeMetadata(definition["_meta"], metadata)
					tools = append(tools, definition)
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
			if s.accessRuntime != nil && len(productManifest.Integrations) == 0 {
				capabilities := s.accessRuntime.Capabilities(r.Context(), productID, principal.Grants)
				if len(capabilities) > 0 {
					metadata := map[string]any{"com.dokosoko/accessConnections": capabilities}
					canCreateInstance, canCreateCredential, canRevokeCredential := false, false, false
					for _, capability := range capabilities {
						canCreateInstance = canCreateInstance || capability.CanCreateInstance
						canCreateCredential = canCreateCredential || capability.CanCreateCredential
						canRevokeCredential = canRevokeCredential || capability.CanRevokeCredential
					}
					tools = append(tools,
						map[string]any{"name": "access.instances.list", "description": "List provider-owned resources available to the authenticated subject. The provider-specific resource label and allowed Integrations are supplied in tool metadata.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"connection_id": map[string]any{"type": "string"}, "integration_id": map[string]any{"type": "string"}}, "required": []string{"connection_id", "integration_id"}}, "_meta": metadata},
						map[string]any{"name": "access.credentials.list", "description": "List credential metadata and fingerprints for an allowed provider connection or resource. Credential material is never returned by list operations.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"connection_id": map[string]any{"type": "string"}, "integration_id": map[string]any{"type": "string"}, "access_instance_id": map[string]any{"type": "string"}}, "required": []string{"connection_id", "integration_id"}}, "_meta": metadata},
					)
					if canCreateInstance {
						tools = append(tools, map[string]any{"name": "access.instances.create", "description": "Create an idempotent provider resource using the provider-specific label shown in tool metadata. This tool is omitted for single-instance services.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"connection_id": map[string]any{"type": "string"}, "integration_id": map[string]any{"type": "string"}, "environment_id": map[string]any{"type": "string"}, "display_name": map[string]any{"type": "string", "maxLength": 160}, "idempotency_key": map[string]any{"type": "string", "minLength": 16}, "ttl_seconds": map[string]any{"type": "integer", "minimum": 300}}, "required": []string{"connection_id", "integration_id", "environment_id", "display_name", "idempotency_key"}}, "_meta": metadata})
					}
					if canCreateCredential {
						tools = append(tools, map[string]any{"name": "access.credentials.create", "description": "Create scoped credential material once for an allowed provider connection or resource. DokoSoko retains only a fingerprint unless the provider definition explicitly requires encrypted managed storage.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"connection_id": map[string]any{"type": "string"}, "integration_id": map[string]any{"type": "string"}, "environment_id": map[string]any{"type": "string"}, "access_instance_id": map[string]any{"type": "string"}, "scopes": map[string]any{"type": "array", "maxItems": 20, "items": map[string]any{"type": "string"}}, "idempotency_key": map[string]any{"type": "string", "minLength": 16}, "ttl_seconds": map[string]any{"type": "integer", "minimum": 300}}, "required": []string{"connection_id", "integration_id", "environment_id", "scopes", "idempotency_key"}}, "_meta": metadata})
					}
					if canRevokeCredential {
						tools = append(tools, map[string]any{"name": "access.credentials.revoke", "description": "Revoke provider credential material owned by the authenticated subject.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"credential_id": map[string]any{"type": "string"}}, "required": []string{"credential_id"}}, "_meta": metadata})
					}
				}
			}
			if s.toolRuntime != nil {
				custom, err := s.toolRuntime.Published(r.Context(), productID)
				if err == nil {
					type namedCustomDefinition struct {
						name       string
						definition map[string]any
					}
					candidates := make([]namedCustomDefinition, 0, len(custom))
					nameCounts := make(map[string]int)
					for _, item := range custom {
						_, allowed, allowErr := s.service.ProductVersionAllowsToolFor(r.Context(), productID, selection, item)
						if allowErr != nil || !allowed {
							continue
						}
						binding, managedByIntegration, bindingErr := s.integrationToolAuthorization(r.Context(), productManifest, item)
						if managedByIntegration {
							if bindingErr != nil {
								continue
							}
							available, availableErr := s.toolRuntime.AvailableBound(r.Context(), productID, []toolruntime.BoundAuthorization{binding}, toolPrincipal(principal, false, "", ""))
							if availableErr != nil || len(available) != 1 || available[0].ID != item.ID {
								continue
							}
						} else {
							available, availableErr := s.toolRuntime.Available(r.Context(), productID, principal.Grants)
							legacyAllowed := false
							for _, candidate := range available {
								legacyAllowed = legacyAllowed || candidate.ID == item.ID
							}
							if availableErr != nil || !legacyAllowed {
								continue
							}
						}
						canonicalName, canonical := canonicalCustomToolName(productManifest, item)
						if !canonical {
							continue
						}
						definition := customToolDefinitionForAuthorization(productManifest, item, binding, managedByIntegration)
						definition["name"] = canonicalName
						if len(item.OutputSchema) > 0 {
							definition["outputSchema"] = item.OutputSchema
						}
						nameCounts[canonicalName]++
						candidates = append(candidates, namedCustomDefinition{name: canonicalName, definition: definition})
					}
					for _, candidate := range candidates {
						if nameCounts[candidate.name] == 1 {
							tools = append(tools, candidate.definition)
						}
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
			metadata, _ := definition["_meta"].(map[string]any)
			if metadata == nil {
				metadata = make(map[string]any)
			}
			metadata["com.dokosoko/productVersion"] = versionMeta
			metadata["com.dokosoko/deploymentRelease"] = versionMeta
			definition["_meta"] = metadata
		}
		writeRPC(w, request.ID, map[string]any{"resultType": "complete", "deployment": productManifest, "product": productManifest, "catalogRevision": productManifest.CatalogRevision, "manifestHash": productManifest.ManifestHash, "tools": tools, "ttlMs": 30000, "cacheScope": cacheScope})
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

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Meta      struct {
		Confirmed             bool   `json:"confirmed"`
		ConfirmationChallenge string `json:"confirmation_challenge"`
		IdempotencyKey        string `json:"idempotency_key"`
	} `json:"_meta"`
}

func decodeToolCallParams(raw json.RawMessage) (toolCallParams, error) {
	var params toolCallParams
	paramsDecoder := json.NewDecoder(bytes.NewReader(raw))
	paramsDecoder.UseNumber()
	err := paramsDecoder.Decode(&params)
	return params, err
}

type managedToolConfirmationChallenge struct {
	Nonce     string
	ExpiresAt time.Time
}

type managedToolConfirmationHashInput struct {
	ProductID                  string         `json:"product_id"`
	ToolID                     string         `json:"tool_id"`
	ToolRevision               int64          `json:"tool_revision"`
	IntegrationID              string         `json:"integration_id"`
	AuthorizationPointID       string         `json:"authorization_point_id"`
	AuthorizationPointRevision int64          `json:"authorization_point_revision"`
	Issuer                     string         `json:"issuer"`
	Subject                    string         `json:"subject"`
	CustomerAccountID          string         `json:"customer_account_id"`
	InstallationID             string         `json:"installation_id"`
	AccessEvaluationID         string         `json:"access_evaluation_id"`
	AccessEvaluatedAt          string         `json:"access_evaluated_at"`
	IdempotencyKey             string         `json:"idempotency_key"`
	Arguments                  map[string]any `json:"arguments"`
}

func managedToolConfirmationArgumentHash(productID string, tool model.Tool, binding toolruntime.BoundAuthorization, principal identity.Principal, arguments map[string]any, idempotencyKey string) ([]byte, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	payload, err := json.Marshal(managedToolConfirmationHashInput{
		ProductID:                  productID,
		ToolID:                     tool.ID,
		ToolRevision:               tool.Revision,
		IntegrationID:              binding.IntegrationID,
		AuthorizationPointID:       binding.AuthorizationPoint.ID,
		AuthorizationPointRevision: binding.AuthorizationPointRevision,
		Issuer:                     principal.Issuer,
		Subject:                    principal.Subject,
		CustomerAccountID:          principal.CustomerAccountID,
		InstallationID:             principal.InstallationID,
		AccessEvaluationID:         principal.AccessEvaluationID,
		AccessEvaluatedAt:          principal.AccessEvaluatedAt.UTC().Format(time.RFC3339Nano),
		IdempotencyKey:             idempotencyKey,
		Arguments:                  arguments,
	})
	if err != nil {
		return nil, errors.New("managed tool arguments are not canonical JSON")
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(managedToolConfirmationDomain))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(payload)
	return digest.Sum(nil), nil
}

func randomManagedToolConfirmationUUID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func randomManagedToolConfirmationNonce() (string, []byte, error) {
	raw := make([]byte, managedToolConfirmationNonceBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	nonce := "mtc_" + base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(nonce))
	return nonce, digest[:], nil
}

func managedToolConfirmationActor(principal identity.Principal) (string, error) {
	actorID := vendorActorID(principal)
	if actorID == "" || strings.TrimSpace(principal.AccessEvaluationID) == "" || principal.AccessEvaluatedAt.IsZero() {
		return "", errors.New("managed tool confirmation requires an exact authenticated access evaluation")
	}
	return actorID, nil
}

func (s *Server) issueManagedToolConfirmation(ctx context.Context, productID string, tool model.Tool, binding toolruntime.BoundAuthorization, principal identity.Principal, arguments map[string]any, idempotencyKey string, now time.Time) (managedToolConfirmationChallenge, error) {
	actorID, err := managedToolConfirmationActor(principal)
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	argumentHash, err := managedToolConfirmationArgumentHash(productID, tool, binding, principal, arguments, idempotencyKey)
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	nonce, nonceDigest, err := randomManagedToolConfirmationNonce()
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	id, err := randomManagedToolConfirmationUUID()
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	expiresAt := now.Add(managedToolConfirmationTTL)
	decisionExpiresAt := principal.AccessEvaluatedAt.Add(time.Duration(binding.AuthorizationPoint.DecisionTTLSeconds) * time.Second)
	if decisionExpiresAt.Before(expiresAt) {
		expiresAt = decisionExpiresAt
	}
	if !now.Before(expiresAt) {
		return managedToolConfirmationChallenge{}, errors.New("the access evaluation expires before confirmation can be issued")
	}
	confirmation := model.ToolTestConfirmation{
		ID:             id,
		OrganisationID: tool.OrganisationID,
		ProductID:      productID,
		ToolID:         tool.ID,
		ToolRevision:   tool.Revision,
		ArgumentHash:   argumentHash,
		NonceDigest:    nonceDigest,
		ActorID:        actorID,
		ExpiresAt:      expiresAt,
		CreatedAt:      now,
	}
	if err := s.service.Store().CreateToolTestConfirmation(ctx, confirmation); err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	return managedToolConfirmationChallenge{Nonce: nonce, ExpiresAt: expiresAt}, nil
}

func (s *Server) consumeManagedToolConfirmation(ctx context.Context, challenge, productID string, tool model.Tool, binding toolruntime.BoundAuthorization, principal identity.Principal, arguments map[string]any, idempotencyKey string, now time.Time) error {
	if len(challenge) != len("mtc_")+base64.RawURLEncoding.EncodedLen(managedToolConfirmationNonceBytes) || !strings.HasPrefix(challenge, "mtc_") {
		return errors.New("managed tool confirmation challenge is malformed")
	}
	actorID, err := managedToolConfirmationActor(principal)
	if err != nil {
		return err
	}
	argumentHash, err := managedToolConfirmationArgumentHash(productID, tool, binding, principal, arguments, idempotencyKey)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(challenge))
	consumptionID, err := randomManagedToolConfirmationUUID()
	if err != nil {
		return err
	}
	_, err = s.service.Store().ConsumeToolTestConfirmation(ctx, digest[:], productID, tool.ID, tool.Revision, argumentHash, actorID, consumptionID, now)
	return err
}

func managedToolPolicy(tool model.Tool, binding toolruntime.BoundAuthorization) (confirmationRequired, idempotencyRequired bool, err error) {
	var policy struct {
		ConfirmationRequired bool `json:"confirmation_required"`
		IdempotencyRequired  bool `json:"idempotency_required"`
	}
	if err := json.Unmarshal(tool.AuthorizationPolicy, &policy); err != nil {
		return false, false, err
	}
	return policy.ConfirmationRequired || binding.AuthorizationPoint.ConfirmationRequired, strings.ToUpper(strings.TrimSpace(tool.HTTPMethod)) != http.MethodGet && policy.IdempotencyRequired, nil
}

func writeManagedToolConfirmationRequired(w http.ResponseWriter, id any, challenge managedToolConfirmationChallenge, tool model.Tool, binding toolruntime.BoundAuthorization) {
	writeRPCErrorData(w, id, -32003, "Client confirmation attestation is required for this exact managed tool invocation", map[string]any{
		"confirmation_required":          true,
		"confirmation_challenge":         challenge.Nonce,
		"expires_at":                     challenge.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"retry_metadata_field":           "params._meta." + managedToolConfirmationMetaField,
		"confirmation_attestation_field": "params._meta.confirmed",
		"confirmation_attestation_value": true,
		"tool_id":                        tool.ID,
		"tool_revision":                  tool.Revision,
		"authorization_point_id":         binding.AuthorizationPoint.ID,
		"authorization_point_revision":   binding.AuthorizationPointRevision,
		"notice":                         "Retrying with the challenge and confirmed=true is the client's attestation that it obtained confirmation; the server does not independently prove that a human approved.",
	})
}

func (s *Server) callTool(ctx context.Context, w http.ResponseWriter, request rpcRequest, productID string, public bool, productManifest model.ProductManifest, manifestErr error) {
	params, err := decodeToolCallParams(request.Params)
	if err != nil {
		writeRPCError(w, request.ID, -32602, "Invalid params")
		return
	}
	principal, _ := ctx.Value(principalKey).(identity.Principal)
	actorKind, actorID, channel := "anonymous", "", "public_mcp"
	if !public {
		actorKind, actorID, channel = "vendor_user", pseudonym(productID, principal), "private_mcp"
	}
	selection := model.ProductSelectionContext{Public: public}
	if !public {
		selection.CustomerAccountID, selection.InstallationID = principal.CustomerAccountID, principal.InstallationID
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
	if manifestErr == nil && len(productManifest.Integrations) > 0 {
		_, generatedBindings := s.apiDefaultToolDefinitions(ctx, productID, productManifest, principal, public)
		if binding, ok := generatedBindings[params.Name]; ok {
			s.executeAPIDefaultTool(ctx, w, request, params, productID, binding, public, productManifest, selection, principal)
			return
		}
	}
	switch params.Name {
	case "integration.recipes.list":
		values, err := s.publishedRecipes(ctx, productID, public)
		if err != nil {
			writeRPCError(w, request.ID, -32603, "Recipes could not be listed")
			return
		}
		writeToolResult(w, request.ID, map[string]any{"recipes": values})
	case "integration.plan":
		outcome, _ := params.Arguments["outcome"].(string)
		values, err := s.publishedRecipes(ctx, productID, public)
		if err != nil || strings.TrimSpace(outcome) == "" {
			writeRPCError(w, request.ID, -32602, "A valid integration outcome is required")
			return
		}
		var selected *model.Recipe
		needle := strings.ToLower(strings.TrimSpace(outcome))
		for index := range values {
			candidate := strings.ToLower(values[index].Title + " " + values[index].Outcome)
			if selected == nil || strings.Contains(candidate, needle) || strings.Contains(needle, strings.ToLower(values[index].Slug)) {
				copy := values[index]
				selected = &copy
				if strings.Contains(candidate, needle) {
					break
				}
			}
		}
		if selected == nil {
			writeRPCError(w, request.ID, -32004, "No published recipe matches this outcome")
			return
		}
		s.recordAnalytics(ctx, productID, "recipe.plan_selected", actorKind, actorID, map[string]any{"recipe_id": selected.ID, "recipe_slug": selected.Slug, "channel": channel})
		writeToolResult(w, request.ID, map[string]any{"recipe_uri": selected.StableURI, "title": selected.Title, "outcome": selected.Outcome, "revision_id": selected.CurrentRevisionID, "next_step": "Read the recipe resource, verify its prerequisites, then implement and validate each step."})
	case "integration.check":
		recipeURI, _ := params.Arguments["recipe_uri"].(string)
		recipe, err := s.publishedRecipeByURI(ctx, productID, recipeURI, public)
		if err != nil {
			writeRPCError(w, request.ID, -32004, "Recipe resource not found")
			return
		}
		writeToolResult(w, request.ID, map[string]any{"recipe_uri": recipe.StableURI, "state": recipe.State, "current": recipe.State == "published" && !recipe.NeedsAttention, "needs_attention": recipe.NeedsAttention, "revision_id": recipe.CurrentRevisionID, "published_at": recipe.PublishedAt})
	case "deployment.get_manifest", "product.get_manifest":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Deployment discovery failed")
			return
		}
		writeToolResult(w, request.ID, productManifest)
	case "deployment.releases.list", "product.versions.list":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Connector release discovery failed")
			return
		}
		writeToolResult(w, request.ID, map[string]any{"product_id": productManifest.ProductID, "catalog_revision": productManifest.CatalogRevision, "manifest_hash": productManifest.ManifestHash, "effective_version": productManifest.EffectiveVersion, "selection_source": productManifest.SelectionSource, "available_versions": productManifest.AvailableVersions, "operational_warnings": productManifest.OperationalWarnings})
	case "support.report_bug":
		if public || s.reporting == nil {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		if !params.Meta.Confirmed {
			writeRPCError(w, request.ID, -32003, "Explicit user confirmation is required after previewing the exact bug report")
			return
		}
		if !s.allowFixedWindow("support-reporting|"+productID+"|"+vendorActorID(principal), 30, time.Now().UTC()) {
			w.Header().Set("Retry-After", "60")
			writeRPCError(w, request.ID, -32029, "Support reporting request limit exceeded")
			return
		}
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Trusted product context is unavailable")
			return
		}
		var input reporting.BugInput
		if err := decodeArguments(params.Arguments, &input); err != nil {
			writeRPCError(w, request.ID, -32602, "Bug report arguments are invalid")
			return
		}
		integration, err := s.reportIntegrationContext(ctx, productID, input.IntegrationID)
		if err != nil {
			writeRPCError(w, request.ID, -32602, "The selected Integration is not available in this deployment")
			return
		}
		requestID, _ := ctx.Value(requestIDKey).(string)
		value, err := s.reporting.SubmitBug(ctx, input, reporting.SubmitContext{Principal: principal, ActorPseudonym: actorID, Product: reportProductContext(productManifest), Integration: integration, RequestID: requestID})
		if err != nil {
			reportingRPCError(w, request.ID, err)
			return
		}
		s.recordAnalytics(ctx, productID, "support.bug_reported", "vendor_user", actorID, map[string]any{"channel": "private_mcp", "state": value.State})
		writeToolResult(w, request.ID, reportToolResult(value))
	case "support.submit_feedback":
		if public || s.reporting == nil {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		if !params.Meta.Confirmed {
			writeRPCError(w, request.ID, -32003, "Explicit user confirmation is required after previewing the exact feedback")
			return
		}
		if !s.allowFixedWindow("support-reporting|"+productID+"|"+vendorActorID(principal), 30, time.Now().UTC()) {
			w.Header().Set("Retry-After", "60")
			writeRPCError(w, request.ID, -32029, "Support reporting request limit exceeded")
			return
		}
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Trusted product context is unavailable")
			return
		}
		var input reporting.FeedbackInput
		if err := decodeArguments(params.Arguments, &input); err != nil {
			writeRPCError(w, request.ID, -32602, "Feedback arguments are invalid")
			return
		}
		integration, err := s.reportIntegrationContext(ctx, productID, input.IntegrationID)
		if err != nil {
			writeRPCError(w, request.ID, -32602, "The selected Integration is not available in this deployment")
			return
		}
		requestID, _ := ctx.Value(requestIDKey).(string)
		value, err := s.reporting.SubmitFeedback(ctx, input, reporting.SubmitContext{Principal: principal, ActorPseudonym: actorID, Product: reportProductContext(productManifest), Integration: integration, RequestID: requestID})
		if err != nil {
			reportingRPCError(w, request.ID, err)
			return
		}
		s.recordAnalytics(ctx, productID, "support.feedback_submitted", "vendor_user", actorID, map[string]any{"channel": "private_mcp", "state": value.State})
		writeToolResult(w, request.ID, reportToolResult(value))
	case "mcp_connections.authorize":
		if public || s.mcpBridge == nil {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		connectionID, _ := params.Arguments["connection_id"].(string)
		requestID, _ := ctx.Value(requestIDKey).(string)
		authorizationURL, err := s.mcpBridge.BeginAuthorization(ctx, productID, connectionID, toolPrincipal(principal, false, requestID, ""))
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
		value, err := s.service.StartIntegrationRun(ctx, productID, input.EnvironmentID, input.RequestedOutcome, platform.Actor{ID: vendorActorID(principal), RequestID: requestID})
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
		value, err := s.service.CompleteIntegrationRun(ctx, productID, input.RunID, input.ReportedSuccess, input.ValidatedSuccess, input.FailureCode, platform.Actor{ID: vendorActorID(principal), RequestID: requestID})
		if err != nil {
			writeRPCError(w, request.ID, -32602, "Integration run could not be completed")
			return
		}
		writeToolResult(w, request.ID, value)
	case "access.instances.list":
		if public || s.accessRuntime == nil || len(productManifest.Integrations) > 0 {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		connectionID, _ := params.Arguments["connection_id"].(string)
		integrationID, _ := params.Arguments["integration_id"].(string)
		values, err := s.accessRuntime.ListInstances(ctx, productID, connectionID, integrationID, accessPrincipal(principal, ctx))
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		writeToolResult(w, request.ID, map[string]any{"instances": values})
	case "access.instances.create":
		if public || s.accessRuntime == nil || len(productManifest.Integrations) > 0 {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		var input struct {
			ConnectionID string `json:"connection_id"`
			accessruntime.InstanceRequest
		}
		if decodeArguments(params.Arguments, &input) != nil {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		value, err := s.accessRuntime.CreateInstance(ctx, productID, input.ConnectionID, input.InstanceRequest, accessPrincipal(principal, ctx))
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		s.recordAnalytics(ctx, productID, "access_instance.created", "vendor_user", pseudonym(productID, principal), map[string]any{"connection_id": input.ConnectionID, "integration_id": input.IntegrationID})
		writeToolResult(w, request.ID, value)
	case "access.credentials.list":
		if public || s.accessRuntime == nil || len(productManifest.Integrations) > 0 {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		connectionID, _ := params.Arguments["connection_id"].(string)
		integrationID, _ := params.Arguments["integration_id"].(string)
		instanceID, _ := params.Arguments["access_instance_id"].(string)
		values, err := s.accessRuntime.ListCredentials(ctx, productID, connectionID, integrationID, instanceID, accessPrincipal(principal, ctx))
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		writeToolResult(w, request.ID, map[string]any{"credentials": values})
	case "access.credentials.create":
		if public || s.accessRuntime == nil || len(productManifest.Integrations) > 0 {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		var input struct {
			ConnectionID string `json:"connection_id"`
			accessruntime.CredentialRequest
		}
		if decodeArguments(params.Arguments, &input) != nil {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		value, err := s.accessRuntime.IssueCredential(ctx, productID, input.ConnectionID, input.CredentialRequest, accessPrincipal(principal, ctx))
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		s.recordAnalytics(ctx, productID, "access_credential.created", "vendor_user", pseudonym(productID, principal), map[string]any{"connection_id": input.ConnectionID, "integration_id": input.IntegrationID, "existing": value.Existing})
		writeToolResult(w, request.ID, value)
	case "access.credentials.revoke":
		if public || s.accessRuntime == nil || len(productManifest.Integrations) > 0 {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		credentialID, _ := params.Arguments["credential_id"].(string)
		value, err := s.accessRuntime.RevokeCredential(ctx, productID, credentialID, accessPrincipal(principal, ctx))
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		writeToolResult(w, request.ID, value)
	case "search_knowledge":
		if len(productManifest.Integrations) > 0 {
			writeRPCError(w, request.ID, -32601, "Tool is not available for Integration-catalog deployments")
			return
		}
		query, _ := params.Arguments["query"].(string)
		integrationID, _ := params.Arguments["integration_id"].(string)
		publicationIDs, scopeErr := s.knowledgePublicationIDs(ctx, productID, integrationID, productManifest)
		if scopeErr != nil {
			writeRPCError(w, request.ID, -32003, "Select exactly one published Integration with reviewed documentation")
			return
		}
		if public {
			items, err := s.service.Store().PublicKnowledge(ctx, productID, publicationIDs, query)
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
		items, err := s.service.Store().PrivateKnowledge(ctx, productID, publicationIDs, query)
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
	default:
		if public {
			writeRPCError(w, request.ID, -32601, "Tool is not available on Public MCP")
			return
		}
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32003, "Tool execution requires a resolved deployment and customer selection context")
			return
		}
		if s.toolRuntime != nil {
			requestID, _ := ctx.Value(requestIDKey).(string)
			principal, _ := ctx.Value(principalKey).(identity.Principal)
			selectedTool, lookupErr := s.executableTool(ctx, productID, params.Name, selection)
			if errors.Is(lookupErr, errToolVersionExcluded) {
				writeRPCError(w, request.ID, -32003, "Tool is not included in the effective product version")
				return
			}
			if lookupErr != nil || selectedTool.ID == "" {
				writeRPCError(w, request.ID, -32601, "Tool not found")
				return
			}
			var value any
			var err error
			bound, managedByIntegration, bindingErr := s.integrationToolAuthorization(ctx, productManifest, selectedTool)
			if managedByIntegration {
				if bindingErr != nil {
					writeRPCError(w, request.ID, -32003, "Tool has no unique exact authorization action in an applicable published Integration revision")
					return
				}
				confirmationRequired, idempotencyRequired, policyErr := managedToolPolicy(selectedTool, bound)
				if policyErr != nil {
					writeRPCError(w, request.ID, -32003, "Tool execution was denied by policy")
					return
				}
				if (params.Meta.IdempotencyKey != "" && !toolruntime.ValidIdempotencyKey(params.Meta.IdempotencyKey)) || (idempotencyRequired && !toolruntime.ValidIdempotencyKey(params.Meta.IdempotencyKey)) {
					writeRPCError(w, request.ID, -32602, "params._meta.idempotency_key must contain 16 to 200 visible ASCII characters")
					return
				}
				managedPrincipal := toolPrincipal(principal, false, requestID, params.Meta.IdempotencyKey)
				managedPrincipal.EnvironmentID = productManifest.EnvironmentID
				if confirmationRequired {
					if params.Arguments == nil {
						params.Arguments = map[string]any{}
					}
					if validationErr := toolruntime.ValidateArguments(selectedTool.InputSchema, params.Arguments); validationErr != nil {
						writeRPCError(w, request.ID, -32602, "Tool arguments do not match the declared input schema")
						return
					}
					available, availabilityErr := s.toolRuntime.AvailableBound(ctx, productID, []toolruntime.BoundAuthorization{bound}, managedPrincipal)
					if availabilityErr != nil || len(available) != 1 || available[0].ID != selectedTool.ID || available[0].Revision != selectedTool.Revision {
						writeRPCError(w, request.ID, -32003, "Tool execution was denied by policy")
						return
					}
					now := time.Now().UTC()
					if strings.TrimSpace(params.Meta.ConfirmationChallenge) == "" {
						challenge, challengeErr := s.issueManagedToolConfirmation(ctx, productID, selectedTool, bound, principal, params.Arguments, params.Meta.IdempotencyKey, now)
						if challengeErr != nil {
							writeRPCError(w, request.ID, -32603, "A confirmation challenge could not be issued safely")
							return
						}
						writeManagedToolConfirmationRequired(w, request.ID, challenge, selectedTool, bound)
						return
					}
					if !params.Meta.Confirmed {
						writeRPCErrorData(w, request.ID, -32003, "Client confirmation attestation is required with the server-issued challenge", map[string]any{
							"confirmation_required":          true,
							"confirmation_attestation_field": "params._meta.confirmed",
							"confirmation_attestation_value": true,
							"notice":                         "confirmed=true is a client attestation; the server does not independently prove that a human approved.",
						})
						return
					}
					if confirmationErr := s.consumeManagedToolConfirmation(ctx, params.Meta.ConfirmationChallenge, productID, selectedTool, bound, principal, params.Arguments, params.Meta.IdempotencyKey, now); confirmationErr != nil {
						writeRPCErrorData(w, request.ID, -32003, "The confirmation challenge is invalid, expired, already used, or does not match this exact invocation", map[string]any{
							"confirmation_required":                        true,
							"retry_without_challenge_to_request_a_new_one": true,
						})
						return
					}
					managedPrincipal.Confirmed = true
				}
				runtimeName := selectedTool.Namespace + "." + selectedTool.Name
				value, err = s.toolRuntime.ExecuteBound(ctx, productID, runtimeName, params.Arguments, managedPrincipal, bound)
			} else {
				runtimePrincipal := toolPrincipal(principal, params.Meta.Confirmed, requestID, params.Meta.IdempotencyKey)
				runtimePrincipal.EnvironmentID = productManifest.EnvironmentID
				runtimeName := selectedTool.Namespace + "." + selectedTool.Name
				value, err = s.toolRuntime.Execute(ctx, productID, runtimeName, params.Arguments, runtimePrincipal)
			}
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
			if errors.Is(err, toolruntime.ErrInvalidIdempotencyKey) {
				writeRPCError(w, request.ID, -32602, "params._meta.idempotency_key must contain 16 to 200 visible ASCII characters")
				return
			}
			if errors.Is(err, toolruntime.ErrRateLimited) {
				writeRPCError(w, request.ID, -32029, "The upstream tool connection request limit was exceeded")
				return
			}
			if errors.Is(err, mcpbridge.ErrGrantRequired) {
				writeRPCError(w, request.ID, -32001, "Authorize this Stateless MCPv2 connection with mcp_connections.authorize before calling its tools")
				return
			}
			writeRPCError(w, request.ID, -32603, "Tool execution failed safely; review the tool activity for the sanitized failure category")
			return
		}
		writeRPCError(w, request.ID, -32601, "Tool not found")
	}
}

var errToolVersionExcluded = errors.New("tool is excluded from the effective product version")

// executableTool resolves the exact row that was checked against the
// effective-version allowlist. Callers must not fall through to Runtime's
// second lookup when this read fails or finds no row: doing so would create a
// publication race that bypasses the version check.
func (s *Server) executableTool(ctx context.Context, productID, fullName string, selection model.ProductSelectionContext) (model.Tool, error) {
	available, err := s.service.Store().Tools(ctx, productID, false)
	if err != nil {
		return model.Tool{}, err
	}
	manifest, manifestErr := s.service.ProductManifestFor(ctx, productID, selection)
	matches := make([]model.Tool, 0, 1)
	excluded := false
	for _, candidate := range available {
		legacyName := candidate.Namespace + "." + candidate.Name
		canonicalName, canonical := canonicalCustomToolName(manifest, candidate)
		if legacyName != fullName && (manifestErr != nil || !canonical || canonicalName != fullName) {
			continue
		}
		_, allowed, allowErr := s.service.ProductVersionAllowsToolFor(ctx, productID, selection, candidate)
		if allowErr != nil || !allowed {
			excluded = true
			continue
		}
		matches = append(matches, candidate)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return model.Tool{}, store.ErrConflict
	}
	if excluded {
		return model.Tool{}, errToolVersionExcluded
	}
	return model.Tool{}, store.ErrNotFound
}

func (s *Server) knowledgePublicationIDs(ctx context.Context, productID, requestedIntegrationID string, manifest model.ProductManifest) ([]string, error) {
	requestedIntegrationID = strings.TrimSpace(requestedIntegrationID)
	type candidate struct {
		integrationID string
		publications  []string
	}
	candidates := make([]candidate, 0)
	for _, integration := range manifest.Integrations {
		publications := make([]string, 0)
		seen := make(map[string]bool)
		for _, resource := range integration.Resources {
			if resource.Kind != "documentation" {
				continue
			}
			for _, publication := range resource.SourcePublications {
				if publication.ID != "" && !seen[publication.ID] {
					seen[publication.ID] = true
					publications = append(publications, publication.ID)
				}
			}
		}
		if len(publications) > 0 {
			sort.Strings(publications)
			candidates = append(candidates, candidate{integrationID: integration.ID, publications: publications})
		}
	}
	if requestedIntegrationID != "" {
		for _, value := range candidates {
			if value.integrationID == requestedIntegrationID {
				return value.publications, nil
			}
		}
		return nil, store.ErrNotFound
	}
	if len(candidates) == 1 {
		return candidates[0].publications, nil
	}
	if len(manifest.Integrations) > 0 {
		return nil, store.ErrConflict
	}
	// Legacy deployments without an Integration catalog remain usable, but are
	// still limited to the latest explicitly reviewed publication per source.
	sources, err := s.service.Store().Sources(ctx, productID)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(sources))
	for _, source := range sources {
		publications, publicationErr := s.service.Store().SourcePublications(ctx, productID, source.ID)
		if publicationErr != nil {
			if errors.Is(publicationErr, store.ErrNotFound) {
				continue
			}
			return nil, publicationErr
		}
		if len(publications) > 0 {
			result = append(result, publications[0].ID)
		}
	}
	if len(result) == 0 {
		return nil, store.ErrNotFound
	}
	sort.Strings(result)
	return result, nil
}

func (s *Server) integrationToolAuthorization(ctx context.Context, manifest model.ProductManifest, tool model.Tool) (toolruntime.BoundAuthorization, bool, error) {
	managed := manifest.ManagedIntegrationTools
	type candidate struct {
		integration model.IntegrationManifest
		tool        model.IntegrationManifestTool
	}
	candidates := make([]candidate, 0, 1)
	managedIntegrations := 0
	for _, integration := range manifest.Integrations {
		if integration.Tools == nil {
			continue
		}
		managed = true
		managedIntegrations++
		for _, binding := range integration.Tools {
			if binding.ToolID == tool.ID && binding.ToolRevision == tool.Revision {
				candidates = append(candidates, candidate{integration: integration, tool: binding})
			}
		}
	}
	if !managed {
		return toolruntime.BoundAuthorization{}, false, nil
	}
	if (manifest.SelectionSource == "" || manifest.SelectionSource == "unversioned") && managedIntegrations > 1 {
		return toolruntime.BoundAuthorization{}, true, errors.New("multiple managed Integrations require an exact customer, installation, or product-version selection")
	}
	if len(candidates) != 1 {
		return toolruntime.BoundAuthorization{}, true, errors.New("tool must resolve to exactly one applicable Integration")
	}
	selected := candidates[0]
	if selected.tool.AuthorizationPointID == "" || selected.tool.AuthorizationPointRevision < 1 {
		return toolruntime.BoundAuthorization{}, true, errors.New("tool has no exact authorization-point binding")
	}
	pointPublished := false
	for _, point := range selected.integration.AuthorizationPoints {
		if point.ID == selected.tool.AuthorizationPointID && point.Revision == selected.tool.AuthorizationPointRevision {
			pointPublished = true
			break
		}
	}
	if !pointPublished {
		return toolruntime.BoundAuthorization{}, true, errors.New("tool authorization point is not part of the same published Integration revision")
	}
	point, err := s.service.Store().AuthorizationPoint(ctx, selected.integration.ID, selected.tool.AuthorizationPointID)
	if err != nil || point.State != "active" || point.Revision != selected.tool.AuthorizationPointRevision {
		return toolruntime.BoundAuthorization{}, true, errors.New("tool authorization point changed or is not active")
	}
	definitions, err := s.service.Store().GrantDefinitions(ctx, manifest.DeploymentID)
	if err != nil {
		return toolruntime.BoundAuthorization{}, true, errors.New("grant registry could not be resolved")
	}
	active := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		active[definition.Key] = definition.State == "active"
	}
	for _, required := range point.RequiredGrants {
		if !active[required] {
			return toolruntime.BoundAuthorization{}, true, errors.New("tool authorization point requires an inactive grant")
		}
	}
	return toolruntime.BoundAuthorization{IntegrationID: selected.integration.ID, ToolID: tool.ID, ToolRevision: tool.Revision, AuthorizationPoint: point, AuthorizationPointRevision: point.Revision}, true, nil
}

func customToolDefinition(manifest model.ProductManifest, tool model.Tool) map[string]any {
	return customToolDefinitionForAuthorization(manifest, tool, toolruntime.BoundAuthorization{}, false)
}

func customToolDefinitionForAuthorization(manifest model.ProductManifest, tool model.Tool, binding toolruntime.BoundAuthorization, managed bool) map[string]any {
	var policy struct {
		RequiredGrants       []string `json:"required_grants"`
		ConfirmationRequired bool     `json:"confirmation_required"`
		Risk                 string   `json:"risk"`
		IdempotencyRequired  bool     `json:"idempotency_required"`
	}
	_ = json.Unmarshal(tool.AuthorizationPolicy, &policy)
	if managed {
		seen := make(map[string]bool, len(policy.RequiredGrants)+len(binding.AuthorizationPoint.RequiredGrants))
		combined := make([]string, 0, len(policy.RequiredGrants)+len(binding.AuthorizationPoint.RequiredGrants))
		for _, required := range append(append([]string(nil), policy.RequiredGrants...), binding.AuthorizationPoint.RequiredGrants...) {
			if !seen[required] {
				seen[required] = true
				combined = append(combined, required)
			}
		}
		sort.Strings(combined)
		policy.RequiredGrants = combined
		policy.ConfirmationRequired = policy.ConfirmationRequired || binding.AuthorizationPoint.ConfirmationRequired
	}
	if policy.Risk == "" {
		policy.Risk = "low"
		if tool.HTTPMethod != http.MethodGet {
			policy.Risk = "medium"
		}
		if tool.HTTPMethod == http.MethodDelete {
			policy.Risk = "critical"
		}
	}
	idempotencyKeyRequired := tool.HTTPMethod != http.MethodGet && policy.IdempotencyRequired
	description := tool.Description
	if policy.ConfirmationRequired {
		if managed {
			description += " The client must preview the exact invocation, obtain confirmation, and retry with the server-issued confirmation challenge; the retry is a client attestation, not independent server proof of human approval."
		} else {
			description += " The client must preview the exact invocation and attest that it obtained confirmation; this client attestation is not independent server proof of human approval."
		}
	}
	if idempotencyKeyRequired {
		description += " Supply one stable params._meta.idempotency_key for the invocation and reuse it across transport retries."
	}
	integrationIDs := make([]string, 0, 1)
	if managed {
		integrationIDs = append(integrationIDs, binding.IntegrationID)
	} else {
		for _, integration := range manifest.Integrations {
			for _, candidate := range integration.Tools {
				if candidate.ToolID == tool.ID && candidate.ToolRevision == tool.Revision {
					integrationIDs = append(integrationIDs, integration.ID)
					break
				}
			}
		}
	}
	actionType := ""
	pointID := ""
	pointRevision := int64(0)
	decisionTTL := 0
	if managed {
		actionType = binding.AuthorizationPoint.ActionType
		pointID = binding.AuthorizationPoint.ID
		pointRevision = binding.AuthorizationPointRevision
		decisionTTL = binding.AuthorizationPoint.DecisionTTLSeconds
	}
	method := strings.ToUpper(strings.TrimSpace(tool.HTTPMethod))
	actionType = strings.ToLower(strings.TrimSpace(actionType))
	// An authorization point may make an operation more restrictive, but it
	// must never erase the safety signal inherent in the HTTP method or tool
	// policy. This also remains fail-safe if stale data contains an invalid
	// read binding for a mutation.
	readOnly := method == http.MethodGet && (actionType == "" || actionType == "read")
	destructive := strings.EqualFold(strings.TrimSpace(policy.Risk), "critical") || method == http.MethodDelete || actionType == "destructive"
	metadata := map[string]any{
		"com.dokosoko/toolRevision":                    tool.Revision,
		"com.dokosoko/integrationIds":                  integrationIDs,
		"com.dokosoko/authorizationPointId":            pointID,
		"com.dokosoko/authorizationPointRevision":      pointRevision,
		"com.dokosoko/authorizationDecisionTtlSeconds": decisionTTL,
		"com.dokosoko/requiredGrants":                  policy.RequiredGrants,
		"com.dokosoko/confirmationRequired":            policy.ConfirmationRequired,
		"com.dokosoko/risk":                            policy.Risk,
		"com.dokosoko/idempotencyKeyRequired":          idempotencyKeyRequired,
		"com.dokosoko/idempotencyKeyMetaField":         "idempotency_key",
	}
	if managed && policy.ConfirmationRequired {
		metadata["com.dokosoko/confirmationChallengeMetaField"] = managedToolConfirmationMetaField
		metadata["com.dokosoko/confirmationAttestationMetaField"] = "confirmed"
	}
	return map[string]any{
		"name":        tool.Namespace + "." + tool.Name,
		"description": description,
		"inputSchema": tool.InputSchema,
		"annotations": map[string]any{
			"readOnlyHint":    readOnly,
			"destructiveHint": destructive,
			"idempotentHint":  method == http.MethodGet || policy.IdempotencyRequired,
		},
		"_meta": metadata,
	}
}

func reportProductContext(manifest model.ProductManifest) reporting.ProductContext {
	value := reporting.ProductContext{ProductID: manifest.ProductID, ProductName: manifest.ProductName, ManifestHash: manifest.ManifestHash, CatalogRevision: manifest.CatalogRevision, SelectionSource: manifest.SelectionSource, EnvironmentID: manifest.EnvironmentID, InstallationID: manifest.InstallationID}
	if manifest.EffectiveVersion != nil {
		value.ProductVersionID = manifest.EffectiveVersion.ID
		value.ProductVersion = manifest.EffectiveVersion.Version
	}
	return value
}

func (s *Server) reportIntegrationContext(ctx context.Context, deploymentID, integrationID string) (*reporting.IntegrationContext, error) {
	integrationID = strings.TrimSpace(integrationID)
	if integrationID == "" {
		return nil, nil
	}
	integration, err := s.service.Store().Integration(ctx, deploymentID, integrationID)
	if err != nil || integration.Lifecycle == "retired" {
		return nil, store.ErrNotFound
	}
	value := &reporting.IntegrationContext{IntegrationID: integration.ID, FamilyKey: integration.FamilyKey, VersionKey: integration.VersionKey, DisplayName: integration.DisplayName, Lifecycle: integration.Lifecycle, Revision: integration.Revision}
	revisions, err := s.service.Store().IntegrationRevisions(ctx, integration.ID)
	if err != nil {
		return nil, err
	}
	for _, revision := range revisions {
		if revision.State == "published" {
			value.Revision, value.ManifestHash, value.Snapshot = revision.Revision, revision.ManifestHash, revision.Snapshot
			break
		}
	}
	return value, nil
}

func reportToolResult(value reporting.SubmissionView) map[string]any {
	result := map[string]any{"submission_id": value.ID, "status": value.State}
	if value.ExternalID != "" {
		result["external_id"] = value.ExternalID
	}
	if value.ExternalURL != "" {
		result["external_url"] = value.ExternalURL
	}
	return result
}

func reportingRPCError(w http.ResponseWriter, id any, err error) {
	switch {
	case errors.Is(err, reporting.ErrSensitiveContent):
		writeRPCError(w, id, -32602, "Potential credential or secret detected; redact it and ask the user to approve the revised report")
	case errors.Is(err, reporting.ErrInvalidReport):
		writeRPCError(w, id, -32602, err.Error())
	case errors.Is(err, reporting.ErrDisabled), errors.Is(err, reporting.ErrNotConfigured):
		writeRPCError(w, id, -32601, "Reporting tool is not enabled for this Integration")
	default:
		writeRPCError(w, id, -32603, "The report could not be held safely")
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
	return providerruntime.Principal{Subject: vendorActorID(principal), ExternalCustomerID: principal.ExternalCustomerID, InstallationID: principal.InstallationID, Grants: principal.Grants, RequestID: requestID}
}

func accessPrincipal(principal identity.Principal, ctx context.Context) accessruntime.Principal {
	requestID, _ := ctx.Value(requestIDKey).(string)
	return accessruntime.Principal{Subject: vendorActorID(principal), ExternalCustomerID: principal.ExternalCustomerID, InstallationID: principal.InstallationID, Grants: principal.Grants, RequestID: requestID}
}

func toolPrincipal(principal identity.Principal, confirmed bool, requestID, idempotencyKey string) toolruntime.Principal {
	return toolruntime.Principal{
		Subject:              principal.Subject,
		Issuer:               principal.Issuer,
		CustomerAccountID:    principal.CustomerAccountID,
		ExternalCustomerID:   principal.ExternalCustomerID,
		InstallationID:       principal.InstallationID,
		Grants:               principal.Grants,
		AccessEvaluationID:   principal.AccessEvaluationID,
		AccessEvaluatedAt:    principal.AccessEvaluatedAt,
		DelegatedAPIOrigin:   principal.DelegatedAPIOrigin,
		DelegatedAccessToken: principal.UpstreamAccessToken,
		Confirmed:            confirmed,
		RequestID:            requestID,
		IdempotencyKey:       idempotencyKey,
	}
}

func accessRPCError(w http.ResponseWriter, id any, err error) {
	switch {
	case errors.Is(err, accessruntime.ErrDenied):
		writeRPCError(w, id, -32003, "Access operation was denied")
	case errors.Is(err, accessruntime.ErrInvalidRequest):
		writeRPCError(w, id, -32602, "Access operation request or provider response was invalid")
	case errors.Is(err, accessruntime.ErrUnsupported):
		writeRPCError(w, id, -32601, "Access operation is not supported by this connection")
	case errors.Is(err, accessruntime.ErrUnsafeDestination):
		writeRPCError(w, id, -32603, "Access provider destination failed safety validation")
	default:
		writeRPCError(w, id, -32603, "Access operation failed closed")
	}
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
	case errors.Is(err, platform.ErrSourceReviewRequired):
		writeError(w, http.StatusBadRequest, "source_review_required", err.Error(), nil)
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

func writeRPCErrorData(w http.ResponseWriter, id any, code int, message string, data map[string]any) {
	writeJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message, "data": data}})
}

func writeToolResult(w http.ResponseWriter, id, value any) {
	encoded, _ := json.Marshal(value)
	writeRPC(w, id, map[string]any{"resultType": "complete", "content": []map[string]any{{"type": "text", "text": string(encoded)}}, "structuredContent": value, "isError": false})
}
