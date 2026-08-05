use base64::Engine;
use serde::Serialize;
use std::path::Path;

use crate::error::AppResult;

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PickedImage {
    pub name: String,
    pub data: String,
    pub mime_type: String,
}

fn image_from_path(path: &Path) -> AppResult<Option<PickedImage>> {
    let name = path
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or_default()
        .to_string();
    let ext = path
        .extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| ext.to_lowercase())
        .unwrap_or_default();
    let mime = match ext.as_str() {
        "png" => "image/png",
        "jpg" | "jpeg" => "image/jpeg",
        "gif" => "image/gif",
        "webp" => "image/webp",
        "bmp" => "image/bmp",
        "svg" => "image/svg+xml",
        _ => return Ok(None),
    };

    let bytes = std::fs::read(path)?;
    Ok(Some(PickedImage {
        name,
        data: base64::engine::general_purpose::STANDARD.encode(bytes),
        mime_type: mime.to_string(),
    }))
}

#[tauri::command]
pub async fn read_dropped_images(paths: Vec<String>) -> AppResult<Vec<PickedImage>> {
    paths
        .iter()
        .map(|path| image_from_path(Path::new(path)))
        .collect::<AppResult<Vec<_>>>()
        .map(|images| images.into_iter().flatten().collect())
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
