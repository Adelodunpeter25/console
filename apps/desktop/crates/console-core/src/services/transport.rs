//! Transport selection for streaming surfaces (QUIC migration, Phase 4).
//!
//! One hard switch, chosen once per app run from `CONSOLE_TRANSPORT`:
//!
//! * `legacy` (the default) — SSE/WS exactly as before, so a plain
//!   `make dev-server` keeps working untouched.
//! * `wt` — chat runs, watches and terminals go over one WebTransport
//!   session. Nothing races the legacy path yet: that waits for real
//!   failure data from daily driving (see the plan's Phase 4/5).
//!
//! Every method here returns the *same* item type on both paths — WT payloads
//! are the legacy bodies byte for byte — so callers hold one type and only
//! the seam moves.
//!
//! Env knobs (all optional):
//!
//! | variable | meaning |
//! |---|---|
//! | `CONSOLE_TRANSPORT` | `wt` or `legacy` |
//! | `CONSOLE_WT_ORIGIN` | WT origin when the QUIC listener is not the API origin (`make dev-server-wt`) |
//! | `CONSOLE_WT_TLS` | `native` (default) or `pin:<sha256 hex>[,<sha256 hex>…]` |

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
use tokio::sync::Mutex;

/// Chat run events: identical shape on both transports.
pub type ChatEventStream = Pin<Box<dyn Stream<Item = Result<AgentSessionEvent>> + Send>>;

/// Which transport the streaming surfaces use.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum TransportMode {
    Legacy,
    Wt,
}

impl TransportMode {
    /// `CONSOLE_TRANSPORT=wt` opts in; anything else (including unset) stays
    /// legacy, so the default install is unchanged.
    pub fn from_env() -> Self {
        Self::from_setting(&std::env::var("CONSOLE_TRANSPORT").unwrap_or_default())
    }

    pub fn from_setting(value: &str) -> Self {
        if value.eq_ignore_ascii_case("wt") {
            Self::Wt
        } else {
            Self::Legacy
        }
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
    session: Mutex<Option<CachedSession>>,
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
            session: Mutex::new(None),
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
                session: Mutex::new(None),
            }),
        }
    }

    pub fn mode(&self) -> TransportMode {
        self.inner.mode
    }

    pub fn is_wt(&self) -> bool {
        self.mode() == TransportMode::Wt
    }

    /// The live WT session, if one has been opened. `None` on legacy, and
    /// before the first WT stream forced the handshake.
    pub async fn session(&self) -> Option<WtService> {
        self.inner
            .session
            .lock()
            .await
            .as_ref()
            .map(|c| c.service.clone())
    }

    /// Where this transport dials. `CONSOLE_WT_ORIGIN` wins; otherwise the
    /// client's current base URL over TLS, resolved per connect so the
    /// in-app server picker is honored.
    async fn resolve_origin(&self, client: &ConsoleClient) -> String {
        match &self.inner.origin {
            Some(origin) => origin.clone(),
            None => upgrade_to_https(&client.base_url().await),
        }
    }

    /// Open the WT session if the mode asks for it.
    ///
    /// One handshake at a time (the mutex is held across the dial, so
    /// concurrent first-streams do not race), and the session is reused until
    /// it stops being usable: redial when QUIC has closed it (server restart,
    /// network loss) or when the origin no longer matches the app's server.
    /// Failures are never cached, so the app can start before its server is
    /// reachable and retry on the next stream.
    pub async fn connect(&self, client: &ConsoleClient) -> Result<Option<WtService>> {
        if !self.is_wt() {
            return Ok(None);
        }
        let origin = self.resolve_origin(client).await;
        let mut cache = self.inner.session.lock().await;
        if let Some(cached) = cache.as_ref()
            && cached.origin == origin
            && !cached.service.is_closed()
        {
            return Ok(Some(cached.service.clone()));
        }
        let service = WtService::connect(client.transport(), Some(&origin), self.inner.tls.clone())
            .await
            .with_context(|| format!("Failed to open a WebTransport session at {origin}"))?;
        *cache = Some(CachedSession {
            origin,
            service: service.clone(),
        });
        Ok(Some(service))
    }

    /// Close the connection and forget it, so the next stream redials.
    pub async fn close(&self, reason: &str) {
        let mut cache = self.inner.session.lock().await;
        if let Some(cached) = cache.take() {
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
