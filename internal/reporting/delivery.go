package reporting

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/netpolicy"
)

const (
	reportDeliveryTimeout     = 10 * time.Second
	reportDeliveryLease       = 30 * time.Second
	reportDeliveryBatchSize   = 25
	reportDeliveryMaxAttempts = 8
	reportDeliveryBodyLimit   = 64 << 10
)

type deliveryFailure struct {
	category  string
	retryable bool
}

func (e *deliveryFailure) Error() string { return e.category }

func reportDeliveryRetryDelay(attempts int) time.Duration {
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

func (s *Service) safeDeliveryDestination(ctx context.Context, raw string) (*url.URL, net.IP, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, nil, &deliveryFailure{category: "invalid delivery destination"}
	}
	hostname := strings.ToLower(parsed.Hostname())
	localDevelopment := identity.IsLocalDevelopmentHostname(hostname)
	if hostname == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (localDevelopment && parsed.Scheme != "http") || (!localDevelopment && (parsed.Scheme != "https" || parsed.Port() != "" && parsed.Port() != "443")) {
		return nil, nil, &deliveryFailure{category: "unsafe delivery destination"}
	}
	resolver := s.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIP(ctx, "ip", hostname)
	if err != nil || len(addresses) == 0 {
		return nil, nil, &deliveryFailure{category: "delivery destination did not resolve", retryable: true}
	}
	for _, address := range addresses {
		if localDevelopment {
			if !netpolicy.LocalDevelopmentIP(address) {
				return nil, nil, &deliveryFailure{category: "unsafe delivery destination"}
			}
			continue
		}
		if netpolicy.UnsafeIP(address) {
			return nil, nil, &deliveryFailure{category: "unsafe delivery destination"}
		}
	}
	return parsed, addresses[0], nil
}

func (s *Service) deliveryClient(parsed *url.URL, address net.IP) interface {
	Do(*http.Request) (*http.Response, error)
} {
	if s.doer != nil {
		return s.doer
	}
	localDevelopment := identity.IsLocalDevelopmentHostname(parsed.Hostname())
	port := parsed.Port()
	if port == "" {
		port = "443"
		if localDevelopment {
			port = "80"
		}
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		DisableCompression:    true,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: reportDeliveryTimeout,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
		},
	}
	if !localDevelopment {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: parsed.Hostname()}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   reportDeliveryTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (s *Service) deliver(ctx context.Context, submission model.ReportSubmission) error {
	parsed, address, err := s.safeDeliveryDestination(ctx, submission.DeliveryURL)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(submission.Payload))
	if err != nil {
		return &deliveryFailure{category: "invalid delivery request"}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", submission.ID)
	request.Header.Set("X-DokoSoko-Submission-Kind", submission.Kind)
	response, err := s.deliveryClient(parsed, address).Do(request)
	if err != nil {
		return &deliveryFailure{category: "delivery transport failed", retryable: true}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, reportDeliveryBodyLimit))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	retryable := response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
	return &deliveryFailure{category: fmt.Sprintf("delivery returned HTTP %d", response.StatusCode), retryable: retryable}
}

// RunDelivery drains the plaintext support outbox. Each row snapshots one root
// destination, is claimed with a lease, and is retried without exposing payloads
// or credentials in the persisted error field.
func (s *Service) RunDelivery(ctx context.Context, interval time.Duration) error {
	if s.store == nil {
		return errors.New("support submission delivery is not configured")
	}
	if interval <= 0 {
		interval = time.Second
	}
	owner, err := randomUUID()
	if err != nil {
		return errors.New("support submission delivery owner could not be created")
	}
	for {
		now := s.now().UTC()
		submissions, err := s.store.ClaimReportSubmissions(ctx, owner, now.Add(reportDeliveryLease), reportDeliveryBatchSize)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		for _, submission := range submissions {
			deliveryCtx, cancel := context.WithTimeout(ctx, reportDeliveryTimeout)
			deliveryErr := s.deliver(deliveryCtx, submission)
			cancel()
			if ctx.Err() != nil {
				return nil
			}
			if deliveryErr == nil {
				if err := s.store.CompleteReportSubmission(ctx, submission.ID, owner, s.now().UTC()); err != nil {
					return err
				}
				continue
			}
			failure := &deliveryFailure{category: "delivery failed", retryable: true}
			if errors.As(deliveryErr, &failure) && (!failure.retryable || submission.Attempts >= reportDeliveryMaxAttempts) {
				if err := s.store.FailReportSubmission(ctx, submission.ID, owner, failure.category); err != nil {
					return err
				}
				continue
			}
			if err := s.store.RetryReportSubmission(ctx, submission.ID, owner, s.now().Add(reportDeliveryRetryDelay(submission.Attempts)), failure.category); err != nil {
				return err
			}
		}
		if len(submissions) == reportDeliveryBatchSize {
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
