// The window title bar, ported from console-ui's `layout/title_bar.rs`:
// the selected chat's title centered in the bar, with the sidebar toggle
// leading (or a back arrow where a view takes the window over). The bar is
// draggable and reserves room for the system's window controls — nsh hides
// the system title bar for the app (`Window.setTitleBar({ style: "hidden"
// })`), and `desktop.windowDrag` plus `titleBar()` give back what the GPUI
// bar did by hand (the traffic-light inset).

import { Text, View, token } from "nsh";
import { titleBar } from "nsh/desktop";
import { Icon } from "../common/icons";
import { Hint } from "../common/tooltip";

export function TitleBar(props: {
  /** The selected session's title; nothing while no chat is open. */
  title?: string | null;
  /** The sidebar's width, so the centered title clears it, as on desktop. */
  sidebarWidth?: number;
  detail?: string | null;
  onToggleSidebar?: () => void;
  onBack?: () => void;
}) {
  // Reactive: re-reads when the system reports its hidden title bar's area.
  const controls = titleBar();
  const leadingInset = Math.max(controls.left, 8);
  const sidebarWidth = props.sidebarWidth ?? 0;
  const toggle = (
    <View
      class="rounded p-1.5 hover:bg-muted"
      onPress={props.onToggleSidebar ?? props.onBack}
      accessibilityLabel={props.onBack ? "Back" : "Toggle sidebar"}
      tooltip={
        <Hint>{props.onBack ? "Back" : "Toggle sidebar"}</Hint>
      }
      role="button"
    >
      <Icon
        name={props.onBack ? "ArrowLeft" : "PanelLeft"}
        size={16}
        color={token("muted-foreground")}
      />
    </View>
  );
  return (
    <View
      class="flex h-12 shrink-0 flex-row items-center border-b border-border bg-card"
      desktop={{ windowDrag: true }}
    >
      <View
        class="flex-none flex-row items-center"
        style={{ width: sidebarWidth, paddingLeft: leadingInset }}
      >
        {props.onBack || props.onToggleSidebar ? toggle : null}
      </View>
      <View class="flex-1 flex-row items-center justify-center gap-2">
        {props.title ? (
          <Text class="truncate text-sm font-semibold text-foreground">
            {props.title}
          </Text>
        ) : null}
        {props.detail ? (
          <Text class="text-xs text-muted-foreground">{props.detail}</Text>
        ) : null}
      </View>
      <View style={{ width: sidebarWidth }} class="flex-none" />
    </View>
  );
}
