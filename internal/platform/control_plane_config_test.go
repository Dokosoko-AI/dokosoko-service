package platform_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func configuredString(value string) *string { return &value }

type conflictOnceStore struct {
	store.Store
	conflicted bool
}

func (s *conflictOnceStore) UpdateDeployment(ctx context.Context, value model.Deployment, revision int64) (model.Deployment, error) {
	if !s.conflicted {
		s.conflicted = true
		return model.Deployment{}, store.ErrConflict
	}
	return s.Store.UpdateDeployment(ctx, value, revision)
}

func TestConfigureControlPlaneCreatesInitialWorkspaceAndIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewEmptyMemory()
	service := platform.New(memory)
	environments := []platform.ControlPlaneEnvironmentConfiguration{
		{Name: "Production", Slug: "production", IsProduction: true},
		{Name: "Staging", Slug: "staging"},
	}
	configuration := platform.ControlPlaneConfiguration{
		Organisation: platform.ControlPlaneIdentityConfiguration{Name: configuredString("Configured Organisation"), Slug: configuredString("configured-organisation")},
		Deployment: platform.ControlPlaneDeploymentConfiguration{
			Name: configuredString("Configured Deployment"), Slug: configuredString("configured-deployment"), Description: configuredString("Managed at startup."),
		},
		Environments: &environments,
	}

	if err := service.ConfigureControlPlane(ctx, configuration); err != nil {
		t.Fatal(err)
	}
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Name != "Configured Deployment" || deployment.Slug != "configured-deployment" || deployment.Description != "Managed at startup." {
		t.Fatalf("unexpected deployment: %#v", deployment)
	}
	organisations, err := memory.Organisations(ctx)
	if err != nil || len(organisations) != 1 || organisations[0].Slug != "configured-organisation" {
		t.Fatalf("unexpected organisations: %#v, err=%v", organisations, err)
	}
	createdEnvironments, err := memory.Environments(ctx, deployment.ID)
	if err != nil || len(createdEnvironments) != 2 {
		t.Fatalf("unexpected environments: %#v, err=%v", createdEnvironments, err)
	}
	if !reflect.DeepEqual(service.DeploymentManagedFields(), []string{"description", "name", "slug"}) {
		t.Fatalf("managed fields = %#v", service.DeploymentManagedFields())
	}

	if err := service.ConfigureControlPlane(ctx, configuration); err != nil {
		t.Fatal(err)
	}
	after, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != deployment.Revision {
		t.Fatalf("idempotent reconciliation changed revision from %d to %d", deployment.Revision, after.Revision)
	}
}

func TestConfigureControlPlaneReconcilesExistingDeploymentAndProtectsManagedFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	description, feedbackURL := "Configured description", "https://support.example.com/feedback"
	environments := []platform.ControlPlaneEnvironmentConfiguration{{Name: "Staging", Slug: "staging"}}
	configuration := platform.ControlPlaneConfiguration{
		Organisation: platform.ControlPlaneIdentityConfiguration{Name: configuredString("Acme"), Slug: configuredString("acme")},
		Deployment:   platform.ControlPlaneDeploymentConfiguration{Description: &description, FeedbackSubmissionURL: &feedbackURL},
		Environments: &environments,
	}
	if err := service.ConfigureControlPlane(ctx, configuration); err != nil {
		t.Fatal(err)
	}
	configured, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if configured.Description != description || configured.FeedbackSubmissionURL != feedbackURL {
		t.Fatalf("deployment was not reconciled: %#v", configured)
	}
	createdEnvironments, err := memory.Environments(ctx, configured.ID)
	if err != nil || len(createdEnvironments) != 2 {
		t.Fatalf("configured environments = %#v, err=%v", createdEnvironments, err)
	}

	changedDescription := configured
	changedDescription.Description = "UI change"
	if _, err := service.UpdateDeployment(ctx, platform.DeploymentInput{
		Name: changedDescription.Name, Slug: changedDescription.Slug, Description: changedDescription.Description,
		FeedbackSubmissionURL: &changedDescription.FeedbackSubmissionURL, ErrorSubmissionURL: &changedDescription.ErrorSubmissionURL,
		PublicMCPEnabled: changedDescription.PublicMCPEnabled, Revision: changedDescription.Revision,
	}, platform.Actor{ID: "root_test"}); !errors.Is(err, platform.ErrManagedByConfiguration) {
		t.Fatalf("managed field update error = %v", err)
	}

	configured.PublicMCPEnabled = true
	updated, err := service.UpdateDeployment(ctx, platform.DeploymentInput{
		Name: configured.Name, Slug: configured.Slug, Description: configured.Description,
		FeedbackSubmissionURL: &configured.FeedbackSubmissionURL, ErrorSubmissionURL: &configured.ErrorSubmissionURL,
		PublicMCPEnabled: configured.PublicMCPEnabled, Revision: configured.Revision,
	}, platform.Actor{ID: "root_test"})
	if err != nil || !updated.PublicMCPEnabled {
		t.Fatalf("unmanaged field update = %#v, err=%v", updated, err)
	}
}

func TestConfigureControlPlaneRejectsImmutableEnvironmentDrift(t *testing.T) {
	t.Parallel()
	service := platform.New(store.NewMemory())
	environments := []platform.ControlPlaneEnvironmentConfiguration{{Name: "Production renamed", Slug: "production", IsProduction: true}}
	if err := service.ConfigureControlPlane(context.Background(), platform.ControlPlaneConfiguration{Environments: &environments}); err == nil {
		t.Fatal("expected immutable configured environment mismatch")
	}
}

func TestConfigureControlPlaneRetriesConcurrentRevisionConflict(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	persistence := &conflictOnceStore{Store: memory}
	service := platform.New(persistence)
	description := "Converged description"
	if err := service.ConfigureControlPlane(context.Background(), platform.ControlPlaneConfiguration{
		Deployment: platform.ControlPlaneDeploymentConfiguration{Description: &description},
	}); err != nil {
		t.Fatal(err)
	}
	deployment, err := memory.Deployment(context.Background())
	if err != nil || deployment.Description != description || !persistence.conflicted {
		t.Fatalf("reconciled deployment=%#v conflicted=%t err=%v", deployment, persistence.conflicted, err)
	}
}
