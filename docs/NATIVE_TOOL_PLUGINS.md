# Native tool plugins

Native tool plugins are reviewed Go source packages compiled into a specific
DokoSoko service build. They let a self-hosting operator add implementation
logic that is awkward or inefficient to express as an HTTP or upstream MCP
tool while preserving DokoSoko's existing publication and authorization model.

They are trusted application code, not sandboxed extensions. DokoSoko does not
load uploaded binaries, `.so` files, WASM modules, or source at runtime. Adding
or changing a plugin requires source review, a new service build, and a normal
deployment.

## Author and operator workflow

1. Add a Go source package that imports `nativeplugin` and implements
   `Plugin.Describe` and `Plugin.Open`.
2. Add a conformance test with `plugintest.TestPlugin(t, New())`.
3. Review the package and its complete dependency tree, then run the strict
   source check.
4. Import the reviewed package in `internal/nativeplugins/registry.go` and add
   its constructor to `Registered`. Registration is explicit; `init`-based
   discovery is forbidden.
5. Set only the environment keys declared by the manifest, build DokoSoko from
   source, and start the service.
6. Inspect **Tools → Native tool plugins**. The console shows lifecycle status,
   source version, declared capabilities, environment key names, and whether
   each key is configured. It never returns configuration values.
7. Review each source-managed draft Tool, bind its exact revision to an
   Integration, run the normal checks, and publish it. Native tools are exposed
   only through Private MCP.

The runnable [`examples/native-tool-plugin`](../examples/native-tool-plugin/README.md)
contains an optional-identity echo tool and an idempotent, customer-scoped
counter.

```sh
go test ./examples/native-tool-plugin
go run ./cmd/dokosoko-native-plugin-check ./examples/native-tool-plugin
```

Changing a manifest version or tool contract does not mutate a published Tool.
At catalog synchronization, DokoSoko stages a new draft revision for review.
Runtime execution requires an exact match for the plugin ID, plugin version,
SDK version, manifest hash, and individual tool-contract hash. A stale release
fails closed until the source-backed draft is reviewed and published.

## Source contract and lifecycle

`Describe` is deterministic and side-effect free. Its manifest declares:

- one canonical lower-case plugin ID and semantic version;
- SDK version `1` and a bounded description;
- registered configuration keys and their types;
- one state schema version;
- optional managed-network claims and the `network` capability;
- one or more tool contracts.

Every tool declares closed JSON input and output object schemas, effect, identity
requirement, state scope, grants, confirmation policy, idempotency behavior,
timeout, maximum concurrency, and maximum result size. Write and destructive
tools require idempotency; destructive tools also require confirmation.

The host calls `Open` once after configuration validation and state upgrades,
then calls `Invoke` for an admitted operation, and calls `Close` when disabling
the plugin or stopping the process. Panics are recovered at every lifecycle
boundary. An `Invoke` context carries the declared deadline, but Go cannot
preempt code that ignores cancellation; a stuck plugin can therefore consume a
process goroutine. Keep work bounded and observe `ctx.Done()`.

## Configuration

A plugin may read only manifest-declared values through `Host.Config`. A key
`KEY` owned by plugin `my_plugin` maps to:

```text
DOKOSOKO_PLUGIN_MY_PLUGIN_KEY
```

Plugin IDs and keys are canonical, so two plugins cannot claim the same config
space. Plugins cannot claim core DokoSoko variables or another plugin's space.
Supported types are string, secret, Boolean, integer, duration, and a fixed
credential-free HTTPS URL. Missing or malformed required values leave the
plugin `misconfigured` and all its tools unavailable.

Secret values are wrapped in a redacting type, omitted from API and UI
responses, and scrubbed from host-managed logs even if embedded in a message or
non-secret-labelled field. A plugin must still never place a secret in tool
output, state, a public error message, or an outbound URL.

This is an API and ecosystem boundary, not a same-process security boundary.
Trusted Go code can read process environment or files if it deliberately
bypasses review. The source checker rejects direct use of `os`, `net`,
`net/http`, database drivers, process execution, `unsafe`, cgo, assembler,
compiled or non-human-readable artifacts, symlinks, DokoSoko internal packages,
direct loggers, `go:linkname`, and `init`. A checked tree is capped at 512 files
and 8 MiB, with 1 MiB per file. The checker does not prove the behavior of
transitive dependencies or source outside the checked directory. Operators
must review and pin the entire dependency graph.

## Identity

Identity is invocation-scoped and passed as an `IdentityView`; plugins never
receive a DokoSoko access token, upstream vendor token, raw OIDC subject, email,
or internal database ID. References contain plugin-specific HMAC-derived opaque
IDs. The same principal is unlinkable between different plugins.

| Requirement | Invocation behavior |
| --- | --- |
| `none` | Always receives an empty identity view. |
| `optional` | Receives available actor, customer, and installation references; anonymous calls remain valid. |
| `actor_required` | Requires and exposes only the actor reference. |
| `customer_required` | Requires and exposes only the customer reference. |
| `actor_and_customer_required` | Requires and exposes actor and customer references. |
| `installation_required` | Requires and exposes only the installation reference. |

An identity requirement is not authorization. DokoSoko still evaluates
publication, account and installation state, required grants, schemas,
confirmation, and idempotency before invoking the plugin. Actor-, customer-, or
installation-scoped state is legal only when the matching identity is required;
an optional-identity tool cannot accidentally share anonymous state under an
empty identity.

## State

Plugins do not receive a database connector or SQL access. The host exposes a
small JSON key/value abstraction backed by the single `native_plugin_state`
table. The table is partitioned logically by plugin ID, declared scope kind,
and an opaque scope ID.

| Scope | Isolation |
| --- | --- |
| `none` | Writes are rejected. |
| `plugin` | Shared only by tools in one plugin. |
| `actor` | Isolated to one opaque actor within one plugin. |
| `customer` | Isolated to one opaque customer within one plugin. |
| `installation` | Isolated to one opaque installation within one plugin. |

The API provides `Get`, revision-checked `Put` and `Delete`, `List`, compare and
swap, TTL, and transactions. Keys are at most 128 bytes, values are valid JSON
at most 64 KiB, lists return at most 100 items, and one scope holds at most
1,000 live records. Reserved host metadata is inaccessible to ordinary plugin
calls.

Increasing `StateVersion` runs each `StateUpgrader` step transactionally before
`Open`. The upgrader can enumerate anonymous scopes but never learns their
identity IDs. A failed upgrade rolls back and leaves the plugin unavailable.
Downgrading below stored state or skipping a required upgrade step fails closed.

## Managed network access

A plugin with outbound HTTP needs both the `network` capability and a manifest
claim for each fixed host or registered URL config key. It uses
`Host.HTTP().Do(ctx, nativeplugin.HTTPRequest{...})`; direct networking imports
are rejected.

The host allows GET, HEAD, POST, PUT, PATCH, and DELETE over HTTPS port 443 only,
uses no environment proxy, resolves only public addresses, rechecks every dial,
restricts redirects to three declared destinations, strips hop-by-hop and
host-controlled headers, caps response headers at 64 KiB, and caps request and
response bodies at 1 MiB. Private, loopback, link-local, multicast, unspecified,
IP-literal, undeclared, and DNS-rebinding destinations fail closed.

## Errors, limits, and observability

Use `nativeplugin.Fail` for a bounded MCP-safe error code and message. DokoSoko
accepts only registered codes and printable messages up to 300 bytes, strips
the internal cause, and converts ordinary errors, malformed errors, and panics
to a generic internal failure. Structured results must satisfy the declared
output schema and byte limit.

Per-tool semaphores enforce declared concurrency and invocation deadlines cap
cooperative work. Existing MCP authorization, confirmation, idempotency,
request correlation, and audit records apply to native tools. Administrative
enable/disable changes are audited. The status API is:

```text
GET   /api/v1/native-plugins
PATCH /api/v1/native-plugins/{plugin_id}/state
```

Set `DOKOSOKO_NATIVE_PLUGINS_REQUIRED` to a comma-separated list that must be
registered and active for startup to succeed. Set
`DOKOSOKO_NATIVE_PLUGINS_DISABLED` for a deployment-owned kill switch. A
required plugin cannot also be environment-disabled or disabled in the UI.

## Trust boundary and v1 exclusions

Native means in-process performance and access to a narrow host SDK; it does
not mean untrusted native code. One malicious plugin can still compromise the
service process. Run community code only after source and dependency review,
and rebuild from a reproducible, pinned tree.

Version 1 intentionally excludes runtime package installation, binary uploads,
Go's dynamic plugin loader, WASM/WASI, arbitrary filesystem or process access,
raw environment access, SQL handles, custom tables, arbitrary sockets, public
MCP exposure, background daemons, event subscriptions, and plugin-defined UI.
Those features require a different isolation and lifecycle design rather than
an additive SDK method.
