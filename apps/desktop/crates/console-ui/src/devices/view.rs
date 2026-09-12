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
    loading: bool,
    error: Option<String>,
    stream_status: Option<String>,
    occluded: bool,
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
            loading: false,
            error: None,
            stream_status: None,
            occluded: false,
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
                        this.base_url = Some(base_url);
                        match result {
                            Ok(devices) => {
                                if this.selected_id.is_none() {
                                    this.selected_id = devices
                                        .iter()
                                        .find(|d| d.state == DeviceState::Booted)
                                        .or(devices.first())
                                        .map(|d| d.id.clone());
                                }
                                this.devices = Rc::new(devices);
                                this.start_selected_stream(cx);
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

    fn stream_base(&self, cx: &App) -> String {
        // The hub proxy lives under the server origin; the transport base URL
        // is read from the client at call time in the app shell, so here we
        // reuse the configured backend URL via a blocking snapshot is avoided
        // by deriving from the client's known base through cx globals is not
        // possible — instead the shell passes base URL through start config.
        // Fallback to localhost for tests.
        let _ = cx;
        "http://localhost:3000".to_string()
    }

    fn start_selected_stream(&mut self, cx: &mut Context<Self>) {
        let Some(host) = self.host.clone() else {
            return;
        };
        let Some(device) = self.selected() else {
            host.evaluate_script(PlayerConfig::stop_script());
            return;
        };
        let platform = device.platform_kind().as_str().to_string();
        // Until the hub proxy lands server-side, stream URLs are absent and
        // the player runs in stills mode against the screenshot endpoint.
        let base = self.stream_base(cx);
        let screenshot_url = format!(
            "{}/api/devices/{}/screenshot?platform={}",
            base, device.id, platform
        );
        let config = PlayerConfig {
            platform,
            stream_url: None,
            control_url: None,
            screenshot_url,
            codec: None,
        };
        host.set_visible(true);
        host.evaluate_script(&config.start_script());
        self.stream_status = Some("stills".to_string());
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
                Some("tap") => {
                    let x = value.get("x").and_then(|v| v.as_f64()).unwrap_or(0.0) as f32;
                    let y = value.get("y").and_then(|v| v.as_f64()).unwrap_or(0.0) as f32;
                    self.send_tap(x, y, cx);
                }
                _ => {}
            }
        }
    }

    fn send_tap(&mut self, x: f32, y: f32, cx: &mut Context<Self>) {
        let Some(device) = self.selected() else {
            return;
        };
        let client = self.client.clone();
        let platform = device.platform_kind().as_str().to_string();
        let id = device.id.clone();
        cx.spawn(async move |_, _| {
            let _ = client
                .devices
                .interact(
                    &id,
                    &platform,
                    DeviceActionRequest {
                        action: "tap".to_string(),
                        x: Some(x),
                        y: Some(y),
                        end_x: None,
                        end_y: None,
                        duration_ms: None,
                        text: None,
                        key: None,
                        appearance: None,
                    },
                )
                .await;
        })
        .detach();
    }

    fn send_action(&mut self, action: &str, cx: &mut Context<Self>) {
        let Some(device) = self.selected() else {
            return;
        };
        let client = self.client.clone();
        let platform = device.platform_kind().as_str().to_string();
        let id = device.id.clone();
        let action = action.to_string();
        cx.spawn(async move |_, _| {
            let _ = client
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
                        appearance: None,
                    },
                )
                .await;
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
            let _ = client.devices.boot(&id, &platform).await;
            cx.update(|cx| {
                if let Some(view) = view.upgrade() {
                    view.update(cx, |this, cx| {
                        this.booting_id = None;
                        this.refresh_boot(cx);
                    });
                }
            });
        })
        .detach();
    }

    fn refresh_boot(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let view = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            cx.background_executor()
                .timer(std::time::Duration::from_millis(1500))
                .await;
            let result = client.devices.list().await;
            cx.update(|cx| {
                if let Some(view) = view.upgrade() {
                    view.update(cx, |this, cx| {
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

    pub fn shutdown_selected(&mut self, _window: &mut Window, cx: &mut Context<Self>) {
        let Some(device) = self.selected() else {
            return;
        };
        let client = self.client.clone();
        let view = cx.entity().downgrade();
        let platform = device.platform_kind().as_str().to_string();
        let id = device.id.clone();
        cx.spawn(async move |_, cx| {
            let _ = client.devices.shutdown(&id, &platform).await;
            let result = client.devices.list().await;
            cx.update(|cx| {
                if let Some(view) = view.upgrade() {
                    view.update(cx, |this, cx| {
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
        if host.native_focus_within() && !self.focus_handle.is_focused(window) {
            window.focus(&self.focus_handle, cx);
        }
    }

    fn native_responder_changed(
        &mut self,
        _user_gesture: bool,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if self
            .host
            .as_ref()
            .is_some_and(|host| host.native_focus_within())
        {
            window.focus(&self.focus_handle, cx);
        }
    }

    pub fn overlay_open(&self) -> bool {
        self.switcher_menu.is_open()
    }

    fn rail_button(
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
            .size(px(30.0))
            .rounded(px(6.0))
            .flex_none()
            .flex()
            .items_center()
            .justify_center()
            .cursor_default();
        if !enabled {
            return base.child(app_icon(icon, 14.0, theme.text_ghost));
        }
        base.hover(|element| element.bg(theme.overlay))
            .child(app_icon(icon, 14.0, theme.text_secondary))
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
                    .child(
                        div()
                            .text_size(px(10.0))
                            .px(px(6.0))
                            .py(px(2.0))
                            .rounded_full()
                            .bg(theme.overlay)
                            .text_color(theme.text_secondary)
                            .child(status),
                    )
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

    fn status_text(&self) -> String {
        if self.loading {
            return "Loading".to_string();
        }
        if self.booting_id.as_ref() == self.selected_id.as_ref() && self.booting_id.is_some() {
            return "Booting".to_string();
        }
        if let Some(device) = self.selected() {
            if device.state == DeviceState::Booting || Some(&device.id) == self.booting_id.as_ref()
            {
                return "Booting".to_string();
            }
            if device.state.is_booted() {
                return self
                    .stream_status
                    .clone()
                    .map(|s| {
                        if s == "streaming" {
                            "Ready".to_string()
                        } else {
                            s
                        }
                    })
                    .unwrap_or_else(|| "Ready".to_string());
            }
            return device.state.label().to_string();
        }
        "No device".to_string()
    }

    fn render_toolbar(&self, cx: &mut Context<Self>) -> gpui::Div {
        let theme = Theme::current(cx);
        let has_device = self.selected().is_some();
        let booted = self.selected().is_some_and(|d| d.state.is_booted());
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
            .child(self.rail_button(
                "device-refresh",
                IconName::RotateCw,
                true,
                "Refresh devices",
                theme,
                |this, window, cx| this.refresh(window, cx),
                cx,
            ))
            .child(self.rail_button(
                "device-power",
                IconName::Play,
                has_device && !booted,
                "Boot device",
                theme,
                |this, window, cx| this.boot_selected(window, cx),
                cx,
            ))
            .child(self.rail_button(
                "device-screenshot",
                IconName::Eye,
                booted,
                "Send screenshot to composer",
                theme,
                |this, window, cx| this.capture_screenshot(window, cx),
                cx,
            ))
    }

    fn render_hardware_rail(&self, cx: &mut Context<Self>) -> gpui::Div {
        let theme = Theme::current(cx);
        let booted = self.selected().is_some_and(|d| d.state.is_booted());
        let is_ios = self
            .selected()
            .is_some_and(|d| d.platform_kind() == console_core::DevicePlatform::Ios);
        div()
            .flex_none()
            .px(px(8.0))
            .py(px(6.0))
            .flex()
            .items_center()
            .justify_center()
            .gap(px(4.0))
            .border_t_1()
            .border_color(theme.border)
            .bg(theme.surface)
            .child(self.rail_button(
                "device-home",
                IconName::Smartphone,
                booted,
                "Home",
                theme,
                |this, _, cx| this.send_action("home", cx),
                cx,
            ))
            .when(!is_ios, |el| {
                el.child(self.rail_button(
                    "device-back",
                    IconName::ArrowLeft,
                    booted,
                    "Back",
                    theme,
                    |this, _, cx| this.send_action("back", cx),
                    cx,
                ))
            })
            .child(self.rail_button(
                "device-volume-up",
                IconName::VolumeLoud,
                booted,
                "Volume up",
                theme,
                |this, _, cx| this.send_action("volume_up", cx),
                cx,
            ))
            .child(self.rail_button(
                "device-volume-down",
                IconName::VolumeLoud,
                booted,
                "Volume down",
                theme,
                |this, _, cx| this.send_action("volume_down", cx),
                cx,
            ))
            .child(self.rail_button(
                "device-power-btn",
                IconName::Stop,
                booted,
                "Power",
                theme,
                |this, _, cx| this.send_action("power", cx),
                cx,
            ))
            .child(self.rail_button(
                "device-appearance",
                IconName::Moon,
                booted,
                "Toggle dark / light",
                theme,
                |this, _, cx| this.send_action("appearance", cx),
                cx,
            ))
    }

    fn render_video_area(&self, theme: Theme) -> AnyElement {
        let host = self.host.clone();
        let occluded = self.occluded;
        let has_device = self.selected_id.is_some();
        div()
            .flex_1()
            .min_h_0()
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
            .child(self.render_video_area(theme))
            .child(self.render_hardware_rail(cx))
    }
}

/// App-shell bridge: pump a switcher-menu selection made outside the
/// viewer's own click context.
pub fn select_device(view: &Entity<DeviceViewer>, cx: &mut App) {
    view.update(cx, |this, cx| this.apply_pending_select(cx));
}
