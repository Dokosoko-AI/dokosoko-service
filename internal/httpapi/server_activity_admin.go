package httpapi

import "net/http"

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
