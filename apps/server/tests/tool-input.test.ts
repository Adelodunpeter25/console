import assert from "node:assert/strict";
import { z } from "zod";
import {
  executeTool,
  pathString,
  ToolInputTelemetry,
  validateToolInput,
  type AgentTool,
  type Model,
} from "../agent/src/index.js";

console.log("Running tool-input harness tests...");

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
  const events: string[] = [];
  const telemetry = new ToolInputTelemetry();
  const model: Model = { id: "repair-model", provider: "opencode", contextWindow: 8192 };
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
    undefined,
    model,
    (event) => events.push(`${event.label}:${event.modelId}`),
    telemetry,
  );
  assert.equal(repaired.isError, undefined);
  assert.deepEqual(events, ["tool_input_repaired:collect:repair-model"]);

  const invalid = await executeTool(
    { id: "invalid-call", name: "collect", arguments: { values: 42 } },
    [tool],
    "full-access",
    undefined,
    () => {},
    undefined,
    undefined,
    undefined,
    model,
    (event) => events.push(`${event.label}:${event.modelId}`),
    telemetry,
  );
  assert.equal(invalid.isError, true);
  assert.equal(String(invalid.content).startsWith("Error:"), false);
  assert.equal(events[1], "tool_input_invalid:collect:repair-model");
  assert.deepEqual(telemetry.snapshot(), [
    {
      modelId: "repair-model",
      provider: "opencode",
      toolName: "collect",
      repaired: 1,
      invalid: 1,
      repairRate: 0.5,
    },
  ]);
  console.log("  ✅ executor repair and per-model telemetry");
}

console.log("Tool-input harness tests passed!\n");
