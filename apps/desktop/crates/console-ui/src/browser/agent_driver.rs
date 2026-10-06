//! Backend-neutral entry point for driving a browser tab with agent-browser.
//!
//! Only the CEF backend can be driven (it exposes CDP). Without the
//! `cef-browser` feature everything here reports "unavailable", so callers
//! need no feature gates and simply fall back to the in-page script path.
//! See `docs/plan/agent-browser-cef.md`.

use std::time::Duration;

/// One executed step of a driven batch.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DriveStep {
    pub command: Vec<String>,
    /// The step failed (e.g. "Could not locate element ...").
    pub error: Option<String>,
    /// Human-readable result (snapshot tree, eval value, page text, ...).
    pub output: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DriveError {
    /// agent-browser cannot be used (no CDP port, not installable). Callers
    /// fall back to the in-page script path.
    Unavailable(String),
    /// The CDP target is gone (tab closed or replaced).
    TargetGone(String),
    Failed(String),
}

impl std::fmt::Display for DriveError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Unavailable(message) | Self::TargetGone(message) | Self::Failed(message) => {
                f.write_str(message)
            }
        }
    }
}

impl std::error::Error for DriveError {}

#[cfg(all(target_os = "macos", feature = "cef-browser"))]
mod imp {
    use super::{DriveError, DriveStep};
    use crate::browser::cef::agent_browser::{self, RunError};
    use crate::browser::cef::runtime::debug_port;
    use std::path::PathBuf;
    use std::sync::Mutex;
    use std::sync::atomic::{AtomicBool, Ordering};
    use std::time::Duration;

    /// Set when agent-browser could not be found or installed, so later
    /// actions skip straight to the fallback instead of retrying the install.
    static DISABLED: AtomicBool = AtomicBool::new(false);
    /// Resolved binary path (the login-shell lookup is slow, do it once).
    static BINARY: Mutex<Option<PathBuf>> = Mutex::new(None);

    pub fn is_enabled() -> bool {
        !DISABLED.load(Ordering::Relaxed)
    }

    pub fn is_ready() -> bool {
        is_enabled() && debug_port().is_some()
    }

    fn binary() -> Result<PathBuf, DriveError> {
        let mut cached = BINARY.lock().unwrap_or_else(|poisoned| poisoned.into_inner());
        if let Some(path) = cached.as_ref().filter(|path| path.is_file()) {
            return Ok(path.clone());
        }
        match agent_browser::ensure_binary() {
            Ok(path) => {
                *cached = Some(path.clone());
                Ok(path)
            }
            Err(message) => {
                log::warn!("agent-browser unavailable, using in-page scripts: {message}");
                DISABLED.store(true, Ordering::Relaxed);
                Err(DriveError::Unavailable(message))
            }
        }
    }

    pub fn drive(
        session: &str,
        target_id: &str,
        commands: Vec<Vec<String>>,
        timeout: Duration,
    ) -> Result<Vec<DriveStep>, DriveError> {
        if !is_enabled() {
            return Err(DriveError::Unavailable("agent-browser is disabled".to_string()));
        }
        let port = debug_port()
            .ok_or_else(|| DriveError::Unavailable("CEF debug port is not ready".to_string()))?;
        let binary = binary()?;
        let invocation = agent_browser::build_invocation(port, session, target_id, &commands)
            .map_err(|err| DriveError::Failed(err.to_string()))?;
        match agent_browser::run(&binary, &invocation, timeout) {
            Ok(steps) => Ok(steps
                .into_iter()
                .map(|step| {
                    let output = step.output();
                    DriveStep {
                        command: step.command,
                        error: step.error,
                        output,
                    }
                })
                .collect()),
            Err(RunError::TargetGone(message)) => Err(DriveError::TargetGone(message)),
            Err(RunError::Failed(message)) => Err(DriveError::Failed(message)),
        }
    }
}

#[cfg(not(all(target_os = "macos", feature = "cef-browser")))]
mod imp {
    use super::{DriveError, DriveStep};
    use std::time::Duration;

    pub fn is_enabled() -> bool {
        false
    }

    pub fn is_ready() -> bool {
        false
    }

    pub fn drive(
        _session: &str,
        _target_id: &str,
        _commands: Vec<Vec<String>>,
        _timeout: Duration,
    ) -> Result<Vec<DriveStep>, DriveError> {
        Err(DriveError::Unavailable(
            "the CEF browser backend is not enabled in this build".to_string(),
        ))
    }
}

/// Whether agent-browser driving may be attempted at all (CEF build and not
/// disabled by an earlier install failure).
pub fn is_enabled() -> bool {
    imp::is_enabled()
}

/// Whether a drive can start right now: enabled and CEF's debug port is up.
/// Callers use this to choose between agent-browser and the in-page fallback.
pub fn is_ready() -> bool {
    imp::is_ready()
}

/// Run `commands` (each `[verb, args...]`) against one CDP target as a single
/// atomic batch. Blocking: call from a background thread.
pub fn drive(
    session: &str,
    target_id: &str,
    commands: Vec<Vec<String>>,
    timeout: Duration,
) -> Result<Vec<DriveStep>, DriveError> {
    imp::drive(session, target_id, commands, timeout)
}

// ---------------------------------------------------------------------------
// Command builders (pure): Console browser action -> agent-browser commands
// ---------------------------------------------------------------------------

/// A command list is a batch of `[verb, args...]` steps.
pub type Commands = Vec<Vec<String>>;

fn command(parts: &[&str]) -> Vec<String> {
    parts.iter().map(|part| part.to_string()).collect()
}

/// Normalize an element ref from a snapshot (`e12`, `@e12`, `ref=e12`) to the
/// `@e12` form agent-browser expects.
pub fn normalize_ref(raw: &str) -> String {
    let trimmed = raw.trim();
    let bare = trimmed
        .strip_prefix("ref=")
        .unwrap_or(trimmed)
        .trim_start_matches('@');
    format!("@{bare}")
}

/// Interactive accessibility snapshot with `@eN` refs.
pub fn snapshot_commands() -> Commands {
    vec![command(&["snapshot", "-i"])]
}

pub fn click_commands(element_ref: &str) -> Commands {
    vec![vec!["click".to_string(), normalize_ref(element_ref)]]
}

/// Fill `text` into the element, then press Enter when `submit` is set.
pub fn type_commands(element_ref: &str, text: &str, submit: bool) -> Commands {
    let mut commands = vec![vec![
        "fill".to_string(),
        normalize_ref(element_ref),
        text.to_string(),
    ]];
    if submit {
        commands.push(command(&["press", "Enter"]));
    }
    commands
}

pub fn run_js_commands(script: &str) -> Commands {
    vec![vec!["eval".to_string(), script.to_string()]]
}

/// Page text, or the text of `selector` when given.
pub fn content_commands(selector: Option<&str>) -> Commands {
    let target = selector.filter(|s| !s.trim().is_empty()).unwrap_or("body");
    vec![vec!["get".to_string(), "text".to_string(), target.to_string()]]
}

/// One `wait` step per condition; all must hold. `url_contains` becomes a
/// `**substring**` glob.
pub fn wait_commands(
    selector: Option<&str>,
    url_contains: Option<&str>,
    text: Option<&str>,
    timeout_ms: u64,
) -> Commands {
    let timeout = timeout_ms.to_string();
    let mut commands = Vec::new();
    if let Some(selector) = selector {
        commands.push(vec![
            "wait".to_string(),
            selector.to_string(),
            "--timeout".to_string(),
            timeout.clone(),
        ]);
    }
    if let Some(text) = text {
        commands.push(vec![
            "wait".to_string(),
            "--text".to_string(),
            text.to_string(),
            "--timeout".to_string(),
            timeout.clone(),
        ]);
    }
    if let Some(fragment) = url_contains {
        commands.push(vec![
            "wait".to_string(),
            "--url".to_string(),
            format!("**{fragment}**"),
            "--timeout".to_string(),
            timeout,
        ]);
    }
    commands
}

pub fn screenshot_commands(path: &str) -> Commands {
    vec![vec!["screenshot".to_string(), path.to_string()]]
}

/// Session name for agent-browser's daemon. One stable name per Console data
/// dir, so repeated runs reuse a single daemon instead of leaking one per run
/// while a test build with its own `CONSOLE_CEF_DATA_DIR` stays separate.
pub fn session_name(data_dir: Option<&str>) -> String {
    match data_dir.filter(|dir| !dir.is_empty()) {
        None => "console".to_string(),
        Some(dir) => {
            // FNV-1a: stable across runs (std's hasher is randomly seeded).
            let hash = dir.bytes().fold(0xcbf2_9ce4_8422_2325_u64, |acc, byte| {
                (acc ^ u64::from(byte)).wrapping_mul(0x0100_0000_01b3)
            });
            format!("console-{:08x}", hash & 0xffff_ffff)
        }
    }
}
