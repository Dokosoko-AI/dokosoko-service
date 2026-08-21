# DokoSoko service

DokoSoko is a self-hosted, UI-first delivery control plane for coding agents. One DokoSoko installation represents one vendor deployment. Inside it, first-class **APIs** represent individual API families and versions such as Voice API v2 or Face API v3. `Integration` remains an internal compatibility name in parts of the API and storage model, not a customer-facing concept.

The implementation follows [the final product plan](docs/FINAL_PLAN.md). Its key security boundary is simple: private by default, explicit and audited publication, vendor-controlled entitlements, separate per-operation authorization, and no persistent credential exposure to browsers or MCP clients.

## Implemented product surface

- Static Next.js console built on Catalyst-style UI primitives and served by the Go service.
- First-run setup with a one-time setup token, Argon2id passwords, TOTP MFA, one-use recovery codes, secure sessions, CSRF, and root-user lifecycle management.
- PostgreSQL/pgvector persistence with checksummed startup migrations and a database-backed readiness check.
- A singleton Deployment with production/non-production environments. The legacy Product API remains as a compatibility facade over that Deployment.
- First-class APIs keyed by API family + version, with draft/active/deprecated/retired lifecycle, immutable published history, replacement/sunset metadata, and stable manifest hashes.
- Reusable, independently revisioned documentation, package, and tool-backend resource sets. APIs may deliberately share a set, follow its latest revision, pin a revision, or duplicate it before diverging.
- Advanced publishing retains immutable compatibility snapshots, generated diffs, Latest/LTS/Preview/deprecated lifecycle, deterministic rollout, artifact-drift shutdown, separation-of-duties promotion, scoped installation/environment/customer pins, and immutable assignment history without making release machinery part of the routine workflow.
- Sources with sitemap-first Crawlee discovery, Cheerio fast path, Playwright fallback, immutable snapshots, incremental HTTP metadata, SSRF controls, crawl budgets, prompt-injection scanning, quarantine, review, and atomic publication.
- npm, Go, Git, Maven/Java, Android, Swift, and NuGet/C# package records in public-link, credential-backed proxy, and short-lived download-service modes.
- Custom MCP tools defined by name, JSON input/output Schema, fixed HTTPS tool backend, encrypted credential, entitlement policy, confirmation policy, schema validation, draft/publish lifecycle, and audited execution.
- Managed third-party MCP imports with the [Stateless MCPv2 Only](https://blog.modelcontextprotocol.io/posts/2026-07-28/) `2026-07-28` contract, fixed HTTPS upstreams, complete catalog inspection, explicit selection, namespacing, schema pins, drift shutdown, and local draft/publish review.
- Separate upstream identity modes: an encrypted service credential or delegated OAuth grants bound to the canonical DokoSoko subject. The inbound DokoSoko token is never forwarded.
- OAuth 2.0 authorization-code + PKCE broker from MCP/widget clients through DokoSoko to the vendor OIDC provider.
- Vendor entitlement resolution during login, an independent per-operation authorization endpoint, and an ephemeral customer-defined usage endpoint.
- Consent-gated `support.report_bug` and `support.submit_feedback` Private MCP tools with fixed server/tool instructions, API-specific reporting policies, immutable trusted API context, encrypted durable holding, secret detection, bounded retention, and separate idempotent support webhooks.
- Provider-owned access definitions and connections. The provider declares whether a service has one fixed instance or many user-created resources (for example Auth0 tenants/projects), plus connection- or instance-scoped credential issuance and revocation.
- Private MCP with product-bound DokoSoko tokens; Public MCP is anonymous, read-only, public-only, rate-limited, and off by default.
- Owner-scoped API runs with deterministic completion, analytics funnel events, and an append-only audit feed.
- Analytics for authorized/active users, MCP channels, tools, packages, API runs, validated success, and daily volume without storing raw queries or secrets.
- Role-based LLM profiles with mandatory untrusted-context controls: no model authority, no model tool execution, citations required, bounded input/output, and no answer on low confidence.
- System Doctor and hardened container deployment.

## Architecture

```text
Static web console ─┐
Private MCP ────────┼──> Go control plane ──> PostgreSQL + encrypted artifact data
Public MCP ─────────┤           │
Widget loaders ─────┘           ├──> isolated Crawlee/Playwright worker
                                └──> vendor IdP, endpoints, packages, tools, Provider API,
                                     reviewed third-party MCPv2 upstreams
```

DokoSoko owns the downstream OAuth token, publication policy, audit, analytics, package streaming, MCP discovery, and runtime authorization. It never forwards the inbound DokoSoko token to a vendor API. Server-side calls use separately encrypted vendor credentials.

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
MCP/widget → DokoSoko OAuth → vendor OIDC → entitlements endpoint
           → product-bound DokoSoko token → Private MCP
```

The vendor entitlements endpoint answers which commercial/account features are enabled during authorization. It narrows discovery; it does not define DokoSoko tools. An optional signed installation claim maps the principal to one registered customer/environment installation for version resolution. Before each configured custom tool operation, DokoSoko can call a separate authorization endpoint with deployment, tool, subject, vendor organisation, installation, and argument names only. Argument values and the Doko token are not sent. When configured once under **Settings → Identity & customer account data**, Private MCP exposes `usage.get`, which calls a deployment-wide usage endpoint for the authenticated subject and proxies up to 50 ordered scalar values without calculating, caching, or persisting them. Labels such as Used, Available, Next renewal, or Trial completion are examples only; the vendor controls each key, label, scalar value, format, unit, description, and order.

All downstream redirect URIs are exact allowlist entries. Authorization code and access-token records are stored by digest, expire quickly, and are product-bound.

For imported upstream MCP tools, DokoSoko completes its own entitlement, confirmation, schema, and per-operation authorization checks first. It then calls the upstream with either a separate service credential or a delegated OAuth grant encrypted and keyed by `connection + issuer|subject`. See the console’s **MCP connections** workflow and the documentation guide for the review and drift lifecycle.

## Bug reports and feedback

Reporting policies can independently enable `support.report_bug` and `support.submit_feedback` for a specific API or as the deployment default. Discovery and each tool definition carry a platform-owned instruction: the agent must preview the exact sanitized report, obtain explicit user approval, preserve user-authored feedback faithfully, and avoid secrets, complete files, unrelated conversation, or unapproved contact information. Execution also requires `_meta.confirmed=true`; the instruction is never the only control.

Approved submissions enter an encrypted durable outbox. A reporting policy may use DokoSoko as a holding inbox without a webhook, or configure separate fixed HTTPS bug and feedback webhooks with independently encrypted service credentials. Each submission pins the selected API, family, version, published revision, manifest hash, and policy at submission time. Delivery uses the submission ID as an idempotency key, retries with bounded backoff, records only sanitized status metadata in plaintext, and deletes expired submissions according to the configured 1–365 day retention window. Public MCP never discovers or executes these tools.

## Provider-owned access contract

Advanced service types describe the vendor service contract; Service Connections configure a particular vendor account and explicitly attach it to allowed APIs. The service type sets `instance_cardinality`:

- `one`: the connected service is the only instance, so no instance-creation tool is exposed;
- `many`: authenticated users may create/list provider resources labelled by the vendor, such as Project, Tenant, Workspace, or Account.

Credentials are scoped either to the connection or to one provider instance. DokoSoko calls fixed HTTPS operations with a separately encrypted server-side management credential:

```text
POST /v1/authorize
POST /v1/instances
POST /v1/credentials
POST /v1/credentials/{credential_id}/revoke
```

Every mutation calls the configured authorization operation first and fails closed. Requests are deployment-, API-, environment-, subject-, owner-, and idempotency-scoped. A newly issued credential is returned to the authorized MCP caller once by default; DokoSoko lists only its fingerprint, scope, lifecycle, expiry, and revocation state. Managed encrypted storage and provider-reference modes are available when the definition explicitly selects them.

Operation paths are declared by the Access Definition; the paths above are the reference contract, not hard-coded DokoSoko routes. Custom vendor actions that do not fit this lifecycle belong in the no-code custom tool proxy, not in the Access Provider API.

## Package delivery

- `public`: DokoSoko returns/streams a fixed public artifact URL.
- `proxy`: DokoSoko authenticates to a fixed upstream and streams the response.
- `download`: DokoSoko calls the vendor's versioned `POST /v1/package/download` endpoint for a short-lived artifact URL, exact checksum, and exact size, then streams it.

All package modes start private and unpublished. Proxy/download credentials are encrypted. Outbound destinations are DNS/IP validated, redirects are disabled or revalidated, sizes are bounded, and configured SHA-256 checksums are enforced.

## Verification

```bash
pnpm test:all
docker compose config
docker compose build
```

The suites cover publication boundaries, deployment/release discovery, scoped selection and pins, manifest integrity and diffs, preview isolation, rollouts, deprecation impact, artifact drift, promotion separation of duties, LLM budgets, Public MCP isolation and rate limits, strict Stateless MCPv2 request/response metadata, third-party catalog import and schema drift, separate service/delegated upstream identity, root MFA/session/CSRF controls, crawler SSRF and injection quarantine, encrypted secret handling, OAuth PKCE and deployment binding, entitlement and authorization fail-closed behavior, private-only bounded usage proxying, consent-gated encrypted support reporting and idempotent delivery, package integrity, custom tool schema/policy execution, provider access issuance/revocation, API-run analytics, audit, and the production static export.

## API and repository map

The DokoSoko control-plane contract is [api/openapi.yaml](api/openapi.yaml). Vendors use the separate [Access Provider API contract](api/provider-openapi.yaml) for authorization, optional provider-owned instance management, credential creation, and revocation; [api/hooks-openapi.yaml](api/hooks-openapi.yaml) covers vendor integration APIs for entitlements, usage reports, support reporting, tool authorization, and the normative `POST /v1/package/download` contract. Important protocol endpoints are:

| Surface | Endpoint |
| --- | --- |
| Health/readiness | `/healthz`, `/readyz` |
| Deployment | `/api/v1/deployment`, `/api/v1/environments` |
| API catalog and reusable sets | `/api/v1/integrations/...`, `/api/v1/resource-sets/...` |
| Access management | `/api/v1/access-definitions`, `/api/v1/access-connections/...` |
| Support routes | `/api/v1/support-routes/...` |
| Browser administration compatibility APIs | `/api/v1/products/...` |
| Managed MCP imports | `/api/v1/products/{product_id}/mcp-connections/...` |
| OAuth broker | `/oauth/authorize`, `/oauth/callback/{product_id}`, `/oauth/token` |
| Upstream OAuth callback | `/oauth/upstream/callback` |
| Private MCP | `/mcp` (legacy alias: `/mcp/{deployment_id}`) |
| Public MCP | `/mcp/public` (legacy alias: `/mcp/public/{deployment_id}`) |
| Package gateway | `/artifacts/{product_id}/{package_id}` |
| Widget loaders | `/widgets/{product_id}/public.js`, `/widgets/{product_id}/private.js` |

The Agent access page copies service-generated endpoint and widget values. The public snippet is unavailable while Public MCP is off; neither snippet contains an access token or upstream credential.

```text
app/                  static Next.js/Catalyst console
cmd/dokosoko/         service entry point and deployment validation
crawler/              isolated Crawlee/Playwright worker
internal/auth/         setup, root accounts, MFA, sessions, and CSRF
internal/identity/     OAuth/OIDC broker, entitlements, usage, and operation authz
internal/packages/     public/proxy/download artifact gateway
internal/providers/    legacy Provider API compatibility runtime
internal/access/       provider-owned instance and credential runtime
internal/reporting/    encrypted reporting outbox, validation, delivery, and retries
internal/mcpbridge/     Stateless MCPv2 import, OAuth grants, drift, and execution
internal/tools/        JSON Schema tool proxy runtime
internal/platform/     validation, state transitions, audit, and analytics
internal/store/        memory/PostgreSQL persistence and migrations
migrations/            checksummed PostgreSQL/pgvector migrations
docs/FINAL_PLAN.md     product, threat model, and acceptance plan
```
