//! QUIC/WebTransport client for Android (plan Phase 3).
//!
//! **The transport is not written here.** It is `console-core` — the same
//! crate the desktop client uses — cross-compiled for Android. Framing, the
//! session cache, certificate pinning and reconnect live there, so there is
//! one implementation of the protocol and one place to fix it.
//!
//! This crate is the JNI surface plus [`bridge`]: stream registry, blocking
//! frame reads, and a shared tokio runtime.
//!
//! Frame wire shape between Rust and Kotlin (one allocation per frame):
//! `[u32BE type_len][type][u64BE seq][payload]`.
//!
//! Build: `apps/android/rust-wt-spike/build.sh`.

mod bridge;

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
            bridge::record_error(format!("Rust panic: {message}"));
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
                .with_tag("console_wt")
                .with_max_level(log::LevelFilter::Debug),
        );
        // Explicit hook: the default one writes to stderr, which Android
        // discards, so a panic would otherwise be a bare SIGABRT.
        let _ = std::panic::set_hook(Box::new(|info| {
            log::error!("PANIC in WebTransport client: {info}");
        }));
    });
}

use jni::objects::{JByteArray, JClass, JString};
use jni::sys::{jbyteArray, jint, jstring};
use jni::JNIEnv;

use crate::bridge::{KIND_CHAT_RUN, KIND_TERMINAL, KIND_WATCH};

/// Result codes, mirrored in `WtNative.kt`.
const OK: jint = 0;
const ERR_ARG: jint = -1;

/// `WtNative.abiVersion()` — build marker. Calling anything on `WtNative`
/// first runs the Kotlin `init` that loads this library, so a wrong ABI or a
/// missing `build.sh` staging step fails here rather than later.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_WtNative_abiVersion(
    _env: JNIEnv,
    _class: JClass,
) -> jint {
    1
}

fn arg(env: &mut JNIEnv, value: &JString) -> Option<String> {
    env.get_string(value).ok().map(|value| value.into())
}

fn byte_array(env: &mut JNIEnv, bytes: &[u8]) -> jbyteArray {
    match env.byte_array_from_slice(bytes) {
        Ok(array) => array.into_raw(),
        Err(_) => std::ptr::null_mut(),
    }
}

/// `WtNative.lastError()` — the most recent failure, or an empty string.
/// There is no logger inside a plain `.so`, so this is how a failure explains
/// itself.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_WtNative_lastError(
    env: JNIEnv,
    _class: JClass,
) -> jstring {
    match env.new_string(bridge::last_error()) {
        Ok(value) => value.into_raw(),
        Err(_) => std::ptr::null_mut(),
    }
}

/// `WtNative.openStream(apiUrl, origin, certSha256, kind, streamId, requestType, requestJson, authToken)`
///
/// Connects if needed and registers the stream. Returns a handle, or 0 on
/// failure.
#[no_mangle]
#[allow(clippy::too_many_arguments)]
pub extern "system" fn Java_com_console_mobile_WtNative_openStream(
    mut env: JNIEnv,
    _class: JClass,
    api_url: JString,
    origin: JString,
    cert_sha256: JString,
    kind: jint,
    stream_id: JString,
    request_type: JString,
    request_json: JString,
    auth_token: JString,
) -> jint {
    init_logging();
    guard(ERR_ARG, || {
    let api_url = arg(&mut env, &api_url);
    let origin = arg(&mut env, &origin);
    let cert = arg(&mut env, &cert_sha256);
    let stream_id = arg(&mut env, &stream_id);
    let request_type = arg(&mut env, &request_type);
    let request_json = arg(&mut env, &request_json);
    let auth_token = arg(&mut env, &auth_token);
    let (Some(api_url), Some(origin), Some(cert), Some(stream_id), Some(request_type), Some(request_json), Some(auth_token)) = (
        api_url,
        origin,
        cert,
        stream_id,
        request_type,
        request_json,
        auth_token,
    ) else {
        bridge::record_error("null argument".into());
        return ERR_ARG;
    };
    bridge::open_stream(
        &api_url,
        &origin,
        &cert,
        kind as u8,
        &stream_id,
        &request_type,
        &request_json,
        &auth_token,
    ) as jint
    })
}

/// `WtNative.readFrame(handle, timeoutMs)` — blocking read of one frame.
///
/// Returns `null` on timeout (the reader loop should just continue), an empty
/// array when the stream ended or broke, or the encoded frame. See the crate
/// doc for the byte layout.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_WtNative_readFrame(
    mut env: JNIEnv,
    _class: JClass,
    handle: jint,
    timeout_ms: jint,
) -> jbyteArray {
    init_logging();
    guard(std::ptr::null_mut(), || match bridge::read_frame(handle as u64, timeout_ms) {
        None => std::ptr::null_mut(),
        Some(Ok(bytes)) => byte_array(&mut env, &bytes),
        Some(Err(error)) => {
            bridge::record_error(error);
            byte_array(&mut env, &[])
        }
    })
}

/// `WtNative.sendFrame(handle, type, payload)` — write one client frame
/// (terminal input, resize, kill). Returns 0 on success.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_WtNative_sendFrame(
    mut env: JNIEnv,
    _class: JClass,
    handle: jint,
    frame_type: JString,
    payload: jni::sys::jbyteArray,
) -> jint {
    init_logging();
    guard(ERR_ARG, || {
    let Some(frame_type) = arg(&mut env, &frame_type) else {
        return ERR_ARG;
    };
    let mut bytes = Vec::new();
    if !payload.is_null() {
        // jni 0.21 wants a typed array, not a raw pointer.
        let array = unsafe { JByteArray::from_raw(payload) };
        match env.get_array_length(&array) {
            Ok(len) => {
                let mut buffer = vec![0i8; len as usize];
                if env.get_byte_array_region(&array, 0, &mut buffer).is_ok() {
                    bytes.extend(buffer.iter().map(|byte| *byte as u8));
                }
            }
            Err(error) => bridge::record_error(format!("read payload: {error}")),
        }
    }
    match bridge::send_frame(handle as u64, &frame_type, &bytes) {
        Ok(()) => OK,
        Err(error) => {
            bridge::record_error(error);
            ERR_ARG
        }
    }
    })
}

/// `WtNative.closeStream(handle, type)` — send a terminal close frame if asked,
/// then drop the stream. `type` is empty for streams that need no frame.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_WtNative_closeStream(
    mut env: JNIEnv,
    _class: JClass,
    handle: jint,
    frame_type: JString,
) -> jint {
    init_logging();
    guard(ERR_ARG, || {
    let frame_type = arg(&mut env, &frame_type).unwrap_or_default();
    match bridge::close_stream(handle as u64, &frame_type) {
        Ok(()) => OK,
        Err(error) => {
            bridge::record_error(error);
            ERR_ARG
        }
    }
    })
}

/// `WtNative.closeAll()` — tear every stream down (app shutdown).
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_WtNative_closeAll(
    _env: JNIEnv,
    _class: JClass,
) {
    bridge::close_all();
}

/// Stream kinds for `openStream`, so Kotlin does not hardcode proto numbers.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_WtNative_kindChatRun(
    _env: JNIEnv,
    _class: JClass,
) -> jint {
    KIND_CHAT_RUN as jint
}

#[no_mangle]
pub extern "system" fn Java_com_console_mobile_WtNative_kindTerminal(
    _env: JNIEnv,
    _class: JClass,
) -> jint {
    KIND_TERMINAL as jint
}

#[no_mangle]
pub extern "system" fn Java_com_console_mobile_WtNative_kindWatch(
    _env: JNIEnv,
    _class: JClass,
) -> jint {
    KIND_WATCH as jint
}