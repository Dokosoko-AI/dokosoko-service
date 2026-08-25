# DokoSoko integration contract

This document records the intended long-lived contract. It is normative design guidance; the OpenAPI documents are the machine-readable source of truth.

## 1. Resource model

One DokoSoko installation is one vendor deployment. Its integration surface consists of:

- `integration` — one API family and version exposed by the deployment;
- `resource_set` — independently revisioned documentation or API content shared by integrations;
- optional singleton `identity_provider` — delegated customer OIDC and API configuration for private access;
- `backend_connection` — service-to-service origin, authentication state, and active credential version;
- `customer_account` — DokoSoko's durable account resource resolved from a trusted external identity;
- `installation` — a registered customer/environment integration instance;
- `tool` — a published operation with fixed destination, schemas, and authorization policy;
- optional `package_artifact` and immutable `package_release` records —
  bounded metadata for externally delivered developer artifacts;
- `integration_package_binding` — compatibility between one Integration draft
  and one exact immutable package release;
- `support_submission` — one consented, encrypted bug report or feedback item;
- `support_route` — reporting policy that references, but never owns, a backend connection;
- optional `ai_provider_connection` and four `ai_workload_profile` records for extraction, authoring, review, and support;
- `integration_analysis` — a versioned evidence manifest, endpoint and identity plan, and explicit unknowns;
- `recipe` and immutable `recipe_revision` — reviewed Markdown implementation guidance published as an MCP resource;
- optional `access_definition`, `access_connection`, `access_instance`, and `access_credential` resources for provider-owned lifecycle management.

Package metadata is an optional DokoSoko catalogue resource; package delivery is not. An HTTP capability still belongs in an API or tool contract. A generated SDK or other package may be identified by an exact metadata release in an Integration manifest, but it is never required to expose an endpoint and does not shape runtime authorization.

## 2. Optional identity and customer accounts

Private customer access optionally configures:

- an exact HTTPS OIDC issuer;
- a client ID and encrypted client secret;
- OIDC scopes and audience;
- an optional OAuth resource indicator independent of the API origin;
- the claim containing the vendor's external customer ID;
- an optional installation claim;
- one credential-free delegated API origin;
- an explicit active or disabled state.

The pair `(issuer, external_customer_id)` resolves to one durable `customer_account` within a deployment. This resolution is just-in-time after a verified ID token. Administrative APIs may list, suspend, and reactivate accounts but cannot manufacture identity-backed accounts.

Internal `customer_account.id` values are used for DokoSoko relationships. External IDs are preserved for vendor context and never accepted as internal foreign keys. Installations must belong to the resolved customer account. An unknown or mismatched authenticated installation fails closed.

Suspension is checked whenever a DokoSoko access token is authenticated. It therefore affects already-issued tokens without waiting for expiry.

Identity is not required for public discovery or backend delivery. Disabling the provider immediately fails private authorization without disabling those independent surfaces.

## 3. OAuth contract

DokoSoko is the authorization server for MCP clients and federates authentication to vendor OIDC.

Required properties:

1. RFC 8414 authorization-server metadata at `/.well-known/oauth-authorization-server`.
2. RFC 9728 protected-resource metadata at `/.well-known/oauth-protected-resource/mcp`.
3. One canonical resource: the exact `/mcp` URL.
4. Authorization code flow with PKCE S256 only.
5. Mandatory RFC 8707 `resource` on authorization and token requests.
6. HTTPS client metadata documents as client IDs; redirect URIs must match exactly.
7. One-time, short-lived authorization state and code records stored by digest.
8. DokoSoko access tokens bound to client, resource, deployment, subject, customer account, scopes, and grant evaluation.
9. The DokoSoko token is never forwarded to a vendor or upstream MCP server.

The upstream vendor access token is stored encrypted and expires no later than the earliest upstream token or access-evaluation boundary.

## 4. Vendor-owned operations

The vendor may configure two independent HTTPS origins: the delegated API origin under the identity provider and a service origin on each backend connection. Neither is a collection of hook URLs. DokoSoko owns the paths and request schemas.

Identity and backend delivery have separate OpenAPI documents. Either may generate an independent server stub without creating a shared base URL, credential, deployment, or lifecycle.

### Access evaluation

`POST /v1/access/evaluations` is called once during authorization with the live delegated vendor access token. Its body is an empty object. The vendor derives the client, authenticated subject, and external customer from the token; DokoSoko does not send a second ambiguous integration identifier.

The response contains:

- a stable evaluation ID;
- a bounded, unique list of `grants`;
- an absolute expiry;
- an optional policy version.

DokoSoko defines tools and `required_grants`. Access evaluation narrows access; it does not remotely define DokoSoko resources. Network errors, non-success HTTP statuses, malformed data, missing identity, expired evaluations, and unavailable dependencies deny authorization.

The `Idempotency-Key` is stable for retries of one evaluation and unique for a later evaluation. Each transport attempt has a new provider-neutral `X-External-Request-ID`.

### Support submission

`POST /v1/support-submissions` receives both bug reports and feedback through a referenced backend connection. Backend credentials rotate independently of support routes and are never reused for delegated customer operations.

DokoSoko accepts a report only after explicit user confirmation, schema and size validation, and secret screening. It stores the payload encrypted and returns a local receipt before vendor delivery. A durable worker provides at-least-once delivery.

Retries preserve `Idempotency-Key` and change `X-External-Request-ID`. Network failures, 408, 429, and 5xx are retryable. Other 4xx statuses are permanent. A vendor returns `409` if a key is reused with a different payload and retains the original result for at least 24 hours.

Usage is an ordinary API operation or tool if required. It has no separate integration mechanism.

## 5. Tools and delegated execution

A tool has:

- a stable namespace and name;
- bounded JSON input and output schemas;
- one fixed HTTPS destination;
- an HTTP method or managed MCP operation;
- `required_grants`;
- an explicit confirmation policy;
- a timeout and immutable published revision.

For vendor HTTP tools, the destination must share the delegated API origin and execution uses the authenticated user's delegated vendor token. For managed MCP tools, DokoSoko uses either a separately encrypted service credential or a delegated upstream OAuth grant bound to the DokoSoko subject.

Before execution DokoSoko checks publication state, schema, confirmation, grants, customer and installation state, destination policy, and upstream schema drift. Any ambiguity fails closed. Request-time destination overrides are forbidden.

## 6. Access Provider API

The optional Access Provider API is reserved for a genuine provider-owned resource lifecycle:

- authorize one mutation;
- create an instance when cardinality is `many`;
- issue a connection- or instance-scoped credential;
- revoke a credential idempotently.

It must not become a generic action bag. Operations that do not fit this lifecycle are ordinary tools.

Every mutation is authorized independently. Create and issue operations require idempotency keys. One-time credentials are returned once; replay returns metadata, never plaintext. DokoSoko persists only encrypted material or fingerprints according to the declared storage mode.

## 7. Package metadata and external verification

A `package_artifact` is the stable metadata identity for one ecosystem and canonical coordinate. It contains a canonical unversioned Package URL whose type matches the ecosystem, a registry URL, an optional source URL, visibility, and lifecycle. Package-specific URLs must use HTTPS, except for loopback HTTP during local development, and cannot contain userinfo, a query, or a fragment. This rejects those common credential and signature channels, but URL paths and other free-text fields are not comprehensive secret scans, so operators must not enter credentials, signatures, package bytes, or executable payloads into them.

A `package_release` is immutable metadata for one exact externally hosted version. It includes a query-free and fragment-free versioned Package URL with the exact artifact identity and a decoded version matching the release version, an operator-supplied display-only install command, a declared SHA-256, SHA-384, or SHA-512 digest, and optional provenance and SBOM URLs using the same strict URL policy. DokoSoko validates field shape, PURL identity, URL policy, obvious credential-bearing install-command forms, exact-version consistency, and digest syntax, then computes a deterministic hash of the metadata. That hash proves metadata identity only; it does not prove anything about external bytes or certify free-text as safe.

An Integration package binding names one exact release. Publishing the Integration embeds the resolved release metadata, lifecycle guidance, and metadata content hash in its immutable manifest. Package bindings never follow latest. A public Integration may embed only public package metadata. Creating a public artifact, changing a private draft to public, and publishing each public release require explicit public acknowledgement.

The external registry delivers bytes and enforces registry authentication and authorization. DokoSoko does not fetch, host, execute, sign, cryptographically verify, or proxy a package, provenance statement, SBOM, or installation. Before operational use, an operator should run a separately operated external verifier to check the registry bytes against the declared digest, validate any declared provenance or SBOM, and exercise the documented installation procedure. DokoSoko neither records this evidence nor enforces that verification occurred. Verifier credentials and results are separate operational evidence and must not be copied into package metadata.

All artifact catalogue fields are editable only in `draft`; the first immutable release activates the artifact, and later releases remain independently immutable. Deprecation can be applied to any non-retired artifact and requires guidance plus the exact current revision. Its optional replacement must be active with a published release. Its optional future sunset is guidance only: the deprecated artifact becomes immediately unavailable for releases, new bindings, and candidate publication. Retirement can likewise be applied to any non-retired artifact, is immediate, requires guidance and the exact revision, and applies the same replacement rule while preserving any existing sunset. Existing bindings remain readable. Neither transition rewrites historical published Integration manifests; a future candidate must remove the unavailable package or bind an available replacement explicitly.

## 8. HTTP and evolution rules

- Resource creation uses `POST` and returns `201`; accepted asynchronous work returns `202`.
- Retrieval uses `GET`; partial state change uses `PATCH`; replacement configuration uses `PUT`; deletion uses `DELETE` and returns `204` when no representation is needed.
- Mutable administrative resources use revisions for optimistic concurrency and return `409` on stale writes.
- Errors use one structured envelope with stable machine codes, safe messages, and request IDs.
- List responses use `{ "items": [...] }`; cursor pagination must be added before an unbounded collection can grow materially.
- Servers ignore unknown additive response fields from vendor APIs. Vendors must tolerate additive request headers but may reject unknown body fields according to the published schema.
- Breaking changes require a new versioned path or explicit API version. Implementation details and database identifiers are not promoted into the public contract accidentally.

## 9. AI analysis and recipes

DokoSoko uses a small provider-neutral interface implemented directly with the official OpenAI, Google, and Anthropic SDKs plus one OpenAI-compatible adapter. There is no agent framework, model-owned tool loop, hidden cross-provider fallback, or second workflow engine. The boundary is intentionally replaceable if a framework later solves a measured problem.

One encrypted credential belongs to one provider connection. Workload profiles reference that connection and select a visible model ID, input and output limits, daily token budget, and enabled state. Extraction may use an efficient model, authoring and support a balanced model, and review a strongest model. These are editable presets, not automatic routing. Every attempt records normalized outcome, provider, requested and resolved model, tokens, latency, error code, and prompt version.

Analysis starts from resources DokoSoko already knows. The result contains the evidence manifest and fingerprints, identity boundary, proposed endpoints, recipe proposals, and questions for anything not justified by evidence. Blocking questions prevent generation. Only bounded API manifests, tool schemas and authorization policy, and excerpts from already-published knowledge may be sent to the configured extraction provider: at most three documents and 6,000 characters per source, with a 32,000-character total cap across all evidence. Retrieved content is untrusted data, never instructions, and there is no arbitrary URL fetch path.

A recipe is Markdown with structured references and dependencies. Model output is visibly generated and always enters review. Direct human edits and AI rework both create immutable revisions and run the same deterministic checks. Model review is advisory; only a human can approve the current revision, and only an approved current revision can be published.

References may select only analysed HTTPS sources or exact canonical documentation and sample-code pages included in their bounded evidence. Markdown URLs outside that allowlist are blocking findings. Public recipes may reference only public, published, non-quarantined sources and pages. Published recipes use stable `dokosoko://products/{product}/recipes/{recipe}` URIs and appear through MCP `resources/list` and `resources/read`. Exact evidence-fingerprint drift marks dependent recipes outdated and removes them from MCP until reviewed again.

The in-product attention inbox contains unanswered questions and drifted recipes. Recipe views and explicit plan selections are distinct analytics events; popularity does not imply correctness or approval.

## 10. Acceptance invariants

The integration is acceptable only while all of these remain true:

1. The OpenAPI and implemented paths agree exactly.
2. There is one canonical Private MCP resource and no legacy ID-bearing alias.
3. A trusted external customer resolves to a durable internal account.
4. Suspended or mismatched accounts and installations fail closed.
5. Access evaluation is resource-bound, short-lived, idempotent, and fail-closed.
6. `grants` is the only authorization vocabulary across identity, tools, provider access, storage, and APIs.
7. DokoSoko tokens are never forwarded.
8. Vendor calls use fixed origins and fixed versioned paths.
9. Support delivery is consented, encrypted, durable, idempotent, and retry-safe.
10. Optional package metadata binds an exact immutable release into an Integration manifest; no package gateway, byte store, download proxy, entitlement hook, usage hook, or arbitrary hook URL exists.
11. Package URL fields reject userinfo, queries, and fragments, obvious credential-bearing install commands are rejected, and operators keep credentials out of URL paths and all other package metadata. Registries deliver bytes; external verification of bytes, digest, optional provenance or SBOM, and installation is operator-controlled and is neither performed nor evidenced by DokoSoko.
12. Package deprecation and retirement immediately block releases, new bindings, and candidate publication without mutating readable existing bindings or historical Integration manifests; replacements are explicit in later candidates.
13. Identity, backend connectivity, package metadata, and public visibility are independent axes.
14. Integrations default private; public transition requires acknowledgement and the global Public MCP switch remains a master kill switch.
15. Support routes contain backend-connection references, never plaintext credentials or copied secret identifiers.
16. AI provider credentials exist once per provider connection and are never returned or copied into workload profiles.
17. Model output cannot authorize, publish, add an arbitrary reference, or silently select another provider.
18. Every published recipe has an explicitly approved immutable revision and is removed from discovery when its evidence drifts.
