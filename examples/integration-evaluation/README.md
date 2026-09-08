# Orders integration application evaluation

This is a fixed reference application and SDK for the five tasks in the
[published retrieval corpus](../../internal/platform/testdata/published-task-retrieval-v1.json).
It runs real HTTP requests against a local fixture vendor and webhook application.
It is not a registry package, production vendor SDK, or deployment example.

Run from the service repository with Go 1.25 and Node.js 22.13 or newer:

```sh
GOCACHE=/private/tmp/dokosoko-go-cache \
DOKOSOKO_ACCEPTANCE_EVIDENCE_DIR=/private/tmp/dokosoko-application-evidence \
go test ./internal/platform -run '^TestPublishedTaskApplicationEvaluation$' -count=1 -v
```

The Go test imports the guidance and these three exact files: `package.json`,
`orders-sdk.js`, and `webhook-app.js`. It runs mandatory AI processing with
explicit network-free fixture responses, reviews and publishes the SDK content,
attaches it to an API and publishes that API. Query Lab retrieves guidance for
each task. Only then does the test invoke the checked-in Node runner. The runner
compares the published file hashes with its local executable files and checks
the task evidence before starting its loopback servers. It never executes code,
commands, paths or endpoints supplied by retrieved content.

The root `pnpm run verify` includes this test. A Go-only environment without
Node explicitly skips it; a Node process that fails is a test failure. No npm
installation is needed inside this example. The SDK is private and accepts only
the loopback fixture endpoint. All API keys and signing secrets are synthetic.

## Checks

| Task | Executed checks |
| --- | --- |
| Authenticated order read | Correct credential and encoded order ID; 401 without retry; incompatible SDK version rejected before HTTP |
| Complete pagination | Three pages with an opaque cursor; repeated cursor rejection; 100-request limit |
| Bounded retry | Retry-After honored; three-attempt limit; excessive delay rejected |
| Idempotent order creation | Response lost after server commit; identical retry creates no duplicate; changed payload returns 409 |
| Webhook validation | Valid and duplicate events; altered/reserialized body; past/future timestamps and boundaries; wrong signature/malformed payload; concurrent duplicates; failed side-effect retry |

The 17 checks passed with Node.js **v22.14.0** on the recorded local run. Six
negative evidence packets also failed before application execution: wrong SDK
version, changed published source, missing task, missing guidance, changed text,
and evidence from another publication.

Retry delays use an injected clock that records requested waits, so the test
asserts two requested 2,000 ms waits without claiming elapsed-time measurements.
Webhook timestamps use a fixed clock. Event deduplication covers a single process
in memory; persistence, restarts and multiple application workers are outside
this fixture. The reference application's behavior is asserted independently of
the SDK implementation against the fixture vendor's observed requests and effects.

## Reports and limits

The evidence directory contains the exact input and JSON report for the successful
run and each rejected packet. Reports identify the runtime, SDK release and source
hashes, API/SDK publications, input and runner hashes, retrieval traces, expected
outcomes, actual results and pass/fail status. An evidence validation failure
reports application execution `not_run`; an execution failure reports `fail`.
Loopback servers are closed when the runner exits.

To replay an exported input using the same checked-in fixture files:

```sh
node examples/integration-evaluation/check.mjs \
  /private/tmp/dokosoko-application-evidence/exact-published-application-input.json
```

Replay validates the retained evidence packet and source bytes. It does not
reconnect to DokoSoko or recheck current access. The original Go fixture uses
temporary publications and traces; their IDs are provenance for that run.

This evaluates a manually authored reference application using published SDK
guidance. It does not test recipe generation, an external AI provider's quality,
OAuth, a production coding client, a real vendor endpoint, or a customer's
repository. Reports label coding-client implementation `not_run` and comparison
with the existing documentation process `not_measured`. They are separate from
the MCP acceptance-client report and do not promote a publication or attachment
to Tested/Verified. The console's recorded implementation evidence remains a
separate client-reported result.
