// The thinking stepper test fixture: `ThinkingStepper` cycling a level list
// in-app, with the current level rendered on screen for the tests to
// observe.
import { ThinkingStepper } from "./thinking_stepper";
import type { ThinkingLevel } from "./composer";
import { Text, View, mount, signal } from "nsh";

const LEVELS: ThinkingLevel[] = ["None", "Low", "Medium", "High"];
const level = signal<ThinkingLevel | null>("Low");

function Fixture() {
  return (
    <View class="h-32 w-72 items-start p-4">
      <ThinkingStepper
        currentLevel={() => level()}
        supportedLevels={LEVELS}
        onCycle={() => {
          const supported = LEVELS;
          const index = supported.indexOf(level() ?? "None");
          level.set(supported[(index + 1) % supported.length]);
        }}
      />
      <Text>Level: {level() ?? "none"}</Text>
    </View>
  );
}

mount(Fixture);
