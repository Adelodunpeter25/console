//! QUIC/WebTransport spike for plan Phase 3 (mobile).
//!
//! Question being answered: **can the desktop's WebTransport client cross-
//! compile to Android and open a session there?** Unlike the Cronet route this
//! has no OS-version floor, so if it works mobile gets QUIC on every `minSdk`
//! the app supports (26) rather than only Android 14+.
//!
//! This is deliberately the smallest surface that answers the question: one
//! blocking JNI call that opens a session and reports whether the handshake
//! completed. The framing and stream layer are already proven on desktop in
//! `apps/desktop/crates/console-core`; wiring those in is the next step, not
//! this one's job.
//!
//! Not production code.

use jni::objects::{JClass, JString};
use jni::sys::jint;
use jni::JNIEnv;

/// Result codes, mirrored in `WtNative.kt`.
const OK: jint = 0;
const ERR_BAD_ARG: jint = -1;
const ERR_RUNTIME: jint = -2;
const ERR_ENDPOINT: jint = -3;
const ERR_HANDSHAKE: jint = -4;
const ERR_CERT: jint = -5;

/// Last failure, so the Kotlin side can print it. The `log` crate has no
/// logger installed inside a plain `.so`, so `log::error!` goes nowhere —
/// a spike that cannot say *why* it failed is half a spike.
static LAST_ERROR: std::sync::Mutex<Option<String>> = std::sync::Mutex::new(None);

/// `WtNative.lastError()` — the most recent failure, or an empty string.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_WtNative_lastError(
    mut env: JNIEnv,
    _class: JClass,
) -> jni::sys::jstring {
    let message = LAST_ERROR
        .lock()
        .ok()
        .and_then(|slot| slot.clone())
        .unwrap_or_default();
    match env.new_string(message) {
        Ok(value) => value.into_raw(),
        Err(_) => std::ptr::null_mut(),
    }
}

fn record_error(message: String) {
    if let Ok(mut slot) = LAST_ERROR.lock() {
        *slot = Some(message.clone());
    }
    log::error!("{message}");
}

/// Build marker: proves the `.so` loaded and ran, before any networking.
/// Returns 1. Lives on `WtNative` rather than on the test class so that
/// calling it runs the Kotlin `init` that loads the library first.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_WtNative_abiVersion(
    _env: JNIEnv,
    _class: JClass,
) -> jint {
    1
}

/// `WtNative.probeSession(origin, token, certSha256)` — opens one
/// WebTransport session and reports whether the handshake completed.
///
/// @param certSha256 hex SHA-256 of the server's DER certificate; empty uses
///   platform roots. Pinning is what a self-signed dev certificate needs, and
///   the desktop client already depends on the same mechanism.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_WtNative_probeSession(
    mut env: JNIEnv,
    _class: JClass,
    origin: JString,
    token: JString,
    cert_sha256: JString,
) -> jint {
    let origin = match read_string(&mut env, &origin) {
        Some(value) => value,
        None => return ERR_BAD_ARG,
    };
    let token = match read_string(&mut env, &token) {
        Some(value) => value,
        None => return ERR_BAD_ARG,
    };
    let cert = read_string(&mut env, &cert_sha256).unwrap_or_default();

    let runtime = match tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
    {
        Ok(runtime) => runtime,
        Err(error) => {
            log::error!("tokio runtime: {error:#}");
            return ERR_RUNTIME;
        }
    };

    runtime.block_on(async move {
        let client_config = match build_client_config(&cert) {
            Ok(config) => config,
            Err(code) => return code,
        };
        let endpoint = match wtransport::Endpoint::client(client_config) {
            Ok(endpoint) => endpoint,
            Err(error) => {
                log::error!("endpoint: {error:#}");
                return ERR_ENDPOINT;
            }
        };
        let url = format!(
            "{}/wt/v1/connect?token={}",
            origin.trim_end_matches('/'),
            token
        );
        match endpoint
            .connect(wtransport::endpoint::ConnectOptions::builder(url).build())
            .await
        {
            Ok(connection) => {
                log::info!("WebTransport session established on Android");
                drop(connection);
                OK
            }
            Err(error) => {
                // The reason has to reach the app: `log::error!` has nowhere
                // to go without a logger, so it goes back over JNI instead.
                record_error(format!("WebTransport handshake failed: {error:#}"));
                ERR_HANDSHAKE
            }
        }
    })
}

fn build_client_config(cert: &str) -> Result<wtransport::ClientConfig, jint> {
    if cert.is_empty() {
        return Ok(wtransport::ClientConfig::builder()
            .with_bind_default()
            .with_native_certs()
            .build());
    }
    let Some(digest) = hex_to_32(cert) else {
        log::error!("cert hash must be 64 hex characters of SHA-256");
        return Err(ERR_CERT);
    };
    Ok(wtransport::ClientConfig::builder()
        .with_bind_default()
        .with_server_certificate_hashes(vec![wtransport::tls::Sha256Digest::new(digest)])
        .build())
}

fn read_string(env: &mut JNIEnv, value: &JString) -> Option<String> {
    env.get_string(value)
        .ok()
        .map(|java_string| java_string.into())
}

fn hex_to_32(hex: &str) -> Option<[u8; 32]> {
    let clean: String = hex.chars().filter(|c| *c != ':').collect();
    if clean.len() != 64 {
        return None;
    }
    let mut out = [0u8; 32];
    for (index, byte) in out.iter_mut().enumerate() {
        *byte = u8::from_str_radix(&clean[2 * index..2 * index + 2], 16).ok()?;
    }
    Some(out)
}