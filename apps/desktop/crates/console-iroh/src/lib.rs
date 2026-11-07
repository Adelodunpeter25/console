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

use anyhow::{Context, Result};
use iroh::endpoint::{Connection, presets};
use iroh::{Endpoint, EndpointAddr, EndpointId};
use tokio::io::{AsyncWriteExt, copy};
use tokio::net::{TcpListener, TcpStream};
use tokio::task::JoinHandle;

/// The protocol the device API speaks over iroh. Must match the server.
pub const ALPN: &[u8] = b"console-device/1";

/// A loopback port that forwards to a Console server over iroh.
pub struct Tunnel {
    addr: SocketAddr,
    task: JoinHandle<()>,
    // Held so the endpoint (and its connection) outlive the accept loop.
    _endpoint: Endpoint,
}

impl Tunnel {
    /// Dial `node_id` through n0's relays and listen on a free loopback port.
    pub async fn connect(node_id: &str) -> Result<Self> {
        let endpoint = Endpoint::builder(presets::N0)
            .bind()
            .await
            .context("bind iroh endpoint")?;
        Self::over(endpoint, EndpointAddr::new(parse_node_id(node_id)?)).await
    }

    /// Like [`Tunnel::connect`] with an endpoint and address the caller built
    /// (tests use direct addresses and no relay).
    pub async fn over(endpoint: Endpoint, addr: EndpointAddr) -> Result<Self> {
        // The relay path needs us registered before it can find the peer.
        endpoint.online().await;
        let conn = Arc::new(
            endpoint
                .connect(addr, ALPN)
                .await
                .context("connect to the server over iroh")?,
        );
        let listener = TcpListener::bind(("127.0.0.1", 0)).await?;
        let addr = listener.local_addr()?;
        let task = tokio::spawn(accept_loop(listener, conn));
        Ok(Self { addr, task, _endpoint: endpoint })
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

async fn accept_loop(listener: TcpListener, conn: Arc<Connection>) {
    loop {
        let Ok((tcp, _)) = listener.accept().await else {
            return;
        };
        let conn = conn.clone();
        tokio::spawn(async move {
            if let Err(err) = pipe(tcp, &conn).await {
                log::debug!("iroh stream ended: {err:#}");
            }
        });
    }
}

/// One local TCP connection <-> one QUIC bidirectional stream.
async fn pipe(tcp: TcpStream, conn: &Connection) -> Result<()> {
    let (mut send, mut recv) = conn.open_bi().await.context("open stream")?;
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
