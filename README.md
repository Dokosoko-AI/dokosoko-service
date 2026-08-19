# DokoSoko service

DokoSoko is a self-hosted, UI-first delivery control plane for coding agents. Vendors use it to publish trustworthy product knowledge, distribute public or private SDK packages, expose policy-wrapped API tools, and issue short-lived projects and credentials.

The implementation follows [the final product plan](docs/FINAL_PLAN.md). Its key security boundary is simple: private by default, explicit and audited publication, vendor-controlled entitlements, separate per-operation authorization, and no persistent credential exposure to browsers or MCP clients.

## Implemented product surface

- Static Next.js console built on Catalyst-style UI primitives and served by the Go service.
- First-run setup with a one-time setup token, Argon2id passwords, TOTP MFA, one-use recovery codes, secure sessions, CSRF, and root-user lifecycle management.
- PostgreSQL/pgvector persistence with checksummed startup migrations and a database-backed readiness check.
- Organisations, products, production/non-production environments, and private-first resources.
- Sources with sitemap-first Crawlee discovery, Cheerio fast path, Playwright fallback, immutable snapshots, incremental HTTP metadata, SSRF controls, crawl budgets, prompt-injection scanning, quarantine, review, and atomic publication.
- npm, Go, Git, Maven/Java, Android, Swift, and NuGet/C# package records in public-link, credential-backed proxy, and short-lived fetch-hook modes.
- Custom MCP tools defined by name, JSON input/output Schema, fixed HTTPS API hook, encrypted credential, entitlement policy, confirmation policy, schema validation, draft/publish lifecycle, and audited execution.
- Managed third-party MCP imports with the [Stateless MCPv2 Only](https://blog.modelcontextprotocol.io/posts/2026-07-28/) `2026-07-28` contract, fixed HTTPS upstreams, complete catalog inspection, explicit selection, namespacing, schema pins, drift shutdown, and local draft/publish review.
- Separate upstream identity modes: an encrypted service credential or delegated OAuth grants bound to the canonical DokoSoko subject. The inbound DokoSoko token is never forwarded.
- OAuth 2.0 authorization-code + PKCE broker from MCP/widget clients through DokoSoko to the vendor OIDC provider.
- Vendor entitlement resolution during login and an independent per-operation authorization hook. Both fail closed.
- Standard Provider API integration for idempotent project creation, one-time credential delivery, short leases, and revocation.
- Private MCP with product-bound DokoSoko tokens; Public MCP is anonymous, read-only, public-only, rate-limited, and off by default.
- Owner-scoped integration runs with deterministic completion, analytics funnel events, and an append-only audit feed.
- Analytics for authorized/active users, MCP channels, tools, packages, integration runs, validated success, and daily volume without storing raw queries or secrets.
- Role-based LLM profiles with mandatory untrusted-context controls: no model authority, no model tool execution, citations required, bounded input/output, and no answer on low confidence.
- System Doctor and hardened container deployment.

## Architecture

```text
Static web console ─┐
Private MCP ────────┼──> Go control plane ──> PostgreSQL + encrypted artifact data
Public MCP ─────────┤           │
Widget loaders ─────┘           ├──> isolated Crawlee/Playwright worker
                                └──> vendor IdP, hooks, packages, tools, Provider API,
                                     reviewed third-party MCPv2 upstreams
```

DokoSoko owns the downstream OAuth token, publication policy, audit, analytics, package streaming, MCP discovery, and runtime authorization. It never forwards the inbound DokoSoko token to a vendor API. Server-side integrations use separately encrypted vendor credentials.

## Deploy with Docker Compose

Requirements: Docker with Compose v2.

```bash
cp .env.example .env
# Replace every required value in .env.
docker compose up --build
```

Open `http://localhost:8080`. On first launch, enter `DOKOSOKO_SETUP_TOKEN`, create the first root administrator, and complete TOTP enrollment. There is no default administrator account or password.

Generate suitable local secret material with a password manager or OS CSPRNG. `DOKOSOKO_MASTER_KEY` must be exactly 32 random bytes encoded with standard base64; keep it stable and backed up because encrypted integration credentials depend on it.

The Compose deployment runs:

- `dokosoko`: Go API, OAuth/MCP/package runtime, migrations, and static console;
- `crawler`: non-root, read-only Crawlee/Playwright worker with dropped Linux capabilities;
- `postgres`: PostgreSQL 17 with pgvector;
- persistent database and artifact volumes.

For production, terminate TLS at a trusted reverse proxy and set `DOKOSOKO_PUBLIC_URL` to the exact external HTTPS origin. Back up both PostgreSQL and the artifact volume together. Test restore and key recovery before onboarding production credentials.

## Required configuration

| Variable | Purpose |
| --- | --- |
| `DOKOSOKO_DATABASE_PASSWORD` | Compose PostgreSQL password. |
| `DOKOSOKO_MASTER_KEY` | Standard-base64 encoding of one 32-byte encryption key. |
| `DOKOSOKO_SETUP_TOKEN` | Strong one-time first-run secret; rotate/remove from deployment configuration after setup. |
| `DOKOSOKO_PUBLIC_URL` | Exact public origin; HTTPS is required outside localhost. |

Optional service variables include `DOKOSOKO_LISTEN`, `DOKOSOKO_DATABASE_URL`, `DOKOSOKO_UI_DIR`, `DOKOSOKO_DATA_DIR`, and `DOKOSOKO_MIGRATIONS_DIR`. The crawler accepts `DOKOSOKO_CRAWLER_MAX_PAGES` and `DOKOSOKO_CRAWLER_MAX_BYTES`.

`DOKOSOKO_DEV_MEMORY=true` is only for disposable local development. Demo bearer tokens are unavailable unless both memory mode and `DOKOSOKO_ALLOW_DEMO_TOKENS=true` are explicitly enabled.

## Local development

Requirements: Node.js 22.13+, pnpm, Go 1.25+, and PostgreSQL 17 with pgvector for persistence testing.

```bash
pnpm install
pnpm dev
```

Build the static console and run the Go service:

```bash
pnpm build
export DOKOSOKO_DATABASE_URL='postgres://...'
export DOKOSOKO_MASTER_KEY='...'
export DOKOSOKO_SETUP_TOKEN='...'
go run ./cmd/dokosoko
```

The UI development server defaults to `http://localhost:3000`; the integrated Go service defaults to `http://localhost:8080` and serves `dist/client`.

To exercise the interactive console against a separately running local Go service, start the UI with `DOKOSOKO_DEV_PROXY=http://127.0.0.1:8080 pnpm dev`. The development server proxies only DokoSoko protocol and API paths; it does not affect production builds.

For documentation screenshots and local design review, append `?preview=fixtures` in development to render the seeded console without API calls.

Run the crawler worker separately against the same database and artifact directory:

```bash
pnpm crawler
```

## Identity and authorization contract

The client-facing flow is:

```text
MCP/widget → DokoSoko OAuth → vendor OIDC → entitlement hook
           → product-bound DokoSoko token → Private MCP
```

The vendor entitlement hook answers which commercial/account features are enabled during authorization. It narrows discovery; it does not define DokoSoko tools. Before each configured custom tool operation, DokoSoko can call a separate authorization hook with product, tool, subject, vendor organisation, and argument names only. Argument values and the Doko token are not sent.

All downstream redirect URIs are exact allowlist entries. Authorization code and access-token records are stored by digest, expire quickly, and are product-bound.

For imported upstream MCP tools, DokoSoko completes its own entitlement, confirmation, schema, and per-operation authorization checks first. It then calls the upstream with either a separate service credential or a delegated OAuth grant encrypted and keyed by `connection + issuer|subject`. See the console’s **MCP connections** workflow and the documentation guide for the review and drift lifecycle.

## Provider API contract

Projects and credentials use a standard vendor-owned HTTPS contract. DokoSoko calls it with an encrypted server-side service credential:

```text
POST /v1/authorize
POST /v1/projects
POST /v1/credentials
POST /v1/credentials/{credential_id}/revoke
```

Every mutation calls `/v1/authorize` first and fails closed. Requests are product-, environment-, subject-, and idempotency-scoped. A newly issued credential is returned to the authorized MCP caller once; DokoSoko stores only its SHA-256 fingerprint, lease metadata, expiry, and revocation state.

Custom vendor actions that do not fit this lifecycle belong in the no-code custom tool proxy, not in the Provider API.

## Package delivery

- `public`: DokoSoko returns/streams a fixed public artifact URL.
- `proxy`: DokoSoko authenticates to a fixed upstream and streams the response.
- `fetch`: DokoSoko calls a fixed vendor hook for a short-lived artifact URL, checksum, and size, then streams it.

All package modes start private and unpublished. Proxy/fetch credentials are encrypted. Outbound destinations are DNS/IP validated, redirects are disabled or revalidated, sizes are bounded, and configured SHA-256 checksums are enforced.

## Verification

```bash
pnpm test:all
docker compose config
docker compose build
```

The suites cover publication boundaries, Public MCP isolation and rate limits, strict Stateless MCPv2 request/response metadata, third-party catalog import and schema drift, separate service/delegated upstream identity, root MFA/session/CSRF controls, crawler SSRF and injection quarantine, encrypted secret handling, OAuth PKCE and product binding, entitlement and authorization fail-closed behavior, package integrity, custom tool schema/policy execution, Provider API issuance/revocation, integration-run analytics, audit, and the production static export.

## API and repository map

The DokoSoko control-plane contract is [api/openapi.yaml](api/openapi.yaml). Vendors use the separate [Provider API contract](api/provider-openapi.yaml) for projects and credentials and the [Vendor Hooks contract](api/hooks-openapi.yaml) for entitlements, tool authorization, and fetch-mode packages. Important protocol endpoints are:

| Surface | Endpoint |
| --- | --- |
| Health/readiness | `/healthz`, `/readyz` |
| Browser administration | `/api/v1/...` |
| Managed MCP imports | `/api/v1/products/{product_id}/mcp-connections/...` |
| OAuth broker | `/oauth/authorize`, `/oauth/callback/{product_id}`, `/oauth/token` |
| Upstream OAuth callback | `/oauth/upstream/callback` |
| Private MCP | `/mcp/{product_id}` |
| Public MCP | `/mcp/public/{product_id}` |
| Package gateway | `/artifacts/{product_id}/{package_id}` |
| Widget loaders | `/widgets/{product_id}/public.js`, `/widgets/{product_id}/private.js` |

The MCP & widgets tab copies service-generated endpoint and widget values. The public snippet is unavailable while Public MCP is off; neither snippet contains an access token or upstream credential.

```text
app/                  static Next.js/Catalyst console
cmd/dokosoko/         service entry point and deployment validation
crawler/              isolated Crawlee/Playwright worker
internal/auth/         setup, root accounts, MFA, sessions, and CSRF
internal/identity/     OAuth/OIDC broker, entitlements, and operation authz
internal/packages/     public/proxy/fetch artifact gateway
internal/providers/    project and credential Provider API runtime
internal/mcpbridge/     Stateless MCPv2 import, OAuth grants, drift, and execution
internal/tools/        JSON Schema tool proxy runtime
internal/platform/     validation, state transitions, audit, and analytics
internal/store/        memory/PostgreSQL persistence and migrations
migrations/            checksummed PostgreSQL/pgvector migrations
docs/FINAL_PLAN.md     product, threat model, and acceptance plan
```
