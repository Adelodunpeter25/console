//! Inline HTML render card for `render` tool results.
//!
//! Shows the snapshot PNG (when present) or a compact interactive-page banner
//! with "Open in browser" (dedicated Browser workspace tab) and "Open in editor".
//! Opening in a real workspace tab gives full space, real developer tools,
//! scrolling, and avoids native NSView window-overlay clipping bugs.

use std::sync::{Arc, LazyLock, Mutex};

use console_core::{ConsoleClient, HtmlArtifact};
use gpui::prelude::FluentBuilder;
use gpui::{
    App, AppContext, ElementId, ImageFormat, InteractiveElement, IntoElement, MouseButton,
    ObjectFit, ParentElement, RenderOnce, Styled, StyledImage, Window, div, img, px,
};

use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

static CLIENT: LazyLock<Mutex<Option<ConsoleClient>>> = LazyLock::new(|| Mutex::new(None));

/// Wire the session's `ConsoleClient` so `HtmlCard` can build authenticated
/// `/api/fs/file/html` URLs.
pub fn set_html_render_client(client: &ConsoleClient) {
    if let Ok(mut slot) = CLIENT.lock() {
        *slot = Some(client.clone());
    }
}

/// Build the authenticated `/api/fs/file/html` URL for `path` if a client is wired.
pub async fn resolve_html_url(path: &str, is_dark: bool) -> Option<String> {
    let client = CLIENT.lock().ok()?.clone()?;
    let token = client.transport().auth_token().await;
    let theme = if is_dark { "dark" } else { "light" };
    Some(client.fs.html_url(path, theme, token.as_deref()).await)
}

pub type HtmlActionHandler = std::rc::Rc<dyn Fn(String, &mut Window, &mut App)>;

/// One inline card for a `render` tool result.
#[derive(IntoElement)]
pub struct HtmlCard {
    row: String,
    artifact: HtmlArtifact,
    on_open_browser: Option<HtmlActionHandler>,
    on_open_editor: Option<HtmlActionHandler>,
}

impl HtmlCard {
    pub fn new(row: impl Into<String>, artifact: HtmlArtifact) -> Self {
        Self {
            row: row.into(),
            artifact,
            on_open_browser: None,
            on_open_editor: None,
        }
    }

    pub fn on_open_browser(mut self, handler: Option<HtmlActionHandler>) -> Self {
        self.on_open_browser = handler;
        self
    }

    pub fn on_open_editor(mut self, handler: Option<HtmlActionHandler>) -> Self {
        self.on_open_editor = handler;
        self
    }
}

impl RenderOnce for HtmlCard {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let display_title = if self.artifact.title.trim().is_empty() {
            std::path::Path::new(&self.artifact.path)
                .file_name()
                .and_then(|n| n.to_str())
                .unwrap_or("HTML page")
                .to_string()
        } else {
            self.artifact.title.clone()
        };
        let file_name = std::path::Path::new(&self.artifact.path)
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or(&self.artifact.path)
            .to_string();

        let path_editor = self.artifact.path.clone();
        let path_browser = self.artifact.path.clone();
        let on_open_editor = self.on_open_editor.clone();
        let on_open_browser = self.on_open_browser.clone();

        let header = div()
            .w_full()
            .px(px(12.0))
            .py(px(9.0))
            .flex()
            .items_center()
            .justify_between()
            .gap(px(8.0))
            .border_b_1()
            .border_color(theme.border)
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(8.0))
                    .min_w_0()
                    .flex_1()
                    .child(app_icon(IconName::Globe, 15.0, theme.accent))
                    .child(
                        div()
                            .text_size(px(13.0))
                            .font_weight(gpui::FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .truncate()
                            .child(display_title),
                    )
                    .child(
                        div()
                            .text_size(px(11.5))
                            .text_color(theme.text_tertiary)
                            .truncate()
                            .child(file_name),
                    ),
            )
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(6.0))
                    .when_some(on_open_editor, |el, handler| {
                        el.child(
                            div()
                                .id(ElementId::Name(format!("html-edit-{}", self.row).into()))
                                .px(px(8.0))
                                .py(px(4.0))
                                .rounded(px(6.0))
                                .hover(|s| s.bg(theme.overlay))
                                .cursor_pointer()
                                .text_size(px(11.5))
                                .text_color(theme.text_secondary)
                                .child("Editor")
                                .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                                    handler(path_editor.clone(), window, cx);
                                }),
                        )
                    })
                    .when_some(on_open_browser.clone(), |el, handler| {
                        let is_dark = theme.is_dark;
                        let p = path_browser.clone();
                        el.child(
                            div()
                                .id(ElementId::Name(format!("html-tab-{}", self.row).into()))
                                .px(px(8.0))
                                .py(px(4.0))
                                .rounded(px(6.0))
                                .bg(theme.raised)
                                .hover(|s| s.bg(theme.overlay_strong))
                                .cursor_pointer()
                                .text_size(px(11.5))
                                .text_color(theme.text)
                                .child("Open in browser")
                                .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                                    let h = handler.clone();
                                    let path_clone = p.clone();
                                    let (tx, rx) = tokio::sync::oneshot::channel();
                                    tokio::spawn(async move {
                                        let _ = tx.send(resolve_html_url(&path_clone, is_dark).await);
                                    });
                                    let win_handle = window.window_handle();
                                    cx.spawn(async move |cx| {
                                        let Some(url) = rx.await.ok().flatten() else { return; };
                                        let _ = cx.update_window(win_handle, move |_view, window, cx| {
                                            h(url, window, cx);
                                        });
                                    })
                                    .detach();
                                }),
                        )
                    }),
            );

        let body = if let Some(bytes) = self.artifact.snapshot {
            let image = Arc::new(gpui::Image::from_bytes(ImageFormat::Png, bytes));
            div()
                .w_full()
                .max_h(px(280.0))
                .overflow_hidden()
                .child(
                    img(image)
                        .w_full()
                        .max_h(px(280.0))
                        .object_fit(ObjectFit::Contain),
                )
                .into_any_element()
        } else {
            let on_browser_click = on_open_browser;
            let is_dark = theme.is_dark;
            let p = self.artifact.path.clone();
            div()
                .w_full()
                .p(px(12.0))
                .flex()
                .items_center()
                .justify_between()
                .rounded(px(6.0))
                .cursor_pointer()
                .hover(|s| s.bg(theme.overlay))
                .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                    if let Some(h) = &on_browser_click {
                        let h = h.clone();
                        let path_clone = p.clone();
                        let (tx, rx) = tokio::sync::oneshot::channel();
                        tokio::spawn(async move {
                            let _ = tx.send(resolve_html_url(&path_clone, is_dark).await);
                        });
                        let win_handle = window.window_handle();
                        cx.spawn(async move |cx| {
                            let Some(url) = rx.await.ok().flatten() else { return; };
                            let _ = cx.update_window(win_handle, move |_view, window, cx| {
                                h(url, window, cx);
                            });
                        })
                        .detach();
                    }
                })
                .child(
                    div()
                        .text_size(px(12.5))
                        .text_color(theme.text_secondary)
                        .child("Interactive HTML page"),
                )
                .child(
                    div()
                        .text_size(px(12.0))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(theme.accent)
                        .child("Open in browser tab →"),
                )
                .into_any_element()
        };

        div()
            .w_full()
            .my(px(6.0))
            .rounded(px(10.0))
            .border_1()
            .border_color(theme.border)
            .bg(theme.surface)
            .overflow_hidden()
            .flex()
            .flex_col()
            .child(header)
            .child(body)
    }
}
