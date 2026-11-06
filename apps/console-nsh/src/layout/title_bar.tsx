// The window title bar: the app's name and, on the right, the backend
// state. Phase 2.1 (`console-ui/src/layout/title_bar.rs` is the GPUI
// original).

import { Text, View } from "nsh";

export function TitleBar(props: { detail?: string | null }) {
  return (
    <View class="flex h-12 shrink-0 items-center gap-2 border-b border-border px-4">
      <Text class="text-sm font-semibold text-foreground">Console</Text>
      {props.detail ? (
        <Text class="text-xs text-muted-foreground">{props.detail}</Text>
      ) : null}
      <View class="flex-1" />
    </View>
  );
}
