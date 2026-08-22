# DokoSoko service

DokoSoko is a self-hosted delivery control plane for a vendor's developer integrations. One installation represents one vendor deployment. It publishes documentation, API contracts, and policy-bound tools through a UI and Stateless MCPv2.

The public contracts are deliberately small:

- [Control Plane API](api/openapi.yaml) — DokoSoko administration, OAuth, and MCP.
- [Customer Identity Integration API](api/identity-integration-openapi.yaml) — the optional delegated-user access evaluation contract.
- [Backend Integration API](api/backend-integration-openapi.yaml) — the separately authenticated service-to-service support delivery contract.
- [Access Provider API](api/provider-openapi.yaml) — optional provider-owned instance and credential lifecycle.

Each integration contract is independently deployable and independently code-generatable. A vendor can generate a server stub for the contract it implements; DokoSoko does not require a package. There is no package registry abstraction and no per-operation hook configuration. If an integration is an HTTP API, expose it as an API or a tool. Client libraries are optional generated artifacts outside the runtime contract.

## Customer identity integration (optional)

Private customer access optionally configures one OIDC provider and one credential-free delegated API origin. This configuration never contains a service-to-service credential. DokoSoko owns the downstream OAuth server and tokens.

```text
MCP client
  -> GET /.well-known/oauth-protected-resource/mcp
  -> DokoSoko authorization code + PKCE flow
  -> vendor OIDC authentication
  -> POST {delegated_api_origin}/v1/access/evaluations
  -> durable DokoSoko customer account
  -> resource-bound DokoSoko access token
  -> POST /mcp
```

The configured OIDC organisation claim is a vendor-owned external customer identifier. DokoSoko resolves it, together with the issuer, to an internal `customer_account` resource. Internal IDs are used for installations and version pins; vendor IDs are never accepted as interchangeable internal references.

Access evaluation returns short-lived `grants`. DokoSoko defines tools and their `required_grants`; the vendor does not define the tool catalog. Evaluation, identity, account state, schema, confirmation, and grant failures all fail closed. Suspending a customer account invalidates an existing token on its next use.

The vendor OAuth access token is encrypted at rest and used only for delegated customer operations. The inbound DokoSoko token is never forwarded. Customer tools have fixed HTTPS destinations on the delegated API origin or a separately configured MCP connection—never arbitrary request-time URLs. Disabling the identity provider invalidates private customer authorization on its next use and does not affect public discovery or backend delivery.

## Backend integration

DokoSoko backend connections are separate resources with their own origin, state, revision, authentication type, and rotatable credential. Support routes reference a backend connection; they do not own or copy credentials. Identity and backend origins may be different.

DokoSoko appends exact, versioned paths to the relevant configured origin:

| Operation | Authentication | Retry contract |
| --- | --- | --- |
| `POST /v1/access/evaluations` | Live delegated vendor access token | Stable evaluation idempotency key; authorization fails closed on any invalid or unavailable response. |
| `POST /v1/support-submissions` | Encrypted service bearer | Durable at-least-once delivery; stable idempotency key and a new request ID per attempt. |

Support reporting is private and consent-gated. A user must preview and explicitly approve a bounded report. DokoSoko encrypts it in a durable outbox and sends bug reports and feedback through the same endpoint and credential. Vendors must retain idempotency results and return `409` when a key is reused with a different payload.

Usage, if a vendor chooses to expose it, is an ordinary API operation or tool. It has no privileged hook type.

## Protocol endpoints

| Surface | Endpoint |
| --- | --- |
| Health and readiness | `/healthz`, `/readyz` |
| OAuth authorization-server metadata | `/.well-known/oauth-authorization-server` |
| MCP protected-resource metadata | `/.well-known/oauth-protected-resource/mcp` |
| OAuth | `/oauth/authorize`, `/oauth/callback`, `/oauth/token` |
| Private MCP | `/mcp` |
| Public MCP | `/mcp/public` |
| Administration | `/api/v1/...` |

Private MCP requires the optional identity integration and a resource-bound DokoSoko token. Public MCP is independent of identity: it is anonymous, read-only, rate-limited, and globally disabled by default. An Integration is private by default and appears publicly only after an explicit per-Integration visibility acknowledgement and publication. The global Public MCP setting is the emergency master switch. Both endpoints implement only the pinned Stateless MCPv2 protocol revision.

## Run locally

Requirements: Go 1.25+, Node.js 22.13+, pnpm, and PostgreSQL 17 with pgvector for persistent development.

```bash
pnpm install
pnpm build
export DOKOSOKO_DATABASE_URL='postgres://...'
export DOKOSOKO_MASTER_KEY='base64-encoded-32-byte-key'
export DOKOSOKO_SETUP_TOKEN='one-time-random-setup-token'
go run ./cmd/dokosoko
```

The integrated service listens on `http://localhost:8080` by default and serves `dist/client`. For disposable development only, set `DOKOSOKO_DEV_MEMORY=true`. Demo bearer tokens additionally require `DOKOSOKO_ALLOW_DEMO_TOKENS=true`.

For the UI development server:

```bash
DOKOSOKO_DEV_PROXY=http://127.0.0.1:8080 pnpm dev
```

## Deploy

```bash
cp .env.example .env
docker compose up --build
```

Required configuration:

| Variable | Purpose |
| --- | --- |
| `DOKOSOKO_DATABASE_URL` | PostgreSQL connection string outside the Compose-provided service configuration. |
| `DOKOSOKO_DATABASE_PASSWORD` | Compose PostgreSQL password. |
| `DOKOSOKO_MASTER_KEY` | Standard-base64 encoding of exactly 32 random bytes. Keep it stable and backed up. |
| `DOKOSOKO_SETUP_TOKEN` | Strong one-time first-run secret. Remove or rotate it after setup. |
| `DOKOSOKO_PUBLIC_URL` | Exact external origin. HTTPS is required outside localhost. |

Back up PostgreSQL, artifact data, and the encryption key as one recovery unit. Production should terminate TLS at a trusted reverse proxy and preserve the configured public origin exactly.

### Breaking v3 upgrade

Migration `0020_contract_v3.sql` deliberately removes the legacy package and open-ended hook contracts. Back up the database and encryption key before deploying it.

The migration preserves installations, customer version pins, and encrypted support submissions. It maps legacy customer identifiers to durable customer accounts. It invalidates outstanding OAuth states, authorization codes, and access tokens; removes identity configuration whose delegated API origin cannot be inferred safely; creates disabled backend-connection placeholders for legacy support routes; and defaults every Integration to private.

After the upgrade, configure customer identity only if private customer access is required. Configure or repair each backend connection, create a fresh credential, attach it to the intended support routes, and then re-enable delivery. Legacy hook credentials are not rebound because their encryption purpose and trust boundary differ. Existing MCP clients must authenticate again. Package artifacts and hook-specific configuration are not migrated because they have no faithful representation in the new contract.

## Verification

```bash
pnpm run test:all
docker compose config
```

The suites cover OAuth resource and PKCE binding, durable customer-account resolution and suspension, fail-closed grants, fixed tool destinations, managed MCP delegation and drift, support consent and idempotent outbox delivery, publication boundaries, access-provider operations, authentication, analytics, crawler isolation, and static console output.

## Repository map

```text
app/                  administration console
cmd/dokosoko/         service entry point
crawler/              isolated documentation crawler
internal/identity/    downstream OAuth, upstream OIDC, customer accounts, grants
internal/reporting/   encrypted support outbox and delivery
internal/tools/       policy-bound HTTP tool execution
internal/mcpbridge/   managed MCP import, delegated OAuth, drift, execution
internal/access/      provider-owned instance and credential lifecycle
internal/platform/    catalog validation, state transitions, audit
internal/store/       memory/PostgreSQL persistence
migrations/           checksummed PostgreSQL migrations
api/                  normative OpenAPI contracts
```

See [the contract plan](docs/FINAL_PLAN.md) for invariants and failure semantics.
