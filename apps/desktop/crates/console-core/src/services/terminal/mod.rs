use crate::types::terminal::{TerminalId, TerminalRecord, TerminalSpawnParams, TerminalStatus};
use crate::utils::HttpTransport;
use std::collections::HashMap;

/// Client-side terminal service — manages WS lifecycle against `GET /api/terminals`
/// and owns the set of live `TerminalRecord`s. The VT grid itself lives in a
/// `TerminalBackend` (alacritty) per session; the service only routes wire
/// frames → backend.
///
/// This module is intentionally **not wired into `ConsoleDesktopApp` yet**;
/// it is the isolated backend the UI crate will drive. See `docs/mobile-parity.md`
/// and `docs/terminal-mobile-plan.md` for the intended data flow:
///
///   server PTY ──WS frames──▶ TerminalService (append) ──snapshot──▶ console-ui
///       ◀── input/resize/kill ── UI ──WS──┘
#[derive(Clone)]
pub struct TerminalService {
    transport: HttpTransport,
}

impl TerminalService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    /// Build the WS URL for spawning/connecting. Mirrors mobile
    /// `packages/api/src/services/terminal.service.ts` `connectTerminal`.
    pub async fn ws_url(&self, params: &TerminalSpawnParams) -> String {
        let base = self.transport.base_url().await;
        let ws_base = if base.starts_with("https://") {
            base.replacen("https://", "wss://", 1)
        } else if base.starts_with("http://") {
            base.replacen("http://", "ws://", 1)
        } else {
            base.clone()
        };
        let mut url = format!(
            "{}/api/terminals?cwd={}&cols={}&rows={}",
            ws_base.trim_end_matches('/'),
            urlencoding::encode(&params.cwd),
            params.cols.unwrap_or(80),
            params.rows.unwrap_or(24),
        );
        if let Some(shell) = &params.shell {
            url.push_str(&format!("&shell={}", urlencoding::encode(shell)));
        }
        if let Some(label) = &params.label {
            url.push_str(&format!("&label={}", urlencoding::encode(label)));
        }
        url
    }

    /// In-memory registry helper — find a live (spawning/running) terminal for a project,
    /// preferring one whose `cwd` matches. Mirrors mobile `findLiveTerminal(projectId,cwd)`.
    pub fn find_live<'a>(
        records: &'a HashMap<TerminalId, TerminalRecord>,
        project_id: &str,
        cwd: Option<&str>,
    ) -> Option<&'a TerminalRecord> {
        let candidates: Vec<&TerminalRecord> = records
            .values()
            .filter(|r| {
                r.project_id.as_deref() == Some(project_id)
                    && matches!(r.status, TerminalStatus::Spawning | TerminalStatus::Running)
            })
            .collect();
        if candidates.is_empty() {
            return None;
        }
        if let Some(c) = cwd {
            if let Some(exact) = candidates.iter().find(|r| r.cwd.as_deref() == Some(c)) {
                return Some(exact);
            }
        }
        candidates.into_iter().next()
    }
}

pub mod alacritty;
pub use alacritty::AlacrittyBackend;
