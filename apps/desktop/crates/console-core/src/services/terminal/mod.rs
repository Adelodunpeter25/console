use crate::types::terminal::{
    TerminalBackend, TerminalClientEvent, TerminalClientMessage, TerminalGridSnapshot, TerminalId,
    TerminalRecord, TerminalServerMessage, TerminalSize, TerminalSpawnParams, TerminalStatus,
};
use crate::utils::HttpTransport;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::{RwLock, mpsc};
use tokio_tungstenite::tungstenite::Error as WsError;

/// Binary wire-frame tags for `proto=binary` connections (mirrors the server's
/// `TERMINAL_OUTPUT_FRAME_TAG` / `TERMINAL_INPUT_FRAME_TAG`; see
/// docs/plan/terminal-binary-protocol-plan.md). A binary WS frame is
/// `[tag, ...payload]`; control messages remain JSON text frames.
pub const TERMINAL_OUTPUT_FRAME_TAG: u8 = 0x01;
pub const TERMINAL_INPUT_FRAME_TAG: u8 = 0x01;

/// How many times a dropped terminal WS is silently redialed before the pane
/// surfaces an error. Benign drops (server restart, relay/proxy blip,
/// sleep/VPN, idle LB close) are the common case — the gold `WS read failed`
/// banner for a bare `close_notify` EOF is noise, and the agent run is
/// unaffected (it streams over SSE, not this socket).
const MAX_RECONNECTS: u32 = 3;
const RECONNECT_DELAYS_MS: [u64; 3] = [500, 1000, 2000];

/// True for transport-level drops with no server verdict: bare TLS `close_notify`
/// EOF, unclean TCP close, or a clean WS close handshake with no terminal
/// `Exit`/`Error` frame. These say nothing about the shell itself, so the
/// caller should respawn quietly instead of blaming the user with raw TLS text.
fn is_benign_ws_disconnect(e: &WsError) -> bool {
    match e {
        WsError::ConnectionClosed | WsError::AlreadyClosed => true,
        WsError::Io(io) if io.kind() == std::io::ErrorKind::UnexpectedEof => true,
        _ => {
            let m = e.to_string().to_lowercase();
            m.contains("unexpected eof")
                || m.contains("close_notify")
                || m.contains("unclean closure")
        }
    }
}

/// How one WS attempt ended: the shell is really gone, or the socket dropped
/// with no server verdict and a redial is worth trying.
enum AttemptEnd {
    Done,
    Redial,
}

/// Next consecutive-attempt count. A connection that proved healthy (10s+)
/// resets the budget so one long-lived terminal doesn't accumulate stale
/// failures across unrelated blips.
fn bump_dials(dials: u32, connected_at: std::time::Instant) -> u32 {
    if connected_at.elapsed() > std::time::Duration::from_secs(10) {
        1
    } else {
        dials + 1
    }
}

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
        // Opt into the binary data frames (raw PTY bytes, tag 0x01); control
        // messages stay JSON. See docs/plan/terminal-binary-protocol-plan.md.
        url.push_str("&proto=binary");
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
    /// Wheel-tick coalescing: accumulates deltas for ~8ms so fast scrolling
    /// applies once + notifies once instead of one task per tick.
    scroll_pending: Arc<std::sync::Mutex<i32>>,
    scroll_scheduled: Arc<std::sync::atomic::AtomicBool>,
    _task: tokio::task::JoinHandle<()>,
}

/// Snapshot plus the metadata the renderer needs, collected under a single
/// backend lock so damage stays in sync with the grid.
pub struct TerminalSnapshotFull {
    pub snapshot: TerminalGridSnapshot,
    pub damage: termy_core::TerminalDamageSnapshot,
    pub mouse_mode: termy_core::TerminalMouseMode,
    pub is_alt_screen: bool,
    pub scroll_state: (usize, usize),
    /// Terminal title set via OSC 0/1/2, if any.
    pub title: Option<String>,
}

impl TerminalHandle {
    pub fn send_input(&self, data: impl Into<String>) {
        let _ = self.sender.send(crate::types::client_input(data));
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
        let _ = self
            .sender
            .send(crate::types::client_resize(size.cols, size.rows));
    }

    pub fn kill(&self) {
        let _ = self.sender.send(crate::types::client_kill());
    }

    pub fn scroll(&self, delta: i32) {
        if delta == 0 {
            return;
        }
        {
            let mut pending = self.scroll_pending.lock().unwrap();
            *pending = pending.saturating_add(delta);
        }
        // A flush is already scheduled — it will pick up this delta.
        if self
            .scroll_scheduled
            .swap(true, std::sync::atomic::Ordering::AcqRel)
        {
            return;
        }
        let backend = self.backend.clone();
        let notify = self.notify.clone();
        let sender = self.sender.clone();
        let pending = self.scroll_pending.clone();
        let scheduled = self.scroll_scheduled.clone();
        tokio::spawn(async move {
            // Coalesce wheel bursts into one backend scroll; the watch loop
            // below adds its own short delay before snapshotting.
            tokio::time::sleep(std::time::Duration::from_millis(4)).await;
            let delta = {
                let mut p = pending.lock().unwrap();
                let d = *p;
                *p = 0;
                d
            };
            scheduled.store(false, std::sync::atomic::Ordering::Release);
            if delta == 0 {
                return;
            }
            let mut b = backend.lock().await;
            if b.is_alt_screen() {
                let key = if delta > 0 { "\x1b[A" } else { "\x1b[B" };
                let count = (delta.abs() as usize).min(10);
                let input = key.repeat(count);
                drop(b);
                let _ = sender.send(crate::types::client_input(input));
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

    /// Snapshot + damage + mouse mode + alt flag under one lock. Prefer
    /// this over separate `snapshot()` / `damage_snapshot()` calls: damage
    /// clears on read, so separate locks can desync grid and dirty rows.
    pub async fn snapshot_full(&self) -> TerminalSnapshotFull {
        let b = self.backend.lock().await;
        let snapshot = b.snapshot();
        let damage = b.take_damage_snapshot();
        let mouse_mode = b.mouse_mode();
        let is_alt_screen = b.is_alt_screen();
        let scroll_state = b.scroll_state();
        let title = b.title().map(|s| s.to_string());
        TerminalSnapshotFull {
            snapshot,
            damage,
            mouse_mode,
            is_alt_screen,
            scroll_state,
            title,
        }
    }

    /// Damage since the last call: `Full` (repaint everything) or the dirty
    /// row spans. Must be collected alongside each snapshot — the backend
    /// clears dirty tracking when read, so snapshot and damage stay in sync.
    /// Prefer `snapshot_full()` for new code.
    pub async fn damage_snapshot(&self) -> termy_core::TerminalDamageSnapshot {
        let b = self.backend.lock().await;
        b.take_damage_snapshot()
    }

    /// Jump the scrollback viewport back to the live prompt. The UI also
    /// mirrors this in its local `scrollback_offset` (see `TerminalView`).
    pub fn scroll_to_bottom(&self) {
        let backend = self.backend.clone();
        let notify = self.notify.clone();
        tokio::spawn(async move {
            let b = backend.lock().await;
            b.scroll_to_bottom();
            drop(b);
            notify.notify_one();
        });
    }

    /// Full link metadata (OSC 8, file paths, URLs) at a viewport cell.
    pub async fn link_at(
        &self,
        row: usize,
        col: usize,
    ) -> Option<termy_core::DetectedViewportLink> {
        let b = self.backend.lock().await;
        b.link_at(row, col)
    }

    pub async fn status(&self) -> TerminalStatus {
        *self.status.read().await
    }
}

impl TerminalHandle {
    /// Assemble a handle around an already-plumbed inbound/outbound pair.
    ///
    /// The WS path builds its halves inline in [`TerminalService::spawn`];
    /// keeping the construction here means spawn logic never reaches into
    /// these fields.
    pub(crate) fn from_parts(
        id: Arc<RwLock<Option<TerminalId>>>,
        status: Arc<RwLock<TerminalStatus>>,
        error: Arc<RwLock<Option<String>>>,
        backend: Arc<tokio::sync::Mutex<TermyBackend>>,
        notify: Arc<tokio::sync::Notify>,
        sender: mpsc::UnboundedSender<TerminalClientMessage>,
        task: tokio::task::JoinHandle<()>,
    ) -> Self {
        Self {
            id,
            status,
            error,
            backend,
            notify,
            sender,
            scroll_pending: Arc::new(std::sync::Mutex::new(0)),
            scroll_scheduled: Arc::new(std::sync::atomic::AtomicBool::new(false)),
            _task: task,
        }
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
        let (tx, rx) = mpsc::unbounded_channel::<TerminalClientMessage>();

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
        // Replies to shell device queries (cursor-position reports, DA1,
        // kitty keyboard queries, ...) must be written back to the PTY;
        // over the wire they travel as regular Input messages.
        let reply_sender = tx.clone();

        let transport = self.transport.clone();
        let task = tokio::spawn(async move {
            let mut rx = rx;
            let mut dials: u32 = 0;
            let mut client_killed = false;

            loop {
                if dials > 0 {
                    let delay_ms = RECONNECT_DELAYS_MS
                        .get((dials - 1) as usize)
                        .copied()
                        .unwrap_or(2000);
                    log::warn!("Terminal WS reconnecting (attempt {dials})");
                    tokio::time::sleep(std::time::Duration::from_millis(delay_ms)).await;
                    *status_clone.write().await = TerminalStatus::Spawning;
                    *error_clone.write().await = None;
                    notify_clone.notify_one();
                }

                let request = match transport.ws_request(&url).await {
                    Ok(r) => r,
                    Err(e) => {
                        *status_clone.write().await = TerminalStatus::Error;
                        *error_clone.write().await = Some(format!("WS connect failed: {e}"));
                        notify_clone.notify_one();
                        return;
                    }
                };

                let ws_stream = match tokio_tungstenite::connect_async(request).await {
                    Ok((stream, _)) => stream,
                    Err(e) => {
                        if matches!(&e, WsError::Http(_) | WsError::Url(_)) {
                            *status_clone.write().await = TerminalStatus::Error;
                            *error_clone.write().await = Some(format!("WS connect failed: {e}"));
                            notify_clone.notify_one();
                            return;
                        }
                        dials += 1;
                        if dials > MAX_RECONNECTS || client_killed {
                            *status_clone.write().await = TerminalStatus::Error;
                            *error_clone.write().await = Some(format!("WS connect failed: {e}"));
                            notify_clone.notify_one();
                            return;
                        }
                        log::warn!("Terminal WS dial failed (attempt {dials}): {e}");
                        continue;
                    }
                };

                let connected_at = std::time::Instant::now();
                let (mut write, mut read) = futures_util::StreamExt::split(ws_stream);

                let end = loop {
                    tokio::select! {
                            incoming = rx.recv() => {
                                let Some(msg) = incoming else {
                                    break AttemptEnd::Done;
                                };
                                if matches!(&msg.event, Some(TerminalClientEvent::Kill(_))) {
                                    client_killed = true;
                                }
                                let wire = match &msg.event {
                                    Some(TerminalClientEvent::Input(input)) => {
                                        let mut frame = Vec::with_capacity(input.data.len() + 1);
                                        frame.push(TERMINAL_INPUT_FRAME_TAG);
                                        frame.extend_from_slice(input.data.as_bytes());
                                        tokio_tungstenite::tungstenite::Message::Binary(frame.into())
                                    }
                                    Some(_) => match serde_json::to_string(&msg) {
                                        Ok(t) => tokio_tungstenite::tungstenite::Message::Text(t.into()),
                                        Err(e) => {
                                            log::warn!("Failed to serialize terminal msg: {e}");
                                            continue;
                                        }
                                    },
                                    None => continue,
                                };
                                if let Err(e) = futures_util::SinkExt::send(&mut write, wire).await {
                                    log::warn!("WS send failed: {e}");
                                    break AttemptEnd::Done;
                                }
                            }
                            msg = futures_util::StreamExt::next(&mut read) => {
                                let Some(msg) = msg else {
                                    if client_killed {
                                        break AttemptEnd::Done;
                                    }
                                    log::warn!("Terminal WS stream ended without close frame");
                                    break AttemptEnd::Redial;
                                };
                                match msg {
                        Ok(tokio_tungstenite::tungstenite::Message::Text(text)) => {
                            let server_msg: Result<TerminalServerMessage, _> =
                                serde_json::from_str(&text);
                            match server_msg {
                                Ok(msg) => match msg.event {
                                    Some(crate::types::TerminalServerEvent::Spawned(s)) => {
                                        *id_clone.write().await = Some(s.id);
                                        *status_clone.write().await = TerminalStatus::Running;
                                        dials = 0;
                                        notify_clone.notify_one();
                                    }
                                    Some(crate::types::TerminalServerEvent::Output(o)) => {
                                        let mut b = backend_clone.lock().await;
                                        let replies = b.advance_and_collect_replies(&o.data);
                                        drop(b);
                                        if !replies.is_empty() {
                                            let _ = reply_sender.send(crate::types::client_input(
                                                String::from_utf8_lossy(&replies).into_owned(),
                                            ));
                                        }
                                        notify_clone.notify_one();
                                    }
                                    Some(crate::types::TerminalServerEvent::Exit(e)) => {
                                        *status_clone.write().await = TerminalStatus::Exited;
                                        if let Some(c) = e.code {
                                            *error_clone.write().await =
                                                Some(format!("Shell exited with code {c}"));
                                        }
                                        notify_clone.notify_one();
                                        break AttemptEnd::Done;
                                    }
                                    Some(crate::types::TerminalServerEvent::Error(e)) => {
                                        *status_clone.write().await = TerminalStatus::Error;
                                        *error_clone.write().await = Some(e.message);
                                        notify_clone.notify_one();
                                        break AttemptEnd::Done;
                                    }
                                    None => {
                                        log::warn!("Empty terminal frame: {text}");
                                    }
                                },
                                Err(e) => {
                                    log::warn!("Invalid terminal frame: {e} — {text}");
                                }
                            }
                        }
                        Ok(tokio_tungstenite::tungstenite::Message::Binary(bytes)) => {
                            if bytes.first() != Some(&TERMINAL_OUTPUT_FRAME_TAG) {
                                log::warn!("Unknown terminal binary frame tag: {:?}", bytes.first());
                                continue;
                            }
                            let payload = &bytes[1..];
                            if payload.is_empty() {
                                continue;
                            }
                            let mut b = backend_clone.lock().await;
                            let replies = b.advance_and_collect_replies_bytes(payload);
                            drop(b);
                            if !replies.is_empty() {
                                let _ = reply_sender.send(crate::types::client_input(
                                    String::from_utf8_lossy(&replies).into_owned(),
                                ));
                            }
                            notify_clone.notify_one();
                        }
                        Ok(tokio_tungstenite::tungstenite::Message::Close(_)) => {
                            if client_killed {
                                *status_clone.write().await = TerminalStatus::Exited;
                                notify_clone.notify_one();
                                break AttemptEnd::Done;
                            }
                            log::warn!("Terminal WS closed without exit frame");
                            break AttemptEnd::Redial;
                        }
                        Err(e) => {
                            if !client_killed && is_benign_ws_disconnect(&e) {
                                log::warn!("Terminal WS dropped: {e}");
                                break AttemptEnd::Redial;
                            }
                            *status_clone.write().await = TerminalStatus::Error;
                            *error_clone.write().await = Some(format!("WS read failed: {e}"));
                            notify_clone.notify_one();
                            break AttemptEnd::Done;
                        }
                        _ => {}
                    }
                            }
                        }
                };

                match end {
                    AttemptEnd::Done => return,
                    AttemptEnd::Redial => {
                        dials = bump_dials(dials, connected_at);
                        if dials > MAX_RECONNECTS {
                            *status_clone.write().await = TerminalStatus::Error;
                            *error_clone.write().await = Some(
                                "Terminal connection lost — close or respawn the terminal"
                                    .to_string(),
                            );
                            notify_clone.notify_one();
                            return;
                        }
                    }
                }
            }
        });

        Ok(TerminalHandle::from_parts(
            id, status, error, backend, notify, tx, task,
        ))
    }
}
