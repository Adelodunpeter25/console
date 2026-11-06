// The full-width strip that centers a card on the 768px content column,
// ported from console-ui's `common/stripe.rs`. Shared by every banner and
// interaction card above the composer so they all align with the transcript
// and composer instead of each re-declaring the wrapper.

import { View } from "nsh";
import type { JSX } from "nsh";

export function CenteredStripe(props: {
  paddingY: number;
  children: JSX.Element;
}) {
  return (
    <View
      class="flex w-full justify-center px-5"
      style={{ paddingTop: props.paddingY, paddingBottom: props.paddingY }}
    >
      {props.children}
    </View>
  );
}
