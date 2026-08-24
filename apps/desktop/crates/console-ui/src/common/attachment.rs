//! Helpers for turning base64 [`ImageAttachment`]s into renderable images.
//!
//! gpui's `img()` element loads strings as *resources* — a `data:` URI is
//! treated as a URL and fetched over HTTP, so it never renders. Images must
//! be decoded into `Arc<gpui::Image>` bytes first.

use std::sync::Arc;

use base64::Engine as _;
use console_core::ImageAttachment;

/// Decode an attachment's base64 payload into a renderable gpui image.
/// Returns `None` when the MIME type is not a supported image format or the
/// payload is empty/undecodable.
pub fn attachment_image(attachment: &ImageAttachment) -> Option<Arc<gpui::Image>> {
    let format = gpui::ImageFormat::from_mime_type(&attachment.mime_type)?;
    let bytes = base64::engine::general_purpose::STANDARD
        .decode(&attachment.data)
        .ok()?;
    (!bytes.is_empty()).then(|| Arc::new(gpui::Image::from_bytes(format, bytes)))
}
