// Popover content for the composer's pill row, ported from console-ui's
// `common/pill_popovers.rs`: the cards the tasks, subagents and shells
// pills open above themselves. The pill triggers live in `composer_pills`;
// this module only builds what shows inside the popover.

import { Scroll, Text, View, token } from "nsh";
import type { JSX } from "nsh";
import { Icon } from "./icons";

// The desktop's `theme.success` has no NSH palette token; green reads as the
// accent here, so success markers use `primary`.


/// One item of the session's task list (`TodoItem` of console-core).
export type TodoItem = {
  content: string;
  status: string;
};

/// One subagent of the session (`SubagentInfo` of console-core).
export type SubagentInfo = {
  subagentId: string;
  name: string;
  role: string;
  status: string;
};

/// One running detached shell (`ShellEntry`).
export type ShellEntry = {
  id: string;
  command: string;
};

function statusOf(item: TodoItem): string {
  return item.status.toLowerCase();
}
function isDone(item: TodoItem): boolean {
  return ["done", "completed", "complete"].includes(statusOf(item));
}
function isInProgress(item: TodoItem): boolean {
  return ["in_progress", "started", "running"].includes(statusOf(item));
}

/// Shared chrome for the pill popovers: width, radius, and the header row
/// with an icon-labelled title on the left and a counter on the right.
function PillPopoverCard(props: {
  icon: Parameters<typeof Icon>[0]["name"];
  title: string;
  counter: string;
  children: JSX.Element;
}) {
  return (
    <View class="h-[340px] max-h-[340px] w-[300px] flex-col overflow-hidden rounded-xl border border-border bg-popover">
      <View class="flex-row items-center justify-between border-b border-border px-2.5 py-2">
        <View class="flex-row items-center gap-1.5">
          <Icon name={props.icon} size={12} color={token("muted-foreground")} />
          <Text class="text-[10.5px] font-bold text-muted-foreground">
            {props.title}
          </Text>
        </View>
        <Text class="text-[10.5px] font-medium text-muted-foreground">
          {props.counter}
        </Text>
      </View>
      <Scroll class="max-h-[300px]" contentClass="flex flex-col gap-0.5 p-1">{props.children}</Scroll>
    </View>
  );
}

function PopoverRow(props: { testID?: string; children: JSX.Element }) {
  return (
    <View
      class="min-w-0 flex-row items-center gap-2 rounded-md px-1.5 py-1.5 hover:bg-muted"
      testID={props.testID}
    >
      {props.children}
    </View>
  );
}

/// The floating task list that opens above the tasks pill: one truncated
/// title per row with a status marker, capped so a long checklist scrolls.
export function TasksPopover(props: { items: TodoItem[] }) {
  const total = props.items.length;
  const completed = props.items.filter(isDone).length;
  return (
    <PillPopoverCard icon="Checklist" title="TASKS" counter={`${completed}/${total}`}>
      {props.items.map((item, index) => {
        const done = isDone(item);
        const inProgress = isInProgress(item);
        return (
          <PopoverRow testID={`task-row-${index}`}>
            <View
              class={
                "flex h-[15px] w-[15px] flex-none flex-row items-center justify-center rounded-full border " +
                (done
                  ? "border-primary bg-primary/20"
                  : inProgress
                    ? "border-primary bg-primary/15"
                    : "border-border")
              }
            >
              {done ? (
                <Icon name="Check" size={9} color={token("primary")} />
              ) : inProgress ? (
                <View class="h-[5px] w-[5px] rounded-full bg-primary" />
              ) : null}
            </View>
            <Text
              class={
                "flex-1 truncate text-[12.5px] " +
                (done ? "text-muted-foreground" : "text-foreground")
              }
            >
              {item.content}
            </Text>
          </PopoverRow>
        );
      })}
    </PillPopoverCard>
  );
}

/// The subagents popover: one truncated human-readable title per subagent,
/// its icon lit while the subagent is still running.
export function SubagentsPopover(props: { items: SubagentInfo[] }) {
  return (
    <PillPopoverCard icon="Bot" title="SUBAGENTS" counter={`${props.items.length}`}>
      {props.items.map((sub, index) => {
        const running = sub.status === "running";
        return (
          <PopoverRow testID={`subagent-row-${index}`}>
            <Icon
              name="Bot"
              size={13}
              color={running ? token("primary") : token("muted-foreground")}
            />
            <Text class="flex-1 truncate text-[12.5px] text-foreground">
              {/* The role is the human-readable name the sidebar list used
                  ("Plan checker"); fall back to the raw name then the id. */}
              {sub.role.trim() || sub.name.trim() || sub.subagentId}
            </Text>
          </PopoverRow>
        );
      })}
    </PillPopoverCard>
  );
}

/// The shells popover: one truncated command per running shell with a stop
/// button on the right.
export function ShellsPopover(props: {
  shells: ShellEntry[];
  onStop: (id: string) => void;
}) {
  return (
    <PillPopoverCard icon="TerminalSquare" title="SHELLS" counter={`${props.shells.length}`}>
      {props.shells.map((shell, index) => (
        <PopoverRow testID={`shell-row-${index}`}>
          <Icon name="TerminalSquare" size={13} color={token("muted-foreground")} />
          <Text class="flex-1 truncate text-[12.5px] text-foreground" style={{ fontFamily: "monospace" }}>
            {shell.command}
          </Text>
          <View
            class="flex h-6 w-6 flex-none flex-row items-center justify-center rounded-full bg-destructive/20 hover:bg-destructive/40"
            onPress={() => props.onStop(shell.id)}
            accessibilityLabel={`Stop ${shell.command}`}
            role="button"
          >
            <Icon name="Stop" size={11} color={token("destructive")} />
          </View>
        </PopoverRow>
      ))}
    </PillPopoverCard>
  );
}
