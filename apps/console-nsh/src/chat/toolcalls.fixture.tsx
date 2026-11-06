// The toolcalls test fixture: one run's activity over local signals, with
// the events switched by on-screen buttons for the tests to observe.
import { signal } from "nsh";
import { mount, Text, View } from "nsh";
import {
  ToolCalls,
  type ActivityEvent,
  type ToolCallEntry,
} from "./toolcalls";

const working = signal(false);

const calls: ToolCallEntry[] = [
  {
    call: {
      id: "c1",
      name: "readFile",
      label: "Reading file",
      kindFlags: 4 | 8,
      arguments: { path: "src/main.rs" },
    },
    result: {
      toolCallId: "c1",
      content: { content: { text: "fn main() {}" } },
      isError: false,
    },
  },
  {
    call: {
      id: "c2",
      name: "bash",
      label: "Running shell",
      kindFlags: 0,
      arguments: { command: "cargo build" },
    },
    result: {
      toolCallId: "c2",
      content: "Compilation finished",
      isError: false,
    },
  },
  {
    call: {
      id: "c3",
      name: "editFile",
      label: "Editing file",
      kindFlags: 1 | 8,
      arguments: { path: "src/lib.rs", oldText: "a\nb", newText: "a\nc" },
    },
    result: null,
  },
];

const events: ActivityEvent[] = [
  { type: "thinking", id: "t1", text: "Inspect the build first" },
  { type: "text", id: "x1", text: "On it — checking the build." },
  ...calls.map((entry) => ({ type: "toolCall" as const, entry })),
];

function Fixture() {
  return (
    <View class="w-full flex-col gap-2 p-1">
      <View
        class="w-full flex-row items-center justify-center rounded border border-border p-1"
        role="button"
        accessibilityLabel="Toggle working"
        onPress={() => working.set(!working())}
      >
        <Text>Working: {working() ? "yes" : "no"}</Text>
      </View>
      <ToolCalls
        runId="run-1"
        events={events}
        working={() => working()}
        startedAt={Math.floor(Date.now() / 1000) - 8}
        elapsedSeconds={37}
      />
    </View>
  );
}

mount(Fixture);
