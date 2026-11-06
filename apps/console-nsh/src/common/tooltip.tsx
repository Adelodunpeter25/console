// The tooltip bubble, ported from console-ui's `primitives/tooltip.rs` —
// there, GPUI owns tooltip behaviour (hover timing, placement, dismissal)
// and the file is only the bubble's look; here nsh's native `Tooltip` owns
// the behaviour and the file is only the look. Attach it with a view's
// `tooltip` prop: `<View tooltip={<Hint>Copy message</Hint>}>`, which shows
// on hover (after the framework's delay) and on open (touch).

import { Text, View } from "nsh";

/// A single-line hint, styled as the desktop's tooltip card.
export function Hint(props: { children: string }) {
  return (
    <View
      class="flex-row items-center gap-1.5 rounded-md border border-border bg-popover px-[7px] py-1 shadow-md"
      style={{ paddingLeft: 7, paddingTop: 4, paddingRight: 7, paddingBottom: 4 }}
    >
      <Text class="text-[11px] text-muted-foreground">{props.children}</Text>
    </View>
  );
}
