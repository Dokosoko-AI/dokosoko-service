import assert from "node:assert/strict";
import { access, readFile, readdir } from "node:fs/promises";
import test from "node:test";

import { clientSource, consoleSource, stylesSource } from "./source-surface.mjs";

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

function componentSource(source, startName, endName) {
  const start = source.indexOf(`function ${startName}`);
  const end = source.indexOf(`function ${endName}`, start);
  assert.notEqual(start, -1, `${startName} should exist`);
  assert.notEqual(end, -1, `${endName} should follow ${startName}`);
  return source.slice(start, end);
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

test("production client chunks exclude development fixture tenants", async () => {
  const directory = new URL("../dist/client/_next/static/chunks/", import.meta.url);
  const files = (await readdir(directory, { recursive: true })).filter((name) => name.endsWith(".js"));
  const source = (await Promise.all(files.map((name) => readFile(new URL(name, directory), "utf8")))).join("\n");
  assert.doesNotMatch(source, /prod_acme|org_acme|Acme Platform|pub_docs_seed|build_acme/);
});

test("keeps the rendered navigation destinations backed by canonical routes", async () => {
  const source = await consoleSource();
  const styles = await stylesSource();
  const routes = await readFile(new URL("../app/lib/console-routes.ts", import.meta.url), "utf8");
  assert.doesNotMatch(source, /Control plane/);
  assert.doesNotMatch(source, /className="environment"/);
  assert.doesNotMatch(styles, /\.environment\s*\{/);
  assert.match(source, /useState<ConsoleRoute>/);
  assert.doesNotMatch(source, /setSection/);
  assert.match(source, /window\.history\[method\]/);
  assert.match(source, /window\.history\.replaceState\(null, "", `\$\{next\.path\}/);
  assert.doesNotMatch(source, /replaceState\(null, "", `\$\{sectionPath\("overview"\)/);
  assert.doesNotMatch(source, /className="section-tabs"/);
  for (const path of ["/integrations", "/identity", "/tools", "/tools/connections", "/recipes", "/agent-access", "/operations/outbox", "/settings"]) {
    assert.ok(routes.includes(`"${path}"`), `${path} should be registered`);
  }
  for (const entity of ["integration", "resource-set", "source", "tool", "connection", "report", "audit-event", "root-user"]) {
    assert.ok(routes.includes(`| "${entity}"`) || routes.includes(`  ${entity}:`), `${entity} should be routable`);
  }
  assert.match(styles, /\.sidebar > nav/);
  assert.match(styles, /\.entity-detail-grid/);
  assert.match(styles, /\.agent-setup-grid/);
  assert.match(styles, /\.content > \.panel \+ \.panel \{ margin-top: var\(--space-section\); \}/);
});

test("gives AI configuration a dedicated, guarded settings workspace", async () => {
  const source = await consoleSource();
  const client = await clientSource();
  const styles = await stylesSource();
  const routes = await readFile(new URL("../app/lib/console-routes.ts", import.meta.url), "utf8");
  const api = await readFile(new URL("../api/openapi.yaml", import.meta.url), "utf8");

  assert.match(routes, /label: "AI configuration"/);
  assert.match(routes, /settingsPath/);
  assert.match(source, /title="AI configuration"/);
  assert.doesNotMatch(source, /Two models, two clear jobs|AI ready|workloads enabled/);
  assert.match(source, /title="Workload"/);
  assert.match(source, /title="Workflow prompts"/);
  assert.match(source, /title="Providers"/);
  assert.doesNotMatch(source, /Choose one strong model for analysis|Fetching, retrieval, authorization/);
  assert.match(source, /OpenAI-compatible/);
  assert.doesNotMatch(source, /Mandatory AI safeguards|No model tools|Grounded output/);
  assert.match(source, /Citations required/);
  assert.match(source, /<strong>Connect \{provider\.name\}<\/strong>/);
  for (const provider of ["OpenAI", "Google", "Anthropic", "DigitalOcean", "xAI", "DeepSeek"]) assert.ok(source.includes(`name: "${provider}"`));
  assert.match(source, /name: "Other OpenAPI compatible providers"/);
  assert.doesNotMatch(source, /Already connected · manage settings/);
  assert.match(source, /function AIProviderLogo/);
  for (const provider of ["openai", "google", "anthropic", "digitalocean", "xai", "deepseek"]) {
    assert.match(source, new RegExp(`provider === "${provider}"`));
  }
  assert.match(source, /Backup provider/);
  assert.match(source, /Analysis backup model/);
  assert.doesNotMatch(source, /Assistant backup model|name: "Assistant"/);
  for (const key of ["integration.analysis", "recipe.brief", "recipe.authoring", "recipe.review"]) assert.match(source, new RegExp(key.replace(".", "\\.")));
  assert.match(source, /Reset to safe default/);
  assert.match(source, /Save new version/);
  assert.match(source, /built-in safety policy cannot be edited or disabled/);
  assert.doesNotMatch(source, /defaultInstructions|default_instructions/);
  assert.doesNotMatch(source, /<Badge[^>]*>Native<\/Badge>|<Badge[^>]*>Custom<\/Badge>/);
  assert.match(source, /title=\{`Configure \$\{/);
  assert.match(source, /Leave blank to keep the stored credential/);
  assert.doesNotMatch(source, /title="Configure LLM profile"/);
  assert.doesNotMatch(styles, /\.ai-settings-hero|\.ai-hero-mark|\.ai-hero-stat/);
  assert.match(styles, /\.ai-table-panel/);
  assert.match(styles, /\.ai-provider-suggestions/);
  assert.match(styles, /\.ai-provider-logo/);
  assert.match(api, /Removed legacy endpoint[\s\S]*'410':/);
  assert.doesNotMatch(api, /^ {4}(?:SaveLLMProfileRequest|LLMHardening|LLMProfile|LLMProfileList):/m);
  assert.match(api, /AIWorkload:\n\s+type: string\n\s+enum:\n\s+- analysis/);
  assert.match(api, /\/api\/v1\/products\/\{product_id\}\/ai-prompts:/);
  assert.match(api, /operationId: listAIWorkflowPrompts/);
  assert.match(api, /operationId: saveAIWorkflowPromptOverride/);
  assert.match(api, /operationId: resetAIWorkflowPrompt/);
  assert.match(api, /AIWorkflowPromptKey:[\s\S]*- integration\.analysis[\s\S]*- recipe\.brief[\s\S]*- recipe\.authoring[\s\S]*- recipe\.review/);
  assert.doesNotMatch(api, /default_instructions:/);
  assert.match(client, /aiPrompts: async/);
  assert.match(client, /saveAIPrompt:[\s\S]*JSON\.stringify\(\{ instructions, revision \}\)/);
  assert.match(client, /resetAIPrompt:[\s\S]*JSON\.stringify\(\{ revision \}\)/);
  assert.match(api, /provider_role:\n\s+type: string\n\s+enum:\n\s+- primary\n\s+- backup/);
  assert.match(api, /endpoint:\n\s+type: string\n\s+format: uri\n\s+description: Fixed HTTPS provider origin/);
  for (const provider of ["digitalocean", "xai", "deepseek"]) assert.match(api, new RegExp(provider));
});

test("ships one evidence-to-recipe review workflow", async () => {
  const source = await consoleSource();
  assert.doesNotMatch(source, /Turn verified integration evidence into implementation guides/);
  const styles = await stylesSource();
  const routes = await readFile(new URL("../app/lib/console-routes.ts", import.meta.url), "utf8");
  const client = await clientSource();
  const api = await readFile(new URL("../api/openapi.yaml", import.meta.url), "utf8");

  assert.match(routes, /recipes: "\/recipes"/);
  for (const label of ["Create recipe", "Generate from evidence", "Setup and usage recipes", "Approve", "Publish", "Rework"]) assert.match(source, new RegExp(label));
  assert.match(source, /What should this recipe help a developer accomplish/);
  assert.match(source, /aria-label="Recipe API"/);
  assert.match(source, /activeRecipeIntegrationID\(integrations, selectedIntegrationID\)/);
  assert.doesNotMatch(source, /disabled=\{busy \|\| analyses\.length/);
  assert.match(source, /disabled=\{busy \|\| !activeIntegrationID\}/);
  assert.match(source, /visibleRecipes = activeIntegrationID[\s\S]*recipeMatchesIntegration\(recipe, activeIntegrationID\)/);
  assert.match(source, /unscopedOrInvalidRecipes\.map\(renderRecipe\)/);
  assert.match(source, /disabled=\{busy \|\| invalidScope\} onClick=\{\(\) => on(?:Edit|Rework|Approve|Publish)/);
  assert.match(source, /Reviewed guidance grounded in published documentation and API evidence/);
  assert.match(source, /What should the AI rework/);
  assert.doesNotMatch(source, /Start from evidence, not a blank prompt|Review queue|Most used · 30 days/);
  assert.doesNotMatch(styles, /\.recipe-library-row|\.recipe-editor-layout|\.recipe-markdown-input/);
  assert.match(client, /createRecipe/);
  assert.match(client, /integrationID \? \{ prompt, integration_id: integrationID \} : \{ prompt \}/);
  assert.match(api, /\/api\/v1\/products\/\{product_id\}\/recipes:/);
  assert.match(api, /operationId: createRecipe[\s\S]*integration_id:[\s\S]*omission retains deployment-wide API compatibility/);
  assert.match(api, /- resources\/list/);
  assert.match(api, /- resources\/read/);
});

test("keeps read-only integration evidence gaps visible", async () => {
  const source = await consoleSource();
  const gaps = await readFile(new URL("../app/components/integrations/IntegrationEvidenceGaps.tsx", import.meta.url), "utf8");
  const guide = await readFile(new URL("../app/components/integrations/IntegrationSetupGuide.tsx", import.meta.url), "utf8");

  assert.match(source, /<IntegrationEvidenceGaps unknowns=\{selectedAnalysis\?\.unknowns \?\? \[\]\}/);
  assert.match(guide, /<IntegrationEvidenceGaps unknowns=\{analysis\.unknowns\}/);
  assert.match(gaps, /unknown\.question/);
  assert.match(gaps, /unknown\.why/);
  assert.match(gaps, /Evidence gaps block generation/);
  assert.match(gaps, /then generate again to run a fresh analysis/);
  assert.match(gaps, /<section className="integration-evidence-gaps" aria-labelledby=\{headingID\}>/);
  assert.match(gaps, /<h3 id=\{headingID\}>/);
  assert.doesNotMatch(gaps, /role=\{blockingCount/);
});

test("refreshes scoped evidence before recipe generation without duplicating active analysis", async () => {
  const source = await consoleSource();
  const domain = await readFile(new URL("../app/lib/console-domain.ts", import.meta.url), "utf8");
  assert.match(source, /recipeAnalysisIsFreshlyRunning\(latestAnalysis\)/);
  assert.match(source, /const analysis = await api\.analyseIntegration\(product\.id, integrationID\)/);
  assert.match(source, /analysis\.state === "review" && analysisMatchesIntegration\(analysis, activeIntegrationID\)/);
  assert.match(domain, /analysis\?\.state !== "running"/);
  assert.match(domain, /scopes\.length === 1 && scopes\[0\]\.resource_id === integrationID/);
});

test("uses an API directory and a complete onboarding workspace", async () => {
  const source = await consoleSource();
  const styles = await stylesSource();
  const routes = await readFile(new URL("../app/lib/console-routes.ts", import.meta.url), "utf8");
  const integrationNavigation = await readFile(new URL("../app/components/integrations/IntegrationNavigation.tsx", import.meta.url), "utf8");
  const quickStart = await readFile(new URL("../app/components/integrations/IntegrationQuickStart.tsx", import.meta.url), "utf8");
  const runtimeAccess = await readFile(new URL("../app/components/integrations/IntegrationRuntimeAccess.tsx", import.meta.url), "utf8");
  const client = await clientSource();
  const directory = componentSource(source, "IntegrationDirectoryView", "IntegrationSwitcher");
  const workspace = componentSource(source, "IntegrationWorkspaceView", "AuthorizationPolicyWorkspace");
  const integrationTabs = routes.slice(routes.indexOf("export const INTEGRATION_TABS"), routes.indexOf("export const INTEGRATION_RESOURCE_TABS"));

  assert.match(integrationTabs, /export const INTEGRATION_TABS:[^=]+=\s*\[\s*\{ id: "overview", label: "Quick Start" \},\s*\{ id: "documentation", label: "Documentation" \},\s*\{ id: "access", label: "Keys & Access" \},\s*\{ id: "tools", label: "Tools" \},\s*\{ id: "test", label: "Test" \},\s*\{ id: "history", label: "History" \},\s*\];/);
  for (const removed of ["authorization", "recipes", "delivery", "resources"]) {
    assert.ok(!integrationTabs.includes(`id: "${removed}"`), `${removed} should not remain an API tab`);
    assert.doesNotMatch(workspace, new RegExp(`activeTab === "${removed}"`));
    assert.doesNotMatch(workspace, new RegExp(`integrationPath\\(integration\\.id,\\s*"${removed}"`));
  }
  for (const label of ["Documentation", "API contracts", "SDKs"]) {
    assert.ok(routes.includes(`label: "${label}"`), `${label} resource sub-tab should be registered`);
  }
  for (const removed of ["Tools & hooks", "label: \"Usage\"", "label: \"Support\"", "label: \"Revisions\""]) assert.ok(!routes.includes(removed));
  assert.match(source, /<IntegrationNavigation integrationID=\{integration\.id\} integrationName=\{integration\.display_name\}/);
  assert.match(integrationNavigation, /<PageTabs label=\{`\$\{integrationName\} sections`\}>/);
  assert.match(integrationNavigation, /INTEGRATION_PRIMARY_TABS\.map/);
  assert.match(integrationNavigation, /<Dropdown>[\s\S]*More[\s\S]*historyPath/);
  assert.match(quickStart, /Get your API ready/);
  assert.match(quickStart, /Optional setup and API details/);
  assert.match(quickStart, /const nextStep = steps\.findIndex/);
  assert.match(source, /label: "Configure runtime access"[^\n]*path: integrationPath\(integration\.id, "access"\)/);
  assert.match(source, /label: "Expose tools"[^\n]*path: integrationPath\(integration\.id, "tools"\)/);
  assert.match(source, /IntegrationDirectoryView/);
  assert.match(source, /IntegrationWorkspaceView/);
  assert.match(source, /integration\.lifecycle !== "retired"/);
  assert.match(source, /Show retired \(\$\{retiredCount\}\)/);
  assert.match(source, /className="retired-directory-toggle"/);
  assert.match(source, /aria-pressed=\{showRetired\}/);
  assert.match(source, /<DataTableEmpty columns=\{5\}>/);
  assert.match(source, /Published history/);
  assert.match(source, /Switch API/);
  assert.doesNotMatch(source, /description=\{integration\.description \|\| undefined\}/);
  assert.doesNotMatch(source, /Ingest → crawl → review → publish → attach\./);
  assert.doesNotMatch(source, /Only unresolved actions appear here/);
  assert.match(source, /Customer identity/);
  assert.match(runtimeAccess, /Credential lifecycle and connection metadata — Advanced/);
  assert.doesNotMatch(directory, /No changes|Filter by API family|Filter by setup state|integration-family-heading|groupedIntegrations/);
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
  assert.match(client, /integrationSDKs/);
  assert.doesNotMatch(client, /setIntegrationAccessConnections|setIntegrationSupportRoute/);
});

test("uses exact API-owned SDK references without a package lifecycle", async () => {
  const source = await consoleSource();
  const client = await clientSource();
  const openapi = await readFile(new URL("../api/openapi.yaml", import.meta.url), "utf8");

  assert.match(source, /Exact API-owned SDK references/);
  assert.match(source, /There is no global package catalogue or release workflow/);
  assert.match(source, /Version ranges and latest tags are rejected/);
  assert.match(client, /integrationSDKs/);
  assert.match(client, /createIntegrationSDK/);
  assert.match(client, /replaceIntegrationSDK/);
  assert.match(client, /deleteIntegrationSDK/);
  assert.doesNotMatch(client, /packageArtifact|packageRelease|retirePackage/);
  assert.match(openapi, /\/api\/v1\/integrations\/\{integration_id\}\/sdks:/);
  assert.match(openapi, /One immutable version; ranges and latest are rejected/);
});

test("uses a focused deployment tool catalog and token-based MCP connections", async () => {
  const source = await consoleSource();
  const styles = await stylesSource();
  const catalog = componentSource(source, "ToolsView", "SettingsTabs");
  const connections = componentSource(source, "MCPConnectionsView", "ToolsView");

  assert.match(source, /function ToolsWorkspaceTabs/);
  assert.match(source, /path=\{sectionPath\("tools"\)\}[\s\S]*?>Catalog<\/ConsoleLink>/);
  assert.match(source, /path=\{sectionPath\("connections"\)\}[\s\S]*?>MCP connections<\/ConsoleLink>/);
  assert.match(catalog, /<ToolsWorkspaceTabs active="catalog"/);
  assert.match(connections, /<ToolsWorkspaceTabs active="connections"/);

  assert.match(catalog, /<DataTable label="Tool catalog">/);
  assert.match(catalog, /<DataTableHeader className="tool-columns"><span>Tool<\/span><span>Backend<\/span><span>Policy<\/span><span>State<\/span><span>Open<\/span><\/DataTableHeader>/);
  assert.match(catalog, /toolIsCommon\(tool\)/);
  assert.match(catalog, /Create HTTP tool/);
  assert.match(catalog, /title="Native tools"/);
  assert.match(catalog, /Reviewed in-process capabilities registered by the service/);
  assert.match(catalog, /onSetNativePluginEnabled/);
  assert.match(catalog, /entityPath\("tool", tool\.id\)/);
  assert.match(catalog, /<DataTableEmpty columns=\{5\}>/);
  assert.match(styles, /\.table-row/);
  assert.doesNotMatch(catalog, /api\.publishTool|MCP Tool Editor|Edit & dry-run|Run contract test/);
  assert.doesNotMatch(catalog, /AuthorizationPolicyWorkspace|API action policies|Grant registry/);

  assert.match(connections, /title="MCP connections"/);
  assert.match(connections, /fixed endpoint uses one encrypted access token/);
  assert.match(connections, /signed user-identity envelope/);
  assert.match(connections, /Inspect tools/);
});

test("keeps API authorization policy authoring in API Access and out of root Tools and Identity", async () => {
  const source = await consoleSource();
  const client = await clientSource();
  const identitySetup = await readFile(new URL("../app/components/OIDCIdentitySetup.tsx", import.meta.url), "utf8");
  const policyWorkspace = componentSource(source, "AuthorizationPolicyWorkspace", "IntegrationToolsWorkspace");
  const toolsView = componentSource(source, "ToolsView", "SettingsTabs");
  const integrationWorkspace = componentSource(source, "IntegrationWorkspaceView", "AuthorizationPolicyWorkspace");

  for (const capability of ["grantDefinitions", "createGrantDefinition", "updateGrantDefinition", "authorizationPoints", "createAuthorizationPoint", "updateAuthorizationPoint"]) {
    assert.match(client, new RegExp(`\\b${capability}\\b`));
  }
  assert.match(client, /\/api\/v1\/grant-definitions/);
  assert.doesNotMatch(client, /APIAuthorizationSimulation|simulateAuthorizationPoint|authorization-points[^\n]*\/simulate|simulateIntegrationAuthorization/);
  assert.doesNotMatch(source, /APIAuthorizationSimulation|simulateAuthorizationPoint/);

  assert.match(source, /\{section === "identity" && <OIDCIdentitySetup/);
  assert.match(source, /\{section === "tools" && <ToolsView/);
  assert.doesNotMatch(toolsView, /AuthorizationPolicyWorkspace|API action policies|Grant registry/);
  assert.match(integrationWorkspace, /activeTab === "access"[\s\S]*<AuthorizationPolicyWorkspace integration=\{integration\} onMessage=\{onMessage\} \/>/);
  for (const label of ["Action policies", "Grant registry", "API action policies"]) {
    assert.ok(policyWorkspace.includes(label), `${label} should be present in the API Access policy workspace`);
  }
  assert.match(policyWorkspace, /Deployment grant registry — Advanced/);
  assert.match(policyWorkspace, /api\.grantDefinitions\(\)/);
  assert.match(policyWorkspace, /api\.authorizationPoints\(integrationID\)/);
  assert.match(policyWorkspace, /api\.updateAuthorizationPoint\(integration\.id, editingPoint\.id/);
  assert.match(policyWorkspace, /api\.createAuthorizationPoint\(integration\.id, input\)/);
  assert.doesNotMatch(policyWorkspace, /API policy scope|selectedIntegration/);
  assert.doesNotMatch(policyWorkspace, /Policy simulator|Simulation only|simulateAuthorizationPoint/);
  assert.doesNotMatch(identitySetup, /grantDefinitions|authorizationPoints|Grant registry|API action policies|Policy simulator|simulateAuthorizationPoint|\bcustomerAccounts\b|APICustomerAccount/);
  assert.match(source, /label: "Configure customer access"[^\n]*path: integrationPath\(integration\.id, "access"\)/);
  assert.match(source, /integrationValidationPath\(integration\.id, tab\)/);
  assert.match(await readFile(new URL("../app/lib/console-routes.ts", import.meta.url), "utf8"), /case "authorization":[\s\S]*case "access": return integrationPath\(uid, "access"\)/);
  assert.match(source, /publishValidationCodes\.has\("authorization_missing"\)/);
});

test("keeps reusable tool authoring in the deployment tool builder and detail", async () => {
  const source = await consoleSource();
  const client = await clientSource();
  const builder = await readFile(new URL("../app/components/ToolBuilderView.tsx", import.meta.url), "utf8");
  const styles = await stylesSource();
  const openapi = await readFile(new URL("../api/openapi.yaml", import.meta.url), "utf8");
  const liveEvidence = componentSource(source, "ToolLiveTestEvidence", "ToolDetailView");
  const detail = componentSource(source, "ToolDetailView", "ConsoleNotFoundView");

  assert.match(client, /tool_revision: number/);
  assert.match(client, /tool: \(productID: string, toolID: string\)/);
  assert.match(client, /cloneTool: \(productID: string, toolID: string, revision: number, namespace: string, name: string, credential = ""\)[\s\S]*JSON\.stringify\(\{ revision, namespace, name, \.\.\.\(credential \? \{ credential \} : \{\}\) \}\)/);
  for (const operation of ["proposeToolDraft", "importToolDraft", "validateToolDraft", "analyseToolDraft"]) {
    assert.match(client, new RegExp(`\\b${operation}\\b`));
  }
  assert.match(client, /\/dry-run`[\s\S]*JSON\.stringify\(\{ arguments: args \}\)/);
  assert.match(client, /createToolTestConfirmation:[\s\S]*\/test-confirmations`[\s\S]*JSON\.stringify\(input\)/);
  assert.match(client, /runToolTest:[\s\S]*\/test-runs`[\s\S]*JSON\.stringify\(input\)/);
  assert.match(client, /analyseToolTestRun:[\s\S]*\/test-runs\/\$\{encodeURIComponent\(runID\)\}\/analyse`[\s\S]*JSON\.stringify/);
  assert.match(client, /evidence_hash: string/);
  assert.match(client, /Server-computed consent binding; the browser never re-serializes evidence/);
  assert.doesNotMatch(client, /crypto\.subtle/);
  for (const field of ["phase", "code", "message", "instance_path", "schema_path"]) assert.match(client, new RegExp(`${field}\\??:`));
  assert.match(client, /\/retire`[\s\S]*JSON\.stringify\(\{ revision \}\)/);
  assert.match(client, /state: "draft" \| "published" \| "retired"/);

  assert.match(source, /const TOOL_DETAIL_TABS:[\s\S]*?"overview"[\s\S]*?"contract"[\s\S]*?"execution"[\s\S]*?"authorization"[\s\S]*?"tests"[\s\S]*?"usage"[\s\S]*?"history"/);
  assert.match(source, /<ToolBuilderView[\s\S]*aiAvailable=/);
  for (const mode of ["AI assist", "Import", "Manual"]) assert.ok(builder.includes(mode), `${mode} should be present in the tool builder`);
  assert.match(builder, /credential_will_be_supplied/);
  assert.match(builder, /Accept change/);
  assert.match(builder, /Check draft/);
  assert.match(client, /TOOL_BUILDER_CHAT_LIMITS[\s\S]*maxMessages: 12[\s\S]*maxMessageBytes: 2_048[\s\S]*maxHistoryBytes: 12_288/);
  assert.match(client, /history\?: APIToolBuilderChatMessage\[\]/);
  assert.match(builder, /chatHistory\.map\(\(message, index\)/);
  assert.match(builder, /role="log"[\s\S]*aria-live="polite"[\s\S]*aria-relevant="additions"/);
  assert.match(builder, /history \}[\s\S]*role: "user"[\s\S]*role: "assistant"/);
  assert.match(builder, /const followUpDraft = useMemo\(\(\) => toolBuilderFollowUpDraft\([\s\S]*proposal\?\.draft \?\? null[\s\S]*proposalDecisions[\s\S]*proposalStale/);
  assert.match(builder, /builderContext\(followUpDraft\)[\s\S]*setActiveProposal\("ai", result, assistanceDraft, followUpDraft\)/);
  assert.match(builder, /current non-secret draft[\s\S]*configured Analysis provider[\s\S]*separate credential field is excluded/);
  assert.match(builder, /non-secret preview composed from the selected API service connection[\s\S]*selected connection, host, authentication, and credential boundary remain locked/);
  assert.match(builder, /keep secrets out of every field[\s\S]*cannot prove arbitrary text is secret-free[\s\S]*cannot accept its own diffs[\s\S]*save, publish, bind, or call the endpoint/);
  assert.doesNotMatch(builder, /containsCredentialMaterial\(importSource\)/);
  assert.match(builder, /const importFindings = result\.findings \?\? \[\]/);
  assert.match(builder, /Embedded credentials are detected and stripped; they never populate \{apiScoped \? "this API tool" : "the separate secret field"\}/);
  assert.match(builder, /initialProposal\?: APIToolBuilderProposal \| null/);
  assert.match(builder, /source: "live-test"/);
  assert.match(builder, /Live-test AI proposal/);
  assert.match(builder, /const draftVersionRef = useRef\(seededProposal \? 1 : 0\)/);
  for (const label of ["Assistant", "Analysis", "Validation"]) assert.match(builder, new RegExp(`acceptCurrentDraftResponse\\(requestVersion, "${label}"\\)`));
  assert.match(builder, /acceptImportResponse\(requestVersion, importInputVersion\)/);
  assert.match(builder, /acceptCurrentDraftResponse\(draftVersion, "Import"\)/);
  assert.match(builder, /const formLocked = !editable \|\| busy === "save"/);
  assert.match(builder, /const commonPersistence = \{[\s\S]*?endpoint: local\.draft\.endpoint[\s\S]*?upstream_auth: local\.draft\.upstream_auth/);
  assert.match(builder, /const runtimePersistence = runtimeContext \? \{[\s\S]*?runtime_service_connection_id: runtimeContext\.runtime_service_connection_id[\s\S]*?http_path: runtimeContext\.http_path/);
  assert.match(builder, /const persisted = \{[\s\S]*?description: local\.draft\.description[\s\S]*?\.\.\.\(runtimePersistence \?\? commonPersistence\)[\s\S]*?\};/);
  assert.match(builder, /api\.updateTool\(product\.id, tool\.id, \{ \.\.\.persisted, revision: tool\.revision \}\)/);
  assert.match(builder, /api\.createTool\(product\.id, \{ \.\.\.persisted, \.\.\.\(runtimeContext \?\? \{ scope: "common" as const \}\), organisation_id: product\.organisation_id, namespace: local\.draft\.namespace, name: local\.draft\.name \}\)/);
  assert.doesNotMatch(builder.match(/const persisted = \{[\s\S]*?\n {6}\};/)?.[0] ?? "", /namespace:|name:/);
  assert.match(builder, /disabled=\{formLocked\}/);
  assert.match(builder, /onDirtyChange\?: \(dirty: boolean\) => void/);
  assert.match(builder, /window\.addEventListener\("beforeunload", warn\)/);
  assert.match(builder, /onDirtyChange\?\.\(dirty\)/);
  assert.doesNotMatch(builder, /window\.confirm/);
  assert.match(source, /const toolBuilderDirtyRef = useRef\(false\)/);
  assert.match(source, /const confirmToolBuilderNavigation = useCallback\(\(nextPath: string\) =>/);
  assert.match(source, /window\.history\.pushState\(null, "", browserRouteURL\(current\.path\)\)/);
  assert.match(source, /onDirtyChange=\{onToolBuilderDirtyChange\}/);
  assert.match(styles, /\.tool-builder-chat-transcript[\s\S]*overflow-y: auto/);
  assert.match(styles, /\.tool-detail-section \.integration-health-check \{ grid-template-columns: 30px minmax\(0, 1fr\) auto;/);
  assert.match(detail, /api\.tool\(productID, toolID\)/);
  assert.match(detail, /api\.publishTool\(productID, currentTool\.id, currentTool\.revision\)/);
  assert.match(detail, /api\.cloneTool\(productID, currentTool\.id, currentTool\.revision, cloneNamespace\.trim\(\), cloneName\.trim\(\), cloneCredential\)/);
  assert.match(detail, /api\.dryRunTool\(productID, currentTool\.id, argumentsObject\)/);
  assert.match(detail, /api\.runToolTest\(productID, currentTool\.id/);
  assert.match(detail, /api\.createToolTestConfirmation\(productID, currentTool\.id,[\s\S]*?await executeLiveToolTest\(pendingTestArguments, requestVersion, idempotencyKey, confirmation\.confirmation_nonce\)/);
  assert.match(detail, /revision: currentTool\.revision,[\s\S]*arguments: pendingTestArguments,[\s\S]*typed_tool_name: testConfirmationName,[\s\S]*acknowledge_side_effects: testSideEffectsAcknowledged/);
  assert.match(detail, /confirmation\.tool_id !== currentTool\.id \|\| confirmation\.tool_revision !== currentTool\.revision/);
  assert.match(detail, /api\.retireTool\(productID, currentTool\.id, currentTool\.revision\)/);
  assert.match(detail, /const \[activeTool, setActiveTool\] = useState<APITool \| null>\(null\)/);
  assert.match(detail, /role="tablist"[\s\S]*?role="tab"[\s\S]*?aria-selected/);
  assert.match(detail, /role="tabpanel"[\s\S]*?aria-labelledby="tool-tab-/);
  assert.match(detail, /activeTool\.backend_kind === "http" && \(owningIntegration \? <Button[\s\S]*?Create another API tool[\s\S]*?: <Button[\s\S]*?Clone as new tool/);
  assert.match(detail, /backendKind === "http" && !apiOwned && <Dialog open=\{cloneOpen\}/);
  assert.match(detail, /Edit in builder/);
  assert.match(detail, /<textarea readOnly value=\{description\}/);
  assert.match(detail, /<textarea spellCheck=\{false\} readOnly value=\{inputSchema\}/);
  assert.doesNotMatch(detail, /saveToolDraft|draftPayload|>Save draft<|api\.updateTool\(/);
  for (const label of ["Agent contract", "Execution", "Baseline authorization", "Contract check", "Live upstream test", "Current API configuration", "Tool activity", "Clone as a new tool", "Retire"]) {
    assert.ok(detail.includes(label), `${label} should be present in the deployment tool detail`);
  }
  assert.match(detail, /Imported MCP tools must be exercised through their reviewed MCP connection/);
  assert.match(detail, /const liveTestUnsupported = backendKind !== "http"/);
  assert.match(detail, /Native tools are source-managed and must be exercised through an authorized Private MCP client/);
  assert.match(detail, /activeTool\.backend_kind === "native" \? "Native plugin"/);
  assert.match(detail, /Execution requires the active plugin instance and exact manifest and tool hashes pinned by this revision/);
  assert.match(detail, /const effectiveAuthenticationType = runtimeRevision\?\.authentication_type \?\? upstreamAuthType/);
  assert.match(detail, /const delegatedOAuthLiveTest = effectiveAuthenticationType === "delegated_oauth"/);
  assert.match(detail, /Administrator live tests cannot accept an end-user delegated OAuth token\. Stage 2 is disabled here and no upstream request will be made/);
  assert.match(detail, /Stage 2 · Unavailable for Delegated OAuth\. Administrator live tests do not accept an end-user token, and no upstream request will be made/);
  assert.match(detail, /delegatedOAuthLiveTest \? "Live test unavailable"/);
  assert.match(detail, /action=\{!liveTestUnsupported && <Button/);
  assert.match(detail, /testConfirmationName !== fullToolName/);
  assert.match(detail, /I understand this test can cause real upstream side effects/);
  assert.match(detail, /const testConfirmationRequired = mutationTest \|\| currentPolicy\.confirmationRequired/);
  assert.match(detail, /const testIdempotencyRequired = mutationTest && currentPolicy\.idempotencyRequired/);
  assert.match(detail, /testConfirmationRequired && !liveTestUnsupported && !delegatedOAuthLiveTest/);
  assert.match(detail, /testIdempotencyRequired && !liveTestUnsupported && !delegatedOAuthLiveTest/);
  assert.match(detail, /stored confirmation policy/);
  assert.match(detail, /validToolTestIdempotencyKey\(testIdempotencyKey\)/);
  assert.match(detail, /16–200 visible ASCII characters/);
  assert.match(detail, /const \[liveTestResult, setLiveTestResult\] = useState<APIToolTestRun \| null>\(null\)/);
  assert.match(liveEvidence, /run\.request_shape/);
  assert.match(liveEvidence, /run\.response_shape/);
  assert.match(liveEvidence, /Raw bodies, headers, field values, and credentials are never returned or displayed/);
  assert.match(liveEvidence, /Nothing is shared until you review this boundary and explicitly consent/);
  assert.match(liveEvidence, /Shapes containing only schema-declared property names, JSON types, and array lengths/);
  assert.match(liveEvidence, /value-free enum cardinality and const-presence markers/);
  assert.match(liveEvidence, /value-free literal-constraint markers/);
  assert.match(liveEvidence, /raw or literal values\/bodies/);
  assert.match(liveEvidence, /Unexpected upstream property names, diagnostic paths, headers, credentials/);
  assert.match(liveEvidence, /Raw values or bodies, response content, request arguments, examples, stored descriptions/);
  assert.match(liveEvidence, /Destination origin, literal path, query, evidence hash, tool\/run\/product IDs, actor, or request ID/);
  assert.match(liveEvidence, /I explicitly consent to send this sanitized evidence and bounded conversation/);
  assert.match(liveEvidence, /TOOL_TEST_ANALYSIS_CHAT_LIMITS\.maxMessageBytes/);
  assert.match(liveEvidence, /new TextEncoder\(\)\.encode\(question\.trim\(\)\)\.byteLength/);
  assert.match(liveEvidence, /boundedToolTestAnalysisHistory/);
  assert.match(liveEvidence, /result\.proposal\.base_tool_id !== tool\.id[\s\S]*result\.proposal\.base_revision !== run\.tool_revision[\s\S]*result\.proposal\.requires_clone !== \(tool\.state === "published"\)/);
  assert.match(liveEvidence, /Clone & review proposal/);
  assert.match(source, /reviewToolTestProposal/);
  assert.match(source, /initialProposal=\{activeToolBuilderSeed\}/);
  assert.match(detail, /pendingCloneProposal/);
  assert.match(detail, /onReviewProposal\(cloned, proposalToReview\)/);
  assert.match(openapi, /\/api\/v1\/products\/\{product_id\}\/tools\/\{tool_id\}\/test-runs\/\{run_id\}\/analyse:/);
  assert.match(openapi, /value-free shapes projected to schema-declared property names[\s\S]*Unexpected upstream property names are excluded/);
  assert.match(openapi, /ToolTestAnalysisRequest:/);
  assert.match(openapi, /consent_to_analysis_provider:\n\s+type: boolean\n\s+const: true/);
  assert.match(openapi, /evidence_hash:[\s\S]*?never sent to the Analysis provider/);
  assert.match(openapi, /stored authorization policy[\s\S]*?requires explicit confirmation/);
  assert.match(openapi, /delegated_oauth is retained only on a controlled authorization-unavailable failure/);
  assert.match(openapi, /Delegated OAuth is not eligible for an[\s\S]*?administrator live upstream call/);
  assert.match(detail, /network_call_performed/);
  assert.match(detail, /bindings\.filter\(\(binding\) => binding\.tool_id === toolID\)/);
  assert.doesNotMatch(detail, /bindings\.filter\(\(binding\) => binding\.tool_id === toolID && binding\.tool_revision/);
  assert.match(detail, /path=\{owningIntegration \? integrationPath\(owningIntegration\.id, "tools"\) : sectionPath\("tools"\)\}[\s\S]*?owningIntegration \? `\$\{owningIntegration\.display_name\} tools` : "Common tools"/);
  assert.doesNotMatch(detail, /MCP Tool Editor|Edit & dry-run|Bounded dry-run|Run dry-run/);
});

test("splits API tools into built-ins, API-owned definitions, and attached common tools", async () => {
  const source = await consoleSource();
  const client = await clientSource();
  const bindingWorkspace = componentSource(source, "IntegrationToolsWorkspace", "IntegrationTestWorkspace");

  assert.match(client, /tools: Array<\{ tool_id: string; revision: number; authorization_point_id: string; authorization_point_revision: number \}>/);
  assert.match(bindingWorkspace, /api\.integrationToolBindings\(integration\.id\)/);
  assert.match(bindingWorkspace, /api\.authorizationPoints\(integration\.id\)/);
  assert.match(bindingWorkspace, /api\.setIntegrationToolBindings\(integration\.id/);
  assert.match(bindingWorkspace, /binding\.tool_revision/);
  assert.match(bindingWorkspace, /tool_id, revision: selection\.revision, authorization_point_id: selection\.authorizationPointID, authorization_point_revision: selection\.authorizationPointRevision/);
  assert.match(bindingWorkspace, /point && point\.state === "active" && point\.revision === selection\.authorizationPointRevision/);
  assert.match(bindingWorkspace, /const availableTools = tools\.filter\(\(tool\) => tool\.state === "published" && !tool\.upstream_drifted && !bindingSelection\[tool\.id\] && toolCanAttachToIntegration\(tool, integration\.id\)\)/);
  assert.match(bindingWorkspace, /const toolCurrent = Boolean\(tool && tool\.state === "published" && !tool\.upstream_drifted && tool\.revision === selection\.revision && toolCanAttachToIntegration\(tool, integration\.id\)\)/);
  assert.match(bindingWorkspace, /sectionPath\("tools"\)/);
  assert.match(bindingWorkspace, /integrationToolBuilderPath\(integration\.id\)/);
  assert.match(bindingWorkspace, /Create API tool/);
  assert.match(bindingWorkspace, /Save API bindings/);
  assert.match(bindingWorkspace, /activePoints\.length === 0 && !bindingsLoading && !bindingsUnavailable/);
  assert.match(bindingWorkspace, /aria-label=\{`Remove \$\{tool/);
  assert.match(bindingWorkspace, /id="save-api-bindings"/);
  for (const label of ["Built-in tools", "Knowledge", "Tools for this API", "API tools", "Attached common tools", "Owned by this API", "Common tools"]) {
    assert.ok(bindingWorkspace.includes(label), `${label} should distinguish API-local and reusable tools`);
  }
  assert.match(bindingWorkspace, /<code>\{integration\.family_key\}\.knowledge\.search<\/code>/);
  assert.match(bindingWorkspace, /requires attached reviewed documentation/);
  assert.match(bindingWorkspace, /They are not custom Tool records and do not need manual attachment/);
  assert.match(bindingWorkspace, /partitionIntegrationTools\(tools, boundToolIDs, integration\.id\)/);
  assert.match(bindingWorkspace, /toolCanAttachToIntegration\(tool, integration\.id\)/);
  assert.match(bindingWorkspace, /owned by another API/);
  assert.match(client, /scope\?: "common" \| "api"/);
  assert.match(client, /owner_integration_id\?: string/);
  assert.match(client, /runtime_service_connection_id\?: string/);
  assert.match(client, /http_path\?: string/);

  assert.doesNotMatch(bindingWorkspace, /api\.(?:tool|updateTool|cloneTool|dryRunTool|publishTool|retireTool)\(/);
  assert.doesNotMatch(bindingWorkspace, /editorTool|populateEditor|openEditor|saveToolDraft|publishEditorTool|retireEditorTool/);
  assert.doesNotMatch(bindingWorkspace, /MCP Tool Editor|Edit & dry-run|Bounded dry-run|Run dry-run|Clone tool to a draft/);
  const catalog = componentSource(source, "ToolsView", "SettingsTabs");
  assert.doesNotMatch(catalog, /API exposure|<IntegrationToolsWorkspace/);
  assert.match(source, /publishValidationCodes\.has\("tools_missing"\)/);
});

test("uploads local knowledge files with a browser-managed multipart request", async () => {
  const source = await consoleSource();
  const styles = await stylesSource();
  const client = await clientSource();

  assert.match(client, /uploadSource: \(productID: string, organisationID: string, name: string, file: File\)/);
  assert.match(client, /const body = new FormData\(\)/);
  for (const field of ["organisation_id", "name", "file"]) assert.match(client, new RegExp(`body\\.append\\("${field}"`));
  assert.match(client, /\/sources\/upload`/);
  assert.match(client, /const multipartBody = typeof FormData !== "undefined" && init\?\.body instanceof FormData/);
  assert.match(client, /init\?\.body && !multipartBody \? \{ "Content-Type": "application\/json" \} : \{\}/);
  assert.doesNotMatch(client, /uploadSource:[\s\S]{0,700}Content-Type/);

  assert.match(source, /type SourceKind = "website" \| "openapi" \| "git" \| "upload"/);
  assert.match(source, /const sourceUploadMaxBytes = 5_000_000/);
  assert.match(source, /new TextDecoder\("utf-8", \{ fatal: true \}\)/);
  assert.match(source, /type="file"/);
  for (const extension of [".md", ".mdx", ".txt", ".html", ".htm", ".json", ".yaml", ".yml"]) assert.ok(source.includes(extension));
  for (const option of ["Website", "OpenAPI", "Git repository", "Upload a file"]) assert.ok(source.includes(`>${option}</option>`));
  assert.match(source, /up to 5 MB in this setup/);
  assert.match(source, /Content is treated as untrusted text, and embedded scripts are never executed/);
  assert.match(source, /sourceKind === "upload"[\s\S]*api\.uploadSource/);
  assert.match(source, /resetSourceForm\(\)/);
  assert.match(styles, /input\[type="file"\]::file-selector-button/);
});

test("shows exact crawler classifier indicators during source review", async () => {
  const source = await consoleSource();
  const openapi = await readFile(new URL("../api/openapi.yaml", import.meta.url), "utf8");

  assert.match(source, /Classifier indicators: \{document\.injection_indicators\.join\(", "\)\}/);
  assert.match(openapi, /injection_indicators:\n\s+type: array\n\s+items:\n\s+type: string/);
});

test("keeps private defaults and guarded public transitions in the client contract", async () => {
  const source = await consoleSource();
  const packageJson = await readFile(new URL("../package.json", import.meta.url), "utf8");

  assert.match(source, /visibility:\s*"private"/);
  assert.match(source, /setPendingPublication/);
  assert.match(source, /disabled=\{!acknowledged\}/);
  assert.match(source, /title=\{`Make \$\{pendingPublication\?\.name \?\? "source"\} public\?`\}/);
  assert.match(source, /available anonymously through Public MCP/);
  assert.match(source, /setPublicMCPEnabled\(false\)/);
  assert.match(source, /distribution\?\.public_mcp_endpoint/);
  assert.match(source, /distribution\?\.agent_setup\.public/);
  assert.match(source, /distribution\?\.agent_setup\.private/);
  assert.match(source, /Copy \$\{kind\} MCP button/);
  for (const client of ["Codex", "Claude Code", "Cursor", "OpenCode"]) assert.match(source, new RegExp(`name: "${client}"`));
  for (const asset of ["codex.svg", "claude-code.svg", "cursor.svg", "opencode.svg"]) {
    assert.match(source, new RegExp(asset.replace(".", "\\.")));
    assert.match(await readFile(new URL(`../public/agent-client-icons/${asset}`, import.meta.url), "utf8"), /<svg\b/);
  }
  assert.match(source, /disabled=\{!setup\.available\}/);
  assert.match(source, /Configure customer identity first/);
  assert.doesNotMatch(source, /widgets\/\$\{product\.id\}\/public\.js/);
  assert.doesNotMatch(source, /widgets\/\$\{product\.id\}\/private\.js/);
  assert.doesNotMatch(source, /dokosoko\.acme\.dev/);
  assert.match(packageJson, /"@headlessui\/react"/);
  assert.doesNotMatch(packageJson, /react-loading-skeleton/);
  await assert.rejects(access(new URL("../app/_sites-preview/SkeletonPreview.tsx", import.meta.url)));
});

test("keeps Agent access headings and setup cards concise", async () => {
  const source = await consoleSource();

  for (const removed of [
    "Control how authenticated and public agents reach your APIs and knowledge.",
    "Add a secret-free MCP connection button to your developer portal.",
    "Anonymous, read-only access to explicitly public resources.",
    "Customer access through the configured identity provider and browser OAuth.",
    "changing to public always requires confirmation.",
  ]) assert.doesNotMatch(source, new RegExp(removed.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
});

test("keeps live customer suspension controls under Agent access and fails closed while unavailable", async () => {
  const source = await consoleSource();
  const client = await clientSource();
  const styles = await stylesSource();
  const distribution = componentSource(source, "DistributionView", "CustomerAccessPanel");
  const customerAccess = componentSource(source, "CustomerAccessPanel", "AgentSetupCard");

  assert.match(source, /const \[customerAccountsStatus, setCustomerAccountsStatus\] = useState<"loading" \| "ready" \| "unavailable">/);
  assert.match(source, /api\.customerAccounts\(product\.id\)/);
  assert.match(source, /setCustomerAccountsStatus\("unavailable"\)/);
  assert.match(source, /api\.updateCustomerAccount\(product\.id, account\.id, state, account\.revision\)/);
  assert.match(source, /const page = await api\.customerAccounts\(product\.id, cursor\)/);
  assert.match(source, /page\.items\.filter\(\(candidate\) => !items\.some\(\(item\) => item\.id === candidate\.id\)\)/);
  assert.match(client, /export type APICustomerAccountPage = \{[\s\S]*items: APICustomerAccount\[\];[\s\S]*has_more: boolean/);
  assert.match(client, /customerAccounts: \(productID: string, startingAfter = ""\) => request<APICustomerAccountPage>/);
  assert.match(client, /starting_after=\$\{encodeURIComponent\(startingAfter\)\}/);
  assert.match(distribution, /<CustomerAccessPanel accounts=\{customerAccounts\} status=\{customerAccountsStatus\} hasMore=\{customerAccountsHaveMore\} onUpdate=\{onUpdateCustomerAccount\} onLoadMore=\{onLoadMoreCustomerAccounts\} \/>/);
  for (const label of ["Customer access", "Loading customer accounts", "Customer accounts unavailable", "No customer accounts yet", "Suspend", "Reactivate", "Load more"]) {
    assert.ok(customerAccess.includes(label), `${label} should be present in the Agent access customer controls`);
  }
  assert.match(customerAccess, /account\.external_id/);
  assert.match(customerAccess, /account\.last_authenticated_at/);
  assert.match(customerAccess, /account\.state === "active"/);
  assert.match(customerAccess, /window\.confirm\(`Suspend \$\{account\.external_id\}\? Customer MCP access will fail closed immediately\.`\)/);
  assert.match(customerAccess, /account\.state === "active" \? "suspended" : "active"/);
  assert.match(customerAccess, /status === "unavailable"/);
  assert.match(customerAccess, /status === "ready" && accounts\.length > 0/);
  assert.match(customerAccess, /hasMore && <Button/);
  assert.match(styles, /\.customer-access-row/);
  assert.doesNotMatch(styles, /\.customer-access-confirm|\.customer-access-more/);
});

test("keeps page and panel headings concise across the console", async () => {
  const source = await consoleSource();
  const identitySetup = await readFile(new URL("../app/components/OIDCIdentitySetup.tsx", import.meta.url), "utf8");
  const runtimeAccess = await readFile(new URL("../app/components/integrations/IntegrationRuntimeAccess.tsx", import.meta.url), "utf8");
  const uiSource = `${source}\n${identitySetup}\n${runtimeAccess}`;

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
  ]) assert.ok(!uiSource.includes(removed), `redundant description should be removed: ${removed}`);

  for (const retained of [
    "Stored credential material is never returned.",
    "DokoSoko encrypts credentials and never shows them again.",
    "Activation is allowed only for the exact configuration revision that passed the OIDC sign-in test.",
    "New private MCP sessions will fail immediately.",
    "MFA-protected administrator",
  ]) assert.ok(uiSource.includes(retained), `safety guidance should remain: ${retained}`);
});

test("creates private APIs without retaining the Product Definition builder", async () => {
  const source = await consoleSource();
  const client = await clientSource();

  assert.match(source, /Add API/);
  assert.match(source, /Create a private draft/);
  assert.match(source, /<span>API name<\/span>/);
  assert.match(source, /apiFamilyKeyFromName\(displayName\)/);
  assert.doesNotMatch(source, /Auto-magic|title="Product definition"|Build product automatically/);
  assert.doesNotMatch(client, /product-builds|publishProductBuild|productDefinition/);
});

test("publishes immutable API revisions without product channels or pins", async () => {
  const source = await consoleSource();
  const client = await clientSource();

  assert.match(source, /Published history/);
  assert.match(source, /<GitBranch data-slot="icon" \/>Publish/);
  assert.match(source, /published revisions are immutable/i);
  assert.doesNotMatch(source, /Scoped version pins|Customer installations|generated release diff|Independent approval required|No silent migration/);
  assert.match(client, /description\/rewrite/);
  assert.match(client, /publishIntegration/);
  assert.doesNotMatch(client, /version-pins|productVersions|promoteProductVersion/);
});

test("ships a provider-neutral OIDC draft, test, and activation workspace", async () => {
  const source = await consoleSource();
  const identitySetup = await readFile(new URL("../app/components/OIDCIdentitySetup.tsx", import.meta.url), "utf8");
  const client = await clientSource();
  const routes = await readFile(new URL("../app/lib/console-routes.ts", import.meta.url), "utf8");
  const styles = await stylesSource();
  const settings = componentSource(source, "SettingsView", "RootAccessPanel");
  const settingsTabs = routes.slice(routes.indexOf("export const SETTINGS_TABS"), routes.indexOf("export const SECTION_PATHS"));
  const identityClient = client.slice(client.indexOf("identity: ()"), client.indexOf("supportSubmissions:", client.indexOf("identity: ()")));
  const callbackErrorCleanup = identitySetup.slice(identitySetup.indexOf("function clearIdentityTestErrorQuery"), identitySetup.indexOf("function testStatusLabel"));
  const issuerValidation = identitySetup.slice(identitySetup.indexOf("function validOIDCIssuer"), identitySetup.indexOf("function validAuthorizationAPIOrigin"));
  const originValidation = identitySetup.slice(identitySetup.indexOf("function validAuthorizationAPIOrigin"), identitySetup.indexOf("function validOAuthResourceIdentifier"));
  const resourceValidation = identitySetup.slice(identitySetup.indexOf("function validOAuthResourceIdentifier"), identitySetup.indexOf("function identityConfigurationNeedsReview"));
  const testGuidance = identitySetup.match(/<h2[^>]*>Test sign-in<\/h2><p[^>]*>([^<]+)<\/p>/)?.[1] ?? "";

  assert.match(source, /eyebrow="Administration" title="Settings" action=/);
  for (const tab of ["storage", "ai", "root"]) {
    assert.match(source, new RegExp(`settingsPath\\("${tab}"\\)`));
  }
  assert.doesNotMatch(settingsTabs, /id: "identity"|label: "Customer identity"/);
  assert.doesNotMatch(settings, /Customer identity|OIDCIdentitySetup|onConfigureIdentity/);
  assert.doesNotMatch(source, /settingsPath\("identity"\)|function CustomerIdentitySettingsView|function IdentityContractPanel|\bidentityOpen\b|\bsetIdentityOpen\b|title="Customer identity integration"/);
  assert.match(source, /\{section === "identity" && <OIDCIdentitySetup/);

  for (const label of ["Confidential web app", "Allowed callback URL", "Issuer URL", "Customer account claim", "Authorization API origin", "Save draft", "Test sign-in", "Activate"]) {
    assert.ok(identitySetup.includes(label), `${label} should be present in the OIDC setup workspace`);
  }
  assert.match(identitySetup, /const callbackURL = identity\?\.callback_url \?\? ""/);
  assert.match(identitySetup, /<code>\{callbackURL\}<\/code>/);
  assert.match(identitySetup, /aria-label="Copy OIDC callback URL"/);
  assert.match(identitySetup, /copyText\(callbackURL\)/);
  assert.doesNotMatch(identitySetup, /identity-provider\/callback/);
  assert.match(identitySetup, /api\.saveIdentityDraft\(/);
  assert.match(identitySetup, /provider: "oidc"/);
  assert.match(identitySetup, /api\.beginIdentityTest\(identity\.revision\)/);
  assert.match(identitySetup, /const \[callbackTestID\] = useState\(identityTestIDFromLocation\)/);
  assert.match(identitySetup, /callbackTestID \? \{ id: callbackTestID \} : identity\?\.last_test \? \{ id: identity\.last_test\.id, value: identity\.last_test \} : \{ id: "" \}/);
  assert.match(identitySetup, /const lastTest = selectedTest\.value/);
  assert.doesNotMatch(identitySetup, /const lastTest = identity\?\.last_test/);
  assert.match(identitySetup, /api\.identityTest\(callbackTestID\)/);
  assert.match(identitySetup, /setSelectedTest\(\{ id: callbackTestID, value: test \}\)/);
  assert.match(identitySetup, /searchParams\.get\("identity_test_id"\)/);
  assert.match(identitySetup, /searchParams\.delete\("identity_test_id"\)/);
  assert.doesNotMatch(identitySetup, /searchParams\.delete\("identity_test"\)/);
  assert.match(identitySetup, /window\.history\.replaceState\(window\.history\.state, "", `\$\{url\.pathname\}\$\{query \? `\?\$\{query\}` : ""\}\$\{url\.hash\}`\)/);
  assert.match(identitySetup, /setSelectedTest\(\{ id: callbackTestID, value: test \}\);[\s\S]*setCallbackTestFailure\(""\);[\s\S]*clearIdentityTestQuery\(callbackTestID\);[\s\S]*\}\)\.catch/);
  assert.match(identitySetup, /setCallbackTestFailure\(caught instanceof APIError \? caught\.message : "The returned OIDC test could not be loaded\."\)/);
  assert.match(identitySetup, /Could not load returned OIDC test/);
  assert.match(identitySetup, /onClick=\{\(\) => void retryCallbackTest\(\)\}>Retry<\/Button>/);
  assert.match(identitySetup, /setSelectedTest\(\{ id: started\.id, value: started \}\)[\s\S]*if \(started\.authorization_url\)[\s\S]*window\.location\.assign\(started\.authorization_url\)/);
  assert.doesNotMatch(identitySetup, /api\.identityTest\(started\.id\)/);
  assert.doesNotMatch(identitySetup, /server did not return an OIDC authorization URL/i);
  assert.match(identitySetup, /Loading returned OIDC test/);
  assert.doesNotMatch(identitySetup, /identity-test-id|Test \{selectedTest\.id\}/);
  assert.match(identitySetup, /searchParams\.get\("identity_test_error"\)/);
  assert.match(identitySetup, /marker === "invalid_or_expired" \? marker : ""/);
  assert.match(callbackErrorCleanup, /searchParams\.delete\("identity_test_error"\)/);
  assert.doesNotMatch(callbackErrorCleanup, /searchParams\.delete\("identity_test_id"\)/);
  assert.match(identitySetup, /if \(callbackTestError && identity && !loading\) clearIdentityTestErrorQuery\(callbackTestError\)/);
  assert.match(identitySetup, /The callback was expired, already used, or invalid\. It did not pass or change the saved test result\. Start a sign-in test again\./);
  assert.match(source, /<OIDCIdentitySetup key=\{identityLoading \? "loading" : identityConfig\?\.id \|\| "identity"\}/);
  assert.match(identitySetup, /api\.activateIdentity\(identity\.revision, lastTest\.id\)/);
  assert.match(identitySetup, /api\.disableIdentity\(identity\.revision\)/);
  assert.match(identitySetup, /api\.disconnectIdentity\(identity\.revision\)/);
  assert.match(identitySetup, /configured && !active && <section className="panel identity-disconnect-zone">/);
  assert.match(identitySetup, /Disconnect this OIDC provider permanently\?/);
  assert.match(identitySetup, /This removes the saved OIDC configuration, encrypted client secret, and test history\. Customer-account and audit records remain\./);
  assert.match(identitySetup, /const disconnected = await api\.disconnectIdentity\(identity\.revision\);[\s\S]*onChanged\(disconnected\)/);
  assert.match(identitySetup, /const draftTestMatchesCurrentRevision = Boolean\(lastTest\?\.status === "passed" && lastTest\.configuration_revision === identity\?\.revision\)/);
  assert.match(identitySetup, /const testStaleForCurrentRevision = Boolean\(!active && lastTest && lastTest\.configuration_revision !== identity\?\.revision\)/);
  assert.match(identitySetup, /const \[activationObservedAt, setActivationObservedAt\] = useState\(\(\) => Date\.now\(\)\)/);
  assert.match(identitySetup, /window\.setTimeout\(\(\) => setActivationObservedAt\(Date\.now\(\)\), Math\.max\(0, expiresAt - Date\.now\(\)\) \+ 1\)/);
  assert.match(identitySetup, /const draftTestPassed = Boolean\(draftTestMatchesCurrentRevision && lastTest && Date\.parse\(lastTest\.expires_at\) > activationObservedAt\)/);
  assert.match(identitySetup, /const draftTestExpiredForActivation = Boolean\(!active && draftTestMatchesCurrentRevision && !draftTestPassed\)/);
  assert.match(identitySetup, /const testPassedForCurrentState = active \? true : draftTestPassed/);
  assert.match(identitySetup, /testStaleForCurrentRevision \? "stale" : draftTestExpiredForActivation \? "expired"/);
  assert.match(identitySetup, /testStaleForCurrentRevision \? "Not tested for this revision"/);
  assert.match(identitySetup, /`Run Test sign-in for revision \$\{identity\.revision\}\.`/);
  assert.match(identitySetup, /Test expired for activation/);
  assert.match(identitySetup, /Run Test sign-in again, then activate before the new test expires\./);
  assert.match(identitySetup, /lastTest\.configuration_revision !== identity\.revision \|\| !\(Date\.parse\(lastTest\.expires_at\) > Date\.now\(\)\)/);
  assert.match(identitySetup, /window\.location\.assign\(started\.authorization_url\)/);
  assert.match(identitySetup, /Changes saved as a disabled draft/);
  assert.match(identitySetup, /Connect one OIDC provider for all private MCP clients\./);
  assert.match(identitySetup, /Review migrated settings/);
  assert.match(identitySetup, /Save reviewed draft/);
  assert.match(identitySetup, /identityConfigurationNeedsReview\(identity\)/);
  assert.match(identitySetup, /identity\.credential_present \? "The encrypted client secret is reused unless you replace it\." : "Enter the OIDC client secret before saving\."/);
  assert.match(identitySetup, /placeholder=\{identity\.credential_present \? "\*{12}" : undefined\} value=\{clientSecret\}/);
  assert.match(identitySetup, /Encrypted secret stored\. Type a new value only to replace it\./);
  assert.match(identitySetup, /client_secret: clientSecret/);
  assert.match(identitySetup, /const hasOpenIDScope = identity\.scopes\.some\(\(scope\) => scope\.trim\(\) === "openid"\)/);
  assert.match(identitySetup, /!validAuthorizationOrigin \|\| !validOAuthResource \|\| !hasOpenIDScope/);
  assert.match(identitySetup, /const openIDScopeReady = scopeValues\.includes\("openid"\)/);
  assert.match(identitySetup, /authorizationAPIOriginReady && oauthResourceReady && openIDScopeReady/);
  assert.match(identitySetup, /const \[issuerInput, setIssuerInput\] = useState\(\(\) => identity\?\.issuer \?\? ""\)/);
  assert.match(identitySetup, /const issuer = issuerInput\.trim\(\)/);
  assert.match(identitySetup, /const issuerReady = validOIDCIssuer\(issuer\)/);
  assert.ok(identitySetup.includes('return `${issuer.replace(/\\/$/, "")}/.well-known/openid-configuration`;'));
  assert.match(identitySetup, /href=\{discoveryURL\}[\s\S]*Open discovery document/);
  assert.match(issuerValidation, /value\.username \|\| value\.password \|\| value\.search \|\| value\.hash/);
  assert.doesNotMatch(issuerValidation, /value\.port|pathname|replace\(|endsWith\("\/"\)/);
  assert.match(originValidation, /value\.username \|\| value\.password \|\| value\.search \|\| value\.hash \|\| value\.pathname !== "\/"/);
  assert.match(originValidation, /value\.protocol === "https:" && value\.port/);
  assert.match(resourceValidation, /new URL\(raw\.trim\(\)\)/);
  assert.match(resourceValidation, /Boolean\(value\.protocol && !value\.username && !value\.password && !value\.hash\)/);
  assert.doesNotMatch(resourceValidation, /value\.host|value\.port/);
  assert.match(identitySetup, /Use a credential-free HTTPS origin on the default port; local HTTP is allowed\./);
  assert.match(identitySetup, /Enter an absolute URI without a fragment\./);
  assert.match(identitySetup, /client_authentication_unsupported/);
  assert.match(identitySetup, /must support client_secret_basic or client_secret_post/);
  assert.match(identitySetup, /placeholder="urn:example:customer-api"/);
  assert.match(identitySetup, /<code>openid<\/code> is required\./);
  assert.match(identitySetup, /configured && !reviewRequired && <Button outline/);
  assert.match(identitySetup, /then activate customer sign-in\./);
  assert.doesNotMatch(identitySetup, /then activate private MCP access\./);
  assert.match(identitySetup, /This verifies only the OIDC sign-in and mapped customer claim\. Use each API&apos;s Test tab to verify end-to-end authorization\./);
  assert.ok(testGuidance, "the OIDC test should explain exactly what it verifies");
  assert.doesNotMatch(testGuidance, /(?:verifies|tests)[^.]*authoriz|authoriz[^.]*(?:verified|tested)/i);
  assert.match(identitySetup, /const \[customerClaim, setCustomerClaim\] = useState\(\(\) => identity\?\.customer_account_claim \?\? ""\)/);
  assert.doesNotMatch(identitySetup, /useState\((?:\(\) => )?(?:identity\?\.customer_account_claim \?\? )?"org_id"\)/);

  for (const localHostname of ['"localhost"', '".localhost"', '"127.0.0.1"', '"[::1]"', '"::1"']) {
    assert.ok(identitySetup.includes(localHostname), `${localHostname} should be treated as a local-development OIDC hostname`);
  }
  assert.match(identitySetup, /<span>Audience \(optional\)<\/span>/);
  assert.match(identitySetup, /<span>OAuth resource \(optional\)<\/span>/);
  assert.doesNotMatch(identitySetup.match(/const formReady =[^;]+;/)?.[0] ?? "", /audience/);
  assert.doesNotMatch(identitySetup, /Auth0|auth0/);

  assert.match(identityClient, /saveIdentityDraft:[\s\S]*method: "PUT"/);
  assert.match(identityClient, /beginIdentityTest:[\s\S]*\/api\/v1\/identity-provider\/tests"[\s\S]*method: "POST"[\s\S]*revision/);
  assert.match(identityClient, /identityTest:[\s\S]*\/api\/v1\/identity-provider\/tests\/\$\{encodeURIComponent\(testID\)\}/);
  assert.match(identityClient, /activateIdentity:[\s\S]*\/api\/v1\/identity-provider\/activate"[\s\S]*test_id: testID/);
  assert.match(identityClient, /disableIdentity:[\s\S]*\/api\/v1\/identity-provider\/disable"[\s\S]*revision/);
  assert.match(identityClient, /disconnectIdentity:[\s\S]*\/api\/v1\/identity-provider"[\s\S]*method: "DELETE"[\s\S]*JSON\.stringify\(\{ revision \}\)/);
  assert.doesNotMatch(identityClient, /\bstate\s*:|configureIdentity|simulateAuthorizationPoint/);
  assert.doesNotMatch(client.slice(client.indexOf("export type APIIdentityTest"), client.indexOf("export type APIIdentity", client.indexOf("export type APIIdentityTest"))), /\bsubject\??:/);
  assert.match(client.slice(client.indexOf("export type APIIdentity"), client.indexOf("export type APICustomerAccount")), /id\?: string;[\s\S]*audience: string;[\s\S]*oauth_resource: string;/);
  assert.doesNotMatch(identitySetup, /\bcustomerAccounts\b|APICustomerAccount|Customer accounts|Grant registry|API action policies|Policy simulator|Simulation only/);
  assert.doesNotMatch(styles, /\.product-identity-panel\b/);

  assert.match(client, /customerAccounts/);
  assert.match(client, /updateCustomerAccount/);
  assert.match(client, /customer_account_claim/);
  assert.match(client, /authorization_api_origin/);
  assert.doesNotMatch(client, /organisation_claim|delegated_api_origin/);
  assert.match(client, /\/api\/v1\/identity-provider/);
  assert.doesNotMatch(client, /usage_hook_url|allowed_redirect_uris|entitlement_hook_url|APIAuthorizationSimulation|simulateAuthorizationPoint/);
});

test("ships a consent-gated plaintext support outbox", async () => {
  const source = await consoleSource();
  const client = await clientSource();

	assert.match(source, /Support outbox/);
	assert.match(source, /Simple local outbox/);
	assert.match(source, /schema-bounded plaintext/);
	assert.match(source, /There is no delivery worker or external routing/);
  assert.match(source, /onClick=\{\(\) => onView\(submission\)\}/);
  assert.match(client, /support-submissions/);
  assert.doesNotMatch(client, /backendConnections|createSupportRoute|replaceSupportRoute|createSupportDeliveryAttempt/);
});

test("ships first-class API, reusable resource, and runtime service management", async () => {
  const source = await consoleSource();
  const client = await clientSource();
  const runtimeAccess = await readFile(new URL("../app/components/integrations/IntegrationRuntimeAccess.tsx", import.meta.url), "utf8");

  assert.match(source, /Add API/);
  assert.match(source, /Create reusable resource set/);
  assert.match(source, /Duplicate resource set/);
  assert.match(source, /Pin the current revision instead of following latest/);
  assert.match(runtimeAccess, /Connect service/);
  assert.match(runtimeAccess, /Credential lifecycle and connection metadata/);
  assert.match(source, /Create a private draft/);
  assert.match(source, /<span>API name<\/span>/);
  assert.match(source, /editingIntegration \? "Save changes" : "Create API"/);
  assert.match(source, /apiFamilyKeyFromName\(displayName\)/);
  assert.doesNotMatch(source, /Each API record represents one family and one version/);
  assert.match(client, /createIntegration/);
  assert.match(client, /updateIntegration/);
  assert.match(client, /duplicateResourceSet/);
  assert.match(client, /createIntegrationRuntimeConnection/);
  assert.doesNotMatch(client, /updateAccessDefinition|createAccessDefinition|createAccessConnection|createProvider:|projects:\s*async/);
});

test("uses the real masked runtime-service model for API-local Access", async () => {
  const source = await consoleSource();
  const access = await readFile(new URL("../app/components/integrations/IntegrationRuntimeAccess.tsx", import.meta.url), "utf8");
  const client = await clientSource();
  const styles = await stylesSource();
  const runtimeTypes = client.slice(client.indexOf("export type APIRuntimeAuthenticationType"), client.indexOf("export type APISource"));

  assert.match(source, /<IntegrationRuntimeAccess integration=\{integration\}[\s\S]{0,80}onMessage=\{onMessage\}/);
  assert.match(access, /api\.integrationRuntimeSetup\(integration\.id\)/);
  assert.match(access, /api\.configureIntegrationRuntimeSetup\(integration\.id/);
  assert.match(access, /Only this API/);
  assert.match(access, /Share across APIs/);
  assert.match(access, /Use existing/);
  assert.match(access, /SERVICE_API_KEY/);
  assert.match(access, /type="password"/);
  assert.match(access, /placeholder=\{selectedCurrentCredential\?\.credential_present \? "••••••••••••"/);
  assert.match(access, /Credential lifecycle and connection metadata — Advanced/);
  assert.match(access, /api\.rotateRuntimeCredential/);
  assert.match(access, /api\.revokeRuntimeCredentialVersion/);
  assert.match(access, /Check configuration/);
  assert.match(access, /metadata only\. Live upstream behavior is tested from an attached tool/);
  assert.doesNotMatch(access, /fetch\(/);

  assert.match(client, /integrationRuntimeSetup:[\s\S]*\/runtime-setup`/);
  assert.match(client, /configureIntegrationRuntimeSetup:[\s\S]*method: "PUT"/);
  assert.match(client, /runtimeCredentialUsage:[\s\S]*\/usage`/);
  assert.match(client, /rotateRuntimeCredential:[\s\S]*\/rotate`/);
  assert.match(client, /revokeRuntimeCredentialVersion:[\s\S]*\/versions\/\$\{encodeURIComponent\(versionID\)\}\/revoke`/);
  assert.match(client, /checkRuntimeServiceConnection:[\s\S]*\/runtime-service-connections\/\$\{encodeURIComponent\(connectionID\)\}\/check`/);
  assert.doesNotMatch(runtimeTypes, /secret_id|SecretID|credential:\s*string;[\s\S]*APIRuntimeCredentialVersion/);
  assert.match(styles, /\.runtime-choice-grid/);
  assert.match(styles, /\.runtime-configuration-check/);
  assert.match(styles, /\.runtime-credential-management/);
});

test("leads API documentation with an advisory setup guide", async () => {
  const source = await consoleSource();
  const guide = await readFile(new URL("../app/components/integrations/IntegrationSetupGuide.tsx", import.meta.url), "utf8");
  const styles = await stylesSource();

  assert.match(source, /analysis\.state === "review" && analysisMatchesIntegration\(analysis, integration\.id\)/);
  assert.match(source, /<IntegrationSetupGuide analysis=\{setupGuideAnalysis\} canGenerate=\{attachedResources\.length > 0\} busy=\{setupGuideBusy\} onGenerate=\{generateSetupGuide\} \/>[\s\S]*Documentation ingestion/);
	assert.match(source, /onGenerateSetupGuide\(integrationID\)/);
	assert.match(guide, /analysis\.plan\.summary/);
  assert.match(guide, /analysis\.plan\.identity\.explanation/);
  assert.match(guide, /analysis\.plan\.endpoints\.slice\(0, 2\)/);
  assert.match(guide, /Orientation, not published evidence/);
  assert.match(guide, /Agents rely only on the reviewed resources attached below/);
  assert.match(guide, /Advisory draft/);
	assert.match(guide, /title="Setup guide"/);
	assert.match(guide, /Generate guide/);
	assert.match(guide, /Refresh guide/);
  assert.doesNotMatch(guide, /Publish guide|Approve guide/);
  assert.match(styles, /\.integration-setup-guide/);
  assert.match(styles, /\.setup-guide-steps/);
});
