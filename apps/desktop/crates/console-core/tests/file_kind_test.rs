//! Unit tests for FileKind classification and parity with TypeScript definitions.

use console_core::utils::file_kind::{
    file_kind_for_path, is_lock_file, FileKind, BLOCKED_FILE_EXTENSIONS, MARKDOWN_EXTENSIONS,
    RASTER_IMAGE_EXTENSIONS,
};

#[test]
fn test_raster_image_extensions_parity_with_typescript() {
    // Exact list from packages/types/src/fs.ts (IMAGE_PREVIEW_EXTENSIONS)
    let ts_expected = [
        ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico",
    ];
    assert_eq!(RASTER_IMAGE_EXTENSIONS, ts_expected);
}

#[test]
fn test_raster_image_classification() {
    for ext in RASTER_IMAGE_EXTENSIONS {
        let path = format!("assets/icon{}", ext);
        assert_eq!(
            file_kind_for_path(&path),
            FileKind::RasterImage,
            "failed for {}",
            path
        );
    }
}

#[test]
fn test_svg_classification() {
    assert_eq!(file_kind_for_path("logo.svg"), FileKind::Svg);
    assert_eq!(file_kind_for_path("path/to/graphic.SVG"), FileKind::Svg);
}

#[test]
fn test_markdown_classification() {
    for ext in MARKDOWN_EXTENSIONS {
        let path = format!("docs/readme{}", ext);
        assert_eq!(
            file_kind_for_path(&path),
            FileKind::Markdown,
            "failed for {}",
            path
        );
    }
}

#[test]
fn test_blocked_binary_classification() {
    for ext in BLOCKED_FILE_EXTENSIONS {
        let path = format!("data/archive{}", ext);
        assert_eq!(
            file_kind_for_path(&path),
            FileKind::Blocked,
            "failed for {}",
            path
        );
    }
}

#[test]
fn test_lockfiles_classification() {
    let lockfiles = [
        "Cargo.lock",
        "bun.lockb",
        "yarn.lock",
        "package-lock.json",
        "pnpm-lock.yaml",
        "composer.lock",
        "npm-shrinkwrap.json",
    ];
    for name in lockfiles {
        let path = format!("/project/{}", name);
        assert!(is_lock_file(name), "is_lock_file failed for {}", name);
        assert_eq!(
            file_kind_for_path(&path),
            FileKind::Blocked,
            "failed for {}",
            path
        );
    }
}

#[test]
fn test_text_and_code_classification() {
    let text_files = [
        "src/main.rs",
        "index.ts",
        "style.css",
        "data.json",
        "Config.toml",
        "schema.yaml",
        "notes.txt",
    ];
    for path in text_files {
        assert_eq!(
            file_kind_for_path(path),
            FileKind::Text,
            "failed for {}",
            path
        );
    }
}

#[test]
fn test_no_extension_files() {
    let no_ext = ["Makefile", "Dockerfile", "LICENSE", ".gitignore", "README"];
    for name in no_ext {
        assert_eq!(
            file_kind_for_path(name),
            FileKind::Text,
            "failed for {}",
            name
        );
    }
}

#[test]
fn test_case_insensitivity() {
    assert_eq!(file_kind_for_path("PHOTO.PNG"), FileKind::RasterImage);
    assert_eq!(file_kind_for_path("PHOTO.JPEG"), FileKind::RasterImage);
    assert_eq!(file_kind_for_path("VECTOR.SVG"), FileKind::Svg);
    assert_eq!(file_kind_for_path("README.MD"), FileKind::Markdown);
    assert_eq!(file_kind_for_path("ARCHIVE.ZIP"), FileKind::Blocked);
    assert_eq!(file_kind_for_path("DOCUMENT.PDF"), FileKind::Blocked);
    assert_eq!(file_kind_for_path("CARGO.LOCK"), FileKind::Blocked);
    assert_eq!(file_kind_for_path("PACKAGE-LOCK.JSON"), FileKind::Blocked);
}
