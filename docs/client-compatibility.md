# Recorded client and task-retrieval evidence

This record distinguishes protocol/retrieval checks from customer implementation
results. Client branding, setup commands and a successful HTTP response do not
establish compatibility or task completion.

## Current evidence

| Client/build | Service and authentication | Checks exercised | Result and limits |
| --- | --- | --- | --- |
| DokoSoko MCP acceptance client 0.2.0, built from this worktree | Actual Go HTTP handler over loopback; in-memory store; private demo identity | Stateless MCPv2 `2026-07-28` discovery; AI-generated, reviewed and published recipe; exact recipe revision, API publication map, contract operation and selected contract/SDK evidence; anonymous denial | Passed with seven exact resources. Deliberately wrong map hash, recipe revision, SDK evidence and contract-operation text failed as required. AI used deterministic fixtures. This does not establish production OAuth or another coding client's behavior. |
| DokoSoko MCP acceptance client 0.2.0, built from this worktree | Actual Go HTTP handler over loopback; in-memory store; anonymous public endpoint | Exact public recipe, API publication, three SDK units, historical global publication and its selected unit after newer global guidance becomes current | Passed with seven exact resources. Wrong map hash, recipe revision, SDK evidence and global evidence failed as required. The historical publication remained readable without appearing in current discovery. |
| Chromium console and acceptance client 0.2.0 | Actual Go service; in-memory store; test administrator session; private demo identity and anonymous public endpoint | Published-task selection; exact browser-exported plans; CLI execution and matching report import; wrong-plan rejection; separate application fields and exported evidence; audience/reload reset; publication-review navigation; readable evidence kinds and global revision; disabled public endpoint and failed-report import | Private contract/SDK and public SDK/historical-global journeys passed, including desktop/light, narrow/dark and keyboard actions. Entered application results were browser-test fixtures; no customer application was executed. Production OAuth remains unverified. |
| Other coding clients | No current run recorded for this implementation | Not evaluated | Setup instructions alone are not compatibility evidence. |

Subsequent runs used DokoSoko MCP acceptance client **0.3.0**, built from this
worktree. All 17 private, public and paginated task scenarios passed their
expected success/rejection assertions against actual local Go HTTP handlers.
The paginated fixture has 35 API maps plus its recipe and locates the task on
page two. Reports preserve each discovery page's request IDs and byte counts;
exact observations identify the page that advertised the task. The SDK/contract
and historical-global evidence checks retain the fixture limitations above.

Catalog version 2 was also exercised with the production console and actual
local service. Chromium passed first-page task/API routing, tool pages of 32+10,
grant-dependent 32+11 pages, public filtering, keyboard navigation, exact JSON
copy and context resets in desktop/light and 390px/dark layouts. Client 0.3.0
found a tool on page two, found `deployment.apis.get`, called
`deployment.apis.list`, and confirmed anonymous private denial. The fixture's
API list was empty; exact nonempty API selection, publication pins and changed
publication rejection are covered separately by HTTP tests. These runs used
in-memory storage and demo identities, not production OAuth or a customer's
coding client. No application implementation was executed.

A further production-console/client 0.3.0 run exercised compact publication maps
with large API indexes: 9,005 private units and 9,003 public units, including 9,000
synthetic unrelated entries in each fixture. Task review performed five and four
exact source lookups respectively and exported only seven required resources.
Both actual client runs passed and their matching reports imported successfully.
The public plan pinned global publication revision 1 after revision 2 became
current. Old exact resource contents remain covered by HTTP regression tests;
explicit legacy catalog version 1 still advertises original full-map URIs.
The extra units isolate response growth and selection behavior; they are not
acquisition, external AI quality, production identity or application-test evidence.

Recipe catalog version 2 was subsequently checked with actual client 0.3.0 calls
for an anonymous first page, an explicitly configured continuation page, and a
private API-filtered query with anonymous private denial. All passed and retained
request correlation in their reports. HTTP tests checked complete 32+3 paging,
exact recipe/API pins, public filtering and invalidated continuations; a synthetic
1,000-task projection measured response growth. Historical API label selection
is covered by focused revision-snapshot tests, including legacy recipe bindings.
These checks establish the fixture's discovery/call behavior, not a customer's
coding client, production OAuth or application completion. The client does not
automatically traverse a tool-specific `next_cursor`; each catalog call was
configured explicitly.

The standalone suite additionally exercises OAuth/DCR/PKCE fixtures, transport
boundaries, grant filtering and confirmation. Those tests prove those fixture
scenarios; they do not establish a production identity-provider/client pairing.

## Reproduce the service/client check

Build the standalone module, then pass its absolute executable path to the root
HTTP suite. The root suite does not silently build or import the separate module.
Run from the repository root:

```sh
(cd examples/mcp-acceptance-client && go build -o /tmp/dokosoko-mcp-task-acceptance ./cmd/mcp-acceptance)
DOKOSOKO_MCP_ACCEPTANCE_CLIENT=/tmp/dokosoko-mcp-task-acceptance \
DOKOSOKO_ACCEPTANCE_EVIDENCE_DIR=/tmp/dokosoko-task-connection-evidence \
go test ./internal/httpapi -run '^TestPublished(Public|Paged)?TaskWithStandaloneAcceptanceClient$' -count=1 -v
```

The tests write JSON reports for successful private/public tasks and rejection of
wrong map hashes, recipe revisions and SDK evidence, plus paginated private
discovery, wrong private contract
evidence, wrong public global evidence and anonymous private denial. Each report contains the
actual client version, protocol revision, timestamp, reviewed-plan fingerprint,
request IDs and observed content hashes. The service fixtures are temporary;
these files are a record of the run, not links to a persistent deployment.
Resource bodies, tokens and tool arguments are excluded from reports.

The SDK fixture uses normal import, mandatory AI processing, explicit content
review, publication and API attachment. The contract fixture supplies a normalized
candidate and reviewed historical revision directly to the store and runs the
mandatory processing stage; it does not exercise contract source acquisition.
The public global fixture supplies reviewed historical collection revisions;
global publication and advancement use the normal service. It does not exercise
global source acquisition. API publication, index construction, recipe
generation/review/publication and MCP reads run through the actual service.

## Evidence still required

The API **Connect** UI now selects a published task, checks its canonical text and
exact API publication maps and selected evidence through the runtime preview, and exports a matching
CLI plan. It imports scoped client reports and exports implementation reports
with their evidence origins kept separate. These records remain local until
downloaded. A server preview using simulated grants cannot prove that a customer's
OAuth login or permissions work. The private browser/CLI journey includes selected
SDK and contract evidence; the public journey includes SDK and historical global
evidence. Focused transport tests also cover multiple API publications, equivalent
global publications, unused private publications, exact member hashes and distinct
global requirements. These transport scenarios do not establish production client
behavior.

For every supported production client/version and authentication mode, record an
actual connection, exact task retrieval, implementation check and failure case.
Record application/runtime/SDK versions, expected result, actual result and
assistance required. Keep independently run evaluations separate from customer
reports; neither a connected badge nor a retrieval pass implies compiled or
working application code. Operational upgrade/restore and comparative task
performance evaluations remain outstanding.
