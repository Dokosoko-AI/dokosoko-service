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

test("publishes the clean Authorization administration surface", () => {
  const paths = boundedSection(
    openapi,
    "  /api/v1/integrations/{integration_id}/authorization:",
    "\n  /api/v1/integrations/{integration_id}/authorization-points:",
  );

  for (const path of [
    "/api/v1/integrations/{integration_id}/authorization:",
    "/api/v1/authorizations:",
    "/api/v1/authorizations/{authorization_id}:",
    "/api/v1/authorizations/{authorization_id}/usage:",
    "/api/v1/authorizations/{authorization_id}/rotate:",
    "/api/v1/authorizations/{authorization_id}/versions/{version_id}/revoke:",
  ]) {
    assert.ok(paths.includes(path), `${path} should be in the Authorization contract`);
  }

  for (const operationId of [
    "getIntegrationAuthorization",
    "configureIntegrationAuthorization",
    "listAuthorizations",
    "getAuthorization",
    "updateAuthorization",
    "getAuthorizationUsage",
    "rotateAuthorizationCredential",
    "revokeAuthorizationCredentialVersion",
  ]) {
    assert.match(paths, new RegExp(`operationId: ${operationId}\\b`));
  }

  for (const obsoletePath of [
    "/runtime-setup:",
    "/runtime-connections:",
    "/runtime-service-connections/",
    "/runtime-credential-sets",
    "/runtime-authorizations",
  ]) {
    assert.ok(!openapi.includes(obsoletePath), `${obsoletePath} must not remain public`);
  }
});

test("keeps Authorization responses credential-redacted and hooks explicit", () => {
  const schemas = boundedSection(openapi, "    RuntimeAuthenticationType:", "\n    Integration:");

  for (const forbidden of ["secret_id", "secretId", "SecretID", "ciphertext", "nonce", "credential_value"]) {
    assert.ok(!schemas.includes(forbidden), `${forbidden} must stay outside every Authorization schema`);
  }

  for (const [name, nextName] of [
    ["RuntimeServiceConnectionRevision", "RuntimeServiceConnection"],
    ["RuntimeServiceConnection", "RuntimeCredentialVersion"],
    ["RuntimeCredentialVersion", "RuntimeCredentialSet"],
    ["RuntimeCredentialSet", "RuntimeAuthorizationProfileList"],
    ["RuntimeAuthorizationProfileList", "RuntimeCredentialUsage"],
    ["RuntimeCredentialUsage", "RuntimeSetup"],
    ["RuntimeSetup", "RuntimeSetupRequest"],
  ]) {
    const schema = runtimeSchema(name, nextName);
    assert.match(schema, /additionalProperties: false/);
    assert.doesNotMatch(schema, /^\s{8}credential:/m, `${name} must not expose a credential field`);
    assert.doesNotMatch(schema, /writeOnly:/, `${name} is a response schema`);
  }

  const authorization = runtimeSchema("RuntimeCredentialSet", "RuntimeAuthorizationProfileList");
  for (const hook of ["key_management_url", "access_evaluation_url", "usage_url"]) {
    assert.match(authorization, new RegExp(`^\\s{8}${hook}:`, "m"));
  }
  assert.doesNotMatch(authorization, /^\s{8}(feedback_submission_url|error_submission_url):/m);

  const deployment = runtimeSchema("DeploymentInput", "Deployment");
  assert.match(deployment, /^\s{8}feedback_submission_url:/m);
  assert.match(deployment, /^\s{8}error_submission_url:/m);
  assert.match(authorization, /^\s{8}environment_variable:/m);

  const authenticationConfiguration = runtimeSchema("RuntimeAuthenticationConfiguration", "RuntimeAuthorizationHeaderInput");
  assert.match(authenticationConfiguration, /^\s{8}headers:/m);
  assert.match(authenticationConfiguration, /Values are never returned/);

  const headerInput = runtimeSchema("RuntimeAuthorizationHeaderInput", "RuntimeServiceConnectionRevision");
  assert.match(headerInput, /^\s{8}name:/m);
  assert.match(headerInput, /^\s{8}value:\n\s{10}type: string\n\s{10}format: password[\s\S]*?^\s{10}writeOnly: true/m);

  const setup = runtimeSchema("RuntimeSetup", "RuntimeSetupRequest");
  assert.match(setup, /^\s{8}endpoint_bindings:/m);
  assert.match(setup, /^\s{8}authorizations:/m);
  assert.doesNotMatch(setup, /^\s{8}(service_connections|credential_sets):/m);

  const setupRequest = runtimeSchema("RuntimeSetupRequest", "UpdateAuthorizationRequest");
  assert.match(setupRequest, /^\s{8}authorization_id:/m);
  assert.match(setupRequest, /^\s{8}additional_headers:/m);
  assert.doesNotMatch(setupRequest, /^\s{8}(credential_name|credential_scope|credential_set_id):/m);

  const updateRequest = runtimeSchema("UpdateAuthorizationRequest", "RotateRuntimeCredentialRequest");
  assert.doesNotMatch(updateRequest, /^\s{8}name:/m);
  assert.match(updateRequest, /^\s{8}additional_headers:/m);

  const credentialProperties = schemas.match(/^\s{8}credential:/gm) ?? [];
  assert.equal(credentialProperties.length, 3, "only setup, update, and rotate requests may accept credentials");
  for (const [name, nextName] of [
    ["RuntimeSetupRequest", "UpdateAuthorizationRequest"],
    ["UpdateAuthorizationRequest", "RotateRuntimeCredentialRequest"],
    ["RotateRuntimeCredentialRequest", "Integration"],
  ]) {
    const schema = runtimeSchema(name, nextName);
    assert.match(schema, /^\s{8}credential:\n\s{10}type: string\n\s{10}format: password[\s\S]*?^\s{10}writeOnly: true/m);
  }
});
