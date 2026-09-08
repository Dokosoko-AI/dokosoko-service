import test from "node:test";
import assert from "node:assert/strict";
import { resolveSDKPackageLocation } from "../app/lib/sdk-package-locator";
test("package names retain the chosen ecosystem and explicit versions", () => {
 assert.deepEqual(resolveSDKPackageLocation("@acme/sdk@1.2.3", "npm"), { ecosystem: "npm", coordinate: "@acme/sdk", sourceURL: "https://registry.npmjs.org", exactVersion: "1.2.3" });
 assert.equal(resolveSDKPackageLocation("package-name", "cargo")?.ecosystem, "cargo");
 assert.equal(resolveSDKPackageLocation("package-name==2.3.4", "pypi")?.exactVersion, "2.3.4");
});
test("known registry links detect coordinates and do not invent a floating version", () => {
 assert.deepEqual(resolveSDKPackageLocation("https://www.npmjs.com/package/@scope/sdk/v/1.2.3", "pypi"), { ecosystem: "npm", coordinate: "@scope/sdk", exactVersion: "1.2.3", sourceURL: "https://registry.npmjs.org" });
 assert.equal(resolveSDKPackageLocation("https://pypi.org/project/acme-sdk/", "npm")?.coordinate, "acme-sdk");
 assert.equal(resolveSDKPackageLocation("https://crates.io/crates/acme/2.1.0", "npm")?.ecosystem, "cargo");
 assert.equal(resolveSDKPackageLocation("https://pkg.go.dev/github.com/acme/sdk@v1.2.3", "npm")?.coordinate, "github.com/acme/sdk");
 assert.equal(resolveSDKPackageLocation("https://pypi.org/project/acme-sdk", "npm")?.exactVersion, undefined);
});
test("custom sources and credential-bearing or ambiguous URLs need explicit correction", () => {
 for (const value of ["https://github.com/acme/sdk", "https://npmjs.com/package/a?token=secret", "https://user:secret@pypi.org/project/a", "http://npmjs.com/package/a", "https://registry.npmjs.org/a/-/a.tgz", "https://pypi.org/project/a/1.0/files", "npm install a"]) assert.equal(resolveSDKPackageLocation(value, "npm"), undefined, value);
});
