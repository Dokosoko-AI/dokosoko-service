import assert from "node:assert/strict";
import test from "node:test";
import { aiSettingsForSetup, aiSetupReturnPath } from "../app/lib/ai-setup-navigation";

test("AI settings retains the exact setup route without accepting external destinations", () => {
  const route = "/developer-assets/api-contracts?contract=exact-contract&api=exact-api&run=exact-import";
  const settings = aiSettingsForSetup(route);
  assert.equal(aiSetupReturnPath(settings.split("?")[1]), route);
  for (const value of ["https://evil.test", "//evil.test", "/\\evil.test", "/bad-route", "/settings/ai", "/settings/ai?returnTo=/recipes", "/recipes\n", "javascript:alert(1)"]) assert.equal(aiSetupReturnPath(new URLSearchParams({ returnTo: value }).toString()), "", value);
  assert.equal(aiSetupReturnPath(""), "");
});
