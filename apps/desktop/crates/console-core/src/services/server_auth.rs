use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PairingRequest<'a> { pub grant: &'a str, pub device_name: &'a str, pub platform: &'a str }

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PairingData { pub device: DeviceInfo, pub token: String, pub expires_at: i64 }

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DeviceInfo { pub id: String, pub name: String, pub platform: String, pub created_at: Option<i64>, pub last_seen_at: Option<i64>, pub revoked_at: Option<i64> }

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PairingGrantRequest<'a> { pub device_name: &'a str, pub platform: &'a str }

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PairingGrantData { pub grant: String, pub expires_at: i64 }

#[derive(Clone)]
pub struct ServerAuthService { transport: HttpTransport }

impl ServerAuthService {
    pub async fn apply_pairing(&self, data: &PairingData) -> Result<()> {
        self.transport.set_auth_token(Some(data.token.clone())).await;
        Ok(())
    }
}



impl ServerAuthService {
    pub fn new(transport: HttpTransport) -> Self { Self { transport } }

    pub async fn pair(&self, request: PairingRequest<'_>) -> Result<PairingData> {
        let response = self.transport.client().post(&self.transport.url("/api/auth/pair").await).headers(self.transport.build_headers().await).json(&request).send().await.context("Failed to pair with server")?;
        let body: serde_json::Value = self.transport.decode_json(response).await?;
        if !body.get("success").and_then(|v| v.as_bool()).unwrap_or(false) { return Err(anyhow!(body.get("error").and_then(|v| v.as_str()).unwrap_or("Pairing failed").to_string())); }
        serde_json::from_value(body.get("data").cloned().ok_or_else(|| anyhow!("Pairing response missing data"))?).context("Failed to parse pairing response")
    }

    pub async fn create_pairing_grant(&self, request: PairingGrantRequest<'_>) -> Result<PairingGrantData> {
        let response = self.transport.client().post(&self.transport.url("/api/auth/pairing-grants").await).headers(self.transport.build_headers().await).json(&request).send().await.context("Failed to create pairing grant")?;
        let body: serde_json::Value = self.transport.decode_json(response).await?;
        if !body.get("success").and_then(|v| v.as_bool()).unwrap_or(false) { return Err(anyhow!(body.get("error").and_then(|v| v.as_str()).unwrap_or("Pairing grant failed").to_string())); }
        serde_json::from_value(body.get("data").cloned().ok_or_else(|| anyhow!("Pairing grant response missing data"))?).context("Failed to parse pairing grant response")
    }

    pub async fn devices(&self) -> Result<Vec<DeviceInfo>> {
        let response = self.transport.client().get(&self.transport.url("/api/auth/devices").await).headers(self.transport.build_headers().await).send().await.context("Failed to list devices")?;
        let body: serde_json::Value = self.transport.decode_json(response).await?;
        if !body.get("success").and_then(|v| v.as_bool()).unwrap_or(false) { return Err(anyhow!("Failed to list devices")); }
        Ok(serde_json::from_value(body["data"]["devices"].clone()).context("Failed to parse devices")?)
    }

    pub async fn revoke_device(&self, id: &str) -> Result<()> {
        let response = self.transport.client().delete(&self.transport.url(&format!("/api/auth/devices/{id}")).await).headers(self.transport.build_headers().await).send().await.context("Failed to revoke device")?;
        if !response.status().is_success() { return Err(anyhow!("Failed to revoke device")); }
        Ok(())
    }
}
