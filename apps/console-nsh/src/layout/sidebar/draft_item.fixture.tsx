// The draft row test fixture: one DraftItem wired to callbacks, with its
// state rendered on screen so the tests can observe it.
import { signal } from "nsh";
import { mount, Text, View } from "nsh";
import { DraftItem } from "./draft_item";

const pressed = signal("");
const deleted = signal("");

function Fixture() {
  return (
    <View class="h-48 w-72 flex-col gap-2 p-4">
      <DraftItem
        draft={{
          sessionId: "s1",
          title: "Half-written prompt",
          preview: "refactor the com",
          projectName: "console",
          updatedAt: 1_700_000_000_000,
        }}
        active={false}
        onSelect={(id) => pressed.set(id)}
        onDelete={() => deleted.set("s1")}
      />
      <DraftItem
        draft={{
          sessionId: null,
          title: "New chat",
          preview: "",
          projectName: null,
          updatedAt: 1_700_000_100_000,
        }}
        active={false}
        onNewChat={() => pressed.set("new")}
        onDelete={() => deleted.set("new")}
      />
      <Text class="text-xs">Pressed: {pressed() || "none"}</Text>
      <Text class="text-xs">Deleted: {deleted() || "none"}</Text>
    </View>
  );
}

mount(Fixture);
