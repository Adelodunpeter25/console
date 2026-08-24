//! Native application menu configuration.
//!
//! This is separate from the in-window dropdown/context menus in `console-ui`.
//! GPUI does not provide the macOS application menu automatically, so the
//! desktop app registers its top-level menus during startup.

use gpui::{App, Menu, MenuItem, actions};

actions!(console_app, [Quit]);

/// Register the native application menu bar and its application-level actions.
pub fn init(cx: &mut App) {
    cx.on_action(|_: &Quit, cx| cx.quit());

    cx.set_menus(vec![
        Menu::new("Console").items([MenuItem::action("Quit", Quit)]),
        Menu::new("File"),
        Menu::new("Edit"),
        Menu::new("View"),
        Menu::new("Window"),
        Menu::new("Help"),
    ]);
}
