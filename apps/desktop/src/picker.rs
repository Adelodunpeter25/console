//! Native file picker — frontend replacement for the removed
//! `POST /api/fs/pick-file` osascript endpoint. (Folder selection now uses the
//! in-app ⌘O directory browser palette instead.)
//!
//! The previous backend spawned `osascript` via HTTP. This runs the same
//! `osascript` directly in the desktop process — no HTTP round-trip, no
//! `rfd`/`objc2` retain crash on macOS 15.4 (see `rfd` 0.15 + `objc2` 0.5
//! `openPanel` `EXC_BAD_ACCESS`). Cancellation returns `None`, matching the
//! backend's `cancelled:true` → empty path contract.

/// Open the native file picker via `osascript`. Returns `None` on cancel.
pub fn pick_image_file_blocking() -> Option<String> {
    pick_via_osascript_blocking()
}

fn pick_via_osascript_blocking() -> Option<String> {
    #[cfg(not(target_os = "macos"))]
    {
        log::warn!("Native picker via osascript is macOS-only");
        return None;
    }

    #[cfg(target_os = "macos")]
    {
        let script = r#"POSIX path of (choose file with prompt "Select an image")"#;

        // `osascript` is a separate process that shows its own modal. Running
        // it synchronously on a background thread avoids freezing the GPUI main
        // thread and avoids the `rfd`/`objc2` retain crash and the
        // `tokio::process` future that never completes when polled by GPUI's
        // executor (previous freeze).
        let output = std::process::Command::new("osascript")
            .arg("-e")
            .arg(script)
            .output()
            .ok()?;

        if output.status.success() {
            let path = String::from_utf8_lossy(&output.stdout).trim().to_string();
            let path = path.trim_end_matches('/').to_string();
            if path.is_empty() {
                None
            } else {
                Some(path)
            }
        } else {
            let stderr = String::from_utf8_lossy(&output.stderr);
            let stdout = String::from_utf8_lossy(&output.stdout);
            let combined = format!("{stderr} {stdout}");
            let cancelled = combined.contains("-128")
                || combined.to_ascii_lowercase().contains("user canceled");
            if cancelled {
                None
            } else {
                log::warn!("osascript picker failed: {combined}");
                None
            }
        }
    }
}
