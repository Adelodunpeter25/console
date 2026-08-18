import assert from "node:assert/strict";
import { z } from "zod";
import { bindToolCwd } from "@console/types";
import {
  executeTool,
  pathString,
  validateToolInput,
  type AgentTool,
} from "../agent/src/index.js";

console.log("Running tool-input harness tests...");

{
  let received: unknown;
  const bound = bindToolCwd(
    {
      name: "boundPath",
      description: "Test bound path tool",
      inputSchema: z.object({ path: z.string(), cwd: z.string().optional() }),
      execute: async (args) => {
        received = args;
        return { content: [{ type: "text", text: "ok" }] };
      },
    },
    "/project",
  );

  assert.equal("cwd" in (bound.inputSchema as z.AnyZodObject).shape, false);
  await bound.execute(bound.inputSchema.parse({ path: "." }));
  assert.deepEqual(received, { path: ".", cwd: "/project" });
  console.log("  ✅ bound tools hide cwd and inject the project directory");
}

const listSchema = z.object({
  items: z.array(z.string()),
  optional: z.string().optional(),
});

// Repairs happen only after the raw parse fails, and compose in the intended order.
{
  const result = validateToolInput(listSchema, {
    items: '["one", "two"]',
    optional: null,
  });
  assert.equal(result.success, true);
  assert.deepEqual(result.data, { items: ["one", "two"] });
  assert.deepEqual(result.repairedPaths.sort(), ["items", "optional"]);
  console.log("  ✅ null omission and stringified array repair");
}

{
  const result = validateToolInput(listSchema, { items: "one" });
  assert.equal(result.success, true);
  assert.deepEqual(result.data, { items: ["one"] });
  console.log("  ✅ bare string array repair");
}

{
  const result = validateToolInput(
    z.object({ items: z.array(z.object({ name: z.string() })) }),
    { items: { name: "one" } },
  );
  assert.equal(result.success, true);
  assert.deepEqual(result.data, { items: [{ name: "one" }] });
  console.log("  ✅ single object array repair");
}

{
  const content = '["this is file content"]';
  const result = validateToolInput(z.object({ content: z.string() }), { content });
  assert.equal(result.success, true);
  assert.deepEqual(result.data, { content });
  assert.deepEqual(result.repairedPaths, []);
  console.log("  ✅ valid content remains untouched");
}

{
  const result = validateToolInput(
    z.object({ path: pathString("Filesystem path") }),
    { path: "[notes.md](http://notes.md)" },
  );
  assert.equal(result.success, true);
  assert.deepEqual(result.data, { path: "notes.md" });
  console.log("  ✅ markdown path leakage repair");
}

{
  const tool: AgentTool = {
    name: "collect",
    description: "Collect strings",
    inputSchema: z.object({ values: z.array(z.string()) }),
    execute: async (args) => ({ content: [{ type: "text", text: args.values.join(",") }] }),
  };

  const repaired = await executeTool(
    { id: "repair-call", name: "collect", arguments: { values: "one" } },
    [tool],
    "full-access",
    undefined,
    () => {},
    undefined,
    undefined,
  );
  assert.equal(repaired.isError, undefined);

  const invalid = await executeTool(
    { id: "invalid-call", name: "collect", arguments: { values: 42 } },
    [tool],
    "full-access",
    undefined,
    () => {},
    undefined,
    undefined,
  );
  assert.equal(invalid.isError, true);
  assert.equal(String(invalid.content).startsWith("Error:"), false);
  console.log("  ✅ executor repair and invalid-input recovery");
}

console.log("Tool-input harness tests passed!\n");
