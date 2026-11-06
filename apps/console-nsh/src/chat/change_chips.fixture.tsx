// Fixture for the turn change chips: a turn of five files (so the row is
// foldable) over local signals, with the callbacks' effects rendered on
// screen for the tests to observe.
import {
  TurnChangeChips,
  type TurnChanges,
} from "./change_chips";
import { Text, View, mount, signal } from "nsh";

const expanded = signal(false);
const lastAction = signal("");

const TURN: TurnChanges = {
  userMessageId: "u1",
  files: [
    {
      path: "src/chat/app.ts",
      status: "modified",
      additions: 12,
      deletions: 3,
      diffText: "diff --git a/src/chat/app.ts b/src/chat/app.ts\n@@ -1,2 +1,2 @@\n-old line\n+new line",
    },
    { path: "src/chat/logo.png", status: "modified", additions: 0, deletions: 0, diffText: null },
    { path: "README.md", status: "modified", additions: 2, deletions: 1, diffText: " " },
    { path: "src/chat/gone.ts", status: "deleted", additions: 0, deletions: 9, diffText: null },
    { path: "notes.txt", status: "modified", additions: 1, deletions: 0, diffText: null },
  ],
};

function Fixture() {
  return (
    <View class="w-full flex-col gap-2 p-2">
      <TurnChangeChips
        turn={TURN}
        expanded={() => expanded()}
        onToggle={() => expanded.set(!expanded())}
        onOpen={(path) => lastAction.set(`open:${path}`)}
      />
      <Text>Expanded: {expanded() ? "yes" : "no"}</Text>
      <Text>Action: {lastAction() || "none"}</Text>
    </View>
  );
}

mount(Fixture);
