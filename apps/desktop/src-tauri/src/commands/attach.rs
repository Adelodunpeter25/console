use base64::Engine;
use serde::Serialize;

use crate::error::AppResult;

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PickedImage {
    pub name: String,
    pub data: String,
    pub mime_type: String,
}

/// Open a native multi-select file dialog restricted to images, read the
/// selected files, and return them base64-encoded for inline attachment.
#[tauri::command]
pub async fn pick_images() -> AppResult<Vec<PickedImage>> {
    let files = rfd::AsyncFileDialog::new()
        .set_title("Attach image(s)")
        .add_filter("Images", &["png", "jpg", "jpeg", "gif", "webp", "bmp", "svg"])
        .pick_files()
        .await;

    let Some(files) = files else {
        return Ok(Vec::new());
    };

    let mut images = Vec::new();
    for file in files {
        let bytes = file.read().await;
        let name = file.file_name();
        let ext = name
            .rsplit('.')
            .next()
            .map(|e| e.to_lowercase())
            .unwrap_or_default();
        let mime = match ext.as_str() {
            "png" => "image/png",
            "jpg" | "jpeg" => "image/jpeg",
            "gif" => "image/gif",
            "webp" => "image/webp",
            "bmp" => "image/bmp",
            "svg" => "image/svg+xml",
            _ => "application/octet-stream",
        };
        images.push(PickedImage {
            name,
            data: base64::engine::general_purpose::STANDARD.encode(bytes),
            mime_type: mime.to_string(),
        });
    }

    Ok(images)
}
