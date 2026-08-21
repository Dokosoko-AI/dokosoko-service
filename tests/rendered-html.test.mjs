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

test("server-renders the DokoSoko console overview", async () => {
  const response = await render();
  assert.equal(response.status, 200);
  assert.match(response.headers.get("content-type") ?? "", /^text\/html\b/i);

  const html = await response.text();
  assert.match(html, /<title>DokoSoko — Agent delivery control plane<\/title>/i);
  assert.match(html, /Connector readiness/);
  assert.match(html, /Quick actions/);
  assert.match(html, /Integrations/);
  assert.match(html, /Operations/);
  assert.match(html, /Insights/);
  assert.match(html, /href="\/overview"/);
  assert.match(html, /href="\/integrations"/);
  assert.match(html, /href="\/operations"/);
  assert.match(html, /href="\/settings"/);
  assert.doesNotMatch(html, /codex-preview|react-loading-skeleton|Your site is taking shape/i);
});

test("groups the console into workflow navigation with contextual sections", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const styles = await readFile(new URL("../app/globals.css", import.meta.url), "utf8");
  const routes = await readFile(new URL("../app/lib/console-routes.ts", import.meta.url), "utf8");

  for (const label of ["Overview", "Integrations", "Access", "Distribution", "Operations", "Insights"]) {
    assert.match(source, new RegExp(`label: "${label}"`));
  }
  for (const secondary of ["Documentation", "Packages", "Tools", "Hooks & MCP", "Connector releases", "Support reporting", "Activity & audit"]) {
    assert.ok(source.includes(`label: "${secondary}"`));
  }
  assert.match(source, /useState<ConsoleRoute>/);
  assert.doesNotMatch(source, /setSection/);
  assert.match(source, /window\.history\[method\]/);
  assert.match(source, /className="section-tabs"/);
  assert.match(source, /className="mobile-navigation"/);
  for (const path of ["/overview", "/integrations", "/integrations/documentation", "/access", "/distribution/releases", "/operations/reporting", "/insights/activity", "/settings"]) {
    assert.ok(routes.includes(`"${path}"`), `${path} should be registered`);
  }
  for (const entity of ["integration", "resource-set", "source", "package", "tool", "connection", "release", "run", "support-route", "report", "audit-event", "root-user"]) {
    assert.ok(routes.includes(`| "${entity}"`) || routes.includes(`  ${entity}:`), `${entity} should be routable`);
  }
  assert.match(styles, /\.sidebar > nav/);
  assert.match(styles, /\.section-tab\.active/);
  assert.match(styles, /\.entity-detail-grid/);
  assert.match(styles, /\.content > \.panel \+ \.panel \{ margin-top: 20px; \}/);
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

test("ships deployment-release discovery, lifecycle, pinning, and AI rewrite controls", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  assert.match(source, /Deployment discovery & connector releases/);
  assert.match(source, /Rewrite for agents/);
  assert.match(source, /Publish connector release/);
  assert.match(source, /Scoped version pins/);
  assert.match(source, /immutable connector-release integrity/);
  assert.match(source, /Integration installations/);
  assert.match(source, /generated release diff/);
  assert.match(source, /Independent approval required/);
  assert.match(source, /No silent migration/);
  assert.match(source, /Latest/);
  assert.match(source, /LTS/);
  assert.match(source, /Deprecated/);
  assert.match(client, /description\/rewrite/);
  assert.match(client, /version-pins/);
  assert.match(client, /productVersions/);
});

test("ships the private vendor-proxied usage report configuration", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  assert.match(source, /Usage report hook/);
  assert.match(source, /read-only usage\.get tool on Private MCP/);
  assert.match(source, /Returned values are proxied without storage/);
  assert.match(source, /Rotate usage credential/);
  assert.match(client, /usage_hook_url/);
  assert.match(client, /usage_credential/);
});

test("ships consent-gated support reporting configuration and inbox", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  assert.match(source, /Support reporting/);
  assert.match(source, /support\.report_bug/);
  assert.match(source, /support\.submit_feedback/);
  assert.match(source, /encrypted holding/);
  assert.match(source, /Submission inbox/);
  assert.match(source, />View<\/Button>/);
  assert.match(source, />Retry<\/Button>/);
  assert.match(source, /Fixed agent policy/);
  assert.match(client, /report-submissions/);
  assert.match(source, /Default route for Integrations without an override/);
  assert.match(source, /Leave empty to hold locally/);
  assert.match(client, /createSupportRoute/);
  assert.match(client, /updateSupportRoute/);
  assert.match(client, /retryReportSubmission/);
});

test("ships first-class Integration, reusable-set, and provider-access management", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  assert.match(source, /New Integration/);
  assert.match(source, /Create reusable resource set/);
  assert.match(source, /Duplicate resource set/);
  assert.match(source, /Pin the current revision instead of following latest/);
  assert.match(source, /Create service definition/);
  assert.match(source, /One fixed instance/);
  assert.match(source, /Multiple provider resources/);
  assert.match(source, /Allowed Integrations/);
  assert.match(client, /createIntegration/);
  assert.match(client, /updateIntegration/);
  assert.match(client, /duplicateResourceSet/);
  assert.match(client, /createAccessDefinition/);
  assert.match(client, /createAccessConnection/);
  assert.doesNotMatch(client, /createProvider:/);
  assert.doesNotMatch(client, /projects:\s*async/);
});
