// The tooltip test fixture: the sidebar's sort toggle, whose tooltip shows
// the other axis's label, over `group_header.fixture.state`-style signals.
import { SortModeToggle, type SortMode } from "../layout/sidebar/group_header";
import { Hint } from "./tooltip";
import { Text, View, mount, signal } from "nsh";

const mode = signal<SortMode>("date");

function Fixture() {
  return (
    <View class="h-64 w-96 justify-center gap-2 p-4">
      <SortModeToggle
        mode={mode()}
        onToggle={() => mode.set(mode() === "date" ? "project" : "date")}
      />
      <Text>elsewhere</Text>
    </View>
  );
}

mount(Fixture);
