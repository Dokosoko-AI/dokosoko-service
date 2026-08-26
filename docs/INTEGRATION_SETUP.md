# Integration setup contract

This document defines the vendor-neutral workflow for configuring one versioned
API Integration in DokoSoko. The console and control-plane API should expose the
same stages, validations, and immutable publication inputs.

## Workspace information architecture

Every Integration workspace exposes these primary tabs:

1. **Overview** — identity, lifecycle, readiness, and the next incomplete stage.
2. **Resources** — Documentation, API contracts, and optional SDKs & Packages.
3. **Authorization** — identity binding, grant registry, authorization points,
   service connections, and policy simulation.
4. **Tools** — reusable HTTP, upstream MCP, or trusted source-compiled native
   tools bound to the Integration by exact reviewed revision.
5. **Recipes** — evidence-grounded guidance generated from published inputs.
6. **Delivery** — Private MCP, optional Public MCP, widget, agent setup, and
   support/reporting configuration.
7. **Test** — deterministic conformance and end-to-end acceptance evidence.
8. **History** — immutable publications, diffs, deprecations, and replacements.

Reusable deployment-wide catalogues may also exist, but they must show their
Integration bindings and must always be reachable from the Integration setup
flow.

## Standard setup flow

### 1. Define

Create a private draft with a stable API family and version, display metadata,
environment applicability, lifecycle, and any replacement or sunset policy.

### 2. Ingest evidence

Ingest authoritative documentation and API contracts from supported sources.
Review fetched pages, validation findings, exclusions, quarantine indicators,
and content diffs. Publish immutable resource revisions and attach exact
revisions to the Integration. A draft may follow the latest resource revision;
a published Integration always resolves and records an exact revision and hash.

SDKs and packages are optional developer artifacts. DokoSoko stores bounded
catalogue metadata: stable package identity, exact release compatibility,
operator-supplied display-only install guidance, a declared digest, and optional
provenance and SBOM locations. Package-specific URLs reject userinfo, queries,
and fragments; URL paths and free-text fields are not comprehensive secret
scanners, so the operator must keep credentials out of every field. The
Integration binds one exact immutable release, whose metadata and metadata
content hash are embedded in the published manifest. The external registry
continues to deliver bytes and enforce registry access. DokoSoko does not
download, host, execute, verify, sign, or proxy package bytes.

### 3. Secure

Choose whether the Integration has public discovery, private customer access,
or both. Private access binds a reusable identity profile and a fixed,
versioned access-evaluation contract. Register grants before assigning them to
tools or authorization points. Creating a public package artifact, changing a
private package draft to public, and publishing each public package release also
require explicit public acknowledgement.

An authorization point is declarative policy, never an arbitrary callback URL.
It records an action, risk, required grants, confirmation policy, and decision
freshness. Simulation covers allow, deny, expiry, suspension, and revocation
without treating simulated results as real authorization.

### 4. Build capabilities

Create an HTTP tool, inspect and import an upstream Stateless MCPv2 tool, or
inspect a source-managed native Tool staged by the current service build.
Review the exact input and output schemas, endpoint mapping, authentication
or identity requirement, grants, effect, confirmation, timeout, idempotency,
state scope, limits, and redaction policy.
Test the draft and bind an exact reviewed revision to the Integration before
publication.

Native packages are operator-installed application source, not console uploads.
Review the package and dependency tree, run its conformance and strict source
checks, add it to the explicit Go registry, configure only its registered
`DOKOSOKO_PLUGIN_<ID>_<KEY>` variables, and rebuild. The console displays key
names and configured/missing state but never values. A native source-contract
change stages a new draft revision instead of rewriting a publication. See
[Native tool plugins](NATIVE_TOOL_PLUGINS.md).

Tools are reusable deployment capabilities; they are not owned by a single API.
Bindings state which published Integration revisions claim compatibility with a
tool revision.

### 5. Generate guidance

Analyse only published, bounded evidence. Resolve explicit unknowns, generate
recipes, review citations and commands, approve an immutable revision, and then
publish it. Evidence drift removes affected guidance from normal discovery until
it is reviewed again.

### 6. Configure delivery

Configure applicable channels independently: Private MCP, optional Public MCP,
widget, agent setup, and support/reporting. Public exposure requires explicit
acknowledgement. Widget activation requires exact origin bindings and a verified
embed test. Support delivery requires a tested backend connection when enabled.

### 7. Verify

Run the reusable MCP acceptance client against the exact candidate manifest. At
minimum it verifies protocol negotiation, OAuth resource metadata, registration
and PKCE where applicable, resources list/read, grant-filtered tool discovery,
successful calls, missing-grant denial, confirmation behavior, revoked access,
upstream proxy behavior, and request/audit correlation.

Conditional checks apply when their capability is configured:

- private access: identity and access-evaluation allow/deny/expiry/revocation;
- mutating tools: idempotency and explicit confirmation on the exact call;
- upstream MCP: authorization, schema pin, drift, timeout, and error handling;
- native tools: source and dependency review, conformance and source checks,
  required configuration, identity/no-identity cases, state isolation and
  upgrade failure, exact source pins, idempotency, timeout, panic, malformed
  output, and plugin disable behavior;
- packages: server preflight verifies that every selected package resolves to
  one exact immutable release on an available active artifact; registry-byte,
  declared-digest, provenance or SBOM, and installation verification is a
  separate operator-controlled process that DokoSoko does not evidence or
  enforce;
- widget: allowed-origin, denied-origin, isolation, knowledge, and tool tests;
- support: consent preview, delivery receipt, idempotency, and retry behavior.

### 8. Publish and operate

Publication stores one immutable snapshot containing resolved resource,
tool, package, authorization, access-connection, and support-route identifiers
and content hashes where those catalogues provide them. Live delivery and
acceptance checks remain separately observable operational evidence. An error
cannot stand in for a passing required check; warnings identify optional or
incomplete stages for operator review. After publication, monitor drift, policy
decisions, proxy failures, credentials, usage, rollout, deprecation, and
replacement.

## Authorization invariants

- `grants` is the only authorization vocabulary shared by identity, access,
  tools, storage, and APIs. Package delivery authorization remains the external
  registry's responsibility and is not encoded in package metadata.
- The vendor access-evaluation origin and path are fixed configuration; an
  individual authorization point cannot supply a destination URL.
- DokoSoko access tokens are never forwarded to vendor services.
- Tool and vendor calls use fixed configured origins and fail closed.
- A policy simulator explains configuration but never issues a real allow
  decision or token.
- Consequential tools declare confirmation in discovery metadata and fail closed
  when the exact MCP call does not carry explicit confirmation. Clients must
  preview the action and arguments before setting that marker.

## Package invariants

- Package artifacts are optional and do not affect unrelated API or tool use.
- A package artifact has a canonical ecosystem coordinate and an unversioned,
  query-free, fragment-free Package URL whose type matches the ecosystem. Its
  required registry location and optional source location must be HTTPS, except
  for loopback HTTP during local development, and cannot contain userinfo, a
  query, or a fragment.
- A published package release is immutable and contains an exact version and
  query-free, fragment-free Package URL with the same artifact identity and a
  decoded version equal to the release version. It also contains a declared
  SHA-256, SHA-384, or SHA-512 digest and display-only install command.
  Provenance and SBOM locations are optional and use the same URL policy.
- DokoSoko validates metadata shape, PURL identity, digest syntax, strict URLs,
  and obvious credential-bearing install-command forms, then hashes the metadata
  deterministically. Other free-text fields are not comprehensive secret scans.
  DokoSoko does not assert that registry bytes match the digest, that provenance
  or an SBOM is authentic, or that the install command works.
- A separately operated external verifier should fetch the registry release and
  verify bytes, digest, any declared provenance or SBOM, and installation before
  operational use. DokoSoko neither records verifier evidence nor blocks a
  package operation when it is absent. Verifier credentials and evidence must
  remain outside package metadata.
- Published Integration bindings always resolve one exact package release and
  embed its metadata in the immutable Integration manifest. There is no
  follow-latest package binding. Selecting packages is optional, but every
  selected artifact must be active and available for a new candidate to pass
  preflight and publication.
- The external registry delivers bytes and enforces access. Credentials and
  package bytes must not be entered in the DokoSoko package catalogue, and
  DokoSoko never acts as a package download or authentication proxy.
- All package-artifact catalogue fields are editable only in `draft`. Publishing
  the first immutable release moves the artifact to `active`. Creating or
  transitioning a public artifact, and publishing each public release, require
  explicit acknowledgement; a public Integration can bind only public releases.
- Deprecation may be applied to any non-retired artifact and requires operator
  guidance plus the exact current revision. An optional replacement must be
  active and already have a published release. An optional sunset must be in the
  future, but is guidance only: deprecation makes the artifact unavailable
  immediately for new releases, new bindings, and candidate publication.
- Retirement may be applied to any non-retired artifact, is immediate, and also
  requires guidance and the exact current revision. Its optional replacement
  follows the same active-and-published rule. It preserves any existing sunset.
  Existing bindings remain readable and historical published Integration
  manifests remain immutable; a future candidate must remove the unavailable
  package or bind an available replacement explicitly.
