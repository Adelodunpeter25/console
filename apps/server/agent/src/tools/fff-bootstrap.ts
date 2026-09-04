/**
 * FFF native library sidecar bootstrap.
 *
 * Must be imported before any `FileFinder.create` call (see `apps/cli/console.ts`
 * and `apps/server/index.ts`). `fff-node` resolves `libfff_c` via the platform
 * bin package (fff-bin-<platform>), which does not exist inside
 * the compiled `console` binary. The patched `findBinary()` (see
 * `scripts/patch-fff-binary.mjs`, bundled at compile time) checks
 * `process.env.FFF_LIB_PATH` first, then the directory containing the running
 * binary. This module sets that env var when a sidecar is found next to the
 * executable so the bundled lookup succeeds. No-op in `bun` dev where
 * `node_modules` resolution already works. Never pre-opens the library here —
 * that would double-`open` against fff-node's own `ffi-rs` handle.
 */
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";

const SIDECAR_FILENAMES =
  process.platform === "darwin"
    ? ["libfff_c.dylib"]
    : process.platform === "win32"
      ? ["fff_c.dll", "libfff_c.dll"]
      : ["libfff_c.so"];

export function resolveFffSidecar(): string | null {
  const override = process.env.FFF_LIB_PATH;
  if (override && existsSync(override)) return override;

  try {
    const exeDir = dirname(process.execPath);
    for (const name of SIDECAR_FILENAMES) {
      const candidate = join(exeDir, name);
      if (existsSync(candidate)) return candidate;
    }
  } catch {
    // process.execPath may be unavailable in some test runners — fall through.
  }

  return null;
}

const sidecar = resolveFffSidecar();
if (sidecar && !process.env.FFF_LIB_PATH) {
  process.env.FFF_LIB_PATH = sidecar;
}
