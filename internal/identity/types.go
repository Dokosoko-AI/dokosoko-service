package identity

import "time"

// ProviderConfig is the optional delegated customer identity boundary. It does
// not contain service-to-service delivery credentials.
type ProviderConfig struct {
	ID                 string    `json:"id"`
	OrganisationID     string    `json:"organisation_id"`
	DeploymentID       string    `json:"deployment_id"`
	Issuer             string    `json:"issuer"`
	ClientID           string    `json:"client_id"`
	ClientSecretID     string    `json:"-"`
	Scopes             []string  `json:"scopes"`
	Audience           string    `json:"audience,omitempty"`
	OAuthResource      string    `json:"oauth_resource,omitempty"`
	OrganisationClaim  string    `json:"customer_account_claim"`
	InstallationClaim  string    `json:"installation_claim"`
	DelegatedAPIOrigin string    `json:"authorization_api_origin"`
	State              string    `json:"state"`
	Revision           int64     `json:"revision"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CustomerAccount struct {
	ID                  string    `json:"id"`
	OrganisationID      string    `json:"organisation_id"`
	ProductID           string    `json:"product_id"`
	Issuer              string    `json:"issuer"`
	ExternalID          string    `json:"external_id"`
	State               string    `json:"state"`
	Revision            int64     `json:"revision"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	LastAuthenticatedAt time.Time `json:"last_authenticated_at"`
}

type OAuthState struct {
	Digest              []byte
	ProductID           string
	ProviderRevision    int64
	ClientID            string
	RedirectURI         string
	Resource            string
	Scopes              []string
	DownstreamState     string
	DownstreamChallenge string
	UpstreamVerifier    string
	Nonce               string
	ExpiresAt           time.Time
}

type OAuthCode struct {
	Digest                 []byte
	ProductID              string
	OrganisationID         string
	ProviderRevision       int64
	ClientID               string
	RedirectURI            string
	Resource               string
	Scopes                 []string
	DownstreamChallenge    string
	Issuer                 string
	Subject                string
	Email                  string
	DisplayName            string
	CustomerAccountID      string
	ExternalCustomerID     string
	InstallationID         string
	Grants                 map[string]bool
	AccessEvaluationID     string
	AccessEvaluatedAt      time.Time
	PolicyVersion          string
	UpstreamAccessSecretID string
	AccessExpiresAt        time.Time
	ExpiresAt              time.Time
}

type AccessToken struct {
	Digest                 []byte
	ProductID              string
	ProviderRevision       int64
	ClientID               string
	Resource               string
	Issuer                 string
	Subject                string
	Email                  string
	DisplayName            string
	CustomerAccountID      string
	ExternalCustomerID     string
	InstallationID         string
	Grants                 map[string]bool
	AccessEvaluationID     string
	AccessEvaluatedAt      time.Time
	PolicyVersion          string
	UpstreamAccessSecretID string
	Scopes                 []string
	ExpiresAt              time.Time
	CreatedAt              time.Time
	RevokedAt              *time.Time
}

// ProviderTest is a short-lived proof that one exact disabled provider
// revision completed the real upstream OIDC authorization-code flow. It never
// contains upstream tokens or a client secret.
type ProviderTest struct {
	ID                    string     `json:"id"`
	OrganisationID        string     `json:"-"`
	DeploymentID          string     `json:"-"`
	ConfigurationRevision int64      `json:"configuration_revision"`
	StateDigest           []byte     `json:"-"`
	UpstreamVerifier      string     `json:"-"`
	Nonce                 string     `json:"-"`
	Status                string     `json:"status"`
	AuthorizationURL      string     `json:"authorization_url,omitempty"`
	FailureCode           string     `json:"failure_code,omitempty"`
	Issuer                string     `json:"issuer,omitempty"`
	Subject               string     `json:"-"`
	CustomerID            string     `json:"customer_id,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	ExpiresAt             time.Time  `json:"expires_at"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
	CallbackClaimedAt     *time.Time `json:"-"`
}

type Principal struct {
	ProductID           string
	ClientID            string
	Resource            string
	Issuer              string
	Subject             string
	Email               string
	DisplayName         string
	CustomerAccountID   string
	ExternalCustomerID  string
	InstallationID      string
	Grants              map[string]bool
	AccessEvaluationID  string
	AccessEvaluatedAt   time.Time
	PolicyVersion       string
	DelegatedAPIOrigin  string
	UpstreamAccessToken string
	Scopes              []string
}

type Claims struct {
	Issuer             string
	Subject            string
	Email              string
	DisplayName        string
	ExternalCustomerID string
	InstallationID     string
}

type UpstreamIdentity struct {
	Claims              Claims
	AccessToken         string
	ExpiresAt           time.Time
	AccessEvaluationKey string
}

type AccessEvaluation struct {
	ID            string    `json:"id"`
	Grants        []string  `json:"grants"`
	ExpiresAt     time.Time `json:"expires_at"`
	PolicyVersion string    `json:"policy_version,omitempty"`
}

type ClientMetadata struct {
	ClientID     string   `json:"client_id"`
	ClientName   string   `json:"client_name,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
}

// OAuthClient is a public downstream MCP client registration. It contains no
// credential: DokoSoko supports public clients with PKCE and exact redirect URI
// matching only.
type OAuthClient struct {
	ClientID     string
	DeploymentID string
	ClientName   string
	RedirectURIs []string
	CreatedAt    time.Time
}
