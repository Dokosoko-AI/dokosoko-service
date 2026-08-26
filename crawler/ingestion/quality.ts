import { canonicalJSON, compareText, sha256 } from "./canonical";
import { blockPlainText } from "./render";
import type { NormalizedDocument, NormalizedSection, QualityDiagnostic } from "./types";

function documentFingerprint(document: NormalizedDocument): string {
  return sha256(canonicalJSON(document.blocks.map((block) => ({ type: block.type, contentSha256: block.contentSha256 }))));
}

function diagnosticOrder(left: QualityDiagnostic, right: QualityDiagnostic): number {
  return compareText(left.documentId ?? "", right.documentId ?? "")
    || compareText(left.sectionId ?? "", right.sectionId ?? "")
    || compareText(left.code, right.code);
}

export function assessCorpusQuality(
  documents: readonly NormalizedDocument[],
  sections: readonly NormalizedSection[],
  additionalDiagnostics: readonly QualityDiagnostic[] = [],
): QualityDiagnostic[] {
  const sortedDocuments = [...documents].sort((left, right) => compareText(left.source.canonicalUrl, right.source.canonicalUrl) || compareText(left.id, right.id));
  const diagnostics: QualityDiagnostic[] = [...documents.flatMap((document) => document.diagnostics), ...additionalDiagnostics];
  const duplicateDocuments = new Set<string>();
  const byFingerprint = new Map<string, NormalizedDocument[]>();
  for (const document of sortedDocuments) {
    const fingerprint = documentFingerprint(document);
    const group = byFingerprint.get(fingerprint) ?? [];
    group.push(document);
    byFingerprint.set(fingerprint, group);
  }
  for (const group of byFingerprint.values()) {
    if (group.length < 2) continue;
    const canonical = group[0];
    for (const duplicate of group.slice(1)) {
      duplicateDocuments.add(duplicate.id);
      diagnostics.push({
        code: "document_duplicate",
        severity: "warning",
        scope: "document",
        documentId: duplicate.id,
        evidenceIds: [canonical.id, duplicate.id],
        message: "This document has the same normalized block content as another source document.",
        details: { canonicalDocumentId: canonical.id },
      });
    }
  }

  if (documents.length >= 2) {
    const blockDocuments = new Map<string, Set<string>>();
    const blockCharacters = new Map<string, number>();
    for (const document of documents) {
      for (const block of document.blocks) {
        if (block.type === "heading" || block.type === "code") continue;
        const characters = blockPlainText(block).length;
        if (characters < 40) continue;
        const owners = blockDocuments.get(block.contentSha256) ?? new Set<string>();
        owners.add(document.id);
        blockDocuments.set(block.contentSha256, owners);
        blockCharacters.set(block.contentSha256, characters);
      }
    }
    const threshold = Math.max(2, Math.ceil(documents.length * 0.6));
    const common = new Set([...blockDocuments.entries()].filter(([, owners]) => owners.size >= threshold).map(([hash]) => hash));
    for (const document of documents) {
      if (duplicateDocuments.has(document.id)) continue;
      const proseBlocks = document.blocks.filter((block) => block.type !== "heading" && block.type !== "code");
      const total = proseBlocks.reduce((sum, block) => sum + blockPlainText(block).length, 0);
      const repeated = proseBlocks.reduce((sum, block) => sum + (common.has(block.contentSha256) ? (blockCharacters.get(block.contentSha256) ?? 0) : 0), 0);
      if (total > 0 && repeated / total >= 0.5) diagnostics.push({
        code: "document_boilerplate",
        severity: "warning",
        scope: "document",
        documentId: document.id,
        evidenceIds: [document.id],
        message: "Repeated corpus-wide prose dominates this document and should be reviewed as possible boilerplate.",
        details: { repeatedCharacters: repeated, totalCharacters: total },
      });
    }
  }

  const knownSections = new Set(sections.map((section) => section.id));
  return diagnostics
    .filter((diagnostic) => !diagnostic.sectionId || knownSections.has(diagnostic.sectionId))
    .sort(diagnosticOrder);
}
