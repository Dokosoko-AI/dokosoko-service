"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { APIContractCandidateRecord, DeveloperAssetRecord } from "../../../lib/developer-assets-api";
import { Badge } from "../../core/control";
import { EvidenceContent, evidenceChange } from "../../core/evidence-content";
import { PrettyJSON } from "./developer-asset-ui";

function object(value: unknown): DeveloperAssetRecord { return value && typeof value === "object" && !Array.isArray(value) ? value as DeveloperAssetRecord : {}; }
function text(value: unknown) { return typeof value === "string" ? value : ""; }
function operationEvidence(value: DeveloperAssetRecord, contract: DeveloperAssetRecord) {
  const path = object(object(contract.paths)[text(value.path_template)]);
  const definition = object(path[text(value.method).toLowerCase()]);
  return JSON.stringify({ definition, pathParameters: path.parameters, pathReference: path.$ref, servers: definition.servers ?? path.servers ?? contract.servers, security: definition.security ?? contract.security });
}
function operationKey(value: DeveloperAssetRecord) { return `${text(value.method).toUpperCase()} ${text(value.path_template)}`; }

export function ContractReviewContent({ record, previous, previousRevision }: { record: APIContractCandidateRecord; previous?: APIContractCandidateRecord; previousRevision?: number }) {
  const { t } = useTranslation();
  const [changesOnly, setChangesOnly] = useState(false);
  const normalized = object(record.candidate.normalized_contract);
  const priorContract = object(previous?.candidate.normalized_contract);
  const structural = (value: DeveloperAssetRecord) => Object.fromEntries(Object.entries(value).filter(([key]) => key !== "paths"));
  const structureChange = previous ? evidenceChange(JSON.stringify(structural(priorContract), null, 2), JSON.stringify(structural(normalized), null, 2)) : null;
  const info = object(normalized.info);
  const paths = object(normalized.paths);
  const priorOperations = new Map(previous?.operations.map((operation) => [operationKey(operation), operation]) ?? []);
  const currentKeys = new Set(record.operations.map(operationKey));
  const removed = previous?.operations.filter((operation) => !currentKeys.has(operationKey(operation))) ?? [];
  const changed = (operation: DeveloperAssetRecord) => {
    const prior = priorOperations.get(operationKey(operation));
    return !prior ? "added" : prior.content_hash !== operation.content_hash || operationEvidence(prior, priorContract) !== operationEvidence(operation, normalized) ? "changed" : "unchanged";
  };
  return <div className="contract-review-content">
    <h3>{text(info.title) || t("contractSetup.contractContent")} {text(info.version)}</h3>
    {text(info.description) && <EvidenceContent text={text(info.description)} label={t("apiContracts.description")} />}
    {Array.isArray(normalized.servers) && normalized.servers.length > 0 && <p>{t("contractSetup.servers")} {normalized.servers.map((server) => <code key={text(object(server).url)}>{text(object(server).url)} </code>)}</p>}
    {previous && <><p>{t("contractSetup.comparedWith", { revision: previousRevision ?? "—" })}</p><label className="compact-check"><input type="checkbox" checked={changesOnly} onChange={(event) => setChangesOnly(event.target.checked)} /><span>{t("contractSetup.onlyChanges")}</span></label></>}
    {structureChange && !structureChange.unchanged && <details><summary>{t("contractSetup.structureChanges")}</summary><div className="source-review-diff"><section><h4>{t("sourceContent.before")}</h4><pre>{structureChange.removed}</pre></section><section><h4>{t("sourceContent.after")}</h4><pre>{structureChange.added}</pre></section></div></details>}
    <p>{t("contractSetup.contentCounts", { operations: record.operations.length, schemas: record.schemas.length, examples: record.examples.length })}</p>
    {record.operations.filter((operation) => !changesOnly || changed(operation) !== "unchanged").map((operation) => {
      const key = operationKey(operation);
      const pathItem = object(paths[text(operation.path_template)]);
      const body = object(pathItem[text(operation.method).toLowerCase()]);
      const beforePath = object(object(object(previous?.candidate.normalized_contract).paths)[text(operation.path_template)]);
      return <details className="contract-operation" key={key}>
        <summary><strong>{key}</strong> {text(operation.summary)} {previous && <Badge color={changed(operation) === "unchanged" ? "zinc" : "amber"}>{t(`contractSetup.${changed(operation)}`)}</Badge>}</summary>
        {text(operation.description) && <EvidenceContent text={text(operation.description)} label={key} />}
        <PrettyJSON value={{ pathParameters: pathItem.parameters ?? [], operationParameters: body.parameters ?? [], requestBody: body.requestBody ?? null, responses: body.responses ?? {}, security: body.security ?? normalized.security ?? [] }} label={t("contractSetup.requestResponse")} />
        {previous && changed(operation) === "changed" && <details><summary>{t("contractSetup.previousDefinition")}</summary><PrettyJSON value={beforePath[text(operation.method).toLowerCase()]} /></details>}
      </details>;
    })}
    {removed.map((operation) => <p key={operationKey(operation)}><Badge color="red">{t("contractSetup.removed")}</Badge> <strong>{operationKey(operation)}</strong> {text(operation.summary)}</p>)}
    <details><summary>{t("contractSetup.schemasExamples")}</summary>{record.schemas.map((schema, index) => <details key={text(schema.id) || index}><summary>{text(schema.schema_key)}</summary><PrettyJSON value={schema.schema} /></details>)}{record.examples.map((example, index) => <details key={text(example.id) || index}><summary>{text(example.name)} · {text(example.media_type)}</summary><PrettyJSON value={example.value} /></details>)}</details>
    <details><summary>{t("contractSetup.fullContract")}</summary><PrettyJSON value={normalized} label={t("apiContracts.normalizedOpenAPIContract")} />{previous && <details><summary>{t("contractSetup.previousDefinition")}</summary><PrettyJSON value={previous.candidate.normalized_contract} /></details>}</details>
    <details><summary>{t("contractSetup.validationProvenance")}</summary><PrettyJSON value={{ validation: record.candidate.validation_result, diagnostics: record.candidate.diagnostics, content_hash: record.candidate.content_hash, ingestion_run_id: record.candidate.ingestion_run_id }} /></details>
  </div>;
}
