import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";

const htmlPath = new URL("../internal/web/index.html", import.meta.url);
const scriptPath = new URL("../internal/web/generated/app.js", import.meta.url);
const versionedScriptUrl = /\/static\/app\.js\?v=[0-9a-f]{64}/g;

async function main(): Promise<void> {
    const script = await readFile(scriptPath);
    const html = await readFile(htmlPath, "utf8");
    const references = html.match(versionedScriptUrl) ?? [];

    if (references.length !== 1) {
        throw new Error(`expected one versioned app.js URL, found ${references.length}`);
    }

    const digest = createHash("sha256").update(script).digest("hex");
    const updated = html.replace(references[0]!, `/static/app.js?v=${digest}`);
    await writeFile(htmlPath, updated, "utf8");
}

await main();
