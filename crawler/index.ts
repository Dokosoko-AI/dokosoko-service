import { createHash, randomUUID } from "node:crypto";
import { mkdir, writeFile } from "node:fs/promises";
import { hostname } from "node:os";
import path from "node:path";
import process from "node:process";
import { CheerioCrawler, Configuration, RequestQueue, log } from "crawlee";
import { chromium } from "playwright";
import pg from "pg";
import {
  persistDeveloperAssetCandidates,
  type LegacyDocumentReference,
} from "./candidate-persistence";
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
  type IngestionDiagnostic,
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
export type { CrawlerSettings, IngestionDiagnostic, IngestionResult, Job, PageRecord };
export {
  DOCUMENTATION_PROCESSOR_VERSIONS,
  OPENAPI_PROCESSOR_VERSIONS,
  deterministicIngestionUUID,
  persistDeveloperAssetCandidates,
} from "./candidate-persistence";
export type { CandidatePersistenceResult, LegacyDocumentReference } from "./candidate-persistence";

const { Pool } = pg;
const LEASE_DURATION_MILLISECONDS = 60_000;
const HEARTBEAT_INTERVAL_MILLISECONDS = 15_000;
const generatedLeaseOwner = `${hostname()}:${process.pid}:${randomUUID()}`;

export function crawlerLeaseOwner(env: Readonly<Record<string, string | undefined>> = process.env): string {
  return env.DOKOSOKO_CRAWLER_WORKER_ID?.trim().slice(0, 255) || generatedLeaseOwner;
}

type SitemapSeeds = {
  urls: string[];
  skippedCount: number;
  diagnostics: IngestionDiagnostic[];
};

function normalizedWebsitePath(value: URL): string | null {
  try {
    const decoded = decodeURIComponent(value.pathname).replaceAll("\\", "/");
    if (decoded.includes("\0")) return null;
    const normalized = path.posix.normalize(decoded.startsWith("/") ? decoded : `/${decoded}`);
    return normalized === "." ? "/" : normalized;
  } catch {
    return null;
  }
}

export function websiteURLWithinScope(root: URL, candidate: URL): boolean {
  if (candidate.origin !== root.origin) return false;
  const rootPath = normalizedWebsitePath(root);
  const candidatePath = normalizedWebsitePath(candidate);
  if (!rootPath || !candidatePath) return false;
  const boundary = rootPath === "/" ? "/" : rootPath.replace(/\/+$/, "");
  return boundary === "/" || candidatePath === boundary || candidatePath.startsWith(`${boundary}/`);
}

function websiteSitemapURL(root: URL): string {
  const rootPath = normalizedWebsitePath(root) ?? "/";
  const boundary = rootPath === "/" ? "" : rootPath.replace(/\/+$/, "");
  const target = new URL(root.origin);
  target.pathname = `${boundary}/sitemap.xml`;
  return target.toString();
}

async function sitemapSeeds(root: URL, settings: CrawlerSettings): Promise<SitemapSeeds> {
  const target = websiteSitemapURL(root);
  try {
    const { response } = await secureFetch(target, settings, "application/xml,text/xml,text/plain;q=0.5", {
      redirectPolicy: async (_from, to) => {
        if (!websiteURLWithinScope(root, to)) {
          throw new CrawlerJobError("source_redirect_not_allowed", "Website sitemap discovery may not leave the configured source path.");
        }
      },
    });
    if (!response.ok) return { urls: [], skippedCount: 0, diagnostics: [] };
    const xml = await boundedResponseText(response, settings.maxBytes);
    const values = [...xml.matchAll(/<loc>\s*([^<]+?)\s*<\/loc>/gi)].map((match) => match[1].replaceAll("&amp;", "&"));
    const result: string[] = [];
    let skippedCount = Math.max(0, values.length - settings.maxPages);
    const diagnostics: IngestionDiagnostic[] = [];
    if (values.length > settings.maxPages) diagnostics.push({
      code: "website_page_limit_reached",
      severity: "warning",
      message: `${values.length - settings.maxPages} sitemap entries were skipped because the configured page limit was reached.`,
      url: target,
    });
    for (const value of values.slice(0, settings.maxPages)) {
      try {
        const url = await assertSafeURL(value, settings);
        if (websiteURLWithinScope(root, url)) result.push(canonicalize(url.toString()));
        else {
          skippedCount++;
          diagnostics.push({
            code: "website_sitemap_entry_skipped",
            severity: "warning",
            message: "A sitemap entry was skipped because it is outside the configured source path.",
            url: target,
          });
        }
      } catch {
        skippedCount++;
        diagnostics.push({
          code: "website_sitemap_entry_skipped",
          severity: "warning",
          message: "A malformed or unsafe sitemap entry was skipped.",
          url: target,
        });
      }
    }
    return { urls: result, skippedCount, diagnostics };
  } catch {
    return { urls: [], skippedCount: 0, diagnostics: [] };
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

export class IngestionRunError extends CrawlerJobError {
  readonly result: IngestionResult;

  constructor(code: string, message: string, result: IngestionResult, options?: ErrorOptions) {
    super(code, message, options);
    this.name = "IngestionRunError";
    this.result = result;
  }
}

function sortedDiagnostics(values: readonly IngestionDiagnostic[]): IngestionDiagnostic[] {
  return [...values].sort((left, right) => {
    const leftKey = `${left.code}\0${left.url ?? ""}\0${left.redirectedTo ?? ""}\0${left.message}`;
    const rightKey = `${right.code}\0${right.url ?? ""}\0${right.redirectedTo ?? ""}\0${right.message}`;
    return leftKey < rightKey ? -1 : leftKey > rightKey ? 1 : 0;
  });
}

async function ingestWebsite(job: Job, settings: CrawlerSettings): Promise<IngestionResult> {
  const root = await assertSafeURL(job.location, settings);
  const sitemap = await sitemapSeeds(root, settings);
  const seeds = [...new Set([canonicalize(root.toString()), ...sitemap.urls])].slice(0, settings.maxPages);
  const pages: PageRecord[] = [];
  const seenFinalURLs = new Set<string>();
  const observedURLs = new Set(seeds);
  const acceptedURLs = new Set(seeds);
  const failedURLs = new Set<string>();
  const redirectedURLs = new Set<string>();
  const diagnostics: IngestionDiagnostic[] = sitemap.diagnostics.slice(0, 200);
  let skippedCount = sitemap.skippedCount;
  let omittedDiagnostics = Math.max(0, sitemap.diagnostics.length - diagnostics.length);
  let pendingCount = 0;
  const addDiagnostic = (diagnostic: IngestionDiagnostic) => {
    if (diagnostics.length < 200) diagnostics.push(diagnostic);
    else omittedDiagnostics++;
  };
  const configuration = new Configuration({ persistStorage: false });
  const queue = await RequestQueue.open(`crawl-${job.id}`, { config: configuration });
  const crawler = new CheerioCrawler({
    httpClient: new PinnedCrawlerHttpClient(settings, root.origin, {}, (candidate) => websiteURLWithinScope(root, candidate)),
    requestQueue: queue,
    maxRequestsPerCrawl: settings.maxPages,
    maxRequestRetries: 1,
    requestHandlerTimeoutSecs: 30,
    navigationTimeoutSecs: 20,
    requestHandler: async ({ request, $, response, body, enqueueLinks }) => {
      const status = response?.statusCode ?? 0;
      if (status < 200 || status >= 400) {
        throw new CrawlerJobError("source_http_error", `The website page returned HTTP ${status || "unknown"}.`);
      }
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
      if (!websiteURLWithinScope(root, final)) throw new CrawlerJobError("source_redirect_not_allowed", "Website crawls may not navigate outside the configured source path.");
      const requestedURL = canonicalize(request.url);
      if (requestedURL !== finalURL && !redirectedURLs.has(requestedURL)) {
        redirectedURLs.add(requestedURL);
        addDiagnostic({
          code: "website_page_redirected",
          severity: "info",
          message: "A discovered website page redirected within the configured source path.",
          url: requestedURL,
          redirectedTo: finalURL,
        });
      }
      if (seenFinalURLs.has(finalURL)) {
        skippedCount++;
        addDiagnostic({
          code: "website_duplicate_page_skipped",
          severity: "warning",
          message: "A discovered URL resolved to content already acquired through another URL.",
          url: requestedURL,
          redirectedTo: finalURL,
        });
        return;
      }
      seenFinalURLs.add(finalURL);

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
            const candidateURL = canonicalize(candidate.url);
            if (!websiteURLWithinScope(root, new URL(candidateURL))) {
              skippedCount++;
              addDiagnostic({
                code: "website_link_outside_scope_skipped",
                severity: "info",
                message: "A discovered page was skipped because it is outside the configured source path.",
                url: candidateURL,
              });
              return null;
            }
            if (observedURLs.has(candidateURL)) return { ...candidate, url: candidateURL };
            observedURLs.add(candidateURL);
            if (acceptedURLs.size >= settings.maxPages) {
              skippedCount++;
              addDiagnostic({
                code: "website_page_limit_reached",
                severity: "warning",
                message: "A discovered page was skipped because the configured page limit was reached.",
                url: candidateURL,
              });
              return null;
            }
            acceptedURLs.add(candidateURL);
            return { ...candidate, url: candidateURL };
          } catch {
            skippedCount++;
            addDiagnostic({
              code: "website_link_skipped",
              severity: "warning",
              message: "A malformed website link was skipped during discovery.",
            });
            return null;
          }
        },
      });
    },
    failedRequestHandler: async ({ request }, error) => {
      let failedURL = request.url;
      try {
        failedURL = canonicalize(request.url);
      } catch {
        // Keep Crawlee's bounded request URL for the operational log only.
      }
      if (!failedURLs.has(failedURL)) {
        failedURLs.add(failedURL);
        addDiagnostic({
          code: error instanceof CrawlerJobError ? error.code : "website_page_failed",
          severity: "error",
          message: "The crawler could not retrieve this discovered page after its bounded retries.",
          ...(failedURL.startsWith("http://") || failedURL.startsWith("https://") ? { url: failedURL } : {}),
        });
      }
      log.warning(`crawl failed for ${request.url}: ${error.message}`);
    },
  }, configuration);

  let crawlError: unknown;
  try {
    await crawler.run(seeds);
  } catch (error) {
    crawlError = error;
  } finally {
    try {
      pendingCount = (await queue.getInfo())?.pendingRequestCount ?? 0;
    } catch {
      // Queue accounting failure is represented by the fatal crawl error when applicable.
    }
    await queue.drop();
  }
  if (pendingCount > 0) {
    skippedCount += pendingCount;
    addDiagnostic({
      code: "website_page_limit_reached",
      severity: "warning",
      message: `${pendingCount} discovered pages remained pending when the configured crawl limit was reached.`,
    });
  }
  if (omittedDiagnostics > 0) diagnostics.push({
    code: "website_diagnostics_truncated",
    severity: "warning",
    message: `${omittedDiagnostics} additional website diagnostics were omitted from the bounded run record.`,
  });
  const failedCount = failedURLs.size;
  if (failedCount > 0 || skippedCount > 0) diagnostics.push({
    code: "website_partial_coverage",
    severity: "warning",
    message: `The website run is partial: ${failedCount} pages failed and ${skippedCount} pages were skipped.`,
  });
  const result: IngestionResult = {
    pages,
    discoveredCount: Math.max(observedURLs.size + sitemap.skippedCount, pages.length + failedCount + skippedCount),
    failedCount,
    skippedCount,
    redirectedCount: redirectedURLs.size,
    diagnostics: sortedDiagnostics(diagnostics),
  };
  if (crawlError) {
    const failure = jobFailure(crawlError);
    throw new IngestionRunError(failure.code, failure.message, result, { cause: crawlError });
  }
  if (pages.length === 0) {
    throw new IngestionRunError(
      "website_no_content",
      "The website crawl did not retrieve any reviewable pages. Check its URL, robots policy, and network availability.",
      result,
    );
  }
  return result;
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

function validLeaseOwner(value: string): string {
  const owner = value.trim();
  if (!owner) throw new CrawlerJobError("crawler_configuration_invalid", "The crawler lease owner must not be empty.");
  return owner.slice(0, 255);
}

export async function claimJob(
  pool: pg.Pool,
  leaseOwner: string = crawlerLeaseOwner(),
  leaseDurationMilliseconds = LEASE_DURATION_MILLISECONDS,
): Promise<Job | null> {
  const owner = validLeaseOwner(leaseOwner);
  if (!Number.isSafeInteger(leaseDurationMilliseconds) || leaseDurationMilliseconds < 1_000) {
    throw new CrawlerJobError("crawler_configuration_invalid", "The crawler lease duration must be at least one second.");
  }
  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    const result = await client.query<Job>(`
      WITH candidate AS (
        SELECT id, state
        FROM crawl_jobs
        WHERE state = 'queued'
           OR (state = 'running' AND (lease_expires_at IS NULL OR lease_expires_at <= now()))
        ORDER BY queued_at, id
        FOR UPDATE SKIP LOCKED
        LIMIT 1
      ), claimed AS (
        UPDATE crawl_jobs AS job
        SET state = 'running',
            attempt = CASE WHEN candidate.state = 'running' THEN job.attempt + 1 ELSE job.attempt END,
            discovered_count = 0,
            fetched_count = 0,
            changed_count = 0,
            failed_count = 0,
            skipped_count = 0,
            redirected_count = 0,
            diagnostics = '{}'::jsonb,
            lease_owner = $1,
            lease_expires_at = now() + ($2::integer * interval '1 millisecond'),
            heartbeat_at = now(),
            started_at = now(),
            finished_at = null,
            error_code = null,
            error_message = null
        FROM candidate
        WHERE job.id = candidate.id
        RETURNING job.id, job.organisation_id, job.product_id, job.source_id,
                  job.attempt, job.lease_owner, job.lease_expires_at, job.heartbeat_at,
                  candidate.state AS previous_state
      ), cleared_previous_attempt AS (
        DELETE FROM crawl_job_documents AS link
        USING claimed
        WHERE claimed.previous_state = 'running'
          AND link.crawl_job_id = claimed.id
        RETURNING link.crawl_job_id
      )
      SELECT c.id::text, c.organisation_id::text, c.product_id::text, c.source_id::text,
             s.name AS source_name, s.kind AS source_kind, s.location, s.visibility::text,
             c.attempt, c.lease_owner, c.lease_expires_at, c.heartbeat_at
      FROM claimed c
      JOIN sources s ON s.id = c.source_id
      LEFT JOIN (SELECT count(*) AS cleared_count FROM cleared_previous_attempt) cleared ON true`,
    [owner, leaseDurationMilliseconds]);
    await client.query("COMMIT");
    return result.rows[0] ?? null;
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
}

export async function heartbeatJob(
  pool: pg.Pool,
  job: Job,
  leaseDurationMilliseconds = LEASE_DURATION_MILLISECONDS,
): Promise<boolean> {
  if (!Number.isSafeInteger(leaseDurationMilliseconds) || leaseDurationMilliseconds < 1_000) {
    throw new CrawlerJobError("crawler_configuration_invalid", "The crawler lease duration must be at least one second.");
  }
  const result = await pool.query(`
    UPDATE crawl_jobs
    SET heartbeat_at = now(),
        lease_expires_at = now() + ($3::integer * interval '1 millisecond')
    WHERE id = $1
      AND state = 'running'
      AND lease_owner = $2
      AND lease_expires_at > now()`,
  [job.id, job.lease_owner, leaseDurationMilliseconds]);
  return result.rowCount === 1;
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

async function lockJobLease(client: pg.PoolClient, job: Job): Promise<void> {
  const lease = await client.query<{ id: string }>(`
    SELECT id::text
    FROM crawl_jobs
    WHERE id = $1
      AND state = 'running'
      AND lease_owner = $2
      AND lease_expires_at > now()
    FOR UPDATE`,
  [job.id, job.lease_owner]);
  if (!lease.rows[0]) {
    throw new CrawlerJobError("crawler_lease_lost", "The crawler no longer owns this job lease; its results were not committed.");
  }
}

export async function storePageWithReference(
  pool: pg.Pool,
  job: Job,
  page: PageRecord,
  settings: CrawlerSettings,
): Promise<{ changed: boolean; reference: LegacyDocumentReference }> {
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
    // Reclaimed workers must be excluded before they can mutate assessment
    // evidence. Completion follows the same job -> source lock order.
    await lockJobLease(client, job);
    // Reassessment and publication serialize on the source row before touching
    // snapshots/documents: job -> source -> snapshot/document -> generation link.
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
    return { changed, reference: { knowledgeDocumentId: documentID, snapshotId: resolvedSnapshot.id } };
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
}

export async function storePage(pool: pg.Pool, job: Job, page: PageRecord, settings: CrawlerSettings): Promise<boolean> {
  return (await storePageWithReference(pool, job, page, settings)).changed;
}

export async function completeJob(
  pool: pg.Pool,
  job: Job,
  result: IngestionResult,
  changed: number,
): Promise<void> {
  const incomplete = result.failedCount > 0 || result.skippedCount > 0;
  const quarantined = incomplete || result.pages.some((page) => page.indicators.length > 0);
  const diagnostics = JSON.stringify({
    partial: incomplete,
    counts: {
      discovered: result.discoveredCount,
      fetched: result.pages.length,
      failed: result.failedCount,
      skipped: result.skippedCount,
      redirected: result.redirectedCount,
    },
    items: sortedDiagnostics(result.diagnostics),
  });
  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    await lockJobLease(client, job);
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
    const completed = await client.query(`
      UPDATE crawl_jobs
      SET state = 'review',
          discovered_count = $2,
          fetched_count = $3,
          changed_count = $4,
          failed_count = $5,
          skipped_count = $6,
          redirected_count = $7,
          diagnostics = $8::jsonb,
          error_code = null,
          error_message = null,
          lease_owner = '',
          lease_expires_at = null,
          finished_at = now()
      WHERE id = $1
        AND state = 'running'
        AND lease_owner = $9
        AND lease_expires_at > now()`,
    [
      job.id,
      result.discoveredCount,
      result.pages.length,
      changed,
      result.failedCount,
      result.skippedCount,
      result.redirectedCount,
      diagnostics,
      job.lease_owner,
    ]);
    if (completed.rowCount !== 1) {
      throw new CrawlerJobError("crawler_lease_lost", "The crawler no longer owns this job lease; completion was rolled back.");
    }
    await client.query("COMMIT");
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
}

function leaseLostError(): CrawlerJobError {
  return new CrawlerJobError("crawler_lease_lost", "The crawler no longer owns this job lease; processing stopped without committing stale results.");
}

type LeaseHeartbeat = {
  assertOwned: () => void;
  stop: () => Promise<void>;
};

function startLeaseHeartbeat(
  pool: pg.Pool,
  job: Job,
  intervalMilliseconds = HEARTBEAT_INTERVAL_MILLISECONDS,
): LeaseHeartbeat {
  let heartbeatFailure: unknown;
  let stopped = false;
  let inFlight = Promise.resolve();
  const pulse = () => {
    if (stopped || heartbeatFailure) return;
    inFlight = inFlight.then(async () => {
      if (!(await heartbeatJob(pool, job))) heartbeatFailure = leaseLostError();
    }).catch((error: unknown) => {
      heartbeatFailure = error;
    });
  };
  const timer = setInterval(pulse, intervalMilliseconds);
  timer.unref();
  return {
    assertOwned: () => {
      if (heartbeatFailure) throw heartbeatFailure;
    },
    stop: async () => {
      stopped = true;
      clearInterval(timer);
      await inFlight;
    },
  };
}

export async function processJob(
  pool: pg.Pool,
  job: Job,
  settings: CrawlerSettings = loadCrawlerSettings(),
  assertLease: () => void = () => undefined,
): Promise<void> {
  const result = await collectSource(job, settings);
  assertLease();
  if (!(await heartbeatJob(pool, job))) throw leaseLostError();
  let changed = 0;
  const legacyDocuments = new Map<string, LegacyDocumentReference>();
  for (const page of result.pages) {
    assertLease();
    const stored = await storePageWithReference(pool, job, page, settings);
    if (stored.changed) changed++;
    legacyDocuments.set(page.url, stored.reference);
  }
  assertLease();
  if (!(await heartbeatJob(pool, job))) throw leaseLostError();
  const candidate = await persistDeveloperAssetCandidates(pool, job, result, legacyDocuments);
  const completedResult = candidate.diagnostics.length > 0
    ? { ...result, diagnostics: sortedDiagnostics([...result.diagnostics, ...candidate.diagnostics]) }
    : result;
  assertLease();
  await completeJob(pool, job, completedResult, changed);
}

export function jobFailure(error: unknown): { code: string; message: string } {
  if (error instanceof CrawlerJobError) return { code: error.code, message: error.message.slice(0, 1000) };
  return {
    code: "crawler_internal_error",
    message: "The ingestion worker failed unexpectedly. Review the worker logs and retry the job.",
  };
}

function failedIngestionResult(error: unknown): IngestionResult {
  if (error instanceof IngestionRunError) return error.result;
  return {
    pages: [],
    discoveredCount: 0,
    failedCount: 1,
    skippedCount: 0,
    redirectedCount: 0,
    diagnostics: [],
  };
}

export async function failJob(pool: pg.Pool, job: Job, error: unknown): Promise<boolean> {
  const failure = jobFailure(error);
  const ingestion = failedIngestionResult(error);
  const diagnostics = sortedDiagnostics([
    ...ingestion.diagnostics,
    {
      code: failure.code,
      severity: "error",
      message: failure.message,
    },
  ]);
  const storedDiagnostics = JSON.stringify({
    partial: true,
    counts: {
      discovered: ingestion.discoveredCount,
      fetched: ingestion.pages.length,
      failed: Math.max(1, ingestion.failedCount),
      skipped: ingestion.skippedCount,
      redirected: ingestion.redirectedCount,
    },
    items: diagnostics,
  });
  const updated = await pool.query(`
    UPDATE crawl_jobs
    SET state = 'failed',
        discovered_count = $2,
        fetched_count = $3,
        failed_count = $4,
        skipped_count = $5,
        redirected_count = $6,
        diagnostics = $7::jsonb,
        error_code = $8,
        error_message = $9,
        lease_owner = '',
        lease_expires_at = null,
        finished_at = now()
    WHERE id = $1
      AND state = 'running'
      AND lease_owner = $10
      AND lease_expires_at > now()`,
  [
    job.id,
    ingestion.discoveredCount,
    ingestion.pages.length,
    Math.max(1, ingestion.failedCount),
    ingestion.skippedCount,
    ingestion.redirectedCount,
    storedDiagnostics,
    failure.code,
    failure.message,
    job.lease_owner,
  ]);
  return updated.rowCount === 1;
}

export async function runOnce(
  pool: pg.Pool,
  settings: CrawlerSettings = loadCrawlerSettings(),
  leaseOwner: string = crawlerLeaseOwner(),
): Promise<boolean> {
  const job = await claimJob(pool, leaseOwner);
  if (!job) return false;
  const heartbeat = startLeaseHeartbeat(pool, job);
  let processingError: unknown;
  try {
    await processJob(pool, job, settings, heartbeat.assertOwned);
  } catch (error) {
    processingError = error;
  } finally {
    await heartbeat.stop();
  }
  if (processingError) {
    const recorded = await failJob(pool, job, processingError);
    if (recorded) log.exception(processingError as Error, `crawl job ${job.id} failed`);
    else log.warning(`crawl job ${job.id} stopped after losing its lease; another worker owns recovery`);
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
