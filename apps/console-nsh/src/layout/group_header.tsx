// A section header for the sidebar's chat list, ported from console-ui's
// `layout/sidebar/group_header.rs` (the label, the count, and the
// date/project sort toggle; the rename affordance stays GPUI-side until the
// context menu lands).

import { Text, View } from "nsh";

export type SortMode = "date" | "project";

/// Toggles between date and project sectioning for the chat list. Shows the
/// current mode label; pressing switches to the other axis.
export function SortModeToggle(props: {
  mode: SortMode;
  onToggle: () => void;
}) {
  const label = props.mode === "date" ? "Date" : "Project";
  return (
    <View
      class="h-5 flex-row items-center gap-1 rounded-md px-1.5 active:bg-muted"
      onPress={props.onToggle}
      role="button"
    >
      <Text class="text-[11px] font-semibold text-muted-foreground">
        {label}
      </Text>
    </View>
  );
}

export function GroupHeader(props: { label: string; count?: number }) {
  return (
    <View class="flex-row items-center justify-between px-3 py-1">
      <Text class="text-xs text-muted-foreground">{props.label}</Text>
      {props.count !== undefined && props.count > 0 ? (
        <Text class="text-xs text-muted-foreground">{props.count}</Text>
      ) : null}
    </View>
  );
}
