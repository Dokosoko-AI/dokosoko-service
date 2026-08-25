package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Service) ProcessPending(ctx context.Context, limit int) (int, error) {
	values, err := s.store.ClaimReportSubmissions(ctx, s.now(), limit)
	if err != nil {
		return 0, err
	}
	for index, value := range values {
		if err := s.deliver(ctx, value); err != nil {
			return index, fmt.Errorf("persist report delivery state for %s: %w", value.ID, err)
		}
	}
	return len(values), nil
}

func (s *Service) deliver(ctx context.Context, value model.ReportSubmission) error {
	endpoint, credentialID, err := s.deliveryRoute(ctx, value)
	if err != nil {
		if errors.Is(err, ErrDeliveryUnavailable) || errors.Is(err, ErrNotConfigured) || errors.Is(err, store.ErrNotFound) {
			value.State, value.NextAttemptAt, value.DeliveryStartedAt, value.LastError = "held", nil, nil, ""
			_, updateErr := s.store.UpdateReportSubmissionDelivery(ctx, value)
			return updateErr
		}
		return s.deliveryFailed(ctx, value, err)
	}
	if endpoint == "" || credentialID == "" {
		value.State, value.NextAttemptAt, value.DeliveryStartedAt, value.LastError = "held", nil, nil, ""
		_, updateErr := s.store.UpdateReportSubmissionDelivery(ctx, value)
		return updateErr
	}
	envelope, err := s.decrypt(value)
	if err != nil {
		return s.deliveryFailed(ctx, value, errors.New("encrypted report payload could not be opened"))
	}
	credential, err := s.deliveryCredential(ctx, value.OrganisationID, credentialID)
	if err != nil {
		return s.deliveryFailed(ctx, value, errors.New("support delivery credential is unavailable"))
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || !validDeliveryURL(endpoint) {
		return s.deliveryFailed(ctx, value, errors.New("support API destination is unsafe"))
	}
	body, _ := json.Marshal(map[string]any{"submission_id": value.ID, "created_at": value.CreatedAt, "submission": envelope})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return s.deliveryFailed(ctx, value, err)
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Idempotency-Key", value.ID)
	request.Header.Set("X-External-Request-ID", requestID())
	client, err := identity.SafeOutboundClient(ctx, parsed, s.Client, s.Resolver)
	if err != nil {
		return s.deliveryFailed(ctx, value, errors.New("support API destination is unsafe"))
	}
	response, err := client.Do(request)
	if err != nil {
		return s.deliveryFailed(ctx, value, errors.New("support API request failed"))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		failure := fmt.Errorf("support API returned status %d", response.StatusCode)
		if response.StatusCode != http.StatusRequestTimeout && response.StatusCode != http.StatusTooManyRequests && response.StatusCode < 500 {
			return s.deliveryFailedPermanently(ctx, value, failure)
		}
		return s.deliveryFailedAfter(ctx, value, failure, retryAfter(response.Header.Get("Retry-After"), s.now()))
	}
	result := struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		ExternalID  string `json:"external_id"`
		ExternalURL string `json:"external_url"`
	}{}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxDeliveryResponse+1))
	if readErr != nil || len(raw) > maxDeliveryResponse {
		return s.deliveryFailed(ctx, value, errors.New("support API response is too large"))
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		if err := decoder.Decode(&result); err != nil {
			return s.deliveryFailed(ctx, value, errors.New("support API returned invalid JSON"))
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			return s.deliveryFailed(ctx, value, errors.New("support API returned multiple JSON values"))
		}
	}
	if result.ID == "" || len(result.ID) > 200 || result.Status != "accepted" || len(result.ExternalID) > 200 || len(result.ExternalURL) > 2000 || (result.ExternalURL != "" && !validExternalURL(result.ExternalURL)) {
		return s.deliveryFailed(ctx, value, errors.New("support API returned invalid receipt"))
	}
	now := s.now()
	value.State, value.NextAttemptAt, value.DeliveryStartedAt, value.LastError = "delivered", nil, nil, ""
	value.ExternalID, value.ExternalURL, value.DeliveredAt = result.ExternalID, result.ExternalURL, &now
	_, err = s.store.UpdateReportSubmissionDelivery(ctx, value)
	return err
}

func (s *Service) deliveryRoute(ctx context.Context, value model.ReportSubmission) (endpoint, credentialID string, err error) {
	if value.SupportRouteID == "" {
		return "", "", ErrNotConfigured
	}
	route, routeErr := s.store.SupportRoute(ctx, value.ProductID, value.SupportRouteID)
	if routeErr != nil {
		return "", "", routeErr
	}
	enabled := route.BugReportsEnabled
	if value.Kind == KindFeedback {
		enabled = route.FeedbackEnabled
	}
	if route.State != "active" || !enabled || route.BackendConnectionID == "" {
		return "", "", ErrDeliveryUnavailable
	}
	connection, connectionErr := s.store.BackendConnection(ctx, value.ProductID, route.BackendConnectionID)
	if connectionErr != nil || connection.State != "active" || connection.AuthenticationType != "bearer" || connection.CredentialSecretID == "" {
		return "", "", ErrDeliveryUnavailable
	}
	return connection.BaseURL + "/v1/support-submissions", connection.CredentialSecretID, nil
}

func validExternalURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil
}

func (s *Service) deliveryCredential(ctx context.Context, organisationID, id string) (string, error) {
	if s.vault == nil {
		return "", errors.New("credential vault is unavailable")
	}
	stored, err := s.store.Secret(ctx, organisationID, id)
	if err != nil {
		return "", err
	}
	plain, err := s.vault.Decrypt(secrets.Encrypted{Ciphertext: stored.Ciphertext, Nonce: stored.Nonce, KeyVersion: stored.KeyVersion, Fingerprint: stored.Fingerprint}, organisationID+":backend_connection:"+id)
	return string(plain), err
}

func (s *Service) deliveryFailed(ctx context.Context, value model.ReportSubmission, failure error) error {
	return s.deliveryFailedAfter(ctx, value, failure, time.Time{})
}

func (s *Service) deliveryFailedPermanently(ctx context.Context, value model.ReportSubmission, failure error) error {
	value.Attempts = max(value.Attempts, maxDeliveryAttempts)
	return s.deliveryFailedAfter(ctx, value, failure, time.Time{})
}

func retryAfter(raw string, now time.Time) time.Time {
	if seconds, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && seconds >= 0 {
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if parsed, err := http.ParseTime(raw); err == nil && parsed.After(now) {
		return parsed
	}
	return time.Time{}
}

func (s *Service) deliveryFailedAfter(ctx context.Context, value model.ReportSubmission, failure error, retryAt time.Time) error {
	value.DeliveryStartedAt = nil
	message := failure.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	value.LastError = message
	if value.Attempts >= maxDeliveryAttempts {
		value.State, value.NextAttemptAt = "failed", nil
	} else {
		next := retryAt
		if next.IsZero() {
			delay := time.Minute * time.Duration(1<<min(value.Attempts-1, 8))
			if delay > 6*time.Hour {
				delay = 6 * time.Hour
			}
			next = s.now().Add(delay)
		}
		value.State, value.NextAttemptAt = "pending", &next
	}
	_, err := s.store.UpdateReportSubmissionDelivery(ctx, value)
	return err
}

func (s *Service) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := s.ProcessPending(ctx, 50); err != nil {
				return fmt.Errorf("process pending reports: %w", err)
			}
			if _, err := s.store.DeleteExpiredReportSubmissions(ctx, s.now()); err != nil {
				return fmt.Errorf("delete expired reports: %w", err)
			}
		}
	}
}
