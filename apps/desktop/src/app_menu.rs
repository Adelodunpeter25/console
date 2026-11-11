//! Native application menu configuration.
//!
//! This is separate from the in-window dropdown/context menus in `console-ui`.
//! GPUI does not provide the macOS application menu automatically, so the
//! desktop app registers its top-level menus during startup.

use crate::keybindings::{
    AddProject, CheckForUpdates, NewBrowser, NewChat, NewProject, NewTerminal, NewWindow,
    OpenSettings, QuickOpenFile, ToggleLeftSidebar, ToggleRightSidebar,
};
use gpui::{App, KeyBinding, Menu, MenuItem, OsAction, SystemMenuType, actions};

actions!(
    console_app,
    [
        Quit,
        /// orderFrontStandardAboutPanel: — the standard About box.
        About,
        /// NSApp hide: (⌘H).
        HideApp,
        /// NSApp hideOtherApplications: (⌥⌘H).
        HideOthers,
        /// NSApp unhideAllApplications:.
        UnhideAll,
        /// keyWindow miniaturize: (⌘M).
        Minimize,
        /// keyWindow performZoom:.
        Zoom,
        /// keyWindow toggleTabBar:.
        ToggleTabBar,
        /// keyWindow selectPreviousTab: (⌃⇧⇥).
        ShowPreviousTab,
        /// keyWindow selectNextTab: (⌃⇥).
        ShowNextTab,
        /// keyWindow moveTabToNewWindow:.
        MoveTabToNewWindow,
        /// keyWindow mergeAllWindows:.
        MergeAllWindows,
        /// NSApp arrangeInFront:.
        BringAllToFront,
        /// orderFrontCharacterPalette: — the system emoji picker.
        ShowEmojiPicker,
    ]
);

#[cfg(target_os = "macos")]
fn with_ns_app(body: impl FnOnce(&objc2_app_kit::NSApplication)) {
    let Some(mtm) = objc2::MainThreadMarker::new() else {
        return;
    };
    let app = objc2_app_kit::NSApplication::sharedApplication(mtm);
    body(&app);
}

/// Run `body` with the key window, if any.
#[cfg(target_os = "macos")]
fn with_key_window(body: impl FnOnce(&objc2_app_kit::NSWindow)) {
    with_ns_app(|app| unsafe {
        let window: Option<objc2::rc::Retained<objc2_app_kit::NSWindow>> =
            objc2::msg_send![app, keyWindow];
        if let Some(window) = window {
            body(&window);
        }
    });
}

/// Register the native application menu bar and its application-level actions.
pub fn init(cx: &mut App) {
    cx.on_action(|_: &Quit, cx| {
        if !crate::window::any_agent_running(cx) {
            cx.quit();
            return;
        }
        // Deferred: this action is dispatched from inside an update of the
        // active window, and GPUI cannot update a window re-entrantly, so
        // asking for the confirmation here would fail and swallow the quit.
        cx.defer(|cx| {
            let asked = cx.active_window().is_some_and(|handle| {
                handle
                    .update(cx, |_, window, cx| {
                        crate::window::confirm_then(window, cx, "Quit Console?", |_, cx| cx.quit());
                    })
                    .is_ok()
            });
            if !asked {
                cx.quit();
            }
        });
    });

    cx.on_action(|_: &About, _| {
        #[cfg(target_os = "macos")]
        with_ns_app(|app| unsafe {
            let _: () = objc2::msg_send![
                app,
                orderFrontStandardAboutPanel: None::<&objc2::runtime::AnyObject>
            ];
        });
    });

    cx.on_action(|_: &HideApp, _| {
        #[cfg(target_os = "macos")]
        with_ns_app(|app| unsafe {
            let _: () = objc2::msg_send![app, hide: None::<&objc2::runtime::AnyObject>];
        });
    });

    cx.on_action(|_: &HideOthers, _| {
        #[cfg(target_os = "macos")]
        with_ns_app(|app| unsafe {
            let _: () =
                objc2::msg_send![app, hideOtherApplications: None::<&objc2::runtime::AnyObject>];
        });
    });

    cx.on_action(|_: &UnhideAll, _| {
        #[cfg(target_os = "macos")]
        with_ns_app(|app| unsafe {
            let _: () =
                objc2::msg_send![app, unhideAllApplications: None::<&objc2::runtime::AnyObject>];
        });
    });

    cx.on_action(|_: &Minimize, _| {
        #[cfg(target_os = "macos")]
        with_key_window(|window| unsafe {
            let _: () = objc2::msg_send![window, miniaturize: None::<&objc2::runtime::AnyObject>];
        });
    });

    cx.on_action(|_: &Zoom, _| {
        #[cfg(target_os = "macos")]
        with_key_window(|window| unsafe {
            let _: () = objc2::msg_send![window, performZoom: None::<&objc2::runtime::AnyObject>];
        });
    });

    cx.on_action(|_: &ToggleTabBar, _| {
        #[cfg(target_os = "macos")]
        with_key_window(|window| unsafe {
            let _: () = objc2::msg_send![window, toggleTabBar: None::<&objc2::runtime::AnyObject>];
        });
    });

    cx.on_action(|_: &ShowPreviousTab, _| {
        #[cfg(target_os = "macos")]
        with_key_window(|window| unsafe {
            let _: () =
                objc2::msg_send![window, selectPreviousTab: None::<&objc2::runtime::AnyObject>];
        });
    });

    cx.on_action(|_: &ShowNextTab, _| {
        #[cfg(target_os = "macos")]
        with_key_window(|window| unsafe {
            let _: () = objc2::msg_send![window, selectNextTab: None::<&objc2::runtime::AnyObject>];
        });
    });

    cx.on_action(|_: &MoveTabToNewWindow, _| {
        #[cfg(target_os = "macos")]
        with_key_window(|window| unsafe {
            let _: () =
                objc2::msg_send![window, moveTabToNewWindow: None::<&objc2::runtime::AnyObject>];
        });
    });

    cx.on_action(|_: &MergeAllWindows, _| {
        #[cfg(target_os = "macos")]
        with_key_window(|window| unsafe {
            let _: () =
                objc2::msg_send![window, mergeAllWindows: None::<&objc2::runtime::AnyObject>];
        });
    });

    cx.on_action(|_: &BringAllToFront, _| {
        #[cfg(target_os = "macos")]
        with_ns_app(|app| unsafe {
            let _: () = objc2::msg_send![app, arrangeInFront: None::<&objc2::runtime::AnyObject>];
        });
    });

    cx.on_action(|_: &ShowEmojiPicker, _| {
        #[cfg(target_os = "macos")]
        with_ns_app(|app| unsafe {
            use objc2::runtime::NSObjectProtocol;

            let sel = objc2::runtime::Sel::register(c"orderFrontCharacterPalette:");
            if app.respondsToSelector(sel) {
                let _: () = objc2::msg_send![
                    app,
                    orderFrontCharacterPalette: None::<&objc2::runtime::AnyObject>
                ];
            }
        });
    });

    // cmd-q — the standard macOS quit shortcut. Binding it here also gives
    // the "Quit" menu item its ⌘Q key equivalent (GPUI menu items derive
    // their displayed keystroke from the keymap).
    cx.bind_keys([KeyBinding::new("secondary-q", Quit, None)]);
    // Standard app/window keystrokes; each also labels its menu item.
    cx.bind_keys([
        KeyBinding::new("secondary-h", HideApp, None),
        KeyBinding::new("alt-secondary-h", HideOthers, None),
        KeyBinding::new("secondary-m", Minimize, None),
        KeyBinding::new("ctrl-shift-tab", ShowPreviousTab, None),
        KeyBinding::new("ctrl-tab", ShowNextTab, None),
    ]);

    cx.set_menus(vec![
        Menu::new("Console").items([
            MenuItem::action("About Console", About),
            MenuItem::action("Check for Updates…", CheckForUpdates),
            MenuItem::Separator,
            MenuItem::action("Settings...", OpenSettings),
            MenuItem::Separator,
            // AppKit populates this submenu with the system's registered
            // services (GPUI's mac backend wires setServicesMenu for it).
            MenuItem::os_submenu("Services", SystemMenuType::Services),
            MenuItem::Separator,
            MenuItem::action("Hide Console", HideApp),
            MenuItem::action("Hide Others", HideOthers),
            MenuItem::action("Show All", UnhideAll),
            MenuItem::Separator,
            MenuItem::action("Quit Console", Quit),
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
            MenuItem::Separator,
            MenuItem::action("Emoji & Symbols", ShowEmojiPicker),
        ]),
        // View menu owns ⌘B / ⌥B key equivalents so AppKit routes them
        // to our actions (same pattern as File for ⌘O / ⌘P).
        Menu::new("View").items([
            MenuItem::action("Toggle Left Sidebar", ToggleLeftSidebar),
            MenuItem::action("Toggle Right Sidebar", ToggleRightSidebar),
        ]),
        // GPUI's mac backend makes any menu named "Window" the app's windows
        // menu, so open windows (and tab items) list under ours.
        Menu::new("Window").items([
            MenuItem::action("Minimize", Minimize),
            MenuItem::action("Zoom", Zoom),
            MenuItem::Separator,
            MenuItem::action("Show Tab Bar", ToggleTabBar),
            MenuItem::action("Show Previous Tab", ShowPreviousTab),
            MenuItem::action("Show Next Tab", ShowNextTab),
            MenuItem::action("Move Tab to New Window", MoveTabToNewWindow),
            MenuItem::action("Merge All Windows", MergeAllWindows),
            MenuItem::Separator,
            MenuItem::action("Bring All to Front", BringAllToFront),
        ]),
        Menu::new("Help"),
    ]);
}
