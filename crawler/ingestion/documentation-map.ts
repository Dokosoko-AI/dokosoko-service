import { canonicalJSON, compareText, sha256 } from "./canonical";
import { renderBlock } from "./render";
import {
  DOCUMENTATION_MAP_SCHEMA_VERSION,
  type DocumentationEvidenceItem,
  type DocumentationMap,
  type DocumentationMapDocument,
  type DocumentationMapEnrichmentInput,
  type DocumentationMapEntryPoint,
  type DocumentationMapEntryPointKind,
  type DocumentationOutlineNode,
  type NormalizedDocument,
  type NormalizedSection,
  type QualityDiagnostic,
} from "./types";

type MutableOutlineNode = {
  title: string;
  anchor: string;
  level: number;
  sectionIds: string[];
  children: MutableOutlineNode[];
};

function unique<T extends string>(values: readonly T[]): T[] {
  return [...new Set(values.filter(Boolean))];
}

function outline(document: NormalizedDocument, sections: readonly NormalizedSection[]): DocumentationOutlineNode[] {
  const roots: MutableOutlineNode[] = [];
  const stack: MutableOutlineNode[] = [];
  for (const block of document.blocks) {
    if (block.type !== "heading") continue;
    while (stack.length > 0 && stack.at(-1)!.level >= block.level) stack.pop();
    const node: MutableOutlineNode = {
      title: block.text,
      anchor: block.anchor,
      level: block.level,
      sectionIds: sections.filter((section) => section.documentId === document.id && section.anchor === block.anchor).map((section) => section.id),
      children: [],
    };
    if (stack.length > 0) stack.at(-1)!.children.push(node);
    else roots.push(node);
    stack.push(node);
  }
  return roots;
}

const entryPointPatterns: ReadonlyArray<[DocumentationMapEntryPointKind, RegExp]> = [
  ["authentication", /\b(?:auth(?:entication|orization)?|oauth|token|credential|sign[ -]?in)\b/i],
  ["setup", /\b(?:setup|install(?:ation)?|getting started|quick ?start|configuration|initialize)\b/i],
  ["errors", /\b(?:error|troubleshoot|failure|exception|retry|status code)\b/i],
  ["examples", /\b(?:example|sample|tutorial|walkthrough|recipe)\b/i],
  ["reference", /\b(?:reference|operation|endpoint|schema|api|method|class|function)\b/i],
  ["concepts", /\b(?:overview|concept|architecture|introduction|guide)\b/i],
];

function entryPoint(section: NormalizedSection): DocumentationMapEntryPoint | null {
  const value = `${section.headingPath.map((item) => item.text).join(" ")} ${section.title}`;
  const kind = entryPointPatterns.find(([, pattern]) => pattern.test(value))?.[0];
  return kind ? { kind, sectionId: section.id, documentId: section.documentId, title: section.title } : null;
}

function documentMap(
  document: NormalizedDocument,
  sections: readonly NormalizedSection[],
  diagnostics: readonly QualityDiagnostic[],
): DocumentationMapDocument {
  const documentSections = sections.filter((section) => section.documentId === document.id);
  const languages = unique(document.blocks
    .filter((block) => block.type === "code" && block.language)
    .map((block) => block.type === "code" ? block.language ?? "" : ""));
  const topics = unique(document.blocks.filter((block) => block.type === "heading").map((block) => block.type === "heading" ? block.text : ""));
  return {
    documentId: document.id,
    title: document.title,
    canonicalUrl: document.source.canonicalUrl,
    format: document.format,
    rawSha256: document.source.rawSha256,
    normalizedSha256: document.normalizedSha256,
    sectionCount: documentSections.length,
    languages,
    topics,
    outline: outline(document, documentSections),
    diagnosticCodes: unique(diagnostics.filter((diagnostic) => diagnostic.documentId === document.id).map((diagnostic) => diagnostic.code)),
  };
}

export function buildDocumentationMap(
  documents: readonly NormalizedDocument[],
  sections: readonly NormalizedSection[],
  diagnostics: readonly QualityDiagnostic[] = [],
): DocumentationMap {
  const sortedDocuments = [...documents].sort((left, right) => compareText(left.source.canonicalUrl, right.source.canonicalUrl) || compareText(left.id, right.id));
  const sortedSections = [...sections].sort((left, right) => {
    const leftDocument = sortedDocuments.findIndex((document) => document.id === left.documentId);
    const rightDocument = sortedDocuments.findIndex((document) => document.id === right.documentId);
    return leftDocument - rightDocument || left.ordinal - right.ordinal || compareText(left.id, right.id);
  });
  const mapDocuments = sortedDocuments.map((document) => documentMap(document, sortedSections, diagnostics));
  const languages = unique(mapDocuments.flatMap((document) => document.languages));
  const formats = unique(mapDocuments.map((document) => document.format));
  const topLevelTopics = unique(mapDocuments.flatMap((document) => document.outline.map((node) => node.title)));
  const diagnosticCounts: Record<string, number> = {};
  for (const diagnostic of diagnostics) diagnosticCounts[diagnostic.code] = (diagnosticCounts[diagnostic.code] ?? 0) + 1;
  const orderedDiagnosticCounts = Object.fromEntries(Object.entries(diagnosticCounts).sort(([left], [right]) => compareText(left, right)));
  const entryPoints = sortedSections.map(entryPoint).filter((value): value is DocumentationMapEntryPoint => value !== null);
  const payload = {
    schemaVersion: DOCUMENTATION_MAP_SCHEMA_VERSION,
    overview: {
      documentCount: mapDocuments.length,
      sectionCount: sortedSections.length,
      formats,
      languages,
      topLevelTopics,
      diagnostics: orderedDiagnosticCounts,
    },
    documents: mapDocuments,
    sections: sortedSections.map((section) => ({
      sectionId: section.id,
      documentId: section.documentId,
      title: section.title,
      anchor: section.anchor,
      headingPath: section.headingPath,
      contentSha256: section.contentSha256,
      characterCount: section.characterCount,
      estimatedTokens: section.estimatedTokens,
    })),
    entryPoints,
  };
  const mapSha256 = sha256(canonicalJSON(payload));
  return { ...payload, id: `documentation_map_${mapSha256.slice(0, 24)}`, mapSha256 };
}

export function documentationMapEnrichmentInput(
  map: DocumentationMap,
  documents: readonly NormalizedDocument[],
  sections: readonly NormalizedSection[],
): DocumentationMapEnrichmentInput {
  const documentsById = new Map(documents.map((document) => [document.id, document]));
  const evidence: DocumentationEvidenceItem[] = [];
  for (const document of [...documents].sort((left, right) => compareText(left.source.canonicalUrl, right.source.canonicalUrl) || compareText(left.id, right.id))) {
    const content = document.blocks.map(renderBlock).join("\n\n");
    evidence.push({
      evidenceId: `document:${document.id}`,
      kind: "document",
      documentId: document.id,
      title: document.title,
      canonicalUrl: document.source.canonicalUrl,
      contentSha256: sha256(content),
      content,
    });
  }
  for (const section of [...sections].sort((left, right) => compareText(left.documentId, right.documentId) || left.ordinal - right.ordinal)) {
    const document = documentsById.get(section.documentId);
    if (!document) throw new TypeError(`Section ${section.id} references unknown document ${section.documentId}.`);
    evidence.push({
      evidenceId: `section:${section.id}`,
      kind: "section",
      documentId: section.documentId,
      sectionId: section.id,
      title: section.title,
      canonicalUrl: document.source.canonicalUrl,
      contentSha256: section.contentSha256,
      content: section.content,
    });
  }
  return { mapId: map.id, mapSha256: map.mapSha256, map, evidence };
}
