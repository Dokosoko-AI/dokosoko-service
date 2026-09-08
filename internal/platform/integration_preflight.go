package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

const (
	preflightPass     = "pass"
	preflightFail     = "fail"
	preflightOptional = "optional"
)

type IntegrationPreflightCheck struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Message  string `json:"message"`
	Status   string `json:"status"`
	Tab      string `json:"tab"`
	Required bool   `json:"required"`
}

// IntegrationPreflightResult is generated entirely on the server from the
// same candidate snapshot used by publication. CandidateRevision is the
// mutable Integration revision that will be represented by that snapshot; a
// draft-to-active publication consumes the next optimistic revision.
type IntegrationPreflightResult struct {
	IntegrationID           string                      `json:"integration_id"`
	CandidateRevision       int64                       `json:"candidate_revision"`
	CandidateManifestHash   string                      `json:"candidate_manifest_hash"`
	LatestPublishedID       string                      `json:"latest_published_id,omitempty"`
	LatestPublishedRevision int64                       `json:"latest_published_revision,omitempty"`
	LatestPublishedHash     string                      `json:"latest_published_hash,omitempty"`
	MatchesLatestPublished  bool                        `json:"matches_latest_published"`
	Ready                   bool                        `json:"ready"`
	Checks                  []IntegrationPreflightCheck `json:"checks"`
	GeneratedAt             time.Time                   `json:"generated_at"`
}

func preflightCheck(code, label, passMessage, failMessage, tab string, passed, required bool) IntegrationPreflightCheck {
	status, message := preflightPass, passMessage
	if !passed {
		status, message = preflightFail, failMessage
		if !required {
			status = preflightOptional
		}
	}
	return IntegrationPreflightCheck{Code: code, Label: label, Message: message, Status: status, Tab: tab, Required: required}
}

func latestPublishedIntegrationRevision(values []model.IntegrationRevision) *model.IntegrationRevision {
	var latest *model.IntegrationRevision
	for index := range values {
		if values[index].State != "published" || (latest != nil && values[index].Revision <= latest.Revision) {
			continue
		}
		value := values[index]
		latest = &value
	}
	return latest
}

func (s *Service) IntegrationPreflight(ctx context.Context, integrationID string) (IntegrationPreflightResult, error) {
	status, err := s.IntegrationPublishStatus(ctx, strings.TrimSpace(integrationID))
	if err != nil {
		return IntegrationPreflightResult{}, err
	}
	// Publication readiness has one authority: validation of the exact candidate.
	// Private identity is a delivery requirement, not a requirement to prepare
	// reviewed knowledge. Every selected tool and runtime connection is still
	// validated by buildIntegrationSnapshot.
	var snapshot integrationSnapshot
	if err = json.Unmarshal(status.CurrentSnapshot, &snapshot); err != nil {
		return IntegrationPreflightResult{}, err
	}
	runtimeRequired, runtimeReady := false, true
	for _, tool := range snapshot.Tools {
		runtimeRequired = runtimeRequired || tool.RuntimeServiceConnectionID != ""
	}
	checks := make([]IntegrationPreflightCheck, 0, len(status.Validations)+2)
	for _, validation := range status.Validations {
		checks = append(checks, preflightCheck(validation.Code, "Publication requirement", "", validation.Message, validation.Tab, false, validation.Level == "error"))
		if validation.Level == "error" && strings.HasPrefix(validation.Code, "runtime_service_") {
			runtimeReady = false
		}
	}
	checks = append(checks, preflightCheck("service_access", "Runtime access", "Selected runtime tools have compatible connection revisions and credentials.", "Runtime access is required only for selected runtime tools; resolve any connection requirements above.", "access", runtimeRequired && runtimeReady, runtimeRequired))
	checks = append(checks, preflightCheck("candidate_integrity", "Reviewed publication", "The selected revisions resolve exactly and are ready to publish.", "Resolve the publication requirements above.", "overview", status.Ready, true))

	result := IntegrationPreflightResult{
		IntegrationID:         status.IntegrationID,
		CandidateRevision:     status.CandidateRevision,
		CandidateManifestHash: status.CurrentManifestHash,
		Checks:                checks,
		Ready:                 true,
		GeneratedAt:           s.now(),
	}
	if status.LatestRevision != nil {
		result.LatestPublishedID = status.LatestRevision.ID
		result.LatestPublishedRevision = status.LatestRevision.Revision
		result.LatestPublishedHash = status.LatestRevision.ManifestHash
		result.MatchesLatestPublished = status.LatestRevision.ManifestHash == status.CurrentManifestHash
	}
	for _, check := range checks {
		if check.Required && check.Status != preflightPass {
			result.Ready = false
			break
		}
	}
	return result, nil
}

func integrationPreflightError(result IntegrationPreflightResult) error {
	for _, check := range result.Checks {
		if check.Required && check.Status != preflightPass {
			return fmt.Errorf("%s: %s", check.Label, check.Message)
		}
	}
	return errors.New("integration preflight failed")
}

type integrationPublishExpectation struct {
	CandidateRevision int64
	ManifestHash      string
}

type integrationPublishExpectationKey struct{}

// PublishIntegrationCandidate binds the publish operation to the exact server
// preflight result the operator reviewed. PublishIntegration performs the same
// required checks for non-HTTP callers.
func (s *Service) PublishIntegrationCandidate(ctx context.Context, integrationID string, candidateRevision int64, manifestHash string, actor Actor) (model.IntegrationRevision, error) {
	manifestHash = strings.TrimSpace(manifestHash)
	if candidateRevision < 1 || manifestHash == "" {
		return model.IntegrationRevision{}, errors.New("candidate_revision and candidate_manifest_hash are required")
	}
	result, err := s.IntegrationPreflight(ctx, integrationID)
	if err != nil {
		return model.IntegrationRevision{}, err
	}
	if !result.Ready {
		return model.IntegrationRevision{}, integrationPreflightError(result)
	}
	if result.CandidateRevision != candidateRevision || result.CandidateManifestHash != manifestHash {
		return model.IntegrationRevision{}, errors.New("the Integration candidate changed after preflight; run preflight again")
	}
	ctx = context.WithValue(ctx, integrationPublishExpectationKey{}, integrationPublishExpectation{CandidateRevision: candidateRevision, ManifestHash: manifestHash})
	return s.PublishIntegration(ctx, integrationID, actor)
}

func integrationPublishExpectationFromContext(ctx context.Context) (integrationPublishExpectation, bool) {
	value, ok := ctx.Value(integrationPublishExpectationKey{}).(integrationPublishExpectation)
	return value, ok
}
