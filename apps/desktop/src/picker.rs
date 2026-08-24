//! Native file pickers — frontend replacement for the removed
//! `POST /api/fs/pick-folder` / `pick-file` osascript endpoints.
//!
//! The previous backend spawned `osascript` via HTTP. This runs the same
//! `osascript` directly in the desktop process — no HTTP round-trip, no
//! `rfd`/`objc2` retain crash on macOS 15.4 (see `rfd` 0.15 + `objc2` 0.5
//! `openPanel` `EXC_BAD_ACCESS`). Cancellation returns `None`, matching the
//! backend's `cancelled:true` → empty path contract.

/// Open the native folder picker via `osascript`. Returns `None` when the user
/// dismisses the dialog. Runs outside the GPUI main thread so the UI stays
/// responsive while the modal is open.
pub async fn pick_folder() -> Option<String> {
    pick_via_osascript(true).await
}

/// Open the native file picker via `osascript`. Returns `None` on cancel.
pub async fn pick_image_file() -> Option<String> {
    pick_via_osascript(false).await
}

async fn pick_via_osascript(is_folder: bool) -> Option<String> {
    #[cfg(not(target_os = "macos"))]
    {
        let _ = is_folder;
        log::warn!("Native picker via osascript is macOS-only");
        return None;
    }

    #[cfg(target_os = "macos")]
    {
        let script = if is_folder {
            r#"POSIX path of (choose folder with prompt "Select Project Folder")"#
        } else {
            r#"POSIX path of (choose file with prompt "Select an image")"# 
        };

        // `osascript` blocks until the user picks or cancels. Run it as a
        // Tokio child process so we don't freeze the GPUI main thread.
        let output = tokio::process::Command::new("osascript")
            .arg("-e")
            .arg(script)
            .output()
            .await
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
            // User cancelled is not an error — silent no-op.
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
