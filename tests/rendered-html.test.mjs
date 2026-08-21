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

test("server-renders the DokoSoko API directory", async () => {
  const response = await render();
  assert.equal(response.status, 200);
  assert.match(response.headers.get("content-type") ?? "", /^text\/html\b/i);

  const html = await response.text();
  assert.match(html, /<title>DokoSoko — Agent delivery control plane<\/title>/i);
  assert.match(html, />APIs</);
  assert.match(html, /Agent access/);
  assert.match(html, /Activity/);
  assert.doesNotMatch(html, /Connector readiness|Quick actions|>Overview</);
  assert.match(html, /href="\/integrations"/);
  assert.match(html, /href="\/agent-access"/);
  assert.match(html, /href="\/activity"/);
  assert.match(html, /href="\/settings"/);
  assert.doesNotMatch(html, /codex-preview|react-loading-skeleton|Your site is taking shape/i);
});

test("keeps the global navigation to four obvious destinations", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const styles = await readFile(new URL("../app/globals.css", import.meta.url), "utf8");
  const routes = await readFile(new URL("../app/lib/console-routes.ts", import.meta.url), "utf8");

  for (const label of ["APIs", "Agent access", "Activity"]) {
    assert.match(source, new RegExp(`label: "${label}"`));
  }
  for (const removed of ["label: \"Overview\"", "label: \"Integrations\"", "label: \"Access\"", "label: \"Distribution\"", "label: \"Operations\"", "label: \"Insights\""]) {
    assert.ok(!source.includes(removed), `${removed} should not remain in primary navigation`);
  }
  assert.match(source, /useState<ConsoleRoute>/);
  assert.doesNotMatch(source, /setSection/);
  assert.match(source, /window\.history\[method\]/);
  assert.match(source, /window\.history\.replaceState\(null, "", `\$\{next\.path\}/);
  assert.doesNotMatch(source, /replaceState\(null, "", `\$\{sectionPath\("overview"\)/);
  assert.doesNotMatch(source, /className="section-tabs"/);
  assert.match(source, /className="mobile-navigation"/);
  for (const path of ["/integrations", "/agent-access", "/activity", "/settings"]) {
    assert.ok(routes.includes(`"${path}"`), `${path} should be registered`);
  }
  for (const entity of ["integration", "resource-set", "source", "package", "tool", "connection", "release", "run", "support-route", "report", "audit-event", "root-user"]) {
    assert.ok(routes.includes(`| "${entity}"`) || routes.includes(`  ${entity}:`), `${entity} should be routable`);
  }
  assert.match(styles, /\.sidebar > nav/);
  assert.match(styles, /\.entity-detail-grid/);
  assert.match(styles, /\.package-columns \{ grid-template-columns: [^}]+ 104px; \}/);
  assert.match(styles, /\.content > \.panel \+ \.panel \{ margin-top: 20px; \}/);
});

test("uses an API directory and a four-tab contextual workspace", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const styles = await readFile(new URL("../app/globals.css", import.meta.url), "utf8");
  const routes = await readFile(new URL("../app/lib/console-routes.ts", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  for (const label of ["Overview", "Resources", "Access", "History"]) {
    assert.ok(routes.includes(`label: "${label}"`), `${label} API tab should be registered`);
  }
  for (const removed of ["Tools & hooks", "label: \"Usage\"", "label: \"Support\"", "label: \"Revisions\""]) assert.ok(!routes.includes(removed));
  assert.match(source, /className="integration-tabs"/);
  assert.match(source, /IntegrationDirectoryView/);
  assert.match(source, /IntegrationWorkspaceView/);
  assert.match(source, /Published history/);
  assert.match(source, /Switch API/);
  assert.match(source, /Only unresolved actions appear here/);
  assert.match(source, /Customer usage/);
  assert.match(source, /Advanced details/);
  assert.doesNotMatch(source, /No changes|Filter by API family|Filter by setup state/);
  assert.match(styles, /\.integration-directory-columns/);
  assert.match(styles, /\.integration-tab\.active/);
  assert.match(styles, /\.advanced-details/);
  assert.match(client, /integration: \(integrationID: string\)/);
  assert.match(client, /APIIntegrationRevision/);
  assert.match(client, /APIIntegrationPublishStatus/);
  assert.match(client, /setIntegrationAccessConnections/);
  assert.match(client, /setIntegrationSupportRoute/);
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

test("imports APIs without exposing Product Definition as a product concept", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  assert.match(source, /title="Import APIs"/);
  assert.match(source, /Review exceptions, not configuration/);
  assert.match(source, /Voice API/);
  assert.match(source, /Messages API/);
  assert.match(source, /Stateless MCPv2 Only/);
  assert.doesNotMatch(source, /Auto-magic|title="Product definition"|Build product automatically/);
  assert.match(client, /product-builds/);
  assert.match(client, /publishProductBuild/);
});

test("keeps immutable publishing controls behind advanced settings", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  assert.match(source, /title="Advanced publishing"/);
  assert.match(source, />Advanced publishing</);
  assert.match(source, /Rewrite for agents/);
  assert.match(source, /Publish compatibility snapshot/);
  assert.match(source, /Scoped version pins/);
  assert.match(source, /Customer installations/);
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

  assert.match(source, /Customer usage endpoint/);
  assert.match(source, /Customer account data/);
  assert.match(source, /read-only usage\.get tool on Private MCP/);
  assert.match(source, /customer-defined values are proxied without storage/i);
  assert.match(source, /Rotate usage credential/);
  assert.match(source, /Up to 50 customer-defined values/);
  assert.doesNotMatch(source, /label: "Usage"/);
  assert.match(client, /usage_hook_url/);
  assert.match(client, /usage_credential/);
});

test("ships consent-gated support reporting configuration and inbox", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  assert.match(source, /Bug reports & feedback/);
  assert.match(source, /Consent is enforced/);
  assert.match(source, /Encrypted local holding/);
  assert.match(source, /Delivery policies/);
  assert.match(source, />View<\/Button>/);
  assert.match(source, />Retry<\/Button>/);
  assert.match(source, /className="activity-toolbar"/);
  assert.match(client, /report-submissions/);
  assert.match(source, /Use as the default for all APIs/);
  assert.match(source, /Support webhook configured/);
  assert.match(source, /Leave empty to hold locally/);
  assert.match(client, /createSupportRoute/);
  assert.match(client, /updateSupportRoute/);
  assert.match(client, /retryReportSubmission/);
});

test("ships first-class API, reusable resource, and service-connection management", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  assert.match(source, /Add API/);
  assert.match(source, /Create reusable resource set/);
  assert.match(source, /Duplicate resource set/);
  assert.match(source, /Pin the current revision instead of following latest/);
  assert.match(source, /Create service type/);
  assert.match(source, /One fixed instance/);
  assert.match(source, /Multiple provider resources/);
  assert.match(source, /Allowed APIs/);
  assert.match(source, /Tool backend/);
  assert.match(client, /createIntegration/);
  assert.match(client, /updateIntegration/);
  assert.match(client, /duplicateResourceSet/);
  assert.match(client, /createAccessDefinition/);
  assert.match(client, /createAccessConnection/);
  assert.doesNotMatch(client, /createProvider:/);
  assert.doesNotMatch(client, /projects:\s*async/);
});
