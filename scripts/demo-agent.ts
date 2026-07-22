/**
 * Interactive Live Demo CLI Runner for Console Agent Engine.
 *
 * Runs real agent prompts against live Gemini / Antigravity LLM endpoints using your local credentials.
 *
 * Usage:
 *   npx tsx scripts/demo-agent.ts "List files in this project and build a hello world script"
 *   npx tsx scripts/demo-agent.ts --provider antigravity --model gemini-2.5-pro "What tools do you have?"
 */
import readline from "node:readline";
import { Agent } from "../server/agent/src/service/agent.js";
import { allTools } from "../server/agent/src/tools/index.js";
import { SqliteSessionStorage } from "../server/agent/src/session/storage.js";
import { buildSystemPrompt } from "../server/agent/src/systemprompt/builder.js";
import { createAntigravityStreamFn } from "../server/providers/src/antigravity/stream-fn.js";
import { geminiStreamFn } from "../server/providers/src/gemini/stream-fn.js";
import type { AgentSessionEvent, Model } from "../shared/src/index.js";

function parseArgs(): { prompt: string; provider: "gemini" | "antigravity"; modelId: string } {
  const args = process.argv.slice(2);
  let provider: "gemini" | "antigravity" = "gemini";
  let modelId = "gemini-2.5-pro";
  const promptParts: string[] = [];

  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--provider" && args[i + 1]) {
      provider = (args[i + 1] ?? "gemini") as "gemini" | "antigravity";
      i++;
    } else if (args[i] === "--model" && args[i + 1]) {
      modelId = args[i + 1] ?? "gemini-2.5-pro";
      i++;
    } else if (args[i]) {
      promptParts.push(args[i]!);
    }
  }

  return {
    prompt: promptParts.join(" "),
    provider,
    modelId,
  };
}

async function promptUserInteractive(): Promise<string> {
  const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  return new Promise((resolve) => {
    rl.question("\nEnter your prompt for Console Agent: ", (answer) => {
      rl.close();
      resolve(answer.trim());
    });
  });
}

async function main() {
  const parsed = parseArgs();
  let prompt = parsed.prompt;

  console.log("Console Agent Engine — Interactive Demo");

  if (!prompt) {
    prompt = await promptUserInteractive();
  }

  if (!prompt) {
    console.log("No prompt provided. Exiting.");
    process.exit(0);
  }

  const cwd = process.cwd();
  const sessionStorage = new SqliteSessionStorage();

  // Create persistent session
  const header = sessionStorage.createSession({
    title: prompt.slice(0, 30),
    cwd,
    modelId: parsed.modelId,
    provider: parsed.provider,
  });

  console.log(`Session ID: ${header.id}`);
  console.log(`Provider: ${parsed.provider}`);
  console.log(`Model: ${parsed.modelId}`);
  console.log(`Workspace CWD: ${cwd}`);
  console.log(`User Prompt: "${prompt}"\n`);

  const model: Model = {
    id: parsed.modelId,
    provider: parsed.provider,
    contextWindow: 1_000_000,
  };

  const streamFn = parsed.provider === "gemini" ? geminiStreamFn : createAntigravityStreamFn();

  const { systemPrompt } = await buildSystemPrompt({
    cwd,
    model: parsed.modelId,
    approvalMode: "accept-edits",
  });

  const agent = new Agent({
    model,
    tools: [...allTools] as any,
    systemPrompt,
    streamFn,
    approvalMode: "accept-edits",
  });

  console.log("Streaming Agent Execution:\n");

  try {
    const eventStream = agent.run(prompt);

    for await (const event of eventStream as AsyncIterable<AgentSessionEvent>) {
      switch (event.type) {
        case "modelStreamPart":
          if (event.part.text) {
            process.stdout.write(event.part.text);
          }
          if (event.part.thinking) {
            process.stdout.write(`\x1b[36m${event.part.thinking}\x1b[0m`);
          }
          break;

        case "toolExecutionStart":
          console.log(`\n\n\x1b[33mExecuting Tools (${event.calls.length}):\x1b[0m`);
          for (const call of event.calls) {
            console.log(`  > [${call.name}] args: ${JSON.stringify(call.arguments)}`);
          }
          break;

        case "toolExecutionResult":
          console.log(
            `\x1b[32m  [Tool ${event.result.toolName || event.result.toolCallId.slice(0, 8)}] Result:\x1b[0m ${
              typeof event.result.content === "string"
                ? event.result.content.slice(0, 120).replace(/\n/g, " ")
                : "Done"
            }...`,
          );
          break;

        case "compaction":
          console.log(`\n\x1b[35mContext compacted (${event.summary})\x1b[0m`);
          break;

        case "error":
          console.error(`\n\x1b[31mError: ${event.error.message}\x1b[0m`);
          break;
      }
    }

    const finalMessages = await eventStream.result();
    sessionStorage.appendMessages(header.id, finalMessages);

    console.log(`\nRun Finished. Total messages stored: ${finalMessages.length}`);
    console.log(`Saved to SQLite Session ID: ${header.id}`);
  } catch (err) {
    console.error("Execution failed:", err);
  }
}

main();
