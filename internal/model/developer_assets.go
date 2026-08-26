package model

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

var (
	developerAssetHashPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	developerAssetDigestPattern = regexp.MustCompile(`^(sha256|sha384|sha512):[0-9a-f]+$`)
	forbiddenVersionRange       = regexp.MustCompile(`[*<>=~^]`)
	pypiCoordinateSeparator     = regexp.MustCompile(`[-_.]+`)
)

type DeveloperAssetKind string

const (
	DeveloperAssetDocumentation DeveloperAssetKind = "documentation"
	DeveloperAssetContract      DeveloperAssetKind = "contract"
	DeveloperAssetSDK           DeveloperAssetKind = "sdk"
)

func (kind DeveloperAssetKind) Valid() bool {
	return kind == DeveloperAssetDocumentation || kind == DeveloperAssetContract || kind == DeveloperAssetSDK
}

type DeveloperAssetIngestionState string

const (
	DeveloperAssetIngestionQueued      DeveloperAssetIngestionState = "queued"
	DeveloperAssetIngestionRunning     DeveloperAssetIngestionState = "running"
	DeveloperAssetIngestionReviewReady DeveloperAssetIngestionState = "review_ready"
	DeveloperAssetIngestionFailed      DeveloperAssetIngestionState = "failed"
	DeveloperAssetIngestionCancelled   DeveloperAssetIngestionState = "cancelled"
	DeveloperAssetIngestionPublished   DeveloperAssetIngestionState = "published"
)

func (state DeveloperAssetIngestionState) Valid() bool {
	switch state {
	case DeveloperAssetIngestionQueued, DeveloperAssetIngestionRunning,
		DeveloperAssetIngestionReviewReady, DeveloperAssetIngestionFailed,
		DeveloperAssetIngestionCancelled, DeveloperAssetIngestionPublished:
		return true
	default:
		return false
	}
}

type IngestionStageName string

const (
	IngestionStageAcquire      IngestionStageName = "acquire"
	IngestionStageValidate     IngestionStageName = "validate"
	IngestionStageParse        IngestionStageName = "parse"
	IngestionStageNormalize    IngestionStageName = "normalize"
	IngestionStageSegment      IngestionStageName = "segment"
	IngestionStageExtract      IngestionStageName = "extract"
	IngestionStageMap          IngestionStageName = "map"
	IngestionStageAIEnrich     IngestionStageName = "ai_enrich"
	IngestionStageQualityCheck IngestionStageName = "quality_check"
	IngestionStageBuildIndex   IngestionStageName = "build_index"
	IngestionStageReview       IngestionStageName = "review"
	IngestionStagePublish      IngestionStageName = "publish"
)

func (stage IngestionStageName) Valid() bool {
	switch stage {
	case IngestionStageAcquire, IngestionStageValidate, IngestionStageParse,
		IngestionStageNormalize, IngestionStageSegment, IngestionStageExtract,
		IngestionStageMap, IngestionStageAIEnrich, IngestionStageQualityCheck,
		IngestionStageBuildIndex, IngestionStageReview, IngestionStagePublish:
		return true
	default:
		return false
	}
}

type ProcessorVersions struct {
	Pipeline   string `json:"pipeline"`
	Parser     string `json:"parser"`
	Normalizer string `json:"normalizer"`
	Mapper     string `json:"mapper"`
}

func (versions ProcessorVersions) Valid() bool {
	return strings.TrimSpace(versions.Pipeline) != "" &&
		strings.TrimSpace(versions.Parser) != "" &&
		strings.TrimSpace(versions.Normalizer) != "" &&
		strings.TrimSpace(versions.Mapper) != ""
}

type DeveloperAssetIngestionRun struct {
	ID                     string                       `json:"id"`
	DeploymentID           string                       `json:"deployment_id"`
	OrganisationID         string                       `json:"organisation_id"`
	AssetKind              DeveloperAssetKind           `json:"asset_kind"`
	TargetID               string                       `json:"target_id,omitempty"`
	TargetKey              string                       `json:"target_key"`
	SourceID               string                       `json:"source_id,omitempty"`
	ResolvedSourceURI      string                       `json:"resolved_source_uri,omitempty"`
	ResolvedSourceRevision string                       `json:"resolved_source_revision,omitempty"`
	ResolvedSourceHash     string                       `json:"resolved_source_hash,omitempty"`
	State                  DeveloperAssetIngestionState `json:"state"`
	Attempt                int                          `json:"attempt"`
	Versions               ProcessorVersions            `json:"versions"`
	RawManifest            json.RawMessage              `json:"raw_manifest"`
	RawManifestHash        string                       `json:"raw_manifest_hash,omitempty"`
	Diagnostics            json.RawMessage              `json:"diagnostics"`
	DiscoveredCount        int                          `json:"discovered_count"`
	AcquiredCount          int                          `json:"acquired_count"`
	FailedCount            int                          `json:"failed_count"`
	SkippedCount           int                          `json:"skipped_count"`
	QuarantinedCount       int                          `json:"quarantined_count"`
	LeaseOwner             string                       `json:"lease_owner,omitempty"`
	LeaseExpiresAt         *time.Time                   `json:"lease_expires_at,omitempty"`
	HeartbeatAt            *time.Time                   `json:"heartbeat_at,omitempty"`
	ErrorCode              string                       `json:"error_code,omitempty"`
	ErrorMessage           string                       `json:"error_message,omitempty"`
	QueuedAt               time.Time                    `json:"queued_at"`
	StartedAt              *time.Time                   `json:"started_at,omitempty"`
	FinishedAt             *time.Time                   `json:"finished_at,omitempty"`
}

func (run DeveloperAssetIngestionRun) Valid() bool {
	if !run.AssetKind.Valid() || !run.State.Valid() || !run.Versions.Valid() ||
		strings.TrimSpace(run.TargetID) == "" || strings.TrimSpace(run.TargetKey) == "" || run.Attempt < 1 {
		return false
	}
	if run.AssetKind != DeveloperAssetSDK && strings.TrimSpace(run.SourceID) == "" {
		return false
	}
	if run.AssetKind == DeveloperAssetDocumentation && run.TargetID != run.SourceID {
		return false
	}
	if (strings.TrimSpace(run.LeaseOwner) == "") != (run.LeaseExpiresAt == nil) {
		return false
	}
	if run.ResolvedSourceHash != "" && !developerAssetHashPattern.MatchString(run.ResolvedSourceHash) {
		return false
	}
	return run.RawManifestHash == "" || developerAssetHashPattern.MatchString(run.RawManifestHash)
}

func (state DeveloperAssetIngestionState) CanTransitionTo(next DeveloperAssetIngestionState) bool {
	switch state {
	case DeveloperAssetIngestionQueued:
		return next == DeveloperAssetIngestionRunning || next == DeveloperAssetIngestionFailed || next == DeveloperAssetIngestionCancelled
	case DeveloperAssetIngestionRunning:
		return next == DeveloperAssetIngestionReviewReady || next == DeveloperAssetIngestionFailed || next == DeveloperAssetIngestionCancelled
	case DeveloperAssetIngestionReviewReady:
		return next == DeveloperAssetIngestionPublished || next == DeveloperAssetIngestionCancelled
	default:
		return false
	}
}

type DeveloperAssetIngestionStage struct {
	ID             string             `json:"id"`
	IngestionRunID string             `json:"ingestion_run_id"`
	Name           IngestionStageName `json:"stage_name"`
	Attempt        int                `json:"attempt"`
	State          string             `json:"state"`
	InputHash      string             `json:"input_hash,omitempty"`
	OutputHash     string             `json:"output_hash,omitempty"`
	Checkpoint     json.RawMessage    `json:"checkpoint"`
	Diagnostics    json.RawMessage    `json:"diagnostics"`
	ErrorCode      string             `json:"error_code,omitempty"`
	ErrorMessage   string             `json:"error_message,omitempty"`
	StartedAt      *time.Time         `json:"started_at,omitempty"`
	FinishedAt     *time.Time         `json:"finished_at,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}
