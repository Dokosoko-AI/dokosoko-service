package httpapi

import (
	"net/http"
)

func markLegacySDKEndpointDeprecated(w http.ResponseWriter) {
	w.Header().Set("Deprecation", "true")
	w.Header().Add("Link", `</api/v1/developer-assets>; rel="successor-version"`)
}

func legacySDKMutationGone(w http.ResponseWriter) {
	writeError(w, http.StatusGone, "legacy_sdk_mutation_removed", "API-scoped SDK mutations have been removed. Manage the deployment-owned SDK Catalog, then attach or detach its exact releases through API Resources.", nil)
}

func (s *Server) integrationSDKs(w http.ResponseWriter, r *http.Request, integrationID string) {
	markLegacySDKEndpointDeprecated(w)
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().SDKReferences(r.Context(), integrationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		legacySDKMutationGone(w)
	default:
		w.Header().Set("Allow", "GET")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) integrationSDK(w http.ResponseWriter, r *http.Request, integrationID, referenceID string) {
	markLegacySDKEndpointDeprecated(w)
	switch r.Method {
	case http.MethodPut, http.MethodDelete:
		legacySDKMutationGone(w)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}
