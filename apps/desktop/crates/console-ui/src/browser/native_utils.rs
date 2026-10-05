//! Shared native-app plumbing for the browser backends (wry and CEF).
//!
//! Both backends embed a native child `NSView` in the GPUI window, so both
//! need native focus tracking (`ResponderObserver`, via `firstResponder`
//! KVO) and overlay z-order repair (`lower_below_scene_overlay`).

use std::ffi::c_void;
use std::ptr::null_mut;

use objc2::rc::Retained;
use objc2::runtime::AnyObject;
use objc2::{AnyThread, DefinedClass, define_class, msg_send};
use objc2_app_kit::{NSApplication, NSEventType, NSView, NSWindow};
use objc2_foundation::{
    MainThreadMarker, NSDictionary, NSKeyValueChangeKey, NSKeyValueObservingOptions,
    NSObjectNSKeyValueObserverRegistration, NSObjectProtocol, NSProcessInfo, NSString, ns_string,
};

pub(crate) fn recent_user_gesture() -> bool {
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

pub(crate) struct ResponderObserverIvars {
    window: Retained<NSWindow>,
    handler: Box<dyn Fn(bool)>,
}

define_class!(
    #[unsafe(super(objc2::runtime::NSObject))]
    #[ivars = ResponderObserverIvars]
    pub(crate) struct ResponderObserver;

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
    pub(crate) fn new(window: Retained<NSWindow>, handler: Box<dyn Fn(bool)>) -> Retained<Self> {
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

/// Park a child view below any `GPUIOverlayView` sibling so menus, palettes,
/// and popovers paint above the native surface.
pub(crate) fn lower_below_scene_overlay(view: &NSView) {
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
