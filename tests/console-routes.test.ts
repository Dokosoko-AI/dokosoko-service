import assert from "node:assert/strict";
import test from "node:test";

import {
  INTEGRATION_TABS,
  SETTINGS_TABS,
  SECTION_PATHS,
  entityPath,
  integrationPath,
  parseConsolePath,
  routeForEntity,
  routeForIntegration,
  sectionPath,
  settingsPath,
} from "../app/lib/console-routes";

test("primary console sections have canonical, round-trippable URLs", () => {
  for (const section of ["product", "distribution", "widgets", "runs", "settings"] as const) {
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
  assert.equal(entityPath("widget", "widget_123"), "/widget/widget_123");
  assert.equal(routeForEntity("widget", "widget_123").section, "widgets");
  assert.equal(routeForEntity("report", "report_123").section, "reporting");
  assert.equal(routeForEntity("audit-event", "event_123").section, "runs");
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

test("settings has stable routes for every overview area", () => {
  assert.deepEqual(SETTINGS_TABS, [
    { id: "overview", label: "Overview" },
    { id: "identity", label: "Customer identity" },
    { id: "connections", label: "Service connections" },
    { id: "reporting", label: "Bug reports & feedback" },
    { id: "storage", label: "Database & storage" },
    { id: "ai", label: "AI providers" },
    { id: "root", label: "Root access" },
  ]);
  assert.equal(settingsPath(), "/settings");
  for (const tab of SETTINGS_TABS.filter((candidate) => candidate.id !== "overview")) {
    const path = settingsPath(tab.id);
    assert.equal(path, `/settings/${tab.id}`);
    assert.deepEqual(parseConsolePath(path), { kind: "section", section: "settings", settingsTab: tab.id, path });
  }
  assert.deepEqual(parseConsolePath("/settings/ai/"), parseConsolePath("/settings/ai"));
  assert.equal(parseConsolePath("/settings/models").kind, "not-found");
});

test("recipes have one stable product-level workspace", () => {
  assert.equal(sectionPath("recipes"), "/recipes");
  assert.deepEqual(parseConsolePath("/recipes"), {
    kind: "section",
    section: "recipes",
    path: "/recipes",
  });
  assert.deepEqual(parseConsolePath("/recipes/"), parseConsolePath("/recipes"));
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
  assert.equal(parseConsolePath("/insights").kind, "not-found");
  assert.equal(parseConsolePath("/insights/activity").kind, "not-found");
});
