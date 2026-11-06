// The interaction-card test fixture: a permission card and a question card
// over local signals, with the callbacks' results rendered on screen for the
// tests to observe.
import { signal } from "nsh";
import { mount, Text, View } from "nsh";
import {
  PermissionInteractionCard,
  QuestionInteractionCard,
} from "./interaction_card";

const approved = signal<string>("none");
const answered = signal<string>("none");
const selected = signal<string[]>([]);
const skipped = signal(false);
const isLast = signal(false);

function Fixture() {
  return (
    <View class="w-full flex-col gap-3 p-2">
      <View
        class="w-full flex-row items-center justify-center rounded border border-border p-1"
        role="button"
        accessibilityLabel="Toggle last"
        onPress={() => isLast.set(!isLast())}
      >
        <Text>Last question: {isLast() ? "yes" : "no"}</Text>
      </View>
      <PermissionInteractionCard
        request={{
          toolName: "editFile",
          args: { path: "src/main.rs" },
          reason: "Needs to **edit** a file",
        }}
        submitting={false}
        onApprove={(ok) => approved.set(ok ? "allow" : "deny")}
      />
      <QuestionInteractionCard
        request={{
          question: "Which database?",
          options: ["Postgres", "SQLite"],
          isMultiSelect: false,
        }}
        selected={() => selected()}
        isLast={() => isLast()}
        submitting={false}
        onAnswer={(answer) =>
          answered.set(JSON.stringify(answer))
        }
        onSelect={(option) =>
          selected.set(
            selected().includes(option)
              ? selected().filter((o) => o !== option)
              : [...selected(), option],
          )
        }
        onSkip={() => skipped.set(true)}
      />
      <Text>Approved: {approved()}</Text>
      <Text>Answer: {answered()}</Text>
      <Text>Skipped: {skipped() ? "yes" : "no"}</Text>
      <Text>Picked: {selected().join(", ") || "none"}</Text>
    </View>
  );
}

mount(Fixture);
