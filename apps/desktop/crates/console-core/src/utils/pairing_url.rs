//! The pairing URL: the one string a user copies and pastes to connect.
//!
//! Shape (produced by `console pair` / `console status`):
//!
//! ```text
//! http://server.example:3000/console/pair#v=1&server=<server-id>&grant=<one-time-grant>
//! ```
//!
//! The secret lives in the fragment, which a browser never sends to the
//! server, so the grant does not end up in access logs. The base URL is
//! part of the same string, so pasting it is enough: the client learns
//! both which server to talk to and how to prove it may.
//!
//! The parser is strict on purpose. The grant is a credential, and a
//! lenient parser is how a malformed paste becomes a pairing with the
//! wrong machine.

/// The parts of a pairing URL.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct PairingUrl {
    /// The server's base URL: scheme, host and port.
    pub base_url: String,
    /// The server the URL was minted by.
    pub server_id: String,
    /// The one-time pairing code.
    pub code: String,
}

/// Parse a pairing URL into its parts.
///
/// Rejects anything it cannot read exactly, rather than repairing it.
///
/// Three shapes are accepted, and they all mean the same thing:
/// - "http://host:3000/console/pair#v=1&server=..&code=abc" — the full URL
/// - "192.168.1.10:3000#abc" — an address and a code
/// - "192.168.1.10:3000" — an address, no code
///
/// The rule is one line: everything after the "#" is the code, everything
/// before it is the address.
pub fn parse_pairing_url(raw: &str) -> Result<PairingUrl, String> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return Err("The pairing URL is empty.".to_string());
    }
    let (base, fragment) = trimmed
        .split_once('#')
        .ok_or_else(|| "That is not a pairing URL — it has no pairing code.".to_string())?;

    // The address, with or without a scheme. A scheme is only assumed when
    // the text has none at all, so "192.168.1.10:3000" and
    // "http://192.168.1.10:3000" both work.
    let (scheme, authority) = match base.split_once("://") {
        Some((scheme, rest)) => (scheme.to_string(), rest.to_string()),
        None => ("http".to_string(), base.to_string()),
    };
    if scheme != "http" && scheme != "https" {
        return Err("A pairing URL must start with http:// or https://.".to_string());
    }
    // The path is /console/pair; the address is just the authority.
    let host = authority.split('/').next().unwrap_or_default().trim();
    if host.is_empty() {
        return Err("A pairing URL must name a server.".to_string());
    }

    let mut server_id = String::new();
    let mut code = String::new();
    let mut version = String::new();
    let mut query = false;
    for pair in fragment.split('&') {
        let Some((key, value)) = pair.split_once('=') else {
            continue;
        };
        query = true;
        let value = percent_decode(value);
        match key {
            "v" => version = value,
            "server" => server_id = value,
            "code" => code = value,
            _ => {}
        }
    }
    if query {
        // The full URL carries a version, so an unknown one is refused
        // rather than guessed at.
        if version != "1" {
            return Err("That pairing URL was made by a newer version of Console.".to_string());
        }
        if code.is_empty() {
            return Err("That pairing URL has no pairing code.".to_string());
        }
    } else {
        // A bare fragment is the code itself.
        code = fragment.to_string();
        if code.is_empty() {
            return Err("That pairing URL has no pairing code.".to_string());
        }
    }
    Ok(PairingUrl {
        base_url: format!("{scheme}://{host}"),
        server_id,
        code,
    })
}

/// Split a pasted line into an address and a pairing code.
///
/// Three shapes are accepted, and they all mean the same thing:
/// - "192.168.1.100:8080" — an address, no code
/// - "192.168.1.100:8080#abc-123-xyz" — an address and a code
/// - "http://192.168.1.100:8080/console/pair#v=1&server=..&code=abc" — the whole URL
///
/// The rule is one line: everything after the "#" is the code, everything
/// before it is the address. The code lives in the fragment because a
/// browser never sends a fragment to the server, so it stays out of access
/// logs.
pub fn split_pairing_line(text: &str) -> (String, String) {
    let trimmed = text.trim();
    let Some((base, fragment)) = trimmed.split_once('#') else {
        return (trimmed.to_string(), String::new());
    };
    let address = match base.split_once("://") {
        Some((_, rest)) => rest.split('/').next().unwrap_or(rest).trim(),
        None => base.trim(),
    };
    // A bare code, or a query string the full URL carries.
    if !fragment.contains('=') {
        return (address.to_string(), fragment.to_string());
    }
    let code = fragment
        .split('&')
        .filter_map(|pair| pair.split_once('='))
        .find(|(key, _)| *key == "code")
        .map(|(_, value)| value.to_string())
        .unwrap_or_default();
    (address.to_string(), code)
}

/// Percent-decode a fragment value.
///
/// Only the escapes that can appear in a URL-encoded query value are
/// handled; anything else is left as-is rather than guessed at.
fn percent_decode(value: &str) -> String {
    let bytes = value.as_bytes();
    let mut out = Vec::with_capacity(bytes.len());
    let mut i = 0;
    while i < bytes.len() {
        match bytes[i] {
            b'%' if i + 2 < bytes.len() => {
                let hex = std::str::from_utf8(&bytes[i + 1..i + 3]).unwrap_or("");
                match u8::from_str_radix(hex, 16) {
                    Ok(byte) => {
                        out.push(byte);
                        i += 3;
                    }
                    Err(_) => {
                        out.push(bytes[i]);
                        i += 1;
                    }
                }
            }
            b'+' => {
                out.push(b' ');
                i += 1;
            }
            byte => {
                out.push(byte);
                i += 1;
            }
        }
    }
    String::from_utf8_lossy(&out).into_owned()
}
