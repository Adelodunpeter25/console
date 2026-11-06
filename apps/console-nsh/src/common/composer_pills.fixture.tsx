// The pills row test fixture: `ComposerPills` over
// `composer_pills.fixture.state`, with the row's callbacks rendered on
// screen for the tests to observe.
import { ComposerPills, type ChangesSummary } from "./composer_pills";
import { openedChanges, stoppedShell } from "./composer_pills.fixture.state";
import { mount, Text, View } from "nsh";

const changes: ChangesSummary = { files: 3, additions: 12, deletions: 4 };

function Fixture() {
  return (
    <View class="h-64 w-96 flex-col p-4">
      <View class="flex-1" />
      <ComposerPills
        changes={{ summary: changes, onOpen: () => openedChanges.set("yes") }}
        tasks={{
          completed: 1,
          total: 3,
          items: [
            { content: "Write the failing test", status: "done" },
            { content: "Make it pass", status: "in_progress" },
            { content: "Ship it", status: "todo" },
          ],
        }}
        subagents={{
          running: true,
          items: [{ subagentId: "s1", name: "plan-checker", role: "Plan checker", status: "running" }],
        }}
        shells={{
          shells: [{ id: "sh1", command: "make dev-server" }],
          onStop: (id) => stoppedShell.set(id),
        }}
      />
      <Text class="text-xs">Changes opened: {openedChanges() || "no"}</Text>
      <Text class="text-xs">Stopped: {stoppedShell() || "none"}</Text>
    </View>
  );
}

mount(Fixture);
