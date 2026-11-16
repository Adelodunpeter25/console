//! Client for the server's device-pairing routes (`/api/auth/pair`,
//! `/api/auth/pairing-grants`, `/api/auth/devices`).
//!
//! These authenticate *this app* to a Console server, as opposed to
//! `services::auth` which authenticates the server to an AI provider. The
//! payloads are the shared protobuf types (console.v1, see
//! proto/console/v1/auth.proto), so they are decoded with the same
//! protojson-named serde impls the rest of the client uses.

use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

pub use console_proto::{
    PairDeviceRequest as PairingRequest, PairDeviceResponse as PairingData,
    PairedDevice as DeviceInfo, PairingGrantRequest, PairingGrantResponse as PairingGrantData,
    ServerDevice, ServerDevicesResponse,
};

#[derive(Clone)]
pub struct ServerAuthService {
    transport: HttpTransport,
}

impl ServerAuthService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    /// Exchange a one-time pairing grant for this device's credential.
    pub async fn pair(&self, request: PairingRequest) -> Result<PairingData> {
        let response = self
            .transport
            .client()
            .post(&self.transport.url("/api/auth/pair").await)
            .headers(self.transport.build_headers().await)
            .json(&request)
            .send()
            .await
            .context("Failed to pair with server")?;
        let data = decode_data(response)
            .await
            .context("Failed to pair with server")?;
        serde_json::from_value(data).context("Failed to parse pairing response")
    }

    /// Pair with a server using a host and a pairing code.
    ///
    /// The host and the code are what the server prints: `console pair`
    /// shows a host and a short code, and the code is redeemed for this
    /// device's own credential.
    pub async fn pair_with_code(
        &self,
        host: &str,
        code: &str,
        device_name: &str,
    ) -> Result<(PairingData, String)> {
        let base_url = normalize_server_url(host);
        // An iroh address is dialled through a tunnel; the address itself
        // is what gets stored for later requests.
        let tunnel = match console_shared::pairing::iroh_node_id(&base_url) {
            Some(node_id) => Some(
                crate::utils::http_transport::get_or_create_tunnel(node_id)
                    .await
                    .context("Failed to reach the server over iroh")?,
            ),
            None => None,
        };
        let request_base = tunnel.as_ref().map_or(base_url.clone(), |t| t.base_url());
        let response = self
            .transport
            .client()
            .post(format!("{request_base}/api/auth/pair"))
            .headers(self.transport.build_headers().await)
            .json(&PairingRequest {
                grant: code.to_string(),
                device_name: device_name.to_string(),
                platform: "desktop".to_string(),
            })
            .send()
            .await
            .context("Failed to pair with server")?;
        let data = decode_data(response)
            .await
            .context("Failed to pair with server")?;
        let paired: PairingData =
            serde_json::from_value(data).context("Failed to parse pairing response")?;
        Ok((paired, base_url))
    }

    /// Create a short-lived, single-use grant another device can redeem.
    pub async fn create_pairing_grant(
        &self,
        request: PairingGrantRequest,
    ) -> Result<PairingGrantData> {
        let response = self
            .transport
            .client()
            .post(&self.transport.url("/api/auth/pairing-grants").await)
            .headers(self.transport.build_headers().await)
            .json(&request)
            .send()
            .await
            .context("Failed to create pairing grant")?;
        let data = decode_data(response)
            .await
            .context("Failed to create pairing grant")?;
        serde_json::from_value(data).context("Failed to parse pairing grant response")
    }

    /// Put a freshly issued device token on the shared transport, so every
    /// later request is authenticated.
    pub async fn apply_pairing(&self, data: &PairingData) -> Result<()> {
        self.transport
            .set_auth_token(Some(data.token.clone()))
            .await;
        Ok(())
    }
}

/// Normalize a host into a base URL.
///
/// The host field takes "192.168.1.10:3000", the same shape the phone
/// uses. A scheme is assumed when the text has none, and which one follows
/// the shared `default_scheme` rule: a local address is http, a public DNS
/// name (the tunnel a QR names) is https, because asking it over http is
/// answered with a not-found page rather than an error a user can act on.
fn normalize_server_url(host: &str) -> String {
    let trimmed = host.trim().trim_end_matches('/');
    if trimmed.is_empty() {
        return "http://localhost:3000".to_string();
    }
    if trimmed.starts_with("http://")
        || trimmed.starts_with("https://")
        || trimmed.starts_with("console-iroh://")
    {
        return trimmed.to_string();
    }
    format!(
        "{}://{trimmed}",
        console_shared::pairing::default_scheme(trimmed)
    )
}

/// Unwrap the `{success, data}` envelope and hand back `data` as raw JSON.
///
/// The server answers a failure with `{"success": false, "error": …}` and no
/// `data`, so the error is surfaced verbatim rather than as a decode failure.
async fn decode_data(response: reqwest::Response) -> Result<serde_json::Value> {
    let body: serde_json::Value = serde_json::from_slice(
        &response
            .bytes()
            .await
            .context("Failed to read response body")?,
    )
    .context("Failed to decode response body")?;
    if !body
        .get("success")
        .and_then(|v| v.as_bool())
        .unwrap_or(false)
    {
        return Err(anyhow!(
            body.get("error")
                .and_then(|v| v.as_str())
                .unwrap_or("The server rejected the request")
                .to_string()
        ));
    }
    body.get("data")
        .cloned()
        .ok_or_else(|| anyhow!("The server response carried no data"))
}
