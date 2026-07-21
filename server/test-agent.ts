#!/usr/bin/env tsx
import { Agent, allTools } from "./agent/src/index.js";
import { createAntigravityStreamFn } from "./providers/src/index.js";

console.log("Starting agent (antigravity)...");

const agent = new Agent({
  model: { id: "gemini-3-flash-agent", contextWindow: 1_000_000, provider: "antigravity" },
  tools: allTools,
  systemPrompt: "You are a helpful assistant. The user's project is located at /Users/techclub/Documents/projects/oh-my-pi.",
  streamFn: createAntigravityStreamFn(),
  onEvent: (event) => {
    if (event.type === "modelStreamPart") {
      if (event.part.text) {
        process.stdout.write(event.part.text);
      }
    } else if (event.type === "toolExecutionStart") {
      console.log(`\n[Tool execution] ${event.calls.map(c => c.name).join(", ")}`);
    } else if (event.type === "toolExecutionResult") {
      console.log(`[Tool result] ${event.result.toolCallId}`);
    } else if (event.type === "error") {
      console.error(`\n[Error] ${event.error.message}`);
    }
  },
});

const stream = agent.run("List the project directory structure in a nice tree format");

for await (const event of stream) {
  // Events are handled in onEvent
}

const result = await stream.result();
console.log("\n\nRun complete! Full history:");
console.log(result.map((m) => `${m.role}: ${JSON.stringify(m.content).slice(0, 100)}...`).join("\n"));
