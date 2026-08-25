package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type HTTPClientMetadataResolver struct {
	Client   *http.Client
	Resolver IPResolver
}

func (r *HTTPClientMetadataResolver) Resolve(ctx context.Context, clientID string) (ClientMetadata, error) {
	parsed, err := url.Parse(clientID)
	if err != nil || parsed.Scheme != "https" || parsed.Path == "" || parsed.Path == "/" || parsed.RawQuery != "" {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	client, err := SafeOutboundClient(ctx, parsed, r.Client, r.Resolver)
	if err != nil {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, clientID, nil)
	if err != nil {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	var metadata ClientMetadata
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&metadata); err != nil || decoder.Decode(&struct{}{}) != io.EOF || metadata.ClientID != clientID || len(metadata.RedirectURIs) == 0 || len(metadata.RedirectURIs) > 20 {
		return ClientMetadata{}, ErrInvalidOAuth
	}
	for _, redirect := range metadata.RedirectURIs {
		if !validRedirect(redirect) {
			return ClientMetadata{}, ErrInvalidOAuth
		}
	}
	return metadata, nil
}

type HTTPAccessEvaluation struct {
	Client   *http.Client
	Resolver IPResolver
	Now      func() time.Time
}

func retryDelay(header string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return min(time.Duration(seconds)*time.Second, 250*time.Millisecond)
	}
	return 100 * time.Millisecond
}

func (h *HTTPAccessEvaluation) Resolve(ctx context.Context, config ProviderConfig, upstream UpstreamIdentity) (AccessEvaluation, error) {
	now := time.Now().UTC
	if h.Now != nil {
		now = h.Now
	}
	if config.DelegatedAPIOrigin == "" {
		return AccessEvaluation{}, errors.New("vendor access evaluation is not configured")
	}
	parsed, err := url.Parse(config.DelegatedAPIOrigin + accessEvaluationPath)
	if err != nil {
		return AccessEvaluation{}, err
	}
	client, err := SafeOutboundClient(ctx, parsed, h.Client, h.Resolver)
	if err != nil {
		return AccessEvaluation{}, err
	}
	body := []byte(`{}`)
	idempotencyKey := upstream.AccessEvaluationKey
	if idempotencyKey == "" {
		return AccessEvaluation{}, errors.New("access evaluation key is unavailable")
	}
	var response *http.Response
	for attempt := 0; attempt < 2; attempt++ {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
		if requestErr != nil {
			return AccessEvaluation{}, requestErr
		}
		request.Header.Set("Authorization", "Bearer "+upstream.AccessToken)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Idempotency-Key", idempotencyKey)
		request.Header.Set("X-External-Request-ID", requestID())
		response, err = client.Do(request)
		if err == nil && response.StatusCode != http.StatusRequestTimeout && response.StatusCode != http.StatusTooManyRequests && response.StatusCode < 500 {
			break
		}
		if response != nil {
			response.Body.Close()
		}
		if attempt == 0 {
			delay := 100 * time.Millisecond
			if response != nil {
				delay = retryDelay(response.Header.Get("Retry-After"))
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return AccessEvaluation{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if err != nil {
		return AccessEvaluation{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return AccessEvaluation{}, fmt.Errorf("access evaluation returned %s", response.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return AccessEvaluation{}, errors.New("access evaluation response is too large")
	}
	var value AccessEvaluation
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&value); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return AccessEvaluation{}, errors.New("invalid access evaluation response")
	}
	if value.ID == "" || len(value.ID) > 200 || value.ExpiresAt.Before(now().Add(-time.Second)) || value.ExpiresAt.After(now().Add(24*time.Hour)) || len(value.PolicyVersion) > 200 || len(value.Grants) > 500 {
		return AccessEvaluation{}, errors.New("invalid access evaluation response")
	}
	seen := make(map[string]bool, len(value.Grants))
	for _, grant := range value.Grants {
		if !grantPattern.MatchString(grant) || seen[grant] {
			return AccessEvaluation{}, errors.New("invalid access evaluation grants")
		}
		seen[grant] = true
	}
	sort.Strings(value.Grants)
	return value, nil
}
