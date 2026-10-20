//! Client for the server's `/api/auth/*` routes: provider login status,
//! OAuth (URL + code exchange), and the Antigravity Cloud project id setting.

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

        let body: ApiResponse<AuthStatusResponse> = self
            .transport
            .decode_json(resp)
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
    pub async fn login_url(&self, provider: OAuthProviderId) -> Result<OAuthLoginUrlResponse> {
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

        let body: ApiResponse<OAuthLoginUrlResponse> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse login URL response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Login URL response missing data"))
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

        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse OAuth callback response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error.unwrap_or_else(|| "OAuth callback failed".into())
            ))
        }
    }

    // --- GitHub git credentials -------------------------------------------
    //
    // GitHub is NOT an OAuth chat provider. It exists so the git CLI can reach
    // private repositories from the server host, so it authenticates with a
    // pasted personal access token rather than a browser flow. That matters for
    // remote backends: the OAuth login below finishes on a loopback listener on
    // *this* machine, whereas the PAT is POSTed to the server and works from
    // anywhere.
    //
    // The token is sent once and never stored client-side — the server keeps it
    // in <storage>/github-creds.json (mirrors providers/shared.CredentialPath
    // permissions: dir 0700 / file 0600).

    /// `GET /api/auth/github/status` — whether a usable token is stored, plus
    /// the cached username and scopes.
    pub async fn github_status(&self) -> Result<GitHubAuthStatus> {
        let url = self.transport.url("/api/auth/github/status").await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch GitHub status")?;

        let body: ApiResponse<GitHubAuthStatus> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse GitHub status response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to fetch GitHub status".into())
            ))
        }
    }

    /// `POST /api/auth/github/pat` — validate a personal access token and, if
    /// good, store it server-side. The token is never echoed back.
    ///
    /// The server distinguishes the two failure modes we care about: 400 for a
    /// token GitHub rejected (fix the input) and 502 for GitHub being
    /// unreachable (retry later). Both arrive here as an `Err`, so the UI shows
    /// the server's wording verbatim rather than inventing one.
    pub async fn connect_github(&self, token: &str) -> Result<GitHubAuthStatus> {
        let url = self.transport.url("/api/auth/github/pat").await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&serde_json::json!({ "token": token }))
            .send()
            .await
            .context("Failed to connect GitHub")?;

        let body: ApiResponse<GitHubAuthStatus> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse connect GitHub response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to connect GitHub".into())
            ))
        }
    }

    /// `GET /api/providers/credentials` — which providers have an API key
    /// stored, keyed by provider id.
    ///
    /// Returns only *whether* a key exists and where it came from. The key
    /// itself is stored on the server and no endpoint will return it, so there
    /// is nothing here to accidentally log or cache.
    pub async fn provider_credentials(&self) -> Result<ProviderCredentialStatuses> {
        let url = self.transport.url("/api/providers/credentials").await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to load provider credentials")?;

        let body: ApiResponse<ProviderCredentialStatuses> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse provider credentials response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to load provider credentials".into())
            ))
        }
    }

    /// `PUT /api/providers/:id/credential` — store a pasted API key on the
    /// server. The key is sent once and never returned.
    pub async fn set_provider_credential(
        &self,
        provider: &str,
        api_key: &str,
    ) -> Result<SetProviderCredentialResponse> {
        let url = self
            .transport
            .url(&format!("/api/providers/{provider}/credential"))
            .await;
        let resp = self
            .transport
            .client()
            .put(&url)
            .headers(self.transport.build_headers().await)
            .json(&serde_json::json!({ "apiKey": api_key }))
            .send()
            .await
            .context("Failed to save API key")?;

        let body: ApiResponse<SetProviderCredentialResponse> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse set credential response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to save API key".into())
            ))
        }
    }

    /// `DELETE /api/providers/:id/credential` — remove a stored key, falling
    /// back to whatever the provider did before. Idempotent.
    ///
    /// This cannot remove a key supplied by an environment variable, which is
    /// why the status payload carries a `source`: the UI hides the remove
    /// action rather than offering one that silently does nothing.
    pub async fn clear_provider_credential(
        &self,
        provider: &str,
    ) -> Result<SetProviderCredentialResponse> {
        let url = self
            .transport
            .url(&format!("/api/providers/{provider}/credential"))
            .await;
        let resp = self
            .transport
            .client()
            .delete(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to remove API key")?;

        let body: ApiResponse<SetProviderCredentialResponse> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse clear credential response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to remove API key".into())
            ))
        }
    }

    // There is no `POST /api/auth/token` client call: the server issues device
    // credentials only through a consumed pairing grant (see
    // `services::server_auth::ServerAuthService`).

    /// `POST /api/auth/github/logout` — remove the stored token. Idempotent.
    pub async fn disconnect_github(&self) -> Result<()> {
        let url = self.transport.url("/api/auth/github/logout").await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to disconnect GitHub")?;

        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse disconnect GitHub response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to disconnect GitHub".into())
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

        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(resp)
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
    pub async fn get_project_id(&self, provider: OAuthProviderId) -> Result<Option<String>> {
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

        let body: ApiResponse<ProjectIdPayload> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse project id response")?;
        if body.success {
            Ok(body
                .data
                .and_then(|d| d.project_id)
                .filter(|id| !id.is_empty()))
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
