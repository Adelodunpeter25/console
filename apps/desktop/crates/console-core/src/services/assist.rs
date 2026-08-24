use crate::types::*;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct AssistService {
    transport: HttpTransport,
}

impl AssistService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn list_commands(&self, session_id: Option<&str>) -> Result<Vec<SlashCommandInfo>> {
        let path = match session_id {
            Some(id) if !id.is_empty() => format!("/api/assist/{}/commands", id),
            _ => "/api/assist/commands".to_string(),
        };
        let url = self.transport.url(&path).await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list slash commands")?;

        let body: ApiResponse<Vec<SlashCommandInfo>> = resp
            .json()
            .await
            .context("Failed to parse commands response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to list slash commands".into())
            ))
        }
    }

    pub async fn search_files(
        &self,
        session_id: Option<&str>,
        query: &str,
        root: Option<&str>,
    ) -> Result<FileSearchResponse> {
        let path = match session_id {
            Some(id) if !id.is_empty() => format!("/api/assist/{}/search", id),
            _ => "/api/assist/search".to_string(),
        };
        let mut url = self.transport.url(&path).await;
        let mut params = Vec::new();
        params.push(format!("q={}", urlencoding::encode(query)));
        if let Some(r) = root {
            params.push(format!("root={}", urlencoding::encode(r)));
        }
        url.push('?');
        url.push_str(&params.join("&"));

        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to perform file search")?;

        let body: ApiResponse<FileSearchResponse> = resp
            .json()
            .await
            .context("Failed to parse file search response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("File search data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to perform file search".into())
            ))
        }
    }
}
