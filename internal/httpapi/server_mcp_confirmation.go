package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

type managedToolConfirmationChallenge struct {
	Nonce     string
	ExpiresAt time.Time
}

type managedToolConfirmationHashInput struct {
	ProductID                  string         `json:"product_id"`
	ToolID                     string         `json:"tool_id"`
	ToolRevision               int64          `json:"tool_revision"`
	IntegrationID              string         `json:"integration_id"`
	AuthorizationPointID       string         `json:"authorization_point_id"`
	AuthorizationPointRevision int64          `json:"authorization_point_revision"`
	Issuer                     string         `json:"issuer"`
	Subject                    string         `json:"subject"`
	CustomerAccountID          string         `json:"customer_account_id"`
	InstallationID             string         `json:"installation_id"`
	AccessEvaluationID         string         `json:"access_evaluation_id"`
	AccessEvaluatedAt          string         `json:"access_evaluated_at"`
	IdempotencyKey             string         `json:"idempotency_key"`
	Arguments                  map[string]any `json:"arguments"`
}

func managedToolConfirmationArgumentHash(productID string, tool model.Tool, binding toolruntime.BoundAuthorization, principal identity.Principal, arguments map[string]any, idempotencyKey string) ([]byte, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	payload, err := json.Marshal(managedToolConfirmationHashInput{
		ProductID:                  productID,
		ToolID:                     tool.ID,
		ToolRevision:               tool.Revision,
		IntegrationID:              binding.IntegrationID,
		AuthorizationPointID:       binding.AuthorizationPoint.ID,
		AuthorizationPointRevision: binding.AuthorizationPointRevision,
		Issuer:                     principal.Issuer,
		Subject:                    principal.Subject,
		CustomerAccountID:          principal.CustomerAccountID,
		InstallationID:             principal.InstallationID,
		AccessEvaluationID:         principal.AccessEvaluationID,
		AccessEvaluatedAt:          principal.AccessEvaluatedAt.UTC().Format(time.RFC3339Nano),
		IdempotencyKey:             idempotencyKey,
		Arguments:                  arguments,
	})
	if err != nil {
		return nil, errors.New("managed tool arguments are not canonical JSON")
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(managedToolConfirmationDomain))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(payload)
	return digest.Sum(nil), nil
}

func randomManagedToolConfirmationUUID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func randomManagedToolConfirmationNonce() (string, []byte, error) {
	raw := make([]byte, managedToolConfirmationNonceBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	nonce := "mtc_" + base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(nonce))
	return nonce, digest[:], nil
}

func managedToolConfirmationActor(principal identity.Principal) (string, error) {
	actorID := vendorActorID(principal)
	if actorID == "" || strings.TrimSpace(principal.AccessEvaluationID) == "" || principal.AccessEvaluatedAt.IsZero() {
		return "", errors.New("managed tool confirmation requires an exact authenticated access evaluation")
	}
	return actorID, nil
}

func (s *Server) issueManagedToolConfirmation(ctx context.Context, productID string, tool model.Tool, binding toolruntime.BoundAuthorization, principal identity.Principal, arguments map[string]any, idempotencyKey string, now time.Time) (managedToolConfirmationChallenge, error) {
	actorID, err := managedToolConfirmationActor(principal)
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	argumentHash, err := managedToolConfirmationArgumentHash(productID, tool, binding, principal, arguments, idempotencyKey)
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
	decisionExpiresAt := principal.AccessEvaluatedAt.Add(time.Duration(binding.AuthorizationPoint.DecisionTTLSeconds) * time.Second)
	if decisionExpiresAt.Before(expiresAt) {
		expiresAt = decisionExpiresAt
	}
	if !now.Before(expiresAt) {
		return managedToolConfirmationChallenge{}, errors.New("the access evaluation expires before confirmation can be issued")
	}
	confirmation := model.ToolTestConfirmation{
		ID:             id,
		OrganisationID: tool.OrganisationID,
		ProductID:      productID,
		ToolID:         tool.ID,
		ToolRevision:   tool.Revision,
		ArgumentHash:   argumentHash,
		NonceDigest:    nonceDigest,
		ActorID:        actorID,
		ExpiresAt:      expiresAt,
		CreatedAt:      now,
	}
	if err := s.service.Store().CreateToolTestConfirmation(ctx, confirmation); err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	return managedToolConfirmationChallenge{Nonce: nonce, ExpiresAt: expiresAt}, nil
}

func (s *Server) consumeManagedToolConfirmation(ctx context.Context, challenge, productID string, tool model.Tool, binding toolruntime.BoundAuthorization, principal identity.Principal, arguments map[string]any, idempotencyKey string, now time.Time) error {
	if len(challenge) != len("mtc_")+base64.RawURLEncoding.EncodedLen(managedToolConfirmationNonceBytes) || !strings.HasPrefix(challenge, "mtc_") {
		return errors.New("managed tool confirmation challenge is malformed")
	}
	actorID, err := managedToolConfirmationActor(principal)
	if err != nil {
		return err
	}
	argumentHash, err := managedToolConfirmationArgumentHash(productID, tool, binding, principal, arguments, idempotencyKey)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(challenge))
	consumptionID, err := randomManagedToolConfirmationUUID()
	if err != nil {
		return err
	}
	_, err = s.service.Store().ConsumeToolTestConfirmation(ctx, digest[:], productID, tool.ID, tool.Revision, argumentHash, actorID, consumptionID, now)
	return err
}

func managedToolPolicy(tool model.Tool, binding toolruntime.BoundAuthorization) (confirmationRequired, idempotencyRequired bool, err error) {
	var policy struct {
		ConfirmationRequired bool `json:"confirmation_required"`
		IdempotencyRequired  bool `json:"idempotency_required"`
	}
	if err := json.Unmarshal(tool.AuthorizationPolicy, &policy); err != nil {
		return false, false, err
	}
	return policy.ConfirmationRequired || binding.AuthorizationPoint.ConfirmationRequired, strings.ToUpper(strings.TrimSpace(tool.HTTPMethod)) != http.MethodGet && policy.IdempotencyRequired, nil
}

func writeManagedToolConfirmationRequired(w http.ResponseWriter, id any, challenge managedToolConfirmationChallenge, tool model.Tool, binding toolruntime.BoundAuthorization) {
	writeRPCErrorData(w, id, -32003, "Client confirmation attestation is required for this exact managed tool invocation", map[string]any{
		"confirmation_required":          true,
		"confirmation_challenge":         challenge.Nonce,
		"expires_at":                     challenge.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"retry_metadata_field":           "params._meta." + managedToolConfirmationMetaField,
		"confirmation_attestation_field": "params._meta.confirmed",
		"confirmation_attestation_value": true,
		"tool_id":                        tool.ID,
		"tool_revision":                  tool.Revision,
		"authorization_point_id":         binding.AuthorizationPoint.ID,
		"authorization_point_revision":   binding.AuthorizationPointRevision,
		"notice":                         "Retrying with the challenge and confirmed=true is the client's attestation that it obtained confirmation; the server does not independently prove that a human approved.",
	})
}
