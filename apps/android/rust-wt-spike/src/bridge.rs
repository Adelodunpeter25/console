//! Android-side bridge over `console-core`'s WebTransport client (plan Phase 3).
//!
//! The transport, framing, session cache and reconnect logic all live in
//! `console-core` — the same code the desktop dogfoods, cross-compiled for
//! Android. This module is only the JNI surface plus the bookkeeping that
//! Kotlin cannot own:
//!
//! * one long-lived [`WtService`] per device, opened lazily;
//! * a registry of open streams, so Kotlin can hold an integer handle;
//! * a blocking `read_frame` that hands Kotlin the raw frame in one
//!   allocation, because mobile's existing parsers expect the SSE frame shape
//!   (`seq`, event name, body) and already decode it.
//!
//! Deliberately **polling** rather than JNI callbacks: a callback would need
//! `AttachCurrentThread` bookkeeping on every tokio worker, and Kotlin already
//! drives streams from background coroutines. `read_frame` blocks briefly and
//! returns null on timeout, so a reader loop is `while (true) { read ?: continue }`.

use std::collections::HashMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex, OnceLock};

use console_core::ConsoleClient;
use console_core::net::{WtStreamKind, WtStreamReader, WtStreamWriter, WtTls};
use console_core::services::Transport;
use console_core::services::wt::WtService;

/// Stream kinds, matching `console.v1.WtStreamKind`. Exposed to Kotlin so it
/// never hardcodes proto numbers.
pub const KIND_CHAT_RUN: u8 = 1;
pub const KIND_TERMINAL: u8 = 2;
pub const KIND_WATCH: u8 = 3;

/// A live stream plus the session that opened it. The session is cloned into
/// every stream so the QUIC connection outlives any single stream.
struct OpenStream {
    /// Held so the QUIC connection outlives this stream: `WtService` owns the
    /// endpoint, and dropping it would close every other stream too.
    #[allow(dead_code)]
    session: WtService,
    #[allow(dead_code)]
    writer: WtStreamWriter,
    reader: Mutex<WtStreamReader>,
}

struct Registry {
    streams: Mutex<HashMap<u64, Arc<OpenStream>>>,
    transport: Mutex<Option<CachedTransport>>,
    next_id: AtomicU64,
}

/// The transport plus what it was built for. The app can point at a different
/// server at runtime (the environment picker), so the cache has to notice —
/// reusing a session dialed for another origin would silently stream from the
/// wrong server.
struct CachedTransport {
    origin: String,
    cert_sha256: String,
    transport: Transport,
}

static REGISTRY: OnceLock<Registry> = OnceLock::new();

fn registry() -> &'static Registry {
    REGISTRY.get_or_init(|| Registry {
        streams: Mutex::new(HashMap::new()),
        transport: Mutex::new(None),
        next_id: AtomicU64::new(1),
    })
}

/// Session shared by every stream on this device. One QUIC connection for the
/// app's lifetime, redialed by `Transport` when it dies or the server changes.
fn transport(origin: &str, cert_sha256: &str) -> Result<Transport, String> {
    let mut slot = registry().transport.lock().map_err(|_| "transport lock poisoned")?;
    if let Some(cached) = slot.as_ref() {
        if cached.origin == origin && cached.cert_sha256 == cert_sha256 {
            return Ok(cached.transport.clone());
        }
    }
    let tls = tls_from_setting(cert_sha256)?;
    // Auto, exactly like the desktop: prefer WebTransport, downgrade to SSE/WS
    // if this server cannot serve it. Android has no Cronet-style version gate
    // to worry about — the Rust client is the only path either way.
    let transport = Transport::with_settings(
        console_core::services::TransportMode::Auto,
        Some(origin.to_string()),
        tls,
    );
    // Switching servers drops the old session with its streams; the caller
    // reopens them against the new origin.
    if let Ok(mut streams) = registry().streams.lock() {
        streams.clear();
    }
    *slot = Some(CachedTransport {
        origin: origin.to_string(),
        cert_sha256: cert_sha256.to_string(),
        transport: transport.clone(),
    });
    Ok(transport)
}

fn tls_from_setting(value: &str) -> Result<WtTls, String> {
    if value.is_empty() {
        return Ok(WtTls::Native);
    }
    let clean: String = value.chars().filter(|c| *c != ':').collect();
    if clean.len() != 64 {
        return Err(format!("certificate hash must be 64 hex characters: {value}"));
    }
    let mut digest = [0u8; 32];
    for (index, byte) in digest.iter_mut().enumerate() {
        *byte = u8::from_str_radix(&clean[2 * index..2 * index + 2], 16)
            .map_err(|_| format!("certificate hash is not hexadecimal: {value}"))?;
    }
    Ok(WtTls::PinnedHashes(vec![digest]))
}

/// Open a stream: connect if needed, send `request` (type `request_type`,
/// body `request_json`), and register the read half under a new handle.
///
/// Returns the handle, or 0 on failure (the reason lands in `last_error`).
pub fn open_stream(
    api_url: &str,
    origin: &str,
    cert_sha256: &str,
    kind: u8,
    stream_id: &str,
    request_type: &str,
    request_json: &str,
) -> u64 {
    let result = (|| -> Result<u64, String> {
        log::debug!("open_stream: begin kind={kind} origin={origin}");
        let transport = transport(origin, cert_sha256)?;
        let client = ConsoleClient::new(Some(api_url.to_string()));
        let kind = match kind {
            KIND_CHAT_RUN => WtStreamKind::ChatRun,
            KIND_TERMINAL => WtStreamKind::Terminal,
            KIND_WATCH => WtStreamKind::Watch,
            other => return Err(format!("unknown stream kind {other}")),
        };
        let session = block_on(transport.connect(&client))
            .map_err(|error| format!("connect: {error:#}"))?
            .ok_or_else(|| {
                "server is not serving WebTransport (legacy fallback in use)".to_string()
            })?;
        let (writer, reader) = block_on(session.open_stream(
            kind,
            stream_id,
            request_type,
            request_json.as_bytes().to_vec(),
        ))
        .map_err(|error| format!("open stream: {error:#}"))?;

        let handle = registry().next_id.fetch_add(1, Ordering::Relaxed);
        let mut streams = registry().streams.lock().map_err(|_| "stream registry poisoned")?;
        streams.insert(
            handle,
            Arc::new(OpenStream {
                session,
                writer,
                reader: Mutex::new(reader),
            }),
        );
        Ok(handle)
    })();

    match result {
        Ok(handle) => handle,
        Err(error) => {
            record_error(error);
            0
        }
    }
}

/// Blocking read of the next frame.
///
/// Returns `None` on timeout, `Some(Err)` when the stream ended or broke, and
/// `Some(Ok(bytes))` for a frame. The frame bytes are
/// `[u32BE type_len][type][u64BE seq][payload]`, so Kotlin allocates once per
/// frame and slices the parts it needs.
pub fn read_frame(handle: u64, timeout_ms: i32) -> Option<Result<Vec<u8>, String>> {
    let stream = {
        let streams = match registry().streams.lock() {
            Ok(streams) => streams,
            Err(_) => return Some(Err("stream registry poisoned".into())),
        };
        streams.get(&handle).cloned()
    };
    let Some(stream) = stream else {
        return Some(Err(format!("unknown stream handle {handle}")));
    };

    let mut reader = match stream.reader.lock() {
        Ok(reader) => reader,
        Err(_) => return Some(Err("stream reader poisoned".into())),
    };
    // The timeout future must be *constructed* inside the runtime: arguments
    // to `block_on` are evaluated first, and `tokio::time::timeout` panics
    // with "there is no reactor running" when built off-runtime.
    let next = block_on(async {
        tokio::time::timeout(
            std::time::Duration::from_millis(timeout_ms.max(1) as u64),
            reader.next_frame(),
        )
        .await
    });
    match next {
        Ok(Ok(None)) => Some(Err("stream ended".into())),
        Ok(Ok(Some(frame))) => Some(Ok(encode_frame(frame.r#type, frame.seq, frame.payload))),
        Ok(Err(error)) => Some(Err(format!("{error:#}"))),
        Err(_) => None,
    }
}

/// Write one client frame (terminal input, resize, kill).
pub fn send_frame(handle: u64, frame_type: &str, payload: &[u8]) -> Result<(), String> {
    let stream = {
        let streams = registry().streams.lock().map_err(|_| "stream registry poisoned")?;
        streams.get(&handle).cloned()
    };
    let Some(stream) = stream else {
        return Err(format!("unknown stream handle {handle}"));
    };
    block_on(async move {
        stream
            .writer
            .send_frame(&console_proto::WtFrame {
                seq: 0,
                r#type: frame_type.to_string(),
                payload: payload.to_vec(),
            })
            .await
    })
    .map_err(|error| format!("{error:#}"))
}

/// Close one stream: kill the server-side resource and drop our half.
pub fn close_stream(handle: u64, frame_type: &str) -> Result<(), String> {
    // `kill` is best-effort: a stream that is already gone is not an error
    // worth surfacing, and the registry entry must go either way.
    let _ = send_frame(handle, frame_type, &[]);
    let mut streams = registry().streams.lock().map_err(|_| "stream registry poisoned")?;
    streams.remove(&handle);
    Ok(())
}

fn encode_frame(frame_type: String, seq: u64, payload: Vec<u8>) -> Vec<u8> {
    let type_bytes = frame_type.as_bytes();
    let mut out = Vec::with_capacity(4 + type_bytes.len() + 8 + payload.len());
    out.extend_from_slice(&(type_bytes.len() as u32).to_be_bytes());
    out.extend_from_slice(type_bytes);
    out.extend_from_slice(&seq.to_be_bytes());
    out.extend_from_slice(&payload);
    out
}

/// Close every open stream. Called when the app tears the transport down.
pub fn close_all() {
    if let Ok(mut streams) = registry().streams.lock() {
        streams.clear();
    }
    if let Ok(mut slot) = registry().transport.lock() {
        if let Some(cached) = slot.take() {
            block_on(cached.transport.close("app shutting down"));
        }
    }
}

/// Sessions keep their own runtime in the spike; in the bridge the streams
/// share one, created on first use.
fn shared_runtime() -> &'static tokio::runtime::Runtime {
    static RUNTIME: OnceLock<tokio::runtime::Runtime> = OnceLock::new();
    RUNTIME.get_or_init(|| {
        tokio::runtime::Builder::new_multi_thread()
            .enable_all()
            .build()
            .expect("tokio runtime for WebTransport streams")
    })
}

fn block_on<F: std::future::Future>(future: F) -> F::Output {
    // Never block on the runtime that is already driving the future: the JNI
    // calls here come from Kotlin background threads, not from Rust workers.
    shared_runtime().block_on(future)
}

// ── Error reporting back to Kotlin ───────────────────────────────────────

static LAST_ERROR: OnceLock<Mutex<Option<String>>> = OnceLock::new();

fn last_error_slot() -> &'static Mutex<Option<String>> {
    LAST_ERROR.get_or_init(|| Mutex::new(None))
}

pub fn record_error(message: String) {
    if let Ok(mut slot) = last_error_slot().lock() {
        *slot = Some(message.clone());
    }
}

pub fn last_error() -> String {
    last_error_slot().lock().ok().and_then(|slot| slot.clone()).unwrap_or_default()
}