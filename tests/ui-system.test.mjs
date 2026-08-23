import assert from "node:assert/strict";
import { readFile, readdir } from "node:fs/promises";
import test from "node:test";

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
  const source = await readFile(appFile("app/components/ConsoleApp.tsx"), "utf8");
  const layout = await readFile(appFile("app/components/core/layout.tsx"), "utf8");
  const styles = await readFile(appFile("app/globals.css"), "utf8");
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
  assert.match(routes, /"audit-event":\s*"runs"/);
});

test("maps the owned typography and Figma semantic theme into one UI contract", async () => {
  const figmaTheme = await readFile(repositoryFile("New Figma Designs - DokoSoko Control Plane UI/src/styles/theme.css"), "utf8");
  const layout = await readFile(appFile("app/layout.tsx"), "utf8");
  const styles = await readFile(appFile("app/globals.css"), "utf8");
  const themeToggle = await readFile(appFile("app/components/ThemeToggle.tsx"), "utf8");
  const consoleApp = await readFile(appFile("app/components/ConsoleApp.tsx"), "utf8");

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

test("keeps page spacing responsive and independent of view-specific max widths", async () => {
  const styles = await readFile(appFile("app/globals.css"), "utf8");

  assert.match(styles, /--page-gutter:\s*clamp\(1rem, 2vw, 2rem\)/);
  assert.match(styles, /--page-block:\s*clamp\(1\.5rem, 2vw, 2rem\)/);
  assert.match(styles, /\.content\s*\{[^}]*width:\s*auto[^}]*padding:\s*var\(--page-block\) var\(--page-gutter\)/);
  assert.match(styles, /\.topbar\s*\{[^}]*padding-inline:\s*var\(--page-gutter\)/);
  assert.match(styles, /@media \(max-width: 720px\)[\s\S]*?\.content\s*\{[^}]*padding:\s*24px 16px 48px/);
  assert.match(styles, /@media \(max-width: 720px\)[\s\S]*?\.compact-audit-row\s*\{[^}]*grid-template-columns:\s*38px minmax\(0, 1fr\)/);
  assert.match(styles, /\.compact-audit-row > code\s*\{[^}]*display:\s*none/);
  assert.doesNotMatch(styles, /--content-max|width:\s*min\(1120px/);
});

test("keeps the Recipes authoring workflow inside the same global route stack", async () => {
  const source = await readFile(appFile("app/components/ConsoleApp.tsx"), "utf8");
  const styles = await readFile(appFile("app/globals.css"), "utf8");

  assert.doesNotMatch(source, /workflow-frame|recipe-workspace/);
  assert.match(source, /className="recipe-library-row"/);
  assert.match(source, /className="recipe-editor-layout"/);
  assert.match(source, /className="recipe-markdown-input"/);
  assert.doesNotMatch(styles, /--workflow-width|\.workflow-frame/);
  assert.match(styles, /\.recipe-library-row\s*\{[^}]*38px minmax\(0, 1fr\) auto 92px 18px/);
  assert.match(styles, /\.recipe-editor-layout\s*\{[^}]*minmax\(0, 1fr\) 340px/);
  assert.match(styles, /\.recipe-markdown-input\s*\{[^}]*min-height:\s*620px/);
  assert.match(source, /<aside className="recipe-editor-sidebar">[\s\S]*?recipe-ai-rework/);
});

test("limits typography to the owned six-step scale and four weights", async () => {
  const styles = await readFile(appFile("app/globals.css"), "utf8");
  const tokenBlock = styles.match(/:root\s*\{([\s\S]*?)\n\}/)?.[1] ?? "";

  for (const [token, value] of [
    ["text-caption", ".75rem"], ["text-body", ".8125rem"], ["text-label", ".875rem"],
    ["text-heading", "1rem"], ["text-metric", "1.25rem"], ["text-title", "1.75rem"],
    ["weight-regular", "400"], ["weight-medium", "500"], ["weight-semibold", "600"], ["weight-bold", "700"],
  ]) assert.match(tokenBlock, new RegExp(`--${token}:\\s*${value.replace(".", "\\.")}`));

  const declarations = styles.replace(tokenBlock, "");
  assert.doesNotMatch(declarations, /font-size:\s*(?!0(?:[;}]))(?:\d*\.)?\d+(?:px|rem|em)/);
  assert.doesNotMatch(declarations, /font-weight:\s*\d+/);
  assert.match(styles, /body\s*\{[^}]*font-family:\s*var\(--font-ui\)/);
  assert.match(styles, /code, pre, kbd, samp\s*\{[^}]*font-family:\s*var\(--font-code\)/);
  assert.match(styles, /strong, b\s*\{[^}]*font-weight:\s*var\(--weight-semibold\)/);
});

test("keeps interactive controls semantic inside shared data tables", async () => {
  const source = await readFile(appFile("app/components/ConsoleApp.tsx"), "utf8");
  const layout = await readFile(appFile("app/components/core/layout.tsx"), "utf8");
  const table = await readFile(appFile("app/components/core/table.tsx"), "utf8");
  const styles = await readFile(appFile("app/globals.css"), "utf8");

  assert.match(layout, /role="table"/);
  assert.match(layout, /role="row"/);
  assert.match(layout, /"columnheader"/);
  assert.match(layout, /aria-colspan=\{columns\}/);
  assert.match(source, /<DataTableRow key=\{integration\.id\}/);
  assert.doesNotMatch(source, /<ConsoleLink[^>]*role="row"/);
  assert.match(source, /<Table label="AI workloads" dense className="ai-settings-table ai-workload-table">/);
  assert.match(source, /<Table label="AI providers" dense className="ai-settings-table ai-provider-table">/);
  assert.match(source, /<colgroup><col className="ai-workload-column"/);
  assert.match(source, /<colgroup><col className="ai-provider-identity-column"/);
  assert.doesNotMatch(source, /className="ai-security-rail"/);
  assert.match(styles, /\.core-table\s*\{[^}]*width:\s*100%[^}]*min-width:\s*100%/);
  assert.match(styles, /\.ai-settings-table table\s*\{[^}]*min-width:\s*860px[^}]*table-layout:\s*fixed/);
  assert.match(styles, /\.ai-table-actions\s*\{[^}]*width:\s*100%[^}]*justify-content:\s*flex-end/);
  assert.match(table, /<caption className="sr-only">\{label\}<\/caption>/);
  assert.match(table, /className="core-table-frame"/);
  assert.match(table, /className=\{clsx\(className, 'core-table-scroll'\)\}/);
  assert.doesNotMatch(table, /-mx-\(--gutter\)|sm:first:pl-1|sm:last:pr-1/);
  assert.match(styles, /\.core-table :is\(\.core-table-header, \.core-table-cell\):first-child\s*\{[^}]*padding-left:\s*var\(--table-gutter, 16px\)/);
  assert.match(styles, /\.row-arrow[^{]*\{[^}]*width:\s*32px[^}]*height:\s*32px/);
  assert.match(styles, /\.core-switch\s*\{[^}]*width:\s*40px[^}]*height:\s*24px/);
  assert.match(styles, /\.entity-link\s*\{[^}]*min-height:\s*24px/);
});

test("keeps application colors inside the semantic light and dark token schemes", async () => {
  const styles = await readFile(appFile("app/globals.css"), "utf8");
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
