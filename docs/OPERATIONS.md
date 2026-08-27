# Operations and launch runbook

This runbook is the minimum production contract for one self-hosted DokoSoko
deployment. Rehearse it against a production-shaped staging environment before
the first customer is invited.

## Production boundary

- Terminate TLS at a trusted reverse proxy and set `DOKOSOKO_PUBLIC_URL` to the
  exact external HTTPS origin. Forward the original scheme and host only from
  that trusted proxy.
- Use PostgreSQL 17 with pgvector on durable storage. Do not use
  `DOKOSOKO_DEV_MEMORY` or `DOKOSOKO_ALLOW_DEMO_TOKENS` in production.
- Generate independent high-entropy database, setup, and 32-byte master-key
  secrets. Store the master key in the deployment secret manager and in the
  protected disaster-recovery escrow. Losing it makes encrypted credentials
  unrecoverable.
- Keep `/uploads` and PostgreSQL private. Only the service writes uploads; only
  the crawler reads them. Neither volume belongs behind a web server.
- Restrict outbound traffic to approved API, identity, AI, crawler, upstream
  MCP, and root-configured support destinations. Public destinations use HTTPS
  on port 443. Local HTTP is a development-only exception.
- Configure root-level feedback and error submission URLs before advertising
  the corresponding MCP tools. Empty URLs intentionally disable those tools.

## Release procedure

1. Build immutable service and crawler images from one reviewed commit. Record
   the commit, image digests, and migration checksum manifest.
2. Run `pnpm run verify`, `go test -race ./...`, `go vet ./...`, production
   dependency audit, and `govulncheck ./...`.
3. Back up PostgreSQL, uploads, and the exact master key. Verify the database
   dump is readable before proceeding.
4. Deploy to staging. Startup applies append-only migrations and rejects a
   checksum mismatch. Do not edit an applied migration.
5. Require `/healthz` and `/readyz` to return 200. Verify anonymous access is
   rejected by Private MCP and that Public MCP is absent or limited to intended
   published resources.
6. Run the standalone client in `examples/mcp-acceptance-client` against both
   enabled MCP surfaces. Exercise OAuth/PKCE, grant filtering, confirmation,
   resource reads, and at least one safe tool call applicable to the release.
7. Submit one staging feedback report and one staging error report. Confirm
   each moves from `queued` to `delivered` and arrives once with the submission
   ID as its idempotency key.
8. Deploy production, repeat the health and read-only acceptance checks, then
   watch the release indicators below through one normal traffic window.

Application rollback is safe only while the older binary understands the
current schema. Database migrations are forward-only. If a release introduces
an incompatible schema, roll forward with a corrective migration or restore the
pre-release backup into a new environment; do not downgrade the live database.

## Backup and restore

Back up at least daily and before every release:

- a PostgreSQL custom-format dump, including migration state;
- a snapshot of the upload volume;
- deployment configuration and the exact master key from secret escrow;
- deployed image digests and the source commit.

Retain encrypted backups in a separate failure domain. Test a restore at least
quarterly:

1. Create an empty isolated PostgreSQL instance and upload volume.
2. Restore the database dump and upload snapshot without exposing the service.
3. Supply the escrowed master key and the restored release configuration.
4. Start the service and crawler at the recorded image digests.
5. Require readiness, root login with MFA, credential decryption, one reviewed
   document query, one safe private tool call, and the MCP acceptance suite to
   pass.
6. Destroy the drill environment and record duration, failures, and follow-up
   work. A backup is not considered valid until this drill succeeds.

## Monitoring and alerts

Collect structured container logs and alert on:

- `/readyz` failures, restart loops, panics, sustained 5xx responses, and rising
  request latency;
- PostgreSQL connection exhaustion, storage growth, slow queries, replication
  or backup failures, and less than 20% free disk;
- crawler jobs that remain queued/running beyond their configured budget;
- support submissions in `failed`, or `queued`/`delivering` beyond 15 minutes;
- authorization-usage deliveries exhausting retries;
- repeated OAuth, access-evaluation, confirmation, or tenant-boundary denials;
- unexpected Public MCP enablement or catalog changes.

The support outbox is intentionally plaintext. Limit administrative access,
apply the configured retention policy, never put credentials in a report, and
treat destination systems as processors of customer-provided support data.

## Incident basics

- Disable Public MCP first if exposure scope is uncertain.
- Disable or rotate the affected root, OIDC, AI, upstream MCP, or runtime
  credential at its owning boundary. Never print a secret while diagnosing.
- Preserve request IDs, audit records, catalog revisions, image digests, and
  database timestamps. Do not preserve raw tool payloads unless the customer
  explicitly approves it.
- For suspected master-key compromise, stop writes, preserve evidence, rotate
  every encrypted downstream credential, and migrate to a new deployment key;
  changing only the environment value does not re-encrypt existing secrets.
