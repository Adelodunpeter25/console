//! `NSApplication` conformance retrofit for CEF.
//!
//! CEF on macOS requires the application object to conform to
//! `CefAppProtocol` (event handling, modal loops, popups, DevTools
//! windows) and normally gets it via an `NSApplication` subclass created
//! before anything else (see `cefsimple`). GPUI owns `NSApplication` here —
//! and it is a private subclass, not vanilla `NSApplication` — so an
//! `object_setClass` swap is not possible (the subclass may carry extra
//! ivars, and the size check would be unsound).
//!
//! Instead this module injects the two required methods
//! (`isHandlingSendEvent` / `setHandlingSendEvent:`) directly into the
//! live application's class with `class_addMethod`, and registers
//! `CefAppProtocol` conformance with `class_addProtocol` when the CEF
//! framework has registered the protocol. Injection is idempotent and
//! works for both vanilla `NSApplication` and GPUI's subclass. The IMPs
//! are borrowed from `ConsoleApplication` below so the type encodings
//! stay compiler-checked.

use std::sync::atomic::{AtomicBool, Ordering};

use cef::application_mac::{CefAppProtocol, CrAppControlProtocol, CrAppProtocol};
use objc2::runtime::{AnyClass, Bool};
use objc2::{define_class, ffi, sel, ClassType, ProtocolType};
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

/// Copy one method implementation from `ConsoleApplication` into `target`.
/// Returns true when the target now responds to `sel` (already did, or the
/// add succeeded). Never replaces an existing implementation.
fn ensure_method(target: *mut AnyClass, sel: objc2::runtime::Sel) -> bool {
    // SAFETY: `target` is the live NSApplication class, valid for the
    // process lifetime. `class_respondsToSelector` is safe to call here.
    let target_ref: &AnyClass = unsafe { &*target };
    if target_ref.responds_to(sel) {
        return true;
    }
    let source = ConsoleApplication::class();
    let Some(method) = source.instance_method(sel) else {
        log::warn!("CEF: ConsoleApplication is missing {sel}");
        return false;
    };
    let imp = method.implementation();
    // SAFETY: `method` is a valid Method pointer; the returned encoding
    // string lives as long as the method (static class, so 'static).
    let types = unsafe { ffi::method_getTypeEncoding(method as *const _) };
    if types.is_null() {
        log::warn!("CEF: no type encoding for {sel}");
        return false;
    }
    // SAFETY: adding a method that the class does not already implement;
    // the IMP and types come from the same source method, so the signature
    // matches what Objective-C callers expect.
    let added = unsafe { ffi::class_addMethod(target, sel, imp, types) };
    if !added.as_bool() {
        log::warn!("CEF: class_addMethod failed for {sel}");
        return target_ref.responds_to(sel);
    }
    true
}

/// Inject CEF's required `NSApplication` methods into the live app object,
/// whatever its class. Idempotent; safe to call repeatedly. Must run on the
/// main thread, after the CEF framework is loaded (so the `CefAppProtocol`
/// protocol object exists) and before `CefInitialize`.
pub(super) fn conform_ns_application() {
    let Some(mtm) = MainThreadMarker::new() else {
        return;
    };
    let app = NSApplication::sharedApplication(mtm);
    // The live class: vanilla `NSApplication` in cefsimple, GPUI's private
    // subclass here. Injection works for both.
    let target: *mut AnyClass = app.class() as *const AnyClass as *mut AnyClass;
    let is_sel = sel!(isHandlingSendEvent);
    let set_sel = sel!(setHandlingSendEvent:);
    let ok_is = ensure_method(target, is_sel);
    let ok_set = ensure_method(target, set_sel);
    // Register protocol conformance so `conformsToProtocol:` checks pass.
    // Best-effort: the protocol object only exists after the CEF framework
    // is loaded; the methods above are what actually prevent the
    // `doesNotRecognizeSelector` crash in `do_message_loop_work`.
    if let Some(proto) = <dyn CefAppProtocol>::protocol() {
        // SAFETY: `target` is valid; `proto` is a registered protocol.
        unsafe {
            ffi::class_addProtocol(target, proto);
        }
    } else {
        log::debug!("CEF: CefAppProtocol not registered yet; methods injected without protocol registration");
    }
    if ok_is && ok_set {
        log::debug!("CEF: NSApplication conformance injected");
    } else {
        log::warn!("CEF: NSApplication method injection incomplete");
    }
}
