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

test("server-renders an authentication-safe loading shell", async () => {
  const response = await render();
  assert.equal(response.status, 200);
  assert.match(response.headers.get("content-type") ?? "", /^text\/html\b/i);

  const html = await response.text();
  assert.match(html, /<title>DokoSoko — Agent delivery control plane<\/title>/i);
  assert.match(html, /Opening DokoSoko/);
  assert.match(html, /Loading the authenticated deployment/);
  assert.doesNotMatch(html, /Acme Platform|prod_acme|org_acme/);
  assert.doesNotMatch(html, /codex-preview|react-loading-skeleton|Your site is taking shape/i);
});

test("keeps the global navigation to five obvious destinations", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const styles = await readFile(new URL("../app/globals.css", import.meta.url), "utf8");
  const routes = await readFile(new URL("../app/lib/console-routes.ts", import.meta.url), "utf8");

  for (const label of ["APIs", "Recipes", "Agent access", "Activity"]) {
    assert.match(source, new RegExp(`label: "${label}"`));
  }
  assert.match(source, /\{ id: "recipes", label: "Recipes", icon: BookOpen, defaultSection: "recipes"/);
  assert.doesNotMatch(source, /<BookOpen data-slot="icon" \/>Recipes<\/Button>/);
  assert.doesNotMatch(source, /Control plane/);
  assert.doesNotMatch(source, /className="environment"/);
  assert.doesNotMatch(styles, /\.environment\s*\{/);
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
  for (const path of ["/integrations", "/recipes", "/agent-access", "/activity", "/settings"]) {
    assert.ok(routes.includes(`"${path}"`), `${path} should be registered`);
  }
  for (const entity of ["integration", "resource-set", "source", "tool", "connection", "release", "run", "support-route", "report", "audit-event", "root-user"]) {
    assert.ok(routes.includes(`| "${entity}"`) || routes.includes(`  ${entity}:`), `${entity} should be routable`);
  }
  assert.match(styles, /\.sidebar > nav/);
  assert.match(styles, /\.entity-detail-grid/);
  assert.match(styles, /\.agent-setup-grid/);
  assert.match(styles, /\.content > \.panel \+ \.panel \{ margin-top: var\(--space-section\); \}/);
});

test("gives AI providers a dedicated, guarded settings workspace", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const styles = await readFile(new URL("../app/globals.css", import.meta.url), "utf8");
  const routes = await readFile(new URL("../app/lib/console-routes.ts", import.meta.url), "utf8");
  const api = await readFile(new URL("../api/openapi.yaml", import.meta.url), "utf8");

  assert.match(routes, /label: "AI providers"/);
  assert.match(routes, /settingsPath/);
  assert.match(source, /title="AI providers"/);
  assert.doesNotMatch(source, /Two models, two clear jobs|AI ready|workloads enabled/);
  assert.match(source, /title="Workloads"/);
  assert.match(source, /title="Providers"/);
  assert.doesNotMatch(source, /Choose one strong model for analysis|Fetching, retrieval, authorization/);
  assert.match(source, /OpenAI-compatible/);
  assert.doesNotMatch(source, /Mandatory AI safeguards|No model tools|Grounded output/);
  assert.match(source, /Citations required/);
  assert.match(source, /<strong>Connect \{provider\.name\}<\/strong>/);
  for (const provider of ["OpenAI", "Google", "Anthropic", "DigitalOcean", "xAI", "DeepSeek"]) assert.ok(source.includes(`name: "${provider}"`));
  assert.match(source, /name: "Other OpenAPI compatible providers"/);
  assert.doesNotMatch(source, /Already connected · manage settings/);
  assert.match(source, /OpenAIProviderMark/);
  assert.match(source, /GeminiProviderMark/);
  assert.match(source, /ClaudeProviderMark/);
  assert.match(source, /DigitalOceanProviderMark/);
  assert.match(source, /XAIProviderMark/);
  assert.match(source, /DeepSeekProviderMark/);
  assert.match(source, /Backup provider/);
  assert.doesNotMatch(source, /<Badge[^>]*>Native<\/Badge>|<Badge[^>]*>Custom<\/Badge>/);
  assert.match(source, /title=\{`Configure \$\{/);
  assert.match(source, /Leave blank to keep the stored credential/);
  assert.doesNotMatch(source, /title="Configure LLM profile"/);
  assert.doesNotMatch(styles, /\.ai-settings-hero|\.ai-hero-mark|\.ai-hero-stat/);
  assert.match(styles, /\.ai-settings-table/);
  assert.match(styles, /\.ai-provider-suggestions/);
  assert.match(styles, /\.ai-provider-logo/);
  assert.match(api, /Credential-redacted role-based AI profiles/);
  assert.match(api, /enum: \[analysis, assistant\]/);
  assert.match(api, /provider_role: \{ type: string, enum: \[primary, backup\] \}/);
  assert.match(api, /endpoint: \{ type: string, format: uri, description: Fixed HTTPS provider origin/);
  for (const provider of ["digitalocean", "xai", "deepseek"]) assert.match(api, new RegExp(provider));
});

test("ships one evidence-to-recipe review workflow", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  assert.doesNotMatch(source, /Turn verified integration evidence into implementation guides/);
  const styles = await readFile(new URL("../app/globals.css", import.meta.url), "utf8");
  const routes = await readFile(new URL("../app/lib/console-routes.ts", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");
  const api = await readFile(new URL("../api/openapi.yaml", import.meta.url), "utf8");

  assert.match(routes, /recipes: "\/recipes"/);
  for (const label of ["Add recipe", "Create recipe", "Build recipe", "All recipes", "Save changes", "Publish recipe"]) assert.match(source, new RegExp(label));
  assert.match(source, /What should this recipe help developers do/);
  assert.match(source, /documentation, API definitions, service connections, MCP connectors, and tools/);
  assert.match(source, /Ask AI to revise this recipe/);
  assert.doesNotMatch(source, /Start from evidence, not a blank prompt|Review queue|Most used · 30 days/);
  assert.match(styles, /\.recipe-library-row/);
  assert.match(styles, /\.recipe-editor-layout/);
  assert.match(client, /createRecipe/);
  assert.match(api, /\/api\/v1\/products\/\{product_id\}\/recipes:/);
  assert.match(api, /operationId: createRecipe/);
  assert.match(api, /resources\/list, resources\/read/);
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
  assert.match(source, /<PageTabs label=.*integration\.display_name/);
  assert.match(source, /IntegrationDirectoryView/);
  assert.match(source, /IntegrationWorkspaceView/);
  assert.match(source, /Published history/);
  assert.match(source, /Switch API/);
  assert.doesNotMatch(source, /Only unresolved actions appear here/);
  assert.match(source, /Customer access/);
  assert.match(source, /Advanced details/);
  assert.doesNotMatch(source, /No changes|Filter by API family|Filter by setup state|integration-family-heading|groupedIntegrations/);
  assert.match(styles, /\.integration-directory-columns/);
  assert.doesNotMatch(styles, /\.integration-family-heading/);
  assert.match(styles, /\.page-tab\.active/);
  assert.match(styles, /\.advanced-details/);
  assert.match(styles, /\.advanced-details > summary::after\s*\{[^}]*content:\s*""[^}]*border-right:[^}]*transform:\s*rotate\(45deg\)/);
  assert.doesNotMatch(styles, /\.advanced-details > summary::after\s*\{[^}]*content:\s*"\+"/);
  assert.match(styles, /\.inline-advanced > summary\s*\{[^}]*min-height:\s*48px[^}]*padding-inline:\s*18px/);
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
  assert.match(source, /api\.widgets\(\)/);
  assert.match(source, /api\.createWidget/);
  assert.match(source, /api\.widgetSessions/);
  assert.match(source, /distribution\?\.public_mcp_endpoint/);
  assert.match(source, /@dokosoko\/widget/);
  assert.match(source, /@dokosoko\/widget-backend/);
  assert.match(source, /distribution\?\.agent_setup\?\.public/);
  assert.match(source, /distribution\?\.agent_setup\?\.private/);
  assert.match(source, /Copy MCP button/);
  assert.match(source, /Copy \$\{kind\} MCP button/);
  for (const client of ["Codex", "Claude Code", "Cursor", "OpenCode"]) assert.match(source, new RegExp(`name: "${client}"`));
  for (const asset of ["codex.svg", "claude-code.svg", "cursor.svg", "opencode.svg"]) {
    assert.match(source, new RegExp(asset.replace(".", "\\.")));
    assert.match(await readFile(new URL(`../public/agent-client-icons/${asset}`, import.meta.url), "utf8"), /<svg\b/);
  }
  for (const placeholder of ["◉", "✳", "◆", "▣"]) assert.doesNotMatch(source, new RegExp(placeholder));
  assert.match(source, /disabled=\{!setup\.available\}/);
  assert.match(source, /Configure and activate customer identity/);
  assert.doesNotMatch(source, /widgets\/\$\{product\.id\}\/public\.js/);
  assert.doesNotMatch(source, /widgets\/\$\{product\.id\}\/private\.js/);
  assert.doesNotMatch(source, /dokosoko\.acme\.dev/);
  assert.match(packageJson, /"@headlessui\/react"/);
  assert.doesNotMatch(packageJson, /react-loading-skeleton/);
  await assert.rejects(access(new URL("../app/_sites-preview/SkeletonPreview.tsx", import.meta.url)));
});

test("keeps Agent access headings and setup cards concise", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");

  for (const removed of [
    "Control how authenticated and public agents reach your APIs and knowledge.",
    "Add a secret-free MCP connection button to your developer portal.",
    "Anonymous, read-only access to explicitly public resources.",
    "Customer access through the configured identity provider and browser OAuth.",
    "changing to public always requires confirmation.",
  ]) assert.doesNotMatch(source, new RegExp(removed.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
});

test("keeps page and panel headings concise across the console", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");

  for (const removed of [
    "Authenticated assistants embedded in your customers' applications.",
    "Install once, authenticate through your backend",
    "Vendor and deployment data is shown read-only",
    "Manage ingestion, review, and publication",
    "Choose an API to configure what developers and agents can use.",
    "Only unresolved actions appear here.",
    "Everything agents need to discover and invoke this API.",
    "Published actions and imported MCP tools available to this API.",
    "Vendor accounts this API can reach.",
    "Open an entry to inspect the immutable manifest.",
    "Runs, developer reports, and administrative changes in one place.",
    "Requested outcomes with deterministic completion evidence.",
    "Consent-gated submissions.",
    "Append-only security and configuration events.",
    "Use one default policy and add API-specific exceptions only when necessary.",
    "Import selected third-party MCP tools behind DokoSoko",
    "Publish reviewed HTTP actions and imported Stateless MCPv2 tools",
    "Immutable compatibility snapshots and scoped pins are retained",
    "Application data, encrypted objects, and schema state are monitored together.",
    "Select the primary provider and model for each job.",
    "Credentials are encrypted once per provider.",
  ]) assert.ok(!source.includes(removed), `redundant description should be removed: ${removed}`);

  for (const retained of [
    "Secrets are never recorded.",
    "Plaintext credentials are never listed.",
    "Suspended accounts and a disabled identity provider fail closed immediately.",
    "Root access is independent from vendor identities and always requires MFA.",
  ]) assert.ok(source.includes(retained), `safety guidance should remain: ${retained}`);
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

test("ships the optional customer-identity and delegated API contract", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  assert.match(source, /Customer identity contract/);
  assert.match(source, /eyebrow="Settings" title="Settings" action=/);
  for (const tab of ["identity", "connections", "reporting", "storage", "ai", "root"]) {
    assert.match(source, new RegExp(`settingsPath\\("${tab}"\\)`));
  }
  assert.doesNotMatch(source, /Shared configuration for identity, customer data, service connections, and security/);
  assert.match(source, /durable internal account/);
	assert.match(source, /Delegated API origin/);
	assert.match(source, /POST \/v1\/access\/evaluations/);
	assert.match(source, /Delegated user token/);
	assert.match(client, /customerAccounts/);
	assert.match(client, /updateCustomerAccount/);
	assert.match(client, /delegated_api_origin/);
	assert.match(client, /\/api\/v1\/identity-provider/);
	assert.doesNotMatch(client, /usage_hook_url|allowed_redirect_uris|entitlement_hook_url/);
});

test("ships consent-gated support reporting configuration and inbox", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

	assert.match(source, /Bug reports & feedback/);
	assert.doesNotMatch(source, /Configure consent-gated reporting and secure delivery/);
	assert.doesNotMatch(source, /Consent is enforced|Agents preview the sanitized report/);
	assert.match(source, /Backend connections/);
	assert.match(source, /independent of customer identity/);
	assert.match(source, /Delivery policies/);
  assert.match(source, />View<\/Button>/);
  assert.match(source, />Retry<\/Button>/);
  assert.match(source, /<SegmentedControl label="Filter activity"/);
  assert.match(client, /support-submissions/);
  assert.match(source, /Use as the default for all APIs/);
  assert.match(source, /\/v1\/support-submissions/);
	assert.match(source, /separately authenticated backend connection/);
	assert.match(source, /credentials rotate on the connection/);
	assert.match(client, /backendConnections/);
	assert.match(client, /createBackendConnectionCredential/);
  assert.match(client, /createSupportRoute/);
  assert.match(client, /replaceSupportRoute/);
  assert.match(client, /createSupportDeliveryAttempt/);
});

test("ships first-class API, reusable resource, and service-connection management", async () => {
  const source = await readFile(new URL("../app/components/ConsoleApp.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/api.ts", import.meta.url), "utf8");

  assert.match(source, /Add API/);
  assert.match(source, /Create reusable resource set/);
  assert.match(source, /Duplicate resource set/);
  assert.match(source, /Pin the current revision instead of following latest/);
  assert.match(source, /Create service type/);
  assert.doesNotMatch(source, /Connect vendor services once/);
  assert.match(source, /Create a private draft/);
  assert.match(source, /<span>API name<\/span>/);
  assert.match(source, /editingIntegration \? "Save changes" : "Create API"/);
  assert.match(source, /apiFamilyKeyFromName\(displayName\)/);
  assert.doesNotMatch(source, /Each API record represents one family and one version/);
  assert.match(source, /One fixed instance/);
  assert.match(source, /Multiple provider resources/);
  assert.match(source, /Allowed APIs/);
  assert.match(source, /API contract set/);
  assert.match(client, /createIntegration/);
  assert.match(client, /updateIntegration/);
  assert.match(client, /duplicateResourceSet/);
  assert.match(client, /createAccessDefinition/);
  assert.match(client, /createAccessConnection/);
  assert.doesNotMatch(client, /createProvider:/);
  assert.doesNotMatch(client, /projects:\s*async/);
});
