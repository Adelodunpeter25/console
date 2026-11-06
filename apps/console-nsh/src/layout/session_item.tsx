// One chat row of the sidebar, ported from console-ui's
// `layout/sidebar/session_item.rs`. Title, the project it falls back to when
// the list is unfiltered, a draft marker, and the time ago. The context
// menu (pin, settle, rename, delete) arrives with the primitives pass.

import { Text, View } from "nsh";
import { formatTimeAgo } from "../utils/time";

export function SessionItem(props: {
  title: string;
  updatedAt: number;
  active: boolean;
  hasDraft?: boolean;
  projectName?: string | null;
  onPress?: () => void;
}) {
  return (
    <View
      class={
        "flex-row items-center gap-2 rounded-md px-3 py-1.5 " +
        (props.active ? "bg-muted" : "active:bg-muted")
      }
      onPress={props.onPress}
      role="button"
    >
      <View class="flex-1 flex-col">
        <Text class={"text-sm text-foreground" + (props.active ? " font-medium" : "")}>
          {props.title}
        </Text>
        {props.projectName ? (
          <Text class="text-xs text-muted-foreground">{props.projectName}</Text>
        ) : null}
      </View>
      {props.hasDraft ? (
        <View class="h-1.5 w-1.5 rounded-full bg-primary" />
      ) : null}
      <Text class="text-xs text-muted-foreground">
        {formatTimeAgo(props.updatedAt)}
      </Text>
    </View>
  );
}
