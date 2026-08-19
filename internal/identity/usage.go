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
	"regexp"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/secrets"
)

const (
	maxUsageItems       = 50
	maxUsageLabelLength = 80
	maxUsageUnitLength  = 32
	maxUsageTextLength  = 500
)

var (
	ErrUsageNotConfigured = errors.New("usage hook is not configured")
	ErrUsageDenied        = errors.New("usage hook denied access")
	ErrUsageUpstream      = errors.New("usage hook failed")
	usageKeyPattern       = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
	usageFormats          = map[string]bool{"": true, "text": true, "number": true, "percentage": true, "date": true, "datetime": true, "duration": true, "currency": true}
)

// UsageItem is a vendor-defined scalar value. DokoSoko preserves the order and
// values supplied by the vendor; the remaining fields are presentation hints.
type UsageItem struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Value       any    `json:"value"`
	Format      string `json:"format,omitempty"`
	Unit        string `json:"unit,omitempty"`
	Description string `json:"description,omitempty"`
}

type UsageReport struct {
	AsOf  string      `json:"as_of"`
	Items []UsageItem `json:"items"`
}

type UsageReporter interface {
	Report(context.Context, string, Principal) (UsageReport, error)
}

type HookUsage struct {
	repository Repository
	vault      *secrets.Vault
	Client     *http.Client
	Resolver   IPResolver
}

func NewHookUsage(repository Repository, vault *secrets.Vault) *HookUsage {
	return &HookUsage{repository: repository, vault: vault}
}

func (h *HookUsage) Report(ctx context.Context, productID string, principal Principal) (UsageReport, error) {
	config, err := h.repository.VendorIdentity(ctx, productID)
	if err != nil {
		return UsageReport{}, err
	}
	if config.UsageHookURL == "" {
		return UsageReport{}, ErrUsageNotConfigured
	}
	parsed, err := url.Parse(config.UsageHookURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Port() != "" || parsed.Fragment != "" {
		return UsageReport{}, fmt.Errorf("%w: unsafe URL", ErrUsageUpstream)
	}
	if h.vault == nil || config.UsageCredentialID == "" {
		return UsageReport{}, fmt.Errorf("%w: credential unavailable", ErrUsageUpstream)
	}
	stored, err := h.repository.Secret(ctx, config.OrganisationID, config.UsageCredentialID)
	if err != nil {
		return UsageReport{}, fmt.Errorf("%w: credential unavailable", ErrUsageUpstream)
	}
	credential, err := h.vault.Decrypt(secrets.Encrypted{Ciphertext: stored.Ciphertext, Nonce: stored.Nonce, KeyVersion: stored.KeyVersion, Fingerprint: stored.Fingerprint}, config.OrganisationID+":usage:"+config.UsageCredentialID)
	if err != nil {
		return UsageReport{}, fmt.Errorf("%w: credential unavailable", ErrUsageUpstream)
	}
	subject := principal.Subject
	if principal.Issuer != "" {
		subject = principal.Issuer + "|" + principal.Subject
	}
	body, err := json.Marshal(map[string]string{"product_id": productID, "subject": subject, "vendor_organisation_id": principal.VendorOrganisation, "installation_id": principal.InstallationID})
	if err != nil {
		return UsageReport{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), strings.NewReader(string(body)))
	if err != nil {
		return UsageReport{}, err
	}
	request.Header.Set("Authorization", "Bearer "+string(credential))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	client, err := (&HookEntitlements{Client: h.Client, Resolver: h.Resolver}).clientFor(ctx, parsed)
	if err != nil {
		return UsageReport{}, fmt.Errorf("%w: unsafe destination", ErrUsageUpstream)
	}
	response, err := client.Do(request)
	if err != nil {
		return UsageReport{}, fmt.Errorf("%w: request failed", ErrUsageUpstream)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusForbidden {
		return UsageReport{}, ErrUsageDenied
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return UsageReport{}, fmt.Errorf("%w: status %d", ErrUsageUpstream, response.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return UsageReport{}, fmt.Errorf("%w: response too large", ErrUsageUpstream)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var report UsageReport
	if err := decoder.Decode(&report); err != nil {
		return UsageReport{}, fmt.Errorf("%w: invalid response", ErrUsageUpstream)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return UsageReport{}, fmt.Errorf("%w: invalid response", ErrUsageUpstream)
	}
	if err := validateUsageReport(report); err != nil {
		return UsageReport{}, fmt.Errorf("%w: %v", ErrUsageUpstream, err)
	}
	return report, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}

func validateUsageReport(report UsageReport) error {
	if report.AsOf == "" {
		return errors.New("as_of is required")
	}
	if _, err := time.Parse(time.RFC3339, report.AsOf); err != nil {
		return errors.New("as_of must be an RFC 3339 timestamp")
	}
	if report.Items == nil {
		return errors.New("items is required")
	}
	if len(report.Items) > maxUsageItems {
		return fmt.Errorf("items must contain no more than %d values", maxUsageItems)
	}
	seen := make(map[string]bool, len(report.Items))
	for _, item := range report.Items {
		if !usageKeyPattern.MatchString(item.Key) || seen[item.Key] {
			return errors.New("item keys must be valid and unique")
		}
		seen[item.Key] = true
		if item.Label != strings.TrimSpace(item.Label) || item.Label == "" || len(item.Label) > maxUsageLabelLength {
			return errors.New("item labels must be between 1 and 80 characters")
		}
		if !usageFormats[item.Format] {
			return errors.New("item format is not supported")
		}
		if len(item.Unit) > maxUsageUnitLength || len(item.Description) > maxUsageTextLength {
			return errors.New("item presentation metadata is too long")
		}
		switch value := item.Value.(type) {
		case nil, bool, json.Number:
		case string:
			if len(value) > maxUsageTextLength {
				return errors.New("item value is too long")
			}
		default:
			return errors.New("item values must be scalar JSON values")
		}
	}
	return nil
}
