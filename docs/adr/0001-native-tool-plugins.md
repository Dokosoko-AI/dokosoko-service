# ADR 0001: Trusted source-compiled native tool plugins

- Status: accepted
- Date: 2026-08-26

## Context

Some self-hosting operators need tool behavior that is substantially easier to
implement in Go than through a separately deployed HTTP service or upstream MCP
server. Runtime-loaded Go binaries are platform-fragile and grant full process
authority. WASI would improve isolation, but it would also introduce a second
ABI, toolchain, runtime, debugging model, and capability bridge before there is
evidence that untrusted plugins are required.

Configuration and persistence must be predictable across plugins without
letting every package invent environment names, schemas, migrations, database
connections, or identity conventions.

## Decision

DokoSoko v1 supports trusted Go source packages compiled statically into the
service. Packages implement the public `nativeplugin` SDK and are composed in
one explicit registry. There is no runtime installation or dynamic loading.

The host provides registered namespaced configuration, opaque invocation
identity, one scoped JSON state abstraction backed by one table, managed HTTPS,
structured logging, a clock, typed safe errors, and declared resource limits.
Plugins never receive raw tokens, environment access, database handles, or
internal model objects through the SDK.

Native tools remain ordinary DokoSoko Tool revisions. They pass through central
grant authorization, identity checks, confirmation, idempotency, schema
validation, human review, immutable publication, and audit. Published revisions
pin source and contract hashes and execute through Private MCP only.

A strict source checker rejects common privilege escapes and non-source
artifacts, but is explicitly a review aid rather than a sandbox. Operators own
the trust decision for all source and transitive dependencies.

## Consequences

- Integration is simple for a trusted fork: add readable source, register it,
  test it, rebuild, and review the generated drafts.
- Plugins share the service's address space, availability, and blast radius.
- A context deadline cannot preempt non-cooperative Go code.
- One config namespace and one state table keep operations and backup behavior
  uniform, but intentionally prevent plugin-specific SQL optimization.
- Source or manifest changes require deployment and a fresh Tool review.
- WASI remains a possible future execution backend if untrusted distribution,
  stronger tenant isolation, or independently shipped artifacts become proven
  requirements. It should not reuse the word “native” or weaken v1's trust
  statement.
