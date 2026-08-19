import { createHash, randomUUID } from "node:crypto";
import { lookup } from "node:dns/promises";
import { mkdir, writeFile } from "node:fs/promises";
import { isIP } from "node:net";
import path from "node:path";
import process from "node:process";
import { CheerioCrawler, Configuration, RequestQueue, log } from "crawlee";
import { chromium } from "playwright";
import pg from "pg";

const { Pool } = pg;
const MAX_PAGES = Number(process.env.DOKOSOKO_CRAWLER_MAX_PAGES ?? 500);
const MAX_BYTES = Number(process.env.DOKOSOKO_CRAWLER_MAX_BYTES ?? 5_000_000);
const DATA_DIR = process.env.DOKOSOKO_DATA_DIR ?? "/data";

type Job = {
  id: string;
  organisation_id: string;
  product_id: string;
  source_id: string;
  source_name: string;
  source_kind: string;
  location: string;
  visibility: "private" | "public";
};

type PageRecord = {
  url: string;
  title: string;
  text: string;
  html: string;
  contentType: string;
  status: number;
  indicators: string[];
  rendered: boolean;
};

const deniedV4 = [
  ["0.0.0.0", 8], ["10.0.0.0", 8], ["100.64.0.0", 10], ["127.0.0.0", 8],
  ["169.254.0.0", 16], ["172.16.0.0", 12], ["192.0.0.0", 24], ["192.0.2.0", 24],
  ["192.168.0.0", 16], ["198.18.0.0", 15], ["198.51.100.0", 24], ["203.0.113.0", 24],
  ["224.0.0.0", 4], ["240.0.0.0", 4],
] as const;

function ipv4Number(value: string): number {
  return value.split(".").reduce((total, part) => (total << 8) + Number(part), 0) >>> 0;
}

export function isDeniedAddress(value: string): boolean {
  const normalized = value.toLowerCase().split("%")[0];
  if (isIP(normalized) === 4) {
    const address = ipv4Number(normalized);
    return deniedV4.some(([network, prefix]) => {
      const mask = (0xffffffff << (32 - prefix)) >>> 0;
      return (address & mask) === (ipv4Number(network) & mask);
    });
  }
  if (isIP(normalized) === 6) {
    return normalized === "::" || normalized === "::1" || normalized.startsWith("fc") || normalized.startsWith("fd") || normalized.startsWith("fe8") || normalized.startsWith("fe9") || normalized.startsWith("fea") || normalized.startsWith("feb") || normalized.startsWith("ff") || normalized.startsWith("2001:db8:") || normalized.startsWith("::ffff:");
  }
  return true;
}

export async function assertSafeURL(value: string): Promise<URL> {
  const url = new URL(value);
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || !url.hostname) {
    throw new Error("crawler URL must be credential-free HTTP(S)");
  }
  if (url.port && !["80", "443"].includes(url.port)) {
    throw new Error("crawler URL uses a disallowed port");
  }
  const addresses = await lookup(url.hostname, { all: true, verbatim: true });
  if (addresses.length === 0 || addresses.some((entry) => isDeniedAddress(entry.address))) {
    throw new Error("crawler URL resolves to a private or reserved network");
  }
  return url;
}

export function canonicalize(value: string): string {
  const url = new URL(value);
  url.hash = "";
  url.hostname = url.hostname.toLowerCase();
  if ((url.protocol === "https:" && url.port === "443") || (url.protocol === "http:" && url.port === "80")) url.port = "";
  for (const key of [...url.searchParams.keys()]) {
    if (/^(utm_|fbclid|gclid)/i.test(key)) url.searchParams.delete(key);
  }
  url.searchParams.sort();
  if (url.pathname !== "/") url.pathname = url.pathname.replace(/\/+$/, "");
  return url.toString();
}

export function injectionIndicators(text: string, html = ""): string[] {
  const combined = `${text}\n${html}`.toLowerCase();
  const patterns: Array<[string, RegExp]> = [
    ["instruction_override", /ignore (all |any )?(previous|prior|system|developer) instructions/],
    ["prompt_exfiltration", /(reveal|print|return|expose).{0,40}(system prompt|secret|api key|access token)/],
    ["tool_coercion", /(call|invoke|execute).{0,30}(tool|function|shell|terminal)/],
    ["role_impersonation", /(^|\W)(system|assistant|developer)\s*:/],
    ["hidden_content", /(display\s*:\s*none|visibility\s*:\s*hidden|font-size\s*:\s*0)/],
    ["encoded_payload", /(base64|decode this|rot13).{0,30}(instruction|prompt|secret)/],
  ];
  return patterns.filter(([, pattern]) => pattern.test(combined)).map(([name]) => name);
}

async function secureFetch(value: string, redirects = 0): Promise<Response> {
  const url = await assertSafeURL(value);
  const response = await fetch(url, { redirect: "manual", signal: AbortSignal.timeout(15_000), headers: { "User-Agent": "DokoSokoCrawler/2.0", Accept: "text/html,application/xml,text/plain;q=0.8" } });
  if (response.status >= 300 && response.status < 400) {
    if (redirects >= 5) throw new Error("crawler redirect limit exceeded");
    const location = response.headers.get("location");
    if (!location) throw new Error("crawler redirect omitted Location");
    return secureFetch(new URL(location, url).toString(), redirects + 1);
  }
  return response;
}

async function sitemapSeeds(root: URL): Promise<string[]> {
  const target = new URL("/sitemap.xml", root).toString();
  try {
    const response = await secureFetch(target);
    if (!response.ok) return [];
    const xml = (await response.text()).slice(0, MAX_BYTES);
    const values = [...xml.matchAll(/<loc>\s*([^<]+?)\s*<\/loc>/gi)].map((match) => match[1].replaceAll("&amp;", "&"));
    const result: string[] = [];
    for (const value of values.slice(0, MAX_PAGES)) {
      const url = await assertSafeURL(value);
      if (url.origin === root.origin) result.push(canonicalize(url.toString()));
    }
    return result;
  } catch {
    return [];
  }
}

async function renderedPage(value: string): Promise<Pick<PageRecord, "html" | "text" | "title"> | null> {
  let browser;
  try {
    await assertSafeURL(value);
    browser = await chromium.launch({ headless: true, args: ["--disable-dev-shm-usage", "--no-sandbox"] });
    const context = await browser.newContext({ serviceWorkers: "block", javaScriptEnabled: true });
    const page = await context.newPage();
    await page.route("**/*", async (route) => {
      try {
        const requestURL = new URL(route.request().url());
        if (!["http:", "https:"].includes(requestURL.protocol)) return route.abort();
        await assertSafeURL(requestURL.toString());
        return route.continue();
      } catch {
        return route.abort();
      }
    });
    await page.goto(value, { waitUntil: "networkidle", timeout: 20_000 });
    const result = { html: (await page.content()).slice(0, MAX_BYTES), text: (await page.locator("body").innerText()).slice(0, MAX_BYTES), title: await page.title() };
    await context.close();
    return result;
  } catch {
    return null;
  } finally {
    await browser?.close();
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
        UPDATE crawl_jobs SET state = 'running', started_at = now()
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

async function storePage(pool: pg.Pool, job: Job, page: PageRecord): Promise<boolean> {
  const digest = createHash("sha256").update(page.html).digest();
  const hex = digest.toString("hex");
  const directory = path.join(DATA_DIR, "snapshots", job.source_id);
  await mkdir(directory, { recursive: true, mode: 0o700 });
  const objectPath = path.join(directory, `${hex}.json`);
  await writeFile(objectPath, JSON.stringify({ url: page.url, fetched_at: new Date().toISOString(), rendered: page.rendered, html: page.html }), { mode: 0o600, flag: "wx" }).catch((error: NodeJS.ErrnoException) => { if (error.code !== "EEXIST") throw error; });

  const snapshotID = randomUUID();
  const snapshot = await pool.query<{ id: string }>(`
    INSERT INTO source_snapshots(id, organisation_id, product_id, source_id, crawl_job_id, canonical_url, object_key, content_sha256, content_type, response_status, trust_indicators)
    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
    ON CONFLICT (source_id, canonical_url, content_sha256) DO NOTHING RETURNING id::text`,
  [snapshotID, job.organisation_id, job.product_id, job.source_id, job.id, page.url, path.relative(DATA_DIR, objectPath), digest, page.contentType, page.status, JSON.stringify({ rendered: page.rendered })]);
  if (snapshot.rowCount === 0) return false;
  const state = page.indicators.length > 0 ? "quarantined" : "validated";
  await pool.query(`
    INSERT INTO knowledge_documents(id, organisation_id, product_id, source_id, snapshot_id, title, canonical_url, body, visibility, state, trust_level, injection_indicators)
    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
  [randomUUID(), job.organisation_id, job.product_id, job.source_id, snapshotID, page.title || page.url, page.url, page.text, job.visibility, state, page.indicators.length ? 10 : 70, JSON.stringify(page.indicators)]);
  return true;
}

async function processJob(pool: pg.Pool, job: Job): Promise<void> {
  const root = await assertSafeURL(job.location);
  const seeds = [...new Set([canonicalize(root.toString()), ...(await sitemapSeeds(root))])].slice(0, MAX_PAGES);
  const pages: PageRecord[] = [];
  const queue = await RequestQueue.open(`crawl-${job.id}`);
  const configuration = new Configuration({ persistStorage: false });
  const crawler = new CheerioCrawler({
    requestQueue: queue,
    maxRequestsPerCrawl: MAX_PAGES,
    maxRequestRetries: 1,
    requestHandlerTimeoutSecs: 30,
    preNavigationHooks: [async ({ request }, options) => {
      await assertSafeURL(request.url);
      options.followRedirect = false;
    }],
    requestHandler: async ({ request, $, response, enqueueLinks }) => {
      const contentLength = Number(response?.headers["content-length"] ?? 0);
      if (contentLength > MAX_BYTES) throw new Error("crawler response exceeds configured size limit");
      const finalURL = canonicalize(request.loadedUrl ?? request.url);
      await assertSafeURL(finalURL);
      const hidden = $("[hidden], [aria-hidden='true'], [style*='display:none'], [style*='visibility:hidden']").text();
      $("script,style,noscript,svg,nav,footer").remove();
      let text = ($("main,article").first().text() || $("body").text()).replace(/\s+/g, " ").trim().slice(0, MAX_BYTES);
      let html = $.html().slice(0, MAX_BYTES);
      let title = $("title").text().trim() || $("h1").first().text().trim();
      let rendered = false;
      if (text.length < 200 && html.includes("<script")) {
        const browserResult = await renderedPage(finalURL);
        if (browserResult) {
          ({ text, html, title } = browserResult);
          rendered = true;
        }
      }
      const indicators = injectionIndicators(`${text}\n${hidden}`, html);
      pages.push({ url: finalURL, title, text, html, contentType: String(response?.headers["content-type"] ?? "text/html"), status: response?.statusCode ?? 200, indicators, rendered });
      await enqueueLinks({ strategy: "same-origin", transformRequestFunction: (candidate) => ({ ...candidate, url: canonicalize(candidate.url) }) });
    },
    failedRequestHandler: async ({ request }, error) => log.warning(`crawl failed for ${request.url}: ${error.message}`),
  }, configuration);
  await crawler.run(seeds);
  await queue.drop();

  let changed = 0;
  let quarantined = false;
  for (const page of pages) {
    quarantined ||= page.indicators.length > 0;
    if (await storePage(pool, job, page)) changed++;
  }
  await pool.query(`UPDATE sources SET state = $2, revision = revision + 1, updated_at = now() WHERE id = $1`, [job.source_id, quarantined ? "quarantined" : "validated"]);
  await pool.query(`UPDATE crawl_jobs SET state = 'review', discovered_count = $2, fetched_count = $3, changed_count = $4, finished_at = now() WHERE id = $1`, [job.id, seeds.length, pages.length, changed]);
}

async function failJob(pool: pg.Pool, job: Job, error: unknown): Promise<void> {
  const message = error instanceof Error ? error.message : String(error);
  await pool.query(`UPDATE crawl_jobs SET state = 'failed', error_code = 'crawl_failed', error_message = $2, finished_at = now() WHERE id = $1`, [job.id, message.slice(0, 1000)]);
}

export async function runOnce(pool: pg.Pool): Promise<boolean> {
  const job = await claim(pool);
  if (!job) return false;
  try {
    await processJob(pool, job);
  } catch (error) {
    await failJob(pool, job, error);
    log.exception(error as Error, `crawl job ${job.id} failed`);
  }
  return true;
}

async function main() {
  const connectionString = process.env.DOKOSOKO_DATABASE_URL;
  if (!connectionString) throw new Error("DOKOSOKO_DATABASE_URL is required");
  const pool = new Pool({ connectionString, max: 4, statement_timeout: 30_000, application_name: "dokosoko-crawler" });
  try {
    if (process.argv.includes("--once")) {
      await runOnce(pool);
      return;
    }
    for (;;) {
      const worked = await runOnce(pool);
      if (!worked) await new Promise((resolve) => setTimeout(resolve, 2_000));
    }
  } finally {
    await pool.end();
  }
}

if (import.meta.url === new URL(process.argv[1], "file:").href) {
  main().catch((error) => { log.exception(error as Error, "crawler stopped"); process.exitCode = 1; });
}
