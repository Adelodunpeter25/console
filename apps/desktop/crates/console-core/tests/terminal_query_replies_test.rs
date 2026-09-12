//! Regression tests for shell device-query replies.
//!
//! The desktop client parses PTY output client-side with a display-only
//! termy backend. When the shell asks the terminal a question (cursor
//! position report, primary device attributes, kitty keyboard protocol
//! query), the answer must come back to us as reply bytes and be forwarded
//! to the server as terminal input — otherwise the shell blocks waiting for
//! it and tab completion hangs for minutes. These tests pin the behavior
//! that `TermyBackend::advance_and_collect_replies` produces those answers.

use console_core::services::terminal::TermyBackend;
use console_core::types::terminal::{TerminalBackend, TerminalSize};

fn backend() -> TermyBackend {
    TermyBackend::new(TerminalSize::new(80, 24))
}

#[test]
fn cursor_position_report_query_produces_a_reply() {
    let mut backend = backend();
    // CSI 6n — "report active cursor position", sent by zsh/bash completion.
    let replies = backend.advance_and_collect_replies("\x1b[6n");
    assert!(!replies.is_empty(), "CSI 6n must be answered");
    let text = String::from_utf8_lossy(&replies).into_owned();
    assert!(
        text.starts_with("\x1b[") && text.ends_with('R'),
        "expected a cursor-position report, got {text:?}"
    );
}

#[test]
fn device_attributes_query_produces_a_reply() {
    let mut backend = backend();
    // CSI c — "primary device attributes".
    let replies = backend.advance_and_collect_replies("\x1b[c");
    assert!(!replies.is_empty(), "DA1 must be answered");
    let text = String::from_utf8_lossy(&replies).into_owned();
    assert!(
        text.starts_with("\x1b[?"),
        "expected a DA1 reply, got {text:?}"
    );
}

#[test]
fn kitty_keyboard_protocol_query_produces_a_reply() {
    let mut backend = backend();
    // CSI ? u — "kick the kitty keyboard protocol".
    let replies = backend.advance_and_collect_replies("\x1b[?u");
    assert!(!replies.is_empty(), "kitty keyboard query must be answered");
}

#[test]
fn plain_output_produces_no_replies() {
    let mut backend = backend();
    let replies = backend.advance_and_collect_replies("hello world\r\n");
    assert!(replies.is_empty(), "plain output must not generate input");
}

/// Battery of queries real TUIs send at startup (Go frameworks: vaxis,
/// bubbletea/termenv, tcell). Any unanswered query here can hang a TUI at
/// launch, since many block on the reply.
#[test]
fn go_tui_startup_query_battery_all_get_replies() {
    let queries: &[(&str, &[u8])] = &[
        ("CSI 6n cursor position", b"\x1b[6n"),
        ("DA1 primary attributes", b"\x1b[c"),
        ("DA2 secondary attributes", b"\x1b[>c"),
        // NOTE: DA3 (`CSI =c`) is deliberately NOT in this battery — real TUIs
        // don't send it, and most terminals ignore it.
        ("DECRQM 2026 synchronized output", b"\x1b[?2026$p"),
        ("DECRQM 1006 SGR mouse", b"\x1b[?1006$p"),
        ("kitty keyboard query", b"\x1b[?u"),
        ("CSI 14t pixel size", b"\x1b[14t"),
        ("CSI 18t text area size", b"\x1b[18t"),
        // NOTE: CSI 16t (cell pixel size) is deliberately NOT in this battery —
        // real TUIs query 14t, tolerate 16t going unanswered, and the wrapped
        // alacritty parser emits no event for it.
        ("XTVERSION", b"\x1b[>0q"),
        ("OSC 11 background color", b"\x1b]11;?\x1b\\"),
        ("OSC 10 foreground color", b"\x1b]10;?\x1b\\"),
    ];
    for (name, query) in queries {
        let mut backend = backend();
        let replies = backend.advance_and_collect_replies_bytes(query);
        assert!(
            !replies.is_empty(),
            "{name} (debug: {query:?}) must be answered or TUIs hang at startup"
        );
    }
}
