import { constants } from "node:fs";
import { open, realpath } from "node:fs/promises";
import { request as httpRequest, type IncomingHttpHeaders, type OutgoingHttpHeaders } from "node:http";
import { request as httpsRequest } from "node:https";
import { isIP } from "node:net";
import path from "node:path";
import { Readable } from "node:stream";
import type { CrawlerSettings } from "./security";
import {
  type AddressResolver,
  canonicalize,
  createPinnedLookup,
  CrawlerJobError,
  resolveSafeURL,
  type SafeURLResolution,
} from "./security";

export type Job = {
  id: string;
  organisation_id: string;
  product_id: string;
  source_id: string;
  source_name: string;
  source_kind: string;
  location: string;
  visibility: "private" | "public";
  attempt: number;
  lease_owner: string;
  lease_expires_at: Date;
  heartbeat_at: Date;
};

export type PageRecord = {
  url: string;
  title: string;
  text: string;
  html: string;
  contentType: string;
  status: number;
  indicators: string[];
  rendered: boolean;
};

export type IngestionResult = {
  pages: PageRecord[];
  discoveredCount: number;
  failedCount: number;
  skippedCount: number;
  redirectedCount: number;
  diagnostics: IngestionDiagnostic[];
};

export type IngestionDiagnostic = {
  code: string;
  severity: "info" | "warning" | "error";
  message: string;
  url?: string;
  redirectedTo?: string;
};

export function singlePageIngestionResult(page: PageRecord): IngestionResult {
  return {
    pages: [page],
    discoveredCount: 1,
    failedCount: 0,
    skippedCount: 0,
    redirectedCount: 0,
    diagnostics: [],
  };
}

export function injectionIndicators(text: string, html = ""): string[] {
  const combined = `${text}\n${html}`.toLowerCase();
  const patterns: Array<[string, RegExp]> = [
    ["instruction_override", /ignore (all |any )?(previous|prior|system|developer) instructions/],
    ["prompt_exfiltration", /(^|[.!?]\s+|\b(?:please|now|immediately|then|and|keep|start|begin|continue)\s+)(?:reveal|print|return|expose|revealing|printing|returning|exposing)\b.{0,40}\b(system prompt|secret|api key|access token)\b/m],
    ["tool_coercion", /(call|invoke|execute).{0,30}(tool|function|shell|terminal)/],
    ["role_impersonation", /(^|\W)(system|assistant|developer)\s*:/],
    ["hidden_content", /(display\s*:\s*none|visibility\s*:\s*hidden|font-size\s*:\s*0)/],
    ["encoded_payload", /(base64|decode this|rot13).{0,30}(instruction|prompt|secret)/],
  ];
  return patterns.filter(([, pattern]) => pattern.test(combined)).map(([name]) => name);
}

export async function boundedResponseBytes(response: Response, maxBytes: number): Promise<Uint8Array> {
  const declaredLength = Number(response.headers.get("content-length") ?? 0);
  if (Number.isFinite(declaredLength) && declaredLength > maxBytes) {
    throw new CrawlerJobError("source_too_large", `The source exceeds the configured ${maxBytes}-byte limit.`);
  }
  if (!response.body) return new Uint8Array();

  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    total += value.byteLength;
    if (total > maxBytes) {
      await reader.cancel();
      throw new CrawlerJobError("source_too_large", `The source exceeds the configured ${maxBytes}-byte limit.`);
    }
    chunks.push(value);
  }
  const bytes = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return bytes;
}

export async function boundedResponseText(response: Response, maxBytes: number): Promise<string> {
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(await boundedResponseBytes(response, maxBytes));
  } catch (error) {
    if (error instanceof CrawlerJobError) throw error;
    throw new CrawlerJobError("source_encoding_unsupported", "Documentation sources must use valid UTF-8 text.", { cause: error });
  }
}

export type SecureFetchHeaders = Readonly<Record<string, string | readonly string[] | undefined>>;

export type SecureFetchTransport = (
  resolution: SafeURLResolution,
  headers: SecureFetchHeaders,
  signal: AbortSignal,
) => Promise<Response>;

export type SecureFetchOptions = {
  resolver?: AddressResolver;
  transport?: SecureFetchTransport;
  headers?: SecureFetchHeaders;
  redirectPolicy?: (from: URL, to: URL) => void | Promise<void>;
};

function requestHeaders(url: URL, values: SecureFetchHeaders): OutgoingHttpHeaders {
  const headers: OutgoingHttpHeaders = {};
  for (const [name, value] of Object.entries(values)) {
    if (value === undefined || /^(?:host|connection|proxy-authorization|transfer-encoding|content-length|accept-encoding)$/i.test(name)) continue;
    headers[name] = typeof value === "string" ? value : [...value];
  }
  headers.Host = url.host;
  headers["Accept-Encoding"] = "identity";
  return headers;
}

function webHeaders(values: IncomingHttpHeaders, rawHeaders: readonly string[]): Headers {
  const headers = new Headers();
  for (let index = 0; index < rawHeaders.length; index += 2) {
    headers.append(rawHeaders[index], rawHeaders[index + 1]);
  }
  if (rawHeaders.length === 0) {
    for (const [name, value] of Object.entries(values)) {
      if (Array.isArray(value)) value.forEach((item) => headers.append(name, item));
      else if (value !== undefined) headers.set(name, value);
    }
  }
  return headers;
}

export const pinnedHTTPTransport: SecureFetchTransport = async (resolution, headers, signal) => new Promise<Response>((resolve, reject) => {
  const request = resolution.url.protocol === "https:" ? httpsRequest : httpRequest;
  const options = {
    method: "GET",
    agent: false,
    headers: requestHeaders(resolution.url, headers),
    lookup: createPinnedLookup(resolution),
    signal,
    ...(resolution.url.protocol === "https:" && isIP(resolution.hostname) === 0 ? { servername: resolution.hostname } : {}),
  };
  const outgoing = request(resolution.url, options, (incoming) => {
    const status = incoming.statusCode ?? 0;
    if (status < 200 || status > 599) {
      incoming.destroy();
      reject(new Error(`The source returned unsupported HTTP status ${status}.`));
      return;
    }
    const contentEncoding = incoming.headers["content-encoding"]?.trim().toLowerCase();
    if (contentEncoding && contentEncoding !== "identity") {
      incoming.destroy();
      reject(new Error(`The source ignored the identity encoding request and returned ${contentEncoding}.`));
      return;
    }
    const body = status === 204 || status === 304
      ? null
      : Readable.toWeb(incoming) as ReadableStream<Uint8Array>;
    resolve(new Response(body, {
      status,
      statusText: incoming.statusMessage,
      headers: webHeaders(incoming.headers, incoming.rawHeaders),
    }));
  });
  outgoing.once("error", reject);
  outgoing.end();
});

export async function secureFetch(
  value: string,
  settings: CrawlerSettings,
  accept: string,
  options: SecureFetchOptions = {},
  redirects = 0,
  redirectURLs: URL[] = [],
): Promise<{ response: Response; url: URL; redirectURLs: readonly URL[] }> {
  const resolution = await resolveSafeURL(value, settings, options.resolver);
  const { url } = resolution;
  let response: Response;
  try {
    response = await (options.transport ?? pinnedHTTPTransport)(resolution, {
      "User-Agent": "DokoSokoCrawler/3.0",
      Accept: accept,
      ...options.headers,
    }, AbortSignal.timeout(15_000));
  } catch (error) {
    if (error instanceof CrawlerJobError) throw error;
    throw new CrawlerJobError("source_fetch_failed", "The crawler could not fetch the source. Check DNS, TLS, and source availability.", { cause: error });
  }
  if ([301, 302, 303, 307, 308].includes(response.status)) {
    const location = response.headers.get("location");
    await response.body?.cancel();
    if (redirects >= 5) throw new CrawlerJobError("source_redirect_limit", "The source exceeded the five-redirect limit.");
    if (!location) throw new CrawlerJobError("source_redirect_invalid", "The source returned a redirect without a Location header.");
    let target: URL;
    try {
      target = new URL(location, url);
    } catch (error) {
      throw new CrawlerJobError("source_redirect_invalid", "The source returned an invalid redirect destination.", { cause: error });
    }
    await options.redirectPolicy?.(url, target);
    return secureFetch(target.toString(), settings, accept, options, redirects + 1, [...redirectURLs, target]);
  }
  return { response, url, redirectURLs };
}

type OpenAPIShape = {
  title: string;
  format: "json" | "yaml";
};

function scalar(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

export function validateOpenAPIDocument(document: string, fallbackTitle = "OpenAPI document"): OpenAPIShape {
  const value = document.trim();
  if (!value) {
    throw new CrawlerJobError("openapi_document_invalid", "The OpenAPI document is empty.");
  }
  if (value.startsWith("{") || value.startsWith("[")) {
    let parsed: unknown;
    try {
      parsed = JSON.parse(value);
    } catch (error) {
      throw new CrawlerJobError("openapi_document_invalid", "The OpenAPI JSON document is malformed.", { cause: error });
    }
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      throw new CrawlerJobError("openapi_document_invalid", "The OpenAPI document must be a JSON object.");
    }
    const object = parsed as Record<string, unknown>;
    const version = scalar(object.openapi) || scalar(object.swagger);
    const info = object.info;
    if (!version || !info || typeof info !== "object" || Array.isArray(info) || !object.paths || typeof object.paths !== "object" || Array.isArray(object.paths)) {
      throw new CrawlerJobError("openapi_document_invalid", "The document must contain OpenAPI/Swagger version, info, and paths objects.");
    }
    return { title: scalar((info as Record<string, unknown>).title) || fallbackTitle, format: "json" };
  }

  const hasVersion = /^(?:openapi|swagger)\s*:\s*[^\s#]+\s*(?:#.*)?$/m.test(value);
  const hasInfo = /^info\s*:\s*(?:\{.*\})?\s*(?:#.*)?$/m.test(value);
  const hasPaths = /^paths\s*:\s*(?:\{.*\})?\s*(?:#.*)?$/m.test(value);
  if (!hasVersion || !hasInfo || !hasPaths) {
    throw new CrawlerJobError("openapi_document_invalid", "The YAML document must contain top-level OpenAPI/Swagger version, info, and paths keys.");
  }
  const titleMatch = value.match(/^info\s*:\s*\n(?:^[ \t]+.*\n)*?^[ \t]+title\s*:\s*["']?([^\n#"']+)/m);
  return { title: titleMatch?.[1]?.trim() || fallbackTitle, format: "yaml" };
}

function mediaType(value: string | null): string {
  return (value ?? "").split(";", 1)[0].trim().toLowerCase();
}

function isOpenAPIMediaType(value: string, url: URL): boolean {
  if ([
    "application/json", "application/yaml", "application/x-yaml", "text/yaml", "text/x-yaml",
    "application/vnd.oai.openapi", "application/vnd.oai.openapi+json", "application/vnd.oai.openapi+yaml",
  ].includes(value)) return true;
  return value === "text/plain" && /\.(?:json|ya?ml)$/i.test(url.pathname);
}

export async function ingestOpenAPI(job: Job, settings: CrawlerSettings): Promise<IngestionResult> {
  const { response, url } = await secureFetch(
    job.location,
    settings,
    "application/vnd.oai.openapi+json,application/vnd.oai.openapi+yaml,application/json,application/yaml,text/yaml;q=0.9,text/plain;q=0.5",
  );
  if (!response.ok) {
    throw new CrawlerJobError("source_http_error", `The OpenAPI source returned HTTP ${response.status}.`);
  }
  const contentType = mediaType(response.headers.get("content-type"));
  if (!isOpenAPIMediaType(contentType, url)) {
    throw new CrawlerJobError("source_media_type_unsupported", `The OpenAPI source returned unsupported media type ${contentType || "unknown"}.`);
  }
  const document = await boundedResponseText(response, settings.maxBytes);
  const shape = validateOpenAPIDocument(document, job.source_name);
  const indicators = injectionIndicators(document);
  return singlePageIngestionResult({
    url: canonicalize(url.toString()),
    title: shape.title,
    text: document,
    html: document,
    contentType: contentType || (shape.format === "json" ? "application/json" : "application/yaml"),
    status: response.status,
    indicators,
    rendered: false,
  });
}

const uploadTypes = new Map<string, string>([
  [".md", "text/markdown"],
  [".mdx", "text/markdown"],
  [".txt", "text/plain"],
  [".html", "text/html"],
  [".htm", "text/html"],
  [".json", "application/json"],
  [".yaml", "application/yaml"],
  [".yml", "application/yaml"],
]);

function uploadText(document: string, contentType: string): string {
  if (contentType !== "text/html") return document;
  return document
    .replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, " ")
    .replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, " ")
    .replace(/<[^>]+>/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

function uploadTitle(document: string, contentType: string, fallback: string): string {
  if (contentType === "text/html") return document.match(/<title\b[^>]*>([^<]+)<\/title>/i)?.[1]?.trim() || fallback;
  if (contentType === "text/markdown") return document.match(/^#\s+(.+)$/m)?.[1]?.trim() || fallback;
  if (contentType === "application/json") {
    try {
      const value = JSON.parse(document) as unknown;
      if (value && typeof value === "object" && !Array.isArray(value)) return scalar((value as Record<string, unknown>).title) || fallback;
    } catch {
      return fallback;
    }
  }
  return fallback;
}

function containedPath(root: string, target: string): boolean {
  return target === root || target.startsWith(`${root}${path.sep}`);
}

export async function ingestUpload(job: Job, settings: CrawlerSettings): Promise<IngestionResult> {
  if (!settings.uploadDir) {
    throw new CrawlerJobError("upload_directory_unconfigured", "Upload ingestion requires a dedicated DOKOSOKO_UPLOAD_DIR.");
  }
  if (!job.location || job.location.includes("\0") || job.location.includes("\\") || path.isAbsolute(job.location)) {
    throw new CrawlerJobError("upload_path_not_allowed", "Upload locations must be relative paths inside DOKOSOKO_UPLOAD_DIR.");
  }
  const normalized = path.normalize(job.location);
  if (normalized === "." || normalized === ".." || normalized.startsWith(`..${path.sep}`)) {
    throw new CrawlerJobError("upload_path_not_allowed", "Upload locations may not escape DOKOSOKO_UPLOAD_DIR.");
  }

  let root: string;
  let target: string;
  let unresolved: string;
  try {
    root = await realpath(settings.uploadDir);
    unresolved = path.resolve(root, normalized);
    target = await realpath(unresolved);
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code;
    if (code === "ENOENT") throw new CrawlerJobError("upload_not_found", "The selected upload does not exist in DOKOSOKO_UPLOAD_DIR.", { cause: error });
    throw new CrawlerJobError("upload_unreadable", "The selected upload could not be read.", { cause: error });
  }
  if (!containedPath(root, target) || target !== unresolved) {
    throw new CrawlerJobError("upload_path_not_allowed", "Upload paths must be canonical, symlink-free, and strictly inside DOKOSOKO_UPLOAD_DIR.");
  }

  const contentType = uploadTypes.get(path.extname(target).toLowerCase());
  if (!contentType) {
    throw new CrawlerJobError("upload_media_type_unsupported", "Uploads must be UTF-8 Markdown, text, HTML, JSON, or YAML files.");
  }

  let handle;
  try {
    handle = await open(target, constants.O_RDONLY | constants.O_NOFOLLOW);
    const metadata = await handle.stat();
    if (!metadata.isFile()) throw new CrawlerJobError("upload_type_not_allowed", "The upload location must identify a regular file.");
    if (metadata.size > settings.maxBytes) throw new CrawlerJobError("source_too_large", `The upload exceeds the configured ${settings.maxBytes}-byte limit.`);
    const bytes = await handle.readFile();
    if (bytes.byteLength > settings.maxBytes) throw new CrawlerJobError("source_too_large", `The upload exceeds the configured ${settings.maxBytes}-byte limit.`);
    let document: string;
    try {
      document = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    } catch (error) {
      throw new CrawlerJobError("source_encoding_unsupported", "Uploads must use valid UTF-8 text.", { cause: error });
    }
    const relativePath = path.relative(root, target).split(path.sep).map(encodeURIComponent).join("/");
    const indicators = injectionIndicators(document);
    return singlePageIngestionResult({
      url: `upload://${job.source_id}/${relativePath}`,
      title: uploadTitle(document, contentType, path.basename(target)),
      text: uploadText(document, contentType),
      html: document,
      contentType,
      status: 200,
      indicators,
      rendered: false,
    });
  } catch (error) {
    if (error instanceof CrawlerJobError) throw error;
    throw new CrawlerJobError("upload_unreadable", "The selected upload could not be read.", { cause: error });
  } finally {
    await handle?.close();
  }
}

export function rejectGitSource(): never {
  throw new CrawlerJobError(
    "git_source_unsupported",
    "Git ingestion is not enabled in this worker. Use a website, an HTTPS OpenAPI document, or a reviewed upload; do not use git:// or repository credentials.",
  );
}
