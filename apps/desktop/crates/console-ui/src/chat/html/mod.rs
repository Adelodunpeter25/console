//! Inline HTML render card for `render` tool results.
//!
//! Collapsed state shows the snapshot PNG (when present) or a compact titled
//! banner; expanding mounts a sandboxed `WebviewHost` pointed at
//! `/api/fs/file/html`. Non-macOS platforms degrade cleanly to snapshot +
//! open-in-browser.

use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;
use std::sync::{Arc, LazyLock, Mutex};

use console_core::{ConsoleClient, HtmlArtifact};
use gpui::{
    App, ElementId, HitboxBehavior, ImageFormat, InteractiveElement, IntoElement, MouseButton,
    ObjectFit, ParentElement, RenderOnce, Styled, Window, canvas, div, img, prelude::*, px,
};

use crate::browser::host::{HostCallbacks, HostContent, SystemWebviewHost as WebviewHost};
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

#[derive(Default)]
struct CardState {
    collapsed: bool,
    measured_height: Option<f32>,
    url: Option<String>,
    url_loading: bool,
}

static CARDS: LazyLock<Mutex<HashMap<String, CardState>>> =
    LazyLock::new(|| Mutex::new(HashMap::new()));

#[cfg(target_os = "macos")]
thread_local! {
    static HOSTS: RefCell<HashMap<String, Rc<WebviewHost>>> = RefCell::new(HashMap::new());
}

fn ensure_url(row: &str, path: &str, is_dark: bool, cx: &mut App) -> Option<String> {
    let mut map = CARDS.lock().ok()?;
    let entry = map.entry(row.to_string()).or_default();
    if let Some(url) = &entry.url {
        return Some(url.clone());
    }
    if entry.url_loading {
        return None;
    }
    entry.url_loading = true;
    drop(map);

    let row_key = row.to_string();
    let path_owned = path.to_string();
    let (tx, rx) = tokio::sync::oneshot::channel();
    tokio::spawn(async move {
        let _ = tx.send(resolve_html_url(&path_owned, is_dark).await);
    });
    cx.spawn(async move |cx| {
        let url = rx.await.ok().flatten();
        cx.update(|cx| {
            if let Ok(mut map) = CARDS.lock() {
                let entry = map.entry(row_key).or_default();
                entry.url_loading = false;
                entry.url = url;
            }
            cx.refresh_windows();
        });
    })
    .detach();
    None
}

/// Hides the native webview when its card stops rendering (tab closed, chat
/// switched). A native view outlives GPUI's element tree, so nothing else
/// would take it off screen.
#[cfg(target_os = "macos")]
struct HostGuard(Rc<WebviewHost>);

#[cfg(target_os = "macos")]
impl Drop for HostGuard {
    fn drop(&mut self) {
        self.0.set_visible(false);
    }
}

#[cfg(target_os = "macos")]
fn ensure_host(row: &str, url: &str, window: &mut Window, cx: &mut App) -> Option<Rc<WebviewHost>> {
    if let Some(existing) = HOSTS.with(|h| h.borrow().get(row).cloned()) {
        return Some(existing);
    }
    let base_page = url.split('#').next().unwrap_or(url).to_string();
    let row_key = row.to_string();
    let nav_cx = cx.to_async();
    let ipc_cx = cx.to_async();
    let win_cx = cx.to_async();
    let content = HostContent {
        user_agent: None,
        initialization_script: Some(
            "document.addEventListener('DOMContentLoaded',function(){var s=document.createElement('style');s.textContent='html,body{overflow:hidden!important}';document.head.appendChild(s);});".to_string(),
        ),
        html: None,
        allow_navigation: Some(Box::new(move |target| {
            let target_base = target.split('#').next().unwrap_or(&target);
            if target_base == base_page {
                return true;
            }
            if target.starts_with("http://") || target.starts_with("https://") {
                let _ = nav_cx.update(|cx| cx.open_url(&target));
            }
            false
        })),
        on_ipc: Some(Box::new(move |body| {
            let Ok(val) = serde_json::from_str::<serde_json::Value>(&body) else {
                return;
            };
            let method = val.get("method").and_then(|m| m.as_str()).unwrap_or("");
            match method {
                "console/size-changed" => {
                    if let Some(h) = val
                        .get("params")
                        .and_then(|p| p.get("height"))
                        .and_then(|h| h.as_f64())
                    {
                        let clamped = (h as f32).clamp(80.0, 4000.0);
                        if let Ok(mut map) = CARDS.lock() {
                            if let Some(entry) = map.get_mut(&row_key) {
                                entry.measured_height = Some(clamped);
                            }
                        }
                        let _ = ipc_cx.update(|cx| cx.refresh_windows());
                    }
                }
                "console/open-link" => {
                    if let Some(link) = val
                        .get("params")
                        .and_then(|p| p.get("url"))
                        .and_then(|u| u.as_str())
                    {
                        let link = link.to_string();
                        let _ = ipc_cx.update(|cx| cx.open_url(&link));
                    }
                }
                _ => {}
            }
        })),
        on_title: None,
        on_new_window_url: Some(Box::new(move |target| {
            if target.starts_with("http://") || target.starts_with("https://") {
                let _ = win_cx.update(|cx| cx.open_url(&target));
            }
        })),
    };
    let callbacks = HostCallbacks {
        on_responder_change: Box::new(|_| {}),
        on_nav_start: Box::new(|| {}),
        on_nav_finish: Box::new(|| {}),
        on_nav_error: Box::new(|_| {}),
    };
    let host = WebviewHost::create(window, content, callbacks).ok()?;
    host.load_url(url);
    let rc = Rc::new(host);
    HOSTS.with(|h| h.borrow_mut().insert(row.to_string(), rc.clone()));
    Some(rc)
}

pub type HtmlActionHandler = Rc<dyn Fn(String, &mut Window, &mut App)>;

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
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let url = ensure_url(&self.row, &self.artifact.path, theme.is_dark, cx);
        let (expanded, measured_height) = CARDS
            .lock()
            .ok()
            .and_then(|m| m.get(&self.row).map(|s| (!s.collapsed, s.measured_height)))
            .unwrap_or((true, None));

        let display_title = if self.artifact.title.trim().is_empty() {
            std::path::Path::new(&self.artifact.path)
                .file_name()
                .and_then(|n| n.to_str())
                .unwrap_or("HTML page")
                .to_string()
        } else {
            self.artifact.title.clone()
        };

        let frame_height = measured_height
            .unwrap_or_else(|| {
                if self.artifact.height > 0 {
                    (self.artifact.height as f32).clamp(80.0, 2000.0)
                } else {
                    420.0
                }
            });

        let row_toggle = self.row.clone();
        let path_editor = self.artifact.path.clone();
        let on_open_editor = self.on_open_editor.clone();
        let on_open_browser = self.on_open_browser.clone();
        let url_for_tab = url.clone();

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
            )
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(6.0))
                    .when(cfg!(target_os = "macos"), |el| {
                        el.child(
                            div()
                                .id(ElementId::Name(format!("html-toggle-{}", self.row).into()))
                                .px(px(8.0))
                                .py(px(4.0))
                                .rounded(px(6.0))
                                .hover(|s| s.bg(theme.overlay))
                                .cursor_pointer()
                                .text_size(px(11.5))
                                .text_color(theme.text_secondary)
                                .child(if expanded { "Collapse" } else { "Expand" })
                                .on_mouse_down(MouseButton::Left, move |_, _window, cx| {
                                    if let Ok(mut map) = CARDS.lock() {
                                        let entry = map.entry(row_toggle.clone()).or_default();
                                        entry.collapsed = !entry.collapsed;
                                        #[cfg(target_os = "macos")]
                                        if entry.collapsed {
                                            HOSTS.with(|h| {
                                                if let Some(host) = h.borrow().get(&row_toggle) {
                                                    host.set_visible(false);
                                                }
                                            });
                                        }
                                    }
                                    cx.refresh_windows();
                                }),
                        )
                    })
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
                    .when_some(url_for_tab.zip(on_open_browser), |el, (u, handler)| {
                        el.child(
                            div()
                                .id(ElementId::Name(format!("html-tab-{}", self.row).into()))
                                .px(px(8.0))
                                .py(px(4.0))
                                .rounded(px(6.0))
                                .hover(|s| s.bg(theme.overlay))
                                .cursor_pointer()
                                .text_size(px(11.5))
                                .text_color(theme.text_secondary)
                                .child("Browser tab")
                                .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                                    handler(u.clone(), window, cx);
                                }),
                        )
                    }),
            );

        let body = if expanded {
            #[cfg(target_os = "macos")]
            {
                if let Some(u) = &url {
                    if let Some(host) = ensure_host(&self.row, u, window, cx) {
                        let guarded = host.clone();
                        let _ = window.use_keyed_state(
                            ElementId::Name(format!("html-host-guard-{}", self.row).into()),
                            cx,
                            move |_, _| HostGuard(guarded),
                        );
                        div()
                            .w_full()
                            .h(px(frame_height))
                            .relative()
                            .child(
                                canvas(
                                    move |bounds, window, _| {
                                        let visible = window.content_mask().bounds;
                                        host.sync_bounds_clipped(bounds, visible);
                                        window.insert_hitbox(bounds, HitboxBehavior::Normal)
                                    },
                                    move |_, _hitbox, _window, _| {},
                                )
                                .absolute()
                                .size_full(),
                            )
                            .into_any_element()
                    } else {
                        div()
                            .p(px(14.0))
                            .text_size(px(12.5))
                            .text_color(theme.text_tertiary)
                            .child("Could not start webview.")
                            .into_any_element()
                    }
                } else {
                    div()
                        .p(px(14.0))
                        .text_size(px(12.5))
                        .text_color(theme.text_tertiary)
                        .child("Loading page…")
                        .into_any_element()
                }
            }
            #[cfg(not(target_os = "macos"))]
            {
                let _ = (window, frame_height);
                div()
                    .p(px(14.0))
                    .text_size(px(12.5))
                    .text_color(theme.text_tertiary)
                    .child("Inline webview is macOS-only; use External to open in your browser.")
                    .into_any_element()
            }
        } else if let Some(bytes) = self.artifact.snapshot {
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
            div()
                .px(px(12.0))
                .py(px(12.0))
                .text_size(px(12.5))
                .text_color(theme.text_secondary)
                .child("Interactive HTML page — click Expand to load inline, or open in a browser tab.")
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
