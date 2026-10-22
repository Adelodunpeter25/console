//! WebTransport client transport (QUIC migration).
//!
//! Transport only: endpoint, session, framed bidi streams. No GPUI types,
//! no app state — read loops and marshaling live in `apps/desktop/src`
//! like the existing SSE/WS paths. Framing mirrors the server byte for
//! byte: stream header `[uvarint kind][uvarint id_len][id]`, then repeated
//! `[u32BE len][protojson(WtFrame)]` frames.
pub mod wt_client;

pub use wt_client::{
    WT_MAX_FRAME_BYTES, WT_MAX_STREAM_ID_LEN, WtClient, WtSession, WtStream, WtStreamKind,
    WtStreamReader, WtStreamWriter, WtTls, decode_varint, decode_wt_frame, encode_stream_header,
    encode_varint, encode_wt_frame, wt_root_store,
};
