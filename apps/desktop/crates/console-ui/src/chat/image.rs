//! Workspace image loading for chat: `image` visual blocks and markdown
//! `![alt](path)` both resolve a path against the session cwd and fetch the
//! bytes through the authenticated fs endpoint, because GPUI's `img` cannot
//! attach the session's Bearer token.
//!
//! The client, decoded images, and the preview-zoom handler live in globals
//! so deeply nested markdown renderers (which have no access to app state)
//! can fetch and zoom without plumbing.

use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::sync::{Arc, LazyLock, Mutex};

use console_core::{ConsoleClient, FsService};
use gpui::{
    AnyElement, App, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, StyledImage, div, img, px,
};

use crate::theme::Theme;

/// Invoked with the decoded image when the user clicks a workspace image,
/// opening the app's image preview modal. Send+Sync so the shared registry
/// can hand it to any thread.
pub type ImagePreviewHandler =
    std::sync::Arc<dyn Fn(Arc<gpui::Image>, &mut gpui::Window, &mut App) + Send + Sync + 'static>;

/// Where a workspace image stands.
#[derive(Clone)]
pub enum WorkspaceImageState {
    /// Decoded and ready to paint.
    Ready(Arc<gpui::Image>),
    /// A fetch is running; the card shows a placeholder until repaint.
    Loading,
    /// The fetch failed or the path is not a fetchable image.
    Failed,
}

struct ImageRegistry {
    fs: Option<FsService>,
    preview: Option<ImagePreviewHandler>,
    states: HashMap<String, WorkspaceImageState>,
    in_flight: std::collections::HashSet<String>,
}

static REGISTRY: LazyLock<Mutex<ImageRegistry>> = LazyLock::new(|| {
    Mutex::new(ImageRegistry {
        fs: None,
        preview: None,
        states: HashMap::new(),
        in_flight: std::collections::HashSet::new(),
    })
});

/// Point image fetching at the session's client. Called once the app's
/// client exists (app boot and after server reconfiguration).
pub fn set_workspace_image_client(client: &ConsoleClient) {
    if let Ok(mut reg) = REGISTRY.lock() {
        reg.fs = Some(client.fs.clone());
    }
}

/// The transcript's image-preview modal handler, shared so markdown and
/// visual cards can offer click-to-zoom without prop drilling.
pub fn set_workspace_image_preview(handler: Option<ImagePreviewHandler>) {
    if let Ok(mut reg) = REGISTRY.lock() {
        reg.preview = handler;
    }
}

/// The registered preview handler, if the transcript wired one.
pub fn workspace_image_preview() -> Option<ImagePreviewHandler> {
    REGISTRY.lock().ok()?.preview.clone()
}

/// Classify a markdown `![alt](url)` / image-block target: `Some(path)` when
/// it names a workspace file (`/abs`, `~/…`, `file://…`, or a bare relative
/// path), `None` for remote/data URLs that keep their own loaders.
pub fn workspace_image_target(url: &str) -> Option<String> {
    let trimmed = url.trim();
    if trimmed.is_empty() || trimmed.starts_with("data:") {
        return None;
    }
    if trimmed.starts_with("http://") || trimmed.starts_with("https://") {
        return None;
    }
    if let Some(rest) = trimmed.strip_prefix("file://") {
        // file:///abs/path or file://localhost/abs/path
        let path = rest.strip_prefix("localhost").unwrap_or(rest);
        return Some(percent_decode(path));
    }
    if trimmed.contains("://") {
        return None;
    }
    Some(percent_decode(trimmed))
}

/// Resolve a workspace path against the session cwd: absolute and `~` paths
/// stay, relative paths join `cwd`, and the result is cleaned.
pub fn resolve_workspace_path(path: &str, cwd: Option<&str>) -> String {
    let mut p = path.trim().to_string();
    if p == "~" {
        if let Some(home) = dirs_home() {
            p = home;
        }
    } else if let Some(rest) = p.strip_prefix("~/").or_else(|| p.strip_prefix("~\\"))
        && let Some(home) = dirs_home()
    {
        p = format!("{home}/{rest}");
    }
    let candidate = if Path::new(&p).is_absolute() {
        p
    } else if let Some(cwd) = cwd.filter(|c| !c.is_empty()) {
        format!("{}/{}", cwd.trim_end_matches('/'), p)
    } else {
        p
    };
    PathBuf::from(&candidate)
        .canonicalize()
        .map(|c| c.to_string_lossy().into_owned())
        .unwrap_or(candidate)
}

fn dirs_home() -> Option<String> {
    std::env::var_os("HOME").map(|h| h.to_string_lossy().into_owned())
}

/// Minimal percent-decoding for the escapes file URLs carry.
fn percent_decode(s: &str) -> String {
    let bytes = s.as_bytes();
    let mut out = Vec::with_capacity(bytes.len());
    let mut i = 0;
    while i < bytes.len() {
        if bytes[i] == b'%'
            && i + 2 < bytes.len()
            && let Ok(v) = u8::from_str_radix(&s[i + 1..i + 3], 16)
        {
            out.push(v);
            i += 3;
            continue;
        }
        out.push(bytes[i]);
        i += 1;
    }
    String::from_utf8_lossy(&out).into_owned()
}

/// The current state for an absolute workspace path, kicking off the fetch
/// when it has not run yet. `None` when no client is wired (tests, before
/// boot); callers then fall back to their placeholder.
pub fn workspace_image_state(path: &str, cx: &mut App) -> Option<WorkspaceImageState> {
    let fs = REGISTRY.lock().ok()?.fs.clone()?;
    let mut reg = REGISTRY.lock().ok()?;
    if let Some(state) = reg.states.get(path) {
        return Some(state.clone());
    }
    if !reg.in_flight.insert(path.to_string()) {
        return Some(WorkspaceImageState::Loading);
    }
    drop(reg);

    let (tx, rx) = tokio::sync::oneshot::channel();
    let fetch_path = path.to_string();
    let fetch_fs = fs;
    // The app holds a Tokio runtime for its whole lifetime (main.rs enters
    // it before gpui starts), so tokio::spawn works from render code.
    tokio::spawn(async move {
        let state = match fetch_fs.read_file_bytes(&fetch_path).await {
            Ok((bytes, content_type)) => decode_image(&bytes, &content_type)
                .map(WorkspaceImageState::Ready)
                .unwrap_or(WorkspaceImageState::Failed),
            Err(_) => WorkspaceImageState::Failed,
        };
        let _ = tx.send(state);
    });
    let cache_path = path.to_string();
    cx.spawn(async move |cx| {
        let state = rx.await.unwrap_or(WorkspaceImageState::Failed);
        cx.update(|cx| {
            if let Ok(mut reg) = REGISTRY.lock() {
                reg.in_flight.remove(&cache_path);
                reg.states.insert(cache_path.clone(), state);
            }
            cx.refresh_windows();
        });
    })
    .detach();
    Some(WorkspaceImageState::Loading)
}

/// Decode raw bytes into a `gpui::Image` when the content type (or magic
/// bytes) names a format GPUI can paint.
fn decode_image(bytes: &[u8], content_type: &str) -> Option<Arc<gpui::Image>> {
    let format = gpui::ImageFormat::from_mime_type(content_type).or_else(|| {
        if bytes.starts_with(&[0x89, 0x50, 0x4e, 0x47]) {
            Some(gpui::ImageFormat::Png)
        } else if bytes.starts_with(&[0xFF, 0xD8, 0xFF]) {
            Some(gpui::ImageFormat::Jpeg)
        } else if bytes.starts_with(b"GIF8") {
            Some(gpui::ImageFormat::Gif)
        } else if bytes.starts_with(b"RIFF") && bytes.len() > 12 && &bytes[8..12] == b"WEBP" {
            Some(gpui::ImageFormat::Webp)
        } else {
            None
        }
    })?;
    Some(Arc::new(gpui::Image::from_bytes(format, bytes.to_vec())))
}

/// A workspace image: bounded height, rounded corners, optional alt caption,
/// click-to-zoom via the registered preview handler. While loading it shows a
/// quiet placeholder; a failed fetch falls back to the alt/path text so
/// nothing silently disappears.
#[derive(gpui::IntoElement)]
pub struct ChatImage {
    path: String,
    alt: Option<String>,
    cwd: Option<String>,
    preview: Option<ImagePreviewHandler>,
}

impl ChatImage {
    pub fn new(path: impl Into<String>, cwd: Option<String>) -> Self {
        Self {
            path: path.into(),
            alt: None,
            cwd,
            preview: None,
        }
    }

    pub fn alt(mut self, alt: Option<String>) -> Self {
        self.alt = alt.filter(|a| !a.trim().is_empty());
        self
    }

    pub fn preview(mut self, handler: Option<ImagePreviewHandler>) -> Self {
        self.preview = handler;
        self
    }
}

impl RenderOnce for ChatImage {
    fn render(self, _window: &mut gpui::Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let abs = resolve_workspace_path(&self.path, self.cwd.as_deref());
        let state = workspace_image_state(&abs, cx);
        let caption = self.alt.clone().unwrap_or_else(|| {
            Path::new(&abs)
                .file_name()
                .map(|n| n.to_string_lossy().into_owned())
                .unwrap_or_else(|| abs.clone())
        });
        let ready_image = match &state {
            Some(WorkspaceImageState::Ready(image)) => Some(image.clone()),
            _ => None,
        };
        let preview = self.preview.clone().or_else(workspace_image_preview);

        let mut col = div().w_full().min_w_0().flex().flex_col().gap(px(4.0));
        match state {
            Some(WorkspaceImageState::Ready(_)) => {
                let image = ready_image.expect("ready state carries an image");
                let tile = div()
                    .id(gpui::ElementId::Name(format!("chat-image-{abs}").into()))
                    .max_w(gpui::relative(1.0))
                    .max_h(px(320.0))
                    .rounded(px(6.0))
                    .overflow_hidden()
                    .cursor_pointer()
                    .child(
                        img(image.clone())
                            .max_w(gpui::relative(1.0))
                            .max_h(px(320.0))
                            .object_fit(gpui::ObjectFit::ScaleDown)
                            .rounded(px(6.0)),
                    );
                let tile = match preview {
                    Some(handler) => tile.on_click(move |_, window, cx| {
                        (handler)(image.clone(), window, cx);
                        cx.stop_propagation();
                    }),
                    None => tile,
                };
                col = col.child(tile);
            }
            Some(WorkspaceImageState::Loading) => {
                col = col.child(
                    div()
                        .h(px(120.0))
                        .w_full()
                        .rounded(px(6.0))
                        .bg(theme.inset)
                        .flex()
                        .items_center()
                        .justify_center()
                        .child(
                            div()
                                .text_size(px(12.0))
                                .text_color(theme.text_tertiary)
                                .child("Loading image…"),
                        ),
                );
            }
            _ => {
                // No client wired or the fetch failed: keep the caption visible
                // so the reference is not silently dropped.
                col = col.child(
                    div()
                        .text_size(px(12.0))
                        .text_color(theme.text_tertiary)
                        .child(caption.clone()),
                );
            }
        }
        if !caption.is_empty() {
            col = col.child(
                div()
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child(caption),
            );
        }
        col.into_any_element()
    }
}

/// Convenience wrapper for markdown `render_image`: a workspace path becomes
/// a [ChatImage]; anything else returns `None` so the caller keeps its own
/// remote/data handling.
pub fn workspace_chat_image(
    url: &str,
    alt: &str,
    cwd: Option<&str>,
    preview: Option<ImagePreviewHandler>,
) -> Option<AnyElement> {
    let target = workspace_image_target(url)?;
    let abs = resolve_workspace_path(&target, cwd);
    Some(
        ChatImage::new(abs, None)
            .alt(Some(alt.to_string()))
            .preview(preview)
            .into_any_element(),
    )
}
