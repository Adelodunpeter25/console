// Fixture for the thinking block: the caller owns the collapsed state, so a
// local signal mirrors it, flipped by the block's onToggle and shown on
// screen for the tests.
import { ThinkingBlock } from "./thinking_block";
import { Text, View, mount, signal } from "nsh";

const collapsed = signal(true);
const text = "Let me trace the bug through the parser.";

function Fixture() {
  return (
    <View class="w-full flex-col gap-2 p-2">
      <ThinkingBlock
        id="t1"
        text={text}
        collapsed={() => collapsed()}
        onToggle={(expanded) => collapsed.set(!expanded)}
      />
      <Text>Collapsed: {collapsed() ? "yes" : "no"}</Text>
    </View>
  );
}

mount(Fixture);
