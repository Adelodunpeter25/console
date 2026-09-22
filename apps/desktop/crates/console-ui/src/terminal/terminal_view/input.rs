//! Keyboard mapping, paste, and file-drop input for the terminal.
use gpui::{Context, KeyDownEvent, Window};
use termy_core::{
    TerminalKeyEventKind, TerminalKeyboardMode, TermyKeystroke, TermyModifiers, keystroke_to_input,
};

use super::TerminalView;

impl TerminalView {
    pub(super) fn key_to_bytes(event: &KeyDownEvent, mode: TerminalKeyboardMode) -> Option<String> {
        Self::keystroke_to_bytes(
            event.keystroke.key.as_str(),
            event.keystroke.key_char.clone(),
            &event.keystroke.modifiers,
            mode,
        )
    }

    /// Mapping core shared by the `on_key_down` path and the Tab actions
    /// (which never see the original key event).
    fn keystroke_to_bytes(
        key: &str,
        key_char: Option<String>,
        modifiers: &gpui::Modifiers,
        mode: TerminalKeyboardMode,
    ) -> Option<String> {
        let mods = TermyModifiers {
            control: modifiers.control,
            alt: modifiers.alt,
            shift: modifiers.shift,
            platform: modifiers.platform,
            function: modifiers.function,
        };
        let ks = TermyKeystroke {
            key: key.to_owned(),
            key_char,
            modifiers: mods,
        };
        let bytes = keystroke_to_input(&ks, TerminalKeyEventKind::Press, mode, true)?;
        Some(String::from_utf8_lossy(&bytes).into_owned())
    }

    /// Send Tab / Shift-Tab from the keymap actions. The binding identity
    /// carries the key; live modifiers are sampled synchronously during
    /// dispatch, so this reconstructs exactly what `on_key_down` would see.
    /// Stops propagation so the global focus-traversal binding never fires
    /// while the terminal is focused.
    fn send_tab(&mut self, shift: bool, window: &mut Window, cx: &mut Context<Self>) {
        let mut modifiers = window.modifiers();
        modifiers.shift = shift;
        let mode = self
            .snapshot
            .as_ref()
            .map(|s| s.keyboard_mode)
            .unwrap_or_default();
        match Self::keystroke_to_bytes("tab", Some("\t".to_owned()), &modifiers, mode) {
            Some(bytes) => {
                log::debug!("terminal tab action -> {} bytes to pty", bytes.len());
                self.send_input(bytes);
                if self.clear_selection() {
                    cx.notify();
                }
                cx.stop_propagation();
            }
            None => {
                log::debug!("terminal tab action swallowed: no bytes produced");
            }
        }
    }
}

/// Quote a path for pasting into a shell: `'...'` with embedded quotes
/// escaped. Mirrors termy's `shell_quote_path`.
fn shell_quote_path(path: &std::path::Path) -> String {
    let s = path.to_string_lossy();
    let mut quoted = String::with_capacity(s.len() + 2);
    quoted.push('\'');
    quoted.push_str(&s.replace('\'', "'\\''"));
    quoted.push('\'');
    quoted
}

/// Dropped files become space-joined quoted paths plus a trailing space,
/// ready to type at the prompt. Mirrors termy's drop input.
pub(super) fn dropped_paths_input(paths: &[std::path::PathBuf]) -> Option<String> {
    if paths.is_empty() {
        return None;
    }
    let mut text = paths
        .iter()
        .map(|p| shell_quote_path(p))
        .collect::<Vec<_>>()
        .join(" ");
    text.push(' ');
    Some(text)
}

fn clipboard_image_extension(format: gpui::ImageFormat) -> &'static str {
    match format {
        gpui::ImageFormat::Png => "png",
        gpui::ImageFormat::Jpeg => "jpg",
        gpui::ImageFormat::Webp => "webp",
        gpui::ImageFormat::Gif => "gif",
        gpui::ImageFormat::Svg => "svg",
        gpui::ImageFormat::Bmp => "bmp",
        gpui::ImageFormat::Tiff => "tiff",
        gpui::ImageFormat::Ico => "ico",
        gpui::ImageFormat::Pnm => "pnm",
    }
}

fn write_clipboard_image_to_temp_file(image: &gpui::Image) -> std::io::Result<std::path::PathBuf> {
    let dir = std::env::temp_dir().join("console-clipboard-images");
    std::fs::create_dir_all(&dir)?;
    let path = dir.join(format!(
        "clipboard-image-{}.{}",
        image.id(),
        clipboard_image_extension(image.format)
    ));
    if !path.exists() {
        std::fs::write(&path, &image.bytes)?;
    }
    Ok(path)
}

/// Paste bytes for a clipboard item. Finder file copies become quoted paths
/// (safer than the raw text fallback for paths with spaces); plain text is
/// bracketed-paste framed when the shell opted in; bare images are staged to
/// a temp file and pasted as a quoted path.
fn paste_input_for_clipboard(
    item: &gpui::ClipboardItem,
    bracketed_paste: bool,
) -> Option<String> {
    let dropped: Vec<std::path::PathBuf> = item
        .entries()
        .iter()
        .filter_map(|entry| match entry {
            gpui::ClipboardEntry::ExternalPaths(paths) => Some(paths.paths().iter().cloned()),
            _ => None,
        })
        .flatten()
        .collect();
    if let Some(text) = dropped_paths_input(&dropped) {
        return Some(text);
    }
    if let Some(text) = item.text() {
        if bracketed_paste {
            return Some(format!("\x1b[200~{text}\x1b[201~"));
        }
        return Some(text);
    }
    let image = item.entries().iter().find_map(|entry| match entry {
        gpui::ClipboardEntry::Image(image) => Some(image),
        _ => None,
    })?;
    write_clipboard_image_to_temp_file(image)
        .ok()
        .map(|path| shell_quote_path(&path))
}
