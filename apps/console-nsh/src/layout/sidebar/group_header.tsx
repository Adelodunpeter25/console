// Section headers for the sidebar's chat list, ported from console-ui's
// `layout/sidebar/group_header.rs`: the collapsible 28px header (a 14.5px
// semibold label whose chevron is revealed on hover) and the date/project
// sort toggle. The rename affordance stays GPUI-side until the context menu
// lands.

import { Text, View, token } from "nsh";
import { Hint } from "../../common/tooltip";
import { Icon } from "../../common/icons";

export type SortMode = "date" | "project";

/// Toggles between date and project sectioning for the chat list. Shows the
/// current mode label; pressing switches to the other axis.
export function SortModeToggle(props: {
  mode: SortMode;
  onToggle: () => void;
}) {
  return (
    <View
      class="h-5 flex-row items-center gap-1 rounded-md px-1.5 hover:bg-muted active:bg-muted"
      onPress={props.onToggle}
      role="button"
      accessibilityLabel="Sort mode"
      tooltip={
        <Hint>{props.mode === "date" ? "Sort by project" : "Sort by date"}</Hint>
      }
    >
      <Icon name="SortVertical" size={12} color={token("muted-foreground")} />
      <Text class="text-[11px] font-semibold text-muted-foreground">
        {props.mode === "date" ? "Date" : "Project"}
      </Text>
    </View>
  );
}

/// A section header: the label collapses or expands the group (a chevron
/// reveals on hover, rotated while collapsed). No count — the GPUI header
/// doesn't show one.
export function GroupHeader(props: {
  label: string;
  collapsed?: boolean;
  onToggle?: () => void;
}) {
  const collapsed = () => props.collapsed ?? false;
  return (
    <View
      class="group h-7 flex-row items-center justify-between"
      onPress={props.onToggle}
      role="button"
      accessibilityLabel={`${collapsed() ? "Expand" : "Collapse"} ${props.label}`}
    >
      <View class="h-[22px] flex-row items-center gap-1 rounded-md px-1 hover:bg-muted">
        <Text class="text-[14.5px] font-semibold text-foreground">
          {props.label}
        </Text>
        <View
          class="opacity-0 group-hover:opacity-100"
          style={
            collapsed()
              ? { transform: [{ rotate: "-90deg" }], opacity: 1 }
              : undefined
          }
        >
          <Icon name="ChevronDown" size={11} color={token("muted-foreground")} />
        </View>
      </View>
    </View>
  );
}
