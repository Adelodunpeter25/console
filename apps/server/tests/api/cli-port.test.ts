import assert from "node:assert/strict";
import { resolvePortHost } from "../../cli/daemon-manager.js";

console.log("Running CLI saved port/host resolution tests...");

const saved = { port: "9090", host: "127.0.0.1", logLevel: "info" };

// 1. Bare start reuses saved config when no flag and no env
{
  const r = resolvePortHost({}, saved, {});
  assert.equal(r.port, "9090");
  assert.equal(r.host, "127.0.0.1");
  console.log("  ✅ bare start reuses saved port/host");
}

// 2. CLI flag wins over saved and env
{
  const r = resolvePortHost({ port: "3001", host: "0.0.0.0" }, saved, { PORT: "9090", HOST: "127.0.0.1" });
  assert.equal(r.port, "3001");
  assert.equal(r.host, "0.0.0.0");
  console.log("  ✅ --port/--host flags win");
}

// 3. Env wins over saved when no flag (PORT=... console start)
{
  const r = resolvePortHost({}, saved, { PORT: "4000", HOST: "10.0.0.1" });
  assert.equal(r.port, "4000");
  assert.equal(r.host, "10.0.0.1");
  console.log("  ✅ env PORT/HOST override saved config");
}

// 4. Falls back to defaults when nothing saved and no flag/env
{
  const r = resolvePortHost({}, { port: "3000", host: "0.0.0.0", logLevel: "info" }, {});
  assert.equal(r.port, "3000");
  assert.equal(r.host, "0.0.0.0");
  console.log("  ✅ defaults to 3000/0.0.0.0 on first run");
}

// 5. Partial flag: --port only keeps saved host
{
  const r = resolvePortHost({ port: "5000" }, saved, {});
  assert.equal(r.port, "5000");
  assert.equal(r.host, "127.0.0.1");
  console.log("  ✅ partial flag keeps saved host");
}

console.log("CLI saved port/host resolution tests passed!\n");
process.exit(0);
