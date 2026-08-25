# DokoSoko service guidance

## Scope and authoritative sources

- This repository contains the Go service (`cmd/dokosoko` and `internal`), the vinext/React console (`app`), the isolated TypeScript crawler (`crawler`), PostgreSQL migrations, and standalone examples. The examples are separate Go modules, so root Go commands do not test them.
- Treat the OpenAPI files in `api/` as the machine-readable source of truth for public contracts. `docs/FINAL_PLAN.md` is the normative design guidance for invariants and failure semantics. Keep public paths, schemas, implementation, and contract tests aligned; breaking public changes require a versioned path or explicit API version.
- Use `README.md` for supported setup, runtime, deployment, and verification flows. Use `docs/INTEGRATION_SETUP.md` for the integration lifecycle and publication model.

## Toolchain and commands

Run these from the repository root unless another working directory is shown.

- Use the versions declared by `package.json`, `go.mod`, and CI: Node.js 22.13 or newer, pnpm 11.19.0, and Go 1.25.
- Install JavaScript dependencies with `pnpm install --frozen-lockfile`.
- Use the declared package scripts instead of invoking vinext, Vite, ESLint, TypeScript, or the crawler test runner directly.
- `pnpm run verify` is the primary repository verification. It runs type checking, lint, the production console build, console tests, crawler tests, and the root Go suite. `pnpm run test:all` is only an alias.
- PostgreSQL integration tests run only when a test database URL documented in `README.md` is present. For store, migration, transaction, locking, or PostgreSQL-specific changes, run `go test ./...` against a disposable PostgreSQL 17 database with pgvector; a passing memory-only run is not sufficient.
- `docker compose config` validates Compose shape. Container changes also require both image builds used by `.github/workflows/ci.yml`.
- For `examples/go-backend-integration`, run `go test ./...` (or `make test`) from that directory. For `examples/mcp-acceptance-client`, run `go test ./...` from that directory. These suites are outside the root module.

## Change constraints

- Migrations are append-only deployment history. Never edit, rename, reorder, or delete an existing `migrations/*.sql` file. Add a uniquely numbered migration and update `migrations/checksums.sha256` in the same change. The duplicate historical `0020` sequence is frozen; do not reuse it. See `migrations/README.md`.
- Preserve the fail-closed trust boundaries in `docs/FINAL_PLAN.md`: DokoSoko tokens are not forwarded, upstream destinations are fixed rather than request-selected, private or ambiguous network targets are rejected outside explicit local-development exceptions, and identity, authorization, confirmation, schema, publication, and drift failures deny access. Add focused regression tests when changing these boundaries.
- Keep the crawler isolated and credential-free. Do not bypass the URL, DNS-rebinding, same-origin, redirect, byte/page budget, upload containment, or renderer-network controls in `crawler/security.ts`, `crawler/sources.ts`, and `crawler/index.ts`.
- Published releases, recipes, manifests, and other revisioned publication inputs are immutable by contract. Changes must create or select a new revision rather than mutating historical data.
- Console routes use the owned UI system. Follow `app/components/core/README.md` and `docs/ui-system-audit.md`: compose routes with the shared layout/control components, use semantic tokens from `app/globals.css`, and preserve keyboard, narrow-layout, light/dark-theme, and reduced-motion behavior. Do not introduce route-specific width systems, raw feature colors, or replacement primitives without updating the UI contract and tests.
- Do not commit ignored build or runtime output such as `dist/`, `.vinext/`, `.next/`, `.wrangler/`, `storage/`, `outputs/`, or `work/`.

## Validation by change area

- Go service or domain logic: run focused package tests while iterating, then `go test ./...`; include the PostgreSQL-backed run when persistence is involved.
- Console or shared TypeScript: run the relevant test in `tests/`, plus `pnpm run typecheck`, `pnpm run lint`, and `pnpm run build`.
- Crawler: run `pnpm run test:crawler`; security-boundary changes also need focused cases for rejected and explicitly allowed local targets.
- API or protocol contract: update the applicable `api/*.yaml`, implementation, and contract tests together, then run the root Go suite and `pnpm run test:console`.
- Migration or container change: run the database-backed Go suite and the corresponding Compose/container checks from CI.
- Before handoff, run `pnpm run verify` unless the change is limited to documentation or repository guidance. Report any skipped environment-dependent checks explicitly.
