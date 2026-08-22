# Go backend integration example

This is a runnable reference implementation of the [DokoSoko Backend Integration API](../../api/backend-integration-openapi.yaml). It implements the vendor-owned endpoint that receives consented bug reports and feedback from DokoSoko:

```text
POST /v1/support-submissions
```

This is a server example, not a DokoSoko SDK. DokoSoko is the client. Your service owns the endpoint, bearer credential, persistence, ticketing workflow, and operational policy.

## What is included

- exact bearer-token authentication using a constant-time comparison;
- strict `Content-Type`, `Idempotency-Key`, and `X-DokoSoko-Request-ID` validation;
- a 256 KiB body limit, unknown-field rejection, and bounded contract validation;
- mutually exclusive bug and feedback payload validation;
- canonical JSON hashing so insignificant whitespace does not create false conflicts;
- PostgreSQL-backed idempotency results retained for at least 24 hours;
- transactional serialization of concurrent attempts for the same key;
- exact replay of the original `202` receipt;
- `409` for idempotency-key reuse with different request content;
- durable storage of accepted submissions;
- retryable `503` responses with `Retry-After` when persistence is unavailable;
- JSON logs that exclude credentials and submission content;
- liveness, readiness, server timeouts, panic recovery, and graceful shutdown;
- unit tests, example requests, a Dockerfile, and Docker Compose.

## Run with Docker Compose

From this directory:

```bash
docker compose up --build
```

In another terminal:

```bash
make send-bug
make send-feedback
```

The first request returns a receipt like:

```json
{
  "id": "receipt_6f67499fbe1e49cbb7f7e6e42f5a80ea",
  "status": "accepted",
  "external_id": "submission_bug_example_0001"
}
```

Running `make send-bug` again sends the same idempotency key and content. The service returns the exact original receipt with:

```http
Idempotency-Replayed: true
```

If the same key is sent with different content, the response is permanent and explicit:

```http
HTTP/1.1 409 Conflict
Content-Type: application/json; charset=utf-8

{
  "error": {
    "code": "idempotency_key_conflict",
    "message": "Idempotency-Key was already used with different request content.",
    "request_id": "req_..."
  }
}
```

Stop the example without removing its PostgreSQL volume:

```bash
docker compose down
```

To remove the disposable example data as well:

```bash
docker compose down --volumes
```

## Run directly

Start PostgreSQL and create an empty database, then:

```bash
cp .env.example .env
set -a
source .env
set +a
go run ./cmd/server
```

The service applies its idempotent schema migration during startup.

Configuration:

| Variable | Required | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | Yes | PostgreSQL connection URL. Use TLS in production. |
| `DOKOSOKO_BACKEND_BEARER_TOKEN` | Yes | Vendor-issued credential configured on the matching DokoSoko backend connection; minimum 32 characters. |
| `LISTEN_ADDR` | No | Listen address; defaults to `:8081`. |

Operational endpoints:

| Endpoint | Meaning |
| --- | --- |
| `GET /healthz` | Process is running. |
| `GET /readyz` | PostgreSQL is reachable. |

## Test

```bash
go test ./...
```

The default tests use an in-memory test double and require no external services. To exercise the real PostgreSQL transaction and concurrent replay path, set `TEST_DATABASE_URL` to a disposable database before running the PostgreSQL package tests.

## Idempotency model

DokoSoko delivers at least once. The `Idempotency-Key` identifies one logical submission, while `X-DokoSoko-Request-ID` identifies one HTTP attempt.

For every request, this example:

1. strictly decodes and validates the payload;
2. re-encodes it to canonical JSON and computes SHA-256;
3. acquires a transaction-scoped PostgreSQL advisory lock derived from the idempotency key;
4. returns the stored receipt when the key and digest match;
5. returns `409` when the key exists with a different digest;
6. otherwise stores the submission and response atomically before returning `202`.

The advisory-lock hash is used only for serialization. A theoretical hash collision can reduce concurrency but cannot mix or overwrite records because the full idempotency key remains the database primary key.

`retain_until` is the earliest safe deletion time, not an automatic expiry. This example keeps results indefinitely because delayed exact replays are safer than silently processing a submission twice. A production retention job may delete results after `retain_until`, subject to your audit and support-retention policy.

## Production adaptation checklist

- Generate a random credential with at least 256 bits of entropy and deliver it through a secure administrative channel.
- Terminate TLS at a trusted proxy or add TLS directly; DokoSoko accepts only an HTTPS backend origin.
- Store `DATABASE_URL` and the bearer token in your secret manager, never in an image or source control.
- Replace the example receipt mapping with your durable queue, issue tracker, or support system transaction.
- Preserve the idempotency transaction when introducing downstream dependencies. Do not mark the request accepted before the durable handoff succeeds.
- Return `503` or `429` with `Retry-After` for temporary failures. Treat other `4xx` responses as permanent contract failures.
- Never log bearer tokens, full request bodies, email addresses, or diagnostic content.
- Apply your data-retention, access-control, encryption-at-rest, and deletion policies to `support_submissions`.
- Run multiple replicas against the same PostgreSQL database; in-memory idempotency is not sufficient.
- Alert on sustained `401`, `409`, `429`, and `5xx` rates, and propagate `X-DokoSoko-Request-ID` into internal tracing.

The sample intentionally does not call DokoSoko, validate customer OIDC tokens, or share credentials with the optional identity integration. Those belong to separate trust boundaries.
