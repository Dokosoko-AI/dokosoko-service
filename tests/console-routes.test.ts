import assert from "node:assert/strict";
import test from "node:test";

import {
  SECTION_PATHS,
  entityPath,
  parseConsolePath,
  routeForEntity,
  sectionPath,
} from "../app/lib/console-routes";

test("every console section has a canonical, round-trippable URL", () => {
  for (const [section, path] of Object.entries(SECTION_PATHS)) {
    assert.equal(sectionPath(section as keyof typeof SECTION_PATHS), path);
    assert.deepEqual(parseConsolePath(path), { kind: "section", section, path });
    assert.deepEqual(parseConsolePath(`${path}/`), { kind: "section", section, path });
  }
});

test("the root path canonicalizes to overview", () => {
  assert.deepEqual(parseConsolePath("/"), {
    kind: "section",
    section: "overview",
    path: "/overview",
  });
});

test("entity URLs encode UIDs and resolve to their owning section", () => {
  const uid = "voice api/v3";
  assert.equal(entityPath("integration", uid), "/integration/voice%20api%2Fv3");
  assert.deepEqual(parseConsolePath(entityPath("integration", uid)), routeForEntity("integration", uid));
  assert.equal(routeForEntity("source", "src_docs").section, "sources");
  assert.equal(routeForEntity("report", "report_123").section, "reporting");
});

test("unknown and malformed paths render the console not-found route", () => {
  assert.deepEqual(parseConsolePath("/not-a-section"), {
    kind: "not-found",
    section: "overview",
    path: "/not-a-section",
  });
  assert.equal(parseConsolePath("/integration/%E0%A4%A").kind, "not-found");
  assert.equal(parseConsolePath("/integration").kind, "not-found");
});
