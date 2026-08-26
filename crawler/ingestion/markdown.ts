import { normalizeCode, normalizeProse, resolveLink, sourceRange } from "./canonical";
import type { DraftBlock, ParseResult } from "./parser-types";
import type { NormalizedLink } from "./types";

type SourceLine = {
  text: string;
  start: number;
  end: number;
};

function sourceLines(content: string): SourceLine[] {
  const result: SourceLine[] = [];
  let start = 0;
  for (const match of content.matchAll(/.*(?:\n|$)/g)) {
    const raw = match[0];
    if (!raw) continue;
    const text = raw.endsWith("\n") ? raw.slice(0, -1) : raw;
    result.push({ text: text.endsWith("\r") ? text.slice(0, -1) : text, start, end: start + raw.length });
    start += raw.length;
  }
  return result;
}

function markdownLinks(value: string, canonicalUrl: string): NormalizedLink[] {
  const links: NormalizedLink[] = [];
  const seen = new Set<string>();
  const add = (label: string, target: string) => {
    const resolved = resolveLink(target, canonicalUrl);
    const normalizedLabel = normalizeProse(label);
    if (!resolved || !normalizedLabel) return;
    const key = `${normalizedLabel}\0${resolved}`;
    if (!seen.has(key)) {
      seen.add(key);
      links.push({ label: normalizedLabel, target: resolved });
    }
  };
  for (const match of value.matchAll(/(?<!!)\[([^\]]+)]\(\s*([^\s)]+)(?:\s+["'][^"']*["'])?\s*\)/g)) {
    add(match[1], match[2]);
  }
  for (const match of value.matchAll(/<((?:https?:\/\/|mailto:)[^>]+)>/gi)) add(match[1], match[1]);
  return links;
}

function inlineText(value: string): string {
  return normalizeProse(value
    .replace(/!\[([^\]]*)]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]+)]\([^)]*\)/g, "$1")
    .replace(/<((?:https?:\/\/|mailto:)[^>]+)>/gi, "$1")
    .replace(/`([^`]+)`/g, "$1")
    .replace(/\{#[A-Za-z][\w:.-]*}\s*$/, "")
    .replace(/<\/?[A-Za-z][^>]*>/g, " ")
    .replace(/(^|\W)(?:\*\*|__)(.+?)(?:\*\*|__)(?=\W|$)/g, "$1$2")
    .replace(/(^|\W)[*_](.+?)[*_](?=\W|$)/g, "$1$2")
    .replace(/\\([\\`*{}[\]()#+.!_>-])/g, "$1"));
}

function splitTableRow(value: string): string[] {
  const trimmed = value.trim().replace(/^\||\|$/g, "");
  const cells: string[] = [];
  let current = "";
  let escaped = false;
  for (const character of trimmed) {
    if (escaped) {
      current += character;
      escaped = false;
    } else if (character === "\\") {
      escaped = true;
    } else if (character === "|") {
      cells.push(inlineText(current));
      current = "";
    } else {
      current += character;
    }
  }
  cells.push(inlineText(current));
  return cells;
}

function isTableDelimiter(value: string): boolean {
  const cells = splitTableRow(value);
  return cells.length > 0 && cells.every((cell) => /^:?-{3,}:?$/.test(cell.replace(/\s/g, "")));
}

function isFence(value: string): RegExpMatchArray | null {
  return value.match(/^\s{0,3}(`{3,}|~{3,})(.*)$/);
}

function isHeading(value: string): RegExpMatchArray | null {
  return value.match(/^\s{0,3}(#{1,6})\s+(.+?)\s*#*\s*$/);
}

function isListItem(value: string): RegExpMatchArray | null {
  return value.match(/^(\s*)([-+*]|\d+[.)])\s+(.+)$/);
}

function isBlockStart(lines: readonly SourceLine[], index: number): boolean {
  const value = lines[index]?.text ?? "";
  if (!value.trim()) return true;
  if (isFence(value) || isHeading(value) || isListItem(value) || /^\s*>/.test(value)) return true;
  if (/^\s*(?:import|export)\s/.test(value)) return true;
  return value.includes("|") && isTableDelimiter(lines[index + 1]?.text ?? "");
}

function frontmatter(lines: readonly SourceLine[]): { end: number; title: string } {
  if (lines[0]?.text.trim() !== "---") return { end: 0, title: "" };
  for (let index = 1; index < lines.length; index++) {
    if (lines[index].text.trim() !== "---") continue;
    const titleLine = lines.slice(1, index).map((line) => line.text).find((line) => /^title\s*:/i.test(line));
    const title = titleLine?.replace(/^title\s*:\s*/i, "").replace(/^['"]|['"]$/g, "").trim() ?? "";
    return { end: index + 1, title };
  }
  return { end: 0, title: "" };
}

export function parseMarkdown(content: string, canonicalUrl: string, mdx = false): ParseResult {
  const lines = sourceLines(content);
  const blocks: DraftBlock[] = [];
  const diagnostics: ParseResult["diagnostics"] extends readonly (infer T)[] ? T[] : never = [];
  const metadata = frontmatter(lines);
  let title = metadata.title;
  let index = metadata.end;

  while (index < lines.length) {
    const line = lines[index];
    if (!line.text.trim()) {
      index++;
      continue;
    }

    const fence = isFence(line.text);
    if (fence) {
      const marker = fence[1][0];
      const minimum = fence[1].length;
      const language = fence[2].trim().replace(/^\{\.?|}.*$/g, "").split(/\s+/)[0] || null;
      const codeLines: string[] = [];
      let endIndex = index + 1;
      for (; endIndex < lines.length; endIndex++) {
        if (new RegExp(`^\\s{0,3}${marker === "`" ? "`" : "~"}{${minimum},}\\s*$`).test(lines[endIndex].text)) break;
        codeLines.push(lines[endIndex].text);
      }
      const closed = endIndex < lines.length;
      const end = closed ? lines[endIndex].end : lines.at(-1)?.end ?? line.end;
      blocks.push({
        type: "code",
        language,
        code: normalizeCode(codeLines.join("\n")),
        sourceRange: sourceRange(content, line.start, end),
      });
      index = closed ? endIndex + 1 : lines.length;
      continue;
    }

    const heading = isHeading(line.text);
    if (heading) {
      const rawText = heading[2];
      const explicitAnchor = rawText.match(/\s*\{#([A-Za-z][\w:.-]*)}\s*$/)?.[1];
      const text = inlineText(rawText);
      if (!title && heading[1].length === 1) title = text;
      blocks.push({
        type: "heading",
        level: heading[1].length,
        text,
        ...(explicitAnchor ? { anchor: explicitAnchor } : {}),
        links: markdownLinks(rawText, canonicalUrl),
        sourceRange: sourceRange(content, line.start, line.end),
      });
      index++;
      continue;
    }

    if (index + 1 < lines.length && /^\s*(?:=+|-+)\s*$/.test(lines[index + 1].text) && line.text.trim()) {
      const text = inlineText(line.text);
      const level = lines[index + 1].text.includes("=") ? 1 : 2;
      if (!title && level === 1) title = text;
      blocks.push({
        type: "heading",
        level,
        text,
        links: markdownLinks(line.text, canonicalUrl),
        sourceRange: sourceRange(content, line.start, lines[index + 1].end),
      });
      index += 2;
      continue;
    }

    if (line.text.includes("|") && isTableDelimiter(lines[index + 1]?.text ?? "")) {
      const headers = splitTableRow(line.text);
      const rawRows = [line.text];
      const rows: string[][] = [];
      let endIndex = index + 2;
      for (; endIndex < lines.length && lines[endIndex].text.includes("|") && lines[endIndex].text.trim(); endIndex++) {
        rawRows.push(lines[endIndex].text);
        rows.push(splitTableRow(lines[endIndex].text));
      }
      blocks.push({
        type: "table",
        headers,
        rows,
        links: markdownLinks(rawRows.join("\n"), canonicalUrl),
        sourceRange: sourceRange(content, line.start, lines[endIndex - 1]?.end ?? line.end),
      });
      index = endIndex;
      continue;
    }

    const list = isListItem(line.text);
    if (list) {
      const ordered = /^\d/.test(list[2]);
      const items: Array<{ text: string; depth: number; links: NormalizedLink[] }> = [];
      const start = line.start;
      let end = line.end;
      for (; index < lines.length; index++) {
        const item = isListItem(lines[index].text);
        if (!item || /^\d/.test(item[2]) !== ordered) break;
        const rawParts = [item[3]];
        end = lines[index].end;
        while (index + 1 < lines.length && lines[index + 1].text.trim() && !isBlockStart(lines, index + 1) && /^\s+/.test(lines[index + 1].text)) {
          index++;
          rawParts.push(lines[index].text.trim());
          end = lines[index].end;
        }
        const rawText = rawParts.join(" ");
        items.push({
          text: inlineText(rawText),
          depth: Math.floor(item[1].replace(/\t/g, "    ").length / 2),
          links: markdownLinks(rawText, canonicalUrl),
        });
      }
      blocks.push({ type: "list", ordered, items, sourceRange: sourceRange(content, start, end) });
      continue;
    }

    if (/^\s*>/.test(line.text)) {
      const start = line.start;
      const raw: string[] = [];
      let end = line.end;
      for (; index < lines.length && /^\s*>/.test(lines[index].text); index++) {
        raw.push(lines[index].text.replace(/^\s*>\s?/, ""));
        end = lines[index].end;
      }
      const joined = raw.join(" ");
      blocks.push({
        type: "quote",
        text: inlineText(joined),
        links: markdownLinks(joined, canonicalUrl),
        sourceRange: sourceRange(content, start, end),
      });
      continue;
    }

    if (mdx && /^\s*(?:import|export)\s/.test(line.text)) {
      const start = line.start;
      const values: string[] = [];
      let end = line.end;
      for (; index < lines.length && (values.length === 0 || lines[index].text.trim()); index++) {
        values.push(lines[index].text);
        end = lines[index].end;
      }
      blocks.push({ type: "code", language: "tsx", code: normalizeCode(values.join("\n")), sourceRange: sourceRange(content, start, end) });
      diagnostics.push({
        code: "mdx_executable_syntax_preserved",
        severity: "info",
        scope: "document",
        message: "MDX import/export syntax was preserved as inert code and was not executed.",
      });
      continue;
    }

    const start = line.start;
    const raw: string[] = [];
    let end = line.end;
    for (; index < lines.length && lines[index].text.trim() && (raw.length === 0 || !isBlockStart(lines, index)); index++) {
      raw.push(lines[index].text.trim());
      end = lines[index].end;
    }
    const joined = raw.join(" ");
    const text = inlineText(joined);
    if (text) {
      blocks.push({
        type: "paragraph",
        text,
        links: markdownLinks(joined, canonicalUrl),
        sourceRange: sourceRange(content, start, end),
      });
    } else {
      index++;
    }
  }

  return { title, blocks, diagnostics };
}
