# DokoSoko service

DokoSoko is a self-hosted delivery control plane for a vendor's developer integrations. One installation represents one vendor deployment. It publishes documentation, API contracts, and policy-bound tools through a UI and Stateless MCPv2.

The public contracts are deliberately small:

- [Control Plane API](api/openapi.yaml) — DokoSoko administration, OAuth, and MCP.
- [Customer Identity Integration API](api/identity-integration-openapi.yaml) — the optional delegated-user access evaluation contract.
- [Backend Integration API](api/backend-integration-openapi.yaml) — the separately authenticated service-to-service support delivery contract.
- [Access Provider API](api/provider-openapi.yaml) — optional provider-owned instance and credential lifecycle.
- [Widget Runtime API](api/widget-runtime.openapi.yaml) — short-lived authenticated sessions and streamed assistant replies for embedded customer applications.

Each integration contract is independently deployable and independently code-generatable. A vendor can generate a server stub for the contract it implements; DokoSoko does not require a package to expose an API. If a client library or other package helps consumers use that API, DokoSoko can catalogue bounded metadata for an exact externally hosted release and embed that metadata in an Integration manifest. The registry remains the delivery system; package metadata never becomes a runtime endpoint or per-operation hook.

The widget is an in-product assistant, not an MCP onboarding screen. A trusted customer backend may attach a small current-view snapshot when it creates the authenticated session—for example, `profile` plus display-ready Plan and Account status facts. The assistant answers from that context and reviewed product knowledge immediately. Opaque customer identifiers never enter model context, and MCP is suggested only when the question indicates a useful automation path.

Runnable reference implementations live under `examples/`. Start with the complete [Go backend integration](examples/go-backend-integration/README.md), which demonstrates authenticated, retry-safe support delivery without requiring an SDK. Operators extending a trusted fork can also use the [native tool plugin example](examples/native-tool-plugin/README.md).

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

## Native tool plugins

Native tool plugins are reviewed Go source packages compiled into a DokoSoko
build. They are intended for trusted self-hosted extensions that benefit from
in-process Go implementation while retaining ordinary Tool review, grants,
identity checks, confirmation, idempotency, publication, and audit. Native tools
are Private MCP capabilities only.

Plugins register deterministic manifests at one explicit composition point.
The host gives them namespaced configuration, opaque invocation identity, one
scoped JSON state abstraction, managed HTTPS, structured logging, deadlines,
and concurrency limits. It does not give them raw tokens, SQL connections, or
internal DokoSoko objects.

This is not a sandbox: compiled-in Go code has the process's authority if it
deliberately bypasses the host API. DokoSoko therefore accepts source only,
forbids dynamic loading and precompiled plugin artifacts, supplies a strict
source checker as a review aid, and requires the operator to review all source
and dependencies before rebuilding. See [Native tool plugins](docs/NATIVE_TOOL_PLUGINS.md)
for the authoring, security, identity, state, and deployment contract.

## Package metadata and external delivery

Packages are optional developer artifacts, represented by metadata rather than hosted content. A `package_artifact` records a stable ecosystem and coordinate, canonical unversioned Package URL identity, registry and optional source locations, visibility, and lifecycle. Package URL types must match the ecosystem. Registry, source, provenance, and SBOM locations must use HTTPS, except for loopback HTTP during local development, and cannot contain userinfo, a query, or a fragment. Each immutable `package_release` records an exact version and versioned Package URL with the same artifact identity, an operator-supplied display-only install command, a declared SHA-256, SHA-384, or SHA-512 digest, and optional provenance and SBOM locations.

An Integration binds one exact release. Publication embeds that release's metadata and DokoSoko metadata content hash in the immutable Integration manifest; there is no follow-latest package binding. This lets clients identify a compatible release without turning DokoSoko into a package registry or delivery proxy.

The external registry delivers package bytes and enforces any registry access policy. DokoSoko does not download, host, sign, execute, cryptographically verify, or proxy packages. Its validation covers metadata shape, PURL identity and exact-version consistency, strict URL policy, obvious credential-bearing install-command forms, digest syntax, and deterministic metadata hashing only. Free-text fields are not comprehensive secret-scanning boundaries, so operators must not enter credentials anywhere in package metadata. A separately operated external verifier should fetch the registry bytes and verify the declared digest, any provenance or SBOM claims, and the documented installation procedure before operational use. This is an operator-controlled process: DokoSoko neither records verifier evidence nor enforces that verification occurred. Verifier credentials and results remain outside package metadata.

All package-artifact catalogue fields are editable only while the artifact is a draft; publishing its first release activates it, and published releases are immutable. Creating a public artifact, changing a private draft to public, and publishing each public release require explicit public acknowledgement. A public Integration may bind only public package metadata.

Deprecation requires guidance and may name only an active replacement that already has a published release. It may also record a future sunset, but deprecation makes the artifact unavailable immediately: it cannot publish another release, receive a new binding, or appear in a newly published Integration candidate. Retirement is also immediate, requires guidance and an optimistic current revision, and applies the same replacement rule. Existing bindings remain readable and already-published Integration manifests remain immutable historical records; a later candidate must remove the unavailable package or explicitly bind an available replacement.

## Protocol endpoints

| Surface | Endpoint |
| --- | --- |
| Health and readiness | `/healthz`, `/readyz` |
| OAuth authorization-server metadata | `/.well-known/oauth-authorization-server` |
| MCP protected-resource metadata | `/.well-known/oauth-protected-resource/mcp` |
| OAuth | `/oauth/register`, `/oauth/authorize`, `/oauth/callback`, `/oauth/token` |
| Private MCP | `/mcp` |
| Public MCP | `/mcp/public` |
| Widget runtime | `/v1/widgets/{widgetID}/configuration`, `/v1/widget-sessions`, `/v1/widget-chat` |
| Agent setup prompts | `/agent-setup/private/prompt.md`, `/agent-setup/public/prompt.md` |
| Administration | `/api/v1/...` |

Private MCP requires the optional identity integration and a resource-bound DokoSoko token. Downstream clients use either Client ID Metadata Documents or idempotent RFC 7591 public-client registration; both require PKCE and exact redirect matching, and neither issues a client secret. Public MCP is independent of identity: it is anonymous, read-only, rate-limited, and globally disabled by default. An Integration is private by default and appears publicly only after an explicit per-Integration visibility acknowledgement and publication. The global Public MCP setting is the emergency master switch. Both endpoints implement only the pinned Stateless MCPv2 protocol revision.

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

The experimental embedded widget is unavailable by default. `DOKOSOKO_WIDGETS_ENABLED=true` restores its console surfaces and runtime routes without changing or deleting stored widget configuration.

For the UI development server:

```bash
DOKOSOKO_DEV_PROXY=http://127.0.0.1:8080 pnpm dev
```

The console always uses live API data by default. A fixture-backed visual preview is available only in a development build at `?preview=fixtures`; production builds ignore that flag. Live load failures stay visible and fail closed instead of being filled with sample data.

## Documentation ingestion

The isolated crawler dispatches each source according to its declared kind; it never treats every location as a website URL.

| Source kind | Worker behavior |
| --- | --- |
| `website` | Crawls a credential-free HTTP(S) origin and its same-origin sitemap/links within the configured page and byte budgets. Non-local source URLs may use only ports 80 and 443. |
| `openapi` | Fetches one credential-free HTTP(S) JSON or YAML document, enforces the byte budget, validates its OpenAPI/Swagger, `info`, and `paths` shape, and stores it as one authoritative document. |
| `upload` | Reads one UTF-8 Markdown, text, HTML, JSON, or YAML file from the dedicated read-only `DOKOSOKO_UPLOAD_DIR`. Stored paths are opaque and relative; crawler reads remain canonical, symlink-free, size-bounded, and contained by that directory. |
| `git` | Fails with the actionable `git_source_unsupported` job code. Repository URLs and credentials are never passed to the website crawler; use a website, an HTTPS OpenAPI document, or a reviewed upload. |

Authenticated administrators upload reviewed files with `POST /api/v1/products/{product_id}/sources/upload`. The service streams the multipart file into the private `dokosoko-uploads` volume and creates a private draft source; it does not queue a crawl or publish anything. Compose mounts that volume read/write at `/uploads` in the service and read-only at the same path in the crawler. `DOKOSOKO_UPLOAD_MAX_BYTES` limits the browser upload and should not exceed `DOKOSOKO_CRAWLER_MAX_BYTES`. Outside Compose, set `DOKOSOKO_UPLOAD_DIR` to a dedicated service-writable directory; leaving it unset disables the endpoint.

Private, loopback, link-local, and reserved network resolution is rejected by default. Local integration testing is an explicit exception: set `DOKOSOKO_CRAWLER_ALLOW_LOCALHOST_SUBDOMAINS=true`, set `DOKOSOKO_CRAWLER_LOCALHOST_HOST` to the vendor-owned Compose host-gateway name, and restrict `DOKOSOKO_CRAWLER_LOCALHOST_PORTS` to the ports needed for that test. The same host variable gives the DokoSoko service and crawler access to the local integration target; add further explicit `extra_hosts` entries when testing more than one local vendor. The exception accepts only `*.localhost` names—not bare `localhost` or IP literals—and only when every resolved address is loopback, RFC1918, or IPv6 unique-local. Keep the switch false in production.

## AI providers and recipes

AI is optional. Without it, DokoSoko still fetches, parses, indexes, authorizes, retrieves, and publishes deterministically. With it, two understandable workloads remove the parts that benefit from judgment: **Analysis** examines integration evidence and creates or reviews recipes, while **Assistant** produces fast, grounded answers from evidence DokoSoko has already retrieved and authorized. Provider credentials are stored once per provider; workload profiles only select a connection, model, token limits, and daily budget.

Configure OpenAI, Google, Anthropic, DigitalOcean Gradient, xAI, DeepSeek, or a fixed HTTPS OpenAI-compatible endpoint in **Settings → AI providers**. Environment-managed installations use one provider connection at a time:

The bundled presets follow the current [OpenAI model guidance](https://developers.openai.com/api/docs/guides/latest-model), [Gemini model catalog](https://ai.google.dev/gemini-api/docs/models), [Claude model catalog](https://platform.claude.com/docs/en/about-claude/models/overview), [DigitalOcean model catalog](https://docs.digitalocean.com/products/inference/details/models/), [xAI model catalog](https://docs.x.ai/developers/models), and [DeepSeek model catalog](https://api-docs.deepseek.com/quick_start/pricing/). They are defaults, not a hidden routing service; every model ID remains visible and editable.

```bash
# OpenAI defaults: Terra for strong analysis and Luna for economical answers.
export DOKOSOKO_AI_PROVIDER=openai
export DOKOSOKO_AI_API_KEY='...'
export DOKOSOKO_AI_MODEL_ANALYSIS=gpt-5.6-terra
export DOKOSOKO_AI_MODEL_ASSISTANT=gpt-5.6-luna
```

```bash
# Google defaults use stable Gemini models.
export DOKOSOKO_AI_PROVIDER=google
export DOKOSOKO_AI_API_KEY='...'
export DOKOSOKO_AI_MODEL_ANALYSIS=gemini-3.5-flash
export DOKOSOKO_AI_MODEL_ASSISTANT=gemini-3.5-flash-lite
```

```bash
# Anthropic defaults use Sonnet for analysis and Haiku for answers.
export DOKOSOKO_AI_PROVIDER=anthropic
export DOKOSOKO_AI_API_KEY='...'
export DOKOSOKO_AI_MODEL_ANALYSIS=claude-sonnet-5
export DOKOSOKO_AI_MODEL_ASSISTANT=claude-haiku-4-5
```

```bash
# DigitalOcean Gradient uses a scoped serverless inference key.
export DOKOSOKO_AI_PROVIDER=digitalocean
export DOKOSOKO_AI_API_KEY='...'
export DOKOSOKO_AI_MODEL_ANALYSIS=openai-gpt-5.6-terra
export DOKOSOKO_AI_MODEL_ASSISTANT=openai-gpt-5.6-luna
```

```bash
# xAI defaults to frontier Grok for analysis and the lower-cost stable model for answers.
export DOKOSOKO_AI_PROVIDER=xai
export DOKOSOKO_AI_API_KEY='...'
export DOKOSOKO_AI_MODEL_ANALYSIS=grok-4.6
export DOKOSOKO_AI_MODEL_ASSISTANT=grok-4.3
```

```bash
# DeepSeek defaults to V4 Pro for analysis and V4 Flash for answers.
export DOKOSOKO_AI_PROVIDER=deepseek
export DOKOSOKO_AI_API_KEY='...'
export DOKOSOKO_AI_MODEL_ANALYSIS=deepseek-v4-pro
export DOKOSOKO_AI_MODEL_ASSISTANT=deepseek-v4-flash
```

For an OpenAI-compatible service, also set `DOKOSOKO_AI_ENDPOINT` to its fixed public HTTPS origin and provide the model IDs it exposes. Native provider origins are fixed and cannot be redirected. DokoSoko sends no model tools, treats retrieved content as untrusted, requires structured analysis, and records usage and normalized failures.

The console can designate one separately configured provider as a backup. DokoSoko tries it once only after a timeout, rate limit, or provider outage. It does not hide invalid credentials, invalid configuration, unsupported models, exhausted quotas or budgets, unsafe input, or invalid model output. Every primary and backup attempt is recorded with its provider role and fallback reason.

Recipes are immutable, versioned Markdown revisions. Generated content is visibly marked, validated, reviewed, and held for human approval. Published recipes appear as MCP resources with stable `dokosoko://products/{product}/recipes/{recipe}` URIs. A recipe may link to an exact canonical documentation or sample-code page only when that published page was included in the analysis evidence; there is no arbitrary URL-fetch tool and no automatic publication.

Integration analysis uses bounded API manifests, tool schemas and authorization policy, identity configuration, and excerpts from already-published knowledge. When the Analysis workload is enabled, that material is sent to its configured provider as untrusted evidence: at most three documents and 6,000 characters per source, with a 32,000-character total cap across knowledge, APIs, and tools. The exact evidence fingerprint is retained as the recipe dependency so a changed crawl or contract moves published guidance into the attention queue.

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
| `DOKOSOKO_PUBLIC_URL` | Exact external origin used for OAuth metadata, MCP endpoints, setup prompts, and copied embed HTML. HTTPS is required outside localhost. |

Native plugin deployment controls are optional:

| Variable | Purpose |
| --- | --- |
| `DOKOSOKO_PLUGIN_<PLUGIN_ID>_<KEY>` | One manifest-registered plugin value. The console exposes only the key name and configured/missing state. |
| `DOKOSOKO_NATIVE_PLUGINS_REQUIRED` | Comma-separated plugin IDs that must be registered and active for startup to succeed. |
| `DOKOSOKO_NATIVE_PLUGINS_DISABLED` | Comma-separated deployment-owned plugin kill switches. |

Back up PostgreSQL, `dokosoko-data`, `dokosoko-uploads`, and the encryption key as one recovery unit. Production should terminate TLS at a trusted reverse proxy and preserve the configured public origin exactly. Configure the browser-reachable origin here—not an internal container hostname—because copied setup buttons deliberately ignore request `Host` and forwarding headers.

The supplied service and crawler containers run as UID/GID `65532`, drop Linux capabilities, use a read-only root filesystem, and write only to their declared data, upload, and temporary mounts. Compose gives the service 30 seconds to complete its graceful shutdown before termination; keep at least that allowance in other orchestrators.

Database migrations are append-only public deployment history. Never edit, rename, or delete an existing migration; add a new uniquely numbered migration for every schema change. Repository and runtime checksum validation enforce this policy.

### Breaking v3 upgrade

Migration `0020_contract_v3.sql` deliberately removes the legacy package gateway and open-ended hook contracts. The later package catalogue is a metadata-only contract and does not restore package hosting, download, authentication, or proxy behavior. Back up the database and encryption key before deploying it.

The migration preserves installations, customer version pins, and encrypted support submissions. It maps legacy customer identifiers to durable customer accounts. It invalidates outstanding OAuth states, authorization codes, and access tokens; removes identity configuration whose delegated API origin cannot be inferred safely; creates disabled backend-connection placeholders for legacy support routes; and defaults every Integration to private.

After the upgrade, configure customer identity only if private customer access is required. Configure or repair each backend connection, create a fresh credential, attach it to the intended support routes, and then re-enable delivery. Legacy hook credentials are not rebound because their encryption purpose and trust boundary differ. Existing MCP clients must authenticate again. Legacy package-gateway credentials, stored bytes, and proxy configuration are not migrated because they have no faithful representation in the metadata-only catalogue. Before creating new bounded package metadata and exact Integration bindings, operators should use a separately operated verifier to check the corresponding registry release. That verification is an operational prerequisite, not a condition enforced or evidenced by DokoSoko.

## Verification

```bash
pnpm run verify
docker compose config
```

`pnpm run test:all` remains an alias for `pnpm run verify`. The verification command runs type checking, lint, behavioral and rendered-console tests, crawler isolation tests, a production console build, and the complete Go suite.

PostgreSQL integration tests run automatically when either `DOKOSOKO_TEST_DATABASE_URL` or `TEST_DATABASE_URL` is set. Use a disposable PostgreSQL 17 database with pgvector; the test suite applies the checked migrations and cleans its own organisation-scoped fixtures.

```bash
export DOKOSOKO_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5432/dokosoko_test?sslmode=disable'
go test ./...
```

For a native plugin source package, also run its conformance tests and source
admission check before adding it to the explicit registry:

```bash
go test ./path/to/plugin
go run ./cmd/dokosoko-native-plugin-check ./path/to/plugin
```

The suites cover OAuth resource and PKCE binding, durable customer-account resolution and suspension, fail-closed grants, fixed tool destinations, managed MCP delegation and drift, support consent and idempotent outbox delivery, atomic catalog mutations, cancellation-safe idempotent audits, publication boundaries, access-provider operations, authentication, analytics, crawler isolation, and static console output. GitHub Actions repeats the frontend, PostgreSQL-backed Go, Compose, and container-build checks for every pull request and `main` update.

## Repository map

```text
app/components/console/ lazy-loaded console domain workspaces
app/lib/              API contracts/client, routes, and pure console domain logic
cmd/dokosoko/         service entry point
crawler/              isolated documentation crawler
internal/identity/    downstream OAuth, upstream OIDC, customer accounts, grants
internal/reporting/   encrypted support outbox and delivery
internal/tools/       policy-bound HTTP tool execution
internal/mcpbridge/   managed MCP import, delegated OAuth, drift, execution
internal/nativeplugins/ native plugin registry, lifecycle, catalog, and managed host capabilities
internal/nativepluginstate/ scoped state binding and upgrade coordination
nativeplugin/         public native plugin SDK, conformance helpers, and source checker
internal/access/      provider-owned instance and credential lifecycle
internal/lifecycle/   process supervision and graceful shutdown coordination
internal/netpolicy/   shared outbound network and DNS-rebinding policy
internal/ratelimit/   bounded fixed-window limiting
internal/model/       domain-split persisted and API model contracts
internal/platform/    domain-split catalog validation, state transitions, audit
internal/store/       capability interfaces and domain-split memory/PostgreSQL persistence
migrations/           checksummed PostgreSQL migrations
api/                  normative OpenAPI contracts
.github/workflows/    repeatable console, crawler, Go/PostgreSQL, and image checks
```

See [the contract plan](docs/FINAL_PLAN.md) for invariants and failure semantics.
