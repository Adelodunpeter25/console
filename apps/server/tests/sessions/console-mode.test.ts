/**
 * Console storage-mode resolution tests.
 * CONSOLE_ENV is authoritative; NODE_ENV=development only counts for
 * non-compiled runtimes (compiled Bun binaries default it to development).
 */
import assert from "node:assert/strict";
import * as os from "node:os";
import * as path from "node:path";
import { getConsoleStorageDir, resolveConsoleMode } from "@/agent/src/session/apppaths.js";

console.log("Running console storage-mode tests...");

function withEnv(env: Record<string, string | undefined>, fn: () => void): void {
  const saved: Record<string, string | undefined> = {};
  for (const key of Object.keys(env)) {
    saved[key] = process.env[key];
    if (env[key] === undefined) delete process.env[key];
    else process.env[key] = env[key];
  }
  try {
    fn();
  } finally {
    for (const key of Object.keys(env)) {
      if (saved[key] === undefined) delete process.env[key];
      else process.env[key] = saved[key];
    }
  }
}

const devDir = path.join(os.homedir(), ".console-dev");
const prodDir = path.join(os.homedir(), ".console");

// 1. Explicit CONSOLE_ENV=dev wins, even with a clean NODE_ENV
{
  withEnv({ CONSOLE_ENV: "dev", NODE_ENV: undefined }, () => {
    assert.equal(resolveConsoleMode(), "dev");
    assert.equal(getConsoleStorageDir(), devDir);
  });
  console.log("  ✅ CONSOLE_ENV=dev selects dev storage");
}

// 2. Explicit CONSOLE_ENV=production beats NODE_ENV=development
{
  withEnv({ CONSOLE_ENV: "production", NODE_ENV: "development" }, () => {
    assert.equal(resolveConsoleMode(), "production");
    assert.equal(getConsoleStorageDir(), prodDir);
  });
  console.log("  ✅ CONSOLE_ENV=production is authoritative");
}

// 3. Legacy NODE_ENV sniffing still works for non-compiled runtimes
{
  withEnv({ CONSOLE_ENV: undefined, NODE_ENV: "development" }, () => {
    // Test runner executes under plain `bun`, never a compiled binary.
    assert.equal(resolveConsoleMode(), "dev");
    assert.equal(getConsoleStorageDir(), devDir);
  });
  console.log("  ✅ NODE_ENV=development keeps working outside compiled binaries");
}

// 4. Clean environment resolves to production (the old compiled-binary bug)
{
  withEnv({ CONSOLE_ENV: undefined, NODE_ENV: undefined }, () => {
    assert.equal(resolveConsoleMode(), "production");
    assert.equal(getConsoleStorageDir(), prodDir);
  });
  console.log("  ✅ clean environment resolves to production storage");
}

console.log("Console storage-mode tests passed!\n");
process.exit(0);
