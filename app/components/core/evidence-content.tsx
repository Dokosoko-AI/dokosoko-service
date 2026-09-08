import { Fragment, type ReactNode } from "react";

// Imported evidence is text, never HTML. This deliberately small presentation
// renderer supports common prose/code without embeds or external image loads.
export function evidenceLink(value: string): string | undefined {
  if (value.startsWith("#") && !/[\s<>]/.test(value)) return value;
  try {
    const url = new URL(value);
    if (["https:", "http:"].includes(url.protocol) && !url.username && !url.password) return url.href;
  } catch { /* Relative or malformed evidence links remain readable text. */ }
  return undefined;
}

function inline(text: string): ReactNode[] {
  const tokens = /(`[^`\n]+`|\*\*[^*\n]+\*\*|\[[^\]\n]+\]\([^\s)]+\))/g;
  const result: ReactNode[] = [];
  let offset = 0;
  for (const match of text.matchAll(tokens)) {
    result.push(text.slice(offset, match.index));
    const value = match[0];
    const key = match.index;
    if (value.startsWith("`")) result.push(<code key={key}>{value.slice(1, -1)}</code>);
    else if (value.startsWith("**")) result.push(<strong key={key}>{value.slice(2, -2)}</strong>);
    else {
      const link = /^\[([^\]]+)\]\((.+)\)$/.exec(value)!;
      const href = evidenceLink(link[2]);
      result.push(href ? <a key={key} href={href} target="_blank" rel="noreferrer noopener">{link[1]}</a> : <Fragment key={key}>{value}</Fragment>);
    }
    offset = match.index + value.length;
  }
  result.push(text.slice(offset));
  return result;
}

export function EvidenceContent({ text, label }: { text: string; label: string }) {
  const lines = text.replace(/\r\n?/g, "\n").split("\n");
  const blocks: ReactNode[] = [];
  for (let i = 0; i < lines.length;) {
    const start = i;
    const line = lines[i++];
    if (!line.trim()) continue;
    const fence = /^\s*(`{3,}|~{3,})([^\s]*)/.exec(line);
    if (fence) {
      const code: string[] = [];
      while (i < lines.length && !lines[i].trimStart().startsWith(fence[1])) code.push(lines[i++]);
      if (i < lines.length) i++;
      blocks.push(<pre key={start}><code>{code.join("\n")}</code></pre>);
      continue;
    }
    const heading = /^(#{1,6})\s+(.+)$/.exec(line);
    if (heading) {
      const Tag = `h${Math.min(heading[1].length + 1, 6)}` as "h2" | "h3" | "h4" | "h5" | "h6";
      blocks.push(<Tag key={start}>{inline(heading[2])}</Tag>);
      continue;
    }
    const list = /^\s*(?:[-*+] |\d+\. )(.+)$/.exec(line);
    if (list) {
      const values = [list[1]];
      while (i < lines.length) {
        const next = /^\s*(?:[-*+] |\d+\. )(.+)$/.exec(lines[i]);
        if (!next) break;
        values.push(next[1]); i++;
      }
      const Tag = /^\s*\d/.test(line) ? "ol" : "ul";
      blocks.push(<Tag key={start}>{values.map((value, index) => <li key={index}>{inline(value)}</li>)}</Tag>);
      continue;
    }
    const paragraph = [line];
    while (i < lines.length && lines[i].trim() && !/^\s*(#{1,6}\s|`{3,}|~{3,}|[-*+] |\d+\. )/.test(lines[i])) paragraph.push(lines[i++]);
    blocks.push(<p key={start}>{inline(paragraph.join("\n"))}</p>);
  }
  return <article className="core-evidence-content" aria-label={label}>{blocks}</article>;
}

export function evidenceChange(before: string, after: string) {
  const left = before.split("\n"), right = after.split("\n");
  let prefix = 0, suffix = 0;
  while (prefix < left.length && prefix < right.length && left[prefix] === right[prefix]) prefix++;
  while (suffix < left.length - prefix && suffix < right.length - prefix && left[left.length - 1 - suffix] === right[right.length - 1 - suffix]) suffix++;
  return { unchanged: before === after, removed: left.slice(prefix, left.length - suffix).join("\n"), added: right.slice(prefix, right.length - suffix).join("\n") };
}
