import assert from "node:assert/strict";
import { createApiApp } from "@/api/src/app.js";

console.log("Running Device Service tests...");

const app = createApiApp();

// 1. Diagnostics endpoint
const diagRes = await app.request("/api/devices/diagnostics");
assert.equal(diagRes.status, 200);
const diagData = (await diagRes.json()) as { success: boolean; data: Record<string, unknown> };
assert.equal(diagData.success, true);
assert.ok(typeof diagData.data.hasEnoughDiskSpace === "boolean");
assert.ok(Array.isArray(diagData.data.errors));
console.log("  ✅ GET /api/devices/diagnostics returns system checks");

// 2. Devices list endpoint
const listRes = await app.request("/api/devices");
assert.equal(listRes.status, 200);
const listData = (await listRes.json()) as { success: boolean; data: Array<{ id: string; platform: string }> };
assert.equal(listData.success, true);
assert.ok(Array.isArray(listData.data));
console.log(`  ✅ GET /api/devices returned ${listData.data.length} discovered devices`);

// 3. Live stream endpoint initializes
const streamRes = await app.request("/api/devices/nonexistent-id/stream?platform=ios", {
  signal: AbortSignal.timeout(2000),
});
assert.equal(streamRes.status, 200);
assert.equal(streamRes.headers.get("content-type"), "multipart/x-mixed-replace; boundary=frame");
console.log("  ✅ GET /api/devices/:id/stream returns live multipart frame stream");

console.log("Device Service tests passed!\n");
