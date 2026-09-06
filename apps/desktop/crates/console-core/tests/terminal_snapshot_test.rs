//! Regression tests for the snapshot link scan in `TermyBackend`.
//!
//! `snapshot()` walks the grid and asks `link_at(r, c)` for the link under
//! each cell, then jumps the scan cursor to `end_col + 1`. For a link that
//! soft-wraps onto later rows, `end_col` is relative to the link's *final*
//! row — so the cursor can rewind and the scan spins forever. Real-world
//! trigger: `git init` prints a long absolute path that wraps at the
//! terminal width, and termy's heuristics classify file paths as links.
//!
//! These tests run `snapshot()` on a worker thread with a watchdog so a
//! regression fails the test instead of hanging CI.

use console_core::services::terminal::TermyBackend;
use console_core::types::terminal::{TerminalBackend, TerminalSize};
use std::sync::mpsc;
use std::thread;
use std::time::Duration;

const SNAPSHOT_TIMEOUT: Duration = Duration::from_secs(5);

/// Run `snapshot()` on a worker thread with an abort watchdog. A normal panic
/// in the watchdog can't interrupt a stuck worker, so on timeout we abort the
/// process with a distinctive message — the test run fails loudly instead of
/// hanging the suite forever.
fn snapshot_bounded(backend: &TermyBackend) -> console_core::types::terminal::TerminalGridSnapshot {
    let (tx, rx) = mpsc::channel();
    thread::scope(|scope| {
        let worker = scope.spawn(|| {
            let _ = tx.send(backend.snapshot());
        });
        match rx.recv_timeout(SNAPSHOT_TIMEOUT) {
            Ok(snapshot) => {
                worker.join().expect("snapshot worker panicked");
                snapshot
            }
            Err(_) => {
                eprintln!(
                    "snapshot() did not finish within {SNAPSHOT_TIMEOUT:?} — \
                     the grid link scan is stuck (likely a wrapped-link cursor rewind)"
                );
                std::process::abort();
            }
        }
    })
}

#[test]
fn snapshot_completes_after_git_init_style_wrapped_path() {
    let mut backend = TermyBackend::new(TerminalSize::new(80, 24));

    // Exactly what a shell prints for `git init` in a deeply nested project:
    // the absolute path soft-wraps at 80 columns, and termy's heuristics
    // classify file paths as file:// links spanning the wrapped rows.
    backend.advance("Initialized empty Git repository in /Users/someone/Developer/Projects/console/.git/\r\n");
    let _ = snapshot_bounded(&backend);

    // The redrawn prompt after the repo exists: git-aware prompts add a git
    // segment and often wrap OSC 8 hyperlinks around the cwd.
    backend.advance("\x1b[32m➜\x1b[0m \x1b]8;;file:///Users/someone/Developer/Projects/console\x1b\\console\x1b]8;;\x1b\\ \x1b[33mgit:(\x1b[31mmaster\x1b[33m)\x1b[0m \r\n");
    let snapshot = snapshot_bounded(&backend);

    // The path link must be detected and must span the wrapped rows.
    assert!(
        !snapshot.links.is_empty(),
        "expected the wrapped file path to be detected as a link"
    );
}

#[test]
fn snapshot_scan_never_rewinds_on_multi_row_links() {
    // An OSC 8 hyperlink whose *text* soft-wraps across row boundaries:
    // grid hyperlink metadata spans rows, so link_at() reports an end_col
    // that belongs to the link's final row. The scan cursor must never
    // rewind to it.
    let mut backend = TermyBackend::new(TerminalSize::new(80, 24));
    let long_target = "file:///Users/someone/Developer/Projects/console/repo";
    let link_text = "/Users/someone/Developer/Projects/console/repo/very/deeply/nested/directory/structure/that/well/past/one/line";
    for _ in 0..6 {
        backend.advance(&format!(
            "\x1b]8;;{long_target}\x1b\\{link_text}\x1b]8;;\x1b\\\r\n"
        ));
    }
    let snapshot = snapshot_bounded(&backend);
    assert!(!snapshot.links.is_empty());

    // Every detected link must still be a sane viewport range.
    for link in &snapshot.links {
        assert!(link.start_row <= link.end_row);
        assert!(link.end_row < 24);
    }
}
