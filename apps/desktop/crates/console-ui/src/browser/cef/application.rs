//! `NSApplication` conformance retrofit for CEF.
//!
//! CEF on macOS requires the application object to conform to
//! `CefAppProtocol` (event handling, modal loops, popups, DevTools
//! windows) and normally gets it via an `NSApplication` subclass created
//! before anything else (see `cefsimple`). GPUI owns `NSApplication` here,
//! so instead this module swaps the class of the live object with
//! `object_setClass` before `CefInitialize` runs.
//!
//! This is safe because `ConsoleApplication` adds no ivars (identical
//! instance size, which `object_setClass` requires) and no method overrides:
//! every existing message resolves through inheritance exactly as before,
//! and only the three protocol methods are added. The swap is skipped
//! unless the live object is a vanilla `NSApplication` (a subclass could
//! carry extra ivars).

use std::sync::atomic::{AtomicBool, Ordering};

use cef::application_mac::{CefAppProtocol, CrAppControlProtocol, CrAppProtocol};
use objc2::runtime::{AnyClass, AnyObject, Bool};
use objc2::{ClassType, define_class, msg_send};
use objc2_app_kit::NSApplication;
use objc2_foundation::MainThreadMarker;

static HANDLING_SEND_EVENT: AtomicBool = AtomicBool::new(false);

define_class!(
    #[unsafe(super(NSApplication))]
    struct ConsoleApplication;

    unsafe impl CefAppProtocol for ConsoleApplication {}

    unsafe impl CrAppControlProtocol for ConsoleApplication {
        #[unsafe(method(setHandlingSendEvent:))]
        unsafe fn set_handling_send_event(&self, handling_send_event: Bool) {
            HANDLING_SEND_EVENT.store(handling_send_event.as_bool(), Ordering::SeqCst);
        }
    }

    unsafe impl CrAppProtocol for ConsoleApplication {
        #[unsafe(method(isHandlingSendEvent))]
        unsafe fn is_handling_send_event(&self) -> Bool {
            Bool::new(HANDLING_SEND_EVENT.load(Ordering::SeqCst))
        }
    }
);

/// Swap the live `NSApplication` to `ConsoleApplication`, adding CEF's
/// required protocol conformance. Idempotent; no-op when the object is
/// already conforming or is not a vanilla `NSApplication`. Must run on the
/// main thread before `CefInitialize`.
pub(super) fn conform_ns_application() {
    let Some(mtm) = MainThreadMarker::new() else {
        return;
    };
    let app = NSApplication::sharedApplication(mtm);
    let current: &AnyClass = unsafe { msg_send![&*app, class] };
    if current == ConsoleApplication::class() {
        return;
    }
    if current != NSApplication::class() {
        log::warn!(
            "CEF: NSApplication is subclassed ({}); skipping CefAppProtocol retrofit",
            current.name().to_string_lossy()
        );
        return;
    }
    unsafe {
        AnyObject::set_class(&*app, ConsoleApplication::class());
    }
    log::debug!("CEF: NSApplication retrofitted with CefAppProtocol conformance");
}
