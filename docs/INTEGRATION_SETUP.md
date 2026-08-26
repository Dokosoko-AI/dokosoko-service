# Integration setup contract

An Integration is one versioned API in DokoSoko. Its workspace has six tabs:

1. **Quick Start** — readiness and the next incomplete step.
2. **Documentation** — reviewed documents, API contracts, and exact SDK refs.
3. **Keys & Access** — fixed runtime service origins and encrypted credentials.
4. **Tools** — API-owned and attached common tools.
5. **Test** — deterministic contract and live-upstream checks.
6. **History** — immutable API publications and audit history.

Recipes, customer identity, agent access, upstream MCP connections, the support
outbox, and service settings are deployment-wide workspaces. They link back to
the APIs that use them.

## Setup flow

### 1. Define the API

Create a private draft with a stable family key, exact version key, display
metadata, and lifecycle. Public visibility always requires explicit
acknowledgement.

### 2. Ingest and review evidence

Add documentation or OpenAPI sources, crawl them, inspect classifier and
quarantine findings, and publish only selected documents. Attach immutable
source publications to documentation or API-contract resource sets.

Add SDK references directly to the API. Each reference names one ecosystem,
coordinate, exact version, install command, optional documentation/source URLs,
optional digest, and visibility. Ranges and `latest` are invalid. External
registries deliver packages; DokoSoko stores no catalogue, bytes, provenance,
release chain, or lifecycle.

### 3. Configure keys and access

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

Analyse bounded published evidence, resolve unknowns, review citations and
commands, and publish an immutable recipe revision. Evidence changes make
affected recipes stale until reviewed; no model output publishes itself.

### 6. Test and publish

Preflight resolves every selected resource, SDK, authorization point, tool, and
service connection. Required failures deny publication. Publishing stores one
immutable Integration snapshot containing exact revisions and content hashes.

The acceptance client should cover protocol negotiation, OAuth metadata and
PKCE when enabled, resources list/read, grant-filtered tool discovery, fixed
upstream calls, confirmation, revoked access, and request/audit correlation.

## Delivery

Private MCP serves authorized customer resources and tools. Optional Public MCP
serves only explicitly public, published, read-only material and is disabled by
default. Agent setup pages describe both endpoints.

Support tools require a user-approved preview and append a bounded plaintext
record to the local queued outbox. They do not route or deliver it.

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
