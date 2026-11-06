// Fixture for the user message bubble: a long foldable prompt with a
// context-file mention, scenario button for the fold, callbacks' results on
// screen.
import { signal } from "nsh";
import { Text, View, mount } from "nsh";
import { UserMessageBubble } from "./message_user";

const expanded = signal(false);

const longPrompt = Array.from(
  { length: 12 },
  (_, i) => `Line ${i + 1} of a long prompt that keeps going.`,
).join("\n");

function Fixture() {
  return (
    <View class="w-full flex-col gap-3 p-2">
      <View
        class="w-full flex-row"
        role="button"
        accessibilityLabel="Toggle expand"
        onPress={() => expanded.set(!expanded())}
      >
        <Text>expanded: {expanded() ? "yes" : "no"}</Text>
      </View>
      <UserMessageBubble
        content={longPrompt + "\nlook at src/main.rs"}
        contextFiles={["src/main.rs"]}
        createdAt={1760000000}
        expanded={() => expanded()}
        onToggleExpand={() => expanded.set(!expanded())}
      />
    </View>
  );
}

mount(Fixture);
