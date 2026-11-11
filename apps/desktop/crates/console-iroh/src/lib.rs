//! Reaching a Console server by iroh node ID.
//!
//! The server (built with the `iroh` tag) accepts QUIC connections on the
//! `console-device/1` ALPN and bridges each bidirectional stream, byte for
//! byte, to its local HTTP port. [`Tunnel`] dials a node ID and exposes it as
//! a loopback TCP port: everything that speaks HTTP or WebSocket to
//! `http://127.0.0.1:<port>` (reqwest, tokio-tungstenite) reaches the server
//! over iroh without knowing it.

use std::net::SocketAddr;
use std::str::FromStr;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use iroh::endpoint::{Connection, RecvStream, SendStream, presets};
use iroh::{Endpoint, EndpointAddr, EndpointId};
use tokio::io::{AsyncWriteExt, copy};
use tokio::net::{TcpListener, TcpStream};
use tokio::sync::Mutex;
use tokio::task::JoinHandle;

/// The protocol the device API speaks over iroh. Must match the server.
pub const ALPN: &[u8] = b"console-device/1";

/// How long to wait for the relay before dialling anyway.
const ONLINE_WAIT: Duration = Duration::from_secs(10);

struct Shared {
    endpoint: Endpoint,
    addr: EndpointAddr,
    // The live connection, dialled on first use and redialled if it drops.
    conn: Mutex<Option<Connection>>,
}

impl Shared {
    async fn open(&self) -> Result<(SendStream, RecvStream)> {
        let mut guard = self.conn.lock().await;
        if let Some(conn) = guard.as_ref() {
            match conn.open_bi().await {
                Ok(stream) => return Ok(stream),
                Err(err) => {
                    log::debug!("iroh connection lost, redialling: {err}");
                    *guard = None;
                }
            }
        }
        // The relay path needs us registered before it can find the peer;
        // offline this can hang, so cap it and dial anyway.
        let _ = tokio::time::timeout(ONLINE_WAIT, self.endpoint.online()).await;
        let conn = self
            .endpoint
            .connect(self.addr.clone(), ALPN)
            .await
            .context("connect to the server over iroh")?;
        let stream = conn.open_bi().await.context("open stream")?;
        *guard = Some(conn);
        Ok(stream)
    }
}

/// A loopback port that forwards to a Console server over iroh.
///
/// The port exists as soon as the tunnel is created; iroh is dialled on the
/// first request, so creating one never blocks on the network.
pub struct Tunnel {
    addr: SocketAddr,
    task: JoinHandle<()>,
    shared: Arc<Shared>,
}

impl Tunnel {
    /// Bind a loopback port for `node_id`. No network traffic yet.
    pub async fn new(node_id: &str) -> Result<Self> {
        let endpoint = Endpoint::builder(presets::N0)
            .bind()
            .await
            .context("bind iroh endpoint")?;
        Self::over(endpoint, EndpointAddr::new(parse_node_id(node_id)?)).await
    }

    /// Like [`Tunnel::new`], then dial now so a bad node ID or an
    /// unreachable server fails here rather than on the first request.
    pub async fn connect(node_id: &str) -> Result<Self> {
        let tunnel = Self::new(node_id).await?;
        tunnel.shared.open().await?;
        Ok(tunnel)
    }

    /// Like [`Tunnel::new`] with an endpoint and address the caller built
    /// (tests use direct addresses and no relay).
    pub async fn over(endpoint: Endpoint, addr: EndpointAddr) -> Result<Self> {
        let listener = TcpListener::bind(("127.0.0.1", 0)).await?;
        let local = listener.local_addr()?;
        let shared = Arc::new(Shared {
            endpoint,
            addr,
            conn: Mutex::new(None),
        });
        let task = tokio::spawn(accept_loop(listener, shared.clone()));
        Ok(Self {
            addr: local,
            task,
            shared,
        })
    }

    /// The loopback address to use as the base URL host.
    pub fn local_addr(&self) -> SocketAddr {
        self.addr
    }

    /// `http://127.0.0.1:<port>`, the base URL for the existing HTTP client.
    pub fn base_url(&self) -> String {
        format!("http://{}", self.addr)
    }
}

impl Drop for Tunnel {
    fn drop(&mut self) {
        self.task.abort();
    }
}

/// Parse the 64-hex node ID a pairing URL carries.
pub fn parse_node_id(node_id: &str) -> Result<EndpointId> {
    EndpointId::from_str(node_id).map_err(|e| anyhow::anyhow!("invalid iroh node id: {e}"))
}

async fn accept_loop(listener: TcpListener, shared: Arc<Shared>) {
    loop {
        let Ok((tcp, _)) = listener.accept().await else {
            return;
        };
        let shared = shared.clone();
        tokio::spawn(async move {
            if let Err(err) = pipe(tcp, &shared).await {
                log::debug!("iroh stream ended: {err:#}");
            }
        });
    }
}

/// One local TCP connection <-> one QUIC bidirectional stream.
async fn pipe(tcp: TcpStream, shared: &Shared) -> Result<()> {
    let (mut send, mut recv) = shared.open().await?;
    let (mut tcp_r, mut tcp_w) = tcp.into_split();
    let up = async {
        copy(&mut tcp_r, &mut send).await?;
        // Half-close: the server sees EOF on the request side.
        send.finish()?;
        anyhow::Ok(())
    };
    let down = async {
        copy(&mut recv, &mut tcp_w).await?;
        tcp_w.shutdown().await?;
        anyhow::Ok(())
    };
    let (u, d) = tokio::join!(up, down);
    u?;
    d?;
    Ok(())
}
