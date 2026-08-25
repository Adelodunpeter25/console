//! Central registry of global keyboard shortcuts.
//!
//! Every window-wide shortcut lives here: the actions are defined in this one
//! module, their keystrokes are bound once in [`init`], and the handlers are
//! methods on `ConsoleDesktopApp` (see `state/global_actions.rs`) attached to
//! the app root in `view.rs`. To add a shortcut: declare an action below, bind
//! a key, write the handler, and attach it with `.on_action(cx.listener(..))`
//! on `"app-root"`.
//!
//! Bindings carry no key context on purpose, so they fire anywhere in the
//! window that a deeper context (composer, menus, palette) has not claimed
//! the same keystroke for itself.

use gpui::{App, KeyBinding, actions};

actions!(
    console_global,
    [
        /// Close the active workspace tab.
        CloseTab,
        /// Create a chat session in the active pane.
        NewChat,
        /// Open the folder picker to add a project.
        AddProject,
        /// Toggle the command palette.
        ToggleCommandPalette,
    ]
);

/// Register the global shortcuts. Called once at startup, after the more
/// specific context bindings, so those keep winning ties inside their own
/// contexts.
pub fn init(cx: &mut App) {
    cx.bind_keys([
        // `secondary` is cmd on macOS and ctrl on Linux/Windows.
        KeyBinding::new("secondary-w", CloseTab, None),
        KeyBinding::new("secondary-n", NewChat, None),
        KeyBinding::new("secondary-o", AddProject, None),
        KeyBinding::new("secondary-k", ToggleCommandPalette, None),
    ]);
}
