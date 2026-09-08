import { verifyWebhook } from "./orders-sdk.js";

// A single-process reference application. Production deduplication requires
// durable application storage and transaction semantics for the side effect.
export function webhookApplication({ signingSecret, onEvent, nowSeconds }) {
  const accepted = new Set(), pending = new Map();
  return async (request, response) => {
    if (request.method !== "POST" || request.url !== "/webhook") { response.writeHead(404).end(); return; }
    const chunks = [];
    let bytes = 0, event;
    try {
      for await (const chunk of request) {
        bytes += chunk.length;
        if (bytes > 64 * 1024) { response.writeHead(413).end(); request.resume(); return; }
        chunks.push(chunk);
      }
      event = verifyWebhook(Buffer.concat(chunks), {
        timestamp: request.headers["x-orders-timestamp"], signature: request.headers["x-orders-signature"],
        signingSecret, nowSeconds: nowSeconds(),
      });
    } catch { response.writeHead(400).end(); return; }
    if (accepted.has(event.event_id)) { response.writeHead(200).end(); return; }
    let operation = pending.get(event.event_id);
    if (!operation) {
      operation = Promise.resolve().then(() => onEvent(event)).then(() => { accepted.add(event.event_id); }).finally(() => { pending.delete(event.event_id); });
      pending.set(event.event_id, operation);
    }
    try { await operation; response.writeHead(200).end(); }
    catch { response.writeHead(500).end(); }
  };
}
