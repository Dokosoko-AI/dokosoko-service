package httpapi

import (
	"net/http"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Server) sdkPackageImports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	var input platform.SDKPackageImportInput
	if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.ImportSDKPackage(r.Context(), input, actor(r))
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	status := http.StatusCreated
	if value.AlreadyImported {
		status = http.StatusOK
	}
	writeJSON(w, status, value)
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
