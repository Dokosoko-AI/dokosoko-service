package identity

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

var ErrProviderConfiguration = errors.New("identity provider configuration is incomplete")

func validNetworkURL(raw string, baseOrigin, allowQuery, allowHTTPSPort bool) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	local := IsLocalDevelopmentHostname(parsed.Hostname())
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && local) {
		return false
	}
	if parsed.Scheme == "https" && parsed.Port() != "" && !allowHTTPSPort {
		return false
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return false
		}
	}
	if (baseOrigin && parsed.Path != "") || (!allowQuery && parsed.RawQuery != "") {
		return false
	}
	return true
}

func validOAuthResourceIdentifier(raw string) bool {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	return err == nil && raw != "" && !strings.ContainsAny(raw, " \t\r\n") && parsed.IsAbs() && parsed.Scheme != "" && parsed.User == nil && parsed.Fragment == ""
}

// ValidateProviderConfig applies the complete persisted OIDC readiness
// contract. It is deliberately shared by test and activation entry points so
// pre-lifecycle database rows cannot bypass fields now required on writes.
func ValidateProviderConfig(config ProviderConfig) error {
	switch {
	case !validNetworkURL(config.Issuer, false, false, true) || strings.TrimSpace(config.ClientID) == "":
		return fmt.Errorf("%w: issuer and client ID must be valid", ErrProviderConfiguration)
	case strings.TrimSpace(config.ClientSecretID) == "":
		return fmt.Errorf("%w: client credential must be saved", ErrProviderConfiguration)
	case strings.TrimSpace(config.OrganisationClaim) == "":
		return fmt.Errorf("%w: customer account claim is required", ErrProviderConfiguration)
	case !validNetworkURL(config.DelegatedAPIOrigin, true, false, false):
		return fmt.Errorf("%w: delegated API origin must be valid", ErrProviderConfiguration)
	case strings.TrimSpace(config.OAuthResource) != "" && !validOAuthResourceIdentifier(config.OAuthResource):
		return fmt.Errorf("%w: OAuth resource must be an absolute URI without a fragment", ErrProviderConfiguration)
	}
	for _, scope := range config.Scopes {
		if strings.TrimSpace(scope) == "openid" {
			return nil
		}
	}
	return fmt.Errorf("%w: openid scope is required", ErrProviderConfiguration)
}
