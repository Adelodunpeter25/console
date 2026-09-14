/**
 * Focused tests for the file-preview gate in FsService.readFileContent
 * (docs/notes/file-preview-gating.md): lockfiles, binary extensions,
 * NUL-byte sniffing, size ceiling. Server is the enforcement layer of record.
 */
import assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import * as os from "node:os";
import path from "node:path";
import { FsService, FilePreviewBlockedError } from "@/api/src/services/fs.service.js";
import { MAX_FILE_PREVIEW_BYTES } from "@console/types";

console.log("Running fs file-preview gate tests...");

const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), "console-fs-gate-test-"));
const fsService = new FsService();

const write = (name: string, content: string | Uint8Array) =>
  fs.writeFile(path.join(tempDir, name), content);

const expectBlocked = async (name: string) => {
  try {
    await fsService.readFileContent(path.join(tempDir, name));
    assert.fail(`${name} should have been blocked`);
  } catch (err) {
    assert.ok(err instanceof FilePreviewBlockedError, `${name}: expected FilePreviewBlockedError`);
    return err as FilePreviewBlockedError;
  }
};

try {
  // 1. Regular text files still read fine (incl. line ranges)
  const textPath = path.join(tempDir, "notes.txt");
  await write("notes.txt", "alpha\nbeta\ngamma\n");
  assert.equal(await fsService.readFileContent(textPath), "alpha\nbeta\ngamma\n");
  assert.equal(await fsService.readFileContent(textPath, 2, 3), "beta\ngamma");
  console.log("  ✅ text files read with line ranges intact");

  // 2. Empty allowed file
  await write("empty.ts", "");
  assert.equal(await fsService.readFileContent(path.join(tempDir, "empty.ts")), "");
  console.log("  ✅ empty file returns empty string");

  // 3. Lockfiles by suffix and exact basename
  for (const name of ["Cargo.lock", "yarn.lock", "bun.lockb", "package-lock.json", "pnpm-lock.yaml"]) {
    await write(name, "generated noise\n");
    const err = await expectBlocked(name);
    assert.equal(err.code, "LOCKFILE_BLOCKED", name);
    assert.equal(err.status, 415, name);
  }
  console.log("  ✅ lockfiles rejected (LOCKFILE_BLOCKED)");

  // 4. Binary extensions
  await write("logo.png", Buffer.from([0x89, 0x50, 0x4e, 0x47]));
  const pngErr = await expectBlocked("logo.png");
  assert.equal(pngErr.code, "BINARY_FILE");
  console.log("  ✅ binary extensions rejected (BINARY_FILE)");

  // 5. NUL-byte sniff catches binaries wearing a text extension
  await write("sneaky.json", Buffer.from([0x7b, 0x00, 0x7d]));
  const sniffErr = await expectBlocked("sneaky.json");
  assert.equal(sniffErr.code, "BINARY_FILE");
  console.log("  ✅ NUL-byte content sniff rejects disguised binaries");

  // 6. Size ceiling with structured detail
  await write("huge.log", "a".repeat(MAX_FILE_PREVIEW_BYTES + 1));
  const bigErr = await expectBlocked("huge.log");
  assert.equal(bigErr.code, "FILE_TOO_LARGE");
  assert.equal(bigErr.status, 413);
  assert.equal(bigErr.detail?.maxBytes, MAX_FILE_PREVIEW_BYTES);
  assert.equal(bigErr.detail?.sizeBytes, MAX_FILE_PREVIEW_BYTES + 1);
  console.log("  ✅ oversize files rejected (FILE_TOO_LARGE, 413 + detail)");

  // 7. Exactly at the cap is allowed
  await write("at-cap.md", "b".repeat(MAX_FILE_PREVIEW_BYTES));
  const atCap = await fsService.readFileContent(path.join(tempDir, "at-cap.md"));
  assert.equal(atCap.length, MAX_FILE_PREVIEW_BYTES);
  console.log("  ✅ file exactly at cap still reads");

  console.log("\nAll file-preview gate tests passed!");
} finally {
  await fs.rm(tempDir, { recursive: true, force: true });
}
