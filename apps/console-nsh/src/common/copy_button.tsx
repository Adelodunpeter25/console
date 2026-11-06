// The copy button with copied-state feedback, ported from console-ui's
// `common/copy_button.rs`: on press it writes `content` to the clipboard and
// swaps its icon from a copy glyph to a checkmark for two seconds. Shared by
// the error banner and, later, message bubbles and code blocks so every copy
// affordance behaves identically.
//
// The desktop tracked copied state in a global keyed by an `id` per copy
// site; each button here owns its state locally, which gives the same
// behaviour (feedback never bleeds between buttons) without the registry.

import { Clipboard, View, signal, token } from "nsh";
import { Icon } from "./icons";

const FEEDBACK_MS = 2000;

export function CopyButton(props: {
  /// Stable per copy site, matching the desktop's `id` (kept for callers that
  /// key off it; state is per component here).
  id: string;
  /// The text to copy. Pass a thunk when it can change after mount.
  content: string | (() => string);
  /// Invoked after a successful copy (the error banner dismisses itself).
  onCopied?: () => void;
}) {
  const copied = signal(false);
  // Generations guard against a stale timer from an earlier copy clearing a
  // newer one's feedback (the desktop's `CopyFeedback` did the same).
  let generation = 0;

  const content = () =>
    typeof props.content === "function" ? props.content() : props.content;

  const copy = () => {
    const mine = ++generation;
    void Clipboard.writeText(content()).then(() => {
      props.onCopied?.();
      copied.set(true);
      setTimeout(() => {
        if (generation === mine) copied.set(false);
      }, FEEDBACK_MS);
    });
  };

  return (
    <View
      class="h-[27px] w-[27px] flex-row items-center justify-center rounded-lg hover:bg-accent"
      role="button"
      accessibilityLabel="Copy"
      onPress={copy}
    >
      <Icon
        name={copied() ? "Check" : "Copy"}
        size={14}
        color={token("muted-foreground")}
      />
    </View>
  );
}

/// The cite button beside it (console-ui's selection popup's "Quote
/// selection" pill, which uses the same reply glyph): presses invoke
/// `onCite` with the message's text for the host to quote into the
/// composer. The desktop's drag-selection popup itself is a platform drop —
/// nsh has no cross-view selection — so this cites the whole message.
export function CiteButton(props: { onCite: () => void }) {
  return (
    <View
      class="h-[27px] w-[27px] flex-row items-center justify-center rounded-lg hover:bg-accent"
      role="button"
      accessibilityLabel="Quote message"
      onPress={props.onCite}
    >
      <Icon name="Reply" size={14} color={token("muted-foreground")} />
    </View>
  );
}
