/**
 * Unit & Integration Tests for Hono API Layer & Services (server/api/).
 * Runs 100% offline using Hono's app.request() in-memory testing — 0 LLM credits used.
 */
import assert from "node:assert/strict";
import path from "node:path";
import { createApiApp } from "../api/src/index.js";

console.log("Running Hono API Layer & Service tests...");

const app = createApiApp();

// 1. Health check endpoint
{
  const res = await app.request("/health");
  assert.equal(res.status, 200);
  const json = await res.json();
  assert.equal(json.status, "ok");
  assert.equal(json.engine, "console-agent");
  console.log("  ✅ GET /health");
}

// 2. Auth status endpoint
{
  const res = await app.request("/api/auth/status");
  assert.equal(res.status, 200);
  const json = await res.json();
  assert.equal(json.success, true);
  assert.ok(json.data.gemini);
  assert.ok(json.data.antigravity);
  console.log("  ✅ GET /api/auth/status");
}

// 3. Remote OAuth Login URL generation
{
  const res = await app.request("/api/auth/login/url", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ provider: "antigravity" }),
  });
  assert.equal(res.status, 200);
  const json = await res.json();
  assert.equal(json.success, true);
  assert.ok(json.data.authUrl.includes("accounts.google.com"));
  console.log("  ✅ POST /api/auth/login/url");
}

// 4. File Browser & File operations endpoints (/api/fs/*)
{
  // Browse directory
  const browseRes = await app.request(`/api/fs/browse?path=${encodeURIComponent(process.cwd())}`);
  assert.equal(browseRes.status, 200);
  const browseJson = await browseRes.json();
  assert.equal(browseJson.success, true);
  assert.ok(Array.isArray(browseJson.data.entries));

  // Directory Tree
  const treeRes = await app.request(`/api/fs/tree?path=${encodeURIComponent(process.cwd())}`);
  assert.equal(treeRes.status, 200);
  const treeJson = await treeRes.json();
  assert.equal(treeJson.success, true);
  assert.ok(treeJson.data.treeFormatted);

  // File write, read, delete
  const testFilePath = path.join(process.cwd(), "scratch", "api-test-file.txt");
  const writeRes = await app.request("/api/fs/file", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path: testFilePath, content: "Hello API Service" }),
  });
  assert.equal(writeRes.status, 200);

  const readRes = await app.request(`/api/fs/file?path=${encodeURIComponent(testFilePath)}`);
  assert.equal(readRes.status, 200);
  const readJson = await readRes.json();
  assert.ok(readJson.data.content.includes("Hello API Service"));

  const delRes = await app.request(`/api/fs/file?path=${encodeURIComponent(testFilePath)}`, {
    method: "DELETE",
  });
  assert.equal(delRes.status, 200);

  // Directory create & delete
  const testDirPath = path.join(process.cwd(), "scratch", "api-test-dir");
  const createDirRes = await app.request("/api/fs/dir", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path: testDirPath }),
  });
  assert.equal(createDirRes.status, 200);

  const delDirRes = await app.request(`/api/fs/dir?path=${encodeURIComponent(testDirPath)}`, {
    method: "DELETE",
  });
  assert.equal(delDirRes.status, 200);
  console.log("  ✅ File Browser & File Operations (/api/fs/*)");
}

// 5. Project Listing & Project Folder Selection
{
  const projRes = await app.request("/api/projects");
  assert.equal(projRes.status, 200);
  const projJson = await projRes.json();
  assert.equal(projJson.success, true);
  assert.ok(Array.isArray(projJson.data));

  // Add selected folder as project
  const addProjRes = await app.request("/api/projects", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path: process.cwd() }),
  });
  assert.equal(addProjRes.status, 200);
  const addProjJson = await addProjRes.json();
  assert.equal(addProjJson.success, true);
  assert.equal(addProjJson.data.path, process.cwd());
  console.log("  ✅ GET /api/projects & POST /api/projects");
}

// 6. Provider & Model endpoints
{
  const provRes = await app.request("/api/providers");
  assert.equal(provRes.status, 200);
  const provJson = await provRes.json();
  assert.equal(provJson.success, true);
  assert.ok(provJson.data.length >= 2);

  const modelRes = await app.request("/api/providers/antigravity/models");
  assert.equal(modelRes.status, 200);
  const modelJson = await modelRes.json();
  assert.equal(modelJson.success, true);
  assert.equal(modelJson.data.provider, "antigravity");
  assert.ok(modelJson.data.models.length > 0);
  console.log("  ✅ GET /api/providers & GET /api/providers/:id/models");
}

// 7. Session Persistence CRUD
{
  // Create
  const createRes = await app.request("/api/sessions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      title: "API Test Session",
      cwd: process.cwd(),
      modelId: "gemini-2.5-pro",
      provider: "antigravity",
    }),
  });
  assert.equal(createRes.status, 200);
  const createJson = await createRes.json();
  assert.equal(createJson.success, true);
  const sessionId = createJson.data.id;

  // Get Detail
  const detailRes = await app.request(`/api/sessions/${sessionId}`);
  assert.equal(detailRes.status, 200);
  const detailJson = await detailRes.json();
  assert.equal(detailJson.success, true);

  // Patch Title
  const patchRes = await app.request(`/api/sessions/${sessionId}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title: "Renamed API Session" }),
  });
  assert.equal(patchRes.status, 200);

  // Delete
  const delRes = await app.request(`/api/sessions/${sessionId}`, {
    method: "DELETE",
  });
  assert.equal(delRes.status, 200);
  console.log("  ✅ Session CRUD (/api/sessions)");
}

console.log("Hono API Layer & Service tests passed!\n");
