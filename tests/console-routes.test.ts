import assert from "node:assert/strict";
import test from "node:test";

import {
  INTEGRATION_TABS,
  SECTION_PATHS,
  entityPath,
  integrationPath,
  parseConsolePath,
  routeForEntity,
  routeForIntegration,
  sectionPath,
} from "../app/lib/console-routes";

test("primary console sections have canonical, round-trippable URLs", () => {
  for (const section of ["product", "distribution", "runs", "settings"] as const) {
    const path = SECTION_PATHS[section];
    assert.equal(sectionPath(section as keyof typeof SECTION_PATHS), path);
    assert.deepEqual(parseConsolePath(path), { kind: "section", section, path });
    assert.deepEqual(parseConsolePath(`${path}/`), { kind: "section", section, path });
  }
});

test("the root and retired overview path canonicalize to APIs", () => {
  assert.deepEqual(parseConsolePath("/"), {
    kind: "section",
    section: "product",
    path: "/integrations",
  });
  assert.deepEqual(parseConsolePath("/overview"), parseConsolePath("/"));
  assert.deepEqual(parseConsolePath("/distribution"), parseConsolePath("/agent-access"));
  assert.deepEqual(parseConsolePath("/operations"), parseConsolePath("/activity"));
});

test("entity URLs encode UIDs and resolve to their owning section", () => {
  const uid = "voice api/v3";
  assert.equal(entityPath("integration", uid), "/integration/voice%20api%2Fv3");
  assert.deepEqual(parseConsolePath(entityPath("integration", uid)), routeForEntity("integration", uid));
  assert.equal(routeForEntity("source", "src_docs").section, "sources");
  assert.equal(routeForEntity("report", "report_123").section, "reporting");
});

test("API workspaces have four stable, round-trippable tab URLs", () => {
  const uid = "voice api/v3";
  for (const tab of INTEGRATION_TABS) {
    const path = integrationPath(uid, tab.id);
    assert.equal(path, tab.id === "overview" ? "/integration/voice%20api%2Fv3" : `/integration/voice%20api%2Fv3/${tab.id}`);
    assert.deepEqual(parseConsolePath(path), routeForIntegration(uid, tab.id));
    assert.deepEqual(parseConsolePath(`${path}/`), routeForIntegration(uid, tab.id));
  }
});

test("retired API tabs resolve to their simpler destination", () => {
  const uid = "voice-api";
  assert.deepEqual(parseConsolePath(`/integration/${uid}/tools`), routeForIntegration(uid, "resources"));
  assert.deepEqual(parseConsolePath(`/integration/${uid}/usage`), routeForIntegration(uid, "overview"));
  assert.deepEqual(parseConsolePath(`/integration/${uid}/support`), routeForIntegration(uid, "overview"));
  assert.deepEqual(parseConsolePath(`/integration/${uid}/revisions`), routeForIntegration(uid, "history"));
});

test("unknown and malformed paths render the console not-found route", () => {
  assert.deepEqual(parseConsolePath("/not-a-section"), {
    kind: "not-found",
    section: "product",
    path: "/not-a-section",
  });
  assert.equal(parseConsolePath("/integration/%E0%A4%A").kind, "not-found");
  assert.equal(parseConsolePath("/integration").kind, "not-found");
  assert.equal(parseConsolePath("/integration/voice-api/not-a-tab").kind, "not-found");
});
