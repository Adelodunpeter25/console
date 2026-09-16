import assert from "node:assert/strict";
import { portRegistry } from "@/api/src/services/port-registry.service.js";

console.log("Running vite port-detection tests...");

// Control: plain vite-style output registers the port.
const plainTarget = Bun.serve({
  port: 0,
  fetch() {
    return new Response("plain");
  },
});
try {
  await portRegistry.observeOutput(
    { kind: "job", id: "test-vite-plain" },
    `  VITE v7.3.6  ready in 652 ms\n\n  Local:   http://localhost:${plainTarget.port}/\n  Network: use --host to expose\n`,
  );
  const detected = await portRegistry.list("localhost");
  assert.ok(
    detected.some((entry) => entry.port === plainTarget.port),
    "plain vite output should register its port",
  );
  console.log("  plain vite output detects ports");
} finally {
  plainTarget.stop(true);
  await portRegistry.removeOwner({ kind: "job", id: "test-vite-plain" });
}

// Real vite shape: OSC-8 hyperlink where the URL appears as the link target
// (terminated by BEL) and again as the visible label.
const oscTarget = Bun.serve({
  port: 0,
  fetch() {
    return new Response("osc");
  },
});
try {
  const e = String.fromCharCode(27);
  const bel = String.fromCharCode(7);
  const open = `${e}]8;;http://localhost:${oscTarget.port}/${bel}`;
  const close = `${e}]8;;${bel}`;
  const output =
    "  VITE v7.3.6  ready in 652 ms\n\n" +
    `  Local:   ${open}http://localhost:${oscTarget.port}/${close}\n` +
    "  Network: use --host to expose\n";
  await portRegistry.observeOutput({ kind: "job", id: "test-vite-osc8" }, output);
  const detected = await portRegistry.list("localhost");
  assert.ok(
    detected.some((entry) => entry.port === oscTarget.port),
    "OSC-8 wrapped vite URL should register its port",
  );
  console.log("  OSC-8 vite output detects ports");
} finally {
  oscTarget.stop(true);
  await portRegistry.removeOwner({ kind: "job", id: "test-vite-osc8" });
}

// Vite on newer Node/macOS binds the IPv6 loopback only ([::1]); the old
// liveness probe tried 127.0.0.1 alone and dropped the port.
let v6Target: ReturnType<typeof Bun.serve> | undefined;
try {
  v6Target = Bun.serve({
    hostname: "::1",
    port: 0,
    fetch() {
      return new Response("v6");
    },
  });
} catch {
  console.log("  (skip: no IPv6 loopback available)");
}
if (v6Target) {
  const target = v6Target;
  try {
    await portRegistry.observeOutput(
      { kind: "job", id: "test-vite-v6" },
      `  Local:   http://localhost:${target.port}/\n`,
    );
    const detected = await portRegistry.list("localhost");
    assert.ok(
      detected.some((entry) => entry.port === target.port),
      "IPv6-loopback dev server should register its port",
    );
    console.log("  IPv6-loopback dev server detects ports");
  } finally {
    target.stop(true);
    await portRegistry.removeOwner({ kind: "job", id: "test-vite-v6" });
  }
}

// Guard: a URL split across two output chunks must still be detected via
// the per-owner line buffer.
const splitTarget = Bun.serve({
  port: 0,
  fetch() {
    return new Response("split");
  },
});
try {
  const owner = { kind: "job", id: "test-vite-split" } as const;
  await portRegistry.observeOutput(owner, `  Local:   http://localhos`);
  await portRegistry.observeOutput(owner, `t:${splitTarget.port}/\n`);
  const detected = await portRegistry.list("localhost");
  assert.ok(
    detected.some((entry) => entry.port === splitTarget.port),
    "chunk-split vite URL should register its port",
  );
  console.log("  chunk-split vite output detects ports");
} finally {
  splitTarget.stop(true);
  await portRegistry.removeOwner({ kind: "job", id: "test-vite-split" });
}

await portRegistry.closeAll();

console.log("Vite port-detection tests passed!\n");
