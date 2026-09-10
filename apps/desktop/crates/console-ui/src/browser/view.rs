//! Native browser view component for GPUI right panel / inspector.
//!
//! Features:
//! - Full-featured navigation toolbar with back, forward, reload/stop, security indicator,
//!   single-line address bar with live progress indicator, and external browser launcher.
//! - Safari-style omnibox input resolution with automatic localhost/IP detection.
//! - Two-way focus reconciliation between native platform responder chain and GPUI focus tree.
//! - Geometry synchronization via layout canvas with sub-pixel edge snapping.
//! - Overlay protection with snapshot freezing when menus or command palettes open.
//! - Start page with instant localhost dev server launchers (3000, 5173, 8080).
//! - Keyboard actions for address focus (⌘L), navigation (⌘[/⌘]), reload (⌘R), and devtools.

use std::rc::Rc;
use std::sync::Arc;

use gpui::{
    App, Context, Div, Entity, FocusHandle, Focusable, HitboxBehavior, IntoElement, ObjectFit,
    ParentElement, Render, SharedString, Stateful, Styled, Subscription, Window, canvas, div, img,
    prelude::*, px,
};

use super::actions::*;
use super::address::{AddressTarget, display_url, is_secure_url, resolve_address, search_url};
use super::host::WebviewHost;
use crate::common::input::{ComposerEvent, ComposerInput};
use crate::primitives::tooltip::Tooltip;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

const TOOLBAR_HEIGHT: f32 = 42.0;

pub struct BrowserView {
    focus_handle: FocusHandle,
    address: Entity<ComposerInput>,
    host: Option<Rc<WebviewHost>>,
    host_error: Option<String>,
    navigation_requested: bool,
    current_url: Option<String>,
    page_title: Option<String>,
    loading: bool,
    can_go_back: bool,
    can_go_forward: bool,
    address_dirty: bool,
    was_natively_focused: bool,
    last_window_focus: Option<FocusHandle>,
    occluded: bool,
    snapshot: Option<Arc<gpui::RenderImage>>,
    snapshot_pending: bool,
    snapshot_epoch: u64,
    _subscriptions: Vec<Subscription>,
}

impl BrowserView {
    pub fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let address = cx.new(|cx| {
            ComposerInput::new(window, cx)
                .search_field()
                .select_all_on_focus_click()
                .placeholder("Search or enter website address (e.g. localhost:3000)")
        });

        let submit_subscription = cx.subscribe(
            &address,
            |this: &mut Self, address, event: &ComposerEvent, cx| match event {
                ComposerEvent::Submit(text) => this.navigate_to_input(text.clone(), cx),
                ComposerEvent::Edited => {
                    let shown = this.current_url.as_deref().map(display_url).unwrap_or("");
                    this.address_dirty = address.read(cx).content() != shown;
                }
                _ => {}
            },
        );

        let focus_handle = cx.focus_handle();
        let address_focus = address.read(cx).focus();
        let weak_for_focus_in = cx.entity().downgrade();
        let weak_for_focus_out = cx.entity().downgrade();

        let focus_in_address = window.on_focus_in(&address_focus, cx, {
            let view = weak_for_focus_in;
            move |_, cx| {
                let _ = view.update(cx, |this: &mut Self, cx| {
                    if this
                        .host
                        .as_ref()
                        .is_some_and(|host| host.native_focus_within())
                    {
                        this.reclaim_native_keyboard(cx);
                    }
                });
            }
        });

        let focus_out_surface = window.on_focus_out(&focus_handle, cx, {
            let view = weak_for_focus_out;
            move |_, window, cx| {
                let focused_elsewhere = window.is_window_active() && window.focused(cx).is_some();
                let _ = view.update(cx, |this: &mut Self, cx| {
                    if focused_elsewhere
                        && this
                            .host
                            .as_ref()
                            .is_some_and(|host| host.native_focus_within())
                    {
                        this.reclaim_native_keyboard(cx);
                    }
                });
            }
        });

        Self {
            focus_handle,
            address,
            host: Some(Rc::new(WebviewHost::new())),
            host_error: None,
            navigation_requested: false,
            current_url: None,
            page_title: None,
            loading: false,
            can_go_back: false,
            can_go_forward: false,
            address_dirty: false,
            was_natively_focused: false,
            last_window_focus: None,
            occluded: false,
            snapshot: None,
            snapshot_pending: false,
            snapshot_epoch: 0,
            _subscriptions: vec![submit_subscription, focus_in_address, focus_out_surface],
        }
    }

    pub fn with_host(mut self, host: Rc<WebviewHost>) -> Self {
        self.host = Some(host);
        self
    }

    pub fn with_host_error(mut self, error: impl Into<String>) -> Self {
        self.host_error = Some(error.into());
        self
    }

    /// Tab label shown in the right sidebar tab bar.
    pub fn tab_label(&self) -> Option<String> {
        if let Some(title) = self.page_title.as_deref().filter(|t| !t.trim().is_empty()) {
            return Some(title.to_owned());
        }
        self.current_url
            .as_deref()
            .map(|url| display_url(url).to_owned())
    }

    pub fn current_url(&self) -> Option<&str> {
        self.current_url.as_deref()
    }

    pub fn page_title(&self) -> Option<&str> {
        self.page_title.as_deref()
    }

    pub fn is_loading(&self) -> bool {
        self.loading
    }

    pub fn can_go_back(&self) -> bool {
        self.can_go_back
    }

    pub fn can_go_forward(&self) -> bool {
        self.can_go_forward
    }

    /// Push the committed page URL into the address field unless the user is mid-edit.
    fn echo_page_url(&mut self, cx: &mut Context<Self>) {
        if self.address_dirty {
            return;
        }
        let Some(url) = self.current_url.clone() else {
            return;
        };
        let shown = display_url(&url).to_owned();
        self.address.update(cx, |address, cx| {
            if address.content() != shown {
                address.set_content(shown, cx);
            }
        });
        self.address_dirty = false;
    }

    pub fn navigate_to_input(&mut self, raw: String, cx: &mut Context<Self>) {
        let Some(target) = resolve_address(&raw) else {
            return;
        };
        let url = match target {
            AddressTarget::Url(url) => url,
            AddressTarget::Search(query) => search_url(&query),
        };
        self.navigate_to_url(url, cx);
    }

    pub fn navigate_to_url(&mut self, url: String, cx: &mut Context<Self>) {
        if let Some(host) = &self.host {
            host.load_url(&url);
        }
        self.navigation_requested = true;
        self.loading = true;
        self.current_url = Some(url);
        self.address_dirty = false;
        self.echo_page_url(cx);
        self.focus_page(cx);
        cx.notify();
    }

    fn focus_page(&mut self, _cx: &mut Context<Self>) {
        if let Some(host) = self.host.clone() {
            host.focus();
        }
    }

    pub fn focus_default(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.navigation_requested {
            self.focus_page(cx);
            window.focus(&self.focus_handle, cx);
        } else {
            self.focus_address(window, cx);
        }
    }

    pub fn focus_address(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.address.update(cx, |address, cx| {
            address.select_all_text(cx);
        });
        window.focus(&self.address.read(cx).focus(), cx);
        self.reclaim_native_keyboard(cx);
        cx.notify();
    }

    pub fn restore_address(&mut self, cx: &mut Context<Self>) {
        self.address_dirty = false;
        self.echo_page_url(cx);
        cx.notify();
    }

    /// Per-frame push from the app: whether this surface is the visible right
    /// panel tab, and whether a GPUI overlay is open above it.
    pub fn sync_native_state(
        &mut self,
        surface_visible: bool,
        occluded: bool,
        cx: &mut Context<Self>,
    ) {
        let _occlusion_started = occluded && !self.occluded;
        self.occluded = occluded;

        let Some(host) = self.host.clone() else {
            return;
        };
        let has_page = self.navigation_requested;

        if !occluded && (self.snapshot.is_some() || self.snapshot_pending) {
            self.snapshot = None;
            self.snapshot_pending = false;
            self.snapshot_epoch += 1;
        }

        let covered_by_snapshot = occluded && !self.snapshot_pending;
        let show = surface_visible && has_page && !covered_by_snapshot;
        if !show && host.native_focus_within() {
            self.reclaim_native_keyboard(cx);
        }
        host.set_visible(show);
    }

    pub fn reclaim_native_keyboard(&mut self, _cx: &mut Context<Self>) {
        if let Some(host) = self.host.clone() {
            host.focus_parent();
        }
    }

    pub fn reconcile_focus(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let Some(host) = &self.host else {
            return;
        };

        let natively_focused = host.native_focus_within();
        let native_became_focused = natively_focused && !self.was_natively_focused;
        let native_lost_focus = !natively_focused && self.was_natively_focused;

        let window_focus = window.focused(cx);
        let window_focus_changed = window_focus != self.last_window_focus;

        if native_lost_focus && !window_focus_changed && self.focus_handle.is_focused(window) {
            self.reclaim_native_keyboard(cx);
        } else if native_became_focused && !window_focus_changed {
            if self.address.read(cx).focus().is_focused(window) {
                self.reclaim_native_keyboard(cx);
            } else {
                window.focus(&self.focus_handle, cx);
            }
        }

        self.was_natively_focused = natively_focused;
        self.last_window_focus = window_focus;
    }

    pub fn go_back(&mut self, cx: &mut Context<Self>) {
        if let Some(host) = &self.host {
            host.go_back();
            self.can_go_back = host.can_go_back();
            self.can_go_forward = host.can_go_forward();
            cx.notify();
        }
    }

    pub fn go_forward(&mut self, cx: &mut Context<Self>) {
        if let Some(host) = &self.host {
            host.go_forward();
            self.can_go_back = host.can_go_back();
            self.can_go_forward = host.can_go_forward();
            cx.notify();
        }
    }

    pub fn reload(&mut self, cx: &mut Context<Self>) {
        if let Some(host) = &self.host {
            if self.navigation_requested {
                host.reload();
                self.loading = true;
                cx.notify();
            }
        }
    }

    pub fn hard_reload(&mut self, cx: &mut Context<Self>) {
        if let Some(host) = &self.host {
            if self.navigation_requested {
                host.hard_reload();
                self.loading = true;
                cx.notify();
            }
        }
    }

    pub fn stop_loading(&mut self, cx: &mut Context<Self>) {
        if let Some(host) = &self.host {
            host.stop();
            self.loading = false;
            cx.notify();
        }
    }

    pub fn toggle_devtools(&mut self) {
        if let Some(host) = &self.host {
            if host.is_devtools_open() {
                host.close_devtools();
            } else {
                host.open_devtools();
            }
        }
    }

    pub fn open_external(&self, cx: &mut Context<Self>) {
        if let Some(url) = &self.current_url {
            cx.open_url(url);
        }
    }

    pub fn webview_copy(&self) {
        if let Some(host) = &self.host {
            host.evaluate_script("document.execCommand('copy')");
        }
    }

    pub fn webview_cut(&self) {
        if let Some(host) = &self.host {
            host.evaluate_script("document.execCommand('cut')");
        }
    }

    pub fn webview_paste(&self) {
        if let Some(host) = &self.host {
            host.evaluate_script("document.execCommand('paste')");
        }
    }

    pub fn webview_select_all(&self) {
        if let Some(host) = &self.host {
            host.evaluate_script("document.execCommand('selectAll')");
        }
    }

    /// The address input's context menu floats above the native webview's area,
    /// so the app's occlusion sync needs to know when it is open.
    pub fn overlay_open(&self, cx: &App) -> bool {
        self.address.read(cx).context_menu_open()
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
    ) -> Stateful<Div> {
        let tooltip_str = tooltip.into();
        let base = div()
            .id(id)
            .size(px(26.0))
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

    fn render_toolbar(&self, cx: &mut Context<Self>) -> Div {
        let theme = Theme::current(cx);
        let has_page = self.navigation_requested;
        let secure = self.current_url.as_deref().is_some_and(is_secure_url);
        let progress = self.loading.then_some(0.65f32);

        div()
            .h(px(TOOLBAR_HEIGHT))
            .flex_none()
            .px(px(10.0))
            .flex()
            .items_center()
            .gap(px(2.0))
            .border_b_1()
            .border_color(theme.border)
            .bg(theme.surface)
            .child(self.toolbar_button(
                "browser-back",
                IconName::ArrowLeft,
                self.can_go_back,
                "Back (⌘[)",
                theme,
                |this, _, cx| this.go_back(cx),
                cx,
            ))
            .child(self.toolbar_button(
                "browser-forward",
                IconName::ArrowRight,
                self.can_go_forward,
                "Forward (⌘])",
                theme,
                |this, _, cx| this.go_forward(cx),
                cx,
            ))
            .child(if self.loading {
                self.toolbar_button(
                    "browser-stop",
                    IconName::X,
                    true,
                    "Stop loading (Esc)",
                    theme,
                    |this, _, cx| this.stop_loading(cx),
                    cx,
                )
            } else {
                self.toolbar_button(
                    "browser-reload",
                    IconName::RotateCw,
                    has_page,
                    "Reload (⌘R)",
                    theme,
                    |this, _, cx| this.reload(cx),
                    cx,
                )
            })
            .child(
                div()
                    .id("browser-address-container")
                    .h(px(28.0))
                    .px(px(8.0))
                    .rounded(px(6.0))
                    .border_1()
                    .border_color(theme.border)
                    .bg(theme.inset)
                    .flex()
                    .items_center()
                    .gap(px(6.0))
                    .min_w_0()
                    .flex_1()
                    .mx(px(4.0))
                    .relative()
                    .child(app_icon(
                        if secure {
                            IconName::Lock
                        } else {
                            IconName::Globe
                        },
                        12.0,
                        theme.text_tertiary,
                    ))
                    .child(div().flex_1().min_w_0().child(self.address.clone()))
                    .when_some(progress, |element, prog| {
                        element.child(
                            div()
                                .absolute()
                                .bottom_0()
                                .left_0()
                                .h(px(2.0))
                                .w(gpui::relative(prog))
                                .rounded_full()
                                .bg(theme.accent),
                        )
                    }),
            )
            .child(self.toolbar_button(
                "browser-open-external",
                IconName::ExternalLink,
                has_page,
                "Open in system browser",
                theme,
                |this, _, cx| this.open_external(cx),
                cx,
            ))
    }

    fn render_start_page(&self, theme: Theme, cx: &mut Context<Self>) -> Div {
        div()
            .flex_1()
            .min_h_0()
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .px(px(48.0))
            .pb(px(40.0))
            .bg(theme.canvas)
            .child(app_icon(IconName::Globe, 28.0, theme.text_ghost))
            .child(
                div()
                    .mt(px(14.0))
                    .text_size(px(14.0))
                    .font_weight(gpui::FontWeight::SEMIBOLD)
                    .text_color(theme.text)
                    .child("Browse the Web"),
            )
            .child(
                div()
                    .mt(px(6.0))
                    .max_w(px(320.0))
                    .text_center()
                    .text_size(px(12.0))
                    .line_height(px(18.0))
                    .text_color(theme.text_tertiary)
                    .child("Press ⌘L to search or enter a URL. Instant preview for local servers."),
            )
            .child(
                div()
                    .mt(px(20.0))
                    .flex()
                    .items_center()
                    .gap(px(8.0))
                    .child(self.quick_launch_button("localhost:3000", theme, cx))
                    .child(self.quick_launch_button("localhost:5173", theme, cx))
                    .child(self.quick_launch_button("localhost:8080", theme, cx)),
            )
    }

    fn quick_launch_button(&self, target: &'static str, theme: Theme, cx: &mut Context<Self>) -> impl IntoElement {
        let url = format!("http://{target}");
        div()
            .id(format!("quick-launch-{target}"))
            .px(px(10.0))
            .py(px(4.0))
            .rounded(px(5.0))
            .bg(theme.surface)
            .border_1()
            .border_color(theme.border)
            .text_size(px(11.0))
            .font_weight(gpui::FontWeight::MEDIUM)
            .text_color(theme.text_secondary)
            .cursor_pointer()
            .hover(|s| s.bg(theme.overlay).text_color(theme.text))
            .on_click(cx.listener(move |this, _, _, cx| {
                this.navigate_to_url(url.clone(), cx);
            }))
            .child(target)
    }

    fn render_host_error(&self, message: SharedString, theme: Theme) -> Div {
        div()
            .flex_1()
            .min_h_0()
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .px(px(48.0))
            .pb(px(40.0))
            .bg(theme.canvas)
            .child(app_icon(IconName::TriangleAlert, 28.0, theme.text_tertiary))
            .child(
                div()
                    .mt(px(14.0))
                    .text_size(px(14.0))
                    .font_weight(gpui::FontWeight::SEMIBOLD)
                    .text_color(theme.text)
                    .child("Browser Unavailable"),
            )
            .child(
                div()
                    .mt(px(6.0))
                    .max_w(px(340.0))
                    .text_center()
                    .text_size(px(12.0))
                    .line_height(px(18.0))
                    .text_color(theme.text_tertiary)
                    .child(message),
            )
    }

    fn render_page_area(&self, theme: Theme) -> Div {
        let host = self.host.clone();
        div()
            .flex_1()
            .min_h_0()
            .relative()
            .bg(theme.surface)
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
            .when_some(
                self.occluded.then(|| self.snapshot.clone()).flatten(),
                |element, snapshot| {
                    element.child(
                        img(snapshot)
                            .absolute()
                            .size_full()
                            .object_fit(ObjectFit::Fill),
                    )
                },
            )
    }
}

impl Focusable for BrowserView {
    fn focus_handle(&self, _: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Render for BrowserView {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        self.reconcile_focus(window, cx);
        if self.loading {
            window.request_animation_frame();
        }

        let body = if let Some(error) = self.host_error.clone() {
            self.render_host_error(error.into(), theme)
                .into_any_element()
        } else if self.navigation_requested {
            self.render_page_area(theme).into_any_element()
        } else {
            self.render_start_page(theme, cx).into_any_element()
        };

        div()
            .id("browser-surface")
            .track_focus(&self.focus_handle)
            .key_context(BROWSER_KEY_CONTEXT)
            .on_action(cx.listener(|this, _: &BrowserBack, _, cx| this.go_back(cx)))
            .on_action(cx.listener(|this, _: &BrowserForward, _, cx| this.go_forward(cx)))
            .on_action(cx.listener(|this, _: &BrowserReload, _, cx| this.reload(cx)))
            .on_action(cx.listener(|this, _: &BrowserHardReload, _, cx| this.hard_reload(cx)))
            .on_action(cx.listener(|this, _: &BrowserStop, _, cx| this.stop_loading(cx)))
            .on_action(cx.listener(|this, _: &BrowserDevtools, _, _| this.toggle_devtools()))
            .on_action(cx.listener(|this, _: &FocusBrowserAddress, window, cx| {
                this.focus_address(window, cx);
            }))
            .on_action(cx.listener(|this, _: &WebviewCopy, _, _| this.webview_copy()))
            .on_action(cx.listener(|this, _: &WebviewCut, _, _| this.webview_cut()))
            .on_action(cx.listener(|this, _: &WebviewPaste, _, _| this.webview_paste()))
            .on_action(cx.listener(|this, _: &WebviewSelectAll, _, _| this.webview_select_all()))
            .size_full()
            .min_h_0()
            .flex()
            .flex_col()
            .child(self.render_toolbar(cx))
            .child(body)
    }
}
