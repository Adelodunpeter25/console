//! Native file pickers — frontend replacement for the removed
//! `POST /api/fs/pick-folder` / `pick-file` osascript endpoints.
//!
//! `rfd::AsyncFileDialog` shows the platform's native dialog (NSOpenPanel on
//! macOS) directly from the desktop process. No HTTP round-trip, no child
//! process, and the dialog is parented correctly. Cancellation returns `None`
//! and is treated as a silent no-op by callers, matching the previous
//! backend's `cancelled:true` → empty path contract.

/// Image extensions accepted as composer attachments. Kept in one place so
/// `pick_file` and `image_mime_from_path` stay consistent.
const IMAGE_FILTER_EXTS: &[&str] = &[
    "png", "jpg", "jpeg", "gif", "webp", "bmp", "svg", "tif", "tiff", "ico", "avif", "heic", "heif",
];

/// Open the native folder picker. Returns `None` when the user dismisses the
/// dialog. This is the frontend replacement for `FsService::pick_folder`.
pub async fn pick_folder() -> Option<String> {
    rfd::AsyncFileDialog::new()
        .set_title("Select Project Folder")
        .pick_folder()
        .await
        .map(|handle| handle.path().to_string_lossy().to_string())
}

/// Open the native file picker filtered to images. Returns `None` on cancel.
/// Replaces `FsService::pick_file` for the composer attachment flow.
pub async fn pick_image_file() -> Option<String> {
    rfd::AsyncFileDialog::new()
        .set_title("Select an image")
        .add_filter("Images", IMAGE_FILTER_EXTS)
        .pick_file()
        .await
        .map(|handle| handle.path().to_string_lossy().to_string())
}
