//! CEF-backed `WebviewHost`: a child-view Chromium surface.
//!
//! Mirrors the wry backend's method surface exactly (`sync_bounds`,
//! `set_visible`, navigation, script evaluation, snapshots, devtools,
//! focus), so owning views need no backend conditionals. Differences from
//! the wry backend, all documented at the relevant method:
//! - `user_agent` and `allow_navigation` are currently ignored (CEF default
//!   UA is equivalent for our pages; navigation gating needs a
//!   `RequestHandler` in a later slice).
//! - `html` loads as a `data:` URL instead of a native initial document.
//! - Page-to-host IPC travels over the console bridge (see `client.rs`).

use std::cell::Cell;
use std::ffi::c_void;

use base64::Engine as _;
use gpui::{Bounds, Pixels, Window};
use objc2::rc::Retained;
use objc2_app_kit::{NSBitmapImageFileType, NSBitmapImageRep, NSResponder, NSView};
use objc2_foundation::{MainThreadMarker, NSDictionary, NSPoint, NSRect, NSSize};
use raw_window_handle::{HasWindowHandle, RawWindowHandle};

use cef::{
    Browser, BrowserSettings, CefString, ImplBrowser, ImplBrowserHost, ImplFrame, Rect as CefRect,
    Registration, RuntimeStyle, WindowInfo, browser_host_create_browser_sync,
};

use crate::browser::cef::client::{
    ClientBundle, NavCallbacks, SharedRef, TARGET_INFO_MESSAGE_ID, TargetInfoObserver, build_client,
    progress_of,
};
use crate::browser::cef::runtime::ensure_initialized;
use crate::browser::host::{HostCallbacks, HostContent};
use crate::browser::native_utils::{ResponderObserver, lower_below_scene_overlay};

/// Initial CEF child bounds (physical pixels). Corrected by the first
/// `sync_bounds` from the layout canvas.
const INITIAL_BOUNDS: CefRect = CefRect {
    x: 0,
    y: 0,
    width: 800,
    height: 600,
};

pub struct WebviewHost {
    browser: Browser,
    _client_bundle: ClientBundle,
    /// Keeps the `Target.getTargetInfo` observer registered for the life of
    /// the browser (dropping it unregisters the observer).
    _target_info_registration: Option<Registration>,
    shared: SharedRef,
    cef_view: Retained<NSView>,
    parent_view: Retained<NSView>,
    last_bounds: Cell<Option<(i32, i32, i32, i32)>>,
    visible: Cell<bool>,
    _responder_observer: Option<Retained<ResponderObserver>>,
}

impl WebviewHost {
    /// Build a Chromium child view for `window`, wiring `content` and
    /// `callbacks`. Returns `Err` (surfaced as `host_error`, exactly like a
    /// wry build failure) when CEF is unavailable or browser creation fails.
    pub fn create(
        window: &mut Window,
        content: HostContent,
        callbacks: HostCallbacks,
    ) -> Result<Self, String> {
        if !ensure_initialized() {
            log::warn!("CEF: runtime unavailable, browser surface not created");
            return Err("CEF is unavailable: the Chromium framework is not bundled with this build.".to_string());
        }

        let parent_view = content_view(window)?;
        let HostContent {
            user_agent: _user_agent,
            initialization_script,
            html,
            allow_navigation: _allow_navigation,
            on_ipc,
            on_title,
            on_new_window_url,
        } = content;
        // NOTE: `_user_agent` is intentionally ignored: setting a custom UA
        // needs an `App::on_before_command_line_processing` handler (a later
        // slice); CEF's default UA is equivalent for our pages. NOTE:
        // `_allow_navigation` is intentionally ignored: both call sites use
        // allow-all/default, which is CEF's default; gating needs a
        // `RequestHandler` (a later slice).
        let HostCallbacks {
            on_responder_change,
            on_nav_start,
            on_nav_finish,
            on_nav_error,
        } = callbacks;

        let bundle = build_client(
            NavCallbacks {
                on_start: on_nav_start,
                on_finish: on_nav_finish,
                on_error: on_nav_error,
            },
            on_title,
            on_ipc,
            on_new_window_url,
            initialization_script,
        );
        let mut client = bundle.client.clone();

        let initial_url = match html {
            Some(html) => format!(
                "data:text/html;base64,{}",
                base64::engine::general_purpose::STANDARD.encode(html.as_bytes())
            ),
            None => "about:blank".to_string(),
        };
        let window_info =
            WindowInfo {
                // ALLOY, not DEFAULT (Chrome): the Chrome runtime manages
                // child bounds through the Views framework and overrides the
                // host's `setFrame`, leaving the view floating at its initial
                // bounds. Alloy leaves the native view fully host-managed,
                // which `sync_bounds` requires.
                runtime_style: RuntimeStyle::ALLOY,
                ..Default::default()
            }
            .set_as_child(
                (&*parent_view as *const NSView).cast_mut() as *mut c_void,
                &INITIAL_BOUNDS,
            );
        let url = CefString::from(initial_url.as_str());
        let browser = browser_host_create_browser_sync(
            Some(&window_info),
            Some(&mut client),
            Some(&url),
            Some(&BrowserSettings::default()),
            None,
            None,
        )
        .ok_or_else(|| {
            log::warn!("CEF: browser_host_create_browser_sync returned no browser");
            "CEF failed to create the browser.".to_string()
        })?;

        let host = browser
            .host()
            .ok_or_else(|| "CEF browser has no host.".to_string())?;
        // Ask this browser for its own CDP target id. The result arrives on
        // `TargetInfoObserver` (same UI thread) and lands in `shared`.
        let mut target_observer = TargetInfoObserver::new(bundle.shared.clone());
        let target_info_registration =
            host.add_dev_tools_message_observer(Some(&mut target_observer));
        let method = CefString::from("Target.getTargetInfo");
        host.execute_dev_tools_method(TARGET_INFO_MESSAGE_ID, Some(&method), None);
        let raw_view = host.window_handle();
        if raw_view.is_null() {
            return Err("CEF browser has no native view.".to_string());
        }
        let cef_view: Retained<NSView> =
            unsafe { Retained::retain(raw_view as *mut NSView) }
                .ok_or_else(|| "CEF browser has no native view.".to_string())?;
        log::debug!(
            "CEF: native view {cef_view:p} attached under parent {parent_view:p}"
        );
        lower_below_scene_overlay(&cef_view);

        let responder_observer = cef_view
            .window()
            .map(|window| ResponderObserver::new(window, on_responder_change));
        bundle.shared.native_view.set(Some(cef_view.clone()));
        let this = Self {
            browser,
            shared: bundle.shared.clone(),
            _client_bundle: bundle,
            _target_info_registration: target_info_registration,
            cef_view,
            parent_view,
            last_bounds: Cell::new(None),
            // CEF hands us a child view that starts out shown, so the flag must
            // start `true`: it makes the `set_visible(false)` below really hide
            // it. With `false`, `set_visible`'s "already in that state" guard
            // skipped the hide, so a view created for a background tab sat on
            // screen (800x600, over the chat) until its first show/hide flip.
            visible: Cell::new(true),
            _responder_observer: responder_observer,
        };
        log::debug!("CEF: browser surface created for {initial_url}");
        this.set_visible(false);
        Ok(this)
    }

    /// The CDP target id of this browser, once CEF has reported it. `None`
    /// until the first `Target.getTargetInfo` result arrives.
    pub fn target_id(&self) -> Option<String> {
        self.shared.target_id.borrow().clone()
    }

    pub fn ns_view(&self) -> &NSView {
        &self.cef_view
    }

    /// Same coordinate mapping as the wry backend: GPUI `Bounds` are already
    /// logical points (which is why wry passes them through
    /// `Position::to_logical` unchanged and this method ignores `_scale`),
    /// so the child frame is those points flipped against the superview
    /// height, which is bottom-left-origin.
    pub fn sync_bounds(&self, bounds: Bounds<Pixels>, _scale: f32) {
        let left = f32::from(bounds.origin.x).round() as i32;
        let top = f32::from(bounds.origin.y).round() as i32;
        let right = f32::from(bounds.origin.x + bounds.size.width).round() as i32;
        let bottom = f32::from(bounds.origin.y + bounds.size.height).round() as i32;
        if self.last_bounds.get() == Some((left, top, right, bottom)) {
            return;
        }
        self.last_bounds.set(Some((left, top, right, bottom)));

        if self.cef_view.window().is_none() {
            return;
        }
        let Some(superview) = (unsafe { self.cef_view.superview() }) else {
            return;
        };
        let x = f64::from(left);
        let y = f64::from(top);
        let width = f64::from((right - left).max(0));
        let height = f64::from((bottom - top).max(0));
        let super_frame = superview.frame();
        let flipped = superview.isFlipped();
        let origin_y = if flipped {
            y
        } else {
            super_frame.size.height - y - height
        };
        log::debug!(
            "CEF: sync_bounds in=({left},{top},{right},{bottom}) superview={superview:p} super_h={} flipped={flipped} -> frame=({x:.1},{origin_y:.1},{width:.1},{height:.1})",
            super_frame.size.height,
        );
        self.cef_view.setFrame(NSRect {
            origin: NSPoint { x, y: origin_y },
            size: NSSize { width, height },
        });
        if let Some(host) = self.browser.host() {
            host.was_resized();
        }
    }

    pub fn set_visible(&self, visible: bool) {
        if self.visible.get() == visible {
            return;
        }
        self.visible.set(visible);
        self.cef_view.setHidden(!visible);
        if let Some(host) = self.browser.host() {
            host.was_hidden(if visible { 0 } else { 1 });
        }
    }

    pub fn native_focus_within(&self) -> bool {
        let view = self.ns_view();
        let Some(window) = view.window() else {
            return false;
        };
        window.firstResponder().is_some_and(|responder| {
            responder
                .downcast_ref::<NSView>()
                .is_some_and(|responder| responder.isDescendantOf(view))
        })
    }

    pub fn load_url(&self, url: &str) {
        if let Some(frame) = self.browser.main_frame() {
            frame.load_url(Some(&CefString::from(url)));
        }
    }

    /// Committed URL of the main frame, if any. Nil-safe like the wry
    /// backend: empty URLs report `None` so a transient blank never clobbers
    /// the address bar.
    pub fn current_url(&self) -> Option<String> {
        let frame = self.browser.main_frame()?;
        let url = CefString::from(&frame.url()).to_string();
        (!url.is_empty()).then(|| url)
    }

    pub fn go_back(&self) {
        self.browser.go_back();
    }

    pub fn go_forward(&self) {
        self.browser.go_forward();
    }

    pub fn reload(&self) {
        self.browser.reload();
    }

    pub fn hard_reload(&self) {
        self.browser.reload_ignore_cache();
    }

    pub fn stop(&self) {
        self.browser.stop_load();
    }

    /// Evaluate JS in the main frame. Like the wry backend this is
    /// fire-and-forget from the host's perspective; agent scripts report
    /// back over IPC.
    pub fn evaluate_script(&self, script: &str) {
        if let Some(frame) = self.browser.main_frame() {
            frame.execute_java_script(
                Some(&CefString::from(script)),
                Some(&CefString::from("console://evaluate")),
                0,
            );
        }
    }

    /// Capture the visible page as PNG by asking the CEF view to draw into a
    /// bitmap rep, then encoding exactly like the wry backend. Synchronous
    /// (unlike WKWebView snapshots) but still main-thread only.
    pub fn take_snapshot(
        &self,
        slot: std::rc::Rc<std::cell::RefCell<Option<Result<Vec<u8>, String>>>>,
    ) {
        if MainThreadMarker::new().is_none() {
            *slot.borrow_mut() = Some(Err("Snapshot must run on the main thread".to_string()));
            return;
        }
        let result = (|| {
            let bounds = self.cef_view.bounds();
            if bounds.size.width <= 0.0 || bounds.size.height <= 0.0 {
                return Err("Snapshot view is empty".to_string());
            }
            let rep = self
                .cef_view
                .bitmapImageRepForCachingDisplayInRect(bounds)
                .ok_or_else(|| "Snapshot could not allocate a bitmap".to_string())?;
            self.cef_view
                .cacheDisplayInRect_toBitmapImageRep(bounds, &rep);
            let tiff = rep
                .TIFFRepresentation()
                .ok_or_else(|| "Snapshot had no bitmap data".to_string())?;
            let bitmap = NSBitmapImageRep::imageRepWithData(&tiff)
                .ok_or_else(|| "Could not decode snapshot bitmap".to_string())?;
            let props = NSDictionary::new();
            let png = unsafe {
                bitmap.representationUsingType_properties(NSBitmapImageFileType::PNG, &props)
            }
            .ok_or_else(|| "Could not encode snapshot as PNG".to_string())?;
            Ok(png.to_vec())
        })();
        *slot.borrow_mut() = Some(result);
    }

    pub fn open_devtools(&self) {
        if let Some(host) = self.browser.host() {
            host.show_dev_tools(None, None, None, None);
        }
    }

    pub fn close_devtools(&self) {
        if let Some(host) = self.browser.host() {
            host.close_dev_tools();
        }
    }

    pub fn is_devtools_open(&self) -> bool {
        self.browser
            .host()
            .is_some_and(|host| host.has_dev_tools() != 0)
    }

    /// Focus the page (same contract as wry's `focus`).
    pub fn focus(&self) {
        if let Some(host) = self.browser.host() {
            host.set_focus(1);
        }
    }

    /// Return keyboard focus to the GPUI parent view (mirrors wry's
    /// `focus_parent`, which makes the webview's parent the first
    /// responder).
    pub fn focus_parent(&self) {
        if let Some(window) = self.cef_view.window() {
            let responder: &NSResponder = &self.parent_view;
            window.makeFirstResponder(Some(responder));
        }
    }

    pub fn can_go_back(&self) -> bool {
        self.browser.can_go_back() != 0
    }

    pub fn can_go_forward(&self) -> bool {
        self.browser.can_go_forward() != 0
    }

    /// Real loading progress from `on_loading_progress_change` (the wry
    /// backend has no equivalent; callers currently ignore it).
    pub fn estimated_progress(&self) -> f64 {
        progress_of(&self.shared)
    }
}

impl Drop for WebviewHost {
    fn drop(&mut self) {
        // Flag teardown first so racing load callbacks skip page script.
        // Then ask CEF to close; the native view is detached later in
        // `on_before_close`, when CEF is done with it. The leaked retain
        // below guards against CEF touching the view after our drop while
        // its async close is still in flight.
        self.shared.closing.set(true);
        if let Some(host) = self.browser.host() {
            // Graceful close (0), not force (1): force tears the browser down
            // synchronously from inside this drop, re-entering CEF mid-GPUI
            // teardown and taking the app window with it.
            host.close_browser(0);
        }
        let leaked = self.cef_view.clone();
        std::mem::forget(leaked);
    }
}

/// The GPUI window's root view: the same attach point wry uses (it treats
/// the raw handle's `ns_view` as the parent), so child frames live in the
/// same coordinate space as `sync_bounds` expects.
fn content_view(window: &mut Window) -> Result<Retained<NSView>, String> {
    let handle = HasWindowHandle::window_handle(&*window)
        .map_err(|error| format!("GPUI window has no raw handle: {error:?}"))?;
    let RawWindowHandle::AppKit(appkit) = handle.as_raw() else {
        return Err("GPUI window is not an AppKit window.".to_string());
    };
    unsafe { Retained::retain(appkit.ns_view.as_ptr() as *mut NSView) }
        .ok_or_else(|| "GPUI window has no root view.".to_string())
}
