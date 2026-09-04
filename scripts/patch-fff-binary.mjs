#!/usr/bin/env node
/**
 * Idempotent build-time patch for @ff-labs/fff-node's findBinary().
 *
 * The stock resolver only checks the platform npm package
 * (@ff-labs/fff-bin-*) and a cargo dev build — both absent inside
 * `bun build --compile` output. This inserts a sidecar check at the top of
 * findBinary(): FFF_LIB_PATH env override, then the directory containing the
 * running binary (install.sh places libfff_c.{dylib,so} next to `console`).
 *
 * Run locally and in CI before `bun build --compile` so the bundled code
 * already contains the lookup. Safe to re-run (marker-guarded).
 */
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const MARKER = "CONSOLE_FFF_SIDECAR";
const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const target = join(repoRoot, "node_modules", "@ff-labs", "fff-node", "dist", "src", "binary.js");

if (!existsSync(target)) {
  console.error(`patch-fff-binary: not found: ${target} (run bun install first)`);
  process.exit(1);
}

const src = readFileSync(target, "utf-8");
if (src.includes(MARKER)) {
  console.log("patch-fff-binary: already patched, skipping");
  process.exit(0);
}

const anchor = "export function findBinary() {";
if (!src.includes(anchor)) {
  console.error("patch-fff-binary: anchor not found — upstream fff-node changed its binary.js layout");
  process.exit(1);
}

const patch = `${anchor}
    // ${MARKER}: sidecar next to the compiled \`console\` binary (set by install.sh).
    // Checked before the npm-package/dev-build lookups below so the single-file
    // build resolves without node_modules. See scripts/patch-fff-binary.mjs.
    try {
        const envPath = typeof process !== "undefined" ? process.env.FFF_LIB_PATH : undefined;
        if (envPath && existsSync(envPath))
            return envPath;
    }
    catch { }
    try {
        const exeDir = dirname(process.execPath);
        const candidates = [join(exeDir, getLibFilename())];
        if (process.platform === "darwin")
            candidates.push(join(exeDir, "libfff_c.dylib"));
        else if (process.platform === "win32")
            candidates.push(join(exeDir, "fff_c.dll"), join(exeDir, "libfff_c.dll"));
        else
            candidates.push(join(exeDir, "libfff_c.so"));
        for (const p of candidates) {
            try {
                if (existsSync(p))
                    return p;
            }
            catch { }
        }
    }
    catch { }`;

writeFileSync(target, src.replace(anchor, patch));
console.log("patch-fff-binary: patched findBinary() with sidecar lookup");
