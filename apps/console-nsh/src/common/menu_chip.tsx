// The compact chip a dropdown opens from, ported from console-ui's
// `primitives/mod.rs` `MenuChip`: an icon, a truncating label and an optional
// caret, with the selected (menu open) and disabled looks. The footer's
// project and branch chips use it; the docked look (no outline, no caret)
// is the only variant they need.

import { Text, View, token } from "nsh";
import { Icon, type IconName } from "./icons";

export function MenuChip(props: {
  icon?: IconName;
  /// The chip's text. Pass a thunk when it can change after mount (the
  /// footer's chips rename as state moves); the trigger's JSX is built once.
  label: string | (() => string);
  /// Whether the menu this chip opens is showing.
  selected?: boolean;
  disabled?: boolean;
  /// Long labels truncate at this width on the footer's chips.
  maxWidth?: number;
  /// What screen readers say, and how tests press the chip.
  accessibilityLabel?: string;
}) {
  const label = () =>
    typeof props.label === "function" ? props.label() : props.label;
  return (
    <View
      class={
        "h-6 flex-none flex-row items-center gap-1.5 rounded-md px-[7px] text-[11.5px] leading-[14px] " +
        (props.selected ? "bg-accent " : "") +
        (props.disabled ? "opacity-70" : "hover:bg-accent")
      }
      style={props.maxWidth ? { maxWidth: props.maxWidth } : undefined}
      accessibilityLabel={props.accessibilityLabel}
    >
      {props.icon ? (
        <Icon
          name={props.icon}
          size={10.5}
          color={token("muted-foreground")}
        />
      ) : null}
      <Text class="min-w-0 flex-1 text-[11.5px] text-muted-foreground"
        style={props.maxWidth ? { maxWidth: props.maxWidth } : undefined}
      >
        {label()}
      </Text>
    </View>
  );
}
