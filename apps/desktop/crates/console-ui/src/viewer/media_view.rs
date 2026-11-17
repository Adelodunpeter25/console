//! Media file viewer: video/audio/PDF inside an embedded WKWebView page
//! served from the server's `/api/fs/file/media` endpoint, plus the fallback
//! panel when the native surface cannot be created.
//!
//! The webview loads a small HTML document whose media element points at the
//! media URL (which carries `?token=` because webviews cannot send an
//! Authorization header on subresource requests either). PDF rides in a
//! full-bleed iframe so WKWebView's PDF plugin renders it while range
//! requests keep working.
//!
//! Mirrors the `DeviceViewer` hosting pattern: the native surface is sized by
//! a layout `canvas`, hidden when the tab is not the active file tab or a GPUI
//! overlay is open, and keyboard focus is reconciled in both directions.

use std::rc::Rc;

use gpui::{
    App, Context, FocusHandle, Focusable, HitboxBehavior, InteractiveElement, IntoElement,
    ParentElement, Render, SharedString, StatefulInteractiveElement, Styled, Window, canvas, div,
    px,
};

use crate::browser::host::{HostCallbacks, HostContent, SystemWebviewHost as WebviewHost};
use crate::primitives::tooltip::Tooltip;
use crate::primitives::{FileTypeIcon, IconName, app_icon, file_type_icon};
use crate::theme::Theme;
use crate::viewer::BlockedFilePanel;

/// Which inline player a media file tab needs.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MediaFileKind {
    Video,
    Audio,
    Pdf,
}

impl MediaFileKind {
    pub fn label(self) -> &'static str {
        match self {
            Self::Video => "Video",
            Self::Audio => "Audio",
            Self::Pdf => "PDF",
        }
    }

    pub fn icon(self) -> IconName {
        IconName::FileType(match self {
            Self::Video => FileTypeIcon::Video,
            Self::Audio => FileTypeIcon::Audio,
            Self::Pdf => FileTypeIcon::Pdf,
        })
    }
}

fn html_escape(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('"', "&quot;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
}

/// The player page embedded in the webview. `media_url` must already be
/// absolute and carry its `?token=` query — the document has no other way to
/// authenticate.
pub fn media_page_html(kind: MediaFileKind, media_url: &str) -> String {
    let src = html_escape(media_url);
    let body = match kind {
        MediaFileKind::Video => format!(
            r#"<video controls playsinline preload="metadata" src="{src}" style="max-width:100%;max-height:100vh;background:#000"></video>"#
        ),
        MediaFileKind::Audio => format!(
            r#"<audio controls preload="metadata" src="{src}" style="width:min(480px,90vw)"></audio>"#
        ),
        MediaFileKind::Pdf => format!(
            r#"<iframe src="{src}" style="position:fixed;inset:0;width:100%;height:100%;border:0;background:#525659"></iframe>"#
        ),
    };
    format!(
        r#"<!doctype html><html><head><meta charset="utf-8"><style>
html,body{{margin:0;height:100%;background:#1e1e1e;display:flex;align-items:center;justify-content:center}}
body{{flex-direction:column;gap:24px}}
</style></head><body>{body}</body></html>"#
    )
}

/// Percent-encode for embedding a `data:` URL in `load_url` (used when the
/// media URL changes after creation — rare, e.g. a server switch).
fn data_url_for_html(html: &str) -> String {
    let mut out = String::with_capacity(html.len() + 32);
    out.push_str("data:text/html;charset=utf-8,");
    for b in html.bytes() {
        match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                out.push(b as char)
            }
            _ => out.push_str(&format!("%{b:02X}")),
        }
    }
    out
}


/// One file tab's media surface. Created per path by the app shell and reused
/// while the tab is open; `close` hides and detaches the native view.
pub struct FileMediaView {
    focus_handle: FocusHandle,
    path: String,
    title: String,
    kind: MediaFileKind,
    /// Absolute `/api/fs/file/media` URL including `?token=` when auth is on.
    media_url: String,
    host: Option<Rc<WebviewHost>>,
    host_error: Option<String>,
    occluded: bool,
    was_natively_focused: bool,
    last_window_focus: Option<FocusHandle>,
}

impl FileMediaView {
    pub fn new(
        path: impl Into<String>,
        title: impl Into<String>,
        kind: MediaFileKind,
        media_url: impl Into<String>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Self {
        let mut this = Self {
            focus_handle: cx.focus_handle(),
            path: path.into(),
            title: title.into(),
            kind,
            media_url: media_url.into(),
            host: None,
            host_error: None,
            occluded: false,
            was_natively_focused: false,
            last_window_focus: None,
        };
        this.build_webview(window, cx);
        this
    }

    /// Point the surface at a (possibly new) media URL — used when the tab is
    /// reopened or the backend origin/token changed.
    pub fn set_media_url(&mut self, url: String, _window: &mut Window, cx: &mut Context<Self>) {
        if url == self.media_url {
            return;
        }
        self.media_url = url;
        if let Some(host) = &self.host {
            host.load_url(&data_url_for_html(&media_page_html(self.kind, &self.media_url)));
        }
        cx.notify();
    }

    #[cfg(target_os = "macos")]
    fn build_webview(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let on_responder_change: Box<dyn Fn(bool)> = {
            let executor = cx.foreground_executor().clone();
            let async_cx = cx.to_async();
            let view = cx.entity().downgrade();
            let window_handle = window.window_handle();
            Box::new(move |user_gesture| {
                let mut cx = async_cx.clone();
                let view = view.clone();
                executor
                    .spawn(async move {
                        let _ = window_handle.update(&mut cx, |_, window, cx| {
                            let _ = view.update(cx, |this, cx| {
                                this.native_responder_changed(user_gesture, window, cx);
                            });
                        });
                    })
                    .detach();
            })
        };

        let content = HostContent {
            user_agent: None,
            initialization_script: None,
            html: Some(media_page_html(self.kind, &self.media_url)),
            allow_navigation: None,
            on_ipc: None,
            on_title: None,
            on_new_window_url: None,
        };
        let callbacks = HostCallbacks {
            on_responder_change,
            on_nav_start: Box::new(|| {}),
            on_nav_finish: Box::new(|| {}),
            on_nav_error: Box::new(|_err| {}),
        };
        match WebviewHost::create(window, content, callbacks) {
            Ok(host) => {
                self.host = Some(Rc::new(host));
            }
            Err(error) => {
                self.host_error = Some(error);
            }
        }
    }

    #[cfg(not(target_os = "macos"))]
    fn build_webview(&mut self, _window: &mut Window, _cx: &mut Context<Self>) {
        self.host_error = Some("Media preview is currently supported on macOS.".to_string());
    }

    /// Push from the app: whether this surface's file tab is the visible one,
    /// and whether a GPUI overlay is open above it.
    pub fn sync_native_state(
        &mut self,
        surface_visible: bool,
        occluded: bool,
        cx: &mut Context<Self>,
    ) {
        let occluded = occluded || self.occluded;
        self.occluded = occluded;
        let show = surface_visible && !occluded;
        let Some(host) = self.host.clone() else {
            return;
        };
        if !show && host.native_focus_within() {
            self.reclaim_native_keyboard(cx);
        }
        host.set_visible(show);
    }

    pub fn reclaim_native_keyboard(&mut self, cx: &mut Context<Self>) {
        if let Some(host) = self.host.clone() {
            cx.foreground_executor()
                .spawn(async move {
                    host.focus_parent();
                })
                .detach();
        }
    }

    fn reconcile_focus(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let Some(host) = &self.host else {
            return;
        };
        let natively_focused = host.native_focus_within();
        let native_became_focused = natively_focused && !self.was_natively_focused;
        let window_focus = window.focused(cx);
        let window_focus_changed = window_focus != self.last_window_focus;
        if native_became_focused && !window_focus_changed {
            window.focus(&self.focus_handle, cx);
        }
        self.was_natively_focused = natively_focused;
        self.last_window_focus = window_focus;
    }

    fn native_responder_changed(
        &mut self,
        user_gesture: bool,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let natively_focused = self
            .host
            .as_ref()
            .is_some_and(|host| host.native_focus_within());
        if natively_focused {
            if user_gesture {
                window.focus(&self.focus_handle, cx);
            } else if !self.focus_handle.is_focused(window) && window.focused(cx).is_some() {
                self.reclaim_native_keyboard(cx);
            }
        }
        self.was_natively_focused = natively_focused;
    }

    /// Fully tear down the native surface when the tab closes or the view is
    /// evicted.
    pub fn close(&mut self, cx: &mut Context<Self>) {
        if let Some(host) = self.host.clone() {
            host.set_visible(false);
            self.reclaim_native_keyboard(cx);
        }
        cx.notify();
    }

    pub fn path(&self) -> &str {
        &self.path
    }

    fn render_fallback(&self, _theme: Theme) -> gpui::AnyElement {
        let message = self
            .host_error
            .clone()
            .unwrap_or_else(|| "The media preview could not start.".to_string());
        // BlockedFilePanel has no secondary action; the "Open externally"
        // affordance lives in the toolbar row, which renders regardless.
        BlockedFilePanel::new(format!("{} preview unavailable", self.kind.label()), message)
            .icon(self.kind.icon())
            .into_any_element()
    }

    fn render_video_area(&self, theme: Theme) -> gpui::AnyElement {
        let host = self.host.clone();
        div()
            .flex_1()
            .min_w_0()
            .h_full()
            .relative()
            .bg(theme.canvas)
            .child(
                canvas(
                    move |bounds, window, _| {
                        if let Some(host) = &host {
                            host.sync_bounds(bounds, window.scale_factor());
                        }
                        window.insert_hitbox(bounds, HitboxBehavior::Normal)
                    },
                    move |_, _hitbox, _window, _| {},
                )
                .absolute()
                .size_full(),
            )
            .into_any_element()
    }
}

impl Focusable for FileMediaView {
    fn focus_handle(&self, _: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Render for FileMediaView {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        self.reconcile_focus(window, cx);
        let has_host = self.host.is_some();
        div()
            .id("file-media-view")
            .track_focus(&self.focus_handle)
            .size_full()
            .flex()
            .flex_col()
            .bg(theme.surface)
            .child(
                div()
                    .flex_none()
                    .h(px(36.0))
                    .px(px(10.0))
                    .flex()
                    .items_center()
                    .gap(px(8.0))
                    .border_b_1()
                    .border_color(theme.border)
                    .bg(theme.canvas)
                    .child(file_type_icon(&self.path, 14.0))
                    .child(
                        div()
                            .flex_1()
                            .min_w_0()
                            .overflow_hidden()
                            .whitespace_nowrap()
                            .text_ellipsis()
                            .text_size(px(12.0))
                            .text_color(theme.text)
                            .child(SharedString::from(self.title.clone())),
                    )
                    .child(
                        div()
                            .flex_none()
                            .text_size(px(10.0))
                            .px(px(6.0))
                            .py(px(2.0))
                            .rounded_full()
                            .bg(theme.overlay)
                            .text_color(theme.text_secondary)
                            .child(self.kind.label()),
                    )
                    .child(self.toolbar_button(
                        "media-open-external",
                        IconName::ExternalLink,
                        true,
                        "Open in default browser",
                        theme,
                        |this, _window, cx| {
                            let url = this.media_url.clone();
                            cx.open_url(&url);
                        },
                        cx,
                    )),
            )
            .child(if has_host {
                self.render_video_area(theme)
            } else {
                self.render_fallback(theme)
            })
    }
}

impl FileMediaView {
    fn toolbar_button(
        &self,
        id: &'static str,
        icon: IconName,
        enabled: bool,
        tooltip: impl Into<SharedString>,
        theme: Theme,
        on_click: impl Fn(&mut Self, &mut Window, &mut Context<Self>) + 'static,
        cx: &mut Context<Self>,
    ) -> gpui::Stateful<gpui::Div> {
        let tooltip_str = tooltip.into();
        let base = div()
            .id(id)
            .size(px(26.0))
            .rounded(px(6.0))
            .flex_none()
            .flex()
            .items_center()
            .justify_center()
            .cursor_pointer();
        if !enabled {
            return base.child(app_icon(icon, 13.0, theme.text_ghost));
        }
        base.hover(|element| element.bg(theme.overlay))
            .child(app_icon(icon, 13.0, theme.text_secondary))
            .tooltip(Tooltip::text(tooltip_str))
            .on_click(cx.listener(move |this, _, window, cx| {
                on_click(this, window, cx);
            }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn html_escapes_url() {
        assert_eq!(html_escape("a?x=1&y=\"2\""), "a?x=1&amp;y=&quot;2&quot;");
    }

    #[test]
    fn page_html_contains_src() {
        let html = media_page_html(
            MediaFileKind::Video,
            "http://h/api/fs/file/media?path=%2Fa.mp4&token=t",
        );
        assert!(html.contains("<video"));
        assert!(html.contains(
            "src=\"http://h/api/fs/file/media?path=%2Fa.mp4&amp;token=t\""
        ));
    }

    #[test]
    fn pdf_uses_iframe() {
        let html = media_page_html(MediaFileKind::Pdf, "http://h/x.pdf");
        assert!(html.contains("<iframe"));
        assert!(html.contains("src=\"http://h/x.pdf\""));
    }

    #[test]
    fn audio_uses_audio_tag() {
        let html = media_page_html(MediaFileKind::Audio, "http://h/a.mp3");
        assert!(html.contains("<audio"));
    }

    #[test]
    fn data_url_is_percent_encoded() {
        let url = data_url_for_html("<p>&\"'");
        assert!(url.starts_with("data:text/html;charset=utf-8,"));
        assert!(url.contains("%3C") || url.contains("%3c"));
    }
}
