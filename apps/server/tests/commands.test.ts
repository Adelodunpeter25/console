/**
 * Unit & Functional Tests for SlashCommandRegistry (Phase 6).
 * Runs 100% offline — zero LLM calls / 0 credit usage.
 */
import assert from "node:assert/strict";
import {
  Agent,
  SlashCommandRegistry,
  SqliteSessionStorage,
  type SlashCommandContext,
} from "@/agent/src/index.js";
import type { ProviderId } from "@console/types";

console.log("Running SlashCommandRegistry tests...");

const storage = new SqliteSessionStorage({ dbPath: ":memory:" });
const session = storage.createSession({
  cwd: process.cwd(),
  modelId: "gemini-3.1-pro-low",
  provider: "antigravity",
  title: "Initial Session",
});

let currentProvider: ProviderId = "antigravity";
let currentSessionId = session.id;

const agent = new Agent({
  model: { id: "gemini-3.1-pro-low", provider: "antigravity", contextWindow: 128_000 },
  tools: [],
  systemPrompt: "Test",
  streamFn: async function* () {},
});

const registry = new SlashCommandRegistry();

function buildContext(): SlashCommandContext {
  return {
    agent,
    sessionStorage: storage,
    currentSessionId,
    currentProvider,
    setProvider: (p) => {
      currentProvider = p;
    },
    setModel: (m) => {
      agent.setModel(m);
    },
    setCurrentSessionId: (id) => {
      currentSessionId = id;
    },
    discoveredCommands: [
      {
        name: "test-cmd",
        content: "Predefined test command text",
        path: "/test",
        level: "project",
        source: {} as any,
      },
    ],
    discoveredSkills: [
      {
        name: "code-review",
        path: "/skill.md",
        content: "Review code quality",
        level: "project",
        source: {} as any,
      },
    ],
  };
}

// 1. Test /help command
{
  const res = await registry.parseAndExecute("/help", buildContext());
  assert.equal(res.handled, true);
  assert.ok(res.message?.includes("/model"));
  assert.ok(res.message?.includes("/provider"));
  assert.ok(res.message?.includes("/test-cmd"));
  assert.ok(res.message?.includes("/code-review"));
  console.log("  ✅ /help command");
}

// 2. Test /model command
{
  // List models
  const listRes = await registry.parseAndExecute("/model", buildContext());
  assert.equal(listRes.handled, true);
  assert.ok(listRes.message?.includes("gemini-3.1-pro-low"));

  // Switch model
  const switchRes = await registry.parseAndExecute("/model claude-sonnet-4-6", buildContext());
  assert.equal(switchRes.handled, true);
  assert.equal(switchRes.action, "switch_model");
  assert.equal(agent.model.id, "claude-sonnet-4-6");

  const loaded = storage.loadSession(currentSessionId);
  assert.equal(loaded?.header.modelId, "claude-sonnet-4-6");
  console.log("  ✅ /model command (list & switch)");
}

// 3. Test /provider command
{
  const switchRes = await registry.parseAndExecute("/provider antigravity", buildContext());
  assert.equal(switchRes.handled, true);
  assert.equal(switchRes.action, "switch_provider");
  assert.equal(currentProvider, "antigravity");
  assert.equal(agent.model.provider, "antigravity");
  assert.equal(agent.model.id, "claude-opus-4-6-thinking");
  console.log("  ✅ /provider command (switch provider & default model)");
}

// 4. Test /rename command
{
  const renameRes = await registry.parseAndExecute(
    "/rename Refactored Session Title",
    buildContext(),
  );
  assert.equal(renameRes.handled, true);
  const loaded = storage.loadSession(currentSessionId);
  assert.equal(loaded?.header.title, "Refactored Session Title");
  console.log("  ✅ /rename command");
}

// 5. Test /new and /resume commands
{
  // Add message to current session
  agent.loadHistory([{ role: "user", content: "First turn" }]);
  assert.equal(agent.messages.length, 1);

  // /new starts a fresh session
  const newRes = await registry.parseAndExecute("/new", buildContext());
  assert.equal(newRes.handled, true);
  assert.equal(agent.messages.length, 0);
  assert.notEqual(currentSessionId, session.id);

  // List recent sessions with /resume
  const listResumeRes = await registry.parseAndExecute("/resume", buildContext());
  assert.equal(listResumeRes.handled, true);
  assert.ok(listResumeRes.message?.includes(session.id));

  // Resume old session
  const resumeRes = await registry.parseAndExecute(`/resume ${session.id}`, buildContext());
  assert.equal(resumeRes.handled, true);
  assert.equal(currentSessionId, session.id);
  console.log("  ✅ /new & /resume commands");
}

// 6. Test /compact command
{
  agent.loadHistory([
    { role: "user", content: "Prompt A" },
    {
      role: "assistant",
      id: "1",
      content: [{ type: "text", text: "Answer A" }],
      stopReason: "stop",
    },
    { role: "user", content: "Prompt B" },
    {
      role: "assistant",
      id: "2",
      content: [{ type: "text", text: "Answer B" }],
      stopReason: "stop",
    },
  ]);

  const compactRes = await registry.parseAndExecute("/compact", buildContext());
  assert.equal(compactRes.handled, true);
  assert.equal(compactRes.action, "compact");
  assert.equal(agent.messages.length, 2);
  console.log("  ✅ /compact command");
}

// 7. Test custom slash commands & skills
{
  const customRes = await registry.parseAndExecute("/test-cmd extra args", buildContext());
  assert.equal(customRes.handled, true);
  assert.equal(customRes.action, "custom_prompt");
  assert.ok(customRes.customPromptText?.includes("Predefined test command text"));
  assert.ok(customRes.customPromptText?.includes("extra args"));

  const skillRes = await registry.parseAndExecute("/code-review", buildContext());
  assert.equal(skillRes.handled, true);
  assert.equal(skillRes.action, "custom_prompt");
  assert.ok(skillRes.customPromptText?.includes("Review code quality"));
  console.log("  ✅ Custom discovered slash commands & skills");
}

storage.close();
console.log("SlashCommandRegistry tests passed!\n");
