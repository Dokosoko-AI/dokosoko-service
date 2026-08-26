package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const (
	maxDeveloperAssetRequestBytes            = 1 << 20
	maxSDKContentIngestionRequestBytes int64 = 24 << 20
)

// decodeDeveloperAssetJSON is deliberately stricter than the historical
// control-plane decoder: one JSON value is required, unknown fields and
// trailing values are rejected, and the body is bounded.
func decodeDeveloperAssetJSON(reader io.Reader, value any) error {
	return decodeDeveloperAssetJSONLimit(reader, value, maxDeveloperAssetRequestBytes)
}

func decodeDeveloperAssetJSONLimit(reader io.Reader, value any, maximum int64) error {
	limited := &io.LimitedReader{R: reader, N: maximum + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("invalid JSON: request body must contain exactly one value")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if limited.N == 0 {
		return fmt.Errorf("invalid JSON: request body exceeds %d bytes", maximum)
	}
	return nil
}

func developerAssetMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
}

func (s *Server) developerAssetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict):
		s.storeError(w, err)
	case errors.Is(err, platform.ErrSourceReviewRequired):
		writeError(w, http.StatusUnprocessableEntity, "human_review_required", "Explicit human review is required before publication.", nil)
	case errors.Is(err, platform.ErrUnsafeForPublic):
		writeError(w, http.StatusUnprocessableEntity, "unsafe_for_public", err.Error(), nil)
	case errors.Is(err, platform.ErrConfirmationRequired):
		writeError(w, http.StatusConflict, "confirmation_required", err.Error(), nil)
	case errors.Is(err, platform.ErrSDKReleaseUnavailable):
		writeError(w, http.StatusConflict, "sdk_release_unavailable", err.Error(), nil)
	case errors.Is(err, platform.ErrInvalidVisibility):
		writeError(w, http.StatusBadRequest, "invalid_visibility", err.Error(), nil)
	default:
		writeError(w, http.StatusBadRequest, "invalid_developer_asset", err.Error(), nil)
	}
}

func developerAssetQueryLimit(r *http.Request, defaultValue, maximum int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > maximum {
		return 0, fmt.Errorf("limit must be between 1 and %d", maximum)
	}
	return value, nil
}

func developerAssetQueryOffset(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("offset"))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, errors.New("offset must be zero or greater")
	}
	return value, nil
}

func (s *Server) developerAssetDeploymentID(r *http.Request) (string, error) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		return "", err
	}
	return deployment.ID, nil
}

func (s *Server) developerAssetCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	value, err := s.service.DeveloperAssetCatalog(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) developerAssetIngestionRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	kind := model.DeveloperAssetKind(strings.TrimSpace(r.URL.Query().Get("asset_kind")))
	targetKey := strings.TrimSpace(r.URL.Query().Get("target_key"))
	if len(targetKey) > 500 {
		writeError(w, http.StatusBadRequest, "invalid_request", "target_key must be no more than 500 characters.", nil)
		return
	}
	values, err := s.service.DeveloperAssetIngestionRuns(r.Context(), kind, targetKey)
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) developerAssetIngestionRun(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	value, err := s.service.DeveloperAssetIngestion(r.Context(), runID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

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

type apiContractRequest struct {
	Name        string           `json:"name"`
	Slug        string           `json:"slug"`
	Description string           `json:"description"`
	Visibility  model.Visibility `json:"visibility"`
	Lifecycle   string           `json:"lifecycle"`
	Revision    int64            `json:"revision"`
}

func apiContractInput(input apiContractRequest) platform.APIContractInput {
	return platform.APIContractInput{Name: input.Name, Slug: input.Slug, Description: input.Description, Visibility: input.Visibility, Lifecycle: input.Lifecycle, Revision: input.Revision}
}

func (s *Server) apiContracts(w http.ResponseWriter, r *http.Request) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().APIContracts(r.Context(), deploymentID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input apiContractRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveAPIContract(r.Context(), "", apiContractInput(input), actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) apiContract(w http.ResponseWriter, r *http.Request, contractID string) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().APIContract(r.Context(), deploymentID, contractID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		var input apiContractRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.SaveAPIContract(r.Context(), contractID, apiContractInput(input), actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodDelete:
		var input struct {
			Revision int64 `json:"revision"`
		}
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		current, err := s.service.Store().APIContract(r.Context(), deploymentID, contractID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.SaveAPIContract(r.Context(), contractID, platform.APIContractInput{
			Name: current.Name, Slug: current.Slug, Description: current.Description,
			Visibility: current.Visibility, Lifecycle: "archived", Revision: input.Revision,
		}, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

type apiContractSourceRequest struct {
	SourceID   string `json:"source_id"`
	SourceRole string `json:"source_role"`
	Revision   int64  `json:"revision"`
}

func (s *Server) apiContractSources(w http.ResponseWriter, r *http.Request, contractID string) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if _, err := s.service.Store().APIContract(r.Context(), deploymentID, contractID); err != nil {
			s.storeError(w, err)
			return
		}
		values, err := s.service.Store().APIContractSources(r.Context(), deploymentID, contractID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input apiContractSourceRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveAPIContractSource(r.Context(), contractID, "", platform.APIContractSourceInput{SourceID: input.SourceID, SourceRole: input.SourceRole}, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) apiContractSource(w http.ResponseWriter, r *http.Request, contractID, associationID string) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	current, err := s.service.Store().APIContractSource(r.Context(), deploymentID, associationID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if current.APIContractID != contractID {
		s.storeError(w, store.ErrNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, current)
	case http.MethodPatch:
		var input apiContractSourceRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.SaveAPIContractSource(r.Context(), contractID, associationID, platform.APIContractSourceInput{SourceID: input.SourceID, SourceRole: input.SourceRole, Revision: input.Revision}, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodDelete:
		var input struct {
			Revision int64 `json:"revision"`
		}
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.DetachAPIContractSource(r.Context(), associationID, input.Revision, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

func (s *Server) apiContractCandidates(w http.ResponseWriter, r *http.Request, contractID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if _, err := s.service.Store().APIContract(r.Context(), deploymentID, contractID); err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().APIContractCandidates(r.Context(), deploymentID, contractID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) apiContractCandidate(w http.ResponseWriter, r *http.Request, contractID, candidateID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().APIContractCandidate(r.Context(), deploymentID, candidateID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if value.Candidate.APIContractID != contractID {
		s.storeError(w, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) publishAPIContractCandidate(w http.ResponseWriter, r *http.Request, contractID, candidateID string) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	var input struct {
		ContractRevision    int64 `json:"contract_revision"`
		AcknowledgeReviewed bool  `json:"acknowledge_reviewed"`
	}
	if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if input.ContractRevision < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "contract_revision is required.", nil)
		return
	}
	contract, revision, err := s.service.PublishAPIContractCandidate(r.Context(), contractID, candidateID, platform.APIContractCandidatePublicationInput{
		ContractRevision: input.ContractRevision, AcknowledgeReviewed: input.AcknowledgeReviewed,
	}, actor(r))
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"contract": contract, "revision": revision})
}

func (s *Server) apiContractRevisions(w http.ResponseWriter, r *http.Request, contractID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().APIContractRevisions(r.Context(), deploymentID, contractID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) apiContractRevision(w http.ResponseWriter, r *http.Request, contractID, revisionID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().APIContractRevision(r.Context(), deploymentID, revisionID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if value.APIContractID != contractID {
		s.storeError(w, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

type sdkPackageRequest struct {
	Ecosystem               string           `json:"ecosystem"`
	Coordinate              string           `json:"coordinate"`
	Name                    string           `json:"name"`
	Description             string           `json:"description"`
	RegistryURL             string           `json:"registry_url"`
	SourceURL               string           `json:"source_url"`
	Language                string           `json:"language"`
	Platform                string           `json:"platform"`
	Visibility              model.Visibility `json:"visibility"`
	Lifecycle               string           `json:"lifecycle"`
	ReplacementSDKPackageID string           `json:"replacement_sdk_package_id"`
	DeprecationMessage      string           `json:"deprecation_message"`
	Revision                int64            `json:"revision"`
}

func sdkPackageInput(input sdkPackageRequest) platform.SDKPackageInput {
	return platform.SDKPackageInput{
		Ecosystem: input.Ecosystem, Coordinate: input.Coordinate, Name: input.Name, Description: input.Description,
		RegistryURL: input.RegistryURL, SourceURL: input.SourceURL, Language: input.Language, Platform: input.Platform,
		Visibility: input.Visibility, Lifecycle: input.Lifecycle, ReplacementSDKPackageID: input.ReplacementSDKPackageID,
		DeprecationMessage: input.DeprecationMessage, Revision: input.Revision,
	}
}

func (s *Server) sdkPackages(w http.ResponseWriter, r *http.Request) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().SDKPackages(r.Context(), deploymentID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input sdkPackageRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveSDKPackage(r.Context(), "", sdkPackageInput(input), actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) sdkPackage(w http.ResponseWriter, r *http.Request, packageID string) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().SDKPackage(r.Context(), deploymentID, packageID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		var input sdkPackageRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.SaveSDKPackage(r.Context(), packageID, sdkPackageInput(input), actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPatch)
	}
}

type sdkReleaseRequest struct {
	ExactVersion           string           `json:"exact_version"`
	InstallCommand         string           `json:"install_command"`
	DocumentationURL       string           `json:"documentation_url"`
	SourceURL              string           `json:"source_url"`
	ResolvedSourceRevision string           `json:"resolved_source_revision"`
	UpstreamDigest         string           `json:"upstream_digest"`
	IdentityAssurance      string           `json:"identity_assurance"`
	Visibility             model.Visibility `json:"visibility"`
	Lifecycle              string           `json:"lifecycle"`
}

func (s *Server) sdkReleases(w http.ResponseWriter, r *http.Request, packageID string) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if _, err := s.service.Store().SDKPackage(r.Context(), deploymentID, packageID); err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().SDKReleases(r.Context(), deploymentID, packageID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input sdkReleaseRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateSDKRelease(r.Context(), packageID, platform.SDKReleaseInput{
			ExactVersion: input.ExactVersion, InstallCommand: input.InstallCommand,
			DocumentationURL: input.DocumentationURL, SourceURL: input.SourceURL,
			ResolvedSourceRevision: input.ResolvedSourceRevision, UpstreamDigest: input.UpstreamDigest,
			IdentityAssurance: input.IdentityAssurance, Visibility: input.Visibility, Lifecycle: input.Lifecycle,
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

func (s *Server) sdkRelease(w http.ResponseWriter, r *http.Request, packageID, releaseID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().SDKRelease(r.Context(), deploymentID, releaseID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if value.SDKPackageID != packageID {
		s.storeError(w, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

type sdkReleaseLifecycleEventRequest struct {
	Lifecycle         string    `json:"lifecycle"`
	Reason            string    `json:"reason"`
	ObservedSourceURI string    `json:"observed_source_uri"`
	ObservedAt        time.Time `json:"observed_at"`
}

func (s *Server) sdkReleaseLifecycleEvents(w http.ResponseWriter, r *http.Request, packageID, releaseID string) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	release, err := s.service.Store().SDKRelease(r.Context(), deploymentID, releaseID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if release.SDKPackageID != packageID {
		s.storeError(w, store.ErrNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.SDKReleaseLifecycle(r.Context(), release.ID)
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPost:
		var input sdkReleaseLifecycleEventRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.AppendSDKReleaseLifecycleEvent(r.Context(), release.ID, platform.SDKReleaseLifecycleEventInput{
			Lifecycle: input.Lifecycle, Reason: input.Reason, ObservedSourceURI: input.ObservedSourceURI,
			ObservedAt: input.ObservedAt,
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

func (s *Server) sdkContentCandidates(w http.ResponseWriter, r *http.Request, releaseID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if _, err := s.service.Store().SDKRelease(r.Context(), deploymentID, releaseID); err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().SDKContentCandidates(r.Context(), deploymentID, releaseID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) sdkContentIngestions(w http.ResponseWriter, r *http.Request, releaseID string) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	var input platform.SDKContentIngestionInput
	if err := decodeDeveloperAssetJSONLimit(r.Body, &input, maxSDKContentIngestionRequestBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.IngestSDKReleaseContent(r.Context(), releaseID, input, actor(r))
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	status := http.StatusCreated
	if value.AlreadyIngested {
		status = http.StatusOK
	}
	writeJSON(w, status, value)
}

func (s *Server) sdkContentCandidate(w http.ResponseWriter, r *http.Request, releaseID, candidateID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().SDKContentCandidate(r.Context(), deploymentID, candidateID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if value.Candidate.SDKReleaseID != releaseID {
		s.storeError(w, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) publishSDKContentCandidate(w http.ResponseWriter, r *http.Request, releaseID, candidateID string) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	var input platform.SDKContentCandidatePublicationInput
	if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.PublishSDKContentCandidate(r.Context(), releaseID, candidateID, input, actor(r))
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) sdkContentPublications(w http.ResponseWriter, r *http.Request, releaseID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().SDKContentPublications(r.Context(), deploymentID, releaseID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) sdkContentPublication(w http.ResponseWriter, r *http.Request, releaseID, publicationID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().SDKContentPublication(r.Context(), deploymentID, publicationID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if value.Publication.SDKReleaseID != releaseID {
		s.storeError(w, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

type apiDocumentationBindingRequest struct {
	DocumentationCollectionID string           `json:"documentation_collection_id"`
	FollowLatest              bool             `json:"follow_latest"`
	PinnedRevisionID          string           `json:"pinned_revision_id"`
	Selector                  json.RawMessage  `json:"selector"`
	Visibility                model.Visibility `json:"visibility"`
	Lifecycle                 string           `json:"lifecycle"`
	Revision                  int64            `json:"revision"`
}

type apiContractBindingRequest struct {
	APIContractID    string           `json:"api_contract_id"`
	FollowLatest     bool             `json:"follow_latest"`
	PinnedRevisionID string           `json:"pinned_revision_id"`
	Primary          bool             `json:"primary"`
	Visibility       model.Visibility `json:"visibility"`
	Lifecycle        string           `json:"lifecycle"`
	Revision         int64            `json:"revision"`
}

type apiSDKBindingRequest struct {
	SDKPackageID             string                          `json:"sdk_package_id"`
	SDKReleaseID             string                          `json:"sdk_release_id"`
	SDKContentPublicationID  string                          `json:"sdk_content_publication_id"`
	APIContractRevisionID    string                          `json:"api_contract_revision_id"`
	CompatibilityAssertionID string                          `json:"compatibility_assertion_id"`
	State                    string                          `json:"state"`
	Coverage                 model.SDKCompatibilityCoverage  `json:"coverage"`
	Assurance                model.SDKCompatibilityAssurance `json:"assurance"`
	ApplicableModules        []string                        `json:"applicable_modules"`
	ApplicableCapabilities   []string                        `json:"applicable_capabilities"`
	ApplicableOperationKeys  []string                        `json:"applicable_operation_keys"`
	Selector                 json.RawMessage                 `json:"selector"`
	Visibility               model.Visibility                `json:"visibility"`
	Revision                 int64                           `json:"revision"`
}

func (s *Server) apiResourceBindings(w http.ResponseWriter, r *http.Request, apiID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	value, err := s.service.APIResourceBindings(r.Context(), apiID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) apiDeveloperAssetPublications(w http.ResponseWriter, r *http.Request, apiID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if _, err = s.service.Store().Integration(r.Context(), deploymentID, apiID); err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().APIDeveloperAssetPublications(r.Context(), deploymentID, apiID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) apiDeveloperAssetPublication(w http.ResponseWriter, r *http.Request, apiID, publicationID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().APIDeveloperAssetPublication(r.Context(), deploymentID, publicationID)
	if err != nil || value.APIID != apiID {
		if err == nil {
			err = store.ErrNotFound
		}
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) apiDocumentationBindings(w http.ResponseWriter, r *http.Request, apiID string) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	s.saveAPIDocumentationBinding(w, r, apiID, "", http.StatusCreated)
}

func (s *Server) apiDocumentationBinding(w http.ResponseWriter, r *http.Request, apiID, bindingID string) {
	switch r.Method {
	case http.MethodGet:
		deploymentID, err := s.developerAssetDeploymentID(r)
		if err != nil {
			s.storeError(w, err)
			return
		}
		value, err := s.service.Store().APIDocumentationBinding(r.Context(), deploymentID, apiID, bindingID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		s.saveAPIDocumentationBinding(w, r, apiID, bindingID, http.StatusOK)
	case http.MethodDelete:
		var input struct {
			Revision int64 `json:"revision"`
		}
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.DetachAPIDocumentationBinding(r.Context(), apiID, bindingID, input.Revision, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

func (s *Server) saveAPIDocumentationBinding(w http.ResponseWriter, r *http.Request, apiID, bindingID string, status int) {
	var input apiDocumentationBindingRequest
	if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if bindingID != "" && input.Revision < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
		return
	}
	value, err := s.service.SaveAPIDocumentationBinding(r.Context(), apiID, bindingID, platform.APIDocumentationBindingInput{
		DocumentationCollectionID: input.DocumentationCollectionID, FollowLatest: input.FollowLatest,
		PinnedRevisionID: input.PinnedRevisionID, Selector: input.Selector, Visibility: input.Visibility,
		Lifecycle: input.Lifecycle, Revision: input.Revision,
	}, actor(r))
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, status, value)
}

func (s *Server) apiContractBindings(w http.ResponseWriter, r *http.Request, apiID string) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	s.saveAPIContractBinding(w, r, apiID, "", http.StatusCreated)
}

func (s *Server) apiContractBinding(w http.ResponseWriter, r *http.Request, apiID, bindingID string) {
	switch r.Method {
	case http.MethodGet:
		deploymentID, err := s.developerAssetDeploymentID(r)
		if err != nil {
			s.storeError(w, err)
			return
		}
		value, err := s.service.Store().APIContractBinding(r.Context(), deploymentID, apiID, bindingID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		s.saveAPIContractBinding(w, r, apiID, bindingID, http.StatusOK)
	case http.MethodDelete:
		var input struct {
			Revision int64 `json:"revision"`
		}
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.DetachAPIContractBinding(r.Context(), apiID, bindingID, input.Revision, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

func (s *Server) saveAPIContractBinding(w http.ResponseWriter, r *http.Request, apiID, bindingID string, status int) {
	var input apiContractBindingRequest
	if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if bindingID != "" && input.Revision < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
		return
	}
	value, err := s.service.SaveAPIContractBinding(r.Context(), apiID, bindingID, platform.APIContractBindingInput{
		APIContractID: input.APIContractID, FollowLatest: input.FollowLatest, PinnedRevisionID: input.PinnedRevisionID,
		Primary: input.Primary, Visibility: input.Visibility, Lifecycle: input.Lifecycle, Revision: input.Revision,
	}, actor(r))
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, status, value)
}

func (s *Server) apiSDKBindings(w http.ResponseWriter, r *http.Request, apiID string) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	s.saveAPISDKBinding(w, r, apiID, "", http.StatusCreated)
}

func (s *Server) apiSDKBinding(w http.ResponseWriter, r *http.Request, apiID, bindingID string) {
	switch r.Method {
	case http.MethodGet:
		deploymentID, err := s.developerAssetDeploymentID(r)
		if err != nil {
			s.storeError(w, err)
			return
		}
		value, err := s.service.Store().APISDKBinding(r.Context(), deploymentID, apiID, bindingID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		s.saveAPISDKBinding(w, r, apiID, bindingID, http.StatusOK)
	case http.MethodDelete:
		var input struct {
			Revision int64 `json:"revision"`
		}
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.DetachAPISDKBinding(r.Context(), apiID, bindingID, input.Revision, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

func (s *Server) saveAPISDKBinding(w http.ResponseWriter, r *http.Request, apiID, bindingID string, status int) {
	var input apiSDKBindingRequest
	if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if bindingID != "" && input.Revision < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
		return
	}
	value, err := s.service.SaveAPISDKBinding(r.Context(), apiID, bindingID, platform.APISDKBindingInput{
		SDKPackageID: input.SDKPackageID, SDKReleaseID: input.SDKReleaseID,
		SDKContentPublicationID: input.SDKContentPublicationID, APIContractRevisionID: input.APIContractRevisionID,
		CompatibilityAssertionID: input.CompatibilityAssertionID, State: input.State, Coverage: input.Coverage,
		Assurance: input.Assurance, ApplicableModules: input.ApplicableModules,
		ApplicableCapabilities: input.ApplicableCapabilities, ApplicableOperationKeys: input.ApplicableOperationKeys,
		Selector: input.Selector, Visibility: input.Visibility, Revision: input.Revision,
	}, actor(r))
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, status, value)
}

func (s *Server) developerAssetQueryLab(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	var input platform.DeveloperAssetQueryLabInput
	if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.RunDeveloperAssetQueryLab(r.Context(), input)
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) developerAssetQueryTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	limit, err := developerAssetQueryLimit(r, 100, 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	var since time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		since, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "since must be an RFC 3339 timestamp.", nil)
			return
		}
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().RetrievalQueryTraces(r.Context(), deploymentID, since, limit)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) developerAssetQueryTrace(w http.ResponseWriter, r *http.Request, traceID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().RetrievalQueryTrace(r.Context(), deploymentID, traceID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
