# FFF native library fix

## Problem

The compiled `console` binary crashes with:

> fff native library not found. Run `npx @ff-labs/fff-node download` or build from source with `cargo build --release -p fff-c`

## Root cause

`@ff-labs/fff-node` loads `libfff_c` at runtime via `dlopen` (`ffi-rs`). Its `findBinary()` looks for the platform package under `node_modules/@ff-labs/fff-bin-*/`. `bun build --compile` embeds JS but not that `.so`/`.dylib`, and `install.sh` ships only the single `console-*` file, so the lookup returns null. Triggered from `FileFinder.create` in the glob, grep, and assist file-search tools.

## Agreed fix (no fallback)

No pure-TS fallback. Single-file output is not required; the user-facing `console` command must just work as one executable.

1. CI: after the per-target `bun install --os=X --cpu=Y`, copy the matching `libfff_c` from `@ff-labs/fff-bin-*` and upload it to the `console-server` release next to each `console-<suffix>` asset.
2. Installer: update `install.sh` to download both `console` and its matching `libfff_c` sidecar into the install dir (`~/.local/bin`).
3. Runtime: add a small bootstrap (imported first in `apps/cli/console.ts` and `apps/server/index.ts`) that locates the sidecar via `FFF_LIB_PATH` env override, then the directory containing the running binary, then the dev `node_modules` path, and pre-loads it before any `FileFinder` use. Alternative if double-`open` proves problematic: patch `findBinary()` to check the binary's directory.

## Future consideration

Rewriting the backend in Rust would remove this class of issue entirely (`fff-c` becomes a statically linked crate, true single binary). Go would not fix it cleanly (same native lib via `cgo`, or a different search library with behavior drift). Recommendation: sidecar fix now; evaluate Rust incrementally (search tools first) rather than a full rewrite.
