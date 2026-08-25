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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_auth_status_payload() {
        let raw = r#"{
            "gemini": {"loggedIn": true, "email": "a@b.c", "projectId": "p1", "configuredProjectId": "p2"},
            "antigravity": {"loggedIn": false},
            "codex": {"loggedIn": false},
            "codebuff": {"loggedIn": true, "email": "x@y.z"}
        }"#;
        let status: AuthStatusResponse = serde_json::from_str(raw).expect("auth status");
        assert!(status.gemini.logged_in);
        assert_eq!(status.gemini.email.as_deref(), Some("a@b.c"));
        assert_eq!(status.gemini.project_id.as_deref(), Some("p1"));
        assert_eq!(status.gemini.configured_project_id.as_deref(), Some("p2"));
        assert!(!status.antigravity.logged_in);
        assert!(status.codebuff.is_some());
    }

    #[test]
    fn parses_auth_status_without_codebuff() {
        let raw = r#"{
            "gemini": {"loggedIn": false},
            "antigravity": {"loggedIn": false},
            "codex": {"loggedIn": false}
        }"#;
        let status: AuthStatusResponse = serde_json::from_str(raw).expect("auth status");
        assert!(status.codebuff.is_none());
    }

    #[test]
    fn parses_login_url_and_codebuff_payloads() {
        let url: OAuthLoginUrlResponse = serde_json::from_str(
            r#"{"authUrl": "https://accounts.google/x", "state": "s1", "redirectUri": "http://localhost:PORT/callback"}"#,
        )
        .expect("login url");
        assert_eq!(url.state, "s1");

        let start: CodebuffLoginStart = serde_json::from_str(
            r#"{"provider": "codebuff", "loginUrl": "https://codebuff/login", "fingerprintId": "f1", "fingerprintHash": "h1", "expiresAt": 1730000000000}"#,
        )
        .expect("codebuff start");
        assert_eq!(start.fingerprint_id, "f1");

        let poll: CodebuffLoginPoll =
            serde_json::from_str(r#"{"completed": false}"#).expect("codebuff poll");
        assert!(!poll.completed);
    }
}

#[cfg(test)]
mod epoch_tests {
    use super::*;

    #[test]
    fn accepts_numeric_and_string_expires_at() {
        let from_number: CodebuffLoginStart = serde_json::from_str(
            r#"{"provider":"codebuff","loginUrl":"u","fingerprintId":"f","fingerprintHash":"h","expiresAt":1730000000000}"#,
        )
        .expect("numeric");
        assert_eq!(from_number.expires_at, 1_730_000_000_000);

        let from_string: CodebuffLoginStart = serde_json::from_str(
            r#"{"provider":"codebuff","loginUrl":"u","fingerprintId":"f","fingerprintHash":"h","expiresAt":"1730000000000"}"#,
        )
        .expect("string");
        assert_eq!(from_string.expires_at, 1_730_000_000_000);
    }
}
