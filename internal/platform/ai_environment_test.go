package platform

import (
	"context"
	"strings"
	"testing"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestDeploymentAIConfigurationPreservesUnmanagedLimitsAndDisablesCleanly(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	service := New(memory)
	maxInput := 64000
	maxOutput := 2048
	dailyBudget := int64(12000)
	configuration := AIEnvironmentConfig{
		Provider: "openai", APIKey: "configured-secret",
		Models:         map[airuntime.Workload]string{airuntime.WorkloadAnalysis: "gpt-configured"},
		MaxInputTokens: &maxInput, MaxOutputTokens: &maxOutput, DailyTokenBudget: &dailyBudget,
	}
	if err := service.ConfigureEnvironmentAI(ctx, configuration); err != nil {
		t.Fatal(err)
	}
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := memory.AIWorkloadProfile(ctx, deployment.ID, string(airuntime.WorkloadAnalysis))
	if err != nil {
		t.Fatal(err)
	}
	if profile.Model != "gpt-configured" || profile.MaxInputTokens != maxInput || profile.MaxOutputTokens != maxOutput || profile.DailyTokenBudget != dailyBudget || !profile.Enabled {
		t.Fatalf("unexpected configured profile: %#v", profile)
	}

	if err := service.ConfigureEnvironmentAI(ctx, AIEnvironmentConfig{Provider: "openai", APIKey: "rotated-secret", Models: map[airuntime.Workload]string{}}); err != nil {
		t.Fatal(err)
	}
	profile, err = memory.AIWorkloadProfile(ctx, deployment.ID, string(airuntime.WorkloadAnalysis))
	if err != nil {
		t.Fatal(err)
	}
	if profile.Model != "gpt-configured" || profile.MaxInputTokens != maxInput || profile.MaxOutputTokens != maxOutput || profile.DailyTokenBudget != dailyBudget {
		t.Fatalf("restart reset existing profile choices: %#v", profile)
	}

	if err := service.ConfigureEnvironmentAI(ctx, AIEnvironmentConfig{}); err != nil {
		t.Fatal(err)
	}
	connections, err := memory.AIProviderConnections(ctx, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 || connections[0].Enabled {
		t.Fatalf("removed deployment configuration left provider enabled: %#v", connections)
	}
	profile, err = memory.AIWorkloadProfile(ctx, deployment.ID, string(airuntime.WorkloadAnalysis))
	if err != nil {
		t.Fatal(err)
	}
	if profile.Enabled {
		t.Fatalf("removed deployment configuration left workload enabled: %#v", profile)
	}
}

func TestConsoleCanTakeOwnershipAfterDeploymentAIConfigurationIsRemoved(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithVault(memory, vault)
	if err := service.ConfigureEnvironmentAI(ctx, AIEnvironmentConfig{Provider: "openai", APIKey: "configured-secret", Models: map[airuntime.Workload]string{}}); err != nil {
		t.Fatal(err)
	}
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	connections, err := memory.AIProviderConnections(ctx, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveAIProviderConnection(ctx, AIProviderConnectionInput{OrganisationID: deployment.OrganisationID, DeploymentID: deployment.ID, Provider: "openai", Credential: "console-secret", Enabled: true, Revision: connections[0].Revision}, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "deployment configuration") {
		t.Fatalf("active deployment configuration takeover error = %v", err)
	}
	if err := service.ConfigureEnvironmentAI(ctx, AIEnvironmentConfig{}); err != nil {
		t.Fatal(err)
	}
	connections, err = memory.AIProviderConnections(ctx, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := service.SaveAIProviderConnection(ctx, AIProviderConnectionInput{OrganisationID: deployment.OrganisationID, DeploymentID: deployment.ID, Provider: "openai", Credential: "console-secret", Enabled: true, Revision: connections[0].Revision}, Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if owned.ManagedBy != "console" || !owned.Enabled || owned.CredentialID == "" {
		t.Fatalf("provider ownership was not transferred: %#v", owned)
	}
}
