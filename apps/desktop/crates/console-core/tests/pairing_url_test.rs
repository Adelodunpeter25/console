//! Pairing URL parsing: the string a user pastes to connect a device.
//!
//! The grant is a credential, so the parser is strict — anything it cannot
//! read exactly is rejected rather than repaired, and the grant never
//! ends up in the address the client will talk to.

use console_core::utils::parse_pairing_url;

#[test]
fn parses_a_pairing_url() {
    let parsed = parse_pairing_url(
        "http://192.168.1.10:3000/console/pair#v=1&server=srv-1&grant=cp1_abc",
    )
    .expect("valid pairing URL");
    assert_eq!(parsed.base_url, "http://192.168.1.10:3000");
    assert_eq!(parsed.server_id, "srv-1");
    assert_eq!(parsed.grant, "cp1_abc");
}

#[test]
fn parses_a_https_pairing_url() {
    let parsed = parse_pairing_url("https://server.example/console/pair#v=1&server=s&grant=g")
        .expect("valid pairing URL");
    assert_eq!(parsed.base_url, "https://server.example");
}

#[test]
fn keeps_the_grant_out_of_the_address() {
    let parsed = parse_pairing_url("http://host:3000/console/pair#v=1&server=s&grant=secret")
        .expect("valid pairing URL");
    assert!(
        !parsed.base_url.contains("secret"),
        "the grant must not end up in the address: {}",
        parsed.base_url
    );
}

#[test]
fn accepts_a_percent_encoded_grant() {
    let parsed = parse_pairing_url("http://host:3000/console/pair#v=1&server=s&grant=a%2Bb")
        .expect("valid pairing URL");
    assert_eq!(parsed.grant, "a+b");
}

#[test]
fn rejects_malformed_urls() {
    for bad in [
        "",
        "   ",
        // No fragment at all: nothing to redeem.
        "http://host:3000/console/pair",
        // No scheme.
        "host:3000/console/pair#v=1&server=s&grant=g",
        // A scheme we do not speak.
        "ftp://host/console/pair#v=1&server=s&grant=g",
        // No host.
        "http:///console/pair#v=1&server=s&grant=g",
        // A version this client does not know.
        "http://host/console/pair#v=2&server=s&grant=g",
        // Missing pieces.
        "http://host/console/pair#v=1&grant=g",
        "http://host/console/pair#v=1&server=s",
        "http://host/console/pair#server=s&grant=g",
    ] {
        assert!(
            parse_pairing_url(bad).is_err(),
            "should have been rejected: {bad}"
        );
    }
}
