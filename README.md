# DokoSoko service

DokoSoko is a self-hosted MCP connector for a vendor's developer platform. One
installation publishes the material developers and agents need to use the
vendor's APIs:

- reviewed documentation and OpenAPI contracts;
- reusable SDK packages with exact, reviewed guidance selected by each API;
- runtime service connections and encrypted credentials;
- reviewed HTTP, upstream MCP, and trusted native tools;
- evidence-grounded recipes;
- optional private customer OAuth and anonymous read-only Public MCP.

The normative administration contract is [api/openapi.yaml](api/openapi.yaml).
Vendors implementing private customer access use the optional
[identity integration contract](api/identity-integration-openapi.yaml).

## Product boundaries

An API is an `Integration`. Publishing it creates an immutable snapshot of its
reviewed documentation, contracts, exact SDK references, authorization points,
tools, and runtime service connections. Shared documentation, contracts, and SDK
guidance have their own review histories; existing API publications retain their
selected revisions when those shared assets advance.

SDK packages belong to the deployment and can be reused across APIs. Package
setup resolves an exact registry or Git release, imports bounded guidance text,
requires AI processing and human review, and publishes that content before API
attachment. Normal setup shows readable files and examples; package metadata,
lifecycle history, symbols and maps are available through advanced inspection.
DokoSoko does not install or execute packages, or host or proxy package bytes.

SDK review shows literal sample code with its language and source location.
Comparisons show the previous published source file; sample approval applies to
the selected sample. Reload preserves decisions for the same content and requires
fresh acknowledgement. Completion verifies the saved exact API attachment,
including its audience, selectors and compatibility claims.

The API **Connect** tab starts with a published task and audience. Review its
canonical instructions, exact API publications and selected supporting evidence,
copy a contextual connection
prompt, and download a check plan. Imported client reports and recorded application
checks stay separate from the administrator preview; export the evidence record
to retain them across reloads.

The standalone MCP acceptance client can check an exact reviewed task plan,
including task revision, guidance text hashes and publication metadata. Its
reports separate client-observed retrieval from application tests, which it does
not run. See [recorded client evidence](docs/client-compatibility.md) for the
current results and the production checks still required.

The [published guidance retrieval evaluation](docs/retrieval-evaluations.md)
records task evidence recall, precision, exact citations and version isolation
against memory and PostgreSQL. It does not establish application test success.
The separate [reference application evaluation](examples/integration-evaluation/README.md)
runs actual HTTP and webhook checks using a published fixture SDK; its reports
record the runtime, exact source hashes, expected outcomes and failure cases.

MCP catalog version 2 is the default: initial discovery contains deployment
summary and routing information. Use `deployment.apis.list` to find an API by
name/version and `deployment.apis.get` with its exact manifest hash to read that
publication. Follow `nextCursor` for tool/resource lists and `next_cursor` for the
API catalog. Existing consumers of the full `result.deployment`/`result.product`
extensions can request `params._meta["com.dokosoko/catalogVersion"] = 1` on each
request while migrating. The [setup guide](docs/INTEGRATION_SETUP.md#mcp-catalog-migration)
describes the compatibility contract and limits.

Compact publication maps use `/map-v2` resource URIs. Their index supports paged
browsing, query words and exact source filters. Connect uses these filters to
retrieve only the task's selected references, even when its API has a large
knowledge index. Original full-map and evidence URLs retain their contents.

Find a published task with `integration.recipes.list`, using `query` words or an
exact `api_id`. Follow `next_cursor` with the same filters. Each result identifies
the exact recipe revision and its pinned API names, versions and publication
hashes. Read the selected recipe URI and confirm its `revision_id`; its recipe
and evidence contain the SDK selections and implementation checks.

Contract setup keeps the chosen primary-contract setting for the exact candidate
and API across reloads. If publication succeeds before attachment fails, continue
with **Attach exact revision**. The saved revision is reused; final approval is
required again and completion checks the actual attachment settings.

Website and file source creation supports recovery after a lost response. Add
content and contract setup retain a request identity for seven days in the current
browser; retry with the same URL or file. API clients can send `Idempotency-Key`
to either source-creation endpoint. The same administrator, deployment, key and
input recover the current source; changed input with the same key returns a
conflict. Creation alone does not ingest, process or publish content. See the
[setup guide](docs/INTEGRATION_SETUP.md) for the remaining review steps.

**Knowledge** is the shared home for documents, API contracts and SDK guidance,
with Sources and Query Lab available in the same area. Existing asset URLs and
saved package/setup links remain valid.

**Add content** in Knowledge or API Resources opens one documentation setup page:
website/file/existing source, import, required AI processing, readable review,
and a saved documentation version. Starting from an API retains that API and
attaches the exact reviewed version to its draft. Import history, diagnostics
and provenance remain secondary. API publication is a separate review action.

Knowledge opens **Reviewed content**: included files from each source’s latest
reviewed publication. New or failed imports do not replace those files.
**Needs attention** lists unfinished website/file imports, including sources
without files, and resumes their exact setup. History and documentation sets are
secondary. API attachment and serving status remain in the API workspace.

For blocked uploaded content, **Replace uploaded file** keeps the source and
earlier reviews, stores a corrected file, and queues its import atomically.
Replacement waits for active imports to finish and preserves quarantine until
the crawler accepts clean content. Reload recovers a committed replacement;
required AI processing and human review run against the new import.
Review and the reviewed-content library compare a replacement with the earlier
approved file from the same source, even when its storage filename changed.

Runtime credentials are write-only, encrypted at rest, and attached to fixed
service origins. HTTP tools cannot choose destinations at request time. Trusted
native tools are reviewed Go source compiled into the service; see
[Native tool plugins](docs/NATIVE_TOOL_PLUGINS.md).

An upstream MCP connection uses one write-only service access token. It may
optionally forward a bounded DokoSoko user-identity envelope signed with that
token. The inbound DokoSoko bearer token is never forwarded. Imported tools
remain local reviewed definitions, and schema changes fail closed until
reviewed.

Support reports are consent-gated, schema-bounded plaintext records in a durable
outbox. Root settings provide separate feedback and error destinations. The
service snapshots the selected destination, delivers without redirects, and
uses bounded leases and retries; no support payload encryption layer is added.

The embedded widget is not part of this service. A future separately deployed
widget can use standard private OAuth and Private MCP; the discovery scaffold is
in [extensions/widget-plugin](extensions/widget-plugin/README.md).

## Private customer access

Private MCP can configure one upstream OIDC provider. DokoSoko owns the
downstream authorization server, PKCE flow, resource-bound access tokens,
customer-account suspension, grant checks, and confirmation enforcement.

```text
MCP client
  -> DokoSoko authorization code + PKCE
  -> vendor OIDC sign-in
  -> fixed /v1/access/evaluations request
  -> resource-bound DokoSoko token
  -> POST /mcp
```

Identity, account state, access evaluation, schema, grants, and confirmation all
fail closed. DokoSoko tokens are never forwarded to vendor services.

## Protocol endpoints

| Surface | Endpoint |
| --- | --- |
| Health and readiness | `/healthz`, `/readyz` |
| OAuth metadata | `/.well-known/oauth-authorization-server` |
| MCP protected-resource metadata | `/.well-known/oauth-protected-resource/mcp` |
| OAuth | `/oauth/register`, `/oauth/authorize`, `/oauth/callback`, `/oauth/token` |
| Private MCP | `/mcp` |
| Public MCP | `/mcp/public` |
| Agent setup | `/agent-setup/private/prompt.md`, `/agent-setup/public/prompt.md` |
| Administration | `/api/v1/...` |

Public MCP is anonymous, read-only, rate-limited, and disabled by default. Only
explicitly public, published API material is discoverable there.

## Run locally

Requirements: Go 1.25+, Node.js 22.13+, pnpm 11.19+, and PostgreSQL with
pgvector.

```bash
pnpm install --frozen-lockfile
pnpm build
export DOKOSOKO_DATABASE_URL='postgres://...'
export DOKOSOKO_MASTER_KEY='base64-encoded-32-byte-key'
export DOKOSOKO_SETUP_TOKEN='one-time-random-setup-token'
go run ./cmd/dokosoko
```

The integrated service listens on `http://localhost:8080` by default and serves
`dist/client`. For disposable development only, set
`DOKOSOKO_DEV_MEMORY=true`. Demo bearer tokens additionally require
`DOKOSOKO_ALLOW_DEMO_TOKENS=true`.

For deployment-owned settings, copy [`dokosoko.config.example.json`](dokosoko.config.example.json),
set `DOKOSOKO_CONFIG_FILE` to its path, and mount the same file into the service
and crawler. The versioned JSON file is strict and uses the precedence
`built-in defaults < central file < environment variables`. Secrets are
references to environment variables or mounted files rather than plaintext
JSON. **Settings → Configuration** shows the effective source of every startup
value with secrets redacted. Startup configuration changes require a process
restart.

The optional `control_plane` section manages stable organisation and deployment
identity, submission URLs, and an additive set of environments. On a new
database it can create the initial organisation, deployment, and environments;
on an existing database the organisation identity must match and configured
deployment fields are reconciled at startup. Managed tenant fields are marked
read-only in the console and conflicting API writes return
`configuration_managed`. Environments are added but never renamed or deleted by
startup reconciliation.

Central configuration intentionally does not own integrations, sources,
reviewed publications, tools, recipes, identity-provider activation, root
accounts, or public MCP publication. Those records have review, revision,
secret-testing, or MFA lifecycles and remain database-backed console/API
workflows. The JSON shape is documented by
[`dokosoko.config.schema.json`](dokosoko.config.schema.json).

The setup token is required only until the first MFA-protected root account has
been created. Remove it from the deployment after setup; subsequent restarts
disable the setup endpoint's token path while keeping normal root login active.

Run the console development server with:

```bash
DOKOSOKO_DEV_PROXY=http://127.0.0.1:8080 pnpm dev
```

## Documentation ingestion

The supported documentation-ingestion paths are `website`, `openapi`, and
`upload`. Website and OpenAPI requests are credential-free, budgeted, redirect
checked, DNS-rebinding resistant, and restricted to allowed public destinations.
Uploads are UTF-8, byte-bounded, path-contained, and read from the dedicated
upload volume. The legacy `git` source kind remains reserved in the API for
compatibility, returns an explicit unsupported-source result, and is not shown
as an available option in the console.

Administrators complete required AI processing, then review the exact content
and quarantine indicators before publishing an immutable source revision. The
crawler retains no AI credentials. The Go service processes bounded batches
with the configured Analysis provider and saves each result for the exact import.
Source, contract, and SDK review screens offer progress and pause/resume controls.
Retries reuse successful batches; model failures cannot silently publish content.
Older imports without normalized evidence must be ingested again; OpenAPI sources
must be attached to a contract before that ingestion. The contract **Create & set
up** flow performs that attachment, starts the import, presents exact content and
AI findings, and can publish and attach the reviewed revision to the originating
API. It retains progress through reload and reuses completed publication steps
when an attachment fails. See [Integration setup](docs/INTEGRATION_SETUP.md).
A published API pins the exact reviewed source
publication rather than a mutable crawl result.

## AI providers and recipes

AI is required for Knowledge processing and recipe authoring/review. One
provider-neutral Analysis workload powers bounded Knowledge processing,
integration planning, tool-authoring assistance, and recipe authoring/review. It
can select OpenAI, Google, Anthropic, DigitalOcean, xAI, DeepSeek, or a fixed
public OpenAI-compatible endpoint. Provider credentials, the model, token
limits, daily budget, and one backup model remain explicit. Failover occurs only
for configured transient failures; invalid configuration, unsafe input,
exhausted budgets, and invalid output do not fail over. A retry sends the same
bounded prompt and reviewed evidence to the configured backup provider once.
Required Knowledge processing uses the configured primary provider only. Its
input is imported content awaiting approval; it has no tools or execution rights.
Existing completed processing and historical publications remain readable without
a new provider call. AI summaries and findings are review aids, not approval,
compatibility assertions, or proof that code was tested.

The console shows AI setup before imports and required processing, and disables
recipe generation while a local prerequisite is missing. AI settings starts with
connecting a provider, then selecting the Analysis model. Setup links retain the
originating API/contract/package route; links from an unsaved form open settings
in another tab so the form remains available. Return to that tab and refresh
readiness after configuration.

The administrative `GET /api/v1/ai/readiness` projection checks the same local
provider/model/credential prerequisites as processing, plus daily usage and live
unexpired token reservations. It neither calls a provider nor reserves tokens.
`can_process` means a request can be attempted, not that the provider is reachable,
the account has model access, or the next batch fits the remaining budget. Actual
calls still reserve their exact budget. **Test connection** explicitly sends a
small test prompt; its result is historical and is invalidated after a provider
save or a newer model configuration. Missing models, disabled workloads/providers,
unavailable credentials, invalid configuration, and exhausted daily budgets have
separate actionable states. Completed processing and publications do not become
unreviewable just because current AI setup is unavailable.

Recipe processing stops if a required AI stage fails; there is no silent
non-AI authoring fallback. The console receives distinct errors for missing
configuration, rejected credentials, exhausted budgets, unavailable providers,
and invalid structured output. Reviewed publication and delivery of existing
material do not require runtime tools or service credentials.

The integration-analysis, recipe-brief, recipe-authoring, and recipe-review
instruction bodies are versioned per product and can be restored to their safe
defaults. DokoSoko applies its immutable safety policy separately; operators
cannot edit or disable it through prompt configuration.

Recipes are immutable, reviewed deployment-level product-integration plans
grounded in bounded published evidence. A coding agent discovers them only
after connecting through MCP, so recipe content never explains how to connect
to DokoSoko. A recipe may attach multiple APIs when one coherent workflow
requires exact capabilities from each; every revision freezes those APIs'
published revisions and manifest hashes. The generator detects eligible APIs
from reviewed evidence, while the server owns the attachments, instructions,
validation, and publication boundary. Generated content is never published
automatically. References are selected by name from exact retained evidence;
manual JSON editing is not required. Saving references or visibility runs AI
review and creates a revision for human approval. Rework changes instructions
through the required AI workflow. Failed saves retain choices in the open dialog,
and an explicit reload recovers the current saved revision.

## Deploy and verify

```bash
cp .env.example .env
docker compose up --build
```

`DOKOSOKO_DATABASE_PASSWORD` and `DOKOSOKO_PUBLIC_URL` are required in the
Compose deployment. Supply the master key through `DOKOSOKO_MASTER_KEY` or a
central-file secret reference. `DOKOSOKO_SETUP_TOKEN` is required for the first
start only. Keep the master key stable and backed up.

```bash
pnpm run verify
docker compose config
```

Production-shaped Terraform roots for AWS, DigitalOcean, Azure, and Google
Cloud are documented in [`deploy/terraform`](deploy/terraform/README.md). They
deploy the service and crawler from immutable image digests and make the
current single-replica shared-filesystem constraint explicit.

PostgreSQL integration tests run when `DOKOSOKO_TEST_DATABASE_URL` or
`TEST_DATABASE_URL` points to a disposable database with pgvector. The MCP
acceptance client is a separate Go module under
`examples/mcp-acceptance-client`.

## Repository map

```text
app/                    React console and generated API client
api/                    normative OpenAPI contracts
cmd/dokosoko/           service entry point
crawler/                isolated documentation crawler
extensions/             external plugin discovery scaffolds
internal/identity/      private OAuth, OIDC, customers, and grants
internal/mcpbridge/     token-authenticated upstream MCP import and proxy
internal/nativeplugins/ trusted compiled-in tool plugins
internal/reporting/     plaintext local support outbox
internal/tools/         reviewed HTTP/native tool execution
internal/platform/      catalog, publication, recipes, and policy
internal/store/         memory and PostgreSQL persistence
migrations/             append-only checksummed schema history
```

See [docs/FINAL_PLAN.md](docs/FINAL_PLAN.md) for the core invariants and
[docs/INTEGRATION_SETUP.md](docs/INTEGRATION_SETUP.md) for the API workflow.
Production operators should follow the [operations and launch runbook](docs/OPERATIONS.md).
