//! Native webview hosting interface and platform integration.
//!
//! Based on the architecture from Waku (`waku/src/browser.rs`) and `docs/plan/browser.md`.
//! Manages native view geometry synchronization, visibility toggling, focus reconciliation,
//! and fallback snapshot pixel repacking.

use std::path::PathBuf;

#[cfg(not(target_os = "macos"))]
use std::cell::Cell;
#[cfg(not(target_os = "macos"))]
use gpui::{Bounds, Pixels};

#[cfg(target_os = "macos")]
mod macos_host {
    use std::cell::Cell;
    use std::ffi::c_void;
    use std::ptr::null_mut;

    use gpui::{Bounds, Pixels};
    use objc2::rc::Retained;
    use objc2::runtime::AnyObject;
    use objc2::{define_class, msg_send, AllocAnyThread, DefinedClass};
    use objc2_app_kit::{NSApplication, NSEventType, NSView, NSWindow};
    use objc2_foundation::{
        ns_string, MainThreadMarker, NSDictionary, NSKeyValueChangeKey, NSKeyValueObservingOptions,
        NSObjectNSKeyValueObserverRegistration, NSObjectProtocol, NSProcessInfo, NSString,
    };
    use objc2_web_kit::WKWebView;
    use wry::dpi::{LogicalPosition, LogicalSize};
    use wry::WebViewExtMacOS;

    fn recent_user_gesture() -> bool {
        let Some(mtm) = MainThreadMarker::new() else {
            return false;
        };
        let Some(event) = NSApplication::sharedApplication(mtm).currentEvent() else {
            return false;
        };
        let pressed = matches!(
            event.r#type(),
            NSEventType::LeftMouseDown
                | NSEventType::LeftMouseUp
                | NSEventType::RightMouseDown
                | NSEventType::OtherMouseDown
        );
        pressed && NSProcessInfo::processInfo().systemUptime() - event.timestamp() < 0.5
    }

    pub(super) struct ResponderObserverIvars {
        window: Retained<NSWindow>,
        handler: Box<dyn Fn(bool)>,
    }

    define_class!(
        #[unsafe(super(objc2::runtime::NSObject))]
        #[ivars = ResponderObserverIvars]
        pub(super) struct ResponderObserver;

        impl ResponderObserver {
            #[unsafe(method(observeValueForKeyPath:ofObject:change:context:))]
            fn observe_value_for_key_path(
                &self,
                key_path: Option<&NSString>,
                _of_object: Option<&AnyObject>,
                _change: Option<&NSDictionary<NSKeyValueChangeKey, AnyObject>>,
                _context: *mut c_void,
            ) {
                if key_path.is_some_and(|path| path.isEqualToString(ns_string!("firstResponder"))) {
                    (self.ivars().handler)(recent_user_gesture());
                }
            }
        }

        unsafe impl NSObjectProtocol for ResponderObserver {}
    );

    impl ResponderObserver {
        fn new(window: Retained<NSWindow>, handler: Box<dyn Fn(bool)>) -> Retained<Self> {
            let observer = Self::alloc().set_ivars(ResponderObserverIvars { window, handler });
            let observer: Retained<Self> = unsafe { msg_send![super(observer), init] };
            unsafe {
                observer
                    .ivars()
                    .window
                    .addObserver_forKeyPath_options_context(
                        &observer,
                        ns_string!("firstResponder"),
                        NSKeyValueObservingOptions::New,
                        null_mut(),
                    );
            }
            observer
        }
    }

    impl Drop for ResponderObserver {
        fn drop(&mut self) {
            unsafe {
                self.ivars()
                    .window
                    .removeObserver_forKeyPath(self, ns_string!("firstResponder"));
            }
        }
    }

    pub struct WebviewHost {
        pub webview: wry::WebView,
        wk: Retained<WKWebView>,
        last_bounds: Cell<Option<(i32, i32, i32, i32)>>,
        visible: Cell<bool>,
        _responder_observer: Option<Retained<ResponderObserver>>,
    }

    impl WebviewHost {
        pub fn new(webview: wry::WebView, on_responder_change: Box<dyn Fn(bool)>) -> Self {
            let wk: Retained<WKWebView> = Retained::into_super(webview.webview());
            lower_below_scene_overlay(&wk);
            let responder_observer = wk
                .window()
                .map(|window| ResponderObserver::new(window, on_responder_change));
            Self {
                webview,
                wk,
                last_bounds: Cell::new(None),
                visible: Cell::new(false),
                _responder_observer: responder_observer,
            }
        }

        pub fn wk(&self) -> &WKWebView {
            &self.wk
        }

        pub fn ns_view(&self) -> &NSView {
            &self.wk
        }

        pub fn sync_bounds(&self, bounds: Bounds<Pixels>, _scale: f32) {
            let left = f32::from(bounds.origin.x).round() as i32;
            let top = f32::from(bounds.origin.y).round() as i32;
            let right = f32::from(bounds.origin.x + bounds.size.width).round() as i32;
            let bottom = f32::from(bounds.origin.y + bounds.size.height).round() as i32;
            if self.last_bounds.get() == Some((left, top, right, bottom)) {
                return;
            }
            self.last_bounds.set(Some((left, top, right, bottom)));
            let _ = self.webview.set_bounds(wry::Rect {
                position: LogicalPosition::new(f64::from(left), f64::from(top)).into(),
                size: LogicalSize::new(f64::from((right - left).max(0)), f64::from((bottom - top).max(0))).into(),
            });
        }

        pub fn set_visible(&self, visible: bool) {
            if self.visible.get() == visible {
                return;
            }
            self.visible.set(visible);
            let _ = self.webview.set_visible(visible);
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
            let _ = self.webview.load_url(url);
        }

        pub fn go_back(&self) {
            let _ = self.webview.go_back();
        }

        pub fn go_forward(&self) {
            let _ = self.webview.go_forward();
        }

        pub fn reload(&self) {
            let _ = self.webview.reload();
        }

        pub fn hard_reload(&self) {
            unsafe { self.wk().reloadFromOrigin() };
        }

        pub fn stop(&self) {
            unsafe { self.wk().stopLoading() };
        }

        pub fn evaluate_script(&self, script: &str) {
            let _ = self.webview.evaluate_script(script);
        }

        pub fn open_devtools(&self) {
            self.webview.open_devtools();
        }

        pub fn close_devtools(&self) {
            self.webview.close_devtools();
        }

        pub fn is_devtools_open(&self) -> bool {
            self.webview.is_devtools_open()
        }

        pub fn focus(&self) {
            let _ = self.webview.focus();
        }

        pub fn focus_parent(&self) {
            let _ = self.webview.focus_parent();
        }

        pub fn can_go_back(&self) -> bool {
            self.webview.can_go_back().unwrap_or(false)
        }

        pub fn can_go_forward(&self) -> bool {
            self.webview.can_go_forward().unwrap_or(false)
        }

        pub fn estimated_progress(&self) -> f64 {
            unsafe { self.wk().estimatedProgress() }
        }
    }

    fn lower_below_scene_overlay(view: &NSView) {
        use objc2_app_kit::NSWindowOrderingMode;

        let Some(superview) = (unsafe { view.superview() }) else {
            return;
        };
        for sibling in superview.subviews().iter() {
            if sibling.class().name() == c"GPUIOverlayView" {
                superview.addSubview_positioned_relativeTo(
                    view,
                    NSWindowOrderingMode::Below,
                    Some(&sibling),
                );
                return;
            }
        }
    }
}

#[cfg(target_os = "macos")]
pub use macos_host::WebviewHost;

#[cfg(not(target_os = "macos"))]
pub struct WebviewHost {
    last_bounds: Cell<Option<(i32, i32, i32, i32)>>,
    visible: Cell<bool>,
}

#[cfg(not(target_os = "macos"))]
impl WebviewHost {
    pub fn new() -> Self {
        Self {
            last_bounds: Cell::new(None),
            visible: Cell::new(false),
        }
    }
    pub fn sync_bounds(&self, _bounds: Bounds<Pixels>, _scale: f32) {}
    pub fn set_visible(&self, _visible: bool) {}
    pub fn native_focus_within(&self) -> bool { false }
    pub fn can_go_back(&self) -> bool { false }
    pub fn can_go_forward(&self) -> bool { false }
    pub fn go_back(&self) {}
    pub fn go_forward(&self) {}
    pub fn reload(&self) {}
    pub fn hard_reload(&self) {}
    pub fn stop(&self) {}
    pub fn load_url(&self, _url: &str) {}
    pub fn evaluate_script(&self, _script: &str) {}
    pub fn open_devtools(&self) {}
    pub fn close_devtools(&self) {}
    pub fn is_devtools_open(&self) -> bool { false }
    pub fn focus(&self) {}
    pub fn focus_parent(&self) {}
    pub fn estimated_progress(&self) -> f64 { 0.0 }
}

/// Repack an `NSBitmapImageRep` pixel buffer as tight BGRA rows for `gpui::RenderImage`.
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

    let [b, g, r] = match (samples, alpha_first, little_endian_words) {
        (4, true, true) => [0, 1, 2],
        (4, false, false) => [2, 1, 0],
        (4, true, false) => [3, 2, 1],
        (4, false, true) => [1, 2, 3],
        _ => [2, 1, 0],
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

/// Compute a collision-free download destination path.
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
                .filter(|name| !name.is_empty())
                .map(ToOwned::to_owned)
        })
        .unwrap_or_else(|| "download".to_owned());

    let mut destination = base.join(&name);
    let (stem, ext) = match name.rsplit_once('.') {
        Some((stem, ext)) if !stem.is_empty() => (stem.to_owned(), format!(".{ext}")),
        _ => (name, String::new()),
    };

    let mut counter = 1;
    while destination.exists() {
        destination = base.join(format!("{stem} ({counter}){ext}"));
        counter += 1;
    }
    Some(destination)
}
