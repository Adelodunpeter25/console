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
 * Supports both layouts:
 * - 0.10.1: dist/src/binary.js with `export function findBinary() {` (ESM)
 * - 0.10.6+: dist/index.js (ESM) + dist/index.cjs (CJS) with `function findBinary() {`
 *
 * Run locally and in CI before `bun build --compile` so the bundled code
 * already contains the lookup. Safe to re-run (marker-guarded).
 */
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const MARKER = "CONSOLE_FFF_SIDECAR";
const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

const pkgCandidates = [
  join(repoRoot, "node_modules", "@ff-labs", "fff-node"),
  join(repoRoot, "apps", "server", "node_modules", "@ff-labs", "fff-node"),
];

let pkgDir = null;
for (const c of pkgCandidates) {
  if (existsSync(c)) {
    pkgDir = c;
    break;
  }
}

if (!pkgDir) {
  for (const c of pkgCandidates) {
    try {
      console.error(`patch-fff-binary: listing ${dirname(c)}:`);
      console.error(readdirSync(dirname(c)).join(" "));
    } catch {
      console.error(`patch-fff-binary: missing ${dirname(c)}`);
    }
  }
  console.error(`patch-fff-binary: fff-node package not found (run bun install first)`);
  process.exit(1);
}

// Candidate files across known layouts (old + new). Order: new bundle first.
const fileCandidates = [
  join(pkgDir, "dist", "index.js"),
  join(pkgDir, "dist", "index.cjs"),
  join(pkgDir, "dist", "src", "binary.js"),
];

const targets = fileCandidates.filter((f) => {
  if (!existsSync(f)) return false;
  try {
    return readFileSync(f, "utf-8").includes("function findBinary()");
  } catch {
    return false;
  }
});

if (targets.length === 0) {
  console.error(`patch-fff-binary: found ${pkgDir} but no file containing findBinary()`);
  try {
    console.error(`patch-fff-binary: dist listing: ${readdirSync(join(pkgDir, "dist")).join(" ")}`);
  } catch {
    console.error(`patch-fff-binary: no dist/ directory in ${pkgDir}`);
  }
  try {
    const pkgJson = JSON.parse(readFileSync(join(pkgDir, "package.json"), "utf-8"));
    console.error(`patch-fff-binary: installed version: ${pkgJson.version}`);
  } catch {}
  process.exit(1);
}

function esmPatch(anchor) {
  return `${anchor}
    // ${MARKER}: sidecar next to the compiled \`console\` binary.
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
}

function cjsPatch(anchor) {
  return `${anchor}
    // ${MARKER}: sidecar next to the compiled \`console\` binary (CJS bundle).
    try {
        const envPath = typeof process !== "undefined" ? process.env.FFF_LIB_PATH : undefined;
        if (envPath && (0, import_node_fs.existsSync)(envPath))
            return envPath;
    }
    catch { }
    try {
        const exeDir = (0, import_node_path.dirname)(process.execPath);
        const candidates = [(0, import_node_path.join)(exeDir, getLibFilename())];
        if (process.platform === "darwin")
            candidates.push((0, import_node_path.join)(exeDir, "libfff_c.dylib"));
        else if (process.platform === "win32")
            candidates.push((0, import_node_path.join)(exeDir, "fff_c.dll"), (0, import_node_path.join)(exeDir, "libfff_c.dll"));
        else
            candidates.push((0, import_node_path.join)(exeDir, "libfff_c.so"));
        for (const p of candidates) {
            try {
                if ((0, import_node_fs.existsSync)(p))
                    return p;
            }
            catch { }
        }
    }
    catch { }`;
}

let patched = 0;
let skipped = 0;
for (const target of targets) {
  const src = readFileSync(target, "utf-8");
  if (src.includes(MARKER)) {
    console.log(`patch-fff-binary: already patched, skipping ${target}`);
    skipped++;
    continue;
  }
  const isCjs = target.endsWith(".cjs") || src.includes("import_node_fs");
  // Old layout uses `export function`, new bundle uses plain `function`.
  const anchor = src.includes("export function findBinary() {")
    ? "export function findBinary() {"
    : "function findBinary() {";
  if (!src.includes(anchor)) {
    console.error(`patch-fff-binary: anchor not found in ${target} — upstream changed layout again`);
    process.exit(1);
  }
  const patch = isCjs ? cjsPatch(anchor) : esmPatch(anchor);
  writeFileSync(target, src.replace(anchor, patch));
  console.log(`patch-fff-binary: patched ${target}`);
  patched++;
}

console.log(`patch-fff-binary: done (patched ${patched}, skipped ${skipped})`);
