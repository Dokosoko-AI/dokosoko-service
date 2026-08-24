package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type widgetAppearanceInput struct {
	Theme            string `json:"theme"`
	AccentColour     string `json:"accent_colour"`
	LauncherPosition string `json:"launcher_position"`
	Greeting         string `json:"greeting"`
}

type widgetAdminInput struct {
	Name           string                `json:"name"`
	AllowedOrigins []string              `json:"allowed_origins"`
	IntegrationIDs []string              `json:"integration_ids"`
	Appearance     widgetAppearanceInput `json:"appearance"`
	Revision       int64                 `json:"revision"`
}

func widgetPlatformInput(input widgetAdminInput) platform.WidgetInput {
	return platform.WidgetInput{
		Name: input.Name, AllowedOrigins: input.AllowedOrigins, IntegrationIDs: input.IntegrationIDs, Revision: input.Revision,
		Appearance: platform.WidgetAppearance{Theme: input.Appearance.Theme, AccentColour: input.Appearance.AccentColour, LauncherPosition: input.Appearance.LauncherPosition, Greeting: input.Appearance.Greeting},
	}
}

func (s *Server) adminWidgets(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Widgets(r.Context(), deployment.ID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input widgetAdminInput
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		created, err := s.service.CreateWidget(r.Context(), widgetPlatformInput(input), actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, created)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) adminWidget(w http.ResponseWriter, r *http.Request, widgetID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().Widget(r.Context(), deployment.ID, widgetID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		var input widgetAdminInput
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.UpdateWidget(r.Context(), widgetID, widgetPlatformInput(input), actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) setAdminWidgetState(w http.ResponseWriter, r *http.Request, widgetID, state string) {
	var input struct {
		Revision int64 `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.SetWidgetState(r.Context(), widgetID, state, input.Revision, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) adminWidgetSecrets(w http.ResponseWriter, r *http.Request, widgetID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	if _, err := s.service.Store().Widget(r.Context(), deployment.ID, widgetID); err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().WidgetSecrets(r.Context(), widgetID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		raw, value, err := s.service.RotateWidgetSecret(r.Context(), widgetID, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, map[string]any{"secret": raw, "credential": value})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) revokeAdminWidgetSecret(w http.ResponseWriter, r *http.Request, widgetID, secretID string) {
	value, err := s.service.Store().RevokeWidgetSecret(r.Context(), widgetID, secretID, time.Now().UTC())
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) adminWidgetSessions(w http.ResponseWriter, r *http.Request, widgetID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	if _, err := s.service.Store().Widget(r.Context(), deployment.ID, widgetID); err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().WidgetSessions(r.Context(), widgetID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) revokeAdminWidgetSession(w http.ResponseWriter, r *http.Request, widgetID, sessionID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	widget, err := s.service.Store().Widget(r.Context(), deployment.ID, widgetID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().RevokeWidgetSession(r.Context(), widgetID, sessionID, time.Now().UTC())
	if err != nil {
		s.storeError(w, err)
		return
	}
	_ = s.service.Store().AppendAudit(r.Context(), model.AuditEvent{ID: "audit_" + sessionID, OrganisationID: widget.OrganisationID, ProductID: widget.DeploymentID, ActorID: actor(r).ID, Action: "widget.session.revoked", TargetType: "widget_session", TargetID: sessionID, Current: map[string]any{"widget_id": widgetID}, RequestID: actor(r).RequestID, CreatedAt: time.Now().UTC()})
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) widgetConfiguration(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().Widget(r.Context(), deployment.ID, r.PathValue("widgetID"))
	if err != nil || value.State != "active" {
		writeError(w, http.StatusNotFound, "widget_not_found", "Widget not found.", nil)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, map[string]any{"widgetId": value.ID, "name": value.Name, "appearance": value.Appearance, "protocolVersion": "1"})
}

func (s *Server) createWidgetSession(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input struct {
		WidgetID       string `json:"widgetId"`
		UserID         string `json:"userId"`
		OrganisationID string `json:"organizationId"`
		Origin         string `json:"origin"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if !s.allowFixedWindow("widget-session|"+input.WidgetID+"|"+remoteHost(r.RemoteAddr), 120, time.Now().UTC()) {
		writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Widget session request limit exceeded.", nil)
		return
	}
	value, err := s.service.CreateWidgetBootstrap(r.Context(), input.WidgetID, bearerToken(r), input.UserID, input.OrganisationID, input.Origin, r.Header.Get("Idempotency-Key"))
	if err != nil {
		s.widgetRuntimeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) exchangeWidgetSession(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var input struct {
		BootstrapToken string `json:"bootstrapToken"`
		Origin         string `json:"origin"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.ExchangeWidgetBootstrap(r.Context(), input.BootstrapToken, input.Origin)
	if err != nil {
		s.widgetRuntimeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusCreated, map[string]any{"sessionToken": value.SessionToken, "expiresAt": value.ExpiresAt, "sessionId": value.Session.ID})
}

func (s *Server) currentWidgetSession(w http.ResponseWriter, r *http.Request) {
	principal, err := s.service.AuthenticateWidgetSession(r.Context(), bearerToken(r))
	if err != nil {
		s.widgetRuntimeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	integrationBindings := make([]map[string]any, 0, len(principal.Widget.IntegrationBindings))
	for _, binding := range principal.Widget.IntegrationBindings {
		integrationBindings = append(integrationBindings, map[string]any{"integrationId": binding.IntegrationID, "revision": binding.IntegrationRevision, "manifestHash": binding.ManifestHash})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"widgetId":            principal.Widget.ID,
		"sessionId":           principal.Session.ID,
		"userId":              principal.Session.UserID,
		"organizationId":      principal.Session.CustomerOrganisationID,
		"origin":              principal.Session.Origin,
		"expiresAt":           principal.Session.ExpiresAt,
		"integrationIds":      principal.Widget.IntegrationIDs,
		"integrationBindings": integrationBindings,
	})
}

func (s *Server) widgetChat(w http.ResponseWriter, r *http.Request) {
	principal, err := s.service.AuthenticateWidgetSession(r.Context(), bearerToken(r))
	if err != nil {
		s.widgetRuntimeError(w, err)
		return
	}
	if !s.allowFixedWindow("widget-chat|"+principal.Session.ID, 60, time.Now().UTC()) {
		writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Widget message limit exceeded.", nil)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input struct {
		Message string `json:"message"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" || len(input.Message) > 4000 {
		writeError(w, http.StatusBadRequest, "invalid_message", "A message between 1 and 4000 characters is required.", nil)
		return
	}

	reply, err := s.service.AnswerWidgetMessage(r.Context(), principal, input.Message)
	if err != nil {
		s.widgetRuntimeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	for index, word := range strings.Fields(reply) {
		if index > 0 {
			word = " " + word
		}
		encoded, _ := json.Marshal(map[string]string{"text": word})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		default:
		}
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	digest := sha256.Sum256([]byte(principal.Widget.ID + "\x00" + principal.Session.UserID))
	_ = s.service.Store().AppendAnalytics(r.Context(), model.AnalyticsEvent{OrganisationID: principal.Widget.OrganisationID, ProductID: principal.Widget.DeploymentID, EventName: "widget.message", ActorKind: "widget_user", ActorPseudonym: hex.EncodeToString(digest[:16]), Dimensions: map[string]any{"channel": "widget", "widget_id": principal.Widget.ID, "integration_count": len(principal.Widget.IntegrationBindings)}, CreatedAt: time.Now().UTC()})
}

func (s *Server) widgetRuntimeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, platform.ErrWidgetAuthentication), errors.Is(err, store.ErrNotFound):
		w.Header().Set("WWW-Authenticate", `Bearer realm="dokosoko-widget"`)
		writeError(w, http.StatusUnauthorized, "widget_authentication_failed", "The widget credential is invalid or expired.", nil)
	case errors.Is(err, platform.ErrWidgetOriginDenied):
		writeError(w, http.StatusForbidden, "widget_origin_denied", "This domain is not allowed for the widget.", nil)
	case errors.Is(err, platform.ErrWidgetDisabled):
		writeError(w, http.StatusConflict, "widget_disabled", "The widget is disabled.", nil)
	case errors.Is(err, platform.ErrWidgetManifestUnavailable):
		writeError(w, http.StatusConflict, "widget_manifest_unavailable", "The widget's pinned Integration manifest is unavailable. Review and reactivate the widget.", nil)
	case errors.Is(err, platform.ErrWidgetAssistantUnavailable):
		writeError(w, http.StatusConflict, "widget_assistant_unavailable", "The assistant is temporarily unavailable.", nil)
	default:
		message := strings.TrimSpace(err.Error())
		if message == "" {
			message = "The widget request could not be completed."
		}
		writeError(w, http.StatusBadRequest, "invalid_widget_request", message, nil)
	}
}
