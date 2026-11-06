// The global workspace shortcuts, ported from the desktop's
// `apps/desktop/src/keybindings.rs` (the subset that has an NSH target):
// cmd+n opens the launcher composer (`NewChat`), cmd+w closes the active
// pane's active tab (`CloseTab`), cmd+l focuses the composer
// (`FocusComposer`). The palette and settings bindings land with those
// screens (2.8–2.9); mod is cmd on macOS and ctrl elsewhere.

import { keymap } from "nsh";
import { focusComposer } from "../common/composer";
import { tabId } from "./model";
import type { WorkspaceStore } from "./workspace_store";

export function WorkspaceShortcuts(props: {
  store: WorkspaceStore;
  onNewChat: () => void;
}) {
  keymap.bind("mod+n", props.onNewChat);
  keymap.bind("mod+l", () => focusComposer());
  keymap.bind("mod+w", () => {
    const pane = props.store.activePane();
    const tab = pane.tabs.find((t) => tabId(t) === pane.activeTabId);
    if (tab != null) props.store.closeTab(pane.id, tab);
  });
  return null;
}
