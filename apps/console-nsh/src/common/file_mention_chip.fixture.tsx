// Fixture for the file-mention chip: one file mention, one chat mention and
// one skill mention side by side.
import { Text, View, mount } from "nsh";
import { FileMentionChip } from "./file_mention_chip";

function Fixture() {
  return (
    <View class="w-full flex-col gap-2 p-2">
      <FileMentionChip path="src/main.rs" label="main.rs" />
      <FileMentionChip path="chat:alpha" label="$alpha" />
      <FileMentionChip path="skill:review" label="/review" />
      <Text>done</Text>
    </View>
  );
}

mount(Fixture);
