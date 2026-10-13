//! Client for the server's `/api/integrations` route: which helper binaries
//! the box has installed, so the UI can say "install gh" instead of letting
//! the agent die on `command not found` mid-turn.

use crate::types::*;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct IntegrationService {
    transport: HttpTransport,
}

impl IntegrationService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    /// `GET /api/integrations` — status rows for every known binary.
    /// Detection runs live server-side on every call, so a refresh after
    /// installing picks it up with no restart.
    pub async fn list(&self) -> Result<Vec<IntegrationStatus>> {
        let url = self.transport.url("/api/integrations").await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch integrations")?;

        let body: ApiResponse<IntegrationsResponse> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse integrations response")?;
        if body.success {
            Ok(body.data.unwrap_or_default().integrations)
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to fetch integrations".into())
            ))
        }
    }

    /// `POST /api/integrations/gh/install` — install gh the official way for
    /// the server's platform (brew on macOS, release tarball into
    /// ~/.local/bin on Linux). Only offered when no `gh` resolves; the call
    /// itself no-ops to current status when one does.
    ///
    /// Answers the full row list, so callers just replace state. Takes a
    /// while (download + unpack) — the caller should show busy until it
    /// resolves.
    pub async fn install_gh(&self) -> Result<Vec<IntegrationStatus>> {
        let url = self.transport.url("/api/integrations/gh/install").await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to install GitHub CLI")?;

        let body: ApiResponse<IntegrationsResponse> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse GitHub CLI install response")?;
        if body.success {
            Ok(body.data.unwrap_or_default().integrations)
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to install GitHub CLI".into())
            ))
        }
    }
}
