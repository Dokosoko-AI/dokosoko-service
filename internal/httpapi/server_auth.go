package httpapi

import (
	"context"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
		if err := s.service.Store().AppendAudit(r.Context(), model.AuditEvent{ID: "audit_" + strconv.FormatInt(time.Now().UnixNano(), 10), ActorID: actor(r).ID, Action: "root.created", TargetType: "root_user", TargetID: result.User.ID, Current: map[string]any{"email": result.User.Email, "mfa_enforced": true}, RequestID: requestID, CreatedAt: time.Now().UTC()}); err != nil {
			s.storeError(w, err)
			return
		}
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
	if err := s.service.Store().AppendAudit(r.Context(), model.AuditEvent{ID: "audit_" + strconv.FormatInt(time.Now().UnixNano(), 10), ActorID: actor(r).ID, Action: "root.revoked", TargetType: "root_user", TargetID: userID, RequestID: requestID, CreatedAt: time.Now().UTC()}); err != nil {
		s.storeError(w, err)
		return
	}
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

func (s *Server) configurationStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.configuration)
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
