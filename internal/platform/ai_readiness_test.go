package platform

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/ai/aitest"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestAIReadinessNamesLocalPrerequisitesWithoutProviderCalls(t *testing.T) {
	for _, code := range []string{"model_missing", "workload_disabled", "provider_disabled", "invalid_configuration", "credential_unavailable", "ready"} {
		t.Run(code, func(t *testing.T) {
			memory := store.NewMemory()
			provider := &aitest.Knowledge{}
			service := New(memory)
			if code != "model_missing" {
				service = configuredKnowledgeService(t, memory, provider)
				profile, err := memory.AIWorkloadProfile(t.Context(), "prod_acme", "analysis")
				if err != nil {
					t.Fatal(err)
				}
				connection, err := memory.AIProviderConnection(t.Context(), "prod_acme", profile.ProviderConnectionID)
				if err != nil {
					t.Fatal(err)
				}
				switch code {
				case "workload_disabled":
					profile.Enabled = false
					_, err = memory.SaveAIWorkloadProfile(t.Context(), profile, profile.Revision)
				case "provider_disabled":
					connection.Enabled = false
					_, err = memory.SaveAIProviderConnection(t.Context(), connection, connection.Revision)
				case "invalid_configuration":
					profile.Model = ""
					_, err = memory.SaveAIWorkloadProfile(t.Context(), profile, profile.Revision)
				case "credential_unavailable":
					delete(service.aiEnvironmentCredentials, connection.Provider)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			status, err := service.AIProcessingReadiness(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if code == "ready" {
				if !status.CanProcess || len(status.Blockers) != 0 {
					t.Fatalf("status=%+v", status)
				}
			} else if status.CanProcess || len(status.Blockers) != 1 || status.Blockers[0] != code {
				t.Fatalf("status=%+v", status)
			}
			if provider.Calls.Load() != 0 {
				t.Fatal("readiness called provider")
			}
			if code != "ready" && code != "credential_unavailable" {
				product, _ := memory.Product(t.Context(), "prod_acme")
				_, _, targetErr := service.aiWorkloadTarget(t.Context(), product, "analysis")
				var setup *AISetupError
				if !errors.As(targetErr, &setup) || setup.Code != code || !errors.Is(targetErr, ErrAIUnavailable) {
					t.Fatalf("runtime disagrees: %v", targetErr)
				}
			}
		})
	}
}

func checkAIReadinessBudget(t *testing.T, backend store.Store) {
	t.Helper()
	provider := &aitest.Knowledge{}
	service := configuredKnowledgeService(t, backend, provider)
	deployment, err := backend.Deployment(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	profile, err := backend.AIWorkloadProfile(t.Context(), deployment.ID, "analysis")
	if err != nil {
		t.Fatal(err)
	}
	profile.DailyTokenBudget = 100
	if _, err = backend.SaveAIWorkloadProfile(t.Context(), profile, profile.Revision); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	id, err := randomUUID()
	if err != nil {
		t.Fatal(err)
	}
	reservation := model.AIBudgetReservation{ID: id, ProductID: deployment.ID, Workload: "analysis", Day: day, ReservedTokens: 100, ExpiresAt: now.Add(time.Minute)}
	ok, err := backend.ReserveAIBudget(t.Context(), reservation, 100)
	if err != nil || !ok {
		t.Fatalf("reserve=%v %v", ok, err)
	}
	for range 2 {
		status, err := service.AIProcessingReadiness(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if status.CanProcess || len(status.Blockers) != 1 || status.Blockers[0] != "budget_exhausted" || status.Budget.Reserved != 100 || status.Budget.Remaining == nil || *status.Budget.Remaining != 0 {
			t.Fatalf("reserved status=%+v budget=%+v", status, status.Budget)
		}
	}
	eventID, _ := randomUUID()
	if err = backend.FinishAIUsage(t.Context(), id, model.AIUsageEvent{ID: eventID, OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, Workload: "analysis", Action: "readiness_test", Provider: "openai-compatible", ProviderRole: "primary", RequestedModel: "fixture", ResolvedModel: "fixture", InputTokens: 10, OutputTokens: 15, Outcome: "succeeded", PromptVersion: "fixture", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	expiredID, _ := randomUUID()
	reservation.ID = expiredID
	reservation.ExpiresAt = now.Add(-time.Second)
	reservation.ReservedTokens = 10
	if ok, err = backend.ReserveAIBudget(t.Context(), reservation, 100); err != nil || !ok {
		t.Fatalf("expired reserve=%v %v", ok, err)
	}
	status, err := service.AIProcessingReadiness(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !status.CanProcess || status.Budget.Used != 25 || status.Budget.Reserved != 0 || *status.Budget.Remaining != 75 {
		t.Fatalf("finished status=%+v budget=%+v", status, status.Budget)
	}
	if provider.Calls.Load() != 0 {
		t.Fatal("readiness consumed AI calls")
	}
	if _, err = backend.AIBudgetStatus(context.Background(), "00000000-0000-4000-8000-000000000099", "analysis", day); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown scope=%v", err)
	}
}

func TestMemoryAIReadinessUsesLiveBudgetReservations(t *testing.T) {
	checkAIReadinessBudget(t, store.NewMemory())
}
func TestPostgresAIReadinessUsesLiveBudgetReservations(t *testing.T) {
	checkAIReadinessBudget(t, knowledgePostgresFixture(t))
}

// A successful historical test must never be presented for a changed credential
// or model. Testing is explicit; readiness must not run a replacement probe.
func TestAIReadinessInvalidatesTestsAfterConfigurationChanges(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x71}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithVault(memory, vault)
	actor := Actor{ID: "reviewer"}
	product, err := memory.Product(t.Context(), "prod_acme")
	if err != nil {
		t.Fatal(err)
	}
	input := AIProviderConnectionInput{OrganisationID: product.OrganisationID, DeploymentID: product.ID, Provider: "openai-compatible", Endpoint: "https://llm.example.test", Credential: "fixture-only", Enabled: true}
	connection, err := service.SaveAIProviderConnection(t.Context(), input, actor)
	if err != nil {
		t.Fatal(err)
	}
	profileInput := AIWorkloadProfileInput{OrganisationID: product.OrganisationID, ProductID: product.ID, Workload: "analysis", ProviderConnectionID: connection.ID, Model: "fixture", MaxInputTokens: 8192, MaxOutputTokens: 1024, Enabled: true}
	profile, err := service.SaveAIWorkloadProfile(t.Context(), profileInput, actor)
	if err != nil {
		t.Fatal(err)
	}
	recordTest := func() {
		t.Helper()
		testedAt := time.Now().UTC()
		connection.LastTestedAt, connection.LastErrorCode = &testedAt, ""
		connection, err = memory.SaveAIProviderConnection(t.Context(), connection, connection.Revision)
		if err != nil {
			t.Fatal(err)
		}
		status, err := service.AIProcessingReadiness(t.Context())
		if err != nil || status.LastTestedAt == nil || !status.CanProcess {
			t.Fatalf("current test not visible: %+v %v", status, err)
		}
	}
	assertUntested := func() {
		t.Helper()
		status, err := service.AIProcessingReadiness(t.Context())
		if err != nil || status.LastTestedAt != nil || status.LastTestErrorCode != "" || !status.CanProcess {
			t.Fatalf("stale test presented: %+v %v", status, err)
		}
	}
	recordTest()
	profileInput.Revision, profileInput.Model = profile.Revision, "changed-model"
	if _, err = service.SaveAIWorkloadProfile(t.Context(), profileInput, actor); err != nil {
		t.Fatal(err)
	}
	assertUntested()
	recordTest()
	input.Revision, input.Credential = connection.Revision, "rotated-fixture"
	connection, err = service.SaveAIProviderConnection(t.Context(), input, actor)
	if err != nil {
		t.Fatal(err)
	}
	if connection.LastTestedAt != nil || connection.LastErrorCode != "" {
		t.Fatal("provider save retained stale probe result")
	}
	assertUntested()
}

func TestAIReadinessDoesNotAttributeAnInFlightProbeToANewModel(t *testing.T) {
	memory := store.NewMemory()
	service := configuredKnowledgeService(t, memory, &aitest.Knowledge{})
	profile, err := memory.AIWorkloadProfile(t.Context(), "prod_acme", "analysis")
	if err != nil {
		t.Fatal(err)
	}
	service.aiRuntime = newAIRuntime(sdkImportDoerFunc(func(request *http.Request) (*http.Response, error) {
		profile.Model = "changed-while-testing"
		if _, err := memory.SaveAIWorkloadProfile(t.Context(), profile, profile.Revision); err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"model":"fixture","choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}]}`))}, nil
	}))
	tested, err := service.TestAIProviderConnection(t.Context(), "prod_acme", profile.ProviderConnectionID, Actor{ID: "reviewer"})
	if err != nil || tested.LastTestedAt == nil {
		t.Fatalf("probe=%+v %v", tested, err)
	}
	status, err := service.AIProcessingReadiness(t.Context())
	if err != nil || status.LastTestedAt != nil || status.Model != "changed-while-testing" || !status.CanProcess {
		t.Fatalf("stale model probe exposed: %+v %v", status, err)
	}
}
