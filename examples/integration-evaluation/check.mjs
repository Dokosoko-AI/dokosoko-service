import assert from "node:assert/strict";
import { createHash, createHmac } from "node:crypto";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import { SDK_VERSION, OrdersClient } from "./orders-sdk.js";
import { webhookApplication } from "./webhook-app.js";

const hash = (value) => `sha256:${createHash("sha256").update(value).digest("hex")}`;
const fixtureKey = "orders-application-fixture-key";
const signingSecret = "webhook-signing-fixture-only";
const now = 1800000000;

async function checkedEvidence(input) {
  const reject = (code) => { throw new Error(code); };
  if (input.schema_version !== "published-application-fixture-v1") reject("unsupported_evidence_format");
  const manifest = JSON.parse(await readFile(new URL("./package.json", import.meta.url), "utf8"));
  if (input.sdk?.exact_version !== SDK_VERSION || manifest.version !== SDK_VERSION || input.sdk.coordinate !== manifest.name) reject("sdk_version_mismatch");
  if (!input.api_publication_id || !input.sdk.publication_id || !Array.isArray(input.sdk.files) || input.sdk.files.length !== 3) reject("incomplete_publication_evidence");
  const files = {};
  for (const name of ["package.json", "orders-sdk.js", "webhook-app.js"]) {
    const source = await readFile(new URL(`./${name}`, import.meta.url));
    const match = input.sdk.files.filter((file) => file.path === name);
    if (match.length !== 1 || !match[0].file_id || match[0].content_hash !== hash(source)) reject("published_source_mismatch");
    files[name] = hash(source);
  }
  const corpusBytes = await readFile(new URL("../../internal/platform/testdata/published-task-retrieval-v1.json", import.meta.url));
  if (input.corpus_sha256 !== hash(corpusBytes)) reject("corpus_mismatch");
  const corpus = JSON.parse(corpusBytes);
  if (!Array.isArray(input.tasks) || input.tasks.length !== corpus.cases.length) reject("incomplete_task_evidence");
  for (const scenario of corpus.cases) {
    const matches = input.tasks.filter((task) => task.name === scenario.name);
    if (matches.length !== 1) reject("ambiguous_task_evidence");
    const task = matches[0];
    if (!task.trace_id || !Array.isArray(task.evidence)) reject("missing_retrieval_trace");
    for (const evidence of task.evidence) {
      if (evidence.publication_id !== input.sdk.publication_id || evidence.api_publication_id !== input.api_publication_id || evidence.exact_version !== SDK_VERSION || !evidence.source_entity_id || !/^sha256:[a-f0-9]{64}$/.test(evidence.content_hash ?? "") || typeof evidence.text !== "string" || hash(evidence.text) !== evidence.text_sha256) reject("changed_task_evidence");
    }
    for (const path of scenario.expected_paths) {
      if (!task.evidence.some((evidence) => evidence.path === path)) reject("missing_task_guidance");
    }
    const text = task.evidence.filter((evidence) => scenario.expected_paths.includes(evidence.path)).map((evidence) => evidence.text).join("\n");
    if (scenario.required_phrases.some((phrase) => !text.includes(phrase))) reject("incomplete_task_guidance");
  }
  return files;
}

async function listen(handler) {
  const server = createServer(handler);
  await new Promise((resolve, reject) => { server.once("error", reject); server.listen(0, "127.0.0.1", resolve); });
  return { url: `http://127.0.0.1:${server.address().port}`, close: () => new Promise((resolve, reject) => { server.closeAllConnections(); server.close((error) => error ? reject(error) : resolve()); }) };
}

async function vendorFixture() {
  const state = { mode: "read", requests: [], attempts: 0, creates: 0, idempotency: new Map(), lostResponse: false };
  const server = await listen(async (request, response) => {
    const chunks = [];
    for await (const chunk of request) chunks.push(chunk);
    const body = Buffer.concat(chunks).toString("utf8"), url = new URL(request.url, "http://fixture.invalid");
    state.requests.push({ method: request.method, path: url.pathname, cursor: url.searchParams.get("cursor"), key: request.headers["idempotency-key"], body });
    const send = (status, value, headers = {}) => { response.writeHead(status, { "Content-Type": "application/json", ...headers }).end(JSON.stringify(value)); };
    if (request.headers.authorization !== `Bearer ${fixtureKey}`) { send(401, { error: "application_credential_invalid" }); return; }
    if (url.pathname === "/v1/orders" && request.method === "POST") {
      const key = request.headers["idempotency-key"], previous = state.idempotency.get(key);
      if (!key) { send(400, { error: "idempotency_key_required" }); return; }
      if (previous) { send(previous.body === body ? 200 : 409, previous.body === body ? previous.order : { error: "idempotency_payload_conflict" }); return; }
      const order = { id: `created-${++state.creates}`, status: "pending" };
      state.idempotency.set(key, { body, order });
      if (state.mode === "lost-response" && !state.lostResponse) { state.lostResponse = true; request.socket.destroy(); return; }
      send(201, order); return;
    }
    if (url.pathname === "/v1/orders" && request.method === "GET") {
      const cursor = url.searchParams.get("cursor");
      if (state.mode === "repeated-cursor") { send(200, { items: [], next_cursor: "repeat" }); return; }
      if (state.mode === "endless-pages") { send(200, { items: [], next_cursor: String(++state.attempts) }); return; }
      if (url.searchParams.get("limit") !== "20") { send(400, { error: "wrong_limit" }); return; }
      const pages = {
        "": { items: [{ id: "a" }], next_cursor: "next /+=?&" },
        "next /+=?&": { items: [{ id: "b" }], next_cursor: "last" },
        last: { items: [{ id: "c" }], next_cursor: "" },
      };
      send(pages[cursor ?? ""] ? 200 : 400, pages[cursor ?? ""] ?? { error: "cursor_changed" }); return;
    }
    if (url.pathname.startsWith("/v1/orders/") && request.method === "GET") {
      state.attempts++;
      if (state.mode === "excessive-delay" || state.mode === "always-limited" || (state.mode === "rate-limit" && state.attempts < 3)) {
        send(429, { error: "rate_limit" }, { "Retry-After": state.mode === "excessive-delay" ? "31" : "2" }); return;
      }
      send(200, { id: decodeURIComponent(url.pathname.slice("/v1/orders/".length)), status: "paid" }); return;
    }
    send(404, { error: "wrong_api_version_or_path" });
  });
  return { ...server, state, reset(mode) { Object.assign(state, { mode, requests: [], attempts: 0, creates: 0, idempotency: new Map(), lostResponse: false }); } };
}

async function runApplication(input, sourceFiles) {
  const vendor = await vendorFixture(), cases = [];
  const sleeps = [];
  const client = new OrdersClient({ baseURL: vendor.url, apiKey: fixtureKey, sleep: async (milliseconds) => { sleeps.push(milliseconds); } });
  const check = async (task, name, expected, operation) => {
    try {
      const actual = await operation();
      cases.push({ task, name, expected, actual, status: "pass" });
    } catch (error) {
      cases.push({ task, name, expected, actual: String(error.message).slice(0, 2000), status: "fail" });
    }
  };
  const reset = (mode) => { vendor.reset(mode); sleeps.length = 0; };
  try {
    await check("authenticated-read", "customer-credential", "HTTP 200, requested order ID, paid status, one request", async () => {
      reset("read");
      assert.deepEqual(await client.getOrder("order /東京"), { id: "order /東京", status: "paid" });
      assert.equal(vendor.state.requests.length, 1);
      return { status: 200, order_id: "order /東京", order_status: "paid", requests: 1 };
    });
    await check("authenticated-read", "invalid-credential-no-retry", "HTTP 401 and no retry", async () => {
      reset("read");
      const invalid = new OrdersClient({ baseURL: vendor.url, apiKey: "invalid-application-fixture", sleep: client.sleep });
      await assert.rejects(invalid.getOrder("order-1"), { status: 401 });
      assert.equal(vendor.state.requests.length, 1); assert.deepEqual(sleeps, []);
      return { status: 401, requests: 1, retry_waits: sleeps };
    });
    await check("authenticated-read", "wrong-sdk-version", "Reject SDK 0.9.0 before an API request", async () => {
      reset("read");
      assert.throws(() => new OrdersClient({ baseURL: vendor.url, apiKey: fixtureKey, version: "0.9.0" }), { code: "sdk_version_mismatch" });
      assert.equal(vendor.state.requests.length, 0);
      return { error: "sdk_version_mismatch", requests: 0 };
    });
    await check("complete-pagination", "all-pages", "Three ordered items; preserve opaque cursor bytes; stop after page three", async () => {
      reset("pages");
      assert.deepEqual(await client.listOrders(), [{ id: "a" }, { id: "b" }, { id: "c" }]);
      assert.deepEqual(vendor.state.requests.map((request) => request.cursor), [null, "next /+=?&", "last"]);
      return { item_ids: ["a", "b", "c"], requests: 3, opaque_cursor_preserved: true };
    });
    await check("complete-pagination", "repeated-cursor", "Reject repeated cursor after two requests", async () => {
      reset("repeated-cursor"); await assert.rejects(client.listOrders(), { code: "repeated_cursor" });
      assert.equal(vendor.state.requests.length, 2); return { error: "repeated_cursor", requests: 2 };
    });
    await check("complete-pagination", "page-budget", "Stop at 100 requests when pagination never terminates", async () => {
      reset("endless-pages"); await assert.rejects(client.listOrders(), { code: "page_limit" });
      assert.equal(vendor.state.requests.length, 100); return { error: "page_limit", requests: 100 };
    });
    await check("bounded-retry", "retry-after", "Two 429 responses, two 2000ms requested waits, then success", async () => {
      reset("rate-limit"); assert.equal((await client.getOrder("order-1")).status, "paid");
      assert.equal(vendor.state.requests.length, 3); assert.deepEqual(sleeps, [2000, 2000]);
      return { final_status: 200, requests: 3, requested_waits_ms: [...sleeps] };
    });
    await check("bounded-retry", "retry-budget", "Reject persistent 429 after three total attempts", async () => {
      reset("always-limited"); await assert.rejects(client.getOrder("order-1"), { status: 429 });
      assert.equal(vendor.state.requests.length, 3); assert.deepEqual(sleeps, [2000, 2000]);
      return { final_status: 429, requests: 3, requested_waits_ms: [...sleeps] };
    });
    await check("bounded-retry", "excessive-delay", "Reject Retry-After 31; no retry or wait", async () => {
      reset("excessive-delay"); await assert.rejects(client.getOrder("order-1"), { code: "retry_delay_unsupported" });
      assert.equal(vendor.state.requests.length, 1); assert.deepEqual(sleeps, []);
      return { error: "retry_delay_unsupported", requests: 1, requested_waits_ms: [] };
    });
    await check("idempotent-create", "lost-response", "Retry identical request with same key; one created order", async () => {
      reset("lost-response"); const payload = { reference: "customer-order-1", quantity: 1 };
      await assert.rejects(client.createOrder(payload, "logical-create-1"));
      assert.equal(vendor.state.requests.length, 1, "write retried automatically");
      const result = await client.createOrder(payload, "logical-create-1");
      assert.equal(result.id, "created-1"); assert.equal(vendor.state.creates, 1);
      assert.equal(vendor.state.requests.length, 2);
      assert.deepEqual(vendor.state.requests[0], vendor.state.requests[1]);
      return { order_id: result.id, created_orders: 1, requests: 2, same_key_and_payload: true };
    });
    await check("idempotent-create", "conflicting-payload", "Same key with changed payload returns 409, no second order", async () => {
      reset("create"); await client.createOrder({ quantity: 1 }, "logical-create-2");
      await assert.rejects(client.createOrder({ quantity: 2 }, "logical-create-2"), { status: 409 });
      assert.equal(vendor.state.creates, 1); assert.equal(vendor.state.requests.length, 2);
      return { status: 409, created_orders: 1, requests: 2 };
    });
    let effects = [], attempts = 0;
    const app = await listen(webhookApplication({ signingSecret, nowSeconds: () => now, onEvent: async (event) => {
      attempts++;
      if (event.event_id === "retry-event" && attempts === 1) throw new Error("fixture_side_effect_failed");
      await new Promise((resolve) => setTimeout(resolve, 5));
      effects.push(event.event_id);
    } }));
    const deliver = async (raw, timestamp = now, signedBody = raw, secret = signingSecret) => {
      const signature = createHmac("sha256", secret).update(`${timestamp}.`).update(signedBody).digest("hex");
      const response = await fetch(`${app.url}/webhook`, { method: "POST", body: raw, headers: { "X-Orders-Timestamp": String(timestamp), "X-Orders-Signature": signature }, signal: AbortSignal.timeout(5000) });
      await response.arrayBuffer(); return response.status;
    };
    try {
      await check("webhook-validation", "valid-and-duplicate", "Valid raw body accepted twice, one side effect", async () => {
        effects = []; attempts = 0;
        const raw = '{ "event_id": "valid-event", "label": "東京" }';
        assert.equal(await deliver(raw), 200); assert.equal(await deliver(raw), 200);
        assert.deepEqual(effects, ["valid-event"]); assert.equal(attempts, 1);
        return { statuses: [200, 200], side_effects: 1 };
      });
      await check("webhook-validation", "altered-or-reserialized-body", "Altered bytes and JSON reserialization rejected before side effects", async () => {
        effects = []; attempts = 0;
        const signed = '{ "event_id": "altered-event", "quantity": 1 }';
        assert.equal(await deliver(signed.replace('1 }', '2 }'), now, signed), 400);
        assert.equal(await deliver(JSON.stringify(JSON.parse(signed)), now, signed), 400);
        assert.equal(attempts, 0); return { statuses: [400, 400], side_effects: 0 };
      });
      await check("webhook-validation", "timestamp-window", "Reject timestamps 301 seconds old or future; accept both 300-second boundaries", async () => {
        effects = []; attempts = 0;
        const statuses = [];
        for (const offset of [-301, 301, -300, 300]) statuses.push(await deliver(JSON.stringify({ event_id: `timestamp-${offset}` }), now + offset));
        assert.deepEqual(statuses, [400, 400, 200, 200]); assert.equal(attempts, 2);
        return { offsets_seconds: [-301, 301, -300, 300], statuses, side_effects: 2 };
      });
      await check("webhook-validation", "invalid-signature-and-payload", "Wrong signing secret, malformed JSON and missing event ID rejected", async () => {
        effects = []; attempts = 0;
        const raw = '{"event_id":"wrong-secret"}';
        const statuses = [await deliver(raw, now, raw, "wrong-fixture-secret"), await deliver("{broken"), await deliver('{"id":"missing-event-id"}')];
        assert.deepEqual(statuses, [400, 400, 400]); assert.equal(attempts, 0);
        return { statuses, side_effects: 0 };
      });
      await check("webhook-validation", "concurrent-duplicate", "Concurrent valid duplicates return 200 with one side effect", async () => {
        effects = []; attempts = 0;
        const raw = '{"event_id":"concurrent-event"}';
        const statuses = await Promise.all([deliver(raw), deliver(raw)]);
        assert.deepEqual(statuses, [200, 200]); assert.deepEqual(effects, ["concurrent-event"]); assert.equal(attempts, 1);
        return { statuses, side_effects: 1 };
      });
      await check("webhook-validation", "failed-side-effect-retry", "Failed handler returns 500; redelivery retries and succeeds exactly once", async () => {
        effects = []; attempts = 0; const raw = '{"event_id":"retry-event"}';
        const statuses = [await deliver(raw), await deliver(raw), await deliver(raw)];
        assert.deepEqual(statuses, [500, 200, 200]); assert.deepEqual(effects, ["retry-event"]); assert.equal(attempts, 2);
        return { statuses, attempts: 2, side_effects: 1 };
      });
    } finally { await app.close(); }
  } finally { await vendor.close(); }
  return {
    schema_version: "application-fixture-report-v1", suite_version: "orders-integration-v1",
    evidence_origin: "local_reference_application_execution", observed_at: new Date().toISOString(),
    runtime: { name: "Node.js", version: process.version, platform: process.platform, architecture: process.arch },
    environment: { vendor: "loopback Orders API fixture v1", retry_clock: "recorded requested waits; no wall-clock delay", webhook_clock_seconds: now, deduplication: "single process, in memory" },
    sdk: { coordinate: input.sdk.coordinate, exact_version: SDK_VERSION, release_id: input.sdk.release_id, release_hash: input.sdk.release_hash, publication_id: input.sdk.publication_id, candidate_hash: input.sdk.candidate_hash, processor_versions: input.sdk.processor_versions, source_files: sourceFiles },
    api_publication_id: input.api_publication_id, api_snapshot_hash: input.api_snapshot_hash, corpus_sha256: input.corpus_sha256,
    task_evidence: input.tasks.map(({ name, trace_id, evidence }) => ({ name, trace_id, evidence: evidence.map((item) => Object.fromEntries(Object.entries(item).filter(([key]) => key !== "text"))) })),
    application_status: cases.every((item) => item.status === "pass") ? "pass" : "fail", cases,
    external_model_quality: "not_measured", coding_client_implementation: "not_run", existing_documentation_comparison: "not_measured",
  };
}

let phase = "evidence_validation";
try {
  if (process.argv.length !== 3) throw new Error("usage: node check.mjs evidence.json");
  const raw = await readFile(process.argv[2]);
  if (raw.length > 512 * 1024) throw new Error("evidence_size_limit");
  const input = JSON.parse(raw);
  const sourceFiles = await checkedEvidence(input);
  phase = "application_execution";
  const report = await runApplication(input, sourceFiles);
  report.evidence_sha256 = hash(raw);
  report.runner_sha256 = hash(await readFile(new URL("./check.mjs", import.meta.url)));
  console.log(JSON.stringify(report, null, 2));
  if (report.application_status !== "pass") process.exitCode = 1;
} catch (error) {
  console.log(JSON.stringify({ schema_version: "application-fixture-report-v1", evidence_origin: "local_reference_application_execution", application_status: phase === "application_execution" ? "fail" : "not_run", stage: phase, error: String(error.message).slice(0, 2000) }));
  process.exitCode = 1;
}
