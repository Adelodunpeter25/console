//! End-to-end TUI launch test: boots the real server PTY stack (Bun fixture)
//! and drives it through `TerminalService` exactly like the desktop app, then
//! launches a real full-screen TUI (`vx`) and requires it to actually start
//! drawing (alternate screen). Guards against startup hangs where a TUI
//! blocks on a terminal query that never gets answered.

use console_core::services::terminal::TerminalService;
use console_core::types::terminal::{TerminalBackend, TerminalSize, TerminalSpawnParams};
use console_core::utils::HttpTransport;
use std::process::Stdio;
use std::time::Duration;

const FIXTURE_PORT: u16 = 47631;

/// True once the grid shows any non-space text.
async fn grid_has_content(
    handle: &console_core::services::terminal::TerminalHandle,
) -> bool {
    let b = handle.backend.lock().await;
    let snap = b.snapshot();
    snap.rows.iter().any(|row| row.iter().any(|c| c.c != ' '))
}

async fn wait_for_grid_content(
    handle: &console_core::services::terminal::TerminalHandle,
    seconds: u64,
) -> bool {
    for _ in 0..seconds * 10 {
        if grid_has_content(handle).await {
            return true;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    false
}

#[tokio::test(flavor = "multi_thread")]
async fn real_tui_launches_and_draws_through_the_full_stack() {
    // Boot the real server PTY stack (Bun fixture, no auth).
    let mut fixture = tokio::process::Command::new("bun")
        .args(["tests/terminal-e2e-fixture.ts", &FIXTURE_PORT.to_string()])
        .current_dir(concat!(env!("CARGO_MANIFEST_DIR"), "/../../../server"))
        .stdout(Stdio::piped())
        .kill_on_drop(true)
        .spawn()
        .expect("failed to start bun terminal fixture");
    tokio::time::timeout(Duration::from_secs(15), async {
        use tokio::io::AsyncBufReadExt;
        let stdout = fixture.stdout.take().expect("fixture stdout");
        let mut lines = tokio::io::BufReader::new(stdout).lines();
        while let Some(line) = lines.next_line().await.unwrap() {
            if line.starts_with("READY") {
                return;
            }
        }
        panic!("fixture exited without READY");
    })
    .await
    .expect("fixture did not become ready");

    let service = TerminalService::new(HttpTransport::new(Some(format!(
        "http://127.0.0.1:{FIXTURE_PORT}"
    ))));

    let handle = service
        .spawn(
            TerminalSpawnParams {
                cwd: "/tmp".into(),
                shell: Some("/bin/bash".into()),
                cols: Some(120),
                rows: Some(40),
                label: None,
            },
            TerminalSize::new(120, 40),
        )
        .await
        .expect("terminal spawn failed");

    // Wait for the shell prompt, then launch the TUI.
    assert!(
        wait_for_grid_content(&handle, 10).await,
        "shell prompt never appeared"
    );
    handle.send_input("vx\r");

    // A healthy launch enters the alternate screen within 6s; a TUI hung on
    // an unanswered query never gets there.
    let started = for_secs(Duration::from_secs(6), &handle).await;
    if !started {
        // Debug: dump the grid so we can see where the TUI is stuck.
        let b = handle.backend.lock().await;
        let snap = b.snapshot();
        for (i, row) in snap.rows.iter().enumerate() {
            let text: String = row.iter().map(|c| c.c).collect();
            eprintln!("GRID {i:02} |{text}|");
        }
        eprintln!("ALT_SCREEN: {}", b.is_alt_screen());
        eprintln!("MOUSE_MODE: {:?}", b.mouse_mode());
    }
    handle.kill();

    assert!(started, "vx did not reach the alternate screen within 6s — TUI startup is hanging");
}

/// Poll until the terminal enters the alternate screen or the budget runs out.
async fn for_secs(budget: Duration, handle: &console_core::services::terminal::TerminalHandle) -> bool {
    let mut elapsed = Duration::ZERO;
    while elapsed < budget {
        let alt = {
            let b = handle.backend.lock().await;
            b.is_alt_screen()
        };
        if alt {
            return true;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
        elapsed += Duration::from_millis(100);
    }
    false
}
