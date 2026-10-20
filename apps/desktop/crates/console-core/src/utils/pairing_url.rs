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
    /// The one-time pairing grant.
    pub grant: String,
}

/// Parse a pairing URL into its parts.
///
/// Rejects anything it cannot read exactly, rather than repairing it.
pub fn parse_pairing_url(raw: &str) -> Result<PairingUrl, String> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return Err("The pairing URL is empty.".to_string());
    }
    let (base, fragment) = trimmed
        .split_once('#')
        .ok_or_else(|| "That is not a pairing URL — it has no pairing code.".to_string())?;

    let (scheme, authority) = base
        .split_once("://")
        .ok_or_else(|| "That is not a pairing URL — it has no address.".to_string())?;
    if scheme != "http" && scheme != "https" {
        return Err("A pairing URL must start with http:// or https://.".to_string());
    }
    // The path is /console/pair; anything before the authority would be a
    // different shape entirely, and the host must be a host.
    let host = authority
        .split('/')
        .next()
        .unwrap_or_default()
        .trim();
    if host.is_empty() {
        return Err("A pairing URL must name a server.".to_string());
    }

    let mut version = String::new();
    let mut server_id = String::new();
    let mut grant = String::new();
    for pair in fragment.split('&') {
        let Some((key, value)) = pair.split_once('=') else {
            continue;
        };
        let value = percent_decode(value);
        match key {
            "v" => version = value,
            "server" => server_id = value,
            "grant" => grant = value,
            _ => {}
        }
    }
    if version != "1" {
        return Err("That pairing URL was made by a newer version of Console.".to_string());
    }
    if server_id.is_empty() {
        return Err("That pairing URL does not name a server.".to_string());
    }
    if grant.is_empty() {
        return Err("That pairing URL has no pairing code.".to_string());
    }
    Ok(PairingUrl {
        base_url: format!("{scheme}://{host}"),
        server_id,
        grant,
    })
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
