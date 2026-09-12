use crate::types::*;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct FsService {
    transport: HttpTransport,
}

impl FsService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    /// Browse directories for the ⌘O project palette. Always includes hidden
    /// (dotfile) folders — project roots like `.config/nvim` or `.dotfiles`
    /// must be navigable, and there is intentionally no visibility toggle.
    pub async fn browse(&self, path: Option<&str>) -> Result<BrowseDirectoryResponse> {
        let mut url = self.transport.url("/api/fs/browse").await;
        // Always show hidden: the palette filters locally and must reach dotfolders.
        let mut params = vec!["hidden=true".to_string()];
        if let Some(p) = path {
            params.push(format!("path={}", urlencoding::encode(p)));
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
            .context("Failed to browse directory")?;

        let body: ApiResponse<BrowseDirectoryResponse> = resp
            .json()
            .await
            .context("Failed to parse browse response")?;
        if body.success {
            body.data.ok_or_else(|| anyhow!("Browse data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to browse directory".into())
            ))
        }
    }

    pub async fn get_entries(
        &self,
        path: &str,
        depth: Option<usize>,
        hidden: Option<bool>,
    ) -> Result<Vec<FsTreeEntry>> {
        let mut url = self.transport.url("/api/fs/entries").await;
        let mut params = vec![format!("path={}", urlencoding::encode(path))];
        if let Some(d) = depth {
            params.push(format!("depth={}", d));
        }
        if let Some(h) = hidden {
            params.push(format!("hidden={}", h));
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
            .context("Failed to get fs entries")?;

        let body: ApiResponse<Vec<FsTreeEntry>> = resp
            .json()
            .await
            .context("Failed to parse fs entries response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Fs entries data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to get fs entries".into())
            ))
        }
    }

    pub async fn get_tree(
        &self,
        path: Option<&str>,
        depth: Option<usize>,
    ) -> Result<DirectoryTreeResponse> {
        let mut url = self.transport.url("/api/fs/tree").await;
        let mut params = Vec::new();
        if let Some(p) = path {
            params.push(format!("path={}", urlencoding::encode(p)));
        }
        if let Some(d) = depth {
            params.push(format!("depth={}", d));
        }
        if !params.is_empty() {
            url.push('?');
            url.push_str(&params.join("&"));
        }

        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to get directory tree")?;

        let body: ApiResponse<DirectoryTreeResponse> = resp
            .json()
            .await
            .context("Failed to parse directory tree response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Directory tree data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to get directory tree".into())
            ))
        }
    }

    pub async fn read_file(&self, path: &str) -> Result<FileContentResponse> {
        let url = format!(
            "{}/api/fs/file?path={}",
            self.transport.url("").await,
            urlencoding::encode(path)
        );
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to read file")?;

        let body: ApiResponse<FileContentResponse> =
            resp.json().await.context("Failed to parse file response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("File content data is missing"))
        } else {
            Err(anyhow!(
                body.error.unwrap_or_else(|| "Failed to read file".into())
            ))
        }
    }

    pub async fn write_file(&self, path: &str, content: &str) -> Result<()> {
        let url = self.transport.url("/api/fs/file").await;
        let payload = WriteFileDto {
            path: path.to_string(),
            content: content.to_string(),
        };

        let resp = self
            .transport
            .client()
            .put(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .context("Failed to write file")?;

        let body: ApiResponse<serde_json::Value> = resp
            .json()
            .await
            .context("Failed to parse write response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error.unwrap_or_else(|| "Failed to write file".into())
            ))
        }
    }

    pub async fn watch_events(
        &self,
        path: &str,
    ) -> Result<std::pin::Pin<Box<dyn futures_util::Stream<Item = Result<serde_json::Value>> + Send>>>
    {
        use eventsource_stream::Eventsource;
        use futures_util::StreamExt;

        let url = format!(
            "{}/api/fs/watch?path={}",
            self.transport.url("").await,
            urlencoding::encode(path)
        );
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to connect to fs watch SSE stream")?;

        let stream = resp.bytes_stream().eventsource().filter_map(|item| async {
            match item {
                Ok(event) if event.event == "fsChange" => {
                    serde_json::from_str(&event.data).ok().map(Ok)
                }
                _ => None,
            }
        });

        Ok(Box::pin(stream))
    }

    /// Read raw file bytes from `GET /api/fs/file/raw` for image/SVG preview.
    ///
    /// Returns `(bytes, content_type)`. If the backend blocks the file or errors,
    /// parses the structured JSON error body into `RawFileError` so callers can
    /// inspect the preview block code and details.
    pub async fn read_file_bytes(&self, path: &str) -> Result<(Vec<u8>, String)> {
        let url = format!(
            "{}/api/fs/file/raw?path={}",
            self.transport.url("").await,
            urlencoding::encode(path)
        );
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to read raw file bytes")?;

        let status = resp.status();
        if status.is_success() {
            let content_type = resp
                .headers()
                .get(reqwest::header::CONTENT_TYPE)
                .and_then(|v| v.to_str().ok())
                .unwrap_or("application/octet-stream")
                .to_string();
            let bytes = resp
                .bytes()
                .await
                .context("Failed to read response body bytes")?
                .to_vec();
            Ok((bytes, content_type))
        } else {
            let body_text = resp.text().await.unwrap_or_default();
            if let Ok(err_obj) = serde_json::from_str::<RawFileError>(&body_text) {
                Err(anyhow::Error::new(err_obj))
            } else {
                Err(anyhow!(
                    "Failed to read raw file (status {}): {}",
                    status,
                    body_text
                ))
            }
        }
    }
}

/// Structured error returned by `GET /api/fs/file/raw` when preview is blocked.
#[derive(Debug, Clone, serde::Deserialize, serde::Serialize)]
pub struct RawFileError {
    #[serde(default)]
    pub success: bool,
    pub error: Option<String>,
    pub code: Option<String>,
    #[serde(rename = "sizeBytes")]
    pub size_bytes: Option<u64>,
    #[serde(rename = "maxBytes")]
    pub max_bytes: Option<u64>,
}

impl std::fmt::Display for RawFileError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        if let Some(msg) = &self.error {
            write!(f, "{}", msg)
        } else if let Some(code) = &self.code {
            write!(f, "{}", code)
        } else {
            write!(f, "Failed to read raw file")
        }
    }
}

impl std::error::Error for RawFileError {}
