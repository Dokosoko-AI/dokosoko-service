package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Server) syncNativePlugins(ctx context.Context, productID string) error {
	if s.nativePlugins == nil {
		return nil
	}
	deployment, err := s.service.Store().Deployment(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	if productID != "" && deployment.ID != productID {
		return store.ErrNotFound
	}
	return s.nativePlugins.SyncCatalog(ctx, s.service.Store(), deployment)
}

func (s *Server) nativePluginStatuses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	if s.nativePlugins == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	_ = s.syncNativePlugins(r.Context(), "")
	writeJSON(w, http.StatusOK, map[string]any{"items": s.nativePlugins.Statuses()})
}

func (s *Server) nativePluginState(w http.ResponseWriter, r *http.Request, pluginID string) {
	if s.nativePlugins == nil {
		writeError(w, http.StatusServiceUnavailable, "native_plugins_unavailable", "Native plugins are unavailable.", nil)
		return
	}
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	pluginID = strings.TrimSpace(pluginID)
	prior := "disabled"
	for _, status := range s.nativePlugins.Statuses() {
		if status.ID == pluginID {
			prior = status.State
			break
		}
	}
	status, err := s.nativePlugins.SetEnabled(r.Context(), pluginID, input.Enabled)
	if err != nil {
		writeError(w, http.StatusConflict, "native_plugin_state_rejected", "Native plugin state change was rejected.", nil)
		return
	}
	deployment, deploymentErr := s.service.Store().Deployment(r.Context())
	if deploymentErr == nil {
		_ = s.syncNativePlugins(r.Context(), deployment.ID)
		admin := actor(r)
		if err := s.service.Store().AppendAudit(r.Context(), model.AuditEvent{ID: nativePluginAuditID(pluginID, time.Now().UTC()), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: admin.ID, Action: "native_plugin.state_changed", TargetType: "native_plugin", TargetID: pluginID, Prior: map[string]any{"state": prior}, Current: map[string]any{"state": status.State, "enabled": input.Enabled}, RequestID: admin.RequestID, Outcome: "success", CreatedAt: time.Now().UTC()}); err != nil {
			writeError(w, http.StatusInternalServerError, "audit_failed", "Native plugin state changed, but its audit event could not be recorded.", nil)
			return
		}
	}
	writeJSON(w, http.StatusOK, status)
}

func nativePluginAuditID(pluginID string, now time.Time) string {
	return "audit_native_plugin_" + strings.ReplaceAll(pluginID, "-", "_") + "_" + now.Format("20060102150405.000000000")
}
