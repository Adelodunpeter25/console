//! File classification helper for desktop preview.
//!
//! Classifies paths into `FileKind` (Markdown, Svg, RasterImage, Video, Audio,
//! Pdf, Details, Text, Blocked) to route to the appropriate viewer and
//! network fetch path. Extension lists are mirrored exactly in
//! `apps/server-go/internal/services/fs/preview.go` and
//! `apps/android/.../data/model/FsModels.kt` — keep all three in sync.

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum FileKind {
    Markdown,
    Svg,
    RasterImage,
    Video,
    Audio,
    Pdf,
    /// Opaque binary (archives, fonts, executables, databases): renders a
    /// metadata card instead of file contents.
    Details,
    Text,
    /// Lockfiles only — generated files that must never be previewed.
    Blocked,
}

/// Raster image extensions that can be previewed as images.
/// Kept in sync with `@console/types` `IMAGE_PREVIEW_EXTENSIONS`.
pub const RASTER_IMAGE_EXTENSIONS: &[&str] =
    &[".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico"];

/// Video extensions served by the media endpoint.
pub const VIDEO_EXTENSIONS: &[&str] = &[".mp4", ".mov", ".avi", ".mkv", ".webm"];

/// Audio extensions served by the media endpoint.
pub const AUDIO_EXTENSIONS: &[&str] = &[".mp3", ".wav", ".flac", ".ogg", ".m4a"];

/// PDF documents served by the media endpoint.
pub const PDF_EXTENSIONS: &[&str] = &[".pdf"];

/// Non-image binary formats with no inline viewer: they get a details card.
pub const DETAILS_EXTENSIONS: &[&str] = &[
    ".icns", ".tiff",
    // Archives
    ".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar", ".jar", ".war",
    // Executables / objects / disk images
    ".exe", ".dll", ".so", ".dylib", ".a", ".o", ".obj", ".bin", ".iso", ".dmg", ".pkg", ".deb",
    ".rpm", ".apk", ".ipa", ".node",
    // Fonts
    ".ttf", ".otf", ".woff", ".woff2", ".eot",
    // Documents / other opaque formats
    ".wasm", ".psd", ".sketch", ".class", ".pyc", ".db", ".sqlite", ".sqlite3",
];

/// Backward-compatible name for [`DETAILS_EXTENSIONS`]: the former
/// `BLOCKED_FILE_EXTENSIONS` now covers only opaque formats that get the
/// details card; `Blocked` itself is reserved for lockfiles.
pub const BLOCKED_FILE_EXTENSIONS: &[&str] = DETAILS_EXTENSIONS;

/// Markdown extensions matching `workspace_content.rs`.
pub const MARKDOWN_EXTENSIONS: &[&str] = &[".md", ".markdown", ".mdx"];

const LOCK_FILE_BASENAMES: &[&str] = &[
    "package-lock.json",
    "npm-shrinkwrap.json",
    "pnpm-lock.yaml",
    "composer.lock",
];

const LOCK_FILE_SUFFIXES: &[&str] = &[".lock", ".lockb"];

/// Check if a filename represents a generated lockfile.
pub fn is_lock_file(file_name: &str) -> bool {
    let lower = file_name.to_lowercase();
    if LOCK_FILE_BASENAMES.iter().any(|&b| lower == b) {
        return true;
    }
    LOCK_FILE_SUFFIXES
        .iter()
        .any(|&suffix| lower.ends_with(suffix))
}

/// Classify a file path by extension / filename into a `FileKind`.
pub fn file_kind_for_path(path: &str) -> FileKind {
    let file_name = std::path::Path::new(path)
        .file_name()
        .and_then(|n| n.to_str())
        .unwrap_or(path);

    if is_lock_file(file_name) {
        return FileKind::Blocked;
    }

    let lower = file_name.to_lowercase();

    if lower.ends_with(".svg") {
        return FileKind::Svg;
    }

    if RASTER_IMAGE_EXTENSIONS
        .iter()
        .any(|&ext| lower.ends_with(ext))
    {
        return FileKind::RasterImage;
    }

    if MARKDOWN_EXTENSIONS.iter().any(|&ext| lower.ends_with(ext)) {
        return FileKind::Markdown;
    }

    if VIDEO_EXTENSIONS.iter().any(|&ext| lower.ends_with(ext)) {
        return FileKind::Video;
    }

    if AUDIO_EXTENSIONS.iter().any(|&ext| lower.ends_with(ext)) {
        return FileKind::Audio;
    }

    if PDF_EXTENSIONS.iter().any(|&ext| lower.ends_with(ext)) {
        return FileKind::Pdf;
    }

    if DETAILS_EXTENSIONS.iter().any(|&ext| lower.ends_with(ext)) {
        return FileKind::Details;
    }

    FileKind::Text
}
