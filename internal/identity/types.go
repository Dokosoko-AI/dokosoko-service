package identity

import "time"

type VendorConfig struct {
	ID                        string    `json:"id"`
	OrganisationID            string    `json:"organisation_id"`
	ProductID                 string    `json:"product_id"`
	Issuer                    string    `json:"issuer"`
	ClientID                  string    `json:"client_id"`
	ClientSecretID            string    `json:"-"`
	Scopes                    []string  `json:"scopes"`
	Audience                  string    `json:"audience"`
	OrganisationClaim         string    `json:"organisation_claim"`
	EntitlementHookURL        string    `json:"entitlement_hook_url"`
	AllowedRedirectURIs       []string  `json:"allowed_redirect_uris"`
	AuthorizationHookURL      string    `json:"authorization_hook_url"`
	AuthorizationCredentialID string    `json:"-"`
	Revision                  int64     `json:"revision"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

type OAuthState struct {
	Digest              []byte
	ProductID           string
	ClientID            string
	RedirectURI         string
	DownstreamState     string
	DownstreamChallenge string
	UpstreamVerifier    string
	Nonce               string
	ExpiresAt           time.Time
}

type OAuthCode struct {
	Digest              []byte
	ProductID           string
	ClientID            string
	RedirectURI         string
	DownstreamChallenge string
	Issuer              string
	Subject             string
	Email               string
	DisplayName         string
	VendorOrganisation  string
	Entitlements        map[string]bool
	ExpiresAt           time.Time
}

type AccessToken struct {
	Digest             []byte
	ProductID          string
	ClientID           string
	Issuer             string
	Subject            string
	Email              string
	DisplayName        string
	VendorOrganisation string
	Entitlements       map[string]bool
	Scopes             []string
	ExpiresAt          time.Time
	CreatedAt          time.Time
	RevokedAt          *time.Time
}

type Principal struct {
	ProductID          string
	ClientID           string
	Issuer             string
	Subject            string
	Email              string
	DisplayName        string
	VendorOrganisation string
	Entitlements       map[string]bool
	Scopes             []string
}

type Claims struct {
	Issuer             string
	Subject            string
	Email              string
	DisplayName        string
	VendorOrganisation string
}
