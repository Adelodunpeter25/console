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
    pub async fn browse(&self, path: Option<&str>) -> Result<FsBrowseResult> {
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

        let body: ApiResponse<FsBrowseResult> = self
            .transport
            .decode_json(resp)
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

        let body: ApiResponse<Vec<FsTreeEntry>> = self
            .transport
            .decode_json(resp)
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
    ) -> Result<FsDirectoryTree> {
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

        let body: ApiResponse<FsDirectoryTree> = self
            .transport
            .decode_json(resp)
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

    pub async fn read_file(&self, path: &str) -> Result<FsFileContent> {
        self.read_file_with_etag(path)
            .await
            .map(|file| file.content)
    }

    /// Read a file together with the ETag it was loaded at.
    ///
    /// The editor records that ETag as its `base_etag`; `POST /api/fs/save`
    /// then refuses a save whose buffer no longer matches the file on disk.
    pub async fn read_file_with_etag(&self, path: &str) -> Result<FsFileWithEtag> {
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

        let etag = resp
            .headers()
            .get(reqwest::header::ETAG)
            .and_then(|value| value.to_str().ok())
            .map(str::to_owned);

        let body: ApiResponse<FsFileContent> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse file response")?;
        let content = if body.success {
            body.data
                .ok_or_else(|| anyhow!("File content data is missing"))?
        } else {
            return Err(anyhow!(
                body.error.unwrap_or_else(|| "Failed to read file".into())
            ));
        };
        Ok(FsFileWithEtag { content, etag })
    }

    /// Write `content` to `path` through `POST /api/fs/save`.
    ///
    /// The desktop client never writes workspace files on its own disk.
    /// `base_etag` is the ETag the buffer was loaded at; when it no longer
    /// matches the file the server writes nothing and this returns
    /// [`FsSaveError::FileChanged`] carrying the current ETag, so the caller
    /// can offer a reload rather than clobbering a concurrent edit.
    pub async fn save_file(
        &self,
        path: &str,
        content: &str,
        base_etag: Option<&str>,
    ) -> std::result::Result<FsSaveOutcome, FsSaveError> {
        let url = self.transport.url("/api/fs/save").await;
        let payload = SaveFileRequest {
            path: path.to_string(),
            content: content.to_string(),
            base_etag: base_etag.map(str::to_owned),
        };

        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .map_err(|err| FsSaveError::Other(format!("Failed to save file: {err}")))?;

        if resp.status() == reqwest::StatusCode::CONFLICT {
            let conflict: SaveConflictEnvelope =
                self.transport.decode_json(resp).await.map_err(|err| {
                    FsSaveError::Other(format!("Failed to parse conflict: {err}"))
                })?;
            return Err(FsSaveError::FileChanged {
                current_etag: conflict.etag,
            });
        }

        let body: ApiResponse<FsSaveOutcome> =
            self.transport.decode_json(resp).await.map_err(|err| {
                FsSaveError::Other(format!("Failed to parse save response: {err}"))
            })?;
        if body.success {
            body.data
                .ok_or_else(|| FsSaveError::Other("Save response data is missing".into()))
        } else {
            Err(FsSaveError::Other(
                body.error.unwrap_or_else(|| "Failed to save file".into()),
            ))
        }
    }

    pub async fn watch_events(
        &self,
        path: &str,
    ) -> Result<std::pin::Pin<Box<dyn futures_util::Stream<Item = Result<FsChangeEvent>> + Send>>>
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

    /// URL for `GET /api/fs/file/media?path=...`, used by embedded players and
    /// webviews that cannot attach the bearer header (WKWebView, HTML media
    /// tags). The token travels in the query string — the server accepts it
    /// for this GET only — so it is URL-encoded here.
    pub async fn media_url(&self, path: &str, auth_token: Option<&str>) -> String {
        let mut url = format!(
            "{}/api/fs/file/media?path={}",
            self.transport.url("").await,
            urlencoding::encode(path)
        );
        if let Some(token) = auth_token.filter(|t| !t.is_empty()) {
            url.push_str("&token=");
            url.push_str(&urlencoding::encode(token));
        }
        url
    }

    /// Probe media file metadata via `GET /api/fs/file/media` with a
    /// `Range: bytes=0-0` request, so headers (size via `Content-Range`,
    /// content type) come back without downloading the body. `None` means the
    /// file is missing, not a media file, or otherwise not servable.
    pub async fn media_file_meta(&self, path: &str) -> Result<Option<MediaFileMeta>> {
        let url = format!(
            "{}/api/fs/file/media?path={}",
            self.transport.url("").await,
            urlencoding::encode(path)
        );
        let resp = self
            .transport
            .client()
            .get(&url)
            .header(reqwest::header::RANGE, "bytes=0-0")
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to probe media file")?;

        let status = resp.status();
        if !status.is_success() {
            if status == reqwest::StatusCode::NOT_FOUND || status == reqwest::StatusCode::BAD_REQUEST
            {
                return Ok(None);
            }
            return Err(anyhow!("Failed to probe media file (status {})", status));
        }

        let content_type = resp
            .headers()
            .get(reqwest::header::CONTENT_TYPE)
            .and_then(|v| v.to_str().ok())
            .unwrap_or("application/octet-stream")
            .to_string();

        let headers = resp.headers().clone();
        // A ranged 206 carries the total in `Content-Range: bytes 0-0/<size>`;
        // an unranged 200 only knows the payload length.
        let size_bytes = headers
            .get(reqwest::header::CONTENT_RANGE)
            .and_then(|v| v.to_str().ok())
            .and_then(|v| v.rsplit('/').next())
            .and_then(|v| v.trim().parse::<u64>().ok())
            .or_else(|| {
                headers
                    .get(reqwest::header::CONTENT_LENGTH)
                    .and_then(|v| v.to_str().ok())
                    .and_then(|v| v.parse::<u64>().ok())
            });

        Ok(Some(MediaFileMeta {
            size_bytes,
            content_type,
        }))
    }

    /// Size of a single file, looked up in its parent directory's entry list
    /// (`GET /api/fs/entries?depth=1`). Used for the details card on file
    /// kinds the media endpoint does not serve. `None` when the listing does
    /// not contain the file — the card then omits the size row.
    pub async fn file_size(&self, path: &str) -> Result<Option<u64>> {
        let Some(parent) = std::path::Path::new(path).parent() else {
            return Ok(None);
        };
        let parent = parent.to_string_lossy().to_string();
        if parent.is_empty() {
            return Ok(None);
        }
        let file_name = std::path::Path::new(path)
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or(path)
            .to_string();
        let mut url = self.transport.url("/api/fs/entries").await;
        url.push_str(&format!(
            "?path={}&depth=1&maxEntries=5000",
            urlencoding::encode(&parent)
        ));
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list directory for file size")?;
        let body: ApiResponse<Vec<FsTreeEntry>> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse fs entries response")?;
        let entries = if body.success {
            body.data.unwrap_or_default()
        } else {
            return Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to list directory".into())
            ));
        };
        Ok(entries
            .iter()
            .find(|e| e.name == file_name || e.path == path)
            .and_then(|e| e.size))
    }
    /// Content search across a workspace root for the global search panel
    /// (⌘⇧F). Requires the server's fff index; returns an error (surfaced as
    /// an empty/unavailable state by the caller) when the backend responds
    /// with 503 because the native library isn't wired in for this build.
    pub async fn grep(&self, root: &str, query: &str, options: &GrepOptions) -> Result<GrepResult> {
        let mut url = self.transport.url("/api/fs/grep").await;
        let mut params = vec![
            format!("root={}", urlencoding::encode(root)),
            format!("q={}", urlencoding::encode(query)),
            format!("mode={}", options.mode.as_query_value()),
            format!("caseMode={}", options.case_mode.as_query_value()),
            format!("wholeWord={}", options.whole_word),
        ];
        if let Some(c) = options.context_lines {
            params.push(format!("contextLines={}", c));
        }
        if let Some(m) = options.max_matches {
            params.push(format!("limit={}", m));
        }
        if let Some(c) = options.cursor {
            params.push(format!("cursor={}", c));
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
            .context("Failed to search file contents")?;

        let body: ApiResponse<GrepResult> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse grep response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Grep result data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to search file contents".into())
            ))
        }
    }
}
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
