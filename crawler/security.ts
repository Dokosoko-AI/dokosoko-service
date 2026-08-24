import { lookup } from "node:dns/promises";
import { isIP, type LookupFunction } from "node:net";

const DEFAULT_MAX_PAGES = 500;
const DEFAULT_MAX_BYTES = 5_000_000;

export type CrawlerSettings = {
  maxPages: number;
  maxBytes: number;
  dataDir: string;
  uploadDir: string | null;
  allowLocalhostSubdomains: boolean;
  localhostPorts: ReadonlySet<number>;
};

export class CrawlerJobError extends Error {
  readonly code: string;

  constructor(code: string, message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = "CrawlerJobError";
    this.code = code;
  }
}

function positiveInteger(value: string | undefined, fallback: number, name: string): number {
  if (value === undefined || value.trim() === "") return fallback;
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed < 1) {
    throw new CrawlerJobError("crawler_configuration_invalid", `${name} must be a positive integer.`);
  }
  return parsed;
}

function booleanSetting(value: string | undefined): boolean {
  if (value === undefined || value.trim() === "") return false;
  if (value === "true" || value === "1") return true;
  if (value === "false" || value === "0") return false;
  throw new CrawlerJobError("crawler_configuration_invalid", "DOKOSOKO_CRAWLER_ALLOW_LOCALHOST_SUBDOMAINS must be true or false.");
}

function portSetting(value: string | undefined): ReadonlySet<number> {
  const ports = new Set<number>();
  for (const item of (value ?? "80,443").split(",")) {
    const port = Number(item.trim());
    if (!Number.isSafeInteger(port) || port < 1 || port > 65_535) {
      throw new CrawlerJobError("crawler_configuration_invalid", "DOKOSOKO_CRAWLER_LOCALHOST_PORTS contains an invalid port.");
    }
    ports.add(port);
  }
  if (ports.size === 0) {
    throw new CrawlerJobError("crawler_configuration_invalid", "DOKOSOKO_CRAWLER_LOCALHOST_PORTS must contain at least one port.");
  }
  return ports;
}

export function loadCrawlerSettings(env: Readonly<Record<string, string | undefined>> = process.env): CrawlerSettings {
  return {
    maxPages: positiveInteger(env.DOKOSOKO_CRAWLER_MAX_PAGES, DEFAULT_MAX_PAGES, "DOKOSOKO_CRAWLER_MAX_PAGES"),
    maxBytes: positiveInteger(env.DOKOSOKO_CRAWLER_MAX_BYTES, DEFAULT_MAX_BYTES, "DOKOSOKO_CRAWLER_MAX_BYTES"),
    dataDir: env.DOKOSOKO_DATA_DIR?.trim() || "/data",
    uploadDir: env.DOKOSOKO_UPLOAD_DIR?.trim() || null,
    allowLocalhostSubdomains: booleanSetting(env.DOKOSOKO_CRAWLER_ALLOW_LOCALHOST_SUBDOMAINS),
    localhostPorts: portSetting(env.DOKOSOKO_CRAWLER_LOCALHOST_PORTS),
  };
}

const deniedV4 = [
  ["0.0.0.0", 8], ["10.0.0.0", 8], ["100.64.0.0", 10], ["127.0.0.0", 8],
  ["169.254.0.0", 16], ["172.16.0.0", 12], ["192.0.0.0", 24], ["192.0.2.0", 24],
  ["192.88.99.0", 24], ["192.168.0.0", 16], ["198.18.0.0", 15], ["198.51.100.0", 24], ["203.0.113.0", 24],
  ["224.0.0.0", 4], ["240.0.0.0", 4],
] as const;

const localDevelopmentV4 = [
  ["10.0.0.0", 8], ["127.0.0.0", 8], ["172.16.0.0", 12], ["192.168.0.0", 16],
] as const;

const deniedV6 = [
  ["::", 96],
  ["::ffff:0:0", 96],
  ["64:ff9b::", 96],
  ["64:ff9b:1::", 48],
  ["100::", 64],
  ["2001::", 23],
  ["2001:db8::", 32],
  ["2002::", 16],
  ["3fff::", 20],
  ["5f00::", 16],
  ["fc00::", 7],
  ["fe80::", 10],
  ["fec0::", 10],
  ["ff00::", 8],
] as const;

function ipv4Number(value: string): number {
  return value.split(".").reduce((total, part) => (total << 8) + Number(part), 0) >>> 0;
}

function inV4Network(value: string, network: string, prefix: number): boolean {
  const address = ipv4Number(value);
  const mask = (0xffffffff << (32 - prefix)) >>> 0;
  return (address & mask) === (ipv4Number(network) & mask);
}

function ipv6Number(value: string): bigint {
  let normalized = value.toLowerCase();
  const embeddedV4 = normalized.match(/(?:^|:)(\d{1,3}(?:\.\d{1,3}){3})$/)?.[1];
  if (embeddedV4) {
    const address = ipv4Number(embeddedV4);
    normalized = `${normalized.slice(0, -embeddedV4.length)}${(address >>> 16).toString(16)}:${(address & 0xffff).toString(16)}`;
  }
  const halves = normalized.split("::");
  const left = halves[0] ? halves[0].split(":") : [];
  const right = halves[1] ? halves[1].split(":") : [];
  const missing = 8 - left.length - right.length;
  const parts = halves.length === 2 ? [...left, ...Array(missing).fill("0"), ...right] : left;
  return parts.reduce((total, part) => (total << BigInt(16)) | BigInt(`0x${part || "0"}`), BigInt(0));
}

function inV6Network(value: string, network: string, prefix: number): boolean {
  const shift = BigInt(128 - prefix);
  return (ipv6Number(value) >> shift) === (ipv6Number(network) >> shift);
}

export function isDeniedAddress(value: string): boolean {
  const normalized = value.toLowerCase().split("%")[0];
  if (isIP(normalized) === 4) {
    return deniedV4.some(([network, prefix]) => inV4Network(normalized, network, prefix));
  }
  if (isIP(normalized) === 6) {
    return deniedV6.some(([network, prefix]) => inV6Network(normalized, network, prefix));
  }
  return true;
}

export function isLocalDevelopmentAddress(value: string): boolean {
  const normalized = value.toLowerCase().split("%")[0];
  if (isIP(normalized) === 4) {
    return localDevelopmentV4.some(([network, prefix]) => inV4Network(normalized, network, prefix));
  }
  if (isIP(normalized) === 6) {
    return normalized === "::1" || inV6Network(normalized, "fc00::", 7);
  }
  return false;
}

export function isLocalhostSubdomain(value: string): boolean {
  return /^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+localhost$/i.test(value);
}

export type AddressResolver = (hostname: string) => Promise<ReadonlyArray<{ address: string; family?: number }>>;

export type ResolvedAddress = {
  address: string;
  family: 4 | 6;
};

export type SafeURLResolution = {
  url: URL;
  hostname: string;
  addresses: readonly ResolvedAddress[];
};

const resolveAddresses: AddressResolver = async (hostname) => lookup(hostname, { all: true, verbatim: true });

export async function resolveSafeURL(
  value: string,
  settings: CrawlerSettings = loadCrawlerSettings(),
  resolver: AddressResolver = resolveAddresses,
): Promise<SafeURLResolution> {
  if (!value || value !== value.trim()) {
    throw new CrawlerJobError("source_location_invalid", "Source location must be a complete HTTP(S) URL without surrounding whitespace.");
  }
  let url: URL;
  try {
    url = new URL(value);
  } catch (error) {
    throw new CrawlerJobError("source_location_invalid", "Source location must be a valid HTTP(S) URL.", { cause: error });
  }
  if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || !url.hostname) {
    throw new CrawlerJobError("source_url_not_allowed", "Source URLs must be credential-free HTTP(S) URLs.");
  }

  const hostname = url.hostname.toLowerCase().replace(/^\[|\]$/g, "");
  const localDevelopment = isLocalhostSubdomain(hostname);
  const effectivePort = Number(url.port || (url.protocol === "https:" ? "443" : "80"));

  if (localDevelopment) {
    if (!settings.allowLocalhostSubdomains) {
      throw new CrawlerJobError("localhost_source_disabled", "Localhost-subdomain crawling is disabled. Enable it only in an isolated development environment.");
    }
    if (!settings.localhostPorts.has(effectivePort)) {
      throw new CrawlerJobError("localhost_port_not_allowed", `The localhost development port ${effectivePort} is not in the crawler allowlist.`);
    }
  } else if (url.port && !["80", "443"].includes(url.port)) {
    throw new CrawlerJobError("source_port_not_allowed", "Public source URLs may use only ports 80 and 443.");
  }

  let addresses: ReadonlyArray<{ address: string; family?: number }>;
  try {
    addresses = await resolver(hostname);
  } catch (error) {
    throw new CrawlerJobError("source_dns_failed", "The source hostname could not be resolved.", { cause: error });
  }
  if (addresses.length === 0) {
    throw new CrawlerJobError("source_dns_failed", "The source hostname did not resolve to an address.");
  }

  const resolved = addresses.map((entry): ResolvedAddress => {
    const address = entry.address.toLowerCase();
    const family = isIP(address);
    if ((family !== 4 && family !== 6) || (entry.family !== undefined && entry.family !== family) || address.includes("%")) {
      throw new CrawlerJobError("source_dns_failed", "The source hostname resolved to an invalid address.");
    }
    return { address, family };
  });

  if (localDevelopment) {
    if (resolved.some((entry) => !isLocalDevelopmentAddress(entry.address))) {
      throw new CrawlerJobError("localhost_resolution_not_allowed", "A localhost development source must resolve only to loopback, RFC1918, or unique-local addresses.");
    }
  } else if (resolved.some((entry) => isDeniedAddress(entry.address))) {
    throw new CrawlerJobError("source_network_not_allowed", "The source hostname resolves to a private, loopback, link-local, or reserved network.");
  }
  return {
    url,
    hostname,
    addresses: [...new Map(resolved.map((entry) => [`${entry.family}:${entry.address}`, entry])).values()],
  };
}

export async function assertSafeURL(
  value: string,
  settings: CrawlerSettings = loadCrawlerSettings(),
  resolver: AddressResolver = resolveAddresses,
): Promise<URL> {
  return (await resolveSafeURL(value, settings, resolver)).url;
}

/**
 * Returns a lookup function that can only hand the HTTP stack an address from
 * one completed safety check. The original hostname remains in the request URL,
 * so HTTP Host and HTTPS SNI/certificate verification are not replaced by the IP.
 */
export function createPinnedLookup(resolution: SafeURLResolution): LookupFunction {
  return (hostname, options, callback) => {
    const requestedHostname = hostname.toLowerCase().replace(/^\[|\]$/g, "");
    if (requestedHostname !== resolution.hostname) {
      const error = new Error("The HTTP client attempted to resolve a hostname outside its vetted destination.") as NodeJS.ErrnoException;
      error.code = "EAI_FAIL";
      callback(error, "", 0);
      return;
    }

    const requestedFamily = options.family === 4 || options.family === 6 ? options.family : 0;
    const candidates = requestedFamily === 0
      ? resolution.addresses
      : resolution.addresses.filter((entry) => entry.family === requestedFamily);
    if (candidates.length === 0) {
      const error = new Error("The vetted destination has no address in the requested family.") as NodeJS.ErrnoException;
      error.code = "EAI_NODATA";
      callback(error, "", 0);
      return;
    }

    if (options.all) {
      callback(null, candidates.map((entry) => ({ ...entry })));
      return;
    }
    callback(null, candidates[0].address, candidates[0].family);
  };
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
