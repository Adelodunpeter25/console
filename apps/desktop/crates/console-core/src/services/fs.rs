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

    pub async fn browse(&self, path: Option<&str>) -> Result<BrowseDirectoryResponse> {
        let mut url = self.transport.url("/api/fs/browse").await;
        if let Some(p) = path {
            url.push_str(&format!("?path={}", urlencoding::encode(p)));
        }

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
}
