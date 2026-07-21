#!/usr/bin/env tsx
import { Agent, allTools } from "./agent/src/index.js";
import { createAntigravityStreamFn } from "./providers/src/index.js";

console.log("Starting agent (antigravity)...");

const agent = new Agent({
  model: { id: "gemini-2.5-flash", contextWindow: 1_000_000 },
  tools: allTools,
  systemPrompt: "You are a helpful assistant. The user's project is located at /Users/techclub/Documents/projects/oh-my-pi.",
  streamFn: createAntigravityStreamFn(),
  onEvent: (event) => {
    if (event.type === "text-delta") {
      process.stdout.write(event.text);
    } else if (event.type === "tool-call-start") {
      console.log(`\nCalling tool: ${event.name}`);
    } else if (event.type === "tool-call-end") {
      console.log(`\nTool ${event.name} finished`);
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
