import { readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const workspaceRoot = resolve(scriptDirectory, "../../..");
const generatedApiPath = resolve(workspaceRoot, "lib/api-zod/src/generated/api.ts");
const generatedBarrelPath = resolve(workspaceRoot, "lib/api-zod/src/index.ts");

let generatedApi = await readFile(generatedApiPath, "utf8");
generatedApi = generatedApi
  .replaceAll("zod.int()", "zod.number().int()")
  .replaceAll("zod.url()", "zod.string().url()");

if (/\bzod\.(?:int|url)\(\)/.test(generatedApi)) {
  throw new Error("Generated Zod source still contains helpers unsupported by the installed Zod 3 runtime.");
}

await writeFile(generatedApiPath, generatedApi);
await writeFile(generatedBarrelPath, 'export * from "./generated/api";\n');