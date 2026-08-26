import assert from "node:assert/strict";
import test from "node:test";

import {
  analysisMatchesIntegration,
  activeRecipeIntegrationID,
  apiFamilyKeyFromName,
  buildAgentSetupEmbedHTML,
  integrationIncludesSourcePublication,
  recipeAnalysisIsFreshlyRunning,
  recipeHasScopeDependencyMismatch,
  recipeMatchesIntegration,
  recipeScopeIDs,
  sourcePublicationManifestEntry,
  toolPolicy,
  unavailableConsoleCapability,
  type Source,
} from "../app/lib/console-domain";
import { APIError, type APIIntegration, type APIIntegrationAnalysis, type APIRecipe, type APISourcePublication, type APITool } from "../app/lib/api";

test("console domain helpers keep integration-scoped analysis and recipes isolated", () => {
  const analysis = { evidence: [{ kind: "integration_scope", resource_id: "integration-a" }] } as APIIntegrationAnalysis;
  const recipe = { dependencies: [{ kind: "integration_scope", resource_id: "integration-a" }] } as APIRecipe;
  const ambiguousAnalysis = { evidence: [{ kind: "integration_scope", resource_id: "integration-a" }, { kind: "integration_scope", resource_id: "integration-b" }] } as APIIntegrationAnalysis;
  const ambiguousRecipe = { dependencies: [{ kind: "integration_scope", resource_id: "integration-a" }, { kind: "integration_scope", resource_id: "integration-b" }] } as APIRecipe;
  const generalAnalysis = { evidence: [] } as unknown as APIIntegrationAnalysis;
  const generalRecipe = { dependencies: [] } as unknown as APIRecipe;

  assert.equal(analysisMatchesIntegration(analysis, "integration-a"), true);
  assert.equal(analysisMatchesIntegration(analysis, "integration-b"), false);
  assert.equal(recipeMatchesIntegration(recipe, "integration-a"), true);
  assert.equal(recipeMatchesIntegration(recipe, "integration-b"), false);
  assert.equal(analysisMatchesIntegration(ambiguousAnalysis, "integration-a"), false);
  assert.equal(recipeMatchesIntegration(ambiguousRecipe, "integration-a"), false);
  assert.equal(analysisMatchesIntegration(ambiguousAnalysis), false);
  assert.equal(recipeMatchesIntegration(ambiguousRecipe), false);
  assert.equal(analysisMatchesIntegration(generalAnalysis), true);
  assert.equal(recipeMatchesIntegration(generalRecipe), true);
});

test("product-integration recipe scope follows the canonical integration and reports dependency drift", () => {
  const current = {
    contract_version: "product-integration-v2",
    integration_id: "integration-canonical",
    dependencies: [{ kind: "integration_scope", resource_id: "integration-canonical" }],
  } as APIRecipe;
  const drifted = {
    ...current,
    dependencies: [{ kind: "integration_scope", resource_id: "integration-stale" }],
  } as APIRecipe;
  const missingDependency = { ...current, dependencies: [] } as unknown as APIRecipe;

  assert.deepEqual(recipeScopeIDs(current), ["integration-canonical"]);
  assert.equal(recipeMatchesIntegration(current, "integration-canonical"), true);
  assert.equal(recipeMatchesIntegration(drifted, "integration-canonical"), true);
  assert.equal(recipeMatchesIntegration(drifted, "integration-stale"), false);
  assert.equal(recipeHasScopeDependencyMismatch(current), false);
  assert.equal(recipeHasScopeDependencyMismatch(drifted), true);
  assert.equal(recipeHasScopeDependencyMismatch(missingDependency), true);
});

test("recipe generation waits only for a recent running analysis", () => {
  const now = Date.parse("2026-08-26T12:00:00Z");
  const analysis = {
    state: "running",
    created_at: "2026-08-26T11:59:00Z",
  } as unknown as APIIntegrationAnalysis;

  assert.equal(recipeAnalysisIsFreshlyRunning(undefined, now), false);
  assert.equal(recipeAnalysisIsFreshlyRunning({ ...analysis, state: "review" }, now), false);
  assert.equal(recipeAnalysisIsFreshlyRunning({ ...analysis, state: "failed" }, now), false);
  assert.equal(recipeAnalysisIsFreshlyRunning(analysis, now), true);
  assert.equal(recipeAnalysisIsFreshlyRunning({ ...analysis, created_at: "2026-08-26T11:55:00Z" }, now), true);
  assert.equal(recipeAnalysisIsFreshlyRunning({ ...analysis, created_at: "2026-08-26T11:54:59Z" }, now), false);
  assert.equal(recipeAnalysisIsFreshlyRunning({ ...analysis, created_at: "not-a-date" }, now), false);
});

test("recipe scope is automatic only when the deployment has one API", () => {
  const first = { id: "integration-first" } as APIIntegration;
  const second = { id: "integration-second" } as APIIntegration;

  assert.equal(activeRecipeIntegrationID([], ""), "");
  assert.equal(activeRecipeIntegrationID([first], ""), first.id);
  assert.equal(activeRecipeIntegrationID([first, second], ""), "");
  assert.equal(activeRecipeIntegrationID([first, second], second.id), second.id);
  assert.equal(activeRecipeIntegrationID([first, second], "missing"), "");
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

test("console embed output escapes tenant-controlled values", () => {
  const embed = buildAgentSetupEmbedHTML(`<img src=x onerror="alert(1)">`, "https://console.example/agent-setup/public/prompt.md?a=1&b=2", "public");
  assert.doesNotMatch(embed, /<img src=x onerror=/);
  assert.match(embed, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
  assert.match(embed, /a=1&amp;b=2/);
});
