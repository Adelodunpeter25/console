/**
 * Test storage isolation. Import this FIRST (before any `@/` import) in
 * tests that hit HTTP routes: ES modules evaluate imports in order, so the
 * env var lands before route modules create the shared session storage.
 *
 * Without this, `createApiApp()` tests write sessions/projects/favorites
 * into the real `~/.console` database. Call `teardownIsolatedStorage()`
 * at the end of linear test scripts (or in `afterAll` for `bun:test`
 * files) to remove the temp dir.
 */
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { __resetSharedSessionStorageForTests } from "@/agent/src/session/storage.js";

if (!process.env.CONSOLE_STORAGE_DIR) {
  process.env.CONSOLE_STORAGE_DIR = fs.mkdtempSync(
    path.join(os.tmpdir(), "console-test-storage-"),
  );
}

export function teardownIsolatedStorage(): void {
  const dir = process.env.CONSOLE_STORAGE_DIR;
  if (!dir || !dir.includes("console-test-storage-")) return;
  delete process.env.CONSOLE_STORAGE_DIR;
  try {
    __resetSharedSessionStorageForTests();
  } catch {
    // Best-effort: handles may already be closed.
  }
  try {
    fs.rmSync(dir, { recursive: true, force: true });
  } catch {
    // Best-effort: OS cleans /tmp eventually.
  }
}
