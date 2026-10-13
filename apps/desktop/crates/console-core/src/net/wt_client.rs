//! WebTransport session + framed streams over QUIC.
//!
//! One [`WtSession`] per device (one `CONNECT /wt/v1/connect?token=...`),
//! multiplexed with QUIC bidi streams. Stream kinds reuse the
//! `console.v1.WtStreamKind` numeric values; payloads are the exact SSE/WS
//! body bytes, so only framing is new.

use anyhow::{Context, Result};
use wtransport::{ClientConfig, Endpoint, endpoint::ConnectOptions, endpoint::endpoint_side};

/// Stream kinds: numeric values must match `console.v1.WtStreamKind`.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum WtStreamKind {
    ChatRun = 1,
    Terminal = 2,
    Watch = 3,
    PortTunnel = 4,
    DeviceStream = 5,
}

/// TLS trust: platform roots, or pinned server-cert hashes for dev servers
/// (self-signed `make dev-server-wt` certs). Hashes are raw SHA-256 bytes;
/// print the dev cert's with `openssl x509 -fingerprint -sha256 -noout`.
pub enum WtTls {
    Native,
    PinnedHashes(Vec<[u8; 32]>),
}

/// 8MB frame cap, mirroring the server bound.
pub const WT_MAX_FRAME_BYTES: usize = 8 * 1024 * 1024;

/// 512-byte stream id cap, mirroring the server bound.
pub const WT_MAX_STREAM_ID_LEN: usize = 512;

/// QUIC variable-length integer (RFC 9000 §16): 2-bit length prefix.
pub fn encode_varint(out: &mut Vec<u8>, v: u64) {
    if v < 0x40 {
        out.push(v as u8);
    } else if v < 0x4000 {
        out.push((0x40 | (v >> 8)) as u8);
        out.push(v as u8);
    } else if v < 0x4000_0000 {
        out.push((0x80 | (v >> 24)) as u8);
        out.push((v >> 16) as u8);
        out.push((v >> 8) as u8);
        out.push(v as u8);
    } else {
        out.push((0xC0 | (v >> 56)) as u8);
        out.push((v >> 48) as u8);
        out.push((v >> 40) as u8);
        out.push((v >> 32) as u8);
        out.push((v >> 24) as u8);
        out.push((v >> 16) as u8);
        out.push((v >> 8) as u8);
        out.push(v as u8);
    }
}

/// Returns `(value, bytes consumed)` or `None` when truncated.
pub fn decode_varint(buf: &[u8]) -> Option<(u64, usize)> {
    let first = *buf.first()?;
    let len = 1usize << (first >> 6);
    if buf.len() < len {
        return None;
    }
    let mut value = (first & 0x3f) as u64;
    for b in &buf[1..len] {
        value = (value << 8) | (*b as u64);
    }
    Some((value, len))
}

/// Encoded `[uvarint kind][uvarint id_len][id]` stream header.
pub fn encode_stream_header(kind: WtStreamKind, id: &str) -> Result<Vec<u8>> {
    if id.len() > WT_MAX_STREAM_ID_LEN {
        anyhow::bail!(
            "WT stream id of {} bytes exceeds the {}-byte cap.",
            id.len(),
            WT_MAX_STREAM_ID_LEN
        );
    }
    let mut out = Vec::with_capacity(id.len() + 4);
    encode_varint(&mut out, kind as u64);
    encode_varint(&mut out, id.len() as u64);
    out.extend_from_slice(id.as_bytes());
    Ok(out)
}

/// Encoded `[u32BE len][protojson]` frame.
pub fn encode_wt_frame(frame: &console_proto::WtFrame) -> Result<Vec<u8>> {
    let body =
        serde_json::to_vec(frame).context("Failed to encode WT frame payload")?;
    if body.len() > WT_MAX_FRAME_BYTES {
        anyhow::bail!(
            "WT frame of {} bytes exceeds the {}-byte cap.",
            body.len(),
            WT_MAX_FRAME_BYTES
        );
    }
    let mut out = Vec::with_capacity(body.len() + 4);
    out.extend_from_slice(&(body.len() as u32).to_be_bytes());
    out.extend_from_slice(&body);
    Ok(out)
}

/// Decoded frame from a length-prefixed buffer; returns the frame plus the
/// total bytes consumed, or `None` when fewer bytes are available.
pub fn decode_wt_frame(buf: &[u8]) -> Result<Option<(console_proto::WtFrame, usize)>> {
    if buf.len() < 4 {
        return Ok(None);
    }
    let len = u32::from_be_bytes([buf[0], buf[1], buf[2], buf[3]]) as usize;
    if len == 0 || len > WT_MAX_FRAME_BYTES {
        anyhow::bail!("WT frame length {len} out of bounds.");
    }
    if buf.len() < 4 + len {
        return Ok(None);
    }
    let frame: console_proto::WtFrame = serde_json::from_slice(&buf[4..4 + len])
        .context("Failed to decode WT frame payload")?;
    Ok(Some((frame, 4 + len)))
}

/// A connected device session. Holds the endpoint: dropping it kills the
/// underlying QUIC socket, so it lives as long as any stream.
pub struct WtSession {
    _endpoint: Endpoint<endpoint_side::Client>,
    connection: wtransport::Connection,
}

/// A framed bidi stream with a small reassembly buffer (frames coalesce on
/// the wire, so reads rarely align to frame boundaries).
pub struct WtStream {
    send: wtransport::SendStream,
    recv: wtransport::RecvStream,
    pending: Vec<u8>,
}

/// Unit struct for the [`WtClient`] namespace.
pub struct WtClient;

impl WtClient {
    /// Connect one device session. The token rides the query string
    /// (base64url needs no further encoding there).
    pub async fn connect(base_url: &str, token: &str, tls: WtTls) -> Result<WtSession> {
        let config = match tls {
            WtTls::Native => ClientConfig::builder()
                .with_bind_default()
                .with_native_certs()
                .build(),
            WtTls::PinnedHashes(hashes) => {
                let digests: Vec<_> = hashes
                    .into_iter()
                    .map(wtransport::tls::Sha256Digest::new)
                    .collect();
                ClientConfig::builder()
                    .with_bind_default()
                    .with_server_certificate_hashes(digests)
                    .build()
            }
        };
        let endpoint =
            Endpoint::client(config).context("Failed to create QUIC endpoint")?;
        let url = format!(
            "{}/wt/v1/connect?token={}",
            base_url.trim_end_matches('/'),
            urlencoding::encode(token)
        );
        let connection = endpoint
            .connect(ConnectOptions::builder(url).build())
            .await
            .context("WebTransport handshake failed")?;
        Ok(WtSession {
            _endpoint: endpoint,
            connection,
        })
    }
}

impl WtSession {
    /// Open a framed stream of `kind` with routing `id`, writing its header.
    pub async fn open_stream(&self, kind: WtStreamKind, id: &str) -> Result<WtStream> {
        let (mut send, recv) = self
            .connection
            .open_bi()
            .await
            .context("Failed to open WT stream")?
            .await
            .context("Failed to open WT stream")?;
        let header = encode_stream_header(kind, id)?;
        send
            .write_all(&header)
            .await
            .context("Failed to write WT stream header")?;
        Ok(WtStream {
            send,
            recv,
            pending: Vec::new(),
        })
    }

    /// Close the session (all streams unwind on the same cause).
    pub fn close(&self, reason: &[u8]) {
        self.connection
            .close(wtransport::VarInt::from_u32(0), reason);
    }
}

impl WtStream {
    /// Send one length-prefixed frame.
    pub async fn send_frame(&mut self, frame: &console_proto::WtFrame) -> Result<()> {
        let raw = encode_wt_frame(frame)?;
        self.send
            .write_all(&raw)
            .await
            .context("Failed to write WT frame")?;
        Ok(())
    }

    /// Next frame, or `None` on clean FIN with no partial data. Errors on
    /// truncation mid-frame (peer died mid-write).
    pub async fn next_frame(&mut self) -> Result<Option<console_proto::WtFrame>> {
        loop {
            if let Some((frame, consumed)) = decode_wt_frame(&self.pending)? {
                self.pending.drain(..consumed);
                return Ok(Some(frame));
            }
            let mut chunk = [0u8; 8192];
            match self.recv.read(&mut chunk).await {
                Ok(Some(0)) | Ok(None) => {
                    if self.pending.is_empty() {
                        return Ok(None);
                    }
                    anyhow::bail!("WT stream ended mid-frame.");
                }
                Ok(Some(n)) => self.pending.extend_from_slice(&chunk[..n]),
                Err(error) => anyhow::bail!("WT stream read failed: {error:?}"),
            }
        }
    }

    /// Raw write for byte-pipe streams (tunnel). No framing.
    pub async fn write_raw(&mut self, buf: &[u8]) -> Result<()> {
        self.send
            .write_all(buf)
            .await
            .context("Failed to write WT stream bytes")?;
        Ok(())
    }

    /// Raw read for byte-pipe streams. `None` on clean FIN.
    pub async fn read_raw(&mut self, buf: &mut [u8]) -> Result<Option<usize>> {
        match self.recv.read(buf).await {
            Ok(n) => Ok(n),
            Err(error) => anyhow::bail!("WT stream read failed: {error:?}"),
        }
    }

    /// Graceful write-side close.
    pub async fn finish(&mut self) -> Result<()> {
        self.send
            .finish()
            .await
            .context("Failed to finish WT stream")?;
        Ok(())
    }
}
