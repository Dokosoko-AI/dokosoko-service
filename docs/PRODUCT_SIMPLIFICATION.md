# Setup, Knowledge, and first integration delivery

Approved implementation scope, 7 September 2026. This is a delivery checklist;
unchecked items are not claims about existing behavior.

AI is required for recipe generation and Knowledge processing. The earlier
review's recommendation for optional AI and deterministic recipe authoring is
superseded. Deterministic parsing, scope checks, schema validation, immutable
lineage, human approval, and runtime authorization remain mandatory safeguards.
AI failure must stop the affected processing stage with an actionable reason;
it must never silently publish or pretend processing succeeded without AI.

## Current handoff — 8 September 2026

The setup and Knowledge redesign is implemented for product review. The current
change includes content-first API creation, resumable package/document/contract
setup, readable publication review, required AI processing, a current-document
library and task-specific connection evidence. The checkpoints below retain the
scope and limitations of each validation run; they are historical records, not
additional product requirements.

Final pre-commit verification passed: `pnpm run verify` (190 console tests,
57 crawler tests, typecheck, lint, production build and root Go tests), the
standalone MCP acceptance-client module, and the full Go suite against a fresh
disposable PostgreSQL 17/pgvector database. The repository verification enabled
the 20 standalone-client scenarios and the reference-application evaluation.

This is not a production-release sign-off. Production coding-client/OAuth
validation, customer outcomes compared with existing documentation, backup
restoration of deployed images, remaining translations and backend catalog/index
scaling are still open. The broad acceptance gates remain unchecked. Product review of the actual
setup and Knowledge journeys takes priority over expanding validation scope.

## Acceptance and implementation evidence

- [x] Content-first API creation: name/version, then documentation, contract, or
  SDK guidance. Runtime credentials, execution hooks, environments, and tools
  are requested only when adding an executable action.
- [x] Reviewed knowledge can be published privately without a tool or runtime
  credential. Private delivery still enforces current identity and permissions.
- [x] API publication review shows named exact selections and readable changes.
  Publish uses the reviewed candidate; concurrent edits require a refreshed review.
- [x] One server readiness result drives overview, publication review, and the
  publish action; all affected mutations refresh it. Current serving state comes
  from the same resolver used by MCP, including index failure and retry.
- [x] Package setup is continuous and resumable: resolve exact release, add
  evidence, AI processing, review content, publish, and attach to the originating
  API. Partial failure must not require duplicate imports or raw identifiers.
- [x] SDK attachments respect explicit audience, preserve compatibility
  assertions, and never invent Tested/Verified evidence.
- [x] Knowledge library shows current content and pending changes; historical
  generations, source maintenance, maps, hashes, and diagnostics are secondary.
- [x] Documentation, contract, and SDK review put exact readable content/diffs,
  findings, audience, and decisions together. Decisions survive reload against
  the same candidate; changed content requires renewed review.
- [x] Contract/source creation leads into processing and exact API attachment.
  Pickers offer supported inputs and never require copying internal IDs.
- [x] AI setup/readiness is a first-class prerequisite for processing. Required
  AI stages retain bounded inputs, no tools, typed failures, and explicit review.
- [x] Recipe creation remains AI-based; missing evidence, provider failure,
  invalid credentials, budget exhaustion, and disabled configuration are distinct.
  Reference management uses content selection, not a JSON array of IDs.
- [x] Customer connection selects a concrete task and publication, verifies
  discovery and exact evidence retrieval, and distinguishes server observations
  from client-reported implementation/test outcomes.
- [ ] Client/version compatibility evidence, bounded agent discovery, meaningful
  retrieval and task evaluations, and upgrade/restore rehearsal are recorded.
- [x] Route-specific loading and lightweight document list queries replace eager
  global loading and per-document detail hydration.
- [ ] README, setup guide, product contract, API contracts, UI guidance, and
  tests reflect the completed behavior and required AI model.

## Invariants

Published revisions, selected evidence, selectors, and hashes remain immutable.
Global documentation can advance independently; existing API publications do
not silently follow new asset versions. Approval, publication, serving state,
and tested evidence are separate facts. Selected tools retain current grants,
credential validity, revocation, drift, confirmation, and fixed-target checks.
Private titles and evidence must remain inaccessible without authorization.

## Verification

Use focused behavior tests while implementing each journey, then the repository
verification suite. Persistence changes require disposable PostgreSQL 17 with
pgvector. Preserve scope/history/security regressions and exercise actual import,
review, retry, publication, and retrieval behavior; source-text assertions alone
do not prove completion. Track all remaining requirements above until there is
direct implementation and verification evidence for each.

## Implementation checkpoint

The first implementation pass has removed the API creation runtime wizard,
replaced independent publication preflight rules with candidate validation,
and demonstrated private documentation-only publication in a regression test.
Selected runtime tools still require compatible live connections and credentials.

Global publication listing now reports `serving_publication_id` through the MCP
ready resolver. The activation retry endpoint reuses the exact publication;
tests cover an unactivated newer publication, anonymous denial, and repeated
activation without duplicate publications.

Knowledge now opens a lightweight current-file library with an explicit history
view, on-demand content, previous-import comparisons, and secondary provenance.
PostgreSQL selects summaries in two bounded queries instead of hydrating every
file. Shared safe content rendering treats HTML as text and rejects unsafe links.
Review decisions are not described as delivery or test evidence.

Package setup carries API/package/release context in the URL. Missing exact
releases fail closed, and an already attached exact publication retains its
compatibility assertion. SDK and source review choices are stored for seven days
in the current browser, scoped to deployment, administrator and exact content;
acknowledgment is never stored. This is local draft recovery, not shared review
state across browsers. SDK review displays exact content next to the decision.
A source publication whose attachment fails remains open for retry.

Recipe authoring and AI review now fail visibly without a working provider.
No recipe revision is persisted when either required stage fails. Provider,
credential, configuration, budget, timeout and invalid-output errors are distinct
from missing-evidence errors. Deterministic recipe validation remains mandatory.

Verification completed for this checkpoint: repository verify (typecheck, clean
lint, production build, 101 console tests, 57 crawler tests, root Go suite), plus
a full Go suite against disposable PostgreSQL 17 with pgvector. Required-AI
failure and private knowledge-only publication regressions also passed after the
last test additions. The standalone MCP acceptance-client unit suite also
passed, including its local OAuth callback fixtures. Real browser journeys and
external agent client acceptance
remain outstanding.

Required Knowledge processing is now implemented before new source, contract,
and SDK content publication. The crawler and SDK normalizer queue AI processing;
the Go service resolves exact bounded evidence, validates IDs and literal quotes,
and saves successful batch checkpoints. Retries reuse completed batches, an
active lease prevents duplicate provider calls, and expired work can be reclaimed.
Publication audit records identify the processing stages. SDK, contract, and
source review screens show required processing, progress, pause/resume, and saved
assessments. These results do not replace human decisions or test evidence.

The processing HTTP journey verifies a missing provider, blocked SDK publication,
configuration and retry, one provider call across repeated requests, and successful
publication afterward. Focused tests cover malformed/invented/missing evidence,
multi-batch recovery, checkpoint persistence failures, Unicode byte bounds, and
concurrent calls. PostgreSQL tests verify jsonb-safe result hashes, expired leases,
and cross-deployment denial in an isolated schema. The full repository verify
and full Go suite against a fresh disposable PostgreSQL 17/pgvector database passed.
An earlier run against the reused test database failed on a pre-existing fixed-name
secret fixture; the fresh database run passed without changing that test.

Source review now opens the exact stored extracted page body and compares it
with an earlier included publication of the same page. Scope checks bind every
read to deployment, source, import, and document. Future imports cannot become
the comparison for an older review. Published reviews expose the actual inclusion
decisions and show audience separately from publication status. Hashes and import
identifiers live under provenance; completed reviews have a Close action.

Documentation set and API attachment forms now select named reviewed sources,
versions, documents, and sections. The document query is bounded to the chosen
publication's included normalized evidence, while history ranking remains intact.
Changing selections or metadata clears acknowledgement. The set editor waits for
its exact revision before allowing revision creation. API attachment retry retains
the created set and first revision in the open dialog, checks existing bindings,
and avoids duplicating a completed attachment. This is not yet reload recovery for
that intermediate state.

Console data reads now follow the active screen. Resources does not load identity,
MCP connections, root administration, reporting, AI settings, recipe history, or
all source histories. Knowledge uses lightweight source metadata and loads exact
review evidence on demand. Source maintenance still loads its histories. AI
configuration and recipe history load on their respective screens. Loading and
errors are scoped to navigation; an unrelated Tools failure does not appear on
Knowledge. Route-level JavaScript code splitting is not implemented.

The Docs sidebar now opens the Knowledge library. Its tabs put Documents and
API contracts ahead of Sources; source maintenance remains directly accessible.

Browser testing found and fixed a wire-contract mismatch in documentation set
revision and contract candidate details: the server emitted capitalized record
keys, but OpenAPI and the console require lowercase fields. Empty contract lists
now serialize as arrays. HTTP regression checks assert the actual JSON keys,
anonymous denial, and cross-root denial, rather than relying on Go's permissive
case-insensitive decoding.

Verification for this checkpoint: the full repository suite (type checking,
lint, production build, 101 console tests, 57 crawler tests, and root Go tests)
and full Go tests against a fresh PostgreSQL 17/pgvector database passed.
Headless Chromium exercised the production console against a disposable Go
service: failed attachment followed by retry without another set, exact stored
source content, named section selection and persisted identity, acknowledgement
invalidation after editing metadata, desktop/dark/narrow review, and navigation
from failing Tools back to Knowledge without carrying its error. Resources issued
no unrelated administration/AI/history requests. AI settings, Recipes, customer
accounts, and the SDK catalog still fetched their screen dependencies. No browser
JavaScript errors or narrow-layout horizontal overflow were found in these checks.
This browser fixture used preloaded evidence and test authentication; it does not
establish live registry/crawler/provider, login/MFA, or external client acceptance.

Outstanding work is still tracked by the acceptance list: better recovery for
legacy/unbound imports;
recovery for lost resource-creation responses;
task-oriented connection and client evidence; full locale translations of new
strings; and operational rehearsal. A quarantined/unsafe item can still block
processing of its entire import; review exclusion/re-ingestion recovery while
completing source setup. Do not mark the overall goal complete at this checkpoint.


Contract setup now connects root creation, supported URL/file/existing-source
selection, source binding, import, required AI processing, exact source/contract
review, separate source and contract publication, and originating API attachment.
Progress records contain identities only and expire after seven days in the
current deployment/reviewer browser scope. Explicit URL selections are validated;
missing or mismatched objects do not silently select another candidate. Starting
another source remains a distinct resumable input state. Unsupported source kinds
are absent from the picker. Public API setup requires public root and source
confirmation. Archived contract roots cannot continue setup.

Contract review shows descriptions and operations before advanced inspection,
keeps path and operation parameters distinct, identifies added/changed/removed
operations, and compares changes to schemas, servers, security and metadata.
Exact source text is available beside review. AI evidence remains available after
publication. Contract candidate scope and creation time are now explicit in the
OpenAPI schema, matching the existing server fields.

The finish transition re-reads the source and root, reuses exact committed
publications, checkpoints the published revision before API attachment, and
rejects a concurrent binding edit. Pending queue progress records the preceding
server job; a lost response can recover even when the new job already finished.
A successful queue or publication is not repeated merely because its response or
later attachment failed. Final acknowledgement resets on reload or review changes.
At this checkpoint, initial root/source creation lacked server idempotency: if
its response was lost before an identity was saved, the existing record had to
be selected manually. Source creation recovery is addressed in the later
checkpoint below; root creation recovery remains open.
New strings currently have English fallback text in all locale files; translated
copy remains in the outstanding acceptance scope.

Verification for this increment: repository verify passed with clean type/lint,
production build, 117 console tests, 57 crawler tests and all root Go tests. The
full Go suite also passed against fresh PostgreSQL 17 with pgvector. Browser
acceptance used the production console, real Go endpoints, uploaded OpenAPI,
real isolated crawler and PostgreSQL, with a deterministic AI test adapter and
test authentication. It covered an import failure followed by retry using the
same source, required processing, readable source and contract evidence, exact
publication, an attachment outage followed by reload and successful retry,
acknowledgement reset, changing input across reload, private and explicitly public
audiences, and a lost queue response producing exactly one import. No duplicate
publication/attachment or narrow-layout overflow occurred; browser JavaScript
errors were absent. Desktop and dark/narrow screenshots were inspected. This
establishes the local pipeline, not external provider quality, registry imports,
real authentication, or customer client acceptance. The overall goal remains open.


AI setup is now a first-class prerequisite in empty API setup, contract/source/SDK
import, incomplete Knowledge processing, and recipe generation. The shared
administrative readiness endpoint uses the runtime's local provider, workload,
model and credential checks, plus the same daily usage and active reservation
counters. It performs no provider call and reserves no tokens. Local readiness
means processing can be attempted; actual processing still validates evidence,
reserves its required budget, and reports provider failures. Missing model,
disabled workload/provider, missing provider/credential, invalid configuration,
and exhausted daily budget are distinct states.

AI settings starts with connecting a provider, then choosing an Analysis model.
Internal return links retain the originating setup route. Unsaved dialogs open
configuration in another tab; returning refreshes readiness and keeps the exact
candidate and review selections. Explicit provider tests are distinct from local
configuration. A failed test is amber. Provider saves invalidate prior tests, and
readiness suppresses tests older than the selected profile. Tests record their
start before reading the profile so a model changed during an in-flight probe
cannot inherit the old model's success. Budget resets include a local date/time.

Browser acceptance also exposed a valid README-only SDK candidate returning null
symbol/sample arrays, which crashed the catalog before review. Candidate reads,
ingestion and retry responses, and publication selection reads now return arrays
as required by OpenAPI. Regression tests inspect the actual JSON and exact-release
scope. SDK file review rows now put the file name above the content/decision
controls, avoid misleading sample-validation requirements for plain files, and
omit an empty Samples section.

Verification for this increment: the final repository suite passed (typecheck,
clean lint, production build, 118 console tests, 57 crawler tests, all root Go
tests), and the full Go suite passed against a fresh PostgreSQL 17/pgvector
database. A clean headless Chromium run used the production console, real Go
endpoints and PostgreSQL, with test authentication and a deterministic provider
adapter. It covered missing AI blocking recipe generation; provider/model setup
and explicit testing; return to the exact originating API; rejected credentials
with redacted errors; configuring in another tab while retaining a file decision;
processing retry on the same SDK import; readable exact content; exhausted-budget
gating; and desktop, narrow and dark layouts. No JavaScript errors or horizontal
page overflow occurred in this run. These checks establish local behavior, not
external-provider quality or real authentication/client acceptance.

The overall goal remains open. Continuous package/source recovery and the broader
SDK catalog still need work, along with recipe reference selection, task-oriented
customer connection and client evidence, complete locale translations, and
operational rehearsal. This increment completes the AI prerequisite checklist
item; it does not close the remaining acceptance criteria.


The SDK package journey now runs in one exact guidance workspace: package name or
supported registry URL, confirmed release, bounded text input, required AI
processing, readable content and comparisons, human decisions, publication, and
originating API attachment. The directory no longer auto-selects a package or
opens raw file/map JSON. Version history and advanced inspection preserve exact
candidate identity. The advanced catalog loads on demand; its duplicate import,
review and attachment dialogs are removed. Public exact release creation in
advanced settings requires an explicit audience and confirmation.

Imported file hashes and interpretation metadata allow recovery of a committed
import whose response was lost. Raw files and registry credentials are never
saved in browser progress; an import that did not commit requires reselecting the
files after reload. Saved review choices are scoped to deployment, reviewer and
exact candidate hash, expire after seven days, and never include acknowledgement.
Publication retry compares the full saved inclusion and structured review evidence.
A publication checkpoint precedes attachment; failures reopen its read-only review,
and lost attachment responses resolve the existing exact binding. Concurrent
replacements fail closed. Existing exact evidence retains compatibility assertions
and their contract revision; changed evidence begins as Related without invented
Tested/Verified claims.

Review shows the APIs currently using the package, previous included files, changed
content and removed files. Published decisions and reasons remain readable.
Configured AI connection/budget details collapse on SDK input/review; errors and
missing prerequisites remain visible. Plain source code stays inert preformatted
text; Markdown uses the shared safe renderer. README and setup guidance now
describe deployment-owned packages and exact API selections.

Verification for this increment: repository verify passed with type checking,
clean lint, production build, 138 console tests (46 JavaScript and 92 TypeScript),
57 crawler tests and all root Go tests. The full Go suite also passed against a
fresh disposable PostgreSQL 17/pgvector database. Headless Chromium used the
production console, actual Go endpoints, PostgreSQL, test authentication and
deterministic registry/AI adapters. It covered package URL resolution, exact
metadata import, lost import response and reload recovery, required AI processing,
keyboard file selection, saved decisions with renewed acknowledgement, an
attachment outage after publication, lost publication and attachment responses,
explicit public guidance, private-to-public denial, sample approval with explicit
review evidence, content comparisons/removals, and exact advanced/history return.
It also published an SDK-only private API through the console and retrieved its
exact reviewed evidence through authenticated MCP; an unauthenticated read was
denied. The run reported no JavaScript errors or horizontal overflow; desktop and
dark/narrow screenshots were inspected. This verifies the local pipeline, not
external registry/provider behavior, real authentication, or named agent clients.

This completes the package setup and SDK attachment checklist items. The overall
goal remains open: the API publication review still exposes raw JSON changes;
source recovery, complete Knowledge workflow acceptance, recipe reference picking,
task-oriented customer connection/client evidence, locale translations, and
operational rehearsal remain in scope.


API publication review now groups frozen resource names, exact SDK versions and
guidance revisions, documentation/contract revisions, global knowledge, selected
tools, action permissions and runtime connections. Added, changed and removed
selections are visible; changed audience is explicit. Exact immutable version
lookups must succeed before confirmation is enabled. Names come from the reviewed
snapshot, and scope checks reject mismatched revision responses. Technical JSON
remains available in a collapsed section. Execution information appears only when
tools are selected. This is publication approval, not integration test evidence.

The status contract now returns the API identity and optimistic candidate revision
alongside its snapshot/hash. Preflight projects that same status. The old console
publish handler fetched a fresh candidate on final click, which could publish an
intervening change without showing it. The new dialog submits only its captured
review input. A concurrent change is rejected and clears confirmation; refreshing
loads new evidence and requires renewed confirmation. A lost response can be
retried against the same candidate; the server reuses the immutable publication.
Tool-binding and action-policy/grant saves now refresh the API readiness display.

Verification: final repository verify passed (typecheck, clean lint, production
build, 141 console tests, 57 crawler tests, all root Go tests). The full Go suite
also passed against a fresh disposable PostgreSQL 17/pgvector database. The final
Chromium pass used the production console, real Go endpoints and PostgreSQL,
with fixture authentication and previously reviewed deterministic SDK content.
It verified unavailable exact-version gating, keyboard confirmation, rejection
of a concurrent API edit, explicit refreshed review, lost publication response
and idempotent retry, changed binding audience, narrow/dark layout and reachable
dialog actions, authenticated MCP discovery/read, and no JavaScript errors.
Desktop and narrow screenshots were inspected. External registries/providers,
real authentication and named agent-client acceptance remain unverified.

The publication review item is complete. The broader readiness item remains open:
the overview/history still need to distinguish a saved publication from the exact
publication currently served by MCP and expose activation/index retry. Remaining
work also includes source recovery and complete Knowledge acceptance, recipe
reference selection, customer task/connection/client evidence, translations,
operational rehearsal and final documentation alignment.


Publication serving and recovery now complete the shared readiness requirement.
The status contract reports the exact serving API revision and developer-asset
publication through MCP's existing ready-publication resolver. A saved newer
revision whose activation/index failed remains visibly pending, while an older
ready revision continues serving. Overview, publication review and history consume
that status; setup completion requires actual ready delivery. Tab/focus changes
and affected mutations refresh it, and late responses cannot replace another
API's current status.

Retry delivery targets an exact saved API revision. It reconstructs its delivery
selection from the immutable snapshot, repairs a missing developer-asset record,
and retries activation/indexing without reading newer draft selections or creating
a duplicate API revision. Original publisher identity is retained, including
recovery by another operator. Anonymous and cross-API retry fail; unsupported
historical snapshot schemas are explicit failures. Current runtime authorization
is unchanged. An SDK-only PostgreSQL publication also exposed null empty category
lists that crashed history; list/detail HTTP responses now return the arrays
promised by OpenAPI without altering stored snapshots or hashes.

Final repository verify passed with type checking, clean lint, production build,
141 console tests, 57 crawler tests and all root Go tests.
Verification includes regression cases for failure before delivery-record creation,
failed index completion, repeated outage and successful retry, preserving a newer
draft and original publisher, older-ready fallback, exact API scope, anonymous
denial, and array-shaped publication responses. The full Go suite passed against
a fresh PostgreSQL 17/pgvector database. The production Chromium journey forced
an actual index persistence failure, confirmed pending status and absence from
MCP, changed the API draft, refreshed on focus, retried the original saved revision,
and read its original content through MCP. It verified one API revision, exact
activation identity, repeated activation, history's serving label, dark/narrow
layout, and no JavaScript errors. The stale-review and lost-publish-response browser
regression also passed with these status changes. These are local fixture-backed
checks; real authentication, external providers and named customer clients remain
outside their evidence.

The goal remains open for source recovery and complete Knowledge acceptance,
recipe reference selection, task-oriented customer connection and client evidence,
translations, operational rehearsal and final documentation alignment.


Recipe reference editing now uses named choices and retained excerpts from exact
recipe dependencies. A read-only, revision-scoped endpoint supplies the same
canonical references accepted by the save path, including previously removed
choices. It rejects stale recipe identities, changed grounding, and foreign
scope without mutating recipe state. The editor accepts at most eight choices
or an empty selection; opaque evidence versions, IDs and fingerprints are
secondary. Unsafe source links cannot become selectable options.

Saving references or visibility remains subject to required AI review and the
existing immutable revision/CAS boundary. Compact AI readiness gates Save.
Provider/save errors remain in the dialog with the local selection intact;
Reload current recipe explicitly resets choices to the saved revision. A lost
save response can be recovered by reloading; an old-revision retry cannot create
a duplicate revision. Rework is available for valid current recipes, and its
input limit now matches the server. No manual instruction or JSON editor was
introduced. New copy currently has English fallback text in all locale files;
full translation remains outstanding.

Verification for this increment: repository verify passed (typecheck, clean lint,
production build, 142 console tests, 57 crawler tests, and root Go tests).
Focused Go tests exercise exact choices, adding/removing/restoring references,
unknown-reference rejection, revision/grounding conflicts, read-only behavior,
HTTP wire shape, and anonymous/customer/foreign-deployment denial. Headless
Chromium exercised the production console against real Go endpoints with an
isolated in-memory store, retained source fixtures, deterministic AI and fixture
authentication. It covered unavailable evidence and AI readiness, retry, keyboard
selection, retained choices after failure, empty selection, concurrent edits,
explicit reload, and a lost save response without duplicate revisions. Desktop,
dark/narrow layouts, and expanded provenance were checked; no JavaScript errors
or horizontal overflow occurred. PostgreSQL-specific persistence was unchanged
in this increment; the prior full PostgreSQL checkpoint remains recorded above.
External provider quality, real authentication and agent client acceptance are
not established by this fixture. The Knowledge/source and customer-onboarding
acceptance items remain open; the overall goal is not complete.


Source creation now retains one logical request across a lost response and retry.
Both URL and upload endpoints accept an optional administrator/deployment-scoped
`Idempotency-Key`. The source, request/input digests and creation audit commit in
one transaction. Concurrent identical requests return one source; changed input
with the same key conflicts. Recovery returns current source state without
resetting an audience, revision or publication. Upload retries retain the original
stored file and discard duplicate temporary bytes; uncertain commits retain the
possibly referenced file. The append-only `0070_source_creation_requests.sql`
migration records these identities. Existing clients that omit the key retain
their previous create-new-source behavior.

Add content and contract setup save a random request key and input digest for
seven days in the current deployment/reviewer browser scope. Raw URLs and file
contents are not stored there. Reload recovery requires re-entering the same URL
or selecting the same file. Errors stay beside the input, and unavailable local
storage is reported. Retrying a recovered source preserves its loaded history
and shows its actual audience. Configured AI details are compact; missing setup
remains expanded. Knowledge search controls now wrap on narrow screens.

Verification for this increment: repository verify passed (typecheck, clean lint,
production build, 146 console tests, 57 crawler tests and root Go tests), as did
the full Go suite against fresh PostgreSQL 17 with pgvector and Compose validation.
Focused tests cover concurrent retries, changed-input conflicts, ownership,
unchanged current audience, upload retention, and rollback of both source and
request record when audit insertion fails. Headless Chromium used the production
console, real Go endpoints and PostgreSQL, with fixture authentication and a
deterministic AI adapter. Lost creation responses followed by reload/retry produced
one website source, one uploaded source/file, and one contract source with one
queued import. Keyboard selection and dark/narrow layout passed, with no browser
JavaScript errors or horizontal page overflow. Screenshots were inspected.
This increment did not run the queued import through a crawler or establish
external-provider, real-authentication or customer-client acceptance.

Source creation recovery is a prerequisite for the continuous Knowledge journey,
not completion of that journey. At that checkpoint, Add content still closed
after creating the source. Joining import, required AI processing, readable
review, publication and exact typed documentation attachment was the next
product change, completed by the later checkpoint below. Contract
root and documentation-set creation recovery, quarantined-import recovery,
task-oriented customer connection, compatibility/retrieval evidence, translations
and operational rehearsal remain open. No broader acceptance item is checked by
this checkpoint, and the overall goal is not complete.


Documentation now has a continuous setup page from Knowledge, source maintenance
and API Resources. It accepts supported website/file/existing-source inputs,
retains the exact source/import and originating API in the URL, shows required
AI processing beside readable content and previous-text comparisons, and saves
selected documents into a reviewed typed documentation version. Starting from an
API attaches that exact version to the draft; standalone setup makes it available
in Knowledge. API and global documentation publication remain separate actions.
Source metadata is refreshed when returning to the library. Opaque upload paths
are now secondary provenance rather than the document's primary caption.

Progress and review decisions use the existing seven-day deployment/reviewer
browser scope. New source creation retains its request identity. Queue recovery
accepts only one unambiguous new job, and history offers explicit import selection.
The final transition re-reads the review and audience, recovers an exact committed
source publication, finds or creates the matching immutable documentation version,
then compares the current API attachment with the reviewed binding. A stable
source-derived collection identity allows later imports to create revisions in
the same collection instead of proliferating sets. Historical versions remain
unchanged. Changed selections, public audience, selectors and concurrent bindings
fail closed. Final acknowledgement resets after reload or changed choices.
Custom sets assembled from existing reviewed content remain available under
Attach existing; their older intermediate reload-recovery gap remains tracked.

Verification: repository verify passed with typecheck, clean lint, production
build, 155 console tests, 57 crawler tests and root Go tests. New behavioral tests
cover exact inclusion, all three publication/creation/attachment lost-response
boundaries, same-collection version advancement, ambiguous queues, changed
content, partial coverage, quarantine, audience mismatch, selectors and concurrent
attachment replacement. Chromium exercised the production console with real Go,
PostgreSQL and the isolated crawler, plus fixture authentication and deterministic
AI. Private setup recovered lost source, queue, source-publication, collection and
attachment responses with one source, import, publication, version and attachment.
Public setup required explicit audience confirmation. Standalone setup returned
to Knowledge with current source metadata. Decisions survived reload, final
acknowledgement did not, processing was required, and no API publication happened
as a side effect. A separate browser check retried the real crawler's earlier
storage-configuration failure from the original source, retained the failed job,
and reached AI review without another source. Desktop and dark/narrow screenshots were inspected; the final
journeys had no JavaScript errors or horizontal page overflow. Go persistence was
unchanged in this increment; the prior fresh PostgreSQL suite remains the
persistence checkpoint. These fixtures do not establish external-provider quality,
real authentication or customer-client compatibility.

This completes the contract/source creation-to-attachment criterion. Knowledge's
pending-change presentation and quarantined-import recovery, the full review
acceptance audit, initial contract-root/custom-set response recovery, customer
task-based connection and compatibility evidence, retrieval/task evaluations,
translations and operational rehearsal remain open. The overall goal remains active.


Knowledge now defaults to Reviewed content and presents Needs attention beside
it. Reviewed content contains only included documents from each source’s latest
immutable publication. New unreviewed or failed imports cannot replace approved
files, and excluded/removed paths cannot reappear through search. Comparisons
use an earlier included version, skipping unapproved intermediate imports.
History and documentation sets are secondary; uploaded storage paths are kept
out of primary captions. Review approval still does not imply API delivery.

Needs attention is a bounded administrator-only source projection. It includes
website/file sources without any import, active or failed jobs, quarantined or
incomplete imports, unfinished processing/review, and source reviews lacking a
full typed documentation version of the same audience. Contract-bound inputs
remain in contract setup. Rows resume the exact source/import; they do not load
all document bodies, source histories or AI status. Filtering, pagination,
explicit refresh and focus refresh are supported. Read failures stay actionable.
This is source preparation status, not a list of pending API attachments.

The shared memory/PostgreSQL behavior tests cover approved content retention,
search after removal/exclusion, earlier included comparisons, exact timestamps,
no-file failures, failed/skipped coverage, quarantine, partial collection scope,
contract attachment/detachment, pagination and deployment isolation. HTTP tests
cover anonymous/customer denial, private metadata, bounded inputs, foreign
sources, unsupported writes and absence of read-side mutations. PostgreSQL
verification exposed and fixed an enum/text comparison in the new projection
and an existing UUID aggregate in source-review persistence.

Verification for this increment: repository verify passed with typecheck, clean
lint, production build, 155 console tests, 57 crawler tests and all root Go tests.
The full Go suite passed against a fresh PostgreSQL 17/pgvector database. Chromium exercised
the production console and actual Go/PostgreSQL endpoints, using retained real
crawler publications and explicitly seeded unfinished import fixtures with
test authentication. It verified the reviewed default, unchanged reviewed files
during a newer queued import, 110 pending sources across bounded pages, source
filtering, keyboard continuation to an exact failed import, incomplete-coverage
publication blocking, recoverable summary failure, and no JavaScript errors or
narrow-page overflow. Desktop and dark/narrow screenshots were inspected. This
does not establish external-provider quality, real authentication or customer
client acceptance.

The library presentation criterion is complete. Quarantined-upload replacement
and safe clean-import recovery, the full content-review acceptance audit,
initial contract-root/custom-set response recovery, task-oriented customer
connection and compatibility evidence, retrieval/task evaluations, full locale
translations and operational rehearsal remain open. AI remains mandatory. The
overall goal remains active.


Blocked uploaded documentation now supports corrected-file replacement inside the
same setup page. The source input, optimistic revision, one queued import,
request recovery record and audit commit atomically. Ordinary queueing uses the
same source lock; queued/running jobs, including expired leases, reject a new
replacement. Source identity, name, audience, quarantine and historical evidence
remain intact. Original upload files are retained, duplicate retry files are
removed, and uncertain commits retain potentially referenced bytes. The
append-only 0071 migration stores scoped request/input digests and the exact job.

The console retains the original request and revision in its existing seven-day
reviewer/deployment/source browser scope. A read-only recovery endpoint finds a
committed replacement after reload without requiring another upload; an
uncommitted request can retry the same selected file. Public replacements require
an explicit audience acknowledgement in the console. File selection is scoped to
the current source. Older imports offer Open current import. Blocked imports lead
with correction instructions and defer AI processing/publication controls until
the corrected import is available. Websites offer a direct reimport action after
the original site has been corrected. AI remains required for the new import.

Verification: repository verify passed with typecheck, clean lint, production
build, 157 console tests, 57 crawler tests and all root Go tests. The full Go suite
passed against a fresh PostgreSQL 17/pgvector database, and Compose validation
passed. Tests cover concurrent identical requests, queue/replacement races, audit
rollback, quarantine preservation, expired running leases, changed-file/revision
conflicts, administrator/source scope, upload validation, original-file retention,
uncertain commits and read-only recovery with uploads disabled.

Chromium used the production console, actual Go/PostgreSQL and the isolated real
crawler, with test authentication and deterministic AI. Private and public files
were actually quarantined for instruction/exfiltration indicators. A pre-commit
outage retained the original request; a post-commit response loss recovered its
exact new import on reload without reselecting the file. Each journey retained
one source, the original quarantined file/assessment, one corrected import and
its exact source publication/documentation attachment. The corrected import had
to complete new AI processing and human review; no API publication occurred as a
side effect. Public replacement required explicit audience confirmation. Keyboard,
dark/narrow layouts and desktop screenshots were checked, with no JavaScript
errors or horizontal overflow. These fixtures do not establish real authentication,
external-provider quality or named customer-client acceptance.

This closes quarantined-upload replacement recovery. The broader content-review
criterion remains open: audit comparison matching across successive uploaded
replacements, whose storage paths differ, and complete the documentation/contract/
SDK review acceptance together. Initial contract-root/custom-set response
recovery, task-oriented customer connection, compatibility/retrieval/task evidence,
full translations, operational rehearsal and final documentation alignment remain
open. The overall goal remains active.

Upload review comparisons now survive corrected-file replacements. Exact source
review and the Reviewed content library resolve an earlier included publication
of the same uploaded source when both complete imports contain one document.
Excluded documents still count toward ambiguity; multi-document imports and
websites retain exact-path matching. The read projection skips unapproved and
excluded intermediate content, stays within source/deployment scope, and cannot
compare an older import with a future one. It preserves historical bodies,
storage paths, hashes and publication selections. The library selects latest
publications before their files and scopes comparison lookup to each displayed
source, retaining bounded list queries without per-file HTTP hydration.

Shared memory/PostgreSQL regressions cover changed storage paths, website
renames, ambiguous current and previous imports, an excluded extra file,
exact-path fallback, unapproved/excluded intermediate imports, foreign-source
content and immutable historical reads. Repository verification passed with
typecheck, clean lint, production build, 157 console tests, 57 crawler tests and
all root Go tests. The full Go suite passed against a fresh PostgreSQL 17/pgvector
database after the final query change. Existing production-build chunk-size and
Node warnings remain; lint and tests passed.

Chromium exercised private and public replacements through the actual crawler,
Go service and PostgreSQL, with test authentication and deterministic required
AI. Both reviews showed exact old/new text despite different storage URLs;
inclusion choices survived reload and acknowledgement reset. Saving the new
version retained the same comparison in Reviewed content and left historical
evidence unchanged. Keyboard publication, light desktop and dark/narrow layouts
were checked without JavaScript errors or horizontal overflow. A final read-only
browser check passed against the rebuilt service and final query. These checks
do not establish real authentication, external-provider quality or named
customer-client acceptance.

The upload-comparison gap is closed. The broader content-review acceptance audit
remains open, including contract and SDK review/decision recovery. In particular,
contract setup currently restores its primary-attachment choice from the saved
API binding on reload rather than preserving an unsaved choice for the same
candidate. Initial contract-root/custom-set response recovery, task-oriented
customer connection, compatibility/retrieval/task evidence, full translations,
operational rehearsal and final documentation alignment remain outstanding.
The overall goal remains active.


Contract review now preserves the primary-attachment choice for the same exact
candidate, source evidence, originating API, audience and reviewer. Publication
increments to source/root revisions do not erase it. A different candidate or
changed evidence/audience starts a new review; returning to the original
candidate restores its local choice. Final acknowledgement is never stored.
When browser storage is unavailable, the open review retains the choice through
automatic retry and explains the limitation beside the control.

An intervening attachment change does not silently replace the intended choice
or count as completion. Completion now checks deployment/API/contract scope,
exact revision, explicit pinning, primary choice and intended attachment audience.
Writes use the reviewed binding revision and verify their read-back. The current
API audience is rechecked before publication and attachment; public API setup
uses an explicitly reviewed public attachment. A saved contract publication
shows Attach exact revision with an explanation of the remaining step. The
publication checkpoint is retained without navigating away mid-attachment, and
successful response-loss recovery clears the stale error. Historical publications
and API publication remain unchanged.

Verification: repository verify passed with typecheck, clean lint, production
build, 164 console tests, 57 crawler tests and all root Go tests. Focused tests
cover exact choice scope, changed evidence/audience, source publication recovery,
lost attachment-change responses, completion mismatch, current API audience,
attachment read-back and public audience selection. This increment changed no
Go persistence or migrations; the browser used the actual PostgreSQL service.

Chromium exercised real uploaded OpenAPI imports through the isolated crawler,
required deterministic-fixture AI, source/contract publication and API attachment,
with test authentication. Private setup retained a non-primary choice across
reload and an outage after publication; an intervening primary attachment to the
same revision was not called complete. Renewed acknowledgement and an exact
PATCH completed the intended choice after a lost response. Public setup proved
that a new candidate starts a fresh choice while returning to the previous
candidate restores its choice; a lost attachment response recovered exact public
attachment. A denied browser-storage write kept the open review choice through
automatic retry and displayed the limitation. Each scenario retained one source
publication, one contract revision and one final exact attachment, without API
publication. Desktop/light, narrow/dark and keyboard actions passed with no
JavaScript errors or horizontal overflow. Screenshots were inspected. These
fixtures do not establish real authentication, external AI quality or customer
client acceptance.

The contract-choice recovery gap is closed. Finish the remaining SDK review
acceptance audit before checking the combined content-review criterion. Initial
contract-root/custom-set response recovery, task-oriented customer connection,
compatibility/retrieval/task evidence, full translations, operational rehearsal
and final documentation alignment remain outstanding. AI remains mandatory.
The overall goal remains active.


The SDK review acceptance audit is complete. Samples render literal code,
including Markdown, and repeated headings have language, source path and
extracted line ranges. Language search includes both files and samples.
Comparisons explicitly show the full source file from a previous publication,
limited to included files of the same release/deployment and an unambiguous path.
They do not imply that an earlier sample was approved. During publication, draft
decisions, manual evidence and both acknowledgements are disabled; published
labels appear only after confirmed publication.

SDK setup rechecks current API audience before publication and attachment and
verifies the saved exact attachment, including scope, audience, selectors and
compatibility claims. Changed evidence clears older claims and applicability;
unchanged evidence preserves reviewed claims. Tests cover wrong read-back,
concurrent audience changes, lost attachment-update responses and exact matching.

Final repository verify passed: typecheck, clean lint, production build,
171 console tests, 57 crawler tests and the root Go suite. This increment changed
no Go persistence or migrations. Chromium used the actual Go/PostgreSQL service,
test authentication and deterministic required AI. Private/public journeys passed
lost import/publication/attachment responses, publication followed by an outage,
reload recovery, changed-candidate review reset, literal Markdown sample code,
whole-file comparison, saved sample decisions/manual evidence, locked pending
controls and exact attachment read-back. Private SDK-only API publication served
its exact MCP evidence and denied anonymous retrieval. Light desktop, dark/narrow
layout and keyboard checks passed with no JavaScript errors or horizontal overflow;
final screenshots were inspected. This does not establish real authentication,
external AI quality or named customer-client compatibility.

Together with the verified documentation and contract journeys above, this closes
the combined content-review criterion. Remaining work: initial contract-root and
custom documentation-set response recovery; task-oriented customer connection;
client/version, retrieval and task evidence; translations; upgrade/restore
rehearsal; and final documentation alignment. AI remains mandatory and the
overall goal remains active.


Customer connection audit found that the standalone acceptance client previously
accepted a successful resources/read response without checking the returned URI
or content. It now requires exactly one matching, non-empty text/binary item.
An optional reviewed task plan pins a concrete task URI/revision and up to 32
exact resources by UTF-8 response-text hash, media type and publication/scope
metadata. The task must be discovered with its selected revision; linked or
historical evidence can be read directly. Invalid plans and endpoint mismatches
fail before network requests. Changed text, stale revisions, duplicate results
and missing evidence fail the check. Plans cannot provide credentials or commands.

Reports identify DokoSoko MCP acceptance client 0.2.0, the protocol, reviewed-plan
fingerprint, request IDs and observed content hashes/byte counts. They label
these as acceptance-client observations, not server attestations or another
coding client's results. Task retrieval and implementation are separate fields;
implementation remains not_run. Resource bodies and credentials are not recorded.

The standalone module suite passed. A new opt-in root HTTP test invoked the
actual built CLI against the Go service, a private published API and a recipe
created through required deterministic-fixture AI, review and publication. It
verified the exact recipe Markdown/revision and selected API publication map,
rejected a wrong content hash and recipe revision, and denied anonymous access.
The client/version evidence and reproduction command are recorded in
client-compatibility.md. This used an in-memory store and demo identity, so it
is not production OAuth or customer application-test evidence.

Repository verify passed with the standalone CLI test enabled: typecheck, clean
lint, production build, 171 console tests, 57 crawler tests and root Go tests.
This increment changed no production Go service behavior, persistence, migrations
or console rendering. No additional PostgreSQL or browser run was needed.

The customer-connection criterion remains open. Next, build the task-selection
and publication-review UI around these exact expectations, provide contextual
connection instructions, and keep server preview, actual client observations and
client-reported implementation/test outcomes distinct. Also outstanding are
initial contract-root/custom-set response recovery, production client/version
runs, bounded discovery and retrieval/task evaluations, translations,
upgrade/restore rehearsal and final documentation alignment. AI remains required;
the overall goal remains active.


The API Connect workspace now starts with a concrete published task and audience.
It re-reads the exact recipe revision, validates canonical Markdown against the
runtime response, and reviews the API publication maps selected by that revision.
Historical publications use explicit reads; discovery must still advertise the
selected task revision. Scope, audience, publication metadata, response URI and
content mismatches stop review. The administrator preview now supports bounded,
read-only resources/read with literal URI validation and existing authorization.
Private previews are explicitly simulated; public previews are anonymous.

Successful review provides a contextual task prompt and a downloadable exact
check plan without an extra acknowledgement gate. Imported acceptance reports
must match its fingerprint, task and complete required check structure. The UI
keeps server preview, acceptance-client observations and client-reported
application checks distinct. Application reports capture the client/version,
environment, performed check, expected result and actual evidence. Downloaded
records preserve these origins and do not change Tested/Verified claims. Open
reports clear when the selection changes or the page reloads. API configuration
preflight is secondary; Resolve publication requirements opens publication review
directly, while the existing test route is labelled Connect.

Final repository verify passed with the standalone CLI check enabled: typecheck,
clean lint, production build, 174 console tests, 57 crawler tests and the root Go
suite. Focused preview and exact-task tests passed. No persistence or migration
changes were made in this increment, so no additional PostgreSQL run was needed.
Chromium exercised the actual Go service with an in-memory store, test identity
and deterministic required AI. A plan downloaded from the UI ran successfully in
acceptance client 0.2.0; the UI imported its matching report and rejected a changed
plan fingerprint. Exported evidence preserved separate origins. Task/audience and
reload reset, direct publication-review navigation, keyboard review, desktop/light
and narrow/dark layout passed with no JavaScript errors or horizontal overflow.
Final screenshots were inspected. The application result entered by this browser
test was explicitly a fixture, not an executed customer implementation.

The customer-connection criterion remains open: this increment verifies the
canonical recipe and exact API publication maps, while individual linked evidence,
public task browser coverage and production client/OAuth journeys still need
verification. Initial contract-root/custom-set response recovery, bounded discovery,
retrieval/task evaluations, translations, upgrade/restore rehearsal and final
cross-document alignment also remain outstanding. AI remains mandatory and the
overall goal remains active.


Task connection now resolves the recipe's selected documentation, contract,
SDK and global evidence, alongside its canonical recipe and exact API maps.
Publication resources/read returns a structured evidence table from the same
ready, selector-scoped generation; resources/list omits that duplicated table.
The console selects exact source publications/entities and validates their hashes,
URI, generation and scope before adding reads to the check plan. Contract operation
dependencies use their immutable contract revision and operation identity; their
stored dependency version is a fact fingerprint, not a content hash. Unrelated
units are not read. Missing, ambiguous or over-budget selections stop without
exporting a partial plan. Plans support 32 resources and inspect at most 8,192
entries per publication. Global evidence uses wrappers pinned by the task's APIs;
equivalent global wrappers use stable publication-ID order.

Supporting evidence is shown under readable, collapsed rows. Guide, Example,
Overview and Operation labels distinguish duplicate source titles. The exact
response text remains available for review and its UTF-8 hash enters the plan.
Imported reports must cover these additional reads as well as task discovery;
retrieval remains separate from client-reported application outcomes.

Final repository verify passed: typecheck, clean lint, production build,
178 console tests, 57 crawler tests and root Go tests with the real standalone
acceptance client enabled. Focused tests cover documentation, SDK, contract and
global selection, public preview context, wrong scopes/hashes/generations,
ambiguous units, missing evidence and the resource limit. Runtime tests verify
public/global structured indexes, selector isolation and historical reads. No
production persistence or migration changes were made, so no additional
PostgreSQL run was required.

The actual Go/CLI fixture passed seven exact resources and rejected wrong map
text, recipe revision, SDK evidence and contract-operation text; anonymous private
access failed. SDK import, required AI processing, explicit review, publication
and attachment used the normal service. The contract fixture supplied a normalized
candidate and reviewed historical revision directly to the store, with required
processing exercised; it did not test contract source acquisition. Recipe
AI/review/publication, index construction and MCP delivery used the real service.

Chromium downloaded the seven-resource plan, ran it in acceptance client 0.2.0,
imported the matching report, rejected a different fingerprint and exported the
separate evidence origins. It checked exact contract and SDK content, distinct
Guide/Example labels, task/audience and reload reset, direct publication-review
navigation, keyboard actions and desktop/light and narrow/dark layouts. Final
screenshots were inspected; no JavaScript errors or horizontal overflow occurred.
Authentication and AI were fixtures. No customer application was executed.

The customer-connection criterion remains open for complete public-task browser
coverage and mixed global-publication scenarios. Production client/OAuth evidence,
initial contract-root/custom-set response recovery, bounded discovery, retrieval
and task evaluations, translations, upgrade/restore rehearsal and final document
alignment remain outstanding. AI remains mandatory and the overall goal is active.


Public task connection now has an end-to-end browser and standalone-client check.
The initial audience follows API visibility. Global evidence is selected from the
task's pinned API publications using exact collection revisions and content hashes;
only publications containing required evidence are read. Equivalent publications
use stable ID order, public review requires public members, and global map reads
pin publication ID, snapshot hash and revision. Review rows show that revision.
Temporary runtime errors are reported separately from changed selections.

The public fixture publishes SDK guidance and a reviewed global collection, then
advances current global documentation to unrelated guidance. The original recipe,
API publication, three SDK units, historical global map and selected global unit
still pass seven exact anonymous reads. Wrong map text, recipe revision, SDK text
and global text fail. Global historical inputs are seeded; global publication,
index activation, SDK processing/review and recipe generation/review/publication
use the actual service. Required AI uses a deterministic test adapter.

Chromium downloaded this exact plan and ran acceptance client 0.2.0 anonymously,
imported its matching report, rejected a different plan fingerprint and exported
separate evidence origins. Disabling public MCP disables the connection prompt;
the actual client fails and its failure report imports without claiming an
application test. Keyboard review, audience/reload reset, direct publication
review, desktop/light and narrow/dark layouts passed. Final screenshots were
inspected; no JavaScript errors or horizontal overflow occurred. Authentication
for console administration was a fixture and no customer application was executed.

Final repository verify passed with typecheck, clean lint, production build,
182 console tests, 57 crawler tests and root Go tests. Focused transport tests
cover multiple API publications, exact member hashes, equivalent publications,
unused private publications, distinct global requirements and unavailable runtime
errors. The private/public standalone-client checks also passed. No production
persistence or migration change occurred in this increment, so no additional
PostgreSQL run was needed.

The customer-connection acceptance criterion is now complete. Production client
and OAuth evidence remains separate and open, together with initial contract-root
and custom-set response recovery, bounded discovery, retrieval/task evaluations,
translations, upgrade/restore rehearsal and final cross-document alignment.


Initial contract-root and custom documentation-set creation now recover from
lost responses. An optional Idempotency-Key identifies one logical creation per
administrator, deployment and resource kind. Migration 0072 records its request
and input digests with scoped foreign keys. Creation, the initial set revision
and its audit commit together. Concurrent matching requests return the same
current root; changed input conflicts. Recovery does not reset later metadata or
create additional revisions/audits. Source review remains required for sets.

API Resources and the contract/set catalog forms retain this key through failed
creation or attachment. Reload restores only the key and input digest for seven
days; the user re-enters the same input and acknowledges review again. Names,
content and approval are not stored. Unavailable browser storage is explained,
while same-page retry retains the key. Incomplete successful responses cannot
clear recovery or navigate to a missing identity. Interrupted saves explain how
to retry instead of showing only a transport error. Custom-set recovery checks
the exact first revision and current attachment audience, selector and API before
reporting completion; concurrent changes require resolution.

Verification passed: the repository suite with clean typecheck/lint, production
build, 185 console tests, 57 crawler tests and root Go tests; full Go tests against
fresh PostgreSQL 17/pgvector; and focused concurrent retry/audit rollback tests in
memory and PostgreSQL. HTTP tests verify key forwarding, input conflicts and
anonymous denial. Browser checks use the production console and real Go/PostgreSQL
endpoints with a test administrator and previously reviewed source fixtures.
They discard committed creation and attachment responses, reload before retry,
and confirm one root, one initial revision and one attachment. Private/public
audiences, incomplete success responses, changed attachment selection, denied
browser storage, renewed review and keyboard submission were exercised. Final
desktop/light and narrow/dark screenshots were inspected; no JavaScript errors
or horizontal overflow occurred. The fixtures do not establish production
authentication or a new source/AI acquisition
run. Required AI and publication approval remain unchanged.

Initial contract-root/custom-set response recovery is complete. Remaining work
is production client/OAuth evidence, bounded discovery, retrieval/task evaluations,
translations, upgrade/restore rehearsal and final cross-document alignment.

## Resource discovery checkpoint — 2026-09-08

Resource discovery now advertises current recipes and ready publication maps,
with at most 32 descriptors and 64 KiB of encoded resource-array JSON per page.
Individual evidence URIs, historical publication reads, full map text and exact
structured evidence indexes remain available. Cursor continuations recheck the
current catalog and authorization context; invalid or changed scope returns an
actionable restart error. Connect follows all pages before accepting a task.
The technical preview adds page navigation and resets on context changes.

The HTTP growth fixture isolates evidence count using a synthetic scoped index
over normal API publication and MCP handlers. Measured response sizes were:

| Evidence units in one API | Previous resources/list | Current resources/list |
| --- | ---: | ---: |
| 1 | 1,935 bytes | 820 bytes |
| 100 | 116,676 bytes | 820 bytes |
| 1,000 | 1,159,776 bytes | 820 bytes |

The last unit remains reachable with its exact content hash through the full
publication map at every size. This measures payload growth, not ingestion/AI
quality or a production retrieval benchmark. Publication validation still loads
the index; this change does not claim bounded backend work. Full manifest aliases
in server/discover and tools/list, large publication-map reads and other catalog
tools still need a compatible bounded-discovery design.

Acceptance client 0.3.0 follows resource and tool pages, including restricted-token
tool lists, and records each page's request correlation and result byte count.
It rejects duplicate entries, invalid/repeated cursors, changed revisions and
exceeded 64-page/4-MiB budgets. Exact resource observations identify their actual
discovery page. This remains client retrieval evidence; implementation stays
`not_run`.

Verification passed: `pnpm run verify` with 188 console tests, 57 crawler tests,
typecheck/lint/build and root Go tests; the separate client module; and 17 actual
CLI scenarios across private, public and paginated local task fixtures. The
paginated fixture has 35 ready API maps and locates the task on page two; changed
recipe, map, SDK/contract/global evidence and anonymous private access fail as
expected in the applicable scenarios. HTTP tests also cover 35-API pagination,
preview continuation, stale cursors and changed audience/grant scopes. Browser
checks passed task review/export/report import, exact-page copying, keyboard
navigation, audience/method/refresh resets and narrow dark layout. These fixtures
do not establish production coding-client/OAuth compatibility. No persistence or
migration changes were required for this checkpoint.

## Compact API and tool catalogs checkpoint — 2026-09-08

Catalog version 2 is the explicit compact default. Initial discovery includes
deployment summary and task/API routing, with no repeated full API manifests.
`deployment.apis.list` provides query-scoped pages of published API summaries;
`deployment.apis.get` reads one exact API/manifest-hash selection and returns its
ready developer-asset publication route. A draft edit keeps the previous
publication; publishing a new revision makes an old manifest selection fail
instead of silently changing it. Private APIs remain excluded from public calls.

Tool discovery puts task and API routing first, then pages currently authorized
tools at 32 descriptors or 960 KiB of array JSON. API summary pages use 32 entries
or 64 KiB; selected manifest structured results are limited to 256 KiB. Cursors
are bound to current catalog, access and query scope. Legacy catalog version 1
retains full deployment/product aliases and an unpaged tool list; existing
consumers must select it explicitly or migrate. Legacy full-manifest tool calls
remain supported. README, setup guidance and OpenAPI document this transition.

The 35-API HTTP fixture measured the complete encoded responses:

| Request | Before compact catalogs | Version 2 |
| --- | ---: | ---: |
| server/discover | 44,740 bytes | 1,615 bytes |
| tools/list | 48,628 bytes | 6,344 bytes |

DokoSoko's upstream importer now follows tool pages before returning a complete
catalog to import. Its 20-second/64-page/4-MiB/4,096-tool limits, repeated cursor,
duplicate name, revision/connection change and cancellation checks fail before
local tools are changed. Empty opaque cursors are supported. Import from a later
page is covered, including the complete catalog hash and selected tool schema.

Verification passed: `pnpm run verify` (188 console tests, 57 crawler tests,
typecheck, lint, production build and root Go), the separate client module, and
17 real CLI fixture scenarios enabled during the root suite. Focused tests
cover exact selection, public/private isolation, legacy opt-in, cursor scope,
large escaped schemas and complete upstream inspection. Production-console
browser checks passed 32+10 tool pages, 32+11 with a grant, public filtering,
keyboard navigation, exact-page copy, context resets and desktop/light plus
390px/dark layouts. Client 0.3.0 found a later-page tool and called the API catalog
against that local service. The temporary service was stopped after verification.
No persistence implementation or migration changed in this checkpoint.

This closes repeated full manifests in default discovery. It does not bound full
publication-map reads, explicit recipe lists, legacy compatibility paths or
backend catalog/index materialization. Those limits, production client/OAuth
evidence, task/retrieval evaluations, translation completion and upgrade/restore
rehearsal remain open; the broad acceptance criteria above stay unchecked.


## Bounded publication maps checkpoint — 2026-09-08

Catalog version 2 now advertises compact `/map-v2` resources. Each identifies the
exact publication and ready index generation and links to a paged evidence index.
The index supports query words and exact source publication/entity/content-hash
filters, with 32 entries or 64 KiB of array JSON per page and a 128-KiB resource
content limit. Continuations remain bound to current access, query and generation.
Malformed or changed cursors require a restart; oversized entries fail explicitly.
Original full-map and evidence URLs retain their text and metadata. Explicit
catalog version 1 continues to advertise the original full-map URLs.

Connect now reads compact maps and resolves each selected recipe dependency
through one exact lookup. Missing, ambiguous, mismatched or partial results stop
review. It does not download the entire index or require it to fit the old
8,192-entry inspection limit. The plan still pins the exact map and evidence
reads and keeps its 32-resource budget. A historical global selection stays on
the task's pinned publication when newer global guidance becomes current.

Complete HTTP response sizes in the synthetic growth fixture were:

| Evidence units | Original full map | Compact map |
| --- | ---: | ---: |
| 1 | 1,694 bytes | 1,596 bytes |
| 100 | 74,162 bytes | 1,600 bytes |
| 1,000 | 732,962 bytes | 1,602 bytes |

All 1,000 entries remain reachable through bounded pages; exact lookup retrieves
the last unit without traversing those pages. Tests cover wrong hashes, changed
query/grant/audience/generation, cross-API access, private evidence, oversized
entries, invalid query parameters, historical reads and legacy discovery.

`pnpm run verify` passed with 190 console tests, 57 crawler tests, typecheck,
lint, production build and root Go, with all 17 standalone CLI fixture scenarios
enabled. The separate client module also passed. The final legacy-discovery
assertion passed in the focused HTTP growth suite. Production-console browser
checks then passed private review with 9,005 units and five exact lookups and
public review with 9,003 units and four exact lookups. Both exported seven-resource
plans, passed in client 0.3.0 and imported matching reports. Public review retained
historical global publication 1. Desktop/light, 390px/dark and keyboard checks
passed; both temporary services were stopped.

These fixtures use synthetic extra index entries, deterministic AI and demo
identities. They do not establish customer application completion or production
OAuth/client compatibility. The change bounds response projections; backend
validation still materializes the full index. Explicit recipe lists, backend
catalog/index work, production client evidence, task/retrieval evaluations,
translation completion and upgrade/restore rehearsal remain open. No persistence
implementation or migration changed in this checkpoint.


## Targeted recipe discovery checkpoint — 2026-09-08

Default catalog version 2 `integration.recipes.list` now supports query words,
an exact API filter and cursor continuation. All words match task title, slug,
outcome or the API name/family/version pinned by that recipe. Results include
exact recipe URI/revision and each API's immutable revision/hash and version
labels. API labels resolve from the bound historical snapshot, never a newer
catalog entry. Current audience visibility and recipe drift checks still apply.
SDK selections remain in the recipe and its exact evidence; discovery does not
invent tested compatibility.

Pages hold at most 32 summaries or 64 KiB of encoded recipe-array JSON, ordered
by title then URI. Continuations bind the matching result, query, API filter and
current catalog/authorization context. Invalid arguments and changed scopes
require a restart. Explicit version 1 preserves the full zero-argument recipe
list and its existing schema. `integration.plan` continues to require an exact
match and never chooses an ambiguous task. Discovery instructions now direct
unknown tasks through the queryable list first.

The growth fixture copies one actual AI-generated, reviewed and published recipe
in a read overlay while preserving its canonical content and dependencies. This
isolates delivery size; it is not evidence of independently authoring 1,000 tasks.
Complete encoded response sizes were:

| Available tasks | Legacy full list | Version 2 first page |
| --- | ---: | ---: |
| 35 | 35,738 bytes | 53,824 bytes |
| 100 | 101,848 bytes | 53,890 bytes |
| 1,000 | 1,016,248 bytes | 53,892 bytes |

Exact API version metadata adds cost for small catalogs; page size stops growing
with the full task count, and query/API filters narrow it further. API history
is reused across tasks within a request. Backend recipe reconciliation and
catalog materialization still process the full scope; this checkpoint does not
claim bounded backend work.

Verification passed: `pnpm run verify` with 190 console tests, 57 crawler tests,
typecheck, lint, production build and root Go, plus the separate acceptance-client
module. Root verification enabled all 17 prior task CLI scenarios and three new
actual client 0.3.0 calls: anonymous first page, configured continuation page and
private API query with anonymous denial. Focused tests cover 32+3 completeness,
public/private filtering, exact historical and legacy API bindings, malformed
arguments, normalized queries, changed query/API/audience/availability cursors,
encoded-byte limits and oversized entries. No console layout, persistence
implementation or migration changed in this checkpoint.

Backend catalog/index work, production coding-client/OAuth evidence, meaningful
retrieval/task evaluations, translation completion and upgrade/restore rehearsal
remain open. The broad acceptance gates above stay unchecked.

## Published task retrieval and SDK upgrade checkpoint — completed

Added a versioned corpus and evaluation through actual ingestion, required AI
processing, review, publication, index activation, Query Lab ranking/context
selection and persisted traces. Both memory and PostgreSQL run five task queries:
authenticated reads, pagination, bounded retry, idempotent creation and webhook
validation. Twelve guides produce thirteen units in the selected SDK index.
Older published, unpublished and other-API SDK content are present as negative
controls. Expected source paths, task phrases, exact citations, candidate scope
and context bounds are checked. The previous embedding-only three-pair smoke
test remains distinct.

Both backends found all expected passages in the first five results. Precision
was 20% for four tasks and 40% for webhook validation; context was 660–767 tokens.
Four unavailable exact scopes returned no results or candidate evidence; a
cross-API publication was rejected. These are synthetic guidance-retrieval
results, not application success or real-user improvements. Reports include
corpus, pipeline, parser, normalizer, mapper, map and index versions, exact
publication/hash identities, expected evidence, metrics and local latency.
Application execution and the existing-documentation baseline remain explicitly
unmeasured. See [the evaluation record](retrieval-evaluations.md).

The PostgreSQL run found a real package workflow failure: after first API
publication, changing its SDK release returned a revision conflict because
historical publication/advisory foreign keys referenced the attachment's mutable
release/content/assertion selection. New append-only migration 0073 retains
stable attachment identity and direct immutable asset references. A locking
insertion guard checks the exact current selection when saving a new snapshot.
Existing history and advisory lineage guards remain in place. The test now
advances the same attachment from 0.9.0 to 1.2.3 and retrieves both exact
publications. PostgreSQL checks cover advisory history, stale/mismatched inserts,
immutable rows, deletion protection and concurrent selection updates.

Verification passed: `pnpm run verify` (190 console tests, 57 crawler tests,
typecheck, lint, production build and root Go, including the 20 enabled standalone
client scenarios), plus the complete Go suite against a fresh disposable
PostgreSQL 17/pgvector database. The final focused evaluation and database guard
suite also passed. A cloned populated SDK fixture upgraded from migration 0071
through 0073 without changing hashes/counts across ten retained tables: four API
publications/SDK selections, nine attachments, eleven SDK content publications,
nine releases, four integration revisions, four indexes and twelve knowledge
units. The cloned fixture had no compatibility assertion or advisory rows; the
focused PostgreSQL fixture separately exercised retained advisories. Replaying
the migrations made no further changes. No existing migration was edited.

This is an upgrade rehearsal, not a backup/restore test. Actual application-task
evaluations, production coding-client/OAuth evidence, broader retrieval cases,
backend catalog/index work, translations and a restore rehearsal remain open.
The broad acceptance gates above stay unchecked.

## Reference application outcomes and AI source processing checkpoint — completed

Added a fixed Orders SDK and reference webhook application, with an executable
evaluation driven by the five versioned retrieval tasks. The test imports the
actual checked-in package manifest, SDK and application source plus the guidance,
runs required AI processing and explicit review, publishes/attaches that SDK,
publishes its API and retrieves the task guidance through Query Lab. The Node
runner checks exact published source hashes and evidence identities before
starting local HTTP fixtures; retrieved content cannot select executable code,
commands, modules or endpoints.

Seventeen application checks passed with Node.js v22.14.0: authenticated reads,
invalid credentials without retries, incompatible SDK rejection, all cursor
pages, repeated/endless cursor limits, bounded Retry-After handling, a lost
creation response without duplicate orders, conflicting idempotency payloads,
webhook signatures/raw bytes/timestamp boundaries, malformed payloads, duplicate
events, concurrent duplicates and a failed side effect retried successfully.
Six wrong or incomplete evidence packets were rejected before application
execution. Reports record the runtime, expected/actual checks, source/input/
runner hashes, SDK/API publication identities and retrieval trace references.

This is a manually authored reference application against a loopback vendor.
Retry waits are recorded with an injected clock; deduplication is in memory
within one process. The report explicitly leaves production coding-client
implementation and the existing-documentation comparison unmeasured. It does not
change compatibility assertions or mark published material Tested/Verified. The
MCP acceptance report and console implementation record remain separate. See
[the executable evaluation](../examples/integration-evaluation/README.md).

The workflow exposed an AI processing false positive: JSON encoding turned the
newline after `this.apiKey = apiKey;` into literal escape characters, making an
ordinary variable assignment appear to contain a credential. The AI boundary now
visits decoded JSON values and preserves the existing literal assignment key
vocabulary/thresholds. It scans every duplicate-key occurrence, escaped Unicode
credentials, strings used as keys, and numeric credential literals. Ordinary
authorization flags and field schemas remain valid. Malformed JSON retains the
text check. Focused regressions verify real/escaped/duplicate secrets are denied
before a provider call with a generic error; required AI processing is preserved.

Full verification passed: 190 console tests, 57 crawler tests, typecheck, lint,
production build and root Go, including the 20 enabled standalone MCP client
scenarios and the new application/AI-boundary checks. No persistence, migration,
public API contract or console layout changed in this checkpoint; PostgreSQL and
browser layout checks were not rerun.

Production coding-client/OAuth evidence, real vendor/customer tasks compared
with existing documentation, broader retrieval cases, backend catalog/index
work, translations and backup restoration remain open. The broad acceptance
gates above stay unchecked.

## Product walkthrough follow-up — 8 September 2026

Browser inspection found and corrected a stale version when changing packages,
an empty-library reader asking users to select a nonexistent file, and a published
SDK screen that put completed processing ahead of its guidance. Package changes
now clear the previous version/ref; empty searches have a distinct message and
Clear filters action; published SDK processing is available in the existing
details section. Required AI remains visible for unpublished guidance. The new
copy is translated in all seven UI locales.

Repository verification passed again (190 console tests, 57 crawler tests,
typecheck, lint, production build and root Go). Actual browser interactions
verified version reset/readiness, published processing disclosure, pending-content
navigation and keyboard search/source-filter recovery. Desktop/light and
390px/dark screenshots were inspected without overflow or JavaScript errors.
Populated-library reading used explicit normalized-document UI fixtures; the
remaining screens used the local Go service with test authentication and AI.
No persistence, public API or runtime authorization changed. These checks do not
close the production client, customer-outcome or operational acceptance gates.

## Knowledge navigation consolidation — 8 September 2026

An audit against the original approved review found that Docs and SDKs still
occupied separate primary navigation entries. They now share Knowledge, which
opens the reviewed-document library and offers Documents, API contracts, SDKs and
packages, Sources and Query Lab. API and Recipe workspaces remain prominent.
Asset IDs, URLs, publication semantics and resumable setup parameters are unchanged.
The Knowledge label is translated in all seven UI locales.

Repository verification passed with 190 console tests, 57 crawler tests,
typecheck, lint, production build and root Go tests. Updated rendering checks
cover grouping and the one current tab for each destination. Production-build
browser checks passed for all five destinations, keyboard navigation, browser
Back/Forward, direct SDK links, the mobile group selector and the existing setup
walkthrough. Desktop/light and narrow/dark captures had no page overflow or
JavaScript errors. The walkthrough's normalized document responses and test
authentication retain their previously stated fixture limitations.

## Local restore rehearsal — 8 September 2026

The opt-in PostgreSQL restore test passed against PostgreSQL 17.11 and pgvector
0.8.6, using actual Homebrew `pg_dump` and `pg_restore` 18.4. It created two
isolated databases, preserved row counts/content hashes across all 123 public
tables, replayed migrations without changing them, and retrieved the same three
guidance results from the same immutable API publication. A real multipart
upload retained its bytes and private file permissions. HTTP root login with MFA,
credential decryption, readiness and anonymous Private MCP denial passed, along
with six missing/corrupt-input and wrong-key checks. The owned databases were
removed successfully at test cleanup.

The test and recovery runbook make database/upload quiescence explicit and can
write a machine-readable evidence report with versions, hashes and limitations.
AI is a fixture; HTTP handlers run in process. Recorded deployment image and
crawler restarts, actual escrow recovery, production OAuth/client validation,
a private runtime call and production-size recovery timing remain outside this
local evidence. The broader operational acceptance gate remains open.

Full repository verification then passed with the restore drill enabled and a
fresh disposable PostgreSQL database for the root Go suite: 190 console tests,
57 crawler tests, typecheck, lint, production build and all root Go packages.
The standalone-client fixture scenarios and reference-application evaluation
were enabled in that run.

## Required AI setup localization — 8 September 2026

The complete AI readiness, Knowledge processing and asset-save recovery sections
now have German, Spanish, French, Japanese, Brazilian Portuguese and Ukrainian
translations: 57 messages per locale. This includes required-AI guidance, provider
and budget blockers, connection-test states, resumable processing, source-review
findings and save recovery. Interpolation fields remain unchanged. Japanese
workload instructions use the same “解析” label as AI settings.

All 342 localized runtime lookups resolved with their expected interpolation.
The existing locale contract checks cover key parity and placeholder parity;
other setup and review sections still contain English copy and remain follow-up
work. No AI policy, processing behavior, persistence or public contract changed.
Final repository verification passed: typecheck, lint, production build,
190 console tests, 57 crawler tests and the root Go suite. Environment-dependent
PostgreSQL and external-client checks were not enabled for this copy-only change.
