package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	accessruntime "github.com/dokosoko/dokosoko-service/internal/access"
	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/mcpbridge"
	"github.com/dokosoko/dokosoko-service/internal/nativeplugins"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	providerruntime "github.com/dokosoko/dokosoko-service/internal/providers"
	"github.com/dokosoko/dokosoko-service/internal/ratelimit"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	demoAdminToken                    = "doko_admin_demo"
	demoPrivateToken                  = "doko_private_demo"
	managedToolConfirmationTTL        = 2 * time.Minute
	managedToolConfirmationMetaField  = "confirmation_challenge"
	managedToolConfirmationDomain     = "dokosoko-managed-tool-confirmation-v1"
	managedToolConfirmationNonceBytes = 32
	maxHTTPRateWindows                = 4096
)

type Server struct {
	service         *platform.Service
	auth            *auth.Manager
	toolRuntime     *toolruntime.Runtime
	identityBroker  *identity.Broker
	accessRuntime   *accessruntime.Runtime
	providerRuntime *providerruntime.Runtime
	mcpBridge       *mcpbridge.Manager
	nativePlugins   *nativeplugins.Manager
	reporting       *reporting.Service
	baseURL         string
	uploadDirectory string
	uploadMaxBytes  int64
	allowDemoTokens bool
	widgetsEnabled  bool
	secureCookies   bool
	rateOnce        sync.Once
	rateLimiter     *ratelimit.FixedWindow
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
	NativePlugins   *nativeplugins.Manager
	Reporting       *reporting.Service
	UploadDirectory string
	UploadMaxBytes  int64
	AllowDemoTokens bool
	WidgetsEnabled  bool
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
	server := &Server{service: service, auth: options.Auth, toolRuntime: options.ToolRuntime, identityBroker: options.IdentityBroker, accessRuntime: options.AccessRuntime, providerRuntime: options.ProviderRuntime, mcpBridge: options.MCPBridge, nativePlugins: options.NativePlugins, reporting: options.Reporting, baseURL: baseURL, uploadDirectory: strings.TrimSpace(options.UploadDirectory), uploadMaxBytes: uploadMaxBytes, allowDemoTokens: options.AllowDemoTokens, widgetsEnabled: options.WidgetsEnabled, secureCookies: strings.HasPrefix(baseURL, "https://"), rateLimiter: ratelimit.NewFixedWindow(time.Minute, maxHTTPRateWindows)}
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
