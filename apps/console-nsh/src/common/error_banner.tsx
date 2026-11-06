// The error banner shown above the composer, ported from console-ui's
// `common/error_banner.rs`: a rounded danger-tinted card that carries a copy
// button; copying the message dismisses the banner via `onCopied`.
//
// Fidelity gap: on the desktop the message is a selectable markdown render
// (drag/double/triple-click selection through `markdown/selection.rs`), which
// is part of the markdown stack this migration has not ported yet. Here the
// message is plain text; selection arrives with the markdown pass.

import { Text, View } from "nsh";
import { CenteredStripe } from "./stripe";
import { CopyButton } from "./copy_button";

export function ErrorBanner(props: {
  message: string | (() => string);
  /// Invoked after the copy button successfully copies the message.
  onCopied?: () => void;
}) {
  const message = () =>
    typeof props.message === "function" ? props.message() : props.message;

  return (
    <CenteredStripe paddingY={6}>
      <View class="w-full max-w-[768px] flex-row items-center gap-1.5 rounded border border-destructive/35 bg-card px-2.5 py-[7px]">
        {/* flex-1 here would break press hit-testing on the copy button
            (an nsh layout quirk), so the text sizes naturally. */}
        <Text class="text-[11.5px] text-destructive">
          {message()}
        </Text>
        <CopyButton id="error-copy" content={message} onCopied={props.onCopied} />
      </View>
    </CenteredStripe>
  );
}
