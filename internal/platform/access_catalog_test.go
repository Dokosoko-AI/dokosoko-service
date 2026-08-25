package platform

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestAccessDefinitionRevisionPreservesConnectionIdentity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := New(memory)
	actor := Actor{ID: "root-test", RequestID: "request-access-revision"}
	integration, err := service.CreateIntegration(ctx, IntegrationInput{FamilyKey: "voice", VersionKey: "v1", DisplayName: "Voice API", Description: "Voice API contract.", Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := service.CreateAccessDefinition(ctx, AccessDefinitionInput{ServiceKey: "voice", Name: "Voice management", InstanceCardinality: "one", InstanceLabelSingular: "account", InstanceLabelPlural: "accounts", CredentialScope: "connection", ManagementAuthType: "none", Operations: json.RawMessage(`{"authorize":{"method":"POST","path":"/v1/authorize"}}`)}, actor)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := service.CreateAccessConnection(ctx, AccessConnectionInput{AccessDefinitionID: definition.ID, Name: "Voice production", BaseURL: "https://voice.example.test", Config: json.RawMessage(`{}`), IntegrationIDs: []string{integration.ID}}, actor)
	if err != nil {
		t.Fatal(err)
	}

	updated, err := service.UpdateAccessDefinition(ctx, definition.ID, AccessDefinitionInput{ServiceKey: "attempted-change", Name: "Voice credential management", InstanceCardinality: "many", InstanceLabelSingular: "tenant", InstanceLabelPlural: "tenants", CredentialScope: "instance", ManagementAuthType: "bearer", Operations: json.RawMessage(`{"credentials.create":{"method":"POST","path":"/v1/external-platform/credentials"}}`)}, definition.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.ServiceKey != definition.ServiceKey || updated.InstanceCardinality != definition.InstanceCardinality || updated.CredentialScope != definition.CredentialScope || updated.ManagementAuthType != definition.ManagementAuthType {
		t.Fatalf("updated immutable contract = %#v", updated)
	}
	if updated.Name != "Voice credential management" || updated.InstanceLabelSingular != "tenant" {
		t.Fatalf("updated mutable contract = %#v", updated)
	}
	if _, err := service.UpdateAccessDefinition(ctx, definition.ID, AccessDefinitionInput{Name: "Stale", InstanceLabelSingular: "tenant", InstanceLabelPlural: "tenants", Operations: updated.Operations}, definition.Revision, actor); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale update error = %v", err)
	}
	storedConnection, err := memory.AccessConnection(ctx, integration.DeploymentID, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedConnection.AccessDefinitionID != definition.ID || storedConnection.Definition == nil || storedConnection.Definition.Revision != 2 {
		t.Fatalf("connection did not resolve the revised definition: %#v", storedConnection)
	}
}
