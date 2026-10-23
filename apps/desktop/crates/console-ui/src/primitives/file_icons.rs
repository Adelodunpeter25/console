//! Filename and path → file-type icon resolution.
//!
//! *Which* icon a name gets is decided in `console_shared::icons`, the same
//! table the Android app uses, so a file looks the same on both. This module
//! only turns that key into an embedded asset path and renders it.
//!
//! The resolved path feeds [`file_type_icon`], which renders the SVG as an
//! image so its authored colors survive — these are multicolor brand marks,
//! unlike the monochrome `icon()` set that is tinted via text color.

use std::path::Path;

use gpui::Img;

use crate::primitives::file_icon;
use crate::primitives::icons::FileTypeIcon;

fn asset_for_key(key: &str) -> &'static str {
    FileTypeIcon::from_key(key)
        .unwrap_or(FileTypeIcon::File)
        .path()
}

/// Resolve a filename to its file-type icon asset path.
pub fn file_icon_for_name(name: &str) -> &'static str {
    asset_for_key(console_shared::icons::file_icon_key(name))
}

/// Highlight language for an extension (the one thing the icon table used to
/// carry that Android has no use for).
fn extension_lang(ext: &str) -> Option<&'static str> {
    Some(match ext {
        "rs" => "rust",
        "js" => "javascript",
        "mjs" => "javascript",
        "cjs" => "javascript",
        "ts" => "typescript",
        "mts" => "typescript",
        "cts" => "typescript",
        "jsx" => "javascript",
        "tsx" => "typescript",
        "py" => "python",
        "pyi" => "python",
        "pyw" => "python",
        "go" => "go",
        "c" => "c",
        "h" => "c",
        "m" => "c",
        "cc" => "cpp",
        "cpp" => "cpp",
        "cxx" => "cpp",
        "hh" => "cpp",
        "hpp" => "cpp",
        "hxx" => "cpp",
        "mm" => "cpp",
        "cs" => "csharp",
        "swift" => "swift",
        "kt" => "kotlin",
        "kts" => "kotlin",
        "java" => "java",
        "class" => "java",
        "rb" => "ruby",
        "php" => "php",
        "html" => "html",
        "htm" => "html",
        "css" => "css",
        "less" => "css",
        "scss" => "scss",
        "sass" => "scss",
        "json" => "json",
        "jsonc" => "json",
        "jsonl" => "json",
        "yaml" => "yaml",
        "yml" => "yaml",
        "toml" => "toml",
        "ini" => "toml",
        "cfg" => "toml",
        "conf" => "toml",
        "config" => "toml",
        "xml" => "xml",
        "xsl" => "xml",
        "plist" => "xml",
        "md" => "markdown",
        "mdx" => "markdown",
        "markdown" => "markdown",
        "sh" => "bash",
        "bash" => "bash",
        "zsh" => "bash",
        "fish" => "bash",
        "sql" => "sql",
        "svelte" => "html",
        "vue" => "html",
        "graphql" => "graphql",
        "gql" => "graphql",
        "diff" => "diff",
        "patch" => "diff",
        _ => return None,
    })
}

/// Language tag for a file path, for syntax highlighting.
pub fn lang_tag_for_path(path: &str) -> Option<&'static str> {
    let basename = path.rsplit(['/', '\\']).next().unwrap_or(path);
    match basename.to_ascii_lowercase().as_str() {
        "dockerfile" => return Some("dockerfile"),
        "makefile" => return Some("makefile"),
        _ => {}
    }
    let ext = Path::new(basename)
        .extension()
        .and_then(|e| e.to_str())
        .unwrap_or("")
        .to_ascii_lowercase();
    extension_lang(&ext)
}

/// Resolve a full path to its file-type icon asset path, using the basename.
pub fn file_icon_for_path(path: &str) -> &'static str {
    let name = Path::new(path)
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or(path);
    file_icon_for_name(name)
}

/// Render the multicolor file-type icon for a filename or path.
pub fn file_type_icon(path_or_name: &str, size: f32) -> Img {
    file_icon(file_icon_for_path(path_or_name), size)
}

/// Resolve a fenced-code language tag (e.g. "ts", "python") to its file-type icon.
/// Returns `None` for unknown/empty tags so the caller can fall back to a
/// text-only header.
pub fn file_icon_for_language(tag: &str) -> Option<&'static str> {
    console_shared::icons::language_icon_key(tag).map(asset_for_key)
}

/// Basename of a path, handling both separator styles.
pub fn base_name(path: &str) -> &str {
    path.rsplit(['/', '\\']).next().unwrap_or(path)
}
