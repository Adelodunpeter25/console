//! Central registry of global keyboard shortcuts.
//!
//! Every window-wide shortcut lives here: the actions are defined in this one
//! module, their keystrokes are bound once in [`init`], and their handlers are
//! registered once per window in [`init_handlers`]. To add a shortcut: declare
//! an action below, bind a key in [`init`], write a method on
//! `ConsoleDesktopApp`, and register it in [`init_handlers`].
//!
//! Bindings carry no key context on purpose, so they fire anywhere in the
//! window that a deeper context (composer, menus, palette) has not claimed
//! the same keystroke for itself.

use gpui::{AnyWindowHandle, App, Entity, KeyBinding, actions};

use crate::state::ConsoleDesktopApp;

actions!(
    console_global,
    [
        /// Close the active workspace tab.
        CloseTab,
        /// Create a chat session in the active pane.
        NewChat,
        /// Open the project directory browser palette to add a project (cmd-shift-o).
        AddProject,
        /// Open the quick file search palette scoped to the active pane's project (cmd-shift-p).
        QuickOpenFile,
        /// Toggle the command palette.
        ToggleCommandPalette,
        /// Move keyboard focus to the active pane's composer.
        FocusComposer,
        /// Open settings window.
        OpenSettings,
        /// Toggle the right workspace inspector panel.
        ToggleRightSidebar,
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
        // Use cmd-shift-o for Add Project to avoid macOS reserved cmd-o
        KeyBinding::new("secondary-shift-o", AddProject, None),
        // Use cmd-shift-p for Quick Open File to avoid macOS reserved cmd-p
        KeyBinding::new("secondary-shift-p", QuickOpenFile, None),
        KeyBinding::new("secondary-k", ToggleCommandPalette, None),
        KeyBinding::new("secondary-l", FocusComposer, None),
        KeyBinding::new("secondary-,", OpenSettings, None),
        KeyBinding::new("secondary-shift-b", ToggleRightSidebar, None),
    ]);
}

/// Register the app-global handlers for the actions bound in [`init`].
///
/// These must be *global* handlers (`App::on_action`), not element-level
/// `.on_action`: an element handler only sees actions dispatched along its
/// focus path, so with nothing focused — a fresh launch, say — the keystroke
/// matches its binding, lands on the window's root dispatch node, and simply
/// vanishes. Global listeners run first, focus or no focus.
pub fn init_handlers(app: Entity<ConsoleDesktopApp>, window: AnyWindowHandle, cx: &mut App) {
    cx.on_action({
        let app = app.clone();
        move |_: &CloseTab, cx| {
            app.update(cx, |this, cx| this.close_tab(cx));
        }
    });
    cx.on_action({
        let app = app.clone();
        move |_: &NewChat, cx| {
            app.update(cx, |this, cx| this.create_new_chat(cx));
        }
    });
    cx.on_action({
        let app = app.clone();
        move |_: &OpenSettings, cx| {
            let app = app.clone();
            cx.defer(move |cx| {
                window
                    .update(cx, |_, window, cx| {
                        app.update(cx, |this, cx| this.open_settings(window, cx));
                    })
                    .ok();
            });
        }
    });
    // Handlers that touch the window itself (focus, overlays) must defer:
    // global listeners run *inside* the window's own update, and a re-entrant
    // `window.update` fails because the window is already checked out —
    // the same reason gpui defers in `Window::dispatch_action`.
    cx.on_action({
        let app = app.clone();
        move |_: &ToggleCommandPalette, cx| {
            let app = app.clone();
            cx.defer(move |cx| {
                window
                    .update(cx, |_, window, cx| {
                        app.update(cx, |this, cx| this.toggle_command_palette(window, cx));
                    })
                    .ok();
            });
        }
    });
    // Same pattern as ⌘K: open the project browser / quick-open palettes.
    // These also power the File menu items (Add Project / Quick Open File).
    cx.on_action({
        let app = app.clone();
        move |_: &AddProject, cx| {
            let app = app.clone();
            cx.defer(move |cx| {
                window
                    .update(cx, |_, window, cx| {
                        app.update(cx, |this, cx| this.open_project_browse(window, cx));
                    })
                    .ok();
            });
        }
    });
    cx.on_action({
        let app = app.clone();
        move |_: &QuickOpenFile, cx| {
            let app = app.clone();
            cx.defer(move |cx| {
                window
                    .update(cx, |_, window, cx| {
                        app.update(cx, |this, cx| this.open_quick_open(window, cx));
                    })
                    .ok();
            });
        }
    });
    cx.on_action({
        let app = app.clone();
        move |_: &FocusComposer, cx| {
            let app = app.clone();
            cx.defer(move |cx| {
                window
                    .update(cx, |_, window, cx| {
                        app.update(cx, |this, cx| this.focus_composer(window, cx));
                    })
                    .ok();
            });
        }
    });
    cx.on_action({
        let app = app.clone();
        move |_: &ToggleRightSidebar, cx| {
            app.update(cx, |this, cx| this.toggle_right_sidebar(cx));
        }
    });
}
