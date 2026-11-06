// The title bar test fixture: one TitleBar wired to callbacks, with its
// actions rendered on screen so the tests can observe them.
import { signal } from "nsh";
import { mount, Text, View } from "nsh";
import { TitleBar } from "./title_bar";

const pressed = signal("");

function Fixture() {
  return (
    <View class="flex-1 flex-col">
      <TitleBar
        title="Fix the login loop"
        detail="— console"
        sidebarWidth={224}
        onToggleSidebar={() => pressed.set("sidebar")}
      />
      <Text class="text-xs">Pressed: {pressed() || "none"}</Text>
    </View>
  );
}

mount(Fixture);
