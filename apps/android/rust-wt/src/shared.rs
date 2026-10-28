//! JNI surface over `console-shared`.
//!
//! The logic is **not** written here. It lives in `console-shared`, the crate
//! the desktop imports directly, so there is one implementation and two
//! callers. This module is only marshalling.
//!
//! Two return shapes, chosen per function:
//!
//! * **Plain `String` / `null`** for a single text result. Nothing to misparse,
//!   so no framing.
//! * **A framed byte array** for anything structured (diffs, pairing). The
//!   frame is `[u32BE magic "CSHR"][u16BE version][body]`, and the body uses
//!   `u32BE`-length-prefixed UTF-8 strings and big-endian integers. A reader
//!   that sees an unknown magic or version must treat the call as failed.
//!
//! Staleness is caught once, up front: Kotlin compares [`abiVersion`] with the
//! value it was built against before it trusts any other symbol, so an APK
//! carrying an older `.so` degrades instead of calling a function that does
//! not exist.
//!
//! Every entry point goes through [`crate::guard`], so a Rust panic becomes a
//! `null` the caller handles rather than an `abort()` that kills the app.

use console_shared::diff::{self, DiffLineKind, DiffResult};
use console_shared::{composer, format, icons, markdown, page_title, pairing, throughput};
use jni::objects::{JClass, JString};
use jni::sys::{jbyteArray, jdouble, jint, jstring};
use jni::JNIEnv;

/// `"CSHR"`.
const MAGIC: u32 = 0x4353_4852;
const VERSION: u16 = 1;

/// Bumped whenever a symbol is added, removed, or changes shape. Kotlin's
/// `SharedNative.EXPECTED_ABI` must match.
const ABI_VERSION: jint = 1;

// ── Encoding ──────────────────────────────────────────────────────────────

struct Frame(Vec<u8>);

impl Frame {
    fn new() -> Self {
        let mut buf = Vec::with_capacity(64);
        buf.extend_from_slice(&MAGIC.to_be_bytes());
        buf.extend_from_slice(&VERSION.to_be_bytes());
        Frame(buf)
    }
    fn u8(&mut self, v: u8) -> &mut Self {
        self.0.push(v);
        self
    }
    fn u32(&mut self, v: u32) -> &mut Self {
        self.0.extend_from_slice(&v.to_be_bytes());
        self
    }
    fn i32(&mut self, v: i32) -> &mut Self {
        self.0.extend_from_slice(&v.to_be_bytes());
        self
    }
    fn str(&mut self, v: &str) -> &mut Self {
        self.u32(v.len() as u32);
        self.0.extend_from_slice(v.as_bytes());
        self
    }
    fn diff(&mut self, result: &DiffResult) -> &mut Self {
        self.u32(result.added as u32)
            .u32(result.removed as u32)
            .u32(result.lines.len() as u32);
        for line in &result.lines {
            self.u8(match line.kind {
                DiffLineKind::Context => 0,
                DiffLineKind::Added => 1,
                DiffLineKind::Removed => 2,
            });
            self.i32(line.old_no.map_or(-1, |n| n as i32));
            self.i32(line.new_no.map_or(-1, |n| n as i32));
            self.str(&line.text);
        }
        self
    }
}

// ── JNI helpers ───────────────────────────────────────────────────────────

fn read(env: &mut JNIEnv, value: &JString) -> Option<String> {
    crate::arg(env, value)
}

fn text(env: &mut JNIEnv, value: Option<String>) -> jstring {
    match value {
        Some(value) => env
            .new_string(value)
            .map(|s| s.into_raw())
            .unwrap_or(std::ptr::null_mut()),
        None => std::ptr::null_mut(),
    }
}

fn frame(env: &mut JNIEnv, frame: Frame) -> jbyteArray {
    crate::byte_array(env, &frame.0)
}

// ── Entry points ──────────────────────────────────────────────────────────

/// `SharedNative.abiVersion()`.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_abiVersion(
    _env: JNIEnv,
    _class: JClass,
) -> jint {
    ABI_VERSION
}

/// `SharedNative.extractHtmlTitle(html)` — the page title, or `null`.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_extractHtmlTitle(
    mut env: JNIEnv,
    _class: JClass,
    html: JString,
) -> jstring {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        let title = read(&mut env, &html).and_then(|html| page_title::extract_html_title(&html));
        text(&mut env, title)
    })
}

/// `SharedNative.closeHanging(text)` — the repaired tail, or `null` when
/// nothing hangs.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_closeHanging(
    mut env: JNIEnv,
    _class: JClass,
    value: JString,
) -> jstring {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        let repaired = read(&mut env, &value).and_then(|v| markdown::mend::close_hanging(&v));
        text(&mut env, repaired)
    })
}

/// `SharedNative.fileIconKey(pathOrName)`.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_fileIconKey(
    mut env: JNIEnv,
    _class: JClass,
    path: JString,
) -> jstring {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        let key = read(&mut env, &path).map(|p| icons::file_icon_key(&p).to_owned());
        text(&mut env, key)
    })
}

/// `SharedNative.languageIconKey(tag)` — or `null` for an unknown tag.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_languageIconKey(
    mut env: JNIEnv,
    _class: JClass,
    tag: JString,
) -> jstring {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        let key = read(&mut env, &tag)
            .and_then(|t| icons::language_icon_key(&t))
            .map(str::to_owned);
        text(&mut env, key)
    })
}

/// `SharedNative.formatContextWindow(tokens)`.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_formatContextWindow(
    mut env: JNIEnv,
    _class: JClass,
    tokens: jint,
) -> jstring {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        text(&mut env, Some(format::format_context_window(tokens.max(0) as u32)))
    })
}

/// `SharedNative.formatModelName(id)`.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_formatModelName(
    mut env: JNIEnv,
    _class: JClass,
    id: JString,
) -> jstring {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        let name = read(&mut env, &id).map(|id| format::format_model_name(&id));
        text(&mut env, name)
    })
}

/// `SharedNative.formatTokensPerSecond(rate)` — `null` when unmeasurable.
/// `NaN` stands for "no value".
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_formatTokensPerSecond(
    mut env: JNIEnv,
    _class: JClass,
    rate: jdouble,
) -> jstring {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        let rate = if rate.is_nan() { None } else { Some(rate) };
        text(&mut env, throughput::format_tokens_per_second(rate))
    })
}

/// `SharedNative.parsePairingUrl(raw)` — framed:
/// `[u8 ok]` then `[str base][str serverId][str code]` or `[str error]`.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_parsePairingUrl(
    mut env: JNIEnv,
    _class: JClass,
    raw: JString,
) -> jbyteArray {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        let Some(raw) = read(&mut env, &raw) else {
            return std::ptr::null_mut();
        };
        let mut out = Frame::new();
        match pairing::parse_pairing_url(&raw) {
            Ok(parsed) => {
                out.u8(1)
                    .str(&parsed.base_url)
                    .str(&parsed.server_id)
                    .str(&parsed.code);
            }
            Err(error) => {
                out.u8(0).str(&error);
            }
        }
        frame(&mut env, out)
    })
}

/// `SharedNative.splitPairingLine(text)` — framed `[str address][str code]`.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_splitPairingLine(
    mut env: JNIEnv,
    _class: JClass,
    raw: JString,
) -> jbyteArray {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        let Some(raw) = read(&mut env, &raw) else {
            return std::ptr::null_mut();
        };
        let (address, code) = pairing::split_pairing_line(&raw);
        let mut out = Frame::new();
        out.str(&address).str(&code);
        frame(&mut env, out)
    })
}

/// `SharedNative.hostPort(address)`.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_hostPort(
    mut env: JNIEnv,
    _class: JClass,
    address: JString,
) -> jstring {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        let host = read(&mut env, &address).map(|a| pairing::host_port(&a));
        text(&mut env, host)
    })
}

/// `SharedNative.diffLines(old, new, context)` — framed diff.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_diffLines(
    mut env: JNIEnv,
    _class: JClass,
    old: JString,
    new: JString,
    context: jint,
) -> jbyteArray {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        let (Some(old), Some(new)) = (read(&mut env, &old), read(&mut env, &new)) else {
            return std::ptr::null_mut();
        };
        let result = diff::diff_lines(&old, &new, context.max(0) as usize);
        let mut out = Frame::new();
        out.diff(&result);
        frame(&mut env, out)
    })
}

/// `SharedNative.parseUnifiedDiff(raw)` — framed diff.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_parseUnifiedDiff(
    mut env: JNIEnv,
    _class: JClass,
    raw: JString,
) -> jbyteArray {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        let Some(raw) = read(&mut env, &raw) else {
            return std::ptr::null_mut();
        };
        let mut out = Frame::new();
        out.diff(&diff::parse_unified_diff(&raw));
        frame(&mut env, out)
    })
}

/// `SharedNative.fileCallDiffs(toolName, argumentsJson)` — framed:
/// `[u32 files]` then per file `[str path]` + a diff body.
#[no_mangle]
pub extern "system" fn Java_com_console_mobile_SharedNative_fileCallDiffs(
    mut env: JNIEnv,
    _class: JClass,
    name: JString,
    arguments_json: JString,
) -> jbyteArray {
    crate::init_logging();
    crate::guard(std::ptr::null_mut(), || {
        let (Some(name), Some(json)) = (read(&mut env, &name), read(&mut env, &arguments_json))
        else {
            return std::ptr::null_mut();
        };
        let arguments: serde_json::Value = serde_json::from_str(&json).unwrap_or_default();
        let files = diff::file_call_diffs(&name, &arguments);
        let mut out = Frame::new();
        out.u32(files.len() as u32);
        for file in &files {
            out.str(&file.path).diff(&file.diff);
        }
        frame(&mut env, out)
    })
}

/// `SharedNative.mentionRanges(text)` is deliberately absent: the two clients
/// model mentions differently (desktop reconciles chip ranges, Android scans
/// raw text), so there is nothing to share yet. See the plan, §3.0.
#[allow(dead_code)]
fn _mentions_are_not_shared() {
    let _ = composer::mentions::mentions_from_context_files;
}
