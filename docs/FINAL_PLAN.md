# DokoSoko — Final Plan

## Product definition

DokoSoko is a vendor-operated delivery control plane for coding agents. It turns a vendor's product knowledge, SDK packages, API capabilities, and privileged developer operations into a versioned connector that agents can discover and use safely.

The v2 product is UI-first: every routine configuration task is available in the web console, while an API and declarative configuration format remain available for automation. The UI is a statically built Next.js application based on Catalyst UI patterns and is served or reverse-proxied by the Go control plane.

## Product principles

1. Private by default. New sources, packages, tools, connectors, and credentials are never public implicitly.
2. Explicit publication. Public access is a reviewed state transition with a warning, confirmation, actor, timestamp, and audit record.
3. Separate identity, entitlement, consent, and policy. A successful login is not sufficient authority for an operation.
4. Server-side secrets. Browser and MCP clients never receive persistent vendor credentials.
5. Versioned, reversible configuration. Draft, validate, publish, and roll back are first-class operations.
6. Untrusted knowledge. Crawled or uploaded content can inform an answer but cannot authorize actions, expose secrets, or redefine tools.
7. Measured outcomes. Success means a validated integration outcome, not merely a tool call or generated answer.

## Implemented v2 baseline

This repository now contains an executable v2 baseline, not only a design document. The following proposed changes are implemented end to end:

- UI-first static Next.js/Catalyst console served by the Go control plane;
- first-run deployment, database migrations, encrypted secrets, root creation, MFA, recovery, secure sessions, CSRF, additional-root lifecycle, readiness, and System Doctor;
- organisations, products, environments, sources, crawl queues/state polling, review/publication controls, packages, tools, identity, providers, projects, credential leases, integration runs, analytics, audit, and LLM profiles in the web UI/API;
- sitemap-first Crawlee/Playwright worker with incremental metadata, immutable snapshots, scope/budget limits, DNS/IP defenses, hidden/encoded instruction detection, quarantine, and atomic source state changes;
- private-first, separately published packages for npm, Go, Git, Maven/Java, Android, Swift, and NuGet/C#, with generic public-link, authenticated proxy, and fetch-hook artifact delivery;
- no-code custom tool definitions with JSON Schemas, fixed HTTPS API hook, encrypted credential, entitlement/confirmation policy, safe runtime validation, versioned publication, and audit;
- managed third-party MCP imports using the [Stateless MCPv2 Only](https://blog.modelcontextprotocol.io/posts/2026-07-28/) `2026-07-28` contract, complete catalog review, explicit namespaced selection, local schema pins, drift shutdown, post-authz execution, and separate service or subject-bound delegated upstream credentials;
- OAuth authorization-code + PKCE brokerage through the vendor OIDC provider, exact redirect allowlists, nonce/ID-token validation, login-time entitlement resolution, opaque product-bound DokoSoko tokens, and separate per-operation authorization;
- standard Provider API hooks for authorize, project create, credential issue, and revoke, with idempotency and one-time credential delivery;
- Private MCP, default-off/read-only Public MCP, secret-free widget loaders, package delivery, owner-scoped integration runs, deterministic success analytics, and append-only audit;
- mandatory LLM/knowledge hardening: content has no authority, model tool calls and authorization are disabled, retrieval is scoped before ranking, citations are required, low confidence returns no answer, and suspicious sources are quarantined.

The executable API contract is [`api/openapi.yaml`](../api/openapi.yaml); deployment and vendor integration instructions are in [`README.md`](../README.md).

### Production follow-ups

The baseline deliberately leaves these scale or ecosystem-specialized items as follow-up releases rather than pretending a generic implementation is sufficient:

1. Native registry-protocol adapters and install-command UX for each package ecosystem. The current gateway provides secure artifact records and streaming for every named ecosystem, but it is not yet a drop-in npm, Maven, NuGet, Go module, or Swift registry.
2. Git clone and uploaded-file ingestion adapters. Website/OpenAPI URL crawling is operational; Git/upload source records require their dedicated workers.
3. A complete embeddable chat SDK. The current public/private scripts are secret-free launchers that hand off through `dokosoko:open`; origin/theme/locale configuration and the hosted chat surface remain to be built.
4. Advanced organisation membership/groups, connector-release diff/rollback UI, support elevation, and multi-step tool mappings.
5. S3/KMS adapters, distributed job control/dead letters, OpenTelemetry, scheduled rollups, automated backup/restore drills, support bundles, upgrades, and disaster-recovery automation.
6. A production adversarial evaluation corpus and external penetration review. Runtime guardrails and unit/integration security tests are present, but these release gates require deployment-specific evidence.

These follow-ups do not weaken the current private/public, identity, authorization, secret, or tenant boundaries. They are required before describing the baseline as a horizontally scaled general-availability service.

## Runtime architecture

```text
Browser console ─────── REST/JSON ───────────────┐
Private MCP client ─── Doko OAuth token ─────────┤
Public MCP client ──── no authentication ────────┤
Public/private widget ─ scoped session ──────────┤
                                                 ▼
                                      Go control plane
                    ┌────────────────────┼─────────────────────┐
                    ▼                    ▼                     ▼
              PostgreSQL          object storage       provider runtime
                    ▲                    ▲                     │
                    │                    │                     ▼
             crawler jobs ◄──── Crawlee/Playwright      vendor APIs/IdP
```

The control plane owns authorization, publication, connector state, audits, analytics events, package streaming, and MCP protocol handling. A dedicated TypeScript worker owns crawling and browser rendering. PostgreSQL is the source of truth. Object storage contains immutable source snapshots and large package/cache artifacts.

## Default deployment

The supported self-hosted deployment is Docker Compose with:

- `dokosoko`: Go control plane and static console;
- `crawler`: TypeScript Crawlee worker with HTTP and Playwright paths;
- `postgres`: PostgreSQL with pgvector;
- a persistent data volume, with S3-compatible object storage and KMS optional.

The minimum bootstrap configuration is the database URL, master encryption key or KMS reference, one-time setup token, public base URL, and storage path. There is no default administrator password.

## Console information architecture

- Overview
- Sources
- Knowledge
- Packages
- Projects & credentials
- Tools
- MCP & widgets
- Integration runs
- Analytics
- Activity & audit
- Organisation settings
- Platform settings (root only)

## Functional scope

### 1. Bootstrap and root administration

The first-run setup flow validates the database, applies migrations, configures encryption, creates the first root administrator, requires MFA enrollment, sets the public URL and storage, and optionally configures email and LLM profiles. Platform root and organisation owner are separate roles. Access to tenant content by platform support requires explicit, time-limited, audited elevation.

The System Console covers database, storage, AI, identity, email, crawling, package gateway, security, observability, backups, updates, organisations, root users, and audit. System Doctor performs non-destructive connectivity and configuration checks and can generate a redacted support bundle.

### 2. Organisations, products, and environments

An organisation owns products. A product owns independently versioned connector releases and one or more environments. Membership uses role assignments and groups. Environment overrides cannot silently broaden a product's published permissions.

### 3. Sources and crawling

Sources include websites, OpenAPI documents, Git repositories, uploaded files, SDK references, and package metadata. The Sources tab exposes configuration, credentials, latest crawl, discovered pages, fetch/render method, errors, diffs, quarantine state, and publication state.

The crawler uses sitemap-first discovery, an HTTP fast path, Playwright fallback, ETag and Last-Modified incremental requests, canonical URL normalization, scope rules, request budgets, and immutable raw/rendered snapshots. It extracts meaningful content, computes changes, scans for prompt injection and hidden text, and publishes atomically after validation or review.

All outbound fetches enforce DNS and IP validation, redirect revalidation, protocol allowlists, tenant request budgets, response size limits, and isolated browser contexts. Private source credentials are encrypted and never enter the knowledge index.

### 4. Knowledge publication and retrieval

Content moves through `draft → validated → published`, with `quarantined` and `retired` side states. Published snapshots are immutable and rollbacks are atomic. Retrieval filters organisation, product, environment, version, visibility, and publication state before lexical, vector, or reranking work.

Knowledge answers carry citations and provenance. Low-confidence retrieval can return no answer. Private and public indexes may share storage, but public queries must have an independent visibility filter in the authoritative query path.

### 5. Packages

Packages support npm, Go, Git, Java/Maven, Android/AAR, Swift, and C#/NuGet. A package is public or private, defaults to private, and supports:

- public metadata/link mode;
- proxy mode, where DokoSoko stores the upstream credential and streams the package;
- fetch mode, where DokoSoko calls a vendor hook for a short-lived URL, expected checksum, and size.

Any mode may be marked public after the guarded warning and confirmation. For proxy and fetch modes this explicitly authorizes anonymous access through DokoSoko; public rate limits, download budgets, checksum enforcement, and rapid disable controls are mandatory.

Initial scope is read/install only. Package credentials are short-lived, product/environment scoped, revocable, rate-limited, and audited. Streams enforce content length and checksum when available. Persistent upstream credentials are never returned to clients.

### 6. Projects and credential issuance

Project creation and credential issuance are separate resources. Provider implementations are either built-in vendor API adapters, a standard remote DokoSoko Provider API, or a DokoSoko-managed proxy. Operations are idempotent, environment scoped, audited, and expose explicit cleanup/revocation.

Credential leases are short lived and store only a fingerprint after delivery. Projects belong to an organisation, user, integration workspace, or declared lifecycle scope—not to a connector incidentally.

### 7. Vendor identity and entitlements

The default authentication flow is:

```text
MCP or widget → DokoSoko OAuth → vendor IdP → DokoSoko audience token
```

At runtime DokoSoko exchanges or acquires a separate vendor-audience token. It never passes the inbound DokoSoko token to a vendor API. Identities are keyed by OIDC `(issuer, subject)`, not email.

DokoSoko defines and publishes product capabilities. The vendor is authoritative for commercial and account entitlements. Effective authorization is:

```text
published capability
∩ vendor entitlement
∩ vendor user role
∩ connector consent
∩ DokoSoko policy
∩ environment
∩ approval state
```

The standard vendor contract provides batch entitlement resolution for discovery, per-operation authorization checks, credential issuance, and revocation. Mutating, private, or credential operations fail closed when the entitlement service is unavailable.

### 8. Custom API tools

The no-code tool builder configures:

- tool name, description, and namespace;
- JSON input schema;
- reusable API connection and credential;
- URL, method, headers, and safe request mapping;
- output schema and response mapping;
- required entitlement, scopes, confirmation, timeout, and rate limit;
- tests and a versioned publication step.

The initial builder allows one HTTP call per tool and safe expressions such as CEL. It does not allow arbitrary JavaScript, agent-controlled hosts, or agent-controlled authorization headers. Existing MCP servers are imported only through the managed Stateless MCPv2 bridge after tools are explicitly selected, pinned, namespaced, reviewed, and policy wrapped. The bridge retains no logical live session and never forwards an inbound DokoSoko token.

### 9. Private MCP

Private MCP is the default agent interface. It requires a DokoSoko-audience access token, resolves the user and organisation, evaluates connector consent and vendor entitlements, and exposes only the applicable knowledge, packages, tools, projects, and credential actions. Token passthrough is forbidden.

### 10. Public MCP

Public MCP is a product-level, authentication-free endpoint and is **off by default**. Enabling it requires an explicit warning and confirmation and creates an audit event. Disabling it takes effect immediately.

Public MCP exposes read-only discovery and retrieval tools only. Its authoritative query path includes only:

- published source records marked `public`;
- published package records marked `public`.

Custom API tools, project provisioning, private package modes, identity data, entitlement data, credentials, and integration-run details are never exposed through Public MCP in the initial release. Public responses are rate-limited, abuse monitored, citation-bearing, and cacheable only where tenant isolation is preserved.

### 11. Visibility safety

Every source and package is `private` by default. A change from private to public is a guarded publication operation, not a casual switch:

1. show which product, environment, record, and currently published content will become anonymous;
2. explain that public MCP and the public widget may expose it without login;
3. require a positive confirmation value in both UI and API;
4. validate that the record contains no configured credentials or quarantined content;
5. write the actor, prior state, new state, revision, and timestamp to audit;
6. invalidate public retrieval caches after commit.

Changing public to private requires no warning and takes effect immediately. The API rejects a public transition without the explicit acknowledgement, even if a client bypasses the UI.

### 12. Widgets

The MCP & widgets console has a dedicated Copy widget section with two independently copyable snippets:

- **Public widget:** no sign-in, uses only Public MCP and public published sources/packages, and is unavailable while Public MCP is disabled.
- **Private widget:** starts an authenticated DokoSoko session and can use private knowledge and authorized actions according to the same policy as Private MCP.

Widget configuration includes allowed origins, product/environment, theme, launcher text, locale, privacy notice, rate limit, and Content Security Policy guidance. Embed snippets contain public identifiers only; secrets are never embedded.

The console obtains endpoint and loader URLs from DokoSoko's distribution API rather than embedding a deployment hostname, so copied snippets always target the active service origin.

### 13. LLM profiles and hardening

LLMs are optional and configured by profile: embedding, extraction, reranking, evaluation, and optional UI assistant. Providers may be hosted, OpenAI-compatible, or local. Configuration includes encrypted credentials, budgets, model tests, embedding dimension validation, and controlled re-embedding.

Crawled and uploaded content is always untrusted. It cannot grant authority, reveal secrets, choose network destinations, add tools, or modify system policy. Models have no direct database, network, or secret-store access. All tool calls are schema validated and allowlisted. Injection indicators, hidden-text checks, provenance, trust levels, quarantine, no-answer behavior, and adversarial evaluation gates are part of publication.

The evaluation corpus covers prompt injection, cross-tenant leakage, package substitution, hidden tools, internal URL access, encoded/obfuscated instructions, unsafe citations, and data exfiltration attempts.

### 14. Integration runs

An integration run records connector authorization, requested outcome, capability discovery, package access, project/credential operations, validation, reported result, timing, and failure reason. Sensitive tool inputs and outputs are redacted or excluded by policy.

### 15. Analytics

Analytics is separate from Activity & audit. Root administrators see aggregated platform health; organisation/product users see their scoped data. User counts distinguish console users, authorized developers, and agent connectors.

Metrics include DAU/WAU/MAU, integration runs, reported versus validated success, first-pass success, time-to-success, human intervention, MCP/tool volume and latency, zero-result rate, source freshness, package downloads and verification, credential issuance/revocation, LLM tokens/cost, and public-versus-private usage.

The primary funnel is:

```text
connector authorized → run started → capability resolved → package acquired
→ credentials issued → implementation validated → success reported
```

Events are append-only in partitioned PostgreSQL tables with scheduled rollups/materialized views. Raw query text, secrets, private content, and sensitive tool arguments/results are not analytics dimensions by default.

### 16. Audit and operations

Audit is append-only, tenant scoped, exportable, and records actor, action, target, policy decision, revision, request ID, timestamp, and outcome without secret values. Operations include health/readiness endpoints, queue visibility, retry/dead-letter controls, backups/restore verification, version/update status, OpenTelemetry export, and redacted support bundles.

## Core entities

`PlatformConfig`, `RootUser`, `Organisation`, `Membership`, `Product`, `Environment`, `ConnectorRelease`, `VendorIdentityProvider`, `VendorGrant`, `EntitlementSnapshot`, `Policy`, `Source`, `CrawlJob`, `SourceSnapshot`, `KnowledgeDocument`, `Package`, `PackageAccessLease`, `Provider`, `Project`, `CredentialLease`, `APIConnection`, `ToolDefinition`, `ToolRelease`, `MCPConfiguration`, `WidgetConfiguration`, `IntegrationRun`, `AnalyticsEvent`, and `AuditEvent`.

All tenant resources include an organisation ID, immutable ID, revision, creation/update timestamps, and lifecycle state. Visibility-bearing resources default to `private` at both application and database layers.

## Delivery phases

1. Contract: entities, API conventions, threat model, audit/analytics event catalog, and this plan.
2. Deployment and bootstrap: Compose, database migrations, static console serving, setup/root flow, System Doctor.
3. Organisation and product control plane: tenancy, roles, products, environments, versioned connector drafts.
4. Sources and knowledge: crawler worker, snapshots, diff/review, publication, retrieval, citations, visibility.
5. Identity and MCP: OAuth/IdP brokerage, connector grants, private MCP, public MCP, widgets.
6. Packages: ecosystem adapters, proxy/fetch modes, checksums, leases, cache policy.
7. Projects and credentials: provider contract, hooks, idempotency, cleanup, revocation.
8. Custom tools: builder, schemas/mappings, authz, test/publish, runtime proxy.
9. Analytics and evaluation: event pipeline, dashboards, success funnel, adversarial LLM suite.
10. Production hardening: scale, rate limits, backups, disaster recovery, penetration review, upgrade path.

## MVP acceptance criteria

The first production candidate is complete only when all of the following are demonstrated end to end:

1. A fresh deployment completes setup without a default password and creates an MFA-protected root user.
2. An organisation/product/environment and vendor IdP can be configured entirely in the console.
3. A source can be crawled, diffed, quarantined/reviewed, and atomically published with citations.
4. New sources and packages are private at both API and database layers.
5. A public visibility transition is rejected without acknowledgement and audited when confirmed.
6. Public MCP is off on a new product and is unavailable anonymously until explicitly enabled.
7. Public MCP returns only public, published sources and packages and has no privileged tools.
8. Disabling Public MCP or making a record private removes anonymous access immediately.
9. The public widget follows the same public-only policy and the private widget follows authenticated MCP policy.
10. Public and private embed snippets can be copied and contain no secrets.
11. A private package can be installed through proxy and fetch modes with short-lived access and checksum verification.
12. A project and temporary credential can be issued, used, revoked, and audited through a provider adapter or hook.
13. A custom API tool can be defined, tested, published, authorized, executed, and audited without arbitrary code.
14. Vendor identity and entitlements narrow effective access; outages fail closed for private or mutating operations.
15. An adversarial source cannot cause secret access, unauthorized tool calls, cross-tenant retrieval, or arbitrary network access.
16. An integration run reaches a deterministic validation result and feeds the analytics success funnel.
17. Analytics and audit provide distinct, scoped views with no stored secrets or private query text by default.
18. Backup/restore, upgrade, health, and System Doctor paths are documented and verified.
19. A third-party MCPv2 catalog can be inspected, explicitly imported as schema-pinned drafts, published behind DokoSoko authorization, executed with a separate upstream identity, and shut down automatically on schema drift.

## Initial non-goals

- package publishing;
- arbitrary workflow scripting in the tool builder;
- blindly proxying every tool from an upstream MCP server;
- using model output as an authorization decision;
- storing persistent vendor credentials in clients;
- public project, credential, entitlement, or custom-tool operations.
