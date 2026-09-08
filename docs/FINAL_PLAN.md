# DokoSoko core product contract

DokoSoko is one MCP connector for reviewed documentation, API contracts, exact
SDK releases, runtime keys,
recipes, and reviewed tools. The service is deliberately not a developer
portal, provisioning platform, widget product, support case-management product,
or product-release orchestrator.

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
- consent-based plaintext support submissions in a root-routed durable outbox;
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

The primary console navigation keeps APIs and Recipes prominent and groups
Documents, API contracts, SDKs, Sources, and Query Lab under **Knowledge**.
Knowledge opens the reviewed-document library. Existing asset URLs remain
stable, including direct package/release links and resumable setup context.
These deployment-owned workspaces own creation, ingestion, review, revision history, visibility,
archival, and the reverse “Used by APIs” relationship. The API workspace calls
its attachment-only tab **Resources** and supports attach existing, create and
attach, open in Knowledge, change exact revision/version, and detach.

Documentation collections may select exact reviewed source publications,
documents, or sections. They can be deployment-global, attached to several
APIs, or both without duplicating content. A contract revision and SDK release
can likewise serve several APIs, and different APIs may choose different exact
releases of the same SDK package.

Normal documentation setup uses the typed collection path. One stable
source-derived collection slug identifies the source's reusable documentation
history; each saved version selects one exact reviewed source publication.
Setup reuses a matching immutable version after a lost response, and only creates
a later version against the reviewed collection revision. It never overwrites
unrelated members or an unexpected API binding. An exact attachment changes the
API draft; publishing the API remains a separate acknowledgement. Custom sets
can still combine named reviewed sources, documents and sections through their
explicit set editor.

Contract setup retains a primary-attachment choice against the exact immutable
candidate/source evidence, originating API and audience in the current
deployment/reviewer browser scope. Source/root revision increments caused by
publication do not invalidate that choice; changed evidence or audience does.
The choice is not an acknowledgement. Reload and changed review selections
require renewed acknowledgement. Attachment writes use the reviewed binding
revision and reject intervening changes. Setup completion requires a read-back
of the same deployment/API/contract, exact pinned revision, primary setting and
audience, with follow-latest disabled. A committed publication can be recovered
independently from attachment, and no API publication is implied. Current API
audience is rechecked before publishing and before attachment; public API
attachments require the reviewed public audience.

## Ingestion, maps, and retrieval

Ingestion is a staged, replayable pipeline: acquire, validate, parse,
normalize, segment, extract, build a deterministic Documentation/Contract/SDK
Map, quality-check, review, publish, and build an index. Raw manifests,
processor versions, hashes, diagnostics, partial coverage, skipped files,
failures, and quarantine decisions are retained. Candidates are immutable and
publication is an explicit human action.

Source creation accepts an optional `Idempotency-Key`, scoped to the current
administrator and deployment. One transaction records the private draft source,
creation audit and digests identifying the request and normalized input. Repeated
requests with the same input return the current source without resetting its
audience, revision or publication state; changed input with the same key conflicts.
Upload identity includes its validated bytes, name and extension, not the random
storage path. A recovered upload retains the original file and removes the new
temporary copy. An uncertain database commit preserves the possibly referenced
file. Requests without a key retain the existing create-new-source behavior.
This boundary does not queue ingestion or acknowledge AI processing/publication.

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

SDK review renders samples as literal code and labels whole-source-file
comparisons explicitly. Comparison evidence is limited to included files from a
previous publication of the same release and deployment. Local draft recovery is
scoped to the exact candidate; acknowledgement must be renewed. Pending writes
cannot be presented as published decisions. Setup completion requires reading
back the exact API attachment, including audience, selectors and compatibility
claims. Changed SDK evidence clears claims tied to the previous publication;
unchanged evidence preserves its reviewed claims without inventing test results.

Only the ready index generation for the current builder and retrieval-profile
versions is eligible for search. Older ready generations remain immutable
history but cannot be mixed into current candidates. API MCP evidence is
derived from that same exact selector-filtered generation and uses API-
publication-scoped URIs, including historical reads, so two APIs can expose
different slices of one reusable asset without leakage or URI collisions.

## Trust boundaries

Customer task checks must identify the selected task revision and exact published
evidence. A successful MCP response alone does not establish correct retrieval.
The standalone acceptance client supports a reviewed task plan with exact URI,
UTF-8 response-text hash and revision/publication metadata expectations. Its
reports identify the observing client/version and reviewed-plan fingerprint,
preserve request correlation, and keep implementation/tests explicitly not run.
These client observations are distinct from server attestations, simulated
administrator previews and independently executed application tests.

The API Connect workspace validates the selected task's canonical text and exact
API publication maps through read-only runtime previews before generating its
check plan. Preview reads preserve the requested literal URI and current audience
authorization; private administrator previews explicitly use simulated grants.
Imported reports must match the reviewed plan and required check structure.
Application checks remain explicitly client-reported and require their own
client/version, environment, performed check, expected result and actual evidence.
Exports preserve these separate origins and do not change Tested/Verified claims.
Changing the task or audience discards the open review and its reports.

Original full publication reads expose a structured evidence index in
`contents[0]._meta.evidence_resources`; discovery omits that duplicated index.
`resources/list` advertises current available recipes and ready publication maps.
Catalog version 2 uses compact `/map-v2` URIs; version 1 advertises original full maps.
Individual evidence units remain available through those maps, resource templates
and their exact historical URIs. Discovery sorts descriptors by URI and pages
them at 32 entries or 64 KiB of encoded array JSON, whichever comes first.
Continue with the opaque `nextCursor` in `params.cursor`. Every page resolves
current publication and authorization state again; catalog, audience, principal,
grant or policy changes invalidate the continuation with `-32602` and require a
restart. Cursors confer no access. A descriptor larger than the page budget fails
explicitly rather than being omitted. Connect and the acceptance client stop at
64 pages or 4 MiB per list and reject duplicate entries or inconsistent pages.
This follows the [MCP pagination contract](https://modelcontextprotocol.io/specification/draft/server/utilities/pagination).

MCP catalog versioning is independent of the MCP protocol version. The explicit
integer request metadata `com.dokosoko/catalogVersion` supports 1 and 2; absent
metadata selects 2, and invalid explicit values fail with `-32602`. Version 2
`server/discover` and `tools/list` contain a compact deployment catalog rather
than embedded API manifests. Tool pages contain at most 32 descriptors or 960 KiB
of encoded array JSON, with task/API routing tools first and other authorized
tools ordered by name. Current authorization is applied before pagination;
cursor scope uses the same current principal/catalog checks as resource lists.
Custom tools cannot shadow built-in tool names.

`deployment.apis.list` returns at most 32 audience-visible API summaries or 64 KiB
of array JSON. All normalized query words must match published API name, family,
version or description. Its `next_cursor` is bound to the query and authorization
context. `deployment.apis.get` requires the selected API ID and manifest hash;
changed publication returns `-32009`, and inaccessible APIs return `-32004`.
Modern developer-asset APIs include the exact ready publication map URI and pins.
The complete structured result is limited to 256 KiB; an oversized read returns
`-32010` directing callers to publication maps and scoped search. Legacy APIs
retain their manifest resource references. Published content is unchanged.

Version 1 retains the full `deployment`/`product` extensions, unpaged tool
catalog and zero-argument full recipe list for existing consumers; it rejects
tool-list cursors and the version 2 recipe-list arguments. Explicit legacy
`deployment.get_manifest` and `product.get_manifest` calls remain supported,
although version 2 does not advertise them. These compatibility paths and original full-map
reads are not covered by the compact response bounds. Full
manifest/index resolution still precedes pagination; bounded response size does
not imply bounded backend work.

Catalog version 2 `integration.recipes.list` accepts query words, an optional
exact API ID and an opaque cursor. All query words match the task's published
title, slug or outcome, or the API names/families/versions frozen into its current
recipe revision, ignoring case and punctuation. The API filter requires actual
recipe membership. Results retain exact recipe URI/revision and API revision/
manifest-hash pins; they do not infer SDK compatibility or testing status. SDK
selections are resolved through the recipe and its selected evidence.

Recipe labels come from their bound historical API snapshots, not a newer
catalog entry. Current audience visibility and existing recipe drift checks still
apply. Missing, invalid or unavailable revision context fails discovery with an
actionable error. API history reads are reused across recipes within one request.
Results sort by case-insensitive title then URI and stop at 32 summaries or 64 KiB
of encoded array JSON per page. `recipe_count` counts the current matching scope.
A changed query, API filter, catalog, availability or authorization context
invalidates continuation with `-32602`. Explicit version 1 preserves the original
full list. `integration.plan` continues to require an exact title, slug or outcome
and never chooses an ambiguous match. Read the selected resource and compare its
recipe revision before implementation.

Upstream MCP inspection follows every tool page before exposing a catalog to
import. One inspection has a 20-second deadline and limits of 64 pages, 4 MiB of
result JSON and 4,096 tools. Duplicate or blank names, repeated/invalid cursors,
inconsistent catalog revisions, connection revision changes and exceeded budgets
fail the entire inspection before import changes local tools. Empty opaque
cursors are forwarded; absent or null `nextCursor` ends the list. Inspection
keeps the fixed upstream endpoint and credential, and uses the minimum page TTL.

Versioned `/map-v2` reads contain publication pins, the ready generation ID,
evidence count and a link to `/map-v2/index`. They omit the full evidence table.
Index reads accept optional query words and exact source publication kind/ID,
source entity ID and content hash filters. All filters apply together. Each page
contains at most 32 entries or 64 KiB of evidence-array JSON; the complete resource
content object is bounded to 128 KiB. Oversized entries fail with `-32010`, never
silently disappear. Follow `next_uri` with unchanged credentials. Query, scope or
generation changes invalidate continuations with `-32602`. Historical indexes
use their exact publication generation, independent of newer publication heads.
Both summaries and index pages enforce the original current audience and ready
index checks before exposing titles, counts or other metadata.

Connect reads compact maps and resolves each selected dependency with exact source
filters. A lookup must identify exactly one unit, match the expected publication
and generation, and have no continuation. Missing, ambiguous, inconsistent or
partial matches stop review. Lookup pages locate references; the exported check
plan pins the compact map and exact evidence reads, without requiring the client
to repeat the browser's lookup. Original full-map/evidence URLs retain their
contents, and explicit legacy catalog version 1 remains available.

Both full and paged tables come from the same ready, selector-scoped generation
as the publication map. Connect resolves developer-asset dependencies by API/global scope, immutable
source publication and entity, and selected content hash. Contract operations use
their immutable contract revision and operation identity because their dependency
version is a canonical fact fingerprint rather than a content hash. Each selected
read must match its URI, generation and source metadata before its response-text
hash enters the plan. Unrelated entries are not read. Global facts resolve only
through global publications pinned by the task's exact API publications. Inspect
their immutable member metadata first and read only publications containing the
selected collection revisions and hashes, with public audience checks when needed.
Equivalent global publications use stable publication-ID order. Global map reads
pin the publication ID, snapshot hash and revision. Missing, ambiguous or over-budget
selections fail without exporting a partial plan.

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

Each of these advisory AI runs is an explicit administrator action against one
immutable publication scope. A successful run records the effective prompt
version, exact allowed evidence IDs, input/evidence/result hashes, a closed
structured result, actor, and timestamp. Invalid, unsafe, unavailable, or
schema-invalid runs persist no advisory result and never change the
deterministic Map, review state, binding, index, or publication. The console
labels these results as advisory and keeps deterministic evidence visible next
to them.
Knowledge processing is a separate required step before publishing a new source,
contract candidate, or SDK content candidate. Its server-owned
`knowledge-processing-v1` contract processes bounded parts of exact normalized
input, without tools, execution, or backup-provider failover. The credential-free
crawler queues this step; the Go service owns provider configuration and calls.
Successful batch checkpoints bind the workflow version, exact input hash,
structured result hash, processor identity, model, and evidence parts. The service
validates every evidence ID and literal quotation; incomplete or invalid results
cannot satisfy the publication gate. A crashed batch can be reclaimed after its
lease expires; completed batches survive retries and reloads. Publication audit
records identify the processing stages used. Human decisions, deterministic maps,
compatibility assertions, and test evidence remain separate. Existing immutable
publications are not rewritten or made dependent on another provider call.

The deployment recipe contract and structured output schema are also
immutable. Recipes are deployment-owned and may attach one or more APIs; every
immutable recipe revision freezes the exact published revision and manifest
hash for each attached API. The recipe AI generator enumerates eligible
published APIs, exposes only their exact reviewed capability and evidence IDs
to the configured model, derives attachments from the selected capabilities,
and fails closed when the request is unsupported or ambiguous.
Editable prompt text may tune editorial guidance but cannot turn a recipe into
an MCP setup guide or ungrounded prose.

Reference selection is a read-only projection of the recipe's exact dependencies,
scoped to deployment, recipe and current revision. Opening it never runs AI or
marks the recipe outdated. Named choices expose retained evidence and provenance;
they cannot widen selection to the live catalog. Reference edits use the same
canonical choices, retain server ownership of instructions, require AI review,
and create an unapproved immutable revision under existing concurrency and
current-grounding checks. A failed or uncertain save retains local choices;
refreshing the dialog explicitly loads the saved revision and resets them.

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
one durable outbox with trusted API and reporter context. Separate deployment-
root feedback and error URLs enable the corresponding tools; no per-API route or
delivery credential exists. Delivery snapshots the selected URL, uses leases,
bounded retries, idempotency keys, pinned DNS, and no redirects. DokoSoko does
not add report encryption, ticket workflow, conversations, or an external
receipt lifecycle.

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
- Require completed AI processing for the exact normalized import. Show progress,
  saved assessments, and a resumable failure state before publication.
- Expose required AI setup before processing and recipe generation. Local
  readiness checks configuration and available daily capacity without a provider
  call; an explicit connection test is separate historical evidence. Actual
  processing must revalidate the exact evidence and reserve its own budget.
  Changing configuration must not inherit a stale successful connection test.
- Require explicit document/file/sample decisions and retain exclusion or
  quarantine reasons. A sample is publishable only with positive named machine
  evidence or non-empty structured human-review evidence.
- Snapshot root display identity, selectors, visibility, content hashes, exact
  revisions/releases, and reviewed maps into immutable publications.
- SDK attachments may advance after publication. Historical publication and
  advisory rows reference stable attachment identity and immutable selected
  assets, never mutable release/content/assertion columns through foreign keys.
  A new SDK snapshot must validate and lock its exact current attachment
  selection until its transaction completes. Historical evidence remains frozen.
- Record the domain publication audit and activation marker before an index can
  become discoverable. A failed newest activation leaves the prior ready
  publication live. Console serving status must use the same resolver as MCP.
  Retry delivery from the exact saved API revision, preserving its publisher,
  content and scope even if the draft has since changed. Repairing its missing
  delivery record or index must not create a new API revision or weaken current
  runtime authorization. Unknown historical snapshot formats fail explicitly.
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

- APIs remain a primary workspace; Knowledge groups deployment-owned
  documentation, API contracts, and SDKs. Detail pages
  show ingestion/review state, maps, publication history, and “Used by APIs.”
- The API Resources workspace stays attachment-only and offers attach existing,
  create and attach, open in Knowledge, change exact revision/version, detach,
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
  The [published task retrieval baseline](retrieval-evaluations.md) now covers
  five synthetic SDK guidance tasks, exact citations, selected unavailable
  scopes and historical upgrades through Query Lab on memory and PostgreSQL.
  Its 100% recall and 20–40% precision at five are fixture results; broader
  no-answer cases, real application outcomes and documentation comparisons
  remain separate acceptance work.
  The separate [reference application fixture](../examples/integration-evaluation/README.md)
  executes 17 checks across those tasks with the exact published SDK source and
  records runtime/publication identities. It does not establish production
  coding-client behavior or improvement over a vendor's documentation process.
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

### Knowledge preparation and reviewed library

Contract and documentation collection creation accept an optional
`Idempotency-Key`, scoped to the administrator, deployment and resource kind.
The key and normalized input are stored as digests. The root, initial collection
revision when applicable, request identity and creation audit commit atomically.
Concurrent matching requests recover one current root; different input conflicts.
Retries preserve later root changes and do not create another initial revision
or audit event. Documentation creation still resolves its exact reviewed members
and requires explicit human acknowledgement on each request.

The console retains creation keys until setup's create/attach transition succeeds.
Reload recovery contains only a digest and random key for seven days in the same
administrator/deployment/API or catalog context. The user selects and reviews the
same input again; approval is never restored. A recovered custom set attaches its
exact first revision only after checking the existing attachment's API, revision,
audience and selector. Recovery never overwrites a concurrent attachment change.

The administrative Knowledge library defaults to included documents from each
source’s latest immutable publication. A newer unreviewed or failed import does
not replace them. Removed or excluded paths must not reappear from older
publications through search. Comparisons resolve an earlier included document
from the same source/path and an earlier import. For an uploaded source, two
single-document imports identify the same logical file across storage-path
changes. Count all imported documents, including excluded material, before using
that fallback. Ambiguous imports require an exact path match. Comparison reads
never rewrite historical paths, hashes, bodies or publication memberships, and
never select excluded, unapproved or foreign-source content. Historical imports
remain accessible separately.

The bounded Needs attention projection includes website/upload sources without
files, active or failed imports, quarantine/incomplete coverage, unfinished
review and a reviewed source publication lacking a full typed documentation
version of the same audience. Active contract source bindings use their contract
workflow. This read-only administrator projection does not claim AI readiness,
API attachment, delivery or test evidence. It returns names, exact resume
identities, states, counts and timestamps; content and diagnostic payloads load
only through their existing scoped review endpoints. Existing publication and
runtime authorization boundaries continue to apply.

### Source input replacement

An uploaded source can replace its future file input without replacing the
source identity or mutating historical evidence. The source input update,
optimistic revision increment, one queued crawl, recovery record and audit
commit in one transaction under the source lock used by ordinary crawl queueing.
Any queued/running import, including expired leases, prevents a new replacement.
The worker only sees a committed replacement input and job together. Replaying
the same administrator/source request and input recovers its original crawl;
changed input or expected revision conflicts. Read-only recovery requires the
same administrator and source scope and does not require file bytes.

The source name, audience, publication history and quarantine state remain
unchanged by replacement. The original uploaded file is retained; cleanup of an
uncertain commit must not discard a potentially referenced replacement file.
New inputs retain the existing bounded multipart, extension, UTF-8 and upload
containment rules. A clean crawler result, required AI processing and new human
review are separate prerequisites. Replacement never publishes or attaches
content. Storage and transaction failures return a redacted, actionable error.
