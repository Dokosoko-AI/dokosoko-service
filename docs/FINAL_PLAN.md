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
- `support_submission` — one consented, encrypted bug report or feedback item;
- `support_route` — reporting policy that references, but never owns, a backend connection;
- optional `access_definition`, `access_connection`, `access_instance`, and `access_credential` resources for provider-owned lifecycle management.

Package ecosystems are not a DokoSoko resource. An HTTP capability belongs in an API or tool contract. Generated SDKs may be published separately, but they are not needed to expose an endpoint and do not shape runtime authorization.

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

The `Idempotency-Key` is stable for retries of one evaluation and unique for a later evaluation. Each transport attempt has a new `X-DokoSoko-Request-ID`.

### Support submission

`POST /v1/support-submissions` receives both bug reports and feedback through a referenced backend connection. Backend credentials rotate independently of support routes and are never reused for delegated customer operations.

DokoSoko accepts a report only after explicit user confirmation, schema and size validation, and secret screening. It stores the payload encrypted and returns a local receipt before vendor delivery. A durable worker provides at-least-once delivery.

Retries preserve `Idempotency-Key` and change `X-DokoSoko-Request-ID`. Network failures, 408, 429, and 5xx are retryable. Other 4xx statuses are permanent. A vendor returns `409` if a key is reused with a different payload and retains the original result for at least 24 hours.

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

## 7. HTTP and evolution rules

- Resource creation uses `POST` and returns `201`; accepted asynchronous work returns `202`.
- Retrieval uses `GET`; partial state change uses `PATCH`; replacement configuration uses `PUT`; deletion uses `DELETE` and returns `204` when no representation is needed.
- Mutable administrative resources use revisions for optimistic concurrency and return `409` on stale writes.
- Errors use one structured envelope with stable machine codes, safe messages, and request IDs.
- List responses use `{ "items": [...] }`; cursor pagination must be added before an unbounded collection can grow materially.
- Servers ignore unknown additive response fields from vendor APIs. Vendors must tolerate additive request headers but may reject unknown body fields according to the published schema.
- Breaking changes require a new versioned path or explicit API version. Implementation details and database identifiers are not promoted into the public contract accidentally.

## 8. Acceptance invariants

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
10. No package gateway, package resource, entitlement hook, usage hook, or arbitrary hook URL remains in the product contract.
11. Identity, backend connectivity, and public visibility are independent axes.
12. Integrations default private; public transition requires acknowledgement and the global Public MCP switch remains a master kill switch.
13. Support routes contain backend-connection references, never plaintext credentials or copied secret identifiers.
