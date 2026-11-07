// The run-event mapping (the desktop's `run.rs` event switch) as pure
// functions over the wire JSON, so the pane's streaming behavior is
// testable headless.

import { expect, test } from "nsh/test";
import { applyRunEvent, type ChatRunEvent } from "./run_events";
import { createTranscriptState } from "./transcript_state";

test("run_events maps stream parts onto the transcript state", () => {
  const state = createTranscriptState();
  const ev = (type: string, extra: Record<string, unknown> = {}): ChatRunEvent => ({
    type,
    ...extra,
  });

  expect(applyRunEvent(state, ev("turnStart"))).toBe(false);
  expect(state.isStreaming()).toBe(true);

  applyRunEvent(state, ev("modelStreamPart", { part: { text: "Hel" } }));
  applyRunEvent(state, ev("modelStreamPart", { part: { text: "lo" } }));
  applyRunEvent(state, ev("modelStreamPart", { part: { thinking: "hmm" } }));
  applyRunEvent(
    state,
    ev("modelStreamPart", { part: { toolCall: { id: "t1", name: "readFile" } } }),
  );

  const list = state.messages();
  expect(list.length).toBe(1);
  const assistant = list[0] as { type: string; content: { type: string; text?: string }[] };
  expect(assistant.type).toBe("assistant");
  expect(assistant.content.map((part) => part.type)).toEqual(["text", "thinking", "toolCall"]);
  expect(assistant.content[0]?.text).toBe("Hello");
});

test("run_events appends tool results and ends the run on sessionEnd", () => {
  const state = createTranscriptState();
  state.beginStreaming();
  const ended = applyRunEvent(state, {
    type: "toolExecutionResult",
    result: { toolCallId: "t1", content: btoa('{"ok":true}') },
  });
  expect(ended).toBe(false);
  expect(state.messages().some((m) => m.type === "toolResult")).toBe(true);

  expect(applyRunEvent(state, { type: "sessionEnd" })).toBe(true);
  expect(state.isStreaming()).toBe(false);
});
