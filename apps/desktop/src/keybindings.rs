//! Central registry of global keyboard shortcuts.
//!
//! Every window-wide shortcut lives here: the actions are defined in this one
//! module, their keystrokes are bound once in [`init`], and their handlers are
//! registered once in [`init_handlers`].
//!
//! Bindings carry no key context on purpose, so they fire anywhere in the
//! window that a deeper context (composer, menus, palette) has not claimed
//! the same keystroke for itself.

use gpui::{App, KeyBinding, actions};

actions!(
    console_global,
    [
        /// Open a new desktop window (cmd-shift-n).
        NewWindow,
        /// Close the active workspace tab.
        CloseTab,
        /// Create a chat session in the active pane.
        NewChat,
        /// Open the project directory browser palette to add a project (cmd-o).
        AddProject,
        /// Open the quick file search palette scoped to the active pane's project (cmd-p).
        QuickOpenFile,
        /// Toggle the command palette.
        ToggleCommandPalette,
        /// Move keyboard focus to the active pane's composer.
        FocusComposer,
        /// Open settings window.
        OpenSettings,
        /// Toggle the left sidebar.
        ToggleLeftSidebar,
        /// Toggle the right workspace inspector panel.
        ToggleRightSidebar,
        /// Jump to the nth visible sidebar session (browser-tab style).
        SwitchSession1,
        SwitchSession2,
        SwitchSession3,
        SwitchSession4,
        SwitchSession5,
        SwitchSession6,
        SwitchSession7,
        SwitchSession8,
        SwitchSession9,
        /// Jump to the nth tab of the active workspace pane, any tab type
        /// (Option+1–9 / Alt+1–9).
        SwitchWorkspaceTab1,
        SwitchWorkspaceTab2,
        SwitchWorkspaceTab3,
        SwitchWorkspaceTab4,
        SwitchWorkspaceTab5,
        SwitchWorkspaceTab6,
        SwitchWorkspaceTab7,
        SwitchWorkspaceTab8,
        SwitchWorkspaceTab9,
    ]
);

/// Register the global shortcuts. Called once at startup, after the more
/// specific context bindings, so those keep winning ties inside their own
/// contexts.
pub fn init(cx: &mut App) {
    cx.bind_keys([
        // `secondary` is cmd on macOS and ctrl on Linux/Windows.
        // Two spellings cover NewWindow: GPUI matches the key letter
        // case-sensitively, and macOS reports an uppercase "N" while
        // Shift is held. (No separate cmd-* bindings — "cmd" parses to
        // the same platform modifier as "secondary".)
        KeyBinding::new("secondary-shift-n", NewWindow, None),
        KeyBinding::new("secondary-shift-N", NewWindow, None),
        KeyBinding::new("secondary-w", CloseTab, None),
        KeyBinding::new("secondary-n", NewChat, None),
        // cmd-o / cmd-p — same as Zed; our File menu owns the key equivalents
        // so AppKit routes them to us instead of the system Open/Print handlers.
        KeyBinding::new("secondary-o", AddProject, None),
        KeyBinding::new("secondary-p", QuickOpenFile, None),
        KeyBinding::new("secondary-k", ToggleCommandPalette, None),
        KeyBinding::new("secondary-l", FocusComposer, None),
        KeyBinding::new("secondary-,", OpenSettings, None),
        KeyBinding::new("secondary-b", ToggleLeftSidebar, None),
        KeyBinding::new("secondary-shift-b", ToggleRightSidebar, None),
        KeyBinding::new("secondary-1", SwitchSession1, None),
        KeyBinding::new("secondary-2", SwitchSession2, None),
        KeyBinding::new("secondary-3", SwitchSession3, None),
        KeyBinding::new("secondary-4", SwitchSession4, None),
        KeyBinding::new("secondary-5", SwitchSession5, None),
        KeyBinding::new("secondary-6", SwitchSession6, None),
        KeyBinding::new("secondary-7", SwitchSession7, None),
        KeyBinding::new("secondary-8", SwitchSession8, None),
        KeyBinding::new("secondary-9", SwitchSession9, None),
        // Option+1–9 (Alt elsewhere): switch tabs within the active workspace
        // pane. `alt` is the Option modifier in GPUI keystroke strings. These
        // intentionally shadow the focused terminal's own key handling —
        // GPUI dispatches action bindings before element key listeners —
        // so tab switching works from anywhere, terminals included.
        KeyBinding::new("alt-1", SwitchWorkspaceTab1, None),
        KeyBinding::new("alt-2", SwitchWorkspaceTab2, None),
        KeyBinding::new("alt-3", SwitchWorkspaceTab3, None),
        KeyBinding::new("alt-4", SwitchWorkspaceTab4, None),
        KeyBinding::new("alt-5", SwitchWorkspaceTab5, None),
        KeyBinding::new("alt-6", SwitchWorkspaceTab6, None),
        KeyBinding::new("alt-7", SwitchWorkspaceTab7, None),
        KeyBinding::new("alt-8", SwitchWorkspaceTab8, None),
        KeyBinding::new("alt-9", SwitchWorkspaceTab9, None),
    ]);
}

/// Register the app-global handlers for the actions bound in [`init`].
///
/// These handlers dynamically route to the active/focused window so multi-window
/// setups dispatch shortcuts to whichever window the user is currently working in.
pub fn init_handlers(cx: &mut App) {
    cx.on_action(|_: &NewWindow, cx| {
        crate::window::open_workspace_window(cx, crate::window::WindowLaunchTarget::Fresh);
    });

    cx.on_action(|_: &CloseTab, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.close_tab(cx));
        }
    });

    cx.on_action(|_: &NewChat, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.create_new_chat(cx));
        }
    });

    cx.on_action(|_: &OpenSettings, cx| {
        if let Some((window, app)) = crate::window::get_active_window(cx) {
            cx.defer(move |cx| {
                window
                    .update(cx, |_, window, cx| {
                        app.update(cx, |this, cx| this.open_settings(window, cx));
                    })
                    .ok();
            });
        }
    });

    cx.on_action(|_: &ToggleCommandPalette, cx| {
        if let Some((window, app)) = crate::window::get_active_window(cx) {
            cx.defer(move |cx| {
                window
                    .update(cx, |_, window, cx| {
                        app.update(cx, |this, cx| this.toggle_command_palette(window, cx));
                    })
                    .ok();
            });
        }
    });

    cx.on_action(|_: &AddProject, cx| {
        if let Some((window, app)) = crate::window::get_active_window(cx) {
            cx.defer(move |cx| {
                window
                    .update(cx, |_, window, cx| {
                        app.update(cx, |this, cx| this.open_project_browse(window, cx));
                    })
                    .ok();
            });
        }
    });

    cx.on_action(|_: &QuickOpenFile, cx| {
        if let Some((window, app)) = crate::window::get_active_window(cx) {
            cx.defer(move |cx| {
                window
                    .update(cx, |_, window, cx| {
                        app.update(cx, |this, cx| this.open_quick_open(window, cx));
                    })
                    .ok();
            });
        }
    });

    cx.on_action(|_: &FocusComposer, cx| {
        if let Some((window, app)) = crate::window::get_active_window(cx) {
            cx.defer(move |cx| {
                window
                    .update(cx, |_, window, cx| {
                        app.update(cx, |this, cx| this.focus_composer(window, cx));
                    })
                    .ok();
            });
        }
    });

    cx.on_action(|_: &ToggleLeftSidebar, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.toggle_left_sidebar(cx));
        }
    });

    cx.on_action(|_: &ToggleRightSidebar, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.toggle_right_sidebar(cx));
        }
    });

    cx.on_action(|_: &SwitchSession1, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_session_by_visible_index(0, cx));
        }
    });

    cx.on_action(|_: &SwitchSession2, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_session_by_visible_index(1, cx));
        }
    });

    cx.on_action(|_: &SwitchSession3, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_session_by_visible_index(2, cx));
        }
    });

    cx.on_action(|_: &SwitchSession4, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_session_by_visible_index(3, cx));
        }
    });

    cx.on_action(|_: &SwitchSession5, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_session_by_visible_index(4, cx));
        }
    });

    cx.on_action(|_: &SwitchSession6, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_session_by_visible_index(5, cx));
        }
    });

    cx.on_action(|_: &SwitchSession7, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_session_by_visible_index(6, cx));
        }
    });

    cx.on_action(|_: &SwitchSession8, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_session_by_visible_index(7, cx));
        }
    });

    cx.on_action(|_: &SwitchSession9, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_session_by_visible_index(8, cx));
        }
    });

    cx.on_action(|_: &SwitchWorkspaceTab1, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_workspace_tab_by_index(0, cx));
        }
    });

    cx.on_action(|_: &SwitchWorkspaceTab2, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_workspace_tab_by_index(1, cx));
        }
    });

    cx.on_action(|_: &SwitchWorkspaceTab3, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_workspace_tab_by_index(2, cx));
        }
    });

    cx.on_action(|_: &SwitchWorkspaceTab4, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_workspace_tab_by_index(3, cx));
        }
    });

    cx.on_action(|_: &SwitchWorkspaceTab5, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_workspace_tab_by_index(4, cx));
        }
    });

    cx.on_action(|_: &SwitchWorkspaceTab6, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_workspace_tab_by_index(5, cx));
        }
    });

    cx.on_action(|_: &SwitchWorkspaceTab7, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_workspace_tab_by_index(6, cx));
        }
    });

    cx.on_action(|_: &SwitchWorkspaceTab8, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_workspace_tab_by_index(7, cx));
        }
    });

    cx.on_action(|_: &SwitchWorkspaceTab9, cx| {
        if let Some((_, app)) = crate::window::get_active_window(cx) {
            app.update(cx, |this, cx| this.select_workspace_tab_by_index(8, cx));
        }
    });
}
