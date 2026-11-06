// The floating compact composer (console-ui's `floating_composer/`, first
// cut): a collapsed pill that expands into the full `Composer` and collapses
// again with Escape over an empty draft — the desktop's open/collapse
// behaviour. Its launcher-card extras (project, branch and GitHub menus)
// land with the workspace panes they launch into.
//
// NSH renders components once and tracks prop reads in JSX, so the draft
// emptiness is read inside the keymap handler rather than normalized into a
// body-level const (which would freeze it at mount).

import { Text, View, keymap, signal, token } from "nsh";
import { Icon } from "./icons";
import { Composer } from "./composer";

export function FloatingComposer(props: Parameters<typeof Composer>[0]) {
  const open = signal(false);

  /// Escape over an empty draft collapses the card back to the pill; a
  /// draft keeps the focus (and the escape for the autocomplete).
  function EscapeCollapses() {
    keymap.bind("escape", () => {
      if (props.value === "") open.set(false);
    });
    return null;
  }

  return (
    <View class="w-full">
      {open() ? (
        <View class="flex-col">
          <EscapeCollapses />
          <Composer {...props} />
        </View>
      ) : (
        <View
          class="flex-row items-center gap-2 rounded-full border border-border bg-card px-4 py-2.5"
          role="button"
          accessibilityLabel="Ask Console anything"
          onPress={() => open.set(true)}
        >
          <Icon name="Compose" size={14} color={token("muted-foreground")} />
          <Text class="flex-1 text-sm text-muted-foreground">
            Ask Console anything…
          </Text>
          <Icon name="Paperclip" size={14} color={token("muted-foreground")} />
        </View>
      )}
    </View>
  );
}
