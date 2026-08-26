import { canonicalJSON, sha256 } from "./canonical";
import { renderBlock } from "./render";
import type {
  BlockSlice,
  HeadingPathItem,
  NormalizedBlock,
  NormalizedDocument,
  NormalizedSection,
  QualityDiagnostic,
  SegmentationOptions,
  SegmentationResult,
} from "./types";

const DEFAULT_TARGET_CHARACTERS = 4_000;
const DEFAULT_OVERSIZED_CHARACTERS = 8_000;

type Piece = {
  blockId: string;
  start: number;
  end: number;
  text: string;
  atomic: boolean;
};

type LogicalSection = {
  title: string;
  anchor: string;
  headingPath: readonly HeadingPathItem[];
  pieces: Piece[];
};

function boundary(value: string, maximum: number): number {
  if (value.length <= maximum) return value.length;
  const floor = Math.max(1, Math.floor(maximum * 0.5));
  const prefix = value.slice(0, maximum + 1);
  const candidates = [
    prefix.lastIndexOf("\n\n"),
    prefix.lastIndexOf("\n"),
    Math.max(prefix.lastIndexOf(". "), prefix.lastIndexOf("! "), prefix.lastIndexOf("? ")) + 1,
    prefix.lastIndexOf(" "),
  ].filter((candidate) => candidate >= floor && candidate <= maximum);
  return candidates.length > 0 ? Math.max(...candidates) : maximum;
}

function mergeSlices(pieces: readonly Piece[]): BlockSlice[] {
  const slices: BlockSlice[] = [];
  for (const piece of pieces) {
    const previous = slices.at(-1);
    if (previous?.blockId === piece.blockId && previous.end === piece.start) previous.end = piece.end;
    else slices.push({ blockId: piece.blockId, start: piece.start, end: piece.end });
  }
  return slices;
}

function blockPiece(block: NormalizedBlock): Piece {
  const text = renderBlock(block);
  return {
    blockId: block.id,
    start: 0,
    end: text.length,
    text,
    atomic: block.type === "code" || block.type === "table" || block.type === "heading",
  };
}

function logicalSections(document: NormalizedDocument): LogicalSection[] {
  const result: LogicalSection[] = [];
  const path: HeadingPathItem[] = [];
  let current: LogicalSection = { title: document.title, anchor: "document", headingPath: [], pieces: [] };
  const flush = () => {
    if (current.pieces.length > 0) result.push(current);
  };

  for (const block of document.blocks) {
    if (block.type === "heading") {
      flush();
      while (path.length > 0 && path.at(-1)!.level >= block.level) path.pop();
      path.push({ level: block.level, text: block.text, anchor: block.anchor });
      current = { title: block.text, anchor: block.anchor, headingPath: [...path], pieces: [blockPiece(block)] };
    } else {
      current.pieces.push(blockPiece(block));
    }
  }
  flush();
  return result;
}

function packLogicalSection(logical: LogicalSection, targetCharacters: number): Piece[][] {
  const result: Piece[][] = [];
  let current: Piece[] = [];
  let currentCharacters = 0;
  const flush = () => {
    if (current.length > 0) result.push(current);
    current = [];
    currentCharacters = 0;
  };

  for (const original of logical.pieces) {
    if (original.atomic) {
      const separator = current.length > 0 ? 2 : 0;
      if (current.length > 0 && currentCharacters + separator + original.text.length > targetCharacters) flush();
      current.push(original);
      currentCharacters += (current.length > 1 ? 2 : 0) + original.text.length;
      if (original.text.length > targetCharacters) flush();
      continue;
    }

    let start = original.start;
    while (start < original.end) {
      const separator = current.length > 0 ? 2 : 0;
      let available = targetCharacters - currentCharacters - separator;
      if (available < Math.min(64, targetCharacters) && current.length > 0) {
        flush();
        available = targetCharacters;
      }
      const remainder = original.text.slice(start - original.start);
      const take = boundary(remainder, Math.max(1, available));
      const rawText = remainder.slice(0, take);
      const leading = rawText.match(/^\s*/)?.[0].length ?? 0;
      const trailing = rawText.match(/\s*$/)?.[0].length ?? 0;
      const text = rawText.slice(leading, rawText.length - trailing || undefined);
      const pieceStart = start + leading;
      const pieceEnd = start + rawText.length - trailing;
      if (text) {
        current.push({ blockId: original.blockId, start: pieceStart, end: pieceEnd, text, atomic: false });
        currentCharacters += (current.length > 1 ? 2 : 0) + text.length;
      }
      start += Math.max(1, rawText.length);
      while (start < original.end && /\s/.test(original.text[start - original.start])) start++;
      if (currentCharacters >= targetCharacters) flush();
    }
  }
  flush();
  return result;
}

export function segmentDocument(document: NormalizedDocument, options: SegmentationOptions = {}): SegmentationResult {
  const targetCharacters = options.targetCharacters ?? DEFAULT_TARGET_CHARACTERS;
  const oversizedCharacters = options.oversizedCharacters ?? DEFAULT_OVERSIZED_CHARACTERS;
  if (!Number.isSafeInteger(targetCharacters) || targetCharacters < 1) throw new TypeError("targetCharacters must be a positive integer.");
  if (!Number.isSafeInteger(oversizedCharacters) || oversizedCharacters < targetCharacters) {
    throw new TypeError("oversizedCharacters must be an integer greater than or equal to targetCharacters.");
  }

  const sections: NormalizedSection[] = [];
  for (const logical of logicalSections(document)) {
    for (const pieces of packLogicalSection(logical, targetCharacters)) {
      const content = pieces.map((piece) => piece.text).join("\n\n");
      const contentSha256 = sha256(content);
      const ordinal = sections.length;
      sections.push({
        id: `section_${sha256(canonicalJSON({ documentId: document.id, anchor: logical.anchor, ordinal, contentSha256 })).slice(0, 24)}`,
        ordinal,
        documentId: document.id,
        title: logical.title,
        anchor: logical.anchor,
        headingPath: logical.headingPath,
        blockSlices: mergeSlices(pieces),
        content,
        contentSha256,
        characterCount: content.length,
        estimatedTokens: Math.ceil(content.length / 4),
        source: { ...document.source, documentNormalizedSha256: document.normalizedSha256 },
      });
    }
  }

  const diagnostics: QualityDiagnostic[] = sections
    .filter((section) => section.characterCount > oversizedCharacters)
    .map((section) => ({
      code: "section_oversized",
      severity: "warning",
      scope: "section",
      documentId: document.id,
      sectionId: section.id,
      evidenceIds: [section.id],
      message: "An indivisible code block or table keeps this section above the configured review size.",
      details: { characterCount: section.characterCount, oversizedCharacters },
    }));
  return { sections, diagnostics };
}

