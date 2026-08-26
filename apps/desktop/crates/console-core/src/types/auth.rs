//! Auth DTOs mirroring the server's `/api/auth/*` contracts and the shared
//! `@console/types` definitions (`packages/types/src/api.ts`, `model.ts`).

use serde::{Deserialize, Serialize};

/// OAuth-capable providers with credential state (`OAuthProviderId`).
#[derive(Clone, Copy, Debug, Serialize, Deserialize, PartialEq, Eq)]
pub enum OAuthProviderId {
    Gemini,
    Antigravity,
    Codex,
}

impl OAuthProviderId {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Gemini => "gemini",
            Self::Antigravity => "antigravity",
            Self::Codex => "codex",
        }
    }
}

/// Per-provider login status from `GET /api/auth/status`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderAuthStatus {
    pub logged_in: bool,
    pub email: Option<String>,
    /// Project id stored inside the provider credential.
    #[serde(default)]
    pub project_id: Option<String>,
    /// Project id configured via the settings/project-id endpoint.
    #[serde(default)]
    pub configured_project_id: Option<String>,
}

/// Response of `GET /api/auth/status`. The three OAuth keys are always present;
/// `codebuff` may be absent on older servers.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct AuthStatusResponse {
    pub gemini: ProviderAuthStatus,
    pub antigravity: ProviderAuthStatus,
    pub codex: ProviderAuthStatus,
    pub codebuff: Option<ProviderAuthStatus>,
}

/// Response of `POST /api/auth/login/url`.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct OAuthLoginUrlResponse {
    pub auth_url: String,
    pub state: String,
    /** Loopback URI the authorization code will land on. */
    pub redirect_uri: String,
}

/// Response of `POST /api/auth/codebuff/start`.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CodebuffLoginStart {
    pub provider: String,
    pub login_url: String,
    pub fingerprint_id: String,
    pub fingerprint_hash: String,
    /// Epoch-ms per the real API; relayed verbatim by the server, which
    /// accepts both numbers and numeric strings — so do we.
    #[serde(deserialize_with = "deserialize_epoch_ms")]
    pub expires_at: i64,
}

fn deserialize_epoch_ms<'de, D>(deserializer: D) -> Result<i64, D::Error>
where
    D: serde::Deserializer<'de>,
{
    let value = serde_json::Value::deserialize(deserializer)?;
    match value {
        serde_json::Value::Number(n) => Ok(n.as_i64().unwrap_or(0)),
        serde_json::Value::String(s) => s.parse::<i64>().map_err(serde::de::Error::custom),
        _ => Err(serde::de::Error::custom("expiresAt must be epoch ms")),
    }
}

/// Response of `GET /api/auth/codebuff/status?...`.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CodebuffLoginPoll {
    pub completed: bool,
    #[serde(default)]
    pub email: Option<String>,
}
