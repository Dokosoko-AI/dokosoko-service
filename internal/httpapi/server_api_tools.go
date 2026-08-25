package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	accessruntime "github.com/dokosoko/dokosoko-service/internal/access"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

type apiDefaultToolKind string

const (
	apiKnowledgeSearch   apiDefaultToolKind = "knowledge.search"
	apiInstancesList     apiDefaultToolKind = "admin.instances.list"
	apiInstancesCreate   apiDefaultToolKind = "admin.instances.create"
	apiCredentialsList   apiDefaultToolKind = "admin.credentials.list"
	apiCredentialsRotate apiDefaultToolKind = "admin.credentials.rotate"
	apiCredentialsRevoke apiDefaultToolKind = "admin.credentials.revoke"
)

type apiDefaultToolBinding struct {
	Name                     string
	Kind                     apiDefaultToolKind
	OrganisationID           string
	IntegrationID            string
	FamilyKey                string
	ConnectionID             string
	ConnectionRevision       int64
	AccessDefinitionID       string
	AccessDefinitionRevision int64
	EnvironmentVariable      string
	InputSchema              json.RawMessage
}

func validGeneratedToolSegment(value string) bool {
	if value == "" || len(value) > 63 {
		return false
	}
	for index, char := range value {
		valid := char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || (index > 0 && (char == '-' || char == '_'))
		if !valid {
			return false
		}
	}
	return true
}

func manifestFamilyCounts(manifest model.ProductManifest) map[string]int {
	result := make(map[string]int, len(manifest.Integrations))
	for _, integration := range manifest.Integrations {
		result[integration.FamilyKey]++
	}
	return result
}

func integrationHasKnowledge(integration model.IntegrationManifest) bool {
	for _, resource := range integration.Resources {
		if resource.Kind == "documentation" && len(resource.SourcePublications) > 0 {
			return true
		}
	}
	return false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func apiToolSchema(properties map[string]any, required ...string) map[string]any {
	value := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
	if len(required) > 0 {
		value["required"] = required
	}
	return value
}

func schemaJSON(value map[string]any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func generatedToolDefinition(binding apiDefaultToolBinding, description string, schema map[string]any, readOnly, destructive, confirmation, idempotency bool, metadata map[string]any) map[string]any {
	metadataCopy := make(map[string]any, len(metadata)+6)
	for key, value := range metadata {
		metadataCopy[key] = value
	}
	metadata = metadataCopy
	metadata["com.dokosoko/integrationId"] = binding.IntegrationID
	metadata["com.dokosoko/confirmationRequired"] = confirmation
	if confirmation {
		metadata["com.dokosoko/confirmationChallengeMetaField"] = managedToolConfirmationMetaField
		metadata["com.dokosoko/confirmationAttestationMetaField"] = "confirmed"
	}
	metadata["com.dokosoko/idempotencyKeyRequired"] = idempotency
	if idempotency {
		metadata["com.dokosoko/idempotencyKeyMetaField"] = "idempotency_key"
	}
	return map[string]any{
		"name":        binding.Name,
		"description": description,
		"inputSchema": schema,
		"annotations": map[string]any{"readOnlyHint": readOnly, "destructiveHint": destructive, "idempotentHint": readOnly || idempotency},
		"_meta":       metadata,
	}
}

type apiAdminCandidate struct {
	integration model.IntegrationManifest
	manifest    model.IntegrationManifestAccessConnection
	connection  model.AccessConnection
	capability  accessruntime.Capability
}

// apiDefaultToolDefinitions derives the generated API knowledge/admin surface
// only from the resolved immutable Integration manifest and exact still-active
// management connection revisions. A changed or ambiguous binding disappears
// fail closed until the Integration is republished.
func (s *Server) apiDefaultToolDefinitions(ctx context.Context, productID string, manifest model.ProductManifest, principal identity.Principal, public bool) ([]map[string]any, map[string]apiDefaultToolBinding) {
	definitions := make([]map[string]any, 0)
	bindings := make(map[string]apiDefaultToolBinding)
	familyCounts := manifestFamilyCounts(manifest)
	add := func(binding apiDefaultToolBinding, description string, schema map[string]any, readOnly, destructive, confirmation, idempotency bool, metadata map[string]any) {
		if _, collision := bindings[binding.Name]; collision {
			delete(bindings, binding.Name)
			for index := 0; index < len(definitions); index++ {
				if definitions[index]["name"] == binding.Name {
					definitions = append(definitions[:index], definitions[index+1:]...)
					break
				}
			}
			return
		}
		binding.InputSchema = schemaJSON(schema)
		bindings[binding.Name] = binding
		definitions = append(definitions, generatedToolDefinition(binding, description, schema, readOnly, destructive, confirmation, idempotency, metadata))
	}

	for _, integration := range manifest.Integrations {
		if familyCounts[integration.FamilyKey] != 1 || !validGeneratedToolSegment(integration.FamilyKey) || !integrationHasKnowledge(integration) {
			continue
		}
		binding := apiDefaultToolBinding{Name: integration.FamilyKey + ".knowledge.search", Kind: apiKnowledgeSearch, IntegrationID: integration.ID, FamilyKey: integration.FamilyKey}
		add(binding, "Search only the reviewed documentation pinned by this published API Integration revision.", apiToolSchema(map[string]any{"query": map[string]any{"type": "string", "minLength": 1, "maxLength": 2000}}, "query"), true, false, false, false, nil)
	}

	if public || s.accessRuntime == nil {
		sort.Slice(definitions, func(i, j int) bool { return definitions[i]["name"].(string) < definitions[j]["name"].(string) })
		return definitions, bindings
	}
	capabilities := s.accessRuntime.Capabilities(ctx, productID, principal.Grants)
	capabilityByConnection := make(map[string]accessruntime.Capability, len(capabilities))
	for _, capability := range capabilities {
		capabilityByConnection[capability.ConnectionID] = capability
	}
	for _, integration := range manifest.Integrations {
		if familyCounts[integration.FamilyKey] != 1 || !validGeneratedToolSegment(integration.FamilyKey) {
			continue
		}
		candidates := make([]apiAdminCandidate, 0, len(integration.AccessConnections))
		seenConnections := make(map[string]bool)
		for _, published := range integration.AccessConnections {
			capability, ok := capabilityByConnection[published.ConnectionID]
			if !ok || seenConnections[published.ConnectionID] || !containsString(capability.IntegrationIDs, integration.ID) || published.State != "active" {
				continue
			}
			connection, err := s.service.Store().AccessConnection(ctx, productID, published.ConnectionID)
			if err != nil || connection.State != "active" || connection.Revision != published.ConnectionRevision || connection.AccessDefinitionID != published.AccessDefinitionID || connection.EnvironmentID != published.EnvironmentID || connection.Definition == nil || connection.Definition.State != "active" || connection.Definition.ID != published.AccessDefinitionID || connection.Definition.Revision != published.AccessDefinitionRevision || capability.DefinitionID != published.AccessDefinitionID {
				continue
			}
			seenConnections[published.ConnectionID] = true
			candidates = append(candidates, apiAdminCandidate{integration: integration, manifest: published, connection: connection, capability: capability})
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].capability.ServiceKey == candidates[j].capability.ServiceKey {
				return candidates[i].connection.ID < candidates[j].connection.ID
			}
			return candidates[i].capability.ServiceKey < candidates[j].capability.ServiceKey
		})
		serviceCounts := make(map[string]int)
		for _, candidate := range candidates {
			serviceCounts[candidate.capability.ServiceKey]++
		}
		for _, candidate := range candidates {
			prefix := integration.FamilyKey + ".admin"
			if len(candidates) > 1 {
				if serviceCounts[candidate.capability.ServiceKey] != 1 || !validGeneratedToolSegment(candidate.capability.ServiceKey) {
					continue
				}
				prefix += "." + candidate.capability.ServiceKey
			}
			shared := len(candidate.capability.IntegrationIDs) > 1
			base := apiDefaultToolBinding{OrganisationID: candidate.connection.OrganisationID, IntegrationID: integration.ID, FamilyKey: integration.FamilyKey, ConnectionID: candidate.connection.ID, ConnectionRevision: candidate.manifest.ConnectionRevision, AccessDefinitionID: candidate.manifest.AccessDefinitionID, AccessDefinitionRevision: candidate.manifest.AccessDefinitionRevision, EnvironmentVariable: platform.RuntimeEnvironmentVariableForFamily(integration.FamilyKey, shared)}
			metadata := map[string]any{"com.dokosoko/accessConnectionId": candidate.connection.ID, "com.dokosoko/accessDefinitionId": candidate.manifest.AccessDefinitionID, "com.dokosoko/serviceKey": candidate.capability.ServiceKey, "com.dokosoko/instanceLabel": candidate.capability.InstanceLabel, "com.dokosoko/environmentVariable": base.EnvironmentVariable}

			binding := base
			binding.Name, binding.Kind = prefix+".instances.list", apiInstancesList
			add(binding, "List provider resources owned by the authenticated subject for this API and management connection.", apiToolSchema(map[string]any{}), true, false, false, false, metadata)
			if candidate.capability.CanCreateInstance {
				binding = base
				binding.Name, binding.Kind = prefix+".instances.create", apiInstancesCreate
				add(binding, "Create a provider resource for this API. This mutation requires a server-issued one-time confirmation challenge and a stable params._meta.idempotency_key.", apiToolSchema(map[string]any{"environment_id": map[string]any{"type": "string"}, "display_name": map[string]any{"type": "string", "minLength": 1, "maxLength": 160}, "ttl_seconds": map[string]any{"type": "integer", "minimum": 300}}, "environment_id", "display_name"), false, false, true, true, metadata)
			}
			binding = base
			binding.Name, binding.Kind = prefix+".credentials.list", apiCredentialsList
			add(binding, "List credential metadata and fingerprints for this API connection or owned provider resource. Credential material is never returned by list operations.", apiToolSchema(map[string]any{"access_instance_id": map[string]any{"type": "string"}}), true, false, false, false, metadata)
			if candidate.capability.CanCreateCredential {
				binding = base
				binding.Name, binding.Kind = prefix+".credentials.rotate", apiCredentialsRotate
				add(binding, "Issue the first credential, or issue replacement credential material while retaining a supplied prior credential for safe overlap. This mutation returns material once and requires a server-issued one-time confirmation challenge plus stable params._meta.idempotency_key; after a rotation, revoke the prior credential separately after cutover.", apiToolSchema(map[string]any{"environment_id": map[string]any{"type": "string"}, "access_instance_id": map[string]any{"type": "string"}, "rotated_from_credential_id": map[string]any{"type": "string", "minLength": 1}, "scopes": map[string]any{"type": "array", "maxItems": 20, "items": map[string]any{"type": "string"}}, "ttl_seconds": map[string]any{"type": "integer", "minimum": 300}}, "environment_id", "scopes"), false, false, true, true, metadata)
			}
			if candidate.capability.CanRevokeCredential {
				binding = base
				binding.Name, binding.Kind = prefix+".credentials.revoke", apiCredentialsRevoke
				add(binding, "Revoke one credential owned by the authenticated subject for this exact API management connection. This is a separate mutation from rotation and requires a server-issued one-time confirmation challenge.", apiToolSchema(map[string]any{"credential_id": map[string]any{"type": "string", "minLength": 1}}, "credential_id"), false, true, true, false, metadata)
			}
		}
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i]["name"].(string) < definitions[j]["name"].(string) })
	return definitions, bindings
}

type apiOperationConfirmationHashInput struct {
	ProductID          string         `json:"product_id"`
	OperationKey       string         `json:"operation_key"`
	Issuer             string         `json:"issuer"`
	Subject            string         `json:"subject"`
	CustomerAccountID  string         `json:"customer_account_id"`
	InstallationID     string         `json:"installation_id"`
	AccessEvaluationID string         `json:"access_evaluation_id"`
	AccessEvaluatedAt  string         `json:"access_evaluated_at"`
	IdempotencyKey     string         `json:"idempotency_key"`
	Arguments          map[string]any `json:"arguments"`
}

func (binding apiDefaultToolBinding) confirmationOperationKey() string {
	canonical := fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%s\x00%d", binding.Name, binding.IntegrationID, binding.ConnectionID, binding.ConnectionRevision, binding.AccessDefinitionID, binding.AccessDefinitionRevision)
	digest := sha256.Sum256([]byte(canonical))
	return "api-admin-v1:" + hex.EncodeToString(digest[:])
}

func apiOperationConfirmationArgumentHash(productID string, binding apiDefaultToolBinding, principal identity.Principal, arguments map[string]any, idempotencyKey string) ([]byte, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	payload, err := json.Marshal(apiOperationConfirmationHashInput{ProductID: productID, OperationKey: binding.confirmationOperationKey(), Issuer: principal.Issuer, Subject: principal.Subject, CustomerAccountID: principal.CustomerAccountID, InstallationID: principal.InstallationID, AccessEvaluationID: principal.AccessEvaluationID, AccessEvaluatedAt: principal.AccessEvaluatedAt.UTC().Format(time.RFC3339Nano), IdempotencyKey: idempotencyKey, Arguments: arguments})
	if err != nil {
		return nil, errors.New("generated API tool arguments are not canonical JSON")
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte("dokosoko-api-admin-confirmation-v1"))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(payload)
	return digest.Sum(nil), nil
}

func (s *Server) issueAPIOperationConfirmation(ctx context.Context, productID string, binding apiDefaultToolBinding, principal identity.Principal, arguments map[string]any, idempotencyKey string, now time.Time) (managedToolConfirmationChallenge, error) {
	actorID, err := managedToolConfirmationActor(principal)
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	argumentHash, err := apiOperationConfirmationArgumentHash(productID, binding, principal, arguments, idempotencyKey)
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	nonce, nonceDigest, err := randomManagedToolConfirmationNonce()
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	id, err := randomManagedToolConfirmationUUID()
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	expiresAt := now.Add(managedToolConfirmationTTL)
	value := model.ManagedOperationConfirmation{ID: id, OrganisationID: binding.OrganisationID, ProductID: productID, OperationKey: binding.confirmationOperationKey(), ArgumentHash: argumentHash, NonceDigest: nonceDigest, ActorID: actorID, ExpiresAt: expiresAt, CreatedAt: now}
	if err := s.service.Store().CreateManagedOperationConfirmation(ctx, value); err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	return managedToolConfirmationChallenge{Nonce: nonce, ExpiresAt: expiresAt}, nil
}

func (s *Server) consumeAPIOperationConfirmation(ctx context.Context, challenge, productID string, binding apiDefaultToolBinding, principal identity.Principal, arguments map[string]any, idempotencyKey string, now time.Time) error {
	if len(challenge) != len("mtc_")+base64.RawURLEncoding.EncodedLen(managedToolConfirmationNonceBytes) || !strings.HasPrefix(challenge, "mtc_") {
		return errors.New("generated API tool confirmation challenge is malformed")
	}
	actorID, err := managedToolConfirmationActor(principal)
	if err != nil {
		return err
	}
	argumentHash, err := apiOperationConfirmationArgumentHash(productID, binding, principal, arguments, idempotencyKey)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(challenge))
	consumptionID, err := randomManagedToolConfirmationUUID()
	if err != nil {
		return err
	}
	_, err = s.service.Store().ConsumeManagedOperationConfirmation(ctx, digest[:], productID, binding.confirmationOperationKey(), argumentHash, actorID, consumptionID, now)
	return err
}

func writeAPIOperationConfirmationRequired(w http.ResponseWriter, id any, challenge managedToolConfirmationChallenge, binding apiDefaultToolBinding) {
	writeRPCErrorData(w, id, -32003, "A server-issued confirmation challenge is required for this exact API administration operation", map[string]any{
		"confirmation_required":          true,
		"confirmation_challenge":         challenge.Nonce,
		"expires_at":                     challenge.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"retry_metadata_field":           "params._meta." + managedToolConfirmationMetaField,
		"confirmation_attestation_field": "params._meta.confirmed",
		"confirmation_attestation_value": true,
		"operation":                      binding.Name,
		"notice":                         "Retry the exact same invocation with the challenge and confirmed=true. The challenge is bound to this actor, access evaluation, API, connection revision, arguments, and idempotency key, and can be consumed only once.",
	})
}

func apiMutationRequiresConfirmation(kind apiDefaultToolKind) bool {
	return kind == apiInstancesCreate || kind == apiCredentialsRotate || kind == apiCredentialsRevoke
}

func apiMutationRequiresIdempotency(kind apiDefaultToolKind) bool {
	return kind == apiInstancesCreate || kind == apiCredentialsRotate
}

func (s *Server) executeKnowledgeSearch(ctx context.Context, w http.ResponseWriter, requestID any, productID, integrationID string, query string, public bool, manifest model.ProductManifest, selection model.ProductSelectionContext, principal identity.Principal) {
	publicationIDs, scopeErr := s.knowledgePublicationIDs(ctx, productID, integrationID, manifest)
	if scopeErr != nil {
		writeRPCError(w, requestID, -32003, "Select exactly one published Integration with reviewed documentation")
		return
	}
	if strings.TrimSpace(query) == "" {
		writeRPCError(w, requestID, -32602, "A knowledge query is required")
		return
	}
	var items []model.KnowledgeRecord
	var err error
	if public {
		items, err = s.service.Store().PublicKnowledge(ctx, productID, publicationIDs, query)
	} else {
		if principal.ProductID != productID {
			writeRPCError(w, requestID, -32003, "Knowledge access was denied by product policy")
			return
		}
		items, err = s.service.Store().PrivateKnowledge(ctx, productID, publicationIDs, query)
	}
	if err != nil {
		writeRPCError(w, requestID, -32603, "Search failed")
		return
	}
	filtered := make([]model.KnowledgeRecord, 0, len(items))
	for _, item := range items {
		allowed := false
		for _, kind := range []string{"docs", "openapi", "git"} {
			managed, candidateAllowed, allowErr := s.service.ProductVersionAllowsArtifactFor(ctx, productID, selection, kind, item.SourceID, "", "")
			if allowErr == nil && (!managed || candidateAllowed) {
				allowed = true
				break
			}
		}
		if allowed {
			filtered = append(filtered, item)
		}
	}
	writeToolResult(w, requestID, filtered)
}

func (s *Server) executeAPIDefaultTool(ctx context.Context, w http.ResponseWriter, request rpcRequest, params toolCallParams, productID string, binding apiDefaultToolBinding, public bool, manifest model.ProductManifest, selection model.ProductSelectionContext, principal identity.Principal) {
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}
	if err := toolruntime.ValidateArguments(binding.InputSchema, params.Arguments); err != nil {
		writeRPCError(w, request.ID, -32602, "Tool arguments do not match the declared input schema")
		return
	}
	if binding.Kind == apiKnowledgeSearch {
		query, _ := params.Arguments["query"].(string)
		s.executeKnowledgeSearch(ctx, w, request.ID, productID, binding.IntegrationID, query, public, manifest, selection, principal)
		return
	}
	if public || s.accessRuntime == nil {
		writeRPCError(w, request.ID, -32601, "Tool is not available")
		return
	}
	if apiMutationRequiresIdempotency(binding.Kind) && !toolruntime.ValidIdempotencyKey(params.Meta.IdempotencyKey) {
		writeRPCError(w, request.ID, -32602, "params._meta.idempotency_key must contain 16 to 200 visible ASCII characters")
		return
	}
	if apiMutationRequiresConfirmation(binding.Kind) {
		now := time.Now().UTC()
		if strings.TrimSpace(params.Meta.ConfirmationChallenge) == "" {
			challenge, err := s.issueAPIOperationConfirmation(ctx, productID, binding, principal, params.Arguments, params.Meta.IdempotencyKey, now)
			if err != nil {
				slog.Error("API Admin confirmation challenge could not be persisted", "error", err, "product_id", productID, "operation", binding.Name)
				writeRPCError(w, request.ID, -32603, "A confirmation challenge could not be issued safely")
				return
			}
			writeAPIOperationConfirmationRequired(w, request.ID, challenge, binding)
			return
		}
		if !params.Meta.Confirmed {
			writeRPCErrorData(w, request.ID, -32003, "confirmed=true is required with the server-issued challenge", map[string]any{"confirmation_required": true, "confirmation_attestation_field": "params._meta.confirmed", "confirmation_attestation_value": true})
			return
		}
		if err := s.consumeAPIOperationConfirmation(ctx, params.Meta.ConfirmationChallenge, productID, binding, principal, params.Arguments, params.Meta.IdempotencyKey, now); err != nil {
			writeRPCErrorData(w, request.ID, -32003, "The confirmation challenge is invalid, expired, already used, or does not match this exact invocation", map[string]any{"confirmation_required": true, "retry_without_challenge_to_request_a_new_one": true})
			return
		}
	}
	accessPrincipal := accessPrincipal(principal, ctx)
	switch binding.Kind {
	case apiInstancesList:
		values, err := s.accessRuntime.ListInstances(ctx, productID, binding.ConnectionID, binding.IntegrationID, accessPrincipal)
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		writeToolResult(w, request.ID, map[string]any{"instances": values})
	case apiInstancesCreate:
		var input accessruntime.InstanceRequest
		if decodeArguments(params.Arguments, &input) != nil {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		input.IntegrationID, input.IdempotencyKey = binding.IntegrationID, params.Meta.IdempotencyKey
		value, err := s.accessRuntime.CreateInstance(ctx, productID, binding.ConnectionID, input, accessPrincipal)
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		writeToolResult(w, request.ID, value)
	case apiCredentialsList:
		instanceID, _ := params.Arguments["access_instance_id"].(string)
		values, err := s.accessRuntime.ListCredentials(ctx, productID, binding.ConnectionID, binding.IntegrationID, instanceID, accessPrincipal)
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		writeToolResult(w, request.ID, map[string]any{"credentials": values})
	case apiCredentialsRotate:
		var input accessruntime.CredentialRequest
		if decodeArguments(params.Arguments, &input) != nil {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		input.IntegrationID, input.IdempotencyKey = binding.IntegrationID, params.Meta.IdempotencyKey
		value, err := s.accessRuntime.IssueCredential(ctx, productID, binding.ConnectionID, input, accessPrincipal)
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		value.EnvironmentVariable = binding.EnvironmentVariable
		writeToolResult(w, request.ID, value)
	case apiCredentialsRevoke:
		credentialID, _ := params.Arguments["credential_id"].(string)
		value, err := s.accessRuntime.RevokeCredentialBound(ctx, productID, binding.ConnectionID, binding.IntegrationID, credentialID, accessPrincipal)
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		writeToolResult(w, request.ID, value)
	default:
		writeRPCError(w, request.ID, -32601, "Tool not found")
	}
}

func canonicalCustomToolName(manifest model.ProductManifest, tool model.Tool) (string, bool) {
	switch tool.Scope {
	case "":
		if !validGeneratedToolSegment(tool.Namespace) || !validGeneratedToolSegment(tool.Name) {
			return "", false
		}
		return tool.Namespace + "." + tool.Name, true
	case model.ToolScopeCommon:
		if !validGeneratedToolSegment(tool.Name) {
			return "", false
		}
		return "common." + tool.Name, true
	case model.ToolScopeAPI:
		counts := manifestFamilyCounts(manifest)
		for _, integration := range manifest.Integrations {
			if integration.ID == tool.OwnerIntegrationID && counts[integration.FamilyKey] == 1 && validGeneratedToolSegment(integration.FamilyKey) && validGeneratedToolSegment(tool.Name) {
				return integration.FamilyKey + ".custom." + tool.Name, true
			}
		}
	}
	return "", false
}
