//! Local port forwarding: expose a remote dev server as real localhost.
//!
//! The server only reports that e.g. remote `8000` exists. It cannot hand us
//! a `localhost` URL because its localhost is the VPS, not this machine.
//! This manager owns one `127.0.0.1` listener per remote port (OS-picked, never
//! hardcoded) and pipes each local TCP connection through the server tunnel:
//!
//!   browser localhost:local <-> TcpListener <-> ws(s) /api/ports/<remote>/tunnel <-> VPS 127.0.0.1:remote
//!
//! Raw TCP is forwarded byte-for-byte, so React roots, `/assets/*`,
//! absolute paths, and HMR websockets behave like a local dev server.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use anyhow::Result;
use futures_util::{SinkExt, StreamExt};
use tokio::net::{TcpListener, TcpStream};
use tokio::sync::Mutex;
use tokio::task::JoinHandle;
use tokio_tungstenite::{connect_async, tungstenite::protocol::Message};

use crate::utils::HttpTransport;

/// One active local forward: remote VPS port exposed as local localhost URL.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct LocalForward {
    pub remote_port: u16,
    pub local_port: u16,
    pub local_url: String,
}

struct ActiveForward {
    local_port: u16,
    task: JoinHandle<()>,
}

#[derive(Clone)]
pub struct LocalForwardManager {
    transport: HttpTransport,
    active: Arc<Mutex<HashMap<u16, ActiveForward>>>,
}

impl LocalForwardManager {
    pub fn new(transport: HttpTransport) -> Self {
        Self {
            transport,
            active: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    /// WebSocket tunnel URL for a remote port, derived from the backend URL.
    pub async fn tunnel_url(&self, remote_port: u16) -> String {
        let base = self.transport.base_url().await;
        let ws_base = if let Some(rest) = base.strip_prefix("https://") {
            format!("wss://{rest}")
        } else if let Some(rest) = base.strip_prefix("http://") {
            format!("ws://{rest}")
        } else {
            format!("ws://{base}")
        };
        format!(
            "{}/api/ports/{remote_port}/tunnel",
            ws_base.trim_end_matches('/')
        )
    }

    /// True when the backend runs on this machine: the remote port is already
    /// local, so no listener is needed.
    pub async fn is_local_backend(&self) -> bool {
        let base = self.transport.base_url().await.to_lowercase();
        base.contains("://localhost")
            || base.contains("://127.0.0.1")
            || base.contains("://[::1]")
    }

    /// Reconcile listeners with the latest remote port list.
    /// Returns one [`LocalForward`] per remote port with its localhost URL.
    pub async fn ensure(&self, remote_ports: &[u16]) -> Vec<LocalForward> {
        if self.is_local_backend().await {
            self.clear().await;
            let mut out: Vec<LocalForward> = remote_ports
                .iter()
                .map(|&remote| LocalForward {
                    remote_port: remote,
                    local_port: remote,
                    local_url: format!("http://localhost:{remote}/"),
                })
                .collect();
            out.sort_by_key(|f| f.remote_port);
            return out;
        }

        let wanted: HashSet<u16> = remote_ports.iter().copied().collect();
        let mut active = self.active.lock().await;
        // Drop forwards whose remote port disappeared.
        let stale: Vec<u16> = active
            .keys()
            .copied()
            .filter(|port| !wanted.contains(port))
            .collect();
        for port in stale {
            if let Some(entry) = active.remove(&port) {
                entry.task.abort();
            }
        }
        // Start listeners for new remote ports.
        for &remote in &wanted {
            if active.contains_key(&remote) {
                continue;
            }
            match self.bind_local(remote).await {
                Some((local_port, task)) => {
                    log::info!("Forwarding localhost:{local_port} to remote port {remote}");
                    active.insert(remote, ActiveForward { local_port, task });
                }
                None => {
                    log::warn!("No free localhost port for remote {remote}");
                }
            }
        }
        let mut out: Vec<LocalForward> = active
            .iter()
            .map(|(&remote, entry)| LocalForward {
                remote_port: remote,
                local_port: entry.local_port,
                local_url: format!("http://localhost:{}/", entry.local_port),
            })
            .collect();
        out.sort_by_key(|f| f.remote_port);
        out
    }

    /// Stop every local listener (e.g. on backend switch).
    pub async fn clear(&self) {
        let mut active = self.active.lock().await;
        for (_, entry) in active.drain() {
            entry.task.abort();
        }
    }

    /// Stop one local listener (e.g. after unforward).
    pub async fn remove(&self, remote_port: u16) {
        let mut active = self.active.lock().await;
        if let Some(entry) = active.remove(&remote_port) {
            entry.task.abort();
        }
    }

    /// Bind 127.0.0.1:preferred (usually the remote number), falling back to
    /// an OS-picked ephemeral port. Never binds 0.0.0.0.
    async fn bind_local(&self, preferred: u16) -> Option<(u16, JoinHandle<()>)> {
        let listener = match TcpListener::bind(format!("127.0.0.1:{preferred}")).await {
            Ok(listener) => listener,
            Err(_) => TcpListener::bind("127.0.0.1:0").await.ok()?,
        };
        let local_port = listener.local_addr().ok()?.port();
        let transport = self.transport.clone();
        let task = tokio::spawn(async move {
            accept_loop(listener, transport, preferred).await;
        });
        Some((local_port, task))
    }
}

/// Accept local browser connections and tunnel each one to the VPS.
async fn accept_loop(listener: TcpListener, transport: HttpTransport, remote_port: u16) {
    loop {
        let (stream, _) = match listener.accept().await {
            Ok(pair) => pair,
            Err(_) => break,
        };
        let manager = LocalForwardManager {
            transport: transport.clone(),
            active: Arc::new(Mutex::new(HashMap::new())),
        };
        tokio::spawn(async move {
            let ws_url = manager.tunnel_url(remote_port).await;
            if let Err(error) = pipe_connection(stream, &ws_url).await {
                // Warn (not debug): a failing tunnel is the difference between
                // a working preview and about:blank. The URL carries no secret.
                log::warn!("Tunnel localhost -> remote {remote_port} via {ws_url} failed: {error:#}");
            }
        });
    }
}

/// Pipe one local TCP connection through one tunnel WebSocket.
/// Each WS message is one TCP chunk in either direction.
async fn pipe_connection(local: TcpStream, ws_url: &str) -> Result<()> {
    let (ws_stream, _) = connect_async(ws_url).await?;
    let (mut ws_sink, mut ws_stream) = ws_stream.split();
    let (mut tcp_read, mut tcp_write) = local.into_split();
    let tcp_to_ws = async move {
        let mut buf = vec![0u8; 8192];
        loop {
            use tokio::io::AsyncReadExt;
            let n = tcp_read.read(&mut buf).await?;
            if n == 0 {
                let _ = ws_sink.send(Message::Close(None)).await;
                break;
            }
            ws_sink.send(Message::Binary(buf[..n].to_vec().into())).await?;
        }
        anyhow::Ok(())
    };
    let ws_to_tcp = async move {
        use tokio::io::AsyncWriteExt;
        while let Some(message) = ws_stream.next().await {
            let message = message?;
            match message {
                Message::Binary(data) => tcp_write.write_all(&data).await?,
                Message::Text(text) => tcp_write.write_all(text.as_bytes()).await?,
                Message::Close(_) => break,
                _ => {}
            }
        }
        anyhow::Ok(())
    };
    tokio::select! {
        result = tcp_to_ws => result?,
        result = ws_to_tcp => result?,
    }
    Ok(())
}
