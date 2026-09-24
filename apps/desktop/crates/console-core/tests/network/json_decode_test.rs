use console_core::utils::decode_json_bytes;
use serde::Deserialize;

#[derive(Debug, Deserialize, PartialEq)]
struct Payload {
    name: String,
    count: u32,
}

#[tokio::test]
async fn decodes_buffered_json_off_thread() {
    let payload: Payload = decode_json_bytes(br#"{"name":"console","count":3}"#.to_vec())
        .await
        .expect("decode payload");

    assert_eq!(
        payload,
        Payload {
            name: "console".to_string(),
            count: 3,
        }
    );
}

#[tokio::test]
async fn reports_invalid_buffered_json() {
    let error = decode_json_bytes::<Payload>(b"not-json".to_vec())
        .await
        .expect_err("invalid JSON must fail");

    assert!(error.to_string().contains("Failed to decode JSON response"));
}
