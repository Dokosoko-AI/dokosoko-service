import { Readable, Transform } from "node:stream";
import type {
  BaseHttpClient,
  HttpRequest,
  HttpResponse,
  ResponseTypes,
  StreamingHttpResponse,
} from "crawlee";
import type { CrawlerSettings } from "./security";
import { CrawlerJobError } from "./security";
import {
  secureFetch,
  type SecureFetchHeaders,
  type SecureFetchOptions,
} from "./sources";

function responseHeaders(response: Response): Record<string, string | string[]> {
  const headers: Record<string, string | string[]> = Object.fromEntries(response.headers.entries());
  const cookies = response.headers.getSetCookie();
  if (cookies.length > 0) headers["set-cookie"] = cookies;
  return headers;
}

function boundedStream(response: Response, maxBytes: number): {
  stream: Readable;
  progress: { readonly percent: number; readonly transferred: number; readonly total?: number };
} {
  const declared = Number(response.headers.get("content-length") ?? 0);
  if (Number.isFinite(declared) && declared > maxBytes) {
    throw new CrawlerJobError("source_too_large", `The source exceeds the configured ${maxBytes}-byte limit.`);
  }

  let transferred = 0;
  const source = response.body
    ? Readable.fromWeb(response.body as unknown as import("node:stream/web").ReadableStream)
    : Readable.from([]);
  const limiter = new Transform({
    transform(chunk: Buffer, _encoding, callback) {
      transferred += chunk.byteLength;
      if (transferred > maxBytes) {
        callback(new CrawlerJobError("source_too_large", `The source exceeds the configured ${maxBytes}-byte limit.`));
        return;
      }
      callback(null, chunk);
    },
  });
  limiter.once("error", () => source.destroy());
  source.pipe(limiter);
  return {
    stream: limiter,
    progress: {
      get percent() { return declared > 0 ? Math.min(1, transferred / declared) : 0; },
      get transferred() { return transferred; },
      ...(declared > 0 ? { total: declared } : {}),
    },
  };
}

function outboundHeaders(request: HttpRequest): SecureFetchHeaders {
  return request.headers ?? {};
}

/**
 * Crawlee HTTP client backed by the same DNS-pinned transport used for direct
 * OpenAPI and sitemap fetches. Crawlee never opens its own unvetted socket.
 */
export class PinnedCrawlerHttpClient implements BaseHttpClient {
  constructor(
    private readonly settings: CrawlerSettings,
    private readonly allowedOrigin: string,
    private readonly fetchOptions: SecureFetchOptions = {},
  ) {}

  async stream(request: HttpRequest): Promise<StreamingHttpResponse> {
    if (request.method && request.method.toUpperCase() !== "GET") {
      throw new CrawlerJobError("source_method_not_allowed", "Website crawling permits only GET requests.");
    }
    if (request.proxyUrl) {
      throw new CrawlerJobError("source_proxy_not_allowed", "Documentation crawling cannot use an outbound proxy.");
    }
    const requestedURL = new URL(request.url);
    if (requestedURL.origin !== this.allowedOrigin) {
      throw new CrawlerJobError("source_redirect_not_allowed", "Website crawls may not navigate to a different origin.");
    }

    const result = await secureFetch(
      requestedURL.toString(),
      this.settings,
      "text/html,application/xhtml+xml,application/xml,text/xml,application/json;q=0.9,text/plain;q=0.5",
      {
        ...this.fetchOptions,
        headers: { ...outboundHeaders(request), ...this.fetchOptions.headers },
        redirectPolicy: async (from, to) => {
          if (to.origin !== this.allowedOrigin) {
            throw new CrawlerJobError("source_redirect_not_allowed", "Website crawls may not navigate to a different origin.");
          }
          await this.fetchOptions.redirectPolicy?.(from, to);
        },
      },
    );
    const { stream, progress } = boundedStream(result.response, this.settings.maxBytes);
    return {
      stream,
      request,
      redirectUrls: [...result.redirectURLs],
      url: result.url.toString(),
      statusCode: result.response.status,
      statusMessage: result.response.statusText,
      headers: responseHeaders(result.response),
      trailers: {},
      complete: true,
      downloadProgress: progress,
      uploadProgress: { percent: 1, transferred: 0, total: 0 },
    };
  }

  async sendRequest<TResponseType extends keyof ResponseTypes = "text">(
    request: HttpRequest<TResponseType>,
  ): Promise<HttpResponse<TResponseType>> {
    const response = await this.stream(request as HttpRequest);
    const chunks: Buffer[] = [];
    for await (const chunk of response.stream) chunks.push(Buffer.from(chunk));
    const buffer = Buffer.concat(chunks);
    let body: unknown = buffer.toString("utf8");
    if (request.responseType === "buffer") body = buffer;
    if (request.responseType === "json") body = JSON.parse(buffer.toString("utf8"));
    return {
      request: response.request,
      redirectUrls: response.redirectUrls,
      url: response.url,
      statusCode: response.statusCode,
      statusMessage: response.statusMessage,
      headers: response.headers,
      trailers: response.trailers,
      complete: response.complete,
      body,
    } as HttpResponse<TResponseType>;
  }
}
