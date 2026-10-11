//! Native file and folder pickers.
//!
//! Backed by `rfd`, which drives NSOpenPanel directly through objc2. This
//! replaced an earlier osascript implementation that shelled out to
//! `choose file` / `choose folder` and parsed stdout — spawning a process per
//! pick, and giving a different (non-native-feeling) panel.
//!
//! THREADING — read before moving these calls off the main thread.
//! These functions BLOCK, and on macOS `NSOpenPanel` must be driven from the
//! main thread. The previous crash (EXC_BAD_ACCESS inside `openPanel`) came
//! from calling the picker on a background executor. `runModal` spins its own
//! nested AppKit loop, so although the calling thread is parked, AppKit keeps
//! servicing events and the window keeps painting. That is why calling this
//! directly from GPUI is correct and a background spawn is not.

use std::path::PathBuf;

/// Image types `read_image_attachment` can decode. Filtering the panel to
/// these means a picked file is almost always one the caller can actually
/// use, instead of failing after the dialog closes.
const IMAGE_EXTENSIONS: &[&str] = &["png", "jpg", "jpeg", "gif", "webp"];

/// Open the native image picker. Returns `None` on cancel.
///
/// Must run on the main thread; see the module comment.
pub fn pick_image_file_blocking() -> Option<String> {
    rfd::FileDialog::new()
        .set_title("Select an image")
        .add_filter("Images", IMAGE_EXTENSIONS)
        .pick_file()
        .map(|path| to_path_string(path))
}

/// Open the native folder picker. Returns `None` on cancel.
///
/// Must run on the main thread; see the module comment.
pub fn pick_folder_blocking() -> Option<String> {
    rfd::FileDialog::new()
        .set_title("Select a folder")
        .pick_folder()
        .map(|path| to_path_string(path))
}

/// Render a picked path as a plain string, dropping the trailing separator.
///
/// `rfd` hands back a `PathBuf`; a folder chosen in Finder usually carries no
/// trailing slash, but stripping one keeps the value we send to the server
/// identical to what the old osascript path produced.
fn to_path_string(path: PathBuf) -> String {
    let mut text = path.to_string_lossy().into_owned();
    while text.len() > 1 && text.ends_with('/') {
        text.pop();
    }
    text
}