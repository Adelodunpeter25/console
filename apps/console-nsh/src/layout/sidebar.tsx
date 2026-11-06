// The project/session sidebar: the app's nav, reading real data from the
// Go server through console-core. Phase 2.1 (`console-ui/src/layout/sidebar/`
// is the GPUI original; drafts and the loading skeleton land with the rest
// of that surface).

import { Text, View } from "nsh";
import type { ProjectSummary, SessionSummary } from "console-nsh/native";
import { groupSessions } from "./session_groups";

export function Sidebar(props: {
  loading: boolean;
  projects: ProjectSummary[];
  selectedProjectId: string | null;
  sessions: SessionSummary[];
  sessionsLoading: boolean;
  onProject: (id: string) => void;
  onSession: (id: string) => void;
}) {
  return (
    <View class="flex w-56 shrink-0 flex-col border-r border-border bg-card">
      <View class="flex h-12 shrink-0 items-center px-4">
        <Text class="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          Projects
        </Text>
      </View>
      <View class="flex flex-col gap-0.5 px-2">
        {props.loading ? (
          <Text class="px-3 py-1.5 text-sm text-muted-foreground">
            Loading…
          </Text>
        ) : props.projects.length === 0 ? (
          <Text class="px-3 py-1.5 text-sm text-muted-foreground">
            No projects yet
          </Text>
        ) : (
          props.projects.map((p) => (
            <View
              class={
                "rounded-md px-3 py-1.5 " +
                (props.selectedProjectId === p.id ? "bg-muted" : "")
              }
              onPress={() => props.onProject(p.id)}
              role="button"
            >
              <Text
                class={
                  "text-sm " +
                  (props.selectedProjectId === p.id
                    ? "font-medium text-foreground"
                    : "text-muted-foreground")
                }
              >
                {p.name}
              </Text>
            </View>
          ))
        )}
      </View>

      <View class="mt-4 flex h-12 shrink-0 items-center px-4">
        <Text class="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          Chats
        </Text>
      </View>
      <View class="flex flex-1 flex-col gap-3 px-2 pb-3">
        {props.sessionsLoading ? (
          <Text class="px-3 py-1.5 text-sm text-muted-foreground">
            Loading…
          </Text>
        ) : props.sessions.length === 0 ? (
          <Text class="px-3 py-1.5 text-sm text-muted-foreground">
            No chats yet
          </Text>
        ) : (
          groupSessions(props.sessions).map((group) => (
            <View class="flex flex-col gap-0.5">
              <Text class="px-3 py-1 text-xs text-muted-foreground">
                {group.label}
              </Text>
              {group.sessions.map((s) => (
                <View
                  class="rounded-md px-3 py-1.5"
                  onPress={() => props.onSession(s.id)}
                  role="button"
                >
                  <Text
                    class="text-sm text-foreground"
                  >
                    {s.title}
                  </Text>
                </View>
              ))}
            </View>
          ))
        )}
      </View>
    </View>
  );
}
