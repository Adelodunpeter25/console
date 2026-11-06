// Fixture for the assistant message bubble over local signals, with
// scenario buttons (streaming, thinking) and the fork callback's result on
// screen.
import { signal } from "nsh";
import { Text, View, mount } from "nsh";
import { AssistantMessageBubble, type AssistantContentPart } from "./message_assistant";

const isStreaming = signal(false);
const thinkingExpanded = signal(new Set<string>());
const forked = signal<string | null>(null);

const parts: AssistantContentPart[] = [
  { type: "thinking", text: "Weigh the options first." },
  { type: "text", text: "Here is the answer, in full." },
];

function Fixture() {
  return (
    <View class="w-full flex-col gap-3 p-2">
      <View
        class="w-full flex-row"
        role="button"
        accessibilityLabel="Toggle streaming"
        onPress={() => isStreaming.set(!isStreaming())}
      >
        <Text>streaming: {isStreaming() ? "yes" : "no"}</Text>
      </View>
      <AssistantMessageBubble
        contentParts={parts}
        isStreaming={() => isStreaming()}
        createdAt={1760000000}
        tokPerSec={12.34}
        forkMessageId="assistant-7"
        onFork={(id) => forked.set(id)}
        thinkingExpanded={() => thinkingExpanded()}
        onThinkingToggle={(id, exp) => {
          const next = new Set(thinkingExpanded());
          if (exp) next.add(id);
          else next.delete(id);
          thinkingExpanded.set(next);
        }}
      />
      <Text>Forked: {forked() ?? "none"}</Text>
    </View>
  );
}

mount(Fixture);
