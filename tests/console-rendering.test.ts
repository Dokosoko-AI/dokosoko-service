import assert from "node:assert/strict";
import test from "node:test";

import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { ConsoleSidebar, ConsoleTopbar } from "../app/components/console/workspace-navigation";

const noop = () => {};

test("renders the promoted developer-asset destinations as accessible primary links", () => {
  const html = renderToStaticMarkup(createElement(ConsoleSidebar, {
    section: "tools",
    activeNavigationID: "tools",
    currentUser: {
      id: "user_1",
      email: "admin@example.com",
      display_name: "Admin User",
      role: "root",
    },
    onNavigate: noop,
  }));

  assert.match(html, /<nav aria-label="Main navigation">/);
  for (const [label, path] of [
    ["APIs", "/integrations"],
    ["Docs", "/integrations/documentation"],
    ["SDKs and packages", "/developer-assets/sdk-packages"],
    ["Identity", "/identity"],
    ["Tools", "/tools"],
    ["Recipes", "/recipes"],
    ["Agent access", "/agent-access"],
    ["Support outbox", "/operations/outbox"],
  ]) {
    assert.match(html, new RegExp(`href="${path}"[^>]*>[\\s\\S]*?<span>${label}</span>`));
  }
  assert.match(html, /href="\/tools" class="nav-item active" aria-current="page"/);
  assert.match(html, /<strong>Admin User<\/strong>/);
  assert.match(html, /<span class="avatar">AU<\/span>/);
});

test("renders the same destination model in the mobile console selector", () => {
  const html = renderToStaticMarkup(createElement(ConsoleTopbar, {
    productName: "Developer Platform",
    section: "recipes",
    activeNavigationID: "recipes",
    onGroupChange: noop,
  }));

  assert.match(html, /<select class="mobile-navigation" aria-label="Console section">/);
  assert.match(html, /<option value="recipes" selected="">Recipes<\/option>/);
  for (const label of ["APIs", "Docs", "SDKs and packages", "Identity", "Tools", "Recipes", "Agent access", "Support outbox", "Settings"]) {
    assert.match(html, new RegExp(`>${label}</option>`));
  }
  assert.match(html, /<strong>Developer Platform<\/strong>/);
});
