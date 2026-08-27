# DokoSoko core product contract

DokoSoko is one MCP connector for reviewed documentation, API contracts, exact
SDK releases, runtime keys,
recipes, and reviewed tools. The service is deliberately not a developer
portal, provisioning platform, widget product, support-delivery system, or
product-release orchestrator.

## Core resources

- `deployment` — the single vendor catalog and Public MCP master switch;
- `integration` — one versioned API and its immutable publications;
- `source` and immutable `source_publication` ingestion evidence;
- deployment-owned documentation collections and immutable revisions;
- deployment-owned API contracts and immutable validated revisions;
- deployment-owned SDK packages, exact immutable releases, and reviewed SDK
  content publications;
- typed API documentation, contract, and SDK-release bindings;
- runtime service connections and versioned encrypted credential sets;
- authorization points and reviewed HTTP, MCP, or native tool revisions;
- immutable evidence-grounded recipes;
- customer identity and OAuth artifacts for optional Private MCP;
- upstream MCP connections using a service access token and optional signed
  user-identity forwarding;
- consent-based plaintext queued support submissions;
- append-only audit plus bounded analytics needed for AI budgets and recipe
  popularity.

## Developer-asset ownership and publication model

Documentation, API contracts, and SDK packages are reusable deployment-owned
Catalog assets. An API owns only typed bindings and its immutable publication
snapshot; attaching an asset never creates an API-owned copy.

Global documentation is published independently as an immutable deployment
snapshot of exact documentation collection revisions. An API publication
resolves every active draft binding and snapshots exact documentation
revisions and selectors, exact API-contract revisions, exact SDK releases and
reviewed content publications, content hashes, and visibility. A newer root
revision or SDK release never changes an existing API publication. Updating
global documentation does not require republishing each API.

SDK packages are stable ecosystem/coordinate identities. SDK releases always
use one exact version: `latest`, ranges, and automatic upgrades are invalid.
DokoSoko stores release metadata and bounded, reviewable normalized source
evidence; it does not host packages, proxy registries, store package-manager
credentials, execute package code, or claim supply-chain provenance.

Legacy documentation resource sets and SDK-reference endpoints remain only as
compatibility projections while data moves to typed assets. Existing SDK
reference IDs become binding IDs. Identical release identities may converge on
one package release, while conflicting URLs, digests, or visibility are placed
in a migration ledger for explicit review. Detaching removes only the API
binding; referenced assets are archived rather than deleted.

An Integration publication remains the API delivery boundary for developer
assets, authorization points, reviewed tool revisions, and runtime connection
revisions. Every delivered snapshot and hash is immutable. There are no
connector releases, Latest/LTS channels, customer pins, installations, staged
promotion, rollout percentages, provider-owned resource instances, or
automatic dependency upgrades.

## Catalog and API workflows

The root Catalog has APIs, Documentation, API contracts, and SDKs. Root
workspaces own creation, ingestion, review, revision history, visibility,
archival, and the reverse “Used by APIs” relationship. The API workspace calls
its attachment-only tab **Resources** and supports attach existing, create and
attach, open in Catalog, change exact revision/version, and detach.

Documentation collections may select exact reviewed source publications,
documents, or sections. They can be deployment-global, attached to several
APIs, or both without duplicating content. A contract revision and SDK release
can likewise serve several APIs, and different APIs may choose different exact
releases of the same SDK package.

## Ingestion, maps, and retrieval

Ingestion is a staged, replayable pipeline: acquire, validate, parse,
normalize, segment, extract, build a deterministic Documentation/Contract/SDK
Map, quality-check, review, publish, and build an index. Raw manifests,
processor versions, hashes, diagnostics, partial coverage, skipped files,
failures, and quarantine decisions are retained. Candidates are immutable and
publication is an explicit human action.

Source text and code are inert, untrusted evidence. Website and upload limits,
network controls, path containment, UTF-8 and size checks, secret detection,
and file classification fail closed. SDK ingestion never installs dependencies,
compiles, or executes samples. Extracted samples retain exact package version,
source path/revision, attribution, license, imports, validation status, and
review decision. Only explicitly approved samples enter a publication.

Every published collection, contract, and SDK content publication has a compact
agent-facing Map/Table of Contents. Retrieval indexes only exact immutable
publications, searches maps and targeted sections with lexical and deterministic
local feature-hash signals, applies exact scope and version filters, enforces a context budget,
and returns citations containing immutable publication, entity, and content
hashes. API search is:

```text
newest ready published global documentation
+ only the selected API publication's attached documentation, contracts, and SDKs
```

Content attached only to another API must never enter the candidate set. Query
Lab exposes global, API, and combined scopes, filters, ranked evidence,
citations, resolved publication IDs, and bounded retrieval traces. MCP exposes
the same reviewed maps and evidence and instructs clients to read maps first,
then run targeted search.

Candidate Maps describe everything the deterministic parser found so an
administrator can review it. Published Maps are separate immutable projections
of only the exact included documents/files, selected sections, derived symbols
and workflows, and approved samples. The store independently rebuilds and
validates an SDK Published Map from the complete review decision set; callers
cannot supply extra map prose even with internally consistent IDs or hashes.
Documentation collection member selectors are applied before member maps are
merged, so an out-of-selector topic cannot reappear through the Table of
Contents.

Only the ready index generation for the current builder and retrieval-profile
versions is eligible for search. Older ready generations remain immutable
history but cannot be mixed into current candidates. API MCP evidence is
derived from that same exact selector-filtered generation and uses API-
publication-scoped URIs, including historical reads, so two APIs can expose
different slices of one reusable asset without leakage or URI collisions.

## Trust boundaries

### Documentation

The crawler is credential-free and isolated. URL, DNS, redirect, same-origin,
byte/page, upload-containment, and renderer-network controls fail closed.
Fetched material is untrusted until an administrator reviews and publishes it.

### Credentials and tools

Secrets are encrypted with purpose-bound authenticated data and never returned.
Runtime destinations are fixed. Tools validate their schema and policy before a
network call. Consequential operations require the exact confirmation marker;
idempotency and grants cannot be weakened by a client.

Native tools are trusted compiled-in source. They receive only scoped host
capabilities, but they are not a security sandbox and therefore require source
and dependency review.

### Customer OAuth

DokoSoko owns its downstream authorization server and resource-bound tokens. An
optional upstream OIDC provider establishes customer identity; a fixed vendor
access-evaluation origin returns grants. Identity, customer status, grant,
freshness, and provider failures deny Private MCP access. DokoSoko tokens are
never sent to vendor services.

### Upstream MCP

Each connection has one fixed public HTTPS endpoint and encrypted service access
token. DokoSoko may add a bounded identity envelope encoded in
`X-DokoSoko-User` and signed with the service token. It never forwards the
inbound bearer. Imported schemas remain reviewed local tool definitions; an
upstream schema change blocks execution until reviewed.

### AI

The Analysis provider, failover model, limits, and daily budget remain explicit.
Retrieved content is untrusted data, the model receives no tools, and invalid
credentials/configuration, unsafe input, exhausted budgets, or invalid output
never trigger silent failover. A transient-failure retry discloses the same
bounded prompt and reviewed evidence to the configured backup once. Analysis is
advisory: generated connector setup guides, tool drafts, and product-integration
recipes require deterministic validation and human review; recipe publication
is immutable and always explicit. Setup guidance and recipe content are separate
contracts: the former connects an agent to MCP, while a recipe is discovered
after connection and contains only minimal, coherent steps for a coding agent to
implement one product capability. An exact operation from the selected published
API contract is the preferred recipe capability; a revision-exact reviewed tool
is used only when no published contract operation is available. Search ranks
candidate operations, but method, path, schemas, security, visibility, and drift
identity are reconstructed from the immutable contract graph rather than from
retrieved prose. An SDK is never inferred from package membership alone.

The four core analysis and recipe workflows and the four developer-asset
enrichment workflows have stable prompt keys. The enrichment keys cover
Documentation Map enrichment, SDK Map enrichment, SDK applicability
suggestions, and static SDK sample review. Sample generation is not a normal
ingestion workflow. Workflow-specific instructions are versioned and
resettable, while the common untrusted-input, exact-scope, evidence-ID, and
no-tool safety policy remains server-owned and immutable.

The developer-asset prompt keys are `documentation.map_enrichment`,
`sdk.map_enrichment`, `sdk.applicability_suggestion`, and `sdk.sample_review`.
Their output is advisory and schema-constrained. They never execute source,
assert compatibility or validation, widen an API scope, approve review, or
publish an asset. Missing, conflicting, cross-version, or out-of-scope evidence
must produce structured uncertainty instead of a guessed result. The runtime
rejects any registered workflow invocation that omits a named, closed-object
JSON output schema.

Each developer-asset AI run is an explicit administrator action against one
immutable publication scope. A successful run records the effective prompt
version, exact allowed evidence IDs, input/evidence/result hashes, a closed
structured result, actor, and timestamp. Invalid, unsafe, unavailable, or
schema-invalid runs persist no advisory result and never change the
deterministic Map, review state, binding, index, or publication. The console
labels these results as advisory and keeps deterministic evidence visible next
to them.
The deployment recipe contract and structured output schema are also
immutable. Recipes are deployment-owned and may attach one or more APIs; every
immutable recipe revision freezes the exact published revision and manifest
hash for each attached API. The recipe AI generator enumerates eligible
published APIs, exposes only their exact reviewed capability and evidence IDs
to the configured model, derives attachments from the selected capabilities,
and fails closed when the request is unsupported or ambiguous.
Editable prompt text may tune editorial guidance but cannot turn a recipe into
an MCP setup guide or ungrounded prose.

MCP delivery exposes both immutable `deployment-recipe-v3` recipes and
historical `product-integration-v2` recipes. Historical `legacy-mcp-v1` setup
recipes are withdrawn from resource discovery. Root
administrators may permanently delete legacy or outdated recipe records and
their immutable revisions through an explicit concurrency-guarded action; the
deletion audit event remains. Recipe listing returns compact delivery metadata, and plan
selection succeeds only for one exact normalized title, slug, or outcome;
unmatched and ambiguous requests return deterministic candidates rather than an
arbitrary recipe.

### Reporting

The reporting tools show a bounded preview and require explicit user consent.
Secret-like content is rejected. The accepted payload is stored as plaintext in
one local `queued` outbox with trusted API and reporter context. No encryption,
routing, delivery credentials, retry worker, or external receipt lifecycle is
part of the service.

## External extension boundary

Widgets and similar experiences are separate applications. They authenticate
through standard private OAuth and consume standard Private MCP. DokoSoko may
publish discovery metadata for such a plugin, but it does not load its browser
code, store its sessions/secrets, or expose widget-specific runtime endpoints.

## Delivery plan and acceptance gates

The implementation is delivered in dependency order. A later phase does not
weaken an earlier deterministic gate.

### Phase 0: baseline and migration safety

- Capture representative messy documentation, OpenAPI, SDK, code-sample, and
  retrieval cases before changing ranking or prompts.
- Add append-only typed tables and compatibility projections before moving UI
  ownership. Preserve legacy IDs where they are externally meaningful.
- Produce a migration ledger for SDK identities that cannot be deduplicated
  safely. Never guess through conflicting coordinates, versions, URLs, hashes,
  visibility, or API ownership.
- Gate: existing reviewed publications remain readable and migration checksum,
  rollback, and immutability tests pass.

### Phase 1: deterministic ingestion and normalization

- Persist raw manifests and content hashes, then run acquire, validate, parse,
  normalize, segment, extract, map, quality-check, and review stages under an
  owned lease.
- Normalize HTML, Markdown/MDX, text, JSON/YAML, and OpenAPI into semantic
  blocks while retaining source URL/path, exact ranges, headings, tables, code
  fences, links, media type, language, and hashes.
- Detect empty documents, missing titles, duplicates, repeated boilerplate,
  oversized atomic sections, partial coverage, unsupported files, malformed
  structured data, unsafe paths, secrets, and prompt-injection patterns.
- Gate: identical inputs and processor versions produce identical normalized
  IDs, ordering, maps, and hashes; failed or stale workers cannot leave partial
  typed candidates.

### Phase 2: review and immutable publication

- Give administrators a complete file/document explorer with search, paging,
  inclusion state, diagnostics, normalized content, lineage, and an exact
  Documentation, Contract, or SDK Map preview.
- Require explicit document/file/sample decisions and retain exclusion or
  quarantine reasons. A sample is publishable only with positive named machine
  evidence or non-empty structured human-review evidence.
- Snapshot root display identity, selectors, visibility, content hashes, exact
  revisions/releases, and reviewed maps into immutable publications.
- Record the domain publication audit and activation marker before an index can
  become discoverable. A failed newest activation leaves the prior ready
  publication live.
- Gate: no draft, partial, quarantined, unreviewed, unactivated, or visibility-
  widened content can enter retrieval.

### Phase 3: scoped retrieval and Query Lab

- Build immutable lexical and local feature-hash search generations from exact
  publications. The feature-hash vector is a deterministic lexical fallback,
  not a learned semantic embedding. Index compact maps as routing evidence and sections, contract
  operations, SDK symbols, and approved samples as targeted evidence.
- Resolve global, API, or combined scope before ranking. Apply API publication,
  asset-kind, language, ecosystem, exact version, selector, visibility, result,
  and context-budget filters before returning evidence.
- Return exact citations and a bounded append-only trace containing resolved
  publication IDs, routing, scores, exclusions, token estimate, and latency.
- Expose the same reviewed current and historical maps/evidence through MCP.
- Gate: cross-API and cross-version forbidden-evidence cases have zero leakage;
  every result cites one exact immutable entity and content hash.

### Phase 4: deployment-owned SDK workflow

- Manage reusable package identities separately from APIs, with exact immutable
  releases, lifecycle events, content candidates, files, symbols, samples,
  maps, content publications, compatibility assertions, and API bindings.
- Never install dependencies, execute package code, crawl with package-manager
  credentials, accept version ranges, or move an API binding automatically.
- Yanked or archived releases remain historically readable but are rejected for
  new bindings and publications. Lifecycle event and audit persistence is one
  atomic mutation.
- Gate: two APIs can select different releases of one package; changing package
  metadata or adding a release cannot change either historical snapshot.

### Phase 5: Catalog and attachment UX

- Root Catalog owns APIs, Documentation, API contracts, and SDKs. Detail pages
  show ingestion/review state, maps, publication history, and “Used by APIs.”
- The API Resources workspace stays attachment-only and offers attach existing,
  create and attach, open in Catalog, change exact revision/version, detach,
  and immutable publication history.
- Query Lab shows the resolved scope and evidence, not only a synthesized
  answer, so an administrator can diagnose poor retrieval directly.
- Gate: every consequential action explains what exact immutable object will
  change and never represents a draft or advisory as published truth.

### Phase 6: bounded advisory AI

- Add explicit persisted runs for Documentation Map enrichment, SDK Map
  enrichment, SDK applicability suggestions, and static SDK sample review.
- Build each input from one immutable reviewed scope and an exact allowed-ID
  set. Give the model no tools, network, credentials, or execution capability.
- Validate a closed JSON schema, enums, IDs, bounds, and hashes before storing a
  result. Store failures only as safe stage diagnostics, never model text.
- Keep deterministic maps and review decisions authoritative. An administrator
  may use an advisory to make a separate normal edit or review decision.
- Gate: prompt changes cannot override the immutable safety contract; no model
  output directly mutates, approves, binds, indexes, or publishes an asset.

### Phase 7: evaluation, rollout, and operations

- Maintain versioned retrieval evaluation sets with expected and forbidden
  evidence. Include ambiguous, no-answer, cross-API, cross-version, prompt-
  injection, malformed, duplicate, stale-index, yanked-release, and secret-like
  cases.
- Compare pipeline, normalizer, map, retrieval-profile, embedding, prompt, and
  model versions before promotion. Keep the previous ready index/publication
  available for rollback.
- Roll out first to private Catalog content, then selected APIs, then public MCP
  only after visibility and forbidden-evidence suites pass.
- Alert on ingestion failures, incomplete coverage, quarantine spikes, stale
  leases, index failures, retrieval no-result rate, forbidden-evidence failures,
  AI schema failures, and budget exhaustion.

## Success measures

The product outcome is faster, safer diagnosis and higher grounded-answer
quality, not the number of files or model calls processed.

- **Ingestion integrity:** 100% of published entities have immutable lineage,
  content hashes, processor versions, review evidence, and a successful
  activation/index record; deterministic replays produce the same result.
- **Scope safety:** zero forbidden cross-deployment, cross-API, visibility, or
  SDK-version evidence in automated regression sets.
- **Retrieval quality:** establish a real-query baseline, then target at least
  90% expected-evidence recall in the top five while tracking precision,
  no-result accuracy, citation validity, latency, and context size by asset
  kind. Thresholds are promoted only with the versioned evaluation set.
- **Review quality:** 100% of included files and samples have an explicit
  decision; no `not_checked` sample is approved without structured human review
  evidence.
- **Operability:** every failed stage has a stable code and bounded diagnostic;
  a retry is idempotent and never duplicates or silently replaces evidence.
- **Admin efficiency:** measure median time from completed ingestion to a
  publish decision and median time to diagnose a bad query before and after the
  explorer, maps, and Query Lab ship.
- **AI value:** compare advisory acceptance and correction rates, unsupported-
  finding rate, latency, and cost per useful accepted suggestion. Disable or
  remove a workflow that does not beat the deterministic/human baseline.

## Explicit non-goals

- A generic documentation chatbot or autonomous publishing agent.
- Package hosting, registry proxying, dependency installation, source
  execution, build/test sandboxes, or supply-chain attestation.
- Automatic SDK compatibility claims, version upgrades, or API bindings.
- AI-generated source-of-truth documentation or code samples during normal
  ingestion.
- Hiding uncertainty behind a synthesized answer when exact evidence is
  missing.
