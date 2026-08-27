import assert from "node:assert/strict";
import { spawnCapture } from "@/api/src/utils/exec.js";
import { bashTool } from "@/agent/src/tools/bash.js";

console.log("Running Subprocess & Bash Tool Abort tests...");

// 1. spawnCapture aborts immediately even with child subshell and pipelines
{
  const controller = new AbortController();
  const start = Date.now();
  setTimeout(() => controller.abort(), 150);

  const res = await spawnCapture(["/bin/sh", "-c", "echo start && (sleep 5; echo done)"], {
    cwd: process.cwd(),
    signal: controller.signal,
  });

  const duration = Date.now() - start;
  assert.ok(res.aborted, "Result should be marked aborted");
  assert.ok(res.killed, "Result should be marked killed");
  assert.ok(
    duration < 800,
    `spawnCapture should abort quickly, took ${duration}ms (expected < 800ms)`,
  );
  console.log(`  ✅ spawnCapture aborted subshell pipeline in ${duration}ms`);
}

// 2. bashTool execute with abort signal produces clean abort error output
{
  const controller = new AbortController();
  const start = Date.now();
  setTimeout(() => controller.abort(), 150);

  const parsed = bashTool.inputSchema.parse({
    command: "(sleep 5; echo done)",
  });

  const res = (await bashTool.execute(parsed, controller.signal)) as {
    content: Array<{ text: string }>;
    isError: boolean;
  };

  const duration = Date.now() - start;
  assert.ok(res.isError, "Result should be isError: true on abort");
  assert.ok(
    res.content[0]?.text.includes("Command cancelled by user abort"),
    "Result should indicate user abort",
  );
  assert.ok(
    duration < 800,
    `bashTool should abort quickly, took ${duration}ms (expected < 800ms)`,
  );
  console.log(`  ✅ bashTool aborted in ${duration}ms with cancelled message`);
}

console.log("All Subprocess & Bash Tool Abort tests passed! ✅");
