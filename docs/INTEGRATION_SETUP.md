# Integration setup contract

An Integration is one versioned API in DokoSoko. Its workspace has six tabs:

1. **Quick Start** — readiness and the next incomplete step.
2. **Resources** — reviewed documentation, API contracts, and exact SDK content.
3. **Keys & Access** — fixed runtime service origins and encrypted credentials.
4. **Tools** — API-owned and attached common tools.
5. **Test** — deterministic contract and live-upstream checks.
6. **History** — immutable API publications and audit history.

Recipes, customer identity, agent access, upstream MCP connections, the support
outbox, and service settings are deployment-wide workspaces. They link back to
the APIs that use them.

## Setup flow

### 1. Define the API

Create a private draft with a name, version, and description, then add resources.
A stable family key and exact version identify the API. Runtime connections and
tools are not prerequisites for a private knowledge-only publication. Public
visibility always requires explicit acknowledgement.

### 2. Ingest and review evidence

For documentation, choose **Add content** in Knowledge or API Resources. The
setup page accepts a website, an uploaded text file, or a supported existing
source. It retains the source and exact import in the URL, shows processing and
readable review together, and saves a reviewed documentation version. Starting
from API Resources also attaches that exact version to the originating API draft.
Standalone setup leaves the version available in Knowledge for later attachment.
Neither route publishes the API or changes global documentation delivery.

Knowledge separates **Reviewed content** from **Needs attention**. The reviewed
view includes only the included files of each source’s latest publication; a
newer unreviewed or failed import cannot displace them. Search excludes files
removed from the latest review, and comparisons use an earlier included version.
Needs attention resumes website/file sources with no import, active or failed
imports, blocked coverage, unfinished review, or a review still awaiting a full
documentation version. Contract-bound inputs remain in contract setup. This list
is source preparation status; API attachment and serving status are separate.

For a quarantined or otherwise failed upload, use **Replace uploaded file** to
choose a corrected file in the same setup page. Replacement keeps the source
name, audience and earlier files/reviews, then queues one new import. It is
rejected while any import is queued or running, including an expired worker
lease. Public replacement requires an explicit audience confirmation in the
console. Quarantine is cleared only by a clean crawler result; the new import
still needs AI processing and a new review. Blocked imports show correction
controls before processing or publication controls.

Replacement requests retain a key and input digest for seven days in the same
administrator/deployment/source browser scope. A reload checks whether the
request committed and resumes its exact import without another file upload.
If it did not commit, reselecting the same file retries the original request and
source revision. Files and acknowledgements are not saved in browser recovery.
For websites, correct the content at the original URL and choose **Import
corrected website**; the same crawler checks still apply.

Import history and diagnostics are secondary. A failed import can retry from the
same source; a lost queue response recovers one unambiguous new job. Multiple new
jobs require selecting the intended import. Review choices survive reload for
seven days in the same deployment/reviewer browser; final acknowledgement does
not. Public API setup requires explicit public-source confirmation. Source
publication, documentation-version creation and attachment retries recover exact
committed results. Later imports advance the source's existing documentation
collection, preserving earlier versions and other APIs' selections. Changed
content, audience, selectors or concurrent attachment edits require renewed
review. A historical unpublished import cannot be published over a newer import.

For a contract, choose **Create & set up**
from API Resources, or create it in the contract catalog. The setup screen accepts
an OpenAPI URL, an uploaded JSON/YAML file, or a supported existing source. It
attaches the source before starting ingestion so normalization retains the exact
contract lineage.
The AI setup panel identifies missing provider, model, credential, or budget
prerequisites. Follow **Configure AI**, connect the provider, select the Analysis
model, and use **Test** for an explicit connection check. Return to setup using
the retained route. Configuration from an unsaved dialog opens another tab to
keep its current file or prompt selection. The local readiness check does not
contact the provider; processing still validates the exact evidence and reserves
its required budget. Complete required AI processing, inspect exact content and quarantine findings, then publish the
selected evidence. Processing runs in bounded batches with durable results;
resume retries failed or unfinished batches without repeating the import.

Source review can open the stored text of each exact imported page and compare
it with the most recent included publication from an earlier import. Replaced
uploads retain this comparison when both imports contain one file, even when
their storage paths differ. Websites and ambiguous imports require the same
path. Unapproved or excluded content cannot become the comparison. Published
reviews show their actual included and excluded pages. Source identifiers and
hashes are available under provenance; the review summary shows the audience.

Contract setup keeps the originating API and exact source/import in the URL,
with seven-day progress recovery in the current browser for that deployment and
reviewer. Operations, inherited and operation parameters, source text, validation,
and AI evidence are reviewable together. Earlier published contracts provide
operation and structural comparisons. Publication coordinates separate source
and contract records, then attaches the exact revision to the originating API.
A lost queue response can recover the new job; publication and attachment retries
reuse committed records. A changed API binding requires renewed review and is
not overwritten. Final acknowledgement never survives reload. Public use requires
explicit source approval, and public API setup creates a public contract.
The primary-contract choice is retained for seven days for the same candidate,
source evidence, API, audience and reviewer. Another candidate starts a new
review; returning to the original candidate restores its choice. A changed
attachment does not silently replace that intention or count as completed.
Review it again before applying the choice to the current binding. Once contract
publication is saved, **Attach exact revision** finishes the remaining step.
Completion reads back the exact pin, primary setting and attachment audience;
it never publishes the API as a side effect. If browser storage is unavailable,
the open review retains the choice during retries and explains the limitation.
Source creation in this screen and Add content retains one request identity for
seven days in the current browser. If a creation response is lost, retry with the
same URL or reselect the same file to recover the existing source. Browser storage
contains only a random request key and input digest. Changing the input starts a
new request; unavailable browser storage is reported beside the form. Contract
creation also retains a request key. After a lost response, enter the same
metadata again to recover that contract and continue setup. Its current state is
preserved; retry does not reset later edits or create another root.

Choose reviewed sources, versions, documents, and sections by name when making
a documentation set. Document choices are limited to that publication's included
normalized evidence. Under **Attach existing**, **Create a set from reviewed
content** can create a custom set and attach its exact first revision in the
same dialog. If creation or attachment loses its response, Retry reuses the
created set and exact first revision. After reload, select the same reviewed
content and metadata and acknowledge the review again. The retained request key
recovers creation; the console checks the current exact attachment before
continuing. A changed revision, audience or selector requires resolving that
change. Catalog contract and set creation use the same recovery mechanism.
Browser recovery lasts seven days in the same deployment, administrator and API
or catalog context. It stores a random key and input digest, without names,
content or acknowledgement. If browser storage is unavailable, keep the form
open to retain its in-page retry key.
An interrupted or incomplete creation response keeps that key and explains how
to retry with the same details; it cannot navigate to an item without a confirmed
identity.

SDKs are deployment-owned reusable packages with exact immutable releases.
From API Resources, resolve a package and exact version, add its guidance files,
complete AI processing, review files and samples, publish the content, and attach
that exact publication to the originating API. Enter a package name with its
ecosystem, or paste a supported npm, PyPI, Cargo or Go package URL. Confirm the
exact version; custom registry/Git inputs and transient credentials remain explicit.
Ranges and `latest` are invalid.

Setup opens a full-page content review with prior published file comparisons,
removed files, saved decisions, and the APIs currently using the package. The URL
retains the exact package, release, API, candidate and publication, including when
opening advanced inspection. Missing explicit selections do not choose another
release or candidate.

Samples display literal code, including Markdown examples, with language, file
path and extracted source lines. Search includes the language. A sample's
comparison opens the whole previous published source file alongside the current
file; it does not claim that an earlier sample was approved. Comparisons stay
within the same release and deployment and require an unambiguous source path.

A lost import response can recover the matching stored candidate using file hashes
and interpretation metadata. If no matching import exists, select the files again:
browser progress never stores their raw contents or registry credentials. Review
choices and progress expire after seven days and are scoped to the current browser,
deployment and reviewer. Final acknowledgement must be renewed after reload or
changed decisions. A completed publication survives a failed or lost attachment
response. Retry reads the exact committed decisions and current attachment before
continuing; it rejects a concurrent replacement. Unchanged evidence retains its
compatibility assertion; changed evidence starts as Related.

An existing attachment can advance to another reviewed release or content
publication. Publishing the API then freezes the new selection. Earlier API
publications and advisories retain their original release, content and assertion;
they do not follow the attachment's current selection. Migration 0073 corrects
the PostgreSQL constraints that previously blocked this update after publication.

Required AI processing inspects decoded JSON input values, including every
duplicate-key occurrence, before a provider call. JSON escaping must not turn
ordinary source variables into apparent credentials or conceal literal secrets.
Schema declarations and authorization flags are metadata, not credential values.

During a save, draft decision controls are disabled; they become published
decisions only after publication is confirmed. Setup checks the API's current
audience before publishing and attaching, then verifies the saved attachment's
scope, exact publication, audience, selectors and compatibility claims. Changing
guidance clears claims and applicability selections tied to the older evidence.

Public APIs require explicitly public packages, releases and attachment confirmation.
Published guidance remains read-only, with its saved review decisions. Package
settings, lifecycle events, maps, symbols and provenance live in advanced inspection.
DokoSoko does not install packages or execute imported code. Human review,
publication, compatibility assertions, and named test results are distinct.

### 3. Configure keys and access when adding executable actions

Create fixed runtime service connections for the API and environment. Store
credentials in versioned, write-only credential sets. Rotation changes the
active credential without rewriting a published tool. Agents cannot supply a
destination, authentication scheme, or credential.

If private customer access is required, configure and test the deployment OIDC
provider separately. Register grants and API authorization points before
publishing tools that require them.

### 4. Build and review tools

Create a fixed HTTP tool, import a tool from a token-authenticated upstream MCP
connection, or review a compiled native tool. Validate schemas, fixed target,
identity requirement, grants, effect, confirmation, timeout, idempotency,
limits, and redaction. Bind one exact published tool revision and authorization
revision to the API.

Upstream MCP may forward a bounded signed identity envelope, never the inbound
DokoSoko token. A changed upstream schema blocks the imported definition until
an administrator reviews and republishes it.

### 5. Add recipes

Use the required Analysis provider to select an API and analyse its bounded published evidence, resolve blocking gaps by
attaching or configuring the missing evidence, then review one minimal plan for
one tangible product capability. A recipe is consumed by a coding agent that is
already connected through MCP; it contains product prerequisites, ordered
implementation steps, observable checks, and grounded references—not DokoSoko
or MCP setup instructions. Publish only an immutable recipe revision whose
structured specification and rendered Markdown agree. Analysis unknowns are
read-only findings, not free-form assertions. Evidence changes make affected
recipes stale until reviewed; no model output publishes itself.

Edit references using named evidence choices and retained excerpts from the
recipe's exact dependencies. Up to eight references can be selected, including
previously removed choices, or all can be removed. Evidence IDs, versions and
fingerprints remain available under details; external links open the live source.
Saving references or visibility requires AI review and creates an unapproved
revision. Rework remains available for AI-generated instruction changes.
Unavailable evidence or AI readiness blocks saving. A failed save retains choices
in the open dialog; Reload current recipe explicitly replaces them with the saved
revision. Concurrent edits or changed grounding are rejected. After an uncertain
response, reload before deciding whether another revision is needed.

### 6. Test and publish

Preflight projects the same candidate validation used by publication. It resolves
selected resources, exact SDK content, and any bound tools and runtime access.
An empty API cannot publish; a reviewed knowledge-only API can publish privately
without runtime tools, service credentials, or an identity-provider setup step.
Private delivery still enforces current identity and authorization. Required
failures deny publication. Publishing stores one
immutable Integration snapshot containing exact revisions and content hashes.

The publication dialog reads the same server status used by the API overview.
It shows frozen resource names, exact package and guidance versions, additions,
changes, removals, audience, selected tools and action permissions. Exact version
lookups must succeed before approval is enabled. Raw snapshots and hashes are
available under Technical details. The confirmation applies to the candidate
revision and manifest hash shown in that review. Publish never substitutes a
newly fetched candidate. A concurrent change is rejected; Refresh review loads
the new selection and clears confirmation. Retrying after a lost response uses
the same reviewed input and reuses the saved immutable publication.

Saving a publication and making its delivery index ready are separate outcomes.
The API overview, review and history identify the exact revision selected by the
MCP ready-publication resolver. A newer failed publication does not replace an
older serving publication or appear as completed setup. Retry delivery targets
`POST /api/v1/integrations/{integration_id}/revisions/{revision_id}/activate`.
It repairs activation/indexing from that saved snapshot, including a missing
developer-asset record, without using newer draft selections or creating another
API revision. The original publisher remains recorded. Anonymous and cross-API
retry are denied; current runtime permissions, credentials and drift checks still
apply when customers use the publication. Unsupported historical snapshot formats
fail explicitly. The console refreshes readiness after affected saves and when
returning to an API tab or window.

The acceptance client should cover protocol negotiation, OAuth metadata and
PKCE when enabled, resources list/read, grant-filtered tool discovery, fixed
upstream calls, confirmation, revoked access, and request/audit correlation.

Open **Connect** in an API to choose a published task and connection audience.
The initial audience follows the API's visibility.
**Review exact task** checks its current published recipe through MCP and resolves
the exact API revisions recorded by that recipe. The page shows canonical task
steps and checks, with each named API publication's map available beside them.
It also reads the documentation, contract operations, SDK guidance and samples
selected by the recipe's exact dependencies. Expand **Evidence used by this task**
to inspect those selections. Global guidance resolves through publications pinned
by the task's APIs, using exact collection revisions and content hashes. Only
publications containing selected evidence are read; their revision numbers appear
in the review. A newer current global publication does not replace the task's pins.
Missing or ambiguous evidence stops review; the page
does not choose unrelated material or silently export a partial plan.
A changed recipe, mismatched publication, missing map or audience conflict stops
review. Temporary runtime failure asks the administrator to check publication
delivery and retry. The setup checklist's **Resolve publication requirements** action opens publication
review directly; connection work does not replace publication approval.

Resource discovery lists recipes and publication maps in pages; individual
evidence units are reached through the maps. Connect follows the pages
automatically, rejecting changed catalogs or incomplete discovery. In the
technical **MCP preview**, Next page and Previous page show each exact tool or
resource list response;
changing the audience, method or simulated grants, or selecting Refresh, starts
again at page one. Copy JSON copies the displayed page.

Copy the contextual task prompt or download its check plan after the preview
succeeds. Connection instructions remain unavailable until the chosen endpoint
is enabled. Administrator review can inspect published public guidance while
public MCP is disabled, but the page disables the connection prompt and an actual
anonymous client fails. That failed client report can still be imported.
Plans pin the task URI/revision, exact UTF-8 response-text hashes and
publication/scope metadata. Historical maps and individual evidence use direct
reads. Plans are bounded to 32 resources. Task review reads compact publication
maps and uses exact source filters to look up each selected dependency; it does
not download the complete index. Missing or ambiguous matches, changed publication
pins and incomplete lookup results stop review. A task requiring more than 32
resources needs a more focused selection.
The standalone
client checks discovery and retrieval against these expectations, including
changes since the preview, and records its actual client/version and plan hash.

Import the client's JSON report on the same page. Reports for a different plan,
incomplete checks and unsupported success claims are rejected. Record application
checks separately with client/version, application/runtime/SDK versions, performed
check, expected and actual result. Downloading the combined evidence record keeps
administrator preview, client observations and client-reported implementation
results distinct. It does not publish content or assign Tested status. Results
stay in the open page until downloaded; a different selection or reload clears
them. Private previews use simulated access and do not prove customer OAuth.
See [recorded client evidence](client-compatibility.md) for verified scope and gaps.

### MCP catalog migration

Catalog version 2 is now the default on both MCP audiences. `server/discover`
returns a small `catalog` with API count and routing tools. `tools/list` puts
`integration.plan`, `deployment.apis.list` and `deployment.apis.get` first, then
continues the authorized catalog across pages. Follow `nextCursor` using
`params.cursor` until it is absent or null. Each tool page holds at most 32
descriptors or 960 KiB of descriptor-array JSON. If the catalog or access context
changes, `-32602` requires restarting without the old cursor.

Find a published task with `integration.recipes.list`. Supply `query` words or
an exact `api_id` from the API catalog; follow `next_cursor` in the next tool
call's arguments with the same filters. The list returns at most 32 summaries or
64 KiB of encoded recipe-array JSON per page. Its `api_versions` come from each
recipe's pinned historical API publications, even when newer unrelated API
publications exist. Query words can match those names and versions as well as
task titles, slugs and outcomes. Select a recipe URI and require its `revision_id`
when reading it. Its exact recipe and evidence provide SDK selections and
implementation checks. A list match does not establish tested compatibility.
Use `integration.plan` when an exact title, slug or outcome already identifies
the task; ambiguous requests require a selection.

Find a published API with `deployment.apis.list`, optionally supplying `query`.
Follow its `next_cursor` in the next call's arguments, preserving the query.
Select an exact `id` and `manifest_hash`, then call `deployment.apis.get` with
`api_id` and `manifest_hash`. Modern APIs return a `publication` with the exact
developer-asset map URI; read it to locate selected evidence. If publication
changes, select again from the catalog. The selected manifest result is limited
to 256 KiB; `-32010` directs larger reads to publication maps and scoped search.

Clients that read `result.deployment`/`result.product`, expect
`deployment.get_manifest` in tool discovery or depend on original full-map URIs
in resource discovery must migrate to the compact tools/maps
or explicitly select legacy catalog version 1 in every request's metadata:

```json
{"_meta":{"com.dokosoko/catalogVersion":1}}
```

Merge that property into existing `params._meta`, retaining protocol/client
metadata. Version 1 preserves the full catalog extensions, unpaged tool list and full
recipe list. Omit tool-list cursors and use empty arguments for
`integration.recipes.list` with that version. Explicit `deployment.get_manifest` and
`product.get_manifest` calls remain callable. The catalog version does not change
the MCP protocol version, endpoint, OAuth flow or immutable publication content.
For administration preview, use the optional `catalog_version=1` or `2` query
parameter; the console uses the compact default.

A compact map's URI ends in `/map-v2`. Read it for publication pins, evidence count
and `index_uri`. Read that index to browse at most 32 entries per page, or add
`query` words and exact `source_publication_kind`, `source_publication_id`,
`source_entity_id` and `content_hash` query parameters. URL-encode each value.
Follow the returned `next_uri` for more matches, then read only the linked exact
evidence URIs needed for the task. Index entries are limited to 64 KiB of encoded
array JSON and each resource content object to 128 KiB. If an index or access
context changes, restart without the old cursor. Original full-map and evidence
URIs remain readable; their content is unchanged. This supports exact historical
evidence without searching a newer publication by accident.

DokoSoko's upstream MCP importer also follows all tool pages. It only offers a
complete catalog after inspection succeeds, within 20 seconds, 64 pages, 4 MiB
and 4,096 tools. Inconsistent discovery or exceeded limits stop inspection before
import changes local tools. Narrow an oversized upstream catalog before retrying.

## Delivery

Private MCP serves authorized customer resources and tools. Optional Public MCP
serves only explicitly public, published, read-only material and is disabled by
default. Agent setup pages describe both endpoints. After connection, agents
list compact recipe metadata, resolve an exact title, slug, or outcome without
fuzzy guessing, and read the selected product-integration recipe resource.

Support tools require a user-approved preview and append a bounded plaintext
record to the durable outbox. When the matching root-level feedback or error
destination is configured, the service snapshots it and delivers with bounded
leases and retries. No per-API route or support credential exists.

An embedded widget is outside this service. A future external widget plugin
uses the same OAuth and MCP surfaces as any other client.

## Invariants

- DokoSoko access tokens are never forwarded upstream.
- Network destinations and authentication modes are fixed configuration.
- Credentials are write-only, encrypted at rest, and excluded from manifests,
  model prompts, logs, reports, and API responses.
- Identity, grants, account state, schema, confirmation, and publication
  failures deny access.
- Public exposure requires explicit acknowledgement at the API/resource level.
- Published API, source, recipe, and tool revisions are immutable.
- SDK compatibility is exact and API-owned; there is no floating selection.
- Native plugins are trusted reviewed source compiled into the service, not
  dynamically loaded or sandboxed extensions.
