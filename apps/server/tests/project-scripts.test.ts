import assert from "node:assert/strict";
import { parseProjectScripts } from "@/api/src/services/project-scripts/config.js";

console.log("Running project script configuration tests...");

assert.deepEqual(parseProjectScripts(`
  [scripts.dev]
  label = "Start Dev"
  command = "bun run dev"
  shortcut = "cmd-r"
  persistent = true

  [scripts.build]
  label = "Build"
  command = "bun run build"
`), [
  { id: "dev", label: "Start Dev", command: "bun run dev", shortcut: "cmd-r", persistent: true },
  { id: "build", label: "Build", command: "bun run build", shortcut: null, persistent: false },
]);
assert.deepEqual(parseProjectScripts("title = \"demo\""), []);
assert.throws(() => parseProjectScripts(`[scripts."bad.id"]\nlabel = "x"\ncommand = "echo x"`), /identifier/);
assert.throws(() => parseProjectScripts(`[scripts.x]\nlabel = "x"\ncommand = "echo x"\nextra = true`), /not supported/);
assert.throws(() => parseProjectScripts(`[scripts.x]\nlabel = "x"\ncommand = "echo x"\npersistent = "yes"`), /boolean/);
assert.throws(() => parseProjectScripts(`[scripts.x]\nlabel = "x"\ncommand = "echo x"\nshortcut = "not-a-shortcut"`), /shortcut/);
assert.throws(() => parseProjectScripts("[scripts.x\nlabel = 'x'"), /Invalid console.toml/);
assert.throws(() => parseProjectScripts("[scripts.x]\ncommand = 'echo x'"), /label/);

console.log("Project script configuration tests passed!\n");
