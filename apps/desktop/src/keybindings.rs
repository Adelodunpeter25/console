//! Central registry of global keyboard shortcuts.
//!
//! Every window-wide shortcut lives here: the actions are defined in this one
//! module, their keystrokes are bound once in [`init`], and their handlers are
//! registered once per window in [`init_handlers`]. To add a shortcut:
//! declare an action below, bind a key in [`init`], write a method on
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
        /// Open the folder picker to add a project.
        AddProject,
        /// Toggle the command palette.
        ToggleCommandPalette,
        /// Move keyboard focus to the active pane's composer.
        FocusComposer,
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
        KeyBinding::new("secondary-l", FocusComposer, None),
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
        move |_: &AddProject, cx| {
            app.update(cx, |this, cx| this.add_project(cx));
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
    cx.on_action(move |_: &FocusComposer, cx| {
        let app = app.clone();
        cx.defer(move |cx| {
            window
                .update(cx, |_, window, cx| {
                    app.update(cx, |this, cx| this.focus_composer(window, cx));
                })
                .ok();
        });
    });
}
