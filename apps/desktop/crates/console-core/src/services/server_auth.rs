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
    PairingGrantRequest, PairingGrantResponse as PairingGrantData,
    PairedDevice as DeviceInfo, ServerDevice, ServerDevicesResponse,
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
        let data = decode_data(response).await.context("Failed to pair with server")?;
        serde_json::from_value(data).context("Failed to parse pairing response")
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
        let data = decode_data(response).await.context("Failed to create pairing grant")?;
        serde_json::from_value(data).context("Failed to parse pairing grant response")
    }

    /// The devices paired with this server.
    pub async fn devices(&self) -> Result<Vec<ServerDevice>> {
        let response = self
            .transport
            .client()
            .get(&self.transport.url("/api/auth/devices").await)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list devices")?;
        let data = decode_data(response).await.context("Failed to list devices")?;
        let list: ServerDevicesResponse =
            serde_json::from_value(data).context("Failed to parse devices")?;
        Ok(list.devices)
    }

    /// Revoke a paired device. Idempotent on the server.
    pub async fn revoke_device(&self, id: &str) -> Result<()> {
        let response = self
            .transport
            .client()
            .delete(&self.transport.url(&format!("/api/auth/devices/{id}")).await)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to revoke device")?;
        if !response.status().is_success() {
            return Err(anyhow!("Failed to revoke device"));
        }
        Ok(())
    }

    /// Put a freshly issued device token on the shared transport, so every
    /// later request — and the WebTransport handshake — is authenticated.
    pub async fn apply_pairing(&self, data: &PairingData) -> Result<()> {
        self.transport
            .set_auth_token(Some(data.token.clone()))
            .await;
        Ok(())
    }
}

/// Unwrap the `{success, data}` envelope and hand back `data` as raw JSON.
///
/// The server answers a failure with `{"success": false, "error": …}` and no
/// `data`, so the error is surfaced verbatim rather than as a decode failure.
async fn decode_data(response: reqwest::Response) -> Result<serde_json::Value> {
    let body: serde_json::Value = serde_json::from_slice(&response.bytes().await.context("Failed to read response body")?)
        .context("Failed to decode response body")?;
    if !body.get("success").and_then(|v| v.as_bool()).unwrap_or(false) {
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
