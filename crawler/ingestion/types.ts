export const NORMALIZED_DOCUMENT_SCHEMA_VERSION = "2026-08-26" as const;
export const DOCUMENTATION_MAP_SCHEMA_VERSION = "2026-08-26" as const;

export type DocumentFormat =
  | "html"
  | "markdown"
  | "mdx"
  | "text"
  | "json"
  | "yaml"
  | "openapi-json"
  | "openapi-yaml";

export type DocumentKind = "documentation" | "openapi";

export type SourceRange = {
  startOffset: number;
  endOffset: number;
  startLine: number;
  endLine: number;
};

export type SourceLineageInput = {
  sourceId: string;
  canonicalUrl: string;
  contentType: string;
  sourceKind?: string;
  snapshotId?: string;
  fetchedAt?: string;
};

export type SourceLineage = SourceLineageInput & {
  rawSha256: string;
};

export type NormalizationInput = {
  content: string;
  source: SourceLineageInput;
  title?: string;
  documentKind?: DocumentKind;
  format?: DocumentFormat;
};

export type NormalizedLink = {
  label: string;
  target: string;
};

type BaseBlock = {
  id: string;
  ordinal: number;
  sourceRange: SourceRange;
  contentSha256: string;
};

export type HeadingBlock = BaseBlock & {
  type: "heading";
  level: number;
  text: string;
  anchor: string;
  links: readonly NormalizedLink[];
};

export type ParagraphBlock = BaseBlock & {
  type: "paragraph";
  text: string;
  links: readonly NormalizedLink[];
};

export type QuoteBlock = BaseBlock & {
  type: "quote";
  text: string;
  links: readonly NormalizedLink[];
};

export type ListItem = {
  text: string;
  depth: number;
  links: readonly NormalizedLink[];
};

export type ListBlock = BaseBlock & {
  type: "list";
  ordered: boolean;
  items: readonly ListItem[];
};

export type TableBlock = BaseBlock & {
  type: "table";
  headers: readonly string[];
  rows: readonly (readonly string[])[];
  links: readonly NormalizedLink[];
};

export type CodeBlock = BaseBlock & {
  type: "code";
  language: string | null;
  code: string;
};

export type NormalizedBlock =
  | HeadingBlock
  | ParagraphBlock
  | QuoteBlock
  | ListBlock
  | TableBlock
  | CodeBlock;

export type DiagnosticSeverity = "info" | "warning" | "error";
export type DiagnosticScope = "document" | "section" | "corpus";

export type QualityDiagnostic = {
  code:
    | "document_empty"
    | "document_missing_title"
    | "document_duplicate"
    | "document_boilerplate"
    | "boilerplate_removed"
    | "section_oversized"
    | "structured_parse_failed"
    | "mdx_executable_syntax_preserved"
    | "unsupported_markup_preserved";
  severity: DiagnosticSeverity;
  scope: DiagnosticScope;
  message: string;
  documentId?: string;
  sectionId?: string;
  evidenceIds?: readonly string[];
  details?: Readonly<Record<string, string | number | boolean>>;
};

export type NormalizedDocument = {
  schemaVersion: typeof NORMALIZED_DOCUMENT_SCHEMA_VERSION;
  id: string;
  title: string;
  format: DocumentFormat;
  kind: DocumentKind;
  source: SourceLineage;
  normalizedSha256: string;
  blocks: readonly NormalizedBlock[];
  diagnostics: readonly QualityDiagnostic[];
};

export type HeadingPathItem = {
  level: number;
  text: string;
  anchor: string;
};

export type BlockSlice = {
  blockId: string;
  start: number;
  end: number;
};

export type NormalizedSection = {
  id: string;
  ordinal: number;
  documentId: string;
  title: string;
  anchor: string;
  headingPath: readonly HeadingPathItem[];
  blockSlices: readonly BlockSlice[];
  content: string;
  contentSha256: string;
  characterCount: number;
  estimatedTokens: number;
  source: SourceLineage & {
    documentNormalizedSha256: string;
  };
};

export type SegmentationOptions = {
  targetCharacters?: number;
  oversizedCharacters?: number;
};

export type SegmentationResult = {
  sections: readonly NormalizedSection[];
  diagnostics: readonly QualityDiagnostic[];
};

export type DocumentationOutlineNode = {
  title: string;
  anchor: string;
  level: number;
  sectionIds: readonly string[];
  children: readonly DocumentationOutlineNode[];
};

export type DocumentationMapDocument = {
  documentId: string;
  title: string;
  canonicalUrl: string;
  format: DocumentFormat;
  rawSha256: string;
  normalizedSha256: string;
  sectionCount: number;
  languages: readonly string[];
  topics: readonly string[];
  outline: readonly DocumentationOutlineNode[];
  diagnosticCodes: readonly QualityDiagnostic["code"][];
};

export type DocumentationMapSection = {
  sectionId: string;
  documentId: string;
  title: string;
  anchor: string;
  headingPath: readonly HeadingPathItem[];
  contentSha256: string;
  characterCount: number;
  estimatedTokens: number;
};

export type DocumentationMapEntryPointKind =
  | "authentication"
  | "setup"
  | "errors"
  | "examples"
  | "reference"
  | "concepts";

export type DocumentationMapEntryPoint = {
  kind: DocumentationMapEntryPointKind;
  sectionId: string;
  documentId: string;
  title: string;
};

export type DocumentationMap = {
  schemaVersion: typeof DOCUMENTATION_MAP_SCHEMA_VERSION;
  id: string;
  mapSha256: string;
  overview: {
    documentCount: number;
    sectionCount: number;
    formats: readonly DocumentFormat[];
    languages: readonly string[];
    topLevelTopics: readonly string[];
    diagnostics: Readonly<Record<string, number>>;
  };
  documents: readonly DocumentationMapDocument[];
  sections: readonly DocumentationMapSection[];
  entryPoints: readonly DocumentationMapEntryPoint[];
};

export type DocumentationEvidenceItem = {
  evidenceId: string;
  kind: "document" | "section";
  documentId: string;
  sectionId?: string;
  title: string;
  canonicalUrl: string;
  contentSha256: string;
  content: string;
};

/**
 * Deterministic input contract for an optional, separately controlled map
 * enrichment step. This module never calls a model and treats enrichment as
 * advisory: every assertion must cite one or more evidence IDs.
 */
export type DocumentationMapEnrichmentInput = {
  mapId: string;
  mapSha256: string;
  map: DocumentationMap;
  evidence: readonly DocumentationEvidenceItem[];
};

