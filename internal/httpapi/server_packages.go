package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type packageArtifactRequest struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	Ecosystem         string `json:"ecosystem"`
	Coordinate        string `json:"coordinate"`
	PURL              string `json:"purl"`
	RegistryURL       string `json:"registry_url"`
	SourceURL         string `json:"source_url"`
	Language          string `json:"language"`
	Platform          string `json:"platform"`
	Visibility        string `json:"visibility"`
	AcknowledgePublic bool   `json:"acknowledge_public"`
	Revision          *int64 `json:"revision,omitempty"`
}

func packageArtifactInput(input packageArtifactRequest) platform.PackageArtifactInput {
	revision := int64(0)
	if input.Revision != nil {
		revision = *input.Revision
	}
	return platform.PackageArtifactInput{Name: input.Name, Description: input.Description, Ecosystem: input.Ecosystem, Coordinate: input.Coordinate, PURL: input.PURL, RegistryURL: input.RegistryURL, SourceURL: input.SourceURL, Language: input.Language, Platform: input.Platform, Visibility: model.Visibility(input.Visibility), AcknowledgePublic: input.AcknowledgePublic, Revision: revision}
}

func (s *Server) packageArtifacts(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().PackageArtifacts(r.Context(), deployment.ID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values, "delivery": "external_registry"})
	case http.MethodPost:
		var input packageArtifactRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is not allowed when creating a package artifact.", nil)
			return
		}
		value, err := s.service.CreatePackageArtifact(r.Context(), packageArtifactInput(input), actor(r))
		if err != nil {
			s.packageError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) packageArtifact(w http.ResponseWriter, r *http.Request, artifactID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().PackageArtifact(r.Context(), deployment.ID, artifactID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPut:
		var input packageArtifactRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required when replacing a package artifact.", nil)
			return
		}
		value, err := s.service.UpdatePackageArtifact(r.Context(), artifactID, packageArtifactInput(input), actor(r))
		if err != nil {
			s.packageError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) packageReleases(w http.ResponseWriter, r *http.Request, artifactID string) {
	values, err := s.service.Store().PackageReleases(r.Context(), artifactID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) publishPackageArtifact(w http.ResponseWriter, r *http.Request, artifactID string) {
	var input struct {
		Version           string `json:"version"`
		PURL              string `json:"purl"`
		InstallCommand    string `json:"install_command"`
		Digest            string `json:"digest"`
		ProvenanceURL     string `json:"provenance_url"`
		SBOMURL           string `json:"sbom_url"`
		ArtifactRevision  *int64 `json:"artifact_revision"`
		AcknowledgePublic bool   `json:"acknowledge_public"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if input.ArtifactRevision == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "artifact_revision is required.", nil)
		return
	}
	artifact, release, err := s.service.PublishPackageArtifact(r.Context(), artifactID, platform.PackageReleaseInput{Version: input.Version, PURL: input.PURL, InstallCommand: input.InstallCommand, Digest: input.Digest, ProvenanceURL: input.ProvenanceURL, SBOMURL: input.SBOMURL, ArtifactRevision: *input.ArtifactRevision, AcknowledgePublic: input.AcknowledgePublic}, actor(r))
	if err != nil {
		s.packageError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"artifact": artifact, "release": release})
}

func (s *Server) deprecatePackageArtifact(w http.ResponseWriter, r *http.Request, artifactID string) {
	var input struct {
		ReplacementPackageArtifactID string     `json:"replacement_package_artifact_id"`
		Message                      string     `json:"message"`
		SunsetAt                     *time.Time `json:"sunset_at"`
		Revision                     *int64     `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if input.Revision == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
		return
	}
	value, err := s.service.DeprecatePackageArtifact(r.Context(), artifactID, platform.PackageDeprecationInput{ReplacementPackageArtifactID: input.ReplacementPackageArtifactID, Message: input.Message, SunsetAt: input.SunsetAt, Revision: *input.Revision}, actor(r))
	if err != nil {
		s.packageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) retirePackageArtifact(w http.ResponseWriter, r *http.Request, artifactID string) {
	var input struct {
		ReplacementPackageArtifactID string `json:"replacement_package_artifact_id"`
		Message                      string `json:"message"`
		Revision                     *int64 `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if input.Revision == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
		return
	}
	value, err := s.service.RetirePackageArtifact(r.Context(), artifactID, platform.PackageRetirementInput{ReplacementPackageArtifactID: input.ReplacementPackageArtifactID, Message: input.Message, Revision: *input.Revision}, actor(r))
	if err != nil {
		s.packageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) integrationPackages(w http.ResponseWriter, r *http.Request, integrationID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().IntegrationPackageBindings(r.Context(), integrationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			PackageReleaseID string `json:"package_release_id"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.BindPackageRelease(r.Context(), integrationID, input.PackageReleaseID, actor(r))
		if err != nil {
			s.packageError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) integrationPackage(w http.ResponseWriter, r *http.Request, integrationID, artifactID string) {
	switch r.Method {
	case http.MethodPut:
		var input struct {
			PackageReleaseID string `json:"package_release_id"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		deployment, err := s.service.Store().Deployment(r.Context())
		if err != nil {
			s.storeError(w, err)
			return
		}
		release, err := s.service.Store().PackageRelease(r.Context(), deployment.ID, input.PackageReleaseID)
		if err != nil {
			s.packageError(w, err)
			return
		}
		if release.PackageArtifactID != artifactID {
			writeError(w, http.StatusBadRequest, "invalid_package_binding", "The exact release does not belong to the package artifact in the route.", nil)
			return
		}
		value, err := s.service.BindPackageRelease(r.Context(), integrationID, input.PackageReleaseID, actor(r))
		if err != nil {
			s.packageError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodDelete:
		if err := s.service.UnbindPackageArtifact(r.Context(), integrationID, artifactID, actor(r)); err != nil {
			s.packageError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "PUT, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) packageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, platform.ErrConfirmationRequired):
		s.platformError(w, err, "Confirm that this package metadata may become discoverable through Public MCP after an exact public binding and published public Integration.")
	case errors.Is(err, store.ErrNotFound):
		s.storeError(w, err)
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "package_catalog_conflict", "The package coordinate, exact version, content hash, or revision conflicts with current state.", nil)
	default:
		writeError(w, http.StatusBadRequest, "invalid_package_catalog", err.Error(), nil)
	}
}
