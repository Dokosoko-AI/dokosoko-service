package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"strings"
)

func (s *Service) StartIntegrationRun(ctx context.Context, productID, environmentID, requestedOutcome string, actor Actor) (model.IntegrationRun, error) {
	requestedOutcome = strings.TrimSpace(requestedOutcome)
	if requestedOutcome == "" || len(requestedOutcome) > 500 {
		return model.IntegrationRun{}, errors.New("requested outcome must be between 1 and 500 characters")
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.IntegrationRun{}, err
	}
	environments, err := s.store.Environments(ctx, productID)
	if err != nil {
		return model.IntegrationRun{}, err
	}
	validEnvironment := false
	for _, environment := range environments {
		if environment.ID == environmentID && environment.OrganisationID == product.OrganisationID {
			validEnvironment = true
			break
		}
	}
	if !validEnvironment {
		return model.IntegrationRun{}, errors.New("environment does not belong to the product")
	}
	id, err := randomUUID()
	if err != nil {
		return model.IntegrationRun{}, err
	}
	actorPseudonym := pseudonymActor(productID, actor.ID)
	if actorPseudonym == "" {
		return model.IntegrationRun{}, errors.New("an authenticated run owner is required")
	}
	value, err := s.store.CreateIntegrationRun(ctx, model.IntegrationRun{ID: id, OrganisationID: product.OrganisationID, ProductID: productID, EnvironmentID: environmentID, ActorPseudonym: actorPseudonym, RequestedOutcome: requestedOutcome, State: "running", StartedAt: s.now()})
	if err != nil {
		return model.IntegrationRun{}, err
	}
	_ = s.store.AppendAnalytics(ctx, model.AnalyticsEvent{OrganisationID: product.OrganisationID, ProductID: productID, EventName: "run_started", ActorKind: "vendor_user", ActorPseudonym: actorPseudonym, IntegrationRunID: value.ID, CreatedAt: s.now()})
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: product.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "integration_run.started", TargetType: "integration_run", TargetID: value.ID, Current: map[string]any{"environment_id": environmentID, "state": value.State}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func (s *Service) CompleteIntegrationRun(ctx context.Context, productID, runID string, reportedSuccess, validatedSuccess *bool, failureCode string, actor Actor) (model.IntegrationRun, error) {
	if validatedSuccess == nil {
		return model.IntegrationRun{}, errors.New("a deterministic validation result is required")
	}
	failureCode = strings.TrimSpace(failureCode)
	if !*validatedSuccess && (failureCode == "" || len(failureCode) > 120) {
		return model.IntegrationRun{}, errors.New("a failure code is required when validation fails")
	}
	if *validatedSuccess {
		failureCode = ""
	}
	current, err := s.store.IntegrationRun(ctx, productID, runID)
	if err != nil {
		return model.IntegrationRun{}, err
	}
	actorPseudonym := pseudonymActor(productID, actor.ID)
	if actorPseudonym == "" || current.ActorPseudonym != actorPseudonym {
		return model.IntegrationRun{}, errors.New("integration run is not owned by this principal")
	}
	value, err := s.store.CompleteIntegrationRun(ctx, productID, runID, reportedSuccess, validatedSuccess, failureCode, s.now())
	if err != nil {
		return model.IntegrationRun{}, err
	}
	_ = s.store.AppendAnalytics(ctx, model.AnalyticsEvent{OrganisationID: value.OrganisationID, ProductID: productID, EventName: "implementation_validated", ActorKind: "vendor_user", ActorPseudonym: actorPseudonym, IntegrationRunID: value.ID, Dimensions: map[string]any{"success": *validatedSuccess}, CreatedAt: s.now()})
	if reportedSuccess != nil {
		_ = s.store.AppendAnalytics(ctx, model.AnalyticsEvent{OrganisationID: value.OrganisationID, ProductID: productID, EventName: "success_reported", ActorKind: "vendor_user", ActorPseudonym: actorPseudonym, IntegrationRunID: value.ID, Dimensions: map[string]any{"success": *reportedSuccess}, CreatedAt: s.now()})
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: value.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "integration_run.completed", TargetType: "integration_run", TargetID: runID, Prior: map[string]any{"state": current.State}, Current: map[string]any{"state": value.State, "reported_success": reportedSuccess, "validated_success": validatedSuccess, "failure_code": failureCode}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, err
}

func pseudonymActor(productID, actorID string) string {
	if actorID == "" || actorID == "anonymous" {
		return ""
	}
	digest := sha256.Sum256([]byte(productID + "\x00" + actorID))
	return hex.EncodeToString(digest[:16])
}
