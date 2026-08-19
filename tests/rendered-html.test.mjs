import assert from "node:assert/strict";
import { access, readFile } from "node:fs/promises";
import test from "node:test";

async function render() {
  const workerUrl = new URL("../dist/server/index.js", import.meta.url);
  workerUrl.searchParams.set("test", `${process.pid}-${Date.now()}`);
  const { default: worker } = await import(workerUrl.href);
  return worker.fetch(
    new Request("http://localhost/", { headers: { accept: "text/html" } }),
    { ASSETS: { fetch: async () => new Response("Not found", { status: 404 }) } },
    { waitUntil() {}, passThroughOnException() {} },
  );
}

test("server-renders the DokoSoko distribution console", async () => {
  const response = await render();
  assert.equal(response.status, 200);
  assert.match(response.headers.get("content-type") ?? "", /^text\/html\b/i);

  const html = await response.text();
  assert.match(html, /<title>DokoSoko — Agent delivery control plane<\/title>/i);
  assert.match(html, /MCP &amp; widgets/);
  assert.match(html, /Public MCP/);
  assert.match(html, /authentication-free/);
  assert.match(html, /Disabled by default/);
  assert.match(html, /Developer documentation/);
  assert.match(html, /@acme\/node/);
  assert.match(html, /Public widget/);
  assert.match(html, /Private widget/);
  assert.match(html, /Copy private widget/);
  assert.doesNotMatch(html, /codex-preview|react-loading-skeleton|Your site is taking shape/i);
});

test("keeps private defaults and guarded public transitions in the client contract", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const packageJson = await readFile(new URL("../package.json", import.meta.url), "utf8");

  assert.match(source, /visibility:\s*"private"/);
  assert.match(source, /setPendingPublication/);
  assert.match(source, /disabled=\{!acknowledged\}/);
  assert.match(source, /Make public/);
  assert.match(source, /available without authentication/);
  assert.match(source, /setPublicMCPEnabled\(false\)/);
  assert.match(source, /api\.widgets\(product\.id\)/);
  assert.match(source, /distribution\?\.public_mcp_endpoint/);
  assert.match(source, /widgetSnippets\?\.public\.snippet/);
  assert.match(source, /widgetSnippets\?\.private\.snippet/);
  assert.match(source, /widgets\/\$\{product\.id\}\/public\.js/);
  assert.match(source, /widgets\/\$\{product\.id\}\/private\.js/);
  assert.doesNotMatch(source, /dokosoko\.acme\.dev/);
  assert.match(packageJson, /"@headlessui\/react"/);
  assert.doesNotMatch(packageJson, /react-loading-skeleton/);
  await assert.rejects(access(new URL("../app/_sites-preview/SkeletonPreview.tsx", import.meta.url)));
});

test("ships the automatic Product Definition review flow", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  assert.match(source, /Build product automatically/);
  assert.match(source, /Review exceptions, not configuration/);
  assert.match(source, /Independent API tracks/);
  assert.match(source, /Voice API/);
  assert.match(source, /Messages API/);
  assert.match(source, /Stateless MCPv2 Only/);
  assert.match(client, /product-builds/);
  assert.match(client, /publishProductBuild/);
});

test("ships product-version discovery, lifecycle, pinning, and AI rewrite controls", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  assert.match(source, /Product discovery & versions/);
  assert.match(source, /Rewrite for agents/);
  assert.match(source, /Publish product version/);
  assert.match(source, /Customer version pins/);
  assert.match(source, /No silent migration/);
  assert.match(source, /Latest/);
  assert.match(source, /LTS/);
  assert.match(source, /Deprecated/);
  assert.match(client, /description\/rewrite/);
  assert.match(client, /version-pins/);
  assert.match(client, /productVersions/);
});
