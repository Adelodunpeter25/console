// The session item test fixture: one SessionItem wired to
// `session_item.fixture.state`, with the item's state rendered on screen so
// the tests can observe it.
import { SessionItem } from "./session_item";
import { pressed, toggled } from "./session_item.fixture.state";
import { mount, Text, View } from "nsh";

function Fixture() {
  return (
    <View class="h-64 w-64 flex-col gap-2 p-4">
      <SessionItem
        title="Fix the login loop"
        updatedAt={1_700_000_000_000}
        active={false}
        hasDraft
        projectName={null}
        onPress={() => pressed.set("opened")}
        onToggleSettle={() => toggled.set("settled")}
      />
      <Text class="text-xs">Pressed: {pressed() || "none"}</Text>
      <Text class="text-xs">Toggled: {toggled() || "none"}</Text>
    </View>
  );
}

mount(Fixture);
