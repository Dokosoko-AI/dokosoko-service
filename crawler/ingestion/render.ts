import type { NormalizedBlock, NormalizedLink } from "./types";

function safeCell(value: string): string {
  return value.replaceAll("|", "\\|").replace(/\s*\n\s*/g, " ");
}

export function renderBlock(block: NormalizedBlock): string {
  switch (block.type) {
    case "heading":
      return `${"#".repeat(block.level)} ${block.text}`;
    case "paragraph":
      return block.text;
    case "quote":
      return block.text.split("\n").map((line) => `> ${line}`).join("\n");
    case "list":
      return block.items.map((item, index) => {
        const marker = block.ordered ? `${index + 1}.` : "-";
        return `${"  ".repeat(item.depth)}${marker} ${item.text}`;
      }).join("\n");
    case "table": {
      const width = Math.max(block.headers.length, ...block.rows.map((row) => row.length), 1);
      const headers = Array.from({ length: width }, (_, index) => safeCell(block.headers[index] ?? `Column ${index + 1}`));
      const divider = headers.map(() => "---");
      const rows = block.rows.map((row) => Array.from({ length: width }, (_, index) => safeCell(row[index] ?? "")));
      return [headers, divider, ...rows].map((row) => `| ${row.join(" | ")} |`).join("\n");
    }
    case "code": {
      const longestFence = Math.max(3, ...[...block.code.matchAll(/`+/g)].map((match) => match[0].length + 1));
      const fence = "`".repeat(longestFence);
      return `${fence}${block.language ?? ""}\n${block.code}\n${fence}`;
    }
  }
}

export function blockPlainText(block: NormalizedBlock): string {
  switch (block.type) {
    case "heading":
    case "paragraph":
    case "quote":
      return block.text;
    case "list":
      return block.items.map((item) => item.text).join("\n");
    case "table":
      return [...block.headers, ...block.rows.flat()].join("\n");
    case "code":
      return block.code;
  }
}

export function blockLinks(block: NormalizedBlock): readonly NormalizedLink[] {
  if (block.type === "code") return [];
  if (block.type === "list") return block.items.flatMap((item) => item.links);
  return block.links;
}

