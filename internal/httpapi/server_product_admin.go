package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Server) productSettings(w http.ResponseWriter, r *http.Request, productID string) {
	var input struct {
		Description string `json:"description"`
		Revision    int64  `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.UpdateProductSettings(r.Context(), productID, input.Description, input.Revision, actor(r))
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
