package platform

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

var authorizationKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+$`)

type authorizationCatalogStore interface {
	GrantDefinitions(context.Context, string) ([]model.GrantDefinition, error)
	GrantDefinition(context.Context, string, string) (model.GrantDefinition, error)
	SaveGrantDefinition(context.Context, model.GrantDefinition, int64) (model.GrantDefinition, error)
	AuthorizationPoints(context.Context, string) ([]model.AuthorizationPoint, error)
	AuthorizationPoint(context.Context, string, string) (model.AuthorizationPoint, error)
	SaveAuthorizationPoint(context.Context, model.AuthorizationPoint, int64) (model.AuthorizationPoint, error)
	IntegrationToolBindings(context.Context, string) ([]model.IntegrationToolBinding, error)
	SaveIntegrationToolBindings(context.Context, string, []model.IntegrationToolBinding) ([]model.IntegrationToolBinding, error)
}

func (s *Service) authorizationCatalog() (authorizationCatalogStore, error) {
	value, ok := s.store.(authorizationCatalogStore)
	if !ok {
		return nil, errors.New("authorization catalogue is unavailable")
	}
	return value, nil
}

type GrantDefinitionInput struct {
	Key         string
	DisplayName string
	Description string
	Risk        string
	State       string
	Revision    int64
}

func normalizeGrantDefinition(input GrantDefinitionInput) (GrantDefinitionInput, error) {
	input.Key = strings.ToLower(strings.TrimSpace(input.Key))
	input.DisplayName, input.Description = strings.TrimSpace(input.DisplayName), strings.TrimSpace(input.Description)
	input.Risk, input.State = strings.TrimSpace(input.Risk), strings.TrimSpace(input.State)
	if !authorizationKeyPattern.MatchString(input.Key) || len(input.Key) > 160 {
		return input, errors.New("grant key must be a dotted lower-case identifier")
	}
	if input.DisplayName == "" || len(input.DisplayName) > 120 || len(input.Description) > 1000 {
		return input, errors.New("grant display name or description is invalid")
	}
	if input.Risk == "" {
		input.Risk = "low"
	}
	if input.Risk != "low" && input.Risk != "medium" && input.Risk != "high" && input.Risk != "critical" {
		return input, errors.New("grant risk must be low, medium, high, or critical")
	}
	if input.State == "" {
		input.State = "active"
	}
	if input.State != "active" && input.State != "deprecated" {
		return input, errors.New("grant state must be active or deprecated")
	}
	return input, nil
}

func (s *Service) GrantDefinitions(ctx context.Context) ([]model.GrantDefinition, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return nil, err
	}
	catalog, err := s.authorizationCatalog()
	if err != nil {
		return nil, err
	}
	return catalog.GrantDefinitions(ctx, deployment.ID)
}

func (s *Service) SaveGrantDefinition(ctx context.Context, id string, input GrantDefinitionInput, actor Actor) (model.GrantDefinition, error) {
	input, err := normalizeGrantDefinition(input)
	if err != nil {
		return model.GrantDefinition{}, err
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.GrantDefinition{}, err
	}
	catalog, err := s.authorizationCatalog()
	if err != nil {
		return model.GrantDefinition{}, err
	}
	value := model.GrantDefinition{ID: strings.TrimSpace(id), DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, Key: input.Key, DisplayName: input.DisplayName, Description: input.Description, Risk: input.Risk, State: input.State}
	prior := map[string]any(nil)
	if value.ID == "" {
		value.ID, err = randomUUID()
		if err != nil {
			return model.GrantDefinition{}, err
		}
	} else {
		current, lookupErr := catalog.GrantDefinition(ctx, deployment.ID, value.ID)
		if lookupErr != nil {
			return model.GrantDefinition{}, lookupErr
		}
		if current.Key != value.Key {
			return model.GrantDefinition{}, errors.New("grant key cannot be changed")
		}
		prior = map[string]any{"display_name": current.DisplayName, "risk": current.Risk, "state": current.State, "revision": current.Revision}
	}
	updated, err := catalog.SaveGrantDefinition(ctx, value, input.Revision)
	if err != nil {
		return model.GrantDefinition{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "authorization.grant.saved", TargetType: "grant_definition", TargetID: updated.ID, Prior: prior, Current: map[string]any{"key": updated.Key, "risk": updated.Risk, "state": updated.State, "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

type AuthorizationPointInput struct {
	Key                  string
	Name                 string
	Description          string
	ActionType           string
	RequiredGrants       []string
	ConfirmationRequired bool
	DecisionTTLSeconds   int
	State                string
	Revision             int64
}

func normalizeAuthorizationPoint(input AuthorizationPointInput) (AuthorizationPointInput, error) {
	input.Key = strings.ToLower(strings.TrimSpace(input.Key))
	input.Name, input.Description = strings.TrimSpace(input.Name), strings.TrimSpace(input.Description)
	input.ActionType, input.State = strings.TrimSpace(input.ActionType), strings.TrimSpace(input.State)
	if !authorizationKeyPattern.MatchString(input.Key) || len(input.Key) > 160 {
		return input, errors.New("authorization point key must be a dotted lower-case identifier")
	}
	if input.Name == "" || len(input.Name) > 120 || len(input.Description) > 1000 {
		return input, errors.New("authorization point name or description is invalid")
	}
	if input.ActionType != "read" && input.ActionType != "write" && input.ActionType != "destructive" {
		return input, errors.New("authorization action type must be read, write, or destructive")
	}
	if input.ActionType == "destructive" && !input.ConfirmationRequired {
		return input, errors.New("destructive authorization points require confirmation")
	}
	if input.DecisionTTLSeconds == 0 {
		input.DecisionTTLSeconds = 300
	}
	if input.DecisionTTLSeconds < 0 || input.DecisionTTLSeconds > 3600 {
		return input, errors.New("authorization decision TTL must be between 0 and 3600 seconds")
	}
	if input.State == "" {
		input.State = "draft"
	}
	if input.State != "draft" && input.State != "active" && input.State != "deprecated" {
		return input, errors.New("authorization point state is invalid")
	}
	seen := make(map[string]bool, len(input.RequiredGrants))
	grants := make([]string, 0, len(input.RequiredGrants))
	for _, key := range input.RequiredGrants {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" || seen[key] || !authorizationKeyPattern.MatchString(key) {
			if key == "" || seen[key] {
				continue
			}
			return input, fmt.Errorf("required grant %q is invalid", key)
		}
		seen[key] = true
		grants = append(grants, key)
	}
	if len(grants) > 32 {
		return input, errors.New("authorization point may require at most 32 grants")
	}
	sort.Strings(grants)
	input.RequiredGrants = grants
	return input, nil
}

func missingRegisteredGrants(definitions []model.GrantDefinition, required []string) []string {
	active := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		if definition.State == "active" {
			active[definition.Key] = true
		}
	}
	missing := make([]string, 0)
	for _, key := range required {
		if !active[key] {
			missing = append(missing, key)
		}
	}
	return missing
}

func (s *Service) AuthorizationPoints(ctx context.Context, integrationID string) ([]model.AuthorizationPoint, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.Integration(ctx, deployment.ID, integrationID); err != nil {
		return nil, err
	}
	catalog, err := s.authorizationCatalog()
	if err != nil {
		return nil, err
	}
	return catalog.AuthorizationPoints(ctx, integrationID)
}

func (s *Service) SaveAuthorizationPoint(ctx context.Context, integrationID, pointID string, input AuthorizationPointInput, actor Actor) (model.AuthorizationPoint, error) {
	input, err := normalizeAuthorizationPoint(input)
	if err != nil {
		return model.AuthorizationPoint{}, err
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.AuthorizationPoint{}, err
	}
	if _, err := s.store.Integration(ctx, deployment.ID, integrationID); err != nil {
		return model.AuthorizationPoint{}, err
	}
	catalog, err := s.authorizationCatalog()
	if err != nil {
		return model.AuthorizationPoint{}, err
	}
	definitions, err := catalog.GrantDefinitions(ctx, deployment.ID)
	if err != nil {
		return model.AuthorizationPoint{}, err
	}
	if missing := missingRegisteredGrants(definitions, input.RequiredGrants); len(missing) > 0 {
		return model.AuthorizationPoint{}, fmt.Errorf("register required grants before use: %s", strings.Join(missing, ", "))
	}
	value := model.AuthorizationPoint{ID: strings.TrimSpace(pointID), DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, IntegrationID: integrationID, Key: input.Key, Name: input.Name, Description: input.Description, ActionType: input.ActionType, RequiredGrants: input.RequiredGrants, ConfirmationRequired: input.ConfirmationRequired, DecisionTTLSeconds: input.DecisionTTLSeconds, State: input.State}
	if value.ID == "" {
		value.ID, err = randomUUID()
		if err != nil {
			return model.AuthorizationPoint{}, err
		}
	} else if current, lookupErr := catalog.AuthorizationPoint(ctx, integrationID, value.ID); lookupErr != nil {
		return model.AuthorizationPoint{}, lookupErr
	} else if current.Key != value.Key {
		return model.AuthorizationPoint{}, errors.New("authorization point key cannot be changed")
	}
	updated, err := catalog.SaveAuthorizationPoint(ctx, value, input.Revision)
	if err != nil {
		return model.AuthorizationPoint{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "authorization.point.saved", TargetType: "authorization_point", TargetID: updated.ID, Current: map[string]any{"integration_id": integrationID, "key": updated.Key, "action_type": updated.ActionType, "required_grants": updated.RequiredGrants, "confirmation_required": updated.ConfirmationRequired, "state": updated.State, "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

type AuthorizationSimulation struct {
	AuthorizationPointID string   `json:"authorization_point_id"`
	Allowed              bool     `json:"allowed"`
	MissingGrants        []string `json:"missing_grants"`
	ConfirmationRequired bool     `json:"confirmation_required"`
	ConfirmationMissing  bool     `json:"confirmation_missing"`
	Explanation          string   `json:"explanation"`
	SimulationOnly       bool     `json:"simulation_only"`
}

func (s *Service) SimulateAuthorizationPoint(ctx context.Context, integrationID, pointID string, granted []string, confirmed bool) (AuthorizationSimulation, error) {
	catalog, err := s.authorizationCatalog()
	if err != nil {
		return AuthorizationSimulation{}, err
	}
	point, err := catalog.AuthorizationPoint(ctx, integrationID, pointID)
	if err != nil {
		return AuthorizationSimulation{}, err
	}
	available := make(map[string]bool, len(granted))
	for _, key := range granted {
		available[strings.ToLower(strings.TrimSpace(key))] = true
	}
	missing := make([]string, 0)
	for _, required := range point.RequiredGrants {
		if !available[required] {
			missing = append(missing, required)
		}
	}
	confirmationMissing := point.ConfirmationRequired && !confirmed
	allowed := point.State == "active" && len(missing) == 0 && !confirmationMissing
	explanation := "The simulated policy allows this action."
	if point.State != "active" {
		explanation = "The authorization point is not active."
	} else if len(missing) > 0 {
		explanation = "One or more required grants are missing."
	} else if confirmationMissing {
		explanation = "Explicit confirmation is required."
	}
	return AuthorizationSimulation{AuthorizationPointID: point.ID, Allowed: allowed, MissingGrants: missing, ConfirmationRequired: point.ConfirmationRequired, ConfirmationMissing: confirmationMissing, Explanation: explanation, SimulationOnly: true}, nil
}

func (s *Service) IntegrationToolBindings(ctx context.Context, integrationID string) ([]model.IntegrationToolBinding, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.Integration(ctx, deployment.ID, integrationID); err != nil {
		return nil, err
	}
	catalog, err := s.authorizationCatalog()
	if err != nil {
		return nil, err
	}
	return catalog.IntegrationToolBindings(ctx, integrationID)
}

type ToolRevisionSelection struct {
	ToolID                     string `json:"tool_id"`
	Revision                   int64  `json:"revision"`
	AuthorizationPointID       string `json:"authorization_point_id"`
	AuthorizationPointRevision int64  `json:"authorization_point_revision"`
}

func (s *Service) SetIntegrationToolBindings(ctx context.Context, integrationID string, selections []ToolRevisionSelection, actor Actor) ([]model.IntegrationToolBinding, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.Integration(ctx, deployment.ID, integrationID); err != nil {
		return nil, err
	}
	catalog, err := s.authorizationCatalog()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(selections))
	bindings := make([]model.IntegrationToolBinding, 0, len(selections))
	for _, selection := range selections {
		selection.ToolID = strings.TrimSpace(selection.ToolID)
		selection.AuthorizationPointID = strings.TrimSpace(selection.AuthorizationPointID)
		if selection.ToolID == "" {
			return nil, errors.New("tool_id is required for every integration tool binding")
		}
		if selection.AuthorizationPointID == "" || selection.AuthorizationPointRevision < 1 {
			return nil, errors.New("authorization_point_id and its exact positive revision are required for every integration tool binding")
		}
		if seen[selection.ToolID] {
			return nil, fmt.Errorf("tool %q is selected more than once", selection.ToolID)
		}
		seen[selection.ToolID] = true
		tool, err := s.store.Tool(ctx, deployment.ID, selection.ToolID)
		if err != nil {
			return nil, err
		}
		if tool.State != "published" || selection.Revision != tool.Revision {
			return nil, fmt.Errorf("tool %s must reference its exact published revision", tool.Namespace+"."+tool.Name)
		}
		point, err := catalog.AuthorizationPoint(ctx, integrationID, selection.AuthorizationPointID)
		if err != nil {
			return nil, fmt.Errorf("authorization point for tool %s could not be resolved: %w", tool.Namespace+"."+tool.Name, err)
		}
		if point.Revision != selection.AuthorizationPointRevision {
			return nil, fmt.Errorf("authorization point %s must reference its exact current revision", point.Key)
		}
		if point.State != "active" {
			return nil, fmt.Errorf("authorization point %s must be active before it can authorize a tool", point.Key)
		}
		bindings = append(bindings, model.IntegrationToolBinding{IntegrationID: integrationID, ToolID: tool.ID, ToolRevision: selection.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision, Tool: &tool, AuthorizationPoint: &point, CreatedBy: actor.ID})
	}
	updated, err := catalog.SaveIntegrationToolBindings(ctx, integrationID, bindings)
	if err != nil {
		return nil, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.tools.updated", TargetType: "integration", TargetID: integrationID, Current: map[string]any{"tools": updated}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}
