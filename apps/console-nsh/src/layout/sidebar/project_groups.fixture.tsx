// The collapsible project sections' test fixture: two projects' chats over
// `session_groups.ts`, with the section state rendered on screen so the
// tests can observe it.
import { signal } from "nsh";
import { mount, Text, View } from "nsh";
import type { SessionSummary } from "console-nsh/native";
import { ProjectGroups } from "./project_groups";

const pressed = signal("");
const toggled = signal("");
const collapsed = signal<string[]>([]);

const sessions: SessionSummary[] = [
  {
    id: "s1",
    title: "Fix the login loop",
    status: "active", pinned: false, settled: false,
    updatedAt: 1_700_000_000_000,
    projectId: "p1",
  },
  {
    id: "s2",
    title: "Ship the sidebar",
    status: "active", pinned: false, settled: false,
    updatedAt: 1_700_000_100_000,
    projectId: "p2",
  },
  {
    id: "s3",
    title: "Loose ends",
    status: "active", pinned: false, settled: false,
    updatedAt: 1_700_000_200_000,
    projectId: null,
  },
];

function Fixture() {
  return (
    <View class="h-96 w-72 flex-col p-2">
      <ProjectGroups
        sessions={sessions}
        projectName={(id) => (id === "p1" ? "console" : id === "p2" ? "nsh" : null)}
        activeSessionId={null}
        collapsed={(label) => collapsed().includes(label)}
        onToggleProject={(label) =>
          collapsed().includes(label)
            ? collapsed.set(collapsed().filter((l) => l !== label))
            : collapsed.set([...collapsed(), label])
        }
        onSession={(id) => pressed.set(id)}
      />
      <Text class="text-xs">Toggled: {toggled() || "none"}</Text>
      <Text class="text-xs">Pressed: {pressed() || "none"}</Text>
    </View>
  );
}

mount(Fixture);
