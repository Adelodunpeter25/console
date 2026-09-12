//! Browser keybinding actions and action handlers.

use gpui::{App, KeyBinding, actions};

actions!(
    browser,
    [
        BrowserBack,
        BrowserForward,
        BrowserReload,
        BrowserHardReload,
        BrowserStop,
        BrowserDevtools,
        FocusBrowserAddress,
        BrowserAddressCancel,
        WebviewCopy,
        WebviewCut,
        WebviewPaste,
        WebviewSelectAll,
    ]
);

pub const BROWSER_KEY_CONTEXT: &str = "Browser";
pub const BROWSER_ADDRESS_KEY_CONTEXT: &str = "BrowserAddress";

/// Register default keyboard shortcuts for browser navigation and editing.
pub fn init_browser_keybindings(cx: &mut App) {
    cx.bind_keys([
        // Browser navigation shortcuts
        KeyBinding::new("cmd-[", BrowserBack, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("ctrl-[", BrowserBack, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("cmd-]", BrowserForward, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("ctrl-]", BrowserForward, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("cmd-r", BrowserReload, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("ctrl-r", BrowserReload, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("cmd-shift-r", BrowserHardReload, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("ctrl-shift-r", BrowserHardReload, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("escape", BrowserStop, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("cmd-shift-i", BrowserDevtools, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("cmd-shift-I", BrowserDevtools, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("ctrl-shift-i", BrowserDevtools, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("ctrl-shift-I", BrowserDevtools, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("cmd-alt-i", BrowserDevtools, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("cmd-l", FocusBrowserAddress, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("ctrl-l", FocusBrowserAddress, Some(BROWSER_KEY_CONTEXT)),
        // Address bar escape cancels editing and reverts to current URL
        KeyBinding::new(
            "escape",
            BrowserAddressCancel,
            Some(BROWSER_ADDRESS_KEY_CONTEXT),
        ),
        // In-webview editing bindings when Browser context has precedence
        KeyBinding::new("cmd-c", WebviewCopy, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("ctrl-c", WebviewCopy, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("cmd-x", WebviewCut, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("ctrl-x", WebviewCut, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("cmd-v", WebviewPaste, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("ctrl-v", WebviewPaste, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("cmd-a", WebviewSelectAll, Some(BROWSER_KEY_CONTEXT)),
        KeyBinding::new("ctrl-a", WebviewSelectAll, Some(BROWSER_KEY_CONTEXT)),
    ]);
}
