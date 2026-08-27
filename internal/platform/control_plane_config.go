package platform

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var ErrManagedByConfiguration = errors.New("field is managed by deployment configuration")

type ControlPlaneIdentityConfiguration struct {
	Name *string
	Slug *string
}

type ControlPlaneDeploymentConfiguration struct {
	Name                  *string
	Slug                  *string
	Description           *string
	FeedbackSubmissionURL *string
	ErrorSubmissionURL    *string
}

type ControlPlaneEnvironmentConfiguration struct {
	Name         string
	Slug         string
	IsProduction bool
}

type ControlPlaneConfiguration struct {
	Organisation ControlPlaneIdentityConfiguration
	Deployment   ControlPlaneDeploymentConfiguration
	Environments *[]ControlPlaneEnvironmentConfiguration
}

func (configuration ControlPlaneConfiguration) configured() bool {
	return configuration.Organisation.Name != nil || configuration.Organisation.Slug != nil ||
		configuration.Deployment.Name != nil || configuration.Deployment.Slug != nil || configuration.Deployment.Description != nil ||
		configuration.Deployment.FeedbackSubmissionURL != nil || configuration.Deployment.ErrorSubmissionURL != nil || configuration.Environments != nil
}

func configuredValue(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return strings.TrimSpace(*value)
}

func (s *Service) ConfigureControlPlane(ctx context.Context, configuration ControlPlaneConfiguration) error {
	s.deploymentManagedFields = make(map[string]bool)
	if !configuration.configured() {
		return nil
	}
	actor := Actor{ID: "deployment_configuration", RequestID: "startup_configuration"}
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		var deployment model.Deployment
		deployment, err = s.store.Deployment(ctx)
		if errors.Is(err, store.ErrNotFound) {
			_, err = s.createConfiguredControlPlane(ctx, configuration, actor)
		} else if err == nil {
			err = s.reconcileConfiguredDeployment(ctx, deployment, configuration, actor)
		}
		if err == nil {
			break
		}
		if !errors.Is(err, store.ErrConflict) {
			return err
		}
	}
	if err != nil {
		return fmt.Errorf("control-plane reconciliation did not converge after concurrent updates: %w", err)
	}
	if configuration.Deployment.Name != nil {
		s.deploymentManagedFields["name"] = true
	}
	if configuration.Deployment.Slug != nil {
		s.deploymentManagedFields["slug"] = true
	}
	if configuration.Deployment.Description != nil {
		s.deploymentManagedFields["description"] = true
	}
	if configuration.Deployment.FeedbackSubmissionURL != nil {
		s.deploymentManagedFields["feedback_submission_url"] = true
	}
	if configuration.Deployment.ErrorSubmissionURL != nil {
		s.deploymentManagedFields["error_submission_url"] = true
	}
	return nil
}

func (s *Service) createConfiguredControlPlane(ctx context.Context, configuration ControlPlaneConfiguration, actor Actor) (model.Deployment, error) {
	if configuration.Organisation.Name == nil || configuration.Organisation.Slug == nil || configuration.Deployment.Name == nil || configuration.Deployment.Slug == nil {
		return model.Deployment{}, errors.New("control_plane must include organisation and deployment names and slugs when creating the initial workspace")
	}
	if configuration.Environments == nil || len(*configuration.Environments) == 0 {
		return model.Deployment{}, errors.New("control_plane.environments must contain at least one environment when creating the initial workspace")
	}
	organisationName := configuredValue(configuration.Organisation.Name, "")
	organisationSlug := configuredValue(configuration.Organisation.Slug, "")
	organisations, err := s.store.Organisations(ctx)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return model.Deployment{}, err
	}
	var organisation model.Organisation
	for _, candidate := range organisations {
		if candidate.Slug == organisationSlug {
			organisation = candidate
			break
		}
	}
	if organisation.ID != "" && organisation.Name != organisationName {
		return model.Deployment{}, errors.New("control_plane organisation slug already exists with a different name")
	}
	if organisation.ID == "" {
		organisation, err = s.CreateOrganisation(ctx, organisationName, organisationSlug, actor)
		if err != nil {
			return model.Deployment{}, err
		}
	}
	feedbackURL := configuredValue(configuration.Deployment.FeedbackSubmissionURL, "")
	errorURL := configuredValue(configuration.Deployment.ErrorSubmissionURL, "")
	deployment, err := s.CreateDeployment(ctx, DeploymentInput{
		OrganisationID: organisation.ID,
		Name:           configuredValue(configuration.Deployment.Name, ""), Slug: configuredValue(configuration.Deployment.Slug, ""),
		Description: configuredValue(configuration.Deployment.Description, ""), FeedbackSubmissionURL: &feedbackURL, ErrorSubmissionURL: &errorURL,
	}, actor)
	if err != nil {
		return model.Deployment{}, err
	}
	if err := s.reconcileConfiguredEnvironments(ctx, deployment, configuration.Environments, actor); err != nil {
		return model.Deployment{}, err
	}
	return deployment, nil
}

func (s *Service) reconcileConfiguredDeployment(ctx context.Context, deployment model.Deployment, configuration ControlPlaneConfiguration, actor Actor) error {
	organisations, err := s.store.Organisations(ctx)
	if err != nil {
		return err
	}
	var organisation model.Organisation
	for _, candidate := range organisations {
		if candidate.ID == deployment.OrganisationID {
			organisation = candidate
			break
		}
	}
	if organisation.ID == "" {
		return store.ErrNotFound
	}
	if configuration.Organisation.Name != nil && configuredValue(configuration.Organisation.Name, "") != organisation.Name {
		return errors.New("control_plane.organisation.name does not match the existing immutable organisation")
	}
	if configuration.Organisation.Slug != nil && configuredValue(configuration.Organisation.Slug, "") != organisation.Slug {
		return errors.New("control_plane.organisation.slug does not match the existing immutable organisation")
	}
	feedbackURL := deployment.FeedbackSubmissionURL
	if configuration.Deployment.FeedbackSubmissionURL != nil {
		feedbackURL = configuredValue(configuration.Deployment.FeedbackSubmissionURL, "")
	}
	errorURL := deployment.ErrorSubmissionURL
	if configuration.Deployment.ErrorSubmissionURL != nil {
		errorURL = configuredValue(configuration.Deployment.ErrorSubmissionURL, "")
	}
	desired := DeploymentInput{
		Name: configuredValue(configuration.Deployment.Name, deployment.Name), Slug: configuredValue(configuration.Deployment.Slug, deployment.Slug),
		Description: configuredValue(configuration.Deployment.Description, deployment.Description), FeedbackSubmissionURL: &feedbackURL, ErrorSubmissionURL: &errorURL,
		PublicMCPEnabled: deployment.PublicMCPEnabled, Revision: deployment.Revision,
	}
	if desired.Name != deployment.Name || desired.Slug != deployment.Slug || desired.Description != deployment.Description || feedbackURL != deployment.FeedbackSubmissionURL || errorURL != deployment.ErrorSubmissionURL {
		updated, updateErr := s.UpdateDeployment(ctx, desired, actor)
		if updateErr != nil {
			return updateErr
		}
		deployment = updated
	}
	return s.reconcileConfiguredEnvironments(ctx, deployment, configuration.Environments, actor)
}

func (s *Service) reconcileConfiguredEnvironments(ctx context.Context, deployment model.Deployment, configured *[]ControlPlaneEnvironmentConfiguration, actor Actor) error {
	if configured == nil {
		return nil
	}
	existing, err := s.store.Environments(ctx, deployment.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	bySlug := make(map[string]model.Environment, len(existing))
	var production *model.Environment
	for _, environment := range existing {
		bySlug[environment.Slug] = environment
		if environment.IsProduction {
			candidate := environment
			production = &candidate
		}
	}
	for _, desired := range *configured {
		desired.Name, desired.Slug = strings.TrimSpace(desired.Name), strings.TrimSpace(desired.Slug)
		current, found := bySlug[desired.Slug]
		if found {
			if current.Name != desired.Name || current.IsProduction != desired.IsProduction {
				return fmt.Errorf("control_plane environment %q already exists with different immutable settings", desired.Slug)
			}
			continue
		}
		if desired.IsProduction && production != nil {
			return fmt.Errorf("control_plane environment %q cannot become production because %q is already the production environment", desired.Slug, production.Slug)
		}
		if _, err := s.CreateEnvironment(ctx, deployment.OrganisationID, deployment.ID, desired.Name, desired.Slug, desired.IsProduction, actor); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) DeploymentManagedFields() []string {
	result := make([]string, 0, len(s.deploymentManagedFields))
	for field := range s.deploymentManagedFields {
		result = append(result, field)
	}
	sort.Strings(result)
	return result
}

func (s *Service) deploymentFieldManaged(field string) bool {
	return s.deploymentManagedFields[field]
}
