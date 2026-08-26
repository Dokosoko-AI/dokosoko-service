import type { NormalizedLink, QualityDiagnostic, SourceRange } from "./types";

type DraftBase = {
  sourceRange: SourceRange;
};

export type DraftHeading = DraftBase & {
  type: "heading";
  level: number;
  text: string;
  anchor?: string;
  links: readonly NormalizedLink[];
};

export type DraftParagraph = DraftBase & {
  type: "paragraph";
  text: string;
  links: readonly NormalizedLink[];
};

export type DraftQuote = DraftBase & {
  type: "quote";
  text: string;
  links: readonly NormalizedLink[];
};

export type DraftList = DraftBase & {
  type: "list";
  ordered: boolean;
  items: readonly {
    text: string;
    depth: number;
    links: readonly NormalizedLink[];
  }[];
};

export type DraftTable = DraftBase & {
  type: "table";
  headers: readonly string[];
  rows: readonly (readonly string[])[];
  links: readonly NormalizedLink[];
};

export type DraftCode = DraftBase & {
  type: "code";
  language: string | null;
  code: string;
};

export type DraftBlock = DraftHeading | DraftParagraph | DraftQuote | DraftList | DraftTable | DraftCode;

export type ParseResult = {
  title: string;
  blocks: readonly DraftBlock[];
  diagnostics: readonly Omit<QualityDiagnostic, "documentId">[];
};

