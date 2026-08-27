package tools

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

const (
	authorizationHookTimeout   = 5 * time.Second
	authorizationHookBodyLimit = 32 << 10
)

type accessEvaluationRequest struct {
	Version         string `json:"version"`
	RequestID       string `json:"request_id,omitempty"`
	AuthorizationID string `json:"authorization_id"`
	APIID           string `json:"api_id"`
	ToolID          string `json:"tool_id"`
	ToolRevision    int64  `json:"tool_revision"`
	Tool            string `json:"tool"`
	Method          string `json:"method"`
	Path            string `json:"path"`
	Subject         string `json:"subject"`
	CustomerID      string `json:"customer_id,omitempty"`
	InstallationID  string `json:"installation_id,omitempty"`
	EnvironmentID   string `json:"environment_id,omitempty"`
	RequestedAt     string `json:"requested_at"`
}

type accessEvaluationResponse struct {
	Allow      bool   `json:"allow"`
	DecisionID string `json:"decision_id"`
}

type authorizationUsagePayload struct {
	Version         string `json:"version"`
	EventID         string `json:"event_id"`
	RequestID       string `json:"request_id,omitempty"`
	DecisionID      string `json:"decision_id"`
	AuthorizationID string `json:"authorization_id"`
	APIID           string `json:"api_id"`
	ToolID          string `json:"tool_id"`
	ToolRevision    int64  `json:"tool_revision"`
	Tool            string `json:"tool"`
	Subject         string `json:"subject"`
	CustomerID      string `json:"customer_id,omitempty"`
	InstallationID  string `json:"installation_id,omitempty"`
	EnvironmentID   string `json:"environment_id,omitempty"`
	Outcome         string `json:"outcome"`
	OccurredAt      string `json:"occurred_at"`
}

func authorizationUsageID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return ""
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

func (r *Runtime) authorizeHookRequest(ctx context.Context, tool model.Tool, principal Principal, request *http.Request) error {
	auth, err := toolUpstreamAuth(tool)
	if err != nil {
		return ErrDenied
	}
	var credentialMaterial toolCredentialMaterial
	if auth.Type != "delegated_oauth" && auth.Type != "none" {
		credentialMaterial, err = r.toolCredentialMaterial(ctx, tool)
		if err != nil {
			return ErrDenied
		}
		defer wipeCredentialMaterial(&credentialMaterial)
	}
	switch auth.Type {
	case "bearer":
		request.Header.Set("Authorization", "Bearer "+string(credentialMaterial.primary))
	case "api_key_header", "custom_header":
		request.Header.Set(auth.HeaderName, prefixedCredential(auth.Prefix, credentialMaterial.primary))
	case "basic":
		request.SetBasicAuth(auth.Username, string(credentialMaterial.primary))
	case "oauth_client_credentials":
		tokenType, token, tokenErr := r.oauthClientTokenTraced(ctx, tool, auth, nil)
		if tokenErr != nil {
			return ErrDenied
		}
		request.Header.Set("Authorization", tokenType+" "+token)
	case "authorization_scheme":
		request.Header.Set("Authorization", auth.Scheme+" "+string(credentialMaterial.primary))
	case "delegated_oauth":
		if principal.DelegatedAccessToken == "" {
			return ErrDenied
		}
		request.Header.Set("Authorization", "Bearer "+principal.DelegatedAccessToken)
	case "none":
		return ErrDenied
	default:
		return ErrDenied
	}
	if auth.Type != "delegated_oauth" && auth.Type != "none" {
		if err := applyAdditionalAuthorizationHeaders(request.Header, auth.Headers, credentialMaterial.headers); err != nil {
			return ErrDenied
		}
	}
	return nil
}

func (r *Runtime) evaluateAuthorization(ctx context.Context, productID, fullName string, tool model.Tool, principal Principal, binding BoundAuthorization) (string, error) {
	if tool.RuntimeCredentialSetID == "" || strings.TrimSpace(tool.AccessEvaluationURL) == "" {
		return "", ErrDenied
	}
	payload, err := json.Marshal(accessEvaluationRequest{
		Version: "2026-08-01", RequestID: principal.RequestID,
		AuthorizationID: tool.RuntimeCredentialSetID, APIID: binding.IntegrationID,
		ToolID: tool.ID, ToolRevision: tool.Revision, Tool: fullName,
		Method: strings.ToUpper(tool.HTTPMethod), Path: tool.HTTPPath,
		Subject: principal.Subject, CustomerID: principal.CustomerAccountID,
		InstallationID: principal.InstallationID, EnvironmentID: principal.EnvironmentID,
		RequestedAt: r.now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return "", ErrDenied
	}
	hookCtx, cancel := context.WithTimeout(ctx, authorizationHookTimeout)
	defer cancel()
	parsed, address, err := r.safeDestination(hookCtx, tool.AccessEvaluationURL)
	if err != nil {
		return "", ErrDenied
	}
	request, err := http.NewRequestWithContext(hookCtx, http.MethodPost, parsed.String(), bytes.NewReader(payload))
	if err != nil {
		return "", ErrDenied
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	if err := r.authorizeHookRequest(hookCtx, tool, principal, request); err != nil {
		return "", ErrDenied
	}
	response, err := r.client(parsed, address, authorizationHookTimeout).Do(request)
	if err != nil {
		return "", ErrDenied
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", ErrDenied
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, authorizationHookBodyLimit+1))
	if err != nil || len(body) > authorizationHookBodyLimit {
		return "", ErrDenied
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var decision accessEvaluationResponse
	if err := decoder.Decode(&decision); err != nil || !decision.Allow || strings.TrimSpace(decision.DecisionID) == "" || len(decision.DecisionID) > 200 {
		return "", ErrDenied
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", ErrDenied
	}
	decision.DecisionID = strings.TrimSpace(decision.DecisionID)
	auditCtx, auditCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer auditCancel()
	if r.store == nil || r.store.AppendAudit(auditCtx, model.AuditEvent{ID: auditID(), OrganisationID: tool.OrganisationID, ProductID: productID, ActorID: principal.Subject, Action: "authorization.access_evaluated", TargetType: "tool", TargetID: tool.ID, Current: map[string]any{"authorization_id": tool.RuntimeCredentialSetID, "api_id": binding.IntegrationID, "decision_id": decision.DecisionID}, RequestID: principal.RequestID, Outcome: "allowed", CreatedAt: r.now()}) != nil {
		return "", ErrDenied
	}
	return decision.DecisionID, nil
}

func (r *Runtime) enqueueAuthorizationUsage(ctx context.Context, fullName, decisionID string, tool model.Tool, principal Principal, binding BoundAuthorization) {
	if r.store == nil || tool.RuntimeCredentialSetID == "" || tool.UsageURL == "" || tool.RuntimeCredentialVersionID == "" || tool.CredentialID == "" {
		return
	}
	eventID := authorizationUsageID()
	if eventID == "" {
		return
	}
	now := r.now().UTC()
	payload, err := json.Marshal(authorizationUsagePayload{
		Version: "2026-08-01", EventID: eventID, RequestID: principal.RequestID, DecisionID: decisionID,
		AuthorizationID: tool.RuntimeCredentialSetID, APIID: binding.IntegrationID,
		ToolID: tool.ID, ToolRevision: tool.Revision, Tool: fullName,
		Subject: principal.Subject, CustomerID: principal.CustomerAccountID,
		InstallationID: principal.InstallationID, EnvironmentID: principal.EnvironmentID,
		Outcome: "success", OccurredAt: now.Format(time.RFC3339Nano),
	})
	if err != nil {
		return
	}
	queueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_, _ = r.store.CreateAuthorizationUsageEvent(queueCtx, model.AuthorizationUsageEvent{
		ID: eventID, OrganisationID: tool.OrganisationID, ProductID: tool.ProductID,
		IntegrationID: binding.IntegrationID, AuthorizationID: tool.RuntimeCredentialSetID,
		URL: tool.UsageURL, AuthenticationType: strings.TrimSpace(toolUpstreamAuthType(tool)),
		HeaderName: hookHeaderName(tool), AuthConfig: hookAuthConfig(tool),
		CredentialVersionID: tool.RuntimeCredentialVersionID, CredentialSecretID: tool.CredentialID,
		CredentialFingerprint: tool.CredentialFingerprint, Payload: payload, AvailableAt: now,
	})
}

func toolUpstreamAuthType(tool model.Tool) string {
	auth, err := toolUpstreamAuth(tool)
	if err != nil {
		return ""
	}
	return auth.Type
}

func hookHeaderName(tool model.Tool) string {
	auth, err := toolUpstreamAuth(tool)
	if err != nil {
		return ""
	}
	return auth.HeaderName
}

func hookAuthConfig(tool model.Tool) json.RawMessage {
	var config map[string]any
	if json.Unmarshal(tool.UpstreamAuth, &config) != nil {
		return json.RawMessage(`{}`)
	}
	delete(config, "type")
	delete(config, "header_name")
	value, err := json.Marshal(config)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return value
}

func usageDeliveryTool(event model.AuthorizationUsageEvent) (model.Tool, error) {
	config := map[string]any{}
	if len(event.AuthConfig) > 0 && json.Unmarshal(event.AuthConfig, &config) != nil {
		return model.Tool{}, ErrDenied
	}
	config["type"] = event.AuthenticationType
	if event.HeaderName != "" {
		config["header_name"] = event.HeaderName
	}
	auth, err := json.Marshal(config)
	if err != nil {
		return model.Tool{}, ErrDenied
	}
	return model.Tool{
		OrganisationID: event.OrganisationID, ProductID: event.ProductID,
		RuntimeServiceConnectionID: "authorization-usage-delivery",
		RuntimeCredentialSetID:     event.AuthorizationID, RuntimeCredentialVersionID: event.CredentialVersionID,
		CredentialID: event.CredentialSecretID, CredentialFingerprint: event.CredentialFingerprint,
		UpstreamAuth: auth, TimeoutMS: int(authorizationHookTimeout / time.Millisecond),
	}, nil
}

func (r *Runtime) deliverAuthorizationUsage(ctx context.Context, event model.AuthorizationUsageEvent) error {
	parsed, address, err := r.safeDestination(ctx, event.URL)
	if err != nil {
		return errors.New("unsafe destination")
	}
	tool, err := usageDeliveryTool(event)
	if err != nil {
		return errors.New("invalid authorization configuration")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(event.Payload))
	if err != nil {
		return errors.New("invalid request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", event.ID)
	if err := r.authorizeHookRequest(ctx, tool, Principal{}, request); err != nil {
		return errors.New("authorization unavailable")
	}
	response, err := r.client(parsed, address, authorizationHookTimeout).Do(request)
	if err != nil {
		return errors.New("transport failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, authorizationHookBodyLimit))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	return nil
}

func authorizationUsageRetryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Minute
	for index := 1; index < attempts && delay < time.Hour; index++ {
		delay *= 2
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

// RunAuthorizationUsageDelivery claims durable usage events and delivers them
// with bounded requests, no redirects, pinned DNS resolution, and retries.
func (r *Runtime) RunAuthorizationUsageDelivery(ctx context.Context, interval time.Duration) error {
	if r.store == nil || r.credentials == nil {
		return errors.New("authorization usage delivery is not configured")
	}
	if interval <= 0 {
		interval = time.Second
	}
	owner := authorizationUsageID()
	if owner == "" {
		return errors.New("authorization usage delivery owner could not be created")
	}
	for {
		now := r.now().UTC()
		events, err := r.store.ClaimAuthorizationUsageEvents(ctx, owner, now.Add(30*time.Second), 25)
		if err != nil {
			return err
		}
		for _, event := range events {
			deliveryCtx, cancel := context.WithTimeout(ctx, authorizationHookTimeout)
			deliveryErr := r.deliverAuthorizationUsage(deliveryCtx, event)
			cancel()
			if deliveryErr == nil {
				if err := r.store.CompleteAuthorizationUsageEvent(ctx, event.ID, owner, r.now().UTC()); err != nil {
					return err
				}
				continue
			}
			if err := r.store.RetryAuthorizationUsageEvent(ctx, event.ID, owner, r.now().Add(authorizationUsageRetryDelay(event.Attempts)), deliveryErr.Error()); err != nil {
				return err
			}
		}
		if len(events) == 25 {
			continue
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
