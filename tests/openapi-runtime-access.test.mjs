import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const openapi = await readFile(new URL("../api/openapi.yaml", import.meta.url), "utf8");

function boundedSection(source, startMarker, endMarker) {
  const start = source.indexOf(startMarker);
  assert.notEqual(start, -1, `${startMarker.trim()} should exist`);
  const end = source.indexOf(endMarker, start + startMarker.length);
  assert.notEqual(end, -1, `${endMarker.trim()} should follow ${startMarker.trim()}`);
  return source.slice(start, end);
}

function runtimeSchema(name, nextName) {
  return boundedSection(openapi, `    ${name}:`, `\n    ${nextName}:`);
}

test("publishes every runtime Access administration route", () => {
  const paths = boundedSection(
    openapi,
    "  /api/v1/integrations/{integration_id}/runtime-setup:",
    "\n  /api/v1/integrations/{integration_id}/access-connections:",
  );

  for (const path of [
    "/api/v1/integrations/{integration_id}/runtime-setup:",
    "/api/v1/integrations/{integration_id}/runtime-connections:",
    "/api/v1/runtime-service-connections/{connection_id}/check:",
    "/api/v1/integrations/{integration_id}/runtime-credential-sets:",
    "/api/v1/runtime-credential-sets/{credential_set_id}:",
    "/api/v1/runtime-credential-sets/{credential_set_id}/usage:",
    "/api/v1/runtime-credential-sets/{credential_set_id}/rotate:",
    "/api/v1/runtime-credential-sets/{credential_set_id}/versions/{version_id}/revoke:",
  ]) {
    assert.ok(paths.includes(path), `${path} should be in the runtime Access contract`);
  }

  for (const operationId of [
    "getIntegrationRuntimeSetup",
    "configureIntegrationRuntimeSetup",
    "listIntegrationRuntimeServiceConnections",
    "createIntegrationRuntimeServiceConnection",
    "checkRuntimeServiceConnection",
    "createIntegrationRuntimeCredentialSet",
    "getRuntimeCredentialSet",
    "getRuntimeCredentialUsage",
    "rotateRuntimeCredential",
    "revokeRuntimeCredentialVersion",
  ]) {
    assert.match(paths, new RegExp(`operationId: ${operationId}\\b`));
  }

  for (const schema of [
    "RuntimeSetup",
    "RuntimeSetupRequest",
    "RuntimeServiceConnection",
    "RuntimeServiceConnectionList",
    "RuntimeServiceConnectionReadiness",
    "RuntimeServiceConnectionRequest",
    "RuntimeCredentialSet",
    "RuntimeCredentialSetRequest",
    "RuntimeCredentialUsage",
    "RotateRuntimeCredentialRequest",
  ]) {
    assert.ok(paths.includes(`#/components/schemas/${schema}`), `${schema} should be referenced by a runtime route`);
  }
});

test("keeps runtime Access response contracts credential-redacted", () => {
  const schemas = boundedSection(openapi, "    RuntimeAuthenticationType:", "\n    Integration:");

  for (const forbidden of ["secret_id", "secretId", "SecretID", "ciphertext", "nonce", "credential_value"]) {
    assert.ok(!schemas.includes(forbidden), `${forbidden} must stay outside every runtime Access schema`);
  }

  const responseSchemas = [
    ["RuntimeServiceConnectionRevision", "RuntimeServiceConnection"],
    ["RuntimeServiceConnection", "RuntimeServiceConnectionList"],
    ["RuntimeServiceConnectionList", "RuntimeServiceConnectionCheck"],
    ["RuntimeServiceConnectionCheck", "RuntimeServiceConnectionReadiness"],
    ["RuntimeServiceConnectionReadiness", "RuntimeCredentialVersion"],
    ["RuntimeCredentialVersion", "RuntimeCredentialSet"],
    ["RuntimeCredentialSet", "RuntimeCredentialSetList"],
    ["RuntimeCredentialSetList", "RuntimeCredentialUsage"],
    ["RuntimeCredentialUsage", "RuntimeSetup"],
    ["RuntimeSetup", "RuntimeSetupRequest"],
  ];
  for (const [name, nextName] of responseSchemas) {
    const schema = runtimeSchema(name, nextName);
    assert.match(schema, /additionalProperties: false/);
    assert.doesNotMatch(schema, /^\s{8}credential:/m, `${name} must not expose a credential field`);
    assert.doesNotMatch(schema, /writeOnly:/, `${name} is a response schema`);
  }

  const credentialProperties = schemas.match(/^\s{8}credential:/gm) ?? [];
  assert.equal(credentialProperties.length, 3, "only the three write request schemas may accept credential values");
  for (const [name, nextName] of [
    ["RuntimeSetupRequest", "RuntimeServiceConnectionRequest"],
    ["RuntimeCredentialSetRequest", "RotateRuntimeCredentialRequest"],
    ["RotateRuntimeCredentialRequest", "Integration"],
  ]) {
    const schema = runtimeSchema(name, nextName);
    assert.match(schema, /^\s{8}credential: \{[^\n]*format: password[^\n]*writeOnly: true/m);
  }

  assert.doesNotMatch(runtimeSchema("RuntimeServiceConnectionRequest", "RuntimeCredentialSetRequest"), /^\s{8}credential:/m);
});
