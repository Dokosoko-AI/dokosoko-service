import { access } from "node:fs/promises";

const requiredFiles = ["dist/client/index.html"];

for (const file of requiredFiles) {
  try {
    await access(file);
  } catch {
    throw new Error(`Static console build is incomplete: ${file} was not generated.`);
  }
}
