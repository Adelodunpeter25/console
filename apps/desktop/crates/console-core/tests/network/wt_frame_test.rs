//! Unit tests for the WT framing pair: QUIC varints, stream headers, and
//! the length-prefixed protojson envelope. No network; the server asserts
//! the same shapes in `tests/api/wt_frame_test.go`.

use console_core::net::{
    WT_MAX_FRAME_BYTES, WT_MAX_STREAM_ID_LEN, WtStreamKind, decode_varint, decode_wt_frame,
    encode_stream_header, encode_varint, encode_wt_frame,
};

#[test]
fn varint_boundaries_round_trip() {
    for value in [
        0u64,
        1,
        63,
        64,
        16383,
        16384,
        0x3FFF_FFFF,
        0x4000_0000,
        u64::MAX >> 2,
    ] {
        let mut out = Vec::new();
        encode_varint(&mut out, value);
        let expected_len = if value < 0x40 {
            1
        } else if value < 0x4000 {
            2
        } else if value < 0x4000_0000 {
            4
        } else {
            8
        };
        assert_eq!(out.len(), expected_len, "varint size for {value}");
        assert_eq!(decode_varint(&out), Some((value, expected_len)));
    }
}

#[test]
fn varint_truncated_returns_none() {
    assert_eq!(decode_varint(&[]), None);
    // 2-byte varint with the tail missing.
    assert_eq!(decode_varint(&[0x40]), None);
    // 8-byte varint with the tail missing.
    assert_eq!(decode_varint(&[0xC0, 0x01, 0x02]), None);
}

#[test]
fn stream_header_round_trip() {
    let header = encode_stream_header(WtStreamKind::ChatRun, "session-1").expect("encode header");
    // kind=1, id_len=9, then the id bytes.
    assert_eq!(&header[..2], &[0x01, 0x09]);
    assert_eq!(&header[2..], b"session-1");
}

#[test]
fn stream_header_rejects_oversize_id() {
    let big = "x".repeat(WT_MAX_STREAM_ID_LEN + 1);
    assert!(encode_stream_header(WtStreamKind::Watch, &big).is_err());
    let max = "x".repeat(WT_MAX_STREAM_ID_LEN);
    assert!(encode_stream_header(WtStreamKind::Watch, &max).is_ok());
}

#[test]
fn frame_envelope_round_trip() {
    let frame = console_proto::WtFrame {
        seq: 42,
        r#type: "modelStreamPart".into(),
        payload: b"hi".to_vec(),
    };
    let raw = encode_wt_frame(&frame).expect("encode frame");
    let len = u32::from_be_bytes([raw[0], raw[1], raw[2], raw[3]]) as usize;
    assert_eq!(len, raw.len() - 4);
    let (decoded, consumed) = decode_wt_frame(&raw)
        .expect("decode frame")
        .expect("complete frame");
    assert_eq!(consumed, raw.len());
    assert_eq!(decoded.seq, 42);
    assert_eq!(decoded.r#type, "modelStreamPart");
    assert_eq!(decoded.payload, b"hi");
    // Protojson shape the server asserts in its golden fixture: seq as a
    // string, payload as base64.
    let body = String::from_utf8(raw[4..].to_vec()).expect("utf8 body");
    assert!(body.contains(r#""seq":"42""#), "seq encodes as string: {body}");
    assert!(
        body.contains(r#""payload":"aGk=""#),
        "payload encodes as base64: {body}"
    );
}

#[test]
fn frame_decode_needs_more_bytes() {
    let frame = console_proto::WtFrame {
        seq: 1,
        r#type: "ping".into(),
        payload: vec![],
    };
    let raw = encode_wt_frame(&frame).expect("encode frame");
    assert!(decode_wt_frame(&raw[..2]).expect("prefix").is_none());
    assert!(decode_wt_frame(&raw[..raw.len() - 1])
        .expect("truncated")
        .is_none());
}

#[test]
fn frame_rejects_bounds() {
    // Zero length.
    assert!(decode_wt_frame(&[0, 0, 0, 0]).is_err());
    // Past the cap.
    let mut huge = ((WT_MAX_FRAME_BYTES + 1) as u32).to_be_bytes().to_vec();
    huge.extend_from_slice(&[0u8; 4]);
    assert!(decode_wt_frame(&huge).is_err());
    // Oversize encode.
    let big = console_proto::WtFrame {
        seq: 0,
        r#type: String::new(),
        payload: vec![0u8; WT_MAX_FRAME_BYTES + 1],
    };
    assert!(encode_wt_frame(&big).is_err());
}
