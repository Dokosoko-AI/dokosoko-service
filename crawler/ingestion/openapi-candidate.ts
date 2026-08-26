import { canonicalJSON, compareText, sha256 } from "./canonical";
import { parseStructuredValue } from "./structured";

const HTTP_METHODS = new Set(["get", "post", "put", "patch", "delete", "head", "options", "trace"]);

type JSONObject = Record<string, unknown>;

export type PersistedContractKnowledgeMapEntry = {
  id: string;
  kind: string;
  title: string;
  summary: string;
  aliases?: readonly string[];
  children?: readonly PersistedContractKnowledgeMapEntry[];
};

export type PersistedContractKnowledgeMapGap = {
  kind: string;
  description: string;
  evidence_ids?: readonly string[];
};

/** Exact JSON boundary consumed by model.ContractMapBody in Go. */
export type PersistedContractMapBody = {
  overview: string;
  servers?: readonly string[];
  authentication?: readonly PersistedContractKnowledgeMapEntry[];
  capabilities: readonly PersistedContractKnowledgeMapEntry[];
  operations: readonly PersistedContractKnowledgeMapEntry[];
  schemas?: readonly PersistedContractKnowledgeMapEntry[];
  errors?: readonly PersistedContractKnowledgeMapEntry[];
  pagination?: readonly PersistedContractKnowledgeMapEntry[];
  webhooks?: readonly PersistedContractKnowledgeMapEntry[];
  gaps?: readonly PersistedContractKnowledgeMapGap[];
  quality_warnings?: readonly string[];
};

export type OpenAPICandidateOperation = {
  logicalId: string;
  operationKey: string;
  operationId: string;
  method: string;
  pathTemplate: string;
  tags: readonly string[];
  summary: string;
  description: string;
  security: readonly unknown[];
  requestSchemaRefs: readonly string[];
  responseSchemaRefs: readonly string[];
  contentHash: string;
  ordinal: number;
};

export type OpenAPICandidateSchema = {
  logicalId: string;
  schemaKey: string;
  schemaDocument: JSONObject;
  contentHash: string;
};

export type OpenAPICandidateExample = {
  logicalId: string;
  operationLogicalId: string | null;
  name: string;
  exampleKind: "request" | "response" | "webhook";
  mediaType: string;
  statusCode: string;
  value: unknown;
  contentHash: string;
};

export type OpenAPICandidate = {
  logicalId: string;
  sourceFormat: "json" | "yaml";
  openapiVersion: string;
  normalizedContract: JSONObject;
  sourceHash: string;
  contentHash: string;
  operations: readonly OpenAPICandidateOperation[];
  schemas: readonly OpenAPICandidateSchema[];
  examples: readonly OpenAPICandidateExample[];
  structuredMap: PersistedContractMapBody;
  agentMarkdown: string;
  mapContentHash: string;
  diagnostics: readonly Readonly<Record<string, unknown>>[];
};

function objectValue(value: unknown): JSONObject | null {
  return value && typeof value === "object" && !Array.isArray(value) ? value as JSONObject : null;
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value)
    ? [...new Set(value.filter((item): item is string => typeof item === "string" && Boolean(item.trim())).map((item) => item.trim()))].sort(compareText)
    : [];
}

function schemaRefs(value: unknown): string[] {
  const refs = new Set<string>();
  const visit = (candidate: unknown) => {
    if (Array.isArray(candidate)) {
      candidate.forEach(visit);
      return;
    }
    const object = objectValue(candidate);
    if (!object) return;
    if (typeof object.$ref === "string" && object.$ref.trim()) refs.add(object.$ref.trim());
    Object.values(object).forEach(visit);
  };
  visit(value);
  return [...refs].sort(compareText);
}

function examplesFromContent(
  operationLogicalId: string,
  operationKey: string,
  kind: "request" | "response" | "webhook",
  statusCode: string,
  value: unknown,
): OpenAPICandidateExample[] {
  const content = objectValue(value);
  if (!content) return [];
  const result: OpenAPICandidateExample[] = [];
  for (const [mediaType, mediaValue] of Object.entries(content).sort(([left], [right]) => compareText(left, right))) {
    const media = objectValue(mediaValue);
    if (!media) continue;
    const values: Array<[string, unknown]> = [];
    if (Object.hasOwn(media, "example")) values.push(["default", media.example]);
    const named = objectValue(media.examples);
    if (named) {
      for (const [name, example] of Object.entries(named).sort(([left], [right]) => compareText(left, right))) {
        const wrapped = objectValue(example);
        values.push([name, wrapped && Object.hasOwn(wrapped, "value") ? wrapped.value : example]);
      }
    }
    for (const [name, exampleValue] of values) {
      const logicalId = `example:${operationLogicalId}:${kind}:${statusCode}:${mediaType}:${name}`;
      const uniqueName = `${operationKey} ${kind} ${statusCode || "default"} ${mediaType} ${name}`;
      result.push({
        logicalId,
        operationLogicalId,
        name: uniqueName,
        exampleKind: kind,
        mediaType,
        statusCode,
        value: exampleValue,
        contentHash: sha256(canonicalJSON(exampleValue)),
      });
    }
  }
  return result;
}

function operationExamples(
  operationLogicalId: string,
  operationKey: string,
  operation: JSONObject,
): OpenAPICandidateExample[] {
  const examples: OpenAPICandidateExample[] = [];
  const requestBody = objectValue(operation.requestBody);
  examples.push(...examplesFromContent(operationLogicalId, operationKey, "request", "", requestBody?.content));
  const responses = objectValue(operation.responses) ?? {};
  for (const [statusCode, responseValue] of Object.entries(responses).sort(([left], [right]) => compareText(left, right))) {
    examples.push(...examplesFromContent(operationLogicalId, operationKey, "response", statusCode, objectValue(responseValue)?.content));
  }
  return examples;
}

function mapMarkdown(
  version: string,
  operations: readonly OpenAPICandidateOperation[],
  schemas: readonly OpenAPICandidateSchema[],
): string {
  const lines = [
    "# API Contract Map",
    "",
    `OpenAPI version: ${version}`,
    `Operations: ${operations.length}`,
    `Schemas: ${schemas.length}`,
    "",
    "## Operations",
    "",
  ];
  if (operations.length === 0) lines.push("- None");
  for (const operation of operations) {
    lines.push(`- ${operation.method} ${operation.pathTemplate} — evidence \`operation:${operation.logicalId}\`${operation.summary ? ` — ${operation.summary.replace(/\s+/g, " ")}` : ""}`);
  }
  lines.push("", "## Schemas", "");
  if (schemas.length === 0) lines.push("- None");
  for (const schema of schemas) lines.push(`- ${schema.schemaKey} — evidence \`schema:${schema.logicalId}\``);
  return `${lines.join("\n").trim()}\n`;
}

function uniqueMapStrings(values: readonly string[]): string[] {
  return [...new Set(values.map((value) => value.trim()).filter(Boolean))].sort(compareText);
}

function operationMapEntry(operation: OpenAPICandidateOperation): PersistedContractKnowledgeMapEntry {
  const aliases = uniqueMapStrings([
    operation.operationId,
    operation.operationKey,
    operation.method,
    operation.pathTemplate,
    ...operation.tags,
  ]);
  return {
    id: `operation:${operation.logicalId}`,
    kind: "operation",
    title: operation.operationId || operation.operationKey,
    summary: operation.summary || operation.description || `${operation.method} operation at ${operation.pathTemplate}.`,
    ...(aliases.length > 0 ? { aliases } : {}),
  };
}

function contractErrorEntries(
  root: JSONObject,
  operations: readonly OpenAPICandidateOperation[],
): PersistedContractKnowledgeMapEntry[] {
  const operationByKey = new Map(operations.map((operation) => [operation.operationKey, operation]));
  const paths = objectValue(root.paths) ?? {};
  const result: PersistedContractKnowledgeMapEntry[] = [];
  for (const [pathTemplate, pathValue] of Object.entries(paths).sort(([left], [right]) => compareText(left, right))) {
    const pathItem = objectValue(pathValue);
    if (!pathItem) continue;
    for (const [rawMethod, operationValue] of Object.entries(pathItem).sort(([left], [right]) => compareText(left, right))) {
      const method = rawMethod.toLowerCase();
      if (!HTTP_METHODS.has(method)) continue;
      const operation = operationByKey.get(`${method.toUpperCase()} ${pathTemplate}`);
      const responses = objectValue(objectValue(operationValue)?.responses) ?? {};
      for (const [status, responseValue] of Object.entries(responses).sort(([left], [right]) => compareText(left, right))) {
        if (!/^[45](?:\d\d|XX)$/i.test(status)) continue;
        const response = objectValue(responseValue);
        const operationEvidenceId = operation ? `operation:${operation.logicalId}` : `${method.toUpperCase()} ${pathTemplate}`;
        result.push({
          id: `${operationEvidenceId}:response:${status.toUpperCase()}`,
          kind: "error_response",
          title: `${method.toUpperCase()} ${pathTemplate} — ${status.toUpperCase()}`,
          summary: text(response?.description) || `Documented ${status.toUpperCase()} error response.`,
          aliases: uniqueMapStrings([status.toUpperCase(), operation?.operationId ?? "", operation?.operationKey ?? ""]),
        });
      }
    }
  }
  return result;
}

function contractMapBody(
  root: JSONObject,
  openapiVersion: string,
  operations: readonly OpenAPICandidateOperation[],
  schemas: readonly OpenAPICandidateSchema[],
  examples: readonly OpenAPICandidateExample[],
  diagnostics: readonly Readonly<Record<string, unknown>>[],
): PersistedContractMapBody {
  const operationEntries = operations.map(operationMapEntry);
  const byTag = new Map<string, PersistedContractKnowledgeMapEntry[]>();
  for (let index = 0; index < operations.length; index++) {
    const operation = operations[index]!;
    const tags = operation.tags.length > 0 ? operation.tags : ["General"];
    for (const tag of tags) {
      const entries = byTag.get(tag) ?? [];
      entries.push(operationEntries[index]!);
      byTag.set(tag, entries);
    }
  }
  const capabilities = [...byTag.entries()]
    .sort(([left], [right]) => compareText(left, right))
    .map(([tag, children]) => ({
      id: `capability_${sha256(tag.toLowerCase()).slice(0, 24)}`,
      kind: "capability",
      title: tag,
      summary: `${children.length} operation(s) grouped under the ${tag} capability.`,
      aliases: [tag],
      children,
    } satisfies PersistedContractKnowledgeMapEntry));

  const components = objectValue(root.components);
  const securitySchemes = objectValue(components?.securitySchemes) ?? {};
  const referencedSecuritySchemes = new Set<string>();
  for (const operation of operations) {
    for (const requirement of operation.security) {
      const object = objectValue(requirement);
      if (object) Object.keys(object).forEach((name) => referencedSecuritySchemes.add(name));
    }
  }
  const authentication = uniqueMapStrings([...Object.keys(securitySchemes), ...referencedSecuritySchemes])
    .map((name) => {
      const scheme = objectValue(securitySchemes[name]);
      const schemeType = text(scheme?.type);
      const detail = uniqueMapStrings([schemeType, text(scheme?.scheme), text(scheme?.bearerFormat)]).join(" / ");
      return {
        id: `security_scheme_${sha256(name).slice(0, 24)}`,
        kind: "authentication",
        title: name,
        summary: scheme
          ? `${detail || "OpenAPI"} security scheme defined by the contract.`
          : "Security scheme referenced by an operation but missing from components.securitySchemes.",
        aliases: uniqueMapStrings([name, schemeType, text(scheme?.scheme)]),
      } satisfies PersistedContractKnowledgeMapEntry;
    });

  const schemaEntries = schemas.map((schema) => {
    const schemaType = text(schema.schemaDocument.type);
    return {
      id: `schema:${schema.logicalId}`,
      kind: "schema",
      title: schema.schemaKey,
      summary: text(schema.schemaDocument.description) || `${schemaType || "OpenAPI"} schema named ${schema.schemaKey}.`,
      aliases: uniqueMapStrings([schema.schemaKey, `#/components/schemas/${schema.schemaKey}`]),
    } satisfies PersistedContractKnowledgeMapEntry;
  });
  const pagination = operations
    .filter((operation) => /\b(?:page|pagination|cursor|offset|limit|continuation)\b/i.test([
      operation.operationKey,
      operation.operationId,
      operation.summary,
      operation.description,
      ...operation.requestSchemaRefs,
      ...operation.responseSchemaRefs,
    ].join(" ")))
    .map((operation) => ({ ...operationMapEntry(operation), kind: "pagination" }));
  const webhooksRoot = objectValue(root.webhooks) ?? {};
  const webhooks = Object.entries(webhooksRoot)
    .sort(([left], [right]) => compareText(left, right))
    .map(([name, value]) => ({
      id: `webhook_${sha256(name).slice(0, 24)}`,
      kind: "webhook",
      title: name,
      summary: text(objectValue(value)?.description) || `Webhook ${name} declared by the OpenAPI contract.`,
      aliases: [name],
    } satisfies PersistedContractKnowledgeMapEntry));
  const gaps: PersistedContractKnowledgeMapGap[] = diagnostics.map((diagnostic) => {
    const evidenceIds = uniqueMapStrings([
      typeof diagnostic.operation_key === "string" ? diagnostic.operation_key : "",
      typeof diagnostic.schema_key === "string" ? diagnostic.schema_key : "",
    ]);
    return {
      kind: typeof diagnostic.code === "string" ? diagnostic.code : "contract_quality",
      description: typeof diagnostic.message === "string" ? diagnostic.message : "Contract quality diagnostic requires review.",
      ...(evidenceIds.length > 0 ? { evidence_ids: evidenceIds } : {}),
    };
  });
  const qualityWarnings = uniqueMapStrings(diagnostics.map((diagnostic) =>
    `${typeof diagnostic.code === "string" ? diagnostic.code : "contract_quality"}: ${typeof diagnostic.message === "string" ? diagnostic.message : "Review required."}`));
  const servers = uniqueMapStrings((Array.isArray(root.servers) ? root.servers : [])
    .map((server) => text(objectValue(server)?.url)));
  const overview = `OpenAPI ${openapiVersion} contract with ${operations.length} operation(s), ${schemas.length} schema(s), and ${examples.length} extracted example(s).`;
  return {
    overview,
    ...(servers.length > 0 ? { servers } : {}),
    ...(authentication.length > 0 ? { authentication } : {}),
    capabilities,
    operations: operationEntries,
    ...(schemaEntries.length > 0 ? { schemas: schemaEntries } : {}),
    ...(contractErrorEntries(root, operations).length > 0 ? { errors: contractErrorEntries(root, operations) } : {}),
    ...(pagination.length > 0 ? { pagination } : {}),
    ...(webhooks.length > 0 ? { webhooks } : {}),
    ...(gaps.length > 0 ? { gaps } : {}),
    ...(qualityWarnings.length > 0 ? { quality_warnings: qualityWarnings } : {}),
  };
}

export function buildOpenAPICandidate(content: string, sourceFormat: "json" | "yaml"): OpenAPICandidate {
  const parsed = parseStructuredValue(content, sourceFormat);
  const root = objectValue(parsed);
  if (!root) throw new TypeError("The OpenAPI source must parse to an object.");
  const openapiVersion = text(root.openapi) || text(root.swagger);
  const paths = objectValue(root.paths);
  if (!openapiVersion || !paths) throw new TypeError("The OpenAPI source is missing its version or paths object.");

  const normalizedContract = JSON.parse(canonicalJSON(root)) as JSONObject;
  const contentHash = sha256(canonicalJSON(normalizedContract));
  const sourceHash = sha256(content);
  const logicalId = `contract_candidate_${contentHash.slice(0, 24)}`;
  const diagnostics: Array<Readonly<Record<string, unknown>>> = [];
  const operationIds = new Set<string>();
  const operations: OpenAPICandidateOperation[] = [];
  const examples: OpenAPICandidateExample[] = [];
  for (const [pathTemplate, pathValue] of Object.entries(paths).sort(([left], [right]) => compareText(left, right))) {
    if (!pathTemplate.startsWith("/")) continue;
    const pathItem = objectValue(pathValue);
    if (!pathItem) continue;
    for (const [rawMethod, operationValue] of Object.entries(pathItem).sort(([left], [right]) => compareText(left, right))) {
      const method = rawMethod.toLowerCase();
      if (!HTTP_METHODS.has(method)) continue;
      const operation = objectValue(operationValue);
      if (!operation) continue;
      const upperMethod = method.toUpperCase();
      const operationKey = `${upperMethod} ${pathTemplate}`;
      const operationLogicalId = `operation_${sha256(`${contentHash}\0${operationKey}`).slice(0, 24)}`;
      let operationId = text(operation.operationId);
      if (operationId && operationIds.has(operationId)) {
        diagnostics.push({
          code: "openapi_operation_id_duplicate",
          severity: "error",
          operation_key: operationKey,
          operation_id: operationId,
          message: "A duplicate operationId was omitted from the extracted index; the normalized contract remains unchanged for review.",
        });
        operationId = "";
      } else if (operationId) operationIds.add(operationId);
      const security = Array.isArray(operation.security)
        ? operation.security
        : Array.isArray(root.security) ? root.security : [];
      const record: OpenAPICandidateOperation = {
        logicalId: operationLogicalId,
        operationKey,
        operationId,
        method: upperMethod,
        pathTemplate,
        tags: stringArray(operation.tags),
        summary: text(operation.summary),
        description: text(operation.description),
        security,
        requestSchemaRefs: schemaRefs({ parameters: operation.parameters, requestBody: operation.requestBody }),
        responseSchemaRefs: schemaRefs(operation.responses),
        contentHash: sha256(canonicalJSON(operation)),
        ordinal: operations.length,
      };
      operations.push(record);
      examples.push(...operationExamples(operationLogicalId, operationKey, operation));
    }
  }

  const schemasRoot = objectValue(objectValue(root.components)?.schemas) ?? objectValue(root.definitions) ?? {};
  const schemas: OpenAPICandidateSchema[] = [];
  for (const [schemaKey, schemaValue] of Object.entries(schemasRoot).sort(([left], [right]) => compareText(left, right))) {
    const schemaDocument = objectValue(schemaValue);
    if (!schemaDocument) {
      diagnostics.push({ code: "openapi_schema_skipped", severity: "warning", schema_key: schemaKey, message: "The schema value is not an object." });
      continue;
    }
    schemas.push({
      logicalId: `schema_${sha256(`${contentHash}\0${schemaKey}`).slice(0, 24)}`,
      schemaKey,
      schemaDocument,
      contentHash: sha256(canonicalJSON(schemaDocument)),
    });
  }

  const structuredMap = contractMapBody(root, openapiVersion, operations, schemas, examples, diagnostics);
  const agentMarkdown = mapMarkdown(openapiVersion, operations, schemas);
  const mapContentHash = sha256(canonicalJSON({ structuredMap, agentMarkdown }));
  return {
    logicalId,
    sourceFormat,
    openapiVersion,
    normalizedContract,
    sourceHash,
    contentHash,
    operations,
    schemas,
    examples,
    structuredMap,
    agentMarkdown,
    mapContentHash,
    diagnostics,
  };
}
