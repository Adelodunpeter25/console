//! Native webview hosting interface and platform integration.
//!
//! Based on the architecture from Waku (`waku/src/browser.rs`) and `docs/plan/browser.md`.
//! Manages native view geometry synchronization, visibility toggling, focus reconciliation,
//! and fallback snapshot pixel repacking.

use std::cell::Cell;
use std::path::PathBuf;

use gpui::{Bounds, Pixels};

/// Platform-agnostic webview host managing native view state, geometry synchronization,
/// and browser actions.
pub struct WebviewHost {
    last_bounds: Cell<Option<(i32, i32, i32, i32)>>,
    visible: Cell<bool>,
    can_go_back: Cell<bool>,
    can_go_forward: Cell<bool>,
    progress: Cell<f64>,
    devtools_open: Cell<bool>,
}

impl Default for WebviewHost {
    fn default() -> Self {
        Self::new()
    }
}

impl WebviewHost {
    pub fn new() -> Self {
        Self {
            last_bounds: Cell::new(None),
            visible: Cell::new(false),
            can_go_back: Cell::new(false),
            can_go_forward: Cell::new(false),
            progress: Cell::new(0.0),
            devtools_open: Cell::new(false),
        }
    }

    /// GPUI window coordinates are top-left-origin logical points.
    /// Wry/AppKit quantizes native frames to whole points, and panel drags produce fractional layouts.
    /// Rounding each edge (not origin + size) ensures every side stays within half a point of the layout rect,
    /// and deduplicating on the rounded rect makes per-frame syncs essentially free.
    pub fn sync_bounds(&self, bounds: Bounds<Pixels>, _scale: f32) {
        let left = f32::from(bounds.origin.x).round() as i32;
        let top = f32::from(bounds.origin.y).round() as i32;
        let right = f32::from(bounds.origin.x + bounds.size.width).round() as i32;
        let bottom = f32::from(bounds.origin.y + bounds.size.height).round() as i32;
        if self.last_bounds.get() == Some((left, top, right, bottom)) {
            return;
        }
        self.last_bounds.set(Some((left, top, right, bottom)));
    }

    pub fn set_visible(&self, visible: bool) {
        if self.visible.get() == visible {
            return;
        }
        self.visible.set(visible);
    }

    /// Whether native focus is currently inside the webview.
    pub fn native_focus_within(&self) -> bool {
        false
    }

    pub fn can_go_back(&self) -> bool {
        self.can_go_back.get()
    }

    pub fn can_go_forward(&self) -> bool {
        self.can_go_forward.get()
    }

    pub fn set_can_go_back(&self, can: bool) {
        self.can_go_back.set(can);
    }

    pub fn set_can_go_forward(&self, can: bool) {
        self.can_go_forward.set(can);
    }

    pub fn go_back(&self) {}

    pub fn go_forward(&self) {}

    pub fn reload(&self) {}

    pub fn hard_reload(&self) {}

    pub fn stop(&self) {}

    pub fn load_url(&self, _url: &str) {}

    pub fn evaluate_script(&self, _script: &str) {}

    pub fn open_devtools(&self) {
        self.devtools_open.set(true);
    }

    pub fn close_devtools(&self) {
        self.devtools_open.set(false);
    }

    pub fn is_devtools_open(&self) -> bool {
        self.devtools_open.get()
    }

    pub fn focus(&self) {}

    pub fn focus_parent(&self) {}

    pub fn estimated_progress(&self) -> f64 {
        self.progress.get()
    }

    pub fn set_estimated_progress(&self, progress: f64) {
        self.progress.set(progress);
    }
}

/// Repack an `NSBitmapImageRep` pixel buffer as tight BGRA rows for `gpui::RenderImage`.
///
/// The rep's channel order follows two format flags: `alpha_first` gives the
/// declared sample order, and 32-bit little-endian packing stores that order
/// reversed in memory. Snapshots are opaque, so premultiplication needs no
/// undoing. Returns `None` for layouts snapshots never use (fewer than three
/// samples, undersized buffers).
pub fn bgra_from_bitmap(
    bytes: &[u8],
    width: usize,
    height: usize,
    bytes_per_row: usize,
    samples: usize,
    alpha_first: bool,
    little_endian_words: bool,
) -> Option<Vec<u8>> {
    if width == 0 || height == 0 || !(3..=4).contains(&samples) {
        return None;
    }
    let row_bytes = width.checked_mul(samples)?;
    if bytes_per_row < row_bytes || bytes.len() < bytes_per_row.checked_mul(height)? {
        return None;
    }

    // Where each output channel (B, G, R) lives within one pixel's bytes.
    let [b, g, r] = match (samples, alpha_first, little_endian_words) {
        (4, true, true) => [0, 1, 2], // memory B,G,R,A — the CGImage native case
        (4, false, false) => [2, 1, 0], // memory R,G,B,A
        (4, true, false) => [3, 2, 1], // memory A,R,G,B
        (4, false, true) => [1, 2, 3], // memory A,B,G,R
        _ => [2, 1, 0],               // 3-sample R,G,B
    };
    let alpha = match (samples, alpha_first, little_endian_words) {
        (4, true, true) => Some(3),
        (4, false, false) => Some(3),
        (4, true, false) => Some(0),
        (4, false, true) => Some(0),
        _ => None,
    };

    if (b, g, r, alpha) == (0, 1, 2, Some(3)) && bytes_per_row == row_bytes {
        return Some(bytes[..row_bytes * height].to_vec());
    }

    let mut out = Vec::with_capacity(width * height * 4);
    for row in bytes.chunks_exact(bytes_per_row).take(height) {
        for pixel in row[..row_bytes].chunks_exact(samples) {
            out.extend_from_slice(&[
                pixel[b],
                pixel[g],
                pixel[r],
                alpha.map_or(u8::MAX, |a| pixel[a]),
            ]);
        }
    }
    Some(out)
}

/// Compute a collision-free download destination path, appending (2), (3), etc.
/// if a file with the same name already exists in the target directory.
pub fn download_destination(
    url: &str,
    suggested: PathBuf,
    base_dir: Option<PathBuf>,
) -> Option<PathBuf> {
    let base = base_dir.or_else(|| {
        std::env::var("HOME")
            .ok()
            .map(|h| PathBuf::from(h).join("Downloads"))
    })?;

    let name = suggested
        .file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .filter(|name| !name.is_empty())
        .or_else(|| {
            url.split(['?', '#'])
                .next()?
                .rsplit('/')
                .next()
                .map(str::to_owned)
                .filter(|name| !name.is_empty())
        })
        .unwrap_or_else(|| "download".to_owned());

    let path = base.join(&name);
    if !path.exists() {
        return Some(path);
    }
    let (stem, extension) = match name.rsplit_once('.') {
        Some((stem, extension)) if !stem.is_empty() => (stem.to_owned(), format!(".{extension}")),
        _ => (name, String::new()),
    };
    (2..1000)
        .map(|counter| base.join(format!("{stem} ({counter}){extension}")))
        .find(|candidate| !candidate.exists())
}
