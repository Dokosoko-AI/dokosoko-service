import { readFile, readdir } from "node:fs/promises";
import ts from "typescript";

const repositoryFile = (path) => new URL(`../${path}`, import.meta.url);

async function readModuleSurface(paths) {
  return (await Promise.all(paths.map((path) => readFile(repositoryFile(path), "utf8")))).join("\n");
}

let englishTranslations;

function propertyName(node) {
  if (ts.isIdentifier(node) || ts.isStringLiteral(node) || ts.isNumericLiteral(node)) return node.text;
  return null;
}

function collectStringProperties(node, prefix, translations) {
  while (ts.isAsExpression(node) || ts.isSatisfiesExpression(node) || ts.isParenthesizedExpression(node)) node = node.expression;
  if (!ts.isObjectLiteralExpression(node)) return;
  for (const property of node.properties) {
    if (!ts.isPropertyAssignment(property)) continue;
    const name = propertyName(property.name);
    if (!name) continue;
    const key = prefix ? `${prefix}.${name}` : name;
    if (ts.isStringLiteralLike(property.initializer)) translations.set(key, property.initializer.text);
    else collectStringProperties(property.initializer, key, translations);
  }
}

async function loadEnglishTranslations() {
  if (englishTranslations) return englishTranslations;
  const translations = new Map();
  for (const path of ["app/i18n/locales/ui/en.ts", "app/i18n/locales/en.ts"]) {
    const source = await readFile(repositoryFile(path), "utf8");
    const file = ts.createSourceFile(path, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
    for (const statement of file.statements) {
      if (!ts.isVariableStatement(statement)) continue;
      for (const declaration of statement.declarationList.declarations) {
        if (declaration.initializer) collectStringProperties(declaration.initializer, "", translations);
      }
    }
  }
  englishTranslations = translations;
  return translations;
}

export async function resolveEnglishTranslations(source) {
  const translations = await loadEnglishTranslations();
  const resolved = source.replace(/\bt\("([^"]+)"\)/g, (call, key) => translations.has(key) ? JSON.stringify(translations.get(key)) : call);
  return resolved
    .replace(/=\{("(?:\\.|[^"\\])*")\}/g, "=$1")
    .replace(/\{("(?:\\.|[^"\\])*")\}/g, (_expression, literal) => JSON.parse(literal));
}

export async function consoleSource() {
  return resolveEnglishTranslations(await readModuleSurface([
    "app/components/console/workspace-navigation.tsx",
    "app/components/ConsoleApp.tsx",
    "app/components/console/integration-views.tsx",
    "app/components/console/integrations/authorization-policy-workspace.tsx",
    "app/components/console/integrations/tools-workspace.tsx",
    "app/components/console/integrations/test-workspace.tsx",
    "app/components/console/agent-access-views.tsx",
    "app/components/console/tool-views.tsx",
    "app/components/console/catalog-settings-views.tsx",
    "app/components/console/developer-assets/api-contracts-view.tsx",
    "app/components/console/developer-assets/api-resource-publication-history.tsx",
    "app/components/console/developer-assets/api-resources-workspace.tsx",
    "app/components/console/developer-assets/developer-asset-navigation.tsx",
    "app/components/console/developer-assets/developer-asset-ui.tsx",
    "app/components/console/developer-assets/documentation-collections-view.tsx",
    "app/components/console/developer-assets/documentation-explorer-view.tsx",
    "app/components/console/developer-assets/query-lab-view.tsx",
    "app/components/console/developer-assets/sdk-catalog-view.tsx",
    "app/components/console/console-link.tsx",
    "app/components/console/dialogs/admin-activity-dialogs.tsx",
    "app/components/console/dialogs/ai-configuration-dialogs.tsx",
    "app/components/console/dialogs/mcp-dialogs.tsx",
    "app/components/console/dialogs/publication-dialogs.tsx",
    "app/components/console/dialogs/recipe-approval-dialog.tsx",
    "app/components/console/dialogs/recipe-approval-review.ts",
    "app/components/console/dialogs/recipe-dialogs.tsx",
    "app/components/console/dialogs/recipe-spec-editor.ts",
    "app/components/console/dialogs/source-dialogs.tsx",
    "app/components/console/shared.tsx",
    "app/components/console/use-admin-activity-workspace.ts",
    "app/components/console/use-ai-workspace.ts",
    "app/components/console/use-console-navigation.ts",
    "app/components/console/use-entity-detail.ts",
    "app/components/console/use-mcp-workspace.ts",
    "app/components/console/use-publication-workflow.ts",
    "app/components/console/use-source-workflow.ts",
    "app/lib/console-domain.ts",
  ]));
}

export async function clientSource() {
  return readModuleSurface(["app/lib/api.ts", "app/lib/api-contracts.ts", "app/lib/api-client.ts", "app/lib/developer-assets-api.ts"]);
}

export async function stylesSource() {
  const paths = ["app/globals.css"];
  try {
    for (const name of (await readdir(repositoryFile("app/styles/"))).filter((value) => value.endsWith(".css")).sort()) {
      paths.push(`app/styles/${name}`);
    }
  } catch {
    // The styles directory is optional for older source layouts.
  }
  return readModuleSurface(paths);
}
