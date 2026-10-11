//! Native file and folder pickers, delegated to GPUI's platform prompt.
//!
//! `App::prompt_for_paths` drives `NSOpenPanel` through
//! `beginWithCompletionHandler:` — the ASYNCHRONOUS AppKit API. That detail is
//! the whole reason this file does not shell out or use `rfd`.
//!
//! DO NOT "simplify" these back to a blocking `rfd::FileDialog::pick_*`.
//! Those call `runModal`, which spins a nested AppKit run loop on the main
//! thread. That loop drains the main dispatch queue, which re-enters GPUI and
//! polls its own queued tasks — including `BlinkCursor::blink`, which calls
//! `AppCell::borrow_mut`. If the caller is inside `entity.update(...)` (which
//! holds that same `RefCell` borrow for its whole body), the re-entrant borrow
//! panics. Under the release profile's `panic = "abort"` that surfaces as
//! `EXC_BAD_ACCESS`, not a readable panic. It only reproduces when a blink
//! task happens to be pending, so it looks intermittent.
//!
//! `cx.defer` does NOT fix that: deferred callbacks are applied inside
//! `flush_effects`, which runs from inside `ApplicationHandle::update`, i.e.
//! still under the held borrow.
//!
//! So the API is deliberately SPLIT, and the split is the fix:
//!   - [`pick_folder`] / [`pick_image_file`] are SYNCHRONOUS. They only ask
//!     AppKit to show the panel and return a receiver, so calling them inside
//!     an `entity.update` is cheap and holds no borrow across any modal loop.
//!   - [`resolve_single_path`] is the async half. Awaiting it yields, and the
//!     dispatcher polls it later when no borrow is held.
//! Callers must await the second half OFF the UI's current stack.

use std::path::PathBuf;

use anyhow::Result as AnyResult;
use futures::channel::oneshot;
use gpui::{App, PathPromptOptions};

/// The platform's reply: the chosen paths, or `None` for cancel.
///
/// The inner error is gpui's `anyhow::Result` (the panel failing to open — a
/// Linux possibility). Awaiting the receiver yields a second layer,
/// `oneshot::Canceled`, for the sender going away. A user pressing Cancel
/// arrives as `Ok(None)`.
pub type PathReply = oneshot::Receiver<AnyResult<Option<Vec<PathBuf>>>>;

/// Open the native image picker. Returns once the panel is *shown* — not once
/// it is dismissed. Await the result with [`resolve_single_path`].
///
/// `PathPromptOptions` has no extension-filter field, so the panel cannot be
/// restricted to decodable types the way the old osascript prompt could. A
/// picked file that is not a supported image is still rejected by the caller
/// after the panel closes, exactly as before.
pub fn pick_image_file(cx: &mut App) -> PathReply {
    cx.prompt_for_paths(PathPromptOptions {
        files: true,
        directories: false,
        multiple: false,
        prompt: Some("Select an image".into()),
    })
}

/// Open the native folder picker. Returns once the panel is *shown* — not once
/// it is dismissed. Await the result with [`resolve_single_path`].
pub fn pick_folder(cx: &mut App) -> PathReply {
    cx.prompt_for_paths(PathPromptOptions {
        files: false,
        directories: true,
        multiple: false,
        prompt: Some("Select a folder".into()),
    })
}

/// Collapse a panel reply to one optional path.
///
/// Three shapes collapse to `None`: an explicit cancel, an empty selection,
/// and an error opening the panel (documented as possible on Linux).
pub async fn resolve_single_path(receiver: PathReply) -> Option<String> {
    let Ok(result) = receiver.await else {
        return None;
    };
    let paths = match result {
        Ok(paths) => paths,
        Err(error) => {
            log::warn!("native path prompt failed: {error}");
            return None;
        }
    };
    // `multiple: false`, so there is at most one — but the platform type is a
    // Vec, so take the first rather than assuming.
    let path = paths.into_iter().flatten().next()?;
    Some(to_path_string(path))
}

/// Render a picked path as a plain string, dropping a trailing separator so
/// the value we send to the server is a clean path.
fn to_path_string(path: PathBuf) -> String {
    let mut text = path.to_string_lossy().into_owned();
    while text.len() > 1 && text.ends_with('/') {
        text.pop();
    }
    text
}
