#!/usr/bin/env node

const esbuild = require("esbuild");
const fs = require("fs");
const path = require("path");

const isMinify = true;

// Recursively collect .ts files (excludes .d.ts)
function collectTsFiles(dir) {
    const files = [];
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const fullPath = path.join(dir, entry.name);
        if (entry.isDirectory()) {
            files.push(...collectTsFiles(fullPath));
        } else if (
            entry.name.endsWith(".ts") &&
            !entry.name.endsWith(".d.ts")
        ) {
            files.push(fullPath);
        }
    }
    return files;
}

async function buildProtobufSDK() {
    console.log("Building protobuf SDK...");

    const tsSourceDir = path.join(__dirname, "ts_source");
    const stroppyFiles = collectTsFiles(
        path.join(tsSourceDir, "proto", "stroppy"),
    );
    const googleFiles = collectTsFiles(
        path.join(tsSourceDir, "google", "protobuf"),
    );

    // Create entry file that re-exports everything. `export * from` silently
    // drops names declared by more than one source module, so each known
    // collision is resolved after the star re-exports by naming the winner
    // explicitly. Today the only collision is `InsertMethod` (legacy
    // `stroppy.InsertMethod` from descriptor_pb vs new
    // `stroppy.datagen.InsertMethod` from datagen_pb); the canonical datagen
    // enum keeps the short name and the legacy one is exposed via the alias
    // `LegacyInsertMethod` for the old InsertDescriptor path.
    const entryPath = path.join(__dirname, "_entry.ts");
    const rel = (file) =>
        "./" + path.relative(__dirname, file).replace(/\\/g, "/").replace(/\.ts$/, "");
    const starLines = stroppyFiles.map((file) => `export * from '${rel(file)}';`);
    const datagenFile = stroppyFiles.find((f) => rel(f).endsWith("/datagen_pb"));
    const descriptorFile = stroppyFiles.find((f) => rel(f).endsWith("/descriptor_pb"));
    const explicitLines = [];
    if (datagenFile) {
        explicitLines.push(`export { InsertMethod } from '${rel(datagenFile)}';`);
    }
    if (descriptorFile) {
        explicitLines.push(
            `export { InsertMethod as LegacyInsertMethod } from '${rel(descriptorFile)}';`,
        );
    }
    const entryContent = [...starLines, ...explicitLines].join("\n");
    fs.writeFileSync(entryPath, entryContent);

    // Bundle to JS
    await esbuild.build({
        entryPoints: [entryPath],
        bundle: true,
        format: "esm",
        platform: "node",
        target: "es2020",
        outfile: path.join(__dirname, "dist", "bundle.js"),
        external: ["k6", "k6/*"],
        minify: isMinify,
        sourcemap: false,
        mainFields: ["module", "main"],
    });

    // Generate combined TypeScript for IDE support
    // @ts-nocheck: generated code has stripped imports that tsc can't resolve (PbLong, JsonWriteOptions, etc.)
    // The file is used for IDE type inference, not direct compilation.
    //
    // Colliding names across the concatenated `_pb.ts` bodies (e.g. legacy
    // `stroppy.InsertMethod` vs new `stroppy.datagen.InsertMethod`) must
    // match the aliases defined in the runtime bundle entry above so that
    // tsc sees the same export surface as esbuild produces.
    const combinedTS = [
        "// @ts-nocheck",
        "// Combined TypeScript definitions for stroppy protobuf",
        "// @generated",
        "",
        'import type { BinaryWriteOptions, IBinaryWriter, BinaryReadOptions, IBinaryReader, PartialMessage, FieldList } from "@protobuf-ts/runtime";',
        'import { MessageType, WireType, UnknownFieldHandler, reflectionMergePartial } from "@protobuf-ts/runtime";',
        "",
        // Process google files first, then stroppy files
        ...[...googleFiles, ...stroppyFiles]
            .map((file) => {
                const content = fs
                    .readFileSync(file, "utf8")
                    .replace(/^import\s+.*?from\s+['"@].*?['"];?\s*$/gm, "")
                    .trim();
                return content;
            })
            .filter(Boolean),
        "",
        "// Collision aliases: the concatenated bodies above redeclare a few",
        "// names; expose the legacy copy under a distinct identifier so",
        "// callers that need it stay explicit. Values mirror descriptor.proto",
        "// exactly (legacy ordering).",
        "export enum LegacyInsertMethod {",
        "    PLAIN_QUERY = 0,",
        "    NATIVE = 1,",
        "    PLAIN_BULK = 2,",
        "}",
    ].join("\n\n");

    fs.writeFileSync(path.join(__dirname, "stroppy.pb.ts"), combinedTS);
    fs.unlinkSync(entryPath);

    console.log("✓ Protobuf SDK built successfully");
}

async function buildParseSQL() {
    console.log("Building parse_sql...");

    await esbuild.build({
        entryPoints: [path.join(__dirname, "parse_sql.ts")],
        bundle: true,
        format: "esm",
        platform: "node",
        target: "es2020",
        outfile: path.join(__dirname, "dist", "parse_sql.js"),
        external: ["k6", "k6/*", "./stroppy.pb.js"],
        minify: isMinify,
        sourcemap: false,
        mainFields: ["module", "main"],
    });

    console.log("✓ parse_sql built successfully");
}

async function main() {
    fs.mkdirSync(path.join(__dirname, "dist"), { recursive: true });
    await buildProtobufSDK();
    await buildParseSQL();
    console.log("\n✓ All bundles built successfully!");
}

main().catch((error) => {
    console.error("Build failed:", error);
    process.exit(1);
});
