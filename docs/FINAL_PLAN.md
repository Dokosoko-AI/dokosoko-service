# DokoSoko core product contract

DokoSoko is one MCP connector for documentation, SDK references, runtime keys,
recipes, and reviewed tools. The service is deliberately not a developer
portal, provisioning platform, widget product, support-delivery system, or
product-release orchestrator.

## Core resources

- `deployment` — the single vendor catalog and Public MCP master switch;
- `integration` — one versioned API and its immutable publications;
- `source`, `source_publication`, and documentation/API `resource_set`;
- `sdk_reference` — one exact API-owned install reference;
- runtime service connections and versioned encrypted credential sets;
- authorization points and reviewed HTTP, MCP, or native tool revisions;
- immutable evidence-grounded recipes;
- customer identity and OAuth artifacts for optional Private MCP;
- upstream MCP connections using a service access token and optional signed
  user-identity forwarding;
- consent-based plaintext queued support submissions;
- append-only audit plus bounded analytics needed for AI budgets and recipe
  popularity.

## Publication model

An Integration publication is the delivery boundary. It resolves and stores the
exact source publications, SDK references, authorization points, tool revisions,
and runtime connection revisions selected for that API. The snapshot and hash
are immutable.

There are no connector releases, Latest/LTS channels, customer pins,
installations, staged promotion, rollout percentages, product drift records,
package releases, provenance lifecycle, provider-owned resource instances, or
integration-run analytics.

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
advisory: generated setup guides, tool drafts, and recipes require deterministic
validation and human review; recipe publication is immutable and always
explicit.

The four core analysis and recipe workflows have stable prompt keys. Their
workflow-specific instructions are versioned and resettable, while the common
untrusted-input and no-tool safety policy remains server-owned and immutable.

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
