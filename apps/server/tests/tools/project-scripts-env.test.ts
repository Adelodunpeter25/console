import assert from "node:assert/strict";
import { buildScriptEnv } from "@/api/src/services/project-scripts/service.js";

console.log("Running project script env sanitization tests...");

// 1. Strips the daemon's PORT/HOST so project dev servers use their own defaults.
{
  const env = buildScriptEnv({ PORT: "9090", HOST: "127.0.0.1", PATH: "/usr/bin" } as NodeJS.ProcessEnv);
  assert.equal("PORT" in env, false);
  assert.equal("HOST" in env, false);
  assert.equal(env.PATH, "/usr/bin");
  console.log("  ✅ strips PORT/HOST, keeps the rest");
}

// 2. Strips CONSOLE_DAEMON so children don't think they are the daemon.
{
  const env = buildScriptEnv({ CONSOLE_DAEMON: "true", FOO: "bar" } as NodeJS.ProcessEnv);
  assert.equal("CONSOLE_DAEMON" in env, false);
  assert.equal(env.FOO, "bar");
  console.log("  ✅ strips CONSOLE_DAEMON");
}

// 3. Skips undefined values and leaves the input untouched.
{
  const base = { PORT: "9090", KEEP: "yes", EMPTY: undefined } as unknown as NodeJS.ProcessEnv;
  const env = buildScriptEnv(base);
  assert.deepEqual(env, { KEEP: "yes" });
  assert.equal(base.PORT, "9090");
  console.log("  ✅ drops undefined, does not mutate input");
}

console.log("Project script env sanitization tests passed!\n");
process.exit(0);
