# DokoSoko service

DokoSoko is a self-hosted MCP connector for a vendor's developer platform. One
installation publishes the material developers and agents need to use the
vendor's APIs:

- reviewed documentation and OpenAPI contracts;
- exact, API-owned SDK installation references;
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
tools, and runtime service connections. There are no product channels, pins,
staged promotion, rollout, package catalogue, or package provenance workflow.

SDK entries contain an ecosystem, coordinate, exact version, install command,
optional HTTPS documentation/source URLs, optional digest, and visibility.
DokoSoko never hosts or proxies package bytes.

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

Administrators review crawl output and quarantine indicators before publishing
an immutable source revision. A published API pins the exact reviewed source
publication rather than a mutable crawl result.

## AI providers and recipes

AI remains optional and provider-neutral. One Analysis workload powers bounded
integration planning, tool-authoring assistance, and recipe authoring/review. It
can select OpenAI, Google, Anthropic, DigitalOcean, xAI, DeepSeek, or a fixed
public OpenAI-compatible endpoint. Provider credentials, the model, token
limits, daily budget, and one backup model remain explicit. Failover occurs only
for configured transient failures; invalid configuration, unsafe input,
exhausted budgets, and invalid output do not fail over. A retry sends the same
bounded prompt and reviewed evidence to the configured backup provider once.

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
automatically.

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
