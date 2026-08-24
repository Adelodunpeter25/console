//! Composer image attachments: staging pasted/dropped/picked images as chips,
//! removing them, and opening the preview modal.

use std::path::Path;

use console_core::ImageAttachment;
use std::rc::Rc;

use console_ui::attachment_image;
use gpui::{ClipboardEntry, Context, ExternalPaths, Window};

use super::ConsoleDesktopApp;

/// Map a file extension to a MIME type for image attachments staged from
/// disk (drops and picks). Unknown extensions are not staged as images.
fn image_mime_from_path(path: &Path) -> Option<&'static str> {
    match path
        .extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| ext.to_ascii_lowercase())
        .as_deref()
    {
        Some("png") => Some("image/png"),
        Some("jpg") | Some("jpeg") => Some("image/jpeg"),
        Some("gif") => Some("image/gif"),
        Some("webp") => Some("image/webp"),
        Some("bmp") => Some("image/bmp"),
        Some("svg") => Some("image/svg+xml"),
        Some("tif") | Some("tiff") => Some("image/tiff"),
        Some("ico") => Some("image/x-icon"),
        Some("avif") => Some("image/avif"),
        Some("heic") | Some("heif") => Some("image/heic"),
        _ => None,
    }
}

/// Read an image file off disk and encode it as a base64 [`ImageAttachment`].
/// Returns `None` for non-image files or unreadable paths.
fn read_image_attachment(path: &Path) -> Option<ImageAttachment> {
    let mime_type = image_mime_from_path(path)?;
    let bytes = std::fs::read(path).ok()?;
    if bytes.is_empty() {
        return None;
    }
    Some(ImageAttachment {
        data: base64::Engine::encode(&base64::engine::general_purpose::STANDARD, bytes),
        mime_type: mime_type.to_string(),
    })
}

/// Stage a pasted clipboard image (raw bytes + format) as an attachment.
fn image_attachment_from_clipboard(image: &gpui::Image) -> ImageAttachment {
    ImageAttachment {
        data: base64::Engine::encode(&base64::engine::general_purpose::STANDARD, &image.bytes),
        mime_type: image.format.mime_type().to_string(),
    }
}

impl ConsoleDesktopApp {
    /// Stage attachments pasted into the composer: clipboard images are
    /// encoded immediately; image files dropped or pasted as paths are read
    /// off disk on the background executor.
    pub fn stage_clipboard_attachments(
        &mut self,
        entries: Vec<ClipboardEntry>,
        cx: &mut Context<Self>,
    ) {
        let mut staged: Vec<ImageAttachment> = Vec::new();
        let mut paths = Vec::new();
        for entry in entries {
            match entry {
                ClipboardEntry::Image(image) if !image.bytes.is_empty() => {
                    staged.push(image_attachment_from_clipboard(&image));
                }
                ClipboardEntry::ExternalPaths(external) => {
                    paths.extend(external.paths().iter().cloned());
                }
                ClipboardEntry::String(_) | ClipboardEntry::Image(_) => {}
            }
        }

        if !staged.is_empty() {
            let pane_id = self.active_pane_id.clone().unwrap_or_else(|| "pane-main".to_string());
            self.append_attachments_for_pane(&pane_id, staged);
            cx.notify();
        }
        if !paths.is_empty() {
            let entity = cx.entity().downgrade();
            cx.spawn(async move |_entity, cx| {
                let attachments = cx
                    .background_executor()
                    .spawn(async move {
                        paths
                            .iter()
                            .filter_map(|path| read_image_attachment(path))
                            .collect::<Vec<_>>()
                    })
                    .await;
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let pane_id = this
                                .active_pane_id
                                .clone()
                                .unwrap_or_else(|| "pane-main".to_string());
                            this.append_attachments_for_pane(&pane_id, attachments);
                            cx.notify();
                        });
                    }
                });
            })
            .detach();
        }
    }

    /// Stage image files dropped onto the composer as attachment chips.
    pub fn stage_dropped_files(
        &mut self,
        paths: &ExternalPaths,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if paths.paths().is_empty() {
            return;
        }
        let paths = paths.paths().to_vec();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            let attachments = cx
                .background_executor()
                .spawn(async move {
                    paths
                        .iter()
                        .filter_map(|path| read_image_attachment(path))
                        .collect::<Vec<_>>()
                })
                .await;
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        let pane_id = this
                            .active_pane_id
                            .clone()
                            .unwrap_or_else(|| "pane-main".to_string());
                        this.append_attachments_for_pane(&pane_id, attachments);
                        cx.notify();
                    });
                }
            });
        })
        .detach();
        window.focus(&self.active_composer_input().read(cx).focus(), cx);
    }

    /// Open the native file picker and stage the chosen image. Dismissing the
    /// dialog is a silent no-op, like the folder picker.
    pub fn pick_image(&mut self, cx: &mut Context<Self>) {
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            let picked = cx
                .background_executor()
                .spawn(async move { crate::picker::pick_image_file_blocking() })
                .await;
            let Some(path) = picked else {
                return;
            };
            if path.trim().is_empty() {
                return;
            }
            let path = std::path::PathBuf::from(path);
            let attachment = cx
                .background_executor()
                .spawn(async move { read_image_attachment(&path) })
                .await;
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| match attachment {
                        Some(attachment) => {
                            let pane_id = this
                                .active_pane_id
                                .clone()
                                .unwrap_or_else(|| "pane-main".to_string());
                            this.append_attachments_for_pane(&pane_id, vec![attachment]);
                            cx.notify();
                        }
                        None => this.set_error("That file is not a supported image.", cx),
                    });
                }
            });
        })
        .detach();
    }

    /// Remove a staged attachment by index.
    pub fn remove_attachment(&mut self, index: usize, cx: &mut Context<Self>) {
        let pane_id = self.active_pane_id.clone().unwrap_or_else(|| "pane-main".to_string());
        if let Some(staged) = self.attachments.get_mut(&pane_id) {
            if index < staged.len() {
                Rc::make_mut(staged).remove(index);
                if staged.is_empty() {
                    self.attachments.remove(&pane_id);
                }
                cx.notify();
            }
        }
    }

    /// Open the image preview modal for a staged attachment.
    pub fn preview_attachment(&mut self, index: usize, cx: &mut Context<Self>) {
        let pane_id = self.active_pane_id.clone().unwrap_or_else(|| "pane-main".to_string());
        if let Some(attachment) = self
            .attachments
            .get(&pane_id)
            .and_then(|v| v.get(index))
            .cloned()
        {
            if let Some(image) = attachment_image(&attachment) {
                self.zoomed_image = Some(image);
                cx.notify();
            }
        }
    }

    /// Open the image preview modal for a decoded message image.
    pub fn preview_image_data(
        &mut self,
        image: std::sync::Arc<gpui::Image>,
        cx: &mut Context<Self>,
    ) {
        self.zoomed_image = Some(image);
        cx.notify();
    }
}
