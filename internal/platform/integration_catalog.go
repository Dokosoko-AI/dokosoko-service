package platform

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

var integrationVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type DeploymentInput struct {
	OrganisationID        string
	Name                  string
	Slug                  string
	Description           string
	FeedbackSubmissionURL *string
	ErrorSubmissionURL    *string
	PublicMCPEnabled      bool
	Revision              int64
}

func validateDeploymentSubmissionURL(label, value string) error {
	if len(value) > 2048 || value != "" && !validOutboundHookURI(value) {
		return errors.New(label + " must be a credential-free HTTPS URL or localhost HTTP URL")
	}
	return nil
}

func normalizedDeploymentSubmissionURL(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func (s *Service) CreateDeployment(ctx context.Context, input DeploymentInput, actor Actor) (model.Deployment, error) {
	input.OrganisationID, input.Name, input.Slug = strings.TrimSpace(input.OrganisationID), strings.TrimSpace(input.Name), strings.TrimSpace(input.Slug)
	input.Description = strings.TrimSpace(input.Description)
	feedbackSubmissionURL := normalizedDeploymentSubmissionURL(input.FeedbackSubmissionURL)
	errorSubmissionURL := normalizedDeploymentSubmissionURL(input.ErrorSubmissionURL)
	if input.OrganisationID == "" || validateNameSlug(input.Name, input.Slug) != nil {
		return model.Deployment{}, errors.New("organisation, deployment name, and a valid slug are required")
	}
	if len(input.Description) > 2000 {
		return model.Deployment{}, errors.New("deployment description must be no more than 2000 characters")
	}
	if err := validateDeploymentSubmissionURL("feedback submission URL", feedbackSubmissionURL); err != nil {
		return model.Deployment{}, err
	}
	if err := validateDeploymentSubmissionURL("error submission URL", errorSubmissionURL); err != nil {
		return model.Deployment{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.Deployment{}, err
	}
	value, err := s.store.CreateDeployment(ctx, model.Deployment{ID: id, OrganisationID: input.OrganisationID, Name: input.Name, Slug: input.Slug, Description: input.Description, FeedbackSubmissionURL: feedbackSubmissionURL, ErrorSubmissionURL: errorSubmissionURL, PublicMCPEnabled: input.PublicMCPEnabled})
	if err != nil {
		return model.Deployment{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: value.OrganisationID, ProductID: value.ID, ActorID: actor.ID, Action: "deployment.created", TargetType: "deployment", TargetID: value.ID, Current: map[string]any{"name": value.Name, "slug": value.Slug, "feedback_submission_url": value.FeedbackSubmissionURL, "error_submission_url": value.ErrorSubmissionURL}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.Deployment{}, err
	}
	if s.pendingAIConfiguration != nil {
		configuration := *s.pendingAIConfiguration
		if err := s.ConfigureEnvironmentAI(ctx, configuration); err != nil {
			return model.Deployment{}, err
		}
	}
	return value, nil
}

func (s *Service) UpdateDeployment(ctx context.Context, input DeploymentInput, actor Actor) (model.Deployment, error) {
	current, err := s.store.Deployment(ctx)
	if err != nil {
		return model.Deployment{}, err
	}
	if s.deploymentFieldManaged("name") && strings.TrimSpace(input.Name) != current.Name ||
		s.deploymentFieldManaged("slug") && strings.TrimSpace(input.Slug) != current.Slug ||
		s.deploymentFieldManaged("description") && strings.TrimSpace(input.Description) != current.Description ||
		s.deploymentFieldManaged("feedback_submission_url") && input.FeedbackSubmissionURL != nil && normalizedDeploymentSubmissionURL(input.FeedbackSubmissionURL) != current.FeedbackSubmissionURL ||
		s.deploymentFieldManaged("error_submission_url") && input.ErrorSubmissionURL != nil && normalizedDeploymentSubmissionURL(input.ErrorSubmissionURL) != current.ErrorSubmissionURL {
		return model.Deployment{}, ErrManagedByConfiguration
	}
	input.Name, input.Slug, input.Description = strings.TrimSpace(input.Name), strings.TrimSpace(input.Slug), strings.TrimSpace(input.Description)
	if validateNameSlug(input.Name, input.Slug) != nil || len(input.Description) > 2000 {
		return model.Deployment{}, errors.New("deployment name, slug, or description is invalid")
	}
	previous := current
	if input.FeedbackSubmissionURL != nil {
		value := normalizedDeploymentSubmissionURL(input.FeedbackSubmissionURL)
		if err := validateDeploymentSubmissionURL("feedback submission URL", value); err != nil {
			return model.Deployment{}, err
		}
		current.FeedbackSubmissionURL = value
	}
	if input.ErrorSubmissionURL != nil {
		value := normalizedDeploymentSubmissionURL(input.ErrorSubmissionURL)
		if err := validateDeploymentSubmissionURL("error submission URL", value); err != nil {
			return model.Deployment{}, err
		}
		current.ErrorSubmissionURL = value
	}
	current.Name, current.Slug, current.Description = input.Name, input.Slug, input.Description
	current.PublicMCPEnabled = input.PublicMCPEnabled
	updated, err := s.store.UpdateDeployment(ctx, current, input.Revision)
	if err != nil {
		return model.Deployment{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.ID, ActorID: actor.ID, Action: "deployment.updated", TargetType: "deployment", TargetID: updated.ID, Prior: map[string]any{"name": previous.Name, "slug": previous.Slug, "feedback_submission_url": previous.FeedbackSubmissionURL, "error_submission_url": previous.ErrorSubmissionURL}, Current: map[string]any{"name": updated.Name, "slug": updated.Slug, "feedback_submission_url": updated.FeedbackSubmissionURL, "error_submission_url": updated.ErrorSubmissionURL}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.Deployment{}, err
	}
	return updated, nil
}

type IntegrationInput struct {
	FamilyKey                string
	VersionKey               string
	DisplayName              string
	Description              string
	Visibility               model.Visibility
	AcknowledgePublic        bool
	Lifecycle                string
	ReplacementIntegrationID string
	SunsetAt                 *time.Time
	Revision                 int64
}

func normalizeIntegrationInput(input IntegrationInput) (IntegrationInput, error) {
	input.FamilyKey, input.VersionKey = strings.ToLower(strings.TrimSpace(input.FamilyKey)), strings.TrimSpace(input.VersionKey)
	input.DisplayName, input.Description = strings.TrimSpace(input.DisplayName), strings.TrimSpace(input.Description)
	input.ReplacementIntegrationID = strings.TrimSpace(input.ReplacementIntegrationID)
	if !slugPattern.MatchString(input.FamilyKey) || len(input.FamilyKey) > 63 {
		return input, errors.New("integration family key must use lower-case letters, numbers, and single hyphens")
	}
	if !integrationVersionPattern.MatchString(input.VersionKey) {
		return input, errors.New("integration version key is invalid")
	}
	if input.DisplayName == "" || len(input.DisplayName) > 120 || len(input.Description) > 2000 {
		return input, errors.New("integration display name or description is invalid")
	}
	if input.Visibility == "" {
		input.Visibility = model.VisibilityPrivate
	}
	if !input.Visibility.Valid() {
		return input, ErrInvalidVisibility
	}
	if input.Lifecycle == "" {
		input.Lifecycle = "draft"
	}
	switch input.Lifecycle {
	case "draft", "active", "deprecated", "retired":
	default:
		return input, errors.New("integration lifecycle is invalid")
	}
	if (input.ReplacementIntegrationID != "" || input.SunsetAt != nil) && input.Lifecycle != "deprecated" && input.Lifecycle != "retired" {
		return input, errors.New("replacement and sunset are only valid for deprecated or retired integrations")
	}
	return input, nil
}

func (s *Service) CreateIntegration(ctx context.Context, input IntegrationInput, actor Actor) (model.Integration, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.Integration{}, err
	}
	input, err = normalizeIntegrationInput(input)
	if err != nil {
		return model.Integration{}, err
	}
	if input.ReplacementIntegrationID != "" {
		if _, err := s.store.Integration(ctx, deployment.ID, input.ReplacementIntegrationID); err != nil {
			return model.Integration{}, errors.New("replacement integration does not exist in this deployment")
		}
	}
	if input.Visibility == model.VisibilityPublic && !input.AcknowledgePublic {
		return model.Integration{}, ErrConfirmationRequired
	}
	id, err := randomUUID()
	if err != nil {
		return model.Integration{}, err
	}
	value, err := s.store.CreateIntegration(ctx, model.Integration{ID: id, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, FamilyKey: input.FamilyKey, VersionKey: input.VersionKey, DisplayName: input.DisplayName, Description: input.Description, Visibility: input.Visibility, Lifecycle: input.Lifecycle, ReplacementIntegrationID: input.ReplacementIntegrationID, SunsetAt: input.SunsetAt})
	if err != nil {
		return model.Integration{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.created", TargetType: "integration", TargetID: value.ID, Current: map[string]any{"family_key": value.FamilyKey, "version_key": value.VersionKey, "visibility": value.Visibility, "lifecycle": value.Lifecycle}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.Integration{}, err
	}
	return value, nil
}

func (s *Service) UpdateIntegration(ctx context.Context, integrationID string, input IntegrationInput, actor Actor) (model.Integration, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.Integration{}, err
	}
	current, err := s.store.Integration(ctx, deployment.ID, integrationID)
	if err != nil {
		return model.Integration{}, err
	}
	if input.Visibility == "" {
		input.Visibility = current.Visibility
	}
	input, err = normalizeIntegrationInput(input)
	if err != nil {
		return model.Integration{}, err
	}
	if input.ReplacementIntegrationID == integrationID {
		return model.Integration{}, errors.New("an integration cannot replace itself")
	}
	if current.Visibility != model.VisibilityPublic && input.Visibility == model.VisibilityPublic && !input.AcknowledgePublic {
		return model.Integration{}, ErrConfirmationRequired
	}
	if input.ReplacementIntegrationID != "" {
		if _, err := s.store.Integration(ctx, deployment.ID, input.ReplacementIntegrationID); err != nil {
			return model.Integration{}, errors.New("replacement integration does not exist in this deployment")
		}
	}
	current.FamilyKey, current.VersionKey, current.DisplayName, current.Description = input.FamilyKey, input.VersionKey, input.DisplayName, input.Description
	current.Visibility, current.Lifecycle, current.ReplacementIntegrationID, current.SunsetAt = input.Visibility, input.Lifecycle, input.ReplacementIntegrationID, input.SunsetAt
	updated, err := s.store.UpdateIntegration(ctx, current, input.Revision)
	if err != nil {
		return model.Integration{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "integration.updated", TargetType: "integration", TargetID: updated.ID, Current: map[string]any{"family_key": updated.FamilyKey, "version_key": updated.VersionKey, "visibility": updated.Visibility, "lifecycle": updated.Lifecycle, "revision": updated.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.Integration{}, err
	}
	return updated, nil
}
