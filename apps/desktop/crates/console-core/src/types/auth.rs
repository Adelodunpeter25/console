//! Auth DTOs mirroring the server's `/api/auth/*` contracts and the shared
//! `@console/types` definitions (`packages/types/src/api.ts`, `model.ts`).

use serde::{Deserialize, Serialize};

/// OAuth-capable providers with credential state (`OAuthProviderId`).
#[derive(Clone, Copy, Debug, Serialize, Deserialize, PartialEq, Eq)]
pub enum OAuthProviderId {
    Antigravity,
    Codex,
}

impl OAuthProviderId {
    pub fn as_str(&self) -> &'static str {
        match self {
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

/// Response of `GET /api/auth/status`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct AuthStatusResponse {
    pub antigravity: ProviderAuthStatus,
    pub codex: ProviderAuthStatus,
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
