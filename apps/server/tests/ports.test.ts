import assert from "node:assert/strict";
import { createApiApp } from "@/api/src/index.js";
import { portRegistry } from "@/api/src/services/port-registry.service.js";

console.log("Running port forwarding tests...");

const target = Bun.serve({
  port: 0,
  fetch(request) {
    return new Response(`target:${new URL(request.url).pathname}`);
  },
});

try {
  const app = createApiApp();
  const forward = await app.request("/api/ports/forward", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ port: target.port }),
  });
  assert.equal(forward.status, 200);
  const forwarded = await forward.json() as { success: boolean; data: { port: number; url: string } };
  assert.equal(forwarded.success, true);
  assert.equal(forwarded.data.port, target.port);
  assert.match(forwarded.data.url, /^http:\/\/localhost:\d+\/$/);

  const preview = await fetch(`${forwarded.data.url}hello?from=test`);
  assert.equal(preview.status, 200);
  assert.equal(await preview.text(), "target:/hello");

  const streamResponse = await app.request("/api/ports/stream", {
    headers: { host: "example.test:3000" },
  });
  assert.equal(streamResponse.status, 200);
  const reader = streamResponse.body!.getReader();
  const decoder = new TextDecoder();
  const firstChunk = decoder.decode((await reader.read()).value);
  assert.match(firstChunk, /event: ports/);
  assert.match(firstChunk, new RegExp(`\\"port\\":${target.port}`));

  const list = await app.request("/api/ports", { headers: { host: "example.test:3000" } });
  assert.equal(list.status, 200);
  const listed = await list.json() as { success: boolean; data: Array<{ port: number; url: string }> };
  assert.deepEqual(listed.data, [{ port: target.port, url: listed.data[0]!.url }]);
  assert.match(listed.data[0]!.url, /^http:\/\/example\.test:\d+\/$/);

  const removed = await app.request(`/api/ports/${target.port}`, { method: "DELETE" });
  assert.equal(removed.status, 200);
  await reader.cancel();
  const afterRemove = await app.request("/api/ports");
  assert.deepEqual((await afterRemove.json()).data, []);

  const missing = await app.request("/api/ports/forward", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ port: 49999 }),
  });
  assert.equal(missing.status, 400);

  console.log("  ✅ manual forward, proxy, list, delete, and probe failure");
} finally {
  await portRegistry.closeAll();
  target.stop(true);
}

console.log("Port forwarding tests passed!\n");
