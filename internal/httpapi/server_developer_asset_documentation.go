package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Server) developerAssetDocuments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	limit, err := developerAssetQueryLimit(r, 100, 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	offset, err := developerAssetQueryOffset(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	page, err := s.service.DocumentationCandidates(r.Context(), platform.DocumentationExplorerInput{
		IngestionRunID:      strings.TrimSpace(r.URL.Query().Get("ingestion_run_id")),
		SourceID:            strings.TrimSpace(r.URL.Query().Get("source_id")),
		SourcePublicationID: strings.TrimSpace(r.URL.Query().Get("source_publication_id")),
		Query:               strings.TrimSpace(r.URL.Query().Get("query")),
		Limit:               limit,
		Offset:              offset,
	})
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) developerAssetDocument(w http.ResponseWriter, r *http.Request, documentID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	value, err := s.service.DocumentationCandidate(r.Context(), documentID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

type documentationCollectionMemberRequest struct {
	Kind               string          `json:"kind"`
	ID                 string          `json:"id"`
	IncludeDescendants bool            `json:"include_descendants"`
	Selector           json.RawMessage `json:"selector"`
}

type documentationCollectionRequest struct {
	Name                string                                 `json:"name"`
	Slug                string                                 `json:"slug"`
	Description         string                                 `json:"description"`
	Visibility          model.Visibility                       `json:"visibility"`
	Lifecycle           string                                 `json:"lifecycle"`
	Revision            int64                                  `json:"revision"`
	Members             []documentationCollectionMemberRequest `json:"members"`
	AcknowledgeReviewed bool                                   `json:"acknowledge_reviewed"`
}

func documentationCollectionInput(input documentationCollectionRequest) platform.DocumentationCollectionInput {
	members := make([]platform.DocumentationCollectionMemberInput, 0, len(input.Members))
	for _, member := range input.Members {
		members = append(members, platform.DocumentationCollectionMemberInput{
			Kind: member.Kind, ID: member.ID, IncludeDescendants: member.IncludeDescendants, Selector: member.Selector,
		})
	}
	return platform.DocumentationCollectionInput{
		Name: input.Name, Slug: input.Slug, Description: input.Description,
		Visibility: input.Visibility, Lifecycle: input.Lifecycle, Revision: input.Revision,
		Members: members, AcknowledgeReviewed: input.AcknowledgeReviewed,
	}
}

func (s *Server) documentationCollections(w http.ResponseWriter, r *http.Request) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().DocumentationCollections(r.Context(), deploymentID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input documentationCollectionRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveDocumentationCollection(r.Context(), "", documentationCollectionInput(input), actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) documentationCollection(w http.ResponseWriter, r *http.Request, collectionID string) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().DocumentationCollection(r.Context(), deploymentID, collectionID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		var input documentationCollectionRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.SaveDocumentationCollection(r.Context(), collectionID, documentationCollectionInput(input), actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPatch)
	}
}

func (s *Server) documentationCollectionRevisions(w http.ResponseWriter, r *http.Request, collectionID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().DocumentationCollectionRevisions(r.Context(), deploymentID, collectionID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) documentationCollectionRevision(w http.ResponseWriter, r *http.Request, collectionID, revisionID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().DocumentationCollectionRevision(r.Context(), deploymentID, revisionID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if value.Revision.DocumentationCollectionID != collectionID {
		s.storeError(w, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

type deploymentDocumentationPublicationRequest struct {
	CollectionRevisionIDs []string         `json:"collection_revision_ids"`
	Visibility            model.Visibility `json:"visibility"`
	ExpectedHeadRevision  int64            `json:"expected_head_revision"`
	AcknowledgeReviewed   bool             `json:"acknowledge_reviewed"`
}

func (s *Server) deploymentDocumentationPublications(w http.ResponseWriter, r *http.Request) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().DeploymentDocumentationPublications(r.Context(), deploymentID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input deploymentDocumentationPublicationRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.PublishDeploymentDocumentation(r.Context(), platform.DeploymentDocumentationPublicationInput{
			CollectionRevisionIDs: input.CollectionRevisionIDs, Visibility: input.Visibility,
			ExpectedHeadRevision: input.ExpectedHeadRevision, AcknowledgeReviewed: input.AcknowledgeReviewed,
		}, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) deploymentDocumentationPublication(w http.ResponseWriter, r *http.Request, publicationID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().DeploymentDocumentationPublication(r.Context(), deploymentID, publicationID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
