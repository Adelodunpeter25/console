// The transcript test fixture: a `createTranscriptState()` over on-screen
// scenario buttons, so the tests drive the streaming lifecycle and observe
// the rendered rows.
import { signal } from "nsh";
import { Text, View, mount } from "nsh";
import { TranscriptView, createTranscriptState } from "./transcript_view";
import type { AgentMessage } from "./transcript_view";

const state = createTranscriptState();
const jumped = signal<number | null>(null);
const forked = signal<string | null>(null);
const cited = signal<string | null>(null);

const ScenarioButton = (props: { label: string; onPress: () => void }) => (
  <View
    class="rounded border border-border p-1"
    role="button"
    accessibilityLabel={props.label}
    onPress={props.onPress}
  >
    <Text>{props.label}</Text>
  </View>
);

const user = (content: string, id?: string): AgentMessage => ({
  type: "user",
  id,
  content,
  createdAt: 1760000000,
});

const call = (id: string) => ({
  id,
  name: "readFile",
  label: "Reading file",
  kindFlags: 4 | 8,
  arguments: { path: "src/main.rs" },
});

function Fixture() {
  return (
    <View class="w-full flex-col gap-2 p-1">
      <View class="w-full flex-row flex-wrap gap-1">
        <ScenarioButton
          label="Add user message"
          onPress={() => state.pushMessage(user("What is this repo?"))}
        />
        <ScenarioButton
          label="Begin streaming"
          onPress={() => state.beginStreaming()}
        />
        <ScenarioButton
          label="Finish streaming"
          onPress={() => state.finishStreaming()}
        />
        <ScenarioButton
          label="Stream text"
          onPress={() => state.appendAssistantText("Working on the answer.")}
        />
        <ScenarioButton
          label="Stream tool call"
          onPress={() =>
            state.upsertAssistantToolCall(call("c1"))
          }
        />
        <ScenarioButton
          label="Add tool result"
          onPress={() =>
            state.appendToolResults([
              { toolCallId: "c1", content: "fn main() {}", isError: false },
            ])
          }
        />
        <ScenarioButton
          label="Load session"
          onPress={() =>
            state.setMessages([
              user("First question", "u1"),
              {
                type: "assistant",
                id: "a1",
                content: [{ type: "text", text: "First answer." }],
              },
              {
                type: "toolResult",
                results: [{ toolCallId: "transport", content: "" }],
              },
              user("Second question", "u2"),
              {
                type: "assistant",
                id: "a2",
                content: [{ type: "text", text: "Second answer." }],
              },
            ])
          }
        />
        <ScenarioButton
          label="Set prompts"
          onPress={() =>
            state.setPrompts([
              { id: "p1", prompt: "Oldest prompt", createdAt: 1760000000 },
              { id: "p2", prompt: "Newest prompt", createdAt: 1760000000 },
            ])
          }
        />
      </View>
      <View class="h-[320px] w-full">
        <TranscriptView
          state={state}
          onFork={(id) => forked.set(id)}
          onCite={(text) => cited.set(text)}
          onJumpToPrompt={(fromEnd) => jumped.set(fromEnd)}
          onRequestPrompts={() => state.setPrompts(state.prompts())}
        />
      </View>
      <Text>Jumped: {jumped() == null ? "none" : String(jumped())}</Text>
      <Text>Forked: {forked() ?? "none"}</Text>
      <Text>Cited: {cited() ?? "none"}</Text>
    </View>
  );
}

mount(Fixture);
