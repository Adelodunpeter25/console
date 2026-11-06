// The project/session sidebar, composed from the ported pieces:
// `session_item`, `draft_item`, `group_header`, `sidebar_loading` (the GPUI
// originals live in `console-ui/src/layout/sidebar/`). Drafts are a
// frontend-only signal for now; their service moves over with the Rust pass.

import { Text, View } from "nsh";
import type { ProjectSummary, SessionSummary } from "console-nsh/native";
import { DraftItem, type DraftSummary } from "./draft_item";
import { GroupHeader, SortModeToggle, type SortMode } from "./group_header";
import { SessionItem } from "./session_item";
import { groupSessions } from "./session_groups";

export function Sidebar(props: {
  loading: boolean;
  projects: ProjectSummary[];
  selectedProjectId: string | null;
  sessions: SessionSummary[];
  sessionsLoading: boolean;
  drafts: DraftSummary[];
  activeSessionId: string | null;
  sortMode: SortMode;
  onProject: (id: string) => void;
  onSession: (id: string) => void;
  onSortMode: (mode: SortMode) => void;
  onNewChat?: () => void;
}) {
  const groups = groupSessions(props.sessions);
  return (
    <View class="flex w-56 shrink-0 flex-col border-r border-border bg-card">
      <View class="flex h-12 shrink-0 items-center justify-between px-4">
        <Text class="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          Projects
        </Text>
        <SortModeToggle
          mode={props.sortMode}
          onToggle={() =>
            props.onSortMode(props.sortMode === "date" ? "project" : "date")
          }
        />
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
      <View class="flex flex-1 flex-col px-2 pb-3">
        {props.sessionsLoading ? (
          <Text class="px-3 py-1.5 text-sm text-muted-foreground">
            Loading…
          </Text>
        ) : props.sessions.length === 0 ? (
          <Text class="px-3 py-1.5 text-sm text-muted-foreground">
            No chats yet
          </Text>
        ) : (
          groups.map((group) => (
            <View class="flex flex-col">
              <GroupHeader label={group.label} count={group.sessions.length} />
              {group.sessions.map((s) => (
                <SessionItem
                  title={s.title}
                  updatedAt={s.updatedAt}
                  active={props.activeSessionId === s.id}
                  onPress={() => props.onSession(s.id)}
                />
              ))}
            </View>
          ))
        )}

        {props.drafts.length > 0 ? (
          <View class="mt-3 flex flex-col">
            <GroupHeader label="Drafts" count={props.drafts.length} />
            {props.drafts.map((d) => (
              <DraftItem
                draft={d}
                active={
                  d.sessionId !== null && props.activeSessionId === d.sessionId
                }
                onSelect={props.onSession}
                onNewChat={props.onNewChat}
              />
            ))}
          </View>
        ) : null}
      </View>
    </View>
  );
}
