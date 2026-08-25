//! Client for the server's `/api/auth/*` routes: provider login status,
//! OAuth (URL + code exchange), Codebuff device-code login, and the Gemini
//! Cloud project id setting.

use crate::types::*;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};
use serde::Deserialize;

#[derive(Clone)]
pub struct AuthService {
    transport: HttpTransport,
}

impl AuthService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    /// `GET /api/auth/status` — per-provider credential status.
    pub async fn status(&self) -> Result<AuthStatusResponse> {
        let url = self.transport.url("/api/auth/status").await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch auth status")?;

        let body: ApiResponse<AuthStatusResponse> = resp
            .json()
            .await
            .context("Failed to parse auth status response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to fetch auth status".into())
            ))
        }
    }

    /// `POST /api/auth/login/url` — build the browser OAuth URL for a
    /// provider, plus the loopback redirect URI to catch the callback on.
    pub async fn login_url(
        &self,
        provider: OAuthProviderId,
    ) -> Result<OAuthLoginUrlResponse> {
        let url = self.transport.url("/api/auth/login/url").await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&serde_json::json!({ "provider": provider.as_str() }))
            .send()
            .await
            .context("Failed to request OAuth login URL")?;

        let body: ApiResponse<OAuthLoginUrlResponse> = resp
            .json()
            .await
            .context("Failed to parse login URL response")?;
        if body.success {
            body.data.ok_or_else(|| anyhow!("Login URL response missing data"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to get login URL".into())
            ))
        }
    }

    /// `POST /api/auth/login/callback` — exchange an authorization code the
    /// client captured on its loopback listener for server-stored tokens.
    pub async fn handle_callback(
        &self,
        provider: OAuthProviderId,
        code: &str,
        state: Option<&str>,
    ) -> Result<()> {
        let url = self.transport.url("/api/auth/login/callback").await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&serde_json::json!({
                "provider": provider.as_str(),
                "code": code,
                "state": state,
            }))
            .send()
            .await
            .context("Failed to send OAuth callback")?;

        let body: ApiResponse<serde_json::Value> = resp
            .json()
            .await
            .context("Failed to parse OAuth callback response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "OAuth callback failed".into())
            ))
        }
    }

    /// `POST /api/auth/codebuff/start` — device-code login: returns the URL to
    /// open in a browser plus fingerprint params used to poll completion.
    pub async fn codebuff_start(&self) -> Result<CodebuffLoginStart> {
        let url = self.transport.url("/api/auth/codebuff/start").await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to start codebuff login")?;

        let body: ApiResponse<CodebuffLoginStart> = resp
            .json()
            .await
            .context("Failed to parse codebuff start response")?;
        if body.success {
            body.data.ok_or_else(|| anyhow!("Codebuff start response missing data"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to start codebuff login".into())
            ))
        }
    }

    /// `GET /api/auth/codebuff/status` — poll whether the device-code login
    /// was approved. Transient errors are returned; callers retry until their
    /// own deadline.
    pub async fn codebuff_poll(
        &self,
        fingerprint_id: &str,
        fingerprint_hash: &str,
        expires_at: &str,
    ) -> Result<CodebuffLoginPoll> {
        let base = self.transport.url("/api/auth/codebuff/status").await;
        let url = format!(
            "{}?fingerprintId={}&fingerprintHash={}&expiresAt={}",
            base,
            urlencoding::encode(fingerprint_id),
            urlencoding::encode(fingerprint_hash),
            urlencoding::encode(expires_at),
        );
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to poll codebuff login")?;

        let body: ApiResponse<CodebuffLoginPoll> = resp
            .json()
            .await
            .context("Failed to parse codebuff poll response")?;
        if body.success {
            body.data.ok_or_else(|| anyhow!("Codebuff poll response missing data"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to poll codebuff login".into())
            ))
        }
    }

    /// `POST /api/auth/project-id` — persist the Google Cloud project id for a
    /// provider. Pass `None` to clear it.
    pub async fn save_project_id(
        &self,
        provider: OAuthProviderId,
        project_id: Option<&str>,
    ) -> Result<()> {
        let url = self.transport.url("/api/auth/project-id").await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&serde_json::json!({
                "provider": provider.as_str(),
                "projectId": project_id,
            }))
            .send()
            .await
            .context("Failed to save project id")?;

        let body: ApiResponse<serde_json::Value> = resp
            .json()
            .await
            .context("Failed to parse save project id response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to save project id".into())
            ))
        }
    }

    /// `GET /api/auth/project-id/:provider` — read the configured project id.
    pub async fn get_project_id(
        &self,
        provider: OAuthProviderId,
    ) -> Result<Option<String>> {
        let url = self
            .transport
            .url(&format!("/api/auth/project-id/{}", provider.as_str()))
            .await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch project id")?;

        let body: ApiResponse<ProjectIdPayload> = resp
            .json()
            .await
            .context("Failed to parse project id response")?;
        if body.success {
            Ok(body.data.and_then(|d| d.project_id).filter(|id| !id.is_empty()))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to fetch project id".into())
            ))
        }
    }
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ProjectIdPayload {
    #[serde(default)]
    project_id: Option<String>,
}
