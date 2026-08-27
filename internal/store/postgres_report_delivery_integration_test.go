package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestPostgresReportSubmissionDeliveryStateMachine(t *testing.T) {
	_, postgres := migratedPostgresForStoreTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deployment, err := postgres.Deployment(ctx)
	if errors.Is(err, ErrNotFound) {
		organisationID := storeTestUUID(t)
		if _, err = postgres.CreateOrganisation(ctx, model.Organisation{ID: organisationID, Name: "Report delivery", Slug: "report-delivery-" + organisationID[:8]}); err != nil {
			t.Fatal(err)
		}
		deployment, err = postgres.CreateDeployment(ctx, model.Deployment{ID: storeTestUUID(t), OrganisationID: organisationID, Name: "Report delivery", Slug: "report-delivery-" + organisationID[:8]})
	}
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	digest := sha256.Sum256([]byte(storeTestUUID(t)))
	created, err := postgres.CreateReportSubmission(ctx, model.ReportSubmission{
		ID: storeTestUUID(t), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID,
		Kind: "feedback", DeliveryURL: "https://support.example.test/feedback",
		ActorPseudonym: "postgres-report-worker", IdempotencyDigest: digest[:],
		Payload: []byte(`{"schema_version":"1","kind":"feedback"}`), AvailableAt: now.Add(-time.Minute), ExpiresAt: now.Add(24 * time.Hour),
	})
	if err != nil || created.State != "queued" || created.Attempts != 0 || created.DeliveryURL == "" {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	claimed, err := postgres.ClaimReportSubmissions(ctx, "worker-one", now.Add(-time.Second), 10)
	if err != nil || len(claimed) != 1 || claimed[0].ID != created.ID || claimed[0].Attempts != 1 {
		t.Fatalf("claimed=%#v err=%v", claimed, err)
	}
	reclaimed, err := postgres.ClaimReportSubmissions(ctx, "worker-two", now.Add(time.Minute), 10)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].ID != created.ID || reclaimed[0].Attempts != 2 {
		t.Fatalf("reclaimed=%#v err=%v", reclaimed, err)
	}
	if err := postgres.RetryReportSubmission(ctx, created.ID, "worker-two", now.Add(-time.Second), "delivery transport failed"); err != nil {
		t.Fatal(err)
	}
	claimed, err = postgres.ClaimReportSubmissions(ctx, "worker-three", now.Add(time.Minute), 10)
	if err != nil || len(claimed) != 1 || claimed[0].Attempts != 3 {
		t.Fatalf("retried claim=%#v err=%v", claimed, err)
	}
	deliveredAt := time.Now().UTC()
	if err := postgres.CompleteReportSubmission(ctx, created.ID, "worker-three", deliveredAt); err != nil {
		t.Fatal(err)
	}
	stored, err := postgres.ReportSubmission(ctx, deployment.ID, created.ID)
	if err != nil || stored.State != "delivered" || stored.DeliveredAt == nil || !stored.DeliveredAt.Equal(deliveredAt) || stored.LastError != "" {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
}
