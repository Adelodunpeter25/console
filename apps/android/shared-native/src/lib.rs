//! JNI backend for the Android app.
//!
//! [`shared`] exposes the pure logic in `console-shared` (diffs, pairing,
//! labels, icons, markdown mending) to Kotlin `SharedNative`. It used to
//! also carry the WebTransport client (`WtNative`, `bridge`); WebTransport
//! is gone and Android rides SSE/WS only.
//!
//! Build: `apps/android/shared-native/build.sh`. It stages
//! `libconsole_shared_android.so` into the app's jniLibs.

mod shared;

/// Run a JNI entry point, turning a panic into an error code.
///
/// A JNI function is `extern "system"`, and unwinding out of one aborts the
/// whole app — a Rust bug in this library would take the user's session with
/// it. Every entry point goes through here.
fn guard<T>(fallback: T, body: impl FnOnce() -> T) -> T {
    // `&mut JNIEnv` is not `UnwindSafe`, and that is fine: the JNIEnv is only
    // borrowed for the duration of the call and nothing escapes a panic.
    match std::panic::catch_unwind(std::panic::AssertUnwindSafe(body)) {
        Ok(value) => value,
        Err(payload) => {
            let message = payload
                .downcast_ref::<&str>()
                .map(|s| (*s).to_string())
                .or_else(|| payload.downcast_ref::<String>().cloned())
                .unwrap_or_else(|| "unknown panic".to_string());
            log::error!("Rust panic escaped a JNI call: {message}");
            fallback
        }
    }
}

/// Install the Android log sink once. Called from the JNI entry points so a
/// panic anywhere below prints to logcat instead of a silent abort.
fn init_logging() {
    static ONCE: std::sync::Once = std::sync::Once::new();
    ONCE.call_once(|| {
        android_logger::init_once(
            android_logger::Config::default()
                .with_tag("console_shared")
                .with_max_level(log::LevelFilter::Debug),
        );
        // Explicit hook: the default one writes to stderr, which Android
        // discards, so a panic would otherwise be a bare SIGABRT.
        let _ = std::panic::set_hook(Box::new(|info| {
            log::error!("PANIC in JNI backend: {info}");
        }));
    });
}

use jni::objects::{JString};
use jni::sys::jbyteArray;
use jni::JNIEnv;

fn arg(env: &mut JNIEnv, value: &JString) -> Option<String> {
    env.get_string(value).ok().map(|value| value.into())
}

fn byte_array(env: &mut JNIEnv, bytes: &[u8]) -> jbyteArray {
    match env.byte_array_from_slice(bytes) {
        Ok(array) => array.into_raw(),
        Err(_) => std::ptr::null_mut(),
    }
}
