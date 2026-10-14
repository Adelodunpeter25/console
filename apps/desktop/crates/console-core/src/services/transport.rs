//! Transport selection for streaming surfaces (QUIC migration, Phase 4).
//!
//! One switch per app run, from `CONSOLE_TRANSPORT`:
//!
//! * `auto` (**default**) — try WebTransport first, fall back to SSE/WS when
//!   the server cannot serve it. This is what makes QUIC the default on a
//!   TLS-capable server without breaking anything that isn't: no cert,
//!   blocked UDP, or a plaintext install all just land on legacy.
//! * `wt` — WebTransport only. A failed handshake surfaces as an error
//!   instead of silently downgrading, which is what dogfooding and CI want.
//! * `legacy` — SSE/WS only, as before the migration.
//!
//! Every method here returns the *same* item type on all three paths — WT
//! payloads are the legacy bodies byte for byte — so callers hold one type
//! and only the seam moves. Android is still legacy-only, so this switch is
//! desktop-local and changes no server default.
//!
//! Env knobs (all optional):
//!
//! | variable | meaning |
//! |---|---|
//! | `CONSOLE_TRANSPORT` | `auto` (default), `wt` or `legacy` |
//! | `CONSOLE_WT_ORIGIN` | WT origin when the QUIC listener is not the API origin (`make dev-server-wt`) |
//! | `CONSOLE_WT_TLS` | `native` (default) or `pin:<sha256 hex>[,<sha256 hex>…]` |
//!
//! Auto mode deliberately does **not** race the two transports (the plan
//! defers racing to Phase 5, pending real failure data). It attempts one
//! bounded handshake and remembers the verdict for [`WT_RETRY_BACKOFF`], so a
//! legacy-only server costs one dial rather than a stall per stream.

use crate::ConsoleClient;
use crate::net::WtTls;
use crate::services::terminal::{TerminalHandle, TerminalService};
use crate::services::wt::{WtService, upgrade_to_https};
use crate::types::events::AgentSessionEvent;
use crate::types::terminal::{TerminalSize, TerminalSpawnParams};
use crate::types::{ForwardedPort, FsChangeEvent, GitStatusSummary, RunPromptDto};
use anyhow::{Context, Result};
use futures_util::Stream;
use std::pin::Pin;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::Mutex;

/// How long `auto` waits for a WT handshake before downgrading. A real QUIC
/// handshake completes well inside this; 500ms (the plan's original mobile
/// figure) is not enough over a real network.
pub const AUTO_DIAL_TIMEOUT: Duration = Duration::from_millis(2500);

/// After a failed `auto` handshake, don't probe again for this long. Without
/// it every stream would pay the dial timeout against a server that can never
/// serve WebTransport. A server that later gains TLS is picked up when the
/// window expires.
pub const WT_RETRY_BACKOFF: Duration = Duration::from_secs(60);

/// Chat run events: identical shape on both transports.
pub type ChatEventStream = Pin<Box<dyn Stream<Item = Result<AgentSessionEvent>> + Send>>;

/// Which transport the streaming surfaces use.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum TransportMode {
    Legacy,
    Wt,
    /// WebTransport when the server can serve it, legacy otherwise.
    Auto,
}

impl TransportMode {
    /// `CONSOLE_TRANSPORT` selects the mode; anything unrecognized (and an
    /// unset variable) is `auto`, so QUIC is the default and an unconfigured
    /// install still works against a legacy-only server.
    pub fn from_env() -> Self {
        Self::from_setting(&std::env::var("CONSOLE_TRANSPORT").unwrap_or_default())
    }

    pub fn from_setting(value: &str) -> Self {
        if value.eq_ignore_ascii_case("wt") {
            Self::Wt
        } else if value.eq_ignore_ascii_case("legacy") {
            Self::Legacy
        } else {
            Self::Auto
        }
    }

    /// Whether this mode ever dials WebTransport.
    fn attempts_wt(self) -> bool {
        matches!(self, Self::Wt | Self::Auto)
    }

    /// Whether a failed WebTransport handshake is fatal (`wt`) or a silent
    /// downgrade to legacy (`auto`).
    fn wt_required(self) -> bool {
        matches!(self, Self::Wt)
    }
}

/// Shared transport selection. Cloning is a refcount bump, and every clone
/// shares the one WebTransport session, so the app opens a single QUIC
/// connection no matter how many streams it opens. The session is replaced
/// when it dies or when the app switches servers — see [`Transport::connect`].
#[derive(Clone, Default)]
pub struct Transport {
    inner: Arc<TransportInner>,
}

struct TransportInner {
    mode: TransportMode,
    /// Fixed WT origin from `CONSOLE_WT_ORIGIN`. `None` derives the origin
    /// from the client's current base URL, which is what production wants
    /// (HTTP/3 shares the API origin) and what makes the in-app server picker
    /// work: switching servers redials to the new one.
    origin: Option<String>,
    tls: WtTls,
    state: Mutex<TransportState>,
}

#[derive(Default)]
struct TransportState {
    session: Option<CachedSession>,
    /// In `auto`, when a failed handshake stops us probing until then.
    /// `legacy` and a live `wt` session ignore it.
    retry_after: Option<Instant>,
}

/// The live session plus the origin it was dialed against, so a server switch
/// is detectable rather than silently reusing the old connection.
struct CachedSession {
    origin: String,
    service: WtService,
}

impl Default for TransportInner {
    fn default() -> Self {
        Self {
            mode: TransportMode::from_env(),
            origin: std::env::var("CONSOLE_WT_ORIGIN").ok(),
            // A malformed pin must not take the app down at construction; it
            // surfaces when the first stream asks to connect.
            tls: tls_from_setting(&std::env::var("CONSOLE_WT_TLS").unwrap_or_default())
                .unwrap_or(WtTls::Native),
            state: Mutex::new(TransportState::default()),
        }
    }
}

impl Transport {
    /// A transport pinned to one mode with the remaining settings from the
    /// environment.
    pub fn new(mode: TransportMode) -> Self {
        Self {
            inner: Arc::new(TransportInner {
                mode,
                ..Default::default()
            }),
        }
    }

    /// A transport with every setting explicit — for the live interop test
    /// and, later, a settings toggle.
    pub fn with_settings(mode: TransportMode, origin: Option<String>, tls: WtTls) -> Self {
        Self {
            inner: Arc::new(TransportInner {
                mode,
                origin,
                tls,
                state: Mutex::new(TransportState::default()),
            }),
        }
    }

    pub fn mode(&self) -> TransportMode {
        self.inner.mode
    }

    /// Whether this transport ever dials WebTransport. In `auto` that is
    /// still true: it prefers QUIC and only downgrades per
    /// [`Transport::connect`].
    pub fn is_wt(&self) -> bool {
        self.inner.mode.attempts_wt()
    }

    /// The live WT session, if one has been opened. `None` on legacy, before
    /// the first WT stream, and while `auto` is downgraded.
    pub async fn session(&self) -> Option<WtService> {
        self.inner
            .state
            .lock()
            .await
            .session
            .as_ref()
            .map(|cached| cached.service.clone())
    }

    /// Where this transport dials, and whether `auto` should probe it at
    /// all.
    ///
    /// WebTransport mandates TLS, so a plaintext origin can never serve it:
    /// probing one would just stall the first stream for the dial timeout
    /// (which is every `make dev-server` boot). An explicit `CONSOLE_WT_ORIGIN`
    /// always wins — asking for it explicitly means asking for it.
    async fn resolve_origin(&self, client: &ConsoleClient) -> (String, bool) {
        let base = client.base_url().await;
        match &self.inner.origin {
            Some(origin) => (origin.clone(), true),
            None => (upgrade_to_https(&base), base.starts_with("https://")),
        }
    }

    /// The session to use, or `None` for "use legacy".
    ///
    /// One handshake at a time (the mutex is held across the dial, so
    /// concurrent first-streams do not race), and the session is reused until
    /// it stops being usable: redial when QUIC has closed it (server restart,
    /// network loss) or when the origin no longer matches the app's server.
    /// Failures are never cached as sessions, so the app can start before its
    /// server is reachable and retry on the next stream.
    ///
    /// `auto` additionally caps the dial at [`AUTO_DIAL_TIMEOUT`] and, on
    /// failure, records a [`WT_RETRY_BACKOFF`] window and returns `None` so the
    /// caller takes the legacy path. `wt` returns the error instead.
    pub async fn connect(&self, client: &ConsoleClient) -> Result<Option<WtService>> {
        let mode = self.mode();
        if !mode.attempts_wt() {
            return Ok(None);
        }
        let (origin, probeable) = self.resolve_origin(client).await;
        if mode == TransportMode::Auto && !probeable {
            return Ok(None);
        }
        let mut state = self.inner.state.lock().await;
        if let Some(cached) = state.session.as_ref()
            && cached.origin == origin
            && !cached.service.is_closed()
        {
            return Ok(Some(cached.service.clone()));
        }
        if !mode.wt_required()
            && state
                .retry_after
                .is_some_and(|retry_at| Instant::now() < retry_at)
        {
            return Ok(None);
        }
        let tls = self.inner.tls.clone();
        let dial = WtService::connect(client.transport(), Some(&origin), tls);
        let dial = match mode.wt_required() {
            // `wt` has no cap: let the transport's own error surface intact.
            true => dial.await,
            // `auto` must not let a black-holed UDP socket hang the app: cap
            // the handshake and downgrade on the deadline.
            false => match tokio::time::timeout(AUTO_DIAL_TIMEOUT, dial).await {
                Ok(result) => result,
                Err(_) => Err(anyhow::anyhow!(
                    "WebTransport handshake at {origin} timed out after {}ms",
                    AUTO_DIAL_TIMEOUT.as_millis()
                )),
            },
        };
        match dial {
            Ok(service) => {
                state.session = Some(CachedSession {
                    origin,
                    service: service.clone(),
                });
                state.retry_after = None;
                Ok(Some(service))
            }
            Err(error) if mode.wt_required() => Err(error)
                .with_context(|| format!("Failed to open a WebTransport session at {origin}")),
            Err(error) => {
                log::warn!(
                    "WebTransport unavailable at {origin}, using the legacy transport for {}: {error:#}",
                    WT_RETRY_BACKOFF.as_secs()
                );
                state.session = None;
                state.retry_after = Some(Instant::now() + WT_RETRY_BACKOFF);
                Ok(None)
            }
        }
    }

    /// Close the connection and forget it, so the next stream redials.
    pub async fn close(&self, reason: &str) {
        let mut state = self.inner.state.lock().await;
        if let Some(cached) = state.session.take() {
            cached.service.close(reason);
        }
    }

    /// Start an agent run over the selected transport.
    pub async fn start_run(
        &self,
        client: &ConsoleClient,
        session_id: &str,
        prompt: &RunPromptDto,
    ) -> Result<ChatEventStream> {
        match self.connect(client).await? {
            Some(wt) => wt.start_run(session_id, prompt).await,
            None => Ok(Box::pin(
                client
                    .runs
                    .stream_prompt(session_id, prompt.clone())
                    .await?,
            )),
        }
    }

    /// Attach to a run in flight over the selected transport.
    pub async fn attach_run(
        &self,
        client: &ConsoleClient,
        session_id: &str,
        since: Option<u64>,
    ) -> Result<ChatEventStream> {
        match self.connect(client).await? {
            Some(wt) => wt.attach_run(session_id, since).await,
            None => Ok(Box::pin(
                client.runs.attach_run_stream(session_id, since).await?,
            )),
        }
    }

    pub async fn watch_fs(
        &self,
        client: &ConsoleClient,
        path: &str,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<FsChangeEvent>> + Send>>> {
        match self.connect(client).await? {
            Some(wt) => wt.watch_fs(path).await,
            None => client.fs.watch_events(path).await,
        }
    }

    pub async fn watch_git_status(
        &self,
        client: &ConsoleClient,
        path: &str,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<GitStatusSummary>> + Send>>> {
        match self.connect(client).await? {
            Some(wt) => wt.watch_git_status(path).await,
            None => client.git.watch_status(path).await,
        }
    }

    pub async fn watch_ports(
        &self,
        client: &ConsoleClient,
        project_id: Option<&str>,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<Vec<ForwardedPort>>> + Send>>> {
        match self.connect(client).await? {
            Some(wt) => wt.watch_ports(project_id).await,
            None => client.ports.watch(project_id).await,
        }
    }

    pub async fn watch_notifications(
        &self,
        client: &ConsoleClient,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<crate::types::NotificationEvent>> + Send>>> {
        match self.connect(client).await? {
            Some(wt) => wt.watch_notifications().await,
            None => client.notifications.stream().await,
        }
    }

    pub async fn watch_script_run(
        &self,
        client: &ConsoleClient,
        project_id: &str,
        run_id: &str,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<crate::types::ScriptRunEvent>> + Send>>> {
        match self.connect(client).await? {
            Some(wt) => wt.watch_script_run(project_id, run_id).await,
            None => client.scripts.stream_run(project_id, run_id).await,
        }
    }

    /// Spawn a terminal over the selected transport. Both paths return the
    /// same [`TerminalHandle`], so the terminal view never learns which
    /// transport it is on.
    pub async fn spawn_terminal(
        &self,
        client: &ConsoleClient,
        params: TerminalSpawnParams,
        initial_size: TerminalSize,
    ) -> Result<TerminalHandle> {
        match self.connect(client).await? {
            Some(wt) => wt.spawn_terminal_handle(&params, initial_size).await,
            None => {
                TerminalService::new(client.transport().clone())
                    .spawn(params, initial_size)
                    .await
            }
        }
    }
}

/// TLS trust for the WT handshake: platform roots by default, or pinned
/// server-certificate hashes for a self-signed dev certificate
/// (`CONSOLE_WT_TLS=pin:<hex>[,<hex>]`).
pub fn tls_from_setting(value: &str) -> Result<WtTls> {
    let Some(pins) = value.strip_prefix("pin:") else {
        return Ok(WtTls::Native);
    };
    let mut hashes = Vec::new();
    for hash in pins.split(',').map(str::trim).filter(|h| !h.is_empty()) {
        let clean: String = hash.chars().filter(|c| *c != ':').collect();
        if clean.len() != 64 {
            anyhow::bail!("WebTransport TLS pin '{hash}' is not 64 hex characters of SHA-256.");
        }
        let mut digest = [0u8; 32];
        for (index, byte) in digest.iter_mut().enumerate() {
            *byte = u8::from_str_radix(&clean[2 * index..2 * index + 2], 16).map_err(|_| {
                anyhow::anyhow!("WebTransport TLS pin '{hash}' is not hexadecimal.")
            })?;
        }
        hashes.push(digest);
    }
    if hashes.is_empty() {
        anyhow::bail!("WebTransport TLS pinning needs at least one SHA-256 hash.");
    }
    Ok(WtTls::PinnedHashes(hashes))
}
