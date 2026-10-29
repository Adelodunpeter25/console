//! Pairing URL parsing: the string a user pastes to connect a device.
//!
//! The grant is a credential, so the parser is strict — anything it cannot
//! read exactly is rejected rather than repaired, and the grant never
//! ends up in the address the client will talk to.

use console_shared::pairing::{host_port, parse_pairing_url, split_pairing_line};

#[test]
fn parses_a_pairing_url() {
    let parsed = parse_pairing_url(
        "http://192.168.1.10:3000/console/pair#v=1&server=481e2796&code=vw9g-zu89-aphc",
    )
    .expect("valid pairing URL");
    assert_eq!(parsed.base_url, "http://192.168.1.10:3000");
    assert_eq!(parsed.server_id, "481e2796");
    assert_eq!(parsed.code, "vw9g-zu89-aphc");
}

#[test]
fn parses_a_https_pairing_url() {
    let parsed = parse_pairing_url("https://server.example/console/pair#v=1&server=s&code=c")
        .expect("valid pairing URL");
    assert_eq!(parsed.base_url, "https://server.example");
}

#[test]
fn keeps_the_code_out_of_the_address() {
    let parsed = parse_pairing_url("http://host:3000/console/pair#v=1&server=s&code=secret")
        .expect("valid pairing URL");
    assert!(
        !parsed.base_url.contains("secret"),
        "the code must not end up in the address: {}",
        parsed.base_url
    );
}

#[test]
fn accepts_a_percent_encoded_code() {
    let parsed = parse_pairing_url("http://host:3000/console/pair#v=1&server=s&code=a%2Bb")
        .expect("valid pairing URL");
    assert_eq!(parsed.code, "a+b");
}

#[test]
fn rejects_malformed_urls() {
    for bad in [
        "",
        "   ",
        // No fragment at all: nothing to redeem.
        "http://host:3000/console/pair",
        // A scheme we do not speak.
        "ftp://host/console/pair#v=1&server=s&code=c",
        // No host.
        "http:///console/pair#v=1&server=s&code=c",
        // A version this client does not know.
        "http://host/console/pair#v=2&server=s&code=c",
        // Missing pieces.
        "http://host/console/pair#v=1&code=",
        "http://host/console/pair#v=1&server=s",
    ] {
        assert!(
            parse_pairing_url(bad).is_err(),
            "should have been rejected: {bad}"
        );
    }
}

#[test]
fn accepts_an_address_with_no_scheme() {
    // The one-liner a user pastes: no scheme, just the address and the
    // code.
    let parsed = parse_pairing_url("192.168.1.10:3000#vw9g-zu89-aphc").expect("valid pairing URL");
    assert_eq!(parsed.base_url, "http://192.168.1.10:3000");
    assert_eq!(parsed.code, "vw9g-zu89-aphc");
}

#[test]
fn names_a_server_by_its_address() {
    // A server that has not named itself is still worth calling
    // something.
    assert_eq!(host_port("http://192.168.1.10:3000"), "192.168.1.10:3000");
    assert_eq!(host_port("192.168.1.10:3000"), "192.168.1.10:3000");
    assert_eq!(host_port("http://server.example/path"), "server.example");
    assert_eq!(host_port("  "), "");
}

// A scheme-less address has its scheme guessed by where it points: a
// public DNS name only speaks https — a tunnel, most often — while a
// ported or local address is still http.
#[test]
fn a_schemeless_public_address_is_https() {
    let parsed =
        parse_pairing_url("k7m2xq9pa4dn.opentunnel.xyz#vw9g-zu89-aphc").expect("valid pairing URL");
    assert_eq!(parsed.base_url, "https://k7m2xq9pa4dn.opentunnel.xyz");
    // A bare machine name on the LAN is still spoken to in http.
    let parsed = parse_pairing_url("dell-xps:3000#abc").expect("valid pairing URL");
    assert_eq!(parsed.base_url, "http://dell-xps:3000");
}

// The address a host field round-trips keeps an https scheme: stripping it
// would send the address back over http, and a public endpoint answers
// that with a not-found page.
#[test]
fn an_https_address_keeps_its_scheme_when_displayed() {
    assert_eq!(
        host_port("https://k7m2xq9pa4dn.opentunnel.xyz"),
        "https://k7m2xq9pa4dn.opentunnel.xyz"
    );
    // http stays the shorthanded default form.
    assert_eq!(host_port("http://server.example"), "server.example");
}

// The host field assumes http when it carries no scheme, so the scheme is
// only dropped when it is the default one. An https address that loses its
// scheme is spoken to in the wrong protocol.
#[test]
fn a_split_line_keeps_an_https_scheme() {
    let (address, code) = split_pairing_line("https://k7m2xq9pa4dn.opentunnel.xyz#vw9g-zu89-aphc");
    assert_eq!(address, "https://k7m2xq9pa4dn.opentunnel.xyz");
    assert_eq!(code, "vw9g-zu89-aphc");
}

#[test]
fn a_split_line_keeps_an_https_scheme_from_a_full_url() {
    let (address, code) = split_pairing_line(
        "https://k7m2xq9pa4dn.opentunnel.xyz/console/pair#v=1&server=481e2796&code=vw9g-zu89-aphc",
    );
    assert_eq!(address, "https://k7m2xq9pa4dn.opentunnel.xyz");
    assert_eq!(code, "vw9g-zu89-aphc");
}

#[test]
fn a_split_line_still_drops_the_default_scheme() {
    // http is what the host field assumes, so the short form is unchanged.
    let (address, code) =
        split_pairing_line("http://192.168.1.10:3000/console/pair#v=1&server=s&code=abc");
    assert_eq!(address, "192.168.1.10:3000");
    assert_eq!(code, "abc");
}

#[test]
fn the_scheme_is_case_insensitive_and_normalised() {
    // `split_pairing_line` already treated `HTTPS://` as https; the full
    // parser used to reject it, so one paste got two answers.
    let parsed = parse_pairing_url("HTTPS://Server.Example/console/pair#v=1&server=s&code=c")
        .expect("uppercase scheme is still a valid pairing URL");
    assert_eq!(parsed.base_url, "https://Server.Example");
    let parsed = parse_pairing_url("Http://host:3000#abc").expect("mixed case");
    assert_eq!(parsed.base_url, "http://host:3000");
    // Case-insensitivity must not widen what is accepted.
    assert!(parse_pairing_url("FTP://host#abc").is_err());
}

// A server behind a path-based relay lives under a prefix, and the pairing URL
// carries it before /console/pair. Dropping it sent every request to the
// relay's root.
#[test]
fn a_relay_path_prefix_is_kept() {
    let parsed = parse_pairing_url(
        "https://relay.example.workers.dev/peter-dev/console/pair#v=1&server=s&code=c",
    )
    .expect("valid pairing URL");
    assert_eq!(parsed.base_url, "https://relay.example.workers.dev/peter-dev");

    let (address, code) = split_pairing_line(
        "https://relay.example.workers.dev/peter-dev/console/pair#v=1&server=s&code=abc",
    );
    assert_eq!(address, "https://relay.example.workers.dev/peter-dev");
    assert_eq!(code, "abc");
}

#[test]
fn a_prefix_with_a_trailing_slash_is_tidied() {
    let parsed = parse_pairing_url("https://r.example/peter-dev//console/pair/#v=1&server=s&code=c")
        .expect("valid pairing URL");
    assert_eq!(parsed.base_url, "https://r.example/peter-dev");
}

#[test]
fn an_unrelated_path_is_still_dropped() {
    let parsed = parse_pairing_url("http://host:3000/some/path#v=1&server=s&code=c")
        .expect("valid pairing URL");
    assert_eq!(parsed.base_url, "http://host:3000");
}
