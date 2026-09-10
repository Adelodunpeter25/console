//! File classification helper for desktop preview.
//!
//! Classifies paths into `FileKind` (Markdown, Svg, RasterImage, Text, Blocked)
//! to route to the appropriate viewer and network fetch path.

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum FileKind {
    Markdown,
    Svg,
    RasterImage,
    Text,
    Blocked,
}

/// Raster image extensions that can be previewed as images.
/// Kept in sync with `@console/types` `IMAGE_PREVIEW_EXTENSIONS`.
pub const RASTER_IMAGE_EXTENSIONS: &[&str] = &[
    ".png",
    ".jpg",
    ".jpeg",
    ".gif",
    ".webp",
    ".bmp",
    ".ico",
];

/// Markdown extensions matching `workspace_content.rs`.
pub const MARKDOWN_EXTENSIONS: &[&str] = &[
    ".md",
    ".markdown",
    ".mdx",
];

/// Non-image binary formats that cannot be rendered as text or images.
pub const BLOCKED_FILE_EXTENSIONS: &[&str] = &[
    // Video / audio
    ".mp4", ".mov", ".avi", ".mkv", ".webm", ".mp3", ".wav", ".flac", ".ogg", ".m4a",
    // Archives
    ".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar", ".jar", ".war",
    // Executables / objects
    ".exe", ".dll", ".so", ".dylib", ".a", ".o", ".obj", ".bin", ".iso", ".dmg",
    ".pkg", ".deb", ".rpm", ".apk", ".ipa", ".node",
    // Fonts
    ".ttf", ".otf", ".woff", ".woff2", ".eot",
    // Documents / other opaque formats
    ".pdf", ".wasm", ".psd", ".sketch", ".class", ".pyc", ".db", ".sqlite", ".sqlite3",
    ".icns", ".tiff",
];

const LOCK_FILE_BASENAMES: &[&str] = &[
    "package-lock.json",
    "npm-shrinkwrap.json",
    "pnpm-lock.yaml",
    "composer.lock",
];

const LOCK_FILE_SUFFIXES: &[&str] = &[
    ".lock",
    ".lockb",
];

/// Check if a filename represents a generated lockfile.
pub fn is_lock_file(file_name: &str) -> bool {
    let lower = file_name.to_lowercase();
    if LOCK_FILE_BASENAMES.iter().any(|&b| lower == b) {
        return true;
    }
    LOCK_FILE_SUFFIXES.iter().any(|&suffix| lower.ends_with(suffix))
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

    if RASTER_IMAGE_EXTENSIONS.iter().any(|&ext| lower.ends_with(ext)) {
        return FileKind::RasterImage;
    }

    if MARKDOWN_EXTENSIONS.iter().any(|&ext| lower.ends_with(ext)) {
        return FileKind::Markdown;
    }

    if BLOCKED_FILE_EXTENSIONS.iter().any(|&ext| lower.ends_with(ext)) {
        return FileKind::Blocked;
    }

    FileKind::Text
}
