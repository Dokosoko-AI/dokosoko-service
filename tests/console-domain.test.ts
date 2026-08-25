import assert from "node:assert/strict";
import test from "node:test";

import {
  analysisMatchesIntegration,
  apiFamilyKeyFromName,
  buildAgentSetupEmbedHTML,
  integrationIncludesSourcePublication,
  recipeMatchesIntegration,
  sourcePublicationManifestEntry,
  toolPolicy,
  unavailableConsoleCapability,
  widgetOriginLabel,
  type Source,
} from "../app/lib/console-domain";
import { APIError, type APIIntegration, type APIIntegrationAnalysis, type APIRecipe, type APISourcePublication, type APITool } from "../app/lib/api";

test("console domain helpers keep integration-scoped analysis and recipes isolated", () => {
  const analysis = { evidence: [{ kind: "integration_scope", resource_id: "integration-a" }] } as APIIntegrationAnalysis;
  const recipe = { dependencies: [{ kind: "integration_scope", resource_id: "integration-a" }] } as APIRecipe;
  const generalAnalysis = { evidence: [] } as unknown as APIIntegrationAnalysis;
  const generalRecipe = { dependencies: [] } as unknown as APIRecipe;

  assert.equal(analysisMatchesIntegration(analysis, "integration-a"), true);
  assert.equal(analysisMatchesIntegration(analysis, "integration-b"), false);
  assert.equal(recipeMatchesIntegration(recipe, "integration-a"), true);
  assert.equal(recipeMatchesIntegration(recipe, "integration-b"), false);
  assert.equal(analysisMatchesIntegration(generalAnalysis), true);
  assert.equal(recipeMatchesIntegration(generalRecipe), true);
});

test("console domain helpers normalize API keys and bind exact source publications", () => {
  assert.equal(apiFamilyKeyFromName("  Voice & Messaging API  "), "voice-messaging-api");
  assert.equal(apiFamilyKeyFromName("🎙️"), "api");

  const source = { id: "source-1", name: "Reference", kind: "website" } as Source;
  const publication = { id: "publication-7", revision: 7, content_hash: "sha256:abc" } as APISourcePublication;
  const entry = sourcePublicationManifestEntry(source, publication);
  assert.deepEqual(entry, {
    source_publication_id: publication.id,
    source_id: source.id,
    revision: publication.revision,
    content_hash: publication.content_hash,
    name: source.name,
  });
  const integration = { resources: [{ kind: "documentation", resolved_revision: { manifest: [entry] } }] } as unknown as APIIntegration;
  assert.equal(integrationIncludesSourcePublication(integration, publication.id), true);
  assert.equal(integrationIncludesSourcePublication(integration, "publication-other"), false);
});

test("console domain helpers derive safe tool policy defaults and capability failures", () => {
  const getTool = { http_method: "GET", state: "published", revision: 2, authorization_policy: {} } as APITool;
  const deleteTool = { http_method: "DELETE", state: "draft", revision: 1, authorization_policy: {} } as APITool;
  const explicit = {
    http_method: "POST",
    state: "published",
    revision: 3,
    authorization_policy: { required_grants: ["items.write", 42], confirmation_required: true, risk: "high", idempotency_required: true },
  } as unknown as APITool;

  assert.equal(toolPolicy(getTool).risk, "low");
  assert.equal(toolPolicy(deleteTool).risk, "critical");
  assert.deepEqual(toolPolicy(explicit), { requiredGrants: ["items.write"], confirmationRequired: true, risk: "high", idempotencyRequired: true });
  assert.equal(unavailableConsoleCapability(new APIError(404, "not_found", "missing")), true);
  assert.equal(unavailableConsoleCapability(new APIError(500, "failed", "failed")), false);
});

test("console embed output escapes tenant-controlled values and labels origins", () => {
  const embed = buildAgentSetupEmbedHTML(`<img src=x onerror="alert(1)">`, "https://console.example/agent-setup/public/prompt.md?a=1&b=2", "public");
  assert.doesNotMatch(embed, /<img src=x onerror=/);
  assert.match(embed, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
  assert.match(embed, /a=1&amp;b=2/);
  assert.equal(widgetOriginLabel("https://app.customer.example/path"), "app.customer.example");
  assert.equal(widgetOriginLabel("not a URL"), "Invalid origin");
});
