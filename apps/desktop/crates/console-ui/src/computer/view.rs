//! Computer viewer: GPUI chrome around a child webview that decodes the
//! server's H.264 desktop stream (/api/computer/player).

use std::rc::Rc;

use console_core::ConsoleClient;
use gpui::{
    AnyElement, App, Context, FocusHandle, Focusable, HitboxBehavior, InteractiveElement,
    IntoElement, ParentElement, Render, Styled, Window, canvas, div, prelude::FluentBuilder, px,
};

use crate::browser::host::{HostCallbacks, HostContent, SystemWebviewHost as WebviewHost};
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

#[derive(Clone)]
struct Deferred {
    executor: gpui::ForegroundExecutor,
    cx: gpui::AsyncApp,
    view: gpui::WeakEntity<ComputerViewer>,
}

impl Deferred {
    fn update(&self, f: impl FnOnce(&mut ComputerViewer, &mut Context<ComputerViewer>) + 'static) {
        let mut cx = self.cx.clone();
        let view = self.view.clone();
        self.executor
            .spawn(async move {
                let _ = view.update(&mut cx, f);
            })
            .detach();
    }
}

pub struct ComputerViewer {
    focus_handle: FocusHandle,
    client: ConsoleClient,
    host: Option<Rc<WebviewHost>>,
    host_error: Option<String>,
    base_url: Option<String>,
    auth_token: Option<String>,
    loaded_player_base: Option<String>,
    stream_status: Option<String>,
    occluded: bool,
    was_natively_focused: bool,
    last_window_focus: Option<FocusHandle>,
}

impl ComputerViewer {
    pub fn new(client: ConsoleClient, window: &mut Window, cx: &mut Context<Self>) -> Self {
        let mut this = Self {
            focus_handle: cx.focus_handle(),
            client,
            host: None,
            host_error: None,
            base_url: None,
            auth_token: None,
            loaded_player_base: None,
            stream_status: None,
            occluded: false,
            was_natively_focused: false,
            last_window_focus: None,
        };
        this.build_webview(window, cx);
        this.connect(cx);
        this
    }

    #[cfg(target_os = "macos")]
    fn build_webview(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let deferred = Deferred {
            executor: cx.foreground_executor().clone(),
            cx: cx.to_async(),
            view: cx.entity().downgrade(),
        };
        let on_ipc = deferred.clone();
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
            html: None,
            allow_navigation: None,
            on_ipc: Some(Box::new(move |body| {
                on_ipc.update(move |this, cx| this.player_event(body, cx));
            })),
            on_title: None,
            on_new_window_url: None,
        };
        let callbacks = HostCallbacks {
            on_responder_change,
            on_nav_start: Box::new(|| {}),
            on_nav_finish: Box::new(|| {}),
            on_nav_error: Box::new(|_| {}),
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
        self.host_error = Some("Embedded Computer viewer is currently supported on macOS.".to_string());
    }

    pub fn connect(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let view = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            let base_url = client.base_url().await;
            let auth_token = client.transport().auth_token().await;
            cx.update(|cx| {
                if let Some(view) = view.upgrade() {
                    view.update(cx, |this, cx| {
                        let trimmed = base_url.trim_end_matches('/').to_string();
                        this.base_url = (!trimmed.is_empty()).then_some(trimmed);
                        this.auth_token = auth_token;
                        this.start_stream(cx);
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    fn start_stream(&mut self, _cx: &mut Context<Self>) {
        let Some(host) = self.host.clone() else {
            return;
        };
        let Some(base) = self.base_url.clone() else {
            self.stream_status = Some("not connected".to_string());
            return;
        };
        let token_qs = self
            .auth_token
            .as_deref()
            .filter(|t| !t.is_empty())
            .map(|t| format!("?token={}", urlencoding_component(t)))
            .unwrap_or_default();
        let player_url = format!("{base}/api/computer/player{token_qs}");
        host.set_visible(true);
        if self.loaded_player_base.as_deref() != Some(player_url.as_str()) {
            self.loaded_player_base = Some(player_url.clone());
            host.load_url(&player_url);
        }
        self.stream_status = Some("connecting".to_string());
    }

    fn player_event(&mut self, body: String, cx: &mut Context<Self>) {
        if let Ok(value) = serde_json::from_str::<serde_json::Value>(&body)
            && value.get("type").and_then(|t| t.as_str()) == Some("status")
        {
            self.stream_status = value
                .get("status")
                .and_then(|s| s.as_str())
                .map(|s| s.to_string());
            cx.notify();
        }
    }

    pub fn close(&mut self, cx: &mut Context<Self>) {
        if let Some(host) = self.host.clone() {
            host.evaluate_script("window.__consoleComputer && window.__consoleComputer.stop()");
            host.set_visible(false);
            host.focus_parent();
        }
        self.stream_status = None;
        cx.notify();
    }

    pub fn sync_native_state(
        &mut self,
        surface_visible: bool,
        overlay_open: bool,
        cx: &mut Context<Self>,
    ) {
        let Some(host) = self.host.clone() else {
            return;
        };
        if self.occluded != overlay_open {
            self.occluded = overlay_open;
        }
        let show = surface_visible && !overlay_open;
        if !show {
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

    pub fn reconcile_focus(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let Some(host) = &self.host else {
            return;
        };
        let natively_focused = host.native_focus_within();
        let native_became_focused = natively_focused && !self.was_natively_focused;
        let window_focus = window.focused(cx);
        let window_focus_changed = window_focus != self.last_window_focus;
        let viewer_focused = self.focus_handle.is_focused(window);

        if natively_focused && window_focus_changed && window_focus.is_some() && !viewer_focused {
            self.reclaim_native_keyboard(cx);
        } else if !natively_focused
            && window_focus_changed
            && window_focus.is_some()
            && !viewer_focused
            && self.was_natively_focused
        {
            self.reclaim_native_keyboard(cx);
        } else if native_became_focused && !window_focus_changed && !viewer_focused {
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

    fn render_video_area(&self, theme: Theme) -> AnyElement {
        let host = self.host.clone();
        let occluded = self.occluded;
        let host_error = self.host_error.clone();
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
            .when_some(host_error, |el, err| {
                el.child(
                    div()
                        .absolute()
                        .size_full()
                        .flex()
                        .flex_col()
                        .items_center()
                        .justify_center()
                        .gap(px(8.0))
                        .bg(theme.canvas)
                        .child(app_icon(IconName::Terminal, 28.0, theme.text_tertiary))
                        .child(
                            div()
                                .text_size(px(12.0))
                                .text_color(theme.text_tertiary)
                                .child(err),
                        ),
                )
            })
            .when(occluded, |el| {
                el.child(
                    div()
                        .absolute()
                        .size_full()
                        .flex()
                        .items_center()
                        .justify_center()
                        .bg(theme.canvas)
                        .text_color(theme.text_tertiary)
                        .child("Computer hidden while menu is open"),
                )
            })
            .into_any_element()
    }
}

impl Focusable for ComputerViewer {
    fn focus_handle(&self, _: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Render for ComputerViewer {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        self.reconcile_focus(window, cx);
        div()
            .id("computer-viewer")
            .track_focus(&self.focus_handle)
            .size_full()
            .flex()
            .flex_col()
            .bg(theme.surface)
            .child(self.render_video_area(theme))
    }
}

fn urlencoding_component(value: &str) -> String {
    let mut out = String::with_capacity(value.len());
    for byte in value.bytes() {
        match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                out.push(byte as char)
            }
            _ => out.push_str(&format!("%{byte:02X}")),
        }
    }
    out
}
