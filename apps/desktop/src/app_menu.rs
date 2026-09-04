//! Native application menu configuration.
//!
//! This is separate from the in-window dropdown/context menus in `console-ui`.
//! GPUI does not provide the macOS application menu automatically, so the
//! desktop app registers its top-level menus during startup.

use crate::keybindings::{AddProject, OpenSettings, QuickOpenFile, ToggleLeftSidebar, ToggleRightSidebar};
use gpui::{App, Menu, MenuItem, actions};

actions!(console_app, [Quit]);

/// Register the native application menu bar and its application-level actions.
pub fn init(cx: &mut App) {
    cx.on_action(|_: &Quit, cx| cx.quit());

    cx.set_menus(vec![
        Menu::new("Console").items([
            MenuItem::action("Settings...", OpenSettings),
            MenuItem::action("Quit", Quit),
        ]),
        // File menu owns ⌘O / ⌘P key equivalents so AppKit dispatches them
        // to our actions (same pattern Zed uses for Open / Go to File).
        Menu::new("File").items([
            MenuItem::action("Add Project…", AddProject),
            MenuItem::action("Quick Open File…", QuickOpenFile),
        ]),
        Menu::new("Edit"),
        // View menu owns ⌘B / ⌘⇧B key equivalents so AppKit routes them
        // to our actions (same pattern as File for ⌘O / ⌘P).
        Menu::new("View").items([
            MenuItem::action("Toggle Left Sidebar", ToggleLeftSidebar),
            MenuItem::action("Toggle Right Sidebar", ToggleRightSidebar),
        ]),
        Menu::new("Window"),
        Menu::new("Help"),
    ]);
}
