use crate::types::terminal::{
    TerminalBackend, TerminalClientMessage, TerminalGridSnapshot, TerminalId, TerminalRecord,
    TerminalServerMessage, TerminalSize, TerminalSpawnParams, TerminalStatus,
};
use crate::utils::HttpTransport;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::{RwLock, mpsc};

/// Client-side terminal service — manages WS lifecycle against `GET /api/terminals`
/// and owns the set of live `TerminalRecord`s. The VT grid itself lives in a
/// `TerminalBackend` (termy_core) per session; the service only routes wire
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

pub mod termy;
pub use termy::TermyBackend;

/// Handle for a live PTY session. Drop-in ready: holds the VT backend,
/// WS sender, and shared status. The UI owns this and calls `send_input` /
/// `resize` / `kill`; the background task feeds `backend` and notifies via
/// the provided `on_update` callback.
pub struct TerminalHandle {
    pub id: Arc<RwLock<Option<TerminalId>>>,
    pub status: Arc<RwLock<TerminalStatus>>,
    pub error: Arc<RwLock<Option<String>>>,
    pub backend: Arc<tokio::sync::Mutex<TermyBackend>>,
    pub notify: Arc<tokio::sync::Notify>,
    sender: mpsc::UnboundedSender<TerminalClientMessage>,
    _task: tokio::task::JoinHandle<()>,
}

impl TerminalHandle {
    pub fn send_input(&self, data: impl Into<String>) {
        let _ = self
            .sender
            .send(TerminalClientMessage::Input { data: data.into() });
    }

    pub fn resize(&self, size: TerminalSize) {
        let backend = self.backend.clone();
        let notify = self.notify.clone();
        tokio::spawn(async move {
            let mut b = backend.lock().await;
            b.resize(size);
            drop(b);
            notify.notify_one();
        });
        let _ = self.sender.send(TerminalClientMessage::Resize {
            cols: size.cols,
            rows: size.rows,
        });
    }

    pub fn kill(&self) {
        let _ = self.sender.send(TerminalClientMessage::Kill);
    }

    pub fn scroll(&self, delta: i32) {
        let backend = self.backend.clone();
        let notify = self.notify.clone();
        let sender = self.sender.clone();
        tokio::spawn(async move {
            let mut b = backend.lock().await;
            if b.is_alt_screen() {
                let key = if delta > 0 { "\x1b[A" } else { "\x1b[B" };
                let count = (delta.abs() as usize).min(5);
                let input = key.repeat(count);
                let _ = sender.send(TerminalClientMessage::Input { data: input });
            } else {
                b.scroll(delta);
                drop(b);
                notify.notify_one();
            }
        });
    }

    pub async fn snapshot(&self) -> TerminalGridSnapshot {
        let b = self.backend.lock().await;
        b.snapshot()
    }

    pub async fn status(&self) -> TerminalStatus {
        *self.status.read().await
    }
}

impl TerminalService {
    /// Spawn a new PTY on the server and return a `TerminalHandle` driving a
    /// `TermyBackend`. The handle is immediately usable: `send_input` /
    /// `resize` queue until the WS is ready. The handle's `notify` is triggered
    /// on every server frame that changes grid/status — the UI should await it
    /// and then call `snapshot().await` + `cx.notify()`.
    pub async fn spawn(
        &self,
        params: TerminalSpawnParams,
        initial_size: TerminalSize,
    ) -> anyhow::Result<TerminalHandle> {
        let url = self.ws_url(&params).await;
        let (tx, mut rx) = mpsc::unbounded_channel::<TerminalClientMessage>();

        let backend = Arc::new(tokio::sync::Mutex::new(TermyBackend::new(initial_size)));
        let id: Arc<RwLock<Option<TerminalId>>> = Arc::new(RwLock::new(None));
        let status: Arc<RwLock<TerminalStatus>> = Arc::new(RwLock::new(TerminalStatus::Spawning));
        let error: Arc<RwLock<Option<String>>> = Arc::new(RwLock::new(None));
        let notify: Arc<tokio::sync::Notify> = Arc::new(tokio::sync::Notify::new());

        let backend_clone = backend.clone();
        let id_clone = id.clone();
        let status_clone = status.clone();
        let error_clone = error.clone();
        let notify_clone = notify.clone();

        let task = tokio::spawn(async move {
            let ws_stream = match tokio_tungstenite::connect_async(&url).await {
                Ok((stream, _)) => stream,
                Err(e) => {
                    *status_clone.write().await = TerminalStatus::Error;
                    *error_clone.write().await = Some(format!("WS connect failed: {e}"));
                    notify_clone.notify_one();
                    return;
                }
            };

            let (mut write, mut read) = futures_util::StreamExt::split(ws_stream);

            let write_task = tokio::spawn(async move {
                while let Some(msg) = rx.recv().await {
                    let text = match serde_json::to_string(&msg) {
                        Ok(t) => t,
                        Err(e) => {
                            log::warn!("Failed to serialize terminal msg: {e}");
                            continue;
                        }
                    };
                    if let Err(e) = futures_util::SinkExt::send(
                        &mut write,
                        tokio_tungstenite::tungstenite::Message::Text(text.into()),
                    )
                    .await
                    {
                        log::warn!("WS send failed: {e}");
                        break;
                    }
                }
            });

            while let Some(msg) = futures_util::StreamExt::next(&mut read).await {
                match msg {
                    Ok(tokio_tungstenite::tungstenite::Message::Text(text)) => {
                        let server_msg: Result<TerminalServerMessage, _> =
                            serde_json::from_str(&text);
                        match server_msg {
                            Ok(TerminalServerMessage::Spawned { id: sid, .. }) => {
                                *id_clone.write().await = Some(sid);
                                *status_clone.write().await = TerminalStatus::Running;
                                notify_clone.notify_one();
                            }
                            Ok(TerminalServerMessage::Output { data }) => {
                                let mut b = backend_clone.lock().await;
                                b.advance(&data);
                                drop(b);
                                notify_clone.notify_one();
                            }
                            Ok(TerminalServerMessage::Exit { code }) => {
                                *status_clone.write().await = TerminalStatus::Exited;
                                if let Some(c) = code {
                                    *error_clone.write().await =
                                        Some(format!("Shell exited with code {c}"));
                                }
                                notify_clone.notify_one();
                                break;
                            }
                            Ok(TerminalServerMessage::Error { message }) => {
                                *status_clone.write().await = TerminalStatus::Error;
                                *error_clone.write().await = Some(message);
                                notify_clone.notify_one();
                                break;
                            }
                            Err(e) => {
                                log::warn!("Invalid terminal frame: {e} — {text}");
                            }
                        }
                    }
                    Ok(tokio_tungstenite::tungstenite::Message::Close(_)) => {
                        *status_clone.write().await = TerminalStatus::Exited;
                        notify_clone.notify_one();
                        break;
                    }
                    Err(e) => {
                        *status_clone.write().await = TerminalStatus::Error;
                        *error_clone.write().await = Some(format!("WS read failed: {e}"));
                        notify_clone.notify_one();
                        break;
                    }
                    _ => {}
                }
            }

            write_task.abort();
        });

        Ok(TerminalHandle {
            id,
            status,
            error,
            backend,
            notify,
            sender: tx,
            _task: task,
        })
    }
}
