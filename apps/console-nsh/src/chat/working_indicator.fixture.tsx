// Fixture for the working indicator: a run that started 5 seconds ago.
import { WorkingIndicator } from "./working_indicator";
import { mount, View } from "nsh";

function Fixture() {
  return (
    <View class="h-32 w-96">
      <WorkingIndicator startedAt={Math.floor(Date.now() / 1000) - 5} />
    </View>
  );
}

mount(Fixture);
