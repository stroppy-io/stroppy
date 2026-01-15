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

    // Create entry file that re-exports everything
    const entryPath = path.join(__dirname, "_entry.ts");
    const entryContent = stroppyFiles
        .map(
            (file) =>
                `export * from './${path.relative(__dirname, file).replace(/\\/g, "/").replace(/\.ts$/, "")}';`,
        )
        .join("\n");
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
    const combinedTS = [
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
    ].join("\n\n");

    fs.writeFileSync(path.join(__dirname, "stroppy.pb.ts"), combinedTS);
    fs.unlinkSync(entryPath);

    console.log("✓ Protobuf SDK built successfully");
}

async function buildAnalyzeDDL() {
    console.log("Building analyze_ddl...");

    await esbuild.build({
        entryPoints: [path.join(__dirname, "analyze_ddl.ts")],
        bundle: true,
        format: "esm",
        platform: "node",
        target: "es2020",
        outfile: path.join(__dirname, "dist", "analyze_ddl.js"),
        external: ["k6", "k6/*", "./stroppy.pb.js"],
        minify: isMinify,
        sourcemap: false,
        mainFields: ["module", "main"],
    });

    console.log("✓ analyze_ddl built successfully");
}

async function buildParseSQL2() {
    console.log("Building parse_sql_2...");

    await esbuild.build({
        entryPoints: [path.join(__dirname, "parse_sql_2.ts")],
        bundle: true,
        format: "esm",
        platform: "node",
        target: "es2020",
        outfile: path.join(__dirname, "dist", "parse_sql_2.js"),
        external: ["k6", "k6/*", "./stroppy.pb.js"],
        minify: isMinify,
        sourcemap: false,
        mainFields: ["module", "main"],
    });

    console.log("✓ parse_sql_2 built successfully");
}

async function main() {
    fs.mkdirSync(path.join(__dirname, "dist"), { recursive: true });
    await buildProtobufSDK();
    await buildAnalyzeDDL();
    await buildParseSQL2();
    console.log("\n✓ All bundles built successfully!");
}

main().catch((error) => {
    console.error("Build failed:", error);
    process.exit(1);
});
