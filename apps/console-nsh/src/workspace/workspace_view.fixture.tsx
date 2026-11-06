// The workspace test fixture: a store over on-screen scenario buttons
// (open/split/close), so the tests drive the pane tree and observe the
// rendered tabs.
import { signal } from "nsh";
import { Text, View, mount } from "nsh";
import { createWorkspaceStore } from "./workspace_store";
import { WorkspaceView } from "./workspace_view";

const state = createWorkspaceStore();
const log = signal<string[]>([]);

const note = (what: string) => log.set([...log(), what]);

const ScenarioButton = (props: { label: string; onPress: () => void }) => (
  <View
    class="rounded border border-border p-1"
    role="button"
    accessibilityLabel={props.label}
    onPress={props.onPress}
  >
    <Text>{props.label}</Text>
  </View>
);

const chat = (sessionId: string) => ({
  type: "chat" as const,
  sessionId,
  title: `Chat ${sessionId}`,
});

function Fixture() {
  return (
    <View class="flex-1 flex-col gap-2 p-1">
      <View class="flex-row flex-wrap gap-1">
        <ScenarioButton
          label="Open chat s1"
          onPress={() => store_open("s1")}
        />
        <ScenarioButton
          label="Open chat s2"
          onPress={() => store_open("s2")}
        />
        <ScenarioButton label="Split active" onPress={() => state.splitPane(state.activePane().id, "horizontal")} />
        <ScenarioButton
          label="Close active tab"
          onPress={() =>
            state.closeTab(state.activePane().id, state.activePane().tabs[state.activePane().tabs.length - 1]!)
          }
        />
      </View>
      <View class="h-[200px] w-full border border-border">
        <WorkspaceView store={state} />
      </View>
      <Text>Panes: {(state.revision(), String(state.panes().length))}</Text>
      <Text>{log().join(" | ") || "none"}</Text>
    </View>
  );
}

function store_open(sessionId: string) {
  const pane = state.openTab(null, chat(sessionId));
  note(`opened ${sessionId} in ${pane}`);
}

mount(Fixture);
