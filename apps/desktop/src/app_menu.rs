//! Native application menu configuration.
//!
//! This is separate from the in-window dropdown/context menus in `console-ui`.
//! GPUI does not provide the macOS application menu automatically, so the
//! desktop app registers its top-level menus during startup.

use crate::keybindings::{
    AddProject, NewBrowser, NewChat, NewProject, NewTerminal, NewWindow, OpenSettings,
    QuickOpenFile, ToggleLeftSidebar, ToggleRightSidebar,
};
use gpui::{App, KeyBinding, Menu, MenuItem, OsAction, actions};

actions!(console_app, [Quit]);

/// Register the native application menu bar and its application-level actions.
pub fn init(cx: &mut App) {
    cx.on_action(|_: &Quit, cx| {
        if crate::window::any_agent_running(cx) {
            if let Some(handle) = cx.active_window() {
                handle
                    .update(cx, |_, window, cx| {
                        crate::window::confirm_then(window, cx, "Quit Console?", |_, cx| cx.quit());
                    })
                    .ok();
                return;
            }
        }
        cx.quit();
    });

    // cmd-q — the standard macOS quit shortcut. Binding it here also gives
    // the "Quit" menu item its ⌘Q key equivalent (GPUI menu items derive
    // their displayed keystroke from the keymap).
    cx.bind_keys([KeyBinding::new("secondary-q", Quit, None)]);

    cx.set_menus(vec![
        Menu::new("Console").items([
            MenuItem::action("Settings...", OpenSettings),
            MenuItem::action("Quit", Quit),
        ]),
        // File menu owns ⌘O / ⌘P / ⌘⇧N key equivalents so AppKit dispatches them
        // to our actions (same pattern Zed uses for Open / Go to File).
        Menu::new("File").items([
            MenuItem::action("New Window", NewWindow),
            // Project actions sit together, and "Add" precedes "New":
            // registering a folder that already exists is the common case, so
            // it is the one reached for first.
            MenuItem::action("New Chat", NewChat),
            MenuItem::action("Add Project…", AddProject),
            MenuItem::action("New Project…", NewProject),
            MenuItem::action("New Terminal", NewTerminal),
            // Browser tabs are deferred on Windows.
            #[cfg(not(target_os = "windows"))]
            MenuItem::action("New Browser Tab", NewBrowser),
            MenuItem::action("Quick Open File…", QuickOpenFile),
        ]),
        // Standard Edit menu: each item carries an OS action so AppKit runs
        // the native selector (undo/redo/cut/copy/paste/select-all) against
        // the first responder — the focused composer input. The GPUI action
        // is the fallback for when the focused view handles it itself, and
        // gives the menu item its displayed keystroke.
        Menu::new("Edit").items([
            MenuItem::os_action("Undo", console_ui::Undo, OsAction::Undo),
            MenuItem::os_action("Redo", console_ui::Redo, OsAction::Redo),
            MenuItem::Separator,
            MenuItem::os_action("Cut", console_ui::Cut, OsAction::Cut),
            MenuItem::os_action("Copy", console_ui::Copy, OsAction::Copy),
            MenuItem::os_action("Paste", console_ui::Paste, OsAction::Paste),
            MenuItem::os_action("Select All", console_ui::SelectAll, OsAction::SelectAll),
        ]),
        // View menu owns ⌘B / ⌥B key equivalents so AppKit routes them
        // to our actions (same pattern as File for ⌘O / ⌘P).
        Menu::new("View").items([
            MenuItem::action("Toggle Left Sidebar", ToggleLeftSidebar),
            MenuItem::action("Toggle Right Sidebar", ToggleRightSidebar),
        ]),
        Menu::new("Window"),
        Menu::new("Help"),
    ]);
}
