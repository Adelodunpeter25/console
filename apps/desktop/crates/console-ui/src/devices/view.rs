//! Device viewer: GPUI chrome (switcher, status pill, hardware rail) around
//! a wry child webview that decodes the hub's H.264 stream.
//!
//! Mirrors the `BrowserView` hosting pattern: the native surface is sized by
//! a layout `canvas`, hidden when the tab is not visible or a GPUI overlay
//! is open, and all input chrome stays in GPUI so a later native-decoder
//! swap only replaces the frame source.

use std::cell::RefCell;
use std::rc::Rc;

use console_core::{ConsoleClient, DeviceActionRequest, DeviceDescriptor, DeviceState};
use gpui::{
    AnyElement, App, AppContext, Context, Entity, FocusHandle, Focusable, HitboxBehavior,
    InteractiveElement, IntoElement, ParentElement, Render, SharedString,
    StatefulInteractiveElement, Styled, Subscription, Window, canvas, div, prelude::FluentBuilder,
    px,
};

use super::player::{PLAYER_HTML, PlayerConfig};
use crate::browser::host::WebviewHost;
use crate::primitives::tooltip::Tooltip;
use crate::primitives::{
    ContextMenuHandle, IconName, MenuAlign, MenuItem, app_icon, dropdown_menu,
};
use crate::theme::Theme;

#[derive(Clone)]
struct Deferred {
    executor: gpui::ForegroundExecutor,
    cx: gpui::AsyncApp,
    view: gpui::WeakEntity<DeviceViewer>,
}

impl Deferred {
    fn update(&self, f: impl FnOnce(&mut DeviceViewer, &mut Context<DeviceViewer>) + 'static) {
        let mut cx = self.cx.clone();
        let view = self.view.clone();
        self.executor
            .spawn(async move {
                let _ = view.update(&mut cx, f);
            })
            .detach();
    }
}

#[derive(Clone)]
pub struct DeviceViewerPending {
    select: Rc<RefCell<Option<String>>>,
}

/// Screenshot capture callback: raw PNG bytes plus the device display name.
pub type ScreenshotHandler = Rc<dyn Fn(Vec<u8>, String, &mut Window, &mut App) + 'static>;

/// The Devices inspector surface. Owned as an entity by the app shell (same
/// lifecycle as `BrowserView`); `RightSidebar` renders it via `AnyElement`.
pub struct DeviceViewer {
    focus_handle: FocusHandle,
    client: ConsoleClient,
    host: Option<Rc<WebviewHost>>,
    host_error: Option<String>,
    base_url: Option<String>,
    devices: Rc<Vec<DeviceDescriptor>>,
    selected_id: Option<String>,
    pending: DeviceViewerPending,
    booting_id: Option<String>,
    shutting_down_id: Option<String>,
    loading: bool,
    error: Option<String>,
    stream_status: Option<String>,
    occluded: bool,
    was_natively_focused: bool,
    last_window_focus: Option<FocusHandle>,
    switcher_menu: ContextMenuHandle,
    on_screenshot: Option<ScreenshotHandler>,
    _subscriptions: Vec<Subscription>,
}

impl DeviceViewer {
    pub fn new(client: ConsoleClient, window: &mut Window, cx: &mut Context<Self>) -> Self {
        let mut this = Self {
            focus_handle: cx.focus_handle(),
            client,
            host: None,
            host_error: None,
            base_url: None,
            devices: Rc::new(Vec::new()),
            selected_id: None,
            pending: DeviceViewerPending {
                select: Rc::new(RefCell::new(None)),
            },
            booting_id: None,
            shutting_down_id: None,
            loading: false,
            error: None,
            stream_status: None,
            occluded: false,
            was_natively_focused: false,
            last_window_focus: None,
            switcher_menu: ContextMenuHandle::new(cx),
            on_screenshot: None,
            _subscriptions: Vec::new(),
        };
        this.build_webview(window, cx);
        this.refresh(window, cx);
        this
    }

    pub fn on_screenshot(
        mut self,
        handler: impl Fn(Vec<u8>, String, &mut Window, &mut App) + 'static,
    ) -> Self {
        // Rc-wrap once here so the struct field stays a plain alias.
        let _ = &handler;
        self.on_screenshot = Some(Rc::new(handler));
        self
    }

    pub fn state_snapshot(&self) -> DeviceViewerPending {
        self.pending.clone()
    }

    #[cfg(target_os = "macos")]
    fn build_webview(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        use wry::dpi::{LogicalPosition, LogicalSize};

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

        let built = wry::WebViewBuilder::new()
            .with_bounds(wry::Rect {
                position: LogicalPosition::new(0.0, 0.0).into(),
                size: LogicalSize::new(0.0, 0.0).into(),
            })
            .with_visible(false)
            .with_focused(false)
            .with_accept_first_mouse(true)
            .with_devtools(true)
            .with_html(PLAYER_HTML)
            .with_ipc_handler(move |request: wry::http::Request<String>| {
                let body = request.body().clone();
                on_ipc.update(move |this, cx| this.player_event(body, cx));
            })
            .build_as_child(window);

        match built {
            Ok(webview) => {
                self.host = Some(Rc::new(WebviewHost::new(webview, on_responder_change)));
            }
            Err(error) => {
                self.host_error = Some(error.to_string());
            }
        }
    }

    #[cfg(not(target_os = "macos"))]
    fn build_webview(&mut self, _window: &mut Window, _cx: &mut Context<Self>) {
        self.host_error = Some("Device preview is currently supported on macOS.".to_string());
    }

    pub fn refresh(&mut self, _window: &mut Window, cx: &mut Context<Self>) {
        self.loading = true;
        self.error = None;
        cx.notify();
        let client = self.client.clone();
        let view = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            let base_url = client.base_url().await;
            let result = client.devices.list().await;
            cx.update(|cx| {
                if let Some(view) = view.upgrade() {
                    view.update(cx, |this, cx| {
                        this.loading = false;
                        let trimmed = base_url.trim_end_matches('/').to_string();
                        this.base_url = (!trimmed.is_empty()).then_some(trimmed);
                        match result {
                            Ok(devices) => {
                                if let Some(selected) = &this.selected_id {
                                    if !devices.iter().any(|d| &d.id == selected) {
                                        this.selected_id = None;
                                    }
                                }
                                this.devices = Rc::new(devices);
                                if this.selected_id.is_some() {
                                    this.start_selected_stream(cx);
                                }
                            }
                            Err(err) => {
                                this.error = Some(err.to_string());
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    fn selected(&self) -> Option<DeviceDescriptor> {
        self.selected_id
            .as_ref()
            .and_then(|id| self.devices.iter().find(|d| &d.id == id).cloned())
    }

    fn select(&mut self, id: String, cx: &mut Context<Self>) {
        self.selected_id = Some(id);
        self.stream_status = None;
        self.start_selected_stream(cx);
        cx.notify();
    }

    fn apply_pending_select(&mut self, cx: &mut Context<Self>) {
        let pending = self.pending.select.borrow_mut().take();
        if let Some(id) = pending {
            self.select(id, cx);
        }
    }

    fn stream_base(&self) -> Option<String> {
        // The hub proxy lives under the server origin; use the backend URL
        // captured from the client in `refresh` / `set_base_url`. No
        // localhost fallback: a missing base means we are not connected.
        let base = self.base_url.clone()?;
        let trimmed = base.trim_end_matches('/').to_string();
        (!trimmed.is_empty()).then_some(trimmed)
    }

    /// Update the backend origin (e.g. after a Server Environment switch)
    /// and restart the selected stream against it.
    pub fn set_base_url(&mut self, url: String, cx: &mut Context<Self>) {
        let trimmed = url.trim_end_matches('/').to_string();
        self.base_url = (!trimmed.is_empty()).then_some(trimmed);
        self.start_selected_stream(cx);
        cx.notify();
    }

    fn start_selected_stream(&mut self, _cx: &mut Context<Self>) {
        let Some(host) = self.host.clone() else {
            return;
        };
        let Some(device) = self.selected() else {
            host.evaluate_script(PlayerConfig::stop_script());
            return;
        };
        let Some(base) = self.stream_base() else {
            self.stream_status = Some("not connected".to_string());
            return;
        };
        // Only booted devices can stream; the server 404s otherwise.
        if !device.state.is_booted() {
            host.evaluate_script(PlayerConfig::stop_script());
            self.stream_status = None;
            return;
        }
        let config = PlayerConfig::for_device(&base, &device.id, device.platform_kind().as_str());
        host.set_visible(true);
        host.evaluate_script(&config.start_script());
        self.stream_status = Some("connecting".to_string());
    }

    fn player_event(&mut self, body: String, cx: &mut Context<Self>) {
        if let Ok(value) = serde_json::from_str::<serde_json::Value>(&body) {
            match value.get("type").and_then(|t| t.as_str()) {
                Some("status") => {
                    self.stream_status = value
                        .get("status")
                        .and_then(|s| s.as_str())
                        .map(|s| s.to_string());
                    cx.notify();
                }
                _ => {}
            }
        }
    }

    fn send_action(&mut self, action: &str, cx: &mut Context<Self>) {
        let Some(device) = self.selected() else {
            return;
        };
        // Hardware buttons ride the live stream socket when it is up.
        if action != "appearance" && self.stream_status.as_deref() == Some("streaming") {
            if let Some(host) = self.host.clone() {
                host.evaluate_script(&PlayerConfig::button_script(&action.replace('_', "-")));
                return;
            }
        }
        let client = self.client.clone();
        let view = cx.entity().downgrade();
        let platform = device.platform_kind().as_str().to_string();
        let id = device.id.clone();
        let action = action.to_string();
        // Empty appearance asks the server to flip the device's real mode.
        let appearance = (action == "appearance").then(|| "toggle".to_string());
        cx.spawn(async move |_, cx| {
            let result = client
                .devices
                .interact(
                    &id,
                    &platform,
                    DeviceActionRequest {
                        action,
                        x: None,
                        y: None,
                        end_x: None,
                        end_y: None,
                        duration_ms: None,
                        text: None,
                        key: None,
                        appearance,
                    },
                )
                .await;
            if let Err(err) = result {
                cx.update(|cx| {
                    if let Some(view) = view.upgrade() {
                        view.update(cx, |this, cx| {
                            this.error = Some(err.to_string());
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();
    }

    pub fn boot_selected(&mut self, _window: &mut Window, cx: &mut Context<Self>) {
        let Some(device) = self.selected() else {
            return;
        };
        self.booting_id = Some(device.id.clone());
        cx.notify();
        let client = self.client.clone();
        let view = cx.entity().downgrade();
        let platform = device.platform_kind().as_str().to_string();
        let id = device.id.clone();
        cx.spawn(async move |_, cx| {
            let result = client.devices.boot(&id, &platform).await;
            cx.update(|cx| {
                if let Some(view) = view.upgrade() {
                    view.update(cx, |this, cx| {
                        if let Err(err) = result {
                            this.booting_id = None;
                            this.error = Some(err.to_string());
                            cx.notify();
                            return;
                        }
                        this.refresh_boot(cx);
                    });
                }
            });
        })
        .detach();
    }

    /// Poll the device list until the booting device reports booted (the
    /// server boots in the background; Android can take minutes).
    fn refresh_boot(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let view = cx.entity().downgrade();
        let target = self.booting_id.clone();
        cx.spawn(async move |_, cx| {
            for _ in 0..100 {
                cx.background_executor()
                    .timer(std::time::Duration::from_millis(3000))
                    .await;
                let base_url = client.base_url().await;
                let result = client.devices.list().await;
                let done = cx.update(|cx| {
                    let Some(view) = view.upgrade() else {
                        return true;
                    };
                    view.update(cx, |this, cx| {
                        // Keep the stream origin in sync in case the backend
                        // changed while booting.
                        let trimmed = base_url.trim_end_matches('/').to_string();
                        this.base_url = (!trimmed.is_empty()).then_some(trimmed);
                        let Ok(devices) = result else {
                            return false;
                        };
                        // An AVD gets a new serial id once it is running;
                        // follow it by name.
                        let booted = target.as_ref().and_then(|id| {
                            let name = this
                                .devices
                                .iter()
                                .find(|d| &d.id == id)
                                .map(|d| d.name.clone());
                            devices.iter().find(|d| {
                                d.state.is_booted()
                                    && (&d.id == id || Some(&d.name) == name.as_ref())
                            })
                        });
                        let finished = target.is_none() || booted.is_some();
                        if let Some(device) = booted {
                            if this.selected_id == target {
                                this.selected_id = Some(device.id.clone());
                            }
                        }
                        this.devices = Rc::new(devices);
                        if finished {
                            this.booting_id = None;
                            this.start_selected_stream(cx);
                        }
                        cx.notify();
                        finished
                    })
                });
                if done {
                    return;
                }
            }
            cx.update(|cx| {
                if let Some(view) = view.upgrade() {
                    view.update(cx, |this, cx| {
                        this.booting_id = None;
                        this.error = Some("Timed out waiting for the device to boot".to_string());
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    pub fn shutdown_selected(&mut self, _window: &mut Window, cx: &mut Context<Self>) {
        let Some(device) = self.selected() else {
            return;
        };
        self.shutting_down_id = Some(device.id.clone());
        if let Some(host) = &self.host {
            host.evaluate_script(PlayerConfig::stop_script());
        }
        self.stream_status = None;
        cx.notify();
        let client = self.client.clone();
        let view = cx.entity().downgrade();
        let platform = device.platform_kind().as_str().to_string();
        let id = device.id.clone();
        cx.spawn(async move |_, cx| {
            let shutdown = client.devices.shutdown(&id, &platform).await;
            let result = client.devices.list().await;
            cx.update(|cx| {
                if let Some(view) = view.upgrade() {
                    view.update(cx, |this, cx| {
                        this.shutting_down_id = None;
                        if let Err(err) = shutdown {
                            this.error = Some(err.to_string());
                        }
                        if let Ok(devices) = result {
                            this.devices = Rc::new(devices);
                            this.start_selected_stream(cx);
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    pub fn capture_screenshot(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let Some(device) = self.selected() else {
            return;
        };
        let name = device.display_name();
        let client = self.client.clone();
        let platform = device.platform_kind().as_str().to_string();
        let id = device.id.clone();
        let handler = self.on_screenshot.clone();
        let window_handle = window.window_handle();
        cx.spawn(async move |_, cx| {
            if let Ok(bytes) = client.devices.screenshot(&id, &platform).await {
                let _ = cx.update_window(window_handle, |_, window, cx| {
                    if let Some(handler) = handler {
                        handler(bytes, name, window, cx);
                    }
                });
            }
        })
        .detach();
    }

    /// Fully tear down the native surface when the tab closes.
    pub fn close(&mut self, cx: &mut Context<Self>) {
        if let Some(host) = self.host.clone() {
            host.evaluate_script(PlayerConfig::stop_script());
            host.set_visible(false);
            host.focus_parent();
        }
        self.stream_status = None;
        cx.notify();
    }

    /// Per-frame push from the app: whether this surface is the visible right
    /// panel tab, and whether a GPUI overlay is open above it.
    pub fn sync_native_state(
        &mut self,
        surface_visible: bool,
        overlay_open: bool,
        cx: &mut Context<Self>,
    ) {
        let Some(host) = self.host.clone() else {
            return;
        };
        let occluded = overlay_open || self.switcher_menu.is_open();
        if self.occluded != occluded {
            self.occluded = occluded;
        }
        let has_device = self.selected_id.is_some();
        let show = surface_visible && has_device && !occluded;
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
            // GPUI focus moved elsewhere (e.g. the composer) while the native
            // surface still holds the keyboard: hand it back.
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

    pub fn overlay_open(&self) -> bool {
        self.switcher_menu.is_open()
    }

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
            .size(px(28.0))
            .rounded(px(6.0))
            .flex_none()
            .flex()
            .items_center()
            .justify_center()
            .cursor_default();
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

    fn floating_control_button(
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
            .size(px(32.0))
            .rounded(px(6.0))
            .flex_none()
            .flex()
            .items_center()
            .justify_center()
            .cursor_pointer();
        if !enabled {
            return base.child(app_icon(icon, 15.0, theme.text_ghost));
        }
        base.hover(|element| element.bg(theme.overlay))
            .active(|element| element.bg(theme.border))
            .child(app_icon(icon, 15.0, theme.text))
            .tooltip(Tooltip::text(tooltip_str))
            .on_click(cx.listener(move |this, _, window, cx| {
                on_click(this, window, cx);
            }))
    }

    fn render_switcher(&self, cx: &mut Context<Self>) -> AnyElement {
        let theme = Theme::current(cx);
        let selected_label = self
            .selected()
            .map(|d| format!("{} · {}", d.display_name(), d.platform_kind().label()))
            .unwrap_or_else(|| "Select device".to_string());
        let status = self.status_text();
        let devices = self.devices.clone();
        let selected_id = self.selected_id.clone();
        let menu = self.switcher_menu.clone();
        let pending_select = self.pending.select.clone();
        let view = cx.entity().downgrade();
        div()
            .flex_1()
            .min_w_0()
            .child(dropdown_menu(
                div()
                    .id("device-switcher-trigger")
                    .h(px(30.0))
                    .px(px(10.0))
                    .rounded(px(6.0))
                    .border_1()
                    .border_color(theme.border)
                    .bg(theme.inset)
                    .flex()
                    .items_center()
                    .gap(px(6.0))
                    .min_w_0()
                    .flex_1()
                    .cursor_pointer()
                    .hover(|s| s.border_color(theme.border_strong))
                    .child(app_icon(IconName::Smartphone, 13.0, theme.text_tertiary))
                    .child(
                        div()
                            .flex_1()
                            .min_w_0()
                            .overflow_hidden()
                            .text_size(px(12.0))
                            .text_color(theme.text)
                            .child(selected_label),
                    )
                    .when_some(status, |el, s| {
                        el.child(
                            div()
                                .text_size(px(10.0))
                                .px(px(6.0))
                                .py(px(2.0))
                                .rounded_full()
                                .bg(theme.overlay)
                                .text_color(theme.text_secondary)
                                .child(s),
                        )
                    })
                    .child(app_icon(IconName::ChevronDown, 12.0, theme.text_tertiary)),
                "device-switcher-menu",
                &menu,
                MenuAlign::BelowLeft,
                move |_cx| {
                    devices
                        .iter()
                        .map(|device| {
                            let id = device.id.clone();
                            let label = format!(
                                "{} · {} · {}",
                                device.display_name(),
                                device.platform_kind().label(),
                                device.state.label()
                            );
                            let selected = selected_id.as_deref() == Some(&device.id);
                            let pending_select = pending_select.clone();
                            let view = view.clone();
                            MenuItem::new(label, move |_, cx| {
                                pending_select.borrow_mut().replace(id.clone());
                                if let Some(view) = view.upgrade() {
                                    view.update(cx, |this, cx| this.apply_pending_select(cx));
                                }
                            })
                            .selected(selected)
                            .disabled(!device.is_available)
                        })
                        .collect()
                },
            ))
            .into_any_element()
    }

    fn status_text(&self) -> Option<String> {
        if self.loading {
            return Some("Loading".to_string());
        }
        if self.booting_id.as_ref() == self.selected_id.as_ref() && self.booting_id.is_some() {
            return Some("Booting".to_string());
        }
        if self.shutting_down_id.as_ref() == self.selected_id.as_ref()
            && self.shutting_down_id.is_some()
        {
            return Some("Stopping".to_string());
        }
        if let Some(device) = self.selected() {
            if device.state == DeviceState::Booting || Some(&device.id) == self.booting_id.as_ref()
            {
                return Some("Booting".to_string());
            }
            if device.state.is_booted() {
                return Some(
                    self.stream_status
                        .clone()
                        .map(|s| {
                            if s == "streaming" {
                                "Ready".to_string()
                            } else {
                                s
                            }
                        })
                        .unwrap_or_else(|| "Ready".to_string()),
                );
            }
            return Some(device.state.label().to_string());
        }
        None
    }

    fn render_toolbar(&self, cx: &mut Context<Self>) -> gpui::Div {
        let theme = Theme::current(cx);
        let has_device = self.selected().is_some();
        let booted = self.selected().is_some_and(|d| d.state.is_booted());
        let is_booting =
            self.booting_id.as_ref() == self.selected_id.as_ref() && self.booting_id.is_some();
        let is_stopping = self.shutting_down_id.as_ref() == self.selected_id.as_ref()
            && self.shutting_down_id.is_some();
        div()
            .flex_none()
            .px(px(8.0))
            .py(px(6.0))
            .flex()
            .items_center()
            .gap(px(6.0))
            .border_b_1()
            .border_color(theme.border)
            .bg(theme.surface)
            .child(self.render_switcher(cx))
            .child(self.toolbar_button(
                "device-refresh",
                IconName::RotateCw,
                true,
                "Refresh devices",
                theme,
                |this, window, cx| this.refresh(window, cx),
                cx,
            ))
            .child(self.toolbar_button(
                "device-power",
                IconName::Play,
                has_device && !booted && !is_booting && !is_stopping,
                "Boot device",
                theme,
                |this, window, cx| this.boot_selected(window, cx),
                cx,
            ))
            .child(self.toolbar_button(
                "device-stop",
                IconName::Stop,
                booted && !is_stopping,
                "Shut down simulator",
                theme,
                |this, window, cx| this.shutdown_selected(window, cx),
                cx,
            ))
            .child(self.toolbar_button(
                "device-screenshot",
                IconName::Camera,
                booted && !is_stopping,
                "Send screenshot to composer",
                theme,
                |this, window, cx| this.capture_screenshot(window, cx),
                cx,
            ))
    }

    fn render_hardware_rail(
        &self,
        theme: Theme,
        cx: &mut Context<Self>,
    ) -> gpui::Stateful<gpui::Div> {
        let is_ios = self
            .selected()
            .is_some_and(|d| d.platform_kind() == console_core::DevicePlatform::Ios);
        div()
            .id("device-right-rail-container")
            .flex_none()
            .h_full()
            .flex()
            .items_center()
            .justify_center()
            .px(px(6.0))
            .bg(theme.canvas)
            .border_l_1()
            .border_color(theme.border)
            .child(
                div()
                    .id("device-right-rail")
                    .px(px(4.0))
                    .py(px(8.0))
                    .flex()
                    .flex_col()
                    .items_center()
                    .gap(px(6.0))
                    .rounded(px(10.0))
                    .bg(theme.raised)
                    .border_1()
                    .border_color(theme.border)
                    .shadow_lg()
                    .child(self.floating_control_button(
                        "device-home",
                        IconName::Home,
                        true,
                        "Home",
                        theme,
                        |this, _, cx| this.send_action("home", cx),
                        cx,
                    ))
                    .when(!is_ios, |el| {
                        el.child(self.floating_control_button(
                            "device-back",
                            IconName::ChevronLeft,
                            true,
                            "Back",
                            theme,
                            |this, _, cx| this.send_action("back", cx),
                            cx,
                        ))
                    })
                    .child(self.floating_control_button(
                        "device-volume-up",
                        IconName::VolumeLoud,
                        true,
                        "Volume up",
                        theme,
                        |this, _, cx| this.send_action("volume_up", cx),
                        cx,
                    ))
                    .child(self.floating_control_button(
                        "device-volume-down",
                        IconName::VolumeLow,
                        true,
                        "Volume down",
                        theme,
                        |this, _, cx| this.send_action("volume_down", cx),
                        cx,
                    ))
                    .child(self.floating_control_button(
                        "device-power-btn",
                        IconName::Lock,
                        true,
                        "Lock / Power",
                        theme,
                        |this, _, cx| this.send_action("power", cx),
                        cx,
                    ))
                    .child(self.floating_control_button(
                        "device-appearance",
                        IconName::Appearance,
                        true,
                        "Toggle dark / light",
                        theme,
                        |this, _, cx| this.send_action("appearance", cx),
                        cx,
                    )),
            )
    }

    fn render_video_area(&self, theme: Theme) -> AnyElement {
        let host = self.host.clone();
        let occluded = self.occluded;
        let has_device = self.selected_id.is_some();
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
            .when(!has_device, |el| {
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
                        .child(app_icon(IconName::Smartphone, 28.0, theme.text_tertiary))
                        .child(
                            div()
                                .text_size(px(13.0))
                                .font_weight(gpui::FontWeight::SEMIBOLD)
                                .text_color(theme.text)
                                .child("No device selected"),
                        )
                        .child(
                            div()
                                .text_size(px(12.0))
                                .text_color(theme.text_tertiary)
                                .child("Pick a simulator or emulator above."),
                        ),
                )
            })
            .when(occluded && has_device, |el| {
                el.child(
                    div()
                        .absolute()
                        .size_full()
                        .flex()
                        .items_center()
                        .justify_center()
                        .bg(theme.canvas)
                        .text_color(theme.text_tertiary)
                        .child("Device hidden while menu is open"),
                )
            })
            .into_any_element()
    }

    fn render_diagnostics(&self, theme: Theme) -> Option<AnyElement> {
        self.error.clone().map(|message| {
            div()
                .flex_none()
                .m(px(8.0))
                .p(px(10.0))
                .rounded(px(6.0))
                .border_1()
                .border_color(theme.danger)
                .bg(theme.canvas)
                .text_size(px(12.0))
                .text_color(theme.text)
                .child(message)
                .into_any_element()
        })
    }
}

impl Focusable for DeviceViewer {
    fn focus_handle(&self, _: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Render for DeviceViewer {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        let has_device = self.selected_id.is_some();
        let booted = self.selected().is_some_and(|d| d.state.is_booted());
        self.reconcile_focus(window, cx);
        div()
            .id("device-viewer")
            .track_focus(&self.focus_handle)
            .size_full()
            .flex()
            .flex_col()
            .bg(theme.surface)
            .child(self.render_toolbar(cx))
            .when_some(self.render_diagnostics(theme), |el, banner| {
                el.child(banner)
            })
            .child(
                div()
                    .flex_1()
                    .min_h_0()
                    .flex()
                    .flex_row()
                    .bg(theme.canvas)
                    .child(self.render_video_area(theme))
                    .when(has_device && booted, |el| {
                        el.child(self.render_hardware_rail(theme, cx))
                    }),
            )
    }
}

/// App-shell bridge: pump a switcher-menu selection made outside the
/// viewer's own click context.
pub fn select_device(view: &Entity<DeviceViewer>, cx: &mut App) {
    view.update(cx, |this, cx| this.apply_pending_select(cx));
}
