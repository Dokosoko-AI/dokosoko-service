import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("keeps console page navigation seamless", async () => {
  const navigation = await readFile(
    new URL("../app/components/console/use-console-navigation.ts", import.meta.url),
    "utf8",
  );
  const consoleApp = await readFile(
    new URL("../app/components/ConsoleApp.tsx", import.meta.url),
    "utf8",
  );

  assert.match(navigation, /import \{ startTransition,/);
  assert.equal(
    navigation.match(/startTransition\(\(\) => setConsoleRoute\(next\)\);/g)?.length,
    2,
    "both direct navigation and history navigation should be non-blocking transitions",
  );
  assert.doesNotMatch(consoleApp, /\blazy\(/);
  assert.doesNotMatch(consoleApp, /<Suspense\b/);
  assert.doesNotMatch(consoleApp, /Loading console/);
});
