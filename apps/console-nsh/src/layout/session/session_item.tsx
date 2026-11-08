// One chat card of the sidebar, ported from console-ui's
// `layout/sidebar/session_item.rs`: the title with its draft marker, the
// hover-revealed settle button, and the folder + time-ago footer. A
// right-click (or held finger) opens the row's context menu.

import { Text, View, token } from "nsh";
import { Icon } from "../../common/icons";
import { Hint } from "../../common/tooltip";
import { formatTimeAgo } from "../../utils/time";
import { SessionContextMenu } from "./session_context_menu";

export function SessionItem(props: {
  title: string;
  updatedAt: number;
  active: boolean;
  /** The chat is settled (archived): dimmed, and the hover button un-settles. */
  settled?: boolean;
  /** The title is still the server's autotitle placeholder. */
  titlePending?: boolean;
  pinned?: boolean;
  hasDraft?: boolean;
  /** The project's name; `null` shows the desktop's "General" fallback. */
  projectName?: string | null;
  onPress?: () => void;
  onToggleSettle?: () => void;
  onOpenInNewWindow?: () => void;
  onTogglePin?: () => void;
  onRename?: () => void;
  onDelete?: () => void;
}) {
  const settled = () => props.settled ?? false;
  const row = (
    <View
      class={
        "group flex-col justify-between gap-1.5 rounded-md px-3 py-1.5 " +
        (props.active || settled() ? "bg-muted " : "hover:bg-muted ") +
        (settled() ? "opacity-70" : "")
      }
      onPress={props.onPress}
      role="button"
    >
      <View class="flex-row items-center justify-between gap-1.5 overflow-hidden">
        <View class="flex-1 flex-row items-center gap-1.5 overflow-hidden">
          <Text
            class={
              "truncate text-[13.5px] " +
              (props.titlePending
                ? "italic text-muted-foreground"
                : props.active
                  ? "font-semibold text-foreground"
                  : "font-medium text-foreground")
            }
          >
            {props.titlePending ? "Generating title…" : props.title}
          </Text>
          {props.hasDraft ? (
            <View class="flex-none rounded-sm bg-secondary px-1 py-0.5">
              <Text class="text-[10px] font-medium text-primary">Draft</Text>
            </View>
          ) : null}
        </View>
        {props.onToggleSettle ? (
          <View
            class={
              "rounded p-1 opacity-0 group-hover:opacity-100 hover:bg-muted" +
              (settled() ? " opacity-100" : "")
            }
            onPress={() => props.onToggleSettle?.()}
            accessibilityLabel={settled() ? "Move back to active" : "Settle chat"}
            tooltip={
              <Hint>{settled() ? "Move back to active" : "Settle chat"}</Hint>
            }
            role="button"
          >
            <Icon
              name={settled() ? "ArrowUp" : "Check"}
              size={14}
              color={token("foreground")}
            />
          </View>
        ) : null}
      </View>
      <View class="flex-row items-center justify-between gap-1">
        <View class="flex-1 flex-row items-center gap-1 overflow-hidden">
          <Icon name="FolderClosed" size={11} color={token("muted-foreground")} />
          <Text class="truncate text-[11.5px] text-muted-foreground">
            {props.projectName ?? "General"}
          </Text>
        </View>
        <Text class="text-[11.5px] text-muted-foreground">
          {formatTimeAgo(props.updatedAt)}
        </Text>
      </View>
    </View>
  );
  const hasMenu = (): boolean =>
    props.onDelete != null ||
    props.onRename != null ||
    props.onTogglePin != null ||
    props.onOpenInNewWindow != null;
  return hasMenu() ? (
    <SessionContextMenu
      pinned={props.pinned}
      settled={settled()}
      onOpenInNewWindow={props.onOpenInNewWindow}
      onTogglePin={props.onTogglePin}
      onRename={props.onRename}
      onSettle={props.onToggleSettle}
      onUnsettle={props.onToggleSettle}
      onDelete={props.onDelete}
    >
      {row}
    </SessionContextMenu>
  ) : (
    row
  );
}
