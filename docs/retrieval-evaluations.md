# Published guidance retrieval evaluations

`TestPublishedTaskRetrievalEvaluation` exercises the production ingestion,
required AI processing, explicit review, API publication, index activation,
Query Lab retrieval, reranking, context selection and trace persistence paths.
It uses the same versioned synthetic corpus against memory and PostgreSQL.
The three pairs in `retrieval-eval-v1.json` remain an embedding smoke test;
they do not establish retrieval quality.

## Reproduce

Run from the service repository. PostgreSQL requires a disposable PostgreSQL 17
database with pgvector, as described in the [README](../README.md).

```sh
GOCACHE=/private/tmp/dokosoko-go-cache \
DOKOSOKO_ACCEPTANCE_EVIDENCE_DIR=/private/tmp/dokosoko-retrieval-evidence \
go test ./internal/platform -run '^TestPublishedTaskRetrievalEvaluation$' -count=1 -v
```

Set `DOKOSOKO_TEST_DATABASE_URL` before that command to run both backends. Without
it, PostgreSQL is explicitly skipped. The database fixture uses an isolated
schema, applies all migrations and removes its schema on completion. Exported
JSON reports retain publication IDs, source IDs/hashes, corpus hash, processor,
map and index versions, queries, expected paths, required phrases, scores,
context size and observed latency. Trace IDs identify that run's temporary
database records; they are not durable production trace links.

## Corpus and scoring

[`published-task-retrieval-v1.json`](../internal/platform/testdata/published-task-retrieval-v1.json)
contains twelve short Orders SDK guides: six relevant to five integration tasks
and six distractors. Every evaluated API index contains thirteen units including
its map. The fixture publishes SDK 0.9.0, advances the same attachment to 1.2.3,
retains the earlier publication, leaves 2.0.0 unpublished, and publishes a similar
private SDK under another API. No registry package is downloaded or executed.

For each query, the gate requires at least 90% of the expected source files in
the first five selected results and all required task phrases in the relevant
excerpts. Every returned unit must cite the exact reviewed release, publication,
section, file and content hash. The entire candidate trace must exclude other
APIs and releases, including candidates omitted by ranking or the context budget.
Selection is limited to five results and 2,000 estimated context tokens.

Recall is the fraction of expected source files found. Precision is the fraction
of returned units from those expected files; a routing map does not count as
task-answer evidence. Precision is recorded as a baseline, with no promoted
threshold yet. These judgments apply to this synthetic corpus, not arbitrary
vendor content.

## Recorded baseline — 8 September 2026

Both backends returned the following results with retrieval profile `hybrid-v1`:

| Task | Recall@5 | Precision@5 | Context tokens |
| --- | ---: | ---: | ---: |
| Authenticate an order read; handle invalid credentials | 100% | 20% | 697 |
| Read all cursor pages without an infinite loop | 100% | 20% | 666 |
| Honor Retry-After with bounded read retries | 100% | 20% | 667 |
| Retry order creation without duplicates | 100% | 20% | 660 |
| Validate webhook signatures, timestamps and duplicate events | 100% | 40% | 767 |

The required passages were found, but three or four returned units per query
were outside the labeled task evidence. This is a retrieval-noise finding,
not proof of a better developer experience. Latency in the exported local
fixture reports is diagnostic, not a production performance benchmark.

Four unavailable-scope cases returned zero results and empty candidate traces:
unpublished version, nonexistent version, another API's SDK release, and a
contradictory version/release pair. A publication from another API was rejected.
The previous publication still retrieved its original 0.9.0 guidance after the
attachment advanced. These checks establish exact-scope behavior; they do not
measure whether arbitrary unsupported natural-language questions return no
answer. Query Lab is an administrator surface, so this suite does not establish
anonymous or customer authorization correctness; the MCP audience suites cover
those boundaries separately.

## SDK upgrade regression discovered by the suite

PostgreSQL initially rejected an SDK attachment update after publication while
memory accepted it. Foreign keys tied historical publication/advisory rows to
the attachment's mutable release selection. Migration
`0073_sdk_binding_publication_history.sql` retains stable attachment identity and
direct immutable asset references. A locking insertion guard checks the exact
current release, content, assertion and selector before a new snapshot is saved.
Historical publication/advisory immutability and lineage guards remain enabled.

The PostgreSQL regression verifies old and new publication reads, advisories
before and after the update, stale/mismatched snapshot rejection, immutable
history, deletion protection and a concurrent update blocked by the snapshot
transaction. A populated fixture was also cloned and upgraded from migrations
0071 through 0073: hashes and counts across ten retained tables were unchanged,
including four published SDK selections and twelve knowledge units. Reapplying
migrations was idempotent. That rehearsal did not test backup restoration.

## Evidence still required

The AI stages use explicit network-free fixture responses; this is not evidence
of external model quality. Reports label application execution `not_run` and the
existing-documentation comparison `not_measured`. No recipe, SDK or integration
receives a Tested or Verified claim from this suite.

The separate [reference application evaluation](../examples/integration-evaluation/README.md)
now executes 17 HTTP/application checks for these five tasks with the exact
published fixture SDK bytes and records Node.js v22.14.0, publication identities
and failure cases. Six mismatched/incomplete evidence packets are rejected before
execution. It remains distinct from the retrieval-only reports above and does
not establish a production coding-client or customer-application outcome.

Next compare representative vendor tasks with the existing documentation process
using actual supported coding clients. Keep application success, support
interventions, completion time and vendor maintenance effort separate from these
retrieval diagnostics and the synthetic reference application.
