// The floating row of status pills just above the composer, ported from
// console-ui's `common/composer_pills.rs`: changed files, active tasks,
// running subagents, and background shells. The row overlays the
// transcript's bottom edge, so chat scrolls behind it. Each pill is a
// compact summary that opens its popover; multiple kinds sit side by side.

import { Text, View, token } from "nsh";
import type { JSX } from "nsh";
import { Popover } from "nsh/base";
import { Icon, type IconName } from "./icons";
import {
  ShellsPopover,
  SubagentsPopover,
  TasksPopover,
  type ShellEntry,
  type SubagentInfo,
  type TodoItem,
} from "./pill_popovers";

/// The session's change totals (`ChangesSummary`).
export type ChangesSummary = {
  files: number;
  additions: number;
  deletions: number;
};

function Pill(props: {
  icon: IconName;
  text: string;
  running?: boolean;
  onPress?: () => void;
  accessibilityLabel?: string;
  children?: JSX.Element;
}) {
  return (
    <Popover
      placement="top"
      align="start"
      popupClass="rounded-xl"
      trigger={
        <View
          class="flex flex-none flex-row items-center gap-1.5 rounded-full bg-card px-2.5 py-1"
          onPress={props.onPress}
          accessibilityLabel={props.accessibilityLabel}
          role="button"
        >
          <Icon
            name={props.icon}
            size={13}
            color={props.running ? token("primary") : token("muted-foreground")}
          />
          <Text class="text-xs font-medium text-muted-foreground">
            {props.text}
          </Text>
        </View>
      }
    >
      {props.children}
    </Popover>
  );
}

/// The changed-files pill: icon, "N files changed", and the colored +N -N
/// totals. Clicking opens the changes review for the session.
function ChangesPill(props: {
  summary: ChangesSummary;
  onPress?: () => void;
}) {
  const s = props.summary;
  return (
    <View
      class="flex flex-none flex-row items-center gap-1.5 rounded-full bg-card px-2.5 py-1"
      onPress={props.onPress}
      accessibilityLabel="Open changes review"
      role="button"
    >
      <Icon name="FileDiff" size={13} color={token("muted-foreground")} />
      <Text class="text-xs font-medium text-muted-foreground">
        {s.files} file{s.files === 1 ? "" : "s"} changed
      </Text>
      {s.additions > 0 ? (
        <Text class="text-xs font-semibold text-primary">+{s.additions}</Text>
      ) : null}
      {s.deletions > 0 ? (
        <Text class="text-xs font-semibold text-destructive">
          -{s.deletions}
        </Text>
      ) : null}
    </View>
  );
}

export function ComposerPills(props: {
  tasks?: { completed: number; total: number; items: TodoItem[] };
  subagents?: { running: boolean; items: SubagentInfo[] };
  shells?: { shells: ShellEntry[]; onStop: (id: string) => void };
  changes?: { summary: ChangesSummary; onOpen: () => void };
}) {
  const hasAny =
    props.tasks || props.subagents || props.shells || props.changes;
  if (!hasAny) return null;
  return (
    <View class="absolute bottom-2 left-0 right-0 flex justify-center px-5">
      <View class="flex max-w-[768px] flex-row items-center gap-2">
        {props.changes ? (
          <ChangesPill
            summary={props.changes.summary}
            onPress={props.changes.onOpen}
          />
        ) : null}
        {props.tasks ? (
          <Pill
            icon="Checklist"
            text={`${props.tasks.completed}/${props.tasks.total} tasks`}
            accessibilityLabel="Tasks"
          >
            <TasksPopover items={props.tasks.items} />
          </Pill>
        ) : null}
        {props.subagents ? (
          <Pill
            icon="Bot"
            running={props.subagents.running}
            text={`${props.subagents.items.length} ${
              props.subagents.items.length === 1 ? "subagent" : "subagents"
            }`}
            accessibilityLabel="Subagents"
          >
            <SubagentsPopover items={props.subagents.items} />
          </Pill>
        ) : null}
        {props.shells ? (
          <Pill
            icon="TerminalSquare"
            running
            text={`${props.shells.shells.length} ${
              props.shells.shells.length === 1 ? "shell" : "shells"
            }`}
            accessibilityLabel="Shells"
          >
            <ShellsPopover
              shells={props.shells.shells}
              onStop={props.shells.onStop}
            />
          </Pill>
        ) : null}
      </View>
    </View>
  );
}
