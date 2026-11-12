import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { build } from "esbuild";
import { MIO_SPRITES } from "../internal/web/src/mio-assets.generated";

const source = "internal/web/src/app.ts";
const generatedPath = new URL("../internal/web/generated/app.js", import.meta.url);
const htmlPath = new URL("../internal/web/index.html", import.meta.url);
const fontPath = new URL("../internal/web/fonts/quicksand-latin-wght-normal.woff2", import.meta.url);
const versionedScriptUrl = /\/static\/app\.js\?v=([0-9a-f]{64})/g;
const versionedFontUrl = /\/static\/fonts\/quicksand-latin-wght-normal\.woff2\?v=([0-9a-f]{64})/g;
const spriteNames = ["angry", "concerned", "curious", "happy", "idle", "looking", "sleeping", "worried"] as const;

async function main(): Promise<void> {
    const result = await build({
        entryPoints: [source],
        bundle: true,
        format: "iife",
        target: "es2020",
        outfile: "internal/web/generated/app.js",
        write: false,
    });
    const compiled = result.outputFiles[0]?.contents;
    if (!compiled) throw new Error("esbuild did not produce a browser bundle");

    const generated = await readFile(generatedPath);
    if (!generated.equals(Buffer.from(compiled))) {
        throw new Error("generated/app.js is stale; run npm run build:web and commit the updated bundle");
    }

    const html = await readFile(htmlPath, "utf8");
    const references = [...html.matchAll(versionedScriptUrl)];
    const digest = createHash("sha256").update(compiled).digest("hex");
    if (references.length !== 1 || references[0]?.[1] !== digest) {
        throw new Error("index.html does not contain the current app.js content hash; run npm run build:web");
    }

    const font = await readFile(fontPath);
    const fontDigest = createHash("sha256").update(font).digest("hex");
    const fontReferences = [...html.matchAll(versionedFontUrl)];
    if (fontReferences.length !== 2 || fontReferences.some(reference => reference[1] !== fontDigest)) {
        throw new Error("index.html does not contain the current Quicksand font content hash; run npm run build:web");
    }

    for (const name of spriteNames) {
        const filename = `mio_pose_${name}.png`;
        const sprite = await readFile(new URL(`../internal/web/mio/${filename}`, import.meta.url));
        const spriteHash = createHash("sha256").update(sprite).digest("hex");
        const expectedURL = `/static/mio/${filename}?v=${spriteHash}`;
        if (MIO_SPRITES[name] !== expectedURL) {
            throw new Error(`${filename} has a stale cache-buster; run npm run build:web`);
        }
    }
}

await main();
