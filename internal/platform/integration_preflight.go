package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
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
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return IntegrationPreflightResult{}, err
	}
	integration, err := s.store.Integration(ctx, deployment.ID, strings.TrimSpace(integrationID))
	if err != nil {
		return IntegrationPreflightResult{}, err
	}
	status, err := s.IntegrationPublishStatus(ctx, integration.ID)
	if err != nil {
		return IntegrationPreflightResult{}, err
	}
	private := integration.Visibility != model.VisibilityPublic
	checks := make([]IntegrationPreflightCheck, 0, 11)

	documentationReady, apiReady := false, false
	for _, resource := range integration.Resources {
		resolved := resource.ResolvedRevision != nil && resource.ResolvedRevision.ID != "" && resource.ResolvedRevision.Revision > 0 && resource.ResolvedRevision.ContentHash != ""
		if !resolved {
			continue
		}
		switch resource.Kind {
		case "documentation":
			documentationReady = true
		case "api":
			apiReady = true
		}
	}
	checks = append(checks,
		preflightCheck("documentation_revision", "Published documentation", "An exact reviewed documentation revision is in the candidate manifest.", "Attach an exact reviewed documentation revision.", "documentation", documentationReady, private),
		preflightCheck("api_contract_revision", "API contract", "An exact immutable API contract revision is in the candidate manifest.", "Attach an exact immutable API contract revision.", "documentation", apiReady, private),
	)

	identityReady := false
	if provider, providerErr := s.store.IdentityProvider(ctx, deployment.ID); providerErr == nil {
		identityReady = provider.State == "active" && strings.TrimSpace(provider.Issuer) != "" && strings.TrimSpace(provider.DelegatedAPIOrigin) != ""
	} else if !errors.Is(providerErr, store.ErrNotFound) {
		return IntegrationPreflightResult{}, providerErr
	}
	checks = append(checks, preflightCheck("customer_identity", "Customer identity", "Customer identity and authorization API origin are active.", "Configure an active customer identity provider and authorization API origin.", "access", identityReady, private))

	toolBindings, err := s.store.IntegrationToolBindings(ctx, integration.ID)
	if err != nil {
		return IntegrationPreflightResult{}, err
	}
	runtimeAccessRequired := false
	for _, binding := range toolBindings {
		if binding.Tool != nil && binding.Tool.Scope == model.ToolScopeAPI && binding.Tool.OwnerIntegrationID == integration.ID && binding.Tool.RuntimeServiceConnectionID != "" {
			runtimeAccessRequired = true
			break
		}
	}
	serviceAccessReady := true
	for _, validation := range status.Validations {
		switch {
		case validation.Code == "access_missing", strings.HasPrefix(validation.Code, "runtime_service_"):
			serviceAccessReady = false
		}
	}
	checks = append(checks, preflightCheck("service_access", "Service access", "Every selected API-owned runtime tool has a publish-ready service connection.", "Configure an API endpoint and active compatible credential for every selected runtime tool.", "access", serviceAccessReady, runtimeAccessRequired))

	grants, err := s.store.GrantDefinitions(ctx, deployment.ID)
	if err != nil {
		return IntegrationPreflightResult{}, err
	}
	points, err := s.store.AuthorizationPoints(ctx, integration.ID)
	if err != nil {
		return IntegrationPreflightResult{}, err
	}
	authorizationReady := false
	for _, point := range points {
		if point.State == "active" && point.Revision > 0 && len(missingRegisteredGrants(grants, point.RequiredGrants)) == 0 {
			authorizationReady = true
		}
	}
	checks = append(checks, preflightCheck("authorization_point", "Authorization point", "At least one active authorization point with registered grants is pinned.", "Create an active authorization point using registered grants.", "tools", authorizationReady, private))

	toolsReady := len(toolBindings) > 0
	for _, binding := range toolBindings {
		if binding.Tool == nil || binding.Tool.ID != binding.ToolID || binding.Tool.State != "published" || binding.Tool.Revision != binding.ToolRevision || binding.Tool.UpstreamDrifted {
			toolsReady = false
		}
	}
	checks = append(checks, preflightCheck("tool_revision", "Published tool", "Every selected tool resolves to one non-drifted published revision.", "Bind at least one exact non-drifted published tool revision.", "tools", toolsReady, private))

	recipeReady := false
	recipes, err := s.store.Recipes(ctx, deployment.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return IntegrationPreflightResult{}, err
	}
	for _, recipe := range recipes {
		if recipe.State != "published" {
			continue
		}
		for _, dependency := range recipe.Dependencies {
			if (dependency.Kind == "integration" || dependency.Kind == "integration_scope") && dependency.ResourceID == integration.ID {
				recipeReady = true
			}
		}
	}
	checks = append(checks, preflightCheck("published_recipe", "Published recipe", "Published guidance is scoped to this Integration.", "A published recipe is optional and can be added after the API manifest is ready.", "overview", recipeReady, false))

	sdksReady := true
	for _, reference := range integration.SDKs {
		if reference.ID == "" || reference.Revision < 1 || reference.IntegrationID != integration.ID || reference.ExactVersion == "" || reference.InstallCommand == "" {
			sdksReady = false
		}
	}
	sdkPassMessage := "No SDK is required for this API."
	if len(integration.SDKs) > 0 {
		sdkPassMessage = "Every SDK reference names one exact version and install command."
	}
	checks = append(checks, preflightCheck("sdk_references", "SDK references", sdkPassMessage, "Resolve every SDK reference to one exact version, or remove it.", "documentation", sdksReady, len(integration.SDKs) > 0))

	candidateReady := status.Ready
	checks = append(checks, preflightCheck("candidate_integrity", "Candidate manifest", "The server reproduced the candidate manifest and all exact bindings are internally consistent.", "The candidate contains an unresolved or unsafe binding.", "overview", candidateReady, true))

	result := IntegrationPreflightResult{
		IntegrationID:         integration.ID,
		CandidateRevision:     integration.Revision,
		CandidateManifestHash: status.CurrentManifestHash,
		Checks:                checks,
		Ready:                 true,
		GeneratedAt:           s.now(),
	}
	if integration.Lifecycle == "draft" {
		result.CandidateRevision++
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
