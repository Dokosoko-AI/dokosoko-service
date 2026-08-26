package store

import (
	"context"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestMemorySDKReleaseLifecycleEventAndAuditAreAtomicAndRetryable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := NewMemory()
	packageValue, err := memory.SaveSDKPackage(ctx, model.SDKPackage{
		ID: "sdk-package-lifecycle-atomic", DeploymentID: "prod_acme", OrganisationID: "org_acme",
		Ecosystem: "npm", CanonicalCoordinate: "@acme/atomic", DisplayCoordinate: "@acme/atomic",
		Name: "Atomic lifecycle SDK", Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	release, err := memory.CreateSDKRelease(ctx, model.SDKRelease{
		ID: "sdk-release-lifecycle-atomic", DeploymentID: "prod_acme", SDKPackageID: packageValue.ID,
		ExactVersion: "1.0.0", InstallCommand: "npm install @acme/atomic@1.0.0", IdentityAssurance: "metadata_only",
		Visibility: model.VisibilityPrivate, Lifecycle: "active", ReleaseHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Now().UTC().Add(-time.Minute)
	event := model.SDKReleaseLifecycleEvent{
		ID: "sdk-release-lifecycle-event-atomic", SDKReleaseID: release.ID, Lifecycle: "yanked",
		Reason: "Upstream registry withdrew the release.", ObservedAt: observedAt, RecordedBy: "release-operator",
	}
	audit := model.AuditEvent{
		ID: "audit-sdk-release-lifecycle-atomic", OrganisationID: "org_acme", ProductID: "prod_acme",
		ActorID: event.RecordedBy, Action: "sdk_release.lifecycle_event.appended", TargetType: "sdk_release", TargetID: release.ID,
		Current: map[string]any{"sdk_release_lifecycle_event_id": event.ID, "effective_lifecycle": "yanked"}, CreatedAt: time.Now().UTC(),
	}
	invalidAudit := audit
	invalidAudit.Current = map[string]any{"sdk_release_lifecycle_event_id": event.ID, "invalid": make(chan int)}
	if _, err := memory.AppendSDKReleaseLifecycleEvent(ctx, "prod_acme", SDKReleaseLifecycleMutation{Event: event, Audit: invalidAudit}); err == nil {
		t.Fatal("non-serializable audit unexpectedly persisted")
	}
	events, err := memory.SDKReleaseLifecycleEvents(ctx, "prod_acme", release.ID)
	if err != nil || len(events) != 0 {
		t.Fatalf("failed audit left lifecycle events = %#v, err = %v", events, err)
	}
	audits, err := memory.AuditEvents(ctx, "org_acme")
	if err != nil || len(audits) != 0 {
		t.Fatalf("failed mutation left audits = %#v, err = %v", audits, err)
	}
	created, err := memory.AppendSDKReleaseLifecycleEvent(ctx, "prod_acme", SDKReleaseLifecycleMutation{Event: event, Audit: audit})
	if err != nil || created.ID != event.ID {
		t.Fatalf("atomic lifecycle mutation = %#v, err = %v", created, err)
	}
	retried, err := memory.AppendSDKReleaseLifecycleEvent(ctx, "prod_acme", SDKReleaseLifecycleMutation{Event: event, Audit: audit})
	if err != nil || retried.ID != created.ID {
		t.Fatalf("exact lifecycle retry = %#v, err = %v", retried, err)
	}
	events, err = memory.SDKReleaseLifecycleEvents(ctx, "prod_acme", release.ID)
	if err != nil || len(events) != 1 || events[0].Lifecycle != "yanked" {
		t.Fatalf("stored lifecycle events = %#v, err = %v", events, err)
	}
	audits, err = memory.AuditEvents(ctx, "org_acme")
	if err != nil || len(audits) != 1 || audits[0].Action != audit.Action {
		t.Fatalf("stored lifecycle audits = %#v, err = %v", audits, err)
	}
}
