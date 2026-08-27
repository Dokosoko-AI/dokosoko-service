package platform

import (
	"context"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestDeploymentOwnsSubmissionURLs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := New(memory)
	current, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	feedbackURL := "https://support.example.test/feedback"
	errorURL := "http://errors.localhost:9080/submissions"

	updated, err := service.UpdateDeployment(ctx, DeploymentInput{
		Name:                  current.Name,
		Slug:                  current.Slug,
		Description:           current.Description,
		FeedbackSubmissionURL: &feedbackURL,
		ErrorSubmissionURL:    &errorURL,
		PublicMCPEnabled:      current.PublicMCPEnabled,
		Revision:              current.Revision,
	}, Actor{ID: "root-test", RequestID: "req-deployment-submission-urls"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.FeedbackSubmissionURL != "https://support.example.test/feedback" || updated.ErrorSubmissionURL != "http://errors.localhost:9080/submissions" {
		t.Fatalf("deployment submission URLs = %#v", updated)
	}
	persisted, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.FeedbackSubmissionURL != updated.FeedbackSubmissionURL || persisted.ErrorSubmissionURL != updated.ErrorSubmissionURL {
		t.Fatalf("persisted deployment = %#v", persisted)
	}

	preserved, err := service.UpdateDeployment(ctx, DeploymentInput{
		Name:             persisted.Name,
		Slug:             persisted.Slug,
		Description:      "updated without submission URL fields",
		PublicMCPEnabled: persisted.PublicMCPEnabled,
		Revision:         persisted.Revision,
	}, Actor{ID: "root-test", RequestID: "req-deployment-submission-urls-omitted"})
	if err != nil {
		t.Fatal(err)
	}
	if preserved.FeedbackSubmissionURL != feedbackURL || preserved.ErrorSubmissionURL != errorURL {
		t.Fatalf("omitted submission URLs were not preserved: %#v", preserved)
	}

	empty := ""
	disabled, err := service.UpdateDeployment(ctx, DeploymentInput{
		Name:                  preserved.Name,
		Slug:                  preserved.Slug,
		Description:           preserved.Description,
		FeedbackSubmissionURL: &empty,
		PublicMCPEnabled:      preserved.PublicMCPEnabled,
		Revision:              preserved.Revision,
	}, Actor{ID: "root-test", RequestID: "req-deployment-submission-url-disabled"})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.FeedbackSubmissionURL != "" || disabled.ErrorSubmissionURL != errorURL {
		t.Fatalf("explicit empty submission URL did not disable only feedback: %#v", disabled)
	}
}

func TestDeploymentRejectsUnsafeSubmissionURLs(t *testing.T) {
	t.Parallel()
	for name, value := range map[string]string{
		"private network": "http://10.0.0.1/feedback",
		"credentials":     "https://user:secret@support.example.test/feedback",
		"query":           "https://support.example.test/feedback?tenant=one",
		"remote port":     "https://support.example.test:8443/feedback",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			memory := store.NewMemory()
			service := New(memory)
			current, err := memory.Deployment(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.UpdateDeployment(ctx, DeploymentInput{Name: current.Name, Slug: current.Slug, Description: current.Description, FeedbackSubmissionURL: &value, PublicMCPEnabled: current.PublicMCPEnabled, Revision: current.Revision}, Actor{ID: "root-test"})
			if err == nil {
				t.Fatalf("unsafe feedback submission URL %q was accepted", value)
			}
		})
	}
}
