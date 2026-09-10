/**
 * Tests for GET /api/fs/file/raw (image & SVG preview raw bytes endpoint).
 * Verifies allowed rasters/SVGs, content-type headers, size limits, and gating rules.
 */
import assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import * as os from "node:os";
import path from "node:path";
import { createApiApp } from "@/api/src/index.js";
import { IMAGE_MAX_BYTES } from "@console/types";

console.log("Running fs raw file preview endpoint tests...");

const app = createApiApp();
const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), "console-fs-raw-test-"));

try {
  // 1. Missing path query parameter -> 400
  {
    const res = await app.request("/api/fs/file/raw");
    assert.equal(res.status, 400);
    const json = await res.json();
    assert.equal(json.success, false);
    assert.ok(json.error.includes("path"));
    console.log("  ✅ missing path returns 400");
  }

  // 2. Non-existent file -> 400
  {
    const missingPath = path.join(tempDir, "non-existent.png");
    const res = await app.request(`/api/fs/file/raw?path=${encodeURIComponent(missingPath)}`);
    assert.equal(res.status, 400);
    const json = await res.json();
    assert.equal(json.success, false);
    console.log("  ✅ non-existent file returns 400");
  }

  // 3. Raster images allowed with correct Content-Type and headers
  {
    const pngBytes = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
    const pngPath = path.join(tempDir, "test.png");
    await fs.writeFile(pngPath, pngBytes);

    const res = await app.request(`/api/fs/file/raw?path=${encodeURIComponent(pngPath)}`);
    assert.equal(res.status, 200);
    assert.equal(res.headers.get("Content-Type"), "image/png");
    assert.equal(res.headers.get("Content-Length"), String(pngBytes.length));
    assert.equal(res.headers.get("Cache-Control"), "private, max-age=30");
    const body = Buffer.from(await res.arrayBuffer());
    assert.deepEqual(body, pngBytes);
    console.log("  ✅ .png returns raw bytes with image/png");
  }

  // 4. Other raster formats (jpg, webp)
  {
    const jpgBytes = Buffer.from([0xff, 0xd8, 0xff, 0xe0]);
    const jpgPath = path.join(tempDir, "photo.jpg");
    await fs.writeFile(jpgPath, jpgBytes);

    const res = await app.request(`/api/fs/file/raw?path=${encodeURIComponent(jpgPath)}`);
    assert.equal(res.status, 200);
    assert.equal(res.headers.get("Content-Type"), "image/jpeg");
    console.log("  ✅ .jpg returns image/jpeg");
  }

  // 5. SVG allowed with image/svg+xml
  {
    const svgContent = '<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"></svg>';
    const svgPath = path.join(tempDir, "icon.svg");
    await fs.writeFile(svgPath, svgContent);

    const res = await app.request(`/api/fs/file/raw?path=${encodeURIComponent(svgPath)}`);
    assert.equal(res.status, 200);
    assert.equal(res.headers.get("Content-Type"), "image/svg+xml");
    const body = await res.text();
    assert.equal(body, svgContent);
    console.log("  ✅ .svg returns image/svg+xml");
  }

  // 6. Non-image binary (.zip) blocked -> 415 BINARY_FILE
  {
    const zipPath = path.join(tempDir, "archive.zip");
    await fs.writeFile(zipPath, Buffer.from([0x50, 0x4b, 0x03, 0x04]));

    const res = await app.request(`/api/fs/file/raw?path=${encodeURIComponent(zipPath)}`);
    assert.equal(res.status, 415);
    const json = await res.json();
    assert.equal(json.code, "BINARY_FILE");
    console.log("  ✅ .zip blocked with 415 BINARY_FILE");
  }

  // 7. Lockfiles blocked -> 415 LOCKFILE_BLOCKED
  {
    const lockPath = path.join(tempDir, "Cargo.lock");
    await fs.writeFile(lockPath, "noise");

    const res = await app.request(`/api/fs/file/raw?path=${encodeURIComponent(lockPath)}`);
    assert.equal(res.status, 415);
    const json = await res.json();
    assert.equal(json.code, "LOCKFILE_BLOCKED");
    console.log("  ✅ lockfile blocked with 415 LOCKFILE_BLOCKED");
  }

  // 8. Text file blocked -> 415 BINARY_FILE
  {
    const txtPath = path.join(tempDir, "readme.txt");
    await fs.writeFile(txtPath, "hello world");

    const res = await app.request(`/api/fs/file/raw?path=${encodeURIComponent(txtPath)}`);
    assert.equal(res.status, 415);
    const json = await res.json();
    assert.equal(json.code, "BINARY_FILE");
    console.log("  ✅ text file blocked from raw image preview");
  }

  // 9. Oversized image (> IMAGE_MAX_BYTES) -> 413 FILE_TOO_LARGE with detail
  {
    const bigPath = path.join(tempDir, "huge.png");
    // Write 1 byte over the limit using truncate to avoid huge memory allocation
    const handle = await fs.open(bigPath, "w");
    await handle.truncate(IMAGE_MAX_BYTES + 1);
    await handle.close();

    const res = await app.request(`/api/fs/file/raw?path=${encodeURIComponent(bigPath)}`);
    assert.equal(res.status, 413);
    const json = await res.json();
    assert.equal(json.code, "FILE_TOO_LARGE");
    assert.equal(json.sizeBytes, IMAGE_MAX_BYTES + 1);
    assert.equal(json.maxBytes, IMAGE_MAX_BYTES);
    console.log("  ✅ oversized image blocked with 413 FILE_TOO_LARGE + detail");
  }

  console.log("\nAll fs raw file preview endpoint tests passed!");
} finally {
  await fs.rm(tempDir, { recursive: true, force: true });
}
