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

test("the root canonicalizes to APIs without legacy aliases", () => {
  assert.deepEqual(parseConsolePath("/"), {
    kind: "section",
    section: "product",
    path: "/integrations",
  });
  assert.equal(parseConsolePath("/overview").kind, "not-found");
  assert.equal(parseConsolePath("/distribution").kind, "not-found");
  assert.equal(parseConsolePath("/operations").kind, "not-found");
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

test("unknown API tabs do not create compatibility aliases", () => {
  const uid = "voice-api";
  for (const tab of ["tools", "usage", "support", "revisions"]) {
    assert.equal(parseConsolePath(`/integration/${uid}/${tab}`).kind, "not-found");
  }
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
