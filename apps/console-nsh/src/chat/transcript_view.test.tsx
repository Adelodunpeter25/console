// Tests for the transcript view, beside `transcript_view.tsx`. The runner
// opens `transcript_view.fixture.tsx`, whose buttons drive the streaming
// lifecycle over a `createTranscriptState()`.
import { expect, open, test } from "nsh/test";
import {
  groupTurnChanges,
  isHiddenToolTransport,
  toolActivityForUser,
  turnChangesForAssistant,
  type AgentMessage,
} from "./transcript_view";
import type { SessionFileChange } from "./transcript_view";

const user = (content: string, id?: string): AgentMessage => ({
  type: "user",
  id,
  content,
});

const assistantWithCall = (id: string, callId: string): AgentMessage => ({
  type: "assistant",
  id,
  content: [
    { type: "thinking", text: "plan it" },
    { type: "toolCall", call: { id: callId, name: "readFile", arguments: {} } },
    { type: "text", text: "progress note" },
  ],
});

test("transcript tool-result messages and call-carrying rows are transport", () => {
  expect(isHiddenToolTransport({ type: "toolResult", results: [] })).toBe(true);
  expect(isHiddenToolTransport(assistantWithCall("a1", "c1"))).toBe(true);
  expect(isHiddenToolTransport(user("hi"))).toBe(false);
});

test("run activity joins results to calls and stops at the next user message", () => {
  const messages: AgentMessage[] = [
    user("q1"),
    assistantWithCall("a1", "c1"),
    { type: "toolResult", results: [{ toolCallId: "c1", content: "done" }] },
    user("q2"),
    { type: "assistant", content: [{ type: "text", text: "final" }] },
  ];
  const activity = toolActivityForUser(messages, 0, false, null)!;
  expect(activity.events.length).toBe(3);
  expect(
    activity.events.find((e) => e.type === "toolCall")?.entry.result?.content,
  ).toBe("done");
  // Not working: the run closed before the second user message.
  expect(activity.working).toBe(false);
  // A run with no tool call yields no activity block.
  expect(toolActivityForUser(messages, 1, false, null)).toBeUndefined();
});

test("turn changes group by user message and land on the turn's last row", () => {
  const changes: SessionFileChange[] = [
    {
      path: "src/a.rs",
      status: "modified",
      additions: 1,
      deletions: 0,
      diffText: null,
      userMessageId: "u1",
    },
    {
      path: "src/b.rs",
      status: "added",
      additions: 2,
      deletions: 0,
      diffText: null,
      userMessageId: "u1",
    },
  ];
  const turns = groupTurnChanges(changes);
  expect(turns.get("u1")?.files.length).toBe(2);
  const messages: AgentMessage[] = [
    user("q", "u1"),
    assistantWithCall("a1", "c1"),
    { type: "assistant", content: [{ type: "text", text: "done" }] },
  ];
  // The hidden transport row does not stop this message from closing the
  // turn; the middle call-carrying row gets nothing.
  expect(turnChangesForAssistant(messages, 2, turns)?.userMessageId).toBe("u1");
  expect(turnChangesForAssistant(messages, 1, turns)).toBeUndefined();
});

test("the empty transcript shows the empty state, not rows", async () => {
  const app = await open("./transcript_view.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("Start a conversation");
});

test("messages render as rows and the working indicator trails streaming", async () => {
  const app = await open("./transcript_view.fixture.tsx", { platform: "linux" });
  await app.press("label:Load session");
  expect(app.texts()).toContain("First question");
  expect(app.texts()).toContain("Second answer.");
  // Tool-result transport rows never render on their own.
  expect(app.texts().some((t) => t.includes("transport"))).toBe(false);
  // The last user's run has no tool calls yet: streaming shows the
  // trailing working row.
  await app.press("label:Begin streaming");
  expect(app.texts().some((t) => /^Working for /.test(t))).toBe(true);
  await app.press("label:Finish streaming");
  expect(app.texts().some((t) => /^Working for /.test(t))).toBe(false);
});

test("a tool-carrying run renders as grouped activity under its user row", async () => {
  const app = await open("./transcript_view.fixture.tsx", { platform: "linux" });
  await app.press("label:Add user message");
  await app.press("label:Stream tool call");
  await app.press("label:Add tool result");
  // The run's activity block, not a trailing working indicator: the run
  // carries a tool call, so the activity is the live element. Settled, its
  // summary is the "Worked for" row (collapsed by default).
  await app.press("label:Begin streaming");
  expect(
    app.texts().filter((t) => /^Working for /.test(t)).length,
  ).toBe(1);
  await app.press("label:Stream text");
  await app.press("label:Finish streaming");
  await app.press("label:Run run-0");
  expect(app.texts()).toContain("Reading file");
  // Streamed text enters the run timeline when the turn has tool calls.
  expect(app.texts()).toContain("Working on the answer.");
});

test("the jump-to-prompt popup lists prompts and jumps", async () => {
  const app = await open("./transcript_view.fixture.tsx", { platform: "linux" });
  await app.press("label:Add user message");
  await app.press("label:Set prompts");
  // The pill is hover-revealed, like the sidebar's settle button.
  await app.hover("text:What is this repo?");
  await app.press("label:Jump to prompt");
  expect(app.texts()).toContain("Newest prompt");
  await app.press("label:Jump to prompt p1");
  expect(app.texts()).toContain("Jumped: 1");
});
