//! Terminal keybindings: Tab must reach the PTY, not the focus system.
//!
//! `gpui-component`'s `Root` binds `tab` globally to focus traversal, and
//! keymap actions dispatch before element `on_key_down` handlers — so without
//! these deeper-context bindings, every Tab press moves focus instead of
//! completing in the shell. `Terminal` sits deeper than `Root` in the context
//! stack, therefore wins precedence, and stops propagation after sending.

use gpui::{App, KeyBinding, actions};

actions!(terminal, [TerminalTab, TerminalShiftTab]);

pub const TERMINAL_KEY_CONTEXT: &str = "Terminal";

pub fn init_terminal_keybindings(cx: &mut App) {
    cx.bind_keys([
        KeyBinding::new("tab", TerminalTab, Some(TERMINAL_KEY_CONTEXT)),
        KeyBinding::new("shift-tab", TerminalShiftTab, Some(TERMINAL_KEY_CONTEXT)),
    ]);
}
