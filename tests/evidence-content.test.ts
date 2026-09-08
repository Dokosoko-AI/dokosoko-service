import assert from "node:assert/strict";
import test from "node:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { EvidenceContent, evidenceChange, evidenceLink } from "../app/components/core/evidence-content";

test("imported evidence renders prose and code without active HTML or remote images", () => {
  const html = renderToStaticMarkup(createElement(EvidenceContent, { label: "Evidence", text: '# Setup\n\nUse **reviewed** `code`.\n\n- First step\n- Next step\n\n```js\n<script>alert(1)</script>\n```\n\n<img src="https://tracker.example/pixel">\n\n[Unsafe](javascript:alert) [Safe](https://docs.example/guide)' }));
  assert.match(html, /<h2>Setup<\/h2>/);
  assert.match(html, /<strong>reviewed<\/strong>/);
  assert.match(html, /<ul><li>First step<\/li>/);
  assert.match(html, /&lt;script&gt;/);
  assert.doesNotMatch(html, /<script|<img|href="javascript:/);
  assert.match(html, /href="https:\/\/docs.example\/guide"/);
});

test("evidence links reject active schemes, credentials and relative destinations", () => {
  for (const value of ["javascript:alert(1)", "data:text/html,test", "//tracker.example", "/admin", "https://secret@host.example", "file:///tmp/private"]) assert.equal(evidenceLink(value), undefined);
  assert.equal(evidenceLink("#setup"), "#setup");
});

test("comparison preserves exact changed text while removing shared context", () => {
  assert.deepEqual(evidenceChange("Intro\nOld\nEnd", "Intro\nNew\nEnd"), { unchanged: false, removed: "Old", added: "New" });
  assert.deepEqual(evidenceChange("Same", "Same"), { unchanged: true, removed: "", added: "" });
  assert.deepEqual(evidenceChange("A", "A\nB"), { unchanged: false, removed: "", added: "B" });
});
