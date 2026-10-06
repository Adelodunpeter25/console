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
