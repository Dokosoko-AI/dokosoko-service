import type { SDKPackageImportInput } from "./developer-assets-api";
type Ecosystem = SDKPackageImportInput["ecosystem"];
export type SDKPackageLocation = { ecosystem: Ecosystem; coordinate: string; sourceURL: string; exactVersion?: string };
const origins: Record<Ecosystem, string> = { npm: "https://registry.npmjs.org", pypi: "https://pypi.org", cargo: "https://crates.io", go: "https://proxy.golang.org" };
export function resolveSDKPackageLocation(input: string, selected: Ecosystem): SDKPackageLocation | undefined {
  const value = input.trim();
  if (!value || value.length > 2048) return undefined;
  if (!value.includes("://")) {
    if (/\s|[?#\\]/.test(value)) return undefined;
    let coordinate = value; let exactVersion: string | undefined;
    const versionAt = selected === "pypi" ? value.indexOf("==") : value.lastIndexOf("@");
    if (versionAt > 0) { coordinate = value.slice(0, versionAt); exactVersion = value.slice(versionAt + (selected === "pypi" ? 2 : 1)) || undefined; }
    return { ecosystem: selected, coordinate, sourceURL: origins[selected], exactVersion };
  }
  try {
    const url = new URL(value);
    if (url.protocol !== "https:" || url.username || url.password || url.search || url.hash || url.port) return undefined;
    const path = decodeURIComponent(url.pathname).replace(/^\/+|\/+$/g, "");
    if (["npmjs.com", "www.npmjs.com", "registry.npmjs.org"].includes(url.hostname)) {
      const parts = path.replace(/^package\//, "").split("/");
      const size = parts[0]?.startsWith("@") ? 2 : 1;
      if (parts.slice(0, size).some((part) => !part) || !parts[0]) return undefined;
      const rest = parts.slice(size);
      if (rest.length && !(rest.length === 2 && rest[0] === "v")) return undefined;
      return { ecosystem: "npm", coordinate: parts.slice(0, size).join("/"), sourceURL: origins.npm, exactVersion: rest[1] };
    }
    if (url.hostname === "pypi.org") {
      const match = /^project\/([^/]+)(?:\/([^/]+))?$/.exec(path);
      if (match) return { ecosystem: "pypi", coordinate: match[1], sourceURL: origins.pypi, exactVersion: match[2] };
    }
    if (url.hostname === "crates.io") {
      const match = /^crates\/([^/]+)(?:\/([^/]+))?$/.exec(path);
      if (match) return { ecosystem: "cargo", coordinate: match[1], sourceURL: origins.cargo, exactVersion: match[2] };
    }
    if (url.hostname === "pkg.go.dev" && path) return resolveSDKPackageLocation(path, "go");
  } catch { /* Ambiguous/custom sources require an explicit correction. */ }
  return undefined;
}
