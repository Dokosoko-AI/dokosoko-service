import { normalizeCode, normalizeProse, resolveLink, sourceRange } from "./canonical";
import type { DraftBlock, ParseResult } from "./parser-types";
import type { NormalizedLink } from "./types";

type HTMLNode = HTMLRoot | HTMLChildNode;
type HTMLChildNode = HTMLElementNode | HTMLTextNode;
type HTMLRoot = { kind: "root"; children: HTMLChildNode[]; start: number; end: number };
type HTMLElementNode = {
  kind: "element";
  tag: string;
  attributes: Readonly<Record<string, string>>;
  children: HTMLChildNode[];
  start: number;
  end: number;
};
type HTMLTextNode = { kind: "text"; value: string; start: number; end: number };

const voidElements = new Set(["area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr"]);
const excludedElements = new Set(["script", "style", "noscript", "svg", "canvas", "iframe", "template", "nav", "footer", "aside", "form", "dialog"]);
const blockElements = new Set([
  "address", "article", "aside", "blockquote", "details", "div", "dl", "fieldset", "figcaption", "figure", "footer",
  "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "main", "nav", "ol", "p", "pre", "section",
  "summary", "table", "ul",
]);

function decodeEntities(value: string): string {
  const named: Readonly<Record<string, string>> = {
    amp: "&",
    apos: "'",
    gt: ">",
    lt: "<",
    nbsp: " ",
    quot: '"',
  };
  return value.replace(/&(?:#(\d+)|#x([\da-f]+)|([a-z][\da-z]+));/gi, (entity, decimal: string, hexadecimal: string, name: string) => {
    try {
      if (decimal) return String.fromCodePoint(Number(decimal));
      if (hexadecimal) return String.fromCodePoint(Number.parseInt(hexadecimal, 16));
      return named[name.toLowerCase()] ?? entity;
    } catch {
      return "�";
    }
  });
}

function parseAttributes(raw: string): Readonly<Record<string, string>> {
  const result: Record<string, string> = {};
  const withoutTag = raw.replace(/^<\/?\s*[A-Za-z][\w:-]*/, "").replace(/\/?>\s*$/, "");
  for (const match of withoutTag.matchAll(/([^\s=/>]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+)))?/g)) {
    result[match[1].toLowerCase()] = decodeEntities(match[2] ?? match[3] ?? match[4] ?? "");
  }
  return result;
}

function parseHTML(content: string): HTMLRoot {
  const root: HTMLRoot = { kind: "root", children: [], start: 0, end: content.length };
  const stack: Array<HTMLRoot | HTMLElementNode> = [root];
  const tokenPattern = /<!--[\s\S]*?-->|<![^>]*>|<\/?[A-Za-z][^>]*>|[^<]+|</g;
  for (const match of content.matchAll(tokenPattern)) {
    const token = match[0];
    const start = match.index;
    const end = start + token.length;
    if (token.startsWith("<!--") || /^<![^-]/.test(token)) continue;
    const closing = token.match(/^<\/\s*([A-Za-z][\w:-]*)[^>]*>/);
    if (closing) {
      const tag = closing[1].toLowerCase();
      for (let index = stack.length - 1; index > 0; index--) {
        const candidate = stack[index];
        if (candidate.kind === "element") candidate.end = end;
        stack.pop();
        if (candidate.kind === "element" && candidate.tag === tag) break;
      }
      continue;
    }
    const opening = token.match(/^<\s*([A-Za-z][\w:-]*)/);
    if (opening) {
      const tag = opening[1].toLowerCase();
      const element: HTMLElementNode = {
        kind: "element",
        tag,
        attributes: parseAttributes(token),
        children: [],
        start,
        end,
      };
      stack.at(-1)?.children.push(element);
      if (!voidElements.has(tag) && !/\/\s*>$/.test(token)) stack.push(element);
      continue;
    }
    stack.at(-1)?.children.push({ kind: "text", value: decodeEntities(token), start, end });
  }
  for (const value of stack) value.end = content.length;
  return root;
}

function isElement(node: HTMLNode, tag?: string): node is HTMLElementNode {
  return node.kind === "element" && (tag === undefined || node.tag === tag);
}

function hidden(element: HTMLElementNode): boolean {
  const style = element.attributes.style ?? "";
  return Object.hasOwn(element.attributes, "hidden")
    || element.attributes["aria-hidden"]?.toLowerCase() === "true"
    || /(?:display\s*:\s*none|visibility\s*:\s*hidden)/i.test(style);
}

function excluded(element: HTMLElementNode): boolean {
  return excludedElements.has(element.tag) || hidden(element) || /^(?:navigation|complementary)$/.test(element.attributes.role ?? "");
}

function findFirst(node: HTMLNode, tags: ReadonlySet<string>): HTMLElementNode | null {
  if (isElement(node) && excluded(node)) return null;
  if (isElement(node) && tags.has(node.tag)) return node;
  if (node.kind === "text") return null;
  for (const child of node.children) {
    const found = findFirst(child, tags);
    if (found) return found;
  }
  return null;
}

function descendants(element: HTMLElementNode, tag: string, stopAtSameTag = false): HTMLElementNode[] {
  const result: HTMLElementNode[] = [];
  const visit = (node: HTMLNode) => {
    if (!isElement(node) || excluded(node)) return;
    if (node.tag === tag) {
      result.push(node);
      if (stopAtSameTag) return;
    }
    node.children.forEach(visit);
  };
  element.children.forEach(visit);
  return result;
}

function nodeText(node: HTMLNode, omitTags: ReadonlySet<string> = new Set()): string {
  if (node.kind === "text") return node.value;
  if (node.kind === "element" && (excluded(node) || omitTags.has(node.tag))) return "";
  const separator = node.kind === "element" && (blockElements.has(node.tag) || node.tag === "br") ? "\n" : " ";
  return node.children.map((child) => nodeText(child, omitTags)).join(separator);
}

function nodeLinks(node: HTMLNode, canonicalUrl: string): NormalizedLink[] {
  const links: NormalizedLink[] = [];
  const seen = new Set<string>();
  const visit = (candidate: HTMLNode) => {
    if (!isElement(candidate) || excluded(candidate)) return;
    if (candidate.tag === "a") {
      const target = resolveLink(candidate.attributes.href ?? "", canonicalUrl);
      const label = normalizeProse(nodeText(candidate));
      const key = `${label}\0${target}`;
      if (target && label && !seen.has(key)) {
        seen.add(key);
        links.push({ label, target });
      }
    }
    candidate.children.forEach(visit);
  };
  visit(node);
  return links;
}

function directChildren(element: HTMLElementNode, tag: string): HTMLElementNode[] {
  return element.children.filter((node): node is HTMLElementNode => isElement(node, tag) && !excluded(node));
}

function elementRange(content: string, element: HTMLElementNode) {
  return sourceRange(content, element.start, element.end);
}

function tableBlock(content: string, element: HTMLElementNode, canonicalUrl: string): DraftBlock | null {
  const rows = descendants(element, "tr", true).map((row) => row.children
    .filter((node): node is HTMLElementNode => isElement(node) && (node.tag === "th" || node.tag === "td") && !excluded(node))
    .map((cell) => normalizeProse(nodeText(cell))));
  const nonEmpty = rows.filter((row) => row.some(Boolean));
  if (nonEmpty.length === 0) return null;
  const firstRow = descendants(element, "tr", true)[0];
  const firstIsHeader = firstRow?.children.some((node) => isElement(node, "th")) ?? false;
  return {
    type: "table",
    headers: firstIsHeader ? nonEmpty[0] : [],
    rows: firstIsHeader ? nonEmpty.slice(1) : nonEmpty,
    links: nodeLinks(element, canonicalUrl),
    sourceRange: elementRange(content, element),
  };
}

function listBlock(content: string, element: HTMLElementNode, canonicalUrl: string): DraftBlock | null {
  const items: Array<{ text: string; depth: number; links: NormalizedLink[] }> = [];
  const collect = (list: HTMLElementNode, depth: number) => {
    for (const item of directChildren(list, "li")) {
      const text = normalizeProse(nodeText(item, new Set(["ol", "ul"])));
      if (text) items.push({ text, depth, links: nodeLinks(item, canonicalUrl) });
      for (const nested of item.children.filter((node): node is HTMLElementNode => isElement(node) && (node.tag === "ol" || node.tag === "ul"))) {
        collect(nested, depth + 1);
      }
    }
  };
  collect(element, 0);
  if (items.length === 0) return null;
  return { type: "list", ordered: element.tag === "ol", items, sourceRange: elementRange(content, element) };
}

export function parseHTMLDocument(content: string, canonicalUrl: string): ParseResult {
  const root = parseHTML(content);
  const titleElement = findFirst(root, new Set(["title"]));
  const main = findFirst(root, new Set(["main"])) ?? findFirst(root, new Set(["article"])) ?? findFirst(root, new Set(["body"])) ?? root;
  const blocks: DraftBlock[] = [];
  let removedElements = 0;

  const pushInlineRun = (nodes: HTMLNode[]) => {
    if (nodes.length === 0) return;
    const text = normalizeProse(nodes.map((node) => nodeText(node)).join(" "));
    if (!text) return;
    const start = Math.min(...nodes.map((node) => node.start));
    const end = Math.max(...nodes.map((node) => node.end));
    const links = nodes.flatMap((node) => nodeLinks(node, canonicalUrl));
    blocks.push({ type: "paragraph", text, links, sourceRange: sourceRange(content, start, end) });
  };

  const walkContainer = (container: HTMLRoot | HTMLElementNode) => {
    let inline: HTMLNode[] = [];
    const flush = () => {
      pushInlineRun(inline);
      inline = [];
    };
    for (const node of container.children) {
      if (node.kind === "text") {
        if (normalizeProse(node.value)) inline.push(node);
        continue;
      }
      if (excluded(node)) {
        removedElements++;
        continue;
      }
      if (!blockElements.has(node.tag)) {
        inline.push(node);
        continue;
      }
      flush();
      if (/^h[1-6]$/.test(node.tag)) {
        const text = normalizeProse(nodeText(node));
        if (text) blocks.push({
          type: "heading",
          level: Number(node.tag[1]),
          text,
          ...(node.attributes.id ? { anchor: node.attributes.id } : {}),
          links: nodeLinks(node, canonicalUrl),
          sourceRange: elementRange(content, node),
        });
      } else if (node.tag === "p" || node.tag === "summary" || node.tag === "figcaption") {
        const text = normalizeProse(nodeText(node));
        if (text) blocks.push({ type: "paragraph", text, links: nodeLinks(node, canonicalUrl), sourceRange: elementRange(content, node) });
      } else if (node.tag === "blockquote") {
        const text = normalizeProse(nodeText(node));
        if (text) blocks.push({ type: "quote", text, links: nodeLinks(node, canonicalUrl), sourceRange: elementRange(content, node) });
      } else if (node.tag === "pre") {
        const codeElement = findFirst(node, new Set(["code"]));
        const languageClass = codeElement?.attributes.class?.match(/(?:^|\s)language-([\w.+#-]+)/i)?.[1] ?? null;
        blocks.push({ type: "code", language: languageClass, code: normalizeCode(nodeText(codeElement ?? node)), sourceRange: elementRange(content, node) });
      } else if (node.tag === "table") {
        const table = tableBlock(content, node, canonicalUrl);
        if (table) blocks.push(table);
      } else if (node.tag === "ul" || node.tag === "ol") {
        const list = listBlock(content, node, canonicalUrl);
        if (list) blocks.push(list);
      } else if (node.tag !== "hr") {
        walkContainer(node);
      }
    }
    flush();
  };

  walkContainer(main);
  const firstH1 = blocks.find((block) => block.type === "heading" && block.level === 1);
  const title = normalizeProse(titleElement ? nodeText(titleElement) : "") || (firstH1?.type === "heading" ? firstH1.text : "");
  return {
    title,
    blocks,
    diagnostics: removedElements > 0 ? [{
      code: "boilerplate_removed",
      severity: "info",
      scope: "document",
      message: "Navigation, hidden, or active HTML elements were excluded from normalized content.",
      details: { removedElements },
    }] : [],
  };
}
