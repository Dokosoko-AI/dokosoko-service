import { createHmac, timingSafeEqual } from "node:crypto";
import { setTimeout as delay } from "node:timers/promises";

export const SDK_VERSION = "1.2.3";

export class OrdersError extends Error {
  constructor(code, status = 0) {
    super(code);
    this.code = code;
    this.status = status;
  }
}

// This checked-in SDK exists only for the loopback evaluation vendor.
export class OrdersClient {
  constructor({ baseURL, apiKey, version = SDK_VERSION, sleep = delay }) {
    const url = new URL(baseURL);
    if (url.protocol !== "http:" || url.hostname !== "127.0.0.1" || url.username || url.password || url.pathname !== "/" || url.search || url.hash) throw new OrdersError("fixture_endpoint_required");
    if (version !== SDK_VERSION) throw new OrdersError("sdk_version_mismatch");
    if (typeof apiKey !== "string" || !apiKey.trim()) throw new OrdersError("application_credential_required");
    this.baseURL = url;
    this.apiKey = apiKey;
    this.sleep = sleep;
  }

  async request(method, path, { body, idempotencyKey } = {}) {
    const headers = { Authorization: `Bearer ${this.apiKey}` };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;
    for (let attempt = 0; attempt < 3; attempt++) {
      const response = await fetch(new URL(path, this.baseURL), { method, headers, body, redirect: "error", signal: AbortSignal.timeout(5000) });
      if (response.status === 429 && method === "GET" && attempt < 2) {
        const retryAfter = response.headers.get("Retry-After");
        await response.body?.cancel();
        if (!/^\d+$/.test(retryAfter ?? "") || Number(retryAfter) > 30) throw new OrdersError("retry_delay_unsupported", 429);
        await this.sleep(Number(retryAfter) * 1000);
        continue;
      }
      if (!response.ok) {
        await response.body?.cancel();
        throw new OrdersError("vendor_request_failed", response.status);
      }
      return response.json();
    }
    throw new OrdersError("retry_limit");
  }

  async getOrder(orderID) {
    const order = await this.request("GET", `/v1/orders/${encodeURIComponent(orderID)}`);
    if (order.id !== orderID || typeof order.status !== "string") throw new OrdersError("invalid_order_response");
    return order;
  }

  async listOrders() {
    const items = [], cursors = new Set();
    let cursor = "";
    for (let page = 0; page < 100; page++) {
      const query = new URLSearchParams({ limit: "20", ...(cursor ? { cursor } : {}) });
      const result = await this.request("GET", `/v1/orders?${query}`);
      if (!Array.isArray(result.items) || typeof result.next_cursor !== "string") throw new OrdersError("invalid_page_response");
      items.push(...result.items);
      if (!result.next_cursor) return items;
      if (cursors.has(result.next_cursor)) throw new OrdersError("repeated_cursor");
      cursors.add(result.next_cursor);
      cursor = result.next_cursor;
    }
    throw new OrdersError("page_limit");
  }

  async createOrder(payload, idempotencyKey) {
    if (typeof idempotencyKey !== "string" || !idempotencyKey.trim()) throw new OrdersError("idempotency_key_required");
    // A lost response is retried by the application with these same arguments.
    // This SDK never silently generates a new key or retries a write.
    return this.request("POST", "/v1/orders", { body: JSON.stringify(payload), idempotencyKey });
  }
}

export function verifyWebhook(rawBody, { timestamp, signature, signingSecret, nowSeconds }) {
  if (!Buffer.isBuffer(rawBody) || !/^\d+$/.test(timestamp ?? "") || !/^[0-9a-f]{64}$/.test(signature ?? "")) throw new OrdersError("invalid_webhook");
  const expected = createHmac("sha256", signingSecret).update(timestamp).update(".").update(rawBody).digest();
  if (!timingSafeEqual(expected, Buffer.from(signature, "hex"))) throw new OrdersError("invalid_webhook");
  if (!Number.isSafeInteger(Number(timestamp)) || Math.abs(nowSeconds - Number(timestamp)) > 300) throw new OrdersError("invalid_webhook");
  let event;
  try { event = JSON.parse(rawBody.toString("utf8")); } catch { throw new OrdersError("invalid_webhook"); }
  if (!event || typeof event.event_id !== "string" || !event.event_id.trim()) throw new OrdersError("invalid_webhook");
  return event;
}
