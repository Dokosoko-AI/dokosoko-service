package identity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

type HookAuthorization struct {
	repository Repository
	vault      *secrets.Vault
	Client     *http.Client
	Resolver   IPResolver
}

func NewHookAuthorization(repository Repository, vault *secrets.Vault) *HookAuthorization {
	return &HookAuthorization{repository: repository, vault: vault}
}

func (h *HookAuthorization) Authorize(ctx context.Context, productID string, tool model.Tool, arguments map[string]any, principal toolruntime.Principal) error {
	config, err := h.repository.VendorIdentity(ctx, productID)
	if err != nil {
		return err
	}
	if config.AuthorizationHookURL == "" {
		return nil
	}
	parsed, err := url.Parse(config.AuthorizationHookURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Port() != "" {
		return errors.New("unsafe authorization hook URL")
	}
	if h.vault == nil || config.AuthorizationCredentialID == "" {
		return errors.New("authorization hook credential unavailable")
	}
	stored, err := h.repository.Secret(ctx, config.OrganisationID, config.AuthorizationCredentialID)
	if err != nil {
		return err
	}
	credential, err := h.vault.Decrypt(secrets.Encrypted{Ciphertext: stored.Ciphertext, Nonce: stored.Nonce, KeyVersion: stored.KeyVersion, Fingerprint: stored.Fingerprint}, config.OrganisationID+":authorization:"+config.AuthorizationCredentialID)
	if err != nil {
		return err
	}
	argumentKeys := make([]string, 0, len(arguments))
	for key := range arguments {
		argumentKeys = append(argumentKeys, key)
	}
	sort.Strings(argumentKeys)
	body, _ := json.Marshal(map[string]any{"operation": "tool.execute", "tool": tool.Namespace + "." + tool.Name, "product_id": productID, "subject": principal.Subject, "vendor_organisation_id": principal.VendorOrganisation, "argument_keys": argumentKeys})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+string(credential))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	client, err := (&HookEntitlements{Client: h.Client, Resolver: h.Resolver}).clientFor(ctx, parsed)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("authorization hook failed")
	}
	var decision struct {
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&decision); err != nil || !decision.Allowed {
		return errors.New("authorization hook denied operation")
	}
	return nil
}
