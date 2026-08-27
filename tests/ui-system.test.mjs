import assert from "node:assert/strict";
import { readFile, readdir } from "node:fs/promises";
import test from "node:test";

import { clientSource, consoleSource, stylesSource } from "./source-surface.mjs";

const appFile = (path) => new URL(`../${path}`, import.meta.url);
const repositoryFile = (path) => new URL(`../../${path}`, import.meta.url);

test("keeps the complete source inventory inside the owned core component layer", async () => {
  const importedDirectory = appFile("app/components/core/");
  const componentFiles = (entries) => entries.filter((name) => name.endsWith(".tsx")).sort();
  const expected = [
    "alert.tsx", "auth-layout.tsx", "avatar.tsx", "badge.tsx", "button.tsx",
    "checkbox.tsx", "combobox.tsx", "description-list.tsx", "dialog.tsx",
    "divider.tsx", "dropdown.tsx", "fieldset.tsx", "heading.tsx", "input.tsx",
    "link.tsx", "listbox.tsx", "navbar.tsx", "pagination.tsx", "radio.tsx",
    "select.tsx", "sidebar-layout.tsx", "sidebar.tsx", "stacked-layout.tsx",
    "switch.tsx", "table.tsx", "text.tsx", "textarea.tsx",
  ];

  const coreFiles = componentFiles(await readdir(importedDirectory));
  for (const file of expected) assert.ok(coreFiles.includes(file), `${file} should exist in core`);
  assert.ok(coreFiles.includes("control.tsx"), "core should include the DokoSoko control composition");
  assert.ok(coreFiles.includes("layout.tsx"), "core should include the shared layout composition");

  const composition = await readFile(appFile("app/components/core/control.tsx"), "utf8");
  for (const component of ["badge", "button", "switch"]) {
    assert.match(composition, new RegExp(`from "\\.\\/${component}"`));
  }
  for (const primitive of ["BaseButton", "BaseBadge", "BaseSwitch"]) {
    assert.match(composition, new RegExp(`<${primitive}\\b`));
  }
});

test("uses one interface contract for headers, tabs, sections, panels, filters, and rows", async () => {
  const source = await consoleSource();
  const layout = await readFile(appFile("app/components/core/layout.tsx"), "utf8");
  const styles = await stylesSource();
  const routes = await readFile(appFile("app/lib/console-routes.ts"), "utf8");

  for (const primitive of ["ViewStack", "PageHeader", "PageTabs", "SectionHeader", "PanelHeader", "SegmentedControl", "DataTable", "DataTableHeader", "DataTableRow", "DataTableEmpty"]) {
    assert.match(layout, new RegExp(`export function ${primitive}\\b`));
    assert.match(source, new RegExp(`\\b${primitive}\\b`));
  }
  for (const legacyClass of ["section-tabs", "settings-tabs", "integration-tabs", "filter-tabs", "activity-toolbar"]) {
    assert.doesNotMatch(`${source}\n${styles}`, new RegExp(`\\b${legacyClass}\\b`));
  }
  for (const token of ["height-tab", "height-row", "space-view-section", "radius-panel"]) {
    assert.match(styles, new RegExp(`--${token}:`));
  }
  assert.match(styles, /\.table-head, \.table-row\s*\{[^}]*min-height:\s*var\(--height-row\)[^}]*padding:\s*10px 18px/);
  assert.match(styles, /\.panel-heading\s*\{[^}]*min-height:\s*var\(--height-row\)[^}]*padding:\s*16px 18px/);
  assert.match(styles, /\.provider-row\s*\{[^}]*min-height:\s*var\(--height-row\)[^}]*padding:\s*12px 18px/);
  assert.doesNotMatch(routes, /analytics:|activity:\s*"\/insights/);
  assert.match(routes, /"audit-event":\s*"reporting"/);
});

test("maps the owned typography and Figma semantic theme into one UI contract", async () => {
  const figmaTheme = await readFile(repositoryFile("New Figma Designs - DokoSoko Control Plane UI/src/styles/theme.css"), "utf8");
  const layout = await readFile(appFile("app/layout.tsx"), "utf8");
  const styles = await stylesSource();
  const themeToggle = await readFile(appFile("app/components/ThemeToggle.tsx"), "utf8");
  const consoleApp = await consoleSource();

  assert.match(figmaTheme, /--primary:\s*#4f46e5/);
  assert.match(layout, /Geist, JetBrains_Mono/);
  assert.match(layout, /--font-geist/);
  assert.match(layout, /--font-jetbrains-mono/);
  assert.match(layout, /<html[\s\S]*className=\{`\$\{geist\.variable\} \$\{jetBrainsMono\.variable\}`\}/);
  assert.doesNotMatch(layout, /<body className=/);
  assert.doesNotMatch(`${layout}\n${styles}`, /font-inter|\bInter\b|Geist_Mono/);
  assert.match(styles, /--font-ui:\s*var\(--font-geist\), Geist/);

  const rootTheme = styles.match(/:root\s*\{([\s\S]*?)\n\}/)?.[1] ?? "";
  const darkTheme = styles.match(/html\[data-theme="dark"\]\s*\{([\s\S]*?)\n\}/)?.[1] ?? "";
  for (const token of ["ink", "text-strong", "text", "muted", "subtle", "line", "line-strong", "surface", "surface-elevated", "surface-subtle", "soft", "accent", "accent-fill", "on-accent", "inverse-surface", "inverse-ink", "code-surface", "code-ink", "overlay", "success", "warning", "danger", "sidebar"]) {
    assert.match(rootTheme, new RegExp(`--${token}:`), `light theme should define --${token}`);
    assert.match(darkTheme, new RegExp(`--${token}:`), `dark theme should define --${token}`);
  }

  assert.match(styles, /\.core-control-shell::before\s*\{[^}]*var\(--surface\)/);
  assert.match(styles, /:is\(\.core-input, \.core-select, \.core-textarea\)\s*\{[^}]*var\(--ink\)[^}]*var\(--line-strong\)[^}]*var\(--surface\)/);
  assert.match(styles, /\.core-select option, \.core-select optgroup\s*\{[^}]*var\(--surface-elevated\)/);
  assert.match(styles, /\.auth-field textarea\s*\{[^}]*var\(--font-ui\)/);
  assert.match(styles, /\.auth-field textarea\.code-input\s*\{[^}]*var\(--font-code\)/);
  assert.match(themeToggle, /role="switch"/);
  assert.match(themeToggle, /aria-checked=\{dark\}/);
  assert.match(themeToggle, /import \{ Moon, Sun \} from "lucide-react"/);
  assert.match(themeToggle, /dark \? <Moon aria-hidden="true" \/> : <Sun aria-hidden="true" \/>/);
  assert.doesNotMatch(themeToggle, /theme-toggle-(?:label|track)/);
  assert.match(consoleApp, /className="sidebar-bottom">\s*<ThemeToggle \/>/);
  assert.match(consoleApp, /className="mobile-theme-toggle"><ThemeToggle \/><\/div>/);
  assert.match(styles, /\.theme-toggle\s*\{[^}]*width:\s*38px[^}]*height:\s*38px[^}]*display:\s*grid[^}]*place-items:\s*center[^}]*border:\s*0[^}]*background:\s*transparent/);
  assert.match(styles, /\.theme-toggle svg\s*\{[^}]*width:\s*17px[^}]*height:\s*17px/);
  assert.doesNotMatch(styles, /theme-toggle-(?:label|track)/);
  assert.match(styles, /\.mobile-theme-toggle \.theme-toggle\s*\{[^}]*width:\s*34px[^}]*height:\s*34px/);
  assert.match(styles, /\.mobile-theme-toggle\s*\{\s*display:\s*none/);
});

test("keeps desktop workspaces focused without constraining builders", async () => {
  const styles = await stylesSource();
  const source = await consoleSource();

  assert.match(styles, /--page-gutter:\s*clamp\(1rem, 2vw, 2rem\)/);
  assert.match(styles, /--page-block:\s*clamp\(1\.5rem, 2vw, 2rem\)/);
  assert.match(styles, /--workspace-default:\s*90rem/);
  assert.match(styles, /--workspace-compact:\s*70rem/);
  assert.match(styles, /--workspace-wide:\s*100rem/);
  assert.match(styles, /main\s*\{[^}]*--workspace-max:\s*var\(--workspace-default\)/);
  assert.match(styles, /main\.workspace-compact\s*\{[^}]*var\(--workspace-compact\)/);
  assert.match(styles, /main\.workspace-wide\s*\{[^}]*var\(--workspace-wide\)/);
  assert.match(styles, /\.content\s*\{[^}]*width:\s*100%[^}]*max-width:\s*var\(--workspace-max\)[^}]*margin-inline:\s*auto[^}]*padding:\s*var\(--page-block\) var\(--page-gutter\)/);
  assert.match(styles, /\.topbar-inner\s*\{[^}]*max-width:\s*var\(--workspace-max\)[^}]*margin-inline:\s*auto[^}]*padding-inline:\s*var\(--page-gutter\)/);
  assert.match(source, /consoleRoute\.kind === "tool-builder"[\s\S]*?"workspace-wide"[\s\S]*?section === "settings"[\s\S]*?"workspace-compact"/);
  assert.doesNotMatch(source, /section === "identity"[\s\S]{0,80}?"workspace-compact"/);
  assert.match(source, /<main id="main-content" className=\{workspaceClass\}/);
  assert.match(source, /<header className="topbar">\s*<div className="topbar-inner">/);
  assert.match(styles, /\.developer-document-columns\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\) 128px 76px/);
  assert.match(styles, /\.developer-document-columns\.table-head > :nth-child\(2\)\s*\{[^}]*white-space:\s*nowrap/);
  assert.match(styles, /@media \(max-width: 720px\)[\s\S]*?\.content\s*\{[^}]*padding:\s*24px 16px 48px/);
});

test("keeps the Recipes authoring workflow inside the same global route stack", async () => {
  const source = await consoleSource();
  const styles = await stylesSource();

  assert.doesNotMatch(source, /workflow-frame|recipe-workspace/);
  assert.match(source, /title="Recipes"/);
  assert.match(source, /Generate from evidence/);
  assert.match(source, /Create recipe/);
  assert.match(source, /visibleRecipes\.map\(renderRecipe\)/);
  assert.match(source, /unscopedOrInvalidRecipes\.map\(renderRecipe\)/);
  assert.doesNotMatch(styles, /--workflow-width|\.workflow-frame/);
  assert.doesNotMatch(styles, /\.recipe-library-row|\.recipe-editor-layout|\.recipe-markdown-input/);
});

test("keeps API Resources attachment-only and pinned to reviewed exact versions", async () => {
  const source = await consoleSource();
  const resources = await readFile(appFile("app/components/console/developer-assets/api-resources-workspace.tsx"), "utf8");

  assert.match(source, /activeTab === "documentation"[\s\S]*<APIResourcesWorkspace/);
  assert.doesNotMatch(resources, /ExactVersionNotice|This page contains attachment records only/);
  assert.match(resources, /Open catalog/);
  assert.match(resources, /panelKind === "contract" \? "Create in Catalog" : "Create & attach"/);
  assert.match(resources, /kind === "contract" \? "Create in Catalog" : "Create & attach exact resource"/);
  assert.match(resources, /Next steps happen in Catalog/);
  assert.match(resources, /Attach existing/);
  assert.match(resources, /Change exact/);
  assert.match(resources, /Detach resource/);
  assert.match(resources, /apiResourcePublications/);
  assert.doesNotMatch(resources, /crawlSource|Crawl source|Documentation ingestion/);
});

test("limits typography to the owned six-step scale, line heights, and four weights", async () => {
  const styles = await stylesSource();
  const tokenBlock = styles.match(/:root\s*\{([\s\S]*?)\n\}/)?.[1] ?? "";

  for (const [token, value] of [
    ["text-caption", ".75rem"], ["text-body", ".8125rem"], ["text-label", ".875rem"],
    ["text-heading", "1rem"], ["text-metric", "1.25rem"], ["text-title", "1.75rem"],
    ["leading-caption", "1.125rem"], ["leading-dense", "1.25rem"], ["leading-control", "1.375rem"],
    ["leading-heading", "1.5rem"], ["leading-section", "1.75rem"], ["leading-title", "2.25rem"],
    ["weight-regular", "400"], ["weight-medium", "500"], ["weight-semibold", "600"], ["weight-bold", "700"],
  ]) assert.match(tokenBlock, new RegExp(`--${token}:\\s*${value.replace(".", "\\.")}`));

  const declarations = styles.replace(tokenBlock, "");
  assert.doesNotMatch(declarations, /font-size:\s*(?!0(?:[;}]))(?:\d*\.)?\d+(?:px|rem|em)/);
  assert.doesNotMatch(declarations, /font-weight:\s*\d+/);
  assert.match(styles, /body\s*\{[^}]*font-family:\s*var\(--font-ui\)/);
  assert.match(styles, /code, pre, kbd, samp\s*\{[^}]*font-family:\s*var\(--font-code\)/);
  assert.match(styles, /strong, b\s*\{[^}]*font-weight:\s*var\(--weight-semibold\)/);
});

test("maps dashboard, RootGate, and OIDC typography onto semantic roles", async () => {
  const styles = await stylesSource();
  const layout = await readFile(appFile("app/components/core/layout.tsx"), "utf8");
  const rootGate = await readFile(appFile("app/components/RootGate.tsx"), "utf8");
  const identitySetup = await readFile(appFile("app/components/OIDCIdentitySetup.tsx"), "utf8");
  const primitiveSources = (await Promise.all([
    "button.tsx", "dialog.tsx", "heading.tsx", "input.tsx", "select.tsx", "text.tsx", "textarea.tsx",
  ].map((file) => readFile(appFile(`app/components/core/${file}`), "utf8")))).join("\n");

  for (const [role, size, weight, leading] of [
    ["page-title", "text-title", "weight-semibold", "leading-title"],
    ["section-title", "text-metric", "weight-semibold", "leading-section"],
    ["heading", "text-heading", "weight-semibold", "leading-heading"],
    ["body-large", "text-heading", "weight-regular", "leading-heading"],
    ["body", "text-label", "weight-regular", "leading-control"],
    ["dense", "text-body", "weight-regular", "leading-dense"],
    ["control", "text-label", "weight-medium", "leading-control"],
    ["caption", "text-caption", "weight-regular", "leading-caption"],
  ]) {
    assert.match(styles, new RegExp(`\\.type-${role}\\s*\\{[^}]*font-size:\\s*var\\(--${size}\\)[^}]*font-weight:\\s*var\\(--${weight}\\)[^}]*line-height:\\s*var\\(--${leading}\\)`));
  }

  assert.match(layout, /<h1 className="type-page-title">\{title\}<\/h1>/);
  assert.match(layout, /<p className="type-body-large">\{description\}<\/p>/);
  assert.match(layout, /<h2 className="type-section-title">\{title\}<\/h2>/);
  assert.match(layout, /<Heading className="type-heading">\{title\}<\/Heading>/);
  assert.match(rootGate, /<h1 className="type-section-title">\{title\}<\/h1><p className="type-body">\{description\}<\/p>/);
  assert.match(rootGate, /<footer className="type-caption">Private by default/);
  assert.match(identitySetup, /<h2 className="type-heading">Test sign-in<\/h2><p className="type-body">/);
  assert.match(identitySetup, /<h2 className="type-heading">Activate customer sign-in<\/h2><p className="type-body">/);

  assert.match(styles, /\.auth-card h1\s*\{[^}]*font-size:\s*var\(--text-metric\)[^}]*font-weight:\s*var\(--weight-semibold\)[^}]*line-height:\s*var\(--leading-section\)/);
  assert.match(styles, /\.auth-card > p\s*\{[^}]*font-size:\s*var\(--text-label\)[^}]*line-height:\s*var\(--leading-control\)/);
  assert.match(styles, /\.auth-field input\s*\{[^}]*font-family:\s*var\(--font-ui\)[^}]*font-size:\s*var\(--text-label\)[^}]*line-height:\s*var\(--leading-control\)/);
  assert.match(styles, /\.auth-field\s*\{[^}]*font-size:\s*var\(--text-label\)[^}]*font-weight:\s*var\(--weight-medium\)[^}]*line-height:\s*var\(--leading-control\)/);
  assert.match(styles, /\.auth-field select\s*\{[^}]*font-size:\s*var\(--text-label\)[^}]*line-height:\s*var\(--leading-control\)/);
  assert.match(styles, /\.auth-field textarea\s*\{[^}]*font-size:\s*var\(--text-label\)[^}]*line-height:\s*var\(--leading-control\)/);
  assert.match(styles, /\.auth-problem\s*\{[^}]*font-size:\s*var\(--text-label\)[^}]*line-height:\s*var\(--leading-control\)/);
  assert.match(styles, /\.core-button\s*\{[^}]*font-size:\s*var\(--text-label\)[^}]*font-weight:\s*var\(--weight-medium\)[^}]*line-height:\s*var\(--leading-control\)/);
  assert.match(styles, /\.identity-verification-card h2\s*\{[^}]*font-size:\s*var\(--text-heading\)[^}]*font-weight:\s*var\(--weight-semibold\)[^}]*line-height:\s*var\(--leading-heading\)/);
  assert.match(styles, /\.identity-verification-card p\s*\{[^}]*font-size:\s*var\(--text-label\)[^}]*line-height:\s*var\(--leading-control\)/);
  assert.match(styles, /\.dialog-title\s*\{[^}]*font-size:\s*var\(--text-heading\)[^}]*font-weight:\s*var\(--weight-semibold\)[^}]*line-height:\s*var\(--leading-heading\)/);
  assert.match(styles, /\.entity-missing h1\s*\{[^}]*font-size:\s*var\(--text-metric\)[^}]*font-weight:\s*var\(--weight-semibold\)[^}]*line-height:\s*var\(--leading-section\)/);
  assert.match(styles, /@media \(max-width: 720px\)[\s\S]*?:is\(input:not\(\[type="checkbox"\]\):not\(\[type="radio"\]\):not\(\[type="range"\]\), select, textarea\)\s*\{[^}]*font-size:\s*var\(--text-heading\)[^}]*line-height:\s*var\(--leading-heading\)/);

  assert.equal(styles.match(/\.recovery-grid\s*\{/g)?.length, 1, "recovery grid should have one owned rule");
  assert.equal(styles.match(/\.recovery-grid code\s*\{/g)?.length, 1, "recovery code typography should not be overridden later");
  assert.match(primitiveSources, /type-page-title/);
  assert.match(primitiveSources, /type-heading/);
  assert.match(primitiveSources, /type-body/);
  assert.match(primitiveSources, /type-dense/);
  assert.match(primitiveSources, /type-control/);
  assert.doesNotMatch(primitiveSources, /text-2xl\/8|text-base\/6|sm:text-(?:xl\/8|sm\/6)|sm:text-\[0\.8125rem\]/);
  assert.doesNotMatch(`${rootGate}\n${identitySetup}`, /\bfont(?:Family|Size|Weight)\s*:/);
});

test("keeps interactive controls semantic inside shared data tables", async () => {
  const source = await consoleSource();
  const layout = await readFile(appFile("app/components/core/layout.tsx"), "utf8");
  const table = await readFile(appFile("app/components/core/table.tsx"), "utf8");
  const coreUI = await readFile(appFile("app/styles/core-ui.css"), "utf8");
  const styles = await stylesSource();

  assert.match(layout, /role="table"/);
  assert.match(layout, /role="row"/);
  assert.match(layout, /"columnheader"/);
  assert.match(layout, /aria-colspan=\{columns\}/);
  assert.match(source, /<DataTableRow key=\{integration\.id\}/);
  assert.doesNotMatch(source, /<ConsoleLink[^>]*role="row"/);
  assert.match(source, /<Table label="AI workload" dense>/);
  assert.match(source, /<TableHead><TableRow><TableHeader>Name<\/TableHeader><TableHeader>Provider<\/TableHeader><TableHeader>Model<\/TableHeader><TableHeader>Actions<\/TableHeader>/);
  assert.match(source, /connections\.map\(\(connection\)/);
  assert.doesNotMatch(source, /className="ai-security-rail"/);
  assert.match(styles, /\.core-table\s*\{[^}]*width:\s*100%[^}]*min-width:\s*100%/);
  assert.match(source, /className="panel ai-table-panel"/);
  assert.match(styles, /\.ai-table-panel\s*\{[^}]*--table-gutter:\s*18px[^}]*overflow:\s*hidden/);
  assert.match(styles, /\.ai-table-actions\s*\{[^}]*width:\s*100%[^}]*justify-content:\s*flex-end/);
  assert.match(table, /<caption className="sr-only">\{label\}<\/caption>/);
  assert.match(table, /className="core-table-frame"/);
  assert.match(table, /className=\{clsx\(className, 'core-table-scroll'\)\}/);
  assert.doesNotMatch(table, /-mx-\(--gutter\)|sm:first:pl-1|sm:last:pr-1/);
  assert.match(styles, /\.core-table :is\(\.core-table-header, \.core-table-cell\):first-child\s*\{[^}]*padding-left:\s*var\(--table-gutter, 16px\)/);
  assert.match(styles, /\.row-arrow[^{]*\{[^}]*width:\s*32px[^}]*height:\s*32px/);
  assert.match(styles, /\.core-switch\s*\{[^}]*width:\s*40px[^}]*height:\s*24px/);
  assert.match(styles, /\.entity-link\s*\{[^}]*min-height:\s*24px/);
  assert.match(coreUI, /\.source-columns\s*\{[^}]*minmax\(220px, 1\.4fr\)[^}]*300px/);
  assert.doesNotMatch(coreUI, /\.source-columns\s*\{[^}]*minmax\(30px, auto\)/);
});

test("keeps application colors inside the semantic light and dark token schemes", async () => {
  const styles = await stylesSource();
  const rootMatch = styles.match(/:root\s*\{([\s\S]*?)\n\}/);
  const darkMatch = styles.match(/html\[data-theme="dark"\]\s*\{([\s\S]*?)\n\}/);
  assert.ok(rootMatch && darkMatch);

  const outsideSchemes = styles.replace(rootMatch[0], "").replace(darkMatch[0], "");
  assert.doesNotMatch(outsideSchemes, /#[\da-f]{3,8}\b|rgba?\(/i);

  const hex = (block, token) => block.match(new RegExp(`--${token}:\\s*(#[\\da-f]{6})`, "i"))?.[1];
  const luminance = (value) => {
    const channels = value.slice(1).match(/../g).map((part) => parseInt(part, 16) / 255).map((part) => part <= .04045 ? part / 12.92 : ((part + .055) / 1.055) ** 2.4);
    return .2126 * channels[0] + .7152 * channels[1] + .0722 * channels[2];
  };
  const contrast = (left, right) => {
    const [bright, dark] = [luminance(left), luminance(right)].sort((a, b) => b - a);
    return (bright + .05) / (dark + .05);
  };

  for (const block of [rootMatch[1], darkMatch[1]]) {
    for (const surface of ["surface", "surface-subtle"]) {
      assert.ok(contrast(hex(block, "subtle"), hex(block, surface)) >= 4.5, `--subtle must remain readable on --${surface}`);
    }
  }
});

test("keeps widgets outside the service behind a future plugin contract", async () => {
  const consoleApp = await consoleSource();
  const client = await clientSource();
  const controlPlane = await readFile(appFile("api/openapi.yaml"), "utf8");
  const pluginReadme = await readFile(appFile("extensions/widget-plugin/README.md"), "utf8");
  const pluginSchema = await readFile(appFile("extensions/widget-plugin/plugin.schema.json"), "utf8");

  assert.doesNotMatch(consoleApp, /WidgetPreviewLauncher|WidgetDetailView|api\.widgets/);
  assert.doesNotMatch(client, /widgetConfiguration|widgetPreviewBootstrap|exchangeWidgetSession|widget-chat/);
  assert.doesNotMatch(controlPlane, /\/api\/v1\/widgets|WidgetSession|WidgetAgent/);
  assert.match(pluginReadme, /external widget/i);
  assert.match(pluginSchema, /"browser_entrypoint"[\s\S]*"backend_origin"[\s\S]*"private_mcp"/);
});
