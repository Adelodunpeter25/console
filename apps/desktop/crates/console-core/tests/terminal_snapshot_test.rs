//! Regression tests for the snapshot link scan in `TermyBackend`.
//!
//! Links are URL-only (http/https/www), detected from the rendered grid text.
//! termy's per-cell `link_at` scan is deliberately not used anymore: it walked
//! OSC 8 metadata and file-path heuristics (with filesystem canonicalization)
//! per cell, and a link that soft-wrapped across rows could rewind its scan
//! cursor and hang the snapshot forever — wedging that terminal (seen in the
//! wild after `git init`, which prints a long wrapping absolute path).
//!
//! These tests run `snapshot()` on a worker thread with a watchdog so a
//! regression fails loudly (SIGABRT) instead of hanging the suite forever.

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
                     the grid link scan is stuck"
                );
                std::process::abort();
            }
        }
    })
}

#[test]
fn snapshot_links_http_and_https_urls() {
    let mut backend = TermyBackend::new(TerminalSize::new(80, 24));
    backend.advance("See https://example.com/docs and http://a.co, plus www.zed.dev.\r\n");
    let snapshot = snapshot_bounded(&backend);

    let targets: Vec<&str> = snapshot.links.iter().map(|l| l.target.as_str()).collect();
    assert!(targets.contains(&"https://example.com/docs"), "got {targets:?}");
    assert!(targets.contains(&"http://a.co"), "trailing comma must be trimmed, got {targets:?}");
    assert!(targets.contains(&"https://www.zed.dev"), "got {targets:?}");
}

#[test]
fn snapshot_ignores_file_paths_from_git_init_output() {
    // Exactly what a shell prints for `git init` in a deeply nested project:
    // the absolute path soft-wraps at 80 columns. It must NOT become a link,
    // and the snapshot must complete (this output used to hang the scan).
    let mut backend = TermyBackend::new(TerminalSize::new(80, 24));
    backend.advance("Initialized empty Git repository in /Users/someone/Developer/Projects/console/.git/\r\n");
    backend.advance("\x1b[32m➜\x1b[0m \x1b[36mconsole\x1b[0m \x1b[33mgit:(\x1b[31mmaster\x1b[33m)\x1b[0m \r\n");
    let snapshot = snapshot_bounded(&backend);

    assert!(
        snapshot.links.is_empty(),
        "file paths must not be links, got {:?}",
        snapshot.links
    );
}

#[test]
fn snapshot_ignores_osc8_hyperlinks_even_when_wrapped() {
    // An OSC 8 hyperlink whose text soft-wraps across rows: OSC 8 metadata is
    // ignored entirely (URLs only), and the old per-cell scan hung forever on
    // this exact input.
    let mut backend = TermyBackend::new(TerminalSize::new(80, 24));
    let long_target = "file:///Users/someone/Developer/Projects/console/repo";
    let link_text = "/Users/someone/Developer/Projects/console/repo/very/deeply/nested/directory/structure/that/well/past/one/line";
    for _ in 0..6 {
        backend.advance(&format!("\x1b]8;;{long_target}\x1b\\{link_text}\x1b]8;;\x1b\\\r\n"));
    }
    let snapshot = snapshot_bounded(&backend);
    assert!(
        snapshot.links.is_empty(),
        "OSC 8 targets must not be links, got {:?}",
        snapshot.links
    );
}
