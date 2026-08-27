import { gzip } from "node:zlib";
import { promisify } from "node:util";
import { readdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";

const gzipAsync = promisify(gzip);
const root = path.resolve("dist/client");
const compressible = new Set([".css", ".html", ".js", ".json", ".map", ".rsc", ".svg", ".txt", ".xml"]);

async function visit(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  await Promise.all(entries.map(async (entry) => {
    const filename = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      await visit(filename);
      return;
    }
    if (!entry.isFile() || !compressible.has(path.extname(entry.name).toLowerCase())) return;
    const source = await readFile(filename);
    if (source.length < 1024) return;
    const compressed = await gzipAsync(source, { level: 9 });
    if (compressed.length >= source.length) return;
    await writeFile(`${filename}.gz`, compressed, { mode: 0o644 });
  }));
}

await visit(root);
