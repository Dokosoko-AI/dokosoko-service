import { createHash, randomUUID } from "node:crypto";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { CheerioCrawler, Configuration, RequestQueue, log } from "crawlee";
import { chromium } from "playwright";
import pg from "pg";
import { PinnedCrawlerHttpClient } from "./pinned-http-client";
import {
  assertSafeURL,
  canonicalize,
  CrawlerJobError,
  type CrawlerSettings,
  isDeniedAddress,
  isLocalDevelopmentAddress,
  isLocalhostSubdomain,
  loadCrawlerSettings,
} from "./security";
import {
  boundedResponseText,
  boundedResponseBytes,
  ingestOpenAPI,
  ingestUpload,
  injectionIndicators,
  type IngestionResult,
  type Job,
  type PageRecord,
  rejectGitSource,
  secureFetch,
  type SecureFetchOptions,
  validateOpenAPIDocument,
} from "./sources";

export {
  assertSafeURL,
  boundedResponseText,
  boundedResponseBytes,
  canonicalize,
  CrawlerJobError,
  ingestOpenAPI,
  ingestUpload,
  injectionIndicators,
  isDeniedAddress,
  isLocalDevelopmentAddress,
  isLocalhostSubdomain,
  loadCrawlerSettings,
  secureFetch,
  validateOpenAPIDocument,
};
export type { CrawlerSettings, IngestionResult, Job, PageRecord };

const { Pool } = pg;

async function sitemapSeeds(root: URL, settings: CrawlerSettings): Promise<string[]> {
  const target = new URL("/sitemap.xml", root).toString();
  try {
    const { response } = await secureFetch(target, settings, "application/xml,text/xml,text/plain;q=0.5");
    if (!response.ok) return [];
    const xml = await boundedResponseText(response, settings.maxBytes);
    const values = [...xml.matchAll(/<loc>\s*([^<]+?)\s*<\/loc>/gi)].map((match) => match[1].replaceAll("&amp;", "&"));
    const result: string[] = [];
    for (const value of values.slice(0, settings.maxPages)) {
      try {
        const url = await assertSafeURL(value, settings);
        if (url.origin === root.origin) result.push(canonicalize(url.toString()));
      } catch {
        // A malformed or unsafe sitemap entry does not invalidate the root source.
      }
    }
    return result;
  } catch {
    return [];
  }
}

export async function fetchRenderedResource(
  value: string,
  rootOrigin: string,
  settings: CrawlerSettings,
  fetchOptions: SecureFetchOptions = {},
): Promise<{ body: Uint8Array; headers: Record<string, string>; status: number; url: URL }> {
  const requested = new URL(value);
  if (requested.origin !== rootOrigin) {
    throw new CrawlerJobError("source_redirect_not_allowed", "Rendered website resources must stay on the source origin.");
  }
  const result = await secureFetch(
    requested.toString(),
    settings,
    "text/html,application/xhtml+xml,text/css,application/javascript,application/json,image/*;q=0.8,*/*;q=0.5",
    {
      ...fetchOptions,
      redirectPolicy: async (from, to) => {
        if (to.origin !== rootOrigin) {
          throw new CrawlerJobError("source_redirect_not_allowed", "Rendered website resources must stay on the source origin.");
        }
        await fetchOptions.redirectPolicy?.(from, to);
      },
    },
  );
  return {
    body: await boundedResponseBytes(result.response, settings.maxBytes),
    headers: Object.fromEntries(result.response.headers.entries()),
    status: result.response.status,
    url: result.url,
  };
}

export async function renderedPage(
  value: string,
  rootOrigin: string,
  settings: CrawlerSettings,
  fetchOptions: SecureFetchOptions = {},
): Promise<Pick<PageRecord, "html" | "text" | "title"> | null> {
  let browser;
  try {
    await assertSafeURL(value, settings, fetchOptions.resolver);
    browser = await chromium.launch({
      headless: true,
      args: [
        "--disable-dev-shm-usage",
        "--disable-quic",
        "--disable-features=WebTransport",
        "--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
        "--no-sandbox",
        "--proxy-server=http://127.0.0.1:9",
        "--proxy-bypass-list=<-loopback>",
      ],
    });
    const context = await browser.newContext({ serviceWorkers: "block", javaScriptEnabled: true, offline: true });
    await context.addInitScript(() => {
      for (const name of ["RTCPeerConnection", "webkitRTCPeerConnection"]) {
        Object.defineProperty(globalThis, name, { configurable: false, value: undefined, writable: false });
      }
    });
    await context.routeWebSocket("**/*", async (socket) => socket.close({ code: 1008, reason: "Network access is disabled in the renderer." }));
    const page = await context.newPage();
    await page.route("**/*", async (route) => {
      try {
        const requestURL = new URL(route.request().url());
        if (!["http:", "https:"].includes(requestURL.protocol) || requestURL.origin !== rootOrigin || route.request().method() !== "GET") return route.abort();
        const resource = await fetchRenderedResource(requestURL.toString(), rootOrigin, settings, fetchOptions);
        return route.fulfill({
          status: resource.status,
          headers: resource.headers,
          body: Buffer.from(resource.body),
        });
      } catch {
        return route.abort();
      }
    });
    await page.goto(value, { waitUntil: "networkidle", timeout: 20_000 });
    const result = {
      html: (await page.content()).slice(0, settings.maxBytes),
      text: (await page.locator("body").innerText()).slice(0, settings.maxBytes),
      title: await page.title(),
    };
    await context.close();
    return result;
  } catch {
    return null;
  } finally {
    await browser?.close();
  }
}

async function ingestWebsite(job: Job, settings: CrawlerSettings): Promise<IngestionResult> {
  const root = await assertSafeURL(job.location, settings);
  const seeds = [...new Set([canonicalize(root.toString()), ...(await sitemapSeeds(root, settings))])].slice(0, settings.maxPages);
  const pages: PageRecord[] = [];
  const seen = new Set<string>();
  const configuration = new Configuration({ persistStorage: false });
  const queue = await RequestQueue.open(`crawl-${job.id}`, { config: configuration });
  const crawler = new CheerioCrawler({
    httpClient: new PinnedCrawlerHttpClient(settings, root.origin),
    requestQueue: queue,
    maxRequestsPerCrawl: settings.maxPages,
    maxRequestRetries: 1,
    requestHandlerTimeoutSecs: 30,
    navigationTimeoutSecs: 20,
    requestHandler: async ({ request, $, response, body, enqueueLinks }) => {
      const contentLength = Number(response?.headers["content-length"] ?? 0);
      if (Number.isFinite(contentLength) && contentLength > settings.maxBytes) {
        throw new CrawlerJobError("source_too_large", `The source exceeds the configured ${settings.maxBytes}-byte limit.`);
      }
      const bodyLength = typeof body === "string" ? Buffer.byteLength(body, "utf8") : body.byteLength;
      if (bodyLength > settings.maxBytes) {
        throw new CrawlerJobError("source_too_large", `The source exceeds the configured ${settings.maxBytes}-byte limit.`);
      }
      const finalURL = canonicalize(request.loadedUrl ?? request.url);
      const final = await assertSafeURL(finalURL, settings);
      if (final.origin !== root.origin) throw new CrawlerJobError("source_redirect_not_allowed", "Website crawls may not navigate to a different origin.");
      if (seen.has(finalURL)) return;
      seen.add(finalURL);

      const rawHTML = $.html().slice(0, settings.maxBytes);
      const hidden = $("[hidden], [aria-hidden='true'], [style*='display:none'], [style*='visibility:hidden']").text();
      $("script,style,noscript,svg,nav,footer").remove();
      let text = ($("main,article").first().text() || $("body").text()).replace(/\s+/g, " ").trim().slice(0, settings.maxBytes);
      let html = $.html().slice(0, settings.maxBytes);
      let title = $("title").text().trim() || $("h1").first().text().trim();
      let rendered = false;
      if (text.length < 200 && /<script\b/i.test(rawHTML)) {
        const browserResult = await renderedPage(finalURL, root.origin, settings);
        if (browserResult) {
          ({ text, html, title } = browserResult);
          rendered = true;
        }
      }
      const indicators = injectionIndicators(`${text}\n${hidden}`, `${rawHTML}\n${html}`);
      pages.push({
        url: finalURL,
        title,
        text,
        html,
        contentType: String(response?.headers["content-type"] ?? "text/html"),
        status: response?.statusCode ?? 200,
        indicators,
        rendered,
      });
      await enqueueLinks({
        strategy: "same-origin",
        transformRequestFunction: (candidate) => {
          try {
            return { ...candidate, url: canonicalize(candidate.url) };
          } catch {
            return null;
          }
        },
      });
    },
    failedRequestHandler: async ({ request }, error) => log.warning(`crawl failed for ${request.url}: ${error.message}`),
  }, configuration);

  try {
    await crawler.run(seeds);
  } finally {
    await queue.drop();
  }
  if (pages.length === 0) {
    throw new CrawlerJobError("website_no_content", "The website crawl did not retrieve any reviewable pages. Check its URL, robots policy, and network availability.");
  }
  return { pages, discoveredCount: Math.max(seeds.length, pages.length) };
}

export async function collectSource(job: Job, settings: CrawlerSettings = loadCrawlerSettings()): Promise<IngestionResult> {
  switch (job.source_kind) {
    case "website":
      return ingestWebsite(job, settings);
    case "openapi":
      return ingestOpenAPI(job, settings);
    case "upload":
      return ingestUpload(job, settings);
    case "git":
      return rejectGitSource();
    default:
      throw new CrawlerJobError("source_kind_unsupported", `Source kind ${job.source_kind || "unknown"} is not supported by this worker.`);
  }
}

async function claim(pool: pg.Pool): Promise<Job | null> {
  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    const result = await client.query<Job>(`
      WITH candidate AS (
        SELECT id FROM crawl_jobs WHERE state = 'queued' ORDER BY queued_at FOR UPDATE SKIP LOCKED LIMIT 1
      ), claimed AS (
        UPDATE crawl_jobs SET state = 'running', started_at = now(), finished_at = null, error_code = null, error_message = null
        WHERE id = (SELECT id FROM candidate)
        RETURNING id, organisation_id, product_id, source_id
      )
      SELECT c.id::text, c.organisation_id::text, c.product_id::text, c.source_id::text,
             s.name AS source_name, s.kind AS source_kind, s.location, s.visibility::text
      FROM claimed c JOIN sources s ON s.id = c.source_id`);
    await client.query("COMMIT");
    return result.rows[0] ?? null;
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
}

function storedIndicatorNames(value: unknown): string[] | null {
  let parsed = value;
  if (typeof value === "string") {
    try {
      parsed = JSON.parse(value);
    } catch {
      return null;
    }
  }
  return Array.isArray(parsed) && parsed.every((indicator) => typeof indicator === "string") ? parsed : null;
}

export async function storePage(pool: pg.Pool, job: Job, page: PageRecord, settings: CrawlerSettings): Promise<boolean> {
  const digest = createHash("sha256").update(page.html).digest();
  const hex = digest.toString("hex");
  const directory = path.join(settings.dataDir, "snapshots", job.source_id);
  await mkdir(directory, { recursive: true, mode: 0o700 });
  const objectPath = path.join(directory, `${hex}.json`);
  await writeFile(objectPath, JSON.stringify({
    url: page.url,
    fetched_at: new Date().toISOString(),
    rendered: page.rendered,
    content_type: page.contentType,
    content: page.html,
  }), { mode: 0o600, flag: "wx" }).catch((error: NodeJS.ErrnoException) => { if (error.code !== "EEXIST") throw error; });

  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    // Reassessment and publication both take the source row first. This makes
    // their knowledge-document writes serial and gives every path one lock
    // order: source -> snapshot/document -> generation link.
    const lockedSource = await client.query<{ id: string }>(`
      SELECT id::text FROM sources
      WHERE id = $1 AND organisation_id = $2 AND product_id = $3
      FOR UPDATE`, [job.source_id, job.organisation_id, job.product_id]);
    if (!lockedSource.rows[0]) throw new Error("crawler source is no longer available");

    const snapshotID = randomUUID();
    const trustAssessment = JSON.stringify({
      rendered: page.rendered,
      source_kind: job.source_kind,
      injection_indicators: page.indicators,
    });
    const assessmentIndicators = JSON.stringify(page.indicators);
    const assessmentTrustLevel = page.indicators.length > 0 ? 10 : 70;
    let assessmentState = page.indicators.length > 0 ? "quarantined" : "validated";
    const snapshot = await client.query<{ id: string }>(`
      INSERT INTO source_snapshots(id, organisation_id, product_id, source_id, crawl_job_id, canonical_url, object_key, content_sha256, content_type, response_status, trust_indicators)
      VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
      ON CONFLICT (source_id, canonical_url, content_sha256) DO NOTHING
      RETURNING id::text`,
    [snapshotID, job.organisation_id, job.product_id, job.source_id, job.id, page.url, path.relative(settings.dataDir, objectPath), digest, page.contentType, page.status, trustAssessment]);
    let resolvedSnapshot = snapshot.rows[0] ? { id: snapshot.rows[0].id, inserted: true } : null;
    if (!resolvedSnapshot) {
      // ON CONFLICT can observe a concurrent uncommitted row that the INSERT
      // statement's snapshot cannot SELECT. A separate statement gets a fresh
      // READ COMMITTED snapshot after the conflicting transaction has finished.
      const existing = await client.query<{ id: string }>(`
        SELECT id::text FROM source_snapshots
        WHERE source_id = $1 AND canonical_url = $2 AND content_sha256 = $3
        FOR UPDATE`,
      [job.source_id, page.url, digest]);
      if (existing.rows[0]) resolvedSnapshot = { id: existing.rows[0].id, inserted: false };
    }
    if (!resolvedSnapshot) throw new Error("crawler could not resolve the stored snapshot");

    let documentID: string;
    let changed = resolvedSnapshot.inserted;
    if (resolvedSnapshot.inserted) {
      const document = await client.query<{ id: string }>(`
        INSERT INTO knowledge_documents(id, organisation_id, product_id, source_id, snapshot_id, title, canonical_url, body, visibility, state, trust_level, injection_indicators)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
        RETURNING id::text`,
      [randomUUID(), job.organisation_id, job.product_id, job.source_id, resolvedSnapshot.id, page.title || page.url, page.url, page.text, job.visibility, assessmentState, assessmentTrustLevel, assessmentIndicators]);
      documentID = document.rows[0].id;
    } else {
      const snapshotAssessment = await client.query(`
        UPDATE source_snapshots SET trust_indicators = $2::jsonb
        WHERE id = $1 AND trust_indicators IS DISTINCT FROM $2::jsonb`,
      [resolvedSnapshot.id, trustAssessment]);
      changed ||= (snapshotAssessment.rowCount ?? 0) > 0;
      const document = await client.query<{ id: string; state: string; trust_level: number; injection_indicators: unknown }>(`
        SELECT id::text, state::text, trust_level, injection_indicators FROM knowledge_documents
        WHERE product_id = $1 AND source_id = $2 AND snapshot_id = $3
        LIMIT 1 FOR UPDATE`, [job.product_id, job.source_id, resolvedSnapshot.id]);
      if (!document.rows[0]) throw new Error("crawler found a snapshot without its knowledge document");
      const current = document.rows[0];
      assessmentState = page.indicators.length > 0 ? "quarantined" : current.state === "published" ? "published" : "validated";
      const indicatorsChanged = JSON.stringify(storedIndicatorNames(current.injection_indicators)) !== JSON.stringify(page.indicators);
      if (current.state !== assessmentState || current.trust_level !== assessmentTrustLevel || indicatorsChanged) {
        await client.query(`
          UPDATE knowledge_documents
          SET state = $2, trust_level = $3, injection_indicators = $4::jsonb,
              revision = revision + 1, updated_at = now()
          WHERE id = $1`,
        [current.id, assessmentState, assessmentTrustLevel, assessmentIndicators]);
        changed = true;
      }
      documentID = current.id;
    }
    await client.query(`
      INSERT INTO crawl_job_documents(
        crawl_job_id, knowledge_document_id, changed,
        assessment_state, assessment_trust_level, assessment_injection_indicators
      )
      VALUES ($1, $2, $3, $4, $5, $6::jsonb)
      ON CONFLICT (crawl_job_id, knowledge_document_id) DO UPDATE
      SET changed = EXCLUDED.changed,
          assessment_state = EXCLUDED.assessment_state,
          assessment_trust_level = EXCLUDED.assessment_trust_level,
          assessment_injection_indicators = EXCLUDED.assessment_injection_indicators`,
    [job.id, documentID, changed, assessmentState, assessmentTrustLevel, assessmentIndicators]);
    await client.query("COMMIT");
    return changed;
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
}

export async function completeJob(
  pool: pg.Pool,
  job: Job,
  result: IngestionResult,
  changed: number,
): Promise<void> {
  const quarantined = result.pages.some((page) => page.indicators.length > 0);
  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    await client.query(`
      WITH assessed AS (
        SELECT id,
               CASE WHEN $2::boolean THEN 'quarantined'::lifecycle_state
                    WHEN state = 'published' THEN 'published'::lifecycle_state
                    ELSE 'validated'::lifecycle_state
               END AS target_state
        FROM sources WHERE id = $1
      )
      UPDATE sources AS source
      SET state = assessed.target_state, revision = source.revision + 1, updated_at = now()
      FROM assessed
      WHERE source.id = assessed.id
        AND (source.state IS DISTINCT FROM assessed.target_state OR $3::integer > 0)`,
    [job.source_id, quarantined, changed]);
    await client.query(`UPDATE crawl_jobs SET state = 'review', discovered_count = $2, fetched_count = $3, changed_count = $4, error_code = null, error_message = null, finished_at = now() WHERE id = $1`, [job.id, result.discoveredCount, result.pages.length, changed]);
    await client.query("COMMIT");
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
}

export async function processJob(pool: pg.Pool, job: Job, settings: CrawlerSettings = loadCrawlerSettings()): Promise<void> {
  const result = await collectSource(job, settings);
  let changed = 0;
  for (const page of result.pages) {
    if (await storePage(pool, job, page, settings)) changed++;
  }
  await completeJob(pool, job, result, changed);
}

export function jobFailure(error: unknown): { code: string; message: string } {
  if (error instanceof CrawlerJobError) return { code: error.code, message: error.message.slice(0, 1000) };
  return {
    code: "crawler_internal_error",
    message: "The ingestion worker failed unexpectedly. Review the worker logs and retry the job.",
  };
}

export async function failJob(pool: pg.Pool, job: Job, error: unknown): Promise<void> {
  const failure = jobFailure(error);
  await pool.query(`UPDATE crawl_jobs SET state = 'failed', error_code = $2, error_message = $3, finished_at = now() WHERE id = $1`, [job.id, failure.code, failure.message]);
}

export async function runOnce(pool: pg.Pool, settings: CrawlerSettings = loadCrawlerSettings()): Promise<boolean> {
  const job = await claim(pool);
  if (!job) return false;
  try {
    await processJob(pool, job, settings);
  } catch (error) {
    await failJob(pool, job, error);
    log.exception(error as Error, `crawl job ${job.id} failed`);
  }
  return true;
}

async function main() {
  const connectionString = process.env.DOKOSOKO_DATABASE_URL;
  if (!connectionString) throw new Error("DOKOSOKO_DATABASE_URL is required");
  const settings = loadCrawlerSettings();
  const pool = new Pool({ connectionString, max: 4, statement_timeout: 30_000, application_name: "dokosoko-crawler" });
  try {
    if (process.argv.includes("--once")) {
      await runOnce(pool, settings);
      return;
    }
    for (;;) {
      const worked = await runOnce(pool, settings);
      if (!worked) await new Promise((resolve) => setTimeout(resolve, 2_000));
    }
  } finally {
    await pool.end();
  }
}

if (import.meta.url === new URL(process.argv[1], "file:").href) {
  main().catch((error) => { log.exception(error as Error, "crawler stopped"); process.exitCode = 1; });
}
