import assert from "node:assert/strict";
import test from "node:test";
import { sdkSetupSelection } from "../app/components/console/developer-assets/sdk-catalog-helpers";

test("resumed package setup keeps its exact requested release and fails closed when missing", () => {
  const values = [{ id: "other-release" }, { id: "requested-release" }];
  assert.equal(sdkSetupSelection("", values, "requested-release"), "requested-release");
  assert.equal(sdkSetupSelection("missing-release", values, "missing-release"), "");
  assert.equal(sdkSetupSelection("other-release", values, "requested-release"), "other-release", "an explicit operator selection is retained");
  assert.equal(sdkSetupSelection("", values, ""), "other-release");
});
