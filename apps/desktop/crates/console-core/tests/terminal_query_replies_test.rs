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
    assert!(text.starts_with("\x1b[?"), "expected a DA1 reply, got {text:?}");
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
