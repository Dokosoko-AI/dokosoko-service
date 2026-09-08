import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { resolveEnglishTranslations } from "./source-surface.mjs";

const read = (path: string) => readFile(new URL(path, import.meta.url), "utf8");

test("developer asset advisory UI is persisted, evidence-bounded, and explicitly non-authoritative", async () => {
  const [component, client] = await Promise.all([
    read("../app/components/console/developer-assets/developer-asset-ai-advisory.tsx").then(resolveEnglishTranslations),
    read("../app/lib/developer-assets-api.ts"),
    read("../app/components/console/developer-assets/sdk-guidance-input.tsx").then(resolveEnglishTranslations),
    read("../app/components/console/developer-assets/sdk-catalog-view.tsx"),
  ]);

  for (const key of ["documentation.map_enrichment", "sdk.map_enrichment", "sdk.applicability_suggestion", "sdk.sample_review"]) {
    assert.match(client, new RegExp(key.replace(".", "\\.")));
  }
  assert.match(client, /\/ai-advisories/);
  assert.match(client, /aiAdvisories:/);
  assert.match(client, /aiAdvisory:/);
  assert.match(client, /runAIAdvisory:/);
  assert.match(component, /cannot approve, validate, attach, publish, or change/i);
  assert.match(component, /Non-authoritative suggestion/);
  assert.match(component, /allowed_evidence_ids/);
  assert.match(component, /evidence_ids/);
  assert.match(component, /selectors/);
  assert.match(component, /Evidence gaps/);
  assert.match(component, /Closed finding codes/);
  assert.match(component, /prompt_version/);
  assert.match(component, /created_at/);
  assert.match(component, /AI unconfigured/);
  assert.match(component, /ai_unavailable/);
  assert.match(component, /No advisory was stored/);
});

test("advisory actions are attached only to exact reviewed or published scopes", async () => {
  const [sources, explorer, sdk, resources] = await Promise.all([
    read("../app/components/console/agent-access-views.tsx"),
    read("../app/components/console/developer-assets/documentation-explorer-view.tsx"),
    read("../app/components/console/developer-assets/sdk-advanced-catalog-view.tsx").then(resolveEnglishTranslations),
    read("../app/components/console/developer-assets/api-resources-workspace.tsx"),
  ]);

  assert.match(sources, /source\.latestPublication\.id/);
  assert.match(sources, /documentation\.map_enrichment/);
  assert.match(explorer, /selection\.decision === "included"/);
  assert.match(explorer, /selection\.content_hash === record\.document\.content_hash/);
  assert.match(explorer, /record\?\.source_publication_selections\.find/);
  assert.match(explorer, /reviewedSourcePublicationID \? \{ prompt_key: "documentation\.map_enrichment"/);
  assert.match(sdk, /sdk\.map_enrichment/);
  assert.match(sdk, /sdk\.sample_review/);
  assert.match(sdk, /api_developer_asset_publication_id: publication\.id/);
  assert.match(resources, /sdk\.applicability_suggestion/);
  assert.match(resources, /asset\.binding_id === sdkBinding\.id/);
  assert.match(resources, /asset\.sdk_content_publication_id === sdkBinding\.sdk_content_publication_id/);
});

test("SDK console exposes effective lifecycle controls and bounded local ingestion/explorer tooling", async () => {
  const [sdk, client, input, catalog] = await Promise.all([
    read("../app/components/console/developer-assets/sdk-advanced-catalog-view.tsx").then(resolveEnglishTranslations),
    read("../app/lib/developer-assets-api.ts"),
    read("../app/components/console/developer-assets/sdk-guidance-input.tsx").then(resolveEnglishTranslations),
    read("../app/components/console/developer-assets/sdk-catalog-view.tsx"),
  ]);

  assert.match(client, /sdkReleaseLifecycle:/);
  assert.match(client, /appendSDKReleaseLifecycleEvent:/);
  assert.match(sdk, /Effective lifecycle and append-only history/);
  assert.match(sdk, /Record append-only event/);
  assert.match(sdk, /Yanked and archived block new selections/);
  assert.match(sdk, /Historical API publications remain readable/);
  assert.match(sdk, /observed_source_uri/);
  assert.match(sdk, /\^https:/);

  assert.match(sdk, /Filter path, title, language, role, or hash/);
  assert.match(sdk, /No files match this local filter/);
  assert.match(sdk, /No symbols match this local filter/);
  assert.match(sdk, /No samples match this local filter/);

  assert.match(input, /type="file" multiple/);
  assert.match(input, /rejected\.length/);
  assert.match(input, /Package code is never executed/);
  assert.match(catalog, /lazy\(\(\) => import\("\.\/sdk-advanced-catalog-view"\)/);
  assert.match(catalog, /<SDKSetupWorkspace/);
  assert.doesNotMatch(sdk, /publishSDKContentCandidate|ingestSDKContent|attachAPISDK|reviewOpen|ingestOpen/);
});
