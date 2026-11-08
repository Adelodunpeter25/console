// The sidebar, ported from console-ui's `layout/sidebar/mod.rs`: the New
// Task and Search action rows on top, the first section's header pinned
// beside the sort toggle, the grouped chat list scrolling under it, and the
// "No tasks yet" empty state. Projects appear as sections (project sort
// mode), not as a picker; the settings footer waits for the settings screen.

import { Scroll, signal, View, Text, token } from "nsh";
import type { ProjectSummary, SessionSummary } from "console-nsh/native";
import { GroupHeader, SortModeToggle, type SortMode } from "./group_header";
import { SessionItem } from "../session/session_item";
import {
  groupSessions,
  groupSessionsByProject,
  partitionSettled,
  sentenceCaseProjectName,
} from "../session/session_groups";
import { Icon } from "../../common/icons";
import { Hint } from "../../common/tooltip";

function ActionRow(props: {
  icon: Parameters<typeof Icon>[0]["name"];
  label: string;
  badge?: string;
  onPress?: () => void;
}) {
  return (
    <View
      class="h-8 flex-row items-center gap-2.5 rounded-[7px] px-1.5 hover:bg-muted active:bg-muted"
      onPress={props.onPress}
      role="button"
      accessibilityLabel={props.label}
    >
      <View class="h-5 w-5 flex-none flex-row items-center justify-center">
        <Icon name={props.icon} size={16} color={token("muted-foreground")} />
      </View>
      <Text class="flex-1 text-[13px] font-medium text-muted-foreground">
        {props.label}
      </Text>
      {props.badge ? (
        <View class="rounded-sm bg-popover px-1 py-0.5">
          <Text class="text-[9.5px] text-muted-foreground">{props.badge}</Text>
        </View>
      ) : null}
    </View>
  );
}

const SETTLED_LABEL = "Settled";
const sentenceCase = (name: string | null) =>
  name == null ? null : sentenceCaseProjectName(name);

export function Sidebar(props: {
  projects: ProjectSummary[];
  sessions: SessionSummary[];
  sessionsLoading: boolean;
  sessionsError?: string | null;
  activeSessionId: string | null;
  sortMode: SortMode;
  onSession: (id: string) => void;
  onSortMode: (mode: SortMode) => void;
  onNewChat?: () => void;
  onSearch?: () => void;
  onToggleSettle?: (id: string) => void;
  onTogglePin?: (id: string) => void;
  onDeleteSession?: (id: string) => void;
}) {
  // Collapsed section labels; the GPUI app keeps these in the app state,
  // here they live with the sidebar until a second consumer needs them.
  // The Settled section starts collapsed, like the desktop.
  const collapsedGroups = signal<string[]>([SETTLED_LABEL]);
  const split = () => partitionSettled(props.sessions);
  const projectNameById = () =>
    new Map(props.projects.map((p) => [p.id, p.name] as const));
  const groups = () =>
    props.sortMode === "project"
      ? groupSessionsByProject(split().active, (id) =>
          projectNameById().get(id) ?? null,
        )
      : groupSessions(split().active);
  const collapsed = (label: string) => collapsedGroups().includes(label);
  const toggleGroup = (label: string) =>
    collapsedGroups().includes(label)
      ? collapsedGroups.set(collapsedGroups().filter((l) => l !== label))
      : collapsedGroups.set([...collapsedGroups(), label]);

  const renderSession = (s: SessionSummary) => (
    <SessionItem
      title={s.title}
      updatedAt={s.updatedAt}
      active={props.activeSessionId === s.id}
      settled={s.settled}
      pinned={s.pinned}
      projectName={
        s.projectId
          ? sentenceCase(projectNameById().get(s.projectId) ?? null)
          : null
      }
      onToggleSettle={
        props.onToggleSettle ? () => props.onToggleSettle?.(s.id) : undefined
      }
      onTogglePin={props.onTogglePin ? () => props.onTogglePin?.(s.id) : undefined}
      onDelete={
        props.onDeleteSession ? () => props.onDeleteSession?.(s.id) : undefined
      }
      onPress={() => props.onSession(s.id)}
    />
  );

  return (
    <View class="flex h-full w-full flex-col bg-card px-3">
      {/* Action rows (New Task & Search) — the unified title bar owns the
          sidebar toggle, so the sidebar starts here, as on desktop. */}
      <View class="flex flex-col gap-1 pb-2 pt-1">
        <ActionRow icon="Compose" label="New Task" onPress={props.onNewChat} />
        <ActionRow icon="Search" label="Search" badge="⌘K" onPress={props.onSearch} />
      </View>

      {props.sessionsLoading ? (
        <Text class="px-3 py-1.5 text-sm text-muted-foreground">Loading…</Text>
      ) : props.sessionsError ? (
        <Text class="px-3 py-1.5 text-sm text-destructive">
          Couldn’t load chats
        </Text>
      ) : props.sessions.length === 0 ? (
        <View class="flex flex-1 flex-col items-center justify-center gap-1.5">
          <Icon name="Folder" size={20} color={token("muted-foreground")} />
          <Text class="text-xs text-muted-foreground">No tasks yet</Text>
        </View>
      ) : (
        <View class="flex flex-1 flex-col overflow-hidden">
          {/* The first section's header stays pinned beside the sort
              toggle; the rest scroll. */}
          <View class="flex flex-row items-center justify-between">
            {groups()[0] ? (
              <GroupHeader
                label={groups()[0].label}
                collapsed={collapsed(groups()[0].label)}
                onToggle={() => toggleGroup(groups()[0].label)}
              />
            ) : (
              <View />
            )}
            <SortModeToggle
              mode={props.sortMode}
              onToggle={() =>
                props.onSortMode(props.sortMode === "date" ? "project" : "date")
              }
            />
          </View>
          <Scroll class="flex-1" contentClass="flex flex-col pb-3">
            {groups()[0] && !collapsed(groups()[0].label)
              ? groups()[0].sessions.map(renderSession)
              : null}
            {groups().slice(1).map((group) => (
              <View class="flex flex-col">
                <GroupHeader
                  label={group.label}
                  collapsed={collapsed(group.label)}
                  onToggle={() => toggleGroup(group.label)}
                />
                {collapsed(group.label)
                  ? null
                  : group.sessions.map(renderSession)}
              </View>
            ))}
            {split().settled.length > 0 ? (
              <View class="flex flex-col">
                <GroupHeader
                  label={SETTLED_LABEL}
                  collapsed={collapsed(SETTLED_LABEL)}
                  onToggle={() => toggleGroup(SETTLED_LABEL)}
                />
                {collapsed(SETTLED_LABEL)
                  ? null
                  : split().settled.map(renderSession)}
              </View>
            ) : null}
          </Scroll>
        </View>
      )}
    </View>
  );
}
